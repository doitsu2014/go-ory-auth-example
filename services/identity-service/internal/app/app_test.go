package app_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/audit"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/identity"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/profile"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/testutil"
)

var t0 = time.Date(2026, 10, 3, 8, 0, 0, 0, time.UTC)

type env struct {
	clock  *testutil.FixedClock
	ids    *testutil.Identities
	store  *testutil.Store
	authz  *testutil.Authz
	roles  *testutil.Roles
	mail   *testutil.Mailer
	verify *testutil.Verifier
}

func newEnv() *env {
	c := &testutil.FixedClock{T: t0}
	return &env{
		clock: c, ids: testutil.NewIdentities(c), store: testutil.NewStore(c), authz: testutil.NewAuthz(),
		roles: testutil.NewRoles(), mail: &testutil.Mailer{}, verify: testutil.NewVerifier(),
	}
}

func (e *env) customers() *app.CustomerService {
	r := e.store.Repos()
	return &app.CustomerService{Authz: e.authz, Identities: e.ids, Profiles: r.Profiles, Tx: e.store, Sessions: e.verify}
}

func (e *env) admins() *app.AdminService {
	return &app.AdminService{Authz: e.authz, Roles: e.roles, Identities: e.ids, Tx: e.store,
		Idempotency: e.store.Repos().Idempotency, Mailer: e.mail, Clock: e.clock}
}

func (e *env) gate() *app.AdminGate {
	return &app.AdminGate{Identities: e.ids, Sessions: e.verify, Tx: e.store, Clock: e.clock}
}

func adminActor(id uuid.UUID) app.Actor {
	return app.Actor{RequestID: "req-1", Principal: identity.Principal{
		IdentityID: id, SessionID: uuid.New(), Kind: identity.KindAdmin, AAL: identity.AAL2,
		AuthenticatedAt: t0.Add(-time.Hour), ExpiresAt: t0.Add(time.Hour),
	}}
}

func customerPrincipal(verified bool) identity.Principal {
	return identity.Principal{IdentityID: uuid.New(), Kind: identity.KindCustomer, AAL: identity.AAL1,
		Email: "an@example.com", EmailVerified: verified, ExpiresAt: t0.Add(time.Hour)}
}

func ptr[T any](v T) *T { return &v }

// --- FR-09 / FR-10: profile ---

func TestFR09_GetMeCreatesProfileLazilyAndIdempotently(t *testing.T) {
	e := newEnv()
	svc := &app.MeService{Profiles: e.store.Repos().Profiles}
	p := customerPrincipal(true)
	for range 2 {
		v, err := svc.GetMe(context.Background(), p)
		if err != nil {
			t.Fatal(err)
		}
		if v.Profile.IdentityID != p.IdentityID || v.Profile.Locale != profile.DefaultLocale {
			t.Fatalf("unexpected profile %+v", v.Profile)
		}
	}
	if len(e.store.Profiles) != 1 {
		t.Fatalf("want 1 profile, got %d", len(e.store.Profiles))
	}
}

func TestFR10_UpdateMe(t *testing.T) {
	long := string(make([]rune, 101))
	tests := []struct {
		name     string
		p        identity.Principal
		patch    profile.Patch
		wantErr  error
		wantCode string
	}{
		{name: "unverified email", p: customerPrincipal(false), patch: profile.Patch{Locale: ptr("en-US")}, wantErr: app.ErrEmailNotVerified},
		{name: "admin principal", p: identity.Principal{IdentityID: uuid.New(), Kind: identity.KindAdmin, EmailVerified: true}, wantErr: app.ErrForbidden},
		{name: "too long", p: customerPrincipal(true), patch: profile.Patch{DisplayName: profile.OptionalString{Set: true, Value: &long}}, wantCode: "too_long"},
		{name: "bad avatar scheme", p: customerPrincipal(true), patch: profile.Patch{AvatarURL: profile.OptionalString{Set: true, Value: ptr("javascript:alert(1)")}}, wantCode: "invalid_url"},
		{name: "ok", p: customerPrincipal(true), patch: profile.Patch{DisplayName: profile.OptionalString{Set: true, Value: ptr("An N.")}, Locale: ptr("en-US")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv()
			svc := &app.MeService{Profiles: e.store.Repos().Profiles}
			v, err := svc.UpdateMe(context.Background(), tt.p, tt.patch)
			var ve *app.ValidationError
			switch {
			case tt.wantErr != nil:
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("want %v, got %v", tt.wantErr, err)
				}
			case tt.wantCode != "":
				if !errors.As(err, &ve) || ve.Fields[0].Code != tt.wantCode {
					t.Fatalf("want validation %s, got %v", tt.wantCode, err)
				}
			default:
				if err != nil {
					t.Fatal(err)
				}
				if *v.Profile.DisplayName != "An N." || v.Profile.Locale != "en-US" {
					t.Fatalf("not applied: %+v", v.Profile)
				}
			}
		})
	}
}

