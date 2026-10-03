//go:build integration

package e2e

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/crypto/envelope"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/audit"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/identity"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/pii"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/testutil"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/testutil/itest"
)

// Values used by the PII e2e tests. None may appear in rows, logs or audit.
const (
	ePhone  = "+84912345678"
	eLine1  = "45 Hai Ba Trung"
	eCity   = "Da Nang"
	eNID    = "048099123456"
	eDOB    = "1988-11-23"
	ePostal = "550000"
)

var eValues = []string{ePhone, "912345678", eLine1, eCity, eNID, eDOB, ePostal}

func ePII() map[string]any {
	return map[string]any{
		"phone_number": ePhone, "date_of_birth": eDOB,
		"address":     map[string]any{"line1": eLine1, "city": eCity, "postal_code": ePostal, "country": "VN"},
		"national_id": map[string]any{"type": "cccd", "number": eNID},
	}
}

func noPII(t *testing.T, where string, b []byte) {
	t.Helper()
	for _, v := range eValues {
		if bytes.Contains(b, []byte(v)) || bytes.Contains(b, []byte(hex.EncodeToString([]byte(v)))) {
			t.Fatalf("%s contains %q", where, v)
		}
	}
}

// verifiedCustomer registers a customer and verifies its email.
func (s *stack) verifiedCustomer(t *testing.T) (string, uuid.UUID) {
	t.Helper()
	email := itest.UniqueEmail("e2e-pii-cust")
	reg := s.env.RegisterCustomer(t, email)
	id := uuid.MustParse(reg.Session.Identity.ID)
	t.Cleanup(func() {
		_, _ = s.store.Repos().SubjectKeys.Delete(context.Background(), id)
		s.env.DeleteIdentity(t, id)
	})
	s.env.VerifyEmail(t, email)
	s.verifier.Invalidate(id)
	return reg.SessionToken, id
}

// adminSession returns an AAL2 admin cookie for role (TOTP enrolment
// upgrades the session to aal2 on Kratos v26.2.0, see TestFR06).
func (s *stack) adminSession(t *testing.T, role identity.Role) (string, uuid.UUID) {
	t.Helper()
	email := itest.UniqueEmail("e2e-pii-" + string(role))
	id := s.env.CreateAdminWithPassword(t, email)
	t.Cleanup(func() {
		_ = s.keto.RemoveAll(context.Background(), id)
		s.env.DeleteIdentity(t, id)
	})
	if err := s.keto.SetRole(context.Background(), id, role); err != nil {
		t.Fatal(err)
	}
	b := s.env.NewBrowser()
	if st, _ := b.Login(t, email, ""); st != 200 {
		t.Fatalf("admin login: %d", st)
	}
	b.EnrolTOTP(t)
	s.verifier.Invalidate(id)
	return b.SessionCookie(), id
}

type rawRow struct {
	phone, dob, addr, nid, bidx []byte
	keyID                       uuid.UUID
	wrapped                     string
	text                        string
}

func (s *stack) raw(t *testing.T, id uuid.UUID) (rawRow, bool) {
	t.Helper()
	var r rawRow
	err := s.pool.QueryRow(context.Background(), `
		SELECT p.phone_ct, p.dob_ct, p.address_ct, p.national_id_ct, p.phone_bidx, k.key_id, k.wrapped_dek,
		       p::text || k::text
		FROM customer_pii p JOIN subject_key k USING (identity_id) WHERE p.identity_id = $1`, id).
		Scan(&r.phone, &r.dob, &r.addr, &r.nid, &r.bidx, &r.keyID, &r.wrapped, &r.text)
	if err != nil {
		return r, false
	}
	return r, true
}

