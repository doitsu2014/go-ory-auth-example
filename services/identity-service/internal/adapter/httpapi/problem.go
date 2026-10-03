// Package httpapi is the HTTP adapter: the generated strict server
// implementation, middleware, per-route policies and problem+json mapping.
package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

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
}

// writeProblem writes an RFC 9457 problem. detail must never carry internals.
func writeProblem(w http.ResponseWriter, r *http.Request, code, detail string, fields []app.FieldError) {
	spec, ok := problemSpecs[code]
	if !ok {
		code, spec = CodeInternal, problemSpecs[CodeInternal]
	}
	p := gen.Problem{Type: problemBase + spec.slug, Title: spec.title, Status: spec.status, Code: code}
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
	if spec.status == http.StatusUnauthorized && planeOf(r) == PlaneCustomer {
		w.Header().Set("WWW-Authenticate", `Bearer realm="identity-service"`)
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
	switch {
	case errors.As(err, &ve):
		return CodeValidationFailed, ve.Fields
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
