package openbao

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
)

func TestPLXFR04_LookupKeyPinsKeyVersion(t *testing.T) {
	c, f, _ := setup(t)
	p1, err := c.LookupKey(context.Background(), []byte("login-id/v1\x00email\x00a@example.com"))
	if err != nil {
		t.Fatal(err)
	}
	p2, _ := c.LookupKey(context.Background(), []byte("login-id/v1\x00email\x00a@example.com"))
	p3, _ := c.LookupKey(context.Background(), []byte("login-id/v1\x00email\x00b@example.com"))
	if p1 != p2 || p1 == p3 {
		t.Fatal("lookup key must be deterministic per input")
	}
	if f.hmacBody["key_version"] != float64(LoginHMACKeyVersion) {
		t.Fatalf("key_version not pinned: %v", f.hmacBody["key_version"])
	}
}

func TestPLINFR03_SealOpenBatchBoundToAD(t *testing.T) {
	c, _, _ := setup(t)
	ctx := context.Background()
	ct1, v, err := c.SealLogin(ctx, []byte("ad-1"), []byte("alice@example.com"))
	if err != nil || v != 1 || !strings.HasPrefix(ct1, "vault:v1:") {
		t.Fatalf("seal: %q %d %v", ct1, v, err)
	}
	ct2, _, _ := c.SealLogin(ctx, []byte("ad-2"), []byte("+84901234567"))
	pts, errs, err := c.OpenLogins(ctx, []app.SealedLogin{
		{AD: []byte("ad-1"), Ciphertext: ct1},
		{AD: []byte("ad-1"), Ciphertext: ct2}, // swapped row: wrong AD
		{AD: []byte("ad-2"), Ciphertext: ct2},
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(pts[0]) != "alice@example.com" || string(pts[2]) != "+84901234567" || errs[0] != nil || errs[2] != nil {
		t.Fatalf("batch results %q %v", pts, errs)
	}
	if pts[1] != nil || !errors.Is(errs[1], app.ErrDataIntegrity) {
		t.Fatalf("swapped ciphertext must fail authentication: %v", errs[1])
	}
	if strings.Contains(errs[1].Error(), "alice") {
		t.Fatal("error leaks value")
	}
}

func TestPLINFR06_LoginKeysUnavailable(t *testing.T) {
	c, f, _ := setup(t)
	f.status = http.StatusServiceUnavailable
	if _, err := c.LookupKey(context.Background(), []byte("x")); !errors.Is(err, app.ErrDependencyUnavailable) {
		t.Fatalf("hmac: %v", err)
	}
	if _, _, err := c.OpenLogins(context.Background(), []app.SealedLogin{{AD: []byte("a"), Ciphertext: "vault:v1:x"}}); !errors.Is(err, app.ErrDependencyUnavailable) {
		t.Fatalf("decrypt: %v", err)
	}
}

func TestPLINFR02_LoginKeyNamesDistinct(t *testing.T) {
	file := filepath.Join(t.TempDir(), "t")
	_ = os.WriteFile(file, []byte("x"), 0o600)
	if _, err := New(Config{Addr: "http://x", TokenFile: file, LoginHMACKeyName: DefaultBidxKeyName}); err == nil {
		t.Fatal("login HMAC key must differ from the PII index key")
	}
	if _, err := New(Config{Addr: "http://x", TokenFile: file, LoginKEKName: DefaultLoginHMACKeyName}); err == nil {
		t.Fatal("login KEK must differ from the login HMAC key")
	}
}
