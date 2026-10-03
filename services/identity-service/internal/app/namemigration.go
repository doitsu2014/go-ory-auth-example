package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"github.com/google/uuid"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/crypto/envelope"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/audit"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/identity"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/pii"
)

// NameMigrationReport summarises a migrate-kratos-names run. Scanned counts
// customer identities that still carry a name trait (even an empty one);
// each ends in exactly one of Migrated (encrypted copy committed, then
// stripped), StrippedOnly (erased customer, a name already stored, an empty
// name, or an invalid one with StripInvalid) or Failed. In a dry run the
// counts say what would happen and nothing is written.
type NameMigrationReport struct {
	Scanned      int
	Migrated     int
	StrippedOnly int
	Failed       int
}

// NameTraitRemovalReason says why a name trait was removed without storing
// it (audit detail "reason" of customer.pii.name_trait_removed).
type NameTraitRemovalReason string

// Removal reasons.
const (
	RemovedErased          NameTraitRemovalReason = "erased"
	RemovedAlreadyMigrated NameTraitRemovalReason = "already_migrated"
	RemovedEmpty           NameTraitRemovalReason = "empty"
	RemovedInvalid         NameTraitRemovalReason = "invalid"
)

// NameMigrationService moves customer names out of the Kratos traits into
// the encrypted personal info (NAME-FR-08). Operator CLI only.
type NameMigrationService struct {
	PersonalInfo *PersonalInfoService
	Kratos       NameTraitAdmin
	Log          *slog.Logger
	// StripInvalid removes names that fail validation from Kratos without
	// storing them (audited with reason "invalid"); by default they are kept
	// in Kratos and counted as failed.
	StripInvalid bool
}

type nameOutcome int

const (
	nameMigrated nameOutcome = iota + 1
	nameStrippedOnly
)

var (
	// errNameNotSet rolls back the migration transaction when SetName
	// matched no row (the record got a name or another key meanwhile).
	errNameNotSet = errors.New("personal info name not set")
	// errErasedConcurrently rolls back the migration transaction when the
	// customer erased their personal info after decide() checked.
	errErasedConcurrently = errors.New("personal info erased concurrently")
)

// MigrateKratosNames migrates every customer identity with traits.name:
// (a) erased customers (customer.pii.erased in the audit log) are only
// stripped — earlier erase requests win; (b) customers whose personal info
// already holds a name are only stripped; (c) otherwise the name is sealed
// under the customer's DEK (created if none) and, in one transaction, the
// audit event customer.pii.name_migrated (system actor, field names only)
// and the name column are committed, after re-checking that no erasure was
// committed meanwhile — only then is the trait removed from Kratos (only
// while it still holds the migrated value). Strip-only removals are audited
// as customer.pii.name_trait_removed (committed before the Kratos call). A
// crash between the commit and the strip leaves the name in Kratos; a
// re-run takes branch (b). Idempotent. Logs carry identity ids only.
// Dependency errors while listing abort the run; errors on one identity are
// counted and the run continues.
func (s *NameMigrationService) MigrateKratosNames(ctx context.Context, dryRun bool) (NameMigrationReport, error) {
	var rep NameMigrationReport
	token := ""
	for {
		items, next, err := s.Kratos.ListIdentities(ctx, IdentityQuery{PageSize: MaxPageSize, PageToken: token})
		if err != nil {
			return rep, fmt.Errorf("list identities: %w", err)
		}
		for _, it := range items {
			if it.SchemaID != string(identity.KindCustomer) || (!it.HasNameTrait && it.Name == (identity.Name{})) {
				continue
			}
			rep.Scanned++
			out, err := s.migrateOne(ctx, it, dryRun)
			switch {
			case err != nil:
				rep.Failed++
				s.log().ErrorContext(ctx, "pii_name_migration_failed", "identity_id", it.ID.String(), "error", err.Error())
			case out == nameMigrated:
				rep.Migrated++
			default:
				rep.StrippedOnly++
			}
		}
		if next == "" {
			break
		}
		token = next
	}
	if rep.Failed > 0 {
		return rep, fmt.Errorf("%d identities could not be migrated", rep.Failed)
	}
	return rep, nil
}

