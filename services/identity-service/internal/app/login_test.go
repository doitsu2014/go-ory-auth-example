package app_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/adapter/localkms"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/audit"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/identity"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/login"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/platform"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/testutil"
)

// Login test values; none may appear in logs.
const (
	tEmail      = "alice.login@example.com"
	tLoginPhone = "0912 345 678"
)

var (
	tIP   = netip.MustParseAddr("203.0.113.7")
	vnPol = login.PhonePolicy{DefaultCountry: "84", AllowedCountries: []string{"84"}}
)

type loginEnv struct {
	*env
	kms   *localkms.KMS
	logs  *bytes.Buffer
	svc   *app.LoginIdentifierService
	auth  *app.CustomerAuthService
	flows *testutil.AuthFlows
}

// tPassword is the password of every test registration.
const tPassword = "correct horse battery staple"

var tClient = app.FlowClient{IP: tIP, UserAgent: "test-app/1.0"}

func newLoginEnv(t *testing.T) *loginEnv {
	t.Helper()
	e := newEnv()
	l := &loginEnv{env: e, kms: localkms.NewRandom(), logs: &bytes.Buffer{}, flows: testutil.NewAuthFlows()}
	l.svc = &app.LoginIdentifierService{
		Keys: l.kms, Logins: e.store.Repos().Logins, Tx: e.store, Identities: e.ids,
		Phone: vnPol, PhoneEnabled: true, Phase: app.PhaseTransition, Clock: e.clock,
		Log:           platform.NewLogger(l.logs, "debug"),
		InsertLimiter: app.NewKeyedLimiter[string](app.InsertGlobalRules, e.clock.Now),
	}
	l.auth = &app.CustomerAuthService{
		Logins: l.svc, Flows: l.flows,
		SignInLimiter:   app.NewKeyedLimiter[string](app.SignInRateRules, e.clock.Now),
		RegisterLimiter: app.NewKeyedLimiter[string](app.RegisterRateRules, e.clock.Now),
		NetLimiter:      app.NewKeyedLimiter[string](app.NetRateRules, e.clock.Now),
		AccountLimiter:  app.NewKeyedLimiter[string](app.AccountRateRules, e.clock.Now),
	}
	t.Cleanup(func() {
		for _, v := range []string{tEmail, "912345678", "alice.login", tPassword} {
			if strings.Contains(l.logs.String(), v) {
				t.Errorf("logs leak %q: %s", v, l.logs.String())
			}
		}
	})
	return l
}

// register registers an address through the proxy and returns the Kratos
// handle it was registered with.
func (l *loginEnv) register(t *testing.T, typ, value string) string {
	t.Helper()
	l.auth.RegisterLimiter = nil // fixtures register many customers
	if _, err := l.auth.Register(context.Background(), app.Credentials{Client: tClient, Type: typ, Value: value, Password: tPassword}); err != nil {
		t.Fatalf("register: %v", err)
	}
	calls := l.flows.CallsOf("register")
	return calls[len(calls)-1].Identifier
}

func creds(typ, value, password string) app.Credentials {
	return app.Credentials{Client: tClient, Type: typ, Value: value, Password: password}
}

func fieldCode(err error) string {
	var ve *app.ValidationError
	if errors.As(err, &ve) && len(ve.Fields) > 0 {
		return ve.Fields[0].Field + ":" + ve.Fields[0].Code
	}
	return ""
}

func flowIDs(err error) []int {
	var fe *app.AuthFlowError
	if !errors.As(err, &fe) {
		return nil
	}
	var out []int
	for _, m := range fe.Messages {
		out = append(out, m.ID)
	}
	return out
}

