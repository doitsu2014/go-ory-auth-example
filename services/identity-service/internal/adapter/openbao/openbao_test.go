package openbao

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
)

// fakeTransit mimics the OpenBao Transit endpoints used by the adapter.
type fakeTransit struct {
	mu        sync.Mutex
	token     string
	version   int
	stored    map[string][2]string // ciphertext -> {plaintext, associated_data}
	status    int                  // forced status (0 = normal)
	hmacBody  map[string]any
	lastToken string
}

func (f *fakeTransit) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastToken = r.Header.Get("X-Vault-Token")
	if f.status != 0 {
		w.WriteHeader(f.status)
		_, _ = w.Write([]byte(`{"errors":["forced: secret-body-text"]}`))
		return
	}
	if f.lastToken != f.token {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"errors":["permission denied"]}`))
		return
	}
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	reply := func(v any) { _ = json.NewEncoder(w).Encode(v) }
	switch r.URL.Path {
	case "/v1/transit/encrypt/identity-pii-kek":
		ct := "vault:v" + string(rune('0'+f.version)) + ":" + base64.StdEncoding.EncodeToString([]byte(uuid.NewString()))
		f.stored[ct] = [2]string{body["plaintext"].(string), body["associated_data"].(string)}
		reply(map[string]any{"data": map[string]any{"ciphertext": ct, "key_version": f.version}})
	case "/v1/transit/decrypt/identity-pii-kek":
		s, ok := f.stored[body["ciphertext"].(string)]
		if !ok || s[1] != body["associated_data"] {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"errors":["cipher: message authentication failed"]}`))
			return
		}
		reply(map[string]any{"data": map[string]any{"plaintext": s[0]}})
	case "/v1/transit/hmac/identity-pii-bidx/sha2-256":
		f.hmacBody = body
		sum := bytes.Repeat([]byte{0xab}, 32)
		reply(map[string]any{"data": map[string]any{"hmac": "vault:v1:" + base64.StdEncoding.EncodeToString(sum)}})
	case "/v1/auth/token/renew-self":
		reply(map[string]any{"auth": map[string]any{"lease_duration": 86400}})
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func setup(t *testing.T) (*Client, *fakeTransit, string) {
	t.Helper()
	f := &fakeTransit{token: "s.first-token", version: 1, stored: map[string][2]string{}}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	file := filepath.Join(t.TempDir(), "identity-service.token")
	if err := os.WriteFile(file, []byte(f.token+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := New(Config{Addr: srv.URL, TokenFile: file})
	if err != nil {
		t.Fatal(err)
	}
	return c, f, file
}

func TestOpenBao_WrapUnwrapWithAssociatedData(t *testing.T) {
	c, f, _ := setup(t)
	ctx := context.Background()
	kc := app.KeyContext{IdentityID: uuid.New(), KeyID: uuid.New()}
	dek := bytes.Repeat([]byte{0x42}, 32)
	w, err := c.WrapDEK(ctx, kc, dek)
	if err != nil || w.KEKVersion != 1 || w.KEKName != DefaultKEKName || !strings.HasPrefix(w.Ciphertext, "vault:v1:") {
		t.Fatalf("wrap: %+v %v", w, err)
	}
	if f.lastToken != "s.first-token" {
		t.Fatalf("token header %q", f.lastToken)
	}
	got, err := c.UnwrapDEK(ctx, kc, w)
	if err != nil || !bytes.Equal(got, dek) {
		t.Fatalf("unwrap: %v", err)
	}
	// §9 A9: a wrapped DEK copied to another subject fails authentication.
	_, err = c.UnwrapDEK(ctx, app.KeyContext{IdentityID: uuid.New(), KeyID: kc.KeyID}, w)
	if !errors.Is(err, app.ErrDataIntegrity) || errors.Is(err, app.ErrDependencyUnavailable) {
		t.Fatalf("wrong AD: %v", err)
	}
	if _, err := c.UnwrapDEK(ctx, kc, app.Wrapped{Ciphertext: w.Ciphertext, KEKName: "../sys/raw"}); !errors.Is(err, app.ErrDataIntegrity) {
		t.Fatalf("path injection via kek name: %v", err)
	}
}

func TestOpenBao_BlindIndexPinsKeyVersion(t *testing.T) {
	c, f, _ := setup(t)
	b, err := c.BlindIndex(context.Background(), []byte("phone_number:+84901234567"))
	if err != nil || len(b.Sum) != 32 || b.KeyVersion != 1 {
		t.Fatalf("hmac: %+v %v", b, err)
	}
	if f.hmacBody["key_version"] != float64(1) || f.hmacBody["input"] != base64.StdEncoding.EncodeToString([]byte("phone_number:+84901234567")) {
		t.Fatalf("hmac request %v", f.hmacBody)
	}
}

func TestOpenBao_TokenFileReReadOnChange(t *testing.T) {
	c, f, file := setup(t)
	ctx := context.Background()
	if _, err := c.RenewSelf(ctx); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.token = "s.second-token-longer"
	f.mu.Unlock()
	if err := os.WriteFile(file, []byte("s.second-token-longer"), 0o600); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(2 * time.Second)
	_ = os.Chtimes(file, future, future)
	ttl, err := c.RenewSelf(ctx)
	if err != nil || ttl != 24*time.Hour || f.lastToken != "s.second-token-longer" {
		t.Fatalf("renew with new token: %v %v %q", ttl, err, f.lastToken)
	}
}

func TestOpenBao_FailuresAreValueFree(t *testing.T) {
	c, f, file := setup(t)
	ctx := context.Background()
	kc := app.KeyContext{IdentityID: uuid.New(), KeyID: uuid.New()}
	dek := []byte("0123456789abcdef0123456789abcdef")
	check := func(name string, err error) {
		t.Helper()
		if !errors.Is(err, app.ErrDependencyUnavailable) {
			t.Fatalf("%s: want ErrDependencyUnavailable, got %v", name, err)
		}
		for _, s := range []string{"secret-body-text", "s.first-token", string(dek), base64.StdEncoding.EncodeToString(dek)} {
			if strings.Contains(err.Error(), s) {
				t.Fatalf("%s: error leaks %q: %v", name, s, err)
			}
		}
	}
	for _, st := range []int{http.StatusServiceUnavailable, http.StatusInternalServerError, http.StatusTooManyRequests, http.StatusForbidden} {
		f.mu.Lock()
		f.status = st
		f.mu.Unlock()
		_, err := c.WrapDEK(ctx, kc, dek)
		check(http.StatusText(st), err)
	}
	f.mu.Lock()
	f.status = 0
	f.mu.Unlock()

	_ = os.Remove(file)
	c.forgetToken()
	_, err := c.WrapDEK(ctx, kc, dek)
	check("missing token file", err)

	down, _ := New(Config{Addr: "http://127.0.0.1:1", TokenFile: file, Timeout: 200 * time.Millisecond})
	_ = os.WriteFile(file, []byte("s.first-token"), 0o600)
	_, err = down.BlindIndex(ctx, []byte("phone_number:+84901234567"))
	check("unreachable", err)
	if strings.Contains(err.Error(), "+84901234567") {
		t.Fatal("blind index input leaked")
	}
}

func TestOpenBao_ConfigValidation(t *testing.T) {
	if _, err := New(Config{Addr: "http://x", TokenFile: "/t", KEKName: "a/b"}); err == nil {
		t.Fatal("invalid key name accepted")
	}
	if _, err := New(Config{TokenFile: "/t"}); err == nil {
		t.Fatal("missing address accepted")
	}
}

// Review #11: redirects are never followed (the token must not leave Addr)
// and map to ErrDependencyUnavailable.
func TestReview11_RedirectNotFollowed(t *testing.T) {
	var followed atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		followed.Add(1)
	}))
	defer target.Close()
	redir := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	defer redir.Close()
	file := filepath.Join(t.TempDir(), "token")
	_ = os.WriteFile(file, []byte("s.tok"), 0o600)
	for _, hc := range []*http.Client{nil, {}} {
		c, err := New(Config{Addr: redir.URL, TokenFile: file, HTTP: hc})
		if err != nil {
			t.Fatal(err)
		}
		_, err = c.BlindIndex(context.Background(), []byte("x"))
		if !errors.Is(err, app.ErrDependencyUnavailable) || followed.Load() != 0 {
			t.Fatalf("redirect: %v followed=%d", err, followed.Load())
		}
	}
}

