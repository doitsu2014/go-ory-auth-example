// Package testutil holds in-memory fakes of the app ports for unit tests.
package testutil

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/audit"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/identity"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/profile"
)

// FixedClock returns a settable time.
type FixedClock struct{ T time.Time }

// Now implements app.Clock.
func (c *FixedClock) Now() time.Time { return c.T }

// Verifier is a fake SessionVerifier keyed by credential value.
type Verifier struct {
	mu          sync.Mutex
	Sessions    map[string]identity.Principal
	Errs        map[string]error
	Invalidated []uuid.UUID
	Calls       int
}

// NewVerifier creates an empty fake verifier.
func NewVerifier() *Verifier {
	return &Verifier{Sessions: map[string]identity.Principal{}, Errs: map[string]error{}}
}

// Verify implements app.SessionVerifier.
func (v *Verifier) Verify(_ context.Context, c app.Credential) (identity.Principal, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.Calls++
	if err, ok := v.Errs[c.Value]; ok {
		return identity.Principal{}, err
	}
	p, ok := v.Sessions[c.Value]
	if !ok {
		return identity.Principal{}, app.ErrUnauthenticated
	}
	return p, nil
}

// Invalidate implements app.SessionVerifier.
func (v *Verifier) Invalidate(id uuid.UUID) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.Invalidated = append(v.Invalidated, id)
}

// Authz is a fake Authorizer: grants[subject][perm].
type Authz struct {
	mu     sync.Mutex
	Grants map[uuid.UUID]map[identity.Permission]bool
	Err    error
	Calls  int
}

// NewAuthz creates an empty fake authorizer.
func NewAuthz() *Authz { return &Authz{Grants: map[uuid.UUID]map[identity.Permission]bool{}} }

// Grant allows perms for subject.
func (a *Authz) Grant(subject uuid.UUID, perms ...identity.Permission) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.Grants[subject] == nil {
		a.Grants[subject] = map[identity.Permission]bool{}
	}
	for _, p := range perms {
		a.Grants[subject][p] = true
	}
}

// GrantRole grants the permissions implied by a role (mirrors the OPL).
func (a *Authz) GrantRole(subject uuid.UUID, r identity.Role) {
	switch r {
	case identity.RoleSuperAdmin:
		a.Grant(subject, identity.AllPermissions...)
	case identity.RoleAdmin:
		a.Grant(subject, identity.PermViewCustomers, identity.PermManageCustomers, identity.PermViewAudit, identity.PermRevealCustomerPII)
	case identity.RoleSupport:
		a.Grant(subject, identity.PermViewCustomers)
	}
}

// Check implements app.Authorizer.
func (a *Authz) Check(_ context.Context, s uuid.UUID, p identity.Permission) (bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.Calls++
	if a.Err != nil {
		return false, a.Err
	}
	return a.Grants[s][p], nil
}

// Roles is a fake RoleStore.
type Roles struct {
	mu  sync.Mutex
	M   map[uuid.UUID][]identity.Role
	Err error
}

// NewRoles creates an empty fake role store.
func NewRoles() *Roles { return &Roles{M: map[uuid.UUID][]identity.Role{}} }

// RolesOf implements app.RoleStore.
func (r *Roles) RolesOf(_ context.Context, s uuid.UUID) ([]identity.Role, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return nil, r.Err
	}
	return append([]identity.Role{}, r.M[s]...), nil
}

// AllAssignments implements app.RoleStore.
func (r *Roles) AllAssignments(context.Context) (map[uuid.UUID][]identity.Role, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[uuid.UUID][]identity.Role{}
	for k, v := range r.M {
		out[k] = append([]identity.Role{}, v...)
	}
	return out, nil
}

// CountRole implements app.RoleStore.
func (r *Roles) CountRole(_ context.Context, role identity.Role) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, rs := range r.M {
		for _, x := range rs {
			if x == role {
				n++
			}
		}
	}
	return n, nil
}

// SetRole implements app.RoleStore.
func (r *Roles) SetRole(_ context.Context, s uuid.UUID, role identity.Role) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	r.M[s] = []identity.Role{role}
	return nil
}

