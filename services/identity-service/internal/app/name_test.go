package app_test

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/crypto/envelope"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/audit"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/identity"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/pii"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/platform"
)

// Name test values; none may appear in storage, logs or audit rows.
const (
	tFirst = "Thanh"
	tLast  = "Trương"
)

func assertNoName(t *testing.T, where, s string) {
	t.Helper()
	for _, v := range []string{tFirst, tLast} {
		if strings.Contains(s, v) {
			t.Fatalf("%s contains name value %q", where, v)
		}
	}
}

func withName(in pii.PersonalInfo) pii.PersonalInfo {
	in.Name = &pii.Name{First: " " + tFirst + " ", Last: tLast}
	return in
}

func TestNameFR02_03_PutGetRevealEraseWithName(t *testing.T) {
	p := newPIIEnv(t)
	ctx := context.Background()
	c := p.customer()
	id := c.Principal.IdentityID
	v, err := p.svc.PutMine(ctx, c, withName(sample(tPhone, tLine1)))
	if err != nil || v.Info.Name == nil || v.Info.Name.First != tFirst {
		t.Fatalf("put: %v", err)
	}
	got, err := p.svc.GetMine(ctx, c.Principal)
	if err != nil || got.Info.Name == nil || got.Info.Name.First != tFirst || got.Info.Name.Last != tLast {
		t.Fatalf("get: %v", err)
	}
	rec := p.store.PII[id]
	if len(rec.Name) < envelope.Overhead || rec.Name[0] != envelope.FormatV1 {
		t.Fatal("name_ct is not an envelope ciphertext")
	}
	assertNoName(t, "ciphertext", string(rec.Name))
	if f, _ := p.lastEvent(t).Details["fields"].([]string); strings.Join(f, ",") != "name,phone_number,date_of_birth,address,national_id" {
		t.Fatalf("audit fields %v", f)
	}

	// Masked (support) and lookup show first rune + ***.
	support := p.admin(identity.RoleSupport)
	m, err := p.svc.GetMasked(ctx, support, id)
	if err != nil || m.Masked.Name == nil || *m.Masked.Name.First != "T***" || *m.Masked.Name.Last != "T***" {
		t.Fatalf("masked: %v", err)
	}
	res, err := p.svc.LookupByPhone(ctx, support, tPhone)
	if err != nil || len(res.Matches) != 1 || res.Matches[0].Masked.Name == nil || *res.Matches[0].Masked.Name.Last != "T***" {
		t.Fatalf("lookup must mask the name too: %v", err)
	}

	// Reveal only the name.
	admin := p.admin(identity.RoleAdmin)
	r, err := p.svc.Reveal(ctx, admin, id, app.RevealRequest{ReasonCode: app.ReasonIdentityVerification, Fields: []string{"name"}})
	if err != nil || r.Info.Name == nil || r.Info.Name.Last != tLast || r.Info.Phone != nil {
		t.Fatalf("reveal name: %v", err)
	}
	if f, _ := p.lastEvent(t).Details["fields"].([]string); len(f) != 1 || f[0] != "name" {
		t.Fatalf("reveal audit fields %v", f)
	}
	// Reveal without the name keeps it hidden.
	r, err = p.svc.Reveal(ctx, admin, id, app.RevealRequest{ReasonCode: app.ReasonIdentityVerification, Fields: []string{"phone_number"}})
	if err != nil || r.Info.Name != nil {
		t.Fatalf("reveal phone only: %v", err)
	}

	// A PUT without a name clears it (full replacement).
	if _, err := p.svc.PutMine(ctx, c, sample(tPhone, tLine1)); err != nil {
		t.Fatal(err)
	}
	if p.store.PII[id].Name != nil {
		t.Fatal("PUT without name must clear name_ct")
	}
	if _, err := p.svc.PutMine(ctx, c, withName(pii.PersonalInfo{})); err != nil {
		t.Fatal(err)
	}

	// Erase crypto-shreds the name with the rest.
	if err := p.svc.EraseMine(ctx, c); err != nil {
		t.Fatal(err)
	}
	if _, ok := p.store.PII[id]; ok {
		t.Fatal("record must cascade")
	}
	if g, err := p.svc.GetMine(ctx, c.Principal); err != nil || g.Info.Name != nil {
		t.Fatalf("after erase: %v", err)
	}
	assertNoName(t, "audit", p.auditText())
	assertNoName(t, "logs", p.logs.String())
}

