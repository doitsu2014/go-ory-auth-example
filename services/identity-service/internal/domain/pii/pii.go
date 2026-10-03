// Package pii holds the customer personal information entity (phone number,
// date of birth, address, national id): normalisation, validation, masking
// and the plaintext encodings that are sealed per column.
//
// Every type that carries a value redacts itself when formatted, logged
// (slog.LogValuer) or JSON-encoded, so an accidental log line or error
// message cannot leak it (DD-13). The only way to obtain the plaintext bytes
// is the explicit Encode*/field accessors used by the encryption path.
package pii

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/profile"
)

// Redacted replaces PII values wherever a value would be printed.
const Redacted = "[REDACTED]"

// ErrCorrupt is returned when a decrypted column cannot be decoded. It never
// carries the offending bytes.
var ErrCorrupt = errors.New("personal info: undecodable column")

// Field names a personal information field; it is also the column name used
// in the AAD and the name recorded in audit details.
type Field string

// Fields.
const (
	FieldPhoneNumber Field = "phone_number"
	FieldDateOfBirth Field = "date_of_birth"
	FieldAddress     Field = "address"
	FieldNationalID  Field = "national_id"
)

// AllFields lists every field in a stable order.
var AllFields = []Field{FieldPhoneNumber, FieldDateOfBirth, FieldAddress, FieldNationalID}

// ParseField validates a field name.
func ParseField(s string) (Field, bool) {
	for _, f := range AllFields {
		if string(f) == s {
			return f, true
		}
	}
	return "", false
}

// Limits mirror the OpenAPI schema (PII-FR-03).
const (
	MaxLine       = 200
	MaxCity       = 100
	MaxRegion     = 100
	MaxPostalCode = 20
	MinAge        = 13
	MaxAge        = 120
)

// Field error codes (api-contract §Decisions).
const (
	CodeInvalidFormat      = "invalid_format"
	CodeTooLong            = "too_long"
	CodeOutOfRange         = "out_of_range"
	CodeRequired           = "required"
	CodeInvalidCharacters  = "invalid_characters"
	phoneBlindIndexPrefix  = "phone_number:"
	nationalIDMaskedPrefix = "******"
	phoneMaskedMiddle      = "*******"
)

var (
	e164Re       = regexp.MustCompile(`^\+[1-9][0-9]{7,14}$`)
	countryRe    = regexp.MustCompile(`^[A-Z]{2}$`)
	nationalIDRe = regexp.MustCompile(`^[A-Z0-9]{6,20}$`)
)

// NationalIDType is the closed set of identity document types.
type NationalIDType string

// National id types.
const (
	NationalIDCCCD     NationalIDType = "cccd"
	NationalIDPassport NationalIDType = "passport"
	NationalIDOther    NationalIDType = "other"
)

// Valid reports whether t is a known type.
func (t NationalIDType) Valid() bool {
	switch t {
	case NationalIDCCCD, NationalIDPassport, NationalIDOther:
		return true
	}
	return false
}

// Date is a civil date (no time zone).
type Date struct {
	Year  int
	Month time.Month
	Day   int
}

// DateOf returns the civil date of t (in t's location).
func DateOf(t time.Time) Date {
	y, m, d := t.Date()
	return Date{Year: y, Month: m, Day: d}
}

// ParseDate parses YYYY-MM-DD and rejects impossible dates.
func ParseDate(s string) (Date, error) {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		return Date{}, ErrCorrupt
	}
	return DateOf(t), nil
}

// ISO returns YYYY-MM-DD. It is the plaintext form; never log it.
func (d Date) ISO() string { return fmt.Sprintf("%04d-%02d-%02d", d.Year, d.Month, d.Day) }

// Time returns the date at 00:00 UTC.
func (d Date) Time() time.Time { return time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, time.UTC) }

// AgeAt returns the age in whole years at now (negative for future dates).
func (d Date) AgeAt(now time.Time) int {
	n := DateOf(now.UTC())
	age := n.Year - d.Year
	if n.Month < d.Month || (n.Month == d.Month && n.Day < d.Day) {
		age--
	}
	return age
}

