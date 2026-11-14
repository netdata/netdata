// SPDX-License-Identifier: GPL-3.0-or-later

package functions_test

import (
	"context"
	"encoding/json"
	"maps"
	"strconv"
	"testing"
	"time"

	model "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/monitor"
	syntheticregistry "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// steppedExecutor holds one attempt at each executor transition until the test releases it.
type steppedExecutor struct {
	queued, running chan struct{}
	admit           chan struct{}
	finish          chan model.Run
	// schedule is what admission returned; written before running is signalled.
	schedule model.Schedule
}

func newSteppedExecutor() *steppedExecutor {
	return &steppedExecutor{
		queued:  make(chan struct{}, 1),
		running: make(chan struct{}, 1),
		admit:   make(chan struct{}),
		finish:  make(chan model.Run),
	}
}

func (e *steppedExecutor) ExecuteScheduled(
	ctx context.Context,
	request model.Request,
	state func(model.JobState),
	admitted func(time.Time) model.Schedule,
) model.Execution {
	state(model.JobWaiting)
	e.queued <- struct{}{}
	select {
	case <-e.admit:
	case <-ctx.Done():
		return model.Execution{
			Drained: true,
			Run: model.Run{
				Outcome:     model.Cancelled,
				CompletedUS: time.Now().UnixMicro(),
			},
		}
	}
	schedule := admitted(time.Now())
	e.schedule = schedule
	state(model.JobRunning)
	e.running <- struct{}{}
	select {
	case run := <-e.finish:
		run.JobID, run.Kind, run.Name, run.VnodeGUID = request.JobID(), request.Kind, request.Name, request.VnodeGUID
		run.ScheduledUS, run.QueuedUS = schedule.DueUS, schedule.QueuedUS
		return model.Execution{
			Drained: true,
			Run:     run,
		}
	case <-ctx.Done():
		// A test that stops early must not leave the session waiting for this attempt.
		return model.Execution{
			Drained: true,
			Run: model.Run{
				Outcome:     model.Cancelled,
				CompletedUS: time.Now().UnixMicro(),
			},
		}
	}
}

// activeJob runs one monitor session registered in hub until the test ends.
type activeJob struct {
	executor *steppedExecutor
	session  *monitor.Session
	stop     func()
}

func startActiveJob(t *testing.T, hub *syntheticregistry.Registry, req model.Request, target string) *activeJob {
	t.Helper()
	executor := newSteppedExecutor()
	session, err := monitor.New(req, 900*time.Second, executor)
	require.NoError(t, err)
	retire, err := hub.Register(session, target)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- session.Run(ctx) }()
	stopped := false
	stop := func() {
		if stopped {
			return
		}
		stopped = true
		cancel()
		require.NoError(t, <-done)
		retire()
	}
	t.Cleanup(stop)
	return &activeJob{
		executor: executor,
		session:  session,
		stop:     stop,
	}
}

func receiveSignal(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("executor did not reach the expected transition")
	}
}

// checksRows returns synthetics-checks rows keyed by job ID, with column names as keys.
func checksRows(t *testing.T, src *source) map[string]map[string]any {
	t.Helper()
	got := call(newHandler(src), "synthetics-checks")
	validate(t, got)
	out := make(map[string]map[string]any)
	for _, row := range responseData(t, got) {
		values := rowValues(t, got, row)
		out[values["job_id"].(string)] = values
	}
	return out
}

// takeSeconds removes a nondeterministic age or delay cell after checking it is a non-negative number.
func takeSeconds(t *testing.T, row map[string]any, column string) {
	t.Helper()
	value, ok := row[column].(float64)
	require.True(t, ok, "%s is %#v", column, row[column])
	assert.GreaterOrEqual(t, value, 0.0, column)
	delete(row, column)
}

