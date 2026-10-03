package machine

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/profile"
)

func fieldCodes(t *testing.T, err error) map[string]string {
	t.Helper()
	var ve *profile.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want validation error, got %v", err)
	}
	out := map[string]string{}
	for _, f := range ve.Fields {
		out[f.Field] = f.Code
	}
	return out
}

func TestNewRegistrationValid(t *testing.T) {
	r, err := NewRegistration(" billing-sync ", " Ops@Example.COM ", []string{"audit:read", "customers:read"})
	if err != nil {
		t.Fatal(err)
	}
	if r.Name != "billing-sync" || r.Owner != "ops@example.com" {
		t.Fatalf("normalisation: %+v", r)
	}
	if !slices.Equal(r.Scopes, []Scope{ScopeCustomersRead, ScopeAuditRead}) {
		t.Fatalf("canonical scope order: %v", r.Scopes)
	}
}

func TestNewRegistrationRejects(t *testing.T) {
	tests := []struct {
		name, owner string
		scopes      []string
		field, code string
	}{
		{"", "a@b.co", []string{"audit:read"}, "name", "required"},
		{"ab", "a@b.co", []string{"audit:read"}, "name", "invalid_length"},
		{strings.Repeat("a", 65), "a@b.co", []string{"audit:read"}, "name", "invalid_length"},
		{"-abc", "a@b.co", []string{"audit:read"}, "name", "invalid_format"},
		{"Billing", "a@b.co", []string{"audit:read"}, "name", "invalid_format"},
		{"bill_ing", "a@b.co", []string{"audit:read"}, "name", "invalid_format"},
		{"billing", "", []string{"audit:read"}, "owner", "required"},
		{"billing", "not-an-email", []string{"audit:read"}, "owner", "invalid_format"},
		{"billing", "Ops <a@b.co>", []string{"audit:read"}, "owner", "invalid_format"},
		{"billing", "a\x00@b.co", []string{"audit:read"}, "owner", "invalid_characters"},
		{"billing", strings.Repeat("a", 250) + "@b.co", []string{"audit:read"}, "owner", "too_long"},
		{"billing", "a@b.co", nil, "scopes", "required"},
		{"billing", "a@b.co", []string{"admin"}, "scopes", "invalid"},
		{"billing", "a@b.co", []string{"audit:read", "audit:read"}, "scopes", "invalid"},
		{"billing", "a@b.co", []string{"offline_access"}, "scopes", "invalid"},
	}
	for _, tt := range tests {
		_, err := NewRegistration(tt.name, tt.owner, tt.scopes)
		got := fieldCodes(t, err)
		if got[tt.field] != tt.code {
			t.Errorf("%q/%q/%v: %s=%q want %q (%v)", tt.name, tt.owner, tt.scopes, tt.field, got[tt.field], tt.code, got)
		}
	}
}

func TestScopes(t *testing.T) {
	if _, ok := ParseScope("customers:write"); ok {
		t.Fatal("unknown scope parsed")
	}
	if got := KnownScopes([]string{"x", "audit:read", "audit:read"}); !slices.Equal(got, []Scope{ScopeAuditRead}) {
		t.Fatalf("known scopes: %v", got)
	}
	p := Principal{ClientID: "c", Scopes: []Scope{ScopeAuditRead}}
	if !p.HasScope(ScopeAuditRead) || p.HasScope(ScopeCustomersRead) || p.HasScope("") {
		t.Fatal("HasScope")
	}
	if ScopeString([]Scope{ScopeCustomersRead, ScopeAuditRead}) != "customers:read audit:read" {
		t.Fatal("ScopeString")
	}
}
