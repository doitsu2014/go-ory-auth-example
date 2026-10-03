//go:build integration

package postgres

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/audit"
)

func newKey(id uuid.UUID) app.SubjectKey {
	return app.SubjectKey{KeyID: uuid.New(), IdentityID: id, Wrapped: app.Wrapped{Ciphertext: "vault:v1:" + uuid.NewString(), KEKName: "identity-pii-kek", KEKVersion: 1}}
}

func TestPII_SubjectKeyAndRecordRepos(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	r := s.Repos()
	id := uuid.New()
	if _, err := r.SubjectKeys.Get(ctx, id); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("want not found: %v", err)
	}
	k := newKey(id)
	got, inserted, err := r.SubjectKeys.Insert(ctx, k)
	if err != nil || !inserted || got.KeyID != k.KeyID || got.CreatedAt.IsZero() {
		t.Fatalf("insert: %+v %v %v", got, inserted, err)
	}
	// Concurrent first write: the loser gets the winner's key.
	got2, inserted, err := r.SubjectKeys.Insert(ctx, newKey(id))
	if err != nil || inserted || got2.KeyID != k.KeyID {
		t.Fatalf("second insert: %v %v", inserted, err)
	}

	bidx := app.BlindIndex{Sum: bytes.Repeat([]byte{0x11}, 32), KeyVersion: 1}
	rec := app.EncryptedPII{IdentityID: id, KeyID: k.KeyID, Phone: []byte{1, 2, 3}, PhoneBidx: &bidx, Address: []byte{4, 5}}
	t1, err := r.PersonalInfo.Upsert(ctx, rec)
	if err != nil {
		t.Fatal(err)
	}
	back, err := r.PersonalInfo.Get(ctx, id)
	if err != nil || !bytes.Equal(back.Phone, rec.Phone) || back.DateOfBirth != nil || back.NationalID != nil ||
		back.PhoneBidx == nil || back.PhoneBidx.KeyVersion != 1 || !back.UpdatedAt.Equal(t1) {
		t.Fatalf("get: %+v %v", back, err)
	}
	// Clearing the phone clears the blind index (CHECK constraint pairs them).
	rec.Phone, rec.PhoneBidx = nil, nil
	if _, err := r.PersonalInfo.Upsert(ctx, rec); err != nil {
		t.Fatal(err)
	}
	if back, _ := r.PersonalInfo.Get(ctx, id); back.Phone != nil || back.PhoneBidx != nil {
		t.Fatal("phone not cleared")
	}
	// The CHECK rejects a ciphertext without an index; the error is value-free.
	rec.Phone = []byte("plain-value-must-not-leak")
	_, err = r.PersonalInfo.Upsert(ctx, rec)
	if err == nil || strings.Contains(err.Error(), "plain-value") || !strings.Contains(err.Error(), "23514") {
		t.Fatalf("check violation: %v", err)
	}
	// §9 A9: composite FK — a record cannot point at another key.
	rec.Phone = nil
	rec.KeyID = uuid.New()
	if _, err := r.PersonalInfo.Upsert(ctx, rec); !errors.Is(err, app.ErrConflict) {
		t.Fatalf("composite FK: %v", err)
	}

	// Optimistic rewrap update.
	nw := app.Wrapped{Ciphertext: "vault:v2:new", KEKName: "identity-pii-kek", KEKVersion: 2}
	if ok, err := r.SubjectKeys.UpdateWrapped(ctx, k.KeyID, "vault:v1:stale", nw); err != nil || ok {
		t.Fatalf("stale old value must not update: %v %v", ok, err)
	}
	if ok, err := r.SubjectKeys.UpdateWrapped(ctx, k.KeyID, k.Wrapped.Ciphertext, nw); err != nil || !ok {
		t.Fatalf("update: %v %v", ok, err)
	}
	if cur, _ := r.SubjectKeys.Get(ctx, id); cur.Wrapped != nw || cur.RewrappedAt == nil {
		t.Fatalf("rewrapped: %+v", cur)
	}
	page, err := r.SubjectKeys.ListForRewrap(ctx, uuid.Nil, 1000)
	if err != nil || len(page) == 0 {
		t.Fatal(err)
	}
	for i := 1; i < len(page); i++ {
		if page[i-1].KeyID.String() >= page[i].KeyID.String() {
			t.Fatal("ListForRewrap must be ordered by key_id")
		}
	}

	// Crypto-shredding: deleting the key cascades to the record.
	ids, err := r.SubjectKeys.Delete(ctx, id)
	if err != nil || len(ids) != 1 || ids[0] != k.KeyID {
		t.Fatalf("delete: %v %v", ids, err)
	}
	if _, err := r.PersonalInfo.Get(ctx, id); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("record must cascade: %v", err)
	}
	if ids, err := r.SubjectKeys.Delete(ctx, id); err != nil || len(ids) != 0 {
		t.Fatalf("idempotent delete: %v %v", ids, err)
	}
}