func TestNameFR02_ValidationNeverEchoes(t *testing.T) {
	p := newPIIEnv(t)
	c := p.customer()
	in := pii.PersonalInfo{Name: &pii.Name{First: "T\u0085" + tFirst, Last: strings.Repeat("ư", 101)}}
	_, err := p.svc.PutMine(context.Background(), c, in)
	var ve *app.ValidationError
	if !errors.As(err, &ve) || len(ve.Fields) != 2 {
		t.Fatalf("validation: %v", err)
	}
	assertNoName(t, "error", err.Error())
	if len(p.store.PII) != 0 {
		t.Fatal("nothing stored")
	}
	// All-empty name = absent: nothing to store.
	v, err := p.svc.PutMine(context.Background(), c, pii.PersonalInfo{Name: &pii.Name{First: "  "}})
	if err != nil || v.Info.Name != nil {
		t.Fatalf("empty name: %v", err)
	}
}

func TestNameFR03_AADBindsColumnAndRow(t *testing.T) {
	p := newPIIEnv(t)
	ctx := context.Background()
	a, b := p.customer(), p.customer()
	for _, c := range []app.Actor{a, b} {
		if _, err := p.svc.PutMine(ctx, c, withName(sample(tPhone, tLine1))); err != nil {
			t.Fatal(err)
		}
	}
	ida := a.Principal.IdentityID
	orig := p.store.PII[ida]
	// Column swap: the name ciphertext moved into the address column.
	rec := orig
	rec.Name, rec.Address = rec.Address, rec.Name
	p.store.PII[ida] = rec
	if _, err := p.svc.GetMine(ctx, a.Principal); !errors.Is(err, app.ErrDataIntegrity) {
		t.Fatalf("column swap: %v", err)
	}
	// Row swap: B's name ciphertext in A's row.
	rec = orig
	rec.Name = p.store.PII[b.Principal.IdentityID].Name
	p.store.PII[ida] = rec
	if _, err := p.svc.GetMine(ctx, a.Principal); !errors.Is(err, app.ErrDataIntegrity) {
		t.Fatalf("row swap: %v", err)
	}
	// The name opens only with AAD column "name" and the subject's id.
	key := p.store.Keys[ida]
	dek, err := p.kms.UnwrapDEK(ctx, key.Context(), key.Wrapped)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := envelope.Open(dek, envelope.AAD("name", ida), orig.Name); err != nil {
		t.Fatalf("own AAD: %v", err)
	}
	if _, err := envelope.Open(dek, envelope.AAD("address", ida), orig.Name); !errors.Is(err, envelope.ErrDecrypt) {
		t.Fatalf("wrong column AAD must fail: %v", err)
	}
	if !strings.Contains(p.logs.String(), "pii_decrypt_failed") {
		t.Fatal("integrity failure must be logged")
	}
	assertNoName(t, "logs", p.logs.String())
}

// --- NAME-FR-08: migrate-kratos-names ---

type migEnv struct {
	*piiEnv
	mig *app.NameMigrationService
}

func newMigEnv(t *testing.T) *migEnv {
	p := newPIIEnv(t)
	return &migEnv{piiEnv: p, mig: &app.NameMigrationService{PersonalInfo: p.svc, Kratos: p.ids, Log: platform.NewLogger(p.logs, logLevel)}}
}

// legacy adds a customer that still carries the name trait in Kratos.
func (m *migEnv) legacy() app.Actor {
	c := m.customer()
	i := m.ids.M[c.Principal.IdentityID]
	i.Name = identity.Name{First: tFirst, Last: tLast}
	m.ids.M[i.ID] = i
	return c
}

