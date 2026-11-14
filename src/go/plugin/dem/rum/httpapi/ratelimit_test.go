// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/config"
	"github.com/stretchr/testify/assert"
)

func TestIPLimitBoundsActiveKeys(t *testing.T) {
	now := time.Unix(1000, 0)
	limit := newLimiter(1, 2, 2)
	assert.True(t, limit.allow("first", now))
	assert.True(t, limit.allow("second", now))
	assert.False(t, limit.allow("third", now), "new clients cannot allocate beyond the IP tracking budget")
	assert.True(t, limit.allow("first", now), "admitted clients retain their remaining tokens")
	assert.False(t, limit.allow("first", now))
	assert.True(t, limit.allow("third", now.Add(3*time.Second)), "fully replenished idle buckets may be reclaimed")
}

func TestPerIPRateLimit(t *testing.T) {
	sink := newRecSink()
	s, ts := newTestServer(t, testCfg(), sink, nil)
	now := time.Unix(1_800_000_000, 0)
	s.now = func() time.Time { return now }
	origin := map[string]string{"Origin": "https://shop.example.com"}

	for i := 0; i < 5; i++ {
		if resp := post(t, ts, "/rum/shop/collect", faroBody, origin); resp.StatusCode != 202 {
			t.Fatalf("request %d: status %d", i, resp.StatusCode)
		}
	}
	resp := post(t, ts, "/rum/shop/collect", faroBody, origin)
	if resp.StatusCode != http.StatusTooManyRequests || resp.Header.Get("Retry-After") == "" {
		t.Fatalf("6th request: status %d retry-after %q", resp.StatusCode, resp.Header.Get("Retry-After"))
	}
	n, rejects := sink.snapshot()
	if n != 5 || rejects["rejected_rate"] != 1 {
		t.Fatalf("ingested %d rejects %v", n, rejects)
	}

	// 5/min refills one token every 12s.
	now = now.Add(12 * time.Second)
	if resp := post(t, ts, "/rum/shop/collect", faroBody, origin); resp.StatusCode != 202 {
		t.Fatalf("after refill: status %d", resp.StatusCode)
	}
	if resp := post(t, ts, "/rum/shop/collect", faroBody, origin); resp.StatusCode != 429 {
		t.Fatalf("second after refill: status %d", resp.StatusCode)
	}
}

func TestPerSiteRateLimit(t *testing.T) {
	cfg := testCfg()
	cfg.RateLimit = config.RateLimit{
		PerIPPerMin:   100000,
		PerSitePerSec: 3,
	}
	sink := newRecSink()
	s, ts := newTestServer(t, cfg, sink, nil)
	now := time.Unix(1_800_000_000, 0)
	s.now = func() time.Time { return now }
	// Distinct client IPs via trusted proxy so the per-IP bucket is not the limiter.
	for i := 0; i < 3; i++ {
		hdr := map[string]string{
			"Origin":          "https://shop.example.com",
			"X-Forwarded-For": "203.0.113." + string(rune('1'+i)),
		}
		if resp := post(t, ts, "/rum/shop/collect", faroBody, hdr); resp.StatusCode != 202 {
			t.Fatalf("request %d: status %d", i, resp.StatusCode)
		}
	}
	hdr := map[string]string{"Origin": "https://shop.example.com", "X-Forwarded-For": "203.0.113.9"}
	if resp := post(t, ts, "/rum/shop/collect", faroBody, hdr); resp.StatusCode != 429 {
		t.Fatalf("4th request: status %d want 429", resp.StatusCode)
	}
	_, rejects := sink.snapshot()
	if rejects["rejected_rate"] != 1 {
		t.Fatalf("rejects %v", rejects)
	}
}

func TestLimiterSweepBoundsMemory(t *testing.T) {
	l := newLimiter(1, 2, 10)
	now := time.Unix(0, 0)
	for i := 0; i < 10; i++ {
		l.allow("ip"+string(rune('a'+i)), now)
	}
	// Idle long enough for every bucket to refill: adding the 11th key sweeps them.
	now = now.Add(5 * time.Second)
	l.allow("fresh", now)
	if len(l.buckets) != 1 {
		t.Fatalf("buckets after sweep = %d want 1", len(l.buckets))
	}
}

// A full table of still-active buckets cannot be swept; scanning it again
// for every new client serialized ingest (one full scan per beacon from a
// new IP). Sweeps are amortized: at most one per second.
func TestLimiterSweepIsAmortizedWhenNothingIsIdle(t *testing.T) {
	l := newLimiter(2, 120, 100)
	now := time.Unix(0, 0)
	for i := 0; i < 100; i++ {
		l.allow(fmt.Sprintf("busy-%d", i), now)
	}
	for i := 0; i < 5000; i++ {
		l.allow(fmt.Sprintf("new-%d", i), now.Add(time.Duration(i)*time.Microsecond))
	}
	// Only the first new client swept; a later sweep would have moved lastSweep.
	if !l.lastSweep.Equal(now) {
		t.Fatalf("last sweep at %v for 5000 new clients within one second, want only the first at %v", l.lastSweep, now)
	}
	now = now.Add(2 * time.Second)
	l.allow("later", now)
	if !l.lastSweep.Equal(now) {
		t.Fatalf("a new client a second later must sweep again: last sweep at %v", l.lastSweep)
	}
}