func TestFR10_UpdateMeNullClearsField(t *testing.T) {
	e := newEnv()
	svc := &app.MeService{Profiles: e.store.Repos().Profiles}
	p := customerPrincipal(true)
	if _, err := svc.UpdateMe(context.Background(), p, profile.Patch{DisplayName: profile.OptionalString{Set: true, Value: ptr("x")}}); err != nil {
		t.Fatal(err)
	}
	v, err := svc.UpdateMe(context.Background(), p, profile.Patch{DisplayName: profile.OptionalString{Set: true}})
	if err != nil || v.Profile.DisplayName != nil {
		t.Fatalf("want cleared, got %v %v", v.Profile.DisplayName, err)
	}
}

func TestFR09_RegistrationWebhookProvisionsProfile(t *testing.T) {
	e := newEnv()
	svc := &app.ProvisioningService{Profiles: e.store.Repos().Profiles}
	id := uuid.New()
	for range 2 { // webhook retry
		if err := svc.HandleRegistration(context.Background(), id, "customer"); err != nil {
			t.Fatal(err)
		}
	}
	if e.store.Profiles[id].Kind != identity.KindCustomer {
		t.Fatal("profile not created with kind customer")
	}
	var ve *app.ValidationError
	if err := svc.HandleRegistration(context.Background(), uuid.New(), "intruder"); !errors.As(err, &ve) {
		t.Fatalf("unknown schema must fail validation, got %v", err)
	}
}

func TestFR05_LoginPopulationGuard(t *testing.T) {
	tests := []struct {
		schema, flow string
		want         bool
	}{
		{"customer", "api", true}, {"customer", "browser", false},
		{"admin", "browser", true}, {"admin", "api", false},
		{"other", "api", false}, {"other", "browser", false},
	}
	for _, tt := range tests {
		if got := app.LoginAllowed(tt.schema, tt.flow); got != tt.want {
			t.Errorf("LoginAllowed(%s,%s)=%v want %v", tt.schema, tt.flow, got, tt.want)
		}
	}
}

// --- FR-06: admin gate ---

func TestFR06_AdminGate(t *testing.T) {
	deactivated := func(t *testing.T, e *env, a app.Actor) {
		t.Helper()
		if e.ids.M[a.Principal.IdentityID].State != identity.StateInactive {
			t.Fatal("identity not deactivated")
		}
		if len(e.ids.RevokedAll) != 1 || len(e.verify.Invalidated) == 0 {
			t.Fatal("sessions not revoked / cache not invalidated")
		}
		if acts := e.store.AuditActions(); len(acts) != 1 || acts[0] != audit.ActionAdminDeactivated {
			t.Fatalf("audit missing: %v", acts)
		}
	}
	tests := []struct {
		name       string
		mutate     func(*app.Actor)
		totpAgo    time.Duration // >0: TOTP enrolled this long ago
		createdAgo time.Duration
		allowAAL1  bool
		want       error
		check      func(t *testing.T, e *env, a app.Actor)
	}{
		{name: "aal2 with timely totp", createdAgo: 48 * time.Hour, totpAgo: 47 * time.Hour, want: nil},
		{name: "customer", mutate: func(a *app.Actor) { a.Principal.Kind = identity.KindCustomer }, want: app.ErrNotAdmin},
		{
			name: "older than 12h is revoked", totpAgo: time.Minute, mutate: func(a *app.Actor) { a.Principal.AuthenticatedAt = t0.Add(-13 * time.Hour) },
			want: app.ErrUnauthenticated,
			check: func(t *testing.T, e *env, a app.Actor) {
				if len(e.ids.RevokedSessions) != 1 || e.ids.RevokedSessions[0] != a.Principal.SessionID {
					t.Fatal("session not revoked in Kratos")
				}
				if len(e.verify.Invalidated) != 1 {
					t.Fatal("cache not invalidated")
				}
			},
		},
		{name: "aal1 with totp on me", mutate: aal1, totpAgo: time.Minute, allowAAL1: true, want: nil},
		{name: "aal1 with totp elsewhere", mutate: aal1, totpAgo: time.Minute, want: app.ErrAAL2Required},
		{name: "aal1 no totp within grace", mutate: aal1, createdAgo: 2 * time.Hour, allowAAL1: true, want: app.ErrMFAEnrollmentRequired},
		{name: "aal1 no totp after grace deactivates", mutate: aal1, createdAgo: 25 * time.Hour, allowAAL1: true, want: app.ErrForbidden, check: deactivated},
		// S1 regression: password-only attacker enrols TOTP at Kratos after the
		// deadline and arrives at AAL2.
		{name: "aal2 with totp enrolled after deadline deactivates", createdAgo: 30 * time.Hour, totpAgo: 2 * time.Hour, want: app.ErrForbidden, check: deactivated},
		{name: "aal2 without totp after deadline deactivates", createdAgo: 30 * time.Hour, want: app.ErrForbidden, check: deactivated},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv()
			ident := identity.Identity{SchemaID: "admin", CreatedAt: t0.Add(-tt.createdAgo)}
			if tt.totpAgo > 0 {
				ident.HasTOTP, ident.TOTPCreatedAt = true, t0.Add(-tt.totpAgo)
			}
			ident = e.ids.Add(ident)
			a := adminActor(ident.ID)
			if tt.mutate != nil {
				tt.mutate(&a)
			}
			err := e.gate().Check(context.Background(), a, tt.allowAAL1)
			if !errors.Is(err, tt.want) && !(tt.want == nil && err == nil) {
				t.Fatalf("want %v, got %v", tt.want, err)
			}
			if tt.check != nil {
				tt.check(t, e, a)
			}
		})
	}
}

