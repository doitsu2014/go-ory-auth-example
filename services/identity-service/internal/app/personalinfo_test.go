package app_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/adapter/localkms"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/crypto/envelope"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/audit"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/identity"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/pii"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/platform"
)

// Test values; none of them may ever appear in storage, logs or audit rows.
const (
	tPhone   = "+84901234567"
	tLine1   = "12 Nguyen Trai"
	tCity    = "Ho Chi Minh"
	tNID     = "079123456123"
	tDOB     = "1990-05-17"
	tPostal  = "700000"
	tPhoneB  = "+84987654321"
	tLine1B  = "99 Le Loi"
	tTicket  = "SUP-1234"
	logLevel = "debug"
)

var piiValues = []string{tPhone, "901234567", tLine1, tCity, tNID, tDOB, tPostal, tPhoneB, tLine1B}

type piiEnv struct {
	*env
	kms   *localkms.KMS
	cache *app.DEKCache
	logs  *bytes.Buffer
	svc   *app.PersonalInfoService
}

func newPIIEnv(t *testing.T) *piiEnv {
	t.Helper()
	e := newEnv()
	p := &piiEnv{env: e, kms: localkms.NewRandom(), logs: &bytes.Buffer{}}
	p.cache = app.NewDEKCache(100, time.Minute, e.clock.Now)
	p.svc = p.service(p.cache)
	return p
}

func (p *piiEnv) service(cache *app.DEKCache) *app.PersonalInfoService {
	r := p.store.Repos()
	return &app.PersonalInfoService{
		Authz: p.authz, Identities: p.ids, Keys: p.kms, Cache: cache, Tx: p.store,
		SubjectKeys: r.SubjectKeys, Records: r.PersonalInfo, Clock: p.clock,
		Log:           platform.NewLogger(p.logs, logLevel),
		LookupLimiter: app.NewRateLimiter(app.LookupRateRules, p.clock.Now),
		RevealLimiter: app.NewRateLimiter(app.RevealRateRules, p.clock.Now),
		MaskedLimiter: app.NewRateLimiter(app.MaskedRateRules, p.clock.Now),
	}
}

// customer registers a verified customer in the fake Kratos.
func (p *piiEnv) customer() app.Actor {
	pr := customerPrincipal(true)
	p.ids.Add(identity.Identity{ID: pr.IdentityID, SchemaID: "customer", Email: pr.Email})
	return app.Actor{Principal: pr, RequestID: "req-c"}
}

func (p *piiEnv) admin(role identity.Role) app.Actor {
	a := adminActor(uuid.New())
	p.authz.GrantRole(a.Principal.IdentityID, role)
	return a
}

func sample(phone, line1 string) pii.PersonalInfo {
	d := pii.Date{Year: 1990, Month: time.May, Day: 17}
	return pii.PersonalInfo{
		Phone: ptr(phone), DateOfBirth: &d,
		Address:    &pii.Address{Line1: line1, City: tCity, PostalCode: ptr(tPostal), Country: "VN"},
		NationalID: &pii.NationalID{Type: pii.NationalIDCCCD, Number: tNID},
	}
}

// assertNoPII fails if any test value appears in s.
func assertNoPII(t *testing.T, where, s string) {
	t.Helper()
	for _, v := range piiValues {
		if strings.Contains(s, v) {
			t.Fatalf("%s contains PII value %q: %s", where, v, s)
		}
	}
}

func (p *piiEnv) auditText() string {
	var sb strings.Builder
	for _, e := range p.store.Events {
		sb.WriteString(string(e.Action))
		for k, v := range e.Details {
			sb.WriteString(k)
			sb.WriteString(strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(fmtAny(v), "\n", ""), " ", "")))
		}
	}
	return sb.String()
}

func fmtAny(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case []string:
		return strings.Join(x, ",")
	}
	return ""
}

func (p *piiEnv) lastEvent(t *testing.T) audit.Event {
	t.Helper()
	if len(p.store.Events) == 0 {
		t.Fatal("no audit event")
	}
	return p.store.Events[len(p.store.Events)-1]
}

