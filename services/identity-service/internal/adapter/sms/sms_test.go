package sms

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/testutil"
)

func TestSink_WritesToMailCatcher(t *testing.T) {
	m := &testutil.Mailer{}
	if err := (&Sink{Mailer: m}).SendSMS(context.Background(), "+84912345678", "code 123456"); err != nil {
		t.Fatal(err)
	}
	if len(m.LoginSent) != 1 || m.LoginSent[0].To != "84912345678@sms.local" || !strings.Contains(m.LoginSent[0].Message.Text, "123456") {
		t.Fatalf("%+v", m.LoginSent)
	}
	if err := (&Sink{Mailer: m}).SendSMS(context.Background(), "+84 x", "t"); err == nil {
		t.Fatal("invalid number accepted")
	}
}

func TestHTTP_ProviderContract(t *testing.T) {
	var got map[string]string
	var auth string
	status := http.StatusAccepted
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"echo":"+84912345678"}`))
	}))
	defer srv.Close()
	if _, err := NewHTTP(srv.URL, "tok", false, nil); err == nil {
		t.Fatal("plain http must be refused outside local")
	}
	h, err := NewHTTP(srv.URL, "tok", true, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	if err := h.SendSMS(context.Background(), "+84912345678", "code 123456"); err != nil {
		t.Fatal(err)
	}
	if auth != "Bearer tok" || got["to"] != "+84912345678" || got["text"] != "code 123456" {
		t.Fatalf("request %v %q", got, auth)
	}
	status = http.StatusBadGateway
	err = h.SendSMS(context.Background(), "+84912345678", "x")
	if !errors.Is(err, app.ErrDependencyUnavailable) || strings.Contains(err.Error(), "849") {
		t.Fatalf("error %v", err)
	}
}