func TestFR06_AdminGateReactivationOpensNewWindow(t *testing.T) {
	e := newEnv()
	ident := e.ids.Add(identity.Identity{SchemaID: "admin", CreatedAt: t0.Add(-30 * 24 * time.Hour),
		StateChangedAt: t0.Add(-time.Hour), HasTOTP: true, TOTPCreatedAt: t0.Add(-10 * time.Minute)})
	if err := e.gate().Check(context.Background(), adminActor(ident.ID), false); err != nil {
		t.Fatalf("TOTP enrolled within the window after reactivation: %v", err)
	}
}

func TestFR06_AdminGateDeactivationFailsClosedWhenAuditFails(t *testing.T) {
	e := newEnv()
	e.store.AuditErr = errors.New("db down")
	ident := e.ids.Add(identity.Identity{SchemaID: "admin", CreatedAt: t0.Add(-30 * time.Hour)})
	err := e.gate().Check(context.Background(), adminActor(ident.ID), false)
	if err == nil || errors.Is(err, app.ErrForbidden) {
		t.Fatalf("want internal error, got %v", err)
	}
	if e.ids.M[ident.ID].State != identity.StateActive {
		t.Fatal("no mutation may happen without an audit row")
	}
}

func TestFR06_MFASweeper(t *testing.T) {
	e := newEnv()
	late := e.ids.Add(identity.Identity{SchemaID: "admin", CreatedAt: t0.Add(-25 * time.Hour)})
	lateTOTP := e.ids.Add(identity.Identity{SchemaID: "admin", CreatedAt: t0.Add(-48 * time.Hour), HasTOTP: true, TOTPCreatedAt: t0.Add(-time.Hour)})
	fresh := e.ids.Add(identity.Identity{SchemaID: "admin", CreatedAt: t0.Add(-time.Hour)})
	ok := e.ids.Add(identity.Identity{SchemaID: "admin", CreatedAt: t0.Add(-48 * time.Hour), HasTOTP: true, TOTPCreatedAt: t0.Add(-47 * time.Hour)})
	cust := e.ids.Add(identity.Identity{SchemaID: "customer", CreatedAt: t0.Add(-48 * time.Hour)})
	n, err := e.gate().Sweep(context.Background())
	if err != nil || n != 2 {
		t.Fatalf("sweep: %d %v", n, err)
	}
	for id, want := range map[uuid.UUID]identity.State{
		late.ID: identity.StateInactive, lateTOTP.ID: identity.StateInactive,
		fresh.ID: identity.StateActive, ok.ID: identity.StateActive, cust.ID: identity.StateActive,
	} {
		if got := e.ids.M[id].State; got != want {
			t.Fatalf("%s: %s want %s", id, got, want)
		}
	}
	if n, _ := e.gate().Sweep(context.Background()); n != 0 {
		t.Fatal("sweep must be idempotent")
	}
}

func aal1(a *app.Actor) { a.Principal.AAL = identity.AAL1 }

// --- FR-11 / FR-12: customers ---