func (m *migEnv) countAction(a audit.Action) int {
	n := 0
	for _, e := range m.store.Events {
		if e.Action == a {
			n++
		}
	}
	return n
}

func TestNameFR08_MigrationBranches(t *testing.T) {
	m := newMigEnv(t)
	ctx := context.Background()

	// (c) no personal info at all → new DEK + name_ct.
	fresh := m.legacy()
	// (c) personal info without a name → name added, other columns untouched.
	partial := m.legacy()
	if _, err := m.svc.PutMine(ctx, partial, sample(tPhone, tLine1)); err != nil {
		t.Fatal(err)
	}
	before := m.store.PII[partial.Principal.IdentityID]
	// (b) personal info already has a name → only stripped (the stored name wins).
	hasName := m.legacy()
	if _, err := m.svc.PutMine(ctx, hasName, pii.PersonalInfo{Name: &pii.Name{First: "Khác"}}); err != nil {
		t.Fatal(err)
	}
	storedName := m.store.PII[hasName.Principal.IdentityID].Name
	// (a) erased → only stripped, nothing re-created.
	erased := m.legacy()
	if _, err := m.svc.PutMine(ctx, erased, sample(tPhoneB, tLine1B)); err != nil {
		t.Fatal(err)
	}
	if err := m.svc.EraseMine(ctx, erased); err != nil {
		t.Fatal(err)
	}
	// Not migrated: a customer without a name trait and an admin with one.
	m.customer() // no name trait: not scanned
	adm := m.ids.Add(identity.Identity{SchemaID: "admin", Name: identity.Name{First: "Admin"}})

	// Dry run: counts only, nothing written.
	events := len(m.store.Events)
	rep, err := m.mig.MigrateKratosNames(ctx, true)
	if err != nil || rep != (app.NameMigrationReport{Scanned: 4, Migrated: 2, StrippedOnly: 2}) {
		t.Fatalf("dry run: %+v %v", rep, err)
	}
	if len(m.store.Events) != events || len(m.ids.NameRemoved) != 0 || m.store.PII[fresh.Principal.IdentityID].Name != nil {
		t.Fatal("dry run must not write")
	}

	rep, err = m.mig.MigrateKratosNames(ctx, false)
	if err != nil || rep != (app.NameMigrationReport{Scanned: 4, Migrated: 2, StrippedOnly: 2}) {
		t.Fatalf("run: %+v %v", rep, err)
	}
	for _, c := range []app.Actor{fresh, partial, hasName, erased} {
		if m.ids.M[c.Principal.IdentityID].Name != (identity.Name{}) {
			t.Fatal("name trait must be stripped")
		}
	}
	if m.ids.M[adm.ID].Name.First != "Admin" {
		t.Fatal("admins keep their name")
	}

	for _, c := range []app.Actor{fresh, partial} {
		g, err := m.svc.GetMine(ctx, c.Principal)
		if err != nil || g.Info.Name == nil || g.Info.Name.First != tFirst || g.Info.Name.Last != tLast {
			t.Fatalf("migrated name: %v", err)
		}
		assertNoName(t, "ciphertext", string(m.store.PII[c.Principal.IdentityID].Name))
	}
	after := m.store.PII[partial.Principal.IdentityID]
	if string(after.Phone) != string(before.Phone) || string(after.Address) != string(before.Address) || after.KeyID != before.KeyID {
		t.Fatal("other columns must be untouched")
	}
	if string(m.store.PII[hasName.Principal.IdentityID].Name) != string(storedName) {
		t.Fatal("an existing name must not be overwritten")
	}
	if _, ok := m.store.Keys[erased.Principal.IdentityID]; ok {
		t.Fatal("an erased customer must not get a new key")
	}
	if n := m.countAction(audit.ActionCustomerPIINameMigrated); n != 2 {
		t.Fatalf("name_migrated events: %d", n)
	}
	for _, e := range m.store.Events {
		if e.Action != audit.ActionCustomerPIINameMigrated {
			continue
		}
		if f, _ := e.Details["fields"].([]string); e.ActorID != audit.SystemActor || len(f) != 1 || f[0] != "name" || len(e.Details) != 1 {
			t.Fatalf("audit %+v", e)
		}
	}

	// Idempotent re-run: nothing left to do.
	rep, err = m.mig.MigrateKratosNames(ctx, false)
	if err != nil || rep != (app.NameMigrationReport{}) {
		t.Fatalf("re-run: %+v %v", rep, err)
	}
	if n := m.countAction(audit.ActionCustomerPIINameMigrated); n != 2 {
		t.Fatalf("re-run must not audit again: %d", n)
	}
	assertNoName(t, "audit", m.auditText())
	assertNoName(t, "logs", m.logs.String())
}

