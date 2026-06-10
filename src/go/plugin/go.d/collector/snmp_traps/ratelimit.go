// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"net/netip"
	"strings"
	"sync"
	"time"
)

type rateLimitMode int

const (
	rateLimitModeDrop   rateLimitMode = 0
	rateLimitModeSample rateLimitMode = 1

	defaultRateLimitPerSourcePPS = 1000
	defaultRateLimitMode         = "drop"
	defaultRateLimitMaxSources   = 10000
)

type rateLimiter struct {
	enabled    bool
	mode       rateLimitMode
	rate       float64
	burst      int
	maxSources int
	mu         sync.Mutex
	buckets    map[netip.Addr]*tokenBucket
	lastSweep  time.Time
}

type tokenBucket struct {
	tokens   float64
	lastFill time.Time
}

func newRateLimiter(enabled bool, perSourcePPS int, mode string) *rateLimiter {
	rl := &rateLimiter{
		enabled: enabled,
		buckets: make(map[netip.Addr]*tokenBucket),
	}
	if !enabled {
		return rl
	}
	if perSourcePPS <= 0 {
		perSourcePPS = defaultRateLimitPerSourcePPS
	}
	mode = normalizeRateLimitMode(mode)
	rl.enabled = true
	rl.rate = float64(perSourcePPS)
	rl.burst = perSourcePPS
	rl.maxSources = defaultRateLimitMaxSources
	switch mode {
	case "sample":
		rl.mode = rateLimitModeSample
	default:
		rl.mode = rateLimitModeDrop
	}
	return rl
}

func normalizeRateLimitMode(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		return defaultRateLimitMode
	}
	return mode
}

func (rl *rateLimiter) Allow(addr netip.Addr) (allowed bool, mode rateLimitMode) {
	if !rl.enabled {
		return true, rateLimitModeDrop
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	tb, ok := rl.buckets[addr]
	if !ok {
		if len(rl.buckets) >= rl.maxSources {
			rl.sweepIdleLocked(now)
			if len(rl.buckets) >= rl.maxSources {
				rl.evictOldestLocked()
			}
			if len(rl.buckets) >= rl.maxSources {
				return false, rl.mode
			}
		}
		tb = &tokenBucket{
			tokens:   float64(rl.burst),
			lastFill: now,
		}
		rl.buckets[addr] = tb
	} else {
		elapsed := now.Sub(tb.lastFill).Seconds()
		tb.tokens += elapsed * rl.rate
		if tb.tokens > float64(rl.burst) {
			tb.tokens = float64(rl.burst)
		}
		tb.lastFill = now
	}

	if tb.tokens >= 1.0 {
		tb.tokens -= 1.0
		return true, rl.mode
	}
	return false, rl.mode
}

func (rl *rateLimiter) maybeSweep(now time.Time) {
	if !rl.enabled {
		return
	}
	rl.mu.Lock()
	defer rl.mu.Unlock()
	if now.Sub(rl.lastSweep) < 5*time.Minute {
		return
	}
	rl.sweepIdleLocked(now)
	rl.lastSweep = now
}

func (rl *rateLimiter) sweepIdleLocked(now time.Time) {
	for addr, tb := range rl.buckets {
		if now.Sub(tb.lastFill) > 10*time.Minute {
			delete(rl.buckets, addr)
		}
	}
}

func (rl *rateLimiter) evictOldestLocked() {
	var oldestAddr netip.Addr
	var oldestTime time.Time
	for addr, tb := range rl.buckets {
		if oldestTime.IsZero() || tb.lastFill.Before(oldestTime) {
			oldestAddr = addr
			oldestTime = tb.lastFill
		}
	}
	if !oldestTime.IsZero() {
		delete(rl.buckets, oldestAddr)
	}
}