// RemoveAll implements app.RoleStore.
func (r *Roles) RemoveAll(_ context.Context, s uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.M, s)
	return nil
}

// Identities is a fake Kratos admin API.
type Identities struct {
	mu              sync.Mutex
	M               map[uuid.UUID]identity.Identity
	RevokedSessions []uuid.UUID
	RevokedAll      []uuid.UUID
	Deleted         []uuid.UUID
	Recovery        app.RecoveryCode
	RecoveryErr     error
	Err             error
	Clock           app.Clock
}

// NewIdentities creates an empty fake.
func NewIdentities(c app.Clock) *Identities {
	return &Identities{M: map[uuid.UUID]identity.Identity{}, Clock: c}
}

// Add stores an identity.
func (f *Identities) Add(i identity.Identity) identity.Identity {
	f.mu.Lock()
	defer f.mu.Unlock()
	if i.ID == uuid.Nil {
		i.ID = uuid.New()
	}
	if i.State == "" {
		i.State = identity.StateActive
	}
	f.M[i.ID] = i
	return i
}

// GetIdentity implements app.IdentityAdmin.
func (f *Identities) GetIdentity(_ context.Context, id uuid.UUID) (identity.Identity, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return identity.Identity{}, f.Err
	}
	i, ok := f.M[id]
	if !ok {
		return identity.Identity{}, app.ErrNotFound
	}
	return i, nil
}

// ListIdentities implements app.IdentityAdmin (single page, sorted by id).
func (f *Identities) ListIdentities(_ context.Context, q app.IdentityQuery) ([]identity.Identity, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return nil, "", f.Err
	}
	var out []identity.Identity
	for _, i := range f.M {
		if q.Email != "" && i.Email != q.Email {
			continue
		}
		out = append(out, i)
	}
	sort.Slice(out, func(a, b int) bool { return out[a].ID.String() < out[b].ID.String() })
	if q.PageSize <= 0 {
		return out, "", nil
	}
	start := 0
	if q.PageToken != "" {
		start, _ = strconv.Atoi(q.PageToken)
	}
	if start > len(out) {
		start = len(out)
	}
	end := start + q.PageSize
	next := ""
	if end < len(out) {
		next = strconv.Itoa(end)
	} else {
		end = len(out)
	}
	return out[start:end], next, nil
}

// CreateIdentity implements app.IdentityAdmin.
func (f *Identities) CreateIdentity(_ context.Context, in app.NewIdentity) (identity.Identity, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return identity.Identity{}, f.Err
	}
	for _, i := range f.M {
		if i.Email == in.Email {
			return identity.Identity{}, app.ErrConflict
		}
	}
	now := time.Now()
	if f.Clock != nil {
		now = f.Clock.Now()
	}
	i := identity.Identity{ID: uuid.New(), SchemaID: in.SchemaID, State: identity.StateActive, Email: in.Email, Name: in.Name, CreatedAt: now}
	f.M[i.ID] = i
	return i, nil
}

// DeleteIdentity implements app.IdentityAdmin.
func (f *Identities) DeleteIdentity(_ context.Context, id uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.M, id)
	f.Deleted = append(f.Deleted, id)
	return nil
}

// SetState implements app.IdentityAdmin.
func (f *Identities) SetState(_ context.Context, id uuid.UUID, s identity.State) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	i, ok := f.M[id]
	if !ok {
		return app.ErrNotFound
	}
	i.State = s
	f.M[id] = i
	return nil
}

// RevokeIdentitySessions implements app.IdentityAdmin.
func (f *Identities) RevokeIdentitySessions(_ context.Context, id uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.RevokedAll = append(f.RevokedAll, id)
	return nil
}

// RevokeSession implements app.IdentityAdmin.
func (f *Identities) RevokeSession(_ context.Context, id uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.RevokedSessions = append(f.RevokedSessions, id)
	return nil
}

