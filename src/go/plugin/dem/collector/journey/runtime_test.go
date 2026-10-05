// SPDX-License-Identifier: GPL-3.0-or-later

package journey_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/journey"
	model "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	syntheticregistry "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/registry"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/jobruntime"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type executor struct {
	check   func(context.Context, model.Kind) error
	execute func(context.Context, model.Request, func(string)) model.Execution
}

func (e executor) Check(ctx context.Context, kind model.Kind) error {
	if e.check != nil {
		return e.check(ctx, kind)
	}
	return nil
}
func (e executor) Execute(ctx context.Context, r model.Request, state func(string)) model.Execution {
	return e.execute(ctx, r, state)
}
func ptr(v float64) *float64 { return &v }
func newJourney(e journey.Executor, hub *syntheticregistry.Registry) *journey.Collector {
	c := journey.New(journey.Dependencies{
		Executor: e,
		Registry: hub,
	})
	c.Name = "checkout"
	c.Script = "import { test } from '@playwright/test'; test('checkout', async () => {});"
	return c
}
func start(t *testing.T, c interface {
	Init(context.Context) error
	Run(context.Context, func()) error
}) func() {
	t.Helper()
	require.NoError(t, c.Init(context.Background()))
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
				t.Error("Run did not stop")
			}
		})
	}
	t.Cleanup(stop)
	return stop
}

