//go:build linux || darwin || freebsd

// SPDX-License-Identifier: GPL-3.0-or-later
package runner

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func receive[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for owned test operation")
		var zero T
		return zero
	}
}

func queued(t *testing.T, e *Engine, name string, count int) {
	t.Helper()
	require.Eventually(t, func() bool {
		e.mu.Lock()
		defer e.mu.Unlock()
		j := e.jobs[executionKey{jobID: "journey:" + name}]
		return j != nil && j.waiters.Len() == count
	}, time.Second, time.Millisecond, "request did not join its logical-job queue")
}

func namedRequest(name, script string) synthetic.Request {
	r := journey()
	r.Name = name
	r.Script = script
	r.Timeout = 5 * time.Second
	return r
}

func TestConstructorRequiresPositiveConcurrency(t *testing.T) {
	for _, n := range []int{-1, 0} {
		e, err := New(nil, nil, nil, n)
		require.Error(t, err)
		assert.Nil(t, e)
	}
	e, err := New(nil, nil, nil, 2)
	require.NoError(t, err)
	require.NotNil(t, e)
}

func TestAdmissionLimitsAttemptsAndSerializesMonitoringWithRehearsal(t *testing.T) {
	e, _, _ := testEngineSlots(t, 2)
	started := make(chan string, 4)
	releases := map[string]chan struct{}{}
	for _, s := range []string{"a-monitor", "a-test", "b", "c"} {
		releases[s] = make(chan struct{})
	}
	var active, maximum atomic.Int32
	e.start = starter(t, func(ctx context.Context, args []string, raw []byte, out, stderr *os.File) ndexec.TreeResult {
		var r wireRequest
		if err := json.Unmarshal(raw, &r); err != nil {
			return ndexec.TreeResult{Drained: true, Err: err}
		}
		n := active.Add(1)
		for old := maximum.Load(); n > old && !maximum.CompareAndSwap(old, n); old = maximum.Load() {
		}
		started <- r.Script
		<-releases[r.Script]
		active.Add(-1)
		return resultPayload(success())(ctx, args, raw, out, stderr)
	})
	monitoring := make(chan synthetic.Execution, 1)
	go func() { monitoring <- e.Execute(t.Context(), namedRequest("a", "a-monitor"), nil) }()
	require.Equal(t, "a-monitor", receive(t, started))
	rehearsal := make(chan error, 1)
	go func() { rehearsal <- e.Rehearse(t.Context(), namedRequest("a", "a-test")) }()
	queued(t, e, "a", 1)
	other := make(chan synthetic.Execution, 2)
	go func() { other <- e.Execute(t.Context(), namedRequest("b", "b"), nil) }()
	require.Equal(t, "b", receive(t, started))
	go func() { other <- e.Execute(t.Context(), namedRequest("c", "c"), nil) }()
	queued(t, e, "c", 1)
	close(releases["b"])
	require.Equal(t, synthetic.Success, receive(t, other).Run.Outcome)
	require.Equal(t, "c", receive(t, started), "same-job rehearsal must not take an unrelated free slot")
	close(releases["c"])
	require.Equal(t, synthetic.Success, receive(t, other).Run.Outcome)
	queued(t, e, "a", 1)
	close(releases["a-monitor"])
	require.Equal(t, synthetic.Success, receive(t, monitoring).Run.Outcome)
	require.Equal(t, "a-test", receive(t, started))
	close(releases["a-test"])
	require.NoError(t, receive(t, rehearsal))
	assert.Equal(t, int32(2), maximum.Load())
	e.mu.Lock()
	assert.Zero(t, e.active)
	assert.Empty(t, e.jobs)
	assert.Zero(t, e.ready.Len())
	e.mu.Unlock()
}

