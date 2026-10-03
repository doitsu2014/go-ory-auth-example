//go:build integration

package e2e

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/crypto/envelope"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/identity"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/pii"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/testutil"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/testutil/itest"
)

const (
	eFirst = "Quỳnh"
	eLast  = "Đặng"
)

func noName(t *testing.T, where string, b []byte) {
	t.Helper()
	for _, v := range []string{eFirst, eLast} {
		if bytes.Contains(b, []byte(v)) || bytes.Contains(b, []byte(hex.EncodeToString([]byte(v)))) {
			t.Fatalf("%s contains the name value %q", where, v)
		}
	}
}

func (s *stack) rawName(t *testing.T, id uuid.UUID) ([]byte, string) {
	t.Helper()
	var ct []byte
	var text string
	if err := s.pool.QueryRow(context.Background(), `SELECT name_ct, p::text FROM customer_pii p WHERE identity_id = $1`, id).
		Scan(&ct, &text); err != nil {
		t.Fatalf("raw row: %v", err)
	}
	return ct, text
}

// TestNameFR01_E2E_KratosRejectsNameTrait: the customer schema holds the
// email only (NAME-FR-01).
func TestNameFR01_E2E_KratosRejectsNameTrait(t *testing.T) {
	env := itest.Load()
	st := env.RegisterCustomerTraits(t, map[string]any{"email": itest.UniqueEmail("e2e-name-reject"), "name": map[string]string{"first": "X"}})
	if st != 400 {
		t.Fatalf("registration with traits.name: %d (want 400; Kratos running the new customer schema?)", st)
	}
}

// TestNameFR02_06_E2E_Lifecycle: PUT/GET/masked/reveal/erase with a name
// over the real stack; the raw row holds ciphertext only.
func TestNameFR02_06_E2E_Lifecycle(t *testing.T) {
	s := newStack(t)
	tok, custID := s.verifiedCustomer(t)
	body := ePII()
	body["name"] = map[string]any{"first": " " + eFirst + " ", "last": eLast}
	var info struct {
		Name *struct{ First, Last string } `json:"name"`
	}
	if st := s.api(t, "PUT", "/v1/me/personal-info", call{bearer: tok}, body, &info); st != 200 || info.Name == nil || info.Name.First != eFirst {
		t.Fatalf("PUT: %d", st)
	}
	info.Name = nil
	if st := s.api(t, "GET", "/v1/me/personal-info", call{bearer: tok}, nil, &info); st != 200 || info.Name == nil || info.Name.Last != eLast {
		t.Fatalf("GET: %d", st)
	}
	ct, text := s.rawName(t, custID)
	if len(ct) < envelope.Overhead || ct[0] != envelope.FormatV1 {
		t.Fatal("name_ct is not an envelope ciphertext")
	}
	noName(t, "raw row", []byte(text))

	supportCookie, _ := s.adminSession(t, identity.RoleSupport)
	adminCookie, _ := s.adminSession(t, identity.RoleAdmin)
	var masked struct {
		HasName bool `json:"has_name"`
		Name    *struct {
			First, Last *string
		} `json:"name"`
	}
	if st := s.api(t, "GET", "/admin/v1/customers/"+custID.String()+"/personal-info", call{cookie: supportCookie}, nil, &masked); st != 200 ||
		!masked.HasName || masked.Name == nil || *masked.Name.First != "Q***" || *masked.Name.Last != "Đ***" {
		t.Fatalf("masked: %d", st)
	}
	var customer map[string]any
	if st := s.api(t, "GET", "/admin/v1/customers/"+custID.String(), call{cookie: supportCookie}, nil, &customer); st != 200 || customer["name"] != nil {
		t.Fatalf("admin customer view must not carry a name: %d", st)
	}
	info.Name = nil
	reveal := "/admin/v1/customers/" + custID.String() + "/personal-info/reveal"
	if st := s.api(t, "POST", reveal, call{cookie: adminCookie, origin: webOrigin},
		map[string]any{"reason_code": "identity_verification", "fields": []string{"name"}}, &info); st != 200 || info.Name == nil || info.Name.First != eFirst {
		t.Fatalf("reveal name: %d", st)
	}
	if n := s.count(t, `SELECT count(*) FROM audit_event WHERE action = 'customer.pii.revealed' AND target_id = $1
		AND details->'fields' = '["name"]'::jsonb`, custID.String()); n != 1 {
		t.Fatalf("reveal audit rows: %d", n)
	}
	if st := s.api(t, "DELETE", "/v1/me/personal-info", call{bearer: tok}, nil, nil); st != 204 {
		t.Fatalf("erase: %d", st)
	}
	if n := s.count(t, `SELECT count(*) FROM customer_pii WHERE identity_id = $1`, custID); n != 0 {
		t.Fatal("name must be crypto-shredded with the record")
	}
	var audits bytes.Buffer
	rows, _ := s.pool.Query(context.Background(), `SELECT details::text FROM audit_event WHERE target_id = $1`, custID.String())
	for rows.Next() {
		var d string
		_ = rows.Scan(&d)
		audits.WriteString(d)
	}
	rows.Close()
	noName(t, "audit details", audits.Bytes())
	noName(t, "service logs", []byte(s.logs.String()))
}

