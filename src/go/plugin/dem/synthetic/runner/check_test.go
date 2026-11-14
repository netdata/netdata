//go:build linux || darwin || freebsd

// SPDX-License-Identifier: GPL-3.0-or-later
package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func checkWaiters(t *testing.T, e *Engine, n int, retiring bool) {
	t.Helper()
	require.Eventually(t, func() bool {
		e.checks.mu.Lock()
		defer e.checks.mu.Unlock()
		f := e.checks.flight
		return f != nil && f.waiters == n && f.retiring == retiring
	}, time.Second, time.Millisecond)
}
func checkReply(out *os.File) ndexec.TreeResult {
	_, err := fmt.Fprintf(out, `{"version":"release-a","contract":2,"uid":%d,"gid":%d}`, os.Geteuid(), os.Getegid())
	return ndexec.TreeResult{Drained: true, Err: err}
}
func TestConcurrentChecksShareOneProbeAndLaterCheckIsFresh(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("requires unprivileged identity")
	}
	e, _, _ := testEngine(t)
	started, release := make(chan struct{}, 2), make(chan struct{})
	var calls atomic.Int32
	e.start = starter(t, func(_ context.Context, args []string, _ []byte, out, _ *os.File) ndexec.TreeResult {
		assert.Contains(t, args, "--check")
		calls.Add(1)
		started <- struct{}{}
		<-release
		return checkReply(out)
	})
	const n = 16
	done := make(chan error, n)
	for i := 0; i < n; i++ {
		kind := synthetic.Journey
		if i%2 == 1 {
			kind = synthetic.Lighthouse
		}
		go func() { done <- e.Check(t.Context(), kind) }()
	}
	receive(t, started)
	checkWaiters(t, e, n, false)
	require.ErrorContains(t, e.Check(t.Context(), synthetic.Kind("invalid")), "unsupported synthetic kind")
	checkWaiters(t, e, n, false)
	assert.Equal(t, int32(1), calls.Load())
	close(release)
	for i := 0; i < n; i++ {
		require.NoError(t, receive(t, done))
	}
	require.NoError(t, e.Check(t.Context(), synthetic.Journey))
	assert.Equal(t, int32(2), calls.Load())
	e.checks.mu.Lock()
	assert.Nil(t, e.checks.flight)
	e.checks.mu.Unlock()
}
func TestCheckWaitersCancelIndependentlyAndLastWaiterJoinsBeforeReplacement(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("requires unprivileged identity")
	}
	e, _, _ := testEngine(t)
	paths := e.source.(fixtureRuntime).Paths
	var mu sync.Mutex
	var uses []*observedUse
	e.source = sourceFunc(func() (Use, error) {
		u := &observedUse{paths: paths}
		mu.Lock()
		uses = append(uses, u)
		mu.Unlock()
		return u, nil
	})
	started, cancelled, join := make(chan struct{}, 2), make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	e.start = starter(t, func(ctx context.Context, _ []string, _ []byte, out, _ *os.File) ndexec.TreeResult {
		n := calls.Add(1)
		started <- struct{}{}
		if n == 1 {
			<-ctx.Done()
			close(cancelled)
			<-join
			return ndexec.TreeResult{Drained: true, Err: ctx.Err()}
		}
		return checkReply(out)
	})
	ctx1, cancel1 := context.WithCancel(t.Context())
	defer cancel1()
	ctx2, cancel2 := context.WithCancel(t.Context())
	defer cancel2()
	first, second, replacement := make(chan error, 1), make(chan error, 1), make(chan error, 1)
	go func() { first <- e.Check(ctx1, synthetic.Journey) }()
	receive(t, started)
	go func() { second <- e.Check(ctx2, synthetic.Lighthouse) }()
	checkWaiters(t, e, 2, false)
	cancel1()
	require.ErrorIs(t, receive(t, first), context.Canceled)
	checkWaiters(t, e, 1, false)
	select {
	case <-cancelled:
		t.Fatal("one waiter cancelled a probe still needed by another")
	default:
	}
	cancel2()
	receive(t, cancelled)
	checkWaiters(t, e, 0, true)
	mu.Lock()
	require.Len(t, uses, 1)
	assert.Empty(t, uses[0].finished())
	mu.Unlock()
	select {
	case <-second:
		t.Fatal("last waiter returned before physical completion")
	default:
	}
	go func() { replacement <- e.Check(t.Context(), synthetic.Journey) }()
	expired, expire := context.WithCancel(t.Context())
	expire()
	require.ErrorIs(t, e.Check(expired, synthetic.Journey), context.Canceled)
	assert.Equal(t, int32(1), calls.Load())
	close(join)
	require.ErrorIs(t, receive(t, second), context.Canceled)
	require.NoError(t, receive(t, replacement))
	assert.Equal(t, int32(2), calls.Load())
	mu.Lock()
	defer mu.Unlock()
	require.Len(t, uses, 2)
	for _, u := range uses {
		assert.Equal(t, []bool{true}, u.finished())
	}
}
func TestLoneCheckDeadlineJoinsAndDoesNotCacheFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("requires unprivileged identity")
	}
	e, _, _ := testEngine(t)
	started, cancelled, joined := make(chan struct{}, 1), make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	e.start = starter(t, func(ctx context.Context, _ []string, _ []byte, out, _ *os.File) ndexec.TreeResult {
		if calls.Add(1) == 1 {
			started <- struct{}{}
			<-ctx.Done()
			close(cancelled)
			<-joined
			return ndexec.TreeResult{Drained: true, Err: ctx.Err()}
		}
		return checkReply(out)
	})
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- e.Check(ctx, synthetic.Journey) }()
	receive(t, started)
	receive(t, cancelled)
	select {
	case <-done:
		t.Fatal("deadline returned before physical join")
	default:
	}
	close(joined)
	require.ErrorIs(t, receive(t, done), context.DeadlineExceeded)
	require.NoError(t, e.Check(t.Context(), synthetic.Lighthouse))
	assert.Equal(t, int32(2), calls.Load())
}
func TestCandidateCheckFlightIsIndependentEvenForMatchingReleaseIdentity(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("requires unprivileged identity")
	}
	e, _, a := testEngine(t)
	started, release := make(chan struct{}, 2), make(chan struct{})
	var calls atomic.Int32
	e.start = starter(t, func(_ context.Context, _ []string, _ []byte, out, _ *os.File) ndexec.TreeResult {
		calls.Add(1)
		started <- struct{}{}
		<-release
		return checkReply(out)
	})
	candidate := e.unrecordedView(e.source, a)
	monitorDone, candidateDone := make(chan error, 2), make(chan error, 2)
	go func() { monitorDone <- e.Check(t.Context(), synthetic.Journey) }()
	receive(t, started)
	go func() { candidateDone <- candidate.Check(t.Context(), synthetic.Lighthouse) }()
	receive(t, started)
	go func() { monitorDone <- e.Check(t.Context(), synthetic.Lighthouse) }()
	go func() { candidateDone <- candidate.Check(t.Context(), synthetic.Journey) }()
	checkWaiters(t, e, 2, false)
	checkWaiters(t, candidate, 2, false)
	assert.Equal(t, int32(2), calls.Load())
	assert.Same(t, e.executionOwner, candidate.executionOwner)
	close(release)
	for i := 0; i < 2; i++ {
		require.NoError(t, receive(t, monitorDone))
		require.NoError(t, receive(t, candidateDone))
	}
}
func TestUnverifiedCheckDrainPoisonsCoalescedWaitersAndAllViews(t *testing.T) {
	e, _, a := testEngine(t)
	started, release := make(chan struct{}, 1), make(chan struct{})
	var calls atomic.Int32
	e.start = starter(t, func(context.Context, []string, []byte, *os.File, *os.File) ndexec.TreeResult {
		calls.Add(1)
		started <- struct{}{}
		<-release
		return ndexec.TreeResult{Drained: false, Err: ndexec.ErrTreeNotDrained}
	})
	candidate := e.unrecordedView(e.source, a)
	done := make(chan error, 2)
	go func() { done <- candidate.Check(t.Context(), synthetic.Journey) }()
	receive(t, started)
	go func() { done <- candidate.Check(t.Context(), synthetic.Lighthouse) }()
	checkWaiters(t, candidate, 2, false)
	close(release)
	for i := 0; i < 2; i++ {
		assert.ErrorIs(t, receive(t, done), ndexec.ErrTreeNotDrained)
	}
	assert.ErrorIs(t, receive(t, e.Fatal()), ndexec.ErrTreeNotDrained)
	assert.ErrorIs(t, e.Check(t.Context(), synthetic.Journey), ndexec.ErrTreeNotDrained)
	assert.False(t, e.Execute(t.Context(), journey(), nil).Drained)
	assert.Equal(t, int32(1), calls.Load())
}
func TestQualificationIdentityDoesNotCollideWithUserJob(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("requires unprivileged identity")
	}
	e, _, _ := testEngineSlots(t, 2)
	monitoring, qualified, release := make(chan struct{}, 1), make(chan struct{}, 1), make(chan struct{})
	e.start = starter(t, func(ctx context.Context, args []string, raw []byte, out, stderr *os.File) ndexec.TreeResult {
		if slices.Contains(args, "--check") {
			return checkReply(out)
		}
		var r wireRequest
		if err := json.Unmarshal(raw, &r); err != nil {
			return ndexec.TreeResult{Drained: true, Err: err}
		}
		if r.Script == "monitor" {
			monitoring <- struct{}{}
			<-release
		} else if r.Kind == synthetic.Journey {
			qualified <- struct{}{}
		}
		result := success()
		if r.Kind == synthetic.Lighthouse {
			result.Tests = nil
			score := 100.0
			result.Metrics = &synthetic.LabMetrics{Performance: &score}
		}
		return resultPayload(result)(ctx, args, raw, out, stderr)
	})
	monitorDone := make(chan synthetic.Execution, 1)
	go func() { monitorDone <- e.Execute(t.Context(), namedRequest("runtime-qualification", "monitor"), nil) }()
	receive(t, monitoring)
	qualification := make(chan error, 1)
	go func() { _, err := e.Qualify(t.Context(), e.source); qualification <- err }()
	receive(t, qualified)
	require.NoError(t, receive(t, qualification))
	close(release)
	require.Equal(t, synthetic.Success, receive(t, monitorDone).Run.Outcome)
}

