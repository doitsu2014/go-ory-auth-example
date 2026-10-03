// Package postgres implements the repository ports with sqlc-generated
// queries over pgx/v5.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/adapter/postgres/sqlcgen"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/audit"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/identity"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/profile"
)

// Store is the database entry point.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore wraps a pool.
func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// Repos returns the non-transactional repositories.
func (s *Store) Repos() app.Repos { return reposFor(sqlcgen.New(s.pool)) }

// WithinTx implements app.TxRunner.
func (s *Store) WithinTx(ctx context.Context, fn func(ctx context.Context, r app.Repos) error) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		return fn(ctx, reposFor(sqlcgen.New(tx)))
	})
}

// Ping checks connectivity (readiness).
func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

func reposFor(q *sqlcgen.Queries) app.Repos {
	return app.Repos{
		Profiles: ProfileRepo{q}, Audit: AuditRepo{q}, Idempotency: IdempotencyRepo{q}, Locks: Locker{q},
		SubjectKeys: SubjectKeyRepo{q}, PersonalInfo: CustomerPIIRepo{q},
	}
}

// ProfileRepo implements app.ProfileRepo.
type ProfileRepo struct{ q *sqlcgen.Queries }

// Ensure implements app.ProfileRepo (INSERT … ON CONFLICT DO NOTHING).
func (r ProfileRepo) Ensure(ctx context.Context, id uuid.UUID, kind identity.Kind) error {
	return r.q.InsertProfileIfAbsent(ctx, sqlcgen.InsertProfileIfAbsentParams{IdentityID: id, Kind: string(kind)})
}

func toProfile(id uuid.UUID, kind string, dn, av *string, locale string, created, updated time.Time) profile.Profile {
	return profile.Profile{
		IdentityID: id, Kind: identity.Kind(kind), DisplayName: dn, AvatarURL: av,
		Locale: locale, CreatedAt: created, UpdatedAt: updated,
	}
}

// Get implements app.ProfileRepo.
func (r ProfileRepo) Get(ctx context.Context, id uuid.UUID) (profile.Profile, error) {
	row, err := r.q.GetProfile(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return profile.Profile{}, profile.ErrNotFound
	}
	if err != nil {
		return profile.Profile{}, err
	}
	return toProfile(row.IdentityID, row.Kind, row.DisplayName, row.AvatarUrl, row.Locale, row.CreatedAt, row.UpdatedAt), nil
}

// GetMany implements app.ProfileRepo.
func (r ProfileRepo) GetMany(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]profile.Profile, error) {
	rows, err := r.q.GetProfilesByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]profile.Profile, len(rows))
	for _, row := range rows {
		out[row.IdentityID] = toProfile(row.IdentityID, row.Kind, row.DisplayName, row.AvatarUrl, row.Locale, row.CreatedAt, row.UpdatedAt)
	}
	return out, nil
}

