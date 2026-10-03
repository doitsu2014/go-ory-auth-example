package app_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/audit"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/identity"
)

// eraseOnceTx deletes the subject key right before the first transaction,
// simulating an erase that commits between keyFor and the upsert.
type eraseOnceTx struct {
	next app.TxRunner
	keys app.SubjectKeyRepo
	id   uuid.UUID
	done atomic.Bool
}

func (e *eraseOnceTx) WithinTx(ctx context.Context, fn func(context.Context, app.Repos) error) error {
	if e.done.CompareAndSwap(false, true) {
		if _, err := e.keys.Delete(ctx, e.id); err != nil {
			return err
		}
	}
	return e.next.WithinTx(ctx, fn)
}

// Review #1: erase racing PUT (FK violation on upsert) → one retry, success.
func TestReview1_EraseBetweenKeyForAndUpsertRetries(t *testing.T) {
	p := newPIIEnv(t)
	ctx := context.Background()
	c := p.customer()
	if _, err := p.svc.PutMine(ctx, c, sample(tPhone, tLine1)); err != nil {
		t.Fatal(err)
	}
	first := p.store.Keys[c.Principal.IdentityID]
	svc := p.service(p.cache)
	svc.Tx = &eraseOnceTx{next: p.store, keys: p.store.Repos().SubjectKeys, id: c.Principal.IdentityID}
	v, err := svc.PutMine(ctx, c, sample(tPhoneB, tLine1B))
	if err != nil || *v.Info.Phone != tPhoneB {
		t.Fatalf("put racing erase must succeed after a retry: %v", err)
	}
	now := p.store.Keys[c.Principal.IdentityID]
	if now.KeyID == first.KeyID {
		t.Fatal("retry must create a new subject key")
	}
	if _, ok := p.cache.Get(first.KeyID); ok {
		t.Fatal("the erased key must not stay cached")
	}
	got, err := p.service(app.NewDEKCache(10, time.Minute, p.clock.Now)).GetMine(ctx, c.Principal)
	if err != nil || *got.Info.Phone != tPhoneB {
		t.Fatalf("readable: %v", err)
	}
	if !strings.Contains(p.logs.String(), "pii_put_retry_after_concurrent_erase") {
		t.Fatal("retry should be logged")
	}
}

// insertRaceKeys makes the first Insert look like "lost the race, and the
// winner was erased before the re-read" (ErrNotFound).
type insertRaceKeys struct {
	app.SubjectKeyRepo
	done atomic.Bool
}

func (k *insertRaceKeys) Insert(ctx context.Context, sk app.SubjectKey) (app.SubjectKey, bool, error) {
	if k.done.CompareAndSwap(false, true) {
		return app.SubjectKey{}, false, app.ErrNotFound
	}
	return k.SubjectKeyRepo.Insert(ctx, sk)
}

func TestReview1_InsertReReadNotFoundRetries(t *testing.T) {
	p := newPIIEnv(t)
	c := p.customer()
	svc := p.service(p.cache)
	svc.SubjectKeys = &insertRaceKeys{SubjectKeyRepo: p.store.Repos().SubjectKeys}
	if _, err := svc.PutMine(context.Background(), c, sample(tPhone, tLine1)); err != nil {
		t.Fatalf("must retry, got %v", err)
	}
	if len(p.store.Keys) != 1 || len(p.store.PII) != 1 {
		t.Fatal("stored after retry")
	}
}

// Review #2: a key evicted on erase cannot be re-cached by a slow reader.
func TestReview2_EvictTombstoneBlocksRecache(t *testing.T) {
	now := t0
	c := app.NewDEKCache(10, time.Minute, func() time.Time { return now })
	k := uuid.New()
	dek := bytes.Repeat([]byte{1}, 32)
	c.Put(k, dek)
	c.Evict(k)
	c.Put(k, dek) // concurrent reader finishing its unwrap after the erase
	if _, ok := c.Get(k); ok {
		t.Fatal("tombstoned key re-cached")
	}
	now = now.Add(2*time.Minute - time.Second)
	c.Put(k, dek)
	if _, ok := c.Get(k); ok {
		t.Fatal("tombstone must outlive the cache TTL")
	}
	now = now.Add(2 * time.Second)
	c.SweepExpired()
	c.Put(k, dek)
	if _, ok := c.Get(k); !ok {
		t.Fatal("tombstone expires")
	}
	other := uuid.New()
	c.Put(other, dek)
	if _, ok := c.Get(other); !ok {
		t.Fatal("other keys unaffected")
	}
	var nilCache *app.DEKCache
	if nilCache.Len() != 0 || nilCache.SweepExpired() != 0 {
		t.Fatal("nil receiver")
	}
}

