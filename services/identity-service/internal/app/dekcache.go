package app

import (
	"container/list"
	"sync"
	"time"

	"github.com/google/uuid"
)

// DEK cache defaults (technical spec §3).
const (
	DefaultDEKCacheTTL  = 5 * time.Minute
	DefaultDEKCacheSize = 10_000
)

// DEKCache is an in-process LRU of unwrapped data keys with a TTL (DD-5).
// It is keyed by key_id, not identity_id: a re-created subject key gets a
// new key_id, so a stale entry on another replica can never decrypt new
// data. Evicted, expired and replaced keys are zeroed. Evict leaves a
// tombstone for twice the TTL, so a concurrent reader that unwrapped the key
// before an erase cannot re-cache it afterwards. Get returns a copy
// the caller owns (and should zero), so an eviction never mutates a key in
// use.
type DEKCache struct {
	mu    sync.Mutex
	size  int
	ttl   time.Duration
	now   func() time.Time
	order *list.List // front = most recently used
	items map[uuid.UUID]*list.Element
	// tombstones: evicted (erased) key ids → until when Put drops them.
	tombstones map[uuid.UUID]time.Time
}

type dekEntry struct {
	keyID    uuid.UUID
	dek      []byte
	deadline time.Time
}

// NewDEKCache creates a cache. now defaults to time.Now.
func NewDEKCache(size int, ttl time.Duration, now func() time.Time) *DEKCache {
	if size <= 0 {
		size = DefaultDEKCacheSize
	}
	if ttl <= 0 {
		ttl = DefaultDEKCacheTTL
	}
	if now == nil {
		now = time.Now
	}
	return &DEKCache{
		size: size, ttl: ttl, now: now, order: list.New(), items: map[uuid.UUID]*list.Element{},
		tombstones: map[uuid.UUID]time.Time{},
	}
}

// Get returns a copy of the cached DEK if present and not expired.
func (c *DEKCache) Get(keyID uuid.UUID) ([]byte, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[keyID]
	if !ok {
		return nil, false
	}
	e := el.Value.(*dekEntry)
	if !c.now().Before(e.deadline) {
		c.remove(el)
		return nil, false
	}
	c.order.MoveToFront(el)
	return append([]byte(nil), e.dek...), true
}

// Put caches a copy of dek.
func (c *DEKCache) Put(keyID uuid.UUID, dek []byte) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if until, ok := c.tombstones[keyID]; ok {
		if c.now().Before(until) {
			return // erased: never re-cache
		}
		delete(c.tombstones, keyID)
	}
	if el, ok := c.items[keyID]; ok {
		c.remove(el)
	}
	e := &dekEntry{keyID: keyID, dek: append([]byte(nil), dek...), deadline: c.now().Add(c.ttl)}
	c.items[keyID] = c.order.PushFront(e)
	for c.order.Len() > c.size {
		c.remove(c.order.Back())
	}
}

// Evict removes and zeroes the key and tombstones its id (on erase).
func (c *DEKCache) Evict(keyID uuid.UUID) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[keyID]; ok {
		c.remove(el)
	}
	c.tombstones[keyID] = c.now().Add(2 * c.ttl)
}

// Purge zeroes and drops every entry and expired entries' memory.
func (c *DEKCache) Purge() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for c.order.Len() > 0 {
		c.remove(c.order.Back())
	}
}

// Len returns the number of entries (including not yet collected expired ones).
func (c *DEKCache) Len() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.order.Len()
}

// SweepExpired zeroes and drops expired entries; run periodically so keys do
// not linger in memory past their TTL when they are not read again.
func (c *DEKCache) SweepExpired() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	for id, until := range c.tombstones {
		if !now.Before(until) {
			delete(c.tombstones, id)
		}
	}
	n := 0
	for el := c.order.Back(); el != nil; {
		prev := el.Prev()
		if !now.Before(el.Value.(*dekEntry).deadline) {
			c.remove(el)
			n++
		}
		el = prev
	}
	return n
}

func (c *DEKCache) remove(el *list.Element) {
	e := el.Value.(*dekEntry)
	clear(e.dek)
	c.order.Remove(el)
	delete(c.items, e.keyID)
}
