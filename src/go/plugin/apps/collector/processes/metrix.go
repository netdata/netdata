// SPDX-License-Identifier: GPL-3.0-or-later

package processes

import (
	"encoding/hex"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/apps/internal/model"
)

var metricNames = [model.MetricCount]string{
	"cpu_user_percent", "cpu_system_percent", "cpu_guest_percent",
	"cpu_children_user_percent", "cpu_children_system_percent", "cpu_children_guest_percent",
	"minor_faults_per_second", "major_faults_per_second", "children_minor_faults_per_second", "children_major_faults_per_second",
	"virtual_memory_bytes", "resident_memory_bytes", "shared_memory_bytes", "swap_memory_bytes", "proportional_memory_bytes", "estimated_memory_bytes",
	"read_bytes_per_second", "write_bytes_per_second", "logical_read_bytes_per_second", "logical_write_bytes_per_second", "read_calls_per_second", "write_calls_per_second",
	"voluntary_switches_per_second", "involuntary_switches_per_second", "threads", "uptime_min_seconds", "fd_limit_percent", "pss_age_seconds",
}
var fdNames = [model.FDTypeCount]string{"file", "socket", "pipe", "inotify", "event", "timer", "signal", "epoll", "other"}
var stateNames = []string{"running", "sleeping", "uninterruptible", "zombie", "stopped", "idle", "other"}

type metricInstruments struct {
	values    [model.MetricCount]metrix.SnapshotGaugeVec
	processes metrix.SnapshotGaugeVec
	uptimeMax metrix.SnapshotGaugeVec
	files     metrix.SnapshotGaugeVec
	states    metrix.SnapshotGaugeVec
	reads     metrix.SnapshotGauge
	links     metrix.SnapshotGauge
	errors    metrix.SnapshotGauge
}

func newMetricInstruments(store metrix.CollectorStore) metricInstruments {
	meter := store.Write().SnapshotMeter("")
	vec := meter.Vec("kind", "group_id", "name")
	var m metricInstruments
	for i, name := range metricNames {
		m.values[i] = vec.Gauge(name, metrix.WithFloat(true))
	}
	m.processes = vec.Gauge("processes")
	m.uptimeMax = vec.Gauge("uptime_max_seconds", metrix.WithFloat(true))
	m.files = meter.Vec("kind", "group_id", "name", "fd_type").Gauge("unique_fds")
	m.states = meter.Vec("state").Gauge("process_state_count")
	m.reads = meter.Gauge("scan_file_reads")
	m.links = meter.Gauge("scan_readlinks")
	m.errors = meter.Gauge("scan_read_errors")
	return m
}

func (c *Collector) writeMetrics(groups []model.Group, snapshot model.Snapshot) {
	for _, g := range groups {
		// The group's durable identity is its exact name, independent of the
		// native backend's transient numeric assignment and display escaping.
		id := hex.EncodeToString([]byte(g.Name))
		labels := []string{g.Kind, id, g.Name}
		c.metrics.processes.WithLabelValues(labels...).Observe(float64(g.Processes))
		for metric, instrument := range c.metrics.values {
			if g.Has(metric) {
				instrument.WithLabelValues(labels...).Observe(g.Values[metric])
			}
		}
		if g.Has(model.Uptime) {
			c.metrics.uptimeMax.WithLabelValues(labels...).Observe(g.UptimeMax)
		}
		if g.FDValid {
			for i, value := range g.FDCounts {
				c.metrics.files.WithLabelValues(g.Kind, id, g.Name, fdNames[i]).Observe(float64(value))
			}
		}
	}
	states := make(map[string]int, len(stateNames))
	for _, p := range snapshot.Processes {
		state := "other"
		switch p.State {
		case "R":
			state = "running"
		case "S":
			state = "sleeping"
		case "D":
			state = "uninterruptible"
		case "Z":
			state = "zombie"
		case "T", "t":
			state = "stopped"
		case "I":
			state = "idle"
		}
		states[state]++
	}
	for _, state := range stateNames {
		c.metrics.states.WithLabelValues(state).Observe(float64(states[state]))
	}
	c.metrics.reads.Observe(float64(snapshot.Stats.FileReads))
	c.metrics.links.Observe(float64(snapshot.Stats.FDLinksRead))
	c.metrics.errors.Observe(float64(snapshot.Stats.ReadErrors))
}
