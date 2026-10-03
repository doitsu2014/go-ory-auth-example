package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/audit"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/identity"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/machine"
)

const jwtShaped = "eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJ4In0.c2lnbmF0dXJl"

type mreq struct {
	method, path         string
	bearer, cookie, xTok string
	origin, extraAuthz   string
	preflight            bool
	remote               string
}

func (f *fixture) m2m(t *testing.T, q mreq) *httptest.ResponseRecorder {
	t.Helper()
	if q.method == "" {
		q.method = "GET"
	}
	path := strings.NewReplacer("{customer}", f.customer.String(), "{target}", f.target.String()).Replace(q.path)
	req := httptest.NewRequest(q.method, path, nil)
	if q.remote != "" {
		req.RemoteAddr = q.remote
	}
	if q.bearer != "" {
		req.Header.Set("Authorization", "Bearer "+q.bearer)
	}
	if q.extraAuthz != "" {
		req.Header.Add("Authorization", q.extraAuthz)
	}
	if q.cookie != "" {
		req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: q.cookie})
	}
	if q.xTok != "" {
		req.Header.Set("X-Session-Token", q.xTok)
	}
	if q.origin != "" {
		req.Header.Set("Origin", q.origin)
	}
	if q.preflight {
		req.Header.Set("Access-Control-Request-Method", "GET")
	}
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, req)
	return rec
}

func TestM2M_AuthMatrix(t *testing.T) {
	const cust = "/m2m/v1/customers/{customer}"
	const auditPath = "/m2m/v1/audit-events"
	cases := []struct {
		name      string
		q         mreq
		status    int
		code      string
		challenge string
	}{
		{"no credential", mreq{path: cust}, 401, CodeUnauthenticated, `Bearer realm="identity-service"`},
		{"kratos cookie", mreq{path: cust, cookie: "super"}, 401, CodeInvalidToken, `error="invalid_token"`},
		{"cookie plus valid jwt", mreq{path: cust, cookie: "super", bearer: "m2m-cust"}, 401, CodeInvalidToken, `error="invalid_token"`},
		{"x-session-token", mreq{path: cust, xTok: "cust"}, 401, CodeInvalidToken, `error="invalid_token"`},
		{"kratos session token as bearer", mreq{path: cust, bearer: "cust"}, 401, CodeInvalidToken, `error="invalid_token"`},
		{"basic auth", mreq{path: cust, extraAuthz: "Basic Y2xpZW50OnNlY3JldA=="}, 400, CodeInvalidRequest, `error="invalid_request"`},
		{"two authorization headers", mreq{path: cust, bearer: "m2m-cust", extraAuthz: "Bearer m2m-cust"}, 400, CodeInvalidRequest, `error="invalid_request"`},
		{"extra spaces", mreq{path: cust, extraAuthz: "Bearer  m2m-cust"}, 400, CodeInvalidRequest, `error="invalid_request"`},
		{"token with inner space", mreq{path: cust, extraAuthz: "Bearer m2m cust"}, 400, CodeInvalidRequest, `error="invalid_request"`},
		{"scheme only", mreq{path: cust, extraAuthz: "Bearer"}, 400, CodeInvalidRequest, `error="invalid_request"`},
		{"hydra down", mreq{path: cust, bearer: "m2m-down"}, 503, CodeDependencyUnavailable, ""},
		{"wrong scope customers", mreq{path: cust, bearer: "m2m-audit"}, 403, CodeInsufficientScope, `error="insufficient_scope", scope="customers:read"`},
		{"wrong scope audit", mreq{path: auditPath, bearer: "m2m-cust"}, 403, CodeInsufficientScope, `scope="audit:read"`},
		{"ok customer", mreq{path: cust, bearer: "m2m-cust"}, 200, "", ""},
		{"ok audit", mreq{path: auditPath, bearer: "m2m-audit"}, 200, "", ""},
		{"admin id is 404", mreq{path: "/m2m/v1/customers/{target}", bearer: "m2m-cust"}, 404, CodeNotFound, ""},
		{"bad uuid is 404", mreq{path: "/m2m/v1/customers/not-a-uuid", bearer: "m2m-cust"}, 404, CodeNotFound, ""},
		{"unknown route after auth", mreq{path: "/m2m/v1/nope", bearer: "m2m-cust"}, 404, CodeNotFound, ""},
		{"unknown route no auth", mreq{path: "/m2m/v1/nope"}, 401, CodeUnauthenticated, ""},
		{"wrong method on existing path", mreq{method: "DELETE", path: cust, bearer: "m2m-cust"}, 405, CodeMethodNotAllowed, ""},
		{"wrong method on audit path", mreq{method: "POST", path: auditPath, bearer: "m2m-audit"}, 405, CodeMethodNotAllowed, ""},
		{"wrong method unauthenticated", mreq{method: "DELETE", path: cust}, 401, CodeUnauthenticated, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			rec := f.m2m(t, tc.q)
			if rec.Code != tc.status {
				t.Fatalf("status %d want %d: %s", rec.Code, tc.status, rec.Body.String())
			}
			if got := problemCode(t, rec); got != tc.code {
				t.Fatalf("code %q want %q", got, tc.code)
			}
			if got := rec.Header().Get("WWW-Authenticate"); !strings.Contains(got, tc.challenge) || (tc.challenge != "" && !strings.HasPrefix(got, "Bearer ")) {
				t.Fatalf("WWW-Authenticate %q want %q", got, tc.challenge)
			}
			if rec.Header().Get("Cache-Control") != "no-store" || rec.Header().Get("Set-Cookie") != "" {
				t.Fatal("machine responses: no-store and never a cookie")
			}
			if f.kratos.Calls != 0 {
				t.Fatal("the machine plane must never call Kratos")
			}
		})
	}
}

