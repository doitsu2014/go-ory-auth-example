package login

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

var vn = PhonePolicy{DefaultCountry: "84", AllowedCountries: []string{"84"}}

func TestPLIFR01_ParseEmail(t *testing.T) {
	for _, tc := range []struct{ in, want, code string }{
		{"  Alice@Example.COM ", "alice@example.com", ""},
		{"a.b+tag@sub.example.com.vn", "a.b+tag@sub.example.com.vn", ""},
		{"", "", CodeInvalidFormat},
		{"alice", "", CodeInvalidFormat},
		{"Alice <alice@example.com>", "", CodeInvalidFormat},
		{"alice@localhost", "", CodeInvalidFormat},
		{"x@login.invalid", "", CodeInvalidFormat},
		{"x@sms.local", "", CodeInvalidFormat},
		{"al​ice@example.com", "", CodeInvalidCharacters},
		{"al\x00ice@example.com", "", CodeInvalidCharacters},
		{strings.Repeat("a", 250) + "@ex.com", "", CodeTooLong},
		{strings.Repeat("a", 321), "", CodeTooLong},
	} {
		id, err := Parse(KindEmail, tc.in, vn)
		if tc.code != "" {
			if code, ok := IsInvalid(err); !ok || code != tc.code {
				t.Errorf("%q: err %v, want %s", tc.in, err, tc.code)
			}
			continue
		}
		if err != nil || id.Value() != tc.want || id.Kind() != KindEmail {
			t.Errorf("%q: got %q %v", tc.in, id.Value(), err)
		}
	}
}

func TestPLIFR01_ParsePhone(t *testing.T) {
	for _, tc := range []struct{ in, want, code string }{
		{"0901 234 567", "+84901234567", ""},
		{"+84 (90) 123-4567", "+84901234567", ""},
		{"0084901234567", "+84901234567", ""},
		{"84901234567", "", CodeInvalidFormat},
		{"+1 202 555 0100", "", CodeUnsupportedCountry},
		{"+84123", "", CodeInvalidFormat},
		{"abc", "", CodeInvalidFormat},
	} {
		id, err := Parse(KindPhone, tc.in, vn)
		if tc.code != "" {
			if code, ok := IsInvalid(err); !ok || code != tc.code {
				t.Errorf("%q: err %v, want %s", tc.in, err, tc.code)
			}
			continue
		}
		if err != nil || id.Value() != tc.want || id.Kind() != KindPhone {
			t.Errorf("%q: got %q %v", tc.in, id.Value(), err)
		}
	}
	if _, err := Parse(KindPhone, "0901234567", PhonePolicy{DefaultCountry: "84"}); err == nil {
		t.Fatal("empty allow-list must reject every country")
	}
	if _, err := Parse("username", "x", vn); err == nil {
		t.Fatal("unknown kind accepted")
	}
}

func TestFromStored(t *testing.T) {
	if _, err := FromStored(KindEmail, "alice@example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := FromStored(KindEmail, "Alice@example.com"); err == nil {
		t.Fatal("non-normalised stored email accepted")
	}
	if _, err := FromStored(KindPhone, "0901234567"); err == nil {
		t.Fatal("non-E.164 stored phone accepted")
	}
}

func TestPLIFR01_PseudonymRoundTrip(t *testing.T) {
	var p Pseudonym
	for i := range p {
		p[i] = byte(i * 7)
	}
	s := p.String()
	if !strings.HasSuffix(s, "@login.invalid") || len(s) != 52+1+len(Domain) {
		t.Fatalf("shape %q", s)
	}
	got, ok := ParsePseudonym(strings.ToUpper(s))
	if !ok || got != p {
		t.Fatal("round trip failed")
	}
	for _, bad := range []string{
		"alice@example.com",
		strings.Repeat("a", 52) + "@other.invalid",
		strings.Repeat("a", 51) + "@login.invalid",
		strings.Repeat("1", 52) + "@login.invalid",
		s[:51] + "b@login.invalid", // last char carries 1 bit: only 'a'/'q' are canonical
	} {
		if _, ok := ParsePseudonym(bad); ok {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestPLIFR01_InputDomainSeparation(t *testing.T) {
	e, _ := Parse(KindEmail, "84901234567@example.com", vn)
	p, _ := Parse(KindPhone, "+84901234567", vn)
	if !bytes.HasPrefix(LookupInput(e), []byte("login-id/v1\x00email\x00")) {
		t.Fatalf("email input %q", LookupInput(e))
	}
	if !bytes.Equal(LookupInput(p), []byte("login-id/v1\x00phone\x00+84901234567")) {
		t.Fatalf("phone input %q", LookupInput(p))
	}
}

func TestPLINFR03_AADBindsKindAndPseudonym(t *testing.T) {
	var a, b Pseudonym
	b[0] = 1
	if bytes.Equal(AAD(KindEmail, a), AAD(KindPhone, a)) || bytes.Equal(AAD(KindEmail, a), AAD(KindEmail, b)) {
		t.Fatal("AAD must differ by kind and pseudonym")
	}
}

func TestPLIFR10_Mask(t *testing.T) {
	for _, tc := range []struct {
		kind     Kind
		in, want string
	}{
		{KindEmail, "alice@example.com.vn", "a***@e***.vn"},
		{KindEmail, "b@x.io", "b***@x***.io"},
		{KindPhone, "+84901234567", "+84*******567"},
	} {
		id, err := Parse(tc.kind, tc.in, vn)
		if err != nil {
			t.Fatal(err)
		}
		if got := id.Mask(); got != tc.want {
			t.Errorf("%s: %q", tc.want, got)
		}
	}
}

func TestIdentifierRedacts(t *testing.T) {
	id, _ := Parse(KindEmail, "alice@example.com", vn)
	var buf bytes.Buffer
	slog.New(slog.NewJSONHandler(&buf, nil)).Info("x", "id", id)
	j, _ := json.Marshal(id)
	for _, out := range []string{fmt.Sprintf("%v %+v %s %q", id, id, id, id), buf.String(), string(j)} {
		if strings.Contains(out, "alice") {
			t.Fatalf("leak: %s", out)
		}
	}
}

func TestPLIFR06_Render(t *testing.T) {
	for _, tt := range []TemplateType{TemplateVerificationCode, TemplateRecoveryCode} {
		for _, loc := range []string{"vi-VN", "en", ""} {
			m, err := Render(tt, loc, "123456", 60)
			if err != nil {
				t.Fatal(err)
			}
			for _, s := range []string{m.Subject, m.Text, m.SMS} {
				if !strings.Contains(s, "123456") || strings.Contains(s, Domain) {
					t.Errorf("%s/%s: %q", tt, loc, s)
				}
			}
			if len(m.SMS) > 160 {
				t.Errorf("%s/%s: sms %d chars", tt, loc, len(m.SMS))
			}
			for _, r := range m.SMS {
				if r > 127 {
					t.Errorf("%s/%s: sms not ASCII", tt, loc)
					break
				}
			}
		}
	}
	if _, err := Render("login_code_valid", "vi", "123456", 5); err != ErrUnsupportedTemplate {
		t.Fatal("unsupported template rendered")
	}
	if _, err := Render(TemplateRecoveryCode, "vi", "12345a", 5); err == nil {
		t.Fatal("invalid code rendered")
	}
	m, _ := Render(TemplateRecoveryCode, "en", "123456", 0)
	if !strings.Contains(m.Text, "15 minutes") {
		t.Fatal("default expiry not applied")
	}
}
