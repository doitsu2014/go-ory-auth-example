package httpapi

import (
	"context"
	"fmt"

	"github.com/oapi-codegen/nullable"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/adapter/httpapi/gen"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/audit"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/machine"
)

func machinePrincipal(ctx context.Context) (machine.Principal, error) {
	p, ok := MachineFrom(ctx)
	if !ok {
		return machine.Principal{}, app.ErrInvalidToken
	}
	return p, nil
}

func (s *Server) machine() (*app.MachineService, error) {
	if s.Machine == nil {
		return nil, fmt.Errorf("%w: machine access not configured", app.ErrDependencyUnavailable)
	}
	return s.Machine, nil
}

func (s *Server) serviceClients() (*app.ServiceClientService, error) {
	if s.ServiceClients == nil {
		return nil, fmt.Errorf("%w: service clients not configured", app.ErrDependencyUnavailable)
	}
	return s.ServiceClients, nil
}

// GetMachineCustomer implements GET /m2m/v1/customers/{id}.
func (s *Server) GetMachineCustomer(ctx context.Context, req gen.GetMachineCustomerRequestObject) (gen.GetMachineCustomerResponseObject, error) {
	p, err := machinePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	svc, err := s.machine()
	if err != nil {
		return nil, err
	}
	v, err := svc.Customer(ctx, p, req.Id)
	if err != nil {
		return nil, err
	}
	return gen.GetMachineCustomer200JSONResponse{
		Id: v.ID, State: gen.IdentityState(v.State), EmailVerified: v.EmailVerified, CreatedAt: v.CreatedAt.UTC(),
	}, nil
}

// ListMachineAuditEvents implements GET /m2m/v1/audit-events.
func (s *Server) ListMachineAuditEvents(ctx context.Context, req gen.ListMachineAuditEventsRequestObject) (gen.ListMachineAuditEventsResponseObject, error) {
	p, err := machinePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	svc, err := s.machine()
	if err != nil {
		return nil, err
	}
	size, err := pageSize(req.Params.PageSize)
	if err != nil {
		return nil, err
	}
	f := audit.Filter{TargetType: req.Params.TargetType, TargetID: req.Params.TargetId, Limit: size}
	if req.Params.ActorId != nil {
		id := *req.Params.ActorId
		f.ActorID = &id
	}
	if t := deref(req.Params.PageToken); t != "" {
		c, err := decodeAuditCursor(t)
		if err != nil {
			return nil, app.NewValidationError("page_token", "invalid")
		}
		f.After = &c
	}
	page, err := svc.AuditEvents(ctx, p, f)
	if err != nil {
		return nil, err
	}
	out := gen.MachineAuditEventPage{Items: make([]gen.MachineAuditEvent, 0, len(page.Items))}
	for _, e := range page.Items {
		ev := gen.MachineAuditEvent{
			Id: e.ID, OccurredAt: e.OccurredAt.UTC(), ActorId: e.ActorID, Action: string(e.Action),
			TargetType: e.TargetType, TargetId: e.TargetID,
		}
		if !e.Details.Empty() {
			d := e.Details
			ev.Details = &struct {
				Name         *string   `json:"name,omitempty"`
				PreviousRole *string   `json:"previous_role,omitempty"`
				Role         *string   `json:"role,omitempty"`
				Scopes       *[]string `json:"scopes,omitempty"`
			}{Name: d.Name, PreviousRole: d.PreviousRole, Role: d.Role}
			if d.Scopes != nil {
				sc := d.Scopes
				ev.Details.Scopes = &sc
			}
		}
		out.Items = append(out.Items, ev)
	}
	if page.Next != nil {
		out.NextPageToken = nullable.NewNullableWithValue(encodeAuditCursor(*page.Next))
	}
	return gen.ListMachineAuditEvents200JSONResponse(out), nil
}

