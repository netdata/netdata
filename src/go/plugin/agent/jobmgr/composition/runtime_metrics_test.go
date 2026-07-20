// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"bytes"
	"context"
	"errors"
	"maps"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/runtimecomp"
	"github.com/stretchr/testify/require"
)

type runMetricsService struct {
	mu sync.Mutex

	components         []runtimecomp.ComponentConfig
	componentRemovals  []string
	componentFinalized []string
	producers          map[string]func() error
	producerRemovals   []string
	producerErr        error
}

type runRetryWorker struct {
	waitErr error
	joined  bool
	stops   int
}

func (rrw *runRetryWorker) StopAutoDetectionRetries() {
	rrw.stops++
}

func (rrw *runRetryWorker) WaitAutoDetectionRetries(
	context.Context,
) error {
	return rrw.waitErr
}

func (rrw *runRetryWorker) AutoDetectionRetriesJoined() bool {
	return rrw.joined
}

func (rms *runMetricsService) RegisterComponent(
	config runtimecomp.ComponentConfig,
) error {
	rms.mu.Lock()
	defer rms.mu.Unlock()
	rms.components = append(rms.components, config)
	return nil
}

func (rms *runMetricsService) UnregisterComponent(name string) {
	rms.mu.Lock()
	defer rms.mu.Unlock()
	rms.componentRemovals = append(rms.componentRemovals, name)
}

func (*runMetricsService) QuarantineComponent(string) {}

func (rms *runMetricsService) FinalizeComponent(name string) {
	rms.mu.Lock()
	defer rms.mu.Unlock()
	rms.componentFinalized = append(rms.componentFinalized, name)
}

func (rms *runMetricsService) RegisterProducer(
	name string,
	producer func() error,
) error {
	rms.mu.Lock()
	defer rms.mu.Unlock()
	if rms.producerErr != nil {
		return rms.producerErr
	}
	if rms.producers == nil {
		rms.producers = make(map[string]func() error)
	}
	rms.producers[name] = producer
	return nil
}

func (rms *runMetricsService) UnregisterProducer(name string) {
	rms.mu.Lock()
	defer rms.mu.Unlock()
	rms.producerRemovals = append(rms.producerRemovals, name)
	delete(rms.producers, name)
}

func (rms *runMetricsService) snapshot() (
	[]runtimecomp.ComponentConfig,
	[]string,
	map[string]func() error,
	[]string,
) {
	rms.mu.Lock()
	defer rms.mu.Unlock()
	components := append([]runtimecomp.ComponentConfig(nil), rms.components...)
	componentRemovals := append([]string(nil), rms.componentRemovals...)
	producers := make(map[string]func() error, len(rms.producers))
	maps.Copy(producers, rms.producers)
	producerRemovals := append([]string(nil), rms.producerRemovals...)
	return components, componentRemovals, producers, producerRemovals
}

func (rms *runMetricsService) finalized() []string {
	rms.mu.Lock()
	defer rms.mu.Unlock()
	return append([]string(nil), rms.componentFinalized...)
}

func TestRunMetricsProjection(t *testing.T) {
	tests := map[string]struct {
		apply func(*runMetrics)
		name  string
		want  float64
	}{
		"sets owner gauge": {
			apply: func(metrics *runMetrics) {
				metrics.SetRuntimeGauge(lifecycle.RuntimeGaugeOperationsActive, 7)
			},
			name: runtimeMetricPrefix + ".operations_active",
			want: 7,
		},
		"adds shared owner gauge": {
			apply: func(metrics *runMetrics) {
				metrics.AddRuntimeGauge(lifecycle.RuntimeGaugeJobsActive, 1)
				metrics.AddRuntimeGauge(lifecycle.RuntimeGaugeJobsActive, 1)
				metrics.AddRuntimeGauge(lifecycle.RuntimeGaugeJobsActive, -1)
			},
			name: runtimeMetricPrefix + ".jobs_active",
			want: 1,
		},
		"adds lifecycle counter": {
			apply: func(metrics *runMetrics) {
				metrics.AddRuntimeCounter(lifecycle.RuntimeCounterTaskPanics, 2)
			},
			name: runtimeMetricPrefix + ".task_panics_total",
			want: 2,
		},
		"zero timestamp reports zero age": {
			apply: func(metrics *runMetrics) {
				metrics.SetRuntimeTimestamp(
					lifecycle.RuntimeTimestampOldestOperation,
					time.Time{},
				)
			},
			name: runtimeMetricPrefix + ".oldest_operation_age",
			want: 0,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			metrics := newRunMetrics()
			test.apply(metrics)

			require.NoError(t, metrics.refreshProjection())

			reader := metrics.store.Read(metrix.ReadRaw())
			got, ok := reader.Value(test.name, nil)
			require.True(t, ok)
			require.EqualValues(t, test.want, got)
		})
	}
}

func TestRunMetricsOwnerUpdatesDoNotAllocate(t *testing.T) {
	metrics := newRunMetrics()
	now := time.Now()
	allocations := testing.AllocsPerRun(100, func() {
		metrics.SetRuntimeGauge(lifecycle.RuntimeGaugeOperationsActive, 1)
		metrics.AddRuntimeGauge(lifecycle.RuntimeGaugeJobsActive, 1)
		metrics.AddRuntimeCounter(lifecycle.RuntimeCounterOperationsAdmitted, 1)
		metrics.SetRuntimeTimestamp(lifecycle.RuntimeTimestampOldestOperation, now)
	})
	require.EqualValues(t, 0, allocations)
}

