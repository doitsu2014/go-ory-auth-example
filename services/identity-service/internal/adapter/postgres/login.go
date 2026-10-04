package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/adapter/postgres/sqlcgen"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/login"
)

// LoginIdentifierRepo implements app.LoginIdentifierRepo. Errors are
// value-free (piiErr).
type LoginIdentifierRepo struct{ q *sqlcgen.Queries }

func toLoginRecord(r sqlcgen.LoginIdentifier) (app.LoginRecord, error) {
	if len(r.Pseudonym) != login.PseudonymLen || len(r.LookupKey) != login.PseudonymLen {
		return app.LoginRecord{}, app.ErrDataIntegrity
	}
	out := app.LoginRecord{
		Kind: login.Kind(r.Kind), Ciphertext: r.ValueCt, KEKVersion: int(r.KekVersion),
		BoundAt: r.BoundAt, LegacyVerified: r.LegacyVerified,
		CreatedAt: r.CreatedAt, LastValidatedAt: r.LastValidatedAt,
	}
	copy(out.Pseudonym[:], r.Pseudonym)
	copy(out.LookupKey[:], r.LookupKey)
	if r.IdentityID.Valid {
		id := r.IdentityID.UUID
		out.IdentityID = &id
	}
	return out, nil
}

func toLoginRecords(rows []sqlcgen.LoginIdentifier) ([]app.LoginRecord, error) {
	out := make([]app.LoginRecord, 0, len(rows))
	for _, r := range rows {
		rec, err := toLoginRecord(r)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, nil
}

func nullUUID(id *uuid.UUID) uuid.NullUUID {
	if id == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *id, Valid: true}
}

// InsertIfAbsent implements app.LoginIdentifierRepo.
func (r LoginIdentifierRepo) InsertIfAbsent(ctx context.Context, rec app.LoginRecord) (bool, error) {
	n, err := r.q.InsertLoginIdentifier(ctx, sqlcgen.InsertLoginIdentifierParams{
		Pseudonym: rec.Pseudonym[:], LookupKey: rec.LookupKey[:], Kind: string(rec.Kind), ValueCt: rec.Ciphertext,
		KekVersion: int32(rec.KEKVersion), IdentityID: nullUUID(rec.IdentityID), LegacyVerified: rec.LegacyVerified,
	})
	if err != nil {
		return false, piiErr("insert login identifier", err)
	}
	return n == 1, nil
}

// Get implements app.LoginIdentifierRepo.
func (r LoginIdentifierRepo) Get(ctx context.Context, p login.Pseudonym) (app.LoginRecord, error) {
	row, err := r.q.GetLoginIdentifier(ctx, p[:])
	if errors.Is(err, pgx.ErrNoRows) {
		return app.LoginRecord{}, app.ErrNotFound
	}
	if err != nil {
		return app.LoginRecord{}, piiErr("get login identifier", err)
	}
	return toLoginRecord(row)
}

// GetByLookupKey implements app.LoginIdentifierRepo.
func (r LoginIdentifierRepo) GetByLookupKey(ctx context.Context, k login.LookupKey) (app.LoginRecord, error) {
	row, err := r.q.GetLoginIdentifierByLookupKey(ctx, k[:])
	if errors.Is(err, pgx.ErrNoRows) {
		return app.LoginRecord{}, app.ErrNotFound
	}
	if err != nil {
		return app.LoginRecord{}, piiErr("get login identifier by lookup key", err)
	}
	return toLoginRecord(row)
}

// GetMany implements app.LoginIdentifierRepo.
func (r LoginIdentifierRepo) GetMany(ctx context.Context, ps []login.Pseudonym) (map[login.Pseudonym]app.LoginRecord, error) {
	out := map[login.Pseudonym]app.LoginRecord{}
	if len(ps) == 0 {
		return out, nil
	}
	keys := make([][]byte, len(ps))
	for i := range ps {
		keys[i] = ps[i][:]
	}
	rows, err := r.q.GetLoginIdentifiers(ctx, keys)
	if err != nil {
		return nil, piiErr("get login identifiers", err)
	}
	recs, err := toLoginRecords(rows)
	if err != nil {
		return nil, err
	}
	for _, rec := range recs {
		out[rec.Pseudonym] = rec
	}
	return out, nil
}