// Address is a postal address.
type Address struct {
	Line1      string
	Line2      *string
	City       string
	Region     *string
	PostalCode *string
	Country    string // ISO 3166-1 alpha-2
}

// NationalID is an identity document number.
type NationalID struct {
	Type   NationalIDType
	Number string
}

// PersonalInfo is a customer's personal information. nil fields are unset.
type PersonalInfo struct {
	Phone       *string // E.164 after Normalize
	DateOfBirth *Date
	Address     *Address
	NationalID  *NationalID
}

// IsZero reports whether no field is set.
func (p PersonalInfo) IsZero() bool {
	return p.Phone == nil && p.DateOfBirth == nil && p.Address == nil && p.NationalID == nil
}

// FieldNames lists the set fields (for audit details: names, never values).
func (p PersonalInfo) FieldNames() []string {
	out := []string{}
	if p.Phone != nil {
		out = append(out, string(FieldPhoneNumber))
	}
	if p.DateOfBirth != nil {
		out = append(out, string(FieldDateOfBirth))
	}
	if p.Address != nil {
		out = append(out, string(FieldAddress))
	}
	if p.NationalID != nil {
		out = append(out, string(FieldNationalID))
	}
	return out
}

// Only keeps the listed fields (data minimisation for reveal).
func (p PersonalInfo) Only(fields []Field) PersonalInfo {
	var out PersonalInfo
	for _, f := range fields {
		switch f {
		case FieldPhoneNumber:
			out.Phone = p.Phone
		case FieldDateOfBirth:
			out.DateOfBirth = p.DateOfBirth
		case FieldAddress:
			out.Address = p.Address
		case FieldNationalID:
			out.NationalID = p.NationalID
		}
	}
	return out
}

// NormalizePhone strips spaces, dashes and dots and surrounding whitespace.
func NormalizePhone(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case ' ', '-', '.', '\t':
			return -1
		}
		return r
	}, strings.TrimSpace(s))
}

// ValidPhone reports whether s is an E.164 number.
func ValidPhone(s string) bool { return e164Re.MatchString(s) }

// PhoneBlindIndexInput is the domain-separated blind index input for a
// normalised phone number.
func PhoneBlindIndexInput(e164 string) []byte { return []byte(phoneBlindIndexPrefix + e164) }

func trimPtr(p *string) *string {
	if p == nil {
		return nil
	}
	v := strings.TrimSpace(*p)
	if v == "" {
		return nil
	}
	return &v
}

// Normalize returns a normalised copy: phone stripped to E.164 form, text
// trimmed, empty optional text dropped, country and document number
// upper-cased.
func (p PersonalInfo) Normalize() PersonalInfo {
	var out PersonalInfo
	if p.Phone != nil {
		v := NormalizePhone(*p.Phone)
		out.Phone = &v
	}
	if p.DateOfBirth != nil {
		d := *p.DateOfBirth
		out.DateOfBirth = &d
	}
	if a := p.Address; a != nil {
		out.Address = &Address{
			Line1: strings.TrimSpace(a.Line1), Line2: trimPtr(a.Line2), City: strings.TrimSpace(a.City),
			Region: trimPtr(a.Region), PostalCode: trimPtr(a.PostalCode),
			Country: strings.ToUpper(strings.TrimSpace(a.Country)),
		}
	}
	if n := p.NationalID; n != nil {
		out.NationalID = &NationalID{
			Type:   NationalIDType(strings.ToLower(strings.TrimSpace(string(n.Type)))),
			Number: strings.ToUpper(strings.TrimSpace(n.Number)),
		}
	}
	return out
}

