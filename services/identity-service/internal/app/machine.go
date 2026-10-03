package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/audit"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/identity"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/machine"
)

// MachineCustomerView is the PII-free customer status machines may read
// (MD-8): no email, name, display name or personal info.
type MachineCustomerView struct {
	ID            uuid.UUID
	State         identity.State
	EmailVerified bool
	CreatedAt     time.Time
}

// MachineAuditEvent is an allowlisted audit event (§6 B1): no request id,
// client IP or non-allowlisted detail keys.
type MachineAuditEvent struct {
	ID         int64
	OccurredAt time.Time
	ActorID    uuid.UUID
	Action     audit.Action
	TargetType string
	TargetID   string
	Details    audit.MachineDetails
}

// MachineAuditPage is one page of the machine audit feed.
type MachineAuditPage struct {
	Items []MachineAuditEvent
	Next  *audit.Cursor
}

// MachineService implements the /m2m/v1 use cases (M2M-FR-05, 06).
type MachineService struct {
	Identities IdentityAdmin
	Audit      AuditRepo
}

// requireScope fails closed unless the principal holds s (the route policy
// checked it already; this is the use-case-level check).
func requireScope(p machine.Principal, s machine.Scope) error {
	if p.ClientID == "" {
		return ErrInvalidToken
	}
	if !p.HasScope(s) {
		return ErrInsufficientScope
	}
	return nil
}

// Customer returns a customer's status. Non-customers are ErrNotFound.
// Scope: customers:read.
func (s *MachineService) Customer(ctx context.Context, p machine.Principal, id uuid.UUID) (MachineCustomerView, error) {
	if err := requireScope(p, machine.ScopeCustomersRead); err != nil {
		return MachineCustomerView{}, err
	}
	ident, err := s.Identities.GetIdentity(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return MachineCustomerView{}, ErrNotFound
		}
		return MachineCustomerView{}, fmt.Errorf("get identity: %w", err)
	}
	if ident.SchemaID != string(identity.KindCustomer) {
		return MachineCustomerView{}, ErrNotFound
	}
	return MachineCustomerView{ID: ident.ID, State: ident.State, EmailVerified: ident.EmailVerified, CreatedAt: ident.CreatedAt}, nil
}

// AuditEvents lists the admin audit log filtered through the machine
// allowlist (M2M-FR-06 as amended by §6 B1): the action restriction is
// applied in the query (so paging stays exact) and again here, and details
// are reduced to the allowlisted keys. Scope: audit:read.
func (s *MachineService) AuditEvents(ctx context.Context, p machine.Principal, f audit.Filter) (MachineAuditPage, error) {
	if err := requireScope(p, machine.ScopeAuditRead); err != nil {
		return MachineAuditPage{}, err
	}
	f = audit.MachineFilter(f)
	f.Limit = ClampPageSize(f.Limit)
	want := f.Limit
	f.Limit = want + 1
	items, err := s.Audit.List(ctx, f)
	if err != nil {
		return MachineAuditPage{}, fmt.Errorf("list audit: %w", err)
	}
	page := MachineAuditPage{Items: make([]MachineAuditEvent, 0, min(len(items), want))}
	if len(items) > want {
		last := items[want-1]
		page.Next = &audit.Cursor{OccurredAt: last.OccurredAt, ID: last.ID}
		items = items[:want]
	}
	for _, e := range items {
		if !audit.MachineVisible(e.Action) {
			continue // defence in depth; the query already excluded it
		}
		page.Items = append(page.Items, MachineAuditEvent{
			ID: e.ID, OccurredAt: e.OccurredAt, ActorID: e.ActorID, Action: e.Action,
			TargetType: e.TargetType, TargetID: e.TargetID, Details: audit.MachineDetailsOf(e.Details),
		})
	}
	return page, nil
}
