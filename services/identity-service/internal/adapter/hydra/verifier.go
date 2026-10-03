package hydra

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"golang.org/x/sync/singleflight"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/machine"
)

// Token rules (§5, §6 A7, A9).
const (
	MaxTokenBytes = 8 << 10
	Leeway        = 30 * time.Second
	MaxLifetime   = 10 * time.Minute
	maxClientID   = 128
)

// forbiddenHeaders must not appear in the JOSE header (§6 A7): key and
// certificate references, critical extensions, unencoded payloads,
// compression and encryption.
var forbiddenHeaders = []string{"jku", "jwk", "x5u", "x5c", "x5t", "x5t#S256", "crit", "b64", "zip", "enc"}

// forbiddenClaims mark OIDC ID tokens or tokens issued to a user agent; a
// client_credentials access token never has them (§6 B2).
var forbiddenClaims = []string{"nonce", "at_hash", "azp", "c_hash"}

// VerifierConfig configures the token verifier.
type VerifierConfig struct {
	JWKSURL  string
	Issuer   string
	Audience string
	// Admin answers client-status lookups.
	Admin *Admin
	// HTTP is the base client for JWKS fetches (redirects are disabled).
	HTTP            *http.Client
	ClientCacheTTL  time.Duration
	ClientCacheSize int
	// UnknownKidInterval overrides JWKSUnknownKidInterval (tests).
	UnknownKidInterval time.Duration
	Now                func() time.Time
	Log                *slog.Logger
}

// Verifier implements app.MachineTokenVerifier for Hydra client_credentials
// JWT access tokens: offline signature and claim checks plus a cached
// client-status lookup. Errors are app.ErrInvalidToken (with a fixed reason,
// never token contents) or app.ErrDependencyUnavailable.
type Verifier struct {
	issuer   string
	audience string
	admin    *Admin
	keys     *keySet
	cache    *statusCache
	sf       singleflight.Group
	now      func() time.Time
	log      *slog.Logger
}

var _ app.MachineTokenVerifier = (*Verifier)(nil)

// NewVerifier validates the configuration.
func NewVerifier(cfg VerifierConfig) (*Verifier, error) {
	if cfg.JWKSURL == "" || cfg.Issuer == "" || cfg.Audience == "" || cfg.Admin == nil {
		return nil, errors.New("hydra verifier: JWKS URL, issuer, audience and admin client are required")
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Verifier{
		issuer: cfg.Issuer, audience: cfg.Audience, admin: cfg.Admin, now: cfg.Now, log: cfg.Log,
		keys:  newKeySet(cfg.JWKSURL, cfg.HTTP, cfg.Now, cfg.UnknownKidInterval, cfg.Log),
		cache: newStatusCache(cfg.ClientCacheSize, cfg.ClientCacheTTL, cfg.Now),
	}, nil
}

func invalid(reason string) error { return fmt.Errorf("%w: %s", app.ErrInvalidToken, reason) }

// claims are the registered and Hydra claims we read. Wrong JSON types fail
// decoding (→ 401).
type claims struct {
	Issuer    string           `json:"iss"`
	Subject   string           `json:"sub"`
	ClientID  string           `json:"client_id"`
	Audience  jwt.Audience     `json:"aud"`
	Expiry    *jwt.NumericDate `json:"exp"`
	NotBefore *jwt.NumericDate `json:"nbf"`
	IssuedAt  *jwt.NumericDate `json:"iat"`
	ID        string           `json:"jti"`
	Scopes    []string         `json:"scp"`
}

// Verify implements app.MachineTokenVerifier.
func (v *Verifier) Verify(ctx context.Context, token string) (machine.Principal, error) {
	if len(token) > MaxTokenBytes {
		return machine.Principal{}, invalid("token too large")
	}
	kid, err := checkHeader(token)
	if err != nil {
		return machine.Principal{}, err
	}
	key, err := v.keys.key(ctx, kid)
	if err != nil {
		if errors.Is(err, errUnknownKey) {
			return machine.Principal{}, invalid("unknown kid")
		}
		return machine.Principal{}, err
	}
	tok, err := jwt.ParseSigned(token, []jose.SignatureAlgorithm{jose.RS256})
	if err != nil || len(tok.Headers) != 1 || tok.Headers[0].KeyID != kid || tok.Headers[0].Algorithm != string(jose.RS256) {
		return machine.Principal{}, invalid("malformed token")
	}
	var raw map[string]json.RawMessage
	if err := tok.Claims(key, &raw); err != nil {
		return machine.Principal{}, invalid("signature")
	}
	for _, k := range forbiddenClaims {
		if _, ok := raw[k]; ok {
			return machine.Principal{}, invalid("forbidden claim")
		}
	}
	b, _ := json.Marshal(raw)
	var c claims
	if err := json.Unmarshal(b, &c); err != nil {
		return machine.Principal{}, invalid("claim types")
	}
	if err := v.checkClaims(c); err != nil {
		return machine.Principal{}, err
	}
	st, err := v.status(ctx, c.ClientID)
	if err != nil {
		return machine.Principal{}, err
	}
	if !st.ok {
		return machine.Principal{}, invalid("client not active")
	}
	if st.tokensValidAfter > 0 && c.IssuedAt.Time().Unix() < st.tokensValidAfter {
		return machine.Principal{}, invalid("issued before secret rotation")
	}
	granted := machine.KnownScopes(c.Scopes)
	scopes := make([]machine.Scope, 0, len(granted))
	for _, s := range granted {
		if slices.Contains(st.scopes, s) {
			scopes = append(scopes, s)
		}
	}
	return machine.Principal{ClientID: c.ClientID, Scopes: scopes, TokenID: c.ID}, nil
}

// checkHeader validates the compact JWS shape and the protected header
// before any parsing by the JOSE library, and returns the kid.
func checkHeader(token string) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "", invalid("not a compact JWS")
	}
	for _, p := range parts {
		if !isBase64URL(p) {
			return "", invalid("not a compact JWS")
		}
	}
	hb, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", invalid("header encoding")
	}
	var h map[string]json.RawMessage
	if err := json.Unmarshal(hb, &h); err != nil {
		return "", invalid("header json")
	}
	for _, k := range forbiddenHeaders {
		if _, ok := h[k]; ok {
			return "", invalid("forbidden header")
		}
	}
	str := func(k string) (string, bool) {
		var s string
		raw, ok := h[k]
		if !ok || json.Unmarshal(raw, &s) != nil {
			return "", false
		}
		return s, true
	}
	if alg, ok := str("alg"); !ok || alg != string(jose.RS256) {
		return "", invalid("alg")
	}
	kid, ok := str("kid")
	if !ok || kid == "" {
		return "", invalid("kid")
	}
	typ, ok := str("typ")
	if !ok || !(strings.EqualFold(typ, "JWT") || strings.EqualFold(typ, "at+jwt") || strings.EqualFold(typ, "application/at+jwt")) {
		return "", invalid("typ")
	}
	return kid, nil
}

