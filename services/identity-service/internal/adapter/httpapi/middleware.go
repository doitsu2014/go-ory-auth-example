package httpapi

import (
	"context"
	"log/slog"
	"mime"
	"net/http"
	"runtime/debug"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/identity"
)

// SessionCookieName is the Kratos session cookie (the only admin-plane credential).
const SessionCookieName = "ory_kratos_session"

// MaxBodyBytes is the request body limit (06-security §6.4).
const MaxBodyBytes = 1 << 20

// requestState installs the per-request state and the request id.
func requestState(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		st := &reqState{requestID: newRequestID(r.Header.Get("X-Request-Id"))}
		w.Header().Set("X-Request-Id", st.requestID)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), stateKey{}, st)))
	})
}

// recoverer turns panics into 500 problems and logs the stack.
func recoverer(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if v := recover(); v != nil {
					if v == http.ErrAbortHandler {
						panic(v)
					}
					log.ErrorContext(r.Context(), "panic", "panic", v, "stack", string(debug.Stack()), "request_id", RequestIDFrom(r.Context()))
					writeProblem(w, r, CodeInternal, "", nil)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (s *statusRecorder) WriteHeader(code int) {
	if s.status == 0 {
		s.status = code
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	n, err := s.ResponseWriter.Write(b)
	s.bytes += n
	return n, err
}

func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// accessLog logs one line per request (never headers or bodies) and records
// RED metrics.
func accessLog(log *slog.Logger, m *Metrics, server string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w}
			next.ServeHTTP(rec, r)
			if rec.status == 0 {
				rec.status = http.StatusOK
			}
			route := "unmatched"
			if rc := chi.RouteContext(r.Context()); rc != nil && rc.RoutePattern() != "" {
				route = rc.RoutePattern()
			}
			dur := time.Since(start)
			if m != nil {
				m.observe(server, r.Method, route, rec.status, dur)
			}
			attrs := []any{
				"server", server, "method", r.Method, "route", route, "status", rec.status,
				"duration_ms", dur.Milliseconds(), "bytes", rec.bytes,
			}
			if st := stateFrom(r.Context()); st != nil {
				st.mu.Lock()
				attrs = append(attrs, "request_id", st.requestID)
				if st.identityID != "" {
					attrs = append(attrs, "identity_id", st.identityID)
				}
				if st.problemCode != "" {
					attrs = append(attrs, "problem_code", st.problemCode)
				}
				st.mu.Unlock()
			}
			lvl := slog.LevelInfo
			if rec.status >= 500 {
				lvl = slog.LevelError
			}
			log.Log(r.Context(), lvl, "http request", attrs...)
		})
	}
}

