package app_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/adapter/localkms"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/audit"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/identity"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/login"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/platform"
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
	kms  *localkms.KMS
	logs *bytes.Buffer
	svc  *app.LoginIdentifierService
}

func newLoginEnv(t *testing.T) *loginEnv {
	t.Helper()
	e := newEnv()
	l := &loginEnv{env: e, kms: localkms.NewRandom(), logs: &bytes.Buffer{}}
	l.svc = &app.LoginIdentifierService{
		Keys: l.kms, Logins: e.store.Repos().Logins, Tx: e.store, Identities: e.ids,
		Phone: vnPol, PhoneEnabled: true, Phase: app.PhaseTransition, Clock: e.clock,
		Log:             platform.NewLogger(l.logs, "debug"),
		ResolveLimiter:  app.NewKeyedLimiter[string](app.ResolveRateRules, e.clock.Now),
		RegisterLimiter: app.NewKeyedLimiter[string](app.RegisterRateRules, e.clock.Now),
		InsertLimiter:   app.NewKeyedLimiter[string](app.InsertGlobalRules, e.clock.Now),
	}
	t.Cleanup(func() {
		for _, v := range []string{tEmail, "912345678", "alice.login"} {
			if strings.Contains(l.logs.String(), v) {
				t.Errorf("logs leak %q: %s", v, l.logs.String())
			}
		}
	})
	return l
}

func (l *loginEnv) resolve(t *testing.T, typ, value, purpose string) string {
	t.Helper()
	p, err := l.svc.Resolve(context.Background(), app.ResolveRequest{ClientIP: tIP, Type: typ, Value: value, Purpose: purpose})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	return p
}

func fieldCode(err error) string {
	var ve *app.ValidationError
	if errors.As(err, &ve) && len(ve.Fields) > 0 {
		return ve.Fields[0].Field + ":" + ve.Fields[0].Code
	}
	return ""
}

func TestPLIFR01_ResolveDeterministicAndTyped(t *testing.T) {
	l := newLoginEnv(t)
	a := l.resolve(t, "email", "  Alice.Login@EXAMPLE.com", "sign_in")
	b := l.resolve(t, "email", tEmail, "recovery")
	if a != b || !login.IsPseudonym(a) {
		t.Fatalf("not deterministic: %s %s", a, b)
	}
	ph := l.resolve(t, "phone", tLoginPhone, "sign_in")
	ph2 := l.resolve(t, "phone", "+84912345678", "verification")
	if ph != ph2 || ph == a {
		t.Fatal("phone pseudonym must be stable and distinct from email")
	}
	if len(l.store.Logins) != 0 {
		t.Fatal("non-registration purposes must not store anything (PLI-FR-02)")
	}
}

func TestPLIFR01_ResolveValidation(t *testing.T) {
	l := newLoginEnv(t)
	ctx := context.Background()
	for _, tc := range []struct{ typ, value, purpose, want string }{
		{"username", "x", "sign_in", "type:invalid"},
		{"email", tEmail, "delete", "purpose:invalid"},
		{"email", "not-an-email", "sign_in", "value:invalid_format"},
		{"phone", "+1 202 555 0100", "sign_in", "value:unsupported_country"},
	} {
		_, err := l.svc.Resolve(ctx, app.ResolveRequest{ClientIP: tIP, Type: tc.typ, Value: tc.value, Purpose: tc.purpose})
		if got := fieldCode(err); got != tc.want {
			t.Errorf("%s/%s: %q (%v)", tc.typ, tc.purpose, got, err)
		}
		if err != nil && strings.Contains(err.Error(), tc.value) {
			t.Errorf("error echoes the value: %v", err)
		}
	}
	l.svc.PhoneEnabled = false
	_, err := l.svc.Resolve(ctx, app.ResolveRequest{ClientIP: tIP, Type: "phone", Value: tLoginPhone, Purpose: "sign_in"})
	if got := fieldCode(err); got != "type:unsupported" {
		t.Fatalf("phone without SMS channel: %q", got)
	}
}

func TestPLIFR02_RegistrationStoresSealedOnce(t *testing.T) {
	l := newLoginEnv(t)
	p1 := l.resolve(t, "email", tEmail, "registration")
	p2 := l.resolve(t, "email", tEmail, "registration")
	if p1 != p2 || len(l.store.Logins) != 1 {
		t.Fatalf("want one row, got %d", len(l.store.Logins))
	}
	for _, rec := range l.store.Logins {
		if rec.IdentityID != nil || rec.Kind != login.KindEmail || strings.Contains(rec.Ciphertext, "alice") {
			t.Fatalf("row %+v", rec)
		}
	}
}

