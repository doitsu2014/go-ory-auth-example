package app

import (
	"context"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"regexp"
	"time"

	"github.com/google/uuid"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/crypto/envelope"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/audit"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/identity"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/pii"
)

// MaxLookupCandidates bounds the blind-index candidates one lookup decrypts
// (§9 A11).
const MaxLookupCandidates = 20

// RevealReasonCode is the closed set of reveal reasons (§9 B2: no free text,
// because the audit log is append-only and must never hold PII).
type RevealReasonCode string

// Reveal reasons.
const (
	ReasonCustomerSupportRequest RevealReasonCode = "customer_support_request"
	ReasonIdentityVerification   RevealReasonCode = "identity_verification"
	ReasonFraudInvestigation     RevealReasonCode = "fraud_investigation"
	ReasonLegalRequest           RevealReasonCode = "legal_request"
)

// Valid reports whether c is a known reason.
func (c RevealReasonCode) Valid() bool {
	switch c {
	case ReasonCustomerSupportRequest, ReasonIdentityVerification, ReasonFraudInvestigation, ReasonLegalRequest:
		return true
	}
	return false
}

var ticketRefRe = regexp.MustCompile(`^[A-Z][A-Z0-9]{1,9}-[0-9]{1,8}$`)

// RevealRequest is the input of Reveal. Fields empty = all fields.
type RevealRequest struct {
	ReasonCode RevealReasonCode
	TicketRef  *string
	Fields     []string
}

// PersonalInfoView is decrypted personal info. UpdatedAt is nil when nothing
// was ever stored.
type PersonalInfoView struct {
	Info      pii.PersonalInfo
	UpdatedAt *time.Time
}

// MaskedPersonalInfoView is the admin (masked) view.
type MaskedPersonalInfoView struct {
	Masked    pii.Masked
	UpdatedAt *time.Time
}

// LookupResult is the outcome of a phone lookup. Truncated reports that
// more than MaxLookupCandidates records carry the blind index (phone numbers
// are self-declared, phone_verified: false, so others may claim a number).
type LookupResult struct {
	Matches   []LookupMatch
	Truncated bool
}

// LookupMatch is one phone lookup result (§9 B3: id, state, masked info; no
// email).
type LookupMatch struct {
	IdentityID uuid.UUID
	State      identity.State
	Masked     pii.Masked
	UpdatedAt  *time.Time
}

// PersonalInfoService implements the personal information use cases
// (PII-FR-01..07). Plaintext exists only in memory for one request; storage
// holds per-column AES-256-GCM ciphertexts under a per-customer DEK wrapped
// by the key manager.
//
// No key-manager or Kratos call runs inside a database transaction.
type PersonalInfoService struct {
	Authz       Authorizer
	Identities  IdentityAdmin
	Keys        KeyManager
	Cache       *DEKCache
	Tx          TxRunner
	SubjectKeys SubjectKeyRepo
	Records     CustomerPIIRepo
	Clock       Clock
	Log         *slog.Logger
	// Per-actor quotas (nil = unlimited). LookupLimiter is charged per
	// candidate unwrapped (§9 A11).
	LookupLimiter *RateLimiter
	RevealLimiter *RateLimiter
	MaskedLimiter *RateLimiter
}

// GetMine returns the caller's decrypted personal info. Permission: self
// (customer principal).
func (s *PersonalInfoService) GetMine(ctx context.Context, p identity.Principal) (PersonalInfoView, error) {
	if p.Kind != identity.KindCustomer {
		return PersonalInfoView{}, ErrForbidden
	}
	return s.read(ctx, p.IdentityID)
}

