// Package login holds the customer login identifier (email or phone number):
// normalisation, validation, the pseudonym that replaces it in Kratos, the
// associated data that binds its ciphertext, masking, and the messages sent
// to it (PLI-FR-01, ADR-0013).
//
// An Identifier redacts itself when formatted, logged (slog.LogValuer) or
// JSON-encoded, so an accidental log line or error cannot leak the address.
// The plaintext is only reachable through Value, used by the encryption path.
package login

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"strings"
	"unicode"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/pii"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/profile"
)

// Kind is the type of a login identifier.
type Kind string

// Kinds.
const (
	KindEmail Kind = "email"
	KindPhone Kind = "phone"
)

// ParseKind validates a kind.
func ParseKind(s string) (Kind, bool) {
	switch Kind(s) {
	case KindEmail, KindPhone:
		return Kind(s), true
	}
	return "", false
}

// Limits and field error codes (api-contract §3).
const (
	MaxRawLength   = 320
	MaxEmailLength = 254

	CodeInvalidFormat      = "invalid_format"
	CodeTooLong            = "too_long"
	CodeInvalidCharacters  = "invalid_characters"
	CodeUnsupportedCountry = "unsupported_country"
)

// ErrInvalid is returned by Parse; Code is one of the field error codes. It
// never carries the input.
type ErrInvalid struct{ Code string }

func (e *ErrInvalid) Error() string { return "login identifier: " + e.Code }

// IsInvalid reports whether err is an *ErrInvalid and returns its code.
func IsInvalid(err error) (string, bool) {
	var e *ErrInvalid
	if errors.As(err, &e) {
		return e.Code, true
	}
	return "", false
}

// reservedDomains can never be a customer's email: the pseudonym domain and
// the local SMS sink domain.
var reservedDomains = map[string]bool{Domain: true, SMSSinkDomain: true}

// SMSSinkDomain is the domain of the local SMS sink addresses (Mailpit).
const SMSSinkDomain = "sms.local"

// PhonePolicy says how national numbers are read and which countries are
// accepted (A3).
type PhonePolicy struct {
	// DefaultCountry is the calling code (digits) used for a leading "0".
	DefaultCountry string
	// AllowedCountries are calling codes (digits); empty allows none.
	AllowedCountries []string
	// AnyCountry accepts every calling code (admin lookups of numbers
	// registered before an allow-list change).
	AnyCountry bool
}

// Identifier is a normalised login identifier: a lower-cased email address
// or an E.164 phone number.
type Identifier struct {
	kind  Kind
	value string
}

// Kind returns the identifier kind.
func (i Identifier) Kind() Kind { return i.kind }

// Value returns the normalised plaintext. Callers must not log it.
func (i Identifier) Value() string { return i.value }

// IsZero reports whether the identifier is unset.
func (i Identifier) IsZero() bool { return i.kind == "" }

// FromStored rebuilds an identifier from a decrypted vault value. The value
// was normalised when it was stored; it is checked again so a corrupt row
// cannot produce an unexpected identifier.
func FromStored(kind Kind, value string) (Identifier, error) {
	switch kind {
	case KindEmail:
		if v, err := parseEmail(value); err == nil && v == value {
			return Identifier{kind: kind, value: v}, nil
		}
	case KindPhone:
		if pii.ValidPhone(value) {
			return Identifier{kind: kind, value: value}, nil
		}
	}
	return Identifier{}, &ErrInvalid{Code: CodeInvalidFormat}
}

// Parse normalises and validates what the customer typed.
func Parse(kind Kind, raw string, p PhonePolicy) (Identifier, error) {
	if len(raw) > MaxRawLength {
		return Identifier{}, &ErrInvalid{Code: CodeTooLong}
	}
	if profile.HasControlChars(raw) || hasFormatChars(raw) {
		return Identifier{}, &ErrInvalid{Code: CodeInvalidCharacters}
	}
	switch kind {
	case KindEmail:
		v, err := parseEmail(raw)
		if err != nil {
			return Identifier{}, err
		}
		return Identifier{kind: KindEmail, value: v}, nil
	case KindPhone:
		v, err := parsePhone(raw, p)
		if err != nil {
			return Identifier{}, err
		}
		return Identifier{kind: KindPhone, value: v}, nil
	}
	return Identifier{}, &ErrInvalid{Code: CodeInvalidFormat}
}

func parseEmail(raw string) (string, error) {
	s := strings.ToLower(strings.TrimSpace(raw))
	if s == "" {
		return "", &ErrInvalid{Code: CodeInvalidFormat}
	}
	if len(s) > MaxEmailLength {
		return "", &ErrInvalid{Code: CodeTooLong}
	}
	addr, err := mail.ParseAddress(s)
	if err != nil || addr.Name != "" || addr.Address != s {
		return "", &ErrInvalid{Code: CodeInvalidFormat}
	}
	at := strings.LastIndexByte(s, '@')
	domain := s[at+1:]
	if at <= 0 || !strings.Contains(domain, ".") || reservedDomains[domain] {
		return "", &ErrInvalid{Code: CodeInvalidFormat}
	}
	return s, nil
}

func parsePhone(raw string, p PhonePolicy) (string, error) {
	s := strings.Map(func(r rune) rune {
		switch r {
		case ' ', '-', '.', '(', ')', '\t':
			return -1
		}
		return r
	}, strings.TrimSpace(raw))
	switch {
	case strings.HasPrefix(s, "+"):
	case strings.HasPrefix(s, "00"):
		s = "+" + s[2:]
	case strings.HasPrefix(s, "0") && p.DefaultCountry != "":
		s = "+" + p.DefaultCountry + s[1:]
	default:
		return "", &ErrInvalid{Code: CodeInvalidFormat}
	}
	if !pii.ValidPhone(s) {
		return "", &ErrInvalid{Code: CodeInvalidFormat}
	}
	if p.AnyCountry {
		return s, nil
	}
	cc := pii.CallingCode(s)
	for _, a := range p.AllowedCountries {
		if a == cc {
			return s, nil
		}
	}
	return "", &ErrInvalid{Code: CodeUnsupportedCountry}
}

func hasFormatChars(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Cf, r) {
			return true
		}
	}
	return false
}

// Mask returns the admin (masked) form: email "a***@e***.vn" (first character
// of the local part and of the first domain label, top-level label kept),
// phone "+84*******567". The shape is fixed so the length does not leak.
func (i Identifier) Mask() string {
	switch i.kind {
	case KindEmail:
		at := strings.LastIndexByte(i.value, '@')
		if at <= 0 {
			return "***"
		}
		local, domain := i.value[:at], i.value[at+1:]
		labels := strings.Split(domain, ".")
		return firstRune(local) + "***@" + firstRune(labels[0]) + "***." + labels[len(labels)-1]
	case KindPhone:
		return pii.MaskPhone(i.value)
	}
	return "***"
}

func firstRune(s string) string {
	for _, r := range s {
		return string(r)
	}
	return ""
}

// --- redaction ---

// Format implements fmt.Formatter.
func (i Identifier) Format(f fmt.State, _ rune) { _, _ = f.Write([]byte(pii.Redacted)) }

// String implements fmt.Stringer.
func (i Identifier) String() string { return pii.Redacted }

// LogValue implements slog.LogValuer.
func (i Identifier) LogValue() slog.Value { return slog.StringValue(pii.Redacted) }

// MarshalJSON implements json.Marshaler.
func (i Identifier) MarshalJSON() ([]byte, error) { return json.Marshal(pii.Redacted) }
