package kratos

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
)

const pseudo = "l4cwc5fmnvxqxufy7wuuh2mfathke4fvwo3curj5ydaoo3iijsgq@login.invalid"

func loginServer(t *testing.T, identity map[string]any) (*Admin, *[][]map[string]any) {
	t.Helper()
	var patches [][]map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(identity)
		case http.MethodPatch:
			var p []map[string]any
			_ = json.NewDecoder(r.Body).Decode(&p)
			patches = append(patches, p)
			_ = json.NewEncoder(w).Encode(identity)
		}
	}))
	t.Cleanup(srv.Close)
	return NewAdmin(srv.URL, srv.Client()), &patches
}

func TestPLIFR13_ReplaceLoginTraitReadCompare(t *testing.T) {
	id := uuid.New()
	a, patches := loginServer(t, map[string]any{"id": id, "schema_id": "customer", "traits": map[string]any{"email": "Alice@Example.com"}})
	if err := a.ReplaceLoginTrait(context.Background(), id, "alice@example.com", pseudo); err != nil {
		t.Fatal(err)
	}
	p := (*patches)[0]
	if len(p) != 2 || p[0]["op"] != "remove" || p[0]["path"] != "/traits/email" || p[1]["op"] != "add" || p[1]["path"] != "/traits/login_id" || p[1]["value"] != pseudo {
		t.Fatalf("patch %v", p)
	}
	if _, has := p[0]["value"]; has {
		t.Fatal("remove op must not carry a value")
	}
	if err := a.ReplaceLoginTrait(context.Background(), id, "other@example.com", pseudo); !errors.Is(err, app.ErrConflict) {
		t.Fatalf("changed email: %v", err)
	}
	b, _ := loginServer(t, map[string]any{"id": id, "schema_id": "admin", "traits": map[string]any{"email": "alice@example.com"}})
	if err := b.ReplaceLoginTrait(context.Background(), id, "alice@example.com", pseudo); !errors.Is(err, app.ErrConflict) {
		t.Fatal("admins are never migrated")
	}
}

func TestPLIFR13_MarkLoginVerifiedSecondPatch(t *testing.T) {
	id := uuid.New()
	a, patches := loginServer(t, map[string]any{"id": id, "schema_id": "customer", "traits": map[string]any{"login_id": pseudo},
		"verifiable_addresses": []map[string]any{{"value": "x@login.invalid", "via": "email"}, {"value": pseudo, "via": "email", "verified": false}}})
	if err := a.MarkLoginVerified(context.Background(), id, pseudo); err != nil {
		t.Fatal(err)
	}
	p := (*patches)[0]
	if p[0]["path"] != "/verifiable_addresses/1/verified" || p[0]["value"] != true || p[1]["path"] != "/verifiable_addresses/1/status" || p[1]["value"] != "completed" {
		t.Fatalf("patch %v", p)
	}
	if err := a.MarkLoginVerified(context.Background(), id, "missing@login.invalid"); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("missing address: %v", err)
	}
}

func TestSECC01_TransportErrorsCarryNoQuery(t *testing.T) {
	a := NewAdmin("http://127.0.0.1:1", nil) // nothing listens on port 1
	_, _, err := a.ListIdentities(context.Background(), app.IdentityQuery{Email: "victim@example.com"})
	if err == nil || !errors.Is(err, app.ErrDependencyUnavailable) {
		t.Fatalf("want dependency error, got %v", err)
	}
	if s := err.Error(); strings.Contains(s, "victim") || strings.Contains(s, "%40") || strings.Contains(s, "credentials_identifier") {
		t.Fatalf("error leaks the query: %s", s)
	}
}