func (s *stack) count(t *testing.T, q string, args ...any) int {
	t.Helper()
	var n int
	if err := s.pool.QueryRow(context.Background(), q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// TestPII_E2E_Lifecycle: customer PUT/GET → ciphertext-only rows → masked
// admin view → reveal (audited) → lookup → erase (crypto-shred).
func TestPII_E2E_Lifecycle(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()
	tok, custID := s.verifiedCustomer(t)

	var p problem
	if st := s.api(t, "PUT", "/v1/me/personal-info", call{bearer: tok}, map[string]any{"phone_number": "12"}, &p); st != 422 || p.Code != "validation_failed" {
		t.Fatalf("invalid PUT: %d %s", st, p.Code)
	}
	var info map[string]any
	if st := s.api(t, "PUT", "/v1/me/personal-info", call{bearer: tok}, ePII(), &info); st != 200 || info["phone_number"] != ePhone {
		t.Fatalf("PUT: %d %v", st, st)
	}
	info = nil
	if st := s.api(t, "GET", "/v1/me/personal-info", call{bearer: tok}, nil, &info); st != 200 || info["date_of_birth"] != eDOB {
		t.Fatalf("GET: %d", st)
	}

	// PII-NFR-04: raw rows hold ciphertext only.
	row, ok := s.raw(t, custID)
	if !ok {
		t.Fatal("no row")
	}
	for _, ct := range [][]byte{row.phone, row.dob, row.addr, row.nid} {
		if len(ct) < envelope.Overhead || ct[0] != envelope.FormatV1 {
			t.Fatal("column is not an envelope ciphertext")
		}
	}
	if len(row.bidx) != 32 || !strings.HasPrefix(row.wrapped, "vault:v") {
		t.Fatalf("bidx %d wrapped %q", len(row.bidx), row.wrapped[:8])
	}
	noPII(t, "raw row", []byte(row.text))

	// Admin plane.
	supportCookie, _ := s.adminSession(t, identity.RoleSupport)
	adminCookie, adminID := s.adminSession(t, identity.RoleAdmin)
	var masked map[string]any
	if st := s.api(t, "GET", "/admin/v1/customers/"+custID.String()+"/personal-info", call{cookie: supportCookie}, nil, &masked); st != 200 ||
		masked["phone_number"] != "+84*******678" || masked["date_of_birth"] != "1988-**-**" || masked["has_national_id"] != true {
		t.Fatalf("masked: %d %v", st, masked)
	}
	var me struct {
		Permissions []string `json:"permissions"`
	}
	if st := s.api(t, "GET", "/admin/v1/me", call{cookie: adminCookie}, nil, &me); st != 200 || !strings.Contains(strings.Join(me.Permissions, ","), "reveal_customer_pii") {
		t.Fatalf("admin me permissions (Keto OPL reloaded?): %d %v", st, me.Permissions)
	}
	reveal := "/admin/v1/customers/" + custID.String() + "/personal-info/reveal"
	body := map[string]any{"reason_code": "identity_verification", "ticket_ref": "KYC-77"}
	if st := s.api(t, "POST", reveal, call{cookie: supportCookie, origin: webOrigin}, body, &p); st != 403 {
		t.Fatalf("support reveal: %d", st)
	}
	info = nil
	if st := s.api(t, "POST", reveal, call{cookie: adminCookie, origin: webOrigin}, body, &info); st != 200 || info["phone_number"] != ePhone {
		t.Fatalf("reveal: %d", st)
	}
	if n := s.count(t, `SELECT count(*) FROM audit_event WHERE action = 'customer.pii.revealed' AND actor_identity_id = $1
		AND target_id = $2 AND details->>'reason_code' = 'identity_verification' AND details->>'ticket_ref' = 'KYC-77'`, adminID, custID.String()); n != 1 {
		t.Fatalf("reveal audit rows: %d", n)
	}
	var lookup struct {
		Items []struct {
			ID    uuid.UUID `json:"id"`
			State string    `json:"state"`
		} `json:"items"`
	}
	if st := s.api(t, "POST", "/admin/v1/customers/lookup", call{cookie: supportCookie, origin: webOrigin},
		map[string]any{"phone_number": "+84 912 345 678"}, &lookup); st != 200 || len(lookup.Items) == 0 {
		t.Fatalf("lookup: %d %+v", st, lookup)
	}
	found := false // other customers may self-declare the same number (phone_verified: false)
	for _, it := range lookup.Items {
		found = found || it.ID == custID
	}
	if !found {
		t.Fatalf("lookup misses the customer: %+v", lookup)
	}

	// Erase: the key row is destroyed and the record cascades.
	if st := s.api(t, "DELETE", "/v1/me/personal-info", call{bearer: tok}, nil, nil); st != 204 {
		t.Fatalf("erase: %d", st)
	}
	if n := s.count(t, `SELECT count(*) FROM subject_key WHERE identity_id = $1`, custID) +
		s.count(t, `SELECT count(*) FROM customer_pii WHERE identity_id = $1`, custID); n != 0 {
		t.Fatalf("rows left after erase: %d", n)
	}
	info = nil
	if st := s.api(t, "GET", "/v1/me/personal-info", call{bearer: tok}, nil, &info); st != 200 || info["phone_number"] != nil {
		t.Fatalf("GET after erase: %d %v", st, info["phone_number"] != nil)
	}
	// The old ciphertext cannot be opened with the new subject key.
	if st := s.api(t, "PUT", "/v1/me/personal-info", call{bearer: tok}, ePII(), nil); st != 200 {
		t.Fatalf("PUT after erase: %d", st)
	}
	k, err := s.store.Repos().SubjectKeys.Get(ctx, custID)
	if err != nil || k.KeyID == row.keyID {
		t.Fatalf("new key expected: %v", err)
	}
	dek, err := s.bao.UnwrapDEK(ctx, k.Context(), k.Wrapped)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := envelope.Open(dek, envelope.AAD("phone_number", custID), row.phone); !errors.Is(err, envelope.ErrDecrypt) {
		t.Fatalf("old ciphertext must stay undecryptable: %v", err)
	}
	if n := s.count(t, `SELECT count(*) FROM audit_event WHERE action IN ('customer.pii.updated','customer.pii.erased') AND target_id = $1`, custID.String()); n != 3 {
		t.Fatalf("updated/erased audit rows: %d", n)
	}

	var audits []string
	rows, _ := s.pool.Query(ctx, `SELECT details::text FROM audit_event WHERE target_id = $1 OR action = 'customer.pii.lookup' ORDER BY id DESC LIMIT 50`, custID.String())
	for rows.Next() {
		var d string
		_ = rows.Scan(&d)
		audits = append(audits, d)
	}
	rows.Close()
	noPII(t, "audit details", []byte(strings.Join(audits, "\n")))
	noPII(t, "service logs", []byte(s.logs.String()))
}

// synthetic returns a verified customer actor that exists only in the
// database (service-level tests do not need Kratos).
func synthetic() app.Actor {
	return app.Actor{RequestID: "itest-pii", Principal: identity.Principal{
		IdentityID: uuid.New(), Kind: identity.KindCustomer, EmailVerified: true,
	}}
}

func samplePII(phone string) pii.PersonalInfo {
	d := pii.Date{Year: 1988, Month: time.November, Day: 23}
	return pii.PersonalInfo{Phone: &phone, DateOfBirth: &d,
		Address:    &pii.Address{Line1: eLine1, City: eCity, Country: "VN"},
		NationalID: &pii.NationalID{Type: pii.NationalIDCCCD, Number: eNID}}
}

func (s *stack) freshService() *app.PersonalInfoService {
	cp := *s.pii
	cp.Cache = app.NewDEKCache(100, time.Minute, nil)
	return &cp
}

func (s *stack) cleanupSubject(t *testing.T, id uuid.UUID) {
	t.Cleanup(func() { _, _ = s.store.Repos().SubjectKeys.Delete(context.Background(), id) })
}

// TestPIINFR02_E2E_SwapsFailAuthentication: ciphertext moved to another row
// or column, or a wrapped DEK moved to another subject, does not decrypt.
func TestPIINFR02_E2E_SwapsFailAuthentication(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()
	a, b := synthetic(), synthetic()
	for i, c := range []app.Actor{a, b} {
		s.cleanupSubject(t, c.Principal.IdentityID)
		if _, err := s.pii.PutMine(ctx, c, samplePII([]string{ePhone, "+84912000111"}[i])); err != nil {
			t.Fatal(err)
		}
	}
	ida, idb := a.Principal.IdentityID, b.Principal.IdentityID
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := s.pool.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	// Row swap: B's phone ciphertext into A's row.
	exec(`UPDATE customer_pii SET phone_ct = (SELECT phone_ct FROM customer_pii WHERE identity_id = $2) WHERE identity_id = $1`, ida, idb)
	if _, err := s.pii.GetMine(ctx, a.Principal); !errors.Is(err, app.ErrDataIntegrity) {
		t.Fatalf("row swap: %v", err)
	}
	// Column swap inside B's row.
	exec(`UPDATE customer_pii SET address_ct = national_id_ct WHERE identity_id = $1`, idb)
	if _, err := s.pii.GetMine(ctx, b.Principal); !errors.Is(err, app.ErrDataIntegrity) {
		t.Fatalf("column swap: %v", err)
	}
	// §9 A9: wrapped DEK copied to another subject → OpenBao rejects the AD.
	exec(`UPDATE subject_key SET wrapped_dek = (SELECT wrapped_dek FROM subject_key WHERE identity_id = $2) WHERE identity_id = $1`, idb, ida)
	if _, err := s.freshService().GetMine(ctx, b.Principal); !errors.Is(err, app.ErrDataIntegrity) {
		t.Fatalf("wrapped DEK swap: %v", err)
	}
	if !strings.Contains(s.logs.String(), "pii_dek_unwrap_failed") || !strings.Contains(s.logs.String(), "pii_decrypt_failed") {
		t.Fatal("integrity failures must be logged")
	}
	noPII(t, "logs", []byte(s.logs.String()))
}

var latestRe = regexp.MustCompile(`kek latest_version: (\d+)`)

// TestPIIFR09_E2E_RotateAndRewrap: `make kek-rotate`, then the rewrap use
// case moves every key to the newest KEK version and data stays readable.
func TestPIIFR09_E2E_RotateAndRewrap(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()
	var actors []app.Actor
	for range 3 {
		c := synthetic()
		s.cleanupSubject(t, c.Principal.IdentityID)
		if _, err := s.pii.PutMine(ctx, c, samplePII(ePhone)); err != nil {
			t.Fatal(err)
		}
		actors = append(actors, c)
	}
	cmd := exec.CommandContext(ctx, "make", "kek-rotate")
	cmd.Dir = itest.RepoRoot()
	out, err := cmd.CombinedOutput()
	m := latestRe.FindSubmatch(out)
	if err != nil || m == nil {
		t.Fatalf("make kek-rotate: %v\n%s", err, out)
	}
	latest, _ := strconv.Atoi(string(m[1]))
	rot := &app.KeyRotationService{Keys: s.bao, SubjectKeys: s.store.Repos().SubjectKeys, Log: s.pii.Log}
	res, err := rot.Rewrap(ctx, 2)
	if err != nil {
		t.Fatalf("rewrap: %+v %v", res, err)
	}
	if res.LatestVersion != latest || res.Rewrapped < len(actors) {
		t.Fatalf("rewrap result %+v (latest %d)", res, latest)
	}
	fresh := s.freshService()
	for _, c := range actors {
		k, err := s.store.Repos().SubjectKeys.Get(ctx, c.Principal.IdentityID)
		if err != nil || k.Wrapped.KEKVersion != latest || !strings.HasPrefix(k.Wrapped.Ciphertext, "vault:v"+string(m[1])+":") || k.RewrappedAt == nil {
			t.Fatalf("key not on v%d: %+v %v", latest, k.Wrapped.KEKVersion, err)
		}
		if v, err := fresh.GetMine(ctx, c.Principal); err != nil || *v.Info.Phone != ePhone {
			t.Fatalf("readable after rewrap: %v", err)
		}
	}
	again, err := rot.Rewrap(ctx, 50)
	if err != nil || again.Rewrapped != 0 {
		t.Fatalf("second run must be a no-op: %+v %v", again, err)
	}
}

// failingAuditTx fails every audit append inside its transactions.
type failingAuditTx struct{ next app.TxRunner }

type failingAudit struct{ app.AuditRepo }

func (failingAudit) Append(context.Context, audit.Event) error {
	return errors.New("audit unavailable")
}

func (f failingAuditTx) WithinTx(ctx context.Context, fn func(context.Context, app.Repos) error) error {
	return f.next.WithinTx(ctx, func(ctx context.Context, r app.Repos) error {
		r.Audit = failingAudit{r.Audit}
		return fn(ctx, r)
	})
}

// TestDD8_E2E_RevealAuditAtomicity: without a committed audit row nothing is
// revealed; with one, the row exists by the time Reveal returns.
func TestDD8_E2E_RevealAuditAtomicity(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()
	c := synthetic()
	s.cleanupSubject(t, c.Principal.IdentityID)
	if _, err := s.pii.PutMine(ctx, c, samplePII(ePhone)); err != nil {
		t.Fatal(err)
	}
	admin := app.Actor{RequestID: "itest-reveal", Principal: identity.Principal{IdentityID: uuid.New(), Kind: identity.KindAdmin}}
	authz := testutil.NewAuthz()
	authz.GrantRole(admin.Principal.IdentityID, identity.RoleAdmin)
	ids := testutil.NewIdentities(nil)
	ids.Add(identity.Identity{ID: c.Principal.IdentityID, SchemaID: "customer"})
	svc := s.freshService()
	svc.Authz, svc.Identities = authz, ids
	req := app.RevealRequest{ReasonCode: app.ReasonLegalRequest}
	countReveals := func() int {
		return s.count(t, `SELECT count(*) FROM audit_event WHERE action = 'customer.pii.revealed' AND target_id = $1`, c.Principal.IdentityID.String())
	}

	broken := *svc
	broken.Tx = failingAuditTx{next: s.store}
	v, err := broken.Reveal(ctx, admin, c.Principal.IdentityID, req)
	if err == nil || v.Info.Phone != nil || countReveals() != 0 {
		t.Fatalf("reveal without audit: err=%v revealed=%v rows=%d", err, v.Info.Phone != nil, countReveals())
	}
	v, err = svc.Reveal(ctx, admin, c.Principal.IdentityID, req)
	if err != nil || *v.Info.Phone != ePhone {
		t.Fatalf("reveal: %v", err)
	}
	if n := countReveals(); n != 1 {
		t.Fatalf("audit row must be committed before Reveal returns: %d", n)
	}
}

// TestPIINFR06_E2E_OpenBaoUnreachable: PII endpoints fail closed with 503
// dependency_unavailable; other endpoints are unaffected.
func TestPIINFR06_E2E_OpenBaoUnreachable(t *testing.T) {
	up := newStack(t)
	tok, _ := up.verifiedCustomer(t)
	if st := up.api(t, "PUT", "/v1/me/personal-info", call{bearer: tok}, ePII(), nil); st != 200 {
		t.Fatalf("PUT while up: %d", st)
	}
	s := newStackWith(t, stackOpts{baoAddr: "http://127.0.0.1:1"}) // same DB, cold cache, OpenBao unreachable
	var p problem
	for _, m := range []string{"PUT", "GET"} {
		var body any
		if m == "PUT" {
			body = ePII()
		}
		if st := s.api(t, m, "/v1/me/personal-info", call{bearer: tok}, body, &p); st != 503 || p.Code != "dependency_unavailable" {
			t.Fatalf("%s with OpenBao down: %d %s", m, st, p.Code)
		}
	}
	if st := s.api(t, "GET", "/v1/me", call{bearer: tok}, nil, nil); st != 200 {
		t.Fatalf("non-PII endpoint: %d", st)
	}
	noPII(t, "logs", []byte(s.logs.String()))
}
