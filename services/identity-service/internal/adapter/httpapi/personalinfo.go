package httpapi

import (
	"context"
	"slices"
	"time"

	"github.com/oapi-codegen/nullable"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/adapter/httpapi/gen"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/pii"
)

// GetMyPersonalInfo implements GET /v1/me/personal-info.
func (s *Server) GetMyPersonalInfo(ctx context.Context, _ gen.GetMyPersonalInfoRequestObject) (gen.GetMyPersonalInfoResponseObject, error) {
	a, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	v, err := s.PersonalInfo.GetMine(ctx, a.Principal)
	if err != nil {
		return nil, err
	}
	return gen.GetMyPersonalInfo200JSONResponse(toPersonalInfo(v)), nil
}

// PutMyPersonalInfo implements PUT /v1/me/personal-info.
func (s *Server) PutMyPersonalInfo(ctx context.Context, req gen.PutMyPersonalInfoRequestObject) (gen.PutMyPersonalInfoResponseObject, error) {
	a, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, app.NewValidationError("body", "required")
	}
	v, err := s.PersonalInfo.PutMine(ctx, a, fromPersonalInfo(*req.Body))
	if err != nil {
		return nil, err
	}
	return gen.PutMyPersonalInfo200JSONResponse(toPersonalInfo(v)), nil
}

// EraseMyPersonalInfo implements DELETE /v1/me/personal-info.
func (s *Server) EraseMyPersonalInfo(ctx context.Context, _ gen.EraseMyPersonalInfoRequestObject) (gen.EraseMyPersonalInfoResponseObject, error) {
	a, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.PersonalInfo.EraseMine(ctx, a); err != nil {
		return nil, err
	}
	return gen.EraseMyPersonalInfo204Response{}, nil
}

// LookupCustomers implements POST /admin/v1/customers/lookup.
func (s *Server) LookupCustomers(ctx context.Context, req gen.LookupCustomersRequestObject) (gen.LookupCustomersResponseObject, error) {
	a, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, app.NewValidationError("body", "required")
	}
	res, err := s.PersonalInfo.LookupByPhone(ctx, a, req.Body.PhoneNumber)
	if err != nil {
		return nil, err
	}
	matches := res.Matches
	out := gen.CustomerLookupResult{Truncated: res.Truncated}
	out.Items = slices.Grow(out.Items, len(matches)+1)[:len(matches)] // never null
	for i, m := range matches {
		st := gen.IdentityState(m.State)
		out.Items[i].Id = m.IdentityID
		out.Items[i].State = &st
		out.Items[i].PersonalInfo = toMasked(app.MaskedPersonalInfoView{Masked: m.Masked, UpdatedAt: m.UpdatedAt})
	}
	return gen.LookupCustomers200JSONResponse(out), nil
}

// GetCustomerPersonalInfoMasked implements GET /admin/v1/customers/{id}/personal-info.
func (s *Server) GetCustomerPersonalInfoMasked(ctx context.Context, req gen.GetCustomerPersonalInfoMaskedRequestObject) (gen.GetCustomerPersonalInfoMaskedResponseObject, error) {
	a, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	v, err := s.PersonalInfo.GetMasked(ctx, a, req.Id)
	if err != nil {
		return nil, err
	}
	return gen.GetCustomerPersonalInfoMasked200JSONResponse(toMasked(v)), nil
}

// RevealCustomerPersonalInfo implements POST /admin/v1/customers/{id}/personal-info/reveal.
func (s *Server) RevealCustomerPersonalInfo(ctx context.Context, req gen.RevealCustomerPersonalInfoRequestObject) (gen.RevealCustomerPersonalInfoResponseObject, error) {
	a, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, app.NewValidationError("body", "required")
	}
	in := app.RevealRequest{ReasonCode: app.RevealReasonCode(req.Body.ReasonCode), TicketRef: req.Body.TicketRef}
	if f := req.Body.Fields; f != nil {
		in.Fields = make([]string, len(*f))
		for i, name := range *f {
			in.Fields[i] = string(name)
		}
	}
	v, err := s.PersonalInfo.Reveal(ctx, a, req.Id, in)
	if err != nil {
		return nil, err
	}
	return gen.RevealCustomerPersonalInfo200JSONResponse(toPersonalInfo(v)), nil
}

