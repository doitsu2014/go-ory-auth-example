//go:build integration

package postgres

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/audit"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/identity"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/profile"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/testutil/itest"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	env := itest.Load()
	ctx := context.Background()
	if err := MigrateUp(ctx, env.MigrateURL, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := Open(ctx, env.DatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return NewStore(pool)
}

func TestFR09_ProfileRepo(t *testing.T) {
	s := newStore(t)
	r := s.Repos().Profiles
	ctx := context.Background()
	id := uuid.New()
	if _, err := r.Get(ctx, id); err != profile.ErrNotFound {
		t.Fatalf("want not found, got %v", err)
	}
	for range 2 {
		if err := r.Ensure(ctx, id, identity.KindCustomer); err != nil {
			t.Fatal(err)
		}
	}
	p, err := r.Get(ctx, id)
	if err != nil || p.Locale != "vi-VN" || p.Kind != identity.KindCustomer || p.DisplayName != nil {
		t.Fatalf("get: %+v %v", p, err)
	}
	dn := "An"
	p.DisplayName, p.Locale = &dn, "en-US"
	up, err := r.Update(ctx, p)
	if err != nil || *up.DisplayName != "An" || up.Locale != "en-US" || !up.UpdatedAt.After(p.CreatedAt.Add(-time.Second)) {
		t.Fatalf("update: %+v %v", up, err)
	}
	many, err := r.GetMany(ctx, []uuid.UUID{id, uuid.New()})
	if err != nil || len(many) != 1 {
		t.Fatalf("many: %v %v", many, err)
	}
	long := strings.Repeat("x", 101)
	p.DisplayName = &long
	if _, err := r.Update(ctx, p); err == nil {
		t.Fatal("CHECK constraint must reject display_name > 100")
	}
}

func TestFR12_AuditRepoKeysetAndAppendOnly(t *testing.T) {
	s := newStore(t)
	r := s.Repos().Audit
	ctx := context.Background()
	target := uuid.NewString()
	actor := uuid.New()
	for i := range 5 {
		if err := r.Append(ctx, audit.Event{ActorID: actor, Action: audit.ActionCustomerDisabled, TargetType: "customer",
			TargetID: target, RequestID: "req", Details: map[string]any{"n": i}}); err != nil {
			t.Fatal(err)
		}
	}
	tt := "customer"
	page, err := r.List(ctx, audit.Filter{TargetType: &tt, TargetID: &target, Limit: 3})
	if err != nil || len(page) != 3 || page[0].Details["n"].(float64) != 4 {
		t.Fatalf("page1: %+v %v", page, err)
	}
	last := page[2]
	page2, err := r.List(ctx, audit.Filter{TargetID: &target, ActorID: &actor, Limit: 3, After: &audit.Cursor{OccurredAt: last.OccurredAt, ID: last.ID}})
	if err != nil || len(page2) != 2 || page2[0].ID >= last.ID {
		t.Fatalf("page2: %+v %v", page2, err)
	}
	// identity_app has no UPDATE/DELETE on audit_event (append-only by grant).
	if _, err := s.pool.Exec(ctx, "DELETE FROM audit_event WHERE target_id = $1", target); err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("delete must be denied, got %v", err)
	}
	if _, err := s.pool.Exec(ctx, "UPDATE audit_event SET action = 'x' WHERE target_id = $1", target); err == nil {
		t.Fatal("update must be denied")
	}
}

func TestFR07_IdempotencyReservation(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	r := s.Repos().Idempotency
	actor := uuid.New()
	key := uuid.NewString()
	hash := []byte{1, 2}
	window := time.Now().Add(-24 * time.Hour)

	// Concurrent reservations of the same key: exactly one wins.
	var wins atomic.Int32
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, ok, err := r.Reserve(ctx, actor, key, hash, window); err == nil && ok {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()
	if wins.Load() != 1 {
		t.Fatalf("want 1 winner, got %d", wins.Load())
	}
	rec, ok, err := r.Reserve(ctx, actor, key, hash, window)
	if err != nil || ok || rec == nil || !rec.Pending() {
		t.Fatalf("pending: %+v %v %v", rec, ok, err)
	}
	err = s.WithinTx(ctx, func(ctx context.Context, rr app.Repos) error {
		return rr.Idempotency.Complete(ctx, actor, key, 201, []byte(`{"id":"x"}`))
	})
	if err != nil {
		t.Fatal(err)
	}
	rec, ok, _ = r.Reserve(ctx, actor, key, hash, window)
	if ok || rec.Pending() || rec.ResponseCode != 201 {
		t.Fatalf("completed: %+v", rec)
	}
	if err := r.Complete(ctx, actor, key, 201, []byte(`{}`)); err == nil {
		t.Fatal("completing a non-pending key must fail")
	}
	// An expired row is replaced by a new reservation.
	if _, ok, err := r.Reserve(ctx, actor, key, hash, time.Now().Add(time.Minute)); err != nil || !ok {
		t.Fatalf("expired key must be reusable: %v %v", ok, err)
	}
	if err := r.Release(ctx, actor, key); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := r.Reserve(ctx, actor, key, hash, window); !ok {
		t.Fatal("released key must be reservable")
	}
	if _, err := r.Purge(ctx, time.Now().Add(-24*time.Hour)); err != nil {
		t.Fatal(err)
	}
}

func TestFR12_AdvisoryLockSerialisesTransactions(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	key := time.Now().UnixNano()
	held := make(chan struct{})
	release := make(chan struct{})
	var order []string
	var mu sync.Mutex
	done := make(chan error, 2)
	go func() {
		done <- s.WithinTx(ctx, func(ctx context.Context, r app.Repos) error {
			if err := r.Locks.XactLock(ctx, key); err != nil {
				return err
			}
			close(held)
			<-release
			mu.Lock()
			order = append(order, "first")
			mu.Unlock()
			return nil
		})
	}()
	<-held
	go func() {
		done <- s.WithinTx(ctx, func(ctx context.Context, r app.Repos) error {
			if err := r.Locks.XactLock(ctx, key); err != nil {
				return err
			}
			mu.Lock()
			order = append(order, "second")
			mu.Unlock()
			return nil
		})
	}()
	time.Sleep(200 * time.Millisecond)
	close(release)
	for range 2 {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	if len(order) != 2 || order[0] != "first" {
		t.Fatalf("lock did not serialise: %v", order)
	}
}
