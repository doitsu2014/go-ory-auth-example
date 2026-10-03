package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/adapter/postgres/sqlcgen"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
)

// piiErr turns a database error on a PII table into a value-free error
// (§9 A16): server errors keep only the SQLSTATE and constraint name (never
// DETAIL, which can contain row values); FK/unique violations become
// app.ErrConflict.
func piiErr(op string, err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		if pg.Code == "23503" || pg.Code == "23505" {
			return fmt.Errorf("%w: %s: sqlstate %s (%s)", app.ErrConflict, op, pg.Code, pg.ConstraintName)
		}
		return fmt.Errorf("%s: sqlstate %s (%s)", op, pg.Code, pg.ConstraintName)
	}
	for _, ce := range []error{context.DeadlineExceeded, context.Canceled} {
		if errors.Is(err, ce) {
			return fmt.Errorf("%s: %w", op, ce)
		}
	}
	return fmt.Errorf("%s: database error", op)
}

// SubjectKeyRepo implements app.SubjectKeyRepo.
type SubjectKeyRepo struct{ q *sqlcgen.Queries }

func toSubjectKey(r sqlcgen.SubjectKey) app.SubjectKey {
	return app.SubjectKey{
		KeyID: r.KeyID, IdentityID: r.IdentityID,
		Wrapped:   app.Wrapped{Ciphertext: r.WrappedDek, KEKName: r.KekName, KEKVersion: int(r.KekVersion)},
		CreatedAt: r.CreatedAt, RewrappedAt: r.RewrappedAt,
	}
}

// Get implements app.SubjectKeyRepo.
func (r SubjectKeyRepo) Get(ctx context.Context, identityID uuid.UUID) (app.SubjectKey, error) {
	row, err := r.q.GetSubjectKey(ctx, identityID)
	if errors.Is(err, pgx.ErrNoRows) {
		return app.SubjectKey{}, app.ErrNotFound
	}
	if err != nil {
		return app.SubjectKey{}, piiErr("get subject key", err)
	}
	return toSubjectKey(row), nil
}