func TestPIIFR01_02_PutGetRoundTripStoresOnlyCiphertext(t *testing.T) {
	p := newPIIEnv(t)
	ctx := context.Background()
	c := p.customer()
	if v, err := p.svc.GetMine(ctx, c.Principal); err != nil || !v.Info.IsZero() || v.UpdatedAt != nil {
		t.Fatalf("empty: %+v %v", v.UpdatedAt, err)
	}
	in := sample(" +84 901 234 567 ", tLine1)
	in.NationalID.Number = strings.ToLower(tNID) // upper-cased by Normalize (digits only here)
	v, err := p.svc.PutMine(ctx, c, in)
	if err != nil || *v.Info.Phone != tPhone || v.UpdatedAt == nil {
		t.Fatalf("put: %v", err)
	}
	got, err := p.svc.GetMine(ctx, c.Principal)
	if err != nil || *got.Info.Phone != tPhone || got.Info.DateOfBirth.ISO() != tDOB || got.Info.Address.Line1 != tLine1 ||
		*got.Info.Address.PostalCode != tPostal || got.Info.NationalID.Number != tNID || got.UpdatedAt == nil {
		t.Fatalf("get: %v", err)
	}

	rec := p.store.PII[c.Principal.IdentityID]
	key := p.store.Keys[c.Principal.IdentityID]
	if rec.KeyID != key.KeyID || rec.PhoneBidx == nil || len(rec.PhoneBidx.Sum) != 32 {
		t.Fatal("record not bound to key / no blind index")
	}
	for _, ct := range [][]byte{rec.Phone, rec.DateOfBirth, rec.Address, rec.NationalID} {
		if len(ct) < envelope.Overhead || ct[0] != envelope.FormatV1 {
			t.Fatal("column is not an envelope ciphertext")
		}
		assertNoPII(t, "ciphertext", string(ct))
	}
	assertNoPII(t, "wrapped dek", key.Wrapped.Ciphertext)

	ev := p.lastEvent(t)
	if ev.Action != audit.ActionCustomerPIIUpdated || ev.ActorID != c.Principal.IdentityID || ev.TargetID != c.Principal.IdentityID.String() {
		t.Fatalf("audit %+v", ev)
	}
	if f, _ := ev.Details["fields"].([]string); strings.Join(f, ",") != "phone_number,date_of_birth,address,national_id" {
		t.Fatalf("audit fields %v", ev.Details)
	}
	assertNoPII(t, "audit", p.auditText())
	assertNoPII(t, "logs", p.logs.String())

	// Full replacement: omitted fields are cleared, same key reused.
	if _, err := p.svc.PutMine(ctx, c, pii.PersonalInfo{Phone: ptr(tPhoneB)}); err != nil {
		t.Fatal(err)
	}
	got, _ = p.svc.GetMine(ctx, c.Principal)
	if *got.Info.Phone != tPhoneB || got.Info.Address != nil || got.Info.NationalID != nil || got.Info.DateOfBirth != nil {
		t.Fatal("PUT must replace the whole record")
	}
	if p.store.Keys[c.Principal.IdentityID].KeyID != key.KeyID {
		t.Fatal("key must be reused")
	}
	if !bytes.Equal(p.store.PII[c.Principal.IdentityID].PhoneBidx.Sum, func() []byte {
		b, _ := p.kms.BlindIndex(ctx, pii.PhoneBlindIndexInput(tPhoneB))
		return b.Sum
	}()) {
		t.Fatal("blind index not updated")
	}
}

