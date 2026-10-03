//go:build integration

// Package e2e runs the real identity-service handler in-process against the
// local stack (Postgres, Kratos, Keto, Mailpit). Kratos itself calls the
// webhooks of the identity-service container (compose), so `make up` must be
// running for the after-login population guard.
package e2e

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/adapter/httpapi"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/adapter/hydra"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/adapter/keto"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/adapter/kratos"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/adapter/mailer"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/adapter/openbao"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/adapter/postgres"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/identity"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/platform"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/testutil/itest"
)

const webOrigin = "http://localhost:5173"

type stack struct {
	kadmin   *kratos.Admin
	env      itest.Env
	srv      *httptest.Server
	verifier *kratos.SessionVerifier
	keto     *keto.Client
	admins   *app.AdminService
	store    *postgres.Store
	pool     *pgxpool.Pool
	bao      *openbao.Client
	pii      *app.PersonalInfoService
	logs     *syncBuffer
	// Machine plane (Hydra).
	hydra   *hydra.Admin
	machine *hydra.Verifier
}

// syncBuffer captures the service logs of one stack (PII-NFR-04).
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

type stackOpts struct {
	// grace shrinks the MFA enrolment grace. Only app.AdminGate.Check is
	// affected (never the sweeper, which would touch other admins in the
	// shared stack).
	grace time.Duration
	// baoAddr overrides the OpenBao address (e.g. an unreachable one).
	baoAddr string
}

func newStack(t *testing.T) *stack { return newStackWith(t, stackOpts{}) }

func newStackWithGrace(t *testing.T, grace time.Duration) *stack {
	return newStackWith(t, stackOpts{grace: grace})
}

