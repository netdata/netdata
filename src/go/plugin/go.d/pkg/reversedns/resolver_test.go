// SPDX-License-Identifier: GPL-3.0-or-later

package reversedns

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net/netip"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type testClock struct {
	mu  sync.Mutex
	now time.Time
}

func newTestClock() *testClock {
	return &testClock{now: time.Date(2026, time.August, 3, 12, 0, 0, 0, time.UTC)}
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *testClock) Add(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

func TestNewUsesDefaults(t *testing.T) {
	r := New(Config{})

	require.Equal(t, DefaultLookupTimeout, r.lookupTimeout)
	require.Equal(t, DefaultPositiveTTL, r.positiveTTL)
	require.Equal(t, DefaultNegativeTTL, r.negativeTTL)
	require.Equal(t, DefaultMaxEntries, r.maxEntries)
	require.Equal(t, DefaultMaxConcurrent, r.maxConcurrent)
	require.Equal(t, 8_000, r.protectedLimit)
	require.NotNil(t, r.lookup)
	require.NotNil(t, r.now)
}

func TestCanonicalAddress(t *testing.T) {
	tests := map[string]struct {
		addr netip.Addr
		want netip.Addr
		ok   bool
	}{
		"invalid": {},
		"ipv4": {
			addr: netip.MustParseAddr("192.0.2.10"),
			want: netip.MustParseAddr("192.0.2.10"),
			ok:   true,
		},
		"mapped-ipv4": {
			addr: netip.MustParseAddr("::ffff:192.0.2.10"),
			want: netip.MustParseAddr("192.0.2.10"),
			ok:   true,
		},
		"ipv6": {
			addr: netip.MustParseAddr("2001:db8::10"),
			want: netip.MustParseAddr("2001:db8::10"),
			ok:   true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, ok := canonicalAddress(tc.addr)
			require.Equal(t, tc.ok, ok)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestNormalizePTRNames(t *testing.T) {
	tests := map[string]struct {
		names []string
		want  string
	}{
		"empty": {},
		"blank": {
			names: []string{"", " ", "."},
		},
		"normalize": {
			names: []string{" Switch-A.Example.Test. "},
			want:  "switch-a.example.test",
		},
		"deterministic": {
			names: []string{"switch-b.example.test.", "SWITCH-A.EXAMPLE.TEST", "switch-a.example.test."},
			want:  "switch-a.example.test",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, normalizePTRNames(tc.names))
		})
	}
}

func TestResolveCachesPositiveAndNegativeResults(t *testing.T) {
	clock := newTestClock()
	var calls atomic.Int64
	r := New(Config{
		Now:         clock.Now,
		PositiveTTL: time.Hour,
		NegativeTTL: time.Minute,
		Lookup: func(_ context.Context, address string) ([]string, error) {
			calls.Add(1)
			if address == "192.0.2.10" {
				return []string{"b.example.test.", "A.EXAMPLE.TEST."}, nil
			}
			return nil, errors.New("not found")
		},
	})

	positive := netip.MustParseAddr("192.0.2.10")
	result, err := r.Resolve(context.Background(), positive)
	require.NoError(t, err)
	require.Equal(t, Result{State: StatePositive, Name: "a.example.test"}, result)
	require.Equal(t, result, r.Lookup(netip.MustParseAddr("::ffff:192.0.2.10")))
	require.Equal(t, int64(1), calls.Load())

	negative := netip.MustParseAddr("192.0.2.11")
	result, err = r.Resolve(context.Background(), negative)
	require.NoError(t, err)
	require.Equal(t, Result{State: StateNegative}, result)
	require.Equal(t, Result{State: StateNegative}, r.Lookup(negative))
	require.Equal(t, int64(2), calls.Load())

	clock.Add(time.Minute)
	require.Equal(t, Result{State: StateMiss}, r.Lookup(negative))
	require.Equal(t, Result{State: StatePositive, Name: "a.example.test"}, r.Lookup(positive))

	clock.Add(59 * time.Minute)
	require.Equal(t, Result{State: StateMiss}, r.Lookup(positive))
}

func TestInvalidAndCancelledResolveDoNotCreateWork(t *testing.T) {
	var calls atomic.Int64
	r := New(Config{Lookup: func(context.Context, string) ([]string, error) {
		calls.Add(1)
		return nil, nil
	}})

	require.Equal(t, Result{State: StateMiss}, r.Lookup(netip.Addr{}))
	require.Equal(t, ScheduleInvalid, r.Schedule(netip.Addr{}))

	result, err := r.Resolve(context.Background(), netip.Addr{})
	require.ErrorIs(t, err, ErrInvalidAddress)
	require.Equal(t, Result{State: StateMiss}, result)

	result, err = r.Resolve(nil, netip.MustParseAddr("192.0.2.10"))
	require.ErrorIs(t, err, ErrNilContext)
	require.Equal(t, Result{State: StateMiss}, result)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err = r.Resolve(ctx, netip.MustParseAddr("192.0.2.10"))
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, Result{State: StateMiss}, result)

	require.Zero(t, calls.Load())
	requireResolverState(t, r, 0, 0, 0)
}

func TestScheduleStatesAndNoWorkBranches(t *testing.T) {
	started := make(chan string, 2)
	release := make(chan struct{})
	var calls atomic.Int64
	r := New(Config{
		MaxConcurrent: 1,
		Lookup: func(ctx context.Context, address string) ([]string, error) {
			calls.Add(1)
			started <- address
			select {
			case <-release:
				return []string{address + ".example.test."}, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		},
	})

	active := netip.MustParseAddr("192.0.2.10")
	require.Equal(t, ScheduleScheduled, r.Schedule(active))
	require.Equal(t, active.String(), <-started)
	require.Equal(t, SchedulePending, r.Schedule(active))
	require.Equal(t, ScheduleSaturated, r.Schedule(netip.MustParseAddr("192.0.2.11")))
	require.Equal(t, ScheduleInvalid, r.Schedule(netip.Addr{}))
	require.Equal(t, int64(1), calls.Load())
	requireResolverState(t, r, 1, 0, 1)

	close(release)
	waitForNoCalls(t, r)
	require.Equal(t, SchedulePositive, r.Schedule(active))
	require.Equal(t, int64(1), calls.Load())
}

func TestActiveAndQueuedCallsCoalesceByAddress(t *testing.T) {
	started := make(chan string, 2)
	releaseFirst := make(chan struct{})
	releaseSecond := make(chan struct{})
	var calls atomic.Int64
	r := New(Config{
		MaxConcurrent: 1,
		Lookup: func(ctx context.Context, address string) ([]string, error) {
			calls.Add(1)
			started <- address
			release := releaseSecond
			if address == "192.0.2.10" {
				release = releaseFirst
			}
			select {
			case <-release:
				return []string{address + ".example.test"}, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		},
	})

	first := netip.MustParseAddr("192.0.2.10")
	second := netip.MustParseAddr("192.0.2.11")
	require.Equal(t, ScheduleScheduled, r.Schedule(first))
	require.Equal(t, first.String(), <-started)

	type outcome struct {
		result Result
		err    error
	}
	outcomes := make(chan outcome, 2)
	go func() {
		result, err := r.Resolve(context.Background(), second)
		outcomes <- outcome{result: result, err: err}
	}()
	waitForCallState(t, r, second, callQueued, 1)
	go func() {
		result, err := r.Resolve(context.Background(), second)
		outcomes <- outcome{result: result, err: err}
	}()
	waitForCallState(t, r, second, callQueued, 2)

	require.Equal(t, SchedulePending, r.Schedule(second))
	require.Equal(t, 1, r.queued.len)
	require.Equal(t, int64(1), calls.Load())

	close(releaseFirst)
	require.Equal(t, second.String(), <-started)
	close(releaseSecond)
	for range 2 {
		got := <-outcomes
		require.NoError(t, got.err)
		require.Equal(t, Result{State: StatePositive, Name: second.String() + ".example.test"}, got.result)
	}
	require.Equal(t, int64(2), calls.Load())
	requireResolverState(t, r, 0, 0, 0)
}

func TestQueuedWaiterCancellationRemovesOnlyFinalOrphan(t *testing.T) {
	started := make(chan string, 2)
	release := make(chan struct{})
	var calls atomic.Int64
	r := New(Config{
		MaxConcurrent: 1,
		Lookup: func(ctx context.Context, address string) ([]string, error) {
			calls.Add(1)
			started <- address
			select {
			case <-release:
				return []string{address + ".example.test"}, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		},
	})

	active := netip.MustParseAddr("192.0.2.10")
	queued := netip.MustParseAddr("192.0.2.11")
	require.Equal(t, ScheduleScheduled, r.Schedule(active))
	require.Equal(t, active.String(), <-started)

	ctx1, cancel1 := context.WithCancel(context.Background())
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel1()
	defer cancel2()
	errs := make(chan error, 2)
	go func() { _, err := r.Resolve(ctx1, queued); errs <- err }()
	waitForCallState(t, r, queued, callQueued, 1)
	go func() { _, err := r.Resolve(ctx2, queued); errs <- err }()
	waitForCallState(t, r, queued, callQueued, 2)

	cancel1()
	require.ErrorIs(t, <-errs, context.Canceled)
	waitForCallState(t, r, queued, callQueued, 1)
	require.Equal(t, 1, r.queued.len)

	cancel2()
	require.ErrorIs(t, <-errs, context.Canceled)
	waitForMissingCall(t, r, queued)
	require.Zero(t, r.queued.len)

	close(release)
	waitForNoCalls(t, r)
	require.Equal(t, int64(1), calls.Load(), "orphaned queued lookup must not start")
}

func TestCancelOneActiveWaiterDoesNotCancelSharedLookup(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	lookupCanceled := make(chan struct{}, 1)
	r := New(Config{
		LookupTimeout: time.Second,
		Lookup: func(ctx context.Context, _ string) ([]string, error) {
			close(started)
			select {
			case <-release:
				return []string{"shared.example.test"}, nil
			case <-ctx.Done():
				lookupCanceled <- struct{}{}
				return nil, ctx.Err()
			}
		},
	})

	addr := netip.MustParseAddr("192.0.2.10")
	ctx, cancel := context.WithCancel(context.Background())
	first := make(chan error, 1)
	second := make(chan struct {
		result Result
		err    error
	}, 1)
	go func() { _, err := r.Resolve(ctx, addr); first <- err }()
	<-started
	go func() {
		result, err := r.Resolve(context.Background(), addr)
		second <- struct {
			result Result
			err    error
		}{result: result, err: err}
	}()
	waitForCallState(t, r, addr, callActive, 2)

	cancel()
	require.ErrorIs(t, <-first, context.Canceled)
	select {
	case <-lookupCanceled:
		t.Fatal("caller cancellation reached shared lookup context")
	default:
	}

	close(release)
	got := <-second
	require.NoError(t, got.err)
	require.Equal(t, Result{State: StatePositive, Name: "shared.example.test"}, got.result)
}

func TestCancelFinalActiveWaiterKeepsLookupAndCachesResult(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	lookupCanceled := make(chan struct{}, 1)
	r := New(Config{
		LookupTimeout: time.Second,
		Lookup: func(ctx context.Context, _ string) ([]string, error) {
			close(started)
			select {
			case <-release:
				return []string{"orphaned.example.test"}, nil
			case <-ctx.Done():
				lookupCanceled <- struct{}{}
				return nil, ctx.Err()
			}
		},
	})

	addr := netip.MustParseAddr("192.0.2.10")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := r.Resolve(ctx, addr); done <- err }()
	<-started
	waitForCallState(t, r, addr, callActive, 1)

	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
	waitForCallState(t, r, addr, callActive, 0)
	select {
	case <-lookupCanceled:
		t.Fatal("final caller cancellation reached active lookup context")
	default:
	}

	close(release)
	waitForNoCalls(t, r)
	require.Equal(t, Result{State: StatePositive, Name: "orphaned.example.test"}, r.Lookup(addr))
}

func TestCompletedResultWinsLaterCallerCancellation(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	r := New(Config{Lookup: func(ctx context.Context, _ string) ([]string, error) {
		close(started)
		select {
		case <-release:
			return []string{"completed.example.test"}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}})

	addr := netip.MustParseAddr("192.0.2.10")
	ctx, cancel := context.WithCancel(context.Background())
	type outcome struct {
		result Result
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := r.Resolve(ctx, addr)
		done <- outcome{result: result, err: err}
	}()
	<-started

	r.mu.Lock()
	callDone := r.calls[addr].done
	r.mu.Unlock()
	close(release)
	<-callDone
	cancel()

	got := <-done
	require.NoError(t, got.err)
	require.Equal(t, Result{State: StatePositive, Name: "completed.example.test"}, got.result)
}

func TestResolveFIFOHasPriorityOverSchedule(t *testing.T) {
	started := make(chan string, 3)
	releases := map[string]chan struct{}{
		"192.0.2.10": make(chan struct{}),
		"192.0.2.11": make(chan struct{}),
		"192.0.2.12": make(chan struct{}),
	}
	r := New(Config{
		MaxConcurrent: 1,
		Lookup: func(ctx context.Context, address string) ([]string, error) {
			started <- address
			select {
			case <-releases[address]:
				return []string{address + ".example.test"}, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		},
	})

	a := netip.MustParseAddr("192.0.2.10")
	b := netip.MustParseAddr("192.0.2.11")
	c := netip.MustParseAddr("192.0.2.12")
	d := netip.MustParseAddr("192.0.2.13")
	require.Equal(t, ScheduleScheduled, r.Schedule(a))
	require.Equal(t, a.String(), <-started)

	done := make(chan error, 2)
	go func() { _, err := r.Resolve(context.Background(), b); done <- err }()
	waitForCallState(t, r, b, callQueued, 1)
	go func() { _, err := r.Resolve(context.Background(), c); done <- err }()
	waitForCallState(t, r, c, callQueued, 1)
	require.Equal(t, ScheduleSaturated, r.Schedule(d))

	close(releases[a.String()])
	require.Equal(t, b.String(), <-started)
	require.Equal(t, ScheduleSaturated, r.Schedule(d))
	close(releases[b.String()])
	require.NoError(t, <-done)
	require.Equal(t, c.String(), <-started)
	close(releases[c.String()])
	require.NoError(t, <-done)
	requireResolverState(t, r, 0, 0, 0)
}

func TestLookupTimeoutProducesNegativeAndReleasesCapacity(t *testing.T) {
	lookupDone := make(chan error, 1)
	r := New(Config{
		LookupTimeout: 10 * time.Millisecond,
		MaxConcurrent: 1,
		Lookup: func(ctx context.Context, _ string) ([]string, error) {
			<-ctx.Done()
			lookupDone <- ctx.Err()
			return nil, ctx.Err()
		},
	})

	result, err := r.Resolve(context.Background(), netip.MustParseAddr("192.0.2.10"))
	require.NoError(t, err)
	require.Equal(t, Result{State: StateNegative}, result)
	require.ErrorIs(t, <-lookupDone, context.DeadlineExceeded)
	requireResolverState(t, r, 0, 0, 0)
}

func TestMaxConcurrentLookups(t *testing.T) {
	const limit = 32
	started := make(chan struct{}, limit)
	release := make(chan struct{})
	var active atomic.Int64
	var maxActive atomic.Int64
	r := New(Config{
		MaxConcurrent: limit,
		Lookup: func(ctx context.Context, _ string) ([]string, error) {
			current := active.Add(1)
			for {
				seen := maxActive.Load()
				if current <= seen || maxActive.CompareAndSwap(seen, current) {
					break
				}
			}
			started <- struct{}{}
			select {
			case <-release:
				active.Add(-1)
				return []string{"host.example.test"}, nil
			case <-ctx.Done():
				active.Add(-1)
				return nil, ctx.Err()
			}
		},
	})

	for i := range limit {
		require.Equal(t, ScheduleScheduled, r.Schedule(testAddr(i)))
	}
	for range limit {
		<-started
	}
	require.Equal(t, ScheduleSaturated, r.Schedule(testAddr(limit)))
	require.Equal(t, int64(limit), maxActive.Load())
	requireResolverState(t, r, limit, 0, limit)

	close(release)
	waitForNoCalls(t, r)
	require.Zero(t, active.Load())
}

func TestSLRUTransitionsAndNegativeRetention(t *testing.T) {
	clock := newTestClock()
	r := New(Config{Now: clock.Now, MaxEntries: 5})
	addrs := make([]netip.Addr, 6)
	for i := range 5 {
		addrs[i] = testAddr(i)
		insertTestResult(r, addrs[i], Result{State: StatePositive, Name: fmt.Sprintf("host-%d.example.test", i)})
	}
	require.Equal(t, 5, r.probationary.len)
	require.Zero(t, r.protected.len)

	for i := range 5 {
		require.Equal(t, StatePositive, r.Lookup(addrs[i]).State)
	}
	require.Equal(t, 4, r.protected.len)
	require.Equal(t, 1, r.probationary.len)
	require.Equal(t, addrs[0], r.probationary.front.addr, "protected LRU must demote to probationary MRU")

	addrs[5] = testAddr(5)
	insertTestResult(r, addrs[5], Result{State: StateNegative})
	require.Len(t, r.cache, 5)
	require.Nil(t, r.cache[addrs[0]], "probationary LRU must be evicted first")
	require.NotNil(t, r.cache[addrs[5]])
	require.Equal(t, segmentProbationary, r.cache[addrs[5]].segment)
	require.Equal(t, StateNegative, r.Lookup(addrs[5]).State)
	require.Equal(t, segmentProbationary, r.cache[addrs[5]].segment, "negative hit must never promote")
	requireCacheInvariants(t, r)
}

func TestOneEntryCacheKeepsPositiveInProbationary(t *testing.T) {
	r := New(Config{Now: newTestClock().Now, MaxEntries: 1})
	first := testAddr(1)
	second := testAddr(2)

	insertTestResult(r, first, Result{State: StatePositive, Name: "first.example.test"})
	require.Equal(t, StatePositive, r.Lookup(first).State)
	require.Zero(t, r.protectedLimit)
	require.Zero(t, r.protected.len)
	require.Equal(t, 1, r.probationary.len)

	insertTestResult(r, second, Result{State: StateNegative})
	require.Equal(t, Result{State: StateMiss}, r.Lookup(first))
	require.Equal(t, Result{State: StateNegative}, r.Lookup(second))
	requireCacheInvariants(t, r)
}

func TestSLRUProtectsReusedPositivesFromOneShotScan(t *testing.T) {
	r := New(Config{Now: newTestClock().Now, MaxEntries: 100})
	protected := make([]netip.Addr, 80)
	for i := range protected {
		protected[i] = testAddr(i)
		insertTestResult(r, protected[i], Result{State: StatePositive, Name: fmt.Sprintf("protected-%d.example.test", i)})
		r.Lookup(protected[i])
	}
	require.Equal(t, 80, r.protected.len)

	for i := 80; i < 10_080; i++ {
		result := Result{State: StateNegative}
		if i%2 == 0 {
			result = Result{State: StatePositive, Name: fmt.Sprintf("scan-%d.example.test", i)}
		}
		insertTestResult(r, testAddr(i), result)
	}

	for _, addr := range protected {
		require.NotNil(t, r.cache[addr])
		require.Equal(t, segmentProtected, r.cache[addr].segment)
	}
	require.Len(t, r.cache, 100)
	require.Equal(t, 80, r.protected.len)
	requireCacheInvariants(t, r)
}

func TestExpiredProtectedEntriesAreReclaimedBeforeLiveEviction(t *testing.T) {
	clock := newTestClock()
	r := New(Config{
		Now:         clock.Now,
		PositiveTTL: time.Hour,
		NegativeTTL: time.Minute,
		MaxEntries:  5,
	})

	for i := range 4 {
		addr := testAddr(i)
		insertTestResult(r, addr, Result{State: StatePositive, Name: fmt.Sprintf("host-%d.example.test", i)})
		r.Lookup(addr)
	}
	require.Equal(t, 4, r.protected.len)

	clock.Add(time.Hour)
	live := testAddr(100)
	insertTestResult(r, live, Result{State: StatePositive, Name: "live.example.test"})

	require.Len(t, r.cache, 1)
	require.NotNil(t, r.cache[live])
	require.Zero(t, r.protected.len)
	require.Equal(t, 1, r.probationary.len)
	requireCacheInvariants(t, r)
}

func TestBackwardClockObservationPreservesExpiryOrdering(t *testing.T) {
	clock := newTestClock()
	r := New(Config{Now: clock.Now, PositiveTTL: time.Hour})
	first := testAddr(1)
	second := testAddr(2)
	third := testAddr(3)

	insertTestResult(r, first, Result{State: StatePositive, Name: "first.example.test"})
	clock.Add(10 * time.Minute)
	insertTestResult(r, second, Result{State: StatePositive, Name: "second.example.test"})
	clock.Add(-5 * time.Minute)
	insertTestResult(r, third, Result{State: StatePositive, Name: "third.example.test"})

	r.mu.Lock()
	secondExpiry := r.cache[second].expiresAt
	thirdExpiry := r.cache[third].expiresAt
	r.mu.Unlock()
	require.Equal(t, secondExpiry, thirdExpiry)
	requireCacheInvariants(t, r)

	clock.Add(66 * time.Minute)
	require.Equal(t, Result{State: StateMiss}, r.Lookup(third))
	require.Empty(t, r.cache)
	requireCacheInvariants(t, r)
}

func TestRandomizedCacheOperationsPreserveInvariants(t *testing.T) {
	clock := newTestClock()
	r := New(Config{
		Now:         clock.Now,
		PositiveTTL: 50 * time.Second,
		NegativeTTL: 10 * time.Second,
		MaxEntries:  50,
	})
	rng := rand.New(rand.NewSource(42))

	for range 2_000 {
		addr := testAddr(rng.Intn(200))
		switch rng.Intn(4) {
		case 0:
			insertTestResult(r, addr, Result{State: StatePositive, Name: "host.example.test"})
		case 1:
			insertTestResult(r, addr, Result{State: StateNegative})
		case 2:
			r.Lookup(addr)
		case 3:
			clock.Add(time.Duration(rng.Intn(3)) * time.Second)
			r.Lookup(addr)
		}
		requireCacheInvariants(t, r)
	}
}

func insertTestResult(r *Resolver, addr netip.Addr, result Result) {
	now := r.now()
	r.mu.Lock()
	now = r.advanceNowLocked(now)
	r.purgeExpiredLocked(now)
	r.insertCacheLocked(addr, result, now)
	r.mu.Unlock()
}

func testAddr(i int) netip.Addr {
	return netip.AddrFrom4([4]byte{198, 18, byte(i >> 8), byte(i)})
}

func waitForCallState(t *testing.T, r *Resolver, addr netip.Addr, state callState, waiters int) {
	t.Helper()
	waitForState(t, func() bool {
		r.mu.Lock()
		defer r.mu.Unlock()
		c := r.calls[addr]
		return c != nil && c.state == state && c.waiters == waiters
	})
}

func waitForMissingCall(t *testing.T, r *Resolver, addr netip.Addr) {
	t.Helper()
	waitForState(t, func() bool {
		r.mu.Lock()
		defer r.mu.Unlock()
		return r.calls[addr] == nil
	})
}

func waitForNoCalls(t *testing.T, r *Resolver) {
	t.Helper()
	waitForState(t, func() bool {
		r.mu.Lock()
		defer r.mu.Unlock()
		return len(r.calls) == 0 && r.active == 0 && r.queued.len == 0
	})
}

func waitForState(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for resolver state")
		}
		runtime.Gosched()
	}
}

func requireResolverState(t *testing.T, r *Resolver, active, queued, calls int) {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	require.Equal(t, active, r.active)
	require.Equal(t, queued, r.queued.len)
	require.Len(t, r.calls, calls)
}

func requireCacheInvariants(t *testing.T, r *Resolver) {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()

	require.LessOrEqual(t, len(r.cache), r.maxEntries)
	require.LessOrEqual(t, r.protected.len, r.protectedLimit)
	require.Equal(t, len(r.cache), r.probationary.len+r.protected.len)

	seen := make(map[*cacheEntry]struct{}, len(r.cache))
	for entry := r.probationary.front; entry != nil; entry = entry.recencyNext {
		require.Equal(t, segmentProbationary, entry.segment)
		require.Same(t, entry, r.cache[entry.addr])
		require.NotContains(t, seen, entry)
		seen[entry] = struct{}{}
	}
	for entry := r.protected.front; entry != nil; entry = entry.recencyNext {
		require.Equal(t, segmentProtected, entry.segment)
		require.Same(t, entry, r.cache[entry.addr])
		require.NotContains(t, seen, entry)
		seen[entry] = struct{}{}
	}
	require.Len(t, seen, len(r.cache))

	expirySeen := make(map[*cacheEntry]struct{}, len(r.cache))
	for entry := r.positiveExpiry.front; entry != nil; entry = entry.expiryNext {
		require.Equal(t, StatePositive, entry.result.State)
		require.Same(t, entry, r.cache[entry.addr])
		require.NotContains(t, expirySeen, entry)
		expirySeen[entry] = struct{}{}
	}
	for entry := r.negativeExpiry.front; entry != nil; entry = entry.expiryNext {
		require.Equal(t, StateNegative, entry.result.State)
		require.Same(t, entry, r.cache[entry.addr])
		require.NotContains(t, expirySeen, entry)
		expirySeen[entry] = struct{}{}
	}
	require.Len(t, expirySeen, len(r.cache))
}
