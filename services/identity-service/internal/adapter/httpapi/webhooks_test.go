package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/testutil"
)

const hookKey = "test-webhook-key-0123456789"

func webhookHandler() (*testutil.Store, *httptest.Server) {
	store := testutil.NewStore(nil)
	h := NewWebhookHandler(WebhookDeps{
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Metrics: NewMetrics(), APIKey: hookKey,
		Provisioning: &app.ProvisioningService{Profiles: store.Repos().Profiles},
	})
	return store, httptest.NewServer(h)
}

func post(t *testing.T, srv *httptest.Server, path, key, body string) (int, string) {
	t.Helper()
	req := httptest.NewRequest("POST", srv.URL+path, strings.NewReader(body))
	req.RequestURI = ""
	if key != "" {
		req.Header.Set("Authorization", key)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func TestFR09_AfterRegistrationWebhook(t *testing.T) {
	store, srv := webhookHandler()
	defer srv.Close()
	id := uuid.New()
	body := `{"identity_id":"` + id.String() + `","schema_id":"customer","flow_type":"api"}`
	if code, _ := post(t, srv, "/internal/hooks/kratos/after-registration", "", body); code != 401 {
		t.Fatalf("no key: %d", code)
	}
	if code, _ := post(t, srv, "/internal/hooks/kratos/after-registration", "wrong", body); code != 401 {
		t.Fatalf("bad key: %d", code)
	}
	for range 2 {
		if code, b := post(t, srv, "/internal/hooks/kratos/after-registration", hookKey, body); code != 204 {
			t.Fatalf("ok: %d %s", code, b)
		}
	}
	if _, ok := store.Profiles[id]; !ok {
		t.Fatal("profile not created")
	}
	if code, _ := post(t, srv, "/internal/hooks/kratos/after-registration", hookKey, `{"identity_id":"x"}`); code != 422 {
		t.Fatalf("bad body: %d", code)
	}
	if code, _ := post(t, srv, "/v1/me", hookKey, body); code != 404 {
		t.Fatalf("public routes must not exist on the webhook port: %d", code)
	}
}

func TestFR05_AfterLoginPopulationGuard(t *testing.T) {
	_, srv := webhookHandler()
	defer srv.Close()
	tests := []struct {
		schema, flow string
		want         int
	}{
		{"customer", "api", 204}, {"customer", "browser", 403},
		{"admin", "browser", 204}, {"admin", "api", 403},
	}
	for _, tt := range tests {
		body := `{"identity_id":"` + uuid.NewString() + `","schema_id":"` + tt.schema + `","flow_type":"` + tt.flow + `"}`
		code, b := post(t, srv, "/internal/hooks/kratos/after-login", hookKey, body)
		if code != tt.want {
			t.Fatalf("%s/%s: %d want %d", tt.schema, tt.flow, code, tt.want)
		}
		if code == 403 {
			var msg struct {
				Messages []struct {
					InstancePtr string `json:"instance_ptr"`
					Messages    []struct {
						ID   int    `json:"id"`
						Type string `json:"type"`
						Text string `json:"text"`
					} `json:"messages"`
				} `json:"messages"`
			}
			if err := json.Unmarshal([]byte(b), &msg); err != nil || len(msg.Messages) != 1 ||
				msg.Messages[0].InstancePtr != "#/" || msg.Messages[0].Messages[0].ID != LoginInterruptMessageID ||
				msg.Messages[0].Messages[0].Type != "error" {
				t.Fatalf("bad Kratos interrupt body: %s", b)
			}
		}
	}
	if code, _ := post(t, srv, "/internal/hooks/kratos/after-login", "nope", `{}`); code != 401 {
		t.Fatalf("bad key: %d", code)
	}
}
