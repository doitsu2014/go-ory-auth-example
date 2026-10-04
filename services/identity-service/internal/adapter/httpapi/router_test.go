package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/adapter/localkms"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/identity"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/login"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/machine"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/platform"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/testutil"
)

const origin = "http://localhost:5173"

var now = time.Date(2026, 10, 3, 8, 0, 0, 0, time.UTC)

type fixture struct {
	h        http.Handler
	ids      *testutil.Identities
	store    *testutil.Store
	kms      *localkms.KMS
	cache    *app.DEKCache
	logs     *syncBuffer
	customer uuid.UUID
	target   uuid.UUID
	seq      atomic.Int64
	// Machine plane.
	kratos   *testutil.Verifier
	mv       *testutil.MachineVerifier
	clients  *testutil.ServiceClients
	clientID string
}

// syncBuffer is a goroutine-safe log sink.
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

func principal(kind identity.Kind, aal identity.AAL, id uuid.UUID, verified bool) identity.Principal {
	return identity.Principal{
		IdentityID: id, SessionID: uuid.New(), Kind: kind, AAL: aal, Email: "u@example.com", LoginID: "u@example.com",
		EmailVerified: verified, AuthenticatedAt: now.Add(-time.Hour), ExpiresAt: now.Add(time.Hour),
	}
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	clock := &testutil.FixedClock{T: now}
	ids := testutil.NewIdentities(clock)
	store := testutil.NewStore(clock)
	authz := testutil.NewAuthz()
	roles := testutil.NewRoles()
	v := testutil.NewVerifier()

	add := func(token string, p identity.Principal, role identity.Role, hasTOTP bool) {
		v.Sessions[token] = p
		if p.Kind == identity.KindAdmin {
			ident := identity.Identity{ID: p.IdentityID, SchemaID: "admin", Email: token + "@example.com", HasTOTP: hasTOTP, CreatedAt: now.Add(-time.Hour)}
			if hasTOTP {
				ident.TOTPCreatedAt = now.Add(-30 * time.Minute)
			}
			ids.Add(ident)
			if role != "" {
				roles.M[p.IdentityID] = []identity.Role{role}
				authz.GrantRole(p.IdentityID, role)
			}
		}
	}
	add("cust", principal(identity.KindCustomer, identity.AAL1, uuid.New(), true), "", false)
	add("cust-unverified", principal(identity.KindCustomer, identity.AAL1, uuid.New(), false), "", false)
	add("super", principal(identity.KindAdmin, identity.AAL2, uuid.New(), true), identity.RoleSuperAdmin, true)
	add("support", principal(identity.KindAdmin, identity.AAL2, uuid.New(), true), identity.RoleSupport, true)
	add("enrol", principal(identity.KindAdmin, identity.AAL1, uuid.New(), true), identity.RoleSuperAdmin, false)
	add("aal1-totp", principal(identity.KindAdmin, identity.AAL1, uuid.New(), true), identity.RoleSuperAdmin, true)
	stale := principal(identity.KindAdmin, identity.AAL2, uuid.New(), true)
	stale.AuthenticatedAt = now.Add(-13 * time.Hour)
	add("stale", stale, identity.RoleSuperAdmin, true)
	v.Errs["aal2req"] = app.ErrAAL2Required
	v.Errs["down"] = fmt.Errorf("%w: boom", app.ErrDependencyUnavailable)

	cust := ids.Add(identity.Identity{SchemaID: "customer", Email: "c@example.com", LoginID: "c@example.com", CreatedAt: now})
	target := ids.Add(identity.Identity{SchemaID: "admin", Email: "t@example.com", HasTOTP: true, TOTPCreatedAt: now, CreatedAt: now})
	roles.M[target.ID] = []identity.Role{identity.RoleSupport}

	mv := testutil.NewMachineVerifier()
	mv.Tokens["m2m-cust"] = machine.Principal{ClientID: "client-cust", Scopes: []machine.Scope{machine.ScopeCustomersRead}, TokenID: "jti-cust"}
	mv.Tokens["m2m-audit"] = machine.Principal{ClientID: "client-audit", Scopes: []machine.Scope{machine.ScopeAuditRead}, TokenID: "jti-audit"}
	mv.Errs["m2m-down"] = fmt.Errorf("%w: hydra", app.ErrDependencyUnavailable)
	clients := testutil.NewServiceClients(clock)
	sc, _, _ := clients.Create(context.Background(), app.NewServiceClient{Registration: machine.Registration{
		Name: "existing", Owner: "ops@example.com", Scopes: []machine.Scope{machine.ScopeAuditRead},
	}})

	repos := store.Repos()
	kms := localkms.NewRandom()
	cache := app.NewDEKCache(100, time.Minute, clock.Now)
	logs := &syncBuffer{}
	log := platform.NewLogger(logs, "debug")
	server := &Server{
		Me:        &app.MeService{Profiles: repos.Profiles},
		Customers: &app.CustomerService{Authz: authz, Identities: ids, Profiles: repos.Profiles, Tx: store, Sessions: v},
		Admins: &app.AdminService{Authz: authz, Roles: roles, Identities: ids, Tx: store, Idempotency: repos.Idempotency,
			Mailer: &testutil.Mailer{}, Clock: clock},
		Audit: &app.AuditService{Authz: authz, Audit: repos.Audit},
		PersonalInfo: &app.PersonalInfoService{
			Authz: authz, Identities: ids, Keys: kms, Cache: cache, Tx: store, SubjectKeys: repos.SubjectKeys,
			Records: repos.PersonalInfo, Clock: clock, Log: log,
			LookupLimiter: app.NewRateLimiter(app.LookupRateRules, clock.Now),
			RevealLimiter: app.NewRateLimiter(app.RevealRateRules, clock.Now),
		},
		Machine: &app.MachineService{Identities: ids, Audit: repos.Audit},
		Logins: &app.LoginIdentifierService{Keys: kms, Logins: repos.Logins, Tx: store, Identities: ids,
			Phone: login.PhonePolicy{DefaultCountry: "84", AllowedCountries: []string{"84"}}, PhoneEnabled: true,
			Phase: app.PhaseComplete, Clock: clock, Log: log,
			ResolveLimiter: app.NewKeyedLimiter[string]([]app.RateRule{{Limit: 3, Window: time.Minute}}, clock.Now)},
		ServiceClients: &app.ServiceClientService{Authz: authz, Clients: clients, Verifier: mv, Tx: store,
			Idempotency: repos.Idempotency, Clock: clock, Log: log},
	}
	h, err := NewPublicHandler(PublicDeps{
		Log: log, Metrics: NewMetrics(), Server: server, Verifier: v, Authz: app.MemoAuthorizer{Next: authz},
		Gate:            &app.AdminGate{Identities: ids, Sessions: v, Tx: store, Clock: clock},
		AllowedOrigins:  []string{origin},
		MachineVerifier: mv, MachineClientLimiter: NewMachineClientLimiter(5, clock.Now),
		MachineFailureLimiter: NewMachineFailureLimiter(clock.Now),
	})
	if err != nil {
		t.Fatal(err)
	}
	return &fixture{h: h, ids: ids, store: store, kms: kms, cache: cache, logs: logs, customer: cust.ID, target: target.ID,
		kratos: v, mv: mv, clients: clients, clientID: sc.ClientID}
}

