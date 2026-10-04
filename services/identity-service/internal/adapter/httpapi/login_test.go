package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/adapter/localkms"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/login"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/testutil"
)

func authReq(t *testing.T, h http.Handler, path, body string, hdr http.Header) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest("POST", path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("User-Agent", "test-app/1.0")
	for k, v := range hdr {
		r.Header[k] = v
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

const (
	regBody   = `{"login":{"type":"email","value":"Alice@Example.com"},"password":"correct horse battery staple"}`
	loginBody = `{"login":{"type":"email","value":"alice@example.com"},"password":"correct horse battery staple"}`
)

func TestPLXFR01_HTTPRegisterAndLogin(t *testing.T) {
	f := newFixture(t)
	w := authReq(t, f.h, "/v1/auth/registration", regBody, nil)
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("%d %s %v", w.Code, w.Body, w.Header())
	}
	var out struct {
		SessionToken       string          `json:"session_token"`
		Session            json.RawMessage `json:"session"`
		VerificationFlowID *string         `json:"verification_flow_id"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	handle := f.flows.CallsOf("register")[0].Identifier
	if out.SessionToken != "token-"+handle || !strings.Contains(string(out.Session), `"id":"sess"`) {
		t.Fatalf("registration response %s", w.Body)
	}
	if c := f.flows.CallsOf("register")[0].Client; c.UserAgent != "test-app/1.0" || !c.IP.IsValid() {
		t.Fatalf("client not forwarded: %+v", c)
	}
	// A credential is neither needed nor used.
	w = authReq(t, f.h, "/v1/auth/login", loginBody, http.Header{"Authorization": {"Bearer cust"}})
	if w.Code != 200 || !strings.Contains(w.Body.String(), "token-"+handle) {
		t.Fatalf("login: %d %s", w.Code, w.Body)
	}
	for _, leak := range []string{"alice", "correct horse"} {
		if strings.Contains(f.logs.String(), leak) {
			t.Fatalf("logs leak %q", leak)
		}
	}
}

func TestPLXFR06_HTTPRejectionCarriesMessageIDsOnly(t *testing.T) {
	f := newFixture(t)
	w := authReq(t, f.h, "/v1/auth/login", `{"login":{"type":"email","value":"nobody@example.com"},"password":"secret-password"}`, nil)
	if w.Code != 400 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	var p struct {
		Code   string `json:"code"`
		Errors []struct{ Field, Code string }
	}
	_ = json.Unmarshal(w.Body.Bytes(), &p)
	if p.Code != "auth_flow_rejected" || len(p.Errors) != 1 || p.Errors[0].Field != "form" || p.Errors[0].Code != "4000006" {
		t.Fatalf("problem %s", w.Body)
	}
	// The decoy handle Kratos was called with never reaches the client.
	decoy := f.flows.CallsOf("login")[0].Identifier
	if strings.Contains(w.Body.String(), decoy) || strings.Contains(w.Body.String(), "login.invalid") {
		t.Fatal("response leaks a handle")
	}
}

func TestPLXFR03_HTTPRecovery(t *testing.T) {
	f := newFixture(t)
	w := authReq(t, f.h, "/v1/auth/recovery", `{"login":{"type":"phone","value":"0912 345 678"}}`, nil)
	var out struct {
		RecoveryID string `json:"recovery_id"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if w.Code != 200 || out.RecoveryID == "" || strings.Contains(w.Body.String(), testutil.RecoveryFlowID) {
		t.Fatalf("recovery must return a sealed reference, never the flow id: %d %s", w.Code, w.Body)
	}
	if got := f.flows.CallsOf("recovery"); len(got) != 1 || !strings.HasSuffix(got[0].Identifier, "@login.invalid") {
		t.Fatalf("recovery call %+v", got)
	}
	// (3 requests per minute in the fixture; the wrong-code path is covered
	// by the app tests.)
	body, _ := json.Marshal(map[string]string{"recovery_id": out.RecoveryID, "code": testutil.RecoveryCode})
	w = authReq(t, f.h, "/v1/auth/recovery/code", string(body), nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"settings_flow_id":"settings-flow"`) || !strings.Contains(w.Body.String(), `"session_token":"token-recovery"`) {
		t.Fatalf("grant: %d %s", w.Code, w.Body)
	}
	f.flows.Err = app.ErrAuthFlowExpired
	if w := authReq(t, f.h, "/v1/auth/recovery/code", string(body), nil); w.Code != 410 || !strings.Contains(w.Body.String(), "auth_flow_expired") {
		t.Fatalf("expired: %d %s", w.Code, w.Body)
	}
}

func TestPLXNFR01_HTTPAuthStrictAndValueFree(t *testing.T) {
	f := newFixture(t)
	for _, tc := range []struct {
		path, body string
		code       int
	}{
		{"/v1/auth/login", `{"login":{"type":"email","value":"a@example.com"},"password":"x","extra":1}`, 422},
		{"/v1/auth/login", `{"login":{"type":"email","value":"secret-not-an-email"},"password":"x"}`, 422},
		{"/v1/auth/login", `{"login":{"type":"email","value":"a@example.com"}}`, 422},
		{"/v1/auth/registration", `{"login":{"type":"phone","value":"+12025550100"},"password":"x"}`, 422},
		{"/v1/auth/recovery", `{"login":{"type":"email","value":"a@example.com"},"password":"x"}`, 422},
	} {
		w := authReq(t, f.h, tc.path, tc.body, nil)
		if w.Code != tc.code || strings.Contains(w.Body.String(), "secret") || strings.Contains(w.Body.String(), "2025550100") {
			t.Errorf("%s %s: %d %s", tc.path, tc.body, w.Code, w.Body)
		}
	}
	if len(f.flows.Calls) != 0 {
		t.Fatal("invalid requests must not reach Kratos")
	}
	// Per-IP limit (3/min in the fixture) → 429.
	var last *httptest.ResponseRecorder
	for range 4 {
		last = authReq(t, f.h, "/v1/auth/login", loginBody, nil)
	}
	if last.Code != 429 || last.Header().Get("Retry-After") != "60" {
		t.Fatalf("limit: %d %q", last.Code, last.Header().Get("Retry-After"))
	}
	// The resolve endpoint is gone (PLX-FR-05); only declared routes are public.
	for _, m := range []string{"POST", "GET"} {
		r := httptest.NewRequest(m, "/v1/auth/identifiers", strings.NewReader(`{}`))
		w := httptest.NewRecorder()
		f.h.ServeHTTP(w, r)
		if w.Code != 404 {
			t.Fatalf("%s identifiers: %d", m, w.Code)
		}
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
	p := registerHandle(t, logins, "a@example.com")
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
	p := registerHandle(t, logins, "a@example.com")
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

// registerHandle registers an address through the proxy (fake Kratos) and
// returns its handle.
func registerHandle(t *testing.T, logins *app.LoginIdentifierService, email string) string {
	t.Helper()
	flows := testutil.NewAuthFlows()
	auth := &app.CustomerAuthService{Logins: logins, Flows: flows}
	in := app.Credentials{Client: app.FlowClient{IP: netip.MustParseAddr("192.0.2.1")}, Type: "email", Value: email, Password: "correct horse battery staple"}
	if _, err := auth.Register(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	return flows.CallsOf("register")[0].Identifier
}
