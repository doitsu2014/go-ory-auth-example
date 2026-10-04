package httpapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/oapi-codegen/nullable"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/adapter/httpapi/gen"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/audit"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/identity"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/login"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/profile"
)

// Server implements the generated strict server interface by delegating to
// the use cases. It only converts between wire and domain types.
type Server struct {
	Me        *app.MeService
	Customers *app.CustomerService
	Admins    *app.AdminService
	Audit     *app.AuditService
	// PersonalInfo serves /v1/me/personal-info and the admin PII routes.
	PersonalInfo *app.PersonalInfoService
	// Machine serves /m2m/v1; ServiceClients the admin service-client routes.
	Machine        *app.MachineService
	ServiceClients *app.ServiceClientService
	// Logins serves POST /v1/auth/identifiers (ADR-0013).
	Logins *app.LoginIdentifierService
}

var _ gen.StrictServerInterface = (*Server)(nil)

func actor(ctx context.Context) (app.Actor, error) {
	a, ok := ActorFrom(ctx)
	if !ok {
		return app.Actor{}, app.ErrUnauthenticated
	}
	return a, nil
}

// GetMe implements GET /v1/me.
func (s *Server) GetMe(ctx context.Context, _ gen.GetMeRequestObject) (gen.GetMeResponseObject, error) {
	a, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	v, err := s.Me.GetMe(ctx, a.Principal)
	if err != nil {
		return nil, err
	}
	return gen.GetMe200JSONResponse(toMe(v)), nil
}

// UpdateMe implements PATCH /v1/me.
func (s *Server) UpdateMe(ctx context.Context, req gen.UpdateMeRequestObject) (gen.UpdateMeResponseObject, error) {
	a, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	var patch profile.Patch
	if b := req.Body; b != nil {
		patch.DisplayName = optional(b.DisplayName)
		patch.AvatarURL = optional(b.AvatarUrl)
		patch.Locale = b.Locale
	}
	v, err := s.Me.UpdateMe(ctx, a.Principal, patch)
	if err != nil {
		return nil, err
	}
	return gen.UpdateMe200JSONResponse(toMe(v)), nil
}

// GetAdminMe implements GET /admin/v1/me.
func (s *Server) GetAdminMe(ctx context.Context, _ gen.GetAdminMeRequestObject) (gen.GetAdminMeResponseObject, error) {
	a, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	v, err := s.Admins.Me(ctx, a)
	if err != nil {
		return nil, err
	}
	out := gen.AdminMe{
		Id: v.Principal.IdentityID, Email: openapi_types.Email(v.Principal.Email),
		Name: personName(v.Principal.Name), Aal: gen.AdminMeAal(v.Principal.AAL),
		Roles: make([]gen.Role, 0, len(v.Roles)), Permissions: make([]gen.Permission, 0, len(v.Permissions)),
		SessionExpiresAt: v.Principal.ExpiresAt.UTC(),
	}
	for _, r := range v.Roles {
		out.Roles = append(out.Roles, gen.Role(r))
	}
	for _, p := range v.Permissions {
		out.Permissions = append(out.Permissions, gen.Permission(p))
	}
	return gen.GetAdminMe200JSONResponse(out), nil
}

// ListCustomers implements GET /admin/v1/customers.
func (s *Server) ListCustomers(ctx context.Context, req gen.ListCustomersRequestObject) (gen.ListCustomersResponseObject, error) {
	a, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	size, err := pageSize(req.Params.PageSize)
	if err != nil {
		return nil, err
	}
	q := app.CustomerQuery{PageSize: size, PageToken: deref(req.Params.PageToken)}
	if req.Params.State != nil {
		st := identity.State(*req.Params.State)
		if st != identity.StateActive && st != identity.StateInactive {
			return nil, app.NewValidationError("state", "invalid")
		}
		q.State = &st
	}
	page, err := s.Customers.List(ctx, a, q)
	if err != nil {
		return nil, err
	}
	out := gen.CustomerPage{Items: make([]gen.Customer, 0, len(page.Items))}
	for _, c := range page.Items {
		out.Items = append(out.Items, toCustomer(c))
	}
	if page.NextPageToken != "" {
		out.NextPageToken = &page.NextPageToken
	}
	return gen.ListCustomers200JSONResponse(out), nil
}

