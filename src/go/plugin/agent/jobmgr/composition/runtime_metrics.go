// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/runtimecomp"
)

const (
	runtimeComponentName = "jobmgr.runtime"
	runtimeProducerName  = "jobmgr.runtime.projection"
	runtimeMetricPrefix  = "netdata.go.plugin.agent.jobmgr.runtime"
)

type runMetrics struct {
	store              metrix.RuntimeStore
	gauges             [lifecycle.RuntimeGaugeJobsActive + 1]metrix.StatefulGauge
	gaugeValues        [lifecycle.RuntimeGaugeJobsActive + 1]atomic.Int64
	counters           [lifecycle.RuntimeCounterDirtyRuns + 1]metrix.StatefulCounter
	counterValues      [lifecycle.RuntimeCounterDirtyRuns + 1]atomic.Uint64
	counterPublished   [lifecycle.RuntimeCounterDirtyRuns + 1]uint64
	timestamps         [lifecycle.RuntimeTimestampOldestTaskWait + 1]atomic.Int64
	ages               [lifecycle.RuntimeTimestampOldestTaskWait + 1]metrix.StatefulGauge
	projectionUpdateMu sync.Mutex
}

func newRunMetrics() *runMetrics {
	store := metrix.NewRuntimeStore()
	meter := store.Write().StatefulMeter(runtimeMetricPrefix)
	metrics := &runMetrics{store: store}
	metrics.gauges[lifecycle.RuntimeGaugeOperationsActive] =
		runtimeGauge(meter, "operations_active", "operations")
	metrics.gauges[lifecycle.RuntimeGaugeFunctionInvocationsActive] =
		runtimeGauge(meter, "function_invocations_active", "invocations")
	metrics.gauges[lifecycle.RuntimeGaugeClaimKeysTracked] =
		runtimeGauge(meter, "claim_keys_tracked", "keys")
	metrics.gauges[lifecycle.RuntimeGaugeClaimWaiters] =
		runtimeGauge(meter, "claim_waiters", "operations")
	metrics.gauges[lifecycle.RuntimeGaugeTasksActive] =
		runtimeGauge(meter, "tasks_active", "tasks")
	metrics.gauges[lifecycle.RuntimeGaugeTasksQueued] =
		runtimeGauge(meter, "tasks_queued", "tasks")
	metrics.gauges[lifecycle.RuntimeGaugeJobsActive] =
		runtimeGauge(meter, "jobs_active", "jobs")

	metrics.counters[lifecycle.RuntimeCounterOperationsAdmitted] =
		runtimeCounter(meter, "operations_admitted_total", "operations")
	metrics.counters[lifecycle.RuntimeCounterOperationsRejected] =
		runtimeCounter(meter, "operations_rejected_total", "operations")
	metrics.counters[lifecycle.RuntimeCounterDuplicateUIDRejected] =
		runtimeCounter(meter, "duplicate_uid_rejected_total", "operations")
	metrics.counters[lifecycle.RuntimeCounterShutdownRejected] =
		runtimeCounter(meter, "shutdown_rejected_total", "operations")
	metrics.counters[lifecycle.RuntimeCounterOperationTimeouts] =
		runtimeCounter(meter, "operation_timeouts_total", "operations")
	metrics.counters[lifecycle.RuntimeCounterResultsDisposed] =
		runtimeCounter(meter, "results_disposed_total", "results")
	metrics.counters[lifecycle.RuntimeCounterTaskPanics] =
		runtimeCounter(meter, "task_panics_total", "panics")
	metrics.counters[lifecycle.RuntimeCounterFramesCommitted] =
		runtimeCounter(meter, "frames_committed_total", "frames")
	metrics.counters[lifecycle.RuntimeCounterFrameFailures] =
		runtimeCounter(meter, "frame_failures_total", "failures")
	metrics.counters[lifecycle.RuntimeCounterDirtyRuns] =
		runtimeCounter(meter, "dirty_runs_total", "runs")

	metrics.ages[lifecycle.RuntimeTimestampOldestOperation] =
		runtimeAgeGauge(meter, "oldest_operation_age")
	metrics.ages[lifecycle.RuntimeTimestampOldestClaimWait] =
		runtimeAgeGauge(meter, "oldest_claim_wait_age")
	metrics.ages[lifecycle.RuntimeTimestampOldestTaskWait] =
		runtimeAgeGauge(meter, "oldest_task_wait_age")
	return metrics
}

func runtimeGauge(
	meter metrix.StatefulMeter,
	name string,
	unit string,
) metrix.StatefulGauge {
	return metrix.SeededGauge(
		meter,
		name,
		metrix.WithDescription("Current Job Manager "+name),
		metrix.WithChartFamily("Agent/JobMgr/Runtime"),
		metrix.WithUnit(unit),
	)
}