// CreateRecoveryCode implements app.IdentityAdmin.
func (f *Identities) CreateRecoveryCode(_ context.Context, _ uuid.UUID, ttl time.Duration) (app.RecoveryCode, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.RecoveryErr != nil {
		return app.RecoveryCode{}, f.RecoveryErr
	}
	rc := f.Recovery
	if rc.Link == "" {
		rc = app.RecoveryCode{Link: "http://localhost:5173/recovery?flow=x", Code: "123456"}
	}
	if rc.ExpiresAt.IsZero() {
		now := time.Now()
		if f.Clock != nil {
			now = f.Clock.Now()
		}
		rc.ExpiresAt = now.Add(ttl)
	}
	return rc, nil
}

// Store is a fake of all repositories plus TxRunner.
type Store struct {
	mu       sync.Mutex
	Profiles map[uuid.UUID]profile.Profile
	Events   []audit.Event
	Idem     map[string]app.IdempotencyRecord
	Keys     map[uuid.UUID]app.SubjectKey   // by identity id
	PII      map[uuid.UUID]app.EncryptedPII // by identity id
	Clock    app.Clock
	nextID   int64

	// AuditErr fails every audit append; CommitErr fails every WithinTx at
	// commit time (after fn succeeded); TxErrOnce fails the next WithinTx.
	AuditErr  error
	CommitErr error
	TxErrOnce error

	txMu sync.Mutex // transactions run serially (models the advisory lock)
	// LockCalls counts XactLock calls.
	LockCalls int
}

// NewStore creates an empty fake store.
func NewStore(c app.Clock) *Store {
	return &Store{
		Profiles: map[uuid.UUID]profile.Profile{}, Idem: map[string]app.IdempotencyRecord{}, Clock: c,
		Keys: map[uuid.UUID]app.SubjectKey{}, PII: map[uuid.UUID]app.EncryptedPII{},
	}
}

func (s *Store) now() time.Time {
	if s.Clock != nil {
		return s.Clock.Now()
	}
	return time.Now()
}

// Repos returns the repositories.
func (s *Store) Repos() app.Repos {
	return app.Repos{
		Profiles: profileRepo{s}, Audit: auditRepo{s}, Idempotency: idemRepo{s}, Locks: noLock{},
		SubjectKeys: subjectKeyRepo{s}, PersonalInfo: piiRepo{s},
	}
}

// WithinTx implements app.TxRunner: changes are rolled back when fn or the
// (simulated) commit fails. Advisory locks taken inside are held until the
// transaction ends.
func (s *Store) WithinTx(ctx context.Context, fn func(context.Context, app.Repos) error) error {
	s.txMu.Lock()
	defer s.txMu.Unlock()
	s.mu.Lock()
	if err := s.TxErrOnce; err != nil {
		s.TxErrOnce = nil
		s.mu.Unlock()
		return err
	}
	snapProfiles := make(map[uuid.UUID]profile.Profile, len(s.Profiles))
	for k, v := range s.Profiles {
		snapProfiles[k] = v
	}
	snapIdem := make(map[string]app.IdempotencyRecord, len(s.Idem))
	for k, v := range s.Idem {
		snapIdem[k] = v
	}
	snapEvents := len(s.Events)
	snapKeys := make(map[uuid.UUID]app.SubjectKey, len(s.Keys))
	for k, v := range s.Keys {
		snapKeys[k] = v
	}
	snapPII := make(map[uuid.UUID]app.EncryptedPII, len(s.PII))
	for k, v := range s.PII {
		snapPII[k] = v
	}
	s.mu.Unlock()

	r := s.Repos()
	r.Locks = txLocker{s: s}
	err := fn(ctx, r)
	if err == nil && s.CommitErr != nil {
		err = s.CommitErr
	}
	if err != nil {
		s.mu.Lock()
		s.Profiles, s.Idem, s.Events = snapProfiles, snapIdem, s.Events[:snapEvents]
		s.Keys, s.PII = snapKeys, snapPII
		s.mu.Unlock()
	}
	return err
}

type txLocker struct{ s *Store }

func (l txLocker) XactLock(context.Context, int64) error {
	l.s.mu.Lock()
	defer l.s.mu.Unlock()
	l.s.LockCalls++
	return nil
}

type noLock struct{}

func (noLock) XactLock(context.Context, int64) error { return nil }

