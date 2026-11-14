// SPDX-License-Identifier: GPL-3.0-or-later

package syntheticjob_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/internal/syntheticjobtest"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/registry"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/jobruntime"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPreparationDoesNotRunOrRegister(t *testing.T) {
	forEachKind(t, func(t *testing.T, _ string, k kind) {
		hub := registry.New()
		calls := 0
		c := k.new(syntheticjobtest.Executor{
			CheckFunc: func(ctx context.Context) error {
				calls++
				deadline, ok := ctx.Deadline()
				assert.True(t, ok)
				assert.LessOrEqual(t, time.Until(deadline), 15*time.Second)
				return errors.New("prepared browser missing")
			},
			ExecuteFunc: func(context.Context, synthetic.Request, func(synthetic.JobState)) synthetic.Execution {
				t.Error("preparation executed workflow")
				return synthetic.Execution{}
			},
		}, hub, settings{})
		require.NoError(t, c.Init(t.Context()))
		assert.Empty(t, hub.Snapshot(time.Now()))
		require.EqualError(t, c.Check(t.Context()), "prepared browser missing")
		assert.Equal(t, 1, calls)
		assert.Empty(t, hub.Snapshot(time.Now()))
		c.Cleanup(t.Context())
		c.Cleanup(t.Context())
	})
}

func TestActiveRegistrationAndReplacement(t *testing.T) {
	forEachKind(t, func(t *testing.T, module string, k kind) {
		hub := registry.New()
		current := k.new(syntheticjobtest.Executor{}, hub, settings{})
		stop := start(t, current)
		jobs := hub.Snapshot(time.Now())
		require.Len(t, jobs, 1)
		assert.False(t, jobs[0].HasObservation)
		assert.Equal(t, module+":checkout", jobs[0].JobID)
		assert.Equal(t, time.Duration(k.cadence)*time.Second, jobs[0].Interval)
		assert.Equal(t, k.target, jobs[0].Target)
		candidate := k.new(syntheticjobtest.Executor{}, hub, settings{})
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
	})
}