type route struct {
	method, path string
	body         func(f *fixture) string
	header       func(f *fixture) http.Header
	okStatus     int
	admin        bool
	mutation     bool
	// minRole is the weakest role allowed; "" = any admin.
	supportOK bool
}

func routes() []route {
	idem := func(f *fixture) http.Header {
		return http.Header{"Idempotency-Key": {uuid.NewString()}}
	}
	return []route{
		{method: "GET", path: "/v1/me", okStatus: 200},
		{method: "PATCH", path: "/v1/me", body: func(*fixture) string { return `{"display_name":"An"}` }, okStatus: 200, mutation: true},
		{method: "GET", path: "/admin/v1/me", okStatus: 200, admin: true, supportOK: true},
		{method: "GET", path: "/admin/v1/customers", okStatus: 200, admin: true, supportOK: true},
		{method: "GET", path: "/admin/v1/customers/{customer}", okStatus: 200, admin: true, supportOK: true},
		{method: "POST", path: "/admin/v1/customers/{customer}/disable", body: func(*fixture) string { return `{"reason":"r"}` }, okStatus: 200, admin: true, mutation: true},
		{method: "POST", path: "/admin/v1/customers/{customer}/enable", okStatus: 200, admin: true, mutation: true},
		{method: "DELETE", path: "/admin/v1/customers/{customer}/sessions", okStatus: 204, admin: true, mutation: true},
		{method: "GET", path: "/admin/v1/admins", okStatus: 200, admin: true},
		{method: "POST", path: "/admin/v1/admins", header: idem, okStatus: 201, admin: true, mutation: true,
			body: func(f *fixture) string {
				return fmt.Sprintf(`{"email":"n%d@example.com","role":"support"}`, f.seq.Add(1))
			}},
		{method: "PUT", path: "/admin/v1/admins/{target}/role", body: func(*fixture) string { return `{"role":"admin"}` }, okStatus: 200, admin: true, mutation: true},
		{method: "GET", path: "/admin/v1/audit-events", okStatus: 200, admin: true},
		// Personal information.
		{method: "GET", path: "/v1/me/personal-info", okStatus: 200},
		{method: "PUT", path: "/v1/me/personal-info", body: func(*fixture) string { return validPII }, okStatus: 200, mutation: true},
		{method: "DELETE", path: "/v1/me/personal-info", okStatus: 204}, // §9 A14: no verified email needed
		{method: "POST", path: "/admin/v1/customers/lookup", body: func(*fixture) string { return `{"phone_number":"+84901234567"}` },
			okStatus: 200, admin: true, mutation: true, supportOK: true},
		{method: "GET", path: "/admin/v1/customers/{customer}/personal-info", okStatus: 200, admin: true, supportOK: true},
		{method: "POST", path: "/admin/v1/customers/{customer}/personal-info/reveal", body: func(*fixture) string { return `{"reason_code":"legal_request"}` },
			okStatus: 200, admin: true, mutation: true},
		// Service clients (super_admin only).
		{method: "GET", path: "/admin/v1/service-clients", okStatus: 200, admin: true},
		{method: "POST", path: "/admin/v1/service-clients", header: idem, okStatus: 201, admin: true, mutation: true,
			body: func(f *fixture) string {
				return fmt.Sprintf(`{"name":"client-%d","owner":"ops@example.com","scopes":["audit:read"]}`, f.seq.Add(1))
			}},
		{method: "GET", path: "/admin/v1/service-clients/{client}", okStatus: 200, admin: true},
		{method: "POST", path: "/admin/v1/service-clients/{client}/rotate-secret", okStatus: 200, admin: true, mutation: true},
		{method: "DELETE", path: "/admin/v1/service-clients/{client}", okStatus: 204, admin: true, mutation: true},
	}
}

