// SPDX-License-Identifier: GPL-3.0-or-later

package consul

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/metrix"
)

func (c *Collector) collectMetricsPrometheus(mx map[string]int64) error {
	mfs, err := c.prom.Scrape()
	if err != nil {
		return err
	}

	// Key Metrics (https://developer.hashicorp.com/consul/docs/agent/telemetry#key-metrics)

	// prometheus metrics are messy:
	// - if 'disable_hostname' is false (default):
	//   - consul_autopilot_failure_tolerance => consul_hostname_autopilot_failure_tolerance
	//   - both are exposed
	//   - only the one with the hostname has the correct value
	// - 1.14.3 (it probably has something to do with cloud management version):
	//   - runtime_sys_bytes => runtime_sys_bytes_sys_bytes; consul_autopilot_healthy => consul_autopilot_healthy_healthy
	//   - both are exposed
	//   - only the one with the double name has the correct value

	if c.isServer() {
		c.collectSummary(mx, mfs, "raft_thread_main_saturation")
		c.collectSummary(mx, mfs, "raft_thread_fsm_saturation")
		c.collectSummary(mx, mfs, "raft_boltdb_logsPerBatch")
		c.collectSummary(mx, mfs, "kvs_apply")
		c.collectSummary(mx, mfs, "txn_apply")
		c.collectSummary(mx, mfs, "raft_boltdb_storeLogs")
		c.collectSummary(mx, mfs, "raft_rpc_installSnapshot") // make sense for followers only
		c.collectSummary(mx, mfs, "raft_commitTime")          // make sense for leader only
		c.collectSummary(mx, mfs, "raft_leader_lastContact")  // make sense for leader only

		c.collectCounter(mx, mfs, "raft_apply", precision) // make sense for leader only
		c.collectCounter(mx, mfs, "raft_state_candidate", 1)
		c.collectCounter(mx, mfs, "raft_state_leader", 1)

		c.collectGaugeBool(mx, mfs, "autopilot_healthy", "autopilot_healthy_healthy")
		c.collectGaugeBool(mx, mfs, "server_isLeader", "server_isLeader_isLeader")
		c.collectGauge(mx, mfs, "autopilot_failure_tolerance", 1, "autopilot_failure_tolerance_failure_tolerance")
		c.collectGauge(mx, mfs, "raft_fsm_lastRestoreDuration", 1)
		c.collectGauge(mx, mfs, "raft_leader_oldestLogAge", 1, "raft_leader_oldestLogAge_oldestLogAge")
		c.collectGauge(mx, mfs, "raft_boltdb_freelistBytes", 1, "raft_boltdb_freelistBytes_freelistBytes")

		if isLeader, ok := c.isLeader(mfs); ok {
			if isLeader && !c.hasLeaderCharts {
				c.addLeaderCharts()
				c.hasLeaderCharts = true
			}
			if !isLeader && c.hasLeaderCharts {
				c.removeLeaderCharts()
				c.hasLeaderCharts = false
			}
			if !isLeader && !c.hasFollowerCharts {
				c.addFollowerCharts()
				c.hasFollowerCharts = true
			}
			if isLeader && c.hasFollowerCharts {
				c.removeFollowerCharts()
				c.hasFollowerCharts = false
			}
		}
	}

	c.collectGauge(mx, mfs, "system_licenseExpiration", 3600, "system_licenseExpiration_licenseExpiration")

	c.collectCounter(mx, mfs, "client_rpc", 1)
	c.collectCounter(mx, mfs, "client_rpc_exceeded", 1)
	c.collectCounter(mx, mfs, "client_rpc_failed", 1)

	c.collectGauge(mx, mfs, "runtime_alloc_bytes", 1, "runtime_alloc_bytes_alloc_bytes")
	c.collectGauge(mx, mfs, "runtime_sys_bytes", 1, "runtime_sys_bytes_sys_bytes")
	c.collectGauge(mx, mfs, "runtime_total_gc_pause_ns", 1, "runtime_total_gc_pause_ns_total_gc_pause_ns")

	return nil
}

