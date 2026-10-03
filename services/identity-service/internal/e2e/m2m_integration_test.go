//go:build integration

package e2e

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/audit"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/identity"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/testutil/itest"
)

const m2mAudience = "identity-service"

type serviceClientResp struct {
	ClientID     string   `json:"client_id"`
	ClientSecret *string  `json:"client_secret"`
	Name         string   `json:"name"`
	Owner        string   `json:"owner"`
	Scopes       []string `json:"scopes"`
	CreatedBy    *string  `json:"created_by"`
}

// m2mCall calls the in-process service with an arbitrary header set and
// returns status, problem code and WWW-Authenticate.
func (s *stack) m2mCall(t *testing.T, path, bearer string, out any) (int, string, string) {
	t.Helper()
	h := http.Header{}
	if bearer != "" {
		h.Set("Authorization", "Bearer "+bearer)
	}
	var p problem
	var target any = &p
	if out != nil {
		target = out
	}
	st, hdr := itest.JSON(t, nil, "GET", s.srv.URL+path, h, nil, target)
	return st, p.Code, hdr.Get("WWW-Authenticate")
}

// TestM2M_E2E: a super_admin registers a client through the service, the
// client gets a JWT from the real Hydra token endpoint and uses the machine
// plane; plane binding, scope, audience, rotation and deletion are enforced.
func TestM2M_E2E(t *testing.T) {
	s := newStack(t)
	cookie, superID := s.adminSession(t, identity.RoleSuperAdmin)
	custTok, custID := s.verifiedCustomer(t)

	// Non-super admins cannot manage service clients (Keto OPL).
	adminCookie, _ := s.adminSession(t, identity.RoleAdmin)
	var p problem
	if st := s.api(t, "GET", "/admin/v1/service-clients", call{cookie: adminCookie}, nil, &p); st != 403 || p.Code != "forbidden" {
		t.Fatalf("admin role on service clients: %d %s", st, p.Code)
	}

	// Create (M2M-FR-08): secret once, replay without it.
	key := uuid.NewString()
	name := "e2e-" + uuid.NewString()[:8]
	body := map[string]any{"name": name, "owner": "ops@example.com", "scopes": []string{"customers:read", "audit:read"}}
	var created serviceClientResp
	if st := s.api(t, "POST", "/admin/v1/service-clients", call{cookie: cookie, origin: webOrigin, idemKey: key}, body, &created); st != 201 ||
		created.ClientSecret == nil || *created.ClientSecret == "" || created.ClientID == "" {
		t.Fatalf("create: %d %+v", st, created)
	}
	t.Cleanup(func() { s.env.DeleteHydraClient(t, created.ClientID) })
	secret := *created.ClientSecret
	if created.CreatedBy == nil || *created.CreatedBy != superID.String() {
		t.Fatalf("created_by: %v", created.CreatedBy)
	}
	var replay serviceClientResp
	if st := s.api(t, "POST", "/admin/v1/service-clients", call{cookie: cookie, origin: webOrigin, idemKey: key}, body, &replay); st != 201 ||
		replay.ClientSecret != nil || replay.ClientID != created.ClientID {
		t.Fatalf("replay: %d %+v", st, replay)
	}
	var list struct {
		Items []serviceClientResp `json:"items"`
	}
	if st := s.api(t, "GET", "/admin/v1/service-clients", call{cookie: cookie}, nil, &list); st != 200 {
		t.Fatalf("list: %d", st)
	}
	found := false
	for _, it := range list.Items {
		found = found || it.ClientID == created.ClientID
		if it.ClientSecret != nil {
			t.Fatal("list must never return secrets")
		}
	}
	if !found {
		t.Fatal("created client not listed")
	}

	// Token from Hydra → /m2m/v1/customers/{id} (M2M-FR-01, 05).
	st, custScopeTok := s.env.ClientCredentialsToken(t, created.ClientID, secret, "customers:read", m2mAudience)
	if st != 200 {
		t.Fatalf("token: %d", st)
	}
	var mc map[string]any
	if st, _, _ := s.m2mCall(t, "/m2m/v1/customers/"+custID.String(), custScopeTok, &mc); st != 200 {
		t.Fatalf("m2m customer: %d %v", st, mc)
	}
	if mc["id"] != custID.String() || mc["state"] != "active" || mc["email_verified"] != true || len(mc) != 4 {
		t.Fatalf("machine customer must hold id/state/email_verified/created_at only: %v", mc)
	}

	// Wrong scope → 403 insufficient_scope with a challenge.
	if st, code, www := s.m2mCall(t, "/m2m/v1/audit-events", custScopeTok, nil); st != 403 || code != "insufficient_scope" ||
		!strings.Contains(www, `error="insufficient_scope"`) || !strings.Contains(www, `scope="audit:read"`) {
		t.Fatalf("wrong scope: %d %s %q", st, code, www)
	}
	// Scope the client was not registered for: Hydra refuses to issue it.
	if st, _ := s.env.ClientCredentialsToken(t, created.ClientID, secret, "customers:write", m2mAudience); st != 400 {
		t.Fatalf("unknown scope at Hydra: %d", st)
	}

	// Audit feed (§6 B1): allowlisted actions and detail keys only.
	_, auditTok := s.env.ClientCredentialsToken(t, created.ClientID, secret, "audit:read", m2mAudience)
	var feed struct {
		Items []struct {
			Action  string         `json:"action"`
			Details map[string]any `json:"details"`
		} `json:"items"`
	}
	if st, _, _ := s.m2mCall(t, "/m2m/v1/audit-events?page_size=100", auditTok, &feed); st != 200 || len(feed.Items) == 0 {
		t.Fatalf("audit feed: %d %d", st, len(feed.Items))
	}
	sawCreate := false
	allowedKeys := map[string]bool{"role": true, "previous_role": true, "scopes": true, "name": true}
	for _, e := range feed.Items {
		if strings.HasPrefix(e.Action, "customer.pii.") || !audit.MachineVisible(audit.Action(e.Action)) {
			t.Fatalf("disallowed action in machine feed: %s", e.Action)
		}
		for k := range e.Details {
			if !allowedKeys[k] {
				t.Fatalf("disallowed detail key %q in %s", k, e.Action)
			}
		}
		sawCreate = sawCreate || (e.Action == "service_client.created" && e.Details["name"] == name)
	}
	if !sawCreate {
		t.Fatal("service_client.created missing from the feed")
	}

	// Malformed Authorization → 400 invalid_request; wrong method → 405.
	if st, _ := itest.JSON(t, nil, "GET", s.srv.URL+"/m2m/v1/audit-events", http.Header{"Authorization": {"Basic eDp5"}}, nil, &p); st != 400 || p.Code != "invalid_request" {
		t.Fatalf("malformed authorization: %d %s", st, p.Code)
	}
	if st, _ := itest.JSON(t, nil, "DELETE", s.srv.URL+"/m2m/v1/customers/"+custID.String(), http.Header{"Authorization": {"Bearer " + auditTok}}, nil, &p); st != 405 {
		t.Fatalf("wrong method: %d", st)
	}

	// Plane binding (M2M-FR-02, §6 A13).
	if st, code, www := s.m2mCall(t, "/m2m/v1/customers/"+custID.String(), custTok, nil); st != 401 || code != "invalid_token" ||
		!strings.Contains(www, `error="invalid_token"`) {
		t.Fatalf("Kratos token on /m2m: %d %s %q", st, code, www)
	}
	if st := s.api(t, "GET", "/m2m/v1/customers/"+custID.String(), call{cookie: cookie}, nil, &p); st != 401 {
		t.Fatalf("Kratos cookie on /m2m: %d", st)
	}
	if st := s.api(t, "GET", "/v1/me", call{bearer: custScopeTok}, nil, &p); st != 401 {
		t.Fatalf("Hydra JWT on /v1/me: %d", st)
	}
	if st := s.api(t, "GET", "/admin/v1/me", call{bearer: custScopeTok}, nil, &p); st != 401 {
		t.Fatalf("Hydra JWT on /admin/v1/me: %d", st)
	}
	if st := s.api(t, "GET", "/admin/v1/customers", call{bearer: custScopeTok}, nil, &p); st != 401 {
		t.Fatalf("Hydra JWT on /admin/v1/customers: %d", st)
	}

	// Token without the audience parameter (aud: []) → 401.
	_, noAud := s.env.ClientCredentialsToken(t, created.ClientID, secret, "customers:read", "")
	if st, code, _ := s.m2mCall(t, "/m2m/v1/customers/"+custID.String(), noAud, nil); st != 401 || code != "invalid_token" {
		t.Fatalf("token without audience: %d %s", st, code)
	}

	// Rotate (M2M-FR-10, §6 B3): old secret and old tokens stop working — a
	// token from the rotation second too (tokens_valid_after = ceil(now)+1).
	_, sameSecondTok := s.env.ClientCredentialsToken(t, created.ClientID, secret, "customers:read", m2mAudience)
	var rotated serviceClientResp
	if st := s.api(t, "POST", "/admin/v1/service-clients/"+created.ClientID+"/rotate-secret", call{cookie: cookie, origin: webOrigin}, nil, &rotated); st != 200 ||
		rotated.ClientSecret == nil || *rotated.ClientSecret == secret {
		t.Fatalf("rotate: %d %+v", st, rotated)
	}
	if st, _ := s.env.ClientCredentialsToken(t, created.ClientID, secret, "customers:read", m2mAudience); st != 401 {
		t.Fatalf("old secret at the token endpoint: %d", st)
	}
	if st, code, _ := s.m2mCall(t, "/m2m/v1/customers/"+custID.String(), custScopeTok, nil); st != 401 || code != "invalid_token" {
		t.Fatalf("token issued before rotation: %d %s", st, code)
	}
	if st, _, _ := s.m2mCall(t, "/m2m/v1/customers/"+custID.String(), sameSecondTok, nil); st != 401 {
		t.Fatalf("token issued just before rotation: %d", st)
	}
	time.Sleep(2100 * time.Millisecond) // documented: fetch a new token 1–2 s after a rotation
	st, newTok := s.env.ClientCredentialsToken(t, created.ClientID, *rotated.ClientSecret, "customers:read", m2mAudience)
	if st != 200 {
		t.Fatalf("token with the new secret: %d", st)
	}
	if st, _, _ := s.m2mCall(t, "/m2m/v1/customers/"+custID.String(), newTok, nil); st != 200 {
		t.Fatalf("new token: %d", st)
	}

	// Delete (M2M-FR-11): 401 at once on this replica (cache purged).
	if st := s.api(t, "DELETE", "/admin/v1/service-clients/"+created.ClientID, call{cookie: cookie, origin: webOrigin}, nil, nil); st != 204 {
		t.Fatalf("delete: %d", st)
	}
	if st, code, _ := s.m2mCall(t, "/m2m/v1/customers/"+custID.String(), newTok, nil); st != 401 || code != "invalid_token" {
		t.Fatalf("deleted client's token: %d %s", st, code)
	}
	if st, _ := s.env.ClientCredentialsToken(t, created.ClientID, *rotated.ClientSecret, "customers:read", m2mAudience); st != 401 {
		t.Fatalf("deleted client at the token endpoint: %d", st)
	}
	if st := s.api(t, "GET", "/admin/v1/service-clients/"+created.ClientID, call{cookie: cookie}, nil, &p); st != 404 {
		t.Fatalf("get deleted: %d", st)
	}

	// Audit trail of the mutations, and nothing secret in the logs.
	var events struct {
		Items []struct {
			Action  string         `json:"action"`
			Details map[string]any `json:"details"`
		} `json:"items"`
	}
	tt := "service_client"
	if st := s.api(t, "GET", "/admin/v1/audit-events?target_type="+tt+"&target_id="+created.ClientID, call{cookie: cookie}, nil, &events); st != 200 {
		t.Fatalf("audit: %d", st)
	}
	seen := map[string]bool{}
	for _, e := range events.Items {
		seen[e.Action] = true
	}
	for _, a := range []string{"service_client.created", "service_client.secret_rotation_started", "service_client.secret_rotated",
		"service_client.deletion_started", "service_client.deleted"} {
		if !seen[a] {
			t.Fatalf("audit missing %s: %v", a, seen)
		}
	}
	logs := s.logs.String()
	for _, leak := range []string{secret, *rotated.ClientSecret, custScopeTok, newTok} {
		if strings.Contains(logs, leak) {
			t.Fatal("a secret or token reached the logs")
		}
	}
	if !strings.Contains(logs, `"msg":"m2m_access"`) || !strings.Contains(logs, created.ClientID) {
		t.Fatal("m2m_access log line missing")
	}
}
