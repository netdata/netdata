// SPDX-License-Identifier: GPL-3.0-or-later

package envoy

import (
	"strconv"
	"strings"

	"github.com/prometheus/prometheus/model/labels"

	"github.com/netdata/netdata/go/plugins/pkg/prometheus"
)

// Server stats: https://www.envoyproxy.io/docs/envoy/latest/configuration/observability/statistics#
// Server state: https://www.envoyproxy.io/docs/envoy/latest/api-v3/admin/v3/server_info.proto#enum-admin-v3-serverinfo-state
// Listener stats: https://www.envoyproxy.io/docs/envoy/latest/configuration/listeners/stats

func (c *Collector) collect() (map[string]int64, error) {
	mfs, err := c.prom.Scrape()
	if err != nil {
		return nil, err
	}

	mx := make(map[string]int64)

	c.collectServerStats(mx, mfs)
	c.collectClusterManagerStats(mx, mfs)
	c.collectClusterUpstreamStats(mx, mfs)
	c.collectListenerManagerStats(mx, mfs)
	c.collectListenerAdminDownstreamStats(mx, mfs)
	c.collectListenerDownstreamStats(mx, mfs)

	return mx, nil
}

func (c *Collector) collectServerStats(mx map[string]int64, mfs prometheus.MetricFamilies) {
	seen := make(map[string]bool)
	for _, n := range []string{
		"envoy_server_uptime",
		"envoy_server_memory_allocated",
		"envoy_server_memory_heap_size",
		"envoy_server_memory_physical_size",
		"envoy_server_parent_connections",
		"envoy_server_total_connections",
	} {
		c.collectGauge(mfs, n, func(name string, m prometheus.Metric) {
			id := c.joinLabels(m.Labels())
			seen[id] = true

			if !c.servers[id] {
				c.servers[id] = true
				c.addServerCharts(id, m.Labels())
			}

			mx[join(name, id)] += int64(m.Gauge().Value())
		})
	}

	c.collectGauge(mfs, "envoy_server_state", func(name string, m prometheus.Metric) {
		id := c.joinLabels(m.Labels())
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

	for id := range c.servers {
		if id != "" && !seen[id] {
			delete(c.servers, id)
			c.removeCharts(id)
		}
	}
}

func (c *Collector) collectClusterManagerStats(mx map[string]int64, mfs prometheus.MetricFamilies) {
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
		c.collectCounter(mfs, n, func(name string, m prometheus.Metric) {
			id := c.joinLabels(m.Labels())
			seen[id] = true

			if !c.clusterMgrs[id] {
				c.clusterMgrs[id] = true
				c.addClusterManagerCharts(id, m.Labels())
			}

			mx[join(name, id)] += int64(m.Counter().Value())
		})
	}

	for _, n := range []string{
		"envoy_cluster_manager_active_clusters",
		"envoy_cluster_manager_warming_clusters",
	} {
		c.collectGauge(mfs, n, func(name string, m prometheus.Metric) {
			id := c.joinLabels(m.Labels())
			mx[join(name, id)] += int64(m.Gauge().Value())
		})
	}

	for id := range c.clusterMgrs {
		if id != "" && !seen[id] {
			delete(c.clusterMgrs, id)
			c.removeCharts(id)
		}
	}
}

func (c *Collector) collectListenerAdminDownstreamStats(mx map[string]int64, mfs prometheus.MetricFamilies) {
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
		c.collectCounter(mfs, n, func(name string, m prometheus.Metric) {
			id := c.joinLabels(m.Labels())
			seen[id] = true

			if !c.listenerAdminDownstream[id] {
				c.listenerAdminDownstream[id] = true
				c.addListenerAdminDownstreamCharts(id, m.Labels())
			}

			mx[join(name, id)] += int64(m.Counter().Value())
		})
	}
	for _, n := range []string{
		"envoy_listener_admin_downstream_cx_active",
		"envoy_listener_admin_downstream_pre_cx_active",
	} {
		c.collectGauge(mfs, n, func(name string, m prometheus.Metric) {
			id := c.joinLabels(m.Labels())
			seen[id] = true

			if !c.listenerAdminDownstream[id] {
				c.listenerAdminDownstream[id] = true
				c.addListenerAdminDownstreamCharts(id, m.Labels())
			}

			mx[join(name, id)] += int64(m.Gauge().Value())
		})
	}

	for id := range c.listenerAdminDownstream {
		if id != "" && !seen[id] {
			delete(c.listenerAdminDownstream, id)
			c.removeCharts(id)
		}
	}
}

