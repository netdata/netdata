// SPDX-License-Identifier: GPL-3.0-or-later

package statsd

import (
	_ "embed"
	"sync/atomic"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
)

//go:embed charts.yaml
var chartsYAML []byte

// diagnosticsEntryID cannot collide with a profile entry: profile names start with a letter.
const diagnosticsEntryID = "_receiver"

func diagnosticsEntry() (chartengine.TemplateEntry, error) {
	spec, err := charttpl.DecodeYAML(chartsYAML)
	if err != nil {
		return chartengine.TemplateEntry{}, err
	}
	return chartengine.TemplateEntry{
		ID:               diagnosticsEntryID,
		ContextNamespace: spec.ContextNamespace,
		Groups:           spec.Groups,
	}, nil
}

// diagnostics is fixed-cardinality receiver self-monitoring. It never carries
// input text, names, tags or sender addresses. Socket readers update the atomic
// counters; everything else is Collect-owned.
type diagnostics struct {
	udpBytes, tcpBytes atomic.Uint64
	tcpConnections     atomic.Int64
	withheld           [len(withheldReasons)]uint64

	udpBytesTotal, tcpBytesTotal     metrix.SnapshotCounter // nil for an unconfigured transport
	updates                          metrix.SnapshotCounter
	rejections                       [len(rejectReasons)]metrix.SnapshotCounter
	series, seriesLimit              metrix.SnapshotGauge
	connections, connectionLimit     metrix.SnapshotGauge // nil without TCP listeners
	withheldTotal                    [len(withheldReasons)]metrix.SnapshotCounter
	seriesCapacity, connectionsLimit float64
}

func newDiagnostics(store metrix.CollectorStore, cfg Config) *diagnostics {
	m := store.Write().SnapshotMeter("receiver")
	d := &diagnostics{
		updates:          m.Counter("updates"),
		series:           m.Gauge("series"),
		seriesLimit:      m.Gauge("series_limit"),
		seriesCapacity:   float64(cfg.MaxSeries),
		connectionsLimit: float64(cfg.MaxTCPConnections),
	}
	bytes := m.Vec("transport").Counter("bytes")
	for _, l := range cfg.Listeners {
		switch {
		case l.Protocol == protocolUDP && d.udpBytesTotal == nil:
			d.udpBytesTotal = bytes.WithLabelValues(protocolUDP)
		case l.Protocol == protocolTCP && d.tcpBytesTotal == nil:
			d.tcpBytesTotal = bytes.WithLabelValues(protocolTCP)
			d.connections = m.Gauge("tcp_connections")
			d.connectionLimit = m.Gauge("tcp_connections_limit")
		}
	}
	rejections := m.Vec("reason").Counter("rejections")
	for i, reason := range rejectReasons {
		d.rejections[i] = rejections.WithLabelValues(string(reason))
	}
	withheld := m.Vec("reason").Counter("percentiles_withheld")
	for i, reason := range withheldReasons {
		d.withheldTotal[i] = withheld.WithLabelValues(reason)
	}
	return d
}

// percentilesWithheld counts one observation window whose percentiles are gapped.
func (d *diagnostics) percentilesWithheld(reason string) {
	for i, known := range withheldReasons {
		if known == reason {
			d.withheld[i]++
			return
		}
	}
}

// write publishes cumulative counters and current gauges; the Agent owns rates.
func (d *diagnostics) write(cut cutResult) {
	if d.udpBytesTotal != nil {
		d.udpBytesTotal.ObserveTotal(float64(d.udpBytes.Load()))
	}
	if d.tcpBytesTotal != nil {
		d.tcpBytesTotal.ObserveTotal(float64(d.tcpBytes.Load()))
		d.connections.Observe(float64(d.tcpConnections.Load()))
		d.connectionLimit.Observe(d.connectionsLimit)
	}
	d.updates.ObserveTotal(float64(cut.accepted))
	for i, total := range cut.rejects {
		d.rejections[i].ObserveTotal(float64(total))
	}
	d.series.Observe(float64(cut.series))
	d.seriesLimit.Observe(d.seriesCapacity)
	for i, total := range d.withheld {
		d.withheldTotal[i].ObserveTotal(float64(total))
	}
}
