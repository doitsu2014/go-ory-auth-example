package testutil

import (
	"bytes"
	"context"
	"encoding/json"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/identity"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/login"
)

// --- login identifiers (ADR-0013) ---

type loginRepo struct{ s *Store }

func (r loginRepo) InsertIfAbsent(_ context.Context, rec app.LoginRecord) (bool, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if _, ok := r.s.Logins[rec.Pseudonym]; ok {
		return false, nil
	}
	for _, other := range r.s.Logins {
		if other.LookupKey == rec.LookupKey {
			return false, nil
		}
	}
	now := r.s.now()
	rec.CreatedAt, rec.LastValidatedAt = now, now
	if rec.IdentityID != nil {
		rec.BoundAt = &now
	}
	r.s.Logins[rec.Pseudonym] = rec
	return true, nil
}

func (r loginRepo) Get(_ context.Context, p login.Pseudonym) (app.LoginRecord, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	rec, ok := r.s.Logins[p]
	if !ok {
		return app.LoginRecord{}, app.ErrNotFound
	}
	return rec, nil
}

func (r loginRepo) GetByLookupKey(_ context.Context, k login.LookupKey) (app.LoginRecord, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if r.s.LoginErr != nil {
		return app.LoginRecord{}, r.s.LoginErr
	}
	for _, rec := range r.s.Logins {
		if rec.LookupKey == k {
			return rec, nil
		}
	}
	return app.LoginRecord{}, app.ErrNotFound
}

func (r loginRepo) GetMany(_ context.Context, ps []login.Pseudonym) (map[login.Pseudonym]app.LoginRecord, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if r.s.LoginErr != nil {
		return nil, r.s.LoginErr
	}
	out := map[login.Pseudonym]app.LoginRecord{}
	for _, p := range ps {
		if rec, ok := r.s.Logins[p]; ok {
			out[p] = rec
		}
	}
	return out, nil
}

func (r loginRepo) GetByIdentity(_ context.Context, id uuid.UUID) (app.LoginRecord, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	for _, rec := range r.s.Logins {
		if rec.IdentityID != nil && *rec.IdentityID == id {
			return rec, nil
		}
	}
	return app.LoginRecord{}, app.ErrNotFound
}

func (r loginRepo) Bind(_ context.Context, p login.Pseudonym, id uuid.UUID, stale *uuid.UUID) (bool, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	rec, ok := r.s.Logins[p]
	if !ok {
		return false, nil
	}
	if rec.IdentityID != nil && *rec.IdentityID != id && (stale == nil || *stale != *rec.IdentityID) {
		return false, nil
	}
	for q, other := range r.s.Logins {
		if q != p && other.IdentityID != nil && *other.IdentityID == id {
			return false, app.ErrConflict // unique (identity_id)
		}
	}
	if rec.IdentityID == nil || *rec.IdentityID != id {
		now := r.s.now()
		rec.IdentityID, rec.BoundAt = &id, &now
	}
	r.s.Logins[p] = rec
	return true, nil
}

func (r loginRepo) SetLegacyVerified(_ context.Context, p login.Pseudonym, v bool) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	rec, ok := r.s.Logins[p]
	if !ok {
		return app.ErrNotFound
	}
	rec.LegacyVerified = v
	r.s.Logins[p] = rec
	return nil
}

func (r loginRepo) Touch(_ context.Context, p login.Pseudonym) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	rec, ok := r.s.Logins[p]
	if !ok {
		return app.ErrNotFound
	}
	rec.LastValidatedAt = r.s.now()
	r.s.Logins[p] = rec
	return nil
}

func (r loginRepo) sorted(keep func(app.LoginRecord) bool) []app.LoginRecord {
	var out []app.LoginRecord
	for _, rec := range r.s.Logins {
		if keep(rec) {
			out = append(out, rec)
		}
	}
	sort.Slice(out, func(a, b int) bool { return bytes.Compare(out[a].Pseudonym[:], out[b].Pseudonym[:]) < 0 })
	return out
}

func limitTo(recs []app.LoginRecord, n int) []app.LoginRecord {
	if len(recs) > n {
		return recs[:n]
	}
	return recs
}

func (r loginRepo) ListStaleUnbound(_ context.Context, before time.Time, limit int) ([]app.LoginRecord, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	return limitTo(r.sorted(func(rec app.LoginRecord) bool {
		return rec.IdentityID == nil && rec.LastValidatedAt.Before(before)
	}), limit), nil
}

func (r loginRepo) DeleteStaleUnbound(_ context.Context, p login.Pseudonym, before time.Time) (bool, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	rec, ok := r.s.Logins[p]
	if !ok || rec.IdentityID != nil || !rec.LastValidatedAt.Before(before) {
		return false, nil
	}
	delete(r.s.Logins, p)
	return true, nil
}

func (r loginRepo) ListBound(_ context.Context, after login.Pseudonym, limit int) ([]app.LoginRecord, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	return limitTo(r.sorted(func(rec app.LoginRecord) bool {
		return rec.IdentityID != nil && bytes.Compare(rec.Pseudonym[:], after[:]) > 0
	}), limit), nil
}

