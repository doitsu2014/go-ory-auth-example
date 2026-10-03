// Package audit defines the append-only admin audit event.
package audit

import (
	"net/netip"
	"time"

	"github.com/google/uuid"
)

// Action is a stable audit action name.
type Action string

// Audit actions.
const (
	ActionCustomerDisabled        Action = "customer.disabled"
	ActionCustomerEnabled         Action = "customer.enabled"
	ActionCustomerSessionsRevoked Action = "customer.sessions_revoked"
	ActionAdminInvited            Action = "admin.invited"
	ActionAdminInvitationFailed   Action = "admin.invitation_failed"
	ActionAdminRoleChanged        Action = "admin.role_changed"
	ActionAdminBootstrapped       Action = "admin.bootstrapped"
	ActionAdminDeactivated        Action = "admin.deactivated_mfa_deadline"
	// Personal information (details hold field names, codes and ids only).
	ActionCustomerPIIUpdated  Action = "customer.pii.updated"
	ActionCustomerPIIErased   Action = "customer.pii.erased"
	ActionCustomerPIIRevealed Action = "customer.pii.revealed"
	ActionCustomerPIILookup   Action = "customer.pii.lookup"
)

// Target types.
const (
	TargetCustomer = "customer"
	TargetAdmin    = "admin"
)

// SystemActor is the actor id for actions taken by the service itself or by
// the operator CLI.
var SystemActor = uuid.Nil

// Event is one audit record. Details never hold PII beyond ids.
type Event struct {
	ID         int64
	OccurredAt time.Time
	ActorID    uuid.UUID
	Action     Action
	TargetType string
	TargetID   string
	RequestID  string
	ClientIP   *netip.Addr
	Details    map[string]any
}

// Cursor is a keyset position on (occurred_at, id).
type Cursor struct {
	OccurredAt time.Time
	ID         int64
}

// Filter narrows the audit list.
type Filter struct {
	TargetType *string
	TargetID   *string
	ActorID    *uuid.UUID
	After      *Cursor
	Limit      int
}