// Insert implements app.SubjectKeyRepo (ON CONFLICT DO NOTHING, then re-read).
func (r SubjectKeyRepo) Insert(ctx context.Context, k app.SubjectKey) (app.SubjectKey, bool, error) {
	row, err := r.q.InsertSubjectKey(ctx, sqlcgen.InsertSubjectKeyParams{
		KeyID: k.KeyID, IdentityID: k.IdentityID, WrappedDek: k.Wrapped.Ciphertext,
		KekName: k.Wrapped.KEKName, KekVersion: int32(k.Wrapped.KEKVersion),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		existing, err := r.Get(ctx, k.IdentityID)
		return existing, false, err
	}
	if err != nil {
		return app.SubjectKey{}, false, piiErr("insert subject key", err)
	}
	return toSubjectKey(row), true, nil
}

// Delete implements app.SubjectKeyRepo.
func (r SubjectKeyRepo) Delete(ctx context.Context, identityID uuid.UUID) ([]uuid.UUID, error) {
	ids, err := r.q.DeleteSubjectKey(ctx, identityID)
	if err != nil {
		return nil, piiErr("delete subject key", err)
	}
	return ids, nil
}

// ListForRewrap implements app.SubjectKeyRepo.
func (r SubjectKeyRepo) ListForRewrap(ctx context.Context, after uuid.UUID, limit int) ([]app.SubjectKey, error) {
	rows, err := r.q.ListSubjectKeysForRewrap(ctx, sqlcgen.ListSubjectKeysForRewrapParams{AfterKeyID: after, PageLimit: int32(limit)})
	if err != nil {
		return nil, piiErr("list subject keys", err)
	}
	out := make([]app.SubjectKey, len(rows))
	for i, row := range rows {
		out[i] = toSubjectKey(row)
	}
	return out, nil
}

// UpdateWrapped implements app.SubjectKeyRepo.
func (r SubjectKeyRepo) UpdateWrapped(ctx context.Context, keyID uuid.UUID, old string, w app.Wrapped) (bool, error) {
	n, err := r.q.UpdateWrappedDEK(ctx, sqlcgen.UpdateWrappedDEKParams{
		NewWrappedDek: w.Ciphertext, KekName: w.KEKName, KekVersion: int32(w.KEKVersion), KeyID: keyID, OldWrappedDek: old,
	})
	if err != nil {
		return false, piiErr("update wrapped key", err)
	}
	return n == 1, nil
}

// DeleteErased implements app.SubjectKeyRepo.
func (r SubjectKeyRepo) DeleteErased(ctx context.Context) (int64, error) {
	n, err := r.q.DeleteErasedSubjectKeys(ctx)
	if err != nil {
		return 0, piiErr("delete erased subject keys", err)
	}
	return n, nil
}

// CustomerPIIRepo implements app.CustomerPIIRepo.
type CustomerPIIRepo struct{ q *sqlcgen.Queries }

func toEncryptedPII(r sqlcgen.CustomerPii) app.EncryptedPII {
	rec := app.EncryptedPII{
		IdentityID: r.IdentityID, KeyID: r.KeyID, Phone: r.PhoneCt, DateOfBirth: r.DobCt,
		Address: r.AddressCt, NationalID: r.NationalIDCt, UpdatedAt: r.UpdatedAt,
	}
	if r.PhoneBidx != nil && r.BidxKeyVersion != nil {
		rec.PhoneBidx = &app.BlindIndex{Sum: r.PhoneBidx, KeyVersion: int(*r.BidxKeyVersion)}
	}
	return rec
}

// Get implements app.CustomerPIIRepo.
func (r CustomerPIIRepo) Get(ctx context.Context, identityID uuid.UUID) (app.EncryptedPII, error) {
	row, err := r.q.GetCustomerPII(ctx, identityID)
	if errors.Is(err, pgx.ErrNoRows) {
		return app.EncryptedPII{}, app.ErrNotFound
	}
	if err != nil {
		return app.EncryptedPII{}, piiErr("get personal info", err)
	}
	return toEncryptedPII(row), nil
}

// Upsert implements app.CustomerPIIRepo.
func (r CustomerPIIRepo) Upsert(ctx context.Context, rec app.EncryptedPII) (time.Time, error) {
	p := sqlcgen.UpsertCustomerPIIParams{
		IdentityID: rec.IdentityID, KeyID: rec.KeyID, PhoneCt: rec.Phone, DobCt: rec.DateOfBirth,
		AddressCt: rec.Address, NationalIDCt: rec.NationalID,
	}
	if rec.PhoneBidx != nil {
		v := int32(rec.PhoneBidx.KeyVersion)
		p.PhoneBidx, p.BidxKeyVersion = rec.PhoneBidx.Sum, &v
	}
	t, err := r.q.UpsertCustomerPII(ctx, p)
	if err != nil {
		return t, piiErr("upsert personal info", err)
	}
	return t, nil
}

// FindByPhoneBidx implements app.CustomerPIIRepo.
func (r CustomerPIIRepo) FindByPhoneBidx(ctx context.Context, bidx app.BlindIndex, limit int) ([]app.EncryptedPII, error) {
	v := int32(bidx.KeyVersion)
	rows, err := r.q.FindCustomerPIIByPhoneBidx(ctx, sqlcgen.FindCustomerPIIByPhoneBidxParams{
		PhoneBidx: bidx.Sum, BidxKeyVersion: &v, PageLimit: int32(limit),
	})
	if err != nil {
		return nil, piiErr("find by blind index", err)
	}
	out := make([]app.EncryptedPII, len(rows))
	for i, row := range rows {
		out[i] = toEncryptedPII(row)
	}
	return out, nil
}