func (c *Collector) collectListenerDownstreamStats(mx map[string]int64, mfs prometheus.MetricFamilies) {
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
		c.collectCounter(mfs, n, func(name string, m prometheus.Metric) {
			id := c.joinLabels(m.Labels())
			seen[id] = true

			if !c.listenerDownstream[id] {
				c.listenerDownstream[id] = true
				c.addListenerDownstreamCharts(id, m.Labels())
			}

			mx[join(name, id)] += int64(m.Counter().Value())
		})
	}
	for _, n := range []string{
		"envoy_listener_downstream_cx_active",
		"envoy_listener_downstream_pre_cx_active",
	} {
		c.collectGauge(mfs, n, func(name string, m prometheus.Metric) {
			id := c.joinLabels(m.Labels())
			seen[id] = true

			if !c.listenerDownstream[id] {
				c.listenerDownstream[id] = true
				c.addListenerDownstreamCharts(id, m.Labels())
			}

			mx[join(name, id)] += int64(m.Gauge().Value())
		})
	}

	for id := range c.listenerDownstream {
		if id != "" && !seen[id] {
			delete(c.listenerDownstream, id)
			c.removeCharts(id)
		}
	}
}

func (c *Collector) collectClusterUpstreamStats(mx map[string]int64, mfs prometheus.MetricFamilies) {
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
		c.collectCounter(mfs, n, func(name string, m prometheus.Metric) {
			id := c.joinLabels(m.Labels())
			seen[id] = true

			if !c.clusterUpstream[id] {
				c.clusterUpstream[id] = true
				c.addClusterUpstreamCharts(id, m.Labels())
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
		c.collectGauge(mfs, n, func(name string, m prometheus.Metric) {
			id := c.joinLabels(m.Labels())
			seen[id] = true

			if !c.clusterUpstream[id] {
				c.clusterUpstream[id] = true
				c.addClusterUpstreamCharts(id, m.Labels())
			}

			mx[join(name, id)] += int64(m.Gauge().Value())
		})
	}

	for id := range c.clusterUpstream {
		if id != "" && !seen[id] {
			delete(c.clusterUpstream, id)
			c.removeCharts(id)
		}
	}
}

func (c *Collector) collectListenerManagerStats(mx map[string]int64, mfs prometheus.MetricFamilies) {
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
		c.collectCounter(mfs, n, func(name string, m prometheus.Metric) {
			id := c.joinLabels(m.Labels())
			seen[id] = true

			if !c.listenerMgrs[id] {
				c.listenerMgrs[id] = true
				c.addListenerManagerCharts(id, m.Labels())
			}

			mx[join(name, id)] += int64(m.Counter().Value())
		})
	}

	for _, n := range []string{
		"envoy_listener_manager_total_listeners_warming",
		"envoy_listener_manager_total_listeners_active",
		"envoy_listener_manager_total_listeners_draining",
	} {
		c.collectGauge(mfs, n, func(name string, m prometheus.Metric) {
			id := c.joinLabels(m.Labels())
			seen[id] = true

			if !c.listenerMgrs[id] {
				c.listenerMgrs[id] = true
				c.addListenerManagerCharts(id, m.Labels())
			}

			mx[join(name, id)] += int64(m.Gauge().Value())
		})
	}

	for id := range c.listenerMgrs {
		if id != "" && !seen[id] {
			delete(c.listenerMgrs, id)
			c.removeCharts(id)
		}
	}
}

func (c *Collector) collectGauge(mfs prometheus.MetricFamilies, metric string, process func(name string, m prometheus.Metric)) {
	if mf := mfs.GetGauge(metric); mf != nil {
		for _, m := range mf.Metrics() {
			process(mf.Name(), m)
		}
	}
}

func (c *Collector) collectCounter(mfs prometheus.MetricFamilies, metric string, process func(name string, m prometheus.Metric)) {
	if mf := mfs.GetCounter(metric); mf != nil {
		for _, m := range mf.Metrics() {
			process(mf.Name(), m)
		}
	}
}

func (c *Collector) joinLabels(labels labels.Labels) string {
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
