// Package identity holds the pure domain types describing who is calling:
// the authenticated Principal, identity kinds, AAL, roles and permissions.
package identity

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// Kind is the identity population, derived from the Kratos schema id.
type Kind string

// Identity kinds (ADR-0004).
const (
	KindCustomer Kind = "customer"
	KindAdmin    Kind = "admin"
)

// ParseKind maps a Kratos schema id to a Kind.
func ParseKind(schemaID string) (Kind, bool) {
	switch Kind(schemaID) {
	case KindCustomer, KindAdmin:
		return Kind(schemaID), true
	}
	return "", false
}

// AAL is the authenticator assurance level of a session.
type AAL string

// Assurance levels.
const (
	AAL1 AAL = "aal1"
	AAL2 AAL = "aal2"
)

// State is the Kratos identity state.
type State string

// Identity states.
const (
	StateActive   State = "active"
	StateInactive State = "inactive"
)

// Name is the optional person name trait.
type Name struct {
	First string
	Last  string
}

// Principal is the authenticated caller. Handlers only ever see this, never
// the raw Kratos session.
type Principal struct {
	IdentityID      uuid.UUID
	SessionID       uuid.UUID
	Kind            Kind
	AAL             AAL
	AuthenticatedAt time.Time
	ExpiresAt       time.Time
	// Email is the plaintext email trait: admins and legacy (unmigrated)
	// customers only. A customer with a pseudonymous login has none.
	Email string
	// LoginID is the Kratos login identifier: the pseudonym
	// "<base32>@login.invalid" for customers (ADR-0013), else the email.
	LoginID string
	// EmailVerified reports that the login identifier (email or phone) is
	// verified; the name is kept for compatibility (api-contract §4).
	EmailVerified     bool
	Name              Name
	IdentityCreatedAt time.Time
}

// Role is the closed set of admin roles exposed by the API.
type Role string

// Admin roles.
const (
	RoleSupport    Role = "support"
	RoleAdmin      Role = "admin"
	RoleSuperAdmin Role = "super_admin"
)

// ErrUnknownRole is returned for roles outside the closed enum.
var ErrUnknownRole = errors.New("unknown role")

// roleRelations is the only mapping from API roles to Keto relations; client
// input never forms a tuple directly (T8b).
var roleRelations = map[Role]string{
	RoleSupport:    "supporters",
	RoleAdmin:      "admins",
	RoleSuperAdmin: "super_admins",
}

// AllRoles lists roles from most to least privileged.
var AllRoles = []Role{RoleSuperAdmin, RoleAdmin, RoleSupport}

// ParseRole validates a role against the closed enum.
func ParseRole(s string) (Role, error) {
	r := Role(s)
	if _, ok := roleRelations[r]; !ok {
		return "", ErrUnknownRole
	}
	return r, nil
}

// Relation returns the Keto relation for the role.
func (r Role) Relation() (string, error) {
	rel, ok := roleRelations[r]
	if !ok {
		return "", ErrUnknownRole
	}
	return rel, nil
}

// RoleFromRelation maps a Keto relation back to a role.
func RoleFromRelation(rel string) (Role, bool) {
	for r, v := range roleRelations {
		if v == rel {
			return r, true
		}
	}
	return "", false
}

// HighestRole returns the most privileged role in rs.
func HighestRole(rs []Role) (Role, bool) {
	for _, candidate := range AllRoles {
		for _, r := range rs {
			if r == candidate {
				return r, true
			}
		}
	}
	return "", false
}

// Permission is a Keto permit on Console:main.
type Permission string

// Console permissions (deploy/ory/keto/namespaces.keto.ts).
const (
	PermViewCustomers   Permission = "view_customers"
	PermManageCustomers Permission = "manage_customers"
	PermManageAdmins    Permission = "manage_admins"
	PermViewAudit       Permission = "view_audit"
	// PermRevealCustomerPII allows the full (unmasked) view of customer PII;
	// every use is audited.
	PermRevealCustomerPII Permission = "reveal_customer_pii"
	// PermManageServiceClients registers, rotates and deletes machine
	// clients (super_admins only).
	PermManageServiceClients Permission = "manage_service_clients"
)

// AllPermissions is the set reported by /admin/v1/me.
var AllPermissions = []Permission{
	PermViewCustomers, PermManageCustomers, PermManageAdmins, PermViewAudit, PermRevealCustomerPII, PermManageServiceClients,
}

// Identity is a Kratos identity as seen through the admin API.
type Identity struct {
	ID       uuid.UUID
	SchemaID string
	State    State
	// Email, LoginID and EmailVerified as in Principal.
	Email         string
	LoginID       string
	EmailVerified bool
	Name          Name
	// HasNameTrait reports that traits.name is present, even as an empty
	// object (legacy customer identities, removed by the name migration).
	HasNameTrait bool
	HasTOTP      bool
	// TOTPCreatedAt is when the TOTP credential was enrolled (zero if none
	// or unknown).
	TOTPCreatedAt time.Time
	CreatedAt     time.Time
	// StateChangedAt is when the state last changed (zero if never).
	StateChangedAt time.Time
}

// MFADeadlineAnchor is the start of the MFA enrolment window: identity
// creation, or the last (re)activation by an operator, whichever is later.
func (i Identity) MFADeadlineAnchor() time.Time {
	if i.StateChangedAt.After(i.CreatedAt) {
		return i.StateChangedAt
	}
	return i.CreatedAt
}

// MFADeadline is the end of the MFA enrolment window.
func (i Identity) MFADeadline(grace time.Duration) time.Time {
	return i.MFADeadlineAnchor().Add(grace)
}

// IsCustomer reports whether the identity uses the customer schema.
func (i Identity) IsCustomer() bool { return i.SchemaID == string(KindCustomer) }

// Kind returns the identity kind, if the schema is known.
func (i Identity) Kind() (Kind, bool) { return ParseKind(i.SchemaID) }