func TestPLINFR05_ResolveRateLimits(t *testing.T) {
	l := newLoginEnv(t)
	ctx := context.Background()
	for i := range 5 {
		if _, err := l.svc.Resolve(ctx, app.ResolveRequest{ClientIP: tIP, Type: "email", Value: tEmail, Purpose: "registration"}); err != nil {
			t.Fatalf("registration %d: %v", i, err)
		}
	}
	if _, err := l.svc.Resolve(ctx, app.ResolveRequest{ClientIP: tIP, Type: "email", Value: tEmail, Purpose: "registration"}); !errors.Is(err, app.ErrRateLimited) {
		t.Fatalf("6th registration resolve: %v", err)
	}
	// Separate budget for sign-in; IPv6 bucketed by /64.
	a := netip.MustParseAddr("2001:db8::1")
	b := netip.MustParseAddr("2001:db8::ffff")
	for range 20 {
		_, _ = l.svc.Resolve(ctx, app.ResolveRequest{ClientIP: a, Type: "email", Value: tEmail, Purpose: "sign_in"})
	}
	if _, err := l.svc.Resolve(ctx, app.ResolveRequest{ClientIP: b, Type: "email", Value: tEmail, Purpose: "sign_in"}); !errors.Is(err, app.ErrRateLimited) {
		t.Fatalf("same /64 must share the budget: %v", err)
	}
	if _, err := l.svc.Resolve(ctx, app.ResolveRequest{ClientIP: tIP, Type: "email", Value: tEmail, Purpose: "sign_in"}); err != nil {
		t.Fatalf("other client: %v", err)
	}
}

func TestPLINFR06_ResolveFailsClosed(t *testing.T) {
	l := newLoginEnv(t)
	l.kms.SetFailure(app.ErrDependencyUnavailable)
	_, err := l.svc.Resolve(context.Background(), app.ResolveRequest{ClientIP: tIP, Type: "email", Value: tEmail, Purpose: "sign_in"})
	if !errors.Is(err, app.ErrDependencyUnavailable) {
		t.Fatalf("got %v", err)
	}
}

func TestPLIFR04_ValidateRegistration(t *testing.T) {
	l := newLoginEnv(t)
	ctx := context.Background()
	if err := l.svc.ValidateRegistration(ctx, app.RegistrationTraits{LegacyEmail: tEmail}); !errors.Is(err, app.ErrLoginLegacyTraits) {
		t.Fatalf("legacy traits: %v", err)
	}
	unknown := l.resolve(t, "email", "nobody@example.com", "sign_in") // well-shaped, no vault row
	for _, id := range []string{"", tEmail, unknown} {
		if err := l.svc.ValidateRegistration(ctx, app.RegistrationTraits{LoginID: id}); !errors.Is(err, app.ErrLoginUnresolved) {
			t.Fatalf("%q: %v", id, err)
		}
	}
	p := l.resolve(t, "email", tEmail, "registration")
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
	p := l.resolve(t, "phone", tLoginPhone, "registration")
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
		its = append(its, l.ids.AddCustomer(l.resolve(t, "email", v, "registration"), true))
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
	p := l.resolve(t, "email", tEmail, "registration")
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
	gone := l.resolve(t, "email", "gone@example.com", "registration")
	webhookMissed := l.resolve(t, "email", "missed@example.com", "registration")
	fresh := ""
	missedOwner := l.ids.AddCustomer(webhookMissed, false)
	orphanP := l.resolve(t, "email", "orphan@example.com", "registration")
	orphan := l.ids.AddCustomer(orphanP, true)
	if err := l.svc.Bind(ctx, orphan.ID, orphanP); err != nil {
		t.Fatal(err)
	}
	delete(l.ids.M, orphan.ID) // deleted directly in Kratos
	l.clock.T = l.clock.T.Add(25 * time.Hour)
	fresh = l.resolve(t, "email", "fresh@example.com", "registration")

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
	a := l.ids.AddCustomer(l.resolve(t, "email", "a1@example.com", "registration"), true)
	b := l.ids.AddCustomer(l.resolve(t, "phone", tLoginPhone, "registration"), true)
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
	ps, err := l.Resolve(ctx, app.ResolveRequest{ClientIP: tIP, Type: "email", Value: tEmail, Purpose: "registration"})
	if err != nil {
		t.Fatal(err)
	}
	c := p.ids.AddCustomer(ps, true)
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
	l.svc.RegisterLimiter = nil
	l.svc.InsertLimiter = app.NewKeyedLimiter[string]([]app.RateRule{{Limit: 1, Window: time.Minute}}, l.clock.Now)
	for _, v := range []string{"a1@example.com", "a2@example.com", "a3@example.com"} {
		l.resolve(t, "email", v, "registration")
	}
	if len(l.store.Logins) != 3 || !strings.Contains(l.logs.String(), "login_insert_rate_high") {
		t.Fatal("the global insert cap must alert, not refuse")
	}
	// Aggregate limit per /24: rotating addresses in one network shares a budget.
	l.svc.NetLimiter = app.NewKeyedLimiter[string]([]app.RateRule{{Limit: 2, Window: time.Minute}}, l.clock.Now)
	ctx := context.Background()
	var last error
	for i := range 3 {
		ip := netip.AddrFrom4([4]byte{198, 51, 100, byte(10 + i)})
		_, last = l.svc.Resolve(ctx, app.ResolveRequest{ClientIP: ip, Type: "email", Value: tEmail, Purpose: "sign_in"})
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
