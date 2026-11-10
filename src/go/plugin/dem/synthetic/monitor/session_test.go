// SPDX-License-Identifier: GPL-3.0-or-later

package monitor

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type controlledCall struct {
	request   synthetic.Request
	admission chan time.Time
	schedule  chan synthetic.Schedule
	finish    chan synthetic.Execution
	cancelled chan struct{}
}

type controlledExecutor struct {
	calls chan *controlledCall
}

func (e *controlledExecutor) ExecuteScheduled(
	ctx context.Context,
	request synthetic.Request,
	state func(string),
	admitted func(time.Time) synthetic.Schedule,
) synthetic.Execution {
	c := &controlledCall{
		request:   request,
		admission: make(chan time.Time),
		schedule:  make(chan synthetic.Schedule, 1),
		finish:    make(chan synthetic.Execution),
		cancelled: make(chan struct{}),
	}
	state("waiting")
	e.calls <- c
	select {
	case now := <-c.admission:
		c.schedule <- admitted(now)
		state("running")
	case <-ctx.Done():
		close(c.cancelled)
		return <-c.finish
	case result := <-c.finish:
		return result
	}
	select {
	case <-ctx.Done():
		close(c.cancelled)
		return <-c.finish
	case result := <-c.finish:
		return result
	}
}

type recordedObserver struct {
	mu     sync.Mutex
	runs   []synthetic.Run
	status Status
	wake   chan struct{}
}

func (o *recordedObserver) SetState(string)     {}
func (o *recordedObserver) SetVnodeGUID(string) {}
func (o *recordedObserver) SetSchedule(status Status) {
	o.mu.Lock()
	o.status = status
	o.mu.Unlock()
	select {
	case o.wake <- struct{}{}:
	default:
	}
}
func (o *recordedObserver) Complete(run synthetic.Run) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.runs = append(o.runs, run)
}

type monitorHarness struct {
	session  *Session
	executor *controlledExecutor
	observer *recordedObserver
	cancel   context.CancelFunc
	done     chan error
	tick     chan time.Time
	reset    chan time.Duration
	mu       sync.Mutex
	now      time.Time
}

func request() synthetic.Request {
	return synthetic.Request{
		Kind:    synthetic.Journey,
		Name:    "test",
		Script:  "test()",
		Timeout: time.Minute,
	}
}

func newHarness(t *testing.T, req synthetic.Request) *monitorHarness {
	t.Helper()
	h := &monitorHarness{
		executor: &controlledExecutor{
			calls: make(chan *controlledCall, 10),
		},
		observer: &recordedObserver{
			wake: make(chan struct{}, 1),
		},
		done:  make(chan error, 1),
		tick:  make(chan time.Time),
		reset: make(chan time.Duration, 10),
		now:   time.Unix(1700000000, 0),
	}
	var err error
	h.session, err = New(req, 10*time.Second, h.executor, h.observer)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel
	go func() {
		h.done <- h.session.run(ctx, h.Now, h.tick, func(d time.Duration) {
			select {
			case h.reset <- d:
			default:
			}
		})
	}()
	t.Cleanup(cancel)
	return h
}

func (h *monitorHarness) Now() time.Time {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.now
}

func (h *monitorHarness) setNow(now time.Time) {
	h.mu.Lock()
	h.now = now
	h.mu.Unlock()
}

func (h *monitorHarness) advance(t *testing.T, now time.Time) {
	t.Helper()
	h.setNow(now)
	select {
	case h.tick <- now:
	case <-time.After(5 * time.Second):
		t.Fatal("monitor did not accept timer wakeup")
	}
}

func receive[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for monitor transition")
		var zero T
		return zero
	}
}

func (h *monitorHarness) wait(t *testing.T, predicate func(Snapshot) bool) Snapshot {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		snapshot := h.session.Snapshot(h.Now())
		if predicate(snapshot) {
			return snapshot
		}
		select {
		case <-h.observer.wake:
		case <-deadline:
			t.Fatalf("monitor did not reach expected state: %+v", snapshot)
		}
	}
}