func newStackWith(t *testing.T, o stackOpts) *stack {
	t.Helper()
	grace := o.grace
	env := itest.Load()
	env.RequireBaoToken(t)
	ctx := context.Background()
	logs := &syncBuffer{}
	log := platform.NewLogger(logs, "debug")
	if err := postgres.MigrateUp(ctx, env.MigrateURL, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatal(err)
	}
	pool, err := postgres.Open(ctx, env.DatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	store := postgres.NewStore(pool)
	repos := store.Repos()
	verifier := kratos.NewSessionVerifier(env.KratosPublic, nil, nil)
	kadmin := kratos.NewAdmin(env.KratosAdmin, nil)
	k := keto.New(env.KetoRead, env.KetoWrite, nil)
	authz := app.MemoAuthorizer{Next: k}
	mail, err := mailer.New(env.SMTPURL, "no-reply@go-ory-auth-example.local")
	if err != nil {
		t.Fatal(err)
	}
	admins := &app.AdminService{Authz: authz, Roles: k, Identities: kadmin, Tx: store, Idempotency: repos.Idempotency,
		Mailer: mail, Clock: app.SystemClock{}, Log: log}
	baoAddr := env.OpenBaoAddr
	if o.baoAddr != "" {
		baoAddr = o.baoAddr
	}
	bao, err := openbao.New(openbao.Config{Addr: baoAddr, TokenFile: env.OpenBaoTokenFile, Timeout: time.Second, Log: log})
	if err != nil {
		t.Fatal(err)
	}
	pi := &app.PersonalInfoService{
		Authz: authz, Identities: kadmin, Keys: bao, Cache: app.NewDEKCache(100, time.Minute, nil), Tx: store,
		SubjectKeys: repos.SubjectKeys, Records: repos.PersonalInfo, Clock: app.SystemClock{}, Log: log,
		LookupLimiter: app.NewRateLimiter(app.LookupRateRules, nil), RevealLimiter: app.NewRateLimiter(app.RevealRateRules, nil),
	}
	hadmin, err := hydra.NewAdmin(env.HydraAdmin, "identity-service", env.ClientTagKey, nil)
	if err != nil {
		t.Fatal(err)
	}
	mverifier, err := hydra.NewVerifier(hydra.VerifierConfig{
		JWKSURL: env.JWKSURL(), Issuer: env.HydraIssuer, Audience: "identity-service", Admin: hadmin, Log: log,
	})
	if err != nil {
		t.Fatal(err)
	}
	server := &httpapi.Server{
		Machine: &app.MachineService{Identities: kadmin, Audit: repos.Audit},
		ServiceClients: &app.ServiceClientService{Authz: authz, Clients: hadmin, Verifier: mverifier, Tx: store,
			Idempotency: repos.Idempotency, Clock: app.SystemClock{}, Log: log},
		Me:           &app.MeService{Profiles: repos.Profiles},
		Customers:    &app.CustomerService{Authz: authz, Identities: kadmin, Profiles: repos.Profiles, Tx: store, Sessions: verifier, Log: log},
		Admins:       admins,
		Audit:        &app.AuditService{Authz: authz, Audit: repos.Audit},
		PersonalInfo: pi,
	}
	h, err := httpapi.NewPublicHandler(httpapi.PublicDeps{
		Log: log, Metrics: httpapi.NewMetrics(), Server: server, Verifier: verifier, Authz: authz,
		Gate:            &app.AdminGate{Identities: kadmin, Sessions: verifier, Tx: store, Clock: app.SystemClock{}, Log: log, MFAGrace: grace},
		AllowedOrigins:  []string{webOrigin},
		MachineVerifier: mverifier,
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &stack{kadmin: kadmin, env: env, srv: srv, verifier: verifier, keto: k, admins: admins,
		store: store, pool: pool, bao: bao, pii: pi, logs: logs, hydra: hadmin, machine: mverifier}
}

type call struct {
	bearer, cookie, origin, idemKey string
}

func (s *stack) api(t *testing.T, method, path string, c call, body, out any) int {
	t.Helper()
	h := http.Header{}
	if c.bearer != "" {
		h.Set("Authorization", "Bearer "+c.bearer)
	}
	if c.cookie != "" {
		h.Set("Cookie", "ory_kratos_session="+c.cookie)
	}
	if c.origin != "" {
		h.Set("Origin", c.origin)
	}
	if c.idemKey != "" {
		h.Set("Idempotency-Key", c.idemKey)
	}
	st, _ := itest.JSON(t, nil, method, s.srv.URL+path, h, body, out)
	return st
}

type problem struct {
	Code string `json:"code"`
}

func hasMessage(f itest.Flow, id int) bool {
	for _, m := range f.UI.Messages {
		if m.ID == id {
			return true
		}
	}
	return false
}

// TestFR08_E2E_CustomerPlane: native registration → bearer on /v1 → admin
// plane rejects the customer credential; population guard on browser login.
func TestFR08_E2E_CustomerPlane(t *testing.T) {
	s := newStack(t)
	email := itest.UniqueEmail("e2e-cust")
	reg := s.env.RegisterCustomer(t, email)
	custID := uuid.MustParse(reg.Session.Identity.ID)
	t.Cleanup(func() { s.env.DeleteIdentity(t, custID) })
	tok := reg.SessionToken

	var me struct {
		ID            uuid.UUID `json:"id"`
		Email         string    `json:"email"`
		EmailVerified bool      `json:"email_verified"`
		Locale        string    `json:"locale"`
		DisplayName   *string   `json:"display_name"`
	}
	if st := s.api(t, "GET", "/v1/me", call{bearer: tok}, nil, &me); st != 200 || me.ID != custID || me.Email != email || me.EmailVerified || me.Locale == "" {
		t.Fatalf("GET /v1/me: %d %+v", st, me)
	}
	var p problem
	if st := s.api(t, "PATCH", "/v1/me", call{bearer: tok}, map[string]any{"display_name": "E2E"}, &p); st != 403 || p.Code != "email_not_verified" {
		t.Fatalf("PATCH unverified: %d %s", st, p.Code)
	}

	// Customer credential on the admin plane: as bearer and as cookie.
	if st := s.api(t, "GET", "/admin/v1/me", call{bearer: tok}, nil, &p); st != 401 || p.Code != "unauthenticated" {
		t.Fatalf("bearer on admin plane: %d %s", st, p.Code)
	}
	if st := s.api(t, "GET", "/admin/v1/me", call{cookie: tok}, nil, &p); st != 401 {
		t.Fatalf("token as cookie on admin plane: %d", st)
	}

	// Verify the email with the emailed code, then PATCH succeeds.
	s.env.VerifyEmail(t, email)
	s.verifier.Invalidate(custID)
	if st := s.api(t, "PATCH", "/v1/me", call{bearer: tok}, map[string]any{"display_name": "E2E", "locale": "en-US"}, &me); st != 200 ||
		me.DisplayName == nil || *me.DisplayName != "E2E" || me.Locale != "en-US" || !me.EmailVerified {
		t.Fatalf("PATCH verified: %d %+v", st, me)
	}

	// Spike S3: customer on a browser login flow is interrupted by the webhook.
	b := s.env.NewBrowser()
	st, f := b.Login(t, email, "")
	if st != 400 || !hasMessage(f, httpapi.LoginInterruptMessageID) || b.SessionCookie() != "" {
		t.Fatalf("customer browser login must be interrupted: %d %+v cookie=%v", st, f.UI.Messages, b.SessionCookie() != "")
	}
	// ...while the API flow works.
	if st, f := s.env.LoginAPI(t, email); st != 200 || f.SessionToken == "" {
		t.Fatalf("customer api login: %d", st)
	}
}

// TestFR06_E2E_AdminPlane: admin browser login → MFA enrolment → AAL2 →
// permissions → customer management, invitation, role change, audit.
func TestFR06_E2E_AdminPlane(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()
	email := itest.UniqueEmail("e2e-admin")
	adminID := s.env.CreateAdminWithPassword(t, email)
	t.Cleanup(func() {
		_ = s.keto.RemoveAll(context.Background(), adminID)
		s.env.DeleteIdentity(t, adminID)
	})
	if err := s.keto.SetRole(ctx, adminID, identity.RoleSuperAdmin); err != nil {
		t.Fatal(err)
	}

	// Spike S3: admin on an API (native) flow is interrupted.
	if st, f := s.env.LoginAPI(t, email); st != 400 || !hasMessage(f, httpapi.LoginInterruptMessageID) || f.SessionToken != "" {
		t.Fatalf("admin api login must be interrupted: %d %+v", st, f.UI.Messages)
	}

	b := s.env.NewBrowser()
	if st, _ := b.Login(t, email, ""); st != 200 || b.SessionCookie() == "" {
		t.Fatalf("admin browser login: %d", st)
	}
	var p problem
	if st := s.api(t, "GET", "/admin/v1/me", call{cookie: b.SessionCookie()}, nil, &p); st != 403 || p.Code != "mfa_enrollment_required" {
		t.Fatalf("no TOTP yet: %d %s", st, p.Code)
	}
	if st := s.api(t, "GET", "/v1/me", call{cookie: b.SessionCookie()}, nil, &p); st != 401 {
		t.Fatalf("cookie on customer plane: %d", st)
	}
	if st := s.api(t, "GET", "/v1/me", call{bearer: b.SessionCookie()}, nil, &p); st != 401 {
		t.Fatalf("cookie value as bearer: %d", st)
	}

	secret := b.EnrolTOTP(t)
	if got, err := s.kadmin.GetIdentity(ctx, adminID); err != nil || !got.HasTOTP || got.TOTPCreatedAt.IsZero() {
		t.Fatalf("TOTP credential created_at not exposed: %+v %v", got, err)
	}
	// Observed on Kratos v26.2.0: enrolling TOTP in the settings flow
	// upgrades the current session to aal2.
	s.verifier.Invalidate(adminID)
	if st := s.api(t, "GET", "/admin/v1/me", call{cookie: b.SessionCookie()}, nil, nil); st != 200 {
		t.Fatalf("session after TOTP enrolment: %d", st)
	}

	// A new password-only login is aal1 → whoami 403 session_aal2_required.
	b = s.env.NewBrowser()
	// Observed: Kratos answers 422 browser_location_change_required (redirect
	// to ?aal=aal2) and sets an aal1 session cookie.
	if st, _ := b.Login(t, email, ""); (st != 200 && st != 422) || b.SessionCookie() == "" {
		t.Fatalf("aal1 login: %d cookie=%v", st, b.SessionCookie() != "")
	}
	if st := s.api(t, "GET", "/admin/v1/me", call{cookie: b.SessionCookie()}, nil, &p); st != 403 || p.Code != "aal2_required" {
		t.Fatalf("aal1 with TOTP enrolled: %d %s", st, p.Code)
	}
	if st := s.api(t, "GET", "/admin/v1/customers", call{cookie: b.SessionCookie()}, nil, &p); st != 403 || p.Code != "aal2_required" {
		t.Fatalf("aal1 on permissioned route: %d %s", st, p.Code)
	}
	if st, f := b.Login(t, email, secret); st != 200 {
		t.Fatalf("aal2 login: %d %+v", st, f.UI.Messages)
	}
	cookie := b.SessionCookie()
	var me struct {
		ID          uuid.UUID `json:"id"`
		AAL         string    `json:"aal"`
		Roles       []string  `json:"roles"`
		Permissions []string  `json:"permissions"`
	}
	if st := s.api(t, "GET", "/admin/v1/me", call{cookie: cookie}, nil, &me); st != 200 || me.ID != adminID || me.AAL != "aal2" ||
		len(me.Roles) != 1 || me.Roles[0] != "super_admin" || len(me.Permissions) != 6 {
		t.Fatalf("admin me: %d %+v", st, me)
	}

	// Customer management.
	custEmail := itest.UniqueEmail("e2e-managed")
	reg := s.env.RegisterCustomer(t, custEmail)
	custID := uuid.MustParse(reg.Session.Identity.ID)
	t.Cleanup(func() { s.env.DeleteIdentity(t, custID) })
	if st := s.api(t, "GET", "/v1/me", call{bearer: reg.SessionToken}, nil, nil); st != 200 {
		t.Fatalf("customer before disable: %d", st)
	}
	var page struct {
		Items []struct {
			ID    uuid.UUID `json:"id"`
			State string    `json:"state"`
		} `json:"items"`
	}
	if st := s.api(t, "GET", "/admin/v1/customers?email="+custEmail, call{cookie: cookie}, nil, &page); st != 200 || len(page.Items) != 1 || page.Items[0].ID != custID {
		t.Fatalf("list by email: %d %+v", st, page)
	}
	if st := s.api(t, "GET", "/admin/v1/customers/"+adminID.String(), call{cookie: cookie}, nil, &p); st != 404 {
		t.Fatalf("admin id via customers must be 404: %d", st)
	}
	if st := s.api(t, "POST", "/admin/v1/customers/"+custID.String()+"/disable", call{cookie: cookie}, map[string]any{"reason": "e2e"}, &p); st != 403 {
		t.Fatalf("CSRF guard (no Origin): %d", st)
	}
	var state struct {
		State string `json:"state"`
	}
	for range 2 { // idempotent
		if st := s.api(t, "POST", "/admin/v1/customers/"+custID.String()+"/disable", call{cookie: cookie, origin: webOrigin}, map[string]any{"reason": "e2e"}, &state); st != 200 || state.State != "inactive" {
			t.Fatalf("disable: %d %+v", st, state)
		}
	}
	if st := s.api(t, "GET", "/v1/me", call{bearer: reg.SessionToken}, nil, nil); st != 401 {
		t.Fatalf("disabled customer's token must be rejected: %d", st)
	}
	if st := s.api(t, "POST", "/admin/v1/customers/"+custID.String()+"/enable", call{cookie: cookie, origin: webOrigin}, nil, &state); st != 200 || state.State != "active" {
		t.Fatalf("enable: %d %+v", st, state)
	}
	if st := s.api(t, "DELETE", "/admin/v1/customers/"+custID.String()+"/sessions", call{cookie: cookie, origin: webOrigin}, nil, nil); st != 204 {
		t.Fatalf("revoke sessions: %d", st)
	}

	// Invitation (email via Mailpit; link never in the response).
	invEmail := itest.UniqueEmail("e2e-invited")
	key := uuid.NewString()
	var inv struct {
		ID   uuid.UUID `json:"id"`
		Role string    `json:"role"`
	}
	body := map[string]any{"email": invEmail, "role": "support", "name": map[string]string{"first": "Inv"}}
	if st := s.api(t, "POST", "/admin/v1/admins", call{cookie: cookie, origin: webOrigin, idemKey: key}, body, &inv); st != 201 || inv.Role != "support" {
		t.Fatalf("invite: %d %+v", st, inv)
	}
	t.Cleanup(func() {
		_ = s.keto.RemoveAll(context.Background(), inv.ID)
		s.env.DeleteIdentity(t, inv.ID)
	})
	mail := s.env.MailBody(t, invEmail)
	if !strings.Contains(mail, "/recovery?flow=") {
		t.Fatalf("invitation email without link: %q", mail)
	}
	var again struct {
		ID uuid.UUID `json:"id"`
	}
	if st := s.api(t, "POST", "/admin/v1/admins", call{cookie: cookie, origin: webOrigin, idemKey: key}, body, &again); st != 201 || again.ID != inv.ID {
		t.Fatalf("idempotent replay: %d %v", st, again.ID)
	}
	if st := s.api(t, "POST", "/admin/v1/admins", call{cookie: cookie, origin: webOrigin, idemKey: uuid.NewString()}, body, &p); st != 409 {
		t.Fatalf("duplicate email: %d", st)
	}
	var adm struct {
		Role string `json:"role"`
	}
	if st := s.api(t, "PUT", "/admin/v1/admins/"+inv.ID.String()+"/role", call{cookie: cookie, origin: webOrigin}, map[string]any{"role": "admin"}, &adm); st != 200 || adm.Role != "admin" {
		t.Fatalf("role change: %d %+v", st, adm)
	}
	if st := s.api(t, "PUT", "/admin/v1/admins/"+adminID.String()+"/role", call{cookie: cookie, origin: webOrigin}, map[string]any{"role": "support"}, &p); st != 403 {
		t.Fatalf("self role change: %d", st)
	}
	var admins struct {
		Items []struct {
			ID          uuid.UUID `json:"id"`
			Role        string    `json:"role"`
			MfaEnrolled bool      `json:"mfa_enrolled"`
		} `json:"items"`
		Next *string `json:"next_page_token"`
	}
	if st := s.api(t, "GET", "/admin/v1/admins?page_size=100", call{cookie: cookie}, nil, &admins); st != 200 {
		t.Fatalf("list admins: %d", st)
	}

	// Audit trail.
	var events struct {
		Items []struct {
			Action    string    `json:"action"`
			ActorID   uuid.UUID `json:"actor_id"`
			TargetID  string    `json:"target_id"`
			RequestID string    `json:"request_id"`
		} `json:"items"`
	}
	if st := s.api(t, "GET", "/admin/v1/audit-events?actor_id="+adminID.String()+"&page_size=50", call{cookie: cookie}, nil, &events); st != 200 {
		t.Fatalf("audit: %d", st)
	}
	seen := map[string]bool{}
	for _, e := range events.Items {
		if e.ActorID != adminID || e.RequestID == "" {
			t.Fatalf("bad event %+v", e)
		}
		seen[e.Action] = true
	}
	for _, a := range []string{"customer.disabled", "customer.enabled", "customer.sessions_revoked", "admin.invited", "admin.role_changed"} {
		if !seen[a] {
			t.Fatalf("audit missing %s: %+v", a, seen)
		}
	}
}

// TestFR07_E2E_Bootstrap: the operator CLI path through the real adapters.
func TestFR07_E2E_Bootstrap(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()
	email := itest.UniqueEmail("e2e-boot")
	res, err := s.admins.Bootstrap(ctx, email, identity.Name{First: "Boot"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = s.keto.RemoveAll(context.Background(), res.Admin.ID)
		s.env.DeleteIdentity(t, res.Admin.ID)
	})
	if !strings.Contains(res.Recovery.Link, "/recovery?flow=") || len(res.Recovery.Code) < 6 {
		t.Fatal("bootstrap must return a recovery link and code")
	}
	ok, err := s.keto.Check(ctx, res.Admin.ID, identity.PermManageAdmins)
	if err != nil || !ok {
		t.Fatalf("bootstrapped admin lacks manage_admins: %v %v", ok, err)
	}
	if _, err := s.admins.Bootstrap(ctx, email, identity.Name{}); !errors.Is(err, app.ErrConflict) {
		t.Fatalf("second bootstrap must conflict: %v", err)
	}
}

// TestS1_E2E_LateTOTPEnrolmentIsBlocked: a password-only attacker enrols TOTP
// directly at Kratos after the deadline, reaches AAL2, and must still be
// rejected and the identity deactivated.
func TestS1_E2E_LateTOTPEnrolmentIsBlocked(t *testing.T) {
	const grace = 2 * time.Second
	s := newStackWithGrace(t, grace)
	ctx := context.Background()
	email := itest.UniqueEmail("e2e-late")
	adminID := s.env.CreateAdminWithPassword(t, email)
	t.Cleanup(func() {
		_ = s.keto.RemoveAll(context.Background(), adminID)
		s.env.DeleteIdentity(t, adminID)
	})
	if err := s.keto.SetRole(ctx, adminID, identity.RoleSuperAdmin); err != nil {
		t.Fatal(err)
	}
	time.Sleep(grace + 500*time.Millisecond) // deadline passes
	b := s.env.NewBrowser()
	if st, _ := b.Login(t, email, ""); st != 200 {
		t.Fatalf("password login: %d", st)
	}
	b.EnrolTOTP(t) // straight at Kratos public; session becomes aal2
	var p problem
	if st := s.api(t, "GET", "/admin/v1/me", call{cookie: b.SessionCookie()}, nil, &p); st != 403 || p.Code != "forbidden" {
		t.Fatalf("late enrolment must be rejected: %d %s", st, p.Code)
	}
	got, err := s.kadmin.GetIdentity(ctx, adminID)
	if err != nil || got.State != identity.StateInactive {
		t.Fatalf("identity must be deactivated: %+v %v", got.State, err)
	}
	if st := s.api(t, "GET", "/admin/v1/me", call{cookie: b.SessionCookie()}, nil, &p); st != 401 {
		t.Fatalf("revoked session: %d", st)
	}
}
