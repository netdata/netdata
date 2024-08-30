// SPDX-License-Identifier: GPL-3.0-or-later

package envoy

import (
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/prometheus"

	"github.com/prometheus/prometheus/model/labels"
)

// Server stats: https://www.envoyproxy.io/docs/envoy/latest/configuration/observability/statistics#
// Server state: https://www.envoyproxy.io/docs/envoy/latest/api-v3/admin/v3/server_info.proto#enum-admin-v3-serverinfo-state
// Listener stats: https://www.envoyproxy.io/docs/envoy/latest/configuration/listeners/stats

func (e *Envoy) collect() (map[string]int64, error) {
	mfs, err := e.prom.Scrape()
	if err != nil {
		return nil, err
	}

	mx := make(map[string]int64)

	e.collectServerStats(mx, mfs)
	e.collectClusterManagerStats(mx, mfs)
	e.collectClusterUpstreamStats(mx, mfs)
	e.collectListenerManagerStats(mx, mfs)
	e.collectListenerAdminDownstreamStats(mx, mfs)
	e.collectListenerDownstreamStats(mx, mfs)

	return mx, nil
}

func (e *Envoy) collectServerStats(mx map[string]int64, mfs prometheus.MetricFamilies) {
	seen := make(map[string]bool)
	for _, n := range []string{
		"envoy_server_uptime",
		"envoy_server_memory_allocated",
		"envoy_server_memory_heap_size",
		"envoy_server_memory_physical_size",
		"envoy_server_parent_connections",
		"envoy_server_total_connections",
	} {
		e.collectGauge(mfs, n, func(name string, m prometheus.Metric) {
			id := e.joinLabels(m.Labels())
			seen[id] = true

			if !e.servers[id] {
				e.servers[id] = true
				e.addServerCharts(id, m.Labels())
			}

			mx[join(name, id)] += int64(m.Gauge().Value())
		})
	}

	e.collectGauge(mfs, "envoy_server_state", func(name string, m prometheus.Metric) {
		id := e.joinLabels(m.Labels())
		for _, v := range []string{"live", "draining", "pre_initializing", "initializing"} {
			mx[join(name, v, id)] = 0
		}

		switch m.Gauge().Value() {
		case 0:
			mx[join(name, "live", id)] = 1
		case 1:
			mx[join(name, "draining", id)] = 1
		case 2:
			mx[join(name, "pre_initializing", id)] = 1
		case 3:
			mx[join(name, "initializing", id)] = 1
		}
	})

	for id := range e.servers {
		if id != "" && !seen[id] {
			delete(e.servers, id)
			e.removeCharts(id)
		}
	}
}

func (e *Envoy) collectClusterManagerStats(mx map[string]int64, mfs prometheus.MetricFamilies) {
	seen := make(map[string]bool)
	for _, n := range []string{
		"envoy_cluster_manager_cluster_added",
		"envoy_cluster_manager_cluster_modified",
		"envoy_cluster_manager_cluster_removed",
		"envoy_cluster_manager_cluster_updated",
		"envoy_cluster_manager_cluster_updated_via_merge",
		"envoy_cluster_manager_update_merge_cancelled",
		"envoy_cluster_manager_update_out_of_merge_window",
	} {
		e.collectCounter(mfs, n, func(name string, m prometheus.Metric) {
			id := e.joinLabels(m.Labels())
			seen[id] = true

			if !e.clusterMgrs[id] {
				e.clusterMgrs[id] = true
				e.addClusterManagerCharts(id, m.Labels())
			}

			mx[join(name, id)] += int64(m.Counter().Value())
		})
	}

	for _, n := range []string{
		"envoy_cluster_manager_active_clusters",
		"envoy_cluster_manager_warming_clusters",
	} {
		e.collectGauge(mfs, n, func(name string, m prometheus.Metric) {
			id := e.joinLabels(m.Labels())
			mx[join(name, id)] += int64(m.Gauge().Value())
		})
	}

	for id := range e.clusterMgrs {
		if id != "" && !seen[id] {
			delete(e.clusterMgrs, id)
			e.removeCharts(id)
		}
	}
}

