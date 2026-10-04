package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/netip"
	"time"

	"github.com/google/uuid"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/audit"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/identity"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/login"
)

// MigrationPhase is the rollout phase of pseudonymous logins (A2, DD-15).
type MigrationPhase string

// Phases. During the transition, legacy customers (plaintext email trait)
// still exist and are honoured; once complete, only pseudonyms are.
const (
	PhaseTransition MigrationPhase = "transition"
	PhaseComplete   MigrationPhase = "complete"
)

// ParseMigrationPhase validates a phase.
func ParseMigrationPhase(s string) (MigrationPhase, bool) {
	switch MigrationPhase(s) {
	case PhaseTransition, PhaseComplete:
		return MigrationPhase(s), true
	}
	return "", false
}

// Pre-registration outcomes (api-contract §8). Each maps to a Kratos flow
// message on the login_id node.
var (
	// ErrLoginLegacyTraits: a registration with the legacy email trait (old
	// app build or direct API call).
	ErrLoginLegacyTraits = errors.New("login: legacy traits")
	// ErrLoginUnresolved: the login_id is not a handle identity-service
	// stored (no vault entry), e.g. a registration sent to Kratos directly.
	ErrLoginUnresolved = errors.New("login: unresolved identifier")
	// ErrLoginDuplicate: a legacy customer already signs in with the same
	// address (migration window).
	ErrLoginDuplicate = errors.New("login: duplicate identifier")
)

// Rate limits of the public customer auth routes (ADR-0014): per client IP
// for sign-in/recovery and for registration, per network, failed sign-ins
// per account, and the courier per recipient.
var (
	SignInRateRules      = []RateRule{{Limit: 20, Window: time.Minute}, {Limit: 200, Window: 24 * time.Hour}}
	RegisterRateRules    = []RateRule{{Limit: 5, Window: time.Minute}, {Limit: 30, Window: 24 * time.Hour}}
	NetRateRules         = []RateRule{{Limit: 300, Window: time.Minute}, {Limit: 5000, Window: 24 * time.Hour}}
	AccountRateRules     = []RateRule{{Limit: 10, Window: 15 * time.Minute}, {Limit: 50, Window: 24 * time.Hour}}
	InsertGlobalRules    = []RateRule{{Limit: 120, Window: time.Minute}}
	CourierRecipientRule = []RateRule{{Limit: 5, Window: time.Hour}, {Limit: 20, Window: 24 * time.Hour}}
)

// IPBucket is the rate-limit key of a client address: the address for IPv4,
// the /64 network for IPv6 (A5).
func IPBucket(ip netip.Addr) string {
	ip = ip.Unmap()
	if ip.Is6() {
		p, _ := ip.Prefix(64)
		return p.String()
	}
	return ip.String()
}

// NetBucket is the aggregate rate-limit key: the /24 network for IPv4, the
// /48 for IPv6 (SEC-C03), so rotating addresses inside one network does not
// multiply the budget.
func NetBucket(ip netip.Addr) string {
	ip = ip.Unmap()
	bits := 24
	if ip.Is6() {
		bits = 48
	}
	p, _ := ip.Prefix(bits)
	return p.String()
}

// MaskedLogin is the admin view of a login identifier.
type MaskedLogin struct {
	Kind   login.Kind
	Masked string
}

// LoginIdentifierService implements the pseudonymous login identifier use
// cases (ADR-0013, ADR-0014): finding and claiming the vault entry of an
// address, pre-registration check, binding, owner and admin views, lookup,
// purge and re-wrap. Plaintext addresses exist only in memory for one
// request; no method logs them.
type LoginIdentifierService struct {
	Keys       LoginKeys
	Logins     LoginIdentifierRepo
	Tx         TxRunner
	Identities IdentityAdmin
	Phone      login.PhonePolicy
	// PhoneEnabled is false when no SMS channel is configured; phone
	// identifiers are then rejected (type unsupported).
	PhoneEnabled bool
	Phase        MigrationPhase
	Clock        Clock
	Log          *slog.Logger
	// InsertLimiter (global, key "", nil = unlimited) counts new vault rows
	// and only alerts: refusing every registration when it trips would let a
	// few networks block all sign-ups (SEC-C03).
	InsertLimiter *KeyedLimiter[string]
}