func TestNameNFR03_CrashBetweenCommitAndStrip(t *testing.T) {
	m := newMigEnv(t)
	ctx := context.Background()
	c := m.legacy()
	id := c.Principal.IdentityID

	// Kratos fails after the encrypted copy is committed: the name stays in
	// Kratos, the run reports a failure.
	m.ids.RemoveNameErr = errors.New("kratos down")
	rep, err := m.mig.MigrateKratosNames(ctx, false)
	if err == nil || rep != (app.NameMigrationReport{Scanned: 1, Failed: 1}) {
		t.Fatalf("failed run: %+v %v", rep, err)
	}
	if m.ids.M[id].Name.First != tFirst || m.store.PII[id].Name == nil {
		t.Fatal("committed copy + name still in Kratos expected")
	}
	// Re-run completes it via branch (b).
	m.ids.RemoveNameErr = nil
	rep, err = m.mig.MigrateKratosNames(ctx, false)
	if err != nil || rep != (app.NameMigrationReport{Scanned: 1, StrippedOnly: 1}) {
		t.Fatalf("re-run: %+v %v", rep, err)
	}
	if m.ids.M[id].Name != (identity.Name{}) || m.countAction(audit.ActionCustomerPIINameMigrated) != 1 {
		t.Fatal("re-run must strip once and not audit again")
	}
	g, err := m.svc.GetMine(ctx, c.Principal)
	if err != nil || g.Info.Name == nil || g.Info.Name.Last != tLast {
		t.Fatalf("name after recovery: %v", err)
	}
}

func TestNameNFR03_NeverStripBeforeCommit(t *testing.T) {
	m := newMigEnv(t)
	ctx := context.Background()
	c := m.legacy()
	id := c.Principal.IdentityID
	// The transaction fails at commit: nothing stored, nothing stripped.
	m.store.CommitErr = errors.New("commit failed")
	rep, err := m.mig.MigrateKratosNames(ctx, false)
	if err == nil || rep.Failed != 1 {
		t.Fatalf("commit failure: %+v %v", rep, err)
	}
	if m.ids.M[id].Name.First != tFirst || len(m.ids.NameRemoved) != 0 {
		t.Fatal("the name must stay in Kratos when the copy was not committed")
	}
	if rec, ok := m.store.PII[id]; ok && rec.Name != nil {
		t.Fatal("rolled back")
	}
	if m.countAction(audit.ActionCustomerPIINameMigrated) != 0 {
		t.Fatal("audit rolled back")
	}
	// Key manager down: fails closed, keeps the trait.
	m.store.CommitErr = nil
	m.kms.SetFailure(app.ErrDependencyUnavailable)
	m.cache.Evict(m.store.Keys[id].KeyID)
	rep, err = m.mig.MigrateKratosNames(ctx, false)
	if err == nil || rep.Failed != 1 || m.ids.M[id].Name.First != tFirst {
		t.Fatalf("key manager down: %+v %v", rep, err)
	}
}

