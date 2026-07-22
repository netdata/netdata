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
	store              metrix.RuntimeStore                                                // runtime metrics store
	gauges             [lifecycle.RuntimeGaugeJobsActive + 1]metrix.StatefulGauge         // stateful gauges by RuntimeGauge id
	gaugeValues        [lifecycle.RuntimeGaugeJobsActive + 1]atomic.Int64                 // current gauge values (written by mutation owners)
	counters           [lifecycle.RuntimeCounterDirtyRuns + 1]metrix.StatefulCounter      // stateful counters by RuntimeCounter id
	counterValues      [lifecycle.RuntimeCounterDirtyRuns + 1]atomic.Uint64               // current counter values (written by mutation owners)
	counterPublished   [lifecycle.RuntimeCounterDirtyRuns + 1]uint64                      // last published counter values (delta computation)
	timestamps         [lifecycle.RuntimeTimestampOldestTaskWait + 1]atomic.Int64         // event timestamps by RuntimeTimestamp id
	ages               [lifecycle.RuntimeTimestampOldestTaskWait + 1]metrix.StatefulGauge // age gauges derived from timestamps
	projectionUpdateMu sync.Mutex                                                         // serializes projection snapshots
}

func newRunMetrics() *runMetrics {
	store := metrix.NewRuntimeStore()
	meter := store.Write().StatefulMeter(runtimeMetricPrefix)
	metrics := &runMetrics{
		store: store,
	}
	metrics.gauges[lifecycle.RuntimeGaugeOperationsActive] = runtimeGauge(meter, "operations_active", "operations")
	metrics.gauges[lifecycle.RuntimeGaugeFunctionInvocationsActive] =
		runtimeGauge(meter, "function_invocations_active", "invocations")
	metrics.gauges[lifecycle.RuntimeGaugeClaimKeysTracked] = runtimeGauge(meter, "claim_keys_tracked", "keys")
	metrics.gauges[lifecycle.RuntimeGaugeClaimWaiters] = runtimeGauge(meter, "claim_waiters", "operations")
	metrics.gauges[lifecycle.RuntimeGaugeTasksActive] = runtimeGauge(meter, "tasks_active", "tasks")
	metrics.gauges[lifecycle.RuntimeGaugeTasksQueued] = runtimeGauge(meter, "tasks_queued", "tasks")
	metrics.gauges[lifecycle.RuntimeGaugeJobsActive] = runtimeGauge(meter, "jobs_active", "jobs")

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
	metrics.counters[lifecycle.RuntimeCounterTaskPanics] = runtimeCounter(meter, "task_panics_total", "panics")
	metrics.counters[lifecycle.RuntimeCounterFramesCommitted] =
		runtimeCounter(meter, "frames_committed_total", "frames")
	metrics.counters[lifecycle.RuntimeCounterFrameFailures] = runtimeCounter(meter, "frame_failures_total", "failures")
	metrics.counters[lifecycle.RuntimeCounterDirtyRuns] = runtimeCounter(meter, "dirty_runs_total", "runs")

	metrics.ages[lifecycle.RuntimeTimestampOldestOperation] = runtimeAgeGauge(meter, "oldest_operation_age")
	metrics.ages[lifecycle.RuntimeTimestampOldestClaimWait] = runtimeAgeGauge(meter, "oldest_claim_wait_age")
	metrics.ages[lifecycle.RuntimeTimestampOldestTaskWait] = runtimeAgeGauge(meter, "oldest_task_wait_age")
	return metrics
}

func runtimeGauge(meter metrix.StatefulMeter, name string, unit string) metrix.StatefulGauge {
	return metrix.SeededGauge(
		meter,
		name,
		metrix.WithDescription("Current Job Manager "+name),
		metrix.WithChartFamily("Agent/JobMgr/Runtime"),
		metrix.WithUnit(unit),
	)
}

func runtimeCounter(meter metrix.StatefulMeter, name string, unit string) metrix.StatefulCounter {
	return metrix.SeededCounter(
		meter,
		name,
		metrix.WithDescription("Cumulative Job Manager "+name),
		metrix.WithChartFamily("Agent/JobMgr/Runtime"),
		metrix.WithUnit(unit),
	)
}

