//go:build integration

package openbao

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/testutil/itest"
)

func realClient(t *testing.T) (*Client, itest.Env) {
	t.Helper()
	env := itest.Load()
	env.RequireBaoToken(t)
	c, err := New(Config{Addr: env.OpenBaoAddr, TokenFile: env.OpenBaoTokenFile})
	if err != nil {
		t.Fatal(err)
	}
	return c, env
}

// TestPIINFR01_OpenBaoTransit runs the adapter against the local OpenBao
// (make up; make bao-token).
func TestPIINFR01_OpenBaoTransit(t *testing.T) {
	c, _ := realClient(t)
	ctx := context.Background()
	kc := app.KeyContext{IdentityID: uuid.New(), KeyID: uuid.New()}
	dek := bytes.Repeat([]byte{0x5a}, 32)
	w, err := c.WrapDEK(ctx, kc, dek)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(w.Ciphertext, "vault:v") || w.KEKVersion < 1 || w.KEKName != DefaultKEKName {
		t.Fatalf("wrapped %+v", w)
	}
	got, err := c.UnwrapDEK(ctx, kc, w)
	if err != nil || !bytes.Equal(got, dek) {
		t.Fatalf("unwrap: %v", err)
	}
	// §9 A9: associated data binds the wrapped DEK to identity_id/key_id.
	for name, other := range map[string]app.KeyContext{
		"other identity": {IdentityID: uuid.New(), KeyID: kc.KeyID},
		"other key id":   {IdentityID: kc.IdentityID, KeyID: uuid.New()},
	} {
		if _, err := c.UnwrapDEK(ctx, other, w); !errors.Is(err, app.ErrDataIntegrity) {
			t.Fatalf("%s: want ErrDataIntegrity, got %v", name, err)
		}
	}

	a, err := c.BlindIndex(ctx, []byte("phone_number:+84901234567"))
	if err != nil || len(a.Sum) != 32 || a.KeyVersion != 1 {
		t.Fatalf("hmac: %+v %v", a, err)
	}
	b, _ := c.BlindIndex(ctx, []byte("phone_number:+84901234567"))
	d, _ := c.BlindIndex(ctx, []byte("phone_number:+84901234568"))
	if !bytes.Equal(a.Sum, b.Sum) || bytes.Equal(a.Sum, d.Sum) {
		t.Fatal("blind index must be deterministic and input-dependent")
	}

	ttl, err := c.RenewSelf(ctx)
	if err != nil || ttl <= time.Hour {
		t.Fatalf("renew-self: %v %v", ttl, err)
	}
}

// TestPIINFR05_AppTokenLeastPrivilege: the app token cannot read, rotate or
// export keys, nor use Transit rewrap (removed from the policy, §9 A9).
func TestPIINFR05_AppTokenLeastPrivilege(t *testing.T) {
	c, _ := realClient(t)
	ctx := context.Background()
	for _, path := range []string{
		"transit/keys/identity-pii-kek/rotate",
		"transit/keys/identity-pii-kek/config",
		"transit/rewrap/identity-pii-kek",
		"transit/encrypt/identity-pii-bidx",
		"transit/hmac/identity-pii-kek/sha2-256",
		"transit/export/encryption-key/identity-pii-kek",
	} {
		err := c.call(ctx, "probe", path, map[string]any{"ciphertext": "vault:v1:AAAA", "input": "AAAA"}, nil)
		if !errors.Is(err, app.ErrDependencyUnavailable) || !strings.Contains(err.Error(), "403") {
			t.Fatalf("%s must be denied (403): %v", path, err)
		}
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, c.addr+"/v1/transit/keys/identity-pii-kek", nil)
	tok, _ := c.currentToken("probe")
	req.Header.Set("X-Vault-Token", tok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("read key must be denied: %d", resp.StatusCode)
	}
}

// TestPIINFR06_OpenBaoUnreachable: an unreachable OpenBao is a dependency
// failure (→ 503), never a fallback.
func TestPIINFR06_OpenBaoUnreachable(t *testing.T) {
	env := itest.Load()
	env.RequireBaoToken(t)
	c, err := New(Config{Addr: "http://127.0.0.1:1", TokenFile: env.OpenBaoTokenFile, Timeout: 300 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.WrapDEK(context.Background(), app.KeyContext{IdentityID: uuid.New(), KeyID: uuid.New()}, make([]byte, 32)); !errors.Is(err, app.ErrDependencyUnavailable) {
		t.Fatalf("want ErrDependencyUnavailable, got %v", err)
	}
}