func isBase64URL(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

func (v *Verifier) checkClaims(c claims) error {
	now := v.now()
	switch {
	case c.Issuer != v.issuer:
		return invalid("iss")
	case len(c.Audience) == 0 || !slices.Contains([]string(c.Audience), v.audience):
		return invalid("aud")
	case c.Expiry == nil || c.IssuedAt == nil:
		return invalid("exp/iat missing")
	case now.After(c.Expiry.Time().Add(Leeway)):
		return invalid("expired")
	case c.IssuedAt.Time().After(now.Add(Leeway)):
		return invalid("iat in the future")
	case c.NotBefore != nil && c.NotBefore.Time().After(now.Add(Leeway)):
		return invalid("not yet valid")
	case c.Expiry.Time().Before(c.IssuedAt.Time()) || c.Expiry.Time().Sub(c.IssuedAt.Time()) > MaxLifetime:
		return invalid("lifetime")
	case c.ID == "":
		return invalid("jti")
	case c.ClientID == "" || len(c.ClientID) > maxClientID || c.Subject != c.ClientID:
		return invalid("sub/client_id")
	case len(c.Scopes) == 0:
		return invalid("scp")
	}
	return nil
}

// statusAttempts bounds re-lookups when an invalidation races a lookup.
const statusAttempts = 3

// status returns the cached client status or looks it up (single-flight per
// client). A result is only used if no invalidation happened while it was
// being fetched; otherwise the lookup is repeated (Invalidate also forgets
// the in-flight call, so the retry starts a fresh request). Hydra
// unreachable and nothing cached → ErrDependencyUnavailable.
func (v *Verifier) status(ctx context.Context, clientID string) (clientStatus, error) {
	for range statusAttempts {
		if st, ok := v.cache.get(clientID); ok {
			return st, nil
		}
		gen := v.cache.generation()
		ch := v.sf.DoChan(clientID, func() (any, error) {
			lgen := v.cache.generation()
			lctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), DefaultTimeout)
			defer cancel()
			st, err := v.admin.status(lctx, clientID)
			if err != nil {
				return clientStatus{}, err
			}
			v.cache.putIfFresh(clientID, st, lgen)
			return st, nil
		})
		var r singleflight.Result
		select {
		case <-ctx.Done():
			return clientStatus{}, fmt.Errorf("%w: client status lookup: %v", app.ErrDependencyUnavailable, ctx.Err())
		case r = <-ch:
		}
		if r.Err != nil {
			return clientStatus{}, r.Err
		}
		if v.cache.generation() != gen {
			continue // invalidated while the (possibly shared) lookup ran
		}
		return r.Val.(clientStatus), nil
	}
	return clientStatus{}, fmt.Errorf("%w: client status kept changing during lookup", app.ErrDependencyUnavailable)
}

// Invalidate implements app.MachineTokenVerifier. It forgets any in-flight
// lookup first, so no caller can be served a result fetched before the
// invalidation.
func (v *Verifier) Invalidate(clientID string) {
	v.sf.Forget(clientID)
	v.cache.invalidate(clientID)
}

// RefreshKeys fetches the JWKS now (startup warm-up); a failure keeps the
// current keys.
func (v *Verifier) RefreshKeys(ctx context.Context) error { return v.keys.refresh(ctx) }

// Run refreshes the JWKS every JWKSRefreshEvery until ctx ends (§6 A8).
func (v *Verifier) Run(ctx context.Context) {
	t := time.NewTicker(JWKSRefreshEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			_ = v.keys.refresh(ctx)
		}
	}
}
