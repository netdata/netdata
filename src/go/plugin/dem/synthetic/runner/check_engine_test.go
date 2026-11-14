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

func TestCheckIsSupervisedWithoutWorkflowInput(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("requires unprivileged identity")
	}
	e, _, a := testEngine(t)
	paths := e.source.(fixtureRuntime).Paths
	calls := 0
	e.start = starter(t, func(_ context.Context, args []string, raw []byte, out, _ *os.File) ndexec.TreeResult {
		calls++
		assert.Empty(t, raw)
		assert.Equal(t, []string{paths.EntryPoint, "--check"}, args)
		fmt.Fprintf(out, `{"version":%q,"contract":2,"uid":%d,"gid":%d}`, paths.Identity.Version, os.Geteuid(), os.Getegid())
		return ndexec.TreeResult{Drained: true}
	})
	require.NoError(t, e.Check(context.Background()))
	assert.Equal(t, 1, calls)
	assert.Zero(t, a.finalized.Load())
}

func TestCheckRequiresTrustedReleaseAndPreservedIdentity(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("requires unprivileged identity")
	}
	for _, tc := range []struct {
		name, version      string
		contract, uid, gid int
		valid              bool
	}{
		{"qualified release", "release-a", synthetic.ExecutionContract, os.Geteuid(), os.Getegid(), true},
		{"another release", "release-b", synthetic.ExecutionContract, os.Geteuid(), os.Getegid(), false},
		{"another contract", "release-a", synthetic.ExecutionContract + 1, os.Geteuid(), os.Getegid(), false},
		{"root", "release-a", synthetic.ExecutionContract, 0, os.Getegid(), false},
		{"another uid", "release-a", synthetic.ExecutionContract, os.Geteuid() + 1, os.Getegid(), false},
		{"another gid", "release-a", synthetic.ExecutionContract, os.Geteuid(), os.Getegid() + 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, _, _ := testEngine(t)
			paths := e.source.(fixtureRuntime).Paths
			calls := 0
			e.start = starter(t, func(_ context.Context, args []string, raw []byte, out, _ *os.File) ndexec.TreeResult {
				calls++
				assert.Equal(t, []string{paths.EntryPoint, "--check"}, args)
				assert.Empty(t, raw)
				fmt.Fprintf(out, `{"version":%q,"contract":%d,"uid":%d,"gid":%d}`, tc.version, tc.contract, tc.uid, tc.gid)
				return ndexec.TreeResult{Drained: true}
			})
			err := e.Check(context.Background())
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			assert.Equal(t, 1, calls)
		})
	}
}

func checkWaiters(t *testing.T, e *Engine, n int, retiring bool) {
	t.Helper()
	require.Eventually(t, func() bool {
		e.checks.mu.Lock()
		defer e.checks.mu.Unlock()
		f := e.checks.flight
		return f != nil && f.waiters == n && (n == 0) == retiring
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
		go func() { done <- e.Check(t.Context()) }()
	}
	receive(t, started)
	checkWaiters(t, e, n, false)
	assert.Equal(t, int32(1), calls.Load())
	close(release)
	for i := 0; i < n; i++ {
		require.NoError(t, receive(t, done))
	}
	require.NoError(t, e.Check(t.Context()))
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
	go func() { first <- e.Check(ctx1) }()
	receive(t, started)
	go func() { second <- e.Check(ctx2) }()
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
	go func() { replacement <- e.Check(t.Context()) }()
	expired, expire := context.WithCancel(t.Context())
	expire()
	require.ErrorIs(t, e.Check(expired), context.Canceled)
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
	go func() { done <- e.Check(ctx) }()
	receive(t, started)
	receive(t, cancelled)
	select {
	case <-done:
		t.Fatal("deadline returned before physical join")
	default:
	}
	close(joined)
	require.ErrorIs(t, receive(t, done), context.DeadlineExceeded)
	require.NoError(t, e.Check(t.Context()))
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
	go func() { monitorDone <- e.Check(t.Context()) }()
	receive(t, started)
	go func() { candidateDone <- candidate.Check(t.Context()) }()
	receive(t, started)
	go func() { monitorDone <- e.Check(t.Context()) }()
	go func() { candidateDone <- candidate.Check(t.Context()) }()
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
	go func() { done <- candidate.Check(t.Context()) }()
	receive(t, started)
	go func() { done <- candidate.Check(t.Context()) }()
	checkWaiters(t, candidate, 2, false)
	close(release)
	for i := 0; i < 2; i++ {
		assert.ErrorIs(t, receive(t, done), ndexec.ErrTreeNotDrained)
	}
	assert.ErrorIs(t, receive(t, e.Fatal()), ndexec.ErrTreeNotDrained)
	assert.ErrorIs(t, e.Check(t.Context()), ndexec.ErrTreeNotDrained)
	assert.False(t, e.execute(t.Context(), journey()).Drained)
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
	go func() { monitorDone <- e.execute(t.Context(), namedRequest("runtime-qualification", "monitor")) }()
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
		"clipped diagnostic": {stderr: strings.Repeat("z", 3*textLimit), failed: true, err: "prepared runtime check: runtime check failed: exit status 1 (" + strings.Repeat("z", textLimit) + ")"},
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
			err := e.Check(t.Context())
			if tc.err == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tc.err)
			}
		})
	}
}

func TestRejectInvalidReleaseBeforeLaunch(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*Paths)
	}{
		{"contract", func(p *Paths) { p.Identity.Contract++ }},
		{"version", func(p *Paths) { p.Identity.Version = "" }},
		{"digest", func(p *Paths) { p.Identity.SHA256 = "not-a-digest" }},
		{"relative node", func(p *Paths) { p.Node = "node" }},
		{"relative entrypoint", func(p *Paths) { p.EntryPoint = "run.mjs" }},
		{"missing entrypoint", func(p *Paths) { p.EntryPoint += ".missing" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, _, _ := testEngine(t)
			source := e.source.(fixtureRuntime)
			tc.change(&source.Paths)
			e.source = source
			e.start = func(context.Context, ndexec.TreeOptions, string, ...string) (treeProcess, error) {
				t.Fatal("invalid release launched")
				return nil, nil
			}
			require.Error(t, e.Check(context.Background()))
			result := e.execute(context.Background(), journey())
			require.Equal(t, synthetic.Error, result.Run.Outcome)
			require.Nil(t, result.Run.Runtime)
		})
	}
}
