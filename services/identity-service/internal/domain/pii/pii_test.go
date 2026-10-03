package pii

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/profile"
)

var now = time.Date(2026, 10, 3, 8, 0, 0, 0, time.UTC)

func ptr[T any](v T) *T { return &v }

func full() PersonalInfo {
	return PersonalInfo{
		Phone:       ptr(" +84 901-234.567 "),
		DateOfBirth: &Date{Year: 1990, Month: time.May, Day: 17},
		Address: &Address{
			Line1: " 1 Nguyen Trai ", Line2: ptr("  "), City: " Ho Chi Minh ", Region: ptr("District 1"),
			PostalCode: ptr("700000"), Country: "vn",
		},
		NationalID: &NationalID{Type: "CCCD", Number: " 079123456123 "},
	}
}

func TestPIIFR03_Normalize(t *testing.T) {
	n := full().Normalize()
	if *n.Phone != "+84901234567" {
		t.Fatalf("phone %q", *n.Phone)
	}
	a := n.Address
	if a.Line1 != "1 Nguyen Trai" || a.Line2 != nil || a.City != "Ho Chi Minh" || a.Country != "VN" || *a.Region != "District 1" {
		t.Fatalf("address %#v", *a) // prints [REDACTED]; fields checked above
	}
	if n.NationalID.Type != NationalIDCCCD || n.NationalID.Number != "079123456123" {
		t.Fatal("national id not normalised")
	}
	if err := n.Validate(now); err != nil {
		t.Fatalf("valid input rejected: %v", err)
	}
	if got := n.FieldNames(); strings.Join(got, ",") != "phone_number,date_of_birth,address,national_id" {
		t.Fatalf("field names %v", got)
	}
	if (PersonalInfo{}).Validate(now) != nil || !(PersonalInfo{}).IsZero() {
		t.Fatal("empty info is valid and zero")
	}
}

func fieldCodes(err error) map[string]string {
	var ve *profile.ValidationError
	if !errors.As(err, &ve) {
		return nil
	}
	out := map[string]string{}
	for _, f := range ve.Fields {
		out[f.Field] = f.Code
	}
	return out
}

func TestPIIFR03_ValidationTable(t *testing.T) {
	tests := []struct {
		name  string
		mut   func(*PersonalInfo)
		field string
		code  string
	}{
		{"phone without plus", func(p *PersonalInfo) { p.Phone = ptr("84901234567") }, "phone_number", CodeInvalidFormat},
		{"phone too short", func(p *PersonalInfo) { p.Phone = ptr("+8490123") }, "phone_number", CodeInvalidFormat},
		{"phone too long", func(p *PersonalInfo) { p.Phone = ptr("+8490123456789012") }, "phone_number", CodeInvalidFormat},
		{"phone leading zero cc", func(p *PersonalInfo) { p.Phone = ptr("+0901234567") }, "phone_number", CodeInvalidFormat},
		{"phone letters", func(p *PersonalInfo) { p.Phone = ptr("+84abc234567") }, "phone_number", CodeInvalidFormat},
		{"phone control char", func(p *PersonalInfo) { p.Phone = ptr("+849012\x0034567") }, "phone_number", CodeInvalidFormat},
		{"dob too young", func(p *PersonalInfo) { p.DateOfBirth = &Date{2014, time.January, 1} }, "date_of_birth", CodeOutOfRange},
		{"dob future", func(p *PersonalInfo) { p.DateOfBirth = &Date{2030, time.January, 1} }, "date_of_birth", CodeOutOfRange},
		{"dob too old", func(p *PersonalInfo) { p.DateOfBirth = &Date{1900, time.January, 1} }, "date_of_birth", CodeOutOfRange},
		{"line1 required", func(p *PersonalInfo) { p.Address.Line1 = " " }, "address.line1", CodeRequired},
		{"line1 too long", func(p *PersonalInfo) { p.Address.Line1 = strings.Repeat("a", 201) }, "address.line1", CodeTooLong},
		{"line2 control", func(p *PersonalInfo) { p.Address.Line2 = ptr("a\u0007b") }, "address.line2", CodeInvalidCharacters},
		{"city required", func(p *PersonalInfo) { p.Address.City = "" }, "address.city", CodeRequired},
		{"city too long", func(p *PersonalInfo) { p.Address.City = strings.Repeat("c", 101) }, "address.city", CodeTooLong},
		{"region too long", func(p *PersonalInfo) { p.Address.Region = ptr(strings.Repeat("r", 101)) }, "address.region", CodeTooLong},
		{"postal too long", func(p *PersonalInfo) { p.Address.PostalCode = ptr(strings.Repeat("1", 21)) }, "address.postal_code", CodeTooLong},
		{"country unknown", func(p *PersonalInfo) { p.Address.Country = "XX" }, "address.country", CodeInvalidFormat},
		{"country alpha3", func(p *PersonalInfo) { p.Address.Country = "VNM" }, "address.country", CodeInvalidFormat},
		{"country required", func(p *PersonalInfo) { p.Address.Country = "" }, "address.country", CodeRequired},
		{"nid type", func(p *PersonalInfo) { p.NationalID.Type = "ssn" }, "national_id.type", CodeInvalidFormat},
		{"nid short", func(p *PersonalInfo) { p.NationalID.Number = "AB12" }, "national_id.number", CodeInvalidFormat},
		{"nid symbols", func(p *PersonalInfo) { p.NationalID.Number = "079-123-456" }, "national_id.number", CodeInvalidFormat},
		{"nid too long", func(p *PersonalInfo) { p.NationalID.Number = strings.Repeat("9", 21) }, "national_id.number", CodeTooLong},
		{"nid required", func(p *PersonalInfo) { p.NationalID.Number = "" }, "national_id.number", CodeRequired},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := full()
			tt.mut(&p)
			p = p.Normalize()
			err := p.Validate(now)
			codes := fieldCodes(err)
			if codes[tt.field] != tt.code {
				t.Fatalf("want %s=%s, got %v", tt.field, tt.code, codes)
			}
			if strings.Contains(err.Error(), "+84") || strings.Contains(err.Error(), "Nguyen") {
				t.Fatal("validation error echoes a value")
			}
		})
	}
}