// Review #3: the skip check compares the KEK name, not only the version.
func TestReview3_RewrapComparesKEKName(t *testing.T) {
	p := newPIIEnv(t)
	ctx := context.Background()
	for range 4 {
		if _, err := p.svc.PutMine(ctx, p.customer(), sample(tPhone, tLine1)); err != nil {
			t.Fatal(err)
		}
	}
	for id, k := range p.store.Keys { // same version, legacy KEK name
		k.Wrapped.KEKName = "legacy-kek"
		p.store.Keys[id] = k
	}
	rot := &app.KeyRotationService{Keys: p.kms, SubjectKeys: p.store.Repos().SubjectKeys}
	res, err := rot.Rewrap(ctx, 2)
	if err != nil || res.Rewrapped != 4 || res.Current != 0 || res.LatestKEKName != "local-kek" {
		t.Fatalf("every legacy-named key must be re-wrapped: %+v %v", res, err)
	}
	for _, k := range p.store.Keys {
		if k.Wrapped.KEKName != "local-kek" {
			t.Fatal("name not updated")
		}
	}
}

// eraseOnUpdate erases the subject right before UpdateWrapped.
type eraseOnUpdate struct{ app.SubjectKeyRepo }

func (e eraseOnUpdate) UpdateWrapped(ctx context.Context, keyID uuid.UUID, old string, w app.Wrapped) (bool, error) {
	page, _ := e.ListForRewrap(ctx, uuid.Nil, 100)
	for _, k := range page {
		if k.KeyID == keyID {
			_, _ = e.Delete(ctx, k.IdentityID)
		}
	}
	return e.SubjectKeyRepo.UpdateWrapped(ctx, keyID, old, w)
}

func TestReview3_RewrapLosingToEraseIsAConflict(t *testing.T) {
	p := newPIIEnv(t)
	ctx := context.Background()
	if _, err := p.svc.PutMine(ctx, p.customer(), sample(tPhone, tLine1)); err != nil {
		t.Fatal(err)
	}
	p.kms.Rotate()
	rot := &app.KeyRotationService{Keys: p.kms, SubjectKeys: eraseOnUpdate{p.store.Repos().SubjectKeys}}
	res, err := rot.Rewrap(ctx, 10)
	if err != nil || res.Conflicts != 1 || res.Rewrapped != 0 {
		t.Fatalf("erase during rewrap: %+v %v", res, err)
	}
	if len(p.store.Keys) != 0 {
		t.Fatal("erase must win")
	}
}

// Review #4 + #6: missing reason_code → required; reveal without a record
// is audited and returns nothing.
func TestReview4_6_RevealReasonRequiredAndEmptyRecord(t *testing.T) {
	p := newPIIEnv(t)
	ctx := context.Background()
	c := p.customer()
	admin := p.admin(identity.RoleAdmin)
	_, err := p.svc.Reveal(ctx, admin, c.Principal.IdentityID, app.RevealRequest{})
	var ve *app.ValidationError
	if !errors.As(err, &ve) || len(ve.Fields) != 1 || ve.Fields[0].Field != "reason_code" || ve.Fields[0].Code != "required" {
		t.Fatalf("missing reason_code: %+v", err)
	}
	v, err := p.svc.Reveal(ctx, admin, c.Principal.IdentityID, app.RevealRequest{ReasonCode: app.ReasonLegalRequest})
	if err != nil || !v.Info.IsZero() || v.UpdatedAt != nil {
		t.Fatalf("empty reveal: %v", err)
	}
	if ev := p.lastEvent(t); ev.Action != audit.ActionCustomerPIIRevealed || ev.TargetID != c.Principal.IdentityID.String() {
		t.Fatalf("empty reveal must be audited: %+v", ev)
	}
}