// AuditActions lists recorded actions.
func (s *Store) AuditActions() []audit.Action {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]audit.Action, len(s.Events))
	for i, e := range s.Events {
		out[i] = e.Action
	}
	return out
}

type profileRepo struct{ s *Store }

func (r profileRepo) Ensure(_ context.Context, id uuid.UUID, k identity.Kind) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if _, ok := r.s.Profiles[id]; !ok {
		now := r.s.now()
		r.s.Profiles[id] = profile.Profile{IdentityID: id, Kind: k, Locale: profile.DefaultLocale, CreatedAt: now, UpdatedAt: now}
	}
	return nil
}

func (r profileRepo) Get(_ context.Context, id uuid.UUID) (profile.Profile, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	p, ok := r.s.Profiles[id]
	if !ok {
		return profile.Profile{}, profile.ErrNotFound
	}
	return p, nil
}

func (r profileRepo) GetMany(_ context.Context, ids []uuid.UUID) (map[uuid.UUID]profile.Profile, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	out := map[uuid.UUID]profile.Profile{}
	for _, id := range ids {
		if p, ok := r.s.Profiles[id]; ok {
			out[id] = p
		}
	}
	return out, nil
}

func (r profileRepo) Update(_ context.Context, p profile.Profile) (profile.Profile, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if _, ok := r.s.Profiles[p.IdentityID]; !ok {
		return profile.Profile{}, profile.ErrNotFound
	}
	p.UpdatedAt = r.s.now()
	r.s.Profiles[p.IdentityID] = p
	return p, nil
}

type auditRepo struct{ s *Store }

func (r auditRepo) Append(_ context.Context, e audit.Event) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if r.s.AuditErr != nil {
		return r.s.AuditErr
	}
	r.s.nextID++
	e.ID = r.s.nextID
	e.OccurredAt = r.s.now()
	r.s.Events = append(r.s.Events, e)
	return nil
}

func (r auditRepo) List(_ context.Context, f audit.Filter) ([]audit.Event, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	var out []audit.Event
	for i := len(r.s.Events) - 1; i >= 0; i-- {
		e := r.s.Events[i]
		if f.TargetType != nil && e.TargetType != *f.TargetType {
			continue
		}
		if f.TargetID != nil && e.TargetID != *f.TargetID {
			continue
		}
		if f.ActorID != nil && e.ActorID != *f.ActorID {
			continue
		}
		if !f.Matches(e.Action) {
			continue
		}
		if f.After != nil && e.ID >= f.After.ID {
			continue
		}
		out = append(out, e)
		if f.Limit > 0 && len(out) == f.Limit {
			break
		}
	}
	return out, nil
}

type idemRepo struct{ s *Store }

func (r idemRepo) Reserve(_ context.Context, actor uuid.UUID, key string, hash []byte, notBefore time.Time) (*app.IdempotencyRecord, bool, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	k := actor.String() + "/" + key
	if rec, ok := r.s.Idem[k]; ok {
		if rec.CreatedAt.After(notBefore) {
			return &rec, false, nil
		}
		delete(r.s.Idem, k)
	}
	r.s.Idem[k] = app.IdempotencyRecord{RequestHash: hash, CreatedAt: r.s.now()}
	return nil, true, nil
}

func (r idemRepo) Complete(_ context.Context, actor uuid.UUID, key string, code int, body []byte) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	k := actor.String() + "/" + key
	rec, ok := r.s.Idem[k]
	if !ok || !rec.Pending() {
		return errors.New("not pending")
	}
	rec.ResponseCode, rec.ResponseBody = code, body
	r.s.Idem[k] = rec
	return nil
}

func (r idemRepo) Release(_ context.Context, actor uuid.UUID, key string) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	delete(r.s.Idem, actor.String()+"/"+key)
	return nil
}

func (r idemRepo) Purge(_ context.Context, before time.Time) (int64, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	var n int64
	for k, v := range r.s.Idem {
		if !v.CreatedAt.After(before) {
			delete(r.s.Idem, k)
			n++
		}
	}
	return n, nil
}

// Mailer records invitations.
type Mailer struct {
	mu   sync.Mutex
	Sent []app.Invitation
	Err  error
}