func TestNameFR08_InvalidLegacyNameIsKept(t *testing.T) {
	m := newMigEnv(t)
	c := m.legacy()
	i := m.ids.M[c.Principal.IdentityID]
	i.Name.First = tFirst + "\x00"
	m.ids.M[i.ID] = i
	rep, err := m.mig.MigrateKratosNames(context.Background(), false)
	if err == nil || rep.Failed != 1 || m.ids.M[i.ID].Name.First == "" {
		t.Fatalf("invalid name: %+v %v", rep, err)
	}
	assertNoName(t, "logs", m.logs.String())
	// A whitespace-only legacy name has nothing to keep: stripped only.
	i.Name = identity.Name{First: "  "}
	m.ids.M[i.ID] = i
	rep, err = m.mig.MigrateKratosNames(context.Background(), false)
	if err != nil || rep.StrippedOnly != 1 || len(m.store.PII) != 0 {
		t.Fatalf("blank name: %+v %v", rep, err)
	}
	// An empty name object (traits.name = {}) is still removed: the customer
	// schema no longer allows the property.
	e := m.ids.Add(identity.Identity{SchemaID: "customer", HasNameTrait: true})
	rep, err = m.mig.MigrateKratosNames(context.Background(), false)
	if err != nil || rep != (app.NameMigrationReport{Scanned: 1, StrippedOnly: 1}) || m.ids.M[e.ID].HasNameTrait || len(m.store.PII) != 0 {
		t.Fatalf("empty name object: %+v %v", rep, err)
	}
}

func TestNameFR08_ConcurrentNameWinsOverMigration(t *testing.T) {
	m := newMigEnv(t)
	ctx := context.Background()
	c := m.legacy()
	id := c.Principal.IdentityID
	if _, err := m.svc.PutMine(ctx, c, sample(tPhone, tLine1)); err != nil {
		t.Fatal(err)
	}
	// The customer stores a name between the migration's check and its
	// commit: SetName matches no row (rolling back the audit event), the
	// retry sees the stored name and only strips.
	m.svc.Records = &racingRecords{CustomerPIIRepo: m.svc.Records, race: func() {
		if _, err := m.svc.PutMine(ctx, c, pii.PersonalInfo{Name: &pii.Name{First: "Khác"}}); err != nil {
			t.Fatal(err)
		}
	}}
	rep, err := m.mig.MigrateKratosNames(ctx, false)
	if err != nil || rep != (app.NameMigrationReport{Scanned: 1, StrippedOnly: 1}) {
		t.Fatalf("existing name must win: %+v %v", rep, err)
	}
	g, err := m.svc.GetMine(ctx, c.Principal)
	if err != nil || g.Info.Name == nil || g.Info.Name.First != "Khác" {
		t.Fatalf("concurrent name kept: %v", err)
	}
	if m.countAction(audit.ActionCustomerPIINameMigrated) != 0 || m.ids.M[id].Name != (identity.Name{}) {
		t.Fatal("no name_migrated audit; trait stripped")
	}
	// SetName under another key never matches an existing record.
	if set, err := m.store.Repos().PersonalInfo.SetName(ctx, id, uuid.New(), []byte{1}); set || err != nil {
		t.Fatalf("another key: %v %v", set, err)
	}
}

// racingRecords returns the record as it was, then runs race once (a
// concurrent writer between the migration's read and its commit).
type racingRecords struct {
	app.CustomerPIIRepo
	race func()
}

func (r *racingRecords) Get(ctx context.Context, id uuid.UUID) (app.EncryptedPII, error) {
	rec, err := r.CustomerPIIRepo.Get(ctx, id)
	if r.race != nil {
		race := r.race
		r.race = nil
		race()
	}
	return rec, err
}

func (m *migEnv) removalReasons() []string {
	var out []string
	for _, e := range m.store.Events {
		if e.Action == audit.ActionCustomerPIINameTraitRemoved {
			f, _ := e.Details["fields"].([]string)
			if e.ActorID != audit.SystemActor || len(f) != 1 || f[0] != "name" || len(e.Details) != 2 {
				panic("bad name_trait_removed event")
			}
			out = append(out, e.Details["reason"].(string))
		}
	}
	return out
}