func (h *monitorHarness) stop(t *testing.T, call *controlledCall) {
	t.Helper()
	h.cancel()
	if call != nil {
		receive(t, call.cancelled)
		call.finish <- synthetic.Execution{
			Run: synthetic.Run{
				Outcome:     synthetic.Cancelled,
				CompletedUS: h.Now().UnixMicro(),
			},
			Drained: true,
		}
	}
	require.NoError(t, receive(t, h.done))
}

func TestQueuedLatestOccurrencePreservesAdmissionTurn(t *testing.T) {
	h := newHarness(t, request())
	bootstrap := h.Now()
	call := receive(t, h.executor.calls)
	firstDelay := receive(t, h.reset)
	require.GreaterOrEqual(t, firstDelay, 10*time.Second)
	require.Less(t, firstDelay, 20*time.Second)
	firstDue := bootstrap.Add(firstDelay)

	// One timer delivery represents a million elapsed periods while the first
	// executor is still queued. No missed-period goroutines or requeue occur.
	latestDue := firstDue.Add(999999 * 10 * time.Second)
	h.advance(t, latestDue)
	snapshot := h.wait(t, func(s Snapshot) bool { return s.Scheduled == 1000001 })
	assert.Equal(t, uint64(1000000), snapshot.Coalesced)
	assert.Equal(t, bootstrap.UnixMicro(), snapshot.QueuedUS)
	assert.Equal(t, latestDue.UnixMicro(), snapshot.PendingDueUS)
	assert.Equal(t, latestDue.Sub(bootstrap), snapshot.QueueAge)
	assert.False(t, snapshot.HasObservation)
	assert.False(t, snapshot.Fresh)
	assert.Empty(t, h.executor.calls)

	// Admission itself must account for a grid point whose timer notification
	// has not arrived yet, and consume only the latest pending occurrence.
	admitted := latestDue.Add(10 * time.Second)
	h.setNow(admitted)
	call.admission <- admitted
	assert.Equal(
		t,
		synthetic.Schedule{
			DueUS:    admitted.UnixMicro(),
			QueuedUS: bootstrap.UnixMicro(),
		},
		receive(t, call.schedule),
	)
	snapshot = h.session.Snapshot(admitted)
	assert.Equal(t, uint64(1000002), snapshot.Scheduled)
	assert.Equal(t, uint64(1000001), snapshot.Coalesced)
	assert.Equal(t, uint64(1), snapshot.Admitted)
	assert.False(t, snapshot.Pending)
	assert.True(t, snapshot.Active)
	assert.Equal(t, bootstrap.UnixMicro(), snapshot.ActiveQueuedUS)
	assert.Zero(t, snapshot.SchedulingDelay)
	h.stop(t, call)
}