func TestPIIFR02_PutGuardsAndValidation(t *testing.T) {
	p := newPIIEnv(t)
	ctx := context.Background()
	c := p.customer()
	unverified := c
	unverified.Principal.EmailVerified = false
	if _, err := p.svc.PutMine(ctx, unverified, sample(tPhone, tLine1)); !errors.Is(err, app.ErrEmailNotVerified) {
		t.Fatalf("unverified: %v", err)
	}
	if _, err := p.svc.PutMine(ctx, adminActor(uuid.New()), sample(tPhone, tLine1)); !errors.Is(err, app.ErrForbidden) {
		t.Fatalf("admin principal: %v", err)
	}
	if _, err := p.svc.GetMine(ctx, adminActor(uuid.New()).Principal); !errors.Is(err, app.ErrForbidden) {
		t.Fatalf("admin get: %v", err)
	}
	bad := sample("0901234567", tLine1)
	bad.Address.Country = "Vietnam"
	_, err := p.svc.PutMine(ctx, c, bad)
	var ve *app.ValidationError
	if !errors.As(err, &ve) || len(ve.Fields) != 2 {
		t.Fatalf("validation: %v", err)
	}
	assertNoPII(t, "validation error", err.Error())
	if len(p.store.Keys) != 0 || len(p.store.PII) != 0 || len(p.store.Events) != 0 {
		t.Fatal("nothing may be stored on validation failure")
	}
}

func TestPIIFR04_EraseCryptoShreds(t *testing.T) {
	p := newPIIEnv(t)
	ctx := context.Background()
	c := p.customer()
	c.Principal.EmailVerified = true
	if _, err := p.svc.PutMine(ctx, c, sample(tPhone, tLine1)); err != nil {
		t.Fatal(err)
	}
	old := p.store.PII[c.Principal.IdentityID]
	oldKey := p.store.Keys[c.Principal.IdentityID]
	if p.cache.Len() != 1 {
		t.Fatal("DEK should be cached after write")
	}
	// §9 A14: erasure does not require a verified email.
	unverified := c
	unverified.Principal.EmailVerified = false
	if err := p.svc.EraseMine(ctx, unverified); err != nil {
		t.Fatal(err)
	}
	if _, ok := p.store.Keys[c.Principal.IdentityID]; ok {
		t.Fatal("subject key must be destroyed")
	}
	if _, ok := p.store.PII[c.Principal.IdentityID]; ok {
		t.Fatal("record must cascade")
	}
	if p.cache.Len() != 0 {
		t.Fatal("cached DEK must be evicted")
	}
	if ev := p.lastEvent(t); ev.Action != audit.ActionCustomerPIIErased || ev.TargetID != c.Principal.IdentityID.String() {
		t.Fatalf("audit %+v", ev)
	}
	if v, err := p.svc.GetMine(ctx, c.Principal); err != nil || !v.Info.IsZero() {
		t.Fatalf("after erase: %v", err)
	}
	n := len(p.store.Events)
	if err := p.svc.EraseMine(ctx, c); err != nil || len(p.store.Events) != n {
		t.Fatalf("idempotent erase must not audit again: %v", err)
	}
	// A new write creates a new key; the old ciphertext stays undecryptable.
	if _, err := p.svc.PutMine(ctx, c, sample(tPhone, tLine1)); err != nil {
		t.Fatal(err)
	}
	newKey := p.store.Keys[c.Principal.IdentityID]
	if newKey.KeyID == oldKey.KeyID {
		t.Fatal("new key id expected")
	}
	dek, err := p.kms.UnwrapDEK(ctx, newKey.Context(), newKey.Wrapped)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := envelope.Open(dek, envelope.AAD("phone_number", c.Principal.IdentityID), old.Phone); !errors.Is(err, envelope.ErrDecrypt) {
		t.Fatalf("old ciphertext must not open with the new key: %v", err)
	}
}