// parse validates the type and normalises what the customer typed. Field
// errors are reported under prefix ("login.type", "login.value").
func (s *LoginIdentifierService) parse(typ, value, prefix string) (login.Identifier, error) {
	kind, ok := login.ParseKind(typ)
	switch {
	case !ok:
		return login.Identifier{}, NewValidationError(prefix+"type", "invalid")
	case kind == login.KindPhone && !s.PhoneEnabled:
		return login.Identifier{}, NewValidationError(prefix+"type", "unsupported")
	}
	id, err := login.Parse(kind, value, s.Phone)
	if err != nil {
		code, _ := login.IsInvalid(err)
		return login.Identifier{}, NewValidationError(prefix+"value", code)
	}
	return id, nil
}

func (s *LoginIdentifierService) lookupKey(ctx context.Context, id login.Identifier) (login.LookupKey, error) {
	input := login.LookupInput(id)
	defer clear(input)
	k, err := s.Keys.LookupKey(ctx, input)
	if err != nil {
		return login.LookupKey{}, fmt.Errorf("login lookup key: %w", err)
	}
	return k, nil
}

// find returns the vault entry of an address (ErrNotFound when there is
// none) and its lookup key.
func (s *LoginIdentifierService) find(ctx context.Context, id login.Identifier) (LoginRecord, login.LookupKey, error) {
	k, err := s.lookupKey(ctx, id)
	if err != nil {
		return LoginRecord{}, k, err
	}
	rec, err := s.Logins.GetByLookupKey(ctx, k)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return LoginRecord{}, k, fmt.Errorf("get login: %w", err)
	}
	return rec, k, err
}

// claim returns the handle of an address, storing the sealed address under a
// new random handle when it has none (registration, migration). A row that
// exists (an earlier, unfinished registration, or a concurrent one that won
// the insert) is re-used, so one address always has one handle.
func (s *LoginIdentifierService) claim(ctx context.Context, id login.Identifier, bindTo *uuid.UUID, verified bool) (login.Pseudonym, error) {
	rec, k, err := s.find(ctx, id)
	if err == nil {
		return rec.Pseudonym, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return login.Pseudonym{}, err
	}
	p, err := login.NewPseudonym()
	if err != nil {
		return login.Pseudonym{}, err
	}
	inserted, err := s.store(ctx, id, k, p, bindTo, verified)
	if err != nil {
		return login.Pseudonym{}, err
	}
	if inserted {
		if !s.InsertLimiter.Allow("") {
			s.log().ErrorContext(ctx, "login_insert_rate_high") // alert (A8); never blocks sign-ups
		}
		return p, nil
	}
	rec, err = s.Logins.GetByLookupKey(ctx, k)
	if err != nil {
		return login.Pseudonym{}, fmt.Errorf("get claimed login: %w", err)
	}
	return rec.Pseudonym, nil
}

// store seals the identifier under its handle and inserts it if absent.
func (s *LoginIdentifierService) store(ctx context.Context, id login.Identifier, k login.LookupKey, p login.Pseudonym, bindTo *uuid.UUID, verified bool) (bool, error) {
	pt := []byte(id.Value())
	ct, v, err := s.Keys.SealLogin(ctx, login.AAD(id.Kind(), p), pt)
	clear(pt)
	if err != nil {
		return false, fmt.Errorf("seal login: %w", err)
	}
	rec := LoginRecord{Pseudonym: p, LookupKey: k, Kind: id.Kind(), Ciphertext: ct, KEKVersion: v, IdentityID: bindTo, LegacyVerified: verified}
	inserted, err := s.Logins.InsertIfAbsent(ctx, rec)
	if err != nil {
		return false, fmt.Errorf("store login: %w", err)
	}
	return inserted, nil
}

// RegistrationTraits are the traits of a registration under validation.
type RegistrationTraits struct {
	LoginID     string
	LegacyEmail string
}