// Validate checks a normalised value. Errors name the field and a code only;
// values are never echoed (PII-FR-02/03).
func (p PersonalInfo) Validate(now time.Time) error {
	var errs []profile.FieldError
	add := func(field, code string) { errs = append(errs, profile.FieldError{Field: field, Code: code}) }
	text := func(field string, v string, maxLen int, required bool) {
		switch {
		case v == "" && required:
			add(field, CodeRequired)
		case utf8.RuneCountInString(v) > maxLen:
			add(field, CodeTooLong)
		case profile.HasControlChars(v):
			add(field, CodeInvalidCharacters)
		}
	}
	optText := func(field string, v *string, maxLen int) {
		if v != nil {
			text(field, *v, maxLen, false)
		}
	}
	if p.Phone != nil && !ValidPhone(*p.Phone) {
		add(string(FieldPhoneNumber), CodeInvalidFormat)
	}
	if d := p.DateOfBirth; d != nil {
		if age := d.AgeAt(now); age < MinAge || age > MaxAge {
			add(string(FieldDateOfBirth), CodeOutOfRange)
		}
	}
	if a := p.Address; a != nil {
		text("address.line1", a.Line1, MaxLine, true)
		optText("address.line2", a.Line2, MaxLine)
		text("address.city", a.City, MaxCity, true)
		optText("address.region", a.Region, MaxRegion)
		optText("address.postal_code", a.PostalCode, MaxPostalCode)
		switch {
		case a.Country == "":
			add("address.country", CodeRequired)
		case !countryRe.MatchString(a.Country) || !isCountry(a.Country):
			add("address.country", CodeInvalidFormat)
		}
	}
	if n := p.NationalID; n != nil {
		if !n.Type.Valid() {
			add("national_id.type", CodeInvalidFormat)
		}
		switch {
		case n.Number == "":
			add("national_id.number", CodeRequired)
		case utf8.RuneCountInString(n.Number) > 20:
			add("national_id.number", CodeTooLong)
		case !nationalIDRe.MatchString(n.Number):
			add("national_id.number", CodeInvalidFormat)
		}
	}
	if len(errs) > 0 {
		return &profile.ValidationError{Fields: errs}
	}
	return nil
}

// --- masking (§9 A12: fixed width) ---

// MaskedAddress is the address as shown to admins: city and country only.
type MaskedAddress struct {
	City    string
	Country string
}

// MaskedNationalID is the national id as shown to admins.
type MaskedNationalID struct {
	Type   NationalIDType
	Number string
}

// Masked is the admin view. nil means "not provided".
type Masked struct {
	Phone       *string
	DateOfBirth *string
	Address     *MaskedAddress
	NationalID  *MaskedNationalID
}

// Mask applies the fixed-width masking rules.
func (p PersonalInfo) Mask() Masked {
	var m Masked
	if p.Phone != nil {
		v := MaskPhone(*p.Phone)
		m.Phone = &v
	}
	if p.DateOfBirth != nil {
		v := fmt.Sprintf("%04d-**-**", p.DateOfBirth.Year)
		m.DateOfBirth = &v
	}
	if a := p.Address; a != nil {
		m.Address = &MaskedAddress{City: a.City, Country: a.Country}
	}
	if n := p.NationalID; n != nil {
		m.NationalID = &MaskedNationalID{Type: n.Type, Number: MaskNationalID(n.Number)}
	}
	return m
}

// MaskPhone keeps "+", the country calling code and the last 3 digits with a
// fixed 7-star middle, so the mask does not reveal the number's length.
func MaskPhone(e164 string) string {
	digits := strings.TrimPrefix(e164, "+")
	if len(digits) < 6 {
		return "+" + phoneMaskedMiddle
	}
	cc := digits[:callingCodeLen(digits)]
	return "+" + cc + phoneMaskedMiddle + digits[len(digits)-3:]
}

// MaskNationalID is a fixed 6 stars, plus the last 3 characters only when the
// number has at least 9 characters.
func MaskNationalID(n string) string {
	if len(n) >= 9 {
		return nationalIDMaskedPrefix + n[len(n)-3:]
	}
	return nationalIDMaskedPrefix
}