// PutMine replaces the caller's personal info (omitted fields are cleared).
// Permission: self with a verified email. Audited as customer.pii.updated
// with field names only.
func (s *PersonalInfoService) PutMine(ctx context.Context, a Actor, in pii.PersonalInfo) (PersonalInfoView, error) {
	p := a.Principal
	if p.Kind != identity.KindCustomer {
		return PersonalInfoView{}, ErrForbidden
	}
	if !p.EmailVerified {
		return PersonalInfoView{}, ErrEmailNotVerified
	}
	in = in.Normalize()
	if err := in.Validate(s.now()); err != nil {
		return PersonalInfoView{}, err
	}
	cols, err := in.Encode()
	if err != nil {
		return PersonalInfoView{}, fmt.Errorf("encode personal info: %w", err)
	}
	defer zeroColumns(cols)
	var bidx *BlindIndex
	if in.Phone != nil {
		input := pii.PhoneBlindIndexInput(*in.Phone)
		b, err := s.Keys.BlindIndex(ctx, input)
		envelope.Zero(input)
		if err != nil {
			return PersonalInfoView{}, fmt.Errorf("blind index: %w", err)
		}
		bidx = &b
	}
	ev := s.event(a, audit.ActionCustomerPIIUpdated, p.IdentityID, map[string]any{"fields": in.FieldNames()})
	// A concurrent erase can delete the subject key between keyFor and the
	// upsert (FK violation → ErrConflict) or between the insert conflict and
	// its re-read (ErrNotFound). Retry once with a fresh key.
	var updated time.Time
	for attempt := 0; ; attempt++ {
		updated, err = s.putOnce(ctx, p.IdentityID, cols, bidx, ev)
		if err == nil {
			break
		}
		if attempt == 0 && (errors.Is(err, ErrConflict) || errors.Is(err, ErrNotFound)) {
			s.log().InfoContext(ctx, "pii_put_retry_after_concurrent_erase", "identity_id", p.IdentityID.String())
			continue
		}
		return PersonalInfoView{}, err
	}
	return PersonalInfoView{Info: in, UpdatedAt: &updated}, nil
}

// putOnce seals cols under the subject's (possibly new) key and commits the
// record and the audit event in one transaction.
func (s *PersonalInfoService) putOnce(ctx context.Context, id uuid.UUID, cols map[pii.Field][]byte, bidx *BlindIndex, ev audit.Event) (time.Time, error) {
	key, dek, err := s.keyFor(ctx, id)
	if err != nil {
		return time.Time{}, err
	}
	defer envelope.Zero(dek)
	rec := EncryptedPII{IdentityID: id, KeyID: key.KeyID, PhoneBidx: bidx}
	for f, pt := range cols {
		ct, err := envelope.Seal(dek, envelope.AAD(string(f), id), pt)
		if err != nil {
			return time.Time{}, fmt.Errorf("seal %s: %w", f, err)
		}
		switch f {
		case pii.FieldPhoneNumber:
			rec.Phone = ct
		case pii.FieldDateOfBirth:
			rec.DateOfBirth = ct
		case pii.FieldAddress:
			rec.Address = ct
		case pii.FieldNationalID:
			rec.NationalID = ct
		}
	}
	var updated time.Time
	tctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), mutationTimeout)
	defer cancel()
	err = s.Tx.WithinTx(tctx, func(ctx context.Context, r Repos) error {
		t, err := r.PersonalInfo.Upsert(ctx, rec)
		if err != nil {
			return fmt.Errorf("store personal info: %w", err)
		}
		updated = t
		if err := r.Audit.Append(ctx, ev); err != nil {
			return fmt.Errorf("append audit: %w", err)
		}
		return nil
	})
	if err != nil && errors.Is(err, ErrConflict) {
		s.Cache.Evict(key.KeyID) // the key was erased concurrently
	}
	return updated, err
}

// EraseMine crypto-shreds the caller's personal info: one transaction
// deletes the subject key (cascading to the encrypted record) and appends
// customer.pii.erased; then the cached DEK is evicted. Idempotent.
// Permission: any authenticated customer (§9 A14).
func (s *PersonalInfoService) EraseMine(ctx context.Context, a Actor) error {
	p := a.Principal
	if p.Kind != identity.KindCustomer {
		return ErrForbidden
	}
	var keyIDs []uuid.UUID
	ev := s.event(a, audit.ActionCustomerPIIErased, p.IdentityID, map[string]any{})
	tctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), mutationTimeout)
	defer cancel()
	err := s.Tx.WithinTx(tctx, func(ctx context.Context, r Repos) error {
		ids, err := r.SubjectKeys.Delete(ctx, p.IdentityID)
		if err != nil {
			return fmt.Errorf("delete subject key: %w", err)
		}
		if len(ids) == 0 {
			return nil // nothing stored: nothing to erase or audit
		}
		keyIDs = ids
		if err := r.Audit.Append(ctx, ev); err != nil {
			return fmt.Errorf("append audit: %w", err)
		}
		return nil
	})
	for _, id := range keyIDs {
		s.Cache.Evict(id)
	}
	return err
}

