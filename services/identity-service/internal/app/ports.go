// Package app holds the use cases (application services) and the ports they
// depend on. It imports only the domain; adapters implement the ports.
package app

import (
	"context"
	"errors"
	"net/netip"
	"time"

	"github.com/google/uuid"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/audit"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/identity"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/login"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/machine"
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

// MachineTokenVerifier resolves a machine-plane bearer token (a Hydra
// client_credentials JWT) to a service client principal.
// Errors: ErrInvalidToken, ErrDependencyUnavailable.
type MachineTokenVerifier interface {
	Verify(ctx context.Context, token string) (machine.Principal, error)
	// Invalidate drops the cached client status of the client (after a
	// secret rotation or a delete on this replica).
	Invalidate(clientID string)
}

// NewServiceClient is the input for registering a client at the
// authorisation server.
type NewServiceClient struct {
	// ClientID is chosen by identity-service (a random UUID) so the
	// registration can be tagged atomically and compensated after an
	// ambiguous failure.
	ClientID     string
	Registration machine.Registration
	CreatedBy    uuid.UUID
}

// ServiceClientAdmin is the authorisation server (Hydra) admin port. It only
// ever sees and returns clients managed by identity-service; anything else is
// ErrNotFound. Secrets are returned by Create only and never logged.
// Errors: ErrNotFound, ErrDependencyUnavailable.
type ServiceClientAdmin interface {
	Create(ctx context.Context, in NewServiceClient) (client machine.ServiceClient, secret string, err error)
	Get(ctx context.Context, clientID string) (machine.ServiceClient, error)
	List(ctx context.Context) ([]machine.ServiceClient, error)
	// SetSecret replaces the secret and records tokensValidAfter so tokens
	// issued earlier are rejected (§6 B3).
	SetSecret(ctx context.Context, clientID, secret string, tokensValidAfter time.Time) error
	Delete(ctx context.Context, clientID string) error
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

// NameTraitAdmin removes the legacy name trait from a Kratos identity
// (customer name migration, NAME-FR-08).
type NameTraitAdmin interface {
	ListIdentities(ctx context.Context, q IdentityQuery) (items []identity.Identity, nextPageToken string, err error)
	// RemoveTraitName removes traits.name only while it still equals old.
	// nil when the identity has no name trait;
	// ErrConflict when the name changed; ErrNotFound when the identity is gone.
	RemoveTraitName(ctx context.Context, id uuid.UUID, old identity.Name) error
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
	Name        []byte
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
	// SetName stores nameCT (sealed under keyID) without touching the other
	// columns: it inserts the record when there is none, else fills the name
	// only when the record is under keyID and has no name yet. set=false
	// otherwise. ErrConflict if keyID is not the subject's current key.
	SetName(ctx context.Context, identityID, keyID uuid.UUID, nameCT []byte) (set bool, err error)
}

// Repos is the set of repositories available inside a transaction.
type Repos struct {
	Profiles     ProfileRepo
	Audit        AuditRepo
	Idempotency  IdempotencyRepo
	Locks        Locker
	SubjectKeys  SubjectKeyRepo
	PersonalInfo CustomerPIIRepo
	Logins       LoginIdentifierRepo
	Dispatches   CourierDispatchRepo
}

// LoginKeys computes login lookup keys and seals login identifiers in the key
// manager (PLI DD-01, DD-03, ADR-0014). The HMAC key and the encryption key are
// distinct and never leave the key manager.
// Errors: ErrDependencyUnavailable, ErrDataIntegrity (OpenLogins
// authentication failure). Errors never carry inputs or key material.
type LoginKeys interface {
	// LookupKey returns the keyed HMAC of input (pinned key version).
	LookupKey(ctx context.Context, input []byte) (login.LookupKey, error)
	// SealLogin encrypts plaintext bound to ad; the ciphertext is opaque
	// ("vault:vN:…") and kekVersion is N.
	SealLogin(ctx context.Context, ad, plaintext []byte) (ciphertext string, kekVersion int, err error)
	// OpenLogins decrypts items in one round trip, in order. A per-item
	// authentication failure yields a nil entry and ErrDataIntegrity in errs
	// at the same index; a transport failure fails the whole call.
	OpenLogins(ctx context.Context, items []SealedLogin) (plaintexts [][]byte, errs []error, err error)
}

// SealedLogin is one OpenLogins input.
type SealedLogin struct {
	AD         []byte
	Ciphertext string
}

// LoginRecord is a login_identifier row: the encrypted login identifier
// stored under its Kratos handle (Pseudonym) and found by its LookupKey
// (PLI-FR-02, ADR-0014).
type LoginRecord struct {
	Pseudonym       login.Pseudonym
	LookupKey       login.LookupKey
	Kind            login.Kind
	Ciphertext      string
	KEKVersion      int
	IdentityID      *uuid.UUID
	BoundAt         *time.Time
	LegacyVerified  bool
	CreatedAt       time.Time
	LastValidatedAt time.Time
}

// LoginIdentifierRepo stores encrypted login identifiers. Errors never carry
// column values.
type LoginIdentifierRepo interface {
	// InsertIfAbsent stores r unless its pseudonym or lookup key exists.
	InsertIfAbsent(ctx context.Context, r LoginRecord) (inserted bool, err error)
	// Get returns the record or ErrNotFound.
	Get(ctx context.Context, p login.Pseudonym) (LoginRecord, error)
	// GetByLookupKey returns the record of an address or ErrNotFound.
	GetByLookupKey(ctx context.Context, k login.LookupKey) (LoginRecord, error)
	// GetMany returns the records that exist.
	GetMany(ctx context.Context, ps []login.Pseudonym) (map[login.Pseudonym]LoginRecord, error)
	// GetByIdentity returns the record bound to the identity or ErrNotFound.
	GetByIdentity(ctx context.Context, id uuid.UUID) (LoginRecord, error)
	// Bind binds the record to id when it is unbound or already bound to id;
	// with stale non-nil it also replaces exactly that binding (the caller
	// has checked that identity is gone). bound=false otherwise.
	// ErrConflict when id is bound to another record.
	Bind(ctx context.Context, p login.Pseudonym, id uuid.UUID, stale *uuid.UUID) (bound bool, err error)
	// SetLegacyVerified records the verification state of a migrated login.
	SetLegacyVerified(ctx context.Context, p login.Pseudonym, verified bool) error
	// Touch sets last_validated_at = now() (pre-registration check, A10).
	Touch(ctx context.Context, p login.Pseudonym) error
	// ListStaleUnbound returns up to limit unbound records not validated
	// since before, oldest first.
	ListStaleUnbound(ctx context.Context, before time.Time, limit int) ([]LoginRecord, error)
	// DeleteStaleUnbound deletes the record only while it is still unbound
	// and not validated since before (A10: a concurrent registration wins).
	DeleteStaleUnbound(ctx context.Context, p login.Pseudonym, before time.Time) (deleted bool, err error)
	// ListBound pages bound records ordered by pseudonym after the given one.
	ListBound(ctx context.Context, after login.Pseudonym, limit int) ([]LoginRecord, error)
	// ListAll pages every record ordered by pseudonym (re-wrap).
	ListAll(ctx context.Context, after login.Pseudonym, limit int) ([]LoginRecord, error)
	// Delete removes the record; with onlyIfUnbound it keeps a bound one.
	Delete(ctx context.Context, p login.Pseudonym, onlyIfUnbound bool) (deleted bool, err error)
	// DeleteForIdentity removes the record bound to the identity.
	DeleteForIdentity(ctx context.Context, id uuid.UUID) (deleted bool, err error)
	// UpdateCiphertext replaces the ciphertext if it still equals old.
	UpdateCiphertext(ctx context.Context, p login.Pseudonym, old, ciphertext string, kekVersion int) (bool, error)
	// CountUnbound returns the number of unbound records.
	CountUnbound(ctx context.Context) (int64, error)
	// DeleteErased deletes records bound to identities with a
	// customer.login.erased audit event (erasure ledger).
	DeleteErased(ctx context.Context) (int64, error)
}

// CourierDispatchRepo de-duplicates courier deliveries (A9): a key is
// reserved before sending and marked sent afterwards.
type CourierDispatchRepo interface {
	// Reserve claims key. reserved=false when the key was sent, or is pending
	// and newer than staleBefore (another delivery is in progress).
	Reserve(ctx context.Context, key [32]byte, staleBefore time.Time) (reserved bool, err error)
	// MarkSent finalises a reservation. A delivered message passes its
	// Delivery (counted by the quotas); a dropped one passes nil.
	MarkSent(ctx context.Context, key [32]byte, d *Delivery) error
	// CountDeliveriesTo counts deliveries to a recipient key since t.
	CountDeliveriesTo(ctx context.Context, recipientKey [32]byte, since time.Time) (int64, error)
	// CountSMS counts SMS deliveries since t, for one calling code or all ("").
	CountSMS(ctx context.Context, country string, since time.Time) (int64, error)
	// Release drops a pending reservation so a retry can send.
	Release(ctx context.Context, key [32]byte) error
	// Purge deletes keys older than before.
	Purge(ctx context.Context, before time.Time) (int64, error)
}

// Delivery describes a delivered courier message for quota accounting.
type Delivery struct {
	Channel      string
	Country      string // SMS calling code, "" for email
	RecipientKey [32]byte
}

// ErrPermanentDelivery marks a delivery the provider rejected for good
// (invalid recipient, SMTP 5xx, SMS 4xx): retrying cannot succeed.
var ErrPermanentDelivery = errors.New("delivery rejected permanently")

// SMSSender sends a text message to an E.164 number.
// Errors: ErrDependencyUnavailable (transient). Errors never carry the
// number or the text.
type SMSSender interface {
	SendSMS(ctx context.Context, toE164, text string) error
}

// LoginTraitAdmin rewrites customer login traits in Kratos (migration,
// PLI-FR-13) and finds identities by credential identifier.
type LoginTraitAdmin interface {
	// ListIdentities pages identities (all schemas).
	ListIdentities(ctx context.Context, q IdentityQuery) (items []identity.Identity, nextPageToken string, err error)
	// ReplaceLoginTrait replaces traits.email (legacy) with traits.login_id
	// only while the identity still has traits.email == oldEmail.
	// ErrConflict when it changed; ErrNotFound when the identity is gone.
	ReplaceLoginTrait(ctx context.Context, id uuid.UUID, oldEmail, loginID string) error
	// MarkLoginVerified marks the verifiable address equal to loginID as
	// verified (a trait rewrite resets it, spike S4b). ErrNotFound when the
	// identity or the address is gone.
	MarkLoginVerified(ctx context.Context, id uuid.UUID, loginID string) error
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
	// SendLoginMessage sends a verification/recovery message (PLI-FR-05).
	// Errors never carry the recipient or the message.
	SendLoginMessage(ctx context.Context, to string, m login.Message) error
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
