// SPDX-License-Identifier: GPL-3.0-or-later

package receiver

import (
	"net/netip"
	"testing"
	"time"
)

func TestRateLimiterAllow(t *testing.T) {
	rl := newRateLimiter(true, 100, "drop")
	addr := netip.MustParseAddr("10.1.2.3")

	for range 100 {
		if allowed, _ := rl.allow(addr); !allowed {
			t.Fatal("token bucket exhausted too early")
		}
	}

	allowed, mode := rl.allow(addr)
	if allowed {
		t.Fatal("expected rate limiter to drop after bucket exhausted")
	}
	if mode != rateLimitModeDrop {
		t.Fatal("expected drop mode")
	}
}

func TestRateLimiterDefaults(t *testing.T) {
	if err := ValidateRateLimit(RateLimitConfig{Enabled: true}); err != nil {
		t.Fatalf("ValidateRateLimit should accept defaulted enabled config: %v", err)
	}
	rl := newRateLimiter(true, 0, "")
	if !rl.enabled {
		t.Fatal("expected enabled rate limiter")
	}
	if rl.burst != defaultRateLimitPPS {
		t.Fatalf("burst = %d, want %d", rl.burst, defaultRateLimitPPS)
	}
	if rl.mode != rateLimitModeDrop {
		t.Fatalf("mode = %v, want drop", rl.mode)
	}
}

func TestRateLimiterEvictsOldestTrackedSourceAtCapacity(t *testing.T) {
	rl := newRateLimiter(true, 100, "drop")
	rl.maxSources = 2

	first := netip.MustParseAddr("10.0.0.1")
	second := netip.MustParseAddr("10.0.0.2")
	third := netip.MustParseAddr("10.0.0.3")

	if allowed, _ := rl.allow(first); !allowed {
		t.Fatal("expected first source to be allowed")
	}
	if allowed, _ := rl.allow(second); !allowed {
		t.Fatal("expected second source to be allowed")
	}

	now := time.Now()
	rl.mu.Lock()
	rl.buckets[first].lastFill = now.Add(-time.Minute)
	rl.buckets[second].lastFill = now
	rl.mu.Unlock()

	allowed, mode := rl.allow(third)
	if !allowed {
		t.Fatal("expected new source above cap to evict the oldest tracked source")
	}
	if mode != rateLimitModeDrop {
		t.Fatalf("mode = %v, want drop", mode)
	}
	if got := len(rl.buckets); got != 2 {
		t.Fatalf("tracked sources = %d, want 2", got)
	}
	if _, ok := rl.buckets[first]; ok {
		t.Fatal("expected oldest tracked source to be evicted")
	}
	if _, ok := rl.buckets[second]; !ok {
		t.Fatal("expected second tracked source to remain")
	}
	if _, ok := rl.buckets[third]; !ok {
		t.Fatal("expected new tracked source to be admitted")
	}
}
