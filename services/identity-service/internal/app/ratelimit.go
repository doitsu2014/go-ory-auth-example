package app

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

// RateRule allows Limit events per Window.
type RateRule struct {
	Limit  int
	Window time.Duration
}

// Rate limits for personal info access per actor (§9 B3, A13).
var (
	LookupRateRules = []RateRule{{Limit: 30, Window: time.Minute}, {Limit: 200, Window: 24 * time.Hour}}
	RevealRateRules = []RateRule{{Limit: 20, Window: time.Hour}}
	MaskedRateRules = []RateRule{{Limit: 300, Window: time.Minute}}
)

// RateLimiter is an in-memory, per-replica sliding-log limiter keyed by
// actor. All rules must allow an event for it to be recorded.
type RateLimiter struct {
	mu     sync.Mutex
	rules  []RateRule
	maxWin time.Duration
	now    func() time.Time
	hits   map[uuid.UUID][]time.Time
	calls  int
}

// NewRateLimiter creates a limiter. now defaults to time.Now.
func NewRateLimiter(rules []RateRule, now func() time.Time) *RateLimiter {
	if now == nil {
		now = time.Now
	}
	var maxWin time.Duration
	for _, r := range rules {
		maxWin = max(maxWin, r.Window)
	}
	return &RateLimiter{rules: rules, maxWin: maxWin, now: now, hits: map[uuid.UUID][]time.Time{}}
}

// Allow records one event for actor and reports whether it is within every
// rule. A nil limiter allows everything.
func (l *RateLimiter) Allow(actor uuid.UUID) bool { return l.AllowN(actor, 1) }

// AllowN records n events (a weighted request) if all of them fit within
// every rule; otherwise it records nothing and returns false.
func (l *RateLimiter) AllowN(actor uuid.UUID, n int) bool { return l.take(actor, n, true) }

// Peek reports whether n events would currently be allowed, without
// recording them.
func (l *RateLimiter) Peek(actor uuid.UUID, n int) bool { return l.take(actor, n, false) }

func (l *RateLimiter) take(actor uuid.UUID, n int, record bool) bool {
	if l == nil {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.calls++
	if l.calls%1024 == 0 {
		l.sweep(now)
	}
	hs := prune(l.hits[actor], now.Add(-l.maxWin))
	for _, r := range l.rules {
		cutoff := now.Add(-r.Window)
		used := 0
		for i := len(hs) - 1; i >= 0 && hs[i].After(cutoff); i-- {
			used++
		}
		if used+n > r.Limit {
			l.hits[actor] = hs
			return false
		}
	}
	if record {
		for range n {
			hs = append(hs, now)
		}
	}
	l.hits[actor] = hs
	return true
}

func prune(hs []time.Time, cutoff time.Time) []time.Time {
	i := 0
	for i < len(hs) && !hs[i].After(cutoff) {
		i++
	}
	return hs[i:]
}

func (l *RateLimiter) sweep(now time.Time) {
	cutoff := now.Add(-l.maxWin)
	for k, hs := range l.hits {
		if hs = prune(hs, cutoff); len(hs) == 0 {
			delete(l.hits, k)
		} else {
			l.hits[k] = hs
		}
	}
}