// TestCheckResponseBounds pins the --check stdout cap and the stderr
// diagnostic kept from a failing check.
func TestCheckResponseBounds(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("requires unprivileged identity")
	}
	reply := fmt.Sprintf(`{"version":"release-a","contract":2,"uid":%d,"gid":%d}`, os.Geteuid(), os.Getegid())
	tests := map[string]struct {
		stdout, stderr string
		failed         bool
		err            string
	}{
		"reply at limit":     {stdout: reply + strings.Repeat(" ", 4096-len(reply))},
		"reply over limit":   {stdout: reply + strings.Repeat(" ", 4097-len(reply)), err: "prepared runtime check: runtime check response exceeds 4096 bytes"},
		"clipped diagnostic": {stderr: strings.Repeat("z", 3*TextLimit), failed: true, err: "prepared runtime check: runtime check failed: exit status 1 (" + strings.Repeat("z", TextLimit) + ")"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			e, _, _ := testEngine(t)
			e.start = starter(t, func(_ context.Context, _ []string, _ []byte, out, stderr *os.File) ndexec.TreeResult {
				_, _ = io.WriteString(out, tc.stdout)
				_, _ = io.WriteString(stderr, tc.stderr)
				if tc.failed {
					return ndexec.TreeResult{Drained: true, Err: errors.New("exit status 1")}
				}
				return ndexec.TreeResult{Drained: true}
			})
			err := e.Check(t.Context(), synthetic.Journey)
			if tc.err == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tc.err)
			}
		})
	}
}
