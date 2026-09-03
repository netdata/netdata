// SPDX-License-Identifier: GPL-3.0-or-later

package s3check

import (
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/s3check/internal/contract"
)

var (
	runtimeStates = []string{string(contract.StatusSuccess), string(contract.StatusFailed)}
	probeStates   = []string{
		string(contract.StatusSuccess),
		string(contract.StatusWaiting),
		string(contract.StatusFailed),
	}
)

type operationKey struct {
	mode      contract.Mode
	endpoint  contract.Endpoint
	operation contract.Operation
}

type operationTotals struct {
	calls    int64
	failures int64
}

type operationCycle struct {
	status   contract.Status
	reason   contract.Reason
	duration time.Duration
	calls    int64
	failures int64
}

type metricRenderer struct {
	store           metrix.CollectorStore
	sourceName      string
	destinationName string
	operations      map[operationKey]operationTotals
}

func newMetricRenderer(store metrix.CollectorStore) *metricRenderer {
	return &metricRenderer{
		store:      store,
		operations: make(map[operationKey]operationTotals),
	}
}

func (r *metricRenderer) reset(sourceName, destinationName string) {
	r.sourceName = sourceName
	r.destinationName = destinationName
	clear(r.operations)
}

func (r *metricRenderer) write(result contract.Result) {
	meter := r.store.Write().SnapshotMeter("")
	r.writeRuntime(meter, result)
	r.writeOperations(meter, result)
	r.writeProbe(meter, result)

	labels := r.commonLabels(result.Mode)
	meter.Gauge("cleanup_pending_objects").Observe(
		metrix.SampleValue(result.Cleanup.Pending), meter.LabelSet(labels...),
	)
	backpressure := int64(0)
	if result.Cleanup.Backpressure {
		backpressure = 1
	}
	meter.Gauge("mutation_backpressure").Observe(
		metrix.SampleValue(backpressure), meter.LabelSet(labels...),
	)
}

func (r *metricRenderer) writeRuntime(meter metrix.SnapshotMeter, result contract.Result) {
	status := contract.StatusSuccess
	reason := contract.ReasonNone
	if result.Err != nil {
		status = contract.StatusFailed
		reason = contract.ReasonInternal
		for _, operation := range result.Operations {
			if operation.Status == contract.StatusFailed {
				reason = operation.Reason
				break
			}
		}
	}
	labels := append(r.commonLabels(result.Mode), metrix.Label{
		Key:   "reason",
		Value: string(reason),
	})
	meter.WithLabels(labels...).StateSet(
		"runtime_status",
		metrix.WithStateSetMode(metrix.ModeEnum),
		metrix.WithStateSetStates(runtimeStates...),
	).Enable(string(status))
}

func (r *metricRenderer) writeOperations(meter metrix.SnapshotMeter, result contract.Result) {
	cycles := make(map[operationKey]operationCycle)
	for _, operation := range result.Operations {
		key := operationKey{
			mode:      result.Mode,
			endpoint:  operation.Endpoint,
			operation: operation.Name,
		}
		cycle := cycles[key]
		if cycle.calls == 0 {
			cycle.status = contract.StatusSuccess
			cycle.reason = contract.ReasonNone
		}
		if operation.Status == contract.StatusFailed {
			cycle.status = contract.StatusFailed
			if cycle.failures == 0 {
				cycle.reason = operation.Reason
			}
		}
		cycle.duration += operation.Duration
		cycle.calls++
		if operation.Status == contract.StatusFailed {
			cycle.failures++
		}
		cycles[key] = cycle
	}

	for key, cycle := range cycles {
		labels := append(r.commonLabels(key.mode),
			metrix.Label{
				Key:   "endpoint",
				Value: string(key.endpoint),
			},
			metrix.Label{
				Key:   "operation",
				Value: string(key.operation),
			},
			metrix.Label{
				Key:   "reason",
				Value: string(cycle.reason),
			},
		)
		labelSet := meter.LabelSet(labels...)
		meter.WithLabels(labels...).StateSet(
			"operation_status",
			metrix.WithStateSetMode(metrix.ModeEnum),
			metrix.WithStateSetStates(runtimeStates...),
		).Enable(string(cycle.status))
		meter.Gauge("operation_duration_seconds").Observe(
			metrix.SampleValue(cycle.duration.Seconds()), labelSet,
		)

		totals := r.operations[key]
		totals.calls += cycle.calls
		totals.failures += cycle.failures
		r.operations[key] = totals
		counterLabels := meter.LabelSet(append(
			r.commonLabels(key.mode),
			metrix.Label{
				Key:   "endpoint",
				Value: string(key.endpoint),
			},
			metrix.Label{
				Key:   "operation",
				Value: string(key.operation),
			},
		)...)
		meter.Counter("operation_calls_total").ObserveTotal(metrix.SampleValue(totals.calls), counterLabels)
		meter.Counter("operation_failures_total").ObserveTotal(metrix.SampleValue(totals.failures), counterLabels)
	}
}

func (r *metricRenderer) writeProbe(meter metrix.SnapshotMeter, result contract.Result) {
	probe := result.Probe
	if probe == nil {
		probe = result.LastTerminal
	}
	if probe == nil {
		return
	}
	labels := append(r.commonLabels(result.Mode), metrix.Label{
		Key:   "reason",
		Value: string(probe.Reason),
	})
	labelSet := meter.LabelSet(labels...)
	meter.WithLabels(labels...).StateSet(
		"probe_status",
		metrix.WithStateSetMode(metrix.ModeEnum),
		metrix.WithStateSetStates(probeStates...),
	).Enable(string(probe.Status))

	if probe.PayloadCompared {
		mismatch := int64(0)
		if probe.PayloadMismatch {
			mismatch = 1
		}
		meter.Gauge("payload_mismatch").Observe(metrix.SampleValue(mismatch), labelSet)
	}
	r.writeObjective(meter, "write_visibility", probe.WriteVisibility, labelSet)
	r.writeObjective(meter, "delete_visibility", probe.DeleteVisibility, labelSet)
}

func (r *metricRenderer) writeObjective(
	meter metrix.SnapshotMeter,
	name string,
	objective contract.ObjectiveResult,
	labels metrix.LabelSet,
) {
	if !objective.Performed {
		return
	}
	meter.Gauge(name+"_lag_seconds").Observe(metrix.SampleValue(objective.Lag.Seconds()), labels)
	breached := int64(0)
	if objective.Status == contract.StatusFailed {
		breached = 1
	}
	meter.Gauge(name+"_objective_breached").Observe(metrix.SampleValue(breached), labels)
}

func (r *metricRenderer) commonLabels(mode contract.Mode) []metrix.Label {
	return []metrix.Label{
		{Key: "mode", Value: string(mode)},
		{Key: "source", Value: r.sourceName},
		{Key: "destination", Value: r.destinationName},
	}
}
