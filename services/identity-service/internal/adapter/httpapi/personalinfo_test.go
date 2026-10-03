package httpapi

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/identity"
)

const validPII = `{"phone_number":"+84 901 234 567","date_of_birth":"1990-05-17",` +
	`"address":{"line1":"12 Nguyen Trai","line2":null,"city":"Ho Chi Minh","postal_code":"700000","country":"VN"},` +
	`"national_id":{"type":"cccd","number":"079123456123"}}`

// Values that must never reach logs, audit rows or error bodies.
var piiTestValues = []string{"901234567", "901 234 567", "Nguyen Trai", "Ho Chi Minh", "079123456123", "1990-05-17", "700000", "0909999999", "Hacker Street"}

func assertNoPIIIn(t *testing.T, where, s string) {
	t.Helper()
	for _, v := range piiTestValues {
		if strings.Contains(s, v) {
			t.Fatalf("%s leaks %q: %s", where, v, s)
		}
	}
}

func jsonRoute(method, path, body string) route {
	r := route{method: method, path: path}
	if body != "" {
		r.body = func(*fixture) string { return body }
	}
	return r
}

// customerID returns the id of the "cust" principal and registers it as a
// customer identity in the fake Kratos (for the admin routes).
func (f *fixture) customerID(t *testing.T) uuid.UUID {
	t.Helper()
	rec := f.do(t, jsonRoute("GET", "/v1/me", ""), cred{bearer: "cust"})
	var me struct {
		ID uuid.UUID `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &me); err != nil || me.ID == uuid.Nil {
		t.Fatalf("me: %s", rec.Body.String())
	}
	f.ids.Add(identity.Identity{ID: me.ID, SchemaID: "customer", Email: "cust@example.com", CreatedAt: now})
	return me.ID
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %d: %s", rec.Code, rec.Body.String())
	}
	return v
}

func TestPII_HTTP_EndToEndWithoutLeaks(t *testing.T) {
	f := newFixture(t)
	id := f.customerID(t)
	adminCred := cred{cookie: "super", origin: origin}

	// Never set → every field null.
	rec := f.do(t, jsonRoute("GET", "/v1/me/personal-info", ""), cred{bearer: "cust"})
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"phone_number":null`) || !strings.Contains(rec.Body.String(), `"address":null`) ||
		!strings.Contains(rec.Body.String(), `"updated_at":null`) {
		t.Fatalf("empty GET: %d %s", rec.Code, rec.Body.String())
	}

	// Invalid input: 422 names fields and codes only, never values.
	bad := `{"phone_number":"0909999999","address":{"line1":"Hacker Street","city":"","country":"VN"},"national_id":{"type":"cccd","number":"0909999999-x"}}`
	rec = f.do(t, jsonRoute("PUT", "/v1/me/personal-info", bad), cred{bearer: "cust"})
	if rec.Code != 422 || problemCode(t, rec) != CodeValidationFailed {
		t.Fatalf("invalid PUT: %d %s", rec.Code, rec.Body.String())
	}
	for _, want := range []string{`{"code":"invalid_format","field":"phone_number"}`, `{"code":"required","field":"address.city"}`, `"field":"national_id.number"`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("422 missing %s: %s", want, rec.Body.String())
		}
	}
	assertNoPIIIn(t, "422 body", rec.Body.String())
	rec = f.do(t, jsonRoute("PUT", "/v1/me/personal-info", `{"date_of_birth":"1990-02-30"}`), cred{bearer: "cust"})
	if rec.Code != 422 {
		t.Fatalf("impossible date: %d", rec.Code)
	}

	rec = f.do(t, jsonRoute("PUT", "/v1/me/personal-info", validPII), cred{bearer: "cust"})
	if rec.Code != 200 {
		t.Fatalf("PUT: %d %s", rec.Code, rec.Body.String())
	}
	put := decode[map[string]any](t, rec)
	if put["phone_number"] != "+84901234567" || put["date_of_birth"] != "1990-05-17" || put["updated_at"] == nil {
		t.Fatalf("PUT body %v", put)
	}
	rec = f.do(t, jsonRoute("GET", "/v1/me/personal-info", ""), cred{bearer: "cust"})
	got := decode[struct {
		Phone   string `json:"phone_number"`
		Address struct {
			Line1, City, Country string
			Line2                *string `json:"line2"`
		} `json:"address"`
		NationalID struct{ Type, Number string } `json:"national_id"`
	}](t, rec)
	if got.Phone != "+84901234567" || got.Address.Line1 != "12 Nguyen Trai" || got.Address.Line2 != nil ||
		got.NationalID.Number != "079123456123" || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("GET: %s", rec.Body.String())
	}

	// Admin: masked view (support allowed).
	rec = f.do(t, jsonRoute("GET", "/admin/v1/customers/"+id.String()+"/personal-info", ""), cred{cookie: "support"})
	if rec.Code != 200 {
		t.Fatalf("masked: %d %s", rec.Code, rec.Body.String())
	}
	masked := rec.Body.String()
	for _, want := range []string{`"phone_number":"+84*******567"`, `"date_of_birth":"1990-**-**"`, `"number":"******123"`,
		`"has_phone_number":true`, `"has_national_id":true`, `"city":"Ho Chi Minh"`} {
		if !strings.Contains(masked, want) {
			t.Fatalf("masked missing %s: %s", want, masked)
		}
	}
	if strings.Contains(masked, "Nguyen Trai") || strings.Contains(masked, "079123456123") || strings.Contains(masked, "700000") {
		t.Fatalf("masked view leaks: %s", masked)
	}

	// Lookup (POST body only) returns id, state and masked info, no email.
	rec = f.do(t, jsonRoute("POST", "/admin/v1/customers/lookup", `{"phone_number":"+84901234567"}`), cred{cookie: "support", origin: origin})
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), id.String()) || !strings.Contains(rec.Body.String(), `"state":"active"`) ||
		strings.Contains(rec.Body.String(), "email") || strings.Contains(rec.Body.String(), "+84901234567") {
		t.Fatalf("lookup: %d %s", rec.Code, rec.Body.String())
	}
	rec = f.do(t, jsonRoute("POST", "/admin/v1/customers/lookup", `{"phone_number":"0909999999"}`), cred{cookie: "support", origin: origin})
	if rec.Code != 422 {
		t.Fatalf("lookup invalid: %d", rec.Code)
	}
	assertNoPIIIn(t, "lookup 422", rec.Body.String())

	// Reveal: reason code enum, support denied, admin allowed and audited.
	revealPath := "/admin/v1/customers/" + id.String() + "/personal-info/reveal"
	rec = f.do(t, jsonRoute("POST", revealPath, `{"reason_code":"legal_request"}`), cred{cookie: "support", origin: origin})
	if rec.Code != 403 {
		t.Fatalf("support reveal: %d", rec.Code)
	}
	rec = f.do(t, jsonRoute("POST", revealPath, `{"reason_code":"I am curious about 0909999999"}`), adminCred)
	if rec.Code != 422 || !strings.Contains(rec.Body.String(), `"field":"reason_code"`) {
		t.Fatalf("free-text reason: %d %s", rec.Code, rec.Body.String())
	}
	rec = f.do(t, jsonRoute("POST", revealPath, `{"reason_code":"customer_support_request","ticket_ref":"SUP-42","fields":["phone_number","national_id"]}`), adminCred)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"phone_number":"+84901234567"`) ||
		!strings.Contains(rec.Body.String(), `"address":null`) || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("reveal: %d %s", rec.Code, rec.Body.String())
	}
	ev := f.store.Events[len(f.store.Events)-1]
	if ev.Action != "customer.pii.revealed" || ev.TargetID != id.String() || ev.Details["ticket_ref"] != "SUP-42" {
		t.Fatalf("reveal audit %+v", ev)
	}

	// Admin /me lists the new permission.
	rec = f.do(t, jsonRoute("GET", "/admin/v1/me", ""), cred{cookie: "super"})
	if !strings.Contains(rec.Body.String(), `"reveal_customer_pii"`) {
		t.Fatalf("admin me: %s", rec.Body.String())
	}
	rec = f.do(t, jsonRoute("GET", "/admin/v1/me", ""), cred{cookie: "support"})
	if strings.Contains(rec.Body.String(), `"reveal_customer_pii"`) {
		t.Fatalf("support must not get reveal: %s", rec.Body.String())
	}

	// Failure paths: audit down → 500 (nothing revealed); key manager down → 503.
	f.store.AuditErr = errors.New("audit insert failed")
	rec = f.do(t, jsonRoute("POST", revealPath, `{"reason_code":"legal_request"}`), adminCred)
	if rec.Code != 500 || problemCode(t, rec) != CodeInternal {
		t.Fatalf("reveal w/o audit: %d %s", rec.Code, rec.Body.String())
	}
	assertNoPIIIn(t, "500 body", rec.Body.String())
	f.store.AuditErr = nil
	f.kms.SetFailure(app.ErrDependencyUnavailable)
	f.cache.Purge()
	for _, rt := range []route{
		jsonRoute("GET", "/v1/me/personal-info", ""),
		jsonRoute("PUT", "/v1/me/personal-info", validPII),
	} {
		rec = f.do(t, rt, cred{bearer: "cust"})
		if rec.Code != 503 || problemCode(t, rec) != CodeDependencyUnavailable {
			t.Fatalf("%s while key manager down: %d %s", rt.method, rec.Code, rec.Body.String())
		}
	}
	rec = f.do(t, jsonRoute("GET", "/v1/me", ""), cred{bearer: "cust"})
	if rec.Code != 200 {
		t.Fatalf("non-PII endpoints unaffected: %d", rec.Code)
	}
	f.kms.SetFailure(nil)

	// Tampered row → 500, logged with ids only.
	r := f.store.PII[id]
	r.Phone[len(r.Phone)-1] ^= 1
	f.store.PII[id] = r
	rec = f.do(t, jsonRoute("GET", "/v1/me/personal-info", ""), cred{bearer: "cust"})
	if rec.Code != 500 {
		t.Fatalf("tampered: %d", rec.Code)
	}

	// Erase → 204, then everything null; a second erase is still 204.
	for range 2 {
		rec = f.do(t, jsonRoute("DELETE", "/v1/me/personal-info", ""), cred{bearer: "cust-unverified"})
		if rec.Code != 204 {
			t.Fatalf("erase (unverified allowed): %d %s", rec.Code, rec.Body.String())
		}
	}
	rec = f.do(t, jsonRoute("DELETE", "/v1/me/personal-info", ""), cred{bearer: "cust"})
	if rec.Code != 204 || len(f.store.Keys) != 0 {
		t.Fatalf("erase: %d keys=%d", rec.Code, len(f.store.Keys))
	}
	rec = f.do(t, jsonRoute("GET", "/v1/me/personal-info", ""), cred{bearer: "cust"})
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"phone_number":null`) {
		t.Fatalf("after erase: %s", rec.Body.String())
	}

	// PII-NFR-04 / §9 A16: no test value in any log line, success or error path.
	logs := f.logs.String()
	if !strings.Contains(logs, "pii_decrypt_failed") || !strings.Contains(logs, "dependency unavailable") || !strings.Contains(logs, "request failed") {
		t.Fatalf("expected error-path logs: %s", logs)
	}
	assertNoPIIIn(t, "logs", logs)
	for _, e := range f.store.Events {
		b, _ := json.Marshal(e.Details)
		assertNoPIIIn(t, "audit "+string(e.Action), string(b))
	}
}

