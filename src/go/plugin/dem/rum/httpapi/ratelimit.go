// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"sync"
	"time"
)

// limiter is a keyed token bucket: burst tokens per key, refilled at
// rate/sec. Keys idle long enough to be full again are swept when the
// map grows past maxKeys, which bounds memory under IP churn.
type limiter struct {
	mu      sync.Mutex
	rate    float64 // tokens per second
	burst   float64
	maxKeys int
	buckets map[string]*bucket
	// lastSweep amortizes sweeping: a table full of still-active buckets
	// is scanned at most once per sweepEvery, not once per new client.
	lastSweep time.Time
}

const sweepEvery = time.Second

type bucket struct {
	tokens float64
	last   time.Time
}

// take refills the bucket at rate tokens per second up to burst, then takes
// one token when a whole one is available.
func (b *bucket) take(rate, burst float64, now time.Time) bool {
	if elapsed := now.Sub(b.last).Seconds(); elapsed > 0 {
		b.tokens = min(burst, b.tokens+elapsed*rate)
		b.last = now
	}
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

func newLimiter(rate, burst float64, maxKeys int) *limiter {
	return &limiter{
		rate:    rate,
		burst:   burst,
		maxKeys: maxKeys,
		buckets: map[string]*bucket{},
	}
}

// allow takes one token for key at time now.
func (l *limiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	b, ok := l.buckets[key]
	if !ok {
		if len(l.buckets) >= l.maxKeys && now.Sub(l.lastSweep) >= sweepEvery {
			l.sweepLocked(now)
		}
		if len(l.buckets) >= l.maxKeys {
			return false
		}
		b = &bucket{
			tokens: l.burst,
			last:   now,
		}
		l.buckets[key] = b
	}
	return b.take(l.rate, l.burst, now)
}

// sweepLocked drops buckets that have fully refilled (idle for at least
// burst/rate seconds); they are indistinguishable from absent ones.
func (l *limiter) sweepLocked(now time.Time) {
	l.lastSweep = now
	idle := time.Duration(l.burst / l.rate * float64(time.Second))
	for k, b := range l.buckets {
		if now.Sub(b.last) >= idle {
			delete(l.buckets, k)
		}
	}
}