// GetMasked returns a customer's masked personal info. Permission:
// view_customers. Non-customers are not found.
func (s *PersonalInfoService) GetMasked(ctx context.Context, a Actor, id uuid.UUID) (MaskedPersonalInfoView, error) {
	if err := require(ctx, s.Authz, a, identity.PermViewCustomers); err != nil {
		return MaskedPersonalInfoView{}, err
	}
	if !s.MaskedLimiter.Allow(a.Principal.IdentityID) {
		return MaskedPersonalInfoView{}, ErrRateLimited
	}
	if _, err := s.customer(ctx, id); err != nil {
		return MaskedPersonalInfoView{}, err
	}
	v, err := s.read(ctx, id)
	if err != nil {
		return MaskedPersonalInfoView{}, err
	}
	return MaskedPersonalInfoView{Masked: v.Info.Mask(), UpdatedAt: v.UpdatedAt}, nil
}

// Reveal returns a customer's full personal info (or the requested fields).
// Permission: reveal_customer_pii; quota 20/hour per actor. Order (DD-8,
// §9 A13): decrypt in memory → COMMIT audit customer.pii.revealed → return.
// If the audit cannot be written, the plaintext buffers are zeroed and
// nothing is returned.
func (s *PersonalInfoService) Reveal(ctx context.Context, a Actor, id uuid.UUID, req RevealRequest) (PersonalInfoView, error) {
	if err := require(ctx, s.Authz, a, identity.PermRevealCustomerPII); err != nil {
		return PersonalInfoView{}, err
	}
	fields, err := validateReveal(req)
	if err != nil {
		return PersonalInfoView{}, err
	}
	if !s.RevealLimiter.Allow(a.Principal.IdentityID) {
		return PersonalInfoView{}, ErrRateLimited
	}
	if _, err := s.customer(ctx, id); err != nil {
		return PersonalInfoView{}, err
	}
	cols, rec, found, err := s.decrypt(ctx, id, fields)
	defer zeroColumns(cols)
	if err != nil {
		return PersonalInfoView{}, err
	}
	names := make([]string, len(fields))
	for i, f := range fields {
		names[i] = string(f)
	}
	details := map[string]any{"fields": names, "reason_code": string(req.ReasonCode)}
	if req.TicketRef != nil {
		details["ticket_ref"] = *req.TicketRef
	}
	if err := s.appendAudit(ctx, s.event(a, audit.ActionCustomerPIIRevealed, id, details)); err != nil {
		return PersonalInfoView{}, fmt.Errorf("audit reveal: %w", err)
	}
	if !found {
		return PersonalInfoView{}, nil
	}
	info, err := s.decode(id, cols)
	if err != nil {
		return PersonalInfoView{}, err
	}
	updated := rec.UpdatedAt
	return PersonalInfoView{Info: info, UpdatedAt: &updated}, nil
}