// GetByIdentity implements app.LoginIdentifierRepo.
func (r LoginIdentifierRepo) GetByIdentity(ctx context.Context, id uuid.UUID) (app.LoginRecord, error) {
	row, err := r.q.GetLoginIdentifierByIdentity(ctx, uuid.NullUUID{UUID: id, Valid: true})
	if errors.Is(err, pgx.ErrNoRows) {
		return app.LoginRecord{}, app.ErrNotFound
	}
	if err != nil {
		return app.LoginRecord{}, piiErr("get login identifier by identity", err)
	}
	return toLoginRecord(row)
}

// Bind implements app.LoginIdentifierRepo. A unique violation (the identity
// is bound to another login) is app.ErrConflict.
func (r LoginIdentifierRepo) Bind(ctx context.Context, p login.Pseudonym, id uuid.UUID, stale *uuid.UUID) (bool, error) {
	n, err := r.q.BindLoginIdentifier(ctx, sqlcgen.BindLoginIdentifierParams{
		IdentityID: uuid.NullUUID{UUID: id, Valid: true}, Pseudonym: p[:], StaleIdentity: nullUUID(stale),
	})
	if err != nil {
		return false, piiErr("bind login identifier", err)
	}
	return n == 1, nil
}

// SetLegacyVerified implements app.LoginIdentifierRepo.
func (r LoginIdentifierRepo) SetLegacyVerified(ctx context.Context, p login.Pseudonym, v bool) error {
	n, err := r.q.SetLoginLegacyVerified(ctx, sqlcgen.SetLoginLegacyVerifiedParams{LegacyVerified: v, Pseudonym: p[:]})
	if err != nil {
		return piiErr("set login legacy verified", err)
	}
	if n == 0 {
		return app.ErrNotFound
	}
	return nil
}

// Touch implements app.LoginIdentifierRepo.
func (r LoginIdentifierRepo) Touch(ctx context.Context, p login.Pseudonym) error {
	n, err := r.q.TouchLoginIdentifier(ctx, p[:])
	if err != nil {
		return piiErr("touch login identifier", err)
	}
	if n == 0 {
		return app.ErrNotFound
	}
	return nil
}

// ListStaleUnbound implements app.LoginIdentifierRepo.
func (r LoginIdentifierRepo) ListStaleUnbound(ctx context.Context, before time.Time, limit int) ([]app.LoginRecord, error) {
	rows, err := r.q.ListStaleUnboundLoginIdentifiers(ctx, sqlcgen.ListStaleUnboundLoginIdentifiersParams{Before: before, PageLimit: int32(limit)})
	if err != nil {
		return nil, piiErr("list stale unbound login identifiers", err)
	}
	return toLoginRecords(rows)
}

// DeleteStaleUnbound implements app.LoginIdentifierRepo.
func (r LoginIdentifierRepo) DeleteStaleUnbound(ctx context.Context, p login.Pseudonym, before time.Time) (bool, error) {
	n, err := r.q.DeleteStaleUnboundLoginIdentifier(ctx, sqlcgen.DeleteStaleUnboundLoginIdentifierParams{Pseudonym: p[:], Before: before})
	if err != nil {
		return false, piiErr("delete stale unbound login identifier", err)
	}
	return n == 1, nil
}

// ListBound implements app.LoginIdentifierRepo.
func (r LoginIdentifierRepo) ListBound(ctx context.Context, after login.Pseudonym, limit int) ([]app.LoginRecord, error) {
	rows, err := r.q.ListBoundLoginIdentifiers(ctx, sqlcgen.ListBoundLoginIdentifiersParams{After: after[:], PageLimit: int32(limit)})
	if err != nil {
		return nil, piiErr("list bound login identifiers", err)
	}
	return toLoginRecords(rows)
}

// ListAll implements app.LoginIdentifierRepo.
func (r LoginIdentifierRepo) ListAll(ctx context.Context, after login.Pseudonym, limit int) ([]app.LoginRecord, error) {
	rows, err := r.q.ListAllLoginIdentifiers(ctx, sqlcgen.ListAllLoginIdentifiersParams{After: after[:], PageLimit: int32(limit)})
	if err != nil {
		return nil, piiErr("list login identifiers", err)
	}
	return toLoginRecords(rows)
}

// Delete implements app.LoginIdentifierRepo.
func (r LoginIdentifierRepo) Delete(ctx context.Context, p login.Pseudonym, onlyIfUnbound bool) (bool, error) {
	n, err := r.q.DeleteLoginIdentifier(ctx, sqlcgen.DeleteLoginIdentifierParams{Pseudonym: p[:], OnlyIfUnbound: onlyIfUnbound})
	if err != nil {
		return false, piiErr("delete login identifier", err)
	}
	return n == 1, nil
}