func TestPreparationDoesNotRunOrRegister(t *testing.T) {
	hub := syntheticregistry.New()
	calls := 0
	c := newJourney(executor{
		check: func(ctx context.Context, kind model.Kind) error {
			calls++
			assert.Equal(t, model.Journey, kind)
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
	require.NoError(t, c.Init(context.Background()))
	assert.Empty(t, hub.Snapshot(time.Now()))
	require.EqualError(t, c.Check(context.Background()), "prepared browser missing")
	assert.Equal(t, 1, calls)
	assert.Empty(t, hub.Snapshot(time.Now()))
	c.Cleanup(context.Background())
	c.Cleanup(context.Background())
}

func TestActiveRegistrationAndReplacement(t *testing.T) {
	hub := syntheticregistry.New()
	deps := executor{}
	current := newJourney(deps, hub)
	stop := start(t, current)
	jobs := hub.Snapshot(time.Now())
	require.Len(t, jobs, 1)
	assert.Equal(t, "unknown", jobs[0].State)
	assert.Nil(t, jobs[0].Latest)
	candidate := newJourney(deps, hub)
	require.NoError(t, candidate.Init(context.Background()))
	require.NoError(t, candidate.Check(context.Background()))
	candidate.Cleanup(context.Background())
	assert.Len(t, hub.Snapshot(time.Now()), 1, "candidate cleanup cannot retire incumbent")
	require.Error(t, candidate.Run(context.Background(), func() { t.Error("duplicate became ready") }))
	stop()
	assert.Empty(t, hub.Snapshot(time.Now()))
	stopNext := start(t, candidate)
	current.Cleanup(context.Background())
	require.Len(t, hub.Snapshot(time.Now()), 1, "old cleanup cannot retire successor")
	stopNext()
	assert.Empty(t, hub.Snapshot(time.Now()))
}

func TestOutcomesAndMissingObservations(t *testing.T) {
	// Executor evidence is the input contract: the collector must preserve each
	// terminal result and its diagnosis rather than infer success from counts.
	for _, outcome := range []model.Outcome{model.Success, model.Failed, model.Timeout, model.Inconclusive, model.Error, model.Cancelled, model.Unknown} {
		t.Run(string(outcome), func(t *testing.T) {
			hub := syntheticregistry.New()
			evidence := model.Run{
				ID:          "run-1",
				JobID:       "journey:checkout",
				Kind:        model.Journey,
				Name:        "checkout",
				CompletedUS: time.Now().UnixMicro(),
				Outcome:     outcome,
				Error:       "fixture teardown failed",
				Events: []model.Event{
					{Kind: "test_end", Phase: "afterAll", Status: "failed", Message: "fixture teardown failed"},
				},
			}
			c := newJourney(
				executor{
					execute: func(ctx context.Context, req model.Request, state func(string)) model.Execution {
						assert.Equal(t, "checkout", req.Name)
						state("waiting")
						assert.Equal(t, "waiting", hub.Snapshot(time.Now())[0].State)
						state("running")
						assert.Equal(t, "running", hub.Snapshot(time.Now())[0].State)
						return model.Execution{
							Run:     evidence,
							Drained: true,
						}
					},
				},
				hub,
			)
			start(t, c)
			got, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
			require.NoError(t, err)
			assert.EqualValues(t, 1, got[`execution_state{execution_state="`+string(outcome)+`"}`])
			assert.NotContains(t, got, "duration")
			assert.NotContains(t, got, "tests_passed")
			assert.NotContains(t, got, "performance")
			snapshot := hub.Snapshot(time.Now())
			require.Len(t, snapshot, 1)
			assert.Equal(t, evidence, *snapshot[0].Latest)
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

func TestJourneyEvidenceAndSecretRequest(t *testing.T) {
	hub := syntheticregistry.New()
	c := newJourney(executor{
		execute: func(ctx context.Context, req model.Request, state func(string)) model.Execution {
			assert.Equal(t, map[string]string{"DEM_SECRET_LOGIN": "synthetic-password"}, req.Secrets)
			assert.NotEmpty(t, req.Script)
			assert.Empty(t, req.ScriptPath)
			assert.Equal(t, 2*time.Minute, req.Timeout)
			assert.True(t, req.Capture)
			return model.Execution{
				Drained: true,
				Run: model.Run{
					CompletedUS: time.Now().UnixMicro(),
					Outcome:     model.Failed,
					DurationMS:  ptr(1250.5),
					Metrics: &model.LabMetrics{
						Performance: ptr(100),
					},
					Tests: &model.TestCounts{
						Declared:        7,
						Passed:          1,
						Failed:          1,
						TimedOut:        1,
						Skipped:         1,
						ExpectedFailure: 1,
						NotRun:          2,
					},
				},
			}
		},
	}, hub)
	c.Secrets = []journey.Secret{{Name: "DEM_SECRET_LOGIN", Value: "synthetic-password"}}
	c.ScreenshotOnFailure = true
	start(t, c)
	got, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.NotContains(t, got, "performance", "journey owns no lab instruments")
	for key, want := range map[string]float64{"duration": 1250.5, "tests_declared": 7, "tests_passed": 1, "tests_failed": 1, "tests_timed_out": 1, "tests_skipped": 1, "tests_expected_failure": 1, "tests_not_run": 2} {
		assert.EqualValues(t, want, got[key], key)
	}
	collecttest.AssertChartCoverage(t, c, collecttest.ChartCoverageExpectation{
		RequiredContexts: map[string][]string{
			"dem_synthetic.duration": {
				"duration",
			}, "dem_synthetic.journey.declared_tests": {"declared"}, "dem_synthetic.journey.test_outcomes": {"passed", "failed", "timed_out", "skipped", "expected_failure", "not_run"},
		},
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
	entered := make(chan struct{})
	cancelled := make(chan struct{})
	var mu sync.Mutex
	attempts := 0
	c := newJourney(executor{
		execute: func(ctx context.Context, req model.Request, state func(string)) model.Execution {
			mu.Lock()
			attempts++
			attempt := attempts
			mu.Unlock()
			state("running")
			if attempt == 1 {
				return model.Execution{
					Drained: true,
					Run: model.Run{
						CompletedUS: time.Now().UnixMicro(),
						Outcome:     model.Failed,
						DurationMS:  ptr(1250.5),
					},
				}
			}
			close(entered)
			<-ctx.Done()
			close(cancelled)
			return model.Execution{
				Drained: true,
				Run: model.Run{
					CompletedUS: time.Now().UnixMicro(),
					Outcome:     model.Cancelled,
				},
			}
		},
	}, hub)
	c.UpdateEvery = 1
	out := &output{}
	job := jobruntime.NewJobV2(
		jobruntime.JobV2Config{
			PluginName:  "dem",
			ModuleName:  "journey",
			Name:        c.Name,
			FullName:    "journey_" + c.Name,
			Module:      c,
			Out:         out,
			UpdateEvery: 1,
			StoreFirst:  true,
		},
	)
	require.NoError(t, job.AutoDetectionManaged(context.Background()))
	run := jobruntime.NewManagedRun(context.Background(), nil)
	done := make(chan struct{})
	go func() { job.StartManaged(run); close(done) }()
	var once sync.Once
	stop := func() {
		once.Do(func() {
			job.Stop()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Error("native job did not stop")
			}
			job.Cleanup()
		})
	}
	t.Cleanup(stop)
	select {
	case <-run.StartupDone():
	case <-time.After(3 * time.Second):
		t.Fatal("native startup timed out")
	}
	require.NoError(t, run.StartupErr())
	require.Eventually(
		t,
		func() bool { job.Tick(1); return strings.Contains(out.String(), "SET 'duration' = 1250.5") },
		3*time.Second,
		10*time.Millisecond,
	)
	assert.Contains(t, out.String(), "dem_synthetic.execution_state")
	assert.Contains(t, out.String(), "store_first")
	assert.Contains(t, out.String(), "CLABEL '_collect_job' 'checkout'")
	assert.Contains(t, out.String(), "SET 'failed' = 1")
	job.Tick(2)
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("second attempt did not start")
	}
	snapshot := hub.Snapshot(time.Now())
	require.Len(t, snapshot, 1)
	assert.Equal(t, "running", snapshot[0].State)
	require.NotNil(t, snapshot[0].Latest)
	assert.Equal(
		t,
		model.Failed,
		snapshot[0].Latest.Outcome,
		"current attempt retains historical diagnosis without re-emission",
	)
	stop()
	select {
	case <-cancelled:
	default:
		t.Error("native stop did not cancel execution")
	}
	assert.Empty(t, hub.Snapshot(time.Now()))
	assert.Nil(t, run.Failure())
}

func TestUnverifiedCompletionFailsRuntime(t *testing.T) {
	hub := syntheticregistry.New()
	c := newJourney(executor{
		execute: func(context.Context, model.Request, func(string)) model.Execution {
			return model.Execution{
				Run: model.Run{
					Outcome: model.Error,
				},
			}
		},
	}, hub)
	require.NoError(t, c.Init(context.Background()))
	ready := make(chan struct{})
	done := make(chan error, 1)
	go func() { done <- c.Run(context.Background(), func() { close(ready) }) }()
	<-ready
	_, err := collecttest.CollectScalarSeries(c)
	require.ErrorContains(t, err, "completion is unverified")
	select {
	case err := <-done:
		require.ErrorContains(t, err, "completion is unverified")
	case <-time.After(3 * time.Second):
		t.Fatal("unverified completion did not fail Run")
	}
	assert.Empty(t, hub.Snapshot(time.Now()))
}

var _ journey.Executor = executor{}
var _ collectorapi.CollectorV2 = (*journey.Collector)(nil)
var _ collectorapi.CollectorV2Runner = (*journey.Collector)(nil)

// Run cancellation can retire the registration while Execute is still joining
// readers. Late callbacks belong to that retired generation only.
func TestLateCallbacksCannotChangeSuccessor(t *testing.T) {
	hub := syntheticregistry.New()
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	c := newJourney(executor{
		execute: func(ctx context.Context, req model.Request, state func(string)) model.Execution {
			close(entered)
			<-release
			state("running")
			return model.Execution{
				Drained: true,
				Run: model.Run{
					Outcome: model.Failed,
				},
			}
		},
	}, hub)
	stop := start(t, c)
	collected := make(chan error, 1)
	go func() { _, err := collecttest.CollectScalarSeries(c); collected <- err }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("execution did not start")
	}
	stop()
	next := newJourney(executor{}, hub)
	start(t, next)
	unblock()
	select {
	case err := <-collected:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("collection did not finish")
	}
	jobs := hub.Snapshot(time.Now())
	require.Len(t, jobs, 1)
	assert.Equal(t, "unknown", jobs[0].State)
	assert.Nil(t, jobs[0].Latest)
	c.Cleanup(context.Background())
	assert.Len(t, hub.Snapshot(time.Now()), 1)
}

func TestCollectionRequiresActivationAndHonorsCancellation(t *testing.T) {
	c := newJourney(executor{
		execute: func(context.Context, model.Request, func(string)) model.Execution {
			t.Error("inactive or cancelled collection executed")
			return model.Execution{}
		},
	}, syntheticregistry.New())
	require.EqualError(t, c.Collect(context.Background()), "synthetic job is not active")
	start(t, c)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, c.Collect(ctx), context.Canceled)
}

func TestJourneyZeroCountsAndLaterGaps(t *testing.T) {
	attempts := 0
	c := newJourney(executor{
		execute: func(context.Context, model.Request, func(string)) model.Execution {
			attempts++
			run := model.Run{
				Outcome:    model.Inconclusive,
				DurationMS: ptr(0),
				Tests:      &model.TestCounts{},
			}
			if attempts == 2 {
				run.Outcome = model.Error
				run.DurationMS = nil
				run.Tests = nil
			}
			return model.Execution{
				Drained: true,
				Run:     run,
			}
		},
	}, syntheticregistry.New())
	start(t, c)
	got, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
	require.NoError(t, err)
	names := []string{
		"duration",
		"tests_declared",
		"tests_passed",
		"tests_failed",
		"tests_timed_out",
		"tests_skipped",
		"tests_expected_failure",
		"tests_not_run",
	}
	for _, name := range names {
		require.Contains(t, got, name)
		assert.Zero(t, got[name])
	}
	got, err = collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
	require.NoError(t, err)
	for _, name := range names {
		assert.NotContains(t, got, name)
	}
}

// Cancellation can retire Run before the executor proves whether it drained.
// Collect must still return the ownership error without waiting for a Run reader.
func TestUnverifiedCompletionAfterRunCancellation(t *testing.T) {
	hub := syntheticregistry.New()
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	c := newJourney(executor{
		execute: func(context.Context, model.Request, func(string)) model.Execution {
			close(entered)
			<-release
			return model.Execution{
				Run: model.Run{
					Outcome: model.Error,
				},
				Drained: false,
			}
		},
	}, hub)
	stop := start(t, c)
	collected := make(chan error, 1)
	go func() { _, err := collecttest.CollectScalarSeries(c); collected <- err }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("execution did not start")
	}
	stop()
	require.Empty(t, hub.Snapshot(time.Now()))
	unblock()
	select {
	case err := <-collected:
		require.ErrorContains(t, err, "completion is unverified")
	case <-time.After(3 * time.Second):
		t.Fatal("collection blocked after Run cancellation")
	}
	assert.Empty(t, hub.Snapshot(time.Now()))
}

func TestUnverifiedExecutionDoesNotPublishLatest(t *testing.T) {
	hub := syntheticregistry.New()
	c := newJourney(executor{
		execute: func(context.Context, model.Request, func(string)) model.Execution {
			return model.Execution{
				Run: model.Run{
					Outcome:   model.Error,
					StartedUS: time.Now().UnixMicro(),
				},
				Drained: false,
			}
		},
	}, hub)
	require.NoError(t, c.Init(context.Background()))
	ready := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	go func() { done <- c.Run(ctx, func() { close(ready); <-release }) }()
	t.Cleanup(func() {
		cancel()
		close(release)
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("Run did not finish")
		}
	})
	select {
	case <-ready:
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not become ready")
	}
	_, err := collecttest.CollectScalarSeries(c)
	require.ErrorContains(t, err, "completion is unverified")
	jobs := hub.Snapshot(time.Now())
	require.Len(t, jobs, 1)
	assert.Nil(t, jobs[0].Latest, "incomplete execution is not a terminal run")
	assert.Equal(t, "unknown", jobs[0].State)
}