// ValidateRegistration is the pre-registration check (PLI-FR-04, A2): the
// login_id must be a pseudonym with a vault entry and, while legacy
// customers exist, must not collide with a legacy customer's email.
func (s *LoginIdentifierService) ValidateRegistration(ctx context.Context, t RegistrationTraits) error {
	if t.LegacyEmail != "" {
		return ErrLoginLegacyTraits
	}
	p, ok := login.ParsePseudonym(t.LoginID)
	if !ok {
		return ErrLoginUnresolved
	}
	rec, err := s.Logins.Get(ctx, p)
	if errors.Is(err, ErrNotFound) {
		return ErrLoginUnresolved
	}
	if err != nil {
		return fmt.Errorf("get login: %w", err)
	}
	if s.Phase != PhaseComplete && rec.Kind == login.KindEmail {
		id, err := s.open(ctx, rec)
		if err != nil {
			return err
		}
		legacy, _, err := s.Identities.ListIdentities(ctx, IdentityQuery{Email: id.Value()})
		if err != nil {
			return fmt.Errorf("find legacy identity: %w", err)
		}
		for _, it := range legacy {
			if it.IsCustomer() {
				return ErrLoginDuplicate
			}
		}
	}
	// A purge that deleted the row after Get makes Touch fail: the
	// registration is rejected rather than left without a vault entry (A10).
	if err := s.Logins.Touch(ctx, p); errors.Is(err, ErrNotFound) {
		return ErrLoginUnresolved
	} else if err != nil {
		return fmt.Errorf("touch login: %w", err)
	}
	return nil
}

// Bind binds the vault entry of loginID to the identity (after-registration
// webhook and lazy binding, A1/A6). The caller passes the login_id read from
// Kratos (webhook context or whoami), so the pairing is authoritative. A
// legacy (non-pseudonym) login is a no-op.
func (s *LoginIdentifierService) Bind(ctx context.Context, id uuid.UUID, loginID string) error {
	p, ok := login.ParsePseudonym(loginID)
	if !ok {
		return nil
	}
	_, err := s.bind(ctx, id, p)
	return err
}

// bind returns the bound record.
func (s *LoginIdentifierService) bind(ctx context.Context, id uuid.UUID, p login.Pseudonym) (LoginRecord, error) {
	bound, err := s.Logins.Bind(ctx, p, id, nil)
	if errors.Is(err, ErrConflict) {
		s.log().ErrorContext(ctx, "login_identity_bound_to_other_login", "identity_id", id.String())
		return LoginRecord{}, fmt.Errorf("bind login: %w", ErrDataIntegrity)
	}
	if err != nil {
		return LoginRecord{}, fmt.Errorf("bind login: %w", err)
	}
	rec, err := s.Logins.Get(ctx, p)
	if errors.Is(err, ErrNotFound) {
		s.log().ErrorContext(ctx, "login_identifier_missing", "identity_id", id.String())
		return LoginRecord{}, fmt.Errorf("bind login: %w", ErrDataIntegrity)
	}
	if err != nil {
		return LoginRecord{}, fmt.Errorf("get login: %w", err)
	}
	if bound {
		return rec, nil
	}
	if rec.IdentityID == nil {
		// Inserted unbound between Bind and Get (concurrent registration): retry once.
		if ok, err := s.Logins.Bind(ctx, p, id, nil); err != nil || !ok {
			return LoginRecord{}, fmt.Errorf("bind login: %w", ErrDataIntegrity)
		}
		return s.Logins.Get(ctx, p)
	}
	// Bound to another identity: re-bind only when that identity is gone
	// (deleted directly in Kratos, A6).
	other := *rec.IdentityID
	if _, err := s.Identities.GetIdentity(ctx, other); !errors.Is(err, ErrNotFound) {
		if err != nil {
			return LoginRecord{}, fmt.Errorf("get bound identity: %w", err)
		}
		s.log().ErrorContext(ctx, "login_bound_elsewhere", "identity_id", id.String(), "bound_identity_id", other.String())
		return LoginRecord{}, fmt.Errorf("bind login: %w", ErrDataIntegrity)
	}
	if ok, err := s.Logins.Bind(ctx, p, id, &other); err != nil || !ok {
		s.log().ErrorContext(ctx, "login_rebind_failed", "identity_id", id.String())
		return LoginRecord{}, fmt.Errorf("rebind login: %w", ErrDataIntegrity)
	}
	s.log().InfoContext(ctx, "login_rebound_from_deleted_identity", "identity_id", id.String())
	rec.IdentityID, rec.BoundAt = &id, ptrTime(s.now())
	return rec, nil
}