// The checks table and active-first run detail follow a live monitor session through queueing, admission,
// completion, staleness and vnode reassignment.
func TestChecksAndRunFollowActiveSessions(t *testing.T) {
	hub := syntheticregistry.New()
	src := &source{
		active: hub,
	}
	audit := startActiveJob(t, hub, model.Request{
		Kind:    model.Lighthouse,
		Name:    "home",
		URL:     "https://example.org/",
		Timeout: time.Minute,
	}, "https://example.org/")
	checkoutRequest := model.Request{
		Kind:    model.Journey,
		Name:    "checkout",
		Script:  "export {};",
		Timeout: time.Minute,
	}
	checkout := startActiveJob(t, hub, checkoutRequest, "")
	receiveSignal(t, audit.executor.queued)
	receiveSignal(t, checkout.executor.queued)

	idle := map[string]any{
		"fresh":            false,
		"run_id":           "",
		"outcome":          "",
		"completed_us":     nil,
		"duration_ms":      nil,
		"last_success_us":  nil,
		"last_failure_us":  nil,
		"error":            "",
		"history_error":    "",
		"vnode_guid":       "",
		"check_every":      900,
		"scheduled":        uint64(1),
		"coalesced":        uint64(0),
		"scheduling_delay": nil,
	}
	identity := map[string]map[string]any{
		"lighthouse:home": {
			"job_id": "lighthouse:home",
			"kind":   model.Lighthouse,
			"name":   "home",
			"target": "https://example.org/",
		},
		"journey:checkout": {
			"job_id": "journey:checkout",
			"kind":   model.Journey,
			"name":   "checkout",
			"target": "",
		},
	}
	// expect builds a row; after admission the nondeterministic scheduling delay is checked by takeSeconds instead.
	expect := func(jobID string, values map[string]any) map[string]any {
		out := maps.Clone(idle)
		maps.Copy(out, identity[jobID])
		maps.Copy(out, values)
		if out["admitted"] != uint64(0) {
			delete(out, "scheduling_delay")
		}
		return out
	}

	rows := checksRows(t, src)
	for jobID := range identity {
		takeSeconds(t, rows[jobID], "queue_age")
	}
	waiting := map[string]any{
		"state":    "waiting",
		"admitted": uint64(0),
		"pending":  true,
	}
	assert.Equal(t, map[string]map[string]any{
		"lighthouse:home":  expect("lighthouse:home", waiting),
		"journey:checkout": expect("journey:checkout", waiting),
	}, rows)

	audit.executor.admit <- struct{}{}
	checkout.executor.admit <- struct{}{}
	receiveSignal(t, audit.executor.running)
	receiveSignal(t, checkout.executor.running)
	rows = checksRows(t, src)
	running := map[string]any{
		"state":     "running",
		"admitted":  uint64(1),
		"pending":   false,
		"queue_age": nil,
	}
	for jobID := range identity {
		takeSeconds(t, rows[jobID], "scheduling_delay")
	}
	assert.Equal(t, map[string]map[string]any{
		"lighthouse:home":  expect("lighthouse:home", running),
		"journey:checkout": expect("journey:checkout", running),
	}, rows)

	// An old failed audit is stale; a current successful journey is fresh.
	old := time.Now().Add(-time.Hour)
	audit.executor.finish <- model.Run{
		ID:           "audit-run",
		Outcome:      model.Failed,
		StartedUS:    old.UnixMicro(),
		CompletedUS:  old.Add(time.Second).UnixMicro(),
		DurationMS:   new(1250.5),
		Error:        "performance budget failed",
		HistoryError: "journal write failed",
		Events:       []model.Event{},
		Artifacts:    []model.Artifact{},
	}
	now := time.Now()
	checkout.executor.finish <- model.Run{
		ID:          "checkout-run",
		Outcome:     model.Success,
		StartedUS:   now.UnixMicro(),
		CompletedUS: now.UnixMicro(),
		DurationMS:  new(10.0),
		Events: []model.Event{
			{
				Kind:    "test_end",
				TestID:  "t1",
				Status:  "passed",
				Message: "ok",
			},
		},
	}
	require.Eventually(t, func() bool {
		rows := checksRows(t, src)
		return rows["lighthouse:home"]["run_id"] == "audit-run" && rows["journey:checkout"]["run_id"] == "checkout-run"
	}, 5*time.Second, time.Millisecond)
	rows = checksRows(t, src)
	for jobID := range identity {
		takeSeconds(t, rows[jobID], "scheduling_delay")
	}
	done := map[string]any{
		"admitted":  uint64(1),
		"pending":   false,
		"queue_age": nil,
	}
	failed := expect("lighthouse:home", done)
	maps.Copy(failed, map[string]any{
		"state":           "stale",
		"run_id":          "audit-run",
		"outcome":         "failed",
		"completed_us":    old.Add(time.Second).UnixMicro(),
		"duration_ms":     1250.5,
		"last_failure_us": old.Add(time.Second).UnixMicro(),
		"error":           "performance budget failed",
		"history_error":   "journal write failed",
	})
	succeeded := expect("journey:checkout", done)
	maps.Copy(succeeded, map[string]any{
		"state":           "success",
		"fresh":           true,
		"run_id":          "checkout-run",
		"outcome":         "success",
		"completed_us":    now.UnixMicro(),
		"duration_ms":     10.0,
		"last_success_us": now.UnixMicro(),
	})
	assert.Equal(t, map[string]map[string]any{
		"lighthouse:home":  failed,
		"journey:checkout": succeeded,
	}, rows)

	// Active-first detail serves the latest run without a history read, in its stored shape.
	got := call(newHandler(src), "synthetics-run", "job_id:lighthouse:home", "run_id:audit-run")
	assert.Empty(t, responseData(t, got))
	detail, err := json.Marshal(got.RawResponse["run"])
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"id": "audit-run",
		"job_id": "lighthouse:home",
		"vnode_guid": "",
		"kind": "lighthouse",
		"name": "home",
		"target": "",
		"started_us": `+jsonInt(old.UnixMicro())+`,
		"scheduled_us": `+jsonInt(audit.executor.schedule.DueUS)+`,
		"queued_us": `+jsonInt(audit.executor.schedule.QueuedUS)+`,
		"completed_us": `+jsonInt(old.Add(time.Second).UnixMicro())+`,
		"duration_ms": 1250.5,
		"outcome": "failed",
		"termination_reason": "",
		"error": "performance budget failed",
		"tests": null,
		"capture_state": "",
		"artifacts": [],
		"captures": null,
		"history_error": "journal write failed",
		"dropped_events": 0,
		"metrics": null,
		"runtime": null,
		"failure_evidence": null,
		"browser_summary": null,
		"browser_summary_status": "not_recorded",
		"failure_evidence_status": "not_recorded"
	}`, string(detail))
	got = call(newHandler(src), "synthetics-run", "job_id:journey:checkout", "run_id:checkout-run")
	assert.Equal(t, [][]any{{int64(0), "test_end", "t1", "", "", "passed", "", nil, "ok"}}, responseData(t, got))
	assert.Zero(t, src.getCalls, "active runs need no history read")
	assert.Equal(t, 404, call(newHandler(src), "synthetics-run", "job_id:journey:checkout", "run_id:audit-run").Status)
	assert.Equal(t, 1, src.getCalls, "another job's run ID falls back to history")

	// A vnode reassignment clears the current observation; occurrence counters continue.
	guid := "11111111-1111-4111-8111-111111111111"
	audit.session.SetVnodeGUID(guid)
	rows = checksRows(t, src)
	takeSeconds(t, rows["lighthouse:home"], "scheduling_delay")
	moved := expect("lighthouse:home", done)
	moved["state"] = "unknown"
	moved["vnode_guid"] = guid
	assert.Equal(t, moved, rows["lighthouse:home"])
	assert.Equal(t, 404, call(newHandler(src), "synthetics-run", "job_id:lighthouse:home", "run_id:audit-run").Status)
	assert.Equal(t, 2, src.getCalls, "a cleared observation falls back to history")

	// Retirement removes the rows; a successor with the same job ID starts without the prior observation.
	audit.stop()
	checkout.stop()
	assert.Empty(t, checksRows(t, src))
	successor := startActiveJob(t, hub, checkoutRequest, "")
	receiveSignal(t, successor.executor.queued)
	rows = checksRows(t, src)
	takeSeconds(t, rows["journey:checkout"], "queue_age")
	assert.Equal(t, map[string]map[string]any{
		"journey:checkout": expect("journey:checkout", waiting),
	}, rows)
	assert.Equal(t, 404, call(newHandler(src), "synthetics-run", "job_id:journey:checkout", "run_id:checkout-run").Status)
	assert.Equal(t, 3, src.getCalls, "the predecessor's run is not served as active")
}

func jsonInt(v int64) string { return strconv.FormatInt(v, 10) }