func TestActiveCoalescingAndEveryCompletion(t *testing.T) {
	h := newHarness(t, request())
	activation := h.Now()
	first := receive(t, h.executor.calls)
	firstDue := activation.Add(receive(t, h.reset))
	first.admission <- activation
	receive(t, first.schedule)
	h.advance(t, firstDue.Add(2*10*time.Second))
	snapshot := h.wait(t, func(s Snapshot) bool { return s.Scheduled == 4 })
	assert.Equal(t, uint64(2), snapshot.Coalesced)
	assert.True(t, snapshot.Active)
	assert.True(t, snapshot.Pending)
	assert.Zero(t, snapshot.QueuedUS)
	assert.Empty(t, h.executor.calls)

	first.finish <- synthetic.Execution{
		Run: synthetic.Run{
			ID:          "first",
			Outcome:     synthetic.Failed,
			CompletedUS: h.Now().UnixMicro(),
		},
		Drained: true,
	}
	second := receive(t, h.executor.calls)
	snapshot = h.session.Snapshot(h.Now())
	assert.Equal(t, Outcomes{
		Failed: 1,
	}, snapshot.Outcomes)
	assert.Equal(t, "first", snapshot.Latest.ID)
	assert.True(t, snapshot.Fresh)
	assert.Equal(t, h.Now().UnixMicro(), snapshot.QueuedUS)
	second.admission <- h.Now().Add(time.Second)
	schedule := receive(t, second.schedule)
	assert.Equal(t, firstDue.Add(2*10*time.Second).UnixMicro(), schedule.DueUS)
	assert.Equal(t, time.Second, h.session.Snapshot(h.Now()).SchedulingDelay)

	duration, performance := 12.5, 0.0
	run := synthetic.Run{
		ID:          "second",
		Outcome:     synthetic.Success,
		CompletedUS: h.Now().UnixMicro(),
		DurationMS:  &duration,
		Tests: &synthetic.TestCounts{
			Declared: 1,
			Passed:   1,
		},
		Metrics: &synthetic.LabMetrics{
			Performance: &performance,
		},
		Events: []synthetic.Event{{Kind: "test"}},
	}
	second.finish <- synthetic.Execution{
		Run:     run,
		Drained: true,
	}
	snapshot = h.wait(t, func(s Snapshot) bool { return s.Outcomes.Success == 1 })
	assert.Equal(t, Outcomes{
		Success: 1,
		Failed:  1,
	}, snapshot.Outcomes)
	assert.Equal(t, Observation{
		ID:          "second",
		Outcome:     synthetic.Success,
		CompletedUS: run.CompletedUS,
		DurationMS: Scalar{
			Value:   12.5,
			Present: true,
		},
		Tests: synthetic.TestCounts{
			Declared: 1,
			Passed:   1,
		},
		HasTests: true,
		Metrics: LabMetrics{
			Performance: Scalar{
				Value:   0,
				Present: true,
			},
		},
		HasMetrics: true,
	}, snapshot.Latest)
	assert.False(t, snapshot.Latest.Metrics.LCPMS.Present)
	h.observer.mu.Lock()
	assert.Len(t, h.observer.runs, 2)
	assert.Equal(t, snapshot.Outcomes, h.observer.status.Outcomes)
	h.observer.mu.Unlock()
	h.stop(t, nil)
}

func TestStopJoinsQueuedAndRunningExecution(t *testing.T) {
	for name, admitted := range map[string]bool{"queued": false, "running": true} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, request())
			call := receive(t, h.executor.calls)
			if admitted {
				call.admission <- h.Now()
				receive(t, call.schedule)
			}
			h.cancel()
			receive(t, call.cancelled)
			select {
			case <-h.done:
				t.Fatal("Run returned before executor joined")
			default:
			}
			call.finish <- synthetic.Execution{
				Run: synthetic.Run{
					Outcome:     synthetic.Cancelled,
					CompletedUS: h.Now().UnixMicro(),
				},
				Drained: true,
			}
			require.NoError(t, receive(t, h.done))
			snapshot := h.session.Snapshot(h.Now())
			assert.False(t, snapshot.Pending)
			assert.False(t, snapshot.Active)
			assert.Zero(t, snapshot.QueuedUS)
			assert.Equal(t, admitted, snapshot.HasObservation)
			if admitted {
				assert.Equal(t, Outcomes{
					Cancelled: 1,
				}, snapshot.Outcomes)
			} else {
				assert.Equal(t, Outcomes{}, snapshot.Outcomes)
			}
		})
	}
}

func TestUnverifiedResultsDoNotPublishObservation(t *testing.T) {
	for name, execution := range map[string]synthetic.Execution{
		"undrained": {Run: synthetic.Run{
			Outcome:     synthetic.Success,
			CompletedUS: 1700000000000000,
		}},
		"missing completion": {Run: synthetic.Run{
			Outcome: synthetic.Success,
		}, Drained: true},
	} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, request())
			call := receive(t, h.executor.calls)
			call.admission <- h.Now()
			receive(t, call.schedule)
			call.finish <- execution
			require.Error(t, receive(t, h.done))
			assert.False(t, h.session.Snapshot(h.Now()).HasObservation)
			h.observer.mu.Lock()
			assert.Empty(t, h.observer.runs)
			h.observer.mu.Unlock()
		})
	}
}