// limits applies the body size limit and a request timeout.
func limits(maxBody int64, timeout time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength > maxBody {
				writeProblem(w, r, CodeValidationFailed, "Request body too large.", []app.FieldError{{Field: "body", Code: "too_large"}})
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, maxBody)
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// securityHeaders sets headers for authenticated API responses.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Cache-Control", "no-store")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

// cors allows the configured origins with credentials. Preflights from
// unknown origins get no CORS headers (the browser blocks them).
func cors(allowed []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			h := w.Header()
			if origin != "" {
				h.Add("Vary", "Origin")
			}
			ok := origin != "" && slices.Contains(allowed, origin)
			if ok {
				h.Set("Access-Control-Allow-Origin", origin)
				h.Set("Access-Control-Allow-Credentials", "true")
				h.Set("Access-Control-Expose-Headers", "X-Request-Id")
			}
			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				if ok {
					h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE")
					h.Set("Access-Control-Allow-Headers", "Content-Type, Idempotency-Key, X-Request-Id")
					h.Set("Access-Control-Max-Age", "600")
					w.WriteHeader(http.StatusNoContent)
					return
				}
				w.WriteHeader(http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// Guard authenticates and authorizes every /v1 and /admin/v1 request before
// parameter binding, in this order:
//
//  1. credential bound to plane (bearer on /v1, cookie on /admin/v1)  → 401
//  2. Kratos whoami (cached)                                         → 401 / 403 aal2_required / 503
//  3. plane population (customer / admin)                            → 403 forbidden / not_admin
//  4. route + policy lookup (fail closed)                            → 404 / 500
//  5. admin gate: 12 h cap, AAL2, MFA enrolment deadline              → 401 / 403
//  6. CSRF guard for cookie-authenticated mutations                   → 403
//  7. Keto permission                                                 → 403 / 503
type Guard struct {
	Verifier       app.SessionVerifier
	Gate           *app.AdminGate
	Authz          app.Authorizer
	Policies       map[string]Policy
	AllowedOrigins []string
	TrustedHops    int
	Errors         errorWriter
	// Router is used to match the route pattern before handlers run.
	Router chi.Routes
}

// Middleware returns the guard as middleware.
func (g *Guard) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		plane := planeOf(r)
		if plane == PlaneNone {
			next.ServeHTTP(w, r)
			return
		}
		cred, ok := credentialFor(plane, r)
		if !ok {
			writeProblem(w, r, CodeUnauthenticated, "", nil)
			return
		}
		ctx := app.WithAuthzMemo(r.Context())
		p, err := g.Verifier.Verify(ctx, cred)
		if err != nil {
			g.Errors.write(w, r, err)
			return
		}
		actor := app.Actor{Principal: p, RequestID: RequestIDFrom(ctx), ClientIP: clientIP(r, g.TrustedHops)}
		setActor(ctx, actor)
		switch plane {
		case PlaneCustomer:
			if p.Kind != identity.KindCustomer {
				writeProblem(w, r, CodeForbidden, "", nil)
				return
			}
		case PlaneAdmin:
			if p.Kind != identity.KindAdmin {
				writeProblem(w, r, CodeNotAdmin, "", nil)
				return
			}
		}
		rctx := chi.NewRouteContext()
		if !g.Router.Match(rctx, r.Method, routingPath(r)) {
			// Fail closed: nothing without a matched route and policy runs.
			writeProblem(w, r, CodeNotFound, "", nil)
			return
		}
		pol, ok := g.Policies[routeKey(r.Method, rctx.RoutePattern())]
		if !ok || pol.Plane != plane {
			// Unreachable after ValidatePolicies; fail closed anyway.
			writeProblem(w, r, CodeInternal, "", nil)
			return
		}
		if plane == PlaneAdmin {
			if err := g.Gate.Check(ctx, actor, pol.AllowAAL1); err != nil {
				g.Errors.write(w, r, err)
				return
			}
			if isMutation(r.Method) && !g.csrfOK(r) {
				writeProblem(w, r, CodeForbidden, "Cross-site request rejected.", nil)
				return
			}
		}
		if pol.Permission != "" {
			allowed, err := g.Authz.Check(ctx, p.IdentityID, pol.Permission)
			if err != nil {
				g.Errors.write(w, r, err)
				return
			}
			if !allowed {
				writeProblem(w, r, CodeForbidden, "", nil)
				return
			}
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func isMutation(m string) bool {
	switch m {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return false
}

// csrfOK requires an allowlisted Origin and, when a body is present, a JSON
// content type (06-security §6.4).
func (g *Guard) csrfOK(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" || !slices.Contains(g.AllowedOrigins, origin) {
		return false
	}
	hasBody := r.ContentLength > 0 || (r.ContentLength < 0 && r.Body != nil && r.Body != http.NoBody)
	if hasBody || r.Header.Get("Content-Type") != "" {
		mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mt != "application/json" {
			return false
		}
	}
	return true
}

// credentialFor extracts the credential bound to the plane. Any credential of
// the other plane's type makes the request unauthenticated.
func credentialFor(plane Plane, r *http.Request) (app.Credential, bool) {
	cookie, cookieErr := r.Cookie(SessionCookieName)
	hasCookie := cookieErr == nil && cookie.Value != ""
	authz := r.Header.Get("Authorization")
	switch plane {
	case PlaneCustomer:
		if hasCookie || r.Header.Get("X-Session-Token") != "" {
			return app.Credential{}, false
		}
		scheme, tok, found := strings.Cut(authz, " ")
		tok = strings.TrimSpace(tok)
		if !found || !strings.EqualFold(scheme, "Bearer") || tok == "" {
			return app.Credential{}, false
		}
		return app.Credential{Kind: app.CredentialToken, Value: tok}, true
	case PlaneAdmin:
		if authz != "" || r.Header.Get("X-Session-Token") != "" || !hasCookie {
			return app.Credential{}, false
		}
		return app.Credential{Kind: app.CredentialCookie, Value: cookie.Value}, true
	}
	return app.Credential{}, false
}

// notFound / methodNotAllowed write problems instead of text.
func notFound(w http.ResponseWriter, r *http.Request) { writeProblem(w, r, CodeNotFound, "", nil) }

func methodNotAllowed(w http.ResponseWriter, r *http.Request) {
	writeProblem(w, r, CodeMethodNotAllowed, "", nil)
}
