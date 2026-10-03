package app

import (
	"errors"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/profile"
)

// Sentinel errors. The HTTP adapter maps them to problem codes in one place.
var (
	ErrUnauthenticated       = errors.New("unauthenticated")
	ErrForbidden             = errors.New("forbidden")
	ErrNotAdmin              = errors.New("not admin")
	ErrAAL2Required          = errors.New("aal2 required")
	ErrMFAEnrollmentRequired = errors.New("mfa enrollment required")
	ErrEmailNotVerified      = errors.New("email not verified")
	ErrNotFound              = errors.New("not found")
	ErrConflict              = errors.New("conflict")
	ErrDependencyUnavailable = errors.New("dependency unavailable")
	ErrRateLimited           = errors.New("rate limited")
	// ErrDataIntegrity means stored personal info or a wrapped key failed
	// authentication (tampering, row/column swap, wrong key). It maps to
	// 500 and never carries values.
	ErrDataIntegrity = errors.New("data integrity check failed")
	// ErrInvalidToken is a machine-plane bearer token that failed
	// verification (401 invalid_token). It never carries token contents.
	ErrInvalidToken = errors.New("invalid token")
	// ErrInsufficientScope is a valid machine token without the route's
	// scope (403 insufficient_scope).
	ErrInsufficientScope = errors.New("insufficient scope")
)

// FieldError is one invalid input field.
type FieldError = profile.FieldError

// ValidationError carries field errors (422 validation_failed).
type ValidationError = profile.ValidationError

// NewValidationError builds a single-field validation error.
func NewValidationError(field, code string) error {
	return &ValidationError{Fields: []FieldError{{Field: field, Code: code}}}
}