type cred struct {
	bearer, cookie, xToken string
	origin                 string
	contentType            string
}

func (f *fixture) do(t *testing.T, rt route, c cred) *httptest.ResponseRecorder {
	t.Helper()
	path := strings.NewReplacer("{customer}", f.customer.String(), "{target}", f.target.String(), "{client}", f.clientID).Replace(rt.path)
	var body io.Reader = http.NoBody
	if rt.body != nil {
		body = bytes.NewBufferString(rt.body(f))
	}
	req := httptest.NewRequest(rt.method, path, body)
	if rt.body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.contentType != "" {
		req.Header.Set("Content-Type", c.contentType)
	}
	if rt.header != nil {
		for k, v := range rt.header(f) {
			req.Header[k] = v
		}
	}
	if c.bearer != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearer)
	}
	if c.xToken != "" {
		req.Header.Set("X-Session-Token", c.xToken)
	}
	if c.cookie != "" {
		req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: c.cookie})
	}
	if c.origin != "" {
		req.Header.Set("Origin", c.origin)
	}
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, req)
	return rec
}

func problemCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/problem+json") {
		return ""
	}
	var p struct {
		Code      string `json:"code"`
		RequestID string `json:"request_id"`
		Status    int    `json:"status"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("bad problem: %v", err)
	}
	if p.RequestID == "" || p.Status != rec.Code {
		t.Fatalf("problem missing request_id/status: %s", rec.Body.String())
	}
	return p.Code
}

type expect struct {
	status int
	code   string
}

// TestFR08_AuthMatrix asserts the 401/403 matrix for every route (FR-06, FR-08).
func TestFR08_AuthMatrix(t *testing.T) {
	for _, rt := range routes() {
		cases := map[string]struct {
			c    cred
			want expect
		}{}
		cases["no credential"] = struct {
			c    cred
			want expect
		}{cred{origin: origin}, expect{401, CodeUnauthenticated}}
		if !rt.admin {
			add := func(name string, c cred, w expect) {
				cases[name] = struct {
					c    cred
					want expect
				}{c, w}
			}
			add("cookie on customer plane", cred{cookie: "cust"}, expect{401, CodeUnauthenticated})
			add("bearer and cookie", cred{bearer: "cust", cookie: "cust"}, expect{401, CodeUnauthenticated})
			add("x-session-token", cred{xToken: "cust"}, expect{401, CodeUnauthenticated})
			add("unknown token", cred{bearer: "nope"}, expect{401, CodeUnauthenticated})
			add("admin principal", cred{bearer: "super"}, expect{403, CodeForbidden})
			add("aal2 required", cred{bearer: "aal2req"}, expect{403, CodeAAL2Required})
			add("kratos down", cred{bearer: "down"}, expect{503, CodeDependencyUnavailable})
			add("ok", cred{bearer: "cust"}, expect{rt.okStatus, ""})
			if rt.mutation {
				add("unverified email", cred{bearer: "cust-unverified"}, expect{403, CodeEmailNotVerified})
			}
		} else {
			add := func(name string, c cred, w expect) {
				if c.origin == "" && c.contentType == "" && !strings.HasPrefix(name, "csrf") {
					c.origin = origin
				}
				cases[name] = struct {
					c    cred
					want expect
				}{c, w}
			}
			add("bearer on admin plane", cred{bearer: "super"}, expect{401, CodeUnauthenticated})
			add("cookie plus authorization", cred{cookie: "super", bearer: "super"}, expect{401, CodeUnauthenticated})
			add("cookie plus x-session-token", cred{cookie: "super", xToken: "super"}, expect{401, CodeUnauthenticated})
			add("unknown cookie", cred{cookie: "nope"}, expect{401, CodeUnauthenticated})
			add("customer principal", cred{cookie: "cust"}, expect{403, CodeNotAdmin})
			add("aal2 required", cred{cookie: "aal2req"}, expect{403, CodeAAL2Required})
			add("kratos down", cred{cookie: "down"}, expect{503, CodeDependencyUnavailable})
			add("mfa enrolment pending", cred{cookie: "enrol"}, expect{403, CodeMFAEnrollmentRequired})
			if rt.path == "/admin/v1/me" {
				add("aal1 with totp on me", cred{cookie: "aal1-totp"}, expect{200, ""})
			} else {
				add("aal1 with totp", cred{cookie: "aal1-totp"}, expect{403, CodeAAL2Required})
			}
			if rt.supportOK {
				add("support allowed", cred{cookie: "support"}, expect{rt.okStatus, ""})
			} else {
				add("support denied", cred{cookie: "support"}, expect{403, CodeForbidden})
			}
			if rt.mutation {
				add("csrf missing origin", cred{cookie: "super"}, expect{403, CodeForbidden})
				add("csrf foreign origin", cred{cookie: "super", origin: "https://evil.example"}, expect{403, CodeForbidden})
				add("csrf form content type", cred{cookie: "super", origin: origin, contentType: "text/plain"}, expect{403, CodeForbidden})
			}
			add("ok", cred{cookie: "super"}, expect{rt.okStatus, ""})
		}
		for name, tc := range cases {
			t.Run(rt.method+" "+rt.path+"/"+name, func(t *testing.T) {
				f := newFixture(t)
				rec := f.do(t, rt, tc.c)
				if rec.Code != tc.want.status {
					t.Fatalf("status %d want %d: %s", rec.Code, tc.want.status, rec.Body.String())
				}
				if got := problemCode(t, rec); got != tc.want.code {
					t.Fatalf("code %q want %q", got, tc.want.code)
				}
				if rec.Header().Get("Cache-Control") != "no-store" || rec.Header().Get("X-Request-Id") == "" {
					t.Fatal("missing security headers / request id")
				}
			})
		}
	}
}

func TestFR06_AdminOlderThan12hIsRevoked(t *testing.T) {
	f := newFixture(t)
	rec := f.do(t, route{method: "GET", path: "/admin/v1/me"}, cred{cookie: "stale"})
	if rec.Code != 401 || problemCode(t, rec) != CodeUnauthenticated {
		t.Fatalf("stale admin session: %d %s", rec.Code, rec.Body.String())
	}
	if len(f.ids.RevokedSessions) != 1 {
		t.Fatal("stale admin session was not revoked in Kratos")
	}
}

func TestFR11_CustomersEndpointHidesAdmins(t *testing.T) {
	f := newFixture(t)
	rt := route{method: "GET", path: "/admin/v1/customers/{target}"}
	rec := f.do(t, rt, cred{cookie: "super"})
	if rec.Code != 404 || problemCode(t, rec) != CodeNotFound {
		t.Fatalf("admin id via customers: %d %s", rec.Code, rec.Body.String())
	}
	rec = f.do(t, route{method: "GET", path: "/admin/v1/customers/not-a-uuid"}, cred{cookie: "super"})
	if rec.Code != 404 {
		t.Fatalf("bad uuid: %d", rec.Code)
	}
}

func TestFR07_InviteRequiresIdempotencyKeyAndNeverReturnsLink(t *testing.T) {
	f := newFixture(t)
	rt := route{method: "POST", path: "/admin/v1/admins", body: func(*fixture) string { return `{"email":"z@example.com","role":"admin"}` }}
	rec := f.do(t, rt, cred{cookie: "super", origin: origin})
	if rec.Code != 422 || problemCode(t, rec) != CodeValidationFailed {
		t.Fatalf("missing key: %d %s", rec.Code, rec.Body.String())
	}
	key := uuid.NewString()
	rt.header = func(*fixture) http.Header { return http.Header{"Idempotency-Key": {key}} }
	rec = f.do(t, rt, cred{cookie: "super", origin: origin})
	if rec.Code != 201 {
		t.Fatalf("invite: %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "recovery") || strings.Contains(rec.Body.String(), "123456") {
		t.Fatalf("response leaks the invitation secret: %s", rec.Body.String())
	}
	first := rec.Body.String()
	rec = f.do(t, rt, cred{cookie: "super", origin: origin})
	if rec.Code != 201 || rec.Body.String() != first {
		t.Fatalf("replay differs: %d %s", rec.Code, rec.Body.String())
	}
	rt.body = func(*fixture) string { return `{"email":"z@example.com","role":"root"}` }
	rt.header = func(*fixture) http.Header { return http.Header{"Idempotency-Key": {uuid.NewString()}} }
	rec = f.do(t, rt, cred{cookie: "super", origin: origin})
	if rec.Code != 422 {
		t.Fatalf("bad role enum: %d %s", rec.Code, rec.Body.String())
	}
}

func TestFR10_PatchMeValidation(t *testing.T) {
	f := newFixture(t)
	long := strings.Repeat("x", 101)
	rt := route{method: "PATCH", path: "/v1/me", body: func(*fixture) string { return `{"display_name":"` + long + `"}` }}
	rec := f.do(t, rt, cred{bearer: "cust"})
	if rec.Code != 422 || problemCode(t, rec) != CodeValidationFailed || !strings.Contains(rec.Body.String(), "too_long") {
		t.Fatalf("validation: %d %s", rec.Code, rec.Body.String())
	}
	rt.body = func(*fixture) string { return `{"display_name":null,"locale":"en-US"}` }
	rec = f.do(t, rt, cred{bearer: "cust"})
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"display_name":null`) || !strings.Contains(rec.Body.String(), `"locale":"en-US"`) {
		t.Fatalf("patch: %d %s", rec.Code, rec.Body.String())
	}
	rt.body = func(*fixture) string { return `not json` }
	rec = f.do(t, rt, cred{bearer: "cust"})
	if rec.Code != 422 {
		t.Fatalf("bad json: %d", rec.Code)
	}
}

