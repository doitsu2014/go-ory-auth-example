// Package app holds the use cases (application services) and the ports they
// depend on. It imports only the domain; adapters implement the ports.
package app

import (
	"context"
	"net/netip"
	"time"

	"github.com/google/uuid"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/audit"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/identity"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/profile"
)

// CredentialKind says how the caller presented its session.
type CredentialKind int

// Credential kinds. Each is bound to one plane.
const (
	CredentialToken  CredentialKind = iota + 1 // Authorization: Bearer (customer plane)
	CredentialCookie                           // ory_kratos_session cookie (admin plane)
)

// Credential is an opaque session credential presented by a client.
type Credential struct {
	Kind  CredentialKind
	Value string
}

// SessionVerifier resolves a credential to a Principal.
// Errors: ErrUnauthenticated, ErrAAL2Required, ErrDependencyUnavailable.
type SessionVerifier interface {
	Verify(ctx context.Context, cred Credential) (identity.Principal, error)
	// Invalidate drops cached sessions of the identity.
	Invalidate(identityID uuid.UUID)
}

// NewIdentity is the input for creating a Kratos identity.
type NewIdentity struct {
	SchemaID string
	Email    string
	Name     identity.Name
}

// IdentityQuery lists identities through the Kratos admin API.
type IdentityQuery struct {
	Email       string // exact credentials identifier
	PageSize    int
	PageToken   string
	IncludeTOTP bool
}

// RecoveryCode is an admin-created recovery code. It is a secret: it is
// emailed or printed to an operator, never returned by the API or logged.
type RecoveryCode struct {
	Link      string
	Code      string
	ExpiresAt time.Time
}

// IdentityAdmin is the Kratos admin API port.
type IdentityAdmin interface {
	GetIdentity(ctx context.Context, id uuid.UUID) (identity.Identity, error)
	ListIdentities(ctx context.Context, q IdentityQuery) (items []identity.Identity, nextPageToken string, err error)
	CreateIdentity(ctx context.Context, in NewIdentity) (identity.Identity, error)
	DeleteIdentity(ctx context.Context, id uuid.UUID) error
	SetState(ctx context.Context, id uuid.UUID, state identity.State) error
	RevokeIdentitySessions(ctx context.Context, id uuid.UUID) error
	RevokeSession(ctx context.Context, sessionID uuid.UUID) error
	CreateRecoveryCode(ctx context.Context, id uuid.UUID, ttl time.Duration) (RecoveryCode, error)
}

// Authorizer checks Keto permissions on Console:main for a User subject.
type Authorizer interface {
	Check(ctx context.Context, subject uuid.UUID, perm identity.Permission) (bool, error)
}

// RoleStore reads and writes role tuples on Console:main.
type RoleStore interface {
	RolesOf(ctx context.Context, subject uuid.UUID) ([]identity.Role, error)
	AllAssignments(ctx context.Context) (map[uuid.UUID][]identity.Role, error)
	// SetRole atomically removes every other role of the subject and grants role.
	SetRole(ctx context.Context, subject uuid.UUID, role identity.Role) error
	RemoveAll(ctx context.Context, subject uuid.UUID) error
}

// ProfileRepo stores profiles.
type ProfileRepo interface {
	// Ensure inserts a profile if absent (idempotent).
	Ensure(ctx context.Context, id uuid.UUID, kind identity.Kind) error
	Get(ctx context.Context, id uuid.UUID) (profile.Profile, error)
	GetMany(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]profile.Profile, error)
	Update(ctx context.Context, p profile.Profile) (profile.Profile, error)
}

// AuditRepo appends and lists audit events.
type AuditRepo interface {
	Append(ctx context.Context, e audit.Event) error
	List(ctx context.Context, f audit.Filter) ([]audit.Event, error)
}

// IdempotencyRecord is a stored response for an Idempotency-Key. A record
// with ResponseCode 0 is a pending reservation (request in progress).
type IdempotencyRecord struct {
	RequestHash  []byte
	ResponseCode int
	ResponseBody []byte
	CreatedAt    time.Time
}

// Pending reports whether the original request is still in progress.
func (r IdempotencyRecord) Pending() bool { return r.ResponseCode == 0 }

// IdempotencyRepo stores responses of creating POSTs.
type IdempotencyRepo interface {
	// Reserve atomically claims (actor, key) with a pending record. Rows
	// created at or before notBefore are expired and replaced. When the key
	// is already taken it returns the existing record and reserved=false.
	Reserve(ctx context.Context, actor uuid.UUID, key string, requestHash []byte, notBefore time.Time) (existing *IdempotencyRecord, reserved bool, err error)
	// Complete stores the final response of a pending reservation.
	Complete(ctx context.Context, actor uuid.UUID, key string, code int, body []byte) error
	// Release drops a reservation so the client can retry.
	Release(ctx context.Context, actor uuid.UUID, key string) error
	Purge(ctx context.Context, before time.Time) (int64, error)
}

// Locker takes transaction-scoped advisory locks.
type Locker interface {
	XactLock(ctx context.Context, key int64) error
}

// KeyContext identifies the subject key a wrapped DEK belongs to. Its
// associated data binds the wrapped DEK to the subject (§9 A9), so a wrapped
// DEK copied to another subject_key row cannot be unwrapped.
type KeyContext struct {
	IdentityID uuid.UUID
	KeyID      uuid.UUID
}