func TestFR11_CustomerUseCasesRequirePermissions(t *testing.T) {
	e := newEnv()
	svc := e.customers()
	cust := e.ids.Add(identity.Identity{SchemaID: "customer", Email: "c@example.com"})
	support := adminActor(uuid.New())
	e.authz.GrantRole(support.Principal.IdentityID, identity.RoleSupport)
	ctx := context.Background()

	if _, err := svc.Get(ctx, support, cust.ID); err != nil {
		t.Fatalf("support may view: %v", err)
	}
	if _, err := svc.List(ctx, support, app.CustomerQuery{}); err != nil {
		t.Fatalf("support may list: %v", err)
	}
	if _, err := svc.Disable(ctx, support, cust.ID, nil); !errors.Is(err, app.ErrForbidden) {
		t.Fatalf("support must not disable: %v", err)
	}
	if _, err := svc.Enable(ctx, support, cust.ID, nil); !errors.Is(err, app.ErrForbidden) {
		t.Fatalf("support must not enable: %v", err)
	}
	if err := svc.RevokeSessions(ctx, support, cust.ID); !errors.Is(err, app.ErrForbidden) {
		t.Fatalf("support must not revoke: %v", err)
	}
	nobody := adminActor(uuid.New())
	if _, err := svc.Get(ctx, nobody, cust.ID); !errors.Is(err, app.ErrForbidden) {
		t.Fatalf("no role must not view: %v", err)
	}
	custActor := app.Actor{Principal: customerPrincipal(true)}
	if _, err := svc.Get(ctx, custActor, cust.ID); !errors.Is(err, app.ErrNotAdmin) {
		t.Fatalf("customer must not view: %v", err)
	}
	if len(e.store.Events) != 0 {
		t.Fatal("denied calls must not audit")
	}
}

func TestFR11_DisableIsIdempotentRevokesAndAudits(t *testing.T) {
	e := newEnv()
	svc := e.customers()
	cust := e.ids.Add(identity.Identity{SchemaID: "customer"})
	a := adminActor(uuid.New())
	e.authz.GrantRole(a.Principal.IdentityID, identity.RoleAdmin)
	for range 2 {
		st, err := svc.Disable(context.Background(), a, cust.ID, ptr("fraud report #123"))
		if err != nil || st != identity.StateInactive {
			t.Fatalf("disable: %v %v", st, err)
		}
	}
	if e.ids.M[cust.ID].State != identity.StateInactive || len(e.ids.RevokedAll) != 2 || len(e.verify.Invalidated) != 2 {
		t.Fatal("disable side effects missing")
	}
	ev := e.store.Events[0]
	if ev.Action != audit.ActionCustomerDisabled || ev.ActorID != a.Principal.IdentityID || ev.TargetID != cust.ID.String() ||
		ev.RequestID != "req-1" || ev.Details["reason"] != "fraud report #123" {
		t.Fatalf("bad audit event %+v", ev)
	}
	if st, err := svc.Enable(context.Background(), a, cust.ID, nil); err != nil || st != identity.StateActive {
		t.Fatalf("enable: %v %v", st, err)
	}
}

func TestFR11_AdminTargetIsNotFound(t *testing.T) {
	e := newEnv()
	svc := e.customers()
	adm := e.ids.Add(identity.Identity{SchemaID: "admin"})
	a := adminActor(uuid.New())
	e.authz.GrantRole(a.Principal.IdentityID, identity.RoleSuperAdmin)
	ctx := context.Background()
	if _, err := svc.Get(ctx, a, adm.ID); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("get admin via customers: %v", err)
	}
	if _, err := svc.Disable(ctx, a, adm.ID, nil); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("disable admin via customers: %v", err)
	}
	if err := svc.RevokeSessions(ctx, a, uuid.New()); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("unknown id: %v", err)
	}
	if e.ids.M[adm.ID].State != identity.StateActive {
		t.Fatal("admin must be untouched")
	}
}

func TestFR11_ListFiltersCustomersAndState(t *testing.T) {
	e := newEnv()
	svc := e.customers()
	e.ids.Add(identity.Identity{SchemaID: "customer", State: identity.StateActive})
	e.ids.Add(identity.Identity{SchemaID: "customer", State: identity.StateInactive})
	e.ids.Add(identity.Identity{SchemaID: "admin"})
	a := adminActor(uuid.New())
	e.authz.GrantRole(a.Principal.IdentityID, identity.RoleSupport)
	page, err := svc.List(context.Background(), a, app.CustomerQuery{})
	if err != nil || len(page.Items) != 2 {
		t.Fatalf("want 2 customers, got %d %v", len(page.Items), err)
	}
	st := identity.StateInactive
	page, _ = svc.List(context.Background(), a, app.CustomerQuery{State: &st})
	if len(page.Items) != 1 {
		t.Fatalf("want 1 inactive, got %d", len(page.Items))
	}
}

// --- FR-07 / FR-12: admins ---