func runtimeCounter(
	meter metrix.StatefulMeter,
	name string,
	unit string,
) metrix.StatefulCounter {
	return metrix.SeededCounter(
		meter,
		name,
		metrix.WithDescription("Cumulative Job Manager "+name),
		metrix.WithChartFamily("Agent/JobMgr/Runtime"),
		metrix.WithUnit(unit),
	)
}

func runtimeAgeGauge(
	meter metrix.StatefulMeter,
	name string,
) metrix.StatefulGauge {
	return metrix.SeededGauge(
		meter,
		name,
		metrix.WithDescription("Age of the Job Manager "+name),
		metrix.WithChartFamily("Agent/JobMgr/Runtime"),
		metrix.WithUnit("seconds"),
	)
}

func (metrics *runMetrics) SetRuntimeGauge(
	kind lifecycle.RuntimeGauge,
	value int,
) {
	if metrics == nil ||
		kind < lifecycle.RuntimeGaugeOperationsActive ||
		int(kind) >= len(metrics.gauges) {
		return
	}
	metrics.gaugeValues[kind].Store(int64(value))
}

func (metrics *runMetrics) AddRuntimeGauge(
	kind lifecycle.RuntimeGauge,
	delta int,
) {
	if metrics == nil ||
		kind < lifecycle.RuntimeGaugeOperationsActive ||
		int(kind) >= len(metrics.gauges) {
		return
	}
	metrics.gaugeValues[kind].Add(int64(delta))
}

func (metrics *runMetrics) AddRuntimeCounter(
	kind lifecycle.RuntimeCounter,
	delta uint64,
) {
	if metrics == nil ||
		kind < lifecycle.RuntimeCounterOperationsAdmitted ||
		int(kind) >= len(metrics.counters) {
		return
	}
	metrics.counterValues[kind].Add(delta)
}

func (metrics *runMetrics) SetRuntimeTimestamp(
	kind lifecycle.RuntimeTimestamp,
	value time.Time,
) {
	if metrics == nil ||
		kind < lifecycle.RuntimeTimestampOldestOperation ||
		int(kind) >= len(metrics.timestamps) {
		return
	}
	if value.IsZero() {
		metrics.timestamps[kind].Store(0)
		return
	}
	metrics.timestamps[kind].Store(value.UnixNano())
}

func (metrics *runMetrics) refreshProjection() error {
	if metrics == nil {
		return nil
	}
	metrics.projectionUpdateMu.Lock()
	defer metrics.projectionUpdateMu.Unlock()

	for kind := lifecycle.RuntimeGaugeOperationsActive; kind <= lifecycle.RuntimeGaugeJobsActive; kind++ {
		metrics.gauges[kind].Set(
			float64(metrics.gaugeValues[kind].Load()),
		)
	}
	for kind := lifecycle.RuntimeCounterOperationsAdmitted; kind <= lifecycle.RuntimeCounterDirtyRuns; kind++ {
		current := metrics.counterValues[kind].Load()
		previous := metrics.counterPublished[kind]
		if current > previous {
			metrics.counters[kind].Add(float64(current - previous))
		}
		metrics.counterPublished[kind] = current
	}
	now := time.Now()
	for kind := lifecycle.RuntimeTimestampOldestOperation; kind <= lifecycle.RuntimeTimestampOldestTaskWait; kind++ {
		since := metrics.timestamps[kind].Load()
		age := float64(0)
		if since != 0 {
			age = now.Sub(time.Unix(0, since)).Seconds()
			if age < 0 {
				age = 0
			}
		}
		metrics.ages[kind].Set(age)
	}
	return nil
}

func (metrics *runMetrics) register(service runtimecomp.Service) error {
	if metrics == nil || service == nil {
		return nil
	}
	if err := service.RegisterComponent(runtimecomp.ComponentConfig{
		Name:        runtimeComponentName,
		Store:       metrics.store,
		UpdateEvery: 1,
		Autogen: runtimecomp.AutogenPolicy{
			Enabled: true,
		},
		Module:  "jobmgr",
		JobName: "runtime",
		JobLabels: map[string]string{
			"component": "jobmgr_runtime",
		},
	}); err != nil {
		return err
	}
	if err := service.RegisterProducer(
		runtimeProducerName,
		metrics.refreshProjection,
	); err != nil {
		service.UnregisterComponent(runtimeComponentName)
		return errors.Join(
			errors.New("jobmgr runtime metrics: register age producer"),
			err,
		)
	}
	return nil
}

func (metrics *runMetrics) unregister(service runtimecomp.Service) {
	if metrics == nil || service == nil {
		return
	}
	service.UnregisterProducer(runtimeProducerName)
	service.UnregisterComponent(runtimeComponentName)
}