// TestNameFR08_E2E_Migration runs the migration against the real database
// and OpenBao (Kratos is faked: the new customer schema no longer accepts a
// name trait, so legacy identities cannot be created there).
func TestNameFR08_E2E_Migration(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()
	ids := testutil.NewIdentities(app.SystemClock{})
	legacy := func() identity.Identity {
		i := ids.Add(identity.Identity{SchemaID: "customer", Name: identity.Name{First: eFirst, Last: eLast}})
		s.cleanupSubject(t, i.ID)
		return i
	}
	pi := s.freshService()
	pi.Identities = ids
	mig := &app.NameMigrationService{PersonalInfo: pi, Kratos: ids, Log: pi.Log}
	actor := func(id uuid.UUID) app.Actor {
		a := synthetic()
		a.Principal.IdentityID = id
		return a
	}

	fresh := legacy() // (c) no record → new DEK + name_ct
	partial := legacy()
	if _, err := pi.PutMine(ctx, actor(partial.ID), samplePII(ePhone)); err != nil {
		t.Fatal(err)
	}
	hasName := legacy() // (b)
	if _, err := pi.PutMine(ctx, actor(hasName.ID), pii.PersonalInfo{Name: &pii.Name{Last: "Khác"}}); err != nil {
		t.Fatal(err)
	}
	erased := legacy() // (a)
	if _, err := pi.PutMine(ctx, actor(erased.ID), samplePII(ePhone)); err != nil {
		t.Fatal(err)
	}
	if err := pi.EraseMine(ctx, actor(erased.ID)); err != nil {
		t.Fatal(err)
	}

	// Kratos fails after the commit of the first identities: their names
	// stay in Kratos and the re-run strips them (NAME-NFR-03).
	ids.RemoveNameErr = errors.New("kratos down")
	rep, err := mig.MigrateKratosNames(ctx, false)
	if err == nil || rep.Scanned != 4 || rep.Failed != 4 {
		t.Fatalf("run with Kratos down: %+v %v", rep, err)
	}
	ids.RemoveNameErr = nil
	rep, err = mig.MigrateKratosNames(ctx, false)
	if err != nil || rep != (app.NameMigrationReport{Scanned: 4, StrippedOnly: 4}) {
		t.Fatalf("re-run: %+v %v", rep, err)
	}
	rep, err = mig.MigrateKratosNames(ctx, false)
	if err != nil || rep != (app.NameMigrationReport{}) {
		t.Fatalf("idempotent: %+v %v", rep, err)
	}

	for _, i := range []identity.Identity{fresh, partial} {
		v, err := pi.GetMine(ctx, actor(i.ID).Principal)
		if err != nil || v.Info.Name == nil || v.Info.Name.First != eFirst || v.Info.Name.Last != eLast {
			t.Fatalf("migrated name: %v", err)
		}
		_, text := s.rawName(t, i.ID)
		noName(t, "raw row", []byte(text))
		if n := s.count(t, `SELECT count(*) FROM audit_event WHERE action = 'customer.pii.name_migrated' AND target_id = $1
			AND actor_identity_id = '00000000-0000-0000-0000-000000000000' AND details = '{"fields":["name"]}'::jsonb`, i.ID.String()); n != 1 {
			t.Fatalf("name_migrated audit rows: %d", n)
		}
	}
	if v, _ := pi.GetMine(ctx, actor(partial.ID).Principal); v.Info.Phone == nil || *v.Info.Phone != ePhone {
		t.Fatal("other columns must survive the migration")
	}
	if v, _ := pi.GetMine(ctx, actor(hasName.ID).Principal); v.Info.Name == nil || v.Info.Name.Last != "Khác" || v.Info.Name.First != "" {
		t.Fatal("an existing name must win")
	}
	if n := s.count(t, `SELECT count(*) FROM subject_key WHERE identity_id = $1`, erased.ID); n != 0 {
		t.Fatal("an erased customer must not get a new key")
	}
	if n := s.count(t, `SELECT count(*) FROM audit_event WHERE action = 'customer.pii.name_migrated' AND target_id = ANY($1)`,
		[]string{hasName.ID.String(), erased.ID.String()}); n != 0 {
		t.Fatalf("strip-only branches must not audit a migration: %d", n)
	}
	for _, i := range []identity.Identity{fresh, partial, hasName, erased} {
		if got, _ := ids.GetIdentity(ctx, i.ID); got.Name != (identity.Name{}) {
			t.Fatal("trait must be stripped")
		}
	}
	noName(t, "service logs", []byte(s.logs.String()))
}