func TestFR07_InviteAdmin(t *testing.T) {
	e := newEnv()
	svc := e.admins()
	a := adminActor(uuid.New())
	e.authz.GrantRole(a.Principal.IdentityID, identity.RoleSuperAdmin)
	req := app.InviteRequest{Email: "New.Admin@Example.com", Name: identity.Name{First: "Binh"}, Role: identity.RoleSupport}

	out, err := svc.Invite(context.Background(), a, "key-1", req)
	if err != nil {
		t.Fatal(err)
	}
	if out.Email != "new.admin@example.com" || out.Role != identity.RoleSupport || !out.InvitationExpiresAt.Equal(t0.Add(24*time.Hour)) {
		t.Fatalf("bad result %+v", out)
	}
	if got := e.roles.M[out.ID]; len(got) != 1 || got[0] != identity.RoleSupport {
		t.Fatalf("role tuple: %v", got)
	}
	if len(e.mail.Sent) != 1 || e.mail.Sent[0].Link == "" || e.mail.Sent[0].Code == "" {
		t.Fatal("invitation not emailed")
	}
	if e.store.Profiles[out.ID].Kind != identity.KindAdmin {
		t.Fatal("admin profile missing")
	}
	if acts := e.store.AuditActions(); len(acts) != 1 || acts[0] != audit.ActionAdminInvited {
		t.Fatalf("audit: %v", acts)
	}

	// Replay with the same key returns the stored result without side effects.
	again, err := svc.Invite(context.Background(), a, "key-1", req)
	if err != nil || again.ID != out.ID || len(e.mail.Sent) != 1 {
		t.Fatalf("replay: %+v %v", again, err)
	}
	// Same key, different payload → conflict.
	other := req
	other.Role = identity.RoleAdmin
	if _, err := svc.Invite(context.Background(), a, "key-1", other); !errors.Is(err, app.ErrConflict) {
		t.Fatalf("key reuse: %v", err)
	}
	// New key, same email → conflict from Kratos.
	if _, err := svc.Invite(context.Background(), a, "key-2", req); !errors.Is(err, app.ErrConflict) {
		t.Fatalf("duplicate email: %v", err)
	}
}

func TestFR07_InviteDenialsAndValidation(t *testing.T) {
	e := newEnv()
	svc := e.admins()
	a := adminActor(uuid.New())
	e.authz.GrantRole(a.Principal.IdentityID, identity.RoleAdmin) // not super_admin
	req := app.InviteRequest{Email: "x@example.com", Role: identity.RoleSupport}
	if _, err := svc.Invite(context.Background(), a, "k", req); !errors.Is(err, app.ErrForbidden) {
		t.Fatalf("admin role must not invite: %v", err)
	}
	e.authz.GrantRole(a.Principal.IdentityID, identity.RoleSuperAdmin)
	var ve *app.ValidationError
	if _, err := svc.Invite(context.Background(), a, "k", app.InviteRequest{Email: "not-an-email", Role: identity.RoleSupport}); !errors.As(err, &ve) {
		t.Fatalf("bad email: %v", err)
	}
	if _, err := svc.Invite(context.Background(), a, "k", app.InviteRequest{Email: "x@example.com", Role: "root"}); !errors.As(err, &ve) {
		t.Fatalf("bad role: %v", err)
	}
	if len(e.ids.M) != 0 {
		t.Fatal("no identity may be created on denial/validation failure")
	}
}

func TestFR07_InviteCompensatesWhenMailFails(t *testing.T) {
	e := newEnv()
	e.mail.Err = errors.New("smtp down")
	svc := e.admins()
	a := adminActor(uuid.New())
	e.authz.GrantRole(a.Principal.IdentityID, identity.RoleSuperAdmin)
	if _, err := svc.Invite(context.Background(), a, "k", app.InviteRequest{Email: "x@example.com", Role: identity.RoleAdmin}); err == nil {
		t.Fatal("want error")
	}
	if len(e.ids.M) != 0 || len(e.ids.Deleted) != 1 || len(e.roles.M) != 0 {
		t.Fatal("identity/tuple not compensated")
	}
	if len(e.store.Idem) != 0 {
		t.Fatal("failed invite must not store an idempotent response")
	}
}