// LookupByPhone finds customers by phone number through the blind index
// only, then decrypts each candidate's phone (and nothing else) and keeps
// exact matches (§9 A10); only matches are decrypted further for masking.
// Permission: view_customers. Quota per actor: 30/min and 200/day, charged
// per candidate unwrapped (minimum 1 per call). Phone numbers are
// self-declared, so at most MaxLookupCandidates candidates are examined and
// Truncated reports an overflow. Audited as customer.pii.lookup with the
// keyed hash (never the number) and the matched ids.
func (s *PersonalInfoService) LookupByPhone(ctx context.Context, a Actor, phone string) (LookupResult, error) {
	if err := require(ctx, s.Authz, a, identity.PermViewCustomers); err != nil {
		return LookupResult{}, err
	}
	e164 := pii.NormalizePhone(phone)
	if !pii.ValidPhone(e164) {
		return LookupResult{}, NewValidationError(string(pii.FieldPhoneNumber), pii.CodeInvalidFormat)
	}
	actorID := a.Principal.IdentityID
	if !s.LookupLimiter.Peek(actorID, 1) {
		return LookupResult{}, ErrRateLimited
	}
	input := pii.PhoneBlindIndexInput(e164)
	bidx, err := s.Keys.BlindIndex(ctx, input)
	envelope.Zero(input)
	if err != nil {
		return LookupResult{}, fmt.Errorf("blind index: %w", err)
	}
	recs, err := s.Records.FindByPhoneBidx(ctx, bidx, MaxLookupCandidates+1)
	if err != nil {
		return LookupResult{}, fmt.Errorf("find by blind index: %w", err)
	}
	res := LookupResult{Matches: []LookupMatch{}}
	if len(recs) > MaxLookupCandidates {
		recs, res.Truncated = recs[:MaxLookupCandidates], true
		s.log().WarnContext(ctx, "pii_lookup_candidate_overflow", "actor_id", actorID.String(), "candidates_over", MaxLookupCandidates)
	}
	if !s.LookupLimiter.AllowN(actorID, max(1, len(recs))) {
		return LookupResult{}, ErrRateLimited
	}
	want := []byte(e164)
	defer envelope.Zero(want)
	ids := []string{}
	for _, rec := range recs {
		m, ok, err := s.lookupCandidate(ctx, rec, want)
		if err != nil {
			return LookupResult{}, err
		}
		if ok {
			res.Matches = append(res.Matches, m)
			ids = append(ids, m.IdentityID.String())
		}
	}
	details := map[string]any{"bidx": hex.EncodeToString(bidx.Sum), "matched_ids": ids, "matches": len(ids), "truncated": res.Truncated}
	if err := s.appendAudit(ctx, s.event(a, audit.ActionCustomerPIILookup, uuid.Nil, details)); err != nil {
		return LookupResult{}, fmt.Errorf("audit lookup: %w", err)
	}
	return res, nil
}

func (s *PersonalInfoService) lookupCandidate(ctx context.Context, rec EncryptedPII, want []byte) (LookupMatch, bool, error) {
	dek, err := s.recordDEK(ctx, rec)
	if err != nil {
		if errors.Is(err, ErrDataIntegrity) {
			return LookupMatch{}, false, nil // logged; never break the lookup
		}
		return LookupMatch{}, false, err
	}
	defer envelope.Zero(dek)
	phone, err := s.openColumns(ctx, dek, rec, []pii.Field{pii.FieldPhoneNumber})
	defer zeroColumns(phone)
	if err != nil {
		return LookupMatch{}, false, nil
	}
	if subtle.ConstantTimeCompare(phone[pii.FieldPhoneNumber], want) != 1 {
		s.log().WarnContext(ctx, "pii_lookup_bidx_mismatch", "identity_id", rec.IdentityID.String())
		return LookupMatch{}, false, nil
	}
	ident, err := s.Identities.GetIdentity(ctx, rec.IdentityID)
	if errors.Is(err, ErrNotFound) {
		return LookupMatch{}, false, nil
	}
	if err != nil {
		return LookupMatch{}, false, fmt.Errorf("get identity: %w", err)
	}
	if ident.SchemaID != string(identity.KindCustomer) {
		return LookupMatch{}, false, nil
	}
	cols, err := s.openColumns(ctx, dek, rec, []pii.Field{pii.FieldDateOfBirth, pii.FieldAddress, pii.FieldNationalID})
	defer zeroColumns(cols)
	if err != nil {
		return LookupMatch{}, false, nil
	}
	cols[pii.FieldPhoneNumber] = phone[pii.FieldPhoneNumber]
	info, err := s.decode(rec.IdentityID, cols)
	if err != nil {
		return LookupMatch{}, false, nil
	}
	updated := rec.UpdatedAt
	return LookupMatch{IdentityID: rec.IdentityID, State: ident.State, Masked: info.Mask(), UpdatedAt: &updated}, true, nil
}

// --- internals ---

func (s *PersonalInfoService) read(ctx context.Context, id uuid.UUID) (PersonalInfoView, error) {
	cols, rec, found, err := s.decrypt(ctx, id, pii.AllFields)
	defer zeroColumns(cols)
	if err != nil || !found {
		return PersonalInfoView{}, err
	}
	info, err := s.decode(id, cols)
	if err != nil {
		return PersonalInfoView{}, err
	}
	updated := rec.UpdatedAt
	return PersonalInfoView{Info: info, UpdatedAt: &updated}, nil
}