// twoDigitCodes are the ITU-T E.164 two-digit country calling codes; 1 and 7
// are the only one-digit codes; every other code has three digits. The
// codes are prefix-free, so this is a longest match.
var twoDigitCodes = map[string]bool{
	"20": true, "27": true, "30": true, "31": true, "32": true, "33": true, "34": true, "36": true,
	"39": true, "40": true, "41": true, "43": true, "44": true, "45": true, "46": true, "47": true,
	"48": true, "49": true, "51": true, "52": true, "53": true, "54": true, "55": true, "56": true,
	"57": true, "58": true, "60": true, "61": true, "62": true, "63": true, "64": true, "65": true,
	"66": true, "81": true, "82": true, "84": true, "86": true, "90": true, "91": true, "92": true,
	"93": true, "94": true, "95": true, "98": true,
}

func callingCodeLen(digits string) int {
	switch {
	case digits[0] == '1' || digits[0] == '7':
		return 1
	case twoDigitCodes[digits[:2]]:
		return 2
	}
	return 3
}

// --- redaction (DD-13) ---

func redactedFormat(f fmt.State, _ rune) { _, _ = f.Write([]byte(Redacted)) }

// Format implements fmt.Formatter for every verb.
func (p PersonalInfo) Format(f fmt.State, verb rune) { redactedFormat(f, verb) }

// String implements fmt.Stringer.
func (p PersonalInfo) String() string { return Redacted }

// LogValue implements slog.LogValuer.
func (p PersonalInfo) LogValue() slog.Value { return slog.StringValue(Redacted) }

// MarshalJSON never encodes values.
func (p PersonalInfo) MarshalJSON() ([]byte, error) { return json.Marshal(Redacted) }

// Format implements fmt.Formatter for every verb.
func (a Address) Format(f fmt.State, verb rune) { redactedFormat(f, verb) }

// String implements fmt.Stringer.
func (a Address) String() string { return Redacted }

// LogValue implements slog.LogValuer.
func (a Address) LogValue() slog.Value { return slog.StringValue(Redacted) }

// MarshalJSON never encodes values (use EncodeAddress for sealing).
func (a Address) MarshalJSON() ([]byte, error) { return json.Marshal(Redacted) }

// Format implements fmt.Formatter for every verb.
func (n NationalID) Format(f fmt.State, verb rune) { redactedFormat(f, verb) }

// String implements fmt.Stringer.
func (n NationalID) String() string { return Redacted }

// LogValue implements slog.LogValuer.
func (n NationalID) LogValue() slog.Value { return slog.StringValue(Redacted) }

// MarshalJSON never encodes values (use EncodeNationalID for sealing).
func (n NationalID) MarshalJSON() ([]byte, error) { return json.Marshal(Redacted) }

// Format implements fmt.Formatter for every verb.
func (d Date) Format(f fmt.State, verb rune) { redactedFormat(f, verb) }

// String implements fmt.Stringer.
func (d Date) String() string { return Redacted }

// LogValue implements slog.LogValuer.
func (d Date) LogValue() slog.Value { return slog.StringValue(Redacted) }

// MarshalJSON never encodes values.
func (d Date) MarshalJSON() ([]byte, error) { return json.Marshal(Redacted) }

// Format implements fmt.Formatter for every verb.
func (m Masked) Format(f fmt.State, verb rune) { redactedFormat(f, verb) }

// LogValue implements slog.LogValuer.
func (m Masked) LogValue() slog.Value { return slog.StringValue(Redacted) }

// --- plaintext encodings (sealed per column) ---

type addressWire struct {
	Line1      string  `json:"line1"`
	Line2      *string `json:"line2,omitempty"`
	City       string  `json:"city"`
	Region     *string `json:"region,omitempty"`
	PostalCode *string `json:"postal_code,omitempty"`
	Country    string  `json:"country"`
}

type nationalIDWire struct {
	Type   string `json:"type"`
	Number string `json:"number"`
}