func (e *Envoy) collectListenerAdminDownstreamStats(mx map[string]int64, mfs prometheus.MetricFamilies) {
	seen := make(map[string]bool)
	for _, n := range []string{
		"envoy_listener_admin_downstream_cx_total",
		"envoy_listener_admin_downstream_cx_destroy",
		"envoy_listener_admin_downstream_cx_transport_socket_connect_timeout",
		"envoy_listener_admin_downstream_cx_overflow",
		"envoy_listener_admin_downstream_cx_overload_reject",
		"envoy_listener_admin_downstream_global_cx_overflow",
		"envoy_listener_admin_downstream_pre_cx_timeout",
		"envoy_listener_admin_downstream_listener_filter_remote_close",
		"envoy_listener_admin_downstream_listener_filter_error",
	} {
		e.collectCounter(mfs, n, func(name string, m prometheus.Metric) {
			id := e.joinLabels(m.Labels())
			seen[id] = true

			if !e.listenerAdminDownstream[id] {
				e.listenerAdminDownstream[id] = true
				e.addListenerAdminDownstreamCharts(id, m.Labels())
			}

			mx[join(name, id)] += int64(m.Counter().Value())
		})
	}
	for _, n := range []string{
		"envoy_listener_admin_downstream_cx_active",
		"envoy_listener_admin_downstream_pre_cx_active",
	} {
		e.collectGauge(mfs, n, func(name string, m prometheus.Metric) {
			id := e.joinLabels(m.Labels())
			seen[id] = true

			if !e.listenerAdminDownstream[id] {
				e.listenerAdminDownstream[id] = true
				e.addListenerAdminDownstreamCharts(id, m.Labels())
			}

			mx[join(name, id)] += int64(m.Gauge().Value())
		})
	}

	for id := range e.listenerAdminDownstream {
		if id != "" && !seen[id] {
			delete(e.listenerAdminDownstream, id)
			e.removeCharts(id)
		}
	}
}

func (e *Envoy) collectListenerDownstreamStats(mx map[string]int64, mfs prometheus.MetricFamilies) {
	seen := make(map[string]bool)
	for _, n := range []string{
		"envoy_listener_downstream_cx_total",
		"envoy_listener_downstream_cx_destroy",
		"envoy_listener_downstream_cx_transport_socket_connect_timeout",
		"envoy_listener_downstream_cx_overflow",
		"envoy_listener_downstream_cx_overload_reject",
		"envoy_listener_downstream_global_cx_overflow",
		"envoy_listener_downstream_pre_cx_timeout",
		"envoy_listener_downstream_listener_filter_remote_close",
		"envoy_listener_downstream_listener_filter_error",
	} {
		e.collectCounter(mfs, n, func(name string, m prometheus.Metric) {
			id := e.joinLabels(m.Labels())
			seen[id] = true

			if !e.listenerDownstream[id] {
				e.listenerDownstream[id] = true
				e.addListenerDownstreamCharts(id, m.Labels())
			}

			mx[join(name, id)] += int64(m.Counter().Value())
		})
	}
	for _, n := range []string{
		"envoy_listener_downstream_cx_active",
		"envoy_listener_downstream_pre_cx_active",
	} {
		e.collectGauge(mfs, n, func(name string, m prometheus.Metric) {
			id := e.joinLabels(m.Labels())
			seen[id] = true

			if !e.listenerDownstream[id] {
				e.listenerDownstream[id] = true
				e.addListenerDownstreamCharts(id, m.Labels())
			}

			mx[join(name, id)] += int64(m.Gauge().Value())
		})
	}

	for id := range e.listenerDownstream {
		if id != "" && !seen[id] {
			delete(e.listenerDownstream, id)
			e.removeCharts(id)
		}
	}
}

func (e *Envoy) collectClusterUpstreamStats(mx map[string]int64, mfs prometheus.MetricFamilies) {
	seen := make(map[string]bool)
	for _, n := range []string{
		"envoy_cluster_upstream_cx_total",
		"envoy_cluster_upstream_cx_http1_total",
		"envoy_cluster_upstream_cx_http2_total",
		"envoy_cluster_upstream_cx_http3_total",
		"envoy_cluster_upstream_cx_http3_total",
		"envoy_cluster_upstream_cx_connect_fail",
		"envoy_cluster_upstream_cx_connect_timeout",
		"envoy_cluster_upstream_cx_idle_timeout",
		"envoy_cluster_upstream_cx_max_duration_reached",
		"envoy_cluster_upstream_cx_connect_attempts_exceeded",
		"envoy_cluster_upstream_cx_overflow",
		"envoy_cluster_upstream_cx_destroy",
		"envoy_cluster_upstream_cx_destroy_local",
		"envoy_cluster_upstream_cx_destroy_remote",
		"envoy_cluster_upstream_cx_rx_bytes_total",
		"envoy_cluster_upstream_cx_tx_bytes_total",
		"envoy_cluster_upstream_rq_total",
		"envoy_cluster_upstream_rq_pending_total",
		"envoy_cluster_upstream_rq_pending_overflow",
		"envoy_cluster_upstream_rq_pending_failure_eject",
		"envoy_cluster_upstream_rq_cancelled",
		"envoy_cluster_upstream_rq_maintenance_mode",
		"envoy_cluster_upstream_rq_timeout",
		"envoy_cluster_upstream_rq_max_duration_reached",
		"envoy_cluster_upstream_rq_per_try_timeout",
		"envoy_cluster_upstream_rq_rx_reset",
		"envoy_cluster_upstream_rq_tx_reset",
		"envoy_cluster_upstream_rq_retry",
		"envoy_cluster_upstream_rq_retry_backoff_exponential",
		"envoy_cluster_upstream_rq_retry_backoff_ratelimited",
		"envoy_cluster_upstream_rq_retry_success",
		"envoy_cluster_membership_change",
		"envoy_cluster_update_success",
		"envoy_cluster_update_failure",
		"envoy_cluster_update_empty",
		"envoy_cluster_update_no_rebuild",
	} {
		e.collectCounter(mfs, n, func(name string, m prometheus.Metric) {
			id := e.joinLabels(m.Labels())
			seen[id] = true

			if !e.clusterUpstream[id] {
				e.clusterUpstream[id] = true
				e.addClusterUpstreamCharts(id, m.Labels())
			}

			mx[join(name, id)] += int64(m.Counter().Value())
		})
	}

	for _, n := range []string{
		"envoy_cluster_upstream_cx_active",
		"envoy_cluster_upstream_cx_rx_bytes_buffered",
		"envoy_cluster_upstream_cx_tx_bytes_buffered",
		"envoy_cluster_upstream_rq_active",
		"envoy_cluster_upstream_rq_pending_active",
		"envoy_cluster_membership_healthy",
		"envoy_cluster_membership_degraded",
		"envoy_cluster_membership_excluded",
	} {
		e.collectGauge(mfs, n, func(name string, m prometheus.Metric) {
			id := e.joinLabels(m.Labels())
			seen[id] = true

			if !e.clusterUpstream[id] {
				e.clusterUpstream[id] = true
				e.addClusterUpstreamCharts(id, m.Labels())
			}

			mx[join(name, id)] += int64(m.Gauge().Value())
		})
	}

	for id := range e.clusterUpstream {
		if id != "" && !seen[id] {
			delete(e.clusterUpstream, id)
			e.removeCharts(id)
		}
	}
}

