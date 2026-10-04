//go:build integration

package openbao

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/login"
)

// TestPLINFR02_LoginKeysAgainstOpenBao: pseudonym HMAC (pinned v1) and the
// login vault AEAD work with the app token; AD binds kind and pseudonym.
func TestPLINFR02_LoginKeysAgainstOpenBao(t *testing.T) {
	c, _ := realClient(t)
	ctx := context.Background()
	id, _ := login.Parse(login.KindEmail, "openbao-itest@example.com", login.PhonePolicy{})
	p1, err := c.Pseudonym(ctx, login.PseudonymInput(id))
	if err != nil {
		t.Fatal(err)
	}
	p2, _ := c.Pseudonym(ctx, login.PseudonymInput(id))
	if p1 != p2 || !login.IsPseudonym(p1.String()) {
		t.Fatal("pseudonym must be deterministic and well-formed")
	}
	ad := login.AAD(id.Kind(), p1)
	ct, v, err := c.SealLogin(ctx, ad, []byte(id.Value()))
	if err != nil || v < 1 || !strings.HasPrefix(ct, "vault:v") {
		t.Fatalf("seal: %v", err)
	}
	pts, errs, err := c.OpenLogins(ctx, []app.SealedLogin{{AD: ad, Ciphertext: ct}, {AD: login.AAD(login.KindPhone, p1), Ciphertext: ct}})
	if err != nil || string(pts[0]) != id.Value() || errs[0] != nil || !errors.Is(errs[1], app.ErrDataIntegrity) {
		t.Fatalf("open batch: %v %v", errs, err)
	}
}

// TestPLINFR02_LoginKeysLeastPrivilege: the app token cannot read, rotate,
// configure or export the login keys, nor use them for other operations.
func TestPLINFR02_LoginKeysLeastPrivilege(t *testing.T) {
	c, _ := realClient(t)
	ctx := context.Background()
	for _, path := range []string{
		"transit/keys/identity-login-pseudonym/rotate",
		"transit/keys/identity-login-pseudonym/config",
		"transit/keys/identity-login-kek/rotate",
		"transit/keys/identity-login-kek/config",
		"transit/export/hmac-key/identity-login-pseudonym",
		"transit/encrypt/identity-login-pseudonym",
		"transit/hmac/identity-login-kek/sha2-256",
		"transit/rewrap/identity-login-kek",
	} {
		err := c.call(ctx, "probe", path, map[string]any{"ciphertext": "vault:v1:AAAA", "input": "AAAA", "plaintext": "AAAA"}, nil)
		if !errors.Is(err, app.ErrDependencyUnavailable) || !strings.Contains(err.Error(), "403") {
			t.Fatalf("%s must be denied (403): %v", path, err)
		}
	}
}
