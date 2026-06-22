// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type reverseDNSTestClock struct {
	mu  sync.Mutex
	now time.Time
}

func newReverseDNSTestClock() *reverseDNSTestClock {
	return &reverseDNSTestClock{now: time.Date(2026, time.June, 22, 12, 0, 0, 0, time.UTC)}
}

func (c *reverseDNSTestClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *reverseDNSTestClock) Add(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

func TestTopologyReverseDNSDefaultConfig(t *testing.T) {
	config := normalizeTopologyReverseDNSConfig(topologyReverseDNSConfig{})

	require.Equal(t, 500*time.Millisecond, config.timeout)
	require.Equal(t, 24*time.Hour, config.positiveTTL)
	require.Equal(t, 24*time.Hour, config.negativeTTL)
	require.Equal(t, 1024, config.maxCandidates)
	require.Equal(t, 4, config.concurrency)
	require.NotNil(t, config.lookup)
	require.NotNil(t, config.now)
}

func TestNormalizeTopologyReverseDNSCandidateIP(t *testing.T) {
	tests := map[string]struct {
		in     string
		want   string
		wantOK bool
	}{
		"ipv4":                 {in: " 192.0.2.10 ", want: "192.0.2.10", wantOK: true},
		"ipv6":                 {in: "2001:db8::10", want: "2001:db8::10", wantOK: true},
		"ipv6-mapped ipv4":     {in: "::ffff:192.0.2.10", want: "192.0.2.10", wantOK: true},
		"private ipv4 kept":    {in: "10.0.0.10", want: "10.0.0.10", wantOK: true},
		"invalid":              {in: "not-an-ip"},
		"unspecified ipv4":     {in: "0.0.0.0"},
		"unspecified ipv6":     {in: "::"},
		"loopback":             {in: "127.0.0.1"},
		"link local":           {in: "169.254.1.1"},
		"multicast":            {in: "224.0.0.1"},
		"ipv4 broadcast":       {in: "255.255.255.255"},
		"link local multicast": {in: "ff02::1"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, ok := normalizeTopologyReverseDNSCandidateIP(tc.in)
			require.Equal(t, tc.wantOK, ok)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestTopologyReverseDNSResolverWarmCachesNormalizedPositiveResult(t *testing.T) {
	clock := newReverseDNSTestClock()
	var calls []string
	resolver := newTopologyReverseDNSResolverWithConfig(topologyReverseDNSConfig{
		now:         clock.Now,
		positiveTTL: time.Hour,
		negativeTTL: time.Minute,
		concurrency: 1,
		lookup: func(_ context.Context, ip string) ([]string, error) {
			calls = append(calls, ip)
			return []string{"Switch-A.Example.Test.", "switch-a.example.test."}, nil
		},
	})

	resolver.warm(context.Background(), []string{"::ffff:192.0.2.10", "127.0.0.1", "192.0.2.10"})

	require.Equal(t, []string{"192.0.2.10"}, calls)
	require.Equal(t, "switch-a.example.test", resolver.lookupCached("192.0.2.10"))
	require.Equal(t, "switch-a.example.test", resolver.lookupCached("::ffff:192.0.2.10"))
}

func TestNormalizeTopologyReverseDNSName(t *testing.T) {
	tests := map[string]struct {
		in   []string
		want string
	}{
		"empty":               {},
		"blank names":         {in: []string{"", " ", "."}},
		"trim lower suffix":   {in: []string{" Switch-A.Example.Test. "}, want: "switch-a.example.test"},
		"dedupe":              {in: []string{"switch-a.example.test.", "SWITCH-A.EXAMPLE.TEST"}, want: "switch-a.example.test"},
		"deterministic first": {in: []string{"switch-b.example.test.", "switch-a.example.test."}, want: "switch-a.example.test"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, normalizeTopologyReverseDNSName(tc.in))
		})
	}
}

func TestTopologyReverseDNSResolverNegativeCacheSuppressesRetriesUntilTTL(t *testing.T) {
	clock := newReverseDNSTestClock()
	var calls atomic.Int64
	resolver := newTopologyReverseDNSResolverWithConfig(topologyReverseDNSConfig{
		now:         clock.Now,
		positiveTTL: time.Hour,
		negativeTTL: time.Minute,
		concurrency: 1,
		lookup: func(context.Context, string) ([]string, error) {
			calls.Add(1)
			return nil, errors.New("not found")
		},
	})

	resolver.warm(context.Background(), []string{"192.0.2.10"})
	resolver.warm(context.Background(), []string{"192.0.2.10"})
	require.Equal(t, int64(1), calls.Load())
	require.Empty(t, resolver.lookupCached("192.0.2.10"))

	clock.Add(time.Minute + time.Second)
	resolver.warm(context.Background(), []string{"192.0.2.10"})
	require.Equal(t, int64(2), calls.Load())
}

func TestTopologyReverseDNSResolverPositiveCacheSuppressesRetriesUntilTTL(t *testing.T) {
	clock := newReverseDNSTestClock()
	var calls atomic.Int64
	resolver := newTopologyReverseDNSResolverWithConfig(topologyReverseDNSConfig{
		now:         clock.Now,
		positiveTTL: time.Minute,
		negativeTTL: time.Hour,
		concurrency: 1,
		lookup: func(context.Context, string) ([]string, error) {
			call := calls.Add(1)
			if call == 1 {
				return []string{"switch-a.example.test"}, nil
			}
			return []string{"switch-b.example.test"}, nil
		},
	})

	resolver.warm(context.Background(), []string{"192.0.2.10"})
	resolver.warm(context.Background(), []string{"192.0.2.10"})
	require.Equal(t, int64(1), calls.Load())
	require.Equal(t, "switch-a.example.test", resolver.lookupCached("192.0.2.10"))

	clock.Add(time.Minute + time.Second)
	resolver.warm(context.Background(), []string{"192.0.2.10"})
	require.Equal(t, int64(2), calls.Load())
	require.Equal(t, "switch-b.example.test", resolver.lookupCached("192.0.2.10"))
}

func TestTopologyReverseDNSCandidateCollectorRecordsCandidatesAndUsesOnlyCache(t *testing.T) {
	clock := newReverseDNSTestClock()
	var liveCalls atomic.Int64
	resolver := newTopologyReverseDNSResolverWithConfig(topologyReverseDNSConfig{
		now: clock.Now,
		lookup: func(context.Context, string) ([]string, error) {
			liveCalls.Add(1)
			return []string{"unexpected.example.test"}, nil
		},
	})
	resolver.store("192.0.2.10", "switch-a.example.test", clock.Now().Add(time.Hour))
	collector := resolver.newCandidateCollector()

	require.Equal(t, "switch-a.example.test", collector.lookupCached("192.0.2.10"))
	require.Empty(t, collector.lookupCached("192.0.2.11"))
	require.Empty(t, collector.lookupCached("127.0.0.1"))

	require.Equal(t, []string{"192.0.2.10", "192.0.2.11"}, collector.collectedCandidates())
	require.Zero(t, liveCalls.Load())
}

func TestTopologyReverseDNSResolverWarmStopsOnContextCancel(t *testing.T) {
	clock := newReverseDNSTestClock()
	started := make(chan struct{}, 3)
	var calls atomic.Int64
	resolver := newTopologyReverseDNSResolverWithConfig(topologyReverseDNSConfig{
		now:         clock.Now,
		timeout:     time.Minute,
		concurrency: 2,
		lookup: func(ctx context.Context, _ string) ([]string, error) {
			calls.Add(1)
			started <- struct{}{}
			<-ctx.Done()
			return nil, ctx.Err()
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		resolver.warm(ctx, []string{"192.0.2.10", "192.0.2.11", "192.0.2.12"})
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		require.FailNow(t, "warm lookup did not start")
	}
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		require.Fail(t, "warm did not stop after context cancellation")
	}
	require.LessOrEqual(t, calls.Load(), int64(2))
}

func TestTopologyReverseDNSResolverWarmHonorsConcurrencyLimit(t *testing.T) {
	clock := newReverseDNSTestClock()
	var calls atomic.Int64
	var inFlight atomic.Int64
	var maxInFlight atomic.Int64
	resolver := newTopologyReverseDNSResolverWithConfig(topologyReverseDNSConfig{
		now:         clock.Now,
		timeout:     time.Second,
		concurrency: 3,
		lookup: func(context.Context, string) ([]string, error) {
			calls.Add(1)
			current := inFlight.Add(1)
			for {
				max := maxInFlight.Load()
				if current <= max || maxInFlight.CompareAndSwap(max, current) {
					break
				}
			}
			time.Sleep(10 * time.Millisecond)
			inFlight.Add(-1)
			return []string{"host.example.test"}, nil
		},
	})

	resolver.warm(context.Background(), []string{
		"192.0.2.1", "192.0.2.2", "192.0.2.3", "192.0.2.4", "192.0.2.5",
		"192.0.2.6", "192.0.2.7", "192.0.2.8", "192.0.2.9", "192.0.2.10",
	})

	require.Equal(t, int64(10), calls.Load())
	require.LessOrEqual(t, maxInFlight.Load(), int64(3))
}

func TestTopologyReverseDNSResolverWarmHonorsCandidateLimit(t *testing.T) {
	clock := newReverseDNSTestClock()
	var calls atomic.Int64
	resolver := newTopologyReverseDNSResolverWithConfig(topologyReverseDNSConfig{
		now:           clock.Now,
		timeout:       time.Second,
		maxCandidates: 3,
		concurrency:   1,
		lookup: func(context.Context, string) ([]string, error) {
			calls.Add(1)
			return []string{"host.example.test"}, nil
		},
	})

	resolver.warm(context.Background(), []string{
		"192.0.2.1", "192.0.2.2", "192.0.2.3", "192.0.2.4", "192.0.2.5",
	})

	require.Equal(t, int64(3), calls.Load())
}

func TestTopologyReverseDNSResolverWarmAsyncDoesNotOverlap(t *testing.T) {
	clock := newReverseDNSTestClock()
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int64
	resolver := newTopologyReverseDNSResolverWithConfig(topologyReverseDNSConfig{
		now:         clock.Now,
		timeout:     time.Second,
		concurrency: 1,
		lookup: func(ctx context.Context, ip string) ([]string, error) {
			calls.Add(1)
			close(started)
			select {
			case <-release:
				return []string{ip + ".example.test"}, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		},
	})

	require.True(t, resolver.warmAsync(context.Background(), []string{"192.0.2.10"}))
	<-started
	require.False(t, resolver.warmAsync(context.Background(), []string{"192.0.2.11"}))
	close(release)

	require.Eventually(t, func() bool { return !resolver.warming.Load() }, time.Second, 10*time.Millisecond)
	require.Equal(t, int64(1), calls.Load())
}