// MAJOR-2: erasing with no stored record but a legacy name trait records
// customer.pii.erased in the real audit log; the migration then stores
// nothing (Kratos faked, see TestNameFR08_E2E_Migration).
func TestNameFR06_E2E_EraseWithLegacyNameTrait(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()
	ids := testutil.NewIdentities(app.SystemClock{})
	i := ids.Add(identity.Identity{SchemaID: "customer", Name: identity.Name{First: eFirst, Last: eLast}})
	s.cleanupSubject(t, i.ID)
	pi := s.freshService()
	pi.Identities, pi.NameTraits = ids, ids
	a := synthetic()
	a.Principal.IdentityID = i.ID

	ids.RemoveNameErr = errors.New("kratos down") // keep the trait: the migration must still not store it
	if err := pi.EraseMine(ctx, a); !errors.Is(err, app.ErrDependencyUnavailable) {
		t.Fatalf("erase with Kratos down: %v", err)
	}
	ids.RemoveNameErr = nil
	if n := s.count(t, `SELECT count(*) FROM audit_event WHERE action = 'customer.pii.erased' AND target_id = $1`, i.ID.String()); n != 1 {
		t.Fatalf("erased audit rows: %d", n)
	}
	mig := &app.NameMigrationService{PersonalInfo: pi, Kratos: ids, Log: pi.Log}
	rep, err := mig.MigrateKratosNames(ctx, false)
	if err != nil || rep != (app.NameMigrationReport{Scanned: 1, StrippedOnly: 1}) {
		t.Fatalf("migration: %+v %v", rep, err)
	}
	if n := s.count(t, `SELECT count(*) FROM subject_key WHERE identity_id = $1`, i.ID) +
		s.count(t, `SELECT count(*) FROM customer_pii WHERE identity_id = $1`, i.ID); n != 0 {
		t.Fatal("nothing may be stored for an erased customer")
	}
	if n := s.count(t, `SELECT count(*) FROM audit_event WHERE action = 'customer.pii.name_trait_removed' AND target_id = $1
		AND details = '{"fields":["name"],"reason":"erased"}'::jsonb`, i.ID.String()); n != 1 {
		t.Fatalf("name_trait_removed audit rows: %d", n)
	}
	if got, _ := ids.GetIdentity(ctx, i.ID); got.Name != (identity.Name{}) {
		t.Fatal("trait must be stripped")
	}
}
