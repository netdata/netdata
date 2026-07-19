// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/runtimecomp"
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

func (service *runMetricsService) RegisterComponent(
	config runtimecomp.ComponentConfig,
) error {
	service.mu.Lock()
	defer service.mu.Unlock()
	service.components = append(service.components, config)
	return nil
}

func (service *runMetricsService) UnregisterComponent(name string) {
	service.mu.Lock()
	defer service.mu.Unlock()
	service.componentRemovals = append(service.componentRemovals, name)
}

func (*runMetricsService) QuarantineComponent(string) {}

func (service *runMetricsService) FinalizeComponent(name string) {
	service.mu.Lock()
	defer service.mu.Unlock()
	service.componentFinalized = append(service.componentFinalized, name)
}

func (service *runMetricsService) RegisterProducer(
	name string,
	producer func() error,
) error {
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.producerErr != nil {
		return service.producerErr
	}
	if service.producers == nil {
		service.producers = make(map[string]func() error)
	}
	service.producers[name] = producer
	return nil
}

func (service *runMetricsService) UnregisterProducer(name string) {
	service.mu.Lock()
	defer service.mu.Unlock()
	service.producerRemovals = append(service.producerRemovals, name)
	delete(service.producers, name)
}

func (service *runMetricsService) snapshot() (
	[]runtimecomp.ComponentConfig,
	[]string,
	map[string]func() error,
	[]string,
) {
	service.mu.Lock()
	defer service.mu.Unlock()
	components := append(
		[]runtimecomp.ComponentConfig(nil),
		service.components...,
	)
	componentRemovals := append(
		[]string(nil),
		service.componentRemovals...,
	)
	producers := make(map[string]func() error, len(service.producers))
	for name, producer := range service.producers {
		producers[name] = producer
	}
	producerRemovals := append(
		[]string(nil),
		service.producerRemovals...,
	)
	return components, componentRemovals, producers, producerRemovals
}

func (service *runMetricsService) finalized() []string {
	service.mu.Lock()
	defer service.mu.Unlock()
	return append([]string(nil), service.componentFinalized...)
}

func TestRunMetricsProjection(t *testing.T) {
	tests := map[string]struct {
		apply func(*runMetrics)
		name  string
		want  float64
	}{
		"sets owner gauge": {
			apply: func(metrics *runMetrics) {
				metrics.SetRuntimeGauge(
					lifecycle.RuntimeGaugeOperationsActive,
					7,
				)
			},
			name: runtimeMetricPrefix + ".operations_active",
			want: 7,
		},
		"adds shared owner gauge": {
			apply: func(metrics *runMetrics) {
				metrics.AddRuntimeGauge(
					lifecycle.RuntimeGaugeJobsActive,
					1,
				)
				metrics.AddRuntimeGauge(
					lifecycle.RuntimeGaugeJobsActive,
					1,
				)
				metrics.AddRuntimeGauge(
					lifecycle.RuntimeGaugeJobsActive,
					-1,
				)
			},
			name: runtimeMetricPrefix + ".jobs_active",
			want: 1,
		},
		"adds lifecycle counter": {
			apply: func(metrics *runMetrics) {
				metrics.AddRuntimeCounter(
					lifecycle.RuntimeCounterTaskPanics,
					2,
				)
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
			if err := metrics.refreshProjection(); err != nil {
				t.Fatal(err)
			}
			reader := metrics.store.Read(metrix.ReadRaw())
			got, ok := reader.Value(test.name, nil)
			if !ok {
				t.Fatalf("metric %q is absent", test.name)
			}
			if got != test.want {
				t.Fatalf("metric %q=%v want=%v", test.name, got, test.want)
			}
		})
	}
}

func TestRunMetricsOwnerUpdatesDoNotAllocate(t *testing.T) {
	metrics := newRunMetrics()
	now := time.Now()
	allocations := testing.AllocsPerRun(100, func() {
		metrics.SetRuntimeGauge(
			lifecycle.RuntimeGaugeOperationsActive,
			1,
		)
		metrics.AddRuntimeGauge(
			lifecycle.RuntimeGaugeJobsActive,
			1,
		)
		metrics.AddRuntimeCounter(
			lifecycle.RuntimeCounterOperationsAdmitted,
			1,
		)
		metrics.SetRuntimeTimestamp(
			lifecycle.RuntimeTimestampOldestOperation,
			now,
		)
	})
	if allocations != 0 {
		t.Fatalf("owner updates allocate %v objects per run", allocations)
	}
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
			if (err != nil) != test.wantErr {
				t.Fatalf("register error=%v wantErr=%v", err, test.wantErr)
			}
			components, removals, producers, _ := service.snapshot()
			if len(components) != test.wantComponents ||
				len(removals) != test.wantUnregistered ||
				len(producers) != test.wantProducers {
				t.Fatalf(
					"components=%d removals=%d producers=%d",
					len(components),
					len(removals),
					len(producers),
				)
			}
			if len(components) == 1 {
				config := components[0]
				if config.Name != runtimeComponentName ||
					config.Store != metrics.store ||
					!config.Autogen.Enabled {
					t.Fatalf("component config=%+v", config)
				}
			}
		})
	}
}

func TestRunGenerationRuntimeMetricsLifecycle(t *testing.T) {
	service := &runMetricsService{}
	jobs := testRunJobServices(t)
	jobs.Runtime = service
	frames, err := lifecycle.NewFrameOwner(&bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	components, removals, producers, producerRemovals :=
		service.snapshot()
	if len(components) != 1 || len(removals) != 0 ||
		len(producers) != 1 || len(producerRemovals) != 0 {
		t.Fatalf(
			"before start components=%d removals=%d producers=%d producerRemovals=%d",
			len(components),
			len(removals),
			len(producers),
			len(producerRemovals),
		)
	}
	if err := generation.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	generation.metrics.AddRuntimeCounter(
		lifecycle.RuntimeCounterDirtyRuns,
		1,
	)
	generation.Stop()
	if err := generation.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, removals, producers, producerRemovals = service.snapshot()
	finalized := service.finalized()
	if len(removals) != 0 ||
		len(finalized) != 1 ||
		finalized[0] != runtimeComponentName ||
		len(producers) != 0 || len(producerRemovals) != 1 ||
		producerRemovals[0] != runtimeProducerName {
		t.Fatalf(
			"after terminal removals=%v finalized=%v producers=%d producerRemovals=%v",
			removals,
			finalized,
			len(producers),
			producerRemovals,
		)
	}
	reader := components[0].Store.Read(metrix.ReadRaw())
	if got, ok := reader.Value(
		runtimeMetricPrefix+".dirty_runs_total",
		nil,
	); !ok || got != 1 {
		t.Fatalf("final dirty runs=%v present=%v want=1", got, ok)
	}
	if err := admission.CloseDrained(1); err != nil {
		t.Fatal(err)
	}
	closeRunTestUIDs(t, uids)
}
