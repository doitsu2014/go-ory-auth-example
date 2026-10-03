package app

import (
	"context"
	"fmt"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/audit"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/identity"
)

// AuditPage is one page of audit events.
type AuditPage struct {
	Items []audit.Event
	Next  *audit.Cursor
}

// AuditService reads the audit log (FR-12).
type AuditService struct {
	Authz Authorizer
	Audit AuditRepo
}

// List returns audit events newest first with keyset pagination.
// Permission: view_audit.
func (s *AuditService) List(ctx context.Context, a Actor, f audit.Filter) (AuditPage, error) {
	if err := require(ctx, s.Authz, a, identity.PermViewAudit); err != nil {
		return AuditPage{}, err
	}
	f.Limit = ClampPageSize(f.Limit)
	want := f.Limit
	f.Limit = want + 1
	items, err := s.Audit.List(ctx, f)
	if err != nil {
		return AuditPage{}, fmt.Errorf("list audit: %w", err)
	}
	page := AuditPage{Items: items}
	if len(items) > want {
		page.Items = items[:want]
		last := page.Items[want-1]
		page.Next = &audit.Cursor{OccurredAt: last.OccurredAt, ID: last.ID}
	}
	return page, nil
}