// AssociatedData is identity_id ‖ "/" ‖ key_id.
func (k KeyContext) AssociatedData() []byte {
	return []byte(k.IdentityID.String() + "/" + k.KeyID.String())
}

// Wrapped is a DEK encrypted by the key-encryption key (KEK) in the key
// manager. Ciphertext is opaque (e.g. "vault:v3:…").
type Wrapped struct {
	Ciphertext string
	KEKName    string
	KEKVersion int
}

// BlindIndex is a keyed hash (HMAC-SHA256) used for equality lookups.
type BlindIndex struct {
	Sum        []byte
	KeyVersion int
}

// KeyManager wraps data keys and computes blind indexes without exposing
// the KEK or the index key to the service (DD-1).
// Errors: ErrDependencyUnavailable (unreachable, sealed, token rejected),
// ErrDataIntegrity (unwrap authentication failure). Errors never carry key
// material or inputs.
//
// Re-wrapping after a KEK rotation is UnwrapDEK + WrapDEK with the same
// context (Transit rewrap does not support associated data, §9 A9).
type KeyManager interface {
	WrapDEK(ctx context.Context, kc KeyContext, dek []byte) (Wrapped, error)
	UnwrapDEK(ctx context.Context, kc KeyContext, w Wrapped) ([]byte, error)
	BlindIndex(ctx context.Context, input []byte) (BlindIndex, error)
}

// SubjectKey is a customer's wrapped DEK (one per customer).
type SubjectKey struct {
	KeyID       uuid.UUID
	IdentityID  uuid.UUID
	Wrapped     Wrapped
	CreatedAt   time.Time
	RewrappedAt *time.Time
}

// Context returns the key context of the subject key.
func (k SubjectKey) Context() KeyContext { return KeyContext{IdentityID: k.IdentityID, KeyID: k.KeyID} }

// SubjectKeyRepo stores wrapped DEKs. Errors never carry column values.
type SubjectKeyRepo interface {
	// Get returns the subject's key or ErrNotFound.
	Get(ctx context.Context, identityID uuid.UUID) (SubjectKey, error)
	// Insert stores k unless the subject already has a key; inserted=false
	// then (the caller re-reads).
	Insert(ctx context.Context, k SubjectKey) (stored SubjectKey, inserted bool, err error)
	// Delete removes the subject's key (cascading to its encrypted record)
	// and returns the deleted key ids.
	Delete(ctx context.Context, identityID uuid.UUID) ([]uuid.UUID, error)
	// ListForRewrap returns keys ordered by key_id after the given id.
	ListForRewrap(ctx context.Context, after uuid.UUID, limit int) ([]SubjectKey, error)
	// UpdateWrapped replaces the wrapped DEK if it still equals old.
	UpdateWrapped(ctx context.Context, keyID uuid.UUID, old string, w Wrapped) (bool, error)
	// DeleteErased deletes every key created at or before the subject's
	// latest customer.pii.erased audit event (erasure ledger, §9 B1).
	DeleteErased(ctx context.Context) (int64, error)
}

// EncryptedPII is a customer_pii row: per-column ciphertexts (nil = unset)
// and the phone blind index.
type EncryptedPII struct {
	IdentityID  uuid.UUID
	KeyID       uuid.UUID
	Phone       []byte
	PhoneBidx   *BlindIndex
	DateOfBirth []byte
	Address     []byte
	NationalID  []byte
	UpdatedAt   time.Time
}

// CustomerPIIRepo stores encrypted personal info. Errors never carry column
// values.
type CustomerPIIRepo interface {
	// Get returns the subject's record or ErrNotFound.
	Get(ctx context.Context, identityID uuid.UUID) (EncryptedPII, error)
	// Upsert stores rec and returns its updated_at. ErrConflict if the key
	// does not match the subject's current key (concurrent erase).
	Upsert(ctx context.Context, rec EncryptedPII) (time.Time, error)
	// FindByPhoneBidx returns at most limit records with the blind index.
	FindByPhoneBidx(ctx context.Context, bidx BlindIndex, limit int) ([]EncryptedPII, error)
}

// Repos is the set of repositories available inside a transaction.
type Repos struct {
	Profiles     ProfileRepo
	Audit        AuditRepo
	Idempotency  IdempotencyRepo
	Locks        Locker
	SubjectKeys  SubjectKeyRepo
	PersonalInfo CustomerPIIRepo
}

// TxRunner runs fn in one database transaction. Never span an Ory call.
type TxRunner interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context, r Repos) error) error
}

// Invitation is the admin invitation email content.
type Invitation struct {
	To        string
	Name      identity.Name
	Role      identity.Role
	Link      string
	Code      string
	ExpiresAt time.Time
}

// Mailer sends the service's own emails.
type Mailer interface {
	SendInvitation(ctx context.Context, inv Invitation) error
}

// Clock is the time source.
type Clock interface{ Now() time.Time }

// SystemClock is the real clock.
type SystemClock struct{}

// Now returns the current UTC time.
func (SystemClock) Now() time.Time { return time.Now().UTC() }

// Actor is the authenticated caller of a use case plus request metadata for
// auditing.
type Actor struct {
	Principal identity.Principal
	RequestID string
	ClientIP  *netip.Addr
}
