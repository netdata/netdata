// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import "github.com/netdata/netdata/go/plugins/pkg/metrix"

var (
	collectionStates     = []string{"success", "partial", "unavailable"}
	logRouteStates       = []string{"disabled", "ready", "unavailable"}
	logAdmissionStates   = []string{"disabled", "ready", "over_limit", "duplicate_owner", "unknown"}
	selectedSystemStates = []string{"present", "unreadable", "absent", "unknown"}
)

type collectorMetrics struct {
	status         metrix.SnapshotStateSetVec
	failures       map[string]metrix.SnapshotGaugeVec
	duration       metrix.SnapshotGaugeVec
	httpRequests   map[string]metrix.SnapshotGaugeVec
	operations     map[string]metrix.SnapshotGaugeVec
	traffic        metrix.SnapshotGaugeVec
	resources      map[string]metrix.SnapshotGaugeVec
	logsRoute      metrix.SnapshotStateSetVec
	logsAdmission  metrix.SnapshotStateSetVec
	logServices    map[string]metrix.SnapshotGaugeVec
	selectedSystem metrix.SnapshotStateSetVec
}

func newCollectorMetrics(store metrix.CollectorStore) *collectorMetrics {
	vec := store.Write().SnapshotMeter("").Vec("endpoint_key", "endpoint_job")
	return &collectorMetrics{
		status: vec.StateSet(
			"collection_status",
			metrix.WithStateSetMode(metrix.ModeEnum),
			metrix.WithStateSetStates(collectionStates...),
		),
		failures: gaugeMap(vec, "collection_failures", "auth", "tls", "transport", "timeout", "protocol", "limit", "internal"),
		duration: vec.Gauge("collection_duration_seconds"),
		httpRequests: gaugeMap(
			vec,
			"collection_http_requests",
			"started",
			"retried",
			"redirected",
		),
		operations: gaugeMap(vec, "collection_operations", "successful", "failed"),
		traffic:    vec.Gauge("collection_traffic_received_bytes"),
		resources:  gaugeMap(vec, "collection_resources", "discovered", "readable", "unreadable", "unknown"),
		logsRoute: vec.StateSet(
			"collection_logs_route",
			metrix.WithStateSetMode(metrix.ModeEnum),
			metrix.WithStateSetStates(logRouteStates...),
		),
		logsAdmission: vec.StateSet(
			"collection_logs_admission",
			metrix.WithStateSetMode(metrix.ModeEnum),
			metrix.WithStateSetStates(logAdmissionStates...),
		),
		logServices: gaugeMap(vec, "collection_log_services", "discovered", "selected", "admitted", "limit"),
		selectedSystem: vec.StateSet(
			"collection_selected_system",
			metrix.WithStateSetMode(metrix.ModeEnum),
			metrix.WithStateSetStates(selectedSystemStates...),
		),
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
	Status         string
	Failures       map[string]int
	Duration       float64
	HTTPRequests   map[string]int
	Operations     map[string]int
	ReceivedBytes  int64
	Resources      map[string]int
	LogsRoute      string
	LogsAdmission  string
	LogServices    map[string]int
	SelectedSystem string
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
	if cycle.LogsRoute != "" {
		m.logsRoute.WithLabelValues(labels...).Enable(cycle.LogsRoute)
	}
	if cycle.LogsAdmission != "" {
		m.logsAdmission.WithLabelValues(labels...).Enable(cycle.LogsAdmission)
	}
	observeGaugeMap(m.logServices, labels, cycle.LogServices)
	if cycle.SelectedSystem != "" {
		m.selectedSystem.WithLabelValues(labels...).Enable(cycle.SelectedSystem)
	}
}

func observeGaugeMap(writers map[string]metrix.SnapshotGaugeVec, labels []string, values map[string]int) {
	for name, writer := range writers {
		writer.WithLabelValues(labels...).Observe(float64(values[name]))
	}
}