// TestA17_AppRoleCannotDeleteRecordsDirectly: identity_app has no DELETE on
// customer_pii; rows only go via the FK cascade.
func TestA17_AppRoleCannotDeleteRecordsDirectly(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	_, err := s.pool.Exec(ctx, "DELETE FROM customer_pii WHERE identity_id = $1", uuid.New())
	var pg *pgconn.PgError
	if !errors.As(err, &pg) || pg.Code != "42501" {
		t.Fatalf("want permission denied (42501), got %v", err)
	}
}

func TestPII_FindByBlindIndexAndLimit(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	r := s.Repos()
	sum := bytes.Repeat([]byte{byte(time.Now().UnixNano())}, 31)
	sum = append(sum, 0x77)
	bidx := app.BlindIndex{Sum: sum, KeyVersion: 1}
	var ids []uuid.UUID
	for range 3 {
		id := uuid.New()
		ids = append(ids, id)
		k, _, err := r.SubjectKeys.Insert(ctx, newKey(id))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := r.PersonalInfo.Upsert(ctx, app.EncryptedPII{IdentityID: id, KeyID: k.KeyID, Phone: []byte{9}, PhoneBidx: &bidx}); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		for _, id := range ids {
			_, _ = r.SubjectKeys.Delete(context.Background(), id)
		}
	})
	got, err := r.PersonalInfo.FindByPhoneBidx(ctx, bidx, 2)
	if err != nil || len(got) != 2 {
		t.Fatalf("find: %d %v", len(got), err)
	}
	if got, _ := r.PersonalInfo.FindByPhoneBidx(ctx, app.BlindIndex{Sum: sum, KeyVersion: 2}, 20); len(got) != 0 {
		t.Fatal("key version must match")
	}
}

// TestB1_ErasureLedger: keys created before the latest erasure audit event
// are deleted again (after a restore); newer keys survive.
func TestB1_ErasureLedger(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	r := s.Repos()
	restored, fresh := uuid.New(), uuid.New()
	if _, _, err := r.SubjectKeys.Insert(ctx, newKey(restored)); err != nil {
		t.Fatal(err)
	}
	for _, id := range []uuid.UUID{restored, fresh} {
		if err := r.Audit.Append(ctx, audit.Event{ActorID: id, Action: audit.ActionCustomerPIIErased, TargetType: audit.TargetCustomer,
			TargetID: id.String(), RequestID: "itest", Details: map[string]any{}}); err != nil {
			t.Fatal(err)
		}
	}
	time.Sleep(10 * time.Millisecond)
	if _, _, err := r.SubjectKeys.Insert(ctx, newKey(fresh)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = r.SubjectKeys.Delete(context.Background(), fresh) })
	n, err := r.SubjectKeys.DeleteErased(ctx)
	if err != nil || n < 1 {
		t.Fatalf("reapply: %d %v", n, err)
	}
	if _, err := r.SubjectKeys.Get(ctx, restored); !errors.Is(err, app.ErrNotFound) {
		t.Fatal("restored key must be deleted")
	}
	if _, err := r.SubjectKeys.Get(ctx, fresh); err != nil {
		t.Fatalf("key created after the erasure must survive: %v", err)
	}
}

// Review #14 (migration 0004): identity_app may only UPDATE the data and
// key-wrapping columns, never the identity binding.
func TestReview14_ColumnLevelUpdateGrants(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	r := s.Repos()
	id := uuid.New()
	k, _, err := r.SubjectKeys.Insert(ctx, newKey(id))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = r.SubjectKeys.Delete(context.Background(), id) })
	if _, err := r.PersonalInfo.Upsert(ctx, app.EncryptedPII{IdentityID: id, KeyID: k.KeyID, Address: []byte{1}}); err != nil {
		t.Fatalf("upsert must still work: %v", err)
	}
	for _, q := range []string{
		"UPDATE customer_pii SET identity_id = gen_random_uuid() WHERE identity_id = $1",
		"UPDATE subject_key SET identity_id = gen_random_uuid() WHERE identity_id = $1",
		"UPDATE subject_key SET key_id = gen_random_uuid() WHERE identity_id = $1",
		"UPDATE subject_key SET created_at = now() WHERE identity_id = $1",
	} {
		_, err := s.pool.Exec(ctx, q, id)
		var pg *pgconn.PgError
		if !errors.As(err, &pg) || pg.Code != "42501" {
			t.Fatalf("%s: want 42501, got %v", q, err)
		}
	}
	for _, q := range []string{
		"UPDATE customer_pii SET dob_ct = NULL, updated_at = now() WHERE identity_id = $1",
		"UPDATE subject_key SET rewrapped_at = now() WHERE identity_id = $1",
	} {
		if _, err := s.pool.Exec(ctx, q, id); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
}
