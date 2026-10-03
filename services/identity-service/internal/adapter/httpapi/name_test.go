package httpapi

import (
	"strings"
	"testing"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/identity"
)

// NAME-FR-07: a legacy Kratos name trait is never returned for customers.
func TestNameFR07_CustomerViewsCarryNoName(t *testing.T) {
	f := newFixture(t)
	p := f.kratos.Sessions["cust"]
	p.Name = identity.Name{First: "Legacy", Last: "Trait"}
	f.kratos.Sessions["cust"] = p
	c := f.ids.M[f.customer]
	c.Name = identity.Name{First: "Legacy", Last: "Trait"}
	f.ids.M[f.customer] = c

	for _, rt := range []route{
		jsonRoute("GET", "/v1/me", ""),
	} {
		rec := f.do(t, rt, cred{bearer: "cust"})
		if rec.Code != 200 || strings.Contains(rec.Body.String(), `"name"`) || strings.Contains(rec.Body.String(), "Legacy") {
			t.Fatalf("%s: %d %s", rt.path, rec.Code, rec.Body.String())
		}
	}
	for _, rt := range []route{
		jsonRoute("GET", "/admin/v1/customers/{customer}", ""),
		jsonRoute("GET", "/admin/v1/customers", ""),
	} {
		rec := f.do(t, rt, cred{cookie: "support"})
		if rec.Code != 200 || strings.Contains(rec.Body.String(), `"name"`) || strings.Contains(rec.Body.String(), "Legacy") {
			t.Fatalf("%s: %d %s", rt.path, rec.Code, rec.Body.String())
		}
	}
}

func TestNameFR02_04_05_HTTPPersonalInfoName(t *testing.T) {
	f := newFixture(t)
	id := f.customerID(t)
	body := `{"name":{"first":" Thanh ","last":"Trương"},"phone_number":"+84901234567"}`
	rec := f.do(t, jsonRoute("PUT", "/v1/me/personal-info", body), cred{bearer: "cust"})
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"name":{"first":"Thanh","last":"Trương"}`) {
		t.Fatalf("PUT: %d %s", rec.Code, rec.Body.String())
	}
	rec = f.do(t, jsonRoute("GET", "/v1/me/personal-info", ""), cred{bearer: "cust"})
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"name":{"first":"Thanh","last":"Trương"}`) {
		t.Fatalf("GET: %d %s", rec.Code, rec.Body.String())
	}
	rec = f.do(t, jsonRoute("GET", "/admin/v1/customers/"+id.String()+"/personal-info", ""), cred{cookie: "support"})
	masked := rec.Body.String()
	if rec.Code != 200 || !strings.Contains(masked, `"has_name":true`) || !strings.Contains(masked, `"name":{"first":"T***","last":"T***"}`) ||
		strings.Contains(masked, "Thanh") || strings.Contains(masked, "Trương") {
		t.Fatalf("masked: %d %s", rec.Code, masked)
	}
	rec = f.do(t, jsonRoute("POST", "/admin/v1/customers/"+id.String()+"/personal-info/reveal",
		`{"reason_code":"identity_verification","fields":["name"]}`), cred{cookie: "super", origin: origin})
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"name":{"first":"Thanh","last":"Trương"}`) ||
		!strings.Contains(rec.Body.String(), `"phone_number":null`) {
		t.Fatalf("reveal: %d %s", rec.Code, rec.Body.String())
	}
	// No name → null in GET, has_name false in the masked view.
	rec = f.do(t, jsonRoute("PUT", "/v1/me/personal-info", `{"name":null,"phone_number":"+84901234567"}`), cred{bearer: "cust"})
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"name":null`) {
		t.Fatalf("PUT null name: %d %s", rec.Code, rec.Body.String())
	}
	rec = f.do(t, jsonRoute("GET", "/admin/v1/customers/"+id.String()+"/personal-info", ""), cred{cookie: "support"})
	if !strings.Contains(rec.Body.String(), `"has_name":false`) || !strings.Contains(rec.Body.String(), `"name":null`) {
		t.Fatalf("masked without name: %s", rec.Body.String())
	}

	// Strict bodies and validation never echo values.
	for _, tc := range []struct{ body, want string }{
		{`{"name":{"first":"x","middle":"Trương"}}`, `{"code":"unknown_field","field":"name"}`},
		{`{"name":{"first":42}}`, `{"code":"invalid_format","field":"name.first"}`},
		{`{"name":"Thanh Trương"}`, `{"code":"invalid_format","field":"name"}`},
		{`{"name":{"first":"Th\u0000anh"}}`, `{"code":"invalid_characters","field":"name.first"}`},
		{`{"name":{"last":"Tr\u202eương"}}`, `{"code":"invalid_characters","field":"name.last"}`},
		{`{"name":{"first":"Th\u200banh"}}`, `{"code":"invalid_characters","field":"name.first"}`},
		{`{"name":{"last":"` + strings.Repeat("ư", 101) + `"}}`, `{"code":"too_long","field":"name.last"}`},
	} {
		rec := f.do(t, jsonRoute("PUT", "/v1/me/personal-info", tc.body), cred{bearer: "cust"})
		if rec.Code != 422 || !strings.Contains(rec.Body.String(), tc.want) {
			t.Fatalf("%s: %d %s (want %s)", tc.body, rec.Code, rec.Body.String(), tc.want)
		}
		if strings.Contains(rec.Body.String(), "Trương") || strings.Contains(rec.Body.String(), "middle") {
			t.Fatalf("422 echoes input: %s", rec.Body.String())
		}
	}
	if logs := f.logs.String(); strings.Contains(logs, "Thanh") || strings.Contains(logs, "Trương") {
		t.Fatal("logs leak the name")
	}
}