func (r loginRepo) ListAll(_ context.Context, after login.Pseudonym, limit int) ([]app.LoginRecord, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	return limitTo(r.sorted(func(rec app.LoginRecord) bool {
		return bytes.Compare(rec.Pseudonym[:], after[:]) > 0
	}), limit), nil
}

func (r loginRepo) Delete(_ context.Context, p login.Pseudonym, onlyIfUnbound bool) (bool, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	rec, ok := r.s.Logins[p]
	if !ok || (onlyIfUnbound && rec.IdentityID != nil) {
		return false, nil
	}
	delete(r.s.Logins, p)
	return true, nil
}

func (r loginRepo) DeleteForIdentity(_ context.Context, id uuid.UUID) (bool, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	for p, rec := range r.s.Logins {
		if rec.IdentityID != nil && *rec.IdentityID == id {
			delete(r.s.Logins, p)
			return true, nil
		}
	}
	return false, nil
}

func (r loginRepo) UpdateCiphertext(_ context.Context, p login.Pseudonym, old, ct string, v int) (bool, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	rec, ok := r.s.Logins[p]
	if !ok || rec.Ciphertext != old {
		return false, nil
	}
	rec.Ciphertext, rec.KEKVersion = ct, v
	r.s.Logins[p] = rec
	return true, nil
}

func (r loginRepo) CountUnbound(context.Context) (int64, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	var n int64
	for _, rec := range r.s.Logins {
		if rec.IdentityID == nil {
			n++
		}
	}
	return n, nil
}

func (r loginRepo) DeleteErased(context.Context) (int64, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	erased := map[string]bool{}
	for _, e := range r.s.Events {
		if e.Action == "customer.login.erased" {
			erased[e.TargetID] = true
		}
	}
	var n int64
	for p, rec := range r.s.Logins {
		if rec.IdentityID != nil && erased[rec.IdentityID.String()] {
			delete(r.s.Logins, p)
			n++
		}
	}
	return n, nil
}

type fakeDispatch struct {
	sent     bool
	updated  time.Time
	delivery *app.Delivery
}

type dispatchRepo struct{ s *Store }

func (r dispatchRepo) Reserve(_ context.Context, key [32]byte, staleBefore time.Time) (bool, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	d, ok := r.s.Dispatch[key]
	if ok && (d.sent || !d.updated.Before(staleBefore)) {
		return false, nil
	}
	r.s.Dispatch[key] = fakeDispatch{updated: r.s.now()}
	return true, nil
}

func (r dispatchRepo) MarkSent(_ context.Context, key [32]byte, d *app.Delivery) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	r.s.Dispatch[key] = fakeDispatch{sent: true, updated: r.s.now(), delivery: d}
	return nil
}

func (r dispatchRepo) CountDeliveriesTo(_ context.Context, rk [32]byte, since time.Time) (int64, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	var n int64
	for _, d := range r.s.Dispatch {
		if d.delivery != nil && d.delivery.RecipientKey == rk && !d.updated.Before(since) {
			n++
		}
	}
	return n, nil
}

func (r dispatchRepo) CountSMS(_ context.Context, country string, since time.Time) (int64, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	var n int64
	for _, d := range r.s.Dispatch {
		if d.delivery != nil && d.delivery.Channel == "sms" && !d.updated.Before(since) && (country == "" || d.delivery.Country == country) {
			n++
		}
	}
	return n, nil
}

func (r dispatchRepo) Release(_ context.Context, key [32]byte) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if d, ok := r.s.Dispatch[key]; ok && !d.sent {
		delete(r.s.Dispatch, key)
	}
	return nil
}

func (r dispatchRepo) Purge(_ context.Context, before time.Time) (int64, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	var n int64
	for k, d := range r.s.Dispatch {
		if d.updated.Before(before) {
			delete(r.s.Dispatch, k)
			n++
		}
	}
	return n, nil
}

// SMS records text messages.
type SMS struct {
	mu   sync.Mutex
	Sent []LoginMessage // To = E.164, Message.SMS = text
	Err  error
}

// SendSMS implements app.SMSSender.
func (s *SMS) SendSMS(_ context.Context, to, text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Err != nil {
		return s.Err
	}
	s.Sent = append(s.Sent, LoginMessage{To: to, Message: login.Message{SMS: text}})
	return nil
}

// ReplaceLoginTrait implements app.LoginTraitAdmin (compare + replace; the
// rewrite resets verification like Kratos, spike S4b).
func (f *Identities) ReplaceLoginTrait(_ context.Context, id uuid.UUID, oldEmail, loginID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ReplaceLoginErr != nil {
		return f.ReplaceLoginErr
	}
	i, ok := f.M[id]
	if !ok {
		return app.ErrNotFound
	}
	if i.Email != oldEmail || i.LoginID != oldEmail {
		return app.ErrConflict
	}
	i.Email, i.LoginID, i.EmailVerified = "", loginID, false
	f.M[id] = i
	return nil
}