// Own returns the caller's login identifier (GET /v1/me, PLI-FR-07),
// binding the vault entry lazily when the webhook did not (PLI-FR-16).
func (s *LoginIdentifierService) Own(ctx context.Context, p identity.Principal) (login.Identifier, error) {
	if p.Kind != identity.KindCustomer {
		return login.Identifier{}, ErrForbidden
	}
	ps, ok := login.ParsePseudonym(p.LoginID)
	if !ok {
		return legacyIdentifier(p.LoginID)
	}
	rec, err := s.Logins.Get(ctx, ps)
	if errors.Is(err, ErrNotFound) {
		s.log().ErrorContext(ctx, "login_identifier_missing", "identity_id", p.IdentityID.String())
		return login.Identifier{}, fmt.Errorf("own login: %w", ErrDataIntegrity)
	}
	if err != nil {
		return login.Identifier{}, fmt.Errorf("get login: %w", err)
	}
	if rec.IdentityID == nil || *rec.IdentityID != p.IdentityID {
		if rec, err = s.bind(ctx, p.IdentityID, ps); err != nil {
			return login.Identifier{}, err
		}
	}
	return s.open(ctx, rec)
}

// Reveal returns a customer's full login identifier for an audited admin
// reveal (PLI-FR-12). The caller checks permission and audits.
func (s *LoginIdentifierService) Reveal(ctx context.Context, it identity.Identity) (login.Identifier, error) {
	ps, ok := login.ParsePseudonym(it.LoginID)
	if !ok {
		return legacyIdentifier(it.LoginID)
	}
	rec, err := s.Logins.Get(ctx, ps)
	if err != nil {
		return login.Identifier{}, fmt.Errorf("get login: %w", err)
	}
	return s.open(ctx, rec)
}

// MaskMany returns the masked logins of the identities in one key-manager
// round trip (PLI-FR-10). Identities whose login cannot be read are missing
// from the map; unavailable reports a key-manager or database failure (the
// list stays usable).
func (s *LoginIdentifierService) MaskMany(ctx context.Context, its []identity.Identity) (out map[uuid.UUID]MaskedLogin, unavailable bool) {
	out = map[uuid.UUID]MaskedLogin{}
	var ps []login.Pseudonym
	owner := map[login.Pseudonym]uuid.UUID{}
	for _, it := range its {
		if p, ok := login.ParsePseudonym(it.LoginID); ok {
			ps = append(ps, p)
			owner[p] = it.ID
			continue
		}
		if id, err := legacyIdentifier(it.LoginID); err == nil {
			out[it.ID] = MaskedLogin{Kind: id.Kind(), Masked: id.Mask()}
		}
	}
	if len(ps) == 0 {
		return out, false
	}
	recs, err := s.Logins.GetMany(ctx, ps)
	if err != nil {
		s.log().WarnContext(ctx, "login_mask_unavailable", "error", err.Error())
		return out, true
	}
	items := make([]SealedLogin, 0, len(recs))
	order := make([]LoginRecord, 0, len(recs))
	for _, p := range ps {
		if rec, ok := recs[p]; ok {
			items = append(items, SealedLogin{AD: login.AAD(rec.Kind, rec.Pseudonym), Ciphertext: rec.Ciphertext})
			order = append(order, rec)
		}
	}
	if len(items) == 0 {
		return out, false
	}
	pts, errs, err := s.Keys.OpenLogins(ctx, items)
	if err != nil {
		s.log().WarnContext(ctx, "login_mask_unavailable", "error", err.Error())
		return out, true
	}
	for i, rec := range order {
		if errs[i] != nil {
			s.log().ErrorContext(ctx, "login_decrypt_failed", "identity_id", owner[rec.Pseudonym].String())
			continue
		}
		id, err := login.FromStored(rec.Kind, string(pts[i]))
		clear(pts[i])
		if err != nil {
			s.log().ErrorContext(ctx, "login_decrypt_failed", "identity_id", owner[rec.Pseudonym].String())
			continue
		}
		out[owner[rec.Pseudonym]] = MaskedLogin{Kind: id.Kind(), Masked: id.Mask()}
	}
	return out, false
}

