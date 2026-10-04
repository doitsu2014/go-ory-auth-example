package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
)

// CodeUnknownField is the field error code for a property the contract does
// not declare (additionalProperties: false). The property name is not
// echoed (it is client input); the error names the enclosing object.
const CodeUnknownField = "unknown_field"

type propKind int

const (
	kindString propKind = iota
	kindDate
	kindObject
	kindStringArray
)

type prop struct {
	kind     propKind
	nullable bool
	props    map[string]prop
}

// strictBodies are the PII (and service client) request bodies validated before the generated
// decoder runs: unknown properties and malformed values become field
// errors (422) that name the field and a code, never the value.
var strictBodies = map[string]prop{
	"PUT /v1/me/personal-info": {kind: kindObject, props: map[string]prop{
		"name": {kind: kindObject, nullable: true, props: map[string]prop{
			"first": {kind: kindString}, "last": {kind: kindString},
		}},
		"phone_number":  {kind: kindString, nullable: true},
		"date_of_birth": {kind: kindDate, nullable: true},
		"address": {kind: kindObject, nullable: true, props: map[string]prop{
			"line1": {kind: kindString}, "line2": {kind: kindString, nullable: true},
			"city": {kind: kindString}, "region": {kind: kindString, nullable: true},
			"postal_code": {kind: kindString, nullable: true}, "country": {kind: kindString},
		}},
		"national_id": {kind: kindObject, nullable: true, props: map[string]prop{
			"type": {kind: kindString}, "number": {kind: kindString},
		}},
		"updated_at": {kind: kindString, nullable: true}, // readOnly; ignored
	}},
	"POST /admin/v1/customers/{id}/personal-info/reveal": {kind: kindObject, props: map[string]prop{
		"reason_code": {kind: kindString}, "ticket_ref": {kind: kindString}, "fields": {kind: kindStringArray},
	}},
	"POST /admin/v1/customers/lookup": {kind: kindObject, props: map[string]prop{
		"phone_number": {kind: kindString},
		"login":        {kind: kindObject, props: map[string]prop{"type": {kind: kindString}, "value": {kind: kindString}}},
	}},
	// Public customer auth (PLX-NFR-01): exactly these properties.
	"POST /v1/auth/login":        credentialsBody,
	"POST /v1/auth/registration": credentialsBody,
	"POST /v1/auth/recovery": {kind: kindObject, props: map[string]prop{
		"login": loginProp,
	}},
	"POST /v1/auth/recovery/code": {kind: kindObject, props: map[string]prop{
		"recovery_id": {kind: kindString}, "code": {kind: kindString},
	}},
	// additionalProperties: false (M2M-FR-08).
	"POST /admin/v1/service-clients": {kind: kindObject, props: map[string]prop{
		"name": {kind: kindString}, "owner": {kind: kindString}, "scopes": {kind: kindStringArray},
	}},
}

var (
	loginProp       = prop{kind: kindObject, props: map[string]prop{"type": {kind: kindString}, "value": {kind: kindString}}}
	credentialsBody = prop{kind: kindObject, props: map[string]prop{"login": loginProp, "password": {kind: kindString}}}
)

// forbiddenQuery lists query parameters a route refuses outright. The
// generated wrapper ignores unknown parameters; these carried PII and were
// removed from the contract (PLI-FR-11: PII never in URLs).
var forbiddenQuery = map[string][]string{
	"GET /admin/v1/customers": {"email"},
}

// strictBodyMiddleware validates the bodies listed in strictBodies. It runs
// inside the generated wrapper, after routing and after the Guard.
func strictBodyMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pattern := ""
		if rc := chi.RouteContext(r.Context()); rc != nil {
			pattern = rc.RoutePattern()
		}
		for _, q := range forbiddenQuery[routeKey(r.Method, pattern)] {
			if r.URL.Query().Has(q) {
				// The value is never echoed (it may be PII).
				writeProblem(w, r, CodeInvalidRequest, "", []app.FieldError{{Field: q, Code: "unsupported"}})
				return
			}
		}
		schema, ok := strictBodies[routeKey(r.Method, pattern)]
		if !ok {
			next.ServeHTTP(w, r)
			return
		}
		b, err := io.ReadAll(r.Body)
		if err != nil {
			writeProblem(w, r, CodeValidationFailed, "", []app.FieldError{{Field: "body", Code: "invalid"}})
			return
		}
		defer clear(b)
		if len(bytes.TrimSpace(b)) > 0 {
			var errs []app.FieldError
			if !checkValue("body", b, schema, &errs) {
				writeProblem(w, r, CodeValidationFailed, "", []app.FieldError{{Field: "body", Code: "invalid"}})
				return
			}
			if len(errs) > 0 {
				sort.Slice(errs, func(i, j int) bool { return errs[i].Field < errs[j].Field })
				writeProblem(w, r, CodeValidationFailed, "", errs)
				return
			}
		}
		r.Body = io.NopCloser(bytes.NewReader(bytes.Clone(b)))
		next.ServeHTTP(w, r)
	})
}

// checkValue validates raw against p. It returns false only when the top
// level is not a JSON object (the whole body is invalid).
func checkValue(path string, raw json.RawMessage, p prop, errs *[]app.FieldError) bool {
	add := func(code string) { *errs = append(*errs, app.FieldError{Field: path, Code: code}) }
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		if !p.nullable {
			if path == "body" {
				return false
			}
			add("invalid_format")
		}
		return true
	}
	switch p.kind {
	case kindString:
		var s string
		if json.Unmarshal(raw, &s) != nil {
			add("invalid_format")
		}
	case kindDate:
		var s string
		if json.Unmarshal(raw, &s) != nil {
			add("invalid_format")
		} else if _, err := time.Parse(time.DateOnly, s); err != nil {
			add("invalid_format")
		}
	case kindStringArray:
		var a []string
		if json.Unmarshal(raw, &a) != nil {
			add("invalid_format")
		}
	case kindObject:
		var m map[string]json.RawMessage
		if json.Unmarshal(raw, &m) != nil {
			if path == "body" {
				return false
			}
			add("invalid_format")
			return true
		}
		unknown := false
		for k, v := range m {
			sub, ok := p.props[k]
			if !ok {
				unknown = true
				continue
			}
			child := k
			if path != "body" {
				child = path + "." + k
			}
			checkValue(child, v, sub, errs)
		}
		if unknown {
			add(CodeUnknownField)
		}
	}
	return true
}
