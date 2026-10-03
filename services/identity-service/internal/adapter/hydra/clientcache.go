package hydra

import (
	"sync"
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
)

// MaxClientCacheTTL is the upper bound of a cached client status (accepted
// revocation lag across replicas, §6 B6).
const MaxClientCacheTTL = 30 * time.Second

type statusEntry struct {
	status   clientStatus
	deadline time.Time
}

// statusCache caches positive and negative client-status results (§6 A16)
// and never serves an entry past its TTL. An invalidation generation stops
// a lookup that started before Invalidate from repopulating the entry.
type statusCache struct {
	lru *expirable.LRU[string, statusEntry]
	ttl time.Duration
	now func() time.Time

	mu  sync.Mutex
	gen uint64
}

func newStatusCache(size int, ttl time.Duration, now func() time.Time) *statusCache {
	if size <= 0 {
		size = 10_000
	}
	if ttl <= 0 || ttl > MaxClientCacheTTL {
		ttl = MaxClientCacheTTL
	}
	return &statusCache{lru: expirable.NewLRU[string, statusEntry](size, nil, ttl), ttl: ttl, now: now}
}

func (c *statusCache) get(id string) (clientStatus, bool) {
	e, ok := c.lru.Get(id)
	if !ok {
		return clientStatus{}, false
	}
	if !c.now().Before(e.deadline) {
		c.lru.Remove(id)
		return clientStatus{}, false
	}
	return e.status, true
}

func (c *statusCache) generation() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.gen
}

// putIfFresh stores s unless an invalidation happened after startGen.
func (c *statusCache) putIfFresh(id string, s clientStatus, startGen uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.gen != startGen {
		return
	}
	c.lru.Add(id, statusEntry{status: s, deadline: c.now().Add(c.ttl)})
}

func (c *statusCache) invalidate(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.gen++
	c.lru.Remove(id)
}
