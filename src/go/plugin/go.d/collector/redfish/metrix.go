// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/acquisition"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/measurement"
)

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
	vec := store.Write().SnapshotMeter("").Vec("endpoint_key")
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
		duration: vec.Gauge("collection_duration_seconds", metrix.WithFloat(true)),
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
	acquisition.Statistics
	Status   string
	Duration float64
}

func (m *collectorMetrics) observe(endpointKey string, cycle cycleMetrics) {
	labels := []string{endpointKey}
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
	for _, definition := range measurement.Definitions() {
		if len(definition.States) == 0 {
			var options []metrix.InstrumentOption
			if definition.Float {
				options = append(options, metrix.WithFloat(true))
			}
			result.gauges[definition.Name] = meter.Gauge(definition.Name, options...)
		} else {
			result.states[definition.Name] = meter.StateSet(definition.Name,
				metrix.WithStateSetMode(metrix.ModeEnum),
				metrix.WithStateSetStates(definition.States...),
			)
		}
	}
	return result
}

func (m *hardwareMetrics) observe(observations []measurement.Observation) {
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