// Update implements app.ProfileRepo.
func (r ProfileRepo) Update(ctx context.Context, p profile.Profile) (profile.Profile, error) {
	row, err := r.q.UpdateProfile(ctx, sqlcgen.UpdateProfileParams{
		IdentityID: p.IdentityID, DisplayName: p.DisplayName, AvatarUrl: p.AvatarURL, Locale: p.Locale,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return profile.Profile{}, profile.ErrNotFound
	}
	if err != nil {
		return profile.Profile{}, err
	}
	return toProfile(row.IdentityID, row.Kind, row.DisplayName, row.AvatarUrl, row.Locale, row.CreatedAt, row.UpdatedAt), nil
}

// AuditRepo implements app.AuditRepo.
type AuditRepo struct{ q *sqlcgen.Queries }

// Append implements app.AuditRepo.
func (r AuditRepo) Append(ctx context.Context, e audit.Event) error {
	details := e.Details
	if details == nil {
		details = map[string]any{}
	}
	b, err := json.Marshal(details)
	if err != nil {
		return fmt.Errorf("encode details: %w", err)
	}
	_, err = r.q.InsertAuditEvent(ctx, sqlcgen.InsertAuditEventParams{
		ActorIdentityID: e.ActorID, Action: string(e.Action), TargetType: e.TargetType,
		TargetID: e.TargetID, RequestID: e.RequestID, ClientIp: e.ClientIP, Details: b,
	})
	return err
}

// List implements app.AuditRepo.
func (r AuditRepo) List(ctx context.Context, f audit.Filter) ([]audit.Event, error) {
	p := sqlcgen.ListAuditEventsParams{TargetType: f.TargetType, TargetID: f.TargetID, PageLimit: int32(f.Limit)}
	if f.ActorID != nil {
		p.ActorID = uuid.NullUUID{UUID: *f.ActorID, Valid: true}
	}
	if f.After != nil {
		t, id := f.After.OccurredAt, f.After.ID
		p.AfterOccurredAt, p.AfterID = &t, &id
	}
	if len(f.Actions) > 0 || len(f.ActionPrefixes) > 0 {
		// Non-nil empty slices: an allowlist that is set but empty matches nothing.
		p.Actions, p.ActionPatterns = make([]string, 0, len(f.Actions)), make([]string, 0, len(f.ActionPrefixes))
		for _, a := range f.Actions {
			p.Actions = append(p.Actions, string(a))
		}
		for _, pre := range f.ActionPrefixes {
			p.ActionPatterns = append(p.ActionPatterns, likeEscape(pre)+"%")
		}
	}
	rows, err := r.q.ListAuditEvents(ctx, p)
	if err != nil {
		return nil, err
	}
	out := make([]audit.Event, 0, len(rows))
	for _, row := range rows {
		ev := audit.Event{
			ID: row.ID, OccurredAt: row.OccurredAt, ActorID: row.ActorIdentityID,
			Action: audit.Action(row.Action), TargetType: row.TargetType, TargetID: row.TargetID,
			RequestID: row.RequestID, Details: map[string]any{},
		}
		if len(row.Details) > 0 {
			if err := json.Unmarshal(row.Details, &ev.Details); err != nil {
				return nil, fmt.Errorf("decode details: %w", err)
			}
		}
		out = append(out, ev)
	}
	return out, nil
}

// IdempotencyRepo implements app.IdempotencyRepo.
type IdempotencyRepo struct{ q *sqlcgen.Queries }

// Reserve implements app.IdempotencyRepo.
func (r IdempotencyRepo) Reserve(ctx context.Context, actor uuid.UUID, key string, hash []byte, notBefore time.Time) (*app.IdempotencyRecord, bool, error) {
	if err := r.q.DeleteExpiredIdempotencyKeyFor(ctx, sqlcgen.DeleteExpiredIdempotencyKeyForParams{
		ActorIdentityID: actor, Key: key, NotBefore: notBefore,
	}); err != nil {
		return nil, false, err
	}
	n, err := r.q.ReserveIdempotencyKey(ctx, sqlcgen.ReserveIdempotencyKeyParams{Key: key, ActorIdentityID: actor, RequestHash: hash})
	if err != nil {
		return nil, false, err
	}
	if n == 1 {
		return nil, true, nil
	}
	row, err := r.q.GetIdempotencyKey(ctx, sqlcgen.GetIdempotencyKeyParams{ActorIdentityID: actor, Key: key})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil // released concurrently; caller reports a conflict
	}
	if err != nil {
		return nil, false, err
	}
	return &app.IdempotencyRecord{RequestHash: row.RequestHash, ResponseCode: int(row.ResponseCode), ResponseBody: row.ResponseBody, CreatedAt: row.CreatedAt}, false, nil
}

// Complete implements app.IdempotencyRepo.
func (r IdempotencyRepo) Complete(ctx context.Context, actor uuid.UUID, key string, code int, body []byte) error {
	n, err := r.q.CompleteIdempotencyKey(ctx, sqlcgen.CompleteIdempotencyKeyParams{
		ActorIdentityID: actor, Key: key, ResponseCode: int32(code), ResponseBody: body,
	})
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("idempotency key %q is not pending", key)
	}
	return nil
}

// Release implements app.IdempotencyRepo.
func (r IdempotencyRepo) Release(ctx context.Context, actor uuid.UUID, key string) error {
	return r.q.DeleteIdempotencyKey(ctx, sqlcgen.DeleteIdempotencyKeyParams{ActorIdentityID: actor, Key: key})
}

// Purge implements app.IdempotencyRepo.
func (r IdempotencyRepo) Purge(ctx context.Context, before time.Time) (int64, error) {
	return r.q.DeleteExpiredIdempotencyKeys(ctx, before)
}

// Locker implements app.Locker with pg_advisory_xact_lock. Outside a
// transaction the lock is released immediately, so use it via WithinTx.
type Locker struct{ q *sqlcgen.Queries }

// XactLock implements app.Locker.
func (l Locker) XactLock(ctx context.Context, key int64) error { return l.q.AdvisoryXactLock(ctx, key) }

// likeEscape escapes LIKE metacharacters (backslash is the default escape).
func likeEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
