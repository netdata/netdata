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
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/containment"
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
	finalizeEntered    chan<- struct{}
	finalizeRelease    <-chan struct{}
}

func (rms *runMetricsService) RegisterComponent(config runtimecomp.ComponentConfig) error {
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
	entered := rms.finalizeEntered
	release := rms.finalizeRelease
	rms.mu.Unlock()
	if entered != nil {
		close(entered)
	}
	if release != nil {
		<-release
	}
	rms.mu.Lock()
	rms.componentFinalized = append(rms.componentFinalized, name)
	rms.mu.Unlock()
}

func (rms *runMetricsService) RegisterProducer(name string, producer func() error) error {
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

func newTestRunMetrics(t *testing.T) *runMetrics {
	t.Helper()
	attempts, err := containment.NewAuthority(nil)
	require.NoError(t, err)
	metrics, err := newRunMetrics(attempts)
	require.NoError(t, err)
	return metrics
}

func TestNewRunMetricsRejectsNilProcessAttemptAuthority(t *testing.T) {
	metrics, err := newRunMetrics(nil)
	require.Nil(t, metrics)
	require.Error(t, err)
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
				metrics.SetRuntimeTimestamp(lifecycle.RuntimeTimestampOldestOperation, time.Time{})
			},
			name: runtimeMetricPrefix + ".oldest_operation_age",
			want: 0,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			metrics := newTestRunMetrics(t)
			test.apply(metrics)

			require.NoError(t, metrics.refreshProjection())

			reader := metrics.store.Read(metrix.ReadRaw())
			got, ok := reader.Value(test.name, nil)
			require.True(t, ok)
			require.EqualValues(t, test.want, got)
		})
	}
}

func TestRunMetricsProjectsProcessAttemptCensus(t *testing.T) {
	attempts, err := containment.NewAuthority(nil)
	require.NoError(t, err)

	metrics, err := newRunMetrics(attempts)
	require.NoError(t, err)
	assertCensus := func(want containment.Census) {
		t.Helper()
		require.Equal(t, want, attempts.Census())
		require.NoError(t, metrics.refreshProjection())
		reader := metrics.store.Read(metrix.ReadRaw())
		for name, value := range map[string]int{
			"process_attempts_active":      want.Active,
			"process_attempts_probing":     want.Probing,
			"process_attempts_admitted":    want.Admitted,
			"process_attempts_contained":   want.Contained,
			"process_attempts_quarantined": want.Quarantined,
		} {
			got, ok := reader.Value(runtimeMetricPrefix+"."+name, nil)
			require.True(t, ok, name)
			require.EqualValues(t, value, got, name)
		}
	}

	started := make(chan struct{})
	admit := make(chan struct{})
	admitted := make(chan error, 1)
	release := make(chan struct{})
	var admitOnce sync.Once
	var releaseOnce sync.Once
	attempt, err := attempts.StartProcessAttempt(context.Background(), jobmgr.ProcessAttemptPlan{
		Identity: jobmgr.ProcessAttemptIdentity{
			Namespace: jobmgr.ProcessAttemptJobRuntime,
			Key:       "module/job",
			Resource:  "module/job",
		},
		Target: 1,
		Work: func(_ context.Context, admission jobmgr.ProcessAttemptAdmission) error {
			close(started)
			<-admit
			err := admission.Admit()
			admitted <- err
			if err != nil {
				return err
			}
			<-release
			return nil
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		admitOnce.Do(func() { close(admit) })
		releaseOnce.Do(func() { close(release) })
		<-attempt.Released()
	})

	<-started
	assertCensus(containment.Census{Active: 1, Probing: 1})

	admitOnce.Do(func() { close(admit) })
	require.NoError(t, <-admitted)
	assertCensus(containment.Census{Active: 1, Admitted: 1})

	require.True(t, attempt.Cut(jobmgr.ErrProcessAttemptDeadline))
	require.ErrorIs(t, attempt.Await(context.Background()), jobmgr.ErrProcessAttemptDeadline)
	assertCensus(containment.Census{Active: 1, Contained: 1})

	releaseOnce.Do(func() { close(release) })
	<-attempt.Released()
	assertCensus(containment.Census{})

	quarantined, err := attempts.StartProcessAttempt(context.Background(), jobmgr.ProcessAttemptPlan{
		Identity: jobmgr.ProcessAttemptIdentity{
			Namespace: jobmgr.ProcessAttemptJobRuntime,
			Key:       "module/quarantined",
			Resource:  "module/quarantined",
		},
		Target: 1,
		Work: func(_ context.Context, admission jobmgr.ProcessAttemptAdmission) error {
			require.NoError(t, admission.Admit())
			return errors.New("cleanup failed")
		},
	})
	require.NoError(t, err)
	require.ErrorIs(t, quarantined.Await(context.Background()), jobmgr.ErrProcessAttemptQuarantined)
	<-quarantined.Released()
	assertCensus(containment.Census{Quarantined: 1})
}

func TestRunMetricsOwnerUpdatesDoNotAllocate(t *testing.T) {
	metrics := newTestRunMetrics(t)
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
		"registers component and producer": {wantComponents: 1, wantProducers: 1},
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
			metrics := newTestRunMetrics(t)
			err := metrics.register(service)
			require.EqualValues(t, test.wantErr, err != nil)
			components, removals, producers, _ := service.snapshot()
			require.False(t, len(components) != test.wantComponents ||
				len(removals) != test.wantUnregistered ||
				len(producers) != test.wantProducers)
			if len(components) == 1 {
				config := components[0]
				require.False(
					t,
					config.Name != runtimeComponentName || config.Store != metrics.store || !config.Autogen.Enabled,
				)
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
	uids := lifecycle.NewUIDLedger()
	generation, err := newTestRunGeneration(t, runGenerationConfig{
		Generation:      1,
		ShutdownTimeout: time.Second,
		UIDs:            uids,
		Frames:          frames,
		Modules:         collectorapi.Registry{},
		Jobs:            jobs,
		Discovery:       testRunDiscoveryServices(t),
	})
	require.NoError(t, err)
	components, removals, producers, producerRemovals := service.snapshot()
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

	closeRunTestUIDs(t, uids)
}

func TestRunGenerationFinalizerStopsRuntimeWriterBeforeTerminalCensus(t *testing.T) {
	finalizeEntered := make(chan struct{})
	finalizeRelease := make(chan struct{})
	service := &runMetricsService{
		finalizeEntered: finalizeEntered,
		finalizeRelease: finalizeRelease,
	}
	jobs := testRunJobServices(t)
	jobs.Runtime = service
	frames, err := lifecycle.NewFrameOwner(&bytes.Buffer{})
	require.NoError(t, err)
	uids := lifecycle.NewUIDLedger()
	generation, err := newTestRunGeneration(t, runGenerationConfig{
		Generation:      1,
		ShutdownTimeout: time.Second,
		UIDs:            uids,
		Frames:          frames,
		Modules:         collectorapi.Registry{},
		Jobs:            jobs,
		Discovery:       testRunDiscoveryServices(t),
	})
	require.NoError(t, err)
	require.NoError(t, generation.start(context.Background()))

	generation.Stop()
	waited := make(chan error, 1)
	go func() { waited <- generation.Wait(context.Background()) }()
	select {
	case <-finalizeEntered:
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "runtime component did not enter finalization")
	}
	select {
	case <-generation.kernel.Done():
		require.FailNow(t, "test failed", "kernel reached terminal before runtime writer stopped")
	default:
	}
	close(finalizeRelease)
	require.NoError(t, <-waited)
	closeRunTestUIDs(t, uids)
}
