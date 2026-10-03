package hydra

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"golang.org/x/sync/singleflight"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
)

// JWKS limits (§6 A8).
const (
	JWKSTimeout            = 2 * time.Second
	JWKSRefreshEvery       = 5 * time.Minute
	JWKSUnknownKidInterval = 10 * time.Second
	maxJWKSBytes           = 64 << 10
	maxJWKSKeys            = 20
	minRSABits             = 2048
)

var errUnknownKey = errors.New("unknown signing key")

// keySet caches the issuer's RS256 verification keys by kid. Refreshes are
// single-flight; a failed refresh keeps the current keys. Refresh on an
// unknown kid happens at most once per minInterval (resists kid flooding).
// Keys are only ever looked up by kid; the set is never tried key by key.
type keySet struct {
	url         string
	http        *http.Client
	now         func() time.Time
	minInterval time.Duration
	log         *slog.Logger

	mu          sync.RWMutex
	keys        map[string]*rsa.PublicKey
	lastAttempt time.Time
	sf          singleflight.Group
}

func newKeySet(u string, hc *http.Client, now func() time.Time, minInterval time.Duration, log *slog.Logger) *keySet {
	if minInterval <= 0 {
		minInterval = JWKSUnknownKidInterval
	}
	return &keySet{url: u, http: newHTTPClient(hc, JWKSTimeout), now: now, minInterval: minInterval, log: log,
		keys: map[string]*rsa.PublicKey{}}
}

// key returns the key for kid, refreshing the set (rate-limited) when the
// kid is unknown. errUnknownKey → 401; ErrDependencyUnavailable when there
// is no usable key set at all or the refresh failed (→ 503).
func (k *keySet) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	k.mu.RLock()
	pk, ok := k.keys[kid]
	have := len(k.keys) > 0
	recent := !k.lastAttempt.IsZero() && k.now().Sub(k.lastAttempt) < k.minInterval
	k.mu.RUnlock()
	if ok {
		return pk, nil
	}
	if recent {
		if !have {
			return nil, fmt.Errorf("%w: no verification keys", app.ErrDependencyUnavailable)
		}
		return nil, errUnknownKey
	}
	if err := k.refresh(ctx); err != nil {
		return nil, err
	}
	k.mu.RLock()
	pk, ok = k.keys[kid]
	k.mu.RUnlock()
	if !ok {
		return nil, errUnknownKey
	}
	return pk, nil
}

// refresh fetches the JWKS once for all concurrent callers. The fetch is
// detached from the caller's cancellation and bounded by JWKSTimeout.
func (k *keySet) refresh(ctx context.Context) error {
	_, err, _ := k.sf.Do("jwks", func() (any, error) {
		k.mu.Lock()
		k.lastAttempt = k.now()
		k.mu.Unlock()
		fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), JWKSTimeout)
		defer cancel()
		keys, err := k.fetch(fctx)
		if err != nil {
			if k.log != nil {
				k.log.WarnContext(ctx, "jwks refresh failed; keeping current keys", "error", err.Error())
			}
			return nil, err
		}
		k.mu.Lock()
		k.keys = keys
		k.mu.Unlock()
		return nil, nil
	})
	return err
}

func (k *keySet) fetch(ctx context.Context) (map[string]*rsa.PublicKey, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, k.url, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: jwks: build request", app.ErrDependencyUnavailable)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := k.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: jwks: transport error", app.ErrDependencyUnavailable)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: jwks: status %d", app.ErrDependencyUnavailable, resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxJWKSBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%w: jwks: read", app.ErrDependencyUnavailable)
	}
	if len(b) > maxJWKSBytes {
		return nil, fmt.Errorf("%w: jwks: larger than %d bytes", app.ErrDependencyUnavailable, maxJWKSBytes)
	}
	return parseJWKS(b)
}

// parseJWKS keeps the usable keys: RSA public keys ≥ 2048 bits with a kid,
// use "sig" or absent, alg "RS256" or absent (§6 A7). Malformed or unusable
// entries are skipped; more than maxJWKSKeys entries or no usable key fails.
func parseJWKS(b []byte) (map[string]*rsa.PublicKey, error) {
	var set struct {
		Keys []json.RawMessage `json:"keys"`
	}
	if err := json.Unmarshal(b, &set); err != nil {
		return nil, fmt.Errorf("%w: jwks: malformed", app.ErrDependencyUnavailable)
	}
	if len(set.Keys) > maxJWKSKeys {
		return nil, fmt.Errorf("%w: jwks: more than %d keys", app.ErrDependencyUnavailable, maxJWKSKeys)
	}
	out := map[string]*rsa.PublicKey{}
	for _, raw := range set.Keys {
		var jwk jose.JSONWebKey
		if err := jwk.UnmarshalJSON(raw); err != nil {
			continue
		}
		if jwk.KeyID == "" || (jwk.Use != "" && jwk.Use != "sig") || (jwk.Algorithm != "" && jwk.Algorithm != string(jose.RS256)) {
			continue
		}
		pk, ok := jwk.Key.(*rsa.PublicKey)
		if !ok || pk.N.BitLen() < minRSABits {
			continue
		}
		if _, dup := out[jwk.KeyID]; dup {
			// Ambiguous kid: use neither.
			out[jwk.KeyID] = nil
			continue
		}
		out[jwk.KeyID] = pk
	}
	for kid, pk := range out {
		if pk == nil {
			delete(out, kid)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: jwks: no usable RS256 key", app.ErrDependencyUnavailable)
	}
	return out, nil
}