// GetCustomer implements GET /admin/v1/customers/{id}.
func (s *Server) GetCustomer(ctx context.Context, req gen.GetCustomerRequestObject) (gen.GetCustomerResponseObject, error) {
	a, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	c, err := s.Customers.Get(ctx, a, req.Id)
	if err != nil {
		return nil, err
	}
	return gen.GetCustomer200JSONResponse(toCustomer(c)), nil
}

// DisableCustomer implements POST /admin/v1/customers/{id}/disable.
func (s *Server) DisableCustomer(ctx context.Context, req gen.DisableCustomerRequestObject) (gen.DisableCustomerResponseObject, error) {
	a, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	reason, err := reasonOf(req.Body)
	if err != nil {
		return nil, err
	}
	st, err := s.Customers.Disable(ctx, a, req.Id, reason)
	if err != nil {
		return nil, err
	}
	return gen.DisableCustomer200JSONResponse{Id: req.Id, State: gen.IdentityState(st)}, nil
}

// EnableCustomer implements POST /admin/v1/customers/{id}/enable.
func (s *Server) EnableCustomer(ctx context.Context, req gen.EnableCustomerRequestObject) (gen.EnableCustomerResponseObject, error) {
	a, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	reason, err := reasonOf(req.Body)
	if err != nil {
		return nil, err
	}
	st, err := s.Customers.Enable(ctx, a, req.Id, reason)
	if err != nil {
		return nil, err
	}
	return gen.EnableCustomer200JSONResponse{Id: req.Id, State: gen.IdentityState(st)}, nil
}

// RevokeCustomerSessions implements DELETE /admin/v1/customers/{id}/sessions.
func (s *Server) RevokeCustomerSessions(ctx context.Context, req gen.RevokeCustomerSessionsRequestObject) (gen.RevokeCustomerSessionsResponseObject, error) {
	a, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.Customers.RevokeSessions(ctx, a, req.Id); err != nil {
		return nil, err
	}
	return gen.RevokeCustomerSessions204Response{}, nil
}

// ListAdmins implements GET /admin/v1/admins.
func (s *Server) ListAdmins(ctx context.Context, req gen.ListAdminsRequestObject) (gen.ListAdminsResponseObject, error) {
	a, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	size, err := pageSize(req.Params.PageSize)
	if err != nil {
		return nil, err
	}
	page, err := s.Admins.List(ctx, a, size, deref(req.Params.PageToken))
	if err != nil {
		return nil, err
	}
	out := gen.AdminPage{Items: make([]gen.Admin, 0, len(page.Items))}
	for _, v := range page.Items {
		out.Items = append(out.Items, toAdmin(v, true))
	}
	if page.NextPageToken != "" {
		out.NextPageToken = &page.NextPageToken
	}
	return gen.ListAdmins200JSONResponse(out), nil
}

// InviteAdmin implements POST /admin/v1/admins.
func (s *Server) InviteAdmin(ctx context.Context, req gen.InviteAdminRequestObject) (gen.InviteAdminResponseObject, error) {
	a, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, app.NewValidationError("body", "required")
	}
	role, err := identity.ParseRole(string(req.Body.Role))
	if err != nil {
		return nil, app.NewValidationError("role", "invalid")
	}
	in := app.InviteRequest{Email: string(req.Body.Email), Role: role}
	if n := req.Body.Name; n != nil {
		in.Name = identity.Name{First: deref(n.First), Last: deref(n.Last)}
	}
	out, err := s.Admins.Invite(ctx, a, req.Params.IdempotencyKey.String(), in)
	if err != nil {
		return nil, err
	}
	return gen.InviteAdmin201JSONResponse{
		Id: out.ID, Email: openapi_types.Email(out.Email), Role: gen.Role(out.Role),
		InvitationExpiresAt: out.InvitationExpiresAt.UTC(),
	}, nil
}