func TestPIINFR02_RowAndColumnSwapFail(t *testing.T) {
	p := newPIIEnv(t)
	ctx := context.Background()
	a, b := p.customer(), p.customer()
	for i, c := range []app.Actor{a, b} {
		if _, err := p.svc.PutMine(ctx, c, sample([]string{tPhone, tPhoneB}[i], []string{tLine1, tLine1B}[i])); err != nil {
			t.Fatal(err)
		}
	}
	ida, idb := a.Principal.IdentityID, b.Principal.IdentityID
	// Column swap inside one row.
	rec := p.store.PII[ida]
	rec.Address, rec.NationalID = rec.NationalID, rec.Address
	p.store.PII[ida] = rec
	if _, err := p.svc.GetMine(ctx, a.Principal); !errors.Is(err, app.ErrDataIntegrity) {
		t.Fatalf("column swap: %v", err)
	}
	// Row swap: B's phone ciphertext copied into A's row.
	rec.Address, rec.NationalID = rec.NationalID, rec.Address
	rec.Phone = p.store.PII[idb].Phone
	p.store.PII[ida] = rec
	if _, err := p.svc.GetMine(ctx, a.Principal); !errors.Is(err, app.ErrDataIntegrity) {
		t.Fatalf("row swap: %v", err)
	}
	// Wrapped DEK swap (§9 A9) with a cold cache.
	ka, kb := p.store.Keys[ida], p.store.Keys[idb]
	kb.Wrapped = ka.Wrapped
	p.store.Keys[idb] = kb
	cold := p.service(app.NewDEKCache(10, time.Minute, p.clock.Now))
	if _, err := cold.GetMine(ctx, b.Principal); !errors.Is(err, app.ErrDataIntegrity) {
		t.Fatalf("wrapped DEK swap: %v", err)
	}
	logs := p.logs.String()
	if !strings.Contains(logs, "pii_decrypt_failed") || !strings.Contains(logs, "pii_dek_unwrap_failed") {
		t.Fatalf("integrity failures must be logged: %s", logs)
	}
	assertNoPII(t, "logs", logs)
}

func TestPIINFR06_KeyManagerDownFailsClosed(t *testing.T) {
	p := newPIIEnv(t)
	ctx := context.Background()
	c := p.customer()
	if _, err := p.svc.PutMine(ctx, c, sample(tPhone, tLine1)); err != nil {
		t.Fatal(err)
	}
	p.kms.SetFailure(app.ErrDependencyUnavailable)
	if _, err := p.svc.PutMine(ctx, c, sample(tPhone, tLine1)); !errors.Is(err, app.ErrDependencyUnavailable) {
		t.Fatalf("put while down: %v", err)
	}
	// Warm cache keeps serving reads (architecture §8); a cold one fails closed.
	if _, err := p.svc.GetMine(ctx, c.Principal); err != nil {
		t.Fatalf("warm read: %v", err)
	}
	cold := p.service(app.NewDEKCache(10, time.Minute, p.clock.Now))
	if _, err := cold.GetMine(ctx, c.Principal); !errors.Is(err, app.ErrDependencyUnavailable) {
		t.Fatalf("cold read while down: %v", err)
	}
	if _, err := p.svc.PutMine(ctx, p.customer(), sample(tPhoneB, tLine1)); !errors.Is(err, app.ErrDependencyUnavailable) {
		t.Fatalf("first write while down: %v", err)
	}
}

func TestPIIFR05_MaskedViewForAdmins(t *testing.T) {
	p := newPIIEnv(t)
	ctx := context.Background()
	c := p.customer()
	if _, err := p.svc.PutMine(ctx, c, sample(tPhone, tLine1)); err != nil {
		t.Fatal(err)
	}
	support := p.admin(identity.RoleSupport)
	v, err := p.svc.GetMasked(ctx, support, c.Principal.IdentityID)
	if err != nil {
		t.Fatal(err)
	}
	m := v.Masked
	if *m.Phone != "+84*******567" || *m.DateOfBirth != "1990-**-**" || m.Address.City != tCity || m.Address.Country != "VN" ||
		m.NationalID.Number != "******123" || v.UpdatedAt == nil {
		t.Fatal("masked values")
	}
	if _, err := p.svc.GetMasked(ctx, c, c.Principal.IdentityID); !errors.Is(err, app.ErrNotAdmin) {
		t.Fatalf("customer: %v", err)
	}
	if _, err := p.svc.GetMasked(ctx, adminActor(uuid.New()), c.Principal.IdentityID); !errors.Is(err, app.ErrForbidden) {
		t.Fatalf("no role: %v", err)
	}
	admin := p.ids.Add(identity.Identity{SchemaID: "admin"})
	if _, err := p.svc.GetMasked(ctx, support, admin.ID); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("non-customer: %v", err)
	}
	if _, err := p.svc.GetMasked(ctx, support, uuid.New()); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("unknown: %v", err)
	}
	empty := p.customer()
	if v, err := p.svc.GetMasked(ctx, support, empty.Principal.IdentityID); err != nil || v.Masked.Phone != nil || v.UpdatedAt != nil {
		t.Fatalf("empty masked: %v", err)
	}
}

