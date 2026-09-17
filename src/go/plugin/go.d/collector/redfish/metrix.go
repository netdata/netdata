// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import "github.com/netdata/netdata/go/plugins/pkg/metrix"

var (
	collectionStates = []string{"success", "partial", "unavailable"}
)

type collectorMetrics struct {
	status       metrix.SnapshotStateSetVec
	failures     map[string]metrix.SnapshotGaugeVec
	duration     metrix.SnapshotGaugeVec
	httpRequests map[string]metrix.SnapshotGaugeVec
	operations   map[string]metrix.SnapshotGaugeVec
	traffic      metrix.SnapshotGaugeVec
	resources    map[string]metrix.SnapshotGaugeVec
}

func newCollectorMetrics(store metrix.CollectorStore) *collectorMetrics {
	vec := store.Write().SnapshotMeter("").Vec("endpoint_key", "endpoint_job")
	return &collectorMetrics{
		status: vec.StateSet(
			"collection_status",
			metrix.WithStateSetMode(metrix.ModeEnum),
			metrix.WithStateSetStates(collectionStates...),
		),
		failures: gaugeMap(
			vec,
			"collection_failures",
			"auth",
			"tls",
			"transport",
			"timeout",
			"protocol",
			"limit",
			"internal",
		),
		duration: vec.Gauge("collection_duration_seconds"),
		httpRequests: gaugeMap(
			vec,
			"collection_http_requests",
			"started",
			"redirected",
		),
		operations: gaugeMap(vec, "collection_operations", "successful", "failed"),
		traffic:    vec.Gauge("collection_traffic_received_bytes"),
		resources:  gaugeMap(vec, "collection_resources", "discovered", "readable", "unreadable", "unknown"),
	}
}

func gaugeMap(vec metrix.SnapshotVecMeter, prefix string, names ...string) map[string]metrix.SnapshotGaugeVec {
	result := make(map[string]metrix.SnapshotGaugeVec, len(names))
	for _, name := range names {
		result[name] = vec.Gauge(prefix + "_" + name)
	}
	return result
}

type cycleMetrics struct {
	Status        string
	Failures      map[string]int
	Duration      float64
	HTTPRequests  map[string]int
	Operations    map[string]int
	ReceivedBytes int64
	Resources     map[string]int
}

func (m *collectorMetrics) observe(endpointKey, endpointJob string, cycle cycleMetrics) {
	labels := []string{endpointKey, endpointJob}
	if cycle.Status != "" {
		m.status.WithLabelValues(labels...).Enable(cycle.Status)
	}
	observeGaugeMap(m.failures, labels, cycle.Failures)
	m.duration.WithLabelValues(labels...).Observe(cycle.Duration)
	observeGaugeMap(m.httpRequests, labels, cycle.HTTPRequests)
	observeGaugeMap(m.operations, labels, cycle.Operations)
	m.traffic.WithLabelValues(labels...).Observe(float64(cycle.ReceivedBytes))
	observeGaugeMap(m.resources, labels, cycle.Resources)
}

func observeGaugeMap(writers map[string]metrix.SnapshotGaugeVec, labels []string, values map[string]int) {
	for name, writer := range writers {
		writer.WithLabelValues(labels...).Observe(float64(values[name]))
	}
}

type hardwareMetrics struct {
	meter  metrix.SnapshotMeter
	gauges map[string]metrix.SnapshotGauge
	states map[string]metrix.StateSetInstrument
}

func newHardwareMetrics(store metrix.CollectorStore) *hardwareMetrics {
	meter := store.Write().SnapshotMeter("")
	result := &hardwareMetrics{
		meter:  meter,
		gauges: make(map[string]metrix.SnapshotGauge),
		states: make(map[string]metrix.StateSetInstrument),
	}
	gauge := func(metric string) {
		if _, exists := result.gauges[metric]; !exists {
			result.gauges[metric] = meter.Gauge(metric)
		}
	}
	states := func(metric string, values []string) {
		if _, exists := result.states[metric]; !exists {
			result.states[metric] = meter.StateSet(
				metric,
				metrix.WithStateSetMode(metrix.ModeEnum),
				metrix.WithStateSetStates(values...),
			)
		}
	}
	for _, field := range scalarFields {
		gauge(field.Metric)
	}
	for _, reading := range readingDescriptors {
		gauge(reading.Metric)
		if reading.AlarmMetric != "" {
			states(reading.AlarmMetric, alarmStates)
		}
	}
	for kind, status := range sourceStatusByKind {
		states(kind+"_acquisition_state", acquisitionStates)
		if status.Status {
			states(kind+"_health", healthStates)
			states(kind+"_health_rollup", healthStates)
			states(kind+"_state", resourceStates)
			for _, state := range healthStates {
				gauge(kind + "_conditions_" + state)
			}
		}
		if status.PowerState {
			states(kind+"_power_state", powerStates)
		}
		if status.FailurePredicted {
			states(kind+"_failure_predicted", failureStates)
		}
	}
	for _, source := range additionalStateSources {
		states(source.Metric, source.States)
	}
	for _, set := range sourceFlagSets {
		for _, member := range set.Members {
			gauge(set.Metric + "_" + member.Role)
		}
	}
	return result
}

func (m *hardwareMetrics) observe(observations []hardwareObservation) {
	for _, observation := range observations {
		labels := m.meter.LabelSet(observation.Labels...)
		if observation.State != "" {
			m.states[observation.Metric].ObserveStateSet(
				metrix.StateSetPoint{
					States: map[string]bool{observation.State: true},
				},
				labels,
			)
		} else {
			m.gauges[observation.Metric].Observe(observation.Value, labels)
		}
	}
}
