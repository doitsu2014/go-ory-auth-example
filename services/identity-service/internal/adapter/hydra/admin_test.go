package hydra

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/machine"
)

type recorded struct {
	method, path, query, contentType string
	body                             []byte
}

func adminServer(t *testing.T, h func(w http.ResponseWriter, r *http.Request)) (*Admin, *[]recorded) {
	t.Helper()
	var mu sync.Mutex
	var reqs []recorded
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		reqs = append(reqs, recorded{r.Method, r.URL.EscapedPath(), r.URL.RawQuery, r.Header.Get("Content-Type"), b})
		mu.Unlock()
		h(w, r)
	}))
	t.Cleanup(srv.Close)
	return mustAdmin(t, srv.URL, nil), &reqs
}

func TestAdminCreateSendsExplicitBody(t *testing.T) {
	a, reqs := adminServer(t, func(w http.ResponseWriter, _ *http.Request) {
		out := managedClientJSON("new-id", nil)
		out["client_secret"] = "s3cr3t-from-hydra"
		out["registration_access_token"] = "ory_at_x"
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(out)
	})
	by := uuid.New()
	reg, err := machine.NewRegistration("billing", "ops@example.com", []string{"audit:read", "customers:read"})
	if err != nil {
		t.Fatal(err)
	}
	c, secret, err := a.Create(context.Background(), app.NewServiceClient{ClientID: "new-id", Registration: reg, CreatedBy: by})
	if err != nil || secret != "s3cr3t-from-hydra" || c.ClientID != "new-id" || c.Owner != "ops@example.com" {
		t.Fatalf("create: %+v %q %v", c, secret, err)
	}
	r := (*reqs)[0]
	if r.method != "POST" || r.path != "/admin/clients" || r.contentType != "application/json" {
		t.Fatalf("request: %+v", r)
	}
	var body map[string]any
	if err := json.Unmarshal(r.body, &body); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"client_name": `"billing"`, "grant_types": `["client_credentials"]`, "response_types": `["token"]`,
		"scope": `"customers:read audit:read"`, "audience": `["identity-service"]`,
		"token_endpoint_auth_method": `"client_secret_basic"`, "access_token_strategy": `"jwt"`,
	}
	for k, v := range want {
		got, _ := json.Marshal(body[k])
		if string(got) != v {
			t.Errorf("%s = %s want %s", k, got, v)
		}
	}
	for _, k := range []string{"redirect_uris", "jwks", "jwks_uri", "client_secret",
		"client_credentials_grant_access_token_lifespan", "post_logout_redirect_uris"} {
		if _, ok := body[k]; ok {
			t.Errorf("body must not set %s", k)
		}
	}
	md := body["metadata"].(map[string]any)
	if md["managed_by"] != "identity-service" || md["owner"] != "ops@example.com" || md["created_by"] != by.String() {
		t.Fatalf("metadata: %v", md)
	}
	if body["client_id"] != "new-id" {
		t.Fatalf("client_id must be chosen by identity-service: %v", body["client_id"])
	}
	wantTag := integrityTag(testTagKey, "new-id", []string{"audit:read", "customers:read"}, testAudience, by.String())
	if md["integrity"] != wantTag {
		t.Fatalf("integrity tag: %v want %v", md["integrity"], wantTag)
	}
}

func TestAdminCreateRejectsMismatchedResponse(t *testing.T) {
	a, reqs := adminServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		out := managedClientJSON("new-id", func(m map[string]any) { m["grant_types"] = []string{"authorization_code"} })
		out["client_secret"] = "s"
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(out)
	})
	reg, _ := machine.NewRegistration("billing", "ops@example.com", []string{"audit:read"})
	_, _, err := a.Create(context.Background(), app.NewServiceClient{ClientID: "new-id", Registration: reg})
	if err == nil || strings.Contains(err.Error(), "s\"") {
		t.Fatalf("want error without body: %v", err)
	}
	if len(*reqs) != 2 || (*reqs)[1].method != "DELETE" || (*reqs)[1].path != "/admin/clients/new-id" {
		t.Fatalf("mismatched client must be deleted: %+v", *reqs)
	}
}