func TestPIIFR06_RevealAuditedBeforeReturn(t *testing.T) {
	p := newPIIEnv(t)
	ctx := context.Background()
	c := p.customer()
	if _, err := p.svc.PutMine(ctx, c, sample(tPhone, tLine1)); err != nil {
		t.Fatal(err)
	}
	id := c.Principal.IdentityID
	req := app.RevealRequest{ReasonCode: app.ReasonCustomerSupportRequest, TicketRef: ptr(tTicket)}
	if _, err := p.svc.Reveal(ctx, p.admin(identity.RoleSupport), id, req); !errors.Is(err, app.ErrForbidden) {
		t.Fatalf("support: %v", err)
	}
	admin := p.admin(identity.RoleAdmin)
	for name, bad := range map[string]app.RevealRequest{
		"free text reason": {ReasonCode: "because I want to"},
		"bad ticket":       {ReasonCode: app.ReasonLegalRequest, TicketRef: ptr("sup 1")},
		"unknown field":    {ReasonCode: app.ReasonLegalRequest, Fields: []string{"email"}},
		"duplicate field":  {ReasonCode: app.ReasonLegalRequest, Fields: []string{"address", "address"}},
		"empty fields":     {ReasonCode: app.ReasonLegalRequest, Fields: []string{}},
	} {
		var ve *app.ValidationError
		if _, err := p.svc.Reveal(ctx, admin, id, bad); !errors.As(err, &ve) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	n := len(p.store.Events)
	v, err := p.svc.Reveal(ctx, admin, id, req)
	if err != nil || *v.Info.Phone != tPhone || v.Info.NationalID.Number != tNID {
		t.Fatalf("reveal: %v", err)
	}
	if len(p.store.Events) != n+1 {
		t.Fatal("reveal must write exactly one audit row")
	}
	ev := p.lastEvent(t)
	if ev.Action != audit.ActionCustomerPIIRevealed || ev.ActorID != admin.Principal.IdentityID || ev.TargetID != id.String() ||
		ev.Details["reason_code"] != "customer_support_request" || ev.Details["ticket_ref"] != tTicket {
		t.Fatalf("audit %+v", ev)
	}
	assertNoPII(t, "audit", p.auditText())

	// Data minimisation: only the requested fields.
	v, err = p.svc.Reveal(ctx, admin, id, app.RevealRequest{ReasonCode: app.ReasonFraudInvestigation, Fields: []string{"phone_number"}})
	if err != nil || v.Info.Phone == nil || v.Info.Address != nil || v.Info.NationalID != nil {
		t.Fatalf("fields subset: %v", err)
	}
	if f, _ := p.lastEvent(t).Details["fields"].([]string); len(f) != 1 || f[0] != "phone_number" {
		t.Fatalf("audit fields %v", f)
	}

	// Audit failure → nothing revealed (DD-8).
	p.store.AuditErr = errors.New("db down")
	v, err = p.svc.Reveal(ctx, admin, id, req)
	if err == nil || v.Info.Phone != nil {
		t.Fatal("reveal without audit must fail and return nothing")
	}
	p.store.AuditErr = nil
}

func TestA13_RevealQuotaPerActor(t *testing.T) {
	p := newPIIEnv(t)
	ctx := context.Background()
	c := p.customer()
	admin, other := p.admin(identity.RoleSuperAdmin), p.admin(identity.RoleSuperAdmin)
	req := app.RevealRequest{ReasonCode: app.ReasonIdentityVerification}
	for i := range 20 {
		if _, err := p.svc.Reveal(ctx, admin, c.Principal.IdentityID, req); err != nil {
			t.Fatalf("reveal %d: %v", i, err)
		}
	}
	if _, err := p.svc.Reveal(ctx, admin, c.Principal.IdentityID, req); !errors.Is(err, app.ErrRateLimited) {
		t.Fatalf("21st reveal: %v", err)
	}
	if _, err := p.svc.Reveal(ctx, other, c.Principal.IdentityID, req); err != nil {
		t.Fatalf("quota is per actor: %v", err)
	}
	p.clock.T = p.clock.T.Add(time.Hour + time.Second)
	if _, err := p.svc.Reveal(ctx, admin, c.Principal.IdentityID, req); err != nil {
		t.Fatalf("after an hour: %v", err)
	}
}

func TestPIIFR07_LookupByPhone(t *testing.T) {
	p := newPIIEnv(t)
	ctx := context.Background()
	a, b := p.customer(), p.customer()
	if _, err := p.svc.PutMine(ctx, a, sample(tPhone, tLine1)); err != nil {
		t.Fatal(err)
	}
	if _, err := p.svc.PutMine(ctx, b, sample(tPhoneB, tLine1B)); err != nil {
		t.Fatal(err)
	}
	support := p.admin(identity.RoleSupport)
	res, err := p.svc.LookupByPhone(ctx, support, "+84 901-234-567")
	ms := res.Matches
	if err != nil || len(ms) != 1 || ms[0].IdentityID != a.Principal.IdentityID || ms[0].State != identity.StateActive ||
		*ms[0].Masked.Phone != "+84*******567" {
		t.Fatalf("lookup: %+v %v", len(ms), err)
	}
	ev := p.lastEvent(t)
	want, _ := p.kms.BlindIndex(ctx, pii.PhoneBlindIndexInput(tPhone))
	ids, _ := ev.Details["matched_ids"].([]string)
	if ev.Action != audit.ActionCustomerPIILookup || ev.Details["bidx"] != hex.EncodeToString(want.Sum) ||
		len(ids) != 1 || ids[0] != a.Principal.IdentityID.String() || ev.Details["matches"] != 1 {
		t.Fatalf("lookup audit %+v", ev.Details)
	}
	assertNoPII(t, "audit", p.auditText())

	// §9 A10: a swapped blind index does not produce a false match.
	rb := p.store.PII[b.Principal.IdentityID]
	rb.PhoneBidx = p.store.PII[a.Principal.IdentityID].PhoneBidx
	p.store.PII[b.Principal.IdentityID] = rb
	res, err = p.svc.LookupByPhone(ctx, support, tPhone)
	ms = res.Matches
	if err != nil || len(ms) != 1 || ms[0].IdentityID != a.Principal.IdentityID {
		t.Fatalf("swapped bidx must be filtered: %d %v", len(ms), err)
	}
	if res, err := p.svc.LookupByPhone(ctx, support, "+15555550000"); err != nil || len(res.Matches) != 0 || res.Truncated {
		t.Fatalf("no match: %v", err)
	}
	var ve *app.ValidationError
	if _, err := p.svc.LookupByPhone(ctx, support, "0901234567"); !errors.As(err, &ve) {
		t.Fatalf("invalid phone: %v", err)
	}
	if _, err := p.svc.LookupByPhone(ctx, a, tPhone); !errors.Is(err, app.ErrNotAdmin) {
		t.Fatalf("customer: %v", err)
	}
	assertNoPII(t, "logs", p.logs.String())
}

func TestB3_LookupQuotaPerActor(t *testing.T) {
	p := newPIIEnv(t)
	ctx := context.Background()
	support := p.admin(identity.RoleSupport)
	for i := range 30 {
		if _, err := p.svc.LookupByPhone(ctx, support, tPhone); err != nil {
			t.Fatalf("lookup %d: %v", i, err)
		}
	}
	if _, err := p.svc.LookupByPhone(ctx, support, tPhone); !errors.Is(err, app.ErrRateLimited) {
		t.Fatalf("31st lookup in a minute: %v", err)
	}
	// 200/day: advance a minute between bursts of 30.
	total := 30
	for total < 200 {
		p.clock.T = p.clock.T.Add(time.Minute + time.Second)
		for range 30 {
			if total == 200 {
				break
			}
			if _, err := p.svc.LookupByPhone(ctx, support, tPhone); err != nil {
				t.Fatalf("lookup %d: %v", total, err)
			}
			total++
		}
	}
	p.clock.T = p.clock.T.Add(time.Minute + time.Second)
	if _, err := p.svc.LookupByPhone(ctx, support, tPhone); !errors.Is(err, app.ErrRateLimited) {
		t.Fatalf("201st lookup in a day: %v", err)
	}
}

func TestPIIFR02_ConcurrentFirstWritesShareOneKey(t *testing.T) {
	p := newPIIEnv(t)
	ctx := context.Background()
	c := p.customer()
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Go(func() {
			_, err := p.svc.PutMine(ctx, c, sample(tPhone, tLine1))
			errs <- err
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(p.store.Keys) != 1 {
		t.Fatal("one key per customer")
	}
	cold := p.service(app.NewDEKCache(10, time.Minute, p.clock.Now))
	if v, err := cold.GetMine(ctx, c.Principal); err != nil || *v.Info.Phone != tPhone {
		t.Fatalf("readable with the stored key: %v", err)
	}
}

func TestPIIFR09_RewrapAfterRotation(t *testing.T) {
	p := newPIIEnv(t)
	ctx := context.Background()
	var actors []app.Actor
	for range 5 {
		c := p.customer()
		actors = append(actors, c)
		if _, err := p.svc.PutMine(ctx, c, sample(tPhone, tLine1)); err != nil {
			t.Fatal(err)
		}
	}
	rot := &app.KeyRotationService{Keys: p.kms, SubjectKeys: p.store.Repos().SubjectKeys}
	res, err := rot.Rewrap(ctx, 2)
	if err != nil || res.Scanned != 5 || res.Rewrapped != 0 || res.Current != 5 || res.LatestVersion != 1 {
		t.Fatalf("before rotation: %+v %v", res, err)
	}
	p.kms.Rotate()
	res, err = rot.Rewrap(ctx, 2)
	if err != nil || res.Scanned != 5 || res.Rewrapped != 5 || res.LatestVersion != 2 {
		t.Fatalf("after rotation: %+v %v", res, err)
	}
	for _, k := range p.store.Keys {
		if k.Wrapped.KEKVersion != 2 || k.RewrappedAt == nil {
			t.Fatalf("key not re-wrapped: %+v", k.Wrapped.KEKVersion)
		}
	}
	cold := p.service(app.NewDEKCache(10, time.Minute, p.clock.Now))
	for _, c := range actors {
		if v, err := cold.GetMine(ctx, c.Principal); err != nil || *v.Info.Phone != tPhone {
			t.Fatalf("readable after rewrap: %v", err)
		}
	}
	res, err = rot.Rewrap(ctx, 2)
	if err != nil || res.Rewrapped != 0 || res.Current != 5 {
		t.Fatalf("idempotent: %+v %v", res, err)
	}
	// A corrupted wrapped key is reported, the rest proceed.
	p.kms.Rotate()
	id := actors[0].Principal.IdentityID
	k := p.store.Keys[id]
	k.Wrapped.Ciphertext = "local:v2:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	p.store.Keys[id] = k
	res, err = rot.Rewrap(ctx, 10)
	if !errors.Is(err, app.ErrDataIntegrity) || res.Failed != 1 || res.Rewrapped != 4 {
		t.Fatalf("partial failure: %+v %v", res, err)
	}
}

func TestB1_ReapplyErasures(t *testing.T) {
	p := newPIIEnv(t)
	ctx := context.Background()
	erased, kept := p.customer(), p.customer()
	for _, c := range []app.Actor{erased, kept} {
		if _, err := p.svc.PutMine(ctx, c, sample(tPhone, tLine1)); err != nil {
			t.Fatal(err)
		}
	}
	backupKey := p.store.Keys[erased.Principal.IdentityID]
	backupRec := p.store.PII[erased.Principal.IdentityID]
	p.clock.T = p.clock.T.Add(time.Minute)
	if err := p.svc.EraseMine(ctx, erased); err != nil {
		t.Fatal(err)
	}
	// Simulate a restore of the pre-erasure row.
	p.store.Keys[erased.Principal.IdentityID] = backupKey
	p.store.PII[erased.Principal.IdentityID] = backupRec
	rot := &app.KeyRotationService{SubjectKeys: p.store.Repos().SubjectKeys}
	n, err := rot.ReapplyErasures(ctx)
	if err != nil || n != 1 {
		t.Fatalf("reapply: %d %v", n, err)
	}
	if _, ok := p.store.Keys[erased.Principal.IdentityID]; ok {
		t.Fatal("restored key must be destroyed again")
	}
	if _, ok := p.store.Keys[kept.Principal.IdentityID]; !ok {
		t.Fatal("other customers are untouched")
	}
	// Data written after the erasure is kept.
	p.clock.T = p.clock.T.Add(time.Minute)
	if _, err := p.svc.PutMine(ctx, erased, sample(tPhone, tLine1)); err != nil {
		t.Fatal(err)
	}
	if n, _ := rot.ReapplyErasures(ctx); n != 0 {
		t.Fatalf("keys newer than the erasure must survive: %d", n)
	}
}

func TestDD5_DEKCache(t *testing.T) {
	clock := &struct{ t time.Time }{t: t0}
	now := func() time.Time { return clock.t }
	c := app.NewDEKCache(2, time.Minute, now)
	k1, k2, k3 := uuid.New(), uuid.New(), uuid.New()
	d1 := bytes.Repeat([]byte{1}, 32)
	c.Put(k1, d1)
	d1[0] = 9 // the cache holds its own copy
	got, ok := c.Get(k1)
	if !ok || got[0] != 1 {
		t.Fatal("copy on put")
	}
	got[1] = 7 // ...and returns a copy
	if again, _ := c.Get(k1); again[1] != 1 {
		t.Fatal("copy on get")
	}
	c.Put(k2, bytes.Repeat([]byte{2}, 32))
	c.Get(k1) // k1 most recently used
	c.Put(k3, bytes.Repeat([]byte{3}, 32))
	if _, ok := c.Get(k2); ok {
		t.Fatal("LRU must evict k2")
	}
	if _, ok := c.Get(k1); !ok {
		t.Fatal("k1 must survive")
	}
	held, _ := c.Get(k3)
	c.Evict(k3)
	if _, ok := c.Get(k3); ok || held[0] != 3 {
		t.Fatal("evict removes the entry; copies already handed out are unaffected")
	}
	clock.t = clock.t.Add(time.Minute)
	if _, ok := c.Get(k1); ok {
		t.Fatal("TTL")
	}
	c.Put(k1, d1)
	clock.t = clock.t.Add(2 * time.Minute)
	if n := c.SweepExpired(); n != 1 || c.Len() != 0 {
		t.Fatalf("sweep: %d", n)
	}
	var nilCache *app.DEKCache
	nilCache.Put(k1, d1)
	if _, ok := nilCache.Get(k1); ok {
		t.Fatal("nil cache is a no-op")
	}
}

func TestRateLimiterWindows(t *testing.T) {
	now := t0
	l := app.NewRateLimiter([]app.RateRule{{Limit: 2, Window: time.Minute}}, func() time.Time { return now })
	a := uuid.New()
	if !l.Allow(a) || !l.Allow(a) || l.Allow(a) {
		t.Fatal("limit 2/min")
	}
	now = now.Add(30 * time.Second)
	if l.Allow(a) {
		t.Fatal("still inside the window")
	}
	now = now.Add(31 * time.Second)
	if !l.Allow(a) {
		t.Fatal("window slid")
	}
	var nilLimiter *app.RateLimiter
	if !nilLimiter.Allow(a) {
		t.Fatal("nil limiter allows")
	}
}
