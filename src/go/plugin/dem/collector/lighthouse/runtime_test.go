// SPDX-License-Identifier: GPL-3.0-or-later
package lighthouse_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/lighthouse"
	model "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	syntheticregistry "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/registry"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/jobruntime"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type executor struct {
	check     func(context.Context, model.Kind) error
	execute   func(context.Context, model.Request, func(string)) model.Execution
	scheduled func(context.Context, model.Request, func(string), func(time.Time) model.Schedule) model.Execution
}

func (e executor) Check(ctx context.Context, k model.Kind) error {
	if e.check != nil {
		return e.check(ctx, k)
	}
	return nil
}
func (e executor) Rehearse(context.Context, model.Request) error { panic("unexpected rehearsal") }

func (e executor) ExecuteScheduled(
	ctx context.Context,
	r model.Request,
	state func(string),
	admitted func(time.Time) model.Schedule,
) model.Execution {
	if e.scheduled != nil {
		return e.scheduled(ctx, r, state, admitted)
	}
	state("waiting")
	// The empty fixture represents genuinely queued work, not an admitted run.
	if e.execute == nil {
		<-ctx.Done()
		return model.Execution{
			Drained: true,
			Run: model.Run{
				Outcome:     model.Cancelled,
				CompletedUS: time.Now().UnixMicro(),
			},
		}
	}
	at := time.Now()
	schedule := admitted(at)
	result := e.execute(ctx, r, state)
	run := &result.Run
	if run.ID == "" {
		run.ID = fmt.Sprint(at.UnixNano())
	}
	if run.JobID == "" {
		run.JobID = r.JobID()
	}
	if run.Name == "" {
		run.Name = r.Name
	}
	if run.Kind == "" {
		run.Kind = r.Kind
	}
	run.VnodeGUID = r.VnodeGUID
	if run.StartedUS == 0 {
		run.StartedUS = at.UnixMicro()
	}
	if result.Drained && run.CompletedUS < run.StartedUS {
		run.CompletedUS = time.Now().UnixMicro()
	}
	run.ScheduledUS = schedule.DueUS
	run.QueuedUS = schedule.QueuedUS
	return result
}
func ptr(v float64) *float64 { return &v }
func newLighthouse(e executor, hub *syntheticregistry.Registry) *lighthouse.Collector {
	c := lighthouse.New(lighthouse.Dependencies{
		Executor: e,
		Registry: hub,
	})
	c.Name = "checkout"
	c.URL = "https://example.org/"
	return c
}
func waitLatest(t *testing.T, hub *syntheticregistry.Registry, id string) model.Run {
	t.Helper()
	var run model.Run
	require.Eventually(t, func() bool {
		for _, j := range hub.Snapshot(time.Now()) {
			if j.JobID == id && j.Latest != nil {
				run = *j.Latest
				return true
			}
		}
		return false
	}, 3*time.Second, time.Millisecond)
	return run
}
func start(t *testing.T, c interface {
	Init(context.Context) error
	Run(context.Context, func()) error
}) func() {
	t.Helper()
	require.NoError(t, c.Init(t.Context()))
	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan struct{})
	done := make(chan error, 1)
	go func() { done <- c.Run(ctx, func() { close(ready) }) }()
	select {
	case <-ready:
	case err := <-done:
		t.Fatalf("startup failed: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("startup timed out")
	}
	var once sync.Once
	stop := func() {
		once.Do(func() {
			cancel()
			select {
			case err := <-done:
				assert.NoError(t, err)
			case <-time.After(3 * time.Second):
				t.Error("Run did not join executor")
			}
		})
	}
	t.Cleanup(stop)
	return stop
}
func receive[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(4 * time.Second):
		t.Fatal("owned operation did not finish")
		var zero T
		return zero
	}
}
func collect(t *testing.T, c collectorapi.CollectorV2) map[string]float64 {
	t.Helper()
	values, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
	require.NoError(t, err)
	return values
}