func TestAdminGetListFilterManaged(t *testing.T) {
	a, reqs := adminServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/admin/clients" && r.URL.Query().Get("page_token") == "":
			w.Header().Set("Link", `</admin/clients?page_size=500&page_token=p2>; rel="next", </admin/clients?page_size=500>; rel="first"`)
			_ = json.NewEncoder(w).Encode([]any{
				managedClientJSON("a", nil),
				managedClientJSON("foreign", func(m map[string]any) { m["metadata"] = map[string]any{"managed_by": "kratos"} }),
			})
		case r.URL.Path == "/admin/clients":
			_ = json.NewEncoder(w).Encode([]any{
				managedClientJSON("b", func(m map[string]any) { m["created_at"] = "2026-10-04T00:00:00Z" }),
				managedClientJSON("code", func(m map[string]any) { m["grant_types"] = []string{"authorization_code"} }),
			})
		case r.URL.Path == "/admin/clients/foreign":
			_ = json.NewEncoder(w).Encode(managedClientJSON("foreign", func(m map[string]any) { delete(m, "metadata") }))
		case r.URL.Path == "/admin/clients/a":
			_ = json.NewEncoder(w).Encode(managedClientJSON("a", nil))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	list, err := a.List(context.Background())
	if err != nil || len(list) != 2 || list[0].ClientID != "b" || list[1].ClientID != "a" {
		t.Fatalf("list (managed only, newest first): %+v %v", list, err)
	}
	if !strings.Contains((*reqs)[1].query, "page_token=p2") {
		t.Fatalf("pagination: %+v", *reqs)
	}
	if _, err := a.Get(context.Background(), "foreign"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("unmanaged client must be 404: %v", err)
	}
	if _, err := a.Get(context.Background(), "missing"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
	c, err := a.Get(context.Background(), "a")
	if err != nil || c.CreatedBy == nil || *c.CreatedBy != uuid.Nil || len(c.Scopes) != 2 {
		t.Fatalf("get: %+v %v", c, err)
	}
	if _, err := a.Get(context.Background(), "../x"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("escaped id: %v", err)
	}
	if last := (*reqs)[len(*reqs)-1]; last.path != "/admin/clients/..%2Fx" {
		t.Fatalf("id must be path-escaped: %q", last.path)
	}
}

func TestAdminSetSecretPatchesSecretAndTokensValidAfter(t *testing.T) {
	a, reqs := adminServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/gone") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(managedClientJSON("a", nil))
	})
	at := time.Unix(1_791_000_000, 0)
	if err := a.SetSecret(context.Background(), "a", "new-secret", at); err != nil {
		t.Fatal(err)
	}
	r := (*reqs)[0]
	if r.method != "PATCH" || r.path != "/admin/clients/a" {
		t.Fatalf("request: %+v", r)
	}
	want := `[{"op":"replace","path":"/client_secret","value":"new-secret"},{"op":"add","path":"/metadata/tokens_valid_after","value":1791000000}]`
	if string(r.body) != want {
		t.Fatalf("patch body:\n%s\nwant\n%s", r.body, want)
	}
	if err := a.SetSecret(context.Background(), "gone", "x", at); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
}

func TestAdminErrorsAreValueFreeAndDoNotFollowRedirects(t *testing.T) {
	var hitTarget bool
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hitTarget = true }))
	defer target.Close()
	a, _ := adminServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"invalid_client_metadata","error_description":"secret-ish detail ops@example.com"}`))
			return
		}
		http.Redirect(w, r, target.URL+"/admin/clients/a", http.StatusTemporaryRedirect)
	})
	reg, _ := machine.NewRegistration("billing", "ops@example.com", []string{"audit:read"})
	_, _, err := a.Create(context.Background(), app.NewServiceClient{ClientID: "x", Registration: reg})
	if err == nil || strings.Contains(err.Error(), "ops@example.com") || strings.Contains(err.Error(), "secret-ish") {
		t.Fatalf("error must not carry the body: %v", err)
	}
	if _, err := a.Get(context.Background(), "a"); err == nil {
		t.Fatal("redirect must not be treated as success")
	}
	if hitTarget {
		t.Fatal("redirect followed")
	}
	if err := a.Delete(context.Background(), "a"); err == nil {
		t.Fatal("redirect on delete must fail")
	}
}

func TestAdminUnavailable(t *testing.T) {
	a := mustAdmin(t, "http://127.0.0.1:1", &http.Client{Timeout: 200 * time.Millisecond})
	if _, err := a.Get(context.Background(), "a"); !errors.Is(err, app.ErrDependencyUnavailable) {
		t.Fatalf("transport error: %v", err)
	}
	if _, err := a.status(context.Background(), "a"); !errors.Is(err, app.ErrDependencyUnavailable) {
		t.Fatalf("status: %v", err)
	}
}

func TestAdminCreateConflictAndIDMismatch(t *testing.T) {
	var deleted []string
	status := http.StatusConflict
	a, _ := adminServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deleted = append(deleted, r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if status == http.StatusConflict {
			w.WriteHeader(status)
			return
		}
		out := managedClientJSON("hydra-chose-another-id", nil)
		out["client_secret"] = "s"
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(out)
	})
	reg, _ := machine.NewRegistration("billing", "ops@example.com", []string{"audit:read"})
	if _, _, err := a.Create(context.Background(), app.NewServiceClient{ClientID: "mine", Registration: reg}); !errors.Is(err, app.ErrConflict) {
		t.Fatalf("409: %v", err)
	}
	if len(deleted) != 0 {
		t.Fatal("a conflict must not delete the existing client")
	}
	status = http.StatusCreated
	if _, _, err := a.Create(context.Background(), app.NewServiceClient{ClientID: "mine", Registration: reg}); err == nil {
		t.Fatal("id mismatch must fail")
	}
	if len(deleted) != 1 || deleted[0] != "/admin/clients/hydra-chose-another-id" {
		t.Fatalf("mismatched client must be deleted: %v", deleted)
	}
	if _, _, err := a.Create(context.Background(), app.NewServiceClient{Registration: reg}); err == nil {
		t.Fatal("client_id is required")
	}
}

func TestAdminNon404StatusesAreDependencyErrors(t *testing.T) {
	for _, st := range []int{401, 403, 429, 409} {
		a, _ := adminServer(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(st) })
		if _, err := a.Get(context.Background(), "a"); !errors.Is(err, app.ErrDependencyUnavailable) {
			t.Fatalf("get %d: %v", st, err)
		}
		if _, err := a.status(context.Background(), "a"); !errors.Is(err, app.ErrDependencyUnavailable) {
			t.Fatalf("status %d: %v", st, err)
		}
		if err := a.Delete(context.Background(), "a"); !errors.Is(err, app.ErrDependencyUnavailable) {
			t.Fatalf("delete %d: %v", st, err)
		}
	}
	a, _ := adminServer(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadRequest) })
	if _, err := a.Get(context.Background(), "a"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("400 = malformed id: %v", err)
	}
}

func TestNewAdminRequiresTagKey(t *testing.T) {
	if _, err := NewAdmin("http://x", testAudience, []byte("short"), nil); err == nil {
		t.Fatal("short tag key must be refused")
	}
}
