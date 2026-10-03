package hydra

import (
	"context"
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/machine"
)

const (
	testIssuer   = "http://localhost:4444"
	testAudience = "identity-service"
	testClient   = "6f1c1a52-6a43-4d6a-9d0e-6f3f2b1b9a10"
)

var (
	keysOnce sync.Once
	keyA     *rsa.PrivateKey
	keyB     *rsa.PrivateKey
)

func testKeys(t *testing.T) (*rsa.PrivateKey, *rsa.PrivateKey) {
	t.Helper()
	keysOnce.Do(func() {
		var err error
		if keyA, err = rsa.GenerateKey(rand.Reader, 2048); err != nil {
			panic(err)
		}
		if keyB, err = rsa.GenerateKey(rand.Reader, 2048); err != nil {
			panic(err)
		}
	})
	return keyA, keyB
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func jwkJSON(kid string, pub *rsa.PublicKey, extra map[string]any) map[string]any {
	m := map[string]any{
		"kty": "RSA", "kid": kid, "use": "sig", "alg": "RS256",
		"n": b64(pub.N.Bytes()), "e": b64(big.NewInt(int64(pub.E)).Bytes()),
	}
	for k, v := range extra {
		if v == nil {
			delete(m, k)
		} else {
			m[k] = v
		}
	}
	return m
}

// clock is a settable time source.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) add(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

type hydraFake struct {
	mu         sync.Mutex
	jwks       []map[string]any
	jwksStatus int
	clients    map[string]map[string]any
	adminDown  bool
	// adminStatus, when set, is returned for every admin request.
	adminStatus int
	// block, when set, holds admin responses (after the state snapshot)
	// until it is closed.
	block     chan struct{}
	jwksHits  atomic.Int64
	adminHits atomic.Int64
	jwksSrv   *httptest.Server
	adminSrv  *httptest.Server
}

func newHydraFake(t *testing.T) *hydraFake {
	f := &hydraFake{clients: map[string]map[string]any{}, jwksStatus: 200}
	f.jwksSrv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.jwksHits.Add(1)
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.jwksStatus != 200 {
			w.WriteHeader(f.jwksStatus)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": f.jwks})
	}))
	f.adminSrv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		down, status, block := f.adminDown, f.adminStatus, f.block
		id := strings.TrimPrefix(r.URL.Path, "/admin/clients/")
		c, ok := f.clients[id]
		f.mu.Unlock()
		f.adminHits.Add(1)
		if block != nil {
			<-block
		}
		if down {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		if status != 0 {
			w.WriteHeader(status)
			return
		}
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"Unable to locate the resource"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(c)
	}))
	t.Cleanup(f.jwksSrv.Close)
	t.Cleanup(f.adminSrv.Close)
	return f
}

func (f *hydraFake) set(fn func(f *hydraFake)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f)
}

func managedClientJSON(id string, mutate func(m map[string]any)) map[string]any {
	m := map[string]any{
		"client_id": id, "client_name": "billing", "grant_types": []string{"client_credentials"},
		"response_types": []string{"token"}, "scope": "customers:read audit:read", "audience": []string{testAudience},
		"token_endpoint_auth_method": "client_secret_basic", "access_token_strategy": "jwt",
		"created_at": "2026-10-03T08:00:00Z",
		"metadata":   map[string]any{"managed_by": "identity-service", "owner": "ops@example.com", "created_by": "00000000-0000-0000-0000-000000000000"},
	}
	if mutate != nil {
		mutate(m)
	}
	signClient(m)
	return m
}

var testTagKey = []byte(strings.Repeat("k", 32))

// signClient sets metadata.integrity like identity-service does at create
// time, unless the test already set (or removed) it.
func signClient(m map[string]any) {
	md, ok := m["metadata"].(map[string]any)
	if !ok {
		return
	}
	if _, set := md["integrity"]; set {
		return
	}
	id, _ := m["client_id"].(string)
	scope, _ := m["scope"].(string)
	by, _ := md["created_by"].(string)
	md["integrity"] = integrityTag(testTagKey, id, strings.Fields(scope), testAudience, by)
}

