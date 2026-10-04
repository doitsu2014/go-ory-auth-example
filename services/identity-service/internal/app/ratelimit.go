package app

import (
	"fmt"
	"strconv"
	"strings"
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

// KeyedLimiter is an in-memory, per-replica sliding-log limiter keyed by K
// (an actor id, a client IP bucket, a login pseudonym, …). All rules must
// allow an event for it to be recorded.
type KeyedLimiter[K comparable] struct {
	mu     sync.Mutex
	rules  []RateRule
	maxWin time.Duration
	now    func() time.Time
	hits   map[K][]time.Time
	calls  int
}

// RateLimiter is the limiter keyed by actor id.
type RateLimiter = KeyedLimiter[uuid.UUID]

// NewRateLimiter creates a limiter keyed by actor. now defaults to time.Now.
func NewRateLimiter(rules []RateRule, now func() time.Time) *RateLimiter {
	return NewKeyedLimiter[uuid.UUID](rules, now)
}

// NewKeyedLimiter creates a limiter. now defaults to time.Now.
func NewKeyedLimiter[K comparable](rules []RateRule, now func() time.Time) *KeyedLimiter[K] {
	if now == nil {
		now = time.Now
	}
	var maxWin time.Duration
	for _, r := range rules {
		maxWin = max(maxWin, r.Window)
	}
	return &KeyedLimiter[K]{rules: rules, maxWin: maxWin, now: now, hits: map[K][]time.Time{}}
}

// Allow records one event for actor and reports whether it is within every
// rule. A nil limiter allows everything.
func (l *KeyedLimiter[K]) Allow(actor K) bool { return l.AllowN(actor, 1) }

// AllowN records n events (a weighted request) if all of them fit within
// every rule; otherwise it records nothing and returns false.
func (l *KeyedLimiter[K]) AllowN(actor K, n int) bool { return l.take(actor, n, true) }

// Peek reports whether n events would currently be allowed, without
// recording them.
func (l *KeyedLimiter[K]) Peek(actor K, n int) bool { return l.take(actor, n, false) }

func (l *KeyedLimiter[K]) take(actor K, n int, record bool) bool {
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

func (l *KeyedLimiter[K]) sweep(now time.Time) {
	cutoff := now.Add(-l.maxWin)
	for k, hs := range l.hits {
		if hs = prune(hs, cutoff); len(hs) == 0 {
			delete(l.hits, k)
		} else {
			l.hits[k] = hs
		}
	}
}

// ParseRateRules parses "20/1m,200/24h" (limit/window, comma separated).
func ParseRateRules(s string) ([]RateRule, error) {
	var out []RateRule
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		ls, ws, ok := strings.Cut(part, "/")
		n, err := strconv.Atoi(ls)
		if !ok || err != nil || n <= 0 {
			return nil, fmt.Errorf("rate rule %q: invalid limit", part)
		}
		w, err := time.ParseDuration(ws)
		if err != nil || w <= 0 {
			return nil, fmt.Errorf("rate rule %q: invalid window", part)
		}
		out = append(out, RateRule{Limit: n, Window: w})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("rate rules: empty")
	}
	return out, nil
}