// DeleteForIdentity implements app.LoginIdentifierRepo.
func (r LoginIdentifierRepo) DeleteForIdentity(ctx context.Context, id uuid.UUID) (bool, error) {
	n, err := r.q.DeleteLoginIdentifierForIdentity(ctx, uuid.NullUUID{UUID: id, Valid: true})
	if err != nil {
		return false, piiErr("delete login identifier for identity", err)
	}
	return n == 1, nil
}

// UpdateCiphertext implements app.LoginIdentifierRepo.
func (r LoginIdentifierRepo) UpdateCiphertext(ctx context.Context, p login.Pseudonym, old, ct string, v int) (bool, error) {
	n, err := r.q.UpdateLoginCiphertext(ctx, sqlcgen.UpdateLoginCiphertextParams{
		NewValueCt: ct, KekVersion: int32(v), Pseudonym: p[:], OldValueCt: old,
	})
	if err != nil {
		return false, piiErr("update login ciphertext", err)
	}
	return n == 1, nil
}

// CountUnbound implements app.LoginIdentifierRepo.
func (r LoginIdentifierRepo) CountUnbound(ctx context.Context) (int64, error) {
	n, err := r.q.CountUnboundLoginIdentifiers(ctx)
	if err != nil {
		return 0, piiErr("count unbound login identifiers", err)
	}
	return n, nil
}

// DeleteErased implements app.LoginIdentifierRepo.
func (r LoginIdentifierRepo) DeleteErased(ctx context.Context) (int64, error) {
	n, err := r.q.DeleteErasedLoginIdentifiers(ctx)
	if err != nil {
		return 0, piiErr("delete erased login identifiers", err)
	}
	return n, nil
}

// CourierDispatchRepo implements app.CourierDispatchRepo.
type CourierDispatchRepo struct{ q *sqlcgen.Queries }

// Reserve implements app.CourierDispatchRepo.
func (r CourierDispatchRepo) Reserve(ctx context.Context, key [32]byte, staleBefore time.Time) (bool, error) {
	n, err := r.q.ReserveCourierDispatch(ctx, sqlcgen.ReserveCourierDispatchParams{DedupeKey: key[:], StaleBefore: staleBefore})
	if err != nil {
		return false, piiErr("reserve courier dispatch", err)
	}
	return n == 1, nil
}

// MarkSent implements app.CourierDispatchRepo.
func (r CourierDispatchRepo) MarkSent(ctx context.Context, key [32]byte, d *app.Delivery) error {
	arg := sqlcgen.MarkCourierDispatchSentParams{DedupeKey: key[:]}
	if d != nil {
		ch, rk := d.Channel, d.RecipientKey
		arg.Channel, arg.RecipientKey = &ch, rk[:]
		if d.Country != "" {
			c := d.Country
			arg.Country = &c
		}
	}
	if _, err := r.q.MarkCourierDispatchSent(ctx, arg); err != nil {
		return piiErr("mark courier dispatch sent", err)
	}
	return nil
}

// CountDeliveriesTo implements app.CourierDispatchRepo.
func (r CourierDispatchRepo) CountDeliveriesTo(ctx context.Context, rk [32]byte, since time.Time) (int64, error) {
	n, err := r.q.CountCourierDeliveriesTo(ctx, sqlcgen.CountCourierDeliveriesToParams{RecipientKey: rk[:], Since: since})
	if err != nil {
		return 0, piiErr("count courier deliveries", err)
	}
	return n, nil
}

// CountSMS implements app.CourierDispatchRepo.
func (r CourierDispatchRepo) CountSMS(ctx context.Context, country string, since time.Time) (int64, error) {
	n, err := r.q.CountSMSDeliveries(ctx, sqlcgen.CountSMSDeliveriesParams{Since: since, Country: country})
	if err != nil {
		return 0, piiErr("count sms deliveries", err)
	}
	return n, nil
}

// Release implements app.CourierDispatchRepo.
func (r CourierDispatchRepo) Release(ctx context.Context, key [32]byte) error {
	if _, err := r.q.ReleaseCourierDispatch(ctx, key[:]); err != nil {
		return piiErr("release courier dispatch", err)
	}
	return nil
}

// Purge implements app.CourierDispatchRepo.
func (r CourierDispatchRepo) Purge(ctx context.Context, before time.Time) (int64, error) {
	n, err := r.q.PurgeCourierDispatch(ctx, before)
	if err != nil {
		return 0, piiErr("purge courier dispatch", err)
	}
	return n, nil
}
