// Package httpapi is the HTTP adapter: the generated strict server
// implementation, middleware, per-route policies and problem+json mapping.
package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/adapter/httpapi/gen"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/profile"
)

const problemBase = "https://docs.go-ory-auth-example.local/problems/"

// Problem codes (docs/principles/05-api-guidelines.md §3).
const (
	CodeUnauthenticated       = "unauthenticated"
	CodeForbidden             = "forbidden"
	CodeNotAdmin              = "not_admin"
	CodeAAL2Required          = "aal2_required"
	CodeMFAEnrollmentRequired = "mfa_enrollment_required"
	CodeEmailNotVerified      = "email_not_verified"
	CodeNotFound              = "not_found"
	CodeConflict              = "conflict"
	CodeValidationFailed      = "validation_failed"
	CodeDependencyUnavailable = "dependency_unavailable"
	CodeRateLimited           = "rate_limited"
	CodeInternal              = "internal"
	CodeMethodNotAllowed      = "method_not_allowed"
	// CodeAuthFlowRejected: Kratos rejected a customer login, registration
	// or recovery (ADR-0014); errors[].code is the Kratos message id.
	CodeAuthFlowRejected = "auth_flow_rejected"
	// CodeAuthFlowExpired: the Kratos flow behind a recovery expired.
	CodeAuthFlowExpired = "auth_flow_expired"
	// Machine plane (RFC 6750 §3.1 error codes, also in WWW-Authenticate).
	CodeInvalidRequest    = "invalid_request"
	CodeInvalidToken      = "invalid_token"
	CodeInsufficientScope = "insufficient_scope"
)

type problemSpec struct {
	status int
	title  string
	slug   string
}

var problemSpecs = map[string]problemSpec{
	CodeUnauthenticated:       {http.StatusUnauthorized, "Authentication required", "unauthenticated"},
	CodeForbidden:             {http.StatusForbidden, "Forbidden", "forbidden"},
	CodeNotAdmin:              {http.StatusForbidden, "Not an admin", "not-admin"},
	CodeAAL2Required:          {http.StatusForbidden, "Second factor required", "aal2-required"},
	CodeMFAEnrollmentRequired: {http.StatusForbidden, "MFA enrolment required", "mfa-enrollment-required"},
	CodeEmailNotVerified:      {http.StatusForbidden, "Email not verified", "email-not-verified"},
	CodeNotFound:              {http.StatusNotFound, "Not found", "not-found"},
	CodeConflict:              {http.StatusConflict, "Conflict", "conflict"},
	CodeValidationFailed:      {http.StatusUnprocessableEntity, "Validation failed", "validation-failed"},
	CodeDependencyUnavailable: {http.StatusServiceUnavailable, "Dependency unavailable", "dependency-unavailable"},
	CodeRateLimited:           {http.StatusTooManyRequests, "Too many requests", "rate-limited"},
	CodeInternal:              {http.StatusInternalServerError, "Internal error", "internal"},
	CodeMethodNotAllowed:      {http.StatusMethodNotAllowed, "Method not allowed", "method-not-allowed"},
	CodeAuthFlowRejected:      {http.StatusBadRequest, "Authentication rejected", "auth-flow-rejected"},
	CodeAuthFlowExpired:       {http.StatusGone, "Flow expired", "auth-flow-expired"},
	CodeInvalidRequest:        {http.StatusBadRequest, "Invalid request", "invalid-request"},
	CodeInvalidToken:          {http.StatusUnauthorized, "Invalid access token", "invalid-token"},
	CodeInsufficientScope:     {http.StatusForbidden, "Insufficient scope", "insufficient-scope"},
}

