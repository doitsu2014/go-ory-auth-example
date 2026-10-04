package app_test

import (
	"context"
	"errors"
	"testing"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/audit"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/identity"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/login"
)

func newMigration(l *loginEnv) *app.LoginMigrationService {
	return &app.LoginMigrationService{Logins: l.svc, Kratos: l.ids, Tx: l.store, Log: l.svc.Log}
}

func TestPLIFR13_MigrateLegacyCustomers(t *testing.T) {
	l := newLoginEnv(t)
	ctx := context.Background()
	verified := l.ids.AddCustomer("verified@example.com", true)
	unverified := l.ids.AddCustomer("unverified@example.com", false)
	admin := l.ids.Add(identity.Identity{SchemaID: "admin", Email: "ops@example.com", LoginID: "ops@example.com", EmailVerified: true})
	bad := l.ids.AddCustomer("not an email", true)
	m := newMigration(l)

	dry, err := m.Migrate(ctx, true)
	if err != nil || dry.Migrated != 2 || dry.Failed != 1 || len(l.store.Logins) != 0 {
		t.Fatalf("dry run %+v %v", dry, err)
	}
	res, err := m.Migrate(ctx, false)
	if err != nil || res.Migrated != 2 || res.Failed != 1 || res.Scanned != 3 {
		t.Fatalf("run %+v %v", res, err)
	}
	for _, c := range []struct {
		id       identity.Identity
		email    string
		verified bool
	}{{verified, "verified@example.com", true}, {unverified, "unverified@example.com", false}} {
		got := l.ids.M[c.id.ID]
		if !login.IsPseudonym(got.LoginID) || got.Email != "" || got.EmailVerified != c.verified {
			t.Fatalf("identity %+v", got)
		}
		// The customer still resolves to the same account with their email.
		p := l.resolve(t, "email", c.email, "sign_in")
		if p != got.LoginID {
			t.Fatal("pseudonym mismatch")
		}
		own, err := l.svc.Own(ctx, identity.Principal{IdentityID: c.id.ID, Kind: identity.KindCustomer, LoginID: got.LoginID})
		if err != nil || own.Value() != c.email {
			t.Fatalf("own: %v", err)
		}
	}
	if l.ids.M[admin.ID].LoginID != "ops@example.com" || l.ids.M[bad.ID].LoginID != "not an email" {
		t.Fatal("admins and invalid emails must be left alone")
	}
	if n := countAction(l, audit.ActionCustomerLoginMigrated); n != 2 {
		t.Fatalf("audits %d", n)
	}
	again, _ := m.Migrate(ctx, false)
	if again.Migrated != 0 || again.Skipped != 2 || again.Failed != 1 {
		t.Fatalf("second run must be a no-op: %+v", again)
	}
}

func TestPLIFR13_CrashBetweenPatchesIsRepaired(t *testing.T) {
	l := newLoginEnv(t)
	ctx := context.Background()
	c := l.ids.AddCustomer("crash@example.com", true)
	m := newMigration(l)
	l.ids.MarkVerifiedErr = errors.New("kratos down")
	res, _ := m.Migrate(ctx, false)
	if res.Failed != 1 || l.ids.M[c.ID].EmailVerified || !login.IsPseudonym(l.ids.M[c.ID].LoginID) {
		t.Fatalf("crash state %+v", res)
	}
	l.ids.MarkVerifiedErr = nil
	res, err := m.Migrate(ctx, false)
	if err != nil || res.Reverified != 1 || !l.ids.M[c.ID].EmailVerified {
		t.Fatalf("repair %+v %v", res, err)
	}
}

func TestPLIA2_MigrationCollision(t *testing.T) {
	l := newLoginEnv(t)
	ctx := context.Background()
	legacy := l.ids.AddCustomer("victim@example.com", true)
	// Before the A2 guard existed, someone registered the pseudonym.
	p := l.resolve(t, "email", "victim@example.com", "registration")
	squatter := l.ids.AddCustomer(p, false)
	if err := l.svc.Bind(ctx, squatter.ID, p); err != nil {
		t.Fatal(err)
	}
	res, err := newMigration(l).Migrate(ctx, false)
	if err != nil || res.Failed != 1 || l.ids.M[legacy.ID].LoginID != "victim@example.com" {
		t.Fatalf("collision must fail and keep the legacy login: %+v %v", res, err)
	}
}

func TestPLINFR06_MigrationStopsWhenKeyManagerDown(t *testing.T) {
	l := newLoginEnv(t)
	l.ids.AddCustomer("a@example.com", true)
	l.kms.SetFailure(app.ErrDependencyUnavailable)
	if _, err := newMigration(l).Migrate(context.Background(), false); !errors.Is(err, app.ErrDependencyUnavailable) {
		t.Fatalf("got %v", err)
	}
}

func countAction(l *loginEnv, a audit.Action) int {
	n := 0
	for _, x := range l.store.AuditActions() {
		if x == a {
			n++
		}
	}
	return n
}