func (e *Envoy) collectListenerManagerStats(mx map[string]int64, mfs prometheus.MetricFamilies) {
	seen := make(map[string]bool)
	for _, n := range []string{
		"envoy_listener_manager_listener_added",
		"envoy_listener_manager_listener_modified",
		"envoy_listener_manager_listener_removed",
		"envoy_listener_manager_listener_stopped",
		"envoy_listener_manager_listener_create_success",
		"envoy_listener_manager_listener_create_failure",
		"envoy_listener_manager_listener_in_place_updated",
	} {
		e.collectCounter(mfs, n, func(name string, m prometheus.Metric) {
			id := e.joinLabels(m.Labels())
			seen[id] = true

			if !e.listenerMgrs[id] {
				e.listenerMgrs[id] = true
				e.addListenerManagerCharts(id, m.Labels())
			}

			mx[join(name, id)] += int64(m.Counter().Value())
		})
	}

	for _, n := range []string{
		"envoy_listener_manager_total_listeners_warming",
		"envoy_listener_manager_total_listeners_active",
		"envoy_listener_manager_total_listeners_draining",
	} {
		e.collectGauge(mfs, n, func(name string, m prometheus.Metric) {
			id := e.joinLabels(m.Labels())
			seen[id] = true

			if !e.listenerMgrs[id] {
				e.listenerMgrs[id] = true
				e.addListenerManagerCharts(id, m.Labels())
			}

			mx[join(name, id)] += int64(m.Gauge().Value())
		})
	}

	for id := range e.listenerMgrs {
		if id != "" && !seen[id] {
			delete(e.listenerMgrs, id)
			e.removeCharts(id)
		}
	}
}

func (e *Envoy) collectGauge(mfs prometheus.MetricFamilies, metric string, process func(name string, m prometheus.Metric)) {
	if mf := mfs.GetGauge(metric); mf != nil {
		for _, m := range mf.Metrics() {
			process(mf.Name(), m)
		}
	}
}

func (e *Envoy) collectCounter(mfs prometheus.MetricFamilies, metric string, process func(name string, m prometheus.Metric)) {
	if mf := mfs.GetCounter(metric); mf != nil {
		for _, m := range mf.Metrics() {
			process(mf.Name(), m)
		}
	}
}

func (e *Envoy) joinLabels(labels labels.Labels) string {
	var buf strings.Builder
	first := true
	for _, lbl := range labels {
		v := lbl.Value
		if v == "" {
			continue
		}
		if strings.IndexByte(v, ' ') != -1 {
			v = spaceReplacer.Replace(v)
		}
		if strings.IndexByte(v, '\\') != -1 {
			if v = decodeLabelValue(v); strings.IndexByte(v, '\\') != -1 {
				v = backslashReplacer.Replace(v)
			}
		}
		if first {
			buf.WriteString(v)
			first = false
		} else {
			buf.WriteString("_" + v)
		}
	}
	return buf.String()
}

var (
	spaceReplacer     = strings.NewReplacer(" ", "_")
	backslashReplacer = strings.NewReplacer(`\`, "_")
)

func decodeLabelValue(value string) string {
	v, err := strconv.Unquote("\"" + value + "\"")
	if err != nil {
		return value
	}
	return v
}

func join(name string, elems ...string) string {
	for _, v := range elems {
		if v != "" {
			name += "_" + v
		}
	}
	return name
}