func TestFR12_ChangeRoleRules(t *testing.T) {
	e := newEnv()
	svc := e.admins()
	ctx := context.Background()
	caller := e.ids.Add(identity.Identity{SchemaID: "admin"})
	a := adminActor(caller.ID)
	e.authz.GrantRole(caller.ID, identity.RoleSuperAdmin)
	e.roles.M[caller.ID] = []identity.Role{identity.RoleSuperAdmin}
	target := e.ids.Add(identity.Identity{SchemaID: "admin"})
	e.roles.M[target.ID] = []identity.Role{identity.RoleSupport}
	cust := e.ids.Add(identity.Identity{SchemaID: "customer"})

	if _, err := svc.ChangeRole(ctx, a, caller.ID, identity.RoleAdmin); !errors.Is(err, app.ErrForbidden) {
		t.Fatalf("self change: %v", err)
	}
	if _, err := svc.ChangeRole(ctx, a, cust.ID, identity.RoleAdmin); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("customer target: %v", err)
	}
	v, err := svc.ChangeRole(ctx, a, target.ID, identity.RoleSuperAdmin)
	if err != nil || v.Role != identity.RoleSuperAdmin {
		t.Fatalf("promote: %v", err)
	}
	if e.store.Events[0].Action != audit.ActionAdminRoleChanged || e.store.Events[0].Details["previous_role"] != "support" {
		t.Fatalf("audit: %+v", e.store.Events[0])
	}
	// Two super admins now: demoting one is fine.
	if _, err := svc.ChangeRole(ctx, a, target.ID, identity.RoleAdmin); err != nil {
		t.Fatalf("demote with another super admin present: %v", err)
	}
	// Caller is now the last super_admin; another super admin demoting the
	// caller must be rejected.
	other := adminActor(target.ID)
	e.authz.GrantRole(target.ID, identity.RoleSuperAdmin)
	if _, err := svc.ChangeRole(ctx, other, caller.ID, identity.RoleSupport); !errors.Is(err, app.ErrConflict) {
		t.Fatalf("last super_admin: %v", err)
	}
	// Without manage_admins.
	plain := adminActor(uuid.New())
	e.authz.GrantRole(plain.Principal.IdentityID, identity.RoleAdmin)
	if _, err := svc.ChangeRole(ctx, plain, target.ID, identity.RoleSupport); !errors.Is(err, app.ErrForbidden) {
		t.Fatalf("admin role must not change roles: %v", err)
	}
}

func TestFR06_AdminMeRolesAndPermissions(t *testing.T) {
	e := newEnv()
	svc := e.admins()
	a := adminActor(uuid.New())
	e.roles.M[a.Principal.IdentityID] = []identity.Role{identity.RoleAdmin}
	e.authz.GrantRole(a.Principal.IdentityID, identity.RoleAdmin)
	v, err := svc.Me(context.Background(), a)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Roles) != 1 || v.Roles[0] != identity.RoleAdmin || len(v.Permissions) != 4 || v.Permissions[3] != identity.PermRevealCustomerPII {
		t.Fatalf("bad me %+v", v)
	}
	for _, p := range v.Permissions {
		if p == identity.PermManageAdmins {
			t.Fatal("admin role must not hold manage_admins")
		}
	}
}