// ChangeAdminRole implements PUT /admin/v1/admins/{id}/role.
func (s *Server) ChangeAdminRole(ctx context.Context, req gen.ChangeAdminRoleRequestObject) (gen.ChangeAdminRoleResponseObject, error) {
	a, err := actor(ctx)
	if err != nil {
		return nil, err
	}
	if req.Body == nil {
		return nil, app.NewValidationError("body", "required")
	}
	role, err := identity.ParseRole(string(req.Body.Role))
	if err != nil {
		return nil, app.NewValidationError("role", "invalid")
	}
	v, err := s.Admins.ChangeRole(ctx, a, req.Id, role)
	if err != nil {
		return nil, err
	}
	return gen.ChangeAdminRole200JSONResponse(toAdmin(v, false)), nil
}

// ListAuditEvents implements GET /admin/v1/audit-events.
func (s *Server) ListAuditEvents(ctx context.Context, req gen.ListAuditEventsRequestObject) (gen.ListAuditEventsResponseObject, error) {
	a, err := actor(ctx)
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
	page, err := s.Audit.List(ctx, a, f)
	if err != nil {
		return nil, err
	}
	out := gen.AuditEventPage{Items: make([]gen.AuditEvent, 0, len(page.Items))}
	for _, e := range page.Items {
		out.Items = append(out.Items, gen.AuditEvent{
			Id: e.ID, OccurredAt: e.OccurredAt.UTC(), ActorId: e.ActorID, Action: string(e.Action),
			TargetType: e.TargetType, TargetId: e.TargetID, RequestId: e.RequestID, Details: e.Details,
		})
	}
	if page.Next != nil {
		t := encodeAuditCursor(*page.Next)
		out.NextPageToken = &t
	}
	return gen.ListAuditEvents200JSONResponse(out), nil
}

// --- conversions ---

type auditCursorWire struct {
	T time.Time `json:"t"`
	I int64     `json:"i"`
}

func encodeAuditCursor(c audit.Cursor) string {
	b, _ := json.Marshal(auditCursorWire{T: c.OccurredAt, I: c.ID})
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeAuditCursor(s string) (audit.Cursor, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return audit.Cursor{}, err
	}
	var w auditCursorWire
	if err := json.Unmarshal(b, &w); err != nil {
		return audit.Cursor{}, err
	}
	return audit.Cursor{OccurredAt: w.T, ID: w.I}, nil
}

// pageSize validates the optional page_size (1..100); 0 means default.
func pageSize(p *int) (int, error) {
	if p == nil {
		return 0, nil
	}
	if *p < 1 || *p > app.MaxPageSize {
		return 0, app.NewValidationError("page_size", "out_of_range")
	}
	return *p, nil
}

func deref[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}

func optional(n nullable.Nullable[string]) profile.OptionalString {
	if !n.IsSpecified() {
		return profile.OptionalString{}
	}
	if n.IsNull() {
		return profile.OptionalString{Set: true}
	}
	v, _ := n.Get()
	return profile.OptionalString{Set: true, Value: &v}
}

func nullableOf(p *string) nullable.Nullable[string] {
	if p == nil {
		return nullable.NewNullNullable[string]()
	}
	return nullable.NewNullableWithValue(*p)
}

func personName(n identity.Name) *gen.PersonName {
	if n.First == "" && n.Last == "" {
		return nil
	}
	out := &gen.PersonName{}
	if n.First != "" {
		f := n.First
		out.First = &f
	}
	if n.Last != "" {
		l := n.Last
		out.Last = &l
	}
	return out
}