// decrypt loads and opens the requested columns of a subject's record.
// found=false when nothing is stored.
func (s *PersonalInfoService) decrypt(ctx context.Context, id uuid.UUID, fields []pii.Field) (map[pii.Field][]byte, EncryptedPII, bool, error) {
	rec, err := s.Records.Get(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return nil, EncryptedPII{}, false, nil
	}
	if err != nil {
		return nil, EncryptedPII{}, false, fmt.Errorf("get personal info: %w", err)
	}
	cols, err := s.open(ctx, rec, fields)
	if err != nil {
		zeroColumns(cols)
		return nil, EncryptedPII{}, false, err
	}
	return cols, rec, true, nil
}

// open unwraps the record's DEK and opens the requested columns.
func (s *PersonalInfoService) open(ctx context.Context, rec EncryptedPII, fields []pii.Field) (map[pii.Field][]byte, error) {
	dek, err := s.recordDEK(ctx, rec)
	if err != nil {
		return nil, err
	}
	defer envelope.Zero(dek)
	return s.openColumns(ctx, dek, rec, fields)
}

// recordDEK returns the DEK of the record's subject key (caller zeroes it).
func (s *PersonalInfoService) recordDEK(ctx context.Context, rec EncryptedPII) ([]byte, error) {
	key, err := s.SubjectKeys.Get(ctx, rec.IdentityID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrDataIntegrity // impossible with the composite FK
		}
		return nil, fmt.Errorf("get subject key: %w", err)
	}
	if key.KeyID != rec.KeyID {
		s.log().ErrorContext(ctx, "pii_key_mismatch", "identity_id", rec.IdentityID.String())
		return nil, ErrDataIntegrity
	}
	return s.dek(ctx, key)
}

// openColumns opens the requested columns with dek.
func (s *PersonalInfoService) openColumns(ctx context.Context, dek []byte, rec EncryptedPII, fields []pii.Field) (map[pii.Field][]byte, error) {
	cols := map[pii.Field][]byte{}
	for _, f := range fields {
		var ct []byte
		switch f {
		case pii.FieldPhoneNumber:
			ct = rec.Phone
		case pii.FieldDateOfBirth:
			ct = rec.DateOfBirth
		case pii.FieldAddress:
			ct = rec.Address
		case pii.FieldNationalID:
			ct = rec.NationalID
		}
		if ct == nil {
			continue
		}
		pt, err := envelope.Open(dek, envelope.AAD(string(f), rec.IdentityID), ct)
		if err != nil {
			s.log().ErrorContext(ctx, "pii_decrypt_failed", "identity_id", rec.IdentityID.String(), "column", string(f))
			return cols, ErrDataIntegrity
		}
		cols[f] = pt
	}
	return cols, nil
}

func (s *PersonalInfoService) decode(id uuid.UUID, cols map[pii.Field][]byte) (pii.PersonalInfo, error) {
	info, err := pii.Decode(cols)
	if err != nil {
		s.log().Error("pii_decode_failed", "identity_id", id.String())
		return pii.PersonalInfo{}, ErrDataIntegrity
	}
	return info, nil
}

// dek returns a copy of the subject's DEK (cache, else unwrap); the caller
// zeroes it.
func (s *PersonalInfoService) dek(ctx context.Context, key SubjectKey) ([]byte, error) {
	if d, ok := s.Cache.Get(key.KeyID); ok {
		return d, nil
	}
	d, err := s.Keys.UnwrapDEK(ctx, key.Context(), key.Wrapped)
	if err != nil {
		if errors.Is(err, ErrDataIntegrity) {
			s.log().ErrorContext(ctx, "pii_dek_unwrap_failed", "identity_id", key.IdentityID.String(), "key_id", key.KeyID.String())
		}
		return nil, fmt.Errorf("unwrap data key: %w", err)
	}
	if len(d) != envelope.KeySize {
		envelope.Zero(d)
		return nil, ErrDataIntegrity
	}
	s.Cache.Put(key.KeyID, d)
	return d, nil
}

