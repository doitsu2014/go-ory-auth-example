package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
)

// Machine plane rate limits (per replica, §6 A12, M2M-NFR-04).
const (
	DefaultMachineRatePerMin = 600
	MachineFailuresPerMin    = 60
)

// Rate limiter keys are name-based UUIDs, so the existing per-actor limiter
// can key service clients and caller IPs without holding raw values.
var (
	clientKeyNS = uuid.MustParse("6c1f3f39-5b0e-4a51-9f3e-3f8f3c1f0a01")
	ipKeyNS     = uuid.MustParse("6c1f3f39-5b0e-4a51-9f3e-3f8f3c1f0a02")
)

func clientKey(id string) uuid.UUID { return uuid.NewSHA1(clientKeyNS, []byte(id)) }

func ipKey(a *netip.Addr) uuid.UUID {
	if a == nil {
		return uuid.NewSHA1(ipKeyNS, nil)
	}
	return uuid.NewSHA1(ipKeyNS, []byte(a.String()))
}

// NewMachineClientLimiter allows perMin requests per client per minute.
func NewMachineClientLimiter(perMin int, now func() time.Time) *app.RateLimiter {
	if perMin <= 0 {
		perMin = DefaultMachineRatePerMin
	}
	return app.NewRateLimiter([]app.RateRule{{Limit: perMin, Window: time.Minute}}, now)
}

// NewMachineFailureLimiter allows MachineFailuresPerMin rejected
// authentications per caller IP per minute.
func NewMachineFailureLimiter(now func() time.Time) *app.RateLimiter {
	return app.NewRateLimiter([]app.RateRule{{Limit: MachineFailuresPerMin, Window: time.Minute}}, now)
}

// MachineGuard authenticates and authorizes every /m2m/v1 request before
// parameter binding, in this order (it never falls back to Kratos):
//
//  1. session credentials present (cookie / X-Session-Token)   → 401 invalid_token
//  2. no Authorization header                                  → 401 unauthenticated
//  3. malformed Authorization (scheme, spacing, repeated)      → 400 invalid_request
//  4. token verification (JWT, JWKS, client status)            → 401 invalid_token / 503
//  5. route + policy lookup (fail closed)                      → 404 / 405 / 500
//  6. route scope in the token                                 → 403 insufficient_scope
//  7. per-client limiter (600/min)                             → 429
//
// Rejections in 1–4 are charged to the caller IP (60/min); over budget they
// answer 429 instead. The budget is never checked before verification, so a
// token that verifies is never refused because of other callers' failures
// behind the same address.
//
// Every request that passes 4 is logged once as "m2m_access" (§6 A14).
type MachineGuard struct {
	Verifier       app.MachineTokenVerifier
	Policies       map[string]Policy
	Router         chi.Routes
	ClientLimiter  *app.RateLimiter
	FailureLimiter *app.RateLimiter
	TrustedHops    int
	Errors         errorWriter
	Log            *slog.Logger
}

func (g *MachineGuard) serve(w http.ResponseWriter, r *http.Request, next http.Handler) {
	ctx := r.Context()
	ipk := ipKey(clientIP(r, g.TrustedHops))
	reject := func(code, reason string) {
		if !g.FailureLimiter.Allow(ipk) {
			code = CodeRateLimited
		}
		if g.Log != nil {
			g.Log.DebugContext(ctx, "machine request rejected", "reason", reason, "request_id", RequestIDFrom(ctx))
		}
		writeProblem(w, r, code, "", nil)
	}
	if c, err := r.Cookie(SessionCookieName); (err == nil && c.Value != "") || r.Header.Get("X-Session-Token") != "" {
		reject(CodeInvalidToken, "session credential on machine plane")
		return
	}
	authz := r.Header.Values("Authorization")
	if len(authz) == 0 {
		reject(CodeUnauthenticated, "no credential")
		return
	}
	scheme, tok, found := strings.Cut(authz[0], " ")
	if len(authz) != 1 || !found || !strings.EqualFold(scheme, "Bearer") || tok == "" || strings.ContainsAny(tok, " \t") {
		reject(CodeInvalidRequest, "malformed authorization header")
		return
	}
	p, err := g.Verifier.Verify(ctx, tok)
	if err != nil {
		if errors.Is(err, app.ErrInvalidToken) {
			reject(CodeInvalidToken, err.Error())
			return
		}
		g.Errors.write(w, r, err)
		return
	}
	setMachine(ctx, p)

	rec := &statusRecorder{ResponseWriter: w}
	rctx := chi.NewRouteContext()
	matched := g.Router.Match(rctx, r.Method, routingPath(r))
	defer func() {
		status := rec.status
		if status == 0 {
			status = http.StatusOK
		}
		route := "unmatched"
		if matched {
			route = rctx.RoutePattern()
		}
		if g.Log != nil {
			g.Log.InfoContext(ctx, "m2m_access", "client_id", p.ClientID, "jti", p.TokenID, "method", r.Method,
				"route", route, "target_id", rctx.URLParam("id"), "status", status, "request_id", RequestIDFrom(ctx))
		}
	}()
	if !matched {
		if allow := g.allowedMethods(r); len(allow) > 0 {
			rec.Header().Set("Allow", strings.Join(allow, ", "))
			writeProblem(rec, r, CodeMethodNotAllowed, "", nil)
			return
		}
		writeProblem(rec, r, CodeNotFound, "", nil)
		return
	}
	pol, ok := g.Policies[routeKey(r.Method, rctx.RoutePattern())]
	if !ok || pol.Plane != PlaneMachine || pol.Scope == "" {
		// Unreachable after ValidatePolicies; fail closed anyway.
		writeProblem(rec, r, CodeInternal, "", nil)
		return
	}
	setRequiredScope(ctx, pol.Scope)
	if !p.HasScope(pol.Scope) {
		writeProblem(rec, r, CodeInsufficientScope, "", nil)
		return
	}
	if !g.ClientLimiter.Allow(clientKey(p.ClientID)) {
		writeProblem(rec, r, CodeRateLimited, "", nil)
		return
	}
	next.ServeHTTP(rec, r)
}

// allowedMethods lists the methods routed for the request path.
func (g *MachineGuard) allowedMethods(r *http.Request) []string {
	var out []string
	for _, m := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		if m != r.Method && g.Router.Match(chi.NewRouteContext(), m, routingPath(r)) {
			out = append(out, m)
		}
	}
	return out
}

// looksLikeJWT reports a JWS compact serialisation (three base64url
// segments, the header starting with `{"` = "eyJ"). Such bearers are machine
// credentials and are rejected on /v1 before Kratos is called (§6 A13).
func looksLikeJWT(tok string) bool {
	if !strings.HasPrefix(tok, "eyJ") {
		return false
	}
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		return false
	}
	for _, p := range parts {
		if !isBase64URL(p) {
			return false
		}
	}
	return true
}

func isBase64URL(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '=') {
			return false
		}
	}
	return true
}
