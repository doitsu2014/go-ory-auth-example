package kratos

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/identity"
)

func sessionJSON(id uuid.UUID, schema, aal string, expires time.Time, verified bool) string {
	return fmt.Sprintf(`{"id":"%s","active":true,"expires_at":"%s","authenticated_at":"%s",
	"authenticator_assurance_level":"%s","identity":{"id":"%s","schema_id":"%s","state":"active",
	"traits":{"email":"a@example.com","name":{"first":"A"}},
	"verifiable_addresses":[{"value":"a@example.com","verified":%v,"via":"email"}],
	"created_at":"2026-10-01T00:00:00Z"}}`,
		uuid.NewString(), expires.Format(time.RFC3339Nano), time.Now().Add(-time.Minute).Format(time.RFC3339Nano),
		aal, id, schema, verified)
}

// TestFR08_WhoamiMapping covers the verifier's status mapping.
func TestFR08_WhoamiMapping(t *testing.T) {
	id := uuid.New()
	tests := []struct {
		name    string
		handler http.HandlerFunc
		want    error
		kind    identity.Kind
	}{
		{name: "200 customer", kind: identity.KindCustomer, handler: func(w http.ResponseWriter, _ *http.Request) {
			_, _ = fmt.Fprint(w, sessionJSON(id, "customer", "aal1", time.Now().Add(time.Hour), true))
		}},
		{name: "401", want: app.ErrUnauthenticated, handler: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(401)
			_, _ = fmt.Fprint(w, `{"error":{"code":401,"id":"session_inactive"}}`)
		}},
		{name: "403 aal2", want: app.ErrAAL2Required, handler: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(403)
			_, _ = fmt.Fprint(w, `{"error":{"code":403,"id":"session_aal2_required"}}`)
		}},
		{name: "403 other", want: app.ErrUnauthenticated, handler: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(403)
		}},
		{name: "500", want: app.ErrDependencyUnavailable, handler: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(500)
		}},
		{name: "timeout", want: app.ErrDependencyUnavailable, handler: func(w http.ResponseWriter, _ *http.Request) {
			time.Sleep(300 * time.Millisecond)
		}},
		{name: "unknown schema", want: app.ErrForbidden, handler: func(w http.ResponseWriter, _ *http.Request) {
			_, _ = fmt.Fprint(w, sessionJSON(id, "robot", "aal1", time.Now().Add(time.Hour), true))
		}},
		{name: "expired", want: app.ErrUnauthenticated, handler: func(w http.ResponseWriter, _ *http.Request) {
			_, _ = fmt.Fprint(w, sessionJSON(id, "customer", "aal1", time.Now().Add(-time.Second), true))
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(tt.handler)
			defer srv.Close()
			v := NewSessionVerifier(srv.URL, &http.Client{Timeout: 100 * time.Millisecond}, nil)
			p, err := v.Verify(context.Background(), app.Credential{Kind: app.CredentialToken, Value: "tok"})
			if tt.want != nil {
				if !errors.Is(err, tt.want) {
					t.Fatalf("want %v got %v", tt.want, err)
				}
				return
			}
			if err != nil || p.Kind != tt.kind || p.IdentityID != id || !p.EmailVerified || p.Name.First != "A" {
				t.Fatalf("principal %+v err %v", p, err)
			}
		})
	}
}

func TestFR08_CredentialForwardingAndCache(t *testing.T) {
	id := uuid.New()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		switch {
		case r.Header.Get("X-Session-Token") == "tok":
		case r.Header.Get("Cookie") == "ory_kratos_session=cookieval":
		default:
			w.WriteHeader(401)
			return
		}
		_, _ = fmt.Fprint(w, sessionJSON(id, "admin", "aal2", time.Now().Add(time.Hour), true))
	}))
	defer srv.Close()
	v := NewSessionVerifier(srv.URL, nil, NewSessionCache(10, nil))
	ctx := context.Background()
	for range 3 {
		if _, err := v.Verify(ctx, app.Credential{Kind: app.CredentialToken, Value: "tok"}); err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("cache miss: %d calls", calls.Load())
	}
	if _, err := v.Verify(ctx, app.Credential{Kind: app.CredentialCookie, Value: "cookieval"}); err != nil {
		t.Fatal(err)
	}
	// Same value as a different credential kind is a different cache key.
	if _, err := v.Verify(ctx, app.Credential{Kind: app.CredentialCookie, Value: "tok"}); !errors.Is(err, app.ErrUnauthenticated) {
		t.Fatalf("kind must be part of the key: %v", err)
	}
	// Negative results are never cached.
	before := calls.Load()
	for range 2 {
		_, _ = v.Verify(ctx, app.Credential{Kind: app.CredentialToken, Value: "bad"})
	}
	if calls.Load() != before+2 {
		t.Fatal("negative result was cached")
	}
	v.Invalidate(id)
	if _, err := v.Verify(ctx, app.Credential{Kind: app.CredentialToken, Value: "tok"}); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != before+3 {
		t.Fatal("Invalidate did not purge")
	}
}