func TestPIIFR03_AgeBoundaries(t *testing.T) {
	for _, tc := range []struct {
		d  Date
		ok bool
	}{
		{Date{2013, time.October, 3}, true},   // 13 today
		{Date{2013, time.October, 4}, false},  // 13 tomorrow
		{Date{1906, time.October, 3}, true},   // 120 today
		{Date{1905, time.October, 2}, false},  // 121
		{Date{2000, time.February, 29}, true}, // leap day
	} {
		p := PersonalInfo{DateOfBirth: &tc.d}
		if got := p.Validate(now) == nil; got != tc.ok {
			t.Errorf("%s: valid=%v want %v", tc.d.ISO(), got, tc.ok)
		}
	}
	if _, err := ParseDate("2023-02-30"); err == nil {
		t.Fatal("impossible date accepted")
	}
}

// TestA12_FixedWidthMasking: the masked shapes never reveal length.
func TestA12_FixedWidthMasking(t *testing.T) {
	phones := map[string]string{
		"+84901234567":     "+84*******567",
		"+849012345678":    "+84*******678",
		"+14155550123":     "+1*******123",
		"+79161234567":     "+7*******567",
		"+442071838750":    "+44*******750",
		"+85291234567":     "+852*******567",
		"+8801712345678":   "+880*******678",
		"+35312345678":     "+353*******678",
		"+861012345678":    "+86*******678",
		"+2348031234567":   "+234*******567",
		"+12":              "+*******",
		"+999123456789012": "+999*******012",
	}
	for in, want := range phones {
		if got := MaskPhone(in); got != want {
			t.Errorf("MaskPhone(%s) = %s want %s", in, got, want)
		}
	}
	nids := map[string]string{"079123456123": "******123", "B1234567": "******", "123456789": "******789", "ABC123": "******"}
	for in, want := range nids {
		if got := MaskNationalID(in); got != want {
			t.Errorf("MaskNationalID(%s) = %s want %s", in, got, want)
		}
	}
	m := full().Normalize().Mask()
	if *m.Phone != "+84*******567" || *m.DateOfBirth != "1990-**-**" || m.Address.City != "Ho Chi Minh" ||
		m.Address.Country != "VN" || m.NationalID.Type != NationalIDCCCD || m.NationalID.Number != "******123" {
		t.Fatal("masked view")
	}
	if e := (PersonalInfo{}).Mask(); e.Phone != nil || e.Address != nil || e.NationalID != nil || e.DateOfBirth != nil {
		t.Fatal("empty mask")
	}
}

// TestDD13_Redaction: values cannot leak through fmt, slog or JSON.
func TestDD13_Redaction(t *testing.T) {
	p := full().Normalize()
	secrets := []string{"901234567", "Nguyen", "079123456123", "1990", "Ho Chi Minh", "700000"}
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))
	log.Info("x", "info", p, "ptr", &p, "addr", *p.Address, "nid", p.NationalID, "dob", *p.DateOfBirth, "masked", p.Mask())
	j, _ := json.Marshal(map[string]any{"p": p, "a": p.Address, "n": p.NationalID, "d": p.DateOfBirth})
	out := buf.String() + string(j) +
		fmt.Sprintf("%v %+v %#v %s %d %x %q", p, p, p, p, p, p, p) +
		fmt.Sprintf("%v %+v %#v", *p.Address, p.NationalID, *p.DateOfBirth) +
		fmt.Errorf("wrapped: %v", p).Error()
	for _, s := range secrets {
		if strings.Contains(out, s) {
			t.Fatalf("%q leaked: %s", s, out)
		}
	}
	if !strings.Contains(out, Redacted) {
		t.Fatal("no redaction marker")
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	p := full().Normalize()
	cols, err := p.Encode()
	if err != nil || len(cols) != 4 {
		t.Fatalf("encode: %v %d", err, len(cols))
	}
	if string(cols[FieldPhoneNumber]) != "+84901234567" || string(cols[FieldDateOfBirth]) != "1990-05-17" {
		t.Fatal("plain encodings")
	}
	got, err := Decode(cols)
	if err != nil {
		t.Fatal(err)
	}
	if *got.Phone != *p.Phone || *got.DateOfBirth != *p.DateOfBirth || got.Address.Line1 != p.Address.Line1 ||
		*got.Address.Region != *p.Address.Region || got.Address.Line2 != nil || *got.NationalID != *p.NationalID {
		t.Fatal("round trip mismatch")
	}
	for _, bad := range []map[Field][]byte{
		{FieldDateOfBirth: []byte("17/05/1990")}, {FieldAddress: []byte("{")}, {FieldNationalID: []byte("[]")},
	} {
		if _, err := Decode(bad); !errors.Is(err, ErrCorrupt) {
			t.Fatalf("want ErrCorrupt, got %v", err)
		}
	}
	if only := p.Only([]Field{FieldPhoneNumber}); only.Address != nil || only.Phone == nil {
		t.Fatal("Only")
	}
}

func TestCountryList(t *testing.T) {
	if len(countries) != 249 {
		t.Fatalf("ISO 3166-1 has 249 assigned codes, got %d", len(countries))
	}
	if string(PhoneBlindIndexInput("+84901234567")) != "phone_number:+84901234567" {
		t.Fatal("blind index input")
	}
}
