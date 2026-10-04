//go:build integration

package postgres

import (
	"context"
	"crypto/rand"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/login"
)

func randomPseudonym() login.Pseudonym {
	var p login.Pseudonym
	_, _ = rand.Read(p[:])
	return p
}

func TestPLIFR02_LoginIdentifierRepo(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	r := s.Repos().Logins
	p := randomPseudonym()
	if _, err := r.Get(ctx, p); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("want not found: %v", err)
	}
	rec := app.LoginRecord{Pseudonym: p, Kind: login.KindEmail, Ciphertext: "vault:v1:abc", KEKVersion: 1}
	for i, want := range []bool{true, false} {
		ins, err := r.InsertIfAbsent(ctx, rec)
		if err != nil || ins != want {
			t.Fatalf("insert %d: %v %v", i, ins, err)
		}
	}
	got, err := r.Get(ctx, p)
	if err != nil || got.IdentityID != nil || got.Kind != login.KindEmail || got.Ciphertext != "vault:v1:abc" {
		t.Fatalf("get: %+v %v", got, err)
	}

	// Bind: unbound → id; same id again ok; other id refused unless stale.
	id, other := uuid.New(), uuid.New()
	if ok, err := r.Bind(ctx, p, id, nil); err != nil || !ok {
		t.Fatalf("bind: %v %v", ok, err)
	}
	if ok, err := r.Bind(ctx, p, id, nil); err != nil || !ok {
		t.Fatalf("rebind same: %v %v", ok, err)
	}
	if ok, _ := r.Bind(ctx, p, other, nil); ok {
		t.Fatal("bound to another identity without a stale binding")
	}
	wrong := uuid.New()
	if ok, _ := r.Bind(ctx, p, other, &wrong); ok {
		t.Fatal("replaced a binding other than the stale one")
	}
	if ok, err := r.Bind(ctx, p, other, &id); err != nil || !ok {
		t.Fatalf("stale rebind: %v %v", ok, err)
	}
	if b, err := r.GetByIdentity(ctx, other); err != nil || b.Pseudonym != p || b.BoundAt == nil {
		t.Fatalf("by identity: %+v %v", b, err)
	}
	// An identity can be bound to one login only.
	p2 := randomPseudonym()
	_, _ = r.InsertIfAbsent(ctx, app.LoginRecord{Pseudonym: p2, Kind: login.KindPhone, Ciphertext: "vault:v1:x", KEKVersion: 1})
	if _, err := r.Bind(ctx, p2, other, nil); !errors.Is(err, app.ErrConflict) {
		t.Fatalf("second login for one identity: %v", err)
	}

	many, err := r.GetMany(ctx, []login.Pseudonym{p, p2, randomPseudonym()})
	if err != nil || len(many) != 2 {
		t.Fatalf("get many: %d %v", len(many), err)
	}
	if ok, _ := r.Delete(ctx, p, true); ok {
		t.Fatal("deleted a bound login with onlyIfUnbound")
	}
	if ok, err := r.UpdateCiphertext(ctx, p, "vault:v1:abc", "vault:v2:def", 2); err != nil || !ok {
		t.Fatalf("rewrap: %v %v", ok, err)
	}
	if ok, _ := r.UpdateCiphertext(ctx, p, "vault:v1:abc", "vault:v2:zzz", 2); ok {
		t.Fatal("stale rewrap applied")
	}
	if ok, err := r.DeleteForIdentity(ctx, other); err != nil || !ok {
		t.Fatalf("delete for identity: %v %v", ok, err)
	}
	if ok, err := r.Delete(ctx, p2, true); err != nil || !ok {
		t.Fatalf("delete unbound: %v %v", ok, err)
	}
}

func TestPLIA10_StaleUnboundDeleteLosesToValidation(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	r := s.Repos().Logins
	p := randomPseudonym()
	_, _ = r.InsertIfAbsent(ctx, app.LoginRecord{Pseudonym: p, Kind: login.KindEmail, Ciphertext: "vault:v1:a", KEKVersion: 1})
	defer func() { _, _ = r.Delete(ctx, p, false) }()
	before := time.Now().Add(time.Hour)
	recs, err := r.ListStaleUnbound(ctx, before, 10000)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, rec := range recs {
		found = found || rec.Pseudonym == p
	}
	if !found {
		t.Fatal("stale unbound row not listed")
	}
	// A registration validated the row after the purge listed it.
	if err := r.Touch(ctx, p); err != nil {
		t.Fatal(err)
	}
	if ok, _ := r.DeleteStaleUnbound(ctx, p, time.Now().Add(-time.Minute)); ok {
		t.Fatal("deleted a row validated after the cutoff")
	}
	if ok, err := r.DeleteStaleUnbound(ctx, p, before); err != nil || !ok {
		t.Fatalf("stale delete: %v %v", ok, err)
	}
}

func TestPLINFR10_CourierDispatchReservation(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	r := s.Repos().Dispatches
	var k [32]byte
	_, _ = rand.Read(k[:])
	now := time.Now()
	if ok, err := r.Reserve(ctx, k, now.Add(-2*time.Minute)); err != nil || !ok {
		t.Fatalf("reserve: %v %v", ok, err)
	}
	if ok, _ := r.Reserve(ctx, k, now.Add(-2*time.Minute)); ok {
		t.Fatal("fresh pending reservation retaken")
	}
	if ok, _ := r.Reserve(ctx, k, now.Add(time.Minute)); !ok {
		t.Fatal("stale pending reservation not retaken")
	}
	rk := [32]byte{7}
	if err := r.MarkSent(ctx, k, &app.Delivery{Channel: "sms", Country: "84", RecipientKey: rk}); err != nil {
		t.Fatal(err)
	}
	if n, err := r.CountDeliveriesTo(ctx, rk, now.Add(-time.Hour)); err != nil || n != 1 {
		t.Fatalf("count deliveries: %d %v", n, err)
	}
	if n, err := r.CountSMS(ctx, "84", now.Add(-time.Hour)); err != nil || n < 1 {
		t.Fatalf("count sms: %d %v", n, err)
	}
	if ok, _ := r.Reserve(ctx, k, now.Add(time.Hour)); ok {
		t.Fatal("sent key reserved again")
	}
	if err := r.Release(ctx, k); err != nil {
		t.Fatal(err)
	}
	if ok, _ := r.Reserve(ctx, k, now.Add(time.Hour)); ok {
		t.Fatal("release must not drop a sent key")
	}
	if n, err := r.Purge(ctx, now.Add(time.Hour)); err != nil || n < 1 {
		t.Fatalf("purge: %d %v", n, err)
	}
}