// ListServiceClients implements GET /admin/v1/service-clients.
func (s *Server) ListServiceClients(ctx context.Context, _ gen.ListServiceClientsRequestObject) (gen.ListServiceClientsResponseObject, error) {
	a, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	svc, err := s.serviceClients()
	if err != nil {
		return nil, err
	}
	list, err := svc.List(ctx, a)
	if err != nil {
		return nil, err
	}
	out := gen.ServiceClientList{Items: make([]gen.ServiceClient, 0, len(list))}
	for _, c := range list {
		out.Items = append(out.Items, toServiceClient(c))
	}
	return gen.ListServiceClients200JSONResponse(out), nil
}

// CreateServiceClient implements POST /admin/v1/service-clients. The secret
// is in this response only (never on a replay).
func (s *Server) CreateServiceClient(ctx context.Context, req gen.CreateServiceClientRequestObject) (gen.CreateServiceClientResponseObject, error) {
	a, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	svc, err := s.serviceClients()
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, app.NewValidationError("body", "required")
	}
	scopes := make([]string, len(req.Body.Scopes))
	for i, sc := range req.Body.Scopes {
		scopes[i] = string(sc)
	}
	out, err := svc.Create(ctx, a, req.Params.IdempotencyKey.String(), req.Body.Name, string(req.Body.Owner), scopes)
	if err != nil {
		return nil, err
	}
	return gen.CreateServiceClient201JSONResponse(withSecret(out)), nil
}

// GetServiceClient implements GET /admin/v1/service-clients/{client_id}.
func (s *Server) GetServiceClient(ctx context.Context, req gen.GetServiceClientRequestObject) (gen.GetServiceClientResponseObject, error) {
	a, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	svc, err := s.serviceClients()
	if err != nil {
		return nil, err
	}
	c, err := svc.Get(ctx, a, req.ClientId)
	if err != nil {
		return nil, err
	}
	return gen.GetServiceClient200JSONResponse(toServiceClient(c)), nil
}

// RotateServiceClientSecret implements POST
// /admin/v1/service-clients/{client_id}/rotate-secret.
func (s *Server) RotateServiceClientSecret(ctx context.Context, req gen.RotateServiceClientSecretRequestObject) (gen.RotateServiceClientSecretResponseObject, error) {
	a, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	svc, err := s.serviceClients()
	if err != nil {
		return nil, err
	}
	out, err := svc.RotateSecret(ctx, a, req.ClientId)
	if err != nil {
		return nil, err
	}
	return gen.RotateServiceClientSecret200JSONResponse(withSecret(out)), nil
}

// DeleteServiceClient implements DELETE /admin/v1/service-clients/{client_id}.
func (s *Server) DeleteServiceClient(ctx context.Context, req gen.DeleteServiceClientRequestObject) (gen.DeleteServiceClientResponseObject, error) {
	a, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	svc, err := s.serviceClients()
	if err != nil {
		return nil, err
	}
	if err := svc.Delete(ctx, a, req.ClientId); err != nil {
		return nil, err
	}
	return gen.DeleteServiceClient204Response{}, nil
}

func scopesOut(s []machine.Scope) []gen.MachineScope {
	out := make([]gen.MachineScope, len(s))
	for i, x := range s {
		out[i] = gen.MachineScope(x)
	}
	return out
}

func createdBy(c machine.ServiceClient) nullable.Nullable[openapi_types.UUID] {
	if c.CreatedBy == nil {
		return nullable.NewNullNullable[openapi_types.UUID]()
	}
	return nullable.NewNullableWithValue(*c.CreatedBy)
}

func toServiceClient(c machine.ServiceClient) gen.ServiceClient {
	return gen.ServiceClient{
		ClientId: c.ClientID, Name: c.Name, Owner: openapi_types.Email(c.Owner), Scopes: scopesOut(c.Scopes),
		CreatedAt: c.CreatedAt.UTC(), CreatedBy: createdBy(c),
	}
}

func withSecret(v app.ServiceClientWithSecret) gen.ServiceClientWithSecret {
	c := v.Client
	out := gen.ServiceClientWithSecret{
		ClientId: c.ClientID, Name: c.Name, Owner: openapi_types.Email(c.Owner), Scopes: scopesOut(c.Scopes),
		CreatedAt: c.CreatedAt.UTC(), CreatedBy: createdBy(c),
	}
	if v.Secret != "" {
		secret := v.Secret
		out.ClientSecret = &secret
	}
	return out
}
