package httpapi

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/adapter/httpapi/gen"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
)

// PublicDeps wires the :8080 router.
type PublicDeps struct {
	Log            *slog.Logger
	Metrics        *Metrics
	Server         *Server
	Verifier       app.SessionVerifier
	Gate           *app.AdminGate
	Authz          app.Authorizer
	AllowedOrigins []string
	TrustedHops    int
	// Policies defaults to RoutePolicies.
	Policies map[string]Policy
	// RequestTimeout defaults to 10 s.
	RequestTimeout time.Duration
}

// NewPublicHandler builds the :8080 handler (/v1, /admin/v1). It fails if any
// route lacks a valid policy (P4).
func NewPublicHandler(d PublicDeps) (http.Handler, error) {
	if d.Policies == nil {
		d.Policies = RoutePolicies
	}
	if d.RequestTimeout == 0 {
		d.RequestTimeout = 10 * time.Second
	}
	ew := errorWriter{log: d.Log}

	api := chi.NewRouter()
	api.NotFound(notFound)
	api.MethodNotAllowed(methodNotAllowed)
	strict := gen.NewStrictHandlerWithOptions(d.Server, nil, gen.StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  ew.requestError,
		ResponseErrorHandlerFunc: ew.write,
	})
	gen.HandlerWithOptions(strict, gen.ChiServerOptions{
		BaseRouter: api, ErrorHandlerFunc: ew.paramError, Middlewares: []gen.MiddlewareFunc{strictBodyMiddleware},
	})
	if err := ValidatePolicies(api, d.Policies); err != nil {
		return nil, err
	}

	guard := &Guard{
		Verifier: d.Verifier, Gate: d.Gate, Authz: d.Authz, Policies: d.Policies,
		AllowedOrigins: d.AllowedOrigins, TrustedHops: d.TrustedHops, Errors: ew, Router: api,
	}

	root := chi.NewRouter()
	root.Use(requestState, recoverer(d.Log), accessLog(d.Log, d.Metrics, "public"),
		limits(MaxBodyBytes, d.RequestTimeout), cors(d.AllowedOrigins), securityHeaders, guard.Middleware)
	root.Mount("/", api)
	return otelhttp.NewHandler(root, "identity-service.public"), nil
}

// KratosHookPayload mirrors the webhook Jsonnet templates.
type KratosHookPayload struct {
	IdentityID uuid.UUID `json:"identity_id"`
	SchemaID   string    `json:"schema_id"`
	FlowType   string    `json:"flow_type"`
}

// WebhookDeps wires the :8081 router.
type WebhookDeps struct {
	Log          *slog.Logger
	Metrics      *Metrics
	APIKey       string
	Provisioning *app.ProvisioningService
}

// LoginInterruptMessageID is the message id shown when the population guard
// rejects a login (custom id, outside Kratos' own ranges' semantics).
const LoginInterruptMessageID = 4000001

// NewWebhookHandler builds the :8081 handler (Kratos webhooks only).
func NewWebhookHandler(d WebhookDeps) http.Handler {
	r := chi.NewRouter()
	r.Use(requestState, recoverer(d.Log), accessLog(d.Log, d.Metrics, "webhook"), limits(64<<10, 5*time.Second))
	r.NotFound(notFound)
	r.MethodNotAllowed(methodNotAllowed)
	want := sha256.Sum256([]byte(d.APIKey))
	auth := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			got := sha256.Sum256([]byte(r.Header.Get("Authorization")))
			if d.APIKey == "" || subtle.ConstantTimeCompare(got[:], want[:]) != 1 {
				writeProblem(w, r, CodeUnauthenticated, "", nil)
				return
			}
			next(w, r)
		}
	}
	decode := func(w http.ResponseWriter, r *http.Request) (KratosHookPayload, bool) {
		var p KratosHookPayload
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil || p.IdentityID == uuid.Nil {
			writeProblem(w, r, CodeValidationFailed, "", []app.FieldError{{Field: "body", Code: "invalid"}})
			return p, false
		}
		return p, true
	}
	r.Post("/internal/hooks/kratos/after-registration", auth(func(w http.ResponseWriter, r *http.Request) {
		p, ok := decode(w, r)
		if !ok {
			return
		}
		if err := d.Provisioning.HandleRegistration(r.Context(), p.IdentityID, p.SchemaID); err != nil {
			errorWriter{log: d.Log}.write(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	r.Post("/internal/hooks/kratos/after-login", auth(func(w http.ResponseWriter, r *http.Request) {
		p, ok := decode(w, r)
		if !ok {
			return
		}
		if app.LoginAllowed(p.SchemaID, p.FlowType) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		d.Log.InfoContext(r.Context(), "login interrupted by population guard",
			"identity_id", p.IdentityID, "schema_id", p.SchemaID, "flow_type", p.FlowType, "request_id", RequestIDFrom(r.Context()))
		writeLoginInterrupt(w)
	}))
	return otelhttp.NewHandler(r, "identity-service.webhook")
}

// writeLoginInterrupt writes the Kratos interrupting-webhook error format.
func writeLoginInterrupt(w http.ResponseWriter) {
	type msg struct {
		ID   int    `json:"id"`
		Text string `json:"text"`
		Type string `json:"type"`
	}
	type entry struct {
		InstancePtr string `json:"instance_ptr"`
		Messages    []msg  `json:"messages"`
	}
	body := struct {
		Messages []entry `json:"messages"`
	}{Messages: []entry{{InstancePtr: "#/", Messages: []msg{{
		ID: LoginInterruptMessageID, Type: "error",
		Text: "This account cannot sign in here. Use the app made for your account type.",
	}}}}}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	_ = json.NewEncoder(w).Encode(body)
}

// Checker is a readiness dependency.
type Checker struct {
	Name  string
	Check func(ctx context.Context) error
}

// NewOpsHandler builds the :9090 handler (/healthz, /readyz, /metrics).
func NewOpsHandler(log *slog.Logger, m *Metrics, checks []Checker) http.Handler {
	r := chi.NewRouter()
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	r.Get("/readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		res := map[string]string{}
		ready := true
		for _, c := range checks {
			if err := c.Check(ctx); err != nil {
				ready = false
				res[c.Name] = "unavailable"
				log.WarnContext(ctx, "readiness check failed", "check", c.Name, "error", err.Error())
				continue
			}
			res[c.Name] = "ok"
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		if !ready {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ready": ready, "checks": res})
	})
	r.Handle("/metrics", promhttp.HandlerFor(m.Registry, promhttp.HandlerOpts{}))
	return r
}