// SendInvitation implements app.Mailer.
func (m *Mailer) SendInvitation(_ context.Context, inv app.Invitation) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return m.Err
	}
	m.Sent = append(m.Sent, inv)
	return nil
}

// --- personal information ---

type subjectKeyRepo struct{ s *Store }

func (r subjectKeyRepo) Get(_ context.Context, id uuid.UUID) (app.SubjectKey, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	k, ok := r.s.Keys[id]
	if !ok {
		return app.SubjectKey{}, app.ErrNotFound
	}
	return k, nil
}

func (r subjectKeyRepo) Insert(_ context.Context, k app.SubjectKey) (app.SubjectKey, bool, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if cur, ok := r.s.Keys[k.IdentityID]; ok {
		return cur, false, nil
	}
	k.CreatedAt = r.s.now()
	r.s.Keys[k.IdentityID] = k
	return k, true, nil
}

// Delete cascades to the encrypted record like the composite FK.
func (r subjectKeyRepo) Delete(_ context.Context, id uuid.UUID) ([]uuid.UUID, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	k, ok := r.s.Keys[id]
	if !ok {
		return nil, nil
	}
	delete(r.s.Keys, id)
	delete(r.s.PII, id)
	return []uuid.UUID{k.KeyID}, nil
}

func (r subjectKeyRepo) ListForRewrap(_ context.Context, after uuid.UUID, limit int) ([]app.SubjectKey, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	var out []app.SubjectKey
	for _, k := range r.s.Keys {
		if k.KeyID.String() > after.String() {
			out = append(out, k)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].KeyID.String() < out[j].KeyID.String() })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r subjectKeyRepo) UpdateWrapped(_ context.Context, keyID uuid.UUID, old string, w app.Wrapped) (bool, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	for id, k := range r.s.Keys {
		if k.KeyID == keyID && k.Wrapped.Ciphertext == old {
			now := r.s.now()
			k.Wrapped, k.RewrappedAt = w, &now
			r.s.Keys[id] = k
			return true, nil
		}
	}
	return false, nil
}

func (r subjectKeyRepo) DeleteErased(_ context.Context) (int64, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	erasedAt := map[string]time.Time{}
	for _, e := range r.s.Events {
		if e.Action == audit.ActionCustomerPIIErased && e.TargetType == audit.TargetCustomer && e.OccurredAt.After(erasedAt[e.TargetID]) {
			erasedAt[e.TargetID] = e.OccurredAt
		}
	}
	var n int64
	for id, k := range r.s.Keys {
		if t, ok := erasedAt[id.String()]; ok && !k.CreatedAt.After(t) {
			delete(r.s.Keys, id)
			delete(r.s.PII, id)
			n++
		}
	}
	return n, nil
}

type piiRepo struct{ s *Store }

func (r piiRepo) Get(_ context.Context, id uuid.UUID) (app.EncryptedPII, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	rec, ok := r.s.PII[id]
	if !ok {
		return app.EncryptedPII{}, app.ErrNotFound
	}
	return rec, nil
}

// Upsert enforces the composite FK (identity_id, key_id) → subject_key.
func (r piiRepo) Upsert(_ context.Context, rec app.EncryptedPII) (time.Time, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if k, ok := r.s.Keys[rec.IdentityID]; !ok || k.KeyID != rec.KeyID {
		return time.Time{}, fmt.Errorf("%w: customer_pii_subject_key_fk", app.ErrConflict)
	}
	rec.UpdatedAt = r.s.now()
	r.s.PII[rec.IdentityID] = rec
	return rec.UpdatedAt, nil
}

func (r piiRepo) FindByPhoneBidx(_ context.Context, b app.BlindIndex, limit int) ([]app.EncryptedPII, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	var out []app.EncryptedPII
	for _, rec := range r.s.PII {
		if rec.PhoneBidx != nil && rec.PhoneBidx.KeyVersion == b.KeyVersion && string(rec.PhoneBidx.Sum) == string(b.Sum) {
			out = append(out, rec)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].IdentityID.String() < out[j].IdentityID.String() })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