func (s *NameMigrationService) migrateOne(ctx context.Context, it identity.Identity, dryRun bool) (nameOutcome, error) {
	id := it.ID
	// One retry covers a concurrent first write or key change between the
	// checks and the commit; the retry re-evaluates every branch.
	for attempt := 0; ; attempt++ {
		reason, sealed, err := s.decide(ctx, it)
		if err != nil {
			return 0, err
		}
		if sealed == nil {
			return nameStrippedOnly, s.stripOnly(ctx, it, reason, dryRun)
		}
		if dryRun {
			return nameMigrated, nil
		}
		err = s.store(ctx, id, *sealed)
		switch {
		case err == nil:
			s.log().InfoContext(ctx, "pii_name_migrated", "identity_id", id.String())
			return nameMigrated, s.strip(ctx, it)
		case errors.Is(err, errErasedConcurrently):
			s.log().InfoContext(ctx, "pii_name_migration_erased_concurrently", "identity_id", id.String())
			return nameStrippedOnly, s.stripOnly(ctx, it, RemovedErased, false)
		case attempt == 0 && (errors.Is(err, errNameNotSet) || errors.Is(err, ErrConflict) || errors.Is(err, ErrNotFound)):
			s.log().InfoContext(ctx, "pii_name_migration_retry", "identity_id", id.String())
			continue
		}
		return 0, err
	}
}

// decide picks the branch: a non-nil name means branch (c) (seal and
// store); otherwise the trait is only removed, for reason.
func (s *NameMigrationService) decide(ctx context.Context, it identity.Identity) (NameTraitRemovalReason, *pii.Name, error) {
	var erased bool
	err := s.PersonalInfo.Tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		var err error
		erased, err = erasedIn(ctx, r, it.ID)
		return err
	})
	if err != nil {
		return "", nil, err
	}
	if erased {
		return RemovedErased, nil, nil
	}
	rec, err := s.PersonalInfo.Records.Get(ctx, it.ID)
	switch {
	case err == nil && rec.Name != nil:
		return RemovedAlreadyMigrated, nil, nil
	case err != nil && !errors.Is(err, ErrNotFound):
		return "", nil, fmt.Errorf("get personal info: %w", err)
	}
	in := pii.PersonalInfo{Name: &pii.Name{First: it.Name.First, Last: it.Name.Last}}.Normalize()
	if in.Name == nil {
		return RemovedEmpty, nil, nil // empty object, whitespace or format characters only
	}
	if err := in.Validate(s.PersonalInfo.now()); err != nil {
		if s.StripInvalid {
			return RemovedInvalid, nil, nil
		}
		// Never strip a name that cannot be stored unless the operator asked
		// for it; the error names fields and codes only.
		return "", nil, fmt.Errorf("stored name is not valid personal info (see --strip-invalid): %w", err)
	}
	return "", in.Name, nil
}