func runtimeAgeGauge(meter metrix.StatefulMeter, name string) metrix.StatefulGauge {
	return metrix.SeededGauge(
		meter,
		name,
		metrix.WithDescription("Age of the Job Manager "+name),
		metrix.WithChartFamily("Agent/JobMgr/Runtime"),
		metrix.WithUnit("seconds"),
	)
}

func (rm *runMetrics) SetRuntimeGauge(kind lifecycle.RuntimeGauge, value int) {
	if rm == nil || kind < lifecycle.RuntimeGaugeOperationsActive || int(kind) >= len(rm.gauges) {
		return
	}
	rm.gaugeValues[kind].Store(int64(value))
}

func (rm *runMetrics) AddRuntimeGauge(kind lifecycle.RuntimeGauge, delta int) {
	if rm == nil || kind < lifecycle.RuntimeGaugeOperationsActive || int(kind) >= len(rm.gauges) {
		return
	}
	rm.gaugeValues[kind].Add(int64(delta))
}

func (rm *runMetrics) AddRuntimeCounter(kind lifecycle.RuntimeCounter, delta uint64) {
	if rm == nil || kind < lifecycle.RuntimeCounterOperationsAdmitted || int(kind) >= len(rm.counters) {
		return
	}
	rm.counterValues[kind].Add(delta)
}

func (rm *runMetrics) SetRuntimeTimestamp(kind lifecycle.RuntimeTimestamp, value time.Time) {
	if rm == nil || kind < lifecycle.RuntimeTimestampOldestOperation || int(kind) >= len(rm.timestamps) {
		return
	}
	if value.IsZero() {
		rm.timestamps[kind].Store(0)
		return
	}
	rm.timestamps[kind].Store(value.UnixNano())
}

func (rm *runMetrics) refreshProjection() error {
	if rm == nil {
		return nil
	}
	rm.projectionUpdateMu.Lock()
	defer rm.projectionUpdateMu.Unlock()

	for kind := lifecycle.RuntimeGaugeOperationsActive; kind <= lifecycle.RuntimeGaugeJobsActive; kind++ {
		rm.gauges[kind].Set(float64(rm.gaugeValues[kind].Load()))
	}
	for kind := lifecycle.RuntimeCounterOperationsAdmitted; kind <= lifecycle.RuntimeCounterDirtyRuns; kind++ {
		current := rm.counterValues[kind].Load()
		previous := rm.counterPublished[kind]
		if current > previous {
			rm.counters[kind].Add(float64(current - previous))
		}
		rm.counterPublished[kind] = current
	}
	now := time.Now()
	for kind := lifecycle.RuntimeTimestampOldestOperation; kind <= lifecycle.RuntimeTimestampOldestTaskWait; kind++ {
		since := rm.timestamps[kind].Load()
		age := float64(0)
		if since != 0 {
			age = now.Sub(time.Unix(0, since)).Seconds()
			if age < 0 {
				age = 0
			}
		}
		rm.ages[kind].Set(age)
	}
	return nil
}

func (rm *runMetrics) register(service runtimecomp.Service) error {
	if rm == nil || service == nil {
		return nil
	}
	if err := service.RegisterComponent(runtimecomp.ComponentConfig{
		Name:        runtimeComponentName,
		Store:       rm.store,
		UpdateEvery: 1,
		Autogen: runtimecomp.AutogenPolicy{
			Enabled: true,
		},
		Module:    "jobmgr",
		JobName:   "runtime",
		JobLabels: map[string]string{"component": "jobmgr_runtime"},
	}); err != nil {
		return err
	}
	if err := service.RegisterProducer(runtimeProducerName, rm.refreshProjection); err != nil {
		service.UnregisterComponent(runtimeComponentName)
		return errors.Join(errors.New("jobmgr runtime metrics: register projection producer"), err)
	}
	return nil
}

func (rm *runMetrics) unregister(service runtimecomp.Service) error {
	if rm == nil || service == nil {
		return nil
	}
	service.UnregisterProducer(runtimeProducerName)
	// refreshProjection cannot fail today; the error branch mirrors the
	// producer-registration contract and quarantines defensively if that
	// ever changes.
	if err := rm.refreshProjection(); err != nil {
		service.QuarantineComponent(runtimeComponentName)
		return err
	}
	service.FinalizeComponent(runtimeComponentName)
	return nil
}