func TestRunMetricsRegistration(t *testing.T) {
	tests := map[string]struct {
		producerErr      error
		wantErr          bool
		wantComponents   int
		wantUnregistered int
		wantProducers    int
	}{
		"registers component and producer": {
			wantComponents: 1,
			wantProducers:  1,
		},
		"producer failure rolls back component": {
			producerErr:      errors.New("producer failed"),
			wantErr:          true,
			wantComponents:   1,
			wantUnregistered: 1,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			service := &runMetricsService{
				producerErr: test.producerErr,
			}
			metrics := newRunMetrics()
			err := metrics.register(service)
			require.EqualValues(t, test.wantErr, err != nil)
			components, removals, producers, _ := service.snapshot()
			require.False(t, len(components) != test.wantComponents ||
				len(removals) != test.wantUnregistered ||
				len(producers) != test.wantProducers)
			if len(components) == 1 {
				config := components[0]
				require.False(t, config.Name != runtimeComponentName || config.Store != metrics.store || !config.Autogen.Enabled)
			}
		})
	}
}

func TestRunGenerationRuntimeMetricsLifecycle(t *testing.T) {
	service := &runMetricsService{}
	jobs := testRunJobServices(t)
	jobs.Runtime = service
	frames, err := lifecycle.NewFrameOwner(&bytes.Buffer{})
	require.NoError(t, err)
	admission := lifecycle.NewAdmissionLedger()
	uids := lifecycle.NewUIDLedger()
	generation, err := newRunGeneration(runGenerationConfig{
		Generation: 1, ShutdownTimeout: time.Second,
		Clock: lifecycle.RealClock{}, Admission: admission, UIDs: uids,
		Frames: frames, Modules: collectorapi.Registry{}, Jobs: jobs,
		Discovery: testRunDiscoveryServices(t),
		Planner: func(
			runPlannerCapabilities,
		) (jobmgr.Planner, jobmgr.RunFinalizer, error) {
			return runRejectingPlanner{},
				jobmgr.RunFinalizerFunc(
					func(context.Context, uint64) error { return nil },
				),
				nil
		},
	})
	require.NoError(t, err)
	components, removals, producers, producerRemovals :=
		service.snapshot()
	require.False(t, len(components) != 1 || len(removals) != 0 || len(producers) != 1 || len(producerRemovals) != 0)

	require.NoError(t, generation.start(context.Background()))

	generation.metrics.AddRuntimeCounter(lifecycle.RuntimeCounterDirtyRuns, 1)
	generation.Stop()

	require.NoError(t, generation.Wait(context.Background()))

	_, removals, producers, producerRemovals = service.snapshot()
	finalized := service.finalized()
	require.False(t, len(removals) != 0 ||
		len(finalized) != 1 ||
		finalized[0] != runtimeComponentName ||
		len(producers) != 0 || len(producerRemovals) != 1 ||
		producerRemovals[0] != runtimeProducerName)
	reader := components[0].Store.Read(metrix.ReadRaw())

	got, ok := reader.Value(runtimeMetricPrefix+".dirty_runs_total", nil)
	require.False(t, !ok || got != 1)

	require.NoError(t, admission.CloseDrained(1))

	closeRunTestUIDs(t, uids)
}

func TestRunGenerationRetainsRuntimeMetricsUntilRetryWorkerJoins(
	t *testing.T,
) {
	service := &runMetricsService{}
	jobs := testRunJobServices(t)
	jobs.Runtime = service
	frames, err := lifecycle.NewFrameOwner(&bytes.Buffer{})
	require.NoError(t, err)
	admission := lifecycle.NewAdmissionLedger()
	uids := lifecycle.NewUIDLedger()
	generation, err := newRunGeneration(runGenerationConfig{
		Generation: 1, ShutdownTimeout: time.Second,
		Clock: lifecycle.RealClock{}, Admission: admission, UIDs: uids,
		Frames: frames, Modules: collectorapi.Registry{}, Jobs: jobs,
		Discovery: testRunDiscoveryServices(t),
		Planner: func(
			runPlannerCapabilities,
		) (jobmgr.Planner, jobmgr.RunFinalizer, error) {
			return runRejectingPlanner{},
				jobmgr.RunFinalizerFunc(
					func(context.Context, uint64) error { return nil },
				),
				nil
		},
	})
	require.NoError(t, err)
	require.NoError(t, generation.start(context.Background()))

	generation.scheduler.StopAutoDetectionRetries()
	require.NoError(t, generation.scheduler.WaitAutoDetectionRetries(
		context.Background(),
	))
	generation.kernel.Stop()
	require.NoError(t, generation.kernel.Wait(context.Background()))

	worker := &runRetryWorker{
		waitErr: context.Canceled,
	}
	generation.retryWorker = worker
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, generation.Wait(ctx), context.Canceled)
	require.EqualValues(t, 1, worker.stops)
	require.True(t, generation.runtimeRegistered)
	_, _, producers, producerRemovals := service.snapshot()
	require.Len(t, producers, 1)
	require.Empty(t, producerRemovals)
	require.Empty(t, service.finalized())

	worker.waitErr = nil
	worker.joined = true
	require.NoError(t, generation.Wait(context.Background()))
	require.EqualValues(t, 2, worker.stops)
	require.False(t, generation.runtimeRegistered)
	_, _, producers, producerRemovals = service.snapshot()
	require.Empty(t, producers)
	require.Equal(t, []string{runtimeProducerName}, producerRemovals)
	require.Equal(t, []string{runtimeComponentName}, service.finalized())

	require.NoError(t, admission.CloseDrained(1))
	closeRunTestUIDs(t, uids)
}