func TestSessionCacheTTL(t *testing.T) {
	cur := time.Date(2026, 10, 3, 8, 0, 0, 0, time.UTC)
	c := NewSessionCache(10, func() time.Time { return cur })
	cred := app.Credential{Kind: app.CredentialToken, Value: "t"}
	id := uuid.New()

	c.Put(cred, identity.Principal{IdentityID: id, ExpiresAt: cur.Add(10 * time.Second)})
	if _, ok := c.Get(cred); !ok {
		t.Fatal("want hit")
	}
	cur = cur.Add(11 * time.Second) // past expires_at (< 30 s)
	if _, ok := c.Get(cred); ok {
		t.Fatal("entry must expire at session expires_at")
	}
	c.Put(cred, identity.Principal{IdentityID: id, ExpiresAt: cur.Add(time.Hour)})
	cur = cur.Add(31 * time.Second) // past the 30 s cap
	if _, ok := c.Get(cred); ok {
		t.Fatal("entry must expire after 30 s")
	}
	c.Put(cred, identity.Principal{IdentityID: id, ExpiresAt: cur.Add(-time.Second)})
	if c.Len() != 0 {
		t.Fatal("expired session must not be cached")
	}
	c.Put(cred, identity.Principal{IdentityID: id, ExpiresAt: cur.Add(time.Hour)})
	c.Put(app.Credential{Kind: app.CredentialCookie, Value: "x"}, identity.Principal{IdentityID: uuid.New(), ExpiresAt: cur.Add(time.Hour)})
	c.Invalidate(id)
	if _, ok := c.Get(cred); ok || c.Len() != 1 {
		t.Fatal("Invalidate must remove only that identity")
	}
}

func TestNextPageToken(t *testing.T) {
	link := `</admin/identities?page_size=1&page_token=00000000-0000-0000-0000-000000000000>; rel="first",</admin/identities?page_size=1&page_token=611cd0d1-426d-4e21-bc40-f3b8a69780bd>; rel="next"`
	tok := nextToken(link)
	if tok == "" || tok == "611cd0d1-426d-4e21-bc40-f3b8a69780bd" {
		t.Fatalf("want opaque token, got %q", tok)
	}
	if nextToken(`</x?page_token=a>; rel="first"`) != "" {
		t.Fatal("no next link")
	}
}

func TestFR11_InvalidateDuringWhoamiIsNotRepopulated(t *testing.T) {
	c := NewSessionCache(10, nil)
	id := uuid.New()
	cred := app.Credential{Kind: app.CredentialToken, Value: "t"}
	p := identity.Principal{IdentityID: id, ExpiresAt: time.Now().Add(time.Hour)}
	start := c.Generation() // whoami starts
	c.Invalidate(id)        // admin disables the customer meanwhile
	c.PutIfFresh(cred, p, start)
	if _, ok := c.Get(cred); ok {
		t.Fatal("stale whoami result repopulated the cache after Invalidate")
	}
	other := identity.Principal{IdentityID: uuid.New(), ExpiresAt: time.Now().Add(time.Hour)}
	c.PutIfFresh(app.Credential{Kind: app.CredentialToken, Value: "o"}, other, start)
	if c.Len() != 1 {
		t.Fatal("other identities must still be cached")
	}
	c.PutIfFresh(cred, p, c.Generation())
	if _, ok := c.Get(cred); !ok {
		t.Fatal("a whoami started after Invalidate may cache")
	}
}

func TestVerifierDropsResultInvalidatedMidFlight(t *testing.T) {
	id := uuid.New()
	var v *SessionVerifier
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		v.Invalidate(id) // revocation lands while whoami is in flight
		_, _ = fmt.Fprint(w, sessionJSON(id, "customer", "aal1", time.Now().Add(time.Hour), true))
	}))
	defer srv.Close()
	cache := NewSessionCache(10, nil)
	v = NewSessionVerifier(srv.URL, nil, cache)
	if _, err := v.Verify(context.Background(), app.Credential{Kind: app.CredentialToken, Value: "x"}); err != nil {
		t.Fatal(err)
	}
	if cache.Len() != 0 {
		t.Fatal("result must not be cached")
	}
}