// MarkLoginVerified implements app.LoginTraitAdmin.
func (f *Identities) MarkLoginVerified(_ context.Context, id uuid.UUID, loginID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.MarkVerifiedErr != nil {
		return f.MarkVerifiedErr
	}
	i, ok := f.M[id]
	if !ok || i.LoginID != loginID {
		return app.ErrNotFound
	}
	i.EmailVerified = true
	f.M[id] = i
	return nil
}

// AddCustomer stores a customer identity with the given login identifier.
func (f *Identities) AddCustomer(loginID string, verified bool) identity.Identity {
	i := identity.Identity{SchemaID: string(identity.KindCustomer), LoginID: loginID, EmailVerified: verified}
	if !login.IsPseudonym(loginID) {
		i.Email = loginID
	}
	return f.Add(i)
}

// AuthFlowCall is one AuthFlows call.
type AuthFlowCall struct {
	Op         string // login | register | recovery
	Identifier string
	Client     app.FlowClient
}

// AuthFlows is an in-memory Kratos for the customer auth flows: accounts are
// identifier → password.
type AuthFlows struct {
	mu        sync.Mutex
	Passwords map[string]string
	Calls     []AuthFlowCall
	// Err fails every call (e.g. app.ErrDependencyUnavailable); RegisterErr
	// fails registrations (e.g. a password policy *app.AuthFlowError).
	Err, RegisterErr error
}

// NewAuthFlows creates an empty fake.
func NewAuthFlows() *AuthFlows { return &AuthFlows{Passwords: map[string]string{}} }

func (f *AuthFlows) record(op, identifier string, c app.FlowClient) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Calls = append(f.Calls, AuthFlowCall{Op: op, Identifier: identifier, Client: c})
	return f.Err
}

// CallsOf returns the calls of one operation.
func (f *AuthFlows) CallsOf(op string) []AuthFlowCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []AuthFlowCall
	for _, c := range f.Calls {
		if c.Op == op {
			out = append(out, c)
		}
	}
	return out
}

func fakeSession(identifier string) app.AuthSession {
	b, _ := json.Marshal(map[string]any{"id": "sess", "identity": map[string]any{"traits": map[string]any{"login_id": identifier}}})
	return app.AuthSession{Token: "token-" + identifier, Session: b}
}

// Login implements app.AuthFlows.
func (f *AuthFlows) Login(_ context.Context, c app.FlowClient, identifier, password string) (app.AuthSession, error) {
	if err := f.record("login", identifier, c); err != nil {
		return app.AuthSession{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if pw, ok := f.Passwords[identifier]; !ok || pw != password {
		return app.AuthSession{}, &app.AuthFlowError{Messages: []app.FlowMessage{{Field: app.FlowFieldForm, ID: app.KratosInvalidCredentials}}}
	}
	return fakeSession(identifier), nil
}

// Register implements app.AuthFlows.
func (f *AuthFlows) Register(_ context.Context, c app.FlowClient, loginID, password string) (app.AuthSession, error) {
	if err := f.record("register", loginID, c); err != nil {
		return app.AuthSession{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.RegisterErr != nil {
		return app.AuthSession{}, f.RegisterErr
	}
	if _, ok := f.Passwords[loginID]; ok {
		return app.AuthSession{}, &app.AuthFlowError{Messages: []app.FlowMessage{{Field: app.FlowFieldLogin, ID: 4000007}}}
	}
	f.Passwords[loginID] = password
	s := fakeSession(loginID)
	s.VerificationFlowID = "verification-flow"
	return s, nil
}

// StartRecovery implements app.AuthFlows.
func (f *AuthFlows) StartRecovery(_ context.Context, c app.FlowClient, email string) (string, error) {
	if err := f.record("recovery", email, c); err != nil {
		return "", err
	}
	return RecoveryFlowID, nil
}

// RecoveryFlowID is the flow id every fake recovery returns.
const RecoveryFlowID = "6f1c2b8e-4d3a-4b6f-9e2d-0c1a2b3c4d5e"

// RecoveryCode is the only code the fake accepts.
const RecoveryCode = "123456"

// SubmitRecoveryCode implements app.AuthFlows.
func (f *AuthFlows) SubmitRecoveryCode(_ context.Context, c app.FlowClient, flowID, code string) (app.RecoveryGrant, error) {
	if err := f.record("recovery-code", flowID, c); err != nil {
		return app.RecoveryGrant{}, err
	}
	switch {
	case flowID != RecoveryFlowID:
		return app.RecoveryGrant{}, app.ErrAuthFlowExpired
	case code != RecoveryCode:
		return app.RecoveryGrant{}, &app.AuthFlowError{Messages: []app.FlowMessage{{Field: app.FlowFieldForm, ID: 4060006}}}
	}
	return app.RecoveryGrant{SessionToken: "token-recovery", SettingsFlowID: "settings-flow"}, nil
}
