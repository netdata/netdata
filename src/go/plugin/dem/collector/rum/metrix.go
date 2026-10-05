// SPDX-License-Identifier: GPL-3.0-or-later
package rum

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/aggregate"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
)

type counterMetric struct {
	name       string
	instrument metrix.SnapshotCounter
}
type percentileMetrics struct{ p50, p75, p95 metrix.SnapshotGauge }
type vitalMetrics struct {
	percentileMetrics
	good, needsImprovement, poor metrix.SnapshotGauge
}
type collectorMetrics struct {
	meter                                           metrix.SnapshotMeter
	ingress                                         metrix.StateSetInstrument
	diagnostics, traffic                            []counterMetric
	activeSessions, windowPageviews, windowJSErrors metrix.SnapshotGauge
	vitals                                          map[string]vitalMetrics
	load, dcl, api                                  percentileMetrics
	firstPartyResources, thirdPartyResources        metrix.SnapshotCounter
}

func newCollectorMetrics(m metrix.SnapshotMeter, logs, traces, frustrationSignals bool) collectorMetrics {
	metrics := collectorMetrics{
		meter: m,
		ingress: m.StateSet(
			"ingress_state",
			metrix.WithStateSetStates("available", "unavailable"),
			metrix.WithStateSetMode(metrix.ModeEnum),
		),
		activeSessions:      m.Gauge("active_sessions"),
		windowPageviews:     m.Gauge("window_pageviews"),
		windowJSErrors:      m.Gauge("window_js_errors"),
		vitals:              make(map[string]vitalMetrics, len(beacon.Vitals)),
		load:                newPercentileMetrics(m, "load"),
		dcl:                 newPercentileMetrics(m, "dcl"),
		api:                 newPercentileMetrics(m, "api"),
		firstPartyResources: m.Counter("resources_first_party"),
		thirdPartyResources: m.Counter("resources_third_party"),
	}
	diagnostics := []string{aggregate.CounterHistoryWritten, aggregate.CounterHistoryDropped}
	if logs {
		diagnostics = append(
			diagnostics,
			aggregate.CounterOTLPSent,
			aggregate.CounterOTLPDropped,
			aggregate.CounterOTLPErrors,
		)
	}
	if traces {
		diagnostics = append(
			diagnostics,
			aggregate.CounterSpansSent,
			aggregate.CounterSpansDropped,
			aggregate.CounterSpansErrors,
		)
	}
	for _, name := range diagnostics {
		metrics.diagnostics = append(metrics.diagnostics, counterMetric{
			name:       name,
			instrument: m.Counter(name),
		})
	}
	traffic := []string{aggregate.CounterPageviews, aggregate.CounterJSErrors, aggregate.CounterAccepted, beacon.RejectOrigin, beacon.RejectRate, beacon.RejectSize, beacon.RejectInvalid, beacon.RejectBot, aggregate.CounterSamplesDropped}
	if frustrationSignals {
		traffic = append(traffic, aggregate.CounterRageClicks, aggregate.CounterDeadClicks, aggregate.CounterErrorClicks)
	}
	for _, name := range traffic {
		metrics.traffic = append(metrics.traffic, counterMetric{
			name:       name,
			instrument: m.Counter(name),
		})
	}
	for _, vital := range beacon.Vitals {
		name := strings.ToLower(vital)
		metrics.vitals[vital] = vitalMetrics{
			percentileMetrics: newPercentileMetrics(m, name),
			good:              m.Gauge(name + "_good"),
			needsImprovement:  m.Gauge(name + "_needs_improvement"),
			poor:              m.Gauge(name + "_poor"),
		}
	}
	return metrics
}
func newPercentileMetrics(m metrix.SnapshotMeter, name string) percentileMetrics {
	return percentileMetrics{
		p50: m.Gauge(name+"_p50", metrix.WithFloat(true)),
		p75: m.Gauge(name+"_p75", metrix.WithFloat(true)),
		p95: m.Gauge(name+"_p95", metrix.WithFloat(true)),
	}
}
func (m percentileMetrics) observe(st aggregate.VitalStats) {
	if st.N == 0 {
		return
	}
	m.p50.Observe(st.P50)
	m.p75.Observe(st.P75)
	m.p95.Observe(st.P95)
}
