// SPDX-License-Identifier: GPL-3.0-or-later

package collector

import (
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/java/ingest"
)

type instruments struct {
	meter                               metrix.SnapshotMeter
	memory, connections, pending, limit metrix.SnapshotGauge
	http                                metrix.SnapshotHistogram
}

func newInstruments(store metrix.CollectorStore) instruments {
	m := store.Write().SnapshotMeter("")
	return instruments{
		meter:       m,
		memory:      m.Gauge("jvm_memory_used_bytes", metrix.WithUnit("bytes")),
		connections: m.Gauge("hikari_connections", metrix.WithUnit("connections")),
		pending:     m.Gauge("hikari_pending_requests", metrix.WithUnit("requests")),
		limit:       m.Gauge("hikari_limit", metrix.WithUnit("connections")),
		http:        m.Histogram("http_server_request_duration_seconds", metrix.WithUnit("seconds")),
	}
}

func (m instruments) write(app ingest.Application, display string) {
	// Instances, pools and source epochs churn. Dynamic labels avoid Vec's
	// permanent per-value handle cache; committed series use store retention.
	for _, sample := range app.Samples {
		labels := make([]metrix.Label, 0, 3+len(sample.Labels))
		labels = append(labels, metrix.Label{Key: "application", Value: app.Application}, metrix.Label{Key: "instance", Value: app.Instance}, metrix.Label{Key: "application_name", Value: display})
		for key, value := range sample.Labels {
			labels = append(labels, metrix.Label{Key: key, Value: value})
		}
		set := m.meter.LabelSet(labels...)
		switch sample.Name {
		case "jvm_memory_used_bytes":
			m.memory.Observe(sample.Value, set)
		case "hikari_connections":
			m.connections.Observe(sample.Value, set)
		case "hikari_pending_requests":
			m.pending.Observe(sample.Value, set)
		case "hikari_limit":
			m.limit.Observe(sample.Value, set)
		case "http_server_request_duration_seconds":
			m.http.ObservePoint(*sample.Histogram, set)
		}
	}
}