func TestAdmissionGivesReadyJobsTurnsInsteadOfDrainingOneJobBacklog(t *testing.T) {
	e, _, _ := testEngine(t)
	started := make(chan string, 8)
	releases := map[string]chan struct{}{}
	for _, s := range []string{"a1", "a2", "a3", "b1", "b2", "c1"} {
		releases[s] = make(chan struct{})
	}
	e.start = starter(t, func(ctx context.Context, args []string, raw []byte, out, stderr *os.File) ndexec.TreeResult {
		var r wireRequest
		if err := json.Unmarshal(raw, &r); err != nil {
			return ndexec.TreeResult{Drained: true, Err: err}
		}
		started <- r.Script
		<-releases[r.Script]
		return resultPayload(success())(ctx, args, raw, out, stderr)
	})
	done := make(chan synthetic.Execution, 6)
	launch := func(name, script string) {
		go func() { done <- e.Execute(t.Context(), namedRequest(name, script), nil) }()
	}
	launch("a", "a1")
	require.Equal(t, "a1", receive(t, started))
	launch("a", "a2")
	queued(t, e, "a", 1)
	launch("a", "a3")
	queued(t, e, "a", 2)
	launch("b", "b1")
	queued(t, e, "b", 1)
	launch("c", "c1")
	queued(t, e, "c", 1)
	close(releases["a1"])
	require.Equal(t, "b1", receive(t, started))
	close(releases["b1"])
	require.Equal(t, "c1", receive(t, started))
	close(releases["c1"])
	require.Equal(t, "a2", receive(t, started))
	launch("b", "b2")
	queued(t, e, "b", 1)
	close(releases["a2"])
	require.Equal(t, "b2", receive(t, started))
	close(releases["b2"])
	require.Equal(t, "a3", receive(t, started))
	close(releases["a3"])
	for i := 0; i < 6; i++ {
		require.Equal(t, synthetic.Success, receive(t, done).Run.Outcome)
	}
}

func TestAdmissionCancellationRemovesReadyJobWithoutChangingOtherPositions(t *testing.T) {
	e, history, _ := testEngine(t)
	started := make(chan string, 4)
	release := make(chan struct{})
	e.start = starter(t, func(ctx context.Context, args []string, raw []byte, out, stderr *os.File) ndexec.TreeResult {
		var r wireRequest
		if err := json.Unmarshal(raw, &r); err != nil {
			return ndexec.TreeResult{Drained: true, Err: err}
		}
		started <- r.Script
		if r.Script == "hold" {
			<-release
		}
		return resultPayload(success())(ctx, args, raw, out, stderr)
	})
	done := make(chan synthetic.Execution, 3)
	go func() { done <- e.Execute(t.Context(), namedRequest("hold", "hold"), nil) }()
	require.Equal(t, "hold", receive(t, started))
	cancelled := make(chan synthetic.Execution, 1)
	ctx, cancel := context.WithCancel(t.Context())
	go func() { cancelled <- e.Execute(ctx, namedRequest("cancel", "must-not-start"), nil) }()
	queued(t, e, "cancel", 1)
	go func() { done <- e.Execute(t.Context(), namedRequest("b", "b"), nil) }()
	queued(t, e, "b", 1)
	go func() { done <- e.Execute(t.Context(), namedRequest("c", "c"), nil) }()
	queued(t, e, "c", 1)
	cancel()
	result := receive(t, cancelled)
	assert.Equal(t, synthetic.Cancelled, result.Run.Outcome)
	assert.Zero(t, result.Run.StartedUS)
	e.mu.Lock()
	assert.NotContains(t, e.jobs, executionKey{jobID: "journey:cancel"})
	e.mu.Unlock()
	close(release)
	require.Equal(t, "b", receive(t, started))
	require.Equal(t, "c", receive(t, started))
	for i := 0; i < 3; i++ {
		require.Equal(t, synthetic.Success, receive(t, done).Run.Outcome)
	}
	history.mu.Lock()
	assert.Len(t, history.runs, 6)
	history.mu.Unlock()
}

func TestScheduledExecutionSnapshotsLatestOccurrenceAfterAdmission(t *testing.T) {
	e, history, _ := testEngine(t)
	running, release := make(chan struct{}), make(chan struct{})
	e.start = starter(t, func(ctx context.Context, args []string, raw []byte, out, stderr *os.File) ndexec.TreeResult {
		var r wireRequest
		if err := json.Unmarshal(raw, &r); err != nil {
			return ndexec.TreeResult{Drained: true, Err: err}
		}
		if r.Script == "hold" {
			close(running)
			<-release
		}
		return resultPayload(success())(ctx, args, raw, out, stderr)
	})
	first := make(chan synthetic.Execution, 1)
	go func() { first <- e.Execute(t.Context(), namedRequest("hold", "hold"), nil) }()
	receive(t, running)
	var due atomic.Int64
	due.Store(100)
	var callbacks atomic.Int32
	result := make(chan synthetic.Execution, 1)
	r := namedRequest("scheduled", "scheduled")
	r.VnodeGUID = "11111111-1111-4111-8111-111111111111"
	go func() {
		result <- e.ExecuteScheduled(t.Context(), r, nil, func(at time.Time) synthetic.Schedule {
			callbacks.Add(1)
			assert.Positive(t, at.UnixMicro())
			return synthetic.Schedule{DueUS: due.Load(), QueuedUS: 50}
		})
	}()
	queued(t, e, "scheduled", 1)
	due.Store(200)
	close(release)
	require.Equal(t, synthetic.Success, receive(t, first).Run.Outcome)
	run := receive(t, result).Run
	require.Equal(t, synthetic.Success, run.Outcome)
	assert.Equal(t, int32(1), callbacks.Load())
	assert.Equal(t, int64(200), run.ScheduledUS)
	assert.Equal(t, int64(50), run.QueuedUS)
	assert.Equal(t, "11111111-1111-4111-8111-111111111111", run.VnodeGUID)
	history.mu.Lock()
	defer history.mu.Unlock()
	for _, saved := range history.runs {
		if saved.JobID == r.JobID() {
			assert.Equal(t, run.ScheduledUS, saved.ScheduledUS)
			assert.Equal(t, run.QueuedUS, saved.QueuedUS)
			assert.Equal(t, run.StartedUS, saved.StartedUS)
		}
	}
}