// FindCustomers returns the customers whose login is the typed identifier
// (admin lookup, PLI-FR-11, PLX-FR-09): the vault entry of the address gives
// the Kratos handle. During the transition, legacy customers with the
// plaintext email are found too.
func (s *LoginIdentifierService) FindCustomers(ctx context.Context, kindStr, raw string) ([]identity.Identity, error) {
	kind, ok := login.ParseKind(kindStr)
	if !ok {
		return nil, NewValidationError("login.type", "invalid")
	}
	policy := s.Phone
	policy.AnyCountry = true
	id, err := login.Parse(kind, raw, policy)
	if err != nil {
		code, _ := login.IsInvalid(err)
		return nil, NewValidationError("login.value", code)
	}
	var queries []string
	rec, _, err := s.find(ctx, id)
	switch {
	case err == nil:
		queries = append(queries, rec.Pseudonym.String())
	case !errors.Is(err, ErrNotFound):
		return nil, err
	}
	if s.Phase != PhaseComplete && kind == login.KindEmail {
		queries = append(queries, id.Value())
	}
	seen := map[uuid.UUID]bool{}
	var out []identity.Identity
	for _, q := range queries {
		items, _, err := s.Identities.ListIdentities(ctx, IdentityQuery{Email: q})
		if err != nil {
			return nil, fmt.Errorf("find identities: %w", err)
		}
		for _, it := range items {
			if it.IsCustomer() && !seen[it.ID] {
				seen[it.ID] = true
				out = append(out, it)
			}
		}
	}
	return out, nil
}

// PurgeResult counts purge outcomes.
type PurgeResult struct {
	UnboundDeleted int
	Rebound        int
	OrphanDeleted  int
	// Conflicts are unbound rows whose Kratos owner is already bound to
	// another row; they are logged and left for an operator.
	Conflicts int
}

// MinPurgeAge is the smallest allowed purge age: a vault row stored for a registration
// must survive until the registration is submitted (A10).
const MinPurgeAge = 24 * time.Hour

// Purge removes unbound entries not validated for olderThan whose
// pseudonym has no Kratos identity (binding those that do), and entries
// bound to identities that no longer exist (erasure for out-of-band
// deletions, A6). Each orphan deletion is audited customer.login.erased.
// A failure on one row is counted and logged; the purge continues.
func (s *LoginIdentifierService) Purge(ctx context.Context, olderThan time.Duration, dryRun bool) (PurgeResult, error) {
	var res PurgeResult
	if olderThan < MinPurgeAge {
		return res, fmt.Errorf("purge age must be at least %s", MinPurgeAge)
	}
	before := s.now().Add(-olderThan)
	const page = 100
	// Unbound rows. Rows that stay (owner found, dry run, conflict) are
	// skipped by remembering the last validation time seen.
	seen := map[login.Pseudonym]bool{}
	for {
		recs, err := s.Logins.ListStaleUnbound(ctx, before, page+len(seen))
		if err != nil {
			return res, err
		}
		fresh := 0
		for _, rec := range recs {
			if seen[rec.Pseudonym] {
				continue
			}
			seen[rec.Pseudonym] = true
			fresh++
			if err := s.purgeUnbound(ctx, rec, before, dryRun, &res); err != nil {
				if errors.Is(err, ErrDependencyUnavailable) {
					return res, err
				}
				res.Conflicts++
				s.log().ErrorContext(ctx, "login_purge_row_failed", "pseudonym_prefix", rec.Pseudonym.Short(), "error", err.Error())
			}
		}
		if fresh == 0 || len(recs) < page+len(seen)-fresh {
			break
		}
	}
	var after login.Pseudonym
	for {
		recs, err := s.Logins.ListBound(ctx, after, page)
		if err != nil {
			return res, err
		}
		for _, rec := range recs {
			after = rec.Pseudonym
			id := *rec.IdentityID
			if _, err := s.Identities.GetIdentity(ctx, id); !errors.Is(err, ErrNotFound) {
				if err != nil {
					return res, fmt.Errorf("get identity: %w", err)
				}
				continue
			}
			res.OrphanDeleted++
			if dryRun {
				continue
			}
			if err := s.erase(ctx, id); err != nil {
				return res, err
			}
		}
		if len(recs) < page {
			break
		}
	}
	return res, nil
}

