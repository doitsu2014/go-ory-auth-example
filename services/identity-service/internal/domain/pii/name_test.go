package pii

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

const (
	tFirst = "Anh"
	tLast  = "Nguyễn"
)

func TestNameFR02_NormalizeAndValidate(t *testing.T) {
	n := PersonalInfo{Name: &Name{First: "  " + tFirst + " ", Last: "\t" + tLast}}.Normalize()
	if n.Name == nil || n.Name.First != tFirst || n.Name.Last != tLast {
		t.Fatal("name parts must be trimmed")
	}
	if err := n.Validate(now); err != nil {
		t.Fatalf("valid name rejected: %v", err)
	}
	if got := n.FieldNames(); strings.Join(got, ",") != "name" {
		t.Fatalf("field names %v", got)
	}
	// Both parts empty (after trimming) = absent.
	for _, in := range []*Name{{}, {First: "  ", Last: "\t"}} {
		if v := (PersonalInfo{Name: in}).Normalize(); v.Name != nil || !v.IsZero() {
			t.Fatal("an all-empty name must be dropped")
		}
	}
	// A part made only of whitespace and format characters is empty.
	if v := (PersonalInfo{Name: &Name{First: "\u200b \u2066", Last: "\u200d"}}).Normalize(); v.Name != nil {
		t.Fatal("format characters alone must not count as a name")
	}
	if v := (PersonalInfo{Name: &Name{First: "\u200b", Last: tLast}}).Normalize(); v.Name == nil || v.Name.First != "" || v.Validate(now) != nil {
		t.Fatal("a format-only part is dropped, the other part kept")
	}
	// One part is enough.
	if v := (PersonalInfo{Name: &Name{Last: tLast}}).Normalize(); v.Name == nil || v.Name.First != "" || v.Validate(now) != nil {
		t.Fatal("last name only is valid")
	}

	cases := []struct {
		name  Name
		field string
		code  string
	}{
		{Name{First: strings.Repeat("ễ", MaxNamePart+1)}, "name.first", CodeTooLong},
		{Name{Last: strings.Repeat("a", MaxNamePart+1)}, "name.last", CodeTooLong},
		{Name{First: "A\x00b"}, "name.first", CodeInvalidCharacters},
		{Name{First: "A\u0085b"}, "name.first", CodeInvalidCharacters}, // C1 (NEL)
		{Name{Last: "A\x1bb"}, "name.last", CodeInvalidCharacters},
		{Name{Last: "A\nb"}, "name.last", CodeInvalidCharacters},
		{Name{First: string([]byte{0xff, 0xfe})}, "name.first", CodeInvalidCharacters},
		{Name{First: "An\u200bh"}, "name.first", CodeInvalidCharacters},        // zero-width space (Cf)
		{Name{Last: "\u202eNguyễn"}, "name.last", CodeInvalidCharacters},       // right-to-left override
		{Name{Last: "Ng\u2066uyễn\u2069"}, "name.last", CodeInvalidCharacters}, // isolates
		{Name{First: "A\ufeffb"}, "name.first", CodeInvalidCharacters},         // BOM / ZWNBSP
	}
	for _, tc := range cases {
		err := PersonalInfo{Name: &tc.name}.Normalize().Validate(now)
		codes := fieldCodes(err)
		if codes[tc.field] != tc.code || len(codes) != 1 {
			t.Errorf("%s: got %v, want %s=%s", tc.field, codes, tc.field, tc.code)
		}
		if err != nil && strings.Contains(err.Error(), "A\x00b") {
			t.Fatal("validation errors must not echo values")
		}
	}
	// Exactly the limit (runes, not bytes) is accepted.
	ok := PersonalInfo{Name: &Name{First: strings.Repeat("ễ", MaxNamePart), Last: strings.Repeat("a", MaxNamePart)}}
	if err := ok.Normalize().Validate(now); err != nil {
		t.Fatalf("limit rejected: %v", err)
	}
}

func TestNameFR04_Mask(t *testing.T) {
	m := PersonalInfo{Name: &Name{First: tFirst, Last: tLast}}.Mask()
	if m.Name == nil || *m.Name.First != "A***" || *m.Name.Last != "N***" {
		t.Fatal("mask = first rune + ***")
	}
	// Multi-byte first rune is kept whole; the width does not depend on the length.
	m = PersonalInfo{Name: &Name{First: "Ễ", Last: strings.Repeat("Ư", 50)}}.Mask()
	if *m.Name.First != "Ễ***" || *m.Name.Last != "Ư***" {
		t.Fatalf("multi-byte mask")
	}
	m = PersonalInfo{Name: &Name{Last: tLast}}.Mask()
	if m.Name == nil || m.Name.First != nil || *m.Name.Last != "N***" {
		t.Fatal("an empty part masks to nil")
	}
	if (PersonalInfo{}).Mask().Name != nil {
		t.Fatal("no name → nil")
	}
}

func TestNameNFR01_Redaction(t *testing.T) {
	n := Name{First: tFirst, Last: tLast}
	masked := PersonalInfo{Name: &n}.Mask()
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))
	log.Info("x", "name", n, "masked_name", *masked.Name, "info", PersonalInfo{Name: &n})
	j, _ := json.Marshal(n)
	all := strings.Join([]string{
		fmt.Sprintf("%v %+v %#v %s %q", n, n, n, n, n), fmt.Sprintf("%v %+v", *masked.Name, *masked.Name),
		n.String(), string(j), buf.String(), fmt.Sprintf("%+v", PersonalInfo{Name: &n}),
	}, " ")
	for _, v := range []string{tFirst, tLast, "A***", "N***"} {
		if strings.Contains(all, v) {
			t.Fatalf("value %q leaked: %s", v, all)
		}
	}
	if !strings.Contains(all, Redacted) {
		t.Fatal("expected the redaction marker")
	}
}

func TestNameFR02_EncodeDecodeAndFields(t *testing.T) {
	in := PersonalInfo{Name: &Name{First: tFirst, Last: tLast}, Phone: ptr("+84901234567")}
	cols, err := in.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if string(cols[FieldName]) != `{"first":"Anh","last":"Nguyễn"}` {
		t.Fatalf("name encoding must be JSON {first,last}")
	}
	out, err := Decode(cols)
	if err != nil || *out.Name != *in.Name || *out.Phone != *in.Phone {
		t.Fatalf("round trip: %v", err)
	}
	if _, err := Decode(map[Field][]byte{FieldName: []byte("not json")}); err != ErrCorrupt {
		t.Fatalf("corrupt name: %v", err)
	}
	if f, ok := ParseField("name"); !ok || f != FieldName || AllFields[0] != FieldName {
		t.Fatal("name is a field")
	}
	only := in.Only([]Field{FieldName})
	if only.Name == nil || only.Phone != nil {
		t.Fatal("Only(name)")
	}
	if only := in.Only([]Field{FieldPhoneNumber}); only.Name != nil {
		t.Fatal("Only(phone) must drop the name")
	}
	if got := strings.Join(in.FieldNames(), ","); got != "name,phone_number" {
		t.Fatalf("field names %s", got)
	}
}