// store seals the name under the subject's key and commits the audit event
// and the name column in one transaction. The erasure ledger is re-checked
// inside the transaction (MAJOR-1): an erase committed after decide() rolls
// the write back (errErasedConcurrently), and a key this call created for
// the erased subject is removed again.
func (s *NameMigrationService) store(ctx context.Context, id uuid.UUID, name pii.Name) error {
	pi := s.PersonalInfo
	key, dek, created, err := pi.keyForCreated(ctx, id)
	if err != nil {
		return err
	}
	defer envelope.Zero(dek)
	pt, err := pii.EncodeName(name)
	if err != nil {
		return fmt.Errorf("encode name: %w", err)
	}
	ct, err := envelope.Seal(dek, envelope.AAD(string(pii.FieldName), id), pt)
	envelope.Zero(pt)
	if err != nil {
		return fmt.Errorf("seal %s: %w", pii.FieldName, err)
	}
	ev := systemEvent(audit.ActionCustomerPIINameMigrated, id, map[string]any{"fields": []string{string(pii.FieldName)}})
	tctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), mutationTimeout)
	defer cancel()
	err = pi.Tx.WithinTx(tctx, func(ctx context.Context, r Repos) error {
		if err := r.Audit.Append(ctx, ev); err != nil {
			return fmt.Errorf("append audit: %w", err)
		}
		set, err := r.PersonalInfo.SetName(ctx, id, key.KeyID, ct)
		if err != nil {
			return fmt.Errorf("store personal info name: %w", err)
		}
		if !set {
			return errNameNotSet
		}
		erased, err := erasedIn(ctx, r, id)
		if err != nil {
			return err
		}
		if erased {
			return errErasedConcurrently
		}
		return nil
	})
	switch {
	case errors.Is(err, ErrConflict):
		pi.Cache.Evict(key.KeyID) // the key was erased concurrently
	case errors.Is(err, errErasedConcurrently) && created:
		if cerr := s.dropCreatedKey(tctx, id, key.KeyID); cerr != nil {
			return fmt.Errorf("%w (and removing the new key failed: %v)", err, cerr)
		}
	}
	return err
}

// dropCreatedKey deletes the key store() created for a subject that erased
// concurrently, unless the subject stored personal info under it meanwhile.
func (s *NameMigrationService) dropCreatedKey(ctx context.Context, id, keyID uuid.UUID) error {
	err := s.PersonalInfo.Tx.WithinTx(ctx, func(ctx context.Context, r Repos) error {
		k, err := r.SubjectKeys.Get(ctx, id)
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("get subject key: %w", err)
		}
		if k.KeyID != keyID {
			return nil
		}
		if _, err := r.PersonalInfo.Get(ctx, id); !errors.Is(err, ErrNotFound) {
			return err // a record exists (or the read failed): keep the key
		}
		_, err = r.SubjectKeys.Delete(ctx, id)
		return err
	})
	s.PersonalInfo.Cache.Evict(keyID)
	return err
}

// stripOnly audits the removal (committed before the Kratos call, so no
// removal is ever unaudited) and removes the trait.
func (s *NameMigrationService) stripOnly(ctx context.Context, it identity.Identity, reason NameTraitRemovalReason, dryRun bool) error {
	if dryRun {
		return nil
	}
	ev := systemEvent(audit.ActionCustomerPIINameTraitRemoved, it.ID,
		map[string]any{"fields": []string{string(pii.FieldName)}, "reason": string(reason)})
	if err := s.PersonalInfo.appendAudit(ctx, ev); err != nil {
		return fmt.Errorf("audit name trait removal: %w", err)
	}
	return s.strip(ctx, it)
}

// strip removes the trait (never before the encrypted copy is committed).
func (s *NameMigrationService) strip(ctx context.Context, it identity.Identity) error {
	err := s.Kratos.RemoveTraitName(ctx, it.ID, it.Name)
	if errors.Is(err, ErrNotFound) {
		return nil // identity deleted meanwhile
	}
	if err != nil {
		return fmt.Errorf("remove name trait: %w", err)
	}
	return nil
}

// erasedIn reports whether the customer ever erased their personal info.
func erasedIn(ctx context.Context, r Repos, id uuid.UUID) (bool, error) {
	target, tt := id.String(), audit.TargetCustomer
	evs, err := r.Audit.List(ctx, audit.Filter{
		TargetType: &tt, TargetID: &target, Actions: []audit.Action{audit.ActionCustomerPIIErased}, Limit: 1,
	})
	if err != nil {
		return false, fmt.Errorf("list audit: %w", err)
	}
	return len(evs) > 0, nil
}

func systemEvent(action audit.Action, target uuid.UUID, details map[string]any) audit.Event {
	return audit.Event{
		ActorID: audit.SystemActor, Action: action, TargetType: audit.TargetCustomer, TargetID: target.String(), Details: details,
	}
}

func (s *NameMigrationService) log() *slog.Logger {
	if s.Log != nil {
		return s.Log
	}
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