// Review #11: only an authentication-failure 400 on decrypt is an integrity
// error; any other 400 is a dependency failure.
func TestReview11_Decrypt400Classification(t *testing.T) {
	for body, wantIntegrity := range map[string]bool{
		`{"errors":["cipher: message authentication failed"]}`: true,
		`{"errors":["invalid ciphertext: no prefix"]}`:         true,
		`{"errors":["missing client token"]}`:                  false,
		`{"errors":["unsupported path"]}`:                      false,
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(body))
		}))
		file := filepath.Join(t.TempDir(), "token")
		_ = os.WriteFile(file, []byte("s.tok"), 0o600)
		c, _ := New(Config{Addr: srv.URL, TokenFile: file})
		_, err := c.UnwrapDEK(context.Background(), app.KeyContext{IdentityID: uuid.New(), KeyID: uuid.New()}, app.Wrapped{Ciphertext: "vault:v1:AAAA"})
		srv.Close()
		if errors.Is(err, app.ErrDataIntegrity) != wantIntegrity || errors.Is(err, app.ErrDependencyUnavailable) == wantIntegrity {
			t.Fatalf("%s: %v", body, err)
		}
		if strings.Contains(err.Error(), "errors") {
			t.Fatalf("body leaked into error: %v", err)
		}
	}
}

// Review #12: PII_OPENBAO_CA_FILE trusts a private CA.
func TestReview12_CustomCAFile(t *testing.T) {
	f := &fakeTransit{token: "s.tok", version: 1, stored: map[string][2]string{}}
	srv := httptest.NewTLSServer(f)
	defer srv.Close()
	dir := t.TempDir()
	tokenFile := filepath.Join(dir, "token")
	_ = os.WriteFile(tokenFile, []byte("s.tok"), 0o600)
	ca := filepath.Join(dir, "ca.pem")
	_ = os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600)

	untrusted, _ := New(Config{Addr: srv.URL, TokenFile: tokenFile})
	if _, err := untrusted.RenewSelf(context.Background()); !errors.Is(err, app.ErrDependencyUnavailable) {
		t.Fatalf("unknown CA must fail: %v", err)
	}
	trusted, err := New(Config{Addr: srv.URL, TokenFile: tokenFile, CAFile: ca})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := trusted.RenewSelf(context.Background()); err != nil {
		t.Fatalf("custom CA: %v", err)
	}
	bad := filepath.Join(dir, "bad.pem")
	_ = os.WriteFile(bad, []byte("not pem"), 0o600)
	if _, err := New(Config{Addr: srv.URL, TokenFile: tokenFile, CAFile: bad}); err == nil {
		t.Fatal("non-PEM CA file accepted")
	}
}
