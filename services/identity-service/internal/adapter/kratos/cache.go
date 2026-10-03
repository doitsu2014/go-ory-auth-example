package kratos

import (
	"crypto/sha256"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/golang-lru/v2/expirable"

	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/app"
	"github.com/doitsu-technology/go-ory-auth-example/services/identity-service/internal/domain/identity"
)

// MaxCacheTTL is the upper bound for a cached session (accepted revocation
// lag, 06-security §6.7).
const MaxCacheTTL = 30 * time.Second

type cacheKey [sha256.Size]byte

type cacheEntry struct {
	principal identity.Principal
	deadline  time.Time
}

// SessionCache is an in-process expirable LRU of verified sessions. Keys are
// SHA-256 of the credential, so raw tokens/cookies are never held as keys.
// Only positive results are cached.
//
// Invalidation epochs close the revocation race: a whoami that started
// before Invalidate(id) must not repopulate the cache for id afterwards.
type SessionCache struct {
	lru *expirable.LRU[cacheKey, cacheEntry]
	now func() time.Time

	mu     sync.Mutex
	gen    uint64
	epochs map[uuid.UUID]epoch
}

type epoch struct {
	gen uint64
	at  time.Time
}

// epochRetention bounds how long an invalidation epoch is remembered; it
// only has to outlive one in-flight whoami call.
const epochRetention = time.Minute

// NewSessionCache creates a cache holding at most size sessions.
func NewSessionCache(size int, now func() time.Time) *SessionCache {
	if size <= 0 {
		size = 10_000
	}
	if now == nil {
		now = time.Now
	}
	return &SessionCache{
		lru: expirable.NewLRU[cacheKey, cacheEntry](size, nil, MaxCacheTTL), now: now,
		epochs: map[uuid.UUID]epoch{},
	}
}

func keyFor(cred app.Credential) cacheKey {
	h := sha256.New()
	h.Write([]byte{byte(cred.Kind)})
	h.Write([]byte(cred.Value))
	var k cacheKey
	copy(k[:], h.Sum(nil))
	return k
}

// Get returns a cached principal if present and not past its deadline.
func (c *SessionCache) Get(cred app.Credential) (identity.Principal, bool) {
	k := keyFor(cred)
	e, ok := c.lru.Get(k)
	if !ok {
		return identity.Principal{}, false
	}
	if !c.now().Before(e.deadline) {
		c.lru.Remove(k)
		return identity.Principal{}, false
	}
	return e.principal, true
}

// Generation returns the current invalidation generation. Take it before
// calling Kratos and pass it to PutIfFresh.
func (c *SessionCache) Generation() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.gen
}

// Put caches p unconditionally (see PutIfFresh).
func (c *SessionCache) Put(cred app.Credential, p identity.Principal) {
	c.PutIfFresh(cred, p, c.Generation())
}

// PutIfFresh caches p with TTL min(30s, expires_at-now), unless the identity
// was invalidated after startGen. Expired sessions are not cached.
func (c *SessionCache) PutIfFresh(cred app.Credential, p identity.Principal, startGen uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.epochs[p.IdentityID]; ok && e.gen > startGen {
		return
	}
	now := c.now()
	ttl := MaxCacheTTL
	if left := p.ExpiresAt.Sub(now); left < ttl {
		ttl = left
	}
	if ttl <= 0 {
		return
	}
	c.lru.Add(keyFor(cred), cacheEntry{principal: p, deadline: now.Add(ttl)})
}

// Invalidate removes every cached session of the identity.
func (c *SessionCache) Invalidate(identityID uuid.UUID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.gen++
	now := c.now()
	c.epochs[identityID] = epoch{gen: c.gen, at: now}
	if len(c.epochs) > 1024 {
		for id, e := range c.epochs {
			if now.Sub(e.at) > epochRetention {
				delete(c.epochs, id)
			}
		}
	}
	for _, k := range c.lru.Keys() {
		if e, ok := c.lru.Peek(k); ok && e.principal.IdentityID == identityID {
			c.lru.Remove(k)
		}
	}
}

// Len returns the number of cached entries.
func (c *SessionCache) Len() int { return c.lru.Len() }