func mustAdmin(t *testing.T, u string, hc *http.Client) *Admin {
	t.Helper()
	a, err := NewAdmin(u, testAudience, testTagKey, hc)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

type vfix struct {
	v     *Verifier
	hf    *hydraFake
	clk   *clock
	keyA  *rsa.PrivateKey
	keyB  *rsa.PrivateKey
	start time.Time
}

func newVFix(t *testing.T) *vfix {
	t.Helper()
	a, b := testKeys(t)
	hf := newHydraFake(t)
	hf.jwks = []map[string]any{jwkJSON("kid-a", &a.PublicKey, nil)}
	hf.clients[testClient] = managedClientJSON(testClient, nil)
	start := time.Unix(1_791_020_000, 0)
	clk := &clock{t: start}
	v, err := NewVerifier(VerifierConfig{
		JWKSURL: hf.jwksSrv.URL, Issuer: testIssuer, Audience: testAudience,
		Admin: mustAdmin(t, hf.adminSrv.URL, nil), Now: clk.now, ClientCacheTTL: 30 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	return &vfix{v: v, hf: hf, clk: clk, keyA: a, keyB: b, start: start}
}

func (f *vfix) header() map[string]any {
	return map[string]any{"alg": "RS256", "kid": "kid-a", "typ": "JWT"}
}

func (f *vfix) claims() map[string]any {
	now := f.clk.now().Unix()
	return map[string]any{
		"iss": testIssuer, "aud": []string{testAudience}, "client_id": testClient, "sub": testClient,
		"exp": now + 299, "iat": now, "nbf": now, "jti": "jti-1", "scp": []string{"customers:read"}, "ext": map[string]any{},
	}
}

// sign builds a compact RS256 JWS with an arbitrary header (no library, so
// any header can be produced).
func sign(t *testing.T, key *rsa.PrivateKey, header, claims map[string]any) string {
	t.Helper()
	hb, _ := json.Marshal(header)
	cb, _ := json.Marshal(claims)
	input := b64(hb) + "." + b64(cb)
	sum := sha256.Sum256([]byte(input))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		t.Fatal(err)
	}
	return input + "." + b64(sig)
}

func (f *vfix) token(t *testing.T, mh func(h map[string]any), mc func(c map[string]any)) string {
	h, c := f.header(), f.claims()
	if mh != nil {
		mh(h)
	}
	if mc != nil {
		mc(c)
	}
	return sign(t, f.keyA, h, c)
}

func wantInvalid(t *testing.T, name string, err error) {
	t.Helper()
	if !errors.Is(err, app.ErrInvalidToken) {
		t.Fatalf("%s: want ErrInvalidToken, got %v", name, err)
	}
}

func TestVerifyValidToken(t *testing.T) {
	f := newVFix(t)
	tok := f.token(t, nil, func(c map[string]any) { c["scp"] = []string{"customers:read", "audit:read", "admin:all"} })
	p, err := f.v.Verify(context.Background(), tok)
	if err != nil {
		t.Fatal(err)
	}
	if p.ClientID != testClient || p.TokenID != "jti-1" || !slices.Equal(p.Scopes, []machine.Scope{machine.ScopeCustomersRead, machine.ScopeAuditRead}) {
		t.Fatalf("principal: %+v", p)
	}
	// Second call: no network (JWKS and status cached), NFR-05.
	j, a := f.hf.jwksHits.Load(), f.hf.adminHits.Load()
	if _, err := f.v.Verify(context.Background(), tok); err != nil {
		t.Fatal(err)
	}
	if f.hf.jwksHits.Load() != j || f.hf.adminHits.Load() != a {
		t.Fatal("cached verification must not call Hydra")
	}
}

func TestVerifyScopesLimitedToClientRegistration(t *testing.T) {
	f := newVFix(t)
	f.hf.set(func(h *hydraFake) {
		h.clients[testClient] = managedClientJSON(testClient, func(m map[string]any) { m["scope"] = "customers:read" })
	})
	p, err := f.v.Verify(context.Background(), f.token(t, nil, func(c map[string]any) { c["scp"] = []string{"customers:read", "audit:read"} }))
	if err != nil || !slices.Equal(p.Scopes, []machine.Scope{machine.ScopeCustomersRead}) {
		t.Fatalf("scopes: %+v %v", p, err)
	}
}

func TestVerifyHeaderRules(t *testing.T) {
	f := newVFix(t)
	ctx := context.Background()
	cases := map[string]func(h map[string]any){
		"alg none":     func(h map[string]any) { h["alg"] = "none" },
		"alg HS256":    func(h map[string]any) { h["alg"] = "HS256" },
		"alg RS512":    func(h map[string]any) { h["alg"] = "RS512" },
		"alg PS256":    func(h map[string]any) { h["alg"] = "PS256" },
		"alg ES256":    func(h map[string]any) { h["alg"] = "ES256" },
		"alg missing":  func(h map[string]any) { delete(h, "alg") },
		"alg array":    func(h map[string]any) { h["alg"] = []string{"RS256"} },
		"kid missing":  func(h map[string]any) { delete(h, "kid") },
		"kid empty":    func(h map[string]any) { h["kid"] = "" },
		"kid number":   func(h map[string]any) { h["kid"] = 7 },
		"typ missing":  func(h map[string]any) { delete(h, "typ") },
		"typ JOSE":     func(h map[string]any) { h["typ"] = "JOSE" },
		"typ id token": func(h map[string]any) { h["typ"] = "id_token+jwt" },
		"jku":          func(h map[string]any) { h["jku"] = "https://evil.example/jwks.json" },
		"x5u":          func(h map[string]any) { h["x5u"] = "https://evil.example/cert.pem" },
		"x5c":          func(h map[string]any) { h["x5c"] = []string{"MIIB"} },
		"x5t":          func(h map[string]any) { h["x5t"] = "abc" },
		"crit":         func(h map[string]any) { h["crit"] = []string{"exp"}; h["exp"] = 1 },
		"b64":          func(h map[string]any) { h["b64"] = false },
		"zip":          func(h map[string]any) { h["zip"] = "DEF" },
		"jwk embedded": func(h map[string]any) {
			h["jwk"] = jwkJSON("kid-a", &f.keyB.PublicKey, nil)
		},
	}
	for name, mh := range cases {
		_, err := f.v.Verify(ctx, f.token(t, mh, nil))
		wantInvalid(t, name, err)
	}
	// typ variants Hydra or RFC 9068 issuers use are accepted.
	for _, typ := range []string{"JWT", "jwt", "at+jwt", "application/at+jwt"} {
		if _, err := f.v.Verify(ctx, f.token(t, func(h map[string]any) { h["typ"] = typ }, nil)); err != nil {
			t.Fatalf("typ %q: %v", typ, err)
		}
	}
}

func TestVerifyRejectsUnsignedAndHMACTokens(t *testing.T) {
	f := newVFix(t)
	ctx := context.Background()
	hb, _ := json.Marshal(map[string]any{"alg": "none", "kid": "kid-a", "typ": "JWT"})
	cb, _ := json.Marshal(f.claims())
	for _, tok := range []string{b64(hb) + "." + b64(cb) + ".", b64(hb) + "." + b64(cb)} {
		_, err := f.v.Verify(ctx, tok)
		wantInvalid(t, "alg none", err)
	}
	// HS256 keyed with the RSA public key (algorithm confusion).
	hb, _ = json.Marshal(map[string]any{"alg": "HS256", "kid": "kid-a", "typ": "JWT"})
	input := b64(hb) + "." + b64(cb)
	mac := hmac.New(sha256.New, f.keyA.N.Bytes())
	mac.Write([]byte(input))
	_, err := f.v.Verify(ctx, input+"."+b64(mac.Sum(nil)))
	wantInvalid(t, "HS256", err)
}

func TestVerifySignatureAndShape(t *testing.T) {
	f := newVFix(t)
	ctx := context.Background()
	good := f.token(t, nil, nil)
	parts := strings.Split(good, ".")
	sig := []byte(parts[2])
	sig[5] ^= 1
	if sig[5] == '.' || sig[5] == '+' {
		sig[5] = 'A'
	}
	bad := []string{
		parts[0] + "." + parts[1] + "." + string(sig),                // tampered signature
		parts[0] + "." + b64([]byte(`{"iss":"x"}`)) + "." + parts[2], // swapped payload
		sign(t, f.keyB, f.header(), f.claims()),                      // right kid, wrong key
		"not-a-jwt", "a.b", "a.b.c.d", "ory_st_abcdef", "eyJ.eyJ.***",
		good + " ",
	}
	for i, tok := range bad {
		_, err := f.v.Verify(ctx, tok)
		wantInvalid(t, fmt.Sprintf("bad[%d]", i), err)
	}
}

func TestVerifySizeCapBeforeParsing(t *testing.T) {
	f := newVFix(t)
	tok := f.token(t, nil, func(c map[string]any) { c["ext"] = map[string]any{"pad": strings.Repeat("x", MaxTokenBytes)} })
	_, err := f.v.Verify(context.Background(), tok)
	wantInvalid(t, "oversized", err)
	if f.hf.jwksHits.Load() != 0 {
		t.Fatal("oversized tokens must be rejected before any key lookup")
	}
}

func TestVerifyClaimRules(t *testing.T) {
	f := newVFix(t)
	ctx := context.Background()
	now := f.clk.now().Unix()
	cases := map[string]func(c map[string]any){
		"iss trailing slash": func(c map[string]any) { c["iss"] = testIssuer + "/" },
		"iss https":          func(c map[string]any) { c["iss"] = "https://localhost:4444" },
		"iss missing":        func(c map[string]any) { delete(c, "iss") },
		"aud missing":        func(c map[string]any) { delete(c, "aud") },
		"aud empty":          func(c map[string]any) { c["aud"] = []string{} },
		"aud other":          func(c map[string]any) { c["aud"] = []string{"billing-api"} },
		"aud number":         func(c map[string]any) { c["aud"] = 1 },
		"expired":            func(c map[string]any) { c["iat"], c["nbf"], c["exp"] = now-400, now-400, now-31 },
		"exp missing":        func(c map[string]any) { delete(c, "exp") },
		"iat missing":        func(c map[string]any) { delete(c, "iat") },
		"nbf future":         func(c map[string]any) { c["nbf"] = now + 60 },
		"iat future":         func(c map[string]any) { c["iat"], c["exp"] = now+60, now+300 },
		"lifetime 11m":       func(c map[string]any) { c["exp"] = now + 660 },
		"exp before iat":     func(c map[string]any) { c["iat"], c["exp"] = now, now-1 },
		"sub other":          func(c map[string]any) { c["sub"] = "someone-else" },
		"sub missing":        func(c map[string]any) { delete(c, "sub") },
		"client_id missing":  func(c map[string]any) { delete(c, "client_id") },
		"client_id too long": func(c map[string]any) { id := strings.Repeat("a", 200); c["client_id"], c["sub"] = id, id },
		"nonce":              func(c map[string]any) { c["nonce"] = "n" },
		"at_hash":            func(c map[string]any) { c["at_hash"] = "h" },
		"azp":                func(c map[string]any) { c["azp"] = testClient },
		"scp missing":        func(c map[string]any) { delete(c, "scp") },
		"scp empty":          func(c map[string]any) { c["scp"] = []string{} },
		"scp string":         func(c map[string]any) { c["scp"] = "customers:read" },
		"scp null":           func(c map[string]any) { c["scp"] = nil },
		"jti missing":        func(c map[string]any) { delete(c, "jti") },
	}
	for name, mc := range cases {
		_, err := f.v.Verify(ctx, f.token(t, nil, mc))
		wantInvalid(t, name, err)
	}
	// Within the 30 s leeway.
	ok := map[string]func(c map[string]any){
		"exp 20s ago":       func(c map[string]any) { c["iat"], c["nbf"], c["exp"] = now-300, now-300, now-20 },
		"nbf in 20s":        func(c map[string]any) { c["nbf"] = now + 20 },
		"iat in 20s":        func(c map[string]any) { c["iat"], c["nbf"] = now+20, now+20 },
		"no nbf":            func(c map[string]any) { delete(c, "nbf") },
		"aud string":        func(c map[string]any) { c["aud"] = testAudience },
		"aud several":       func(c map[string]any) { c["aud"] = []string{"other", testAudience} },
		"lifetime exact 10": func(c map[string]any) { c["exp"] = now + 600 },
	}
	for name, mc := range ok {
		if _, err := f.v.Verify(ctx, f.token(t, nil, mc)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

func TestVerifyUnknownKidRefreshIsRateLimited(t *testing.T) {
	f := newVFix(t)
	ctx := context.Background()
	if _, err := f.v.Verify(ctx, f.token(t, nil, nil)); err != nil {
		t.Fatal(err)
	}
	if n := f.hf.jwksHits.Load(); n != 1 {
		t.Fatalf("initial fetch: %d", n)
	}
	unknown := func(kid string) string {
		return sign(t, f.keyB, map[string]any{"alg": "RS256", "kid": kid, "typ": "JWT"}, f.claims())
	}
	f.clk.add(11 * time.Second)
	_, err := f.v.Verify(ctx, unknown("kid-x1"))
	wantInvalid(t, "unknown kid", err)
	if n := f.hf.jwksHits.Load(); n != 2 {
		t.Fatalf("unknown kid must refresh once: %d", n)
	}
	for i := range 20 { // kid flooding within 10 s: no further fetches
		_, err := f.v.Verify(ctx, unknown(fmt.Sprintf("kid-flood-%d", i)))
		wantInvalid(t, "flood", err)
	}
	if n := f.hf.jwksHits.Load(); n != 2 {
		t.Fatalf("refresh not rate-limited: %d fetches", n)
	}
	// Key rollover: Hydra publishes kid-b; after the interval it is picked up.
	f.hf.set(func(h *hydraFake) { h.jwks = append(h.jwks, jwkJSON("kid-b", &f.keyB.PublicKey, nil)) })
	_, err = f.v.Verify(ctx, unknown("kid-b"))
	wantInvalid(t, "kid-b within interval", err)
	f.clk.add(11 * time.Second)
	if _, err := f.v.Verify(ctx, unknown("kid-b")); err != nil {
		t.Fatalf("rolled-over key: %v", err)
	}
	if n := f.hf.jwksHits.Load(); n != 3 {
		t.Fatalf("fetches: %d", n)
	}
}

func TestVerifyConcurrentUnknownKidIsSingleFlight(t *testing.T) {
	f := newVFix(t)
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = f.v.Verify(context.Background(), f.token(t, nil, nil))
		}()
	}
	wg.Wait()
	if n := f.hf.jwksHits.Load(); n != 1 {
		t.Fatalf("concurrent first use must fetch once: %d", n)
	}
}

func TestVerifyJWKSUnavailable(t *testing.T) {
	f := newVFix(t)
	f.hf.set(func(h *hydraFake) { h.jwksStatus = 503 })
	_, err := f.v.Verify(context.Background(), f.token(t, nil, nil))
	if !errors.Is(err, app.ErrDependencyUnavailable) {
		t.Fatalf("no keys and JWKS down: want 503, got %v", err)
	}
	// Within the interval: still 503 (no keys at all), without a new fetch.
	_, err = f.v.Verify(context.Background(), f.token(t, nil, nil))
	if !errors.Is(err, app.ErrDependencyUnavailable) || f.hf.jwksHits.Load() != 1 {
		t.Fatalf("second call: %v hits=%d", err, f.hf.jwksHits.Load())
	}
	// Recovery; then a failed periodic refresh keeps the keys.
	f.hf.set(func(h *hydraFake) { h.jwksStatus = 200 })
	f.clk.add(11 * time.Second)
	if _, err := f.v.Verify(context.Background(), f.token(t, nil, nil)); err != nil {
		t.Fatal(err)
	}
	f.hf.set(func(h *hydraFake) { h.jwksStatus = 500 })
	if err := f.v.RefreshKeys(context.Background()); err == nil {
		t.Fatal("refresh against a failing JWKS must report an error")
	}
	if _, err := f.v.Verify(context.Background(), f.token(t, nil, nil)); err != nil {
		t.Fatalf("keys must survive a failed refresh: %v", err)
	}
}

func TestParseJWKSLimits(t *testing.T) {
	a, _ := testKeys(t)
	small, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	enc := func(keys ...map[string]any) []byte { b, _ := json.Marshal(map[string]any{"keys": keys}); return b }
	got, err := parseJWKS(enc(
		jwkJSON("ok", &a.PublicKey, nil),
		jwkJSON("no-use-no-alg", &a.PublicKey, map[string]any{"use": nil, "alg": nil}),
		jwkJSON("enc", &a.PublicKey, map[string]any{"use": "enc"}),
		jwkJSON("rs512", &a.PublicKey, map[string]any{"alg": "RS512"}),
		jwkJSON("", &a.PublicKey, nil),
		jwkJSON("small", &small.PublicKey, nil),
		map[string]any{"kty": "oct", "kid": "hmac", "k": b64([]byte("secret"))},
		map[string]any{"kty": "weird", "kid": "w"},
		jwkJSON("dup", &a.PublicKey, nil), jwkJSON("dup", &a.PublicKey, nil),
	))
	if err != nil {
		t.Fatal(err)
	}
	var kids []string
	for k := range got {
		kids = append(kids, k)
	}
	slices.Sort(kids)
	if !slices.Equal(kids, []string{"no-use-no-alg", "ok"}) {
		t.Fatalf("usable kids: %v", kids)
	}
	many := make([]map[string]any, 21)
	for i := range many {
		many[i] = jwkJSON(fmt.Sprintf("k%d", i), &a.PublicKey, nil)
	}
	if _, err := parseJWKS(enc(many...)); err == nil {
		t.Fatal("more than 20 keys must fail")
	}
	if _, err := parseJWKS(enc(jwkJSON("enc", &a.PublicKey, map[string]any{"use": "enc"}))); err == nil {
		t.Fatal("no usable key must fail")
	}
	if _, err := parseJWKS([]byte("{")); err == nil {
		t.Fatal("malformed")
	}
}

func TestJWKSSizeCap(t *testing.T) {
	big := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"keys":[],"pad":"` + strings.Repeat("x", maxJWKSBytes) + `"}`))
	}))
	defer big.Close()
	ks := newKeySet(big.URL, nil, time.Now, 0, nil)
	if _, err := ks.fetch(context.Background()); err == nil || !strings.Contains(err.Error(), "larger") {
		t.Fatalf("size cap: %v", err)
	}
}

func TestVerifyClientStatus(t *testing.T) {
	ctx := context.Background()
	cases := map[string]func(m map[string]any){
		"unmanaged":              func(m map[string]any) { m["metadata"] = map[string]any{"managed_by": "someone-else"} },
		"no metadata":            func(m map[string]any) { delete(m, "metadata") },
		"metadata malformed":     func(m map[string]any) { m["metadata"] = map[string]any{"managed_by": 5} },
		"extra grant":            func(m map[string]any) { m["grant_types"] = []string{"client_credentials", "authorization_code"} },
		"other grant":            func(m map[string]any) { m["grant_types"] = []string{"authorization_code"} },
		"extra audience":         func(m map[string]any) { m["audience"] = []string{testAudience, "billing"} },
		"other audience":         func(m map[string]any) { m["audience"] = []string{"billing"} },
		"client_id mismatch":     func(m map[string]any) { m["client_id"] = "other" },
		"no scopes registered":   func(m map[string]any) { m["scope"] = "" },
		"rotated after issuance": func(m map[string]any) {},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			f := newVFix(t)
			f.hf.set(func(h *hydraFake) {
				h.clients[testClient] = managedClientJSON(testClient, func(m map[string]any) {
					mutate(m)
					if name == "rotated after issuance" {
						m["metadata"].(map[string]any)["tokens_valid_after"] = f.clk.now().Unix() + 1
					}
				})
			})
			p, err := f.v.Verify(ctx, f.token(t, nil, nil))
			if name == "no scopes registered" {
				// A valid client with no registered scopes gets no scopes (→ 403 at the route).
				if err != nil || len(p.Scopes) != 0 {
					t.Fatalf("no scopes: %+v %v", p, err)
				}
				return
			}
			wantInvalid(t, name, err)
		})
	}
}

func TestVerifyTokensValidAfter(t *testing.T) {
	f := newVFix(t)
	ctx := context.Background()
	tok := f.token(t, nil, nil) // iat = start
	f.hf.set(func(h *hydraFake) {
		h.clients[testClient] = managedClientJSON(testClient, func(m map[string]any) {
			m["metadata"].(map[string]any)["tokens_valid_after"] = f.start.Unix()
		})
	})
	if _, err := f.v.Verify(ctx, tok); err != nil {
		t.Fatalf("iat == tokens_valid_after is valid: %v", err)
	}
	f.hf.set(func(h *hydraFake) {
		h.clients[testClient] = managedClientJSON(testClient, func(m map[string]any) {
			m["metadata"].(map[string]any)["tokens_valid_after"] = f.start.Unix() + 5
		})
	})
	f.v.Invalidate(testClient) // as the rotating replica does
	_, err := f.v.Verify(ctx, tok)
	wantInvalid(t, "rotated", err)
	f.clk.add(6 * time.Second)
	if _, err := f.v.Verify(ctx, f.token(t, nil, nil)); err != nil {
		t.Fatalf("token issued after rotation: %v", err)
	}
}

func TestVerifyNegativeCacheAndTTL(t *testing.T) {
	f := newVFix(t)
	ctx := context.Background()
	f.hf.set(func(h *hydraFake) { delete(h.clients, testClient) })
	tok := f.token(t, nil, nil)
	_, err := f.v.Verify(ctx, tok)
	wantInvalid(t, "deleted client", err)
	hits := f.hf.adminHits.Load()
	for range 5 {
		_, err = f.v.Verify(ctx, tok)
		wantInvalid(t, "deleted client cached", err)
	}
	if f.hf.adminHits.Load() != hits {
		t.Fatal("negative result must be cached")
	}
	// Re-created (e.g. restored) client: visible after the TTL, never before.
	f.hf.set(func(h *hydraFake) { h.clients[testClient] = managedClientJSON(testClient, nil) })
	f.clk.add(29 * time.Second)
	_, err = f.v.Verify(ctx, f.token(t, nil, nil))
	wantInvalid(t, "within ttl", err)
	f.clk.add(2 * time.Second)
	if _, err := f.v.Verify(ctx, f.token(t, nil, nil)); err != nil {
		t.Fatalf("after ttl: %v", err)
	}
	if f.hf.adminHits.Load() != hits+1 {
		t.Fatalf("one lookup after expiry: %d", f.hf.adminHits.Load()-hits)
	}
}

func TestVerifyClientDeletedAfterCachingIsRejectedAfterTTL(t *testing.T) {
	f := newVFix(t)
	ctx := context.Background()
	if _, err := f.v.Verify(ctx, f.token(t, nil, nil)); err != nil {
		t.Fatal(err)
	}
	f.hf.set(func(h *hydraFake) { delete(h.clients, testClient) })
	// Other replica: cached up to the TTL (accepted lag, §6 B6) ...
	if _, err := f.v.Verify(ctx, f.token(t, nil, nil)); err != nil {
		t.Fatalf("within ttl: %v", err)
	}
	// ... then rejected.
	f.clk.add(31 * time.Second)
	_, err := f.v.Verify(ctx, f.token(t, nil, nil))
	wantInvalid(t, "after ttl", err)
	// This replica deleted it: rejected at once.
	g := newVFix(t)
	if _, err := g.v.Verify(ctx, g.token(t, nil, nil)); err != nil {
		t.Fatal(err)
	}
	g.hf.set(func(h *hydraFake) { delete(h.clients, testClient) })
	g.v.Invalidate(testClient)
	_, err = g.v.Verify(ctx, g.token(t, nil, nil))
	wantInvalid(t, "after purge", err)
}

func TestVerifyHydraDown(t *testing.T) {
	f := newVFix(t)
	ctx := context.Background()
	if _, err := f.v.Verify(ctx, f.token(t, nil, nil)); err != nil {
		t.Fatal(err)
	}
	f.hf.set(func(h *hydraFake) { h.adminDown = true })
	// Cached entry: still OK while within the TTL.
	f.clk.add(20 * time.Second)
	if _, err := f.v.Verify(ctx, f.token(t, nil, nil)); err != nil {
		t.Fatalf("cached status while Hydra is down: %v", err)
	}
	// Never served past the TTL: 503, not 401.
	f.clk.add(11 * time.Second)
	if _, err := f.v.Verify(ctx, f.token(t, nil, nil)); !errors.Is(err, app.ErrDependencyUnavailable) {
		t.Fatalf("expired entry and Hydra down: want 503, got %v", err)
	}
	// Errors are not cached: recovery is immediate.
	f.hf.set(func(h *hydraFake) { h.adminDown = false })
	if _, err := f.v.Verify(ctx, f.token(t, nil, nil)); err != nil {
		t.Fatalf("after recovery: %v", err)
	}
}

func TestVerifyErrorsCarryNoTokenContents(t *testing.T) {
	f := newVFix(t)
	tok := f.token(t, nil, func(c map[string]any) { c["iss"] = "https://evil.example/" + strings.Repeat("Z", 10) })
	_, err := f.v.Verify(context.Background(), tok)
	if err == nil || strings.Contains(err.Error(), "evil") || strings.Contains(err.Error(), tok[:20]) {
		t.Fatalf("error leaks token contents: %v", err)
	}
}

func TestNewVerifierRequiresConfig(t *testing.T) {
	if _, err := NewVerifier(VerifierConfig{}); err == nil {
		t.Fatal("empty config must fail")
	}
}

// Security S4: a client created straight at the (unauthenticated) Hydra
// admin API with managed_by=identity-service has no valid tag.
func TestVerifyClientIntegrityTag(t *testing.T) {
	ctx := context.Background()
	cases := map[string]func() map[string]any{
		"tag missing": func() map[string]any {
			return managedClientJSON(testClient, func(m map[string]any) { m["metadata"].(map[string]any)["integrity"] = "" })
		},
		"tag garbage": func() map[string]any {
			return managedClientJSON(testClient, func(m map[string]any) { m["metadata"].(map[string]any)["integrity"] = "!!!" })
		},
		"tag from another key": func() map[string]any {
			return managedClientJSON(testClient, func(m map[string]any) {
				m["metadata"].(map[string]any)["integrity"] = integrityTag([]byte(strings.Repeat("x", 32)), testClient,
					[]string{"customers:read", "audit:read"}, testAudience, "00000000-0000-0000-0000-000000000000")
			})
		},
		"scopes changed after tagging": func() map[string]any {
			m := managedClientJSON(testClient, nil)
			m["scope"] = "customers:read"
			return m
		},
		"created_by changed after tagging": func() map[string]any {
			m := managedClientJSON(testClient, nil)
			m["metadata"].(map[string]any)["created_by"] = uuidString()
			return m
		},
	}
	for name, mk := range cases {
		t.Run(name, func(t *testing.T) {
			f := newVFix(t)
			f.hf.set(func(h *hydraFake) { h.clients[testClient] = mk() })
			_, err := f.v.Verify(ctx, f.token(t, nil, nil))
			wantInvalid(t, name, err)
		})
	}
	// Scope order does not matter (tag over sorted scopes).
	f := newVFix(t)
	f.hf.set(func(h *hydraFake) {
		m := managedClientJSON(testClient, nil)
		m["scope"] = "audit:read customers:read"
		h.clients[testClient] = m
	})
	if _, err := f.v.Verify(ctx, f.token(t, nil, nil)); err != nil {
		t.Fatalf("reordered scopes: %v", err)
	}
}

func uuidString() string { return "11111111-2222-3333-4444-555555555555" }

// Only 404 (and 400) mean "no such client"; anything else is a dependency
// failure that is never cached as negative.
func TestVerifyAdminNon404IsDependencyErrorAndNotCached(t *testing.T) {
	for _, st := range []int{401, 403, 409, 429, 302} {
		t.Run(fmt.Sprint(st), func(t *testing.T) {
			f := newVFix(t)
			f.hf.set(func(h *hydraFake) { h.adminStatus = st })
			_, err := f.v.Verify(context.Background(), f.token(t, nil, nil))
			if !errors.Is(err, app.ErrDependencyUnavailable) {
				t.Fatalf("status %d: want 503, got %v", st, err)
			}
			f.hf.set(func(h *hydraFake) { h.adminStatus = 0 })
			if _, err := f.v.Verify(context.Background(), f.token(t, nil, nil)); err != nil {
				t.Fatalf("status %d must not be cached: %v", st, err)
			}
		})
	}
	f := newVFix(t)
	f.hf.set(func(h *hydraFake) { h.adminStatus = 400 })
	_, err := f.v.Verify(context.Background(), f.token(t, nil, nil))
	wantInvalid(t, "400 = malformed id = not found", err)
}

// An Invalidate during an in-flight (shared) lookup must not let the stale
// result through: the lookup is forgotten and repeated.
func TestVerifyInvalidateDuringInflightLookup(t *testing.T) {
	f := newVFix(t)
	ctx := context.Background()
	if err := f.v.RefreshKeys(ctx); err != nil {
		t.Fatal(err)
	}
	tok := f.token(t, nil, nil)
	block := make(chan struct{})
	f.hf.set(func(h *hydraFake) { h.block = block })
	done := make(chan error, 2)
	for range 2 { // two callers share the in-flight lookup
		go func() { _, err := f.v.Verify(ctx, tok); done <- err }()
	}
	deadline := time.Now().Add(5 * time.Second)
	for f.hf.adminHits.Load() < 1 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	// The snapshot of the in-flight request still holds the client.
	f.hf.set(func(h *hydraFake) { delete(h.clients, testClient); h.block = nil })
	f.v.Invalidate(testClient)
	close(block)
	for range 2 {
		wantInvalid(t, "stale in-flight result", <-done)
	}
}

// §6 B3 with the ceil(now)+1 rule: a token issued in the rotation second is
// rejected, one issued from tokens_valid_after on is accepted.
func TestVerifyTokenIssuedInRotationSecondIsRejected(t *testing.T) {
	f := newVFix(t)
	ctx := context.Background()
	rotation := f.start.Add(300 * time.Millisecond)
	tva := app.TokensValidAfter(rotation)
	if tva.Unix() != f.start.Unix()+2 {
		t.Fatalf("tokens_valid_after = %d, want ceil(now)+1 = %d", tva.Unix(), f.start.Unix()+2)
	}
	f.hf.set(func(h *hydraFake) {
		h.clients[testClient] = managedClientJSON(testClient, func(m map[string]any) {
			m["metadata"].(map[string]any)["tokens_valid_after"] = tva.Unix()
		})
	})
	_, err := f.v.Verify(ctx, f.token(t, nil, nil)) // iat == rotation second
	wantInvalid(t, "iat == rotation second", err)
	f.clk.add(time.Second)
	_, err = f.v.Verify(ctx, f.token(t, nil, nil))
	wantInvalid(t, "iat == tva-1", err)
	f.clk.add(time.Second)
	if _, err := f.v.Verify(ctx, f.token(t, nil, nil)); err != nil {
		t.Fatalf("iat == tokens_valid_after: %v", err)
	}
	if got := app.TokensValidAfter(f.start); got.Unix() != f.start.Unix()+1 {
		t.Fatalf("whole second: %d", got.Unix())
	}
}
