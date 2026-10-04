package kratos

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"testing"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
)

const handle = "l4cwc5fmnvxqxufy7wuuh2mfathke4fvwo3curj5ydaoo3iijsgq@login.invalid"

// fakeKratos answers the native self-service endpoints and records what
// identity-service sent.
type fakeKratos struct {
	mu      sync.Mutex
	headers []http.Header
	bodies  []map[string]any
	submit  func(kind string, body map[string]any) (int, string)
}

func (f *fakeKratos) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.headers = append(f.headers, r.Header.Clone())
	f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	kind := strings.TrimPrefix(r.URL.Path, "/self-service/")
	if r.Method == http.MethodGet {
		_, _ = w.Write([]byte(`{"id":"` + strings.TrimSuffix(kind, "/api") + `-flow"}`))
		return
	}
	if r.URL.Query().Get("flow") != kind+"-flow" {
		w.WriteHeader(http.StatusGone)
		return
	}
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	f.mu.Lock()
	f.bodies = append(f.bodies, body)
	f.mu.Unlock()
	status, out := f.submit(kind, body)
	w.WriteHeader(status)
	_, _ = w.Write([]byte(out))
}

func newSelfService(t *testing.T, submit func(string, map[string]any) (int, string)) (*SelfService, *fakeKratos) {
	t.Helper()
	f := &fakeKratos{submit: submit}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return NewSelfService(srv.URL, nil), f
}

var endClient = app.FlowClient{IP: netip.MustParseAddr("203.0.113.7"), UserAgent: "test-app/1.0"}

func TestPLXFR01_SelfServiceLogin(t *testing.T) {
	s, f := newSelfService(t, func(kind string, b map[string]any) (int, string) {
		if b["identifier"] == handle && b["password"] == "right" {
			return 200, `{"session_token":"tok","session":{"id":"sess","identity":{"traits":{"login_id":"` + handle + `"}}}}`
		}
		// Kratos echoes the identifier in the node value: it must not leak.
		return 400, `{"id":"login-flow","ui":{"messages":[{"id":4000006,"type":"error","text":"The provided credentials are invalid"}],
			"nodes":[{"attributes":{"name":"identifier","value":"` + handle + `"},"messages":[]},
			{"attributes":{"name":"password"},"messages":[{"id":4000032,"type":"error","context":{"min_length":12}},{"id":1,"type":"info"}]}]}}`
	})
	sess, err := s.Login(context.Background(), endClient, handle, "right")
	if err != nil || sess.Token != "tok" || !strings.Contains(string(sess.Session), `"id":"sess"`) {
		t.Fatalf("login: %+v %v", sess, err)
	}
	if h := f.headers[1]; h.Get("X-Forwarded-For") != "203.0.113.7" || h.Get("User-Agent") != "test-app/1.0" {
		t.Fatalf("endClient not forwarded: %v", h)
	}
	if b := f.bodies[0]; b["method"] != "password" || len(b) != 3 {
		t.Fatalf("body %v", b)
	}
	_, err = s.Login(context.Background(), endClient, handle, "wrong")
	var fe *app.AuthFlowError
	if !errors.As(err, &fe) || len(fe.Messages) != 2 ||
		fe.Messages[0] != (app.FlowMessage{Field: app.FlowFieldForm, ID: 4000006}) ||
		fe.Messages[1] != (app.FlowMessage{Field: app.FlowFieldPassword, ID: 4000032}) {
		t.Fatalf("rejection: %#v", err)
	}
	if strings.Contains(err.Error(), handle) || strings.Contains(err.Error(), "wrong") {
		t.Fatal("error leaks a value")
	}
	if str := sess.String(); strings.Contains(str, "tok") {
		t.Fatal("session must redact itself")
	}
}

func TestPLXFR02_SelfServiceRegistration(t *testing.T) {
	s, f := newSelfService(t, func(kind string, b map[string]any) (int, string) {
		if kind != "registration" {
			return 500, `{}`
		}
		return 200, `{"session_token":"tok","session":{"id":"sess"},"continue_with":[{"action":"show_verification_ui","flow":{"id":"9b2f8c1e-0000-4000-8000-000000000001","verifiable_address":"` + handle + `"}}]}`
	})
	sess, err := s.Register(context.Background(), endClient, handle, "a long password")
	if err != nil || sess.VerificationFlowID != "9b2f8c1e-0000-4000-8000-000000000001" {
		t.Fatalf("register: %+v %v", sess, err)
	}
	traits, _ := f.bodies[0]["traits"].(map[string]any)
	if len(traits) != 1 || traits["login_id"] != handle || f.bodies[0]["method"] != "password" {
		t.Fatalf("body %v", f.bodies[0])
	}
}

func TestPLXFR03_SelfServiceRecovery(t *testing.T) {
	s, f := newSelfService(t, func(kind string, b map[string]any) (int, string) {
		return 200, `{"id":"recovery-flow","state":"sent_email"}`
	})
	id, err := s.StartRecovery(context.Background(), endClient, handle)
	if err != nil || id != "recovery-flow" {
		t.Fatalf("recovery: %q %v", id, err)
	}
	if b := f.bodies[0]; b["method"] != "code" || b["email"] != handle {
		t.Fatalf("body %v", b)
	}
}

func TestPLXNFR04_SelfServiceFailures(t *testing.T) {
	s, _ := newSelfService(t, func(string, map[string]any) (int, string) { return 503, `{}` })
	if _, err := s.Login(context.Background(), endClient, handle, "pw"); !errors.Is(err, app.ErrDependencyUnavailable) {
		t.Fatalf("5xx: %v", err)
	}
	s, _ = newSelfService(t, func(string, map[string]any) (int, string) { return 400, `{"error":{"id":"bad"}}` })
	_, err := s.Login(context.Background(), endClient, handle, "pw")
	var fe *app.AuthFlowError
	if err == nil || errors.As(err, &fe) {
		t.Fatalf("a 400 without a flow is not a rejection: %v", err)
	}
	s, _ = newSelfService(t, func(string, map[string]any) (int, string) { return 200, `{"session":{}}` })
	if _, err := s.Login(context.Background(), endClient, handle, "pw"); err == nil {
		t.Fatal("a success without a token must fail")
	}
}
