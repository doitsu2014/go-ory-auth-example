//go:build integration

package hydra_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/adapter/hydra"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/machine"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/testutil/itest"
)

const audience = "identity-service"

func setup(t *testing.T) (itest.Env, *hydra.Admin, *hydra.Verifier) {
	t.Helper()
	env := itest.Load()
	admin, err := hydra.NewAdmin(env.HydraAdmin, audience, env.ClientTagKey, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := admin.Ready(context.Background()); err != nil {
		t.Fatalf("hydra admin not reachable at %s (make infra-up): %v", env.HydraAdmin, err)
	}
	v, err := hydra.NewVerifier(hydra.VerifierConfig{JWKSURL: env.JWKSURL(), Issuer: env.HydraIssuer, Audience: audience, Admin: admin})
	if err != nil {
		t.Fatal(err)
	}
	return env, admin, v
}

func register(t *testing.T, admin *hydra.Admin, scopes ...string) (machine.ServiceClient, string) {
	t.Helper()
	reg, err := machine.NewRegistration("itest-"+uuid.NewString()[:8], "ops@example.com", scopes)
	if err != nil {
		t.Fatal(err)
	}
	c, secret, err := admin.Create(context.Background(), app.NewServiceClient{ClientID: uuid.NewString(), Registration: reg, CreatedBy: uuid.New()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Delete(context.Background(), c.ClientID) })
	return c, secret
}

func TestHydraAdminLifecycle(t *testing.T) {
	env, admin, v := setup(t)
	ctx := context.Background()
	c, secret := register(t, admin, "customers:read", "audit:read")
	if secret == "" || c.ClientID == "" || !slices.Equal(c.Scopes, []machine.Scope{machine.ScopeCustomersRead, machine.ScopeAuditRead}) {
		t.Fatalf("create: %+v", c)
	}
	got, err := admin.Get(ctx, c.ClientID)
	if err != nil || got.Name != c.Name || got.Owner != "ops@example.com" || got.CreatedBy == nil {
		t.Fatalf("get: %+v %v", got, err)
	}
	list, err := admin.List(ctx)
	if err != nil || !slices.ContainsFunc(list, func(x machine.ServiceClient) bool { return x.ClientID == c.ClientID }) {
		t.Fatalf("list: %v", err)
	}

	st, tok := env.ClientCredentialsToken(t, c.ClientID, secret, "customers:read", audience)
	if st != 200 {
		t.Fatalf("token: %d", st)
	}
	p, err := v.Verify(ctx, tok)
	if err != nil || p.ClientID != c.ClientID || !slices.Equal(p.Scopes, []machine.Scope{machine.ScopeCustomersRead}) || p.TokenID == "" {
		t.Fatalf("verify real Hydra token: %+v %v", p, err)
	}

	// Rotation: old secret dead at the token endpoint, old token dead here —
	// even though the token was issued in the same second (ceil(now)+1).
	if err := admin.SetSecret(ctx, c.ClientID, "rotated-"+uuid.NewString(), app.TokensValidAfter(time.Now())); err != nil {
		t.Fatal(err)
	}
	v.Invalidate(c.ClientID)
	if st, _ := env.ClientCredentialsToken(t, c.ClientID, secret, "customers:read", audience); st != 401 {
		t.Fatalf("old secret at token endpoint: %d", st)
	}
	if _, err := v.Verify(ctx, tok); !errors.Is(err, app.ErrInvalidToken) {
		t.Fatalf("token issued before rotation: %v", err)
	}

	if err := admin.Delete(ctx, c.ClientID); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Get(ctx, c.ClientID); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("deleted: %v", err)
	}
	if err := admin.Delete(ctx, c.ClientID); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("second delete: %v", err)
	}
}

func TestHydraTokenRules(t *testing.T) {
	env, admin, v := setup(t)
	ctx := context.Background()
	c, secret := register(t, admin, "customers:read")
	// Spike fact: without the audience parameter Hydra issues aud: [].
	st, noAud := env.ClientCredentialsToken(t, c.ClientID, secret, "customers:read", "")
	if st != 200 {
		t.Fatalf("token without audience: %d", st)
	}
	if _, err := v.Verify(ctx, noAud); !errors.Is(err, app.ErrInvalidToken) {
		t.Fatalf("token without audience must be rejected: %v", err)
	}
	if st, _ := env.ClientCredentialsToken(t, c.ClientID, secret, "audit:read", audience); st != 400 {
		t.Fatalf("scope outside the registration: %d", st)
	}
	// Issuer is matched exactly (a trailing slash is a different issuer).
	strict, err := hydra.NewVerifier(hydra.VerifierConfig{JWKSURL: env.JWKSURL(), Issuer: env.HydraIssuer + "/", Audience: audience, Admin: admin})
	if err != nil {
		t.Fatal(err)
	}
	_, tok := env.ClientCredentialsToken(t, c.ClientID, secret, "customers:read", audience)
	if _, err := strict.Verify(ctx, tok); !errors.Is(err, app.ErrInvalidToken) {
		t.Fatalf("issuer with trailing slash: %v", err)
	}
}

// TestHydraUnmanagedClientIsRejected: a client created directly in Hydra
// (not by identity-service) gets valid, correctly signed tokens for our
// audience — the verifier must still refuse them (§6 B2).
func TestHydraUnmanagedClientIsRejected(t *testing.T) {
	env, _, v := setup(t)
	var out struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	}
	// Security S4: forging managed_by (and even a made-up integrity value)
	// is not enough without M2M_CLIENT_TAG_KEY.
	for name, md := range map[string]any{
		"no metadata":       nil,
		"forged managed_by": map[string]any{"managed_by": "identity-service", "owner": "x@example.com", "created_by": uuid.Nil.String()},
		"forged integrity":  map[string]any{"managed_by": "identity-service", "created_by": uuid.Nil.String(), "integrity": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"},
	} {
		body := map[string]any{
			"client_name": "foreign", "grant_types": []string{"client_credentials"}, "response_types": []string{"token"},
			"scope": "customers:read", "audience": []string{audience}, "token_endpoint_auth_method": "client_secret_basic",
			"access_token_strategy": "jwt",
		}
		if md != nil {
			body["metadata"] = md
		}
		st, _ := itest.JSON(t, nil, "POST", env.HydraAdmin+"/admin/clients", nil, body, &out)
		if st != 201 {
			t.Fatalf("%s: create foreign client: %d", name, st)
		}
		id := out.ClientID
		t.Cleanup(func() { env.DeleteHydraClient(t, id) })
		st, tok := env.ClientCredentialsToken(t, out.ClientID, out.ClientSecret, "customers:read", audience)
		if st != 200 {
			t.Fatalf("%s: token: %d", name, st)
		}
		if _, err := v.Verify(context.Background(), tok); !errors.Is(err, app.ErrInvalidToken) {
			t.Fatalf("%s: unmanaged client token must be rejected: %v", name, err)
		}
	}
}