func TestScheduledCancellationDoesNotAdmitAndInfrastructureErrorsAreRecorded(t *testing.T) {
	e, history, _ := testEngine(t)
	var callbacks int
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	got := e.ExecuteScheduled(cancelled, journey(), nil, func(time.Time) synthetic.Schedule { callbacks++; return synthetic.Schedule{DueUS: 1, QueuedUS: 1} })
	assert.Equal(t, synthetic.Cancelled, got.Run.Outcome)
	assert.Zero(t, callbacks)
	assert.Empty(t, history.runs)
	e.source = sourceFunc(func() (Use, error) { return nil, context.DeadlineExceeded })
	r := journey()
	r.VnodeGUID = "11111111-1111-4111-8111-111111111111"
	got = e.ExecuteScheduled(t.Context(), r, nil, func(time.Time) synthetic.Schedule { callbacks++; return synthetic.Schedule{DueUS: 2, QueuedUS: 1} })
	require.True(t, got.Drained)
	assert.Equal(t, synthetic.Error, got.Run.Outcome)
	assert.Positive(t, got.Run.StartedUS)
	assert.Equal(t, 1, callbacks)
	assert.Equal(t, int64(2), got.Run.ScheduledUS)
	assert.Equal(t, "11111111-1111-4111-8111-111111111111", got.Run.VnodeGUID)
	assert.Equal(t, []string{"start", "complete"}, history.phases)
	assert.Zero(t, history.runs[0].CompletedUS)
	assert.Positive(t, history.runs[1].CompletedUS)
}

func TestUnverifiedDrainPoisonsAllSlotsWithoutReleasingUnknownAttempt(t *testing.T) {
	e, _, _ := testEngineSlots(t, 2)
	started := make(chan string, 3)
	releaseBad, releaseGood := make(chan struct{}), make(chan struct{})
	e.start = starter(t, func(ctx context.Context, args []string, raw []byte, out, stderr *os.File) ndexec.TreeResult {
		var r wireRequest
		if err := json.Unmarshal(raw, &r); err != nil {
			return ndexec.TreeResult{Drained: true, Err: err}
		}
		started <- r.Script
		if r.Script == "bad" {
			<-releaseBad
			return ndexec.TreeResult{Drained: false, Err: errors.New("unknown descendant")}
		}
		<-releaseGood
		return resultPayload(success())(ctx, args, raw, out, stderr)
	})
	bad, good, waiting := make(chan synthetic.Execution, 1), make(chan synthetic.Execution, 1), make(chan synthetic.Execution, 1)
	go func() { bad <- e.Execute(t.Context(), namedRequest("bad", "bad"), nil) }()
	require.Equal(t, "bad", receive(t, started))
	go func() { good <- e.Execute(t.Context(), namedRequest("good", "good"), nil) }()
	require.Equal(t, "good", receive(t, started))
	go func() { waiting <- e.Execute(t.Context(), namedRequest("waiting", "must-not-start"), nil) }()
	queued(t, e, "waiting", 1)
	close(releaseBad)
	assert.False(t, receive(t, bad).Drained)
	assert.False(t, receive(t, waiting).Drained)
	assert.ErrorIs(t, receive(t, e.Fatal()), ndexec.ErrTreeNotDrained)
	close(releaseGood)
	require.Equal(t, synthetic.Success, receive(t, good).Run.Outcome)
	assert.False(t, e.Execute(t.Context(), namedRequest("future", "must-not-start"), nil).Drained)
	e.mu.Lock()
	assert.Equal(t, 1, e.active)
	assert.Len(t, e.jobs, 1)
	assert.Zero(t, e.ready.Len())
	e.mu.Unlock()
}