func TestOutcomesAndMissingObservations(t *testing.T) {
	forEachKind(t, func(t *testing.T, module string, k kind) {
		for _, outcome := range []synthetic.Outcome{
			synthetic.Success,
			synthetic.Failed,
			synthetic.Timeout,
			synthetic.Inconclusive,
			synthetic.Error,
			synthetic.Cancelled,
			synthetic.Unknown,
		} {
			t.Run(string(outcome), func(t *testing.T) {
				hub := registry.New()
				evidence := synthetic.Run{
					Outcome: outcome,
					Error:   "fixture teardown failed",
					Events: []synthetic.Event{
						{
							Kind:    "test_end",
							Phase:   "afterAll",
							Status:  "failed",
							Message: "fixture teardown failed",
						},
					},
				}
				c := k.new(syntheticjobtest.Executor{
					ExecuteFunc: func(_ context.Context, r synthetic.Request, state func(synthetic.JobState)) synthetic.Execution {
						assert.Equal(t, "checkout", r.Name)
						state(synthetic.JobRunning)
						assert.Equal(t, synthetic.JobRunning, hub.Snapshot(time.Now())[0].State)
						return synthetic.Execution{
							Drained: true,
							Run:     evidence,
						}
					},
				}, hub, settings{})
				start(t, c)
				latest := waitLatest(t, hub, module+":checkout")
				got := collect(t, c)
				assert.EqualValues(t, 1, got[`execution_state{execution_state="`+string(outcome)+`"}`])
				assert.EqualValues(t, 1, got["attempts_"+string(outcome)])
				assert.NotContains(t, got, "duration")
				for _, name := range measurementNames() {
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
	})
}

func TestRunExecutesWithoutCollectAndRepeatedCollectionsHoldLatest(t *testing.T) {
	forEachKind(t, func(t *testing.T, module string, k kind) {
		hub := registry.New()
		var calls atomic.Int32
		c := k.new(syntheticjobtest.Executor{
			ExecuteFunc: func(_ context.Context, r synthetic.Request, state func(synthetic.JobState)) synthetic.Execution {
				calls.Add(1)
				state(synthetic.JobRunning)
				assert.Equal(t, k.request, r, "Init stores the configured request and Run hands it to the executor")
				return synthetic.Execution{
					Drained: true,
					Run:     fixture(),
				}
			},
		}, hub, settings{})
		start(t, c)
		run := waitLatest(t, hub, module+":checkout")
		assert.Equal(t, int32(1), calls.Load(), "Run must execute bootstrap without any Collect")
		for range 3 {
			got := collect(t, c)
			want := map[string]float64{"duration": 1250.5}
			for name, value := range k.measurements {
				want[name] = value
			}
			for name, value := range want {
				require.Contains(t, got, name)
				assert.EqualValues(t, value, got[name], name)
			}
			for _, name := range measurementNames() {
				if _, ok := want[name]; !ok {
					assert.NotContains(t, got, name, "another kind's measurement")
				}
			}
			assert.EqualValues(t, 1, got["admitted"])
			assert.EqualValues(t, 1, got["scheduled"])
			assert.Zero(t, got["coalesced"])
			assert.Zero(t, got["active"])
			assert.Zero(t, got["pending"])
			assert.Contains(t, got, "observation_age")
			assert.EqualValues(t, 1, got["attempts_"+string(fixture().Outcome)])
		}
		contexts := map[string][]string{"dem_synthetic.duration": {"duration"}}
		for name, dimensions := range k.charts {
			contexts[name] = dimensions
		}
		collecttest.AssertChartCoverage(t, c, collecttest.ChartCoverageExpectation{
			RequiredContexts: contexts,
		})
		assert.Equal(t, int32(1), calls.Load(), "publication cannot start more browser work")
		assert.Equal(t, run.ID, hub.Snapshot(time.Now())[0].Latest.ID)
	})
}

func TestLaterObservationWithMissingValuesCreatesGaps(t *testing.T) {
	forEachKind(t, func(t *testing.T, module string, k kind) {
		hub := registry.New()
		var calls atomic.Int32
		c := k.new(syntheticjobtest.Executor{
			ExecuteFunc: func(context.Context, synthetic.Request, func(synthetic.JobState)) synthetic.Execution {
				run := fixture()
				if calls.Add(1) == 1 {
					// Zero values are measurements, not gaps.
					run.Outcome = synthetic.Inconclusive
					run.DurationMS = new(0.0)
					run.Tests = &synthetic.TestCounts{}
				} else {
					run.Outcome = synthetic.Error
					run.DurationMS = nil
					run.Tests = nil
					// A measurement object without values is a gap, not a zero.
					run.Metrics = &synthetic.LabMetrics{}
				}
				return synthetic.Execution{
					Drained: true,
					Run:     run,
				}
			},
		}, hub, settings{
			checkEvery: 1,
		})
		start(t, c)
		waitLatest(t, hub, module+":checkout")
		names := []string{"duration"}
		for name := range k.measurements {
			names = append(names, name)
		}
		got := collect(t, c)
		for _, name := range names {
			require.Contains(t, got, name)
		}
		require.Eventually(t, func() bool {
			jobs := hub.Snapshot(time.Now())
			return len(jobs) == 1 && jobs[0].HasObservation && jobs[0].Latest.Outcome == synthetic.Error
		}, 3*time.Second, time.Millisecond)
		got = collect(t, c)
		for _, name := range names {
			assert.NotContains(t, got, name)
		}
		assert.EqualValues(t, 1, got[`execution_state{execution_state="error"}`])
		assert.EqualValues(t, 1, got["attempts_error"])
	})
}

func TestNeverObservedAndStaleSnapshotsOmitMeasurementsAndRetainCharts(t *testing.T) {
	forEachKind(t, func(t *testing.T, module string, k kind) {
		t.Run("never observed", func(t *testing.T) {
			c := k.new(syntheticjobtest.Executor{}, registry.New(), settings{})
			start(t, c)
			got := collect(t, c)
			assert.EqualValues(t, 1, got[`execution_state{execution_state="unknown"}`])
			assert.NotContains(t, got, "duration")
			assert.NotContains(t, got, "observation_age")
			assert.Zero(t, got["admitted"])
		})
		t.Run("stale", func(t *testing.T) {
			hub := registry.New()
			var calls atomic.Int32
			c := k.new(syntheticjobtest.Executor{
				ExecuteFunc: func(ctx context.Context, _ synthetic.Request, _ func(synthetic.JobState)) synthetic.Execution {
					if calls.Add(1) == 1 {
						return synthetic.Execution{
							Drained: true,
							Run:     fixture(),
						}
					}
					<-ctx.Done()
					return synthetic.Execution{
						Drained: true,
						Run: synthetic.Run{
							Outcome: synthetic.Cancelled,
						},
					}
				},
			}, hub, settings{
				checkEvery: 1,
			})
			start(t, c)
			waitLatest(t, hub, module+":checkout")
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
			assert.EqualValues(t, 1, stale["attempts_"+string(fixture().Outcome)])
			for range 7 {
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
	})
}

func TestNativeJobExecutionCancellationAndOutput(t *testing.T) {
	forEachKind(t, func(t *testing.T, module string, k kind) {
		hub := registry.New()
		entered, cancelled := make(chan struct{}), make(chan struct{})
		var attempts atomic.Int32
		c := k.new(syntheticjobtest.Executor{
			ExecuteFunc: func(ctx context.Context, _ synthetic.Request, state func(synthetic.JobState)) synthetic.Execution {
				state(synthetic.JobRunning)
				if attempts.Add(1) == 1 {
					return synthetic.Execution{
						Drained: true,
						Run: synthetic.Run{
							Outcome:    synthetic.Failed,
							DurationMS: new(1250.5),
						},
					}
				}
				close(entered)
				<-ctx.Done()
				close(cancelled)
				return synthetic.Execution{
					Drained: true,
					Run: synthetic.Run{
						Outcome: synthetic.Cancelled,
					},
				}
			},
		}, hub, settings{
			checkEvery:  1,
			updateEvery: 1,
		})
		out := &output{}
		job := jobruntime.NewJobV2(
			jobruntime.JobV2Config{
				PluginName:  "dem",
				ModuleName:  module,
				Name:        "checkout",
				FullName:    module + "_checkout",
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
		waitLatest(t, hub, module+":checkout")
		assert.Positive(t, attempts.Load(), "bootstrap execution completes before requesting a metric tick")
		require.Eventually(
			t,
			func() bool { job.Tick(1); return strings.Contains(out.String(), "SET 'duration' = 1250.5") },
			3*time.Second,
			10*time.Millisecond,
		)
		for _, text := range []string{
			"dem_synthetic.execution_state",
			"store_first",
			"CLABEL '_collect_job' 'checkout'",
			"SET 'failed' = 1",
		} {
			assert.Contains(t, out.String(), text)
		}
		receive(t, entered)
		snapshot := hub.Snapshot(time.Now())
		require.Len(t, snapshot, 1)
		assert.Equal(t, synthetic.JobRunning, snapshot[0].State)
		require.True(t, snapshot[0].HasObservation)
		assert.Equal(t, synthetic.Failed, snapshot[0].Latest.Outcome)
		stop()
		receive(t, cancelled)
		assert.Empty(t, hub.Snapshot(time.Now()))
		assert.Nil(t, run.Failure())
	})
}

func TestStopJoinsLateCallbacksBeforeSuccessorCanActivate(t *testing.T) {
	forEachKind(t, func(t *testing.T, _ string, k kind) {
		hub := registry.New()
		entered, cancelled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
		var unblock sync.Once
		t.Cleanup(func() { unblock.Do(func() { close(release) }) })
		c := k.new(syntheticjobtest.Executor{
			ExecuteFunc: func(ctx context.Context, _ synthetic.Request, state func(synthetic.JobState)) synthetic.Execution {
				close(entered)
				<-ctx.Done()
				close(cancelled)
				<-release
				state(synthetic.JobRunning)
				return synthetic.Execution{
					Drained: true,
					Run: synthetic.Run{
						Outcome: synthetic.Failed,
					},
				}
			},
		}, hub, settings{})
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
		next := k.new(syntheticjobtest.Executor{}, hub, settings{})
		require.NoError(t, next.Init(t.Context()))
		require.Error(t, next.Run(t.Context(), func() { t.Error("successor activated before predecessor join") }))
		unblock.Do(func() { close(release) })
		require.NoError(t, receive(t, done))
		start(t, next)
		c.Cleanup(t.Context())
		jobs := hub.Snapshot(time.Now())
		require.Len(t, jobs, 1)
		assert.False(t, jobs[0].HasObservation, "joined predecessor cannot publish to successor")
	})
}

func TestUnverifiedCompletionFailsRunEvenAfterCancellation(t *testing.T) {
	forEachKind(t, func(t *testing.T, _ string, k kind) {
		for _, stop := range []bool{false, true} {
			t.Run(fmt.Sprint(stop), func(t *testing.T) {
				hub := registry.New()
				entered, release := make(chan struct{}), make(chan struct{})
				var once sync.Once
				t.Cleanup(func() { once.Do(func() { close(release) }) })
				c := k.new(syntheticjobtest.Executor{
					ExecuteFunc: func(context.Context, synthetic.Request, func(synthetic.JobState)) synthetic.Execution {
						close(entered)
						<-release
						return synthetic.Execution{
							Drained: false,
							Run: synthetic.Run{
								Outcome: synthetic.Error,
							},
						}
					},
				}, hub, settings{})
				require.NoError(t, c.Init(t.Context()))
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				ready, done := make(chan struct{}), make(chan error, 1)
				go func() { done <- c.Run(ctx, func() { close(ready) }) }()
				receive(t, ready)
				receive(t, entered)
				jobs := hub.Snapshot(time.Now())
				require.Len(t, jobs, 1)
				assert.False(t, jobs[0].HasObservation)
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
	})
}

func TestCollectionRequiresActivationAndHonorsCancellation(t *testing.T) {
	forEachKind(t, func(t *testing.T, _ string, k kind) {
		c := k.new(syntheticjobtest.Executor{}, registry.New(), settings{})
		require.EqualError(t, c.Collect(t.Context()), "synthetic job is not active")
		start(t, c)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		require.ErrorIs(t, c.Collect(ctx), context.Canceled)
	})
}
