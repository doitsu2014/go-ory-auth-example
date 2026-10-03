// Package audit defines the append-only admin audit event.
package audit

import (
	"net/netip"
	"strings"
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
	// ActionCustomerPIINameMigrated: the operator CLI moved the name trait
	// from Kratos into the encrypted personal info (system actor).
	ActionCustomerPIINameMigrated Action = "customer.pii.name_migrated"
	// ActionCustomerPIINameTraitRemoved: the name trait was removed from
	// Kratos without storing it (details: fields, reason erased |
	// already_migrated | empty | invalid; never the value).
	ActionCustomerPIINameTraitRemoved Action = "customer.pii.name_trait_removed"
	// Machine-to-machine service clients (details hold name and scopes,
	// never the secret).
	ActionServiceClientCreated       Action = "service_client.created"
	ActionServiceClientSecretRotated Action = "service_client.secret_rotated"
	ActionServiceClientDeleted       Action = "service_client.deleted"
	// Two-phase markers: committed before the Hydra call, so an outcome is
	// always attributable even if the final row cannot be written.
	ActionServiceClientSecretRotationStarted Action = "service_client.secret_rotation_started"
	ActionServiceClientSecretRotationFailed  Action = "service_client.secret_rotation_failed"
	ActionServiceClientDeletionStarted       Action = "service_client.deletion_started"
	ActionServiceClientDeletionFailed        Action = "service_client.deletion_failed"
)

// Target types.
const (
	TargetCustomer = "customer"
	TargetAdmin    = "admin"
	// TargetServiceClient targets carry the Hydra client_id.
	TargetServiceClient = "service_client"
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
	// Actions and ActionPrefixes, when either is set, restrict the list to
	// events whose action equals one of Actions or starts with one of
	// ActionPrefixes (the machine allowlist, see MachineFilter).
	Actions        []Action
	ActionPrefixes []string
}

// Matches reports whether the action passes the Actions/ActionPrefixes
// restriction of f (true when neither is set).
func (f Filter) Matches(a Action) bool {
	if len(f.Actions) == 0 && len(f.ActionPrefixes) == 0 {
		return true
	}
	for _, x := range f.Actions {
		if a == x {
			return true
		}
	}
	for _, p := range f.ActionPrefixes {
		if strings.HasPrefix(string(a), p) {
			return true
		}
	}
	return false
}
