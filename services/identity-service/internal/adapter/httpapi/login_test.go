package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"regexp"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/adapter/localkms"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/login"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/testutil"
)

var pseudonymRe = regexp.MustCompile(`^[a-z2-7]{52}@login\.invalid$`)

func resolveReq(t *testing.T, h http.Handler, body string, hdr http.Header) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest("POST", "/v1/auth/identifiers", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	for k, v := range hdr {
		r.Header[k] = v
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestPLIFR01_HTTPResolveIsPublic(t *testing.T) {
	f := newFixture(t)
	w := resolveReq(t, f.h, `{"type":"email","value":"Alice@Example.com","purpose":"sign_in"}`, nil)
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("%d %s %v", w.Code, w.Body, w.Header())
	}
	var out struct{ Identifier string }
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if !pseudonymRe.MatchString(out.Identifier) {
		t.Fatalf("identifier %q", out.Identifier)
	}
	// A credential is neither needed nor used.
	w2 := resolveReq(t, f.h, `{"type":"email","value":"alice@example.com","purpose":"recovery"}`, http.Header{"Authorization": {"Bearer cust"}})
	if !strings.Contains(w2.Body.String(), out.Identifier) {
		t.Fatalf("same address must resolve identically: %s", w2.Body)
	}
	if strings.Contains(f.logs.String(), "alice") {
		t.Fatal("logs leak the address")
	}
}

func TestPLINFR04_HTTPResolveStrictAndValueFree(t *testing.T) {
	f := newFixture(t)
	for _, tc := range []struct {
		body string
		code int
	}{
		{`{"type":"email","value":"a@example.com","purpose":"sign_in","password":"x"}`, 422},
		{`{"type":"email","value":"secret-not-an-email","purpose":"sign_in"}`, 422},
		{`{"type":"phone","value":"+12025550100","purpose":"sign_in"}`, 422},
	} {
		w := resolveReq(t, f.h, tc.body, nil)
		if w.Code != tc.code || strings.Contains(w.Body.String(), "secret") || strings.Contains(w.Body.String(), "2025550100") {
			t.Errorf("%s: %d %s", tc.body, w.Code, w.Body)
		}
	}
	// Per-IP limit (3/min in the fixture) → 429.
	var last int
	for range 4 {
		last = resolveReq(t, f.h, `{"type":"email","value":"a@example.com","purpose":"sign_in"}`, nil).Code
	}
	if last != 429 {
		t.Fatalf("limit: %d", last)
	}
	if ra := resolveReq(t, f.h, `{"type":"email","value":"a@example.com","purpose":"sign_in"}`, nil).Header().Get("Retry-After"); ra != "60" {
		t.Fatalf("Retry-After %q", ra)
	}
	// Only the declared route is public.
	r := httptest.NewRequest("GET", "/v1/auth/identifiers", nil)
	w := httptest.NewRecorder()
	f.h.ServeHTTP(w, r)
	if w.Code != 404 {
		t.Fatalf("GET: %d", w.Code)
	}
}

const courierKey = "test-courier-key-0123456789"

func loginWebhook(t *testing.T) (*testutil.Identities, *testutil.Mailer, *app.LoginIdentifierService, *httptest.Server) {
	t.Helper()
	clock := &testutil.FixedClock{T: now}
	store := testutil.NewStore(clock)
	ids := testutil.NewIdentities(clock)
	mail := &testutil.Mailer{}
	logins := &app.LoginIdentifierService{Keys: localkms.NewRandom(), Logins: store.Repos().Logins, Tx: store, Identities: ids,
		Phone: login.PhonePolicy{DefaultCountry: "84", AllowedCountries: []string{"84"}}, Phase: app.PhaseTransition, Clock: clock}
	courier := &app.CourierDispatcher{Logins: logins, Identities: ids, Mailer: mail, Dedupe: store.Repos().Dispatches,
		DedupeKey: []byte("k"), Phase: app.PhaseTransition, Clock: clock}
	h := NewWebhookHandler(WebhookDeps{
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Metrics: NewMetrics(), APIKey: hookKey,
		Provisioning: &app.ProvisioningService{Profiles: store.Repos().Profiles},
		Logins:       logins, Courier: courier, CourierAPIKey: courierKey,
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return ids, mail, logins, srv
}

func TestPLIFR04_PreRegistrationWebhook(t *testing.T) {
	_, _, logins, srv := loginWebhook(t)
	const path = "/internal/hooks/kratos/pre-registration"
	if code, _ := post(t, srv, path, courierKey, `{"schema_id":"customer"}`); code != 401 {
		t.Fatal("courier key must not open the registration hook")
	}
	code, body := post(t, srv, path, hookKey, `{"schema_id":"customer","flow_type":"api","email":"a@example.com","login_id":null}`)
	if code != 400 || !strings.Contains(body, `"instance_ptr":"#/traits/login_id"`) || !strings.Contains(body, "4049001") {
		t.Fatalf("legacy: %d %s", code, body)
	}
	if code, body := post(t, srv, path, hookKey, `{"schema_id":"customer","flow_type":"api","login_id":"x@login.invalid","email":null}`); code != 400 || !strings.Contains(body, "4049002") {
		t.Fatalf("unresolved: %d %s", code, body)
	}
	p, err := logins.Resolve(context.Background(), app.ResolveRequest{ClientIP: mustAddr("192.0.2.1"), Type: "email", Value: "a@example.com", Purpose: "registration"})
	if err != nil {
		t.Fatal(err)
	}
	if code, body := post(t, srv, path, hookKey, `{"schema_id":"customer","flow_type":"api","login_id":"`+p+`","email":null}`); code != 200 || body != "{}" {
		t.Fatalf("ok: %d %s", code, body)
	}
	if code, _ := post(t, srv, path, hookKey, `{"schema_id":"admin","flow_type":"browser","login_id":null,"email":"x@example.com"}`); code != 200 {
		t.Fatal("non-customer schemas pass through")
	}
}

func TestPLIFR05_CourierWebhook(t *testing.T) {
	ids, mail, logins, srv := loginWebhook(t)
	const path = "/internal/hooks/kratos/courier"
	p, _ := logins.Resolve(context.Background(), app.ResolveRequest{ClientIP: mustAddr("192.0.2.1"), Type: "email", Value: "a@example.com", Purpose: "registration"})
	c := ids.AddCustomer(p, false)
	body := `{"recipient":"` + p + `","template_type":"verification_code_valid","identity_id":"` + c.ID.String() + `","code":"123456","expires_in_minutes":60}`
	if code, _ := post(t, srv, path, hookKey, body); code != 401 {
		t.Fatal("webhook key must not open the courier")
	}
	for range 2 { // Kratos retry → one delivery
		if code, b := post(t, srv, path, courierKey, body); code != 204 {
			t.Fatalf("courier: %d %s", code, b)
		}
	}
	if len(mail.LoginSent) != 1 || mail.LoginSent[0].To != "a@example.com" {
		t.Fatalf("mail %+v", mail.LoginSent)
	}
	// Unknown pseudonym / garbage: acknowledged (no retry storm), nothing sent.
	for _, b := range []string{
		`{"recipient":"zz@login.invalid","template_type":"verification_code_valid","identity_id":"` + uuid.NewString() + `","code":"123456","expires_in_minutes":null}`,
		`{"subject":"x"}`,
	} {
		if code, _ := post(t, srv, path, courierKey, b); code != 204 {
			t.Fatalf("permanent failure must be 204: %s", b)
		}
	}
	mail.Err = io.ErrUnexpectedEOF
	body2 := strings.Replace(body, "123456", "654321", 1)
	if code, _ := post(t, srv, path, courierKey, body2); code != 503 {
		t.Fatal("transient failure must be 503 so Kratos retries")
	}
	if len(mail.LoginSent) != 1 {
		t.Fatal("unexpected sends")
	}
}

func mustAddr(s string) netip.Addr { return netip.MustParseAddr(s) }
