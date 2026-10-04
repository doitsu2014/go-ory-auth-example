//go:build integration

package e2e

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/identity"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/login"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/testutil/itest"
)

// TestPLI_E2E_LoginLifecycle (G2, G4): pseudonymous registration → /v1/me
// shows the decrypted login → Kratos holds only the pseudonym → admin
// masked list/lookup → audited reveal of "login" → no address in the
// service logs or audit rows.
func TestPLI_E2E_LoginLifecycle(t *testing.T) {
	s := newStack(t)
	email := itest.UniqueEmail("e2e-login")
	reg := s.env.RegisterCustomer(t, email)
	custID := uuid.MustParse(reg.Session.Identity.ID)
	t.Cleanup(func() { s.env.DeleteIdentity(t, custID) })

	var me struct {
		Login struct{ Type, Value string } `json:"login"`
		Email string                       `json:"email"`
	}
	if st := s.api(t, "GET", "/v1/me", call{bearer: reg.SessionToken}, nil, &me); st != 200 ||
		me.Login.Type != "email" || me.Login.Value != email || me.Email != email {
		t.Fatalf("GET /v1/me: %d %+v", st, me)
	}
	k, err := s.kadmin.GetIdentity(context.Background(), custID)
	if err != nil || k.Email != "" || !login.IsPseudonym(k.LoginID) {
		t.Fatalf("Kratos must hold only the pseudonym: %+v %v", k, err)
	}

	cookie, _ := s.adminSession(t, identity.RoleAdmin)
	var found struct {
		Items []struct {
			ID    uuid.UUID                     `json:"id"`
			Login struct{ Type, Masked string } `json:"login"`
		} `json:"items"`
	}
	if st := s.api(t, "POST", "/admin/v1/customers/lookup", call{cookie: cookie, origin: webOrigin},
		map[string]any{"login": map[string]string{"type": "email", "value": strings.ToUpper(email)}}, &found); st != 200 ||
		len(found.Items) != 1 || found.Items[0].ID != custID || !strings.Contains(found.Items[0].Login.Masked, "***@") {
		t.Fatalf("lookup: %d %+v", st, found)
	}
	var detail struct {
		Login            struct{ Type, Masked string } `json:"login"`
		LoginUnavailable bool                          `json:"login_unavailable"`
	}
	if st := s.api(t, "GET", "/admin/v1/customers/"+custID.String(), call{cookie: cookie}, nil, &detail); st != 200 ||
		detail.Login.Masked == "" || detail.LoginUnavailable || strings.Contains(detail.Login.Masked, email) {
		t.Fatalf("detail: %d %+v", st, detail)
	}
	var revealed struct {
		Login struct{ Type, Value string } `json:"login"`
	}
	if st := s.api(t, "POST", "/admin/v1/customers/"+custID.String()+"/personal-info/reveal", call{cookie: cookie, origin: webOrigin},
		map[string]any{"reason_code": "customer_support_request", "fields": []string{"login"}}, &revealed); st != 200 || revealed.Login.Value != email {
		t.Fatalf("reveal: %d %+v", st, revealed)
	}

	local := email[:strings.IndexByte(email, '@')]
	if n := s.count(t, `SELECT count(*) FROM audit_event WHERE details::text ILIKE '%' || $1 || '%'`, local); n != 0 {
		t.Fatalf("audit rows hold the address: %d", n)
	}
	if n := s.count(t, `SELECT count(*) FROM audit_event WHERE action = 'customer.pii.revealed' AND target_id = $1 AND details->'fields' ? 'login'`, custID.String()); n != 1 {
		t.Fatalf("reveal audit: %d", n)
	}
	if n := s.count(t, `SELECT count(*) FROM login_identifier WHERE identity_id = $1 AND value_ct NOT ILIKE '%' || $2 || '%'`, custID, local); n != 1 {
		t.Fatalf("vault row must exist, bound and sealed: %d", n)
	}
	if logs := s.logs.String(); strings.Contains(logs, local) || strings.Contains(logs, k.LoginID) {
		t.Fatal("service logs hold the address or the pseudonym")
	}
}

// TestPLIFR13_E2E_MigrateLegacyCustomer (G3): a legacy customer (plaintext
// email, verified) is migrated against the real Kratos and signs in with the
// pseudonym, still verified.
func TestPLIFR13_E2E_MigrateLegacyCustomer(t *testing.T) {
	s := newStack(t)
	email := itest.UniqueEmail("e2e-legacy")
	var created struct {
		ID uuid.UUID `json:"id"`
	}
	if st, _ := itest.JSON(t, nil, "POST", s.env.KratosAdmin+"/admin/identities", nil, map[string]any{
		"schema_id": "customer", "state": "active", "traits": map[string]any{"email": email},
		"credentials":          map[string]any{"password": map[string]any{"config": map[string]any{"password": itest.Password}}},
		"verifiable_addresses": []map[string]any{{"value": email, "verified": true, "via": "email", "status": "completed"}},
	}, &created); st != 201 {
		t.Fatalf("create legacy customer: %d (transition schema deployed?)", st)
	}
	t.Cleanup(func() {
		_, _ = s.store.Repos().Logins.DeleteForIdentity(context.Background(), created.ID)
		s.env.DeleteIdentity(t, created.ID)
	})
	m := &app.LoginMigrationService{Logins: s.logins, Kratos: s.kadmin, Tx: s.store}
	// The shared stack may hold other legacy customers; only ours is asserted.
	if _, err := m.Migrate(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	k, err := s.kadmin.GetIdentity(context.Background(), created.ID)
	if err != nil || k.Email != "" || !login.IsPseudonym(k.LoginID) || !k.EmailVerified {
		t.Fatalf("after migration: %+v %v", k, err)
	}
	if st, f := s.env.LoginCustomerAPI(t, email); st != 200 || f.SessionToken == "" {
		t.Fatalf("sign in with the pseudonym after migration: %d", st)
	}
	// Idempotent: a second run leaves our customer untouched.
	if _, err := m.Migrate(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if k2, _ := s.kadmin.GetIdentity(context.Background(), created.ID); k2.LoginID != k.LoginID || !k2.EmailVerified {
		t.Fatalf("second run changed the identity: %+v", k2)
	}
}
