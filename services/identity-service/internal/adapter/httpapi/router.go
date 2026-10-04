package httpapi

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
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
	// MachineVerifier enables /m2m/v1 (nil: the machine plane answers 503).
	MachineVerifier app.MachineTokenVerifier
	// MachineClientLimiter / MachineFailureLimiter default to 600 requests
	// per client and 60 rejected authentications per IP, per minute.
	MachineClientLimiter  *app.RateLimiter
	MachineFailureLimiter *app.RateLimiter
}

// NewPublicHandler builds the :8080 handler (/v1, /admin/v1, /m2m/v1). It
// fails if any route lacks a valid policy (P4).
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
	if d.MachineVerifier != nil {
		if d.MachineClientLimiter == nil {
			d.MachineClientLimiter = NewMachineClientLimiter(DefaultMachineRatePerMin, nil)
		}
		if d.MachineFailureLimiter == nil {
			d.MachineFailureLimiter = NewMachineFailureLimiter(nil)
		}
		guard.Machine = &MachineGuard{
			Verifier: d.MachineVerifier, Policies: d.Policies, Router: api, ClientLimiter: d.MachineClientLimiter,
			FailureLimiter: d.MachineFailureLimiter, TrustedHops: d.TrustedHops, Errors: ew, Log: d.Log,
		}
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
	// LoginID is set by the after-registration template (ADR-0013).
	LoginID string `json:"login_id,omitempty"`
}

// KratosPreRegistrationPayload mirrors pre-registration.jsonnet.
type KratosPreRegistrationPayload struct {
	SchemaID string  `json:"schema_id"`
	FlowType string  `json:"flow_type"`
	LoginID  *string `json:"login_id"`
	Email    *string `json:"email"`
}

// KratosCourierPayload mirrors courier.jsonnet (api-contract §10).
type KratosCourierPayload struct {
	Recipient        string     `json:"recipient"`
	TemplateType     string     `json:"template_type"`
	IdentityID       *uuid.UUID `json:"identity_id"`
	Code             *string    `json:"code"`
	ExpiresInMinutes *int       `json:"expires_in_minutes"`
}

// WebhookDeps wires the :8081 router.
type WebhookDeps struct {
	Log          *slog.Logger
	Metrics      *Metrics
	APIKey       string
	Provisioning *app.ProvisioningService
	// Logins checks registrations and binds logins (ADR-0013); Courier
	// delivers Kratos messages, authenticated with CourierAPIKey (A4).
	Logins        *app.LoginIdentifierService
	Courier       *app.CourierDispatcher
	CourierAPIKey string
}

// Pre-registration message ids (api-contract §8). 4049xxx is an unused slot
// of Kratos's registration range; 4000007 is Kratos's own "an account with
// the same identifier exists already".
const (
	PreRegLegacyMessageID     = 4049001
	PreRegUnresolvedMessageID = 4049002
	PreRegDuplicateMessageID  = 4000007
)

// LoginInterruptMessageID is the message id shown when the population guard
// rejects a login (custom id, outside Kratos' own ranges' semantics).
const LoginInterruptMessageID = 4000001