func TestPreparationDoesNotRunOrRegister(t *testing.T) {
	hub := syntheticregistry.New()
	calls := 0
	c := newLighthouse(executor{
		check: func(ctx context.Context, kind model.Kind) error {
			calls++
			assert.Equal(t, model.Lighthouse, kind)
			deadline, ok := ctx.Deadline()
			assert.True(t, ok)
			assert.LessOrEqual(t, time.Until(deadline), 15*time.Second)
			return errors.New("prepared browser missing")
		},
		execute: func(context.Context, model.Request, func(string)) model.Execution {
			t.Error("preparation executed workflow")
			return model.Execution{}
		},
	}, hub)
	require.NoError(t, c.Init(t.Context()))
	assert.Empty(t, hub.Snapshot(time.Now()))
	require.EqualError(t, c.Check(t.Context()), "prepared browser missing")
	assert.Equal(t, 1, calls)
	assert.Empty(t, hub.Snapshot(time.Now()))
	c.Cleanup(t.Context())
	c.Cleanup(t.Context())
}
func TestActiveRegistrationAndReplacement(t *testing.T) {
	hub := syntheticregistry.New()
	current := newLighthouse(executor{}, hub)
	stop := start(t, current)
	jobs := hub.Snapshot(time.Now())
	require.Len(t, jobs, 1)
	assert.Nil(t, jobs[0].Latest)
	assert.Equal(t, 1800, jobs[0].CadenceSeconds)
	candidate := newLighthouse(executor{}, hub)
	require.NoError(t, candidate.Init(t.Context()))
	require.NoError(t, candidate.Check(t.Context()))
	candidate.Cleanup(t.Context())
	assert.Len(t, hub.Snapshot(time.Now()), 1)
	require.Error(t, candidate.Run(t.Context(), func() { t.Error("duplicate became ready") }))
	stop()
	assert.Empty(t, hub.Snapshot(time.Now()))
	stopNext := start(t, candidate)
	current.Cleanup(t.Context())
	assert.Len(t, hub.Snapshot(time.Now()), 1, "old cleanup cannot retire successor")
	stopNext()
	assert.Empty(t, hub.Snapshot(time.Now()))
}
func TestOutcomesAndMissingObservations(t *testing.T) {
	for _, outcome := range []model.Outcome{model.Success, model.Failed, model.Timeout, model.Inconclusive, model.Error, model.Cancelled, model.Unknown} {
		t.Run(string(outcome), func(t *testing.T) {
			hub := syntheticregistry.New()
			evidence := model.Run{
				Outcome: outcome,
				Error:   "fixture teardown failed",
				Events: []model.Event{
					{Kind: "test_end", Phase: "afterAll", Status: "failed", Message: "fixture teardown failed"},
				},
			}
			c := newLighthouse(
				executor{
					execute: func(_ context.Context, r model.Request, state func(string)) model.Execution {
						assert.Equal(t, "checkout", r.Name)
						state("running")
						assert.Equal(t, "running", hub.Snapshot(time.Now())[0].State)
						return model.Execution{
							Drained: true,
							Run:     evidence,
						}
					},
				},
				hub,
			)
			start(t, c)
			latest := waitLatest(t, hub, "lighthouse:checkout")
			got := collect(t, c)
			assert.EqualValues(t, 1, got[`execution_state{execution_state="`+string(outcome)+`"}`])
			assert.EqualValues(t, 1, got["attempts_"+string(outcome)])
			for _, name := range []string{"duration", "tests_passed", "performance"} {
				assert.NotContains(t, got, name)
			}
			assert.Equal(t, evidence.Outcome, latest.Outcome)
			assert.Equal(t, evidence.Error, latest.Error)
			assert.Equal(t, evidence.Events, latest.Events)
			assert.Positive(t, latest.StartedUS)
			assert.GreaterOrEqual(t, latest.CompletedUS, latest.StartedUS)
			collecttest.AssertChartCoverage(
				t,
				c,
				collecttest.ChartCoverageExpectation{
					RequiredContexts: map[string][]string{"dem_synthetic.execution_state": {string(outcome)}},
				},
			)
		})
	}
}
func observation() model.Run {
	return model.Run{
		Outcome:    model.Success,
		DurationMS: ptr(1250.5),
		Tests: &model.TestCounts{
			Declared:        7,
			Passed:          1,
			Failed:          1,
			TimedOut:        1,
			Skipped:         1,
			ExpectedFailure: 1,
			NotRun:          2,
		},
		Metrics: &model.LabMetrics{
			Performance: ptr(0),
			FCPMS:       ptr(100.5),
			LCPMS:       ptr(200.5),
			TBTMS:       ptr(0),
			SIMS:        ptr(300.5),
			CLS:         ptr(0.12),
		},
	}
}
func TestRunExecutesWithoutCollectAndRepeatedCollectionsHoldLatest(t *testing.T) {
	hub := syntheticregistry.New()
	var calls atomic.Int32
	c := newLighthouse(executor{
		execute: func(_ context.Context, r model.Request, state func(string)) model.Execution {
			calls.Add(1)
			state("running")
			assert.Equal(t, 3*time.Minute, r.Timeout)
			assert.Equal(t, "https://example.org/", r.URL)
			assert.True(t, r.Capture)
			assert.Equal(t, model.Lighthouse, r.Kind)
			return model.Execution{
				Drained: true,
				Run:     observation(),
			}
		},
	}, hub)
	c.SaveReport = true
	start(t, c)
	run := waitLatest(t, hub, "lighthouse:checkout")
	assert.Equal(t, int32(1), calls.Load(), "Run must execute bootstrap without any Collect")
	for i := 0; i < 3; i++ {
		got := collect(t, c)
		for key, want := range map[string]float64{"performance": 0, "fcp": 100.5, "lcp": 200.5, "tbt": 0, "si": 300.5, "cls": 0.12, "duration": 1250.5} {
			require.Contains(t, got, key)
			assert.EqualValues(t, want, got[key], key)
		}
		assert.EqualValues(t, 1, got["admitted"])
		assert.EqualValues(t, 1, got["scheduled"])
		assert.Zero(t, got["coalesced"])
		assert.Zero(t, got["active"])
		assert.Zero(t, got["pending"])
		assert.Contains(t, got, "observation_age")
		assert.EqualValues(t, 1, got["attempts_success"])
		assert.NotContains(t, got, "tests_declared")
	}
	collecttest.AssertChartCoverage(
		t,
		c,
		collecttest.ChartCoverageExpectation{
			RequiredContexts: map[string][]string{
				"dem_synthetic.duration":               {"duration"},
				"dem_synthetic.lighthouse.performance": {"performance"},
				"dem_synthetic.lighthouse.timings":     {"fcp", "lcp", "tbt", "speed_index"},
				"dem_synthetic.lighthouse.cls":         {"cls"},
			},
		},
	)
	assert.Equal(t, int32(1), calls.Load(), "publication cannot start more browser work")
	assert.Equal(t, run.ID, hub.Snapshot(time.Now())[0].Latest.ID)
}
func TestLaterObservationWithMissingValuesCreatesGaps(t *testing.T) {
	hub := syntheticregistry.New()
	var calls atomic.Int32
	c := newLighthouse(executor{
		execute: func(context.Context, model.Request, func(string)) model.Execution {
			n := calls.Add(1)
			run := observation()
			run.DurationMS = ptr(0)
			if n == 1 {
				run.Metrics.Performance = ptr(0)
			} else {
				run.Outcome = model.Error
				run.DurationMS = nil
				run.Tests = nil
				run.Metrics = &model.LabMetrics{
					Performance: ptr(math.NaN()),
					LCPMS:       ptr(math.Inf(1)),
				}
			}
			return model.Execution{
				Drained: true,
				Run:     run,
			}
		},
	}, hub)
	c.CheckEvery = 1
	start(t, c)
	waitLatest(t, hub, "lighthouse:checkout")
	got := collect(t, c)
	for _, name := range []string{"performance", "fcp", "lcp", "tbt", "si", "cls", "duration"} {
		require.Contains(t, got, name)
	}
	require.Eventually(t, func() bool {
		jobs := hub.Snapshot(time.Now())
		return len(jobs) == 1 && jobs[0].Latest != nil && jobs[0].Latest.Outcome == model.Error
	}, 3*time.Second, time.Millisecond)
	got = collect(t, c)
	for _, name := range []string{"performance", "fcp", "lcp", "tbt", "si", "cls", "duration"} {
		assert.NotContains(t, got, name)
	}
	assert.EqualValues(t, 1, got[`execution_state{execution_state="error"}`])
	assert.EqualValues(t, 1, got["attempts_error"])
}
func TestNeverObservedAndStaleSnapshotsOmitMeasurementsAndRetainCharts(t *testing.T) {
	t.Run("never observed", func(t *testing.T) {
		c := newLighthouse(executor{}, syntheticregistry.New())
		start(t, c)
		got := collect(t, c)
		assert.EqualValues(t, 1, got[`execution_state{execution_state="unknown"}`])
		assert.NotContains(t, got, "duration")
		assert.NotContains(t, got, "observation_age")
		assert.Zero(t, got["admitted"])
	})
	t.Run("stale", func(t *testing.T) {
		hub := syntheticregistry.New()
		var calls atomic.Int32
		c := newLighthouse(
			executor{
				execute: func(ctx context.Context, _ model.Request, _ func(string)) model.Execution {
					if calls.Add(1) == 1 {
						return model.Execution{
							Drained: true,
							Run:     observation(),
						}
					}
					<-ctx.Done()
					return model.Execution{
						Drained: true,
						Run: model.Run{
							Outcome: model.Cancelled,
						},
					}
				},
			},
			hub,
		)
		c.CheckEvery = 1
		start(t, c)
		waitLatest(t, hub, "lighthouse:checkout")
		fresh := collect(t, c)
		require.Contains(t, fresh, "duration")
		set, err := chartengine.NewTemplateSetYAML([]byte(c.ChartTemplateYAML()))
		require.NoError(t, err)
		engine, err := chartengine.New(chartengine.WithRuntimeStore(nil))
		require.NoError(t, err)
		plan, err := engine.PreparePlanWithOptions(
			c.MetricStore().Read(metrix.ReadFlatten()),
			chartengine.PlanOptions{
				TemplateSet: set,
			},
		)
		require.NoError(t, err)
		require.NoError(t, plan.Commit())
		var stale map[string]float64
		require.Eventually(
			t,
			func() bool { stale = collect(t, c); return stale[`execution_state{execution_state="stale"}`] == 1 },
			4*time.Second,
			10*time.Millisecond,
		)
		assert.NotContains(t, stale, "duration")
		assert.Greater(t, stale["observation_age"], 2.0)
		assert.EqualValues(t, 1, stale["attempts_success"])
		for i := 0; i < 7; i++ {
			got := collect(t, c)
			assert.NotContains(t, got, "duration")
			next, err := engine.PreparePlanWithOptions(
				c.MetricStore().Read(metrix.ReadFlatten()),
				chartengine.PlanOptions{
					TemplateSet: set,
				},
			)
			require.NoError(t, err)
			for _, action := range next.Plan().Actions {
				_, removed := action.(chartengine.RemoveChartAction)
				assert.False(t, removed, "known metric charts survive missing observations")
			}
			require.NoError(t, next.Commit())
		}
	})
}

