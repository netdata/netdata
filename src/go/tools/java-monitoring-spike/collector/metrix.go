// SPDX-License-Identifier: GPL-3.0-or-later

package collector

import (
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/tools/java-monitoring-spike/ingest"
)

type instruments struct {
	memory, connections, pending, limit metrix.SnapshotGaugeVec
	http                                metrix.SnapshotHistogramVec
}

func newInstruments(store metrix.CollectorStore) instruments {
	m := store.Write().SnapshotMeter("")
	return instruments{
		memory: m.Vec("application", "instance", "application_name", "memory_type", "memory_pool").Gauge(
			"jvm_memory_used_bytes", metrix.WithUnit("bytes")),
		connections: m.Vec("application", "instance", "application_name", "pool_name", "pool_id", "state").Gauge(
			"hikari_connections", metrix.WithUnit("connections")),
		pending: m.Vec("application", "instance", "application_name", "pool_name", "pool_id").Gauge(
			"hikari_pending_requests", metrix.WithUnit("requests")),
		limit: m.Vec("application", "instance", "application_name", "pool_name", "pool_id").Gauge(
			"hikari_limit", metrix.WithUnit("connections")),
		http: m.Vec("application", "instance", "application_name", "method", "route", "status", "source_epoch").Histogram(
			"http_server_request_duration_seconds", metrix.WithUnit("seconds")),
	}
}

func (m instruments) write(app ingest.Application, display string) {
	for _, sample := range app.Samples {
		l := sample.Labels
		switch sample.Name {
		case "jvm_memory_used_bytes":
			m.memory.WithLabelValues(app.Application, app.Instance, display, l["memory_type"], l["memory_pool"]).Observe(sample.Value)
		case "hikari_connections":
			m.connections.WithLabelValues(app.Application, app.Instance, display, l["pool_name"], l["pool_id"], l["state"]).Observe(sample.Value)
		case "hikari_pending_requests":
			m.pending.WithLabelValues(app.Application, app.Instance, display, l["pool_name"], l["pool_id"]).Observe(sample.Value)
		case "hikari_limit":
			m.limit.WithLabelValues(app.Application, app.Instance, display, l["pool_name"], l["pool_id"]).Observe(sample.Value)
		case "http_server_request_duration_seconds":
			m.http.WithLabelValues(app.Application, app.Instance, display, l["method"], l["route"], l["status"], l["source_epoch"]).ObservePoint(*sample.Histogram)
		}
	}
}