// Review #7 + #9: more than 20 candidates → first 20, truncated, overflow
// logged; the limiter is charged per candidate.
func TestReview7_9_LookupOverflowAndWeightedQuota(t *testing.T) {
	p := newPIIEnv(t)
	ctx := context.Background()
	for range 21 {
		if _, err := p.svc.PutMine(ctx, p.customer(), sample(tPhone, tLine1)); err != nil {
			t.Fatal(err)
		}
	}
	support := p.admin(identity.RoleSupport)
	res, err := p.svc.LookupByPhone(ctx, support, tPhone)
	if err != nil || len(res.Matches) != 20 || !res.Truncated {
		t.Fatalf("overflow: %d %v %v", len(res.Matches), res.Truncated, err)
	}
	if ev := p.lastEvent(t); ev.Details["truncated"] != true {
		t.Fatalf("audit truncated: %v", ev.Details)
	}
	if !strings.Contains(p.logs.String(), "pii_lookup_candidate_overflow") {
		t.Fatal("overflow must be logged")
	}
	// 20 charged; the minute quota (30) leaves room for 10, not 20.
	if _, err := p.svc.LookupByPhone(ctx, support, tPhone); !errors.Is(err, app.ErrRateLimited) {
		t.Fatalf("second 20-candidate lookup within a minute: %v", err)
	}
	for range 10 { // zero-candidate lookups cost 1 each
		if _, err := p.svc.LookupByPhone(ctx, support, "+15555550000"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := p.svc.LookupByPhone(ctx, support, "+15555550000"); !errors.Is(err, app.ErrRateLimited) {
		t.Fatalf("quota exhausted: %v", err)
	}
	assertNoPII(t, "logs", p.logs.String())
}

// Review #8: non-matching candidates only have their phone decrypted.
func TestReview8_LookupDecryptsOnlyPhoneForNonMatches(t *testing.T) {
	p := newPIIEnv(t)
	ctx := context.Background()
	a, b := p.customer(), p.customer()
	if _, err := p.svc.PutMine(ctx, a, sample(tPhone, tLine1)); err != nil {
		t.Fatal(err)
	}
	if _, err := p.svc.PutMine(ctx, b, sample(tPhoneB, tLine1B)); err != nil {
		t.Fatal(err)
	}
	rb := p.store.PII[b.Principal.IdentityID]
	rb.PhoneBidx = p.store.PII[a.Principal.IdentityID].PhoneBidx // B claims A's index
	rb.Address = append([]byte(nil), rb.Address...)
	rb.Address[len(rb.Address)-1] ^= 1 // would fail if decrypted
	p.store.PII[b.Principal.IdentityID] = rb
	res, err := p.svc.LookupByPhone(ctx, p.admin(identity.RoleSupport), tPhone)
	if err != nil || len(res.Matches) != 1 || res.Matches[0].IdentityID != a.Principal.IdentityID {
		t.Fatalf("lookup: %v", err)
	}
	if strings.Contains(p.logs.String(), "pii_decrypt_failed") {
		t.Fatal("non-matching candidate's other columns must not be decrypted")
	}
}

// Review #10: masked reads are limited per actor (300/min).
func TestReview10_MaskedQuota(t *testing.T) {
	p := newPIIEnv(t)
	ctx := context.Background()
	c := p.customer()
	support := p.admin(identity.RoleSupport)
	for i := range 300 {
		if _, err := p.svc.GetMasked(ctx, support, c.Principal.IdentityID); err != nil {
			t.Fatalf("masked %d: %v", i, err)
		}
	}
	if _, err := p.svc.GetMasked(ctx, support, c.Principal.IdentityID); !errors.Is(err, app.ErrRateLimited) {
		t.Fatalf("301st masked read: %v", err)
	}
}

// Review #13: the reveal quota is checked before the Kratos lookup.
func TestReview13_RevealQuotaBeforeKratos(t *testing.T) {
	p := newPIIEnv(t)
	ctx := context.Background()
	c := p.customer()
	admin := p.admin(identity.RoleAdmin)
	req := app.RevealRequest{ReasonCode: app.ReasonLegalRequest}
	for range 20 {
		if _, err := p.svc.Reveal(ctx, admin, c.Principal.IdentityID, req); err != nil {
			t.Fatal(err)
		}
	}
	p.ids.Err = app.ErrDependencyUnavailable // Kratos would fail if called
	if _, err := p.svc.Reveal(ctx, admin, c.Principal.IdentityID, req); !errors.Is(err, app.ErrRateLimited) {
		t.Fatalf("want rate limited before Kratos, got %v", err)
	}
}

func TestReview9_RateLimiterAllowNAndPeek(t *testing.T) {
	now := t0
	l := app.NewRateLimiter([]app.RateRule{{Limit: 5, Window: time.Minute}}, func() time.Time { return now })
	a := uuid.New()
	if !l.AllowN(a, 4) || l.AllowN(a, 2) || !l.Peek(a, 1) || !l.AllowN(a, 1) || l.Peek(a, 1) {
		t.Fatal("weighted accounting")
	}
}
