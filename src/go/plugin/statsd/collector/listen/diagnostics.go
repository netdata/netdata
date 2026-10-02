// SPDX-License-Identifier: GPL-3.0-or-later

package listen

import (
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/statsd/collector/listen/internal/percentile"
	"github.com/netdata/netdata/go/plugins/plugin/statsd/collector/listen/internal/server"
)

// diagnostics is fixed-cardinality receiver self-monitoring. It never carries
// input text, names, tags or sender addresses. Socket readers update transport,
// which outlives any one server; everything else is Collect-owned.
type diagnostics struct {
	transport server.Stats
	withheld  [len(percentile.WithheldReasons)]uint64

	udpBytesTotal, tcpBytesTotal metrix.SnapshotCounter // nil for an unconfigured transport
	updates                      metrix.SnapshotCounter
	rejections                   [len(recordRejections)]metrix.SnapshotCounter
	oversize, unterminated       metrix.SnapshotCounter
	series, seriesLimit          metrix.SnapshotGauge
	connections, connectionLimit metrix.SnapshotGauge   // nil without TCP listeners
	connectionsRefused           metrix.SnapshotCounter // nil without TCP listeners
	withheldTotal                [len(percentile.WithheldReasons)]metrix.SnapshotCounter
	maxSeries, maxConnections    float64
}

func newDiagnostics(store metrix.CollectorStore, cfg Config) *diagnostics {
	m := store.Write().SnapshotMeter("receiver")
	d := &diagnostics{
		updates:        m.Counter("updates"),
		series:         m.Gauge("series"),
		seriesLimit:    m.Gauge("series_limit"),
		maxSeries:      float64(cfg.MaxSeries),
		maxConnections: float64(cfg.MaxTCPConnections),
	}
	bytes := m.Vec("transport").Counter("bytes")
	if cfg.hasListener(protocolUDP) {
		d.udpBytesTotal = bytes.WithLabelValues(protocolUDP)
	}
	if cfg.hasListener(protocolTCP) {
		d.tcpBytesTotal = bytes.WithLabelValues(protocolTCP)
		d.connections = m.Gauge("tcp_connections")
		d.connectionLimit = m.Gauge("tcp_connections_limit")
		d.connectionsRefused = m.Counter("tcp_connections_refused")
	}
	rejections := m.Vec("reason").Counter("rejections")
	for i, reason := range recordRejections {
		d.rejections[i] = rejections.WithLabelValues(string(reason))
	}
	d.oversize = rejections.WithLabelValues(string(rejectOversize))
	d.unterminated = rejections.WithLabelValues(string(rejectUnterminated))
	withheld := m.Vec("reason").Counter("percentiles_withheld")
	for i, reason := range percentile.WithheldReasons {
		d.withheldTotal[i] = withheld.WithLabelValues(reason)
	}
	return d
}

// percentilesWithheld counts one observation window whose percentiles are gapped.
func (d *diagnostics) percentilesWithheld(reason string) {
	for i, known := range percentile.WithheldReasons {
		if known == reason {
			d.withheld[i]++
			return
		}
	}
}

// write publishes cumulative counters and current gauges; the Agent owns rates.
func (d *diagnostics) write(stats receiverStats) {
	t := &d.transport
	if d.udpBytesTotal != nil {
		d.udpBytesTotal.ObserveTotal(float64(t.UDPBytes.Load()))
	}
	if d.tcpBytesTotal != nil {
		d.tcpBytesTotal.ObserveTotal(float64(t.TCPBytes.Load()))
		d.connections.Observe(float64(t.TCPConnections.Load()))
		d.connectionLimit.Observe(d.maxConnections)
		d.connectionsRefused.ObserveTotal(float64(t.TCPRefused.Load()))
	}
	d.updates.ObserveTotal(float64(stats.accepted))
	for i, total := range stats.rejects {
		d.rejections[i].ObserveTotal(float64(total))
	}
	d.oversize.ObserveTotal(float64(t.Oversize.Load()))
	d.unterminated.ObserveTotal(float64(t.Unterminated.Load()))
	d.series.Observe(float64(stats.series))
	d.seriesLimit.Observe(d.maxSeries)
	for i, total := range d.withheld {
		d.withheldTotal[i].ObserveTotal(float64(total))
	}
}