// Encode returns the plaintext bytes of every set field, keyed by field. The
// caller seals and then zeroes them.
func (p PersonalInfo) Encode() (map[Field][]byte, error) {
	out := map[Field][]byte{}
	if p.Phone != nil {
		out[FieldPhoneNumber] = []byte(*p.Phone)
	}
	if p.DateOfBirth != nil {
		out[FieldDateOfBirth] = []byte(p.DateOfBirth.ISO())
	}
	if a := p.Address; a != nil {
		b, err := json.Marshal(addressWire{Line1: a.Line1, Line2: a.Line2, City: a.City, Region: a.Region, PostalCode: a.PostalCode, Country: a.Country})
		if err != nil {
			return nil, ErrCorrupt
		}
		out[FieldAddress] = b
	}
	if n := p.NationalID; n != nil {
		b, err := json.Marshal(nationalIDWire{Type: string(n.Type), Number: n.Number})
		if err != nil {
			return nil, ErrCorrupt
		}
		out[FieldNationalID] = b
	}
	return out, nil
}

// Decode builds a PersonalInfo from decrypted column plaintexts. Errors are
// ErrCorrupt and never carry the bytes.
func Decode(cols map[Field][]byte) (PersonalInfo, error) {
	var p PersonalInfo
	if b, ok := cols[FieldPhoneNumber]; ok {
		v := string(b)
		p.Phone = &v
	}
	if b, ok := cols[FieldDateOfBirth]; ok {
		d, err := ParseDate(string(b))
		if err != nil {
			return PersonalInfo{}, ErrCorrupt
		}
		p.DateOfBirth = &d
	}
	if b, ok := cols[FieldAddress]; ok {
		var w addressWire
		if err := json.Unmarshal(b, &w); err != nil {
			return PersonalInfo{}, ErrCorrupt
		}
		p.Address = &Address{Line1: w.Line1, Line2: w.Line2, City: w.City, Region: w.Region, PostalCode: w.PostalCode, Country: w.Country}
	}
	if b, ok := cols[FieldNationalID]; ok {
		var w nationalIDWire
		if err := json.Unmarshal(b, &w); err != nil {
			return PersonalInfo{}, ErrCorrupt
		}
		p.NationalID = &NationalID{Type: NationalIDType(w.Type), Number: w.Number}
	}
	return p, nil
}

// --- ISO 3166-1 alpha-2 (officially assigned codes) ---

const countryCodes = "AD AE AF AG AI AL AM AO AQ AR AS AT AU AW AX AZ " +
	"BA BB BD BE BF BG BH BI BJ BL BM BN BO BQ BR BS BT BV BW BY BZ " +
	"CA CC CD CF CG CH CI CK CL CM CN CO CR CU CV CW CX CY CZ " +
	"DE DJ DK DM DO DZ EC EE EG EH ER ES ET FI FJ FK FM FO FR " +
	"GA GB GD GE GF GG GH GI GL GM GN GP GQ GR GS GT GU GW GY " +
	"HK HM HN HR HT HU ID IE IL IM IN IO IQ IR IS IT JE JM JO JP " +
	"KE KG KH KI KM KN KP KR KW KY KZ LA LB LC LI LK LR LS LT LU LV LY " +
	"MA MC MD ME MF MG MH MK ML MM MN MO MP MQ MR MS MT MU MV MW MX MY MZ " +
	"NA NC NE NF NG NI NL NO NP NR NU NZ OM PA PE PF PG PH PK PL PM PN PR PS PT PW PY " +
	"QA RE RO RS RU RW SA SB SC SD SE SG SH SI SJ SK SL SM SN SO SR SS ST SV SX SY SZ " +
	"TC TD TF TG TH TJ TK TL TM TN TO TR TT TV TW TZ UA UG UM US UY UZ " +
	"VA VC VE VG VI VN VU WF WS YE YT ZA ZM ZW"

var countries = func() map[string]bool {
	m := map[string]bool{}
	for _, c := range strings.Fields(countryCodes) {
		m[c] = true
	}
	return m
}()

func isCountry(c string) bool { return countries[c] }