// keyFor returns the subject's key and DEK, creating and wrapping a new DEK
// on first write. A concurrent first write wins via ON CONFLICT DO NOTHING
// and the loser re-reads.
func (s *PersonalInfoService) keyFor(ctx context.Context, id uuid.UUID) (SubjectKey, []byte, error) {
	key, err := s.SubjectKeys.Get(ctx, id)
	if err == nil {
		dek, err := s.dek(ctx, key)
		return key, dek, err
	}
	if !errors.Is(err, ErrNotFound) {
		return SubjectKey{}, nil, fmt.Errorf("get subject key: %w", err)
	}
	dek, err := envelope.NewDEK()
	if err != nil {
		return SubjectKey{}, nil, fmt.Errorf("new data key: %w", err)
	}
	kc := KeyContext{IdentityID: id, KeyID: uuid.New()}
	w, err := s.Keys.WrapDEK(ctx, kc, dek)
	if err != nil {
		envelope.Zero(dek)
		return SubjectKey{}, nil, fmt.Errorf("wrap data key: %w", err)
	}
	stored, inserted, err := s.SubjectKeys.Insert(ctx, SubjectKey{KeyID: kc.KeyID, IdentityID: id, Wrapped: w})
	if err != nil {
		envelope.Zero(dek)
		return SubjectKey{}, nil, fmt.Errorf("insert subject key: %w", err)
	}
	if !inserted {
		envelope.Zero(dek)
		dek, err := s.dek(ctx, stored)
		return stored, dek, err
	}
	s.Cache.Put(stored.KeyID, dek)
	return stored, dek, nil
}

func (s *PersonalInfoService) customer(ctx context.Context, id uuid.UUID) (identity.Identity, error) {
	ident, err := s.Identities.GetIdentity(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return identity.Identity{}, ErrNotFound
		}
		return identity.Identity{}, fmt.Errorf("get identity: %w", err)
	}
	if ident.SchemaID != string(identity.KindCustomer) {
		return identity.Identity{}, ErrNotFound
	}
	return ident, nil
}

// appendAudit commits one audit event in its own transaction, detached from
// request cancellation.
func (s *PersonalInfoService) appendAudit(ctx context.Context, ev audit.Event) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), mutationTimeout)
	defer cancel()
	return s.Tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		return r.Audit.Append(ctx, ev)
	})
}

func (s *PersonalInfoService) event(a Actor, action audit.Action, target uuid.UUID, details map[string]any) audit.Event {
	ev := audit.Event{
		ActorID: a.Principal.IdentityID, Action: action, TargetType: audit.TargetCustomer, TargetID: target.String(),
		RequestID: a.RequestID, ClientIP: a.ClientIP, Details: details,
	}
	if target == uuid.Nil {
		ev.TargetID = "lookup"
	}
	return ev
}

func (s *PersonalInfoService) now() time.Time {
	if s.Clock != nil {
		return s.Clock.Now()
	}
	return time.Now().UTC()
}

func (s *PersonalInfoService) log() *slog.Logger {
	if s.Log != nil {
		return s.Log
	}
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func validateReveal(req RevealRequest) ([]pii.Field, error) {
	var errs []FieldError
	if req.ReasonCode == "" {
		errs = append(errs, FieldError{Field: "reason_code", Code: pii.CodeRequired})
	} else if !req.ReasonCode.Valid() {
		errs = append(errs, FieldError{Field: "reason_code", Code: pii.CodeInvalidFormat})
	}
	if req.TicketRef != nil && !ticketRefRe.MatchString(*req.TicketRef) {
		errs = append(errs, FieldError{Field: "ticket_ref", Code: pii.CodeInvalidFormat})
	}
	fields := pii.AllFields
	if req.Fields != nil {
		fields = nil
		seen := map[pii.Field]bool{}
		if len(req.Fields) == 0 {
			errs = append(errs, FieldError{Field: "fields", Code: pii.CodeRequired})
		}
		for _, name := range req.Fields {
			f, ok := pii.ParseField(name)
			if !ok || seen[f] {
				errs = append(errs, FieldError{Field: "fields", Code: pii.CodeInvalidFormat})
				break
			}
			seen[f] = true
			fields = append(fields, f)
		}
	}
	if len(errs) > 0 {
		return nil, &ValidationError{Fields: errs}
	}
	return fields, nil
}

func zeroColumns(cols map[pii.Field][]byte) {
	for _, b := range cols {
		envelope.Zero(b)
	}
}