func TestPLXFR02_RegistrationStoresSealedUnderRandomHandle(t *testing.T) {
	l := newLoginEnv(t)
	ctx := context.Background()
	sess, err := l.auth.Register(ctx, creds("email", "  Alice.Login@EXAMPLE.com", tPassword))
	if err != nil || sess.Token == "" || sess.VerificationFlowID == "" {
		t.Fatalf("register: %+v %v", sess, err)
	}
	handle := l.flows.CallsOf("register")[0].Identifier
	p, ok := login.ParsePseudonym(handle)
	if !ok || len(l.store.Logins) != 1 {
		t.Fatalf("handle %q rows %d", handle, len(l.store.Logins))
	}
	rec := l.store.Logins[p]
	if rec.IdentityID != nil || rec.Kind != login.KindEmail || strings.Contains(rec.Ciphertext, "alice") {
		t.Fatalf("row %+v", rec)
	}
	// The handle is random, not the keyed hash of the address (ADR-0014).
	if [32]byte(rec.LookupKey) == [32]byte(p) {
		t.Fatal("handle must not equal the lookup key")
	}
	// Same address again: same handle, one row; Kratos rejects the duplicate.
	_, err = l.auth.Register(ctx, creds("email", tEmail, tPassword))
	if ids := flowIDs(err); len(ids) != 1 || ids[0] != 4000007 {
		t.Fatalf("duplicate: %v", err)
	}
	if again := l.flows.CallsOf("register")[1].Identifier; again != handle || len(l.store.Logins) != 1 {
		t.Fatal("one address must keep one handle")
	}
	// Each new address gets its own random handle.
	other := l.register(t, "phone", tLoginPhone)
	if other == handle || !login.IsPseudonym(other) {
		t.Fatal("phone handle")
	}
	if c := l.flows.CallsOf("register")[0].Client; c != tClient {
		t.Fatalf("client not forwarded: %+v", c)
	}
}

func TestPLXFR01_LoginWithAddress(t *testing.T) {
	l := newLoginEnv(t)
	ctx := context.Background()
	handle := l.register(t, "email", tEmail)
	sess, err := l.auth.Login(ctx, creds("email", strings.ToUpper(tEmail), tPassword))
	if err != nil || sess.Token != "token-"+handle {
		t.Fatalf("login: %v", err)
	}
	if got := l.flows.CallsOf("login")[0].Identifier; got != handle {
		t.Fatal("Kratos must be called with the stored handle")
	}
	// Wrong password and unknown address: the same rejection; the unknown
	// address runs the same flow with a random decoy handle (PLX-NFR-03).
	_, wrong := l.auth.Login(ctx, creds("email", tEmail, "wrong password!"))
	l.svc.Phase = app.PhaseComplete
	_, unknown := l.auth.Login(ctx, creds("email", "nobody@example.com", tPassword))
	for _, err := range []error{wrong, unknown} {
		if ids := flowIDs(err); len(ids) != 1 || ids[0] != app.KratosInvalidCredentials {
			t.Fatalf("rejection: %v", err)
		}
	}
	calls := l.flows.CallsOf("login")
	decoy := calls[len(calls)-1].Identifier
	if !login.IsPseudonym(decoy) || decoy == handle || len(l.store.Logins) != 1 {
		t.Fatalf("decoy %q", decoy)
	}
}

func TestPLXFR07_LegacyCustomerSignsInDuringTransition(t *testing.T) {
	l := newLoginEnv(t)
	ctx := context.Background()
	l.flows.Passwords["legacy@example.com"] = tPassword
	if _, err := l.auth.Login(ctx, creds("email", "Legacy@Example.com", tPassword)); err != nil {
		t.Fatalf("legacy login: %v", err)
	}
	if _, err := l.auth.StartRecovery(ctx, app.RecoveryRequest{Client: tClient, Type: "email", Value: "legacy@example.com"}); err != nil {
		t.Fatal(err)
	}
	if got := l.flows.CallsOf("recovery")[0].Identifier; got != "legacy@example.com" {
		t.Fatal("legacy recovery must use the email trait")
	}
	l.svc.Phase = app.PhaseComplete
	if _, err := l.auth.Login(ctx, creds("email", "legacy@example.com", tPassword)); flowIDs(err) == nil {
		t.Fatal("after the migration only handles sign in")
	}
}