func TestCORSPreflight(t *testing.T) {
	f := newFixture(t)
	req := httptest.NewRequest("OPTIONS", "/admin/v1/customers", nil)
	req.Header.Set("Origin", origin)
	req.Header.Set("Access-Control-Request-Method", "POST")
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, req)
	if rec.Code != 204 || rec.Header().Get("Access-Control-Allow-Origin") != origin || rec.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatalf("preflight: %d %v", rec.Code, rec.Header())
	}
	req.Header.Set("Origin", "https://evil.example")
	rec = httptest.NewRecorder()
	f.h.ServeHTTP(rec, req)
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("foreign origin allowed")
	}
}

func TestP4_StartupFailsWithoutPolicy(t *testing.T) {
	r := chi.NewRouter()
	r.Get("/admin/v1/me", func(http.ResponseWriter, *http.Request) {})
	r.Get("/admin/v1/new-thing", func(http.ResponseWriter, *http.Request) {})
	err := ValidatePolicies(r, map[string]Policy{"GET /admin/v1/me": {Plane: PlaneAdmin, Self: true}})
	if err == nil || !strings.Contains(err.Error(), "new-thing") {
		t.Fatalf("want missing-policy error, got %v", err)
	}
	err = ValidatePolicies(r, map[string]Policy{
		"GET /admin/v1/me":        {Plane: PlaneAdmin, Self: true},
		"GET /admin/v1/new-thing": {Plane: PlaneAdmin},
	})
	if err == nil || !strings.Contains(err.Error(), "declares no permission") {
		t.Fatalf("want no-permission error, got %v", err)
	}
	if err := ValidatePolicies(r, map[string]Policy{
		"GET /admin/v1/me": {Plane: PlaneAdmin, Self: true}, "GET /admin/v1/new-thing": {Plane: PlaneAdmin, Permission: identity.PermViewAudit},
		"GET /admin/v1/gone": {Plane: PlaneAdmin, Permission: identity.PermViewAudit},
	}); err == nil || !strings.Contains(err.Error(), "has no route") {
		t.Fatalf("want stale-policy error, got %v", err)
	}
}