// writeProblem writes an RFC 9457 problem. detail must never carry internals.
func writeProblem(w http.ResponseWriter, r *http.Request, code, detail string, fields []app.FieldError) {
	spec, ok := problemSpecs[code]
	if !ok {
		code, spec = CodeInternal, problemSpecs[CodeInternal]
	}
	p := gen.Problem{Type: problemBase + spec.slug, Title: spec.title, Status: spec.status, Code: code}
	if code == CodeRateLimited && w.Header().Get("Retry-After") == "" {
		// Sliding windows have no single reset time; one minute is the
		// shortest window of every limit (api-contract §3).
		w.Header().Set("Retry-After", "60")
	}
	if detail != "" {
		p.Detail = &detail
	}
	if rid := RequestIDFrom(r.Context()); rid != "" {
		p.RequestId = &rid
	}
	if len(fields) > 0 {
		fe := make([]gen.FieldError, len(fields))
		for i, f := range fields {
			fe[i] = gen.FieldError{Field: f.Field, Code: f.Code}
		}
		p.Errors = &fe
	}
	switch plane := planeOf(r); {
	case spec.status == http.StatusUnauthorized && plane == PlaneCustomer:
		w.Header().Set("WWW-Authenticate", `Bearer realm="identity-service"`)
	case plane == PlaneMachine:
		setMachineChallenge(w, r, code)
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(spec.status)
	_ = json.NewEncoder(w).Encode(p)
	setProblemCode(r.Context(), code)
}

// classify maps an error to a problem code. This is the only place domain
// and app errors become HTTP semantics.
func classify(err error) (code string, fields []app.FieldError) {
	var ve *app.ValidationError
	var fe *app.AuthFlowError
	switch {
	case errors.As(err, &ve):
		return CodeValidationFailed, ve.Fields
	case errors.As(err, &fe):
		fields := make([]app.FieldError, len(fe.Messages))
		for i, m := range fe.Messages {
			fields[i] = app.FieldError{Field: m.Field, Code: strconv.Itoa(m.ID)}
		}
		return CodeAuthFlowRejected, fields
	case errors.Is(err, app.ErrAuthFlowExpired):
		return CodeAuthFlowExpired, nil
	case errors.Is(err, app.ErrUnauthenticated):
		return CodeUnauthenticated, nil
	case errors.Is(err, app.ErrAAL2Required):
		return CodeAAL2Required, nil
	case errors.Is(err, app.ErrMFAEnrollmentRequired):
		return CodeMFAEnrollmentRequired, nil
	case errors.Is(err, app.ErrNotAdmin):
		return CodeNotAdmin, nil
	case errors.Is(err, app.ErrEmailNotVerified):
		return CodeEmailNotVerified, nil
	case errors.Is(err, app.ErrForbidden):
		return CodeForbidden, nil
	case errors.Is(err, app.ErrNotFound), errors.Is(err, profile.ErrNotFound):
		return CodeNotFound, nil
	case errors.Is(err, app.ErrConflict):
		return CodeConflict, nil
	case errors.Is(err, app.ErrDependencyUnavailable):
		return CodeDependencyUnavailable, nil
	case errors.Is(err, app.ErrRateLimited):
		return CodeRateLimited, nil
	case errors.Is(err, app.ErrInvalidToken):
		return CodeInvalidToken, nil
	case errors.Is(err, app.ErrInsufficientScope):
		return CodeInsufficientScope, nil
	}
	return CodeInternal, nil
}

// errorWriter writes errors as problems and logs the ones that are ours.
type errorWriter struct{ log *slog.Logger }

func (e errorWriter) write(w http.ResponseWriter, r *http.Request, err error) {
	code, fields := classify(err)
	switch code {
	case CodeInternal:
		e.log.ErrorContext(r.Context(), "request failed", "error", err.Error(), "request_id", RequestIDFrom(r.Context()))
	case CodeDependencyUnavailable:
		e.log.WarnContext(r.Context(), "dependency unavailable", "error", err.Error(), "request_id", RequestIDFrom(r.Context()))
	}
	writeProblem(w, r, code, "", fields)
}

// requestError handles body decode errors from the strict server.
func (e errorWriter) requestError(w http.ResponseWriter, r *http.Request, err error) {
	writeProblem(w, r, CodeValidationFailed, "The request body is not valid JSON for this operation.", []app.FieldError{{Field: "body", Code: "invalid"}})
}

// paramError handles parameter binding errors from the generated wrapper.
func (e errorWriter) paramError(w http.ResponseWriter, r *http.Request, err error) {
	var inv *gen.InvalidParamFormatError
	if errors.As(err, &inv) && inv.ParamName == "id" {
		writeProblem(w, r, CodeNotFound, "", nil)
		return
	}
	field, code := "parameter", "invalid"
	var req *gen.RequiredHeaderError
	var reqP *gen.RequiredParamError
	switch {
	case errors.As(err, &inv):
		field = inv.ParamName
	case errors.As(err, &req):
		field, code = req.ParamName, "required"
	case errors.As(err, &reqP):
		field, code = reqP.ParamName, "required"
	}
	writeProblem(w, r, CodeValidationFailed, "", []app.FieldError{{Field: field, Code: code}})
}

// setMachineChallenge sets the RFC 6750 §3 challenge on machine-plane 401
// and 403 responses. Values are fixed strings or a scope from the closed
// enum; nothing from the request is echoed.
func setMachineChallenge(w http.ResponseWriter, r *http.Request, code string) {
	const realm = `Bearer realm="identity-service"`
	switch code {
	case CodeUnauthenticated:
		w.Header().Set("WWW-Authenticate", realm)
	case CodeInvalidRequest:
		w.Header().Set("WWW-Authenticate", realm+`, error="invalid_request"`)
	case CodeInvalidToken:
		w.Header().Set("WWW-Authenticate", realm+`, error="invalid_token"`)
	case CodeInsufficientScope:
		v := realm + `, error="insufficient_scope"`
		if sc := requiredScopeFrom(r.Context()); sc != "" {
			v += `, scope="` + string(sc) + `"`
		}
		w.Header().Set("WWW-Authenticate", v)
	}
}