func TestPLXFR03_RecoveryNeverHandsOutTheFlow(t *testing.T) {
	l := newLoginEnv(t)
	ctx := context.Background()
	l.svc.Phase = app.PhaseComplete
	handle := l.register(t, "phone", tLoginPhone)
	known, err := l.auth.StartRecovery(ctx, app.RecoveryRequest{Client: tClient, Type: "phone", Value: "+84912345678"})
	if err != nil {
		t.Fatal(err)
	}
	unknown, err := l.auth.StartRecovery(ctx, app.RecoveryRequest{Client: tClient, Type: "email", Value: "nobody@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	// Sealed references: same shape, never the Kratos flow id (anyone can
	// read a recovery flow by id, and it shows the handle).
	for _, ref := range []string{known, unknown} {
		if !strings.Contains(ref, ":v1:") || strings.Contains(ref, testutil.RecoveryFlowID) {
			t.Fatalf("reference %q", ref)
		}
	}
	calls := l.flows.CallsOf("recovery")
	if calls[0].Identifier != handle || !login.IsPseudonym(calls[1].Identifier) || calls[1].Identifier == handle {
		t.Fatal("recovery must use the handle, or a decoy")
	}
	code := func(ref, c string) (app.RecoveryGrant, error) {
		return l.auth.SubmitRecoveryCode(ctx, app.RecoveryCodeRequest{Client: tClient, RecoveryID: ref, Code: c})
	}
	if _, err := code(known, "000000"); flowIDs(err) == nil || flowIDs(err)[0] != 4060006 {
		t.Fatalf("wrong code: %v", err)
	}
	g, err := code(known, testutil.RecoveryCode)
	if err != nil || g.SessionToken == "" || g.SettingsFlowID == "" {
		t.Fatalf("grant: %v", err)
	}
	if l.flows.CallsOf("recovery-code")[0].Identifier != testutil.RecoveryFlowID {
		t.Fatal("code must be submitted on the sealed flow")
	}
	if strings.Contains(fmt.Sprint(g), g.SessionToken) {
		t.Fatal("grant must redact itself")
	}
	// Tampered, foreign or malformed references are rejected before Kratos.
	other, _, _ := l.kms.SealLogin(ctx, []byte("identity-service/login/v1"), []byte(testutil.RecoveryFlowID))
	before := len(l.flows.CallsOf("recovery-code"))
	for _, ref := range []string{"", "not-a-reference", other, known[:len(known)-4] + "AAAA"} {
		if _, err := code(ref, testutil.RecoveryCode); fieldCode(err) != "recovery_id:invalid" {
			t.Errorf("ref %q: %v", ref, err)
		}
	}
	if _, err := code(known, "12ab56"); fieldCode(err) != "code:invalid_format" {
		t.Fatalf("bad code: %v", err)
	}
	if len(l.flows.CallsOf("recovery-code")) != before {
		t.Fatal("invalid input reached Kratos")
	}
}

func TestPLXFR07_LegacyRecoveryWithStrayUnboundRow(t *testing.T) {
	l := newLoginEnv(t)
	ctx := context.Background()
	// A legacy customer; someone tried to register their address (the
	// pre-registration webhook refuses it, the vault row stays unbound).
	l.ids.AddCustomer("legacy@example.com", true)
	l.flows.RegisterErr = &app.AuthFlowError{Messages: []app.FlowMessage{{Field: app.FlowFieldLogin, ID: 4000007}}}
	_, _ = l.auth.Register(ctx, creds("email", "legacy@example.com", tPassword))
	if len(l.store.Logins) != 1 {
		t.Fatal("expected a stray unbound row")
	}
	if _, err := l.auth.StartRecovery(ctx, app.RecoveryRequest{Client: tClient, Type: "email", Value: "legacy@example.com"}); err != nil {
		t.Fatal(err)
	}
	if got := l.flows.CallsOf("recovery")[0].Identifier; got != "legacy@example.com" {
		t.Fatalf("recovery must reach the legacy account, got %q", got)
	}
	// Once a Kratos identity uses the handle, the handle wins.
	handle := l.flows.CallsOf("register")[0].Identifier
	l.ids.AddCustomer(handle, false)
	_, _ = l.auth.StartRecovery(ctx, app.RecoveryRequest{Client: tClient, Type: "email", Value: "legacy@example.com"})
	if got := l.flows.CallsOf("recovery")[1].Identifier; got != handle {
		t.Fatalf("handle in use must win, got %q", got)
	}
}

func TestPLXValidation(t *testing.T) {
	l := newLoginEnv(t)
	ctx := context.Background()
	for _, tc := range []struct{ typ, value, password, want string }{
		{"username", "x", tPassword, "login.type:invalid"},
		{"email", "not-an-email", tPassword, "login.value:invalid_format"},
		{"phone", "+1 202 555 0100", tPassword, "login.value:unsupported_country"},
		{"email", tEmail, "", "password:required"},
		{"email", tEmail, strings.Repeat("x", app.MaxPasswordLength+1), "password:too_long"},
	} {
		_, err := l.auth.Login(ctx, creds(tc.typ, tc.value, tc.password))
		if got := fieldCode(err); got != tc.want {
			t.Errorf("%s/%s: %q (%v)", tc.typ, tc.value, got, err)
		}
		if err != nil && strings.Contains(err.Error(), tc.value) {
			t.Errorf("error echoes the value: %v", err)
		}
	}
	l.svc.PhoneEnabled = false
	_, err := l.auth.Register(ctx, creds("phone", tLoginPhone, tPassword))
	if got := fieldCode(err); got != "login.type:unsupported" {
		t.Fatalf("phone without SMS channel: %q", got)
	}
	if len(l.flows.Calls) != 0 || len(l.store.Logins) != 0 {
		t.Fatal("invalid input must not reach Kratos or the vault")
	}
}

func TestPLXNFR02_RateLimits(t *testing.T) {
	l := newLoginEnv(t)
	ctx := context.Background()
	for i := range 5 {
		if _, err := l.auth.Register(ctx, creds("email", fmt.Sprintf("r%d@example.com", i), tPassword)); err != nil {
			t.Fatalf("registration %d: %v", i, err)
		}
	}
	if _, err := l.auth.Register(ctx, creds("email", tEmail, tPassword)); !errors.Is(err, app.ErrRateLimited) {
		t.Fatalf("6th registration: %v", err)
	}
	// Separate budget for sign-in; IPv6 bucketed by /64.
	a := app.FlowClient{IP: netip.MustParseAddr("2001:db8::1")}
	b := app.FlowClient{IP: netip.MustParseAddr("2001:db8::ffff")}
	for i := range 20 {
		in := creds("email", fmt.Sprintf("s%d@example.com", i), tPassword)
		in.Client = a
		_, _ = l.auth.Login(ctx, in)
	}
	in := creds("email", tEmail, tPassword)
	in.Client = b
	if _, err := l.auth.Login(ctx, in); !errors.Is(err, app.ErrRateLimited) {
		t.Fatalf("same /64 must share the budget: %v", err)
	}
	if _, err := l.auth.StartRecovery(ctx, app.RecoveryRequest{Client: b, Type: "email", Value: tEmail}); !errors.Is(err, app.ErrRateLimited) {
		t.Fatalf("recovery shares the sign-in budget: %v", err)
	}
}

func TestPLXNFR02_FailedSignInsPerAccount(t *testing.T) {
	l := newLoginEnv(t)
	ctx := context.Background()
	l.register(t, "email", tEmail)
	l.auth.SignInLimiter, l.auth.NetLimiter = nil, nil // attempts from many networks
	for range 10 {
		if _, err := l.auth.Login(ctx, creds("email", tEmail, "wrong password!")); flowIDs(err) == nil {
			t.Fatalf("expected a rejection: %v", err)
		}
	}
	if _, err := l.auth.Login(ctx, creds("email", tEmail, tPassword)); !errors.Is(err, app.ErrRateLimited) {
		t.Fatalf("11th attempt on the account: %v", err)
	}
	if _, err := l.auth.Login(ctx, creds("email", "other@example.com", tPassword)); errors.Is(err, app.ErrRateLimited) {
		t.Fatal("other accounts keep their budget")
	}
	// The window slides: the account signs in again later.
	l.clock.T = l.clock.T.Add(16 * time.Minute)
	if _, err := l.auth.Login(ctx, creds("email", tEmail, tPassword)); err != nil {
		t.Fatal(err)
	}
}

func TestPLXNFR02_AccountLimitHoldsUnderConcurrency(t *testing.T) {
	l := newLoginEnv(t)
	l.svc.Phase = app.PhaseComplete // one Kratos call per attempt
	l.register(t, "email", tEmail)
	l.auth.SignInLimiter, l.auth.NetLimiter = nil, nil
	var wg sync.WaitGroup
	var mu sync.Mutex
	limited := 0
	for range 40 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := l.auth.Login(context.Background(), creds("email", tEmail, "wrong password!"))
			if errors.Is(err, app.ErrRateLimited) {
				mu.Lock()
				limited++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if got := len(l.flows.CallsOf("login")); got != 10 || limited != 30 {
		t.Fatalf("Kratos saw %d attempts, %d limited; want 10 / 30", got, limited)
	}
}

func TestPLINFR06_AuthFailsClosed(t *testing.T) {
	l := newLoginEnv(t)
	l.kms.SetFailure(app.ErrDependencyUnavailable)
	_, err := l.auth.Login(context.Background(), creds("email", tEmail, tPassword))
	if !errors.Is(err, app.ErrDependencyUnavailable) || len(l.flows.Calls) != 0 {
		t.Fatalf("got %v", err)
	}
	l.kms.SetFailure(nil)
	l.flows.Err = app.ErrDependencyUnavailable
	if _, err := l.auth.StartRecovery(context.Background(), app.RecoveryRequest{Client: tClient, Type: "email", Value: tEmail}); !errors.Is(err, app.ErrDependencyUnavailable) {
		t.Fatalf("kratos down: %v", err)
	}
}

func TestPLIFR04_ValidateRegistration(t *testing.T) {
	l := newLoginEnv(t)
	ctx := context.Background()
	if err := l.svc.ValidateRegistration(ctx, app.RegistrationTraits{LegacyEmail: tEmail}); !errors.Is(err, app.ErrLoginLegacyTraits) {
		t.Fatalf("legacy traits: %v", err)
	}
	decoy, _ := login.NewPseudonym() // well-shaped, no vault row
	for _, id := range []string{"", tEmail, decoy.String()} {
		if err := l.svc.ValidateRegistration(ctx, app.RegistrationTraits{LoginID: id}); !errors.Is(err, app.ErrLoginUnresolved) {
			t.Fatalf("%q: %v", id, err)
		}
	}
	p := l.register(t, "email", tEmail)
	l.clock.T = l.clock.T.Add(time.Hour)
	if err := l.svc.ValidateRegistration(ctx, app.RegistrationTraits{LoginID: p}); err != nil {
		t.Fatal(err)
	}
	ps, _ := login.ParsePseudonym(p)
	if !l.store.Logins[ps].LastValidatedAt.Equal(l.clock.T) {
		t.Fatal("validation must touch the row (A10)")
	}
	// Migration window: a legacy customer already signs in with the address.
	l.ids.AddCustomer(tEmail, true)
	if err := l.svc.ValidateRegistration(ctx, app.RegistrationTraits{LoginID: p}); !errors.Is(err, app.ErrLoginDuplicate) {
		t.Fatalf("legacy collision: %v", err)
	}
	l.svc.Phase = app.PhaseComplete
	if err := l.svc.ValidateRegistration(ctx, app.RegistrationTraits{LoginID: p}); err != nil {
		t.Fatalf("after migration the legacy check is off: %v", err)
	}
}

func TestPLIFR16_BindAndOwn(t *testing.T) {
	l := newLoginEnv(t)
	ctx := context.Background()
	p := l.register(t, "phone", tLoginPhone)
	cust := l.ids.AddCustomer(p, true)
	pr := identity.Principal{IdentityID: cust.ID, Kind: identity.KindCustomer, LoginID: p}
	// Webhook missed: /v1/me binds lazily.
	id, err := l.svc.Own(ctx, pr)
	if err != nil || id.Kind() != login.KindPhone || id.Value() != "+84912345678" {
		t.Fatalf("own: %v", err)
	}
	ps, _ := login.ParsePseudonym(p)
	if got := l.store.Logins[ps].IdentityID; got == nil || *got != cust.ID {
		t.Fatal("not bound")
	}
	// Another live identity cannot take the row.
	other := l.ids.AddCustomer(p, true)
	if _, err := l.svc.Own(ctx, identity.Principal{IdentityID: other.ID, Kind: identity.KindCustomer, LoginID: p}); !errors.Is(err, app.ErrDataIntegrity) {
		t.Fatalf("bound elsewhere: %v", err)
	}
	// A6: the first identity was deleted directly in Kratos → re-bind.
	delete(l.ids.M, cust.ID)
	if err := l.svc.Bind(ctx, other.ID, p); err != nil {
		t.Fatalf("stale rebind: %v", err)
	}
	if got := l.store.Logins[ps].IdentityID; *got != other.ID {
		t.Fatal("stale binding not replaced")
	}
	// Legacy customers read their plaintext email trait.
	legacy, err := l.svc.Own(ctx, identity.Principal{IdentityID: uuid.New(), Kind: identity.KindCustomer, LoginID: tEmail})
	if err != nil || legacy.Value() != tEmail {
		t.Fatalf("legacy: %v", err)
	}
	if _, err := l.svc.Own(ctx, identity.Principal{Kind: identity.KindAdmin}); !errors.Is(err, app.ErrForbidden) {
		t.Fatal("admins have no customer login")
	}
}

func TestPLIFR10_MaskManyOneBatch(t *testing.T) {
	l := newLoginEnv(t)
	var its []identity.Identity
	for _, v := range []string{"a1@example.com", "b2@example.vn", "c3@example.org"} {
		its = append(its, l.ids.AddCustomer(l.register(t, "email", v), true))
	}
	its = append(its, l.ids.AddCustomer(tEmail, false)) // legacy
	out, unavailable := l.svc.MaskMany(context.Background(), its)
	if unavailable || len(out) != 4 {
		t.Fatalf("masked %d unavailable=%v", len(out), unavailable)
	}
	if m := out[its[1].ID]; m.Kind != login.KindEmail || m.Masked != "b***@e***.vn" {
		t.Fatalf("mask %+v", m)
	}
	if m := out[its[3].ID]; m.Masked != "a***@e***.com" {
		t.Fatalf("legacy mask %+v", m)
	}
	l.kms.SetFailure(app.ErrDependencyUnavailable)
	out, unavailable = l.svc.MaskMany(context.Background(), its)
	if !unavailable || len(out) != 1 {
		t.Fatalf("key manager down: %d %v", len(out), unavailable)
	}
}

func TestPLIFR11_FindCustomers(t *testing.T) {
	l := newLoginEnv(t)
	ctx := context.Background()
	p := l.register(t, "email", tEmail)
	c := l.ids.AddCustomer(p, true)
	l.ids.Add(identity.Identity{SchemaID: "admin", Email: tEmail, LoginID: tEmail})
	legacy := l.ids.AddCustomer("legacy@example.com", true)
	got, err := l.svc.FindCustomers(ctx, "email", strings.ToUpper(tEmail))
	if err != nil || len(got) != 1 || got[0].ID != c.ID {
		t.Fatalf("find: %v %v", got, err)
	}
	got, _ = l.svc.FindCustomers(ctx, "email", "legacy@example.com")
	if len(got) != 1 || got[0].ID != legacy.ID {
		t.Fatal("transition lookup must find legacy customers")
	}
	if _, err := l.svc.FindCustomers(ctx, "phone", "+1 202 555 0100"); err != nil {
		t.Fatalf("lookups accept any country: %v", err)
	}
	if code := fieldCode(func() error { _, err := l.svc.FindCustomers(ctx, "email", "x"); return err }()); code != "login.value:invalid_format" {
		t.Fatalf("invalid lookup: %s", code)
	}
}

func TestPLIFR15_PurgeUnboundAndOrphans(t *testing.T) {
	l := newLoginEnv(t)
	ctx := context.Background()
	gone := l.register(t, "email", "gone@example.com")
	webhookMissed := l.register(t, "email", "missed@example.com")
	fresh := ""
	missedOwner := l.ids.AddCustomer(webhookMissed, false)
	orphanP := l.register(t, "email", "orphan@example.com")
	orphan := l.ids.AddCustomer(orphanP, true)
	if err := l.svc.Bind(ctx, orphan.ID, orphanP); err != nil {
		t.Fatal(err)
	}
	delete(l.ids.M, orphan.ID) // deleted directly in Kratos
	l.clock.T = l.clock.T.Add(25 * time.Hour)
	fresh = l.register(t, "email", "fresh@example.com")

	dry, err := l.svc.Purge(ctx, 24*time.Hour, true)
	if err != nil || dry.UnboundDeleted != 1 || dry.Rebound != 1 || dry.OrphanDeleted != 1 || len(l.store.Logins) != 4 {
		t.Fatalf("dry run %+v %v (rows %d)", dry, err, len(l.store.Logins))
	}
	res, err := l.svc.Purge(ctx, 24*time.Hour, false)
	if err != nil || res != dry {
		t.Fatalf("purge %+v %v", res, err)
	}
	for _, s := range []string{gone, orphanP} {
		p, _ := login.ParsePseudonym(s)
		if _, ok := l.store.Logins[p]; ok {
			t.Fatalf("%s not purged", s[:8])
		}
	}
	p, _ := login.ParsePseudonym(webhookMissed)
	if got := l.store.Logins[p].IdentityID; got == nil || *got != missedOwner.ID {
		t.Fatal("unbound row with a Kratos identity must be bound, not deleted")
	}
	if p, _ := login.ParsePseudonym(fresh); l.store.Logins[p].Kind == "" {
		t.Fatal("fresh unbound row purged")
	}
	if acts := l.store.AuditActions(); len(acts) != 1 || acts[0] != audit.ActionCustomerLoginErased {
		t.Fatalf("audit %v", acts)
	}
}

func TestPLINFR03_RewrapAndTamper(t *testing.T) {
	l := newLoginEnv(t)
	ctx := context.Background()
	a := l.ids.AddCustomer(l.register(t, "email", "a1@example.com"), true)
	b := l.ids.AddCustomer(l.register(t, "phone", tLoginPhone), true)
	l.kms.Rotate()
	n, err := l.svc.Rewrap(ctx)
	if err != nil || n != 2 {
		t.Fatalf("rewrap %d %v", n, err)
	}
	for _, rec := range l.store.Logins {
		if rec.KEKVersion != 2 {
			t.Fatal("not re-wrapped")
		}
	}
	if n, _ := l.svc.Rewrap(ctx); n != 0 {
		t.Fatal("second rewrap must be a no-op")
	}
	// Swap the two ciphertexts: AAD binding makes both fail (masked view drops them).
	pa, _ := login.ParsePseudonym(a.LoginID)
	pb, _ := login.ParsePseudonym(b.LoginID)
	ra, rb := l.store.Logins[pa], l.store.Logins[pb]
	ra.Ciphertext, rb.Ciphertext = rb.Ciphertext, ra.Ciphertext
	l.store.Logins[pa], l.store.Logins[pb] = ra, rb
	out, _ := l.svc.MaskMany(ctx, []identity.Identity{a, b})
	if len(out) != 0 {
		t.Fatalf("swapped ciphertexts decrypted: %v", out)
	}
	if _, err := l.svc.Reveal(ctx, a); !errors.Is(err, app.ErrDataIntegrity) {
		t.Fatalf("reveal swapped: %v", err)
	}
}

func TestPLIFR12_RevealIncludesLoginAndAuditsFieldName(t *testing.T) {
	p := newPIIEnv(t)
	l := &app.LoginIdentifierService{Keys: p.kms, Logins: p.store.Repos().Logins, Tx: p.store, Identities: p.ids,
		Phone: vnPol, PhoneEnabled: true, Phase: app.PhaseComplete, Clock: p.clock}
	p.svc.Logins = l
	ctx := context.Background()
	auth := &app.CustomerAuthService{Logins: l, Flows: testutil.NewAuthFlows()}
	if _, err := auth.Register(ctx, app.Credentials{Client: tClient, Type: "email", Value: tEmail, Password: tPassword}); err != nil {
		t.Fatal(err)
	}
	c := p.ids.AddCustomer(auth.Flows.(*testutil.AuthFlows).CallsOf("register")[0].Identifier, true)
	adm := p.admin(identity.RoleAdmin)
	v, err := p.svc.Reveal(ctx, adm, c.ID, app.RevealRequest{ReasonCode: app.ReasonCustomerSupportRequest, Fields: []string{"login"}})
	if err != nil || v.Login == nil || v.Login.Value() != tEmail || v.UpdatedAt != nil {
		t.Fatalf("reveal login only: %+v %v", v, err)
	}
	v, err = p.svc.Reveal(ctx, adm, c.ID, app.RevealRequest{ReasonCode: app.ReasonCustomerSupportRequest})
	if err != nil || v.Login == nil {
		t.Fatalf("reveal all must include login: %v", err)
	}
	if _, err := p.svc.Reveal(ctx, adm, c.ID, app.RevealRequest{ReasonCode: app.ReasonCustomerSupportRequest, Fields: []string{"login", "login"}}); err == nil {
		t.Fatal("duplicate field accepted")
	}
	ev := p.store.Events[0]
	if f, _ := ev.Details["fields"].([]string); ev.Action != audit.ActionCustomerPIIRevealed || len(f) != 1 || f[0] != "login" {
		t.Fatalf("audit %+v", ev)
	}
	for _, e := range p.store.Events {
		if strings.Contains(fmt.Sprint(e.Details), "alice") {
			t.Fatal("audit holds the address")
		}
	}

	got, err := p.svc.LookupByLogin(ctx, p.admin(identity.RoleSupport), "email", strings.ToUpper(tEmail))
	if err != nil || len(got.Matches) != 1 || got.Matches[0].Login == nil || got.Matches[0].Login.Masked != "a***@e***.com" {
		t.Fatalf("lookup %+v %v", got, err)
	}
	if last := p.store.Events[len(p.store.Events)-1]; last.Action != audit.ActionCustomerLoginLookup {
		t.Fatalf("lookup audit %s", last.Action)
	}
}

func TestSECC03_InsertCapNeverBlocksSignUp(t *testing.T) {
	l := newLoginEnv(t)
	l.svc.InsertLimiter = app.NewKeyedLimiter[string]([]app.RateRule{{Limit: 1, Window: time.Minute}}, l.clock.Now)
	for _, v := range []string{"a1@example.com", "a2@example.com", "a3@example.com"} {
		l.register(t, "email", v)
	}
	if len(l.store.Logins) != 3 || !strings.Contains(l.logs.String(), "login_insert_rate_high") {
		t.Fatal("the global insert cap must alert, not refuse")
	}
	// Aggregate limit per /24: rotating addresses in one network shares a budget.
	l.auth.NetLimiter = app.NewKeyedLimiter[string]([]app.RateRule{{Limit: 2, Window: time.Minute}}, l.clock.Now)
	ctx := context.Background()
	var last error
	for i := range 3 {
		in := creds("email", tEmail, tPassword)
		in.Client.IP = netip.AddrFrom4([4]byte{198, 51, 100, byte(10 + i)})
		_, last = l.auth.Login(ctx, in)
	}
	if !errors.Is(last, app.ErrRateLimited) {
		t.Fatalf("net bucket: %v", last)
	}
}

func TestSECC10_PurgeAgeFloor(t *testing.T) {
	l := newLoginEnv(t)
	if _, err := l.svc.Purge(context.Background(), time.Hour, true); err == nil {
		t.Fatal("purge below 24h must be refused")
	}
}