func TestM2M_CustomerHasNoPII(t *testing.T) {
	f := newFixture(t)
	rec := f.m2m(t, mreq{path: "/m2m/v1/customers/{customer}", bearer: "m2m-cust"})
	if rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"id", "state", "email_verified", "created_at"} {
		if _, ok := got[k]; !ok {
			t.Fatalf("missing %s: %v", k, got)
		}
	}
	if len(got) != 4 || strings.Contains(rec.Body.String(), "c@example.com") {
		t.Fatalf("PII or extra fields: %s", rec.Body.String())
	}
}

func TestM2M_NoCORS(t *testing.T) {
	f := newFixture(t)
	for _, q := range []mreq{
		{path: "/m2m/v1/audit-events", bearer: "m2m-audit", origin: origin},
		{method: "OPTIONS", path: "/m2m/v1/audit-events", origin: origin, preflight: true},
	} {
		rec := f.m2m(t, q)
		for k := range rec.Header() {
			if strings.HasPrefix(k, "Access-Control-") {
				t.Fatalf("%s %s: CORS header %s on the machine plane", q.method, q.path, k)
			}
		}
		if q.preflight && rec.Code != 401 {
			t.Fatalf("preflight must be unauthenticated, got %d", rec.Code)
		}
	}
}

func TestM2M_PlaneBindingJWTOnOtherPlanes(t *testing.T) {
	f := newFixture(t)
	f.kratos.Sessions[jwtShaped] = principal(identity.KindCustomer, identity.AAL1, uuid.New(), true)
	for _, p := range []string{"/v1/me", "/v1/me/personal-info"} {
		rec := f.do(t, route{method: "GET", path: p}, cred{bearer: jwtShaped})
		if rec.Code != 401 || problemCode(t, rec) != CodeUnauthenticated {
			t.Fatalf("%s: JWT bearer must be 401: %d", p, rec.Code)
		}
	}
	if f.kratos.Calls != 0 {
		t.Fatalf("a JWT-shaped bearer reached Kratos (%d calls)", f.kratos.Calls)
	}
	for _, p := range []string{"/admin/v1/me", "/admin/v1/service-clients"} {
		rec := f.do(t, route{method: "GET", path: p}, cred{bearer: jwtShaped})
		if rec.Code != 401 {
			t.Fatalf("%s: JWT bearer on admin plane: %d", p, rec.Code)
		}
		rec = f.do(t, route{method: "GET", path: p}, cred{bearer: "m2m-cust", cookie: "super"})
		if rec.Code != 401 {
			t.Fatalf("%s: machine token next to cookie: %d", p, rec.Code)
		}
	}
	// A Kratos session token (ory_st_…) is not JWT-shaped.
	if looksLikeJWT("ory_st_abc.def.ghi") || !looksLikeJWT(jwtShaped) || looksLikeJWT("eyJabc.def") || looksLikeJWT("eyJ.a.b!") {
		t.Fatal("looksLikeJWT")
	}
}