func (s *LoginIdentifierService) purgeUnbound(ctx context.Context, rec LoginRecord, before time.Time, dryRun bool, res *PurgeResult) error {
	owners, _, err := s.Identities.ListIdentities(ctx, IdentityQuery{Email: rec.Pseudonym.String()})
	if err != nil {
		return fmt.Errorf("find identity: %w", err)
	}
	var owner *identity.Identity
	for i := range owners {
		if owners[i].IsCustomer() {
			owner = &owners[i]
		}
	}
	switch {
	case owner != nil && dryRun:
		res.Rebound++
	case owner != nil:
		ok, err := s.Logins.Bind(ctx, rec.Pseudonym, owner.ID, nil)
		if err != nil {
			return fmt.Errorf("bind login: %w", err)
		}
		if ok {
			res.Rebound++
		}
	case dryRun:
		res.UnboundDeleted++
	default:
		ok, err := s.Logins.DeleteStaleUnbound(ctx, rec.Pseudonym, before)
		if err != nil {
			return err
		}
		if ok {
			res.UnboundDeleted++
		}
	}
	return nil
}

// erase deletes the identity's login and audits it in one transaction.
func (s *LoginIdentifierService) erase(ctx context.Context, id uuid.UUID) error {
	ev := systemEvent(audit.ActionCustomerLoginErased, id, map[string]any{"reason": "identity_deleted"})
	return s.Tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		if err := r.Audit.Append(ctx, ev); err != nil {
			return fmt.Errorf("append audit: %w", err)
		}
		_, err := r.Logins.DeleteForIdentity(ctx, id)
		return err
	})
}

// Rewrap re-encrypts every entry with the newest login KEK version (A13).
// Rows changed concurrently are skipped (optimistic).
func (s *LoginIdentifierService) Rewrap(ctx context.Context) (int, error) {
	var after login.Pseudonym
	n, latest := 0, 0
	const page = 100
	for {
		recs, err := s.Logins.ListAll(ctx, after, page)
		if err != nil {
			return n, err
		}
		if len(recs) == 0 {
			return n, nil
		}
		items := make([]SealedLogin, len(recs))
		for i, rec := range recs {
			items[i] = SealedLogin{AD: login.AAD(rec.Kind, rec.Pseudonym), Ciphertext: rec.Ciphertext}
		}
		pts, errs, err := s.Keys.OpenLogins(ctx, items)
		if err != nil {
			return n, err
		}
		for i, rec := range recs {
			after = rec.Pseudonym
			if latest > 0 && rec.KEKVersion >= latest {
				clear(pts[i])
				continue // already on the newest version (learned from an earlier seal)
			}
			if errs[i] != nil {
				s.log().ErrorContext(ctx, "login_decrypt_failed", "pseudonym_prefix", rec.Pseudonym.Short())
				continue
			}
			ct, v, err := s.Keys.SealLogin(ctx, items[i].AD, pts[i])
			clear(pts[i])
			if err != nil {
				return n, err
			}
			latest = max(latest, v)
			if v == rec.KEKVersion {
				continue
			}
			ok, err := s.Logins.UpdateCiphertext(ctx, rec.Pseudonym, rec.Ciphertext, ct, v)
			if err != nil {
				return n, err
			}
			if ok {
				n++
			}
		}
		if len(recs) < page {
			return n, nil
		}
	}
}

// open decrypts one record.
func (s *LoginIdentifierService) open(ctx context.Context, rec LoginRecord) (login.Identifier, error) {
	pts, errs, err := s.Keys.OpenLogins(ctx, []SealedLogin{{AD: login.AAD(rec.Kind, rec.Pseudonym), Ciphertext: rec.Ciphertext}})
	if err != nil {
		return login.Identifier{}, fmt.Errorf("open login: %w", err)
	}
	if errs[0] != nil {
		return login.Identifier{}, fmt.Errorf("open login: %w", ErrDataIntegrity)
	}
	id, err := login.FromStored(rec.Kind, string(pts[0]))
	clear(pts[0])
	if err != nil {
		return login.Identifier{}, fmt.Errorf("open login: %w", ErrDataIntegrity)
	}
	return id, nil
}

// legacyIdentifier reads a plaintext email login (pre-migration customer).
func legacyIdentifier(email string) (login.Identifier, error) {
	id, err := login.Parse(login.KindEmail, email, login.PhonePolicy{})
	if err != nil {
		return login.Identifier{}, fmt.Errorf("legacy login: %w", ErrDataIntegrity)
	}
	return id, nil
}

func ptrTime(t time.Time) *time.Time { return &t }

func (s *LoginIdentifierService) now() time.Time {
	if s.Clock != nil {
		return s.Clock.Now()
	}
	return time.Now().UTC()
}

func (s *LoginIdentifierService) log() *slog.Logger {
	if s.Log != nil {
		return s.Log
	}
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
