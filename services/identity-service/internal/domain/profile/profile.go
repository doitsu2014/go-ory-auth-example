// Package profile holds the app-specific profile entity and its rules.
package profile

import (
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/identity"
)

// ErrNotFound is returned when no profile exists.
var ErrNotFound = errors.New("profile not found")

// Limits mirror the DB CHECK constraints and the OpenAPI schema.
const (
	MaxDisplayName = 100
	MaxAvatarURL   = 2048
	MaxLocale      = 35
	DefaultLocale  = "vi-VN"
)

// Profile is app data keyed by the Kratos identity id.
type Profile struct {
	IdentityID  uuid.UUID
	Kind        identity.Kind
	DisplayName *string
	AvatarURL   *string
	Locale      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// OptionalString is a PATCH field: unset, set to null, or set to a value.
type OptionalString struct {
	Set   bool
	Value *string
}

// Patch is a partial update of a profile.
type Patch struct {
	DisplayName OptionalString
	AvatarURL   OptionalString
	Locale      *string
}

// FieldError describes one invalid field.
type FieldError struct {
	Field string
	Code  string
}

// ValidationError aggregates field errors.
type ValidationError struct{ Fields []FieldError }

func (e *ValidationError) Error() string { return "validation failed" }

// Validate checks the patch against the domain rules.
func (p Patch) Validate() error {
	var errs []FieldError
	check := func(field string, v *string, maxLen int) {
		if v == nil {
			return
		}
		if utf8.RuneCountInString(*v) > maxLen {
			errs = append(errs, FieldError{Field: field, Code: "too_long"})
		}
		if HasControlChars(*v) {
			errs = append(errs, FieldError{Field: field, Code: "invalid_characters"})
		}
	}
	if p.DisplayName.Set {
		check("display_name", p.DisplayName.Value, MaxDisplayName)
	}
	if p.AvatarURL.Set {
		check("avatar_url", p.AvatarURL.Value, MaxAvatarURL)
		if v := p.AvatarURL.Value; v != nil && *v != "" &&
			!strings.HasPrefix(*v, "https://") && !strings.HasPrefix(*v, "http://") {
			errs = append(errs, FieldError{Field: "avatar_url", Code: "invalid_url"})
		}
	}
	if p.Locale != nil {
		check("locale", p.Locale, MaxLocale)
		if strings.TrimSpace(*p.Locale) == "" {
			errs = append(errs, FieldError{Field: "locale", Code: "required"})
		}
	}
	if len(errs) > 0 {
		return &ValidationError{Fields: errs}
	}
	return nil
}

// HasControlChars reports NUL and other control characters (also invalid
// UTF-8), which are never valid in profile or name fields.
func HasControlChars(s string) bool {
	if !utf8.ValidString(s) {
		return true
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

// Apply returns a copy of the profile with the patch applied.
func (p Profile) Apply(patch Patch) Profile {
	out := p
	if patch.DisplayName.Set {
		out.DisplayName = patch.DisplayName.Value
	}
	if patch.AvatarURL.Set {
		out.AvatarURL = patch.AvatarURL.Value
	}
	if patch.Locale != nil {
		out.Locale = *patch.Locale
	}
	return out
}