// MINOR-3: every strip-only removal is audited with its reason.
func TestNameFR08_StripOnlyIsAudited(t *testing.T) {
	m := newMigEnv(t)
	ctx := context.Background()
	erased := m.legacy()
	if _, err := m.svc.PutMine(ctx, erased, sample(tPhone, tLine1)); err != nil {
		t.Fatal(err)
	}
	if err := m.svc.EraseMine(ctx, erased); err != nil {
		t.Fatal(err)
	}
	if rep, err := m.mig.MigrateKratosNames(ctx, false); err != nil || rep.StrippedOnly != 1 {
		t.Fatalf("erased: %+v %v", rep, err)
	}
	stored := m.legacy()
	if _, err := m.svc.PutMine(ctx, stored, withName(pii.PersonalInfo{})); err != nil {
		t.Fatal(err)
	}
	m.ids.Add(identity.Identity{SchemaID: "customer", HasNameTrait: true})                                  // {}
	m.ids.Add(identity.Identity{SchemaID: "customer", HasNameTrait: true, Name: identity.Name{First: "​"}}) // Cf only
	if rep, err := m.mig.MigrateKratosNames(ctx, false); err != nil || rep.StrippedOnly != 3 {
		t.Fatalf("already/empty: %+v %v", rep, err)
	}
	// Invalid names: kept and failed by default, removed with StripInvalid.
	bad := m.ids.Add(identity.Identity{SchemaID: "customer", HasNameTrait: true, Name: identity.Name{First: "A‮B"}})
	if rep, err := m.mig.MigrateKratosNames(ctx, false); err == nil || rep.Failed != 1 || m.ids.M[bad.ID].Name.First == "" {
		t.Fatalf("invalid default: %+v %v", rep, err)
	}
	m.mig.StripInvalid = true
	if rep, err := m.mig.MigrateKratosNames(ctx, false); err != nil || rep != (app.NameMigrationReport{Scanned: 1, StrippedOnly: 1}) ||
		m.ids.M[bad.ID].HasNameTrait || m.ids.M[bad.ID].Name != (identity.Name{}) {
		t.Fatalf("strip invalid: %+v %v", rep, err)
	}
	if _, ok := m.store.PII[bad.ID]; ok {
		t.Fatal("an invalid name is never stored")
	}
	reasons := m.removalReasons()
	sort.Strings(reasons) // the fake lists identities in id order
	got := strings.Join(reasons, ",")
	if got != "already_migrated,empty,empty,erased,invalid" {
		t.Fatalf("reasons %s", got)
	}
	// Dry run writes no removal audit.
	n := len(m.store.Events)
	m.ids.Add(identity.Identity{SchemaID: "customer", HasNameTrait: true})
	if rep, err := m.mig.MigrateKratosNames(ctx, true); err != nil || rep.StrippedOnly != 1 || len(m.store.Events) != n {
		t.Fatalf("dry run: %+v %v", rep, err)
	}
	assertNoName(t, "audit", m.auditText())
}