func TestFR07_BootstrapCreatesSuperAdmin(t *testing.T) {
	e := newEnv()
	svc := e.admins()
	res, err := svc.Bootstrap(context.Background(), "Admin@Example.local", identity.Name{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Recovery.Link == "" || res.Recovery.Code == "" {
		t.Fatal("recovery code missing")
	}
	if got := e.roles.M[res.Admin.ID]; len(got) != 1 || got[0] != identity.RoleSuperAdmin {
		t.Fatalf("tuple: %v", got)
	}
	if len(e.mail.Sent) != 0 {
		t.Fatal("bootstrap must not email")
	}
	if _, err := svc.Bootstrap(context.Background(), "admin@example.local", identity.Name{}); !errors.Is(err, app.ErrConflict) {
		t.Fatalf("second bootstrap: %v", err)
	}
}

func TestFR12_AuditListPermissionAndPagination(t *testing.T) {
	e := newEnv()
	svc := &app.AuditService{Authz: e.authz, Audit: e.store.Repos().Audit}
	for i := range 5 {
		_ = e.store.Repos().Audit.Append(context.Background(), audit.Event{Action: "x", TargetType: "customer", TargetID: string(rune('a' + i))})
	}
	a := adminActor(uuid.New())
	if _, err := svc.List(context.Background(), a, audit.Filter{}); !errors.Is(err, app.ErrForbidden) {
		t.Fatalf("no view_audit: %v", err)
	}
	e.authz.Grant(a.Principal.IdentityID, identity.PermViewAudit)
	page, err := svc.List(context.Background(), a, audit.Filter{Limit: 2})
	if err != nil || len(page.Items) != 2 || page.Next == nil || page.Items[0].ID != 5 {
		t.Fatalf("page1: %+v %v", page, err)
	}
	page, _ = svc.List(context.Background(), a, audit.Filter{Limit: 2, After: page.Next})
	page, _ = svc.List(context.Background(), a, audit.Filter{Limit: 2, After: page.Next})
	if len(page.Items) != 1 || page.Next != nil {
		t.Fatalf("last page: %+v", page)
	}
}

func TestMemoAuthorizerCachesPerRequest(t *testing.T) {
	fa := testutil.NewAuthz()
	id := uuid.New()
	fa.Grant(id, identity.PermViewAudit)
	m := app.MemoAuthorizer{Next: fa}
	ctx := app.WithAuthzMemo(context.Background())
	for range 3 {
		ok, _ := m.Check(ctx, id, identity.PermViewAudit)
		if !ok {
			t.Fatal("want allowed")
		}
	}
	if fa.Calls != 1 {
		t.Fatalf("want 1 call, got %d", fa.Calls)
	}
	_, _ = m.Check(context.Background(), id, identity.PermViewAudit)
	if fa.Calls != 2 {
		t.Fatal("no memo outside request scope")
	}
}

// --- review regressions (C1–C4, minors) ---

func TestFR07_InviteTxFailureCompensatesAndReleasesKey(t *testing.T) {
	e := newEnv()
	svc := e.admins()
	a := adminActor(uuid.New())
	e.authz.GrantRole(a.Principal.IdentityID, identity.RoleSuperAdmin)
	e.store.CommitErr = errors.New("commit failed")
	req := app.InviteRequest{Email: "tx@example.com", Role: identity.RoleAdmin}
	if _, err := svc.Invite(context.Background(), a, "k1", req); err == nil {
		t.Fatal("want error")
	}
	if len(e.ids.M) != 0 || len(e.roles.M) != 0 || len(e.ids.Deleted) != 1 {
		t.Fatal("orphaned identity/tuple after failed record tx")
	}
	if len(e.mail.Sent) != 0 {
		t.Fatal("no email may be sent before the invitation is recorded")
	}
	if len(e.store.Events) != 0 || len(e.store.Idem) != 0 {
		t.Fatalf("rolled back tx left state: events=%d idem=%d", len(e.store.Events), len(e.store.Idem))
	}
	// Retry with the same key now succeeds (no 409).
	e.store.CommitErr = nil
	if _, err := svc.Invite(context.Background(), a, "k1", req); err != nil {
		t.Fatalf("retry: %v", err)
	}
}

func TestFR07_InviteMailFailureIsAuditedAndRetryable(t *testing.T) {
	e := newEnv()
	e.mail.Err = errors.New("smtp down")
	svc := e.admins()
	a := adminActor(uuid.New())
	e.authz.GrantRole(a.Principal.IdentityID, identity.RoleSuperAdmin)
	req := app.InviteRequest{Email: "m@example.com", Role: identity.RoleAdmin}
	if _, err := svc.Invite(context.Background(), a, "k", req); !errors.Is(err, app.ErrDependencyUnavailable) {
		t.Fatalf("want dependency unavailable, got %v", err)
	}
	acts := e.store.AuditActions()
	if len(acts) != 2 || acts[0] != audit.ActionAdminInvited || acts[1] != audit.ActionAdminInvitationFailed {
		t.Fatalf("audit: %v", acts)
	}
	e.mail.Err = nil
	if _, err := svc.Invite(context.Background(), a, "k", req); err != nil {
		t.Fatalf("retry after mail failure: %v", err)
	}
}

func TestFR07_InviteConcurrentSameKey(t *testing.T) {
	e := newEnv()
	svc := e.admins()
	a := adminActor(uuid.New())
	e.authz.GrantRole(a.Principal.IdentityID, identity.RoleSuperAdmin)
	req := app.InviteRequest{Email: "c@example.com", Role: identity.RoleSupport}
	// A pending reservation (request in flight) yields 409, not a second admin.
	if _, ok, _ := e.store.Repos().Idempotency.Reserve(context.Background(), a.Principal.IdentityID, "k", nil, t0.Add(-time.Hour)); !ok {
		t.Fatal("reserve")
	}
	_, err := svc.Invite(context.Background(), a, "k", req)
	if !errors.Is(err, app.ErrConflict) || len(e.ids.M) != 0 {
		t.Fatalf("pending key: %v, identities=%d", err, len(e.ids.M))
	}
}

func TestFR12_ConcurrentLastSuperAdminDemotion(t *testing.T) {
	e := newEnv()
	svc := e.admins()
	s1 := e.ids.Add(identity.Identity{SchemaID: "admin", Email: "s1@example.com"})
	s2 := e.ids.Add(identity.Identity{SchemaID: "admin", Email: "s2@example.com"})
	gone := uuid.New() // tuple of a deleted identity must not count
	inactive := e.ids.Add(identity.Identity{SchemaID: "admin", State: identity.StateInactive})
	for _, id := range []uuid.UUID{s1.ID, s2.ID, gone, inactive.ID} {
		e.roles.M[id] = []identity.Role{identity.RoleSuperAdmin}
		e.authz.GrantRole(id, identity.RoleSuperAdmin)
	}
	// s1 demotes s2 while s2 demotes s1: exactly one may succeed.
	errs := make(chan error, 2)
	go func() {
		_, err := svc.ChangeRole(context.Background(), adminActor(s1.ID), s2.ID, identity.RoleAdmin)
		errs <- err
	}()
	go func() {
		_, err := svc.ChangeRole(context.Background(), adminActor(s2.ID), s1.ID, identity.RoleAdmin)
		errs <- err
	}()
	var ok, conflict int
	for range 2 {
		switch err := <-errs; {
		case err == nil:
			ok++
		case errors.Is(err, app.ErrConflict):
			conflict++
		default:
			t.Fatalf("unexpected: %v", err)
		}
	}
	if ok != 1 || conflict != 1 {
		t.Fatalf("ok=%d conflict=%d", ok, conflict)
	}
	if e.store.LockCalls < 2 {
		t.Fatal("demotions must take the advisory lock")
	}
}

func TestFR12_MutationNotAppliedWhenAuditFails(t *testing.T) {
	e := newEnv()
	svc := e.customers()
	cust := e.ids.Add(identity.Identity{SchemaID: "customer"})
	a := adminActor(uuid.New())
	e.authz.GrantRole(a.Principal.IdentityID, identity.RoleAdmin)
	e.store.AuditErr = errors.New("db down")
	if _, err := svc.Disable(context.Background(), a, cust.ID, nil); err == nil {
		t.Fatal("want error")
	}
	if e.ids.M[cust.ID].State != identity.StateActive || len(e.ids.RevokedAll) != 0 {
		t.Fatal("mutation applied without an audit row")
	}
	e.store.AuditErr = nil
	// External failure rolls the audit row back.
	e.ids.Err = nil
	adm := adminActor(uuid.New())
	e.authz.GrantRole(adm.Principal.IdentityID, identity.RoleSuperAdmin)
	target := e.ids.Add(identity.Identity{SchemaID: "admin"})
	e.roles.M[target.ID] = []identity.Role{identity.RoleSupport}
	e.roles.Err = errors.New("keto down")
	if _, err := e.admins().ChangeRole(context.Background(), adm, target.ID, identity.RoleAdmin); err == nil {
		t.Fatal("want keto error")
	}
	if len(e.store.Events) != 0 {
		t.Fatal("audit row of a failed mutation must be rolled back")
	}
}

func TestFR11_ListScansPastAdminOnlyPage(t *testing.T) {
	e := newEnv()
	svc := e.customers()
	// Fake pages are sorted by id: make the first page admins only.
	for i := range 3 {
		e.ids.Add(identity.Identity{ID: uuid.MustParse(fmt.Sprintf("00000000-0000-0000-0000-00000000000%d", i)), SchemaID: "admin"})
	}
	c := e.ids.Add(identity.Identity{ID: uuid.MustParse("ffffffff-0000-0000-0000-000000000000"), SchemaID: "customer", Email: "an@example.com"})
	a := adminActor(uuid.New())
	e.authz.GrantRole(a.Principal.IdentityID, identity.RoleSupport)
	page, err := svc.List(context.Background(), a, app.CustomerQuery{PageSize: 3})
	if err != nil || len(page.Items) != 1 || page.Items[0].Identity.ID != c.ID {
		t.Fatalf("scan: %+v %v", page, err)
	}
	page, err = svc.List(context.Background(), a, app.CustomerQuery{Email: "  AN@Example.com "})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("email filter must be normalised: %+v %v", page, err)
	}
}

func TestFR10_ControlCharactersRejected(t *testing.T) {
	e := newEnv()
	svc := &app.MeService{Profiles: e.store.Repos().Profiles}
	var ve *app.ValidationError
	_, err := svc.UpdateMe(context.Background(), customerPrincipal(true), profile.Patch{DisplayName: profile.OptionalString{Set: true, Value: ptr("a\x00b")}})
	if !errors.As(err, &ve) || ve.Fields[0].Code != "invalid_characters" {
		t.Fatalf("NUL: %v", err)
	}
	a := adminActor(uuid.New())
	e.authz.GrantRole(a.Principal.IdentityID, identity.RoleSuperAdmin)
	_, err = e.admins().Invite(context.Background(), a, "k", app.InviteRequest{Email: "x@example.com", Name: identity.Name{First: "a\nb"}, Role: identity.RoleSupport})
	if !errors.As(err, &ve) {
		t.Fatalf("newline in name: %v", err)
	}
}