func TestP4_EveryGeneratedRouteHasPolicy(t *testing.T) {
	newFixture(t) // NewPublicHandler validates RoutePolicies against the generated router.
}

func TestFR07_AdminListReportsMFAEnrolment(t *testing.T) {
	f := newFixture(t)
	f.ids.Add(identity.Identity{SchemaID: "admin", Email: "nomfa@example.com", CreatedAt: now})
	rec := f.do(t, route{method: "GET", path: "/admin/v1/admins"}, cred{cookie: "super"})
	if rec.Code != 200 {
		t.Fatalf("list: %d %s", rec.Code, rec.Body.String())
	}
	var page struct {
		Items []struct {
			Email       string `json:"email"`
			MfaEnrolled *bool  `json:"mfa_enrolled"`
		} `json:"items"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &page)
	got := map[string]bool{}
	for _, it := range page.Items {
		if it.MfaEnrolled == nil {
			t.Fatalf("mfa_enrolled missing for %s", it.Email)
		}
		got[it.Email] = *it.MfaEnrolled
	}
	if !got["t@example.com"] || got["nomfa@example.com"] || len(got) < 3 {
		t.Fatalf("mfa_enrolled: %v", got)
	}
}

func TestPageSizeOutOfRangeIs422(t *testing.T) {
	f := newFixture(t)
	for _, q := range []string{"0", "101", "-1"} {
		for _, p := range []string{"/admin/v1/customers", "/admin/v1/admins", "/admin/v1/audit-events"} {
			rec := f.do(t, route{method: "GET", path: p + "?page_size=" + q}, cred{cookie: "super"})
			if rec.Code != 422 || !strings.Contains(rec.Body.String(), "page_size") {
				t.Fatalf("%s page_size=%s: %d %s", p, q, rec.Code, rec.Body.String())
			}
		}
	}
	if rec := f.do(t, route{method: "GET", path: "/admin/v1/customers?page_size=100"}, cred{cookie: "super"}); rec.Code != 200 {
		t.Fatalf("100 is valid: %d", rec.Code)
	}
}

func TestP4_UnmatchedRouteFailsClosed(t *testing.T) {
	f := newFixture(t)
	for _, p := range []string{"/admin/v1/nope", "/admin/v1/customers%2Fx", "/v1/other"} {
		c := cred{cookie: "super"}
		if strings.HasPrefix(p, "/v1") {
			c = cred{bearer: "cust"}
		}
		rec := f.do(t, route{method: "GET", path: p}, c)
		if rec.Code != 404 || problemCode(t, rec) != CodeNotFound {
			t.Fatalf("%s: %d", p, rec.Code)
		}
	}
	rec := f.do(t, route{method: "DELETE", path: "/admin/v1/me"}, cred{cookie: "super", origin: origin})
	if rec.Code != 404 {
		t.Fatalf("unknown method must fail closed: %d", rec.Code)
	}
}

func TestClientIPTrustedHops(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.9:1234"
	req.Header.Add("X-Forwarded-For", "6.6.6.6, 203.0.113.7")
	req.Header.Add("X-Forwarded-For", "10.0.0.2")
	tests := []struct {
		hops int
		want string
	}{{0, "10.0.0.9"}, {1, "10.0.0.2"}, {2, "203.0.113.7"}, {3, "6.6.6.6"}, {9, "10.0.0.9"}}
	for _, tt := range tests {
		if got := clientIP(req, tt.hops); got == nil || got.String() != tt.want {
			t.Fatalf("hops=%d: %v want %s", tt.hops, got, tt.want)
		}
	}
}

func TestRequestIDValidation(t *testing.T) {
	if newRequestID("abc-DEF_123.xyz") != "abc-DEF_123.xyz" {
		t.Fatal("valid id must be kept")
	}
	for _, bad := range []string{"", "short", "has space here", "evil\r\ninjected-0000", strings.Repeat("a", 65), "<script>alert(1)</script>"} {
		if got := newRequestID(bad); got == bad {
			t.Fatalf("invalid id %q accepted", bad)
		}
	}
}