// MAJOR-1: an erase committed between decide() and store() wins: the write
// is rolled back, the key created for it removed, the trait only stripped.
func TestNameFR08_EraseBetweenDecideAndStore(t *testing.T) {
	for _, withRecord := range []bool{true, false} {
		m := newMigEnv(t)
		ctx := context.Background()
		c := m.legacy()
		id := c.Principal.IdentityID
		if withRecord {
			if _, err := m.svc.PutMine(ctx, c, sample(tPhone, tLine1)); err != nil {
				t.Fatal(err)
			}
		}
		// Without a record the erase still audits because of the legacy name
		// (MAJOR-2); the trait strip is made to fail so the name stays for
		// the migration to see.
		erase := &app.PersonalInfoService{}
		*erase = *m.svc
		erase.NameTraits = m.ids
		m.svc.Records = &racingRecords{CustomerPIIRepo: m.svc.Records, race: func() {
			m.ids.RemoveNameErr = errors.New("kratos down")
			_ = erase.EraseMine(ctx, c)
			m.ids.RemoveNameErr = nil
		}}
		rep, err := m.mig.MigrateKratosNames(ctx, false)
		if err != nil || rep != (app.NameMigrationReport{Scanned: 1, StrippedOnly: 1}) {
			t.Fatalf("withRecord=%v: %+v %v", withRecord, rep, err)
		}
		if _, ok := m.store.Keys[id]; ok {
			t.Fatalf("withRecord=%v: no key may be left behind", withRecord)
		}
		if _, ok := m.store.PII[id]; ok {
			t.Fatalf("withRecord=%v: nothing may be stored", withRecord)
		}
		if m.countAction(audit.ActionCustomerPIINameMigrated) != 0 || strings.Join(m.removalReasons(), ",") != "erased" {
			t.Fatalf("withRecord=%v: audit %v", withRecord, m.store.AuditActions())
		}
		if m.ids.M[id].Name != (identity.Name{}) {
			t.Fatal("trait must be stripped")
		}
		if !strings.Contains(m.logs.String(), "pii_name_migration_erased_concurrently") {
			t.Fatalf("withRecord=%v: the in-transaction re-check must have fired", withRecord)
		}
	}
}

// MAJOR-2: erasing with no stored record but a legacy name trait records the
// erasure and removes the trait; a later migration stores nothing.
func TestNameFR06_EraseRemovesLegacyNameTrait(t *testing.T) {
	m := newMigEnv(t)
	ctx := context.Background()
	m.svc.NameTraits = m.ids
	c := m.legacy()
	id := c.Principal.IdentityID
	if err := m.svc.EraseMine(ctx, c); err != nil {
		t.Fatal(err)
	}
	if m.countAction(audit.ActionCustomerPIIErased) != 1 || m.ids.M[id].Name != (identity.Name{}) {
		t.Fatal("erase must audit and strip the legacy name")
	}
	if rep, err := m.mig.MigrateKratosNames(ctx, false); err != nil || rep != (app.NameMigrationReport{}) || len(m.store.Keys) != 0 {
		t.Fatalf("migration after erase: %+v %v", rep, err)
	}

	// Kratos fails: the erase is recorded but incomplete (503 → retry); the
	// migration must still not store the name.
	d := m.legacy()
	m.ids.RemoveNameErr = errors.New("kratos down")
	if err := m.svc.EraseMine(ctx, d); !errors.Is(err, app.ErrDependencyUnavailable) {
		t.Fatalf("strip failure must be a dependency error: %v", err)
	}
	m.ids.RemoveNameErr = nil
	if rep, err := m.mig.MigrateKratosNames(ctx, false); err != nil || rep != (app.NameMigrationReport{Scanned: 1, StrippedOnly: 1}) ||
		len(m.store.Keys) != 0 || len(m.store.PII) != 0 {
		t.Fatalf("migration after failed strip: %+v %v", rep, err)
	}
	// Retry of the erase succeeds (idempotent).
	if err := m.svc.EraseMine(ctx, d); err != nil {
		t.Fatal(err)
	}

	// Kratos unreadable: stored data is shredded anyway, erase reported incomplete.
	e := m.legacy()
	if _, err := m.svc.PutMine(ctx, e, sample(tPhone, tLine1)); err != nil {
		t.Fatal(err)
	}
	m.ids.Err = errors.New("kratos unreachable")
	if err := m.svc.EraseMine(ctx, e); !errors.Is(err, app.ErrDependencyUnavailable) {
		t.Fatalf("lookup failure: %v", err)
	}
	m.ids.Err = nil
	if _, ok := m.store.Keys[e.Principal.IdentityID]; ok {
		t.Fatal("the key must be destroyed even when Kratos is down")
	}

	// No data and no trait: nothing to erase, nothing audited.
	plain := m.customer()
	n := len(m.store.Events)
	if err := m.svc.EraseMine(ctx, plain); err != nil || len(m.store.Events) != n {
		t.Fatalf("no-op erase: %v", err)
	}
	assertNoName(t, "logs", m.logs.String())
}