func TestPII_HTTP_RateLimited(t *testing.T) {
	f := newFixture(t)
	for i := range 31 {
		rec := f.do(t, jsonRoute("POST", "/admin/v1/customers/lookup", `{"phone_number":"+84901234567"}`), cred{cookie: "super", origin: origin})
		if i < 30 && rec.Code != 200 {
			t.Fatalf("lookup %d: %d", i, rec.Code)
		}
		if i == 30 && (rec.Code != 429 || problemCode(t, rec) != CodeRateLimited) {
			t.Fatalf("31st lookup: %d %s", rec.Code, rec.Body.String())
		}
	}
}

// Review #5: unknown properties and malformed values are field errors (422)
// on the offending field, never echoing values or property names.
func TestReview5_StrictPIIBodies(t *testing.T) {
	f := newFixture(t)
	id := f.customerID(t)
	cases := []struct {
		rt        route
		c         cred
		wantField string
		wantCode  string
	}{
		{jsonRoute("PUT", "/v1/me/personal-info", `{"date_of_birth":"17/05/1990"}`), cred{bearer: "cust"}, "date_of_birth", "invalid_format"},
		{jsonRoute("PUT", "/v1/me/personal-info", `{"date_of_birth":"1990-02-30"}`), cred{bearer: "cust"}, "date_of_birth", "invalid_format"},
		{jsonRoute("PUT", "/v1/me/personal-info", `{"date_of_birth":19900517}`), cred{bearer: "cust"}, "date_of_birth", "invalid_format"},
		{jsonRoute("PUT", "/v1/me/personal-info", `{"phone_number":901234567}`), cred{bearer: "cust"}, "phone_number", "invalid_format"},
		{jsonRoute("PUT", "/v1/me/personal-info", `{"Hacker Street":"0909999999"}`), cred{bearer: "cust"}, "body", CodeUnknownField},
		{jsonRoute("PUT", "/v1/me/personal-info", `{"address":{"line1":"x","city":"y","country":"VN","geo":"0909999999"}}`), cred{bearer: "cust"}, "address", CodeUnknownField},
		{jsonRoute("PUT", "/v1/me/personal-info", `{"address":{"line1":null,"city":"y","country":"VN"}}`), cred{bearer: "cust"}, "address.line1", "invalid_format"},
		{jsonRoute("PUT", "/v1/me/personal-info", `{"national_id":"0909999999"}`), cred{bearer: "cust"}, "national_id", "invalid_format"},
		{jsonRoute("POST", "/admin/v1/customers/"+id.String()+"/personal-info/reveal", `{"reason_code":"legal_request","reason":"Hacker Street"}`),
			cred{cookie: "super", origin: origin}, "body", CodeUnknownField},
		{jsonRoute("POST", "/admin/v1/customers/"+id.String()+"/personal-info/reveal", `{"fields":"phone_number"}`),
			cred{cookie: "super", origin: origin}, "fields", "invalid_format"},
		{jsonRoute("POST", "/admin/v1/customers/"+id.String()+"/personal-info/reveal", `{}`),
			cred{cookie: "super", origin: origin}, "reason_code", "required"},
		{jsonRoute("POST", "/admin/v1/customers/lookup", `{"phone_number":"+84901234567","email":"x@example.com"}`),
			cred{cookie: "super", origin: origin}, "body", CodeUnknownField},
	}
	for _, tc := range cases {
		rec := f.do(t, tc.rt, tc.c)
		want := `{"code":"` + tc.wantCode + `","field":"` + tc.wantField + `"}`
		if rec.Code != 422 || !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("%s %s: %d %s (want %s)", tc.rt.method, tc.rt.body(f), rec.Code, rec.Body.String(), want)
		}
		assertNoPIIIn(t, "422 body", rec.Body.String())
		if strings.Contains(rec.Body.String(), "geo") || strings.Contains(rec.Body.String(), `"reason"`) {
			t.Fatalf("property names must not be echoed: %s", rec.Body.String())
		}
	}
	// The PUT accepts what GET returns (readOnly updated_at is ignored).
	rec := f.do(t, jsonRoute("PUT", "/v1/me/personal-info", `{"phone_number":"+84901234567","updated_at":"2026-01-01T00:00:00Z"}`), cred{bearer: "cust"})
	if rec.Code != 200 {
		t.Fatalf("round-trip body: %d %s", rec.Code, rec.Body.String())
	}
	if len(f.store.Events) != 1 {
		t.Fatalf("rejected bodies must not reach the service: %d events", len(f.store.Events))
	}
	assertNoPIIIn(t, "logs", f.logs.String())
}

// Review #7: lookup responses always carry truncated.
func TestReview7_LookupTruncatedField(t *testing.T) {
	f := newFixture(t)
	rec := f.do(t, jsonRoute("POST", "/admin/v1/customers/lookup", `{"phone_number":"+84901234567"}`), cred{cookie: "super", origin: origin})
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"truncated":false`) || !strings.Contains(rec.Body.String(), `"items":[]`) {
		t.Fatalf("lookup: %d %s", rec.Code, rec.Body.String())
	}
}