type output struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (o *output) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.buf.Write(p)
}
func (o *output) String() string { o.mu.Lock(); defer o.mu.Unlock(); return o.buf.String() }
func TestNativeJobExecutionCancellationAndOutput(t *testing.T) {
	hub := syntheticregistry.New()
	entered, cancelled := make(chan struct{}), make(chan struct{})
	var attempts atomic.Int32
	c := newLighthouse(
		executor{
			execute: func(ctx context.Context, _ model.Request, state func(string)) model.Execution {
				state("running")
				if attempts.Add(1) == 1 {
					return model.Execution{
						Drained: true,
						Run: model.Run{
							Outcome:    model.Failed,
							DurationMS: ptr(1250.5),
						},
					}
				}
				close(entered)
				<-ctx.Done()
				close(cancelled)
				return model.Execution{
					Drained: true,
					Run: model.Run{
						Outcome: model.Cancelled,
					},
				}
			},
		},
		hub,
	)
	c.UpdateEvery = 1
	c.CheckEvery = 1
	out := &output{}
	job := jobruntime.NewJobV2(
		jobruntime.JobV2Config{
			PluginName:  "dem",
			ModuleName:  "lighthouse",
			Name:        c.Name,
			FullName:    "lighthouse_" + c.Name,
			Module:      c,
			Out:         out,
			UpdateEvery: 1,
			StoreFirst:  true,
		},
	)
	require.NoError(t, job.AutoDetectionManaged(t.Context()))
	run := jobruntime.NewManagedRun(context.Background(), nil)
	done := make(chan struct{})
	go func() { job.StartManaged(run); close(done) }()
	var once sync.Once
	stop := func() { once.Do(func() { job.Stop(); receive(t, done); job.Cleanup() }) }
	t.Cleanup(stop)
	receive(t, run.StartupDone())
	require.NoError(t, run.StartupErr())
	waitLatest(t, hub, "lighthouse:checkout")
	assert.Positive(t, attempts.Load(), "bootstrap execution completes before requesting a metric tick")
	require.Eventually(
		t,
		func() bool { job.Tick(1); return strings.Contains(out.String(), "SET 'duration' = 1250.5") },
		3*time.Second,
		10*time.Millisecond,
	)
	for _, text := range []string{"dem_synthetic.execution_state", "store_first", "CLABEL '_collect_job' 'checkout'", "SET 'failed' = 1"} {
		assert.Contains(t, out.String(), text)
	}
	receive(t, entered)
	snapshot := hub.Snapshot(time.Now())
	require.Len(t, snapshot, 1)
	assert.Equal(t, "running", snapshot[0].State)
	require.NotNil(t, snapshot[0].Latest)
	assert.Equal(t, model.Failed, snapshot[0].Latest.Outcome)
	stop()
	receive(t, cancelled)
	assert.Empty(t, hub.Snapshot(time.Now()))
	assert.Nil(t, run.Failure())
}
func TestStopJoinsLateCallbacksBeforeSuccessorCanActivate(t *testing.T) {
	hub := syntheticregistry.New()
	entered, cancelled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var unblock sync.Once
	t.Cleanup(func() { unblock.Do(func() { close(release) }) })
	c := newLighthouse(
		executor{
			execute: func(ctx context.Context, _ model.Request, state func(string)) model.Execution {
				close(entered)
				<-ctx.Done()
				close(cancelled)
				<-release
				state("running")
				return model.Execution{
					Drained: true,
					Run: model.Run{
						Outcome: model.Failed,
					},
				}
			},
		},
		hub,
	)
	require.NoError(t, c.Init(t.Context()))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready, done := make(chan struct{}), make(chan error, 1)
	go func() { done <- c.Run(ctx, func() { close(ready) }) }()
	receive(t, ready)
	receive(t, entered)
	cancel()
	receive(t, cancelled)
	select {
	case <-done:
		t.Fatal("Run returned before physical executor join")
	default:
	}
	next := newLighthouse(executor{}, hub)
	require.NoError(t, next.Init(t.Context()))
	require.Error(t, next.Run(t.Context(), func() { t.Error("successor activated before predecessor join") }))
	unblock.Do(func() { close(release) })
	require.NoError(t, receive(t, done))
	start(t, next)
	c.Cleanup(t.Context())
	jobs := hub.Snapshot(time.Now())
	require.Len(t, jobs, 1)
	assert.Nil(t, jobs[0].Latest, "joined predecessor cannot publish to successor")
}
func TestUnverifiedCompletionFailsRunEvenAfterCancellation(t *testing.T) {
	for _, stop := range []bool{false, true} {
		t.Run(fmt.Sprint(stop), func(t *testing.T) {
			hub := syntheticregistry.New()
			entered, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			t.Cleanup(func() { once.Do(func() { close(release) }) })
			c := newLighthouse(executor{
				execute: func(context.Context, model.Request, func(string)) model.Execution {
					close(entered)
					<-release
					return model.Execution{
						Drained: false,
						Run: model.Run{
							Outcome: model.Error,
						},
					}
				},
			}, hub)
			require.NoError(t, c.Init(t.Context()))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			ready, done := make(chan struct{}), make(chan error, 1)
			go func() { done <- c.Run(ctx, func() { close(ready) }) }()
			receive(t, ready)
			receive(t, entered)
			jobs := hub.Snapshot(time.Now())
			require.Len(t, jobs, 1)
			assert.Nil(t, jobs[0].Latest)
			assert.NotContains(t, collect(t, c), "duration")
			if stop {
				cancel()
			}
			once.Do(func() { close(release) })
			require.ErrorContains(t, receive(t, done), "completion is unverified")
			assert.Empty(t, hub.Snapshot(time.Now()))
			assert.NotContains(t, collect(t, c), "duration")
		})
	}
}
func TestCollectionRequiresActivationAndHonorsCancellation(t *testing.T) {
	c := newLighthouse(executor{}, syntheticregistry.New())
	require.EqualError(t, c.Collect(t.Context()), "synthetic job is not active")
	start(t, c)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, c.Collect(ctx), context.Canceled)
}

var _ collectorapi.CollectorV2 = (*lighthouse.Collector)(nil)
var _ collectorapi.CollectorV2Runner = (*lighthouse.Collector)(nil)