func TestM2M_FailureLimiterPerIP(t *testing.T) {
	f := newFixture(t)
	q := mreq{path: "/m2m/v1/audit-events", bearer: "forged", remote: "198.51.100.7:4000"}
	for i := range MachineFailuresPerMin {
		if rec := f.m2m(t, q); rec.Code != 401 {
			t.Fatalf("attempt %d: %d", i, rec.Code)
		}
	}
	// Over budget: further rejections from the IP answer 429 …
	if rec := f.m2m(t, q); rec.Code != 429 || problemCode(t, rec) != CodeRateLimited {
		t.Fatalf("61st failure: %d", rec.Code)
	}
	if rec := f.m2m(t, mreq{path: "/m2m/v1/audit-events", remote: "198.51.100.7:4000", extraAuthz: "Basic x"}); rec.Code != 429 {
		t.Fatalf("malformed header over budget: %d", rec.Code)
	}
	// … but a token that verifies is never refused by the failure limiter
	// (other callers may share the address behind a proxy).
	for range 3 {
		if rec := f.m2m(t, mreq{path: "/m2m/v1/audit-events", bearer: "m2m-audit", remote: "198.51.100.7:4001"}); rec.Code != 200 {
			t.Fatalf("valid token from a throttled ip: %d", rec.Code)
		}
	}
	if rec := f.m2m(t, mreq{path: "/m2m/v1/audit-events", bearer: "forged", remote: "198.51.100.8:4000"}); rec.Code != 401 {
		t.Fatalf("other ip: %d", rec.Code)
	}
	// Dependency failures are not charged to the caller.
	g := newFixture(t)
	for range MachineFailuresPerMin + 5 {
		if rec := g.m2m(t, mreq{path: "/m2m/v1/audit-events", bearer: "m2m-down", remote: "198.51.100.9:1"}); rec.Code != 503 {
			t.Fatalf("hydra down: %d", rec.Code)
		}
	}
	if rec := g.m2m(t, mreq{path: "/m2m/v1/audit-events", bearer: "forged", remote: "198.51.100.9:1"}); rec.Code != 401 {
		t.Fatalf("503s must not consume the failure budget: %d", rec.Code)
	}
}

func TestM2M_MethodNotAllowedListsAllow(t *testing.T) {
	f := newFixture(t)
	rec := f.m2m(t, mreq{method: "PUT", path: "/m2m/v1/audit-events", bearer: "m2m-audit"})
	if rec.Code != 405 || rec.Header().Get("Allow") != "GET" {
		t.Fatalf("405: %d Allow=%q", rec.Code, rec.Header().Get("Allow"))
	}
}

func TestM2M_PerClientLimiter(t *testing.T) {
	f := newFixture(t) // limiter: 5/min per client
	for i := range 5 {
		if rec := f.m2m(t, mreq{path: "/m2m/v1/audit-events", bearer: "m2m-audit"}); rec.Code != 200 {
			t.Fatalf("request %d: %d", i, rec.Code)
		}
	}
	if rec := f.m2m(t, mreq{path: "/m2m/v1/audit-events", bearer: "m2m-audit"}); rec.Code != 429 || problemCode(t, rec) != CodeRateLimited {
		t.Fatalf("over the limit: %d", rec.Code)
	}
	if rec := f.m2m(t, mreq{path: "/m2m/v1/customers/{customer}", bearer: "m2m-cust"}); rec.Code != 200 {
		t.Fatalf("other client: %d", rec.Code)
	}
}

func TestM2M_AccessLogLine(t *testing.T) {
	f := newFixture(t)
	f.m2m(t, mreq{path: "/m2m/v1/customers/{customer}", bearer: "m2m-cust"})
	f.m2m(t, mreq{path: "/m2m/v1/audit-events", bearer: "m2m-cust"})
	var lines []map[string]any
	for _, l := range strings.Split(f.logs.String(), "\n") {
		var m map[string]any
		if json.Unmarshal([]byte(l), &m) == nil && m["msg"] == "m2m_access" {
			lines = append(lines, m)
		}
	}
	if len(lines) != 2 {
		t.Fatalf("want 2 m2m_access lines, got %d: %s", len(lines), f.logs.String())
	}
	l := lines[0]
	if l["client_id"] != "client-cust" || l["jti"] != "jti-cust" || l["method"] != "GET" || l["route"] != "/m2m/v1/customers/{id}" ||
		l["target_id"] != f.customer.String() || l["status"] != float64(200) {
		t.Fatalf("log line: %v", l)
	}
	if lines[1]["status"] != float64(403) || lines[1]["route"] != "/m2m/v1/audit-events" {
		t.Fatalf("403 line: %v", lines[1])
	}
	if strings.Contains(f.logs.String(), "m2m-cust") {
		t.Fatal("the bearer token was logged")
	}
	if !strings.Contains(f.logs.String(), `"client_id":"client-cust"`) {
		t.Fatal("access log lacks client_id")
	}
}