// --- conversions ---

func nullPtr(n nullable.Nullable[string]) *string {
	if v, err := n.Get(); err == nil {
		return &v
	}
	return nil
}

func fromPersonalInfo(b gen.PersonalInfo) pii.PersonalInfo {
	var p pii.PersonalInfo
	if v, err := b.PhoneNumber.Get(); err == nil {
		p.Phone = &v
	}
	if v, err := b.DateOfBirth.Get(); err == nil {
		d := pii.DateOf(v.Time)
		p.DateOfBirth = &d
	}
	if v, err := b.Address.Get(); err == nil {
		p.Address = &pii.Address{
			Line1: v.Line1, Line2: nullPtr(v.Line2), City: v.City, Region: nullPtr(v.Region),
			PostalCode: nullPtr(v.PostalCode), Country: v.Country,
		}
	}
	if v, err := b.NationalId.Get(); err == nil {
		p.NationalID = &pii.NationalID{Type: pii.NationalIDType(v.Type), Number: v.Number}
	}
	return p
}

func nullableTime(t *time.Time) nullable.Nullable[time.Time] {
	if t == nil {
		return nullable.NewNullNullable[time.Time]()
	}
	return nullable.NewNullableWithValue(t.UTC())
}

// toPersonalInfo writes every field, null when unset (PII-FR-01).
func toPersonalInfo(v app.PersonalInfoView) gen.PersonalInfo {
	p := v.Info
	out := gen.PersonalInfo{
		PhoneNumber: nullableOf(p.Phone),
		DateOfBirth: nullable.NewNullNullable[openapi_types.Date](),
		Address:     nullable.NewNullNullable[gen.Address](),
		NationalId:  nullable.NewNullNullable[gen.NationalId](),
		UpdatedAt:   nullableTime(v.UpdatedAt),
	}
	if d := p.DateOfBirth; d != nil {
		out.DateOfBirth.Set(openapi_types.Date{Time: d.Time()})
	}
	if a := p.Address; a != nil {
		out.Address.Set(gen.Address{
			Line1: a.Line1, Line2: nullableOf(a.Line2), City: a.City, Region: nullableOf(a.Region),
			PostalCode: nullableOf(a.PostalCode), Country: a.Country,
		})
	}
	if n := p.NationalID; n != nil {
		out.NationalId.Set(gen.NationalId{Type: gen.NationalIdType(n.Type), Number: n.Number})
	}
	return out
}

func toMasked(v app.MaskedPersonalInfoView) gen.MaskedPersonalInfo {
	m := v.Masked
	out := gen.MaskedPersonalInfo{
		PhoneNumber: nullableOf(m.Phone), DateOfBirth: nullableOf(m.DateOfBirth),
		HasPhoneNumber: m.Phone != nil, HasDateOfBirth: m.DateOfBirth != nil,
		HasAddress: m.Address != nil, HasNationalId: m.NationalID != nil,
		UpdatedAt: nullableTime(v.UpdatedAt),
	}
	// The address and national id schemas are inline (anonymous) types.
	addr, _ := out.Address.Get()
	if a := m.Address; a != nil {
		city, country := a.City, a.Country
		addr.City, addr.Country = &city, &country
		out.Address.Set(addr)
	} else {
		out.Address.SetNull()
	}
	nid, _ := out.NationalId.Get()
	if n := m.NationalID; n != nil {
		t, num := gen.MaskedPersonalInfoNationalIdType(n.Type), n.Number
		nid.Type, nid.Number = &t, &num
		out.NationalId.Set(nid)
	} else {
		out.NationalId.SetNull()
	}
	return out
}