// toMe never fills the deprecated name: a customer's name is encrypted
// personal info (NAME-FR-07), not a Kratos trait.
// The login identifier comes from the vault (PLI-FR-07); the deprecated
// email is set only for email logins.
func toMe(v app.MeView) gen.Me {
	out := gen.Me{
		Id: v.Principal.IdentityID, EmailVerified: v.Principal.EmailVerified,
		Login:       gen.LoginIdentifier{Type: gen.LoginType(v.Login.Kind()), Value: v.Login.Value()},
		DisplayName: nullableOf(v.Profile.DisplayName), AvatarUrl: nullableOf(v.Profile.AvatarURL),
		Locale: v.Profile.Locale, CreatedAt: v.Profile.CreatedAt.UTC(),
	}
	if v.Login.Kind() == login.KindEmail {
		e := openapi_types.Email(v.Login.Value())
		out.Email = &e
	}
	return out
}

// toCustomer never fills the deprecated name (NAME-FR-07); admins see the
// masked name through the personal-info endpoints.
// The customer's contact is only ever the masked login (PLI-FR-10).
func toCustomer(c app.CustomerView) gen.Customer {
	out := gen.Customer{
		Id: c.Identity.ID, EmailVerified: c.Identity.EmailVerified,
		State:       gen.IdentityState(c.Identity.State),
		DisplayName: nullableOf(c.DisplayName), CreatedAt: c.Identity.CreatedAt.UTC(),
		Login: nullable.NewNullNullable[gen.MaskedLogin](), LoginUnavailable: c.LoginUnavailable,
	}
	if c.Login != nil {
		out.Login.Set(toMaskedLogin(*c.Login))
	}
	return out
}

func toMaskedLogin(m app.MaskedLogin) gen.MaskedLogin {
	return gen.MaskedLogin{Type: gen.LoginType(m.Kind), Masked: m.Masked}
}

// ResolveLoginIdentifier implements POST /v1/auth/identifiers (public).
func (s *Server) ResolveLoginIdentifier(ctx context.Context, req gen.ResolveLoginIdentifierRequestObject) (gen.ResolveLoginIdentifierResponseObject, error) {
	if req.Body == nil {
		return nil, app.NewValidationError("body", "required")
	}
	ip, ok := PublicClientIPFrom(ctx)
	if !ok || s.Logins == nil {
		return nil, app.ErrDependencyUnavailable
	}
	id, err := s.Logins.Resolve(ctx, app.ResolveRequest{
		ClientIP: ip, Type: string(req.Body.Type), Value: req.Body.Value, Purpose: string(req.Body.Purpose),
	})
	if err != nil {
		return nil, err
	}
	noStore := "no-store"
	return gen.ResolveLoginIdentifier200JSONResponse{
		Body:    gen.ResolvedLoginIdentifier{Identifier: id},
		Headers: gen.ResolveLoginIdentifier200ResponseHeaders{CacheControl: &noStore},
	}, nil
}

// toAdmin converts an admin. The contract requires a role; an admin identity
// without any role tuple has no permissions and is reported as "support"
// (lowest role) — see the contract note in the report.
func toAdmin(v app.AdminView, withMFA bool) gen.Admin {
	role := v.Role
	if !v.HasRole {
		role = identity.RoleSupport
	}
	out := gen.Admin{
		Id: v.Identity.ID, Email: openapi_types.Email(v.Identity.Email), Name: personName(v.Identity.Name),
		Role: gen.Role(role), State: gen.IdentityState(v.Identity.State), CreatedAt: v.Identity.CreatedAt.UTC(),
	}
	if withMFA {
		mfa := v.Identity.HasTOTP
		out.MfaEnrolled = &mfa
	}
	return out
}

func reasonOf(b *gen.ReasonRequest) (*string, error) {
	if b == nil || b.Reason == nil {
		return nil, nil
	}
	if len([]rune(*b.Reason)) > 500 {
		return nil, app.NewValidationError("reason", "too_long")
	}
	return b.Reason, nil
}