func TestM2M_AuditFeedOverHTTP(t *testing.T) {
	f := newFixture(t)
	r := f.store.Repos()
	ctx := context.Background()
	_ = r.Audit.Append(ctx, audit.Event{ActorID: uuid.New(), Action: audit.ActionCustomerPIIRevealed, TargetType: "customer", TargetID: "x",
		RequestID: "rq", Details: map[string]any{"fields": []any{"phone_number"}, "ticket_ref": "T-9", "reason_code": "legal_request"}})
	_ = r.Audit.Append(ctx, audit.Event{ActorID: uuid.New(), Action: audit.ActionCustomerDisabled, TargetType: "customer", TargetID: "y",
		RequestID: "rq-secret-id", Details: map[string]any{"reason": "fraud suspicion"}})
	_ = r.Audit.Append(ctx, audit.Event{ActorID: uuid.New(), Action: audit.ActionAdminRoleChanged, TargetType: "admin", TargetID: "z",
		RequestID: "rq", Details: map[string]any{"role": "admin", "previous_role": "support"}})
	rec := f.m2m(t, mreq{path: "/m2m/v1/audit-events?page_size=1", bearer: "m2m-audit"})
	if rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var page struct {
		Items []map[string]any `json:"items"`
		Next  *string          `json:"next_page_token"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &page)
	if len(page.Items) != 1 || page.Next == nil || page.Items[0]["action"] != "admin.role_changed" {
		t.Fatalf("page 1: %s", rec.Body.String())
	}
	rec = f.m2m(t, mreq{path: "/m2m/v1/audit-events?page_size=10&page_token=" + *page.Next, bearer: "m2m-audit"})
	body := rec.Body.String()
	if rec.Code != 200 || !strings.Contains(body, "customer.disabled") {
		t.Fatalf("page 2: %d %s", rec.Code, body)
	}
	for _, leak := range []string{"customer.pii", "fraud", "ticket_ref", "T-9", "phone_number", "request_id", "rq-secret-id", "client_ip", "reason"} {
		if strings.Contains(body, leak) {
			t.Fatalf("machine audit feed leaks %q: %s", leak, body)
		}
	}
	if rec := f.m2m(t, mreq{path: "/m2m/v1/audit-events?page_size=0", bearer: "m2m-audit"}); rec.Code != 422 {
		t.Fatalf("page_size: %d", rec.Code)
	}
}

func TestServiceClients_CreateSecretOnceAndStrictBody(t *testing.T) {
	f := newFixture(t)
	key := uuid.NewString()
	rt := route{method: "POST", path: "/admin/v1/service-clients",
		header: func(*fixture) http.Header { return http.Header{"Idempotency-Key": {key}} },
		body: func(*fixture) string {
			return `{"name":"billing-sync","owner":"ops@example.com","scopes":["customers:read"]}`
		}}
	rec := f.do(t, rt, cred{cookie: "super", origin: origin})
	if rec.Code != 201 {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		ClientID     string   `json:"client_id"`
		ClientSecret *string  `json:"client_secret"`
		Scopes       []string `json:"scopes"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out.ClientSecret == nil || *out.ClientSecret == "" || out.ClientID == "" || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("create response: %s", rec.Body.String())
	}
	rec = f.do(t, rt, cred{cookie: "super", origin: origin})
	if rec.Code != 201 || strings.Contains(rec.Body.String(), "client_secret") || !strings.Contains(rec.Body.String(), out.ClientID) {
		t.Fatalf("replay must omit the secret: %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(f.logs.String(), *out.ClientSecret) {
		t.Fatal("secret logged")
	}
	for name, body := range map[string]string{
		"unknown field": `{"name":"billing-sync","owner":"ops@example.com","scopes":["customers:read"],"redirect_uris":["https://x"]}`,
		"bad scope":     `{"name":"billing-sync","owner":"ops@example.com","scopes":["admin"]}`,
		"no scopes":     `{"name":"billing-sync","owner":"ops@example.com","scopes":[]}`,
		"bad name":      `{"name":"Billing Sync","owner":"ops@example.com","scopes":["customers:read"]}`,
		"bad owner":     `{"name":"billing-sync","owner":"nope","scopes":["customers:read"]}`,
	} {
		rt.header = func(*fixture) http.Header { return http.Header{"Idempotency-Key": {uuid.NewString()}} }
		rt.body = func(*fixture) string { return body }
		if rec := f.do(t, rt, cred{cookie: "super", origin: origin}); rec.Code != 422 {
			t.Fatalf("%s: %d %s", name, rec.Code, rec.Body.String())
		}
	}
	rt.body = func(*fixture) string {
		return `{"name":"billing-two","owner":"ops@example.com","scopes":["customers:read"]}`
	}
	rt.header = nil
	if rec := f.do(t, rt, cred{cookie: "super", origin: origin}); rec.Code != 422 {
		t.Fatalf("missing Idempotency-Key: %d", rec.Code)
	}
}

func TestServiceClients_RotateReturnsNewSecretAndUnknownIs404(t *testing.T) {
	f := newFixture(t)
	old := f.clients.Secret(f.clientID)
	rec := f.do(t, route{method: "POST", path: "/admin/v1/service-clients/{client}/rotate-secret"}, cred{cookie: "super", origin: origin})
	var out struct {
		ClientSecret string `json:"client_secret"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if rec.Code != 200 || out.ClientSecret == "" || out.ClientSecret == old || f.clients.Secret(f.clientID) != out.ClientSecret {
		t.Fatalf("rotate: %d %s", rec.Code, rec.Body.String())
	}
	if len(f.mv.Invalidated) != 1 {
		t.Fatal("rotation must purge the client status cache")
	}
	for _, p := range []string{"/admin/v1/service-clients/missing", "/admin/v1/service-clients/" + strings.Repeat("a", 65)} {
		if rec := f.do(t, route{method: "GET", path: p}, cred{cookie: "super"}); rec.Code != 404 {
			t.Fatalf("%s: %d", p, rec.Code)
		}
	}
}

func TestP4_MachinePolicyValidation(t *testing.T) {
	r := chi.NewRouter()
	r.Get("/m2m/v1/things", func(http.ResponseWriter, *http.Request) {})
	r.Get("/admin/v1/me", func(http.ResponseWriter, *http.Request) {})
	admin := Policy{Plane: PlaneAdmin, Self: true}
	bad := map[string]Policy{
		"machine route without scope":   {Plane: PlaneMachine},
		"machine route unknown scope":   {Plane: PlaneMachine, Scope: "customers:write"},
		"machine route with permission": {Plane: PlaneMachine, Scope: machine.ScopeAuditRead, Permission: identity.PermViewAudit},
		"machine route with self":       {Plane: PlaneMachine, Scope: machine.ScopeAuditRead, Self: true},
		"machine route with aal1":       {Plane: PlaneMachine, Scope: machine.ScopeAuditRead, AllowAAL1: true},
		"machine route on admin plane":  {Plane: PlaneAdmin, Permission: identity.PermViewAudit},
	}
	for name, p := range bad {
		if err := ValidatePolicies(r, map[string]Policy{"GET /m2m/v1/things": p, "GET /admin/v1/me": admin}); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
	scoped := admin
	scoped.Scope = machine.ScopeAuditRead
	if err := ValidatePolicies(r, map[string]Policy{"GET /m2m/v1/things": {Plane: PlaneMachine, Scope: machine.ScopeAuditRead}, "GET /admin/v1/me": scoped}); err == nil ||
		!strings.Contains(err.Error(), "declares a scope") {
		t.Fatalf("non-machine route with a scope: %v", err)
	}
	if err := ValidatePolicies(r, map[string]Policy{"GET /m2m/v1/things": {Plane: PlaneMachine, Scope: machine.ScopeAuditRead}, "GET /admin/v1/me": admin}); err != nil {
		t.Fatalf("valid: %v", err)
	}
	// Every generated /m2m route carries a scope in RoutePolicies.
	n := 0
	for k, p := range RoutePolicies {
		if strings.Contains(k, " /m2m/") {
			n++
			if p.Plane != PlaneMachine || p.Scope == "" {
				t.Fatalf("%s: %+v", k, p)
			}
		}
	}
	if n != 2 {
		t.Fatalf("machine routes: %d", n)
	}
}

func TestM2M_NotConfiguredFailsClosed(t *testing.T) {
	f := newFixture(t)
	h, err := NewPublicHandler(PublicDeps{
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Server: &Server{}, Verifier: f.kratos, AllowedOrigins: []string{origin},
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/m2m/v1/audit-events", nil)
	req.Header.Set("Authorization", "Bearer m2m-audit")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 503 {
		t.Fatalf("no machine verifier: %d", rec.Code)
	}
}