func (c *Collector) isLeader(mfs prometheus.MetricFamilies) (bool, bool) {
	var mf *prometheus.MetricFamily
	for _, v := range []string{"server_isLeader_isLeader", "server_isLeader"} {
		if mf = mfs.GetGauge(c.promMetricNameWithHostname(v)); mf != nil {
			break
		}
		if mf = mfs.GetGauge(c.promMetricName(v)); mf != nil {
			break
		}
	}

	if mf == nil {
		return false, false
	}

	return mf.Metrics()[0].Gauge().Value() == 1, true
}

func (c *Collector) collectGauge(mx map[string]int64, mfs prometheus.MetricFamilies, name string, mul float64, aliases ...string) {
	var mf *prometheus.MetricFamily
	for _, v := range append(aliases, name) {
		if mf = mfs.GetGauge(c.promMetricNameWithHostname(v)); mf != nil {
			break
		}
		if mf = mfs.GetGauge(c.promMetricName(v)); mf != nil {
			break
		}
	}

	if mf == nil {
		return
	}

	v := mf.Metrics()[0].Gauge().Value()

	if !math.IsNaN(v) {
		mx[name] = int64(v * mul)
	}
}

func (c *Collector) collectGaugeBool(mx map[string]int64, mfs prometheus.MetricFamilies, name string, aliases ...string) {
	var mf *prometheus.MetricFamily
	for _, v := range append(aliases, name) {
		if mf = mfs.GetGauge(c.promMetricNameWithHostname(v)); mf != nil {
			break
		}
		if mf = mfs.GetGauge(c.promMetricName(v)); mf != nil {
			break
		}
	}

	if mf == nil {
		return
	}

	v := mf.Metrics()[0].Gauge().Value()

	if !math.IsNaN(v) {
		mx[name+"_yes"] = metrix.Bool(v == 1)
		mx[name+"_no"] = metrix.Bool(v == 0)
	}
}

func (c *Collector) collectCounter(mx map[string]int64, mfs prometheus.MetricFamilies, name string, mul float64) {
	mf := mfs.GetCounter(c.promMetricName(name))
	if mf == nil {
		return
	}

	v := mf.Metrics()[0].Counter().Value()

	if !math.IsNaN(v) {
		mx[name] = int64(v * mul)
	}
}

func (c *Collector) collectSummary(mx map[string]int64, mfs prometheus.MetricFamilies, name string) {
	mf := mfs.GetSummary(c.promMetricName(name))
	if mf == nil {
		return
	}

	m := mf.Metrics()[0]

	for _, q := range m.Summary().Quantiles() {
		v := q.Value()
		// MaxAge is 10 seconds (hardcoded)
		// https://github.com/hashicorp/go-metrics/blob/b6d5c860c07ef6eeec89f4a662c7b452dd4d0c93/prometheus/prometheus.go#L227
		if math.IsNaN(v) {
			v = 0
		}

		id := fmt.Sprintf("%s_quantile=%s", name, formatFloat(q.Quantile()))
		mx[id] = int64(v * precision * precision)
	}

	mx[name+"_sum"] = int64(m.Summary().Sum() * precision)
	mx[name+"_count"] = int64(m.Summary().Count())
}

func (c *Collector) promMetricName(name string) string {
	px := c.cfg.DebugConfig.Telemetry.MetricsPrefix
	return px + "_" + name
}

var forbiddenCharsReplacer = strings.NewReplacer(" ", "_", ".", "_", "=", "_", "-", "_", "/", "_")

// controlled by 'disable_hostname'
// https://developer.hashicorp.com/consul/docs/agent/config/config-files#telemetry-disable_hostname
func (c *Collector) promMetricNameWithHostname(name string) string {
	px := c.cfg.DebugConfig.Telemetry.MetricsPrefix
	node := forbiddenCharsReplacer.Replace(c.cfg.Config.NodeName)

	return px + "_" + node + "_" + name
}

func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}
