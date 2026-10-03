// Package machine holds the domain types of machine-to-machine access
// (intent 261003-add-ory-hydra): OAuth2 scopes, the authenticated service
// client principal and the service client registration rules.
package machine

import (
	"net/mail"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/profile"
)

// Scope is an OAuth2 scope of the machine plane (closed enum).
type Scope string

// Machine scopes. A route declares exactly one (httpapi.RoutePolicies).
const (
	ScopeCustomersRead Scope = "customers:read"
	ScopeAuditRead     Scope = "audit:read"
)

// AllScopes lists the scopes in canonical order.
var AllScopes = []Scope{ScopeCustomersRead, ScopeAuditRead}

// ParseScope validates a scope against the closed enum.
func ParseScope(s string) (Scope, bool) {
	sc := Scope(s)
	if slices.Contains(AllScopes, sc) {
		return sc, true
	}
	return "", false
}

// KnownScopes keeps the known scopes of raw in canonical order, dropping
// unknown and duplicate values.
func KnownScopes(raw []string) []Scope {
	out := make([]Scope, 0, len(AllScopes))
	for _, s := range AllScopes {
		if slices.Contains(raw, string(s)) {
			out = append(out, s)
		}
	}
	return out
}

// Principal is an authenticated service client. Handlers only ever see this,
// never the raw token.
type Principal struct {
	ClientID string
	Scopes   []Scope
	// TokenID is the token's jti, for the machine access log only.
	TokenID string
}

// HasScope reports whether the principal was granted s.
func (p Principal) HasScope(s Scope) bool { return s != "" && slices.Contains(p.Scopes, s) }

// ManagedBy is the metadata marker of clients owned by identity-service.
const ManagedBy = "identity-service"

// ServiceClient is a registered client_credentials client. The secret is
// never part of it.
type ServiceClient struct {
	ClientID  string
	Name      string
	Owner     string
	Scopes    []Scope
	CreatedAt time.Time
	// CreatedBy is the admin who registered it (uuid.Nil = system / CLI).
	CreatedBy *uuid.UUID
}

// Registration is the validated input for a new service client.
type Registration struct {
	Name   string
	Owner  string
	Scopes []Scope
}

// Name rules (contract CreateServiceClientRequest).
const (
	minNameLen  = 3
	maxNameLen  = 64
	maxOwnerLen = 254
)

var nameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// NewRegistration normalises and validates a registration. Field errors name
// the field and a code, never the value.
func NewRegistration(name, owner string, scopes []string) (Registration, error) {
	var errs []profile.FieldError
	r := Registration{Name: strings.TrimSpace(name), Owner: strings.ToLower(strings.TrimSpace(owner))}
	switch {
	case r.Name == "":
		errs = append(errs, profile.FieldError{Field: "name", Code: "required"})
	case len(r.Name) < minNameLen || len(r.Name) > maxNameLen:
		errs = append(errs, profile.FieldError{Field: "name", Code: "invalid_length"})
	case !nameRe.MatchString(r.Name):
		errs = append(errs, profile.FieldError{Field: "name", Code: "invalid_format"})
	}
	switch {
	case r.Owner == "":
		errs = append(errs, profile.FieldError{Field: "owner", Code: "required"})
	case len(r.Owner) > maxOwnerLen:
		errs = append(errs, profile.FieldError{Field: "owner", Code: "too_long"})
	case profile.HasControlChars(r.Owner):
		errs = append(errs, profile.FieldError{Field: "owner", Code: "invalid_characters"})
	default:
		if a, err := mail.ParseAddress(r.Owner); err != nil || a.Address != r.Owner {
			errs = append(errs, profile.FieldError{Field: "owner", Code: "invalid_format"})
		}
	}
	if len(scopes) == 0 {
		errs = append(errs, profile.FieldError{Field: "scopes", Code: "required"})
	} else {
		seen := map[string]bool{}
		bad := false
		for _, s := range scopes {
			if _, ok := ParseScope(s); !ok || seen[s] {
				bad = true
			}
			seen[s] = true
		}
		if bad {
			errs = append(errs, profile.FieldError{Field: "scopes", Code: "invalid"})
		} else {
			r.Scopes = KnownScopes(scopes)
		}
	}
	if len(errs) > 0 {
		return Registration{}, &profile.ValidationError{Fields: errs}
	}
	return r, nil
}

// ScopeString joins scopes for the OAuth2 "scope" parameter.
func ScopeString(s []Scope) string {
	parts := make([]string, len(s))
	for i, x := range s {
		parts[i] = string(x)
	}
	return strings.Join(parts, " ")
}