// NewWebhookHandler builds the :8081 handler (Kratos webhooks only).
func NewWebhookHandler(d WebhookDeps) http.Handler {
	r := chi.NewRouter()
	r.Use(requestState, recoverer(d.Log), accessLog(d.Log, d.Metrics, "webhook"), limits(64<<10, 5*time.Second))
	r.NotFound(notFound)
	r.MethodNotAllowed(methodNotAllowed)
	keyed := func(key string) func(http.HandlerFunc) http.HandlerFunc {
		want := sha256.Sum256([]byte(key))
		return func(next http.HandlerFunc) http.HandlerFunc {
			return func(w http.ResponseWriter, r *http.Request) {
				got := sha256.Sum256([]byte(r.Header.Get("Authorization")))
				if key == "" || subtle.ConstantTimeCompare(got[:], want[:]) != 1 {
					writeProblem(w, r, CodeUnauthenticated, "", nil)
					return
				}
				next(w, r)
			}
		}
	}
	auth := keyed(d.APIKey)
	courierAuth := keyed(d.CourierAPIKey)
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
		// Bind the login vault entry (A1). Failures are repaired lazily on
		// GET /v1/me; the webhook response is ignored by Kratos anyway.
		if d.Logins != nil && p.SchemaID == "customer" {
			if err := d.Logins.Bind(r.Context(), p.IdentityID, p.LoginID); err != nil {
				d.Log.WarnContext(r.Context(), "login_bind_failed", "identity_id", p.IdentityID.String(), "error", err.Error())
			}
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	r.Post("/internal/hooks/kratos/pre-registration", auth(func(w http.ResponseWriter, r *http.Request) {
		var p KratosPreRegistrationPayload
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&p); err != nil {
			writeProblem(w, r, CodeValidationFailed, "", []app.FieldError{{Field: "body", Code: "invalid"}})
			return
		}
		if p.SchemaID != "customer" {
			writeEmptyJSON(w)
			return
		}
		if d.Logins == nil { // fail closed (SEC-C08)
			writeProblem(w, r, CodeDependencyUnavailable, "", nil)
			return
		}
		err := d.Logins.ValidateRegistration(r.Context(), app.RegistrationTraits{LoginID: deref(p.LoginID), LegacyEmail: deref(p.Email)})
		switch {
		case err == nil:
			d.Metrics.PreRegistration("allowed")
			writeEmptyJSON(w)
		case errors.Is(err, app.ErrLoginLegacyTraits):
			d.Metrics.PreRegistration("legacy_traits")
			writeKratosFieldError(w, PreRegLegacyMessageID, "Update the app to sign up.")
		case errors.Is(err, app.ErrLoginUnresolved):
			d.Metrics.PreRegistration("unresolved")
			writeKratosFieldError(w, PreRegUnresolvedMessageID, "We could not confirm this email or phone number. Please try again.")
		case errors.Is(err, app.ErrLoginDuplicate):
			d.Metrics.PreRegistration("duplicate")
			writeKratosFieldError(w, PreRegDuplicateMessageID, "An account with the same identifier (email, phone, username, ...) exists already.")
		default:
			d.Metrics.PreRegistration("error")
			d.Log.ErrorContext(r.Context(), "pre_registration_failed", "error", err.Error(), "request_id", RequestIDFrom(r.Context()))
			writeProblem(w, r, CodeDependencyUnavailable, "", nil)
		}
	}))
	r.Post("/internal/hooks/kratos/courier", courierAuth(func(w http.ResponseWriter, r *http.Request) {
		var p KratosCourierPayload
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if d.Courier == nil { // not configured: let Kratos keep the message (SEC-C08)
			writeProblem(w, r, CodeDependencyUnavailable, "", nil)
			return
		}
		if err := dec.Decode(&p); err != nil {
			// Undecodable: permanent, acknowledge so Kratos does not retry.
			d.Metrics.CourierOutcome(string(app.CourierDropped), app.DropBadPayload, "")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		m := app.CourierMessage{Recipient: p.Recipient, TemplateType: p.TemplateType, Code: deref(p.Code)}
		if p.IdentityID != nil {
			m.IdentityID = *p.IdentityID
		}
		if p.ExpiresInMinutes != nil {
			m.ExpiresInMinutes = *p.ExpiresInMinutes
		}
		res, err := d.Courier.Dispatch(r.Context(), m)
		if err != nil {
			d.Metrics.CourierOutcome("error", "", "")
			d.Log.ErrorContext(r.Context(), "courier_dispatch_failed", "error", err.Error(), "request_id", RequestIDFrom(r.Context()))
			writeProblem(w, r, CodeDependencyUnavailable, "", nil)
			return
		}
		d.Metrics.CourierOutcome(string(res.Outcome), res.Reason, res.Channel)
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

func writeEmptyJSON(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("{}"))
}

// writeKratosFieldError rejects a registration with a message on the
// login_id node (Kratos interrupting-webhook format, spike S3).
func writeKratosFieldError(w http.ResponseWriter, id int, text string) {
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
	}{Messages: []entry{{InstancePtr: "#/traits/login_id", Messages: []msg{{ID: id, Type: "error", Text: text}}}}}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
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
