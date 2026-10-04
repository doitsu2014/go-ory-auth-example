package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/audit"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/identity"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/login"
)

// LoginMigrationResult counts migration outcomes.
type LoginMigrationResult struct {
	Scanned    int
	Migrated   int
	Reverified int
	Skipped    int
	Failed     int
}

// LoginMigrationService moves legacy customers (plaintext traits.email) to
// pseudonymous logins (PLI-FR-13, DD-12). Per identity, in crash-safe order:
//
//  1. store the email in the vault, bound to the identity, recording whether
//     it was verified (legacy_verified);
//  2. audit customer.login.migrated (committed before the Kratos call);
//  3. replace traits.email with traits.login_id (read-compare-patch);
//  4. if it was verified, mark the new address verified (second patch, S4b).
//
// A crash after 3 leaves an unverified pseudonymous identity whose vault row
// says legacy_verified; the next run finishes step 4. Re-runs are no-ops.
type LoginMigrationService struct {
	Logins *LoginIdentifierService
	Kratos LoginTraitAdmin
	Tx     TxRunner
	Log    *slog.Logger
}

// Migrate migrates every customer. dryRun reports what would change and
// writes nothing.
func (s *LoginMigrationService) Migrate(ctx context.Context, dryRun bool) (LoginMigrationResult, error) {
	var res LoginMigrationResult
	token := ""
	for {
		items, next, err := s.Kratos.ListIdentities(ctx, IdentityQuery{PageSize: MaxPageSize, PageToken: token})
		if err != nil {
			return res, fmt.Errorf("list identities: %w", err)
		}
		for _, it := range items {
			if it.SchemaID != string(identity.KindCustomer) {
				continue
			}
			res.Scanned++
			if err := s.one(ctx, it, dryRun, &res); err != nil {
				if errors.Is(err, ErrDependencyUnavailable) {
					return res, err
				}
				res.Failed++
				s.log().ErrorContext(ctx, "login_migration_failed", "identity_id", it.ID.String(), "error", err.Error())
			}
		}
		if next == "" {
			return res, nil
		}
		token = next
	}
}

func (s *LoginMigrationService) one(ctx context.Context, it identity.Identity, dryRun bool, res *LoginMigrationResult) error {
	if p, ok := login.ParsePseudonym(it.LoginID); ok {
		return s.finish(ctx, it, p, dryRun, res)
	}
	id, err := login.Parse(login.KindEmail, it.Email, login.PhonePolicy{})
	if err != nil {
		return fmt.Errorf("legacy email: %w", err) // value-free (*login.ErrInvalid)
	}
	p, err := s.Logins.pseudonym(ctx, id)
	if err != nil {
		return err
	}
	if dryRun {
		res.Migrated++
		return nil
	}
	if _, err := s.Logins.store(ctx, id, p, &it.ID, it.EmailVerified); err != nil {
		return err
	}
	// The row may have existed unbound (a registration resolve for the
	// same address): bind it, and record the verification state.
	if _, err := s.Logins.bind(ctx, it.ID, p); err != nil {
		return fmt.Errorf("collision: %w", err)
	}
	if err := s.Logins.Logins.SetLegacyVerified(ctx, p, it.EmailVerified); err != nil {
		return err
	}
	ev := systemEvent(audit.ActionCustomerLoginMigrated, it.ID, map[string]any{"kind": string(login.KindEmail), "verified": it.EmailVerified})
	if err := s.Tx.WithinTx(ctx, func(ctx context.Context, r Repos) error { return r.Audit.Append(ctx, ev) }); err != nil {
		return fmt.Errorf("audit: %w", err)
	}
	err = s.Kratos.ReplaceLoginTrait(ctx, it.ID, it.Email, p.String())
	if errors.Is(err, ErrNotFound) {
		res.Skipped++ // deleted meanwhile; the purge removes the orphan row
		return nil
	}
	if err != nil {
		return fmt.Errorf("replace login trait: %w", err)
	}
	if it.EmailVerified {
		if err := s.Kratos.MarkLoginVerified(ctx, it.ID, p.String()); err != nil {
			return fmt.Errorf("mark verified (re-run finishes it): %w", err)
		}
	}
	res.Migrated++
	return nil
}

// finish completes an already-pseudonymous identity: re-applies the
// verification lost by a crash between steps 3 and 4, and binds the row.
func (s *LoginMigrationService) finish(ctx context.Context, it identity.Identity, p login.Pseudonym, dryRun bool, res *LoginMigrationResult) error {
	rec, err := s.Logins.Logins.Get(ctx, p)
	if errors.Is(err, ErrNotFound) {
		return errors.New("pseudonymous identity without a vault entry")
	}
	if err != nil {
		return err
	}
	if !rec.LegacyVerified || it.EmailVerified {
		res.Skipped++
		return nil
	}
	if dryRun {
		res.Reverified++
		return nil
	}
	if rec.IdentityID == nil || *rec.IdentityID != it.ID {
		if _, err := s.Logins.bind(ctx, it.ID, p); err != nil {
			return err
		}
	}
	if err := s.Kratos.MarkLoginVerified(ctx, it.ID, p.String()); err != nil {
		return fmt.Errorf("mark verified: %w", err)
	}
	res.Reverified++
	return nil
}

func (s *LoginMigrationService) log() *slog.Logger {
	if s.Log != nil {
		return s.Log
	}
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