func TestObservationFreshnessUsesActualCompletion(t *testing.T) {
	h := newHarness(t, request())
	call := receive(t, h.executor.calls)
	call.admission <- h.Now()
	receive(t, call.schedule)
	completed := h.Now()
	call.finish <- synthetic.Execution{
		Run: synthetic.Run{
			Outcome:     synthetic.Error,
			CompletedUS: completed.UnixMicro(),
		},
		Drained: true,
	}
	h.wait(t, func(s Snapshot) bool { return s.HasObservation })
	for name, tc := range map[string]struct {
		age   time.Duration
		fresh bool
	}{
		"completed":    {age: 0, fresh: true},
		"held":         {age: 15 * time.Second, fresh: true},
		"boundary":     {age: 20 * time.Second, fresh: true},
		"stale":        {age: 20*time.Second + time.Microsecond, fresh: false},
		"future clock": {age: -time.Microsecond, fresh: false},
	} {
		t.Run(name, func(t *testing.T) {
			snapshot := h.session.Snapshot(completed.Add(tc.age))
			assert.Equal(t, tc.age, snapshot.ObservationAge)
			assert.Equal(t, tc.fresh, snapshot.Fresh)
			assert.Equal(t, synthetic.Error, snapshot.Latest.Outcome)
			assert.Equal(t, Outcomes{
				Error: 1,
			}, snapshot.Outcomes)
			assert.False(t, snapshot.Latest.DurationMS.Present)
		})
	}
	h.stop(t, nil)
}

func TestSiteAssociationDoesNotChooseScheduleOrExecutionKey(t *testing.T) {
	a, b := request(), request()
	a.VnodeGUID, b.VnodeGUID = "11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222"
	assert.Equal(t, phase(a.JobID(), time.Minute), phase(b.JobID(), time.Minute))
	b.Name = "other"
	assert.NotEqual(t, phase(a.JobID(), time.Minute), phase(b.JobID(), time.Minute))
	h := newHarness(t, a)
	call := receive(t, h.executor.calls)
	assert.Equal(t, a.JobID(), call.request.JobID())
	assert.Equal(t, a.VnodeGUID, call.request.VnodeGUID)
	h.stop(t, call)
}

func TestNewRejectsInvalidInputsAndCopiesSecrets(t *testing.T) {
	e := &controlledExecutor{
		calls: make(chan *controlledCall),
	}
	for name, tc := range map[string]struct {
		request  synthetic.Request
		interval time.Duration
		executor Executor
	}{
		"interval": {request: request(), executor: e},
		"executor": {request: request(), interval: time.Second},
		"request":  {interval: time.Second, executor: e},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := New(tc.request, tc.interval, tc.executor, nil)
			require.Error(t, err)
		})
	}
	req := request()
	req.Secrets = map[string]string{"DEM_SECRET_TEST": "test-value"}
	s, err := New(req, time.Second, e, nil)
	require.NoError(t, err)
	req.Secrets["DEM_SECRET_TEST"] = "changed"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	call := receive(t, e.calls)
	assert.Equal(t, "test-value", call.request.Secrets["DEM_SECRET_TEST"])
	cancel()
	receive(t, call.cancelled)
	call.finish <- synthetic.Execution{
		Drained: true,
	}
	require.NoError(t, receive(t, done))
}

func TestCancelledBeforeStartAndSecondRun(t *testing.T) {
	s, err := New(request(), time.Second, &controlledExecutor{}, nil)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.NoError(t, s.Run(ctx))
	assert.Equal(t, Snapshot{}, s.Snapshot(time.Now()))
	require.ErrorContains(t, s.Run(ctx), "already started")
}

type executorFunc func(context.Context, synthetic.Request, func(string), func(time.Time) synthetic.Schedule) synthetic.Execution

func (f executorFunc) ExecuteScheduled(
	ctx context.Context,
	req synthetic.Request,
	state func(string),
	admitted func(time.Time) synthetic.Schedule,
) synthetic.Execution {
	return f(ctx, req, state, admitted)
}

