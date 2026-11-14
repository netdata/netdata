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

// TestRehearsalSharesMonitoringAdmission runs a job's rehearsal only after its
// monitoring attempt, even while another slot is free.
func TestRehearsalSharesMonitoringAdmission(t *testing.T) {
	e, _, _ := testEngineSlots(t, 2)
	started := make(chan string, 3)
	releases := map[string]chan struct{}{"a-monitor": make(chan struct{}), "a-test": make(chan struct{}), "b": make(chan struct{})}
	e.start = starter(t, func(ctx context.Context, args []string, raw []byte, out, stderr *os.File) ndexec.TreeResult {
		var r wireRequest
		if err := json.Unmarshal(raw, &r); err != nil {
			return ndexec.TreeResult{Drained: true, Err: err}
		}
		started <- r.Script
		<-releases[r.Script]
		return resultPayload(success())(ctx, args, raw, out, stderr)
	})
	monitoring := make(chan synthetic.Execution, 1)
	go func() { monitoring <- e.execute(t.Context(), namedRequest("a", "a-monitor")) }()
	require.Equal(t, "a-monitor", receive(t, started))
	rehearsal := make(chan error, 1)
	go func() { rehearsal <- e.Rehearse(t.Context(), namedRequest("a", "a-test")) }()
	waitQueued(t, e, "journey:a", 1)
	other := make(chan synthetic.Execution, 1)
	go func() { other <- e.execute(t.Context(), namedRequest("b", "b")) }()
	require.Equal(t, "b", receive(t, started), "the free slot goes to another job")
	close(releases["b"])
	require.Equal(t, synthetic.Success, receive(t, other).Run.Outcome)
	close(releases["a-monitor"])
	require.Equal(t, synthetic.Success, receive(t, monitoring).Run.Outcome)
	require.Equal(t, "a-test", receive(t, started))
	close(releases["a-test"])
	require.NoError(t, receive(t, rehearsal))
}

// TestQueuedCancellationRecordsNothing withdraws a waiting request: it never
// starts, records no history and leaves later requests in order.
func TestQueuedCancellationRecordsNothing(t *testing.T) {
	e, history, _ := testEngine(t)
	started := make(chan string, 2)
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
	done := make(chan synthetic.Execution, 2)
	go func() { done <- e.execute(t.Context(), namedRequest("hold", "hold")) }()
	require.Equal(t, "hold", receive(t, started))
	ctx, cancel := context.WithCancel(t.Context())
	cancelled := make(chan synthetic.Execution, 1)
	go func() { cancelled <- e.execute(ctx, namedRequest("cancel", "must-not-start")) }()
	waitQueued(t, e, "journey:cancel", 1)
	go func() { done <- e.execute(t.Context(), namedRequest("b", "b")) }()
	waitQueued(t, e, "journey:b", 1)
	cancel()
	result := receive(t, cancelled)
	assert.Equal(t, synthetic.Cancelled, result.Run.Outcome)
	assert.Equal(t, synthetic.CallerCancelled, result.Run.TerminationReason)
	assert.Zero(t, result.Run.StartedUS)
	close(release)
	require.Equal(t, "b", receive(t, started))
	for range 2 {
		require.Equal(t, synthetic.Success, receive(t, done).Run.Outcome)
	}
	history.mu.Lock()
	defer history.mu.Unlock()
	for _, run := range history.runs {
		assert.NotEqual(t, "journey:cancel", run.JobID)
	}
	assert.Len(t, history.runs, 4)
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
	go func() { first <- e.execute(t.Context(), namedRequest("hold", "hold")) }()
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
	waitQueued(t, e, "journey:scheduled", 1)
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
	var phases []string
	for i, saved := range history.runs {
		if saved.JobID == r.JobID() {
			phases = append(phases, history.phases[i])
			assert.Equal(t, r.VnodeGUID, saved.VnodeGUID)
			assert.Equal(t, run.ScheduledUS, saved.ScheduledUS)
			assert.Equal(t, run.QueuedUS, saved.QueuedUS)
			assert.Equal(t, run.StartedUS, saved.StartedUS)
		}
	}
	assert.Equal(t, []string{"start", "complete"}, phases)
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
	go func() { bad <- e.execute(t.Context(), namedRequest("bad", "bad")) }()
	require.Equal(t, "bad", receive(t, started))
	go func() { good <- e.execute(t.Context(), namedRequest("good", "good")) }()
	require.Equal(t, "good", receive(t, started))
	go func() { waiting <- e.execute(t.Context(), namedRequest("waiting", "must-not-start")) }()
	waitQueued(t, e, "journey:waiting", 1)
	close(releaseBad)
	assert.False(t, receive(t, bad).Drained)
	assert.False(t, receive(t, waiting).Drained)
	assert.ErrorIs(t, receive(t, e.Fatal()), ndexec.ErrTreeNotDrained)
	close(releaseGood)
	require.Equal(t, synthetic.Success, receive(t, good).Run.Outcome)
	assert.False(t, e.execute(t.Context(), namedRequest("future", "must-not-start")).Drained)
	// The unknown attempt keeps its slot; nothing else is queued.
	e.mu.Lock()
	assert.Equal(t, 1, e.active)
	assert.Len(t, e.jobs, 1)
	assert.Empty(t, e.ready)
	e.mu.Unlock()
}

// stopAfterGrant reports no error to acquire's first check and a cancellation
// afterwards; its Done channel never fires, so the slot is granted first.
type stopAfterGrant struct {
	context.Context
	calls atomic.Int32
}

func (c *stopAfterGrant) Err() error {
	if c.calls.Add(1) == 1 {
		return nil
	}
	return context.Canceled
}

func (c *stopAfterGrant) Done() <-chan struct{} { return nil }

// TestAdmissionReleasesSlotGrantedAfterStop releases a slot whose grant raced
// the caller's stop: the attempt is cancelled before it starts.
func TestAdmissionReleasesSlotGrantedAfterStop(t *testing.T) {
	e, history, _ := testEngine(t)
	e.start = func(context.Context, ndexec.TreeOptions, string, ...string) (treeProcess, error) {
		t.Error("a stopped caller launched")
		return nil, errors.New("unexpected launch")
	}
	got := e.execute(&stopAfterGrant{Context: t.Context()}, journey())
	assert.Equal(t, synthetic.Cancelled, got.Run.Outcome)
	assert.Equal(t, synthetic.CallerCancelled, got.Run.TerminationReason)
	assert.Zero(t, got.Run.StartedUS)
	assert.Empty(t, history.runs)
	requireSlotFree(t, e)
}

// TestAdmissionReleasesSlotGrantedAfterPoison never keeps a slot granted to a
// request that the poisoned owner then refuses, whichever way the grant and
// the poison race.
func TestAdmissionReleasesSlotGrantedAfterPoison(t *testing.T) {
	for range 20 {
		e, _, _ := testEngine(t)
		ctx := &poisonOnWait{Context: t.Context(), poison: func() { e.poison(errors.New("unknown descendant")) }}
		got := e.execute(ctx, journey())
		assert.False(t, got.Drained)
		e.mu.Lock()
		assert.Zero(t, e.active, "the refused request kept its slot")
		e.mu.Unlock()
	}
}

// poisonOnWait poisons the owner when acquire starts waiting, after the slot
// was already granted under the owner's lock.
type poisonOnWait struct {
	context.Context
	poison func()
}

func (c *poisonOnWait) Done() <-chan struct{} {
	c.poison()
	return nil
}
