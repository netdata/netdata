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
type percentileMetrics struct{ p50, p75, p95, observations, lost metrix.SnapshotGauge }
type vitalMetrics struct {
	percentileMetrics
	good, needsImprovement, poor metrix.SnapshotGauge
}
type collectorMetrics struct {
	meter                                                                         metrix.SnapshotMeter
	ingress                                                                       metrix.StateSetInstrument
	diagnostics, traffic                                                          []counterMetric
	observedSessions, windowDocumentViews, windowApplicationViews, windowJSErrors metrix.SnapshotGauge
	windowLost, sessionsLost                                                      metrix.SnapshotGauge
	vitals                                                                        map[string]vitalMetrics
	load, dcl, fetch                                                              percentileMetrics
	sameSiteResources, crossSiteResources, unknownResources                       metrix.SnapshotCounter
}

func newCollectorMetrics(m metrix.SnapshotMeter, logs, traces, frustrationSignals bool) collectorMetrics {
	metrics := collectorMetrics{
		meter:                  m,
		ingress:                m.StateSet("ingress_state", metrix.WithStateSetStates("available", "unavailable"), metrix.WithStateSetMode(metrix.ModeEnum)),
		observedSessions:       m.Gauge("observed_sessions"),
		windowDocumentViews:    m.Gauge("window_document_views"),
		windowApplicationViews: m.Gauge("window_application_views"),
		windowJSErrors:         m.Gauge("window_js_errors"),
		windowLost:             m.Gauge("window_lost_reports"),
		sessionsLost:           m.Gauge("sessions_lost_reports"),
		vitals:                 make(map[string]vitalMetrics, len(beacon.Vitals)),
		load:                   newPercentileMetrics(m, "load"),
		dcl:                    newPercentileMetrics(m, "dom_content_loaded_handler"),
		fetch:                  newPercentileMetrics(m, "same_site_fetch"),
		sameSiteResources:      m.Counter("resources_same_site"),
		crossSiteResources:     m.Counter("resources_cross_site"),
		unknownResources:       m.Counter("resources_unknown"),
	}
	diagnostics := []string{aggregate.CounterHistoryWritten, aggregate.CounterHistoryDropped}
	if logs {
		diagnostics = append(diagnostics, aggregate.CounterOTLPSent, aggregate.CounterOTLPDropped, aggregate.CounterOTLPErrors)
	}
	if traces {
		diagnostics = append(diagnostics, aggregate.CounterSpansSent, aggregate.CounterSpansDropped, aggregate.CounterSpansErrors)
	}
	for _, name := range diagnostics {
		metrics.diagnostics = append(metrics.diagnostics, counterMetric{
			name:       name,
			instrument: m.Counter(name),
		})
	}
	traffic := []string{aggregate.CounterPageviews, aggregate.CounterApplicationViews, aggregate.CounterJSErrors, aggregate.CounterAccepted, beacon.RejectOrigin, beacon.RejectRate, beacon.RejectSize, beacon.RejectInvalid, beacon.RejectBot, aggregate.CounterSamplesDropped, aggregate.CounterInvalidMeasurements}
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
		p50:          m.Gauge(name+"_p50", metrix.WithFloat(true)),
		p75:          m.Gauge(name+"_p75", metrix.WithFloat(true)),
		p95:          m.Gauge(name+"_p95", metrix.WithFloat(true)),
		observations: m.Gauge(name + "_observations"),
		lost:         m.Gauge(name + "_lost_reports"),
	}
}
func (m percentileMetrics) observe(st aggregate.VitalStats) {
	m.observations.Observe(float64(st.N))
	m.lost.Observe(float64(st.Lost))
	if st.N == 0 || st.Lost != 0 {
		return
	}
	m.p50.Observe(st.P50)
	m.p75.Observe(st.P75)
	m.p95.Observe(st.P95)
}