func TestAdmissionRacingStopRetainsOccurrenceUntilJoin(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	schedules := make(chan synthetic.Schedule, 1)
	executor := executorFunc(
		func(ctx context.Context, _ synthetic.Request, state func(string), admitted func(time.Time) synthetic.Schedule) synthetic.Execution {
			close(entered)
			<-ctx.Done()
			// The owner checked cancellation immediately before invoking admission.
			// Shutdown must retain its selected occurrence until this call joins.
			<-release
			schedules <- admitted(time.Now())
			state("running")
			return synthetic.Execution{
				Run: synthetic.Run{
					Outcome:     synthetic.Cancelled,
					CompletedUS: time.Now().UnixMicro(),
				},
				Drained: true,
			}
		},
	)
	s, err := New(request(), time.Hour, executor, nil)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	receive(t, entered)
	queued := s.Snapshot(time.Now())
	cancel()
	select {
	case <-done:
		t.Fatal("monitor returned before admission callback finished")
	default:
	}
	close(release)
	schedule := receive(t, schedules)
	assert.Equal(t, synthetic.Schedule{
		DueUS:    queued.PendingDueUS,
		QueuedUS: queued.QueuedUS,
	}, schedule)
	require.NoError(t, receive(t, done))
	assert.Equal(t, Outcomes{
		Cancelled: 1,
	}, s.Snapshot(time.Now()).Outcomes)
}

func TestEveryAdmittedOutcomeIsCounted(t *testing.T) {
	for name, tc := range map[string]struct {
		outcome synthetic.Outcome
		counts  Outcomes
	}{
		"success": {outcome: synthetic.Success, counts: Outcomes{
			Success: 1,
		}},
		"failed": {outcome: synthetic.Failed, counts: Outcomes{
			Failed: 1,
		}},
		"timeout": {outcome: synthetic.Timeout, counts: Outcomes{
			Timeout: 1,
		}},
		"inconclusive": {outcome: synthetic.Inconclusive, counts: Outcomes{
			Inconclusive: 1,
		}},
		"error": {outcome: synthetic.Error, counts: Outcomes{
			Error: 1,
		}},
		"cancelled": {outcome: synthetic.Cancelled, counts: Outcomes{
			Cancelled: 1,
		}},
		"unknown": {outcome: synthetic.Unknown, counts: Outcomes{
			Unknown: 1,
		}},
	} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, request())
			call := receive(t, h.executor.calls)
			call.admission <- h.Now()
			receive(t, call.schedule)
			call.finish <- synthetic.Execution{
				Run: synthetic.Run{
					Outcome:     tc.outcome,
					CompletedUS: h.Now().UnixMicro(),
				},
				Drained: true,
			}
			snapshot := h.wait(t, func(s Snapshot) bool { return s.HasObservation })
			assert.Equal(t, tc.counts, snapshot.Outcomes)
			assert.Equal(t, tc.outcome, snapshot.Latest.Outcome)
			h.stop(t, nil)
		})
	}
}

func TestSnapshotCopiesScalarsAndDoesNotAllocate(t *testing.T) {
	h := newHarness(t, request())
	call := receive(t, h.executor.calls)
	call.admission <- h.Now()
	receive(t, call.schedule)
	duration, performance := 10.0, 0.5
	tests := &synthetic.TestCounts{
		Declared: 1,
		Failed:   1,
	}
	run := synthetic.Run{
		Outcome:     synthetic.Failed,
		CompletedUS: h.Now().UnixMicro(),
		DurationMS:  &duration,
		Tests:       tests,
		Metrics: &synthetic.LabMetrics{
			Performance: &performance,
		},
		Events:    make([]synthetic.Event, 500),
		Artifacts: make([]synthetic.Artifact, 100),
	}
	call.finish <- synthetic.Execution{
		Run:     run,
		Drained: true,
	}
	before := h.wait(t, func(s Snapshot) bool { return s.HasObservation })
	duration, performance, tests.Failed = 0, 1, 0
	assert.Equal(t, before, h.session.Snapshot(h.Now()))
	assert.Zero(t, testing.AllocsPerRun(100, func() { h.session.Snapshot(h.Now()) }))
	h.stop(t, nil)
}
