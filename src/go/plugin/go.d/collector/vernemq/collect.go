// SPDX-License-Identifier: GPL-3.0-or-later

package vernemq

import (
	"errors"
	"strings"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"

	"github.com/netdata/netdata/go/plugins/pkg/prometheus"
)

func (c *Collector) collect() (map[string]int64, error) {
	mfs, err := c.prom.Scrape()
	if err != nil {
		return nil, err
	}

	if !c.namespace.found {
		name, err := c.getMetricNamespace(mfs)
		if err != nil {
			return nil, err
		}
		c.namespace.found = true
		c.namespace.name = name
	}

	mx := make(map[string]int64)

	c.collectMetrics(mx, mfs)

	return mx, nil
}

func (c *Collector) collectMetrics(mx map[string]int64, mfs prometheus.MetricFamilies) {
	nodes := c.getNodesStats(mfs)

	for node, st := range nodes {
		if !c.seenNodes[node] {
			c.seenNodes[node] = true
			c.addNodeCharts(node, st)
		}

		st.stats["open_sockets"] = st.stats[metricSocketOpen] - st.stats[metricSocketClose]
		st.stats["netsplit_unresolved"] = st.stats[metricNetSplitDetected] - st.stats[metricNetSplitResolved]
		// https://github.com/vernemq/vernemq/blob/a55ada8dfb6051362fcc468d888194bdcd6eb346/apps/vmq_server/priv/static/js/status.js#L167
		queued := st.stats[metricQueueMessageIn] - (st.stats[metricQueueMessageOut] + st.stats[metricQueueMessageDrop] + st.stats[metricQueueMessageUnhandled])
		st.stats["queued_messages"] = max(0, queued)

		px := join("node", node)

		for k, val := range st.stats {
			mx[join(px, k)] = val
		}
		for k, val := range st.mqtt4 {
			mx[join(px, "mqtt4", k)] = val
		}
		for k, val := range st.mqtt5 {
			mx[join(px, "mqtt5", k)] = val
		}
	}

	for node := range c.seenNodes {
		if _, ok := nodes[node]; !ok {
			delete(c.seenNodes, node)
			c.removeNodeCharts(node)
		}
	}

}

func (c *Collector) getNodesStats(mfs prometheus.MetricFamilies) map[string]*nodeStats {
	nodes := make(map[string]*nodeStats)

	for _, mf := range mfs {
		name, _ := strings.CutPrefix(mf.Name(), c.namespace.name+"_")
		if isSchedulerUtilizationMetric(name) {
			continue
		}

		for _, m := range mf.Metrics() {
			var value float64

			switch mf.Type() {
			case model.MetricTypeGauge:
				value = m.Gauge().Value()
			case model.MetricTypeCounter:
				value = m.Counter().Value()
			default:
				continue
			}

			node := m.Labels().Get("node")
			if node == "" {
				continue
			}

			if _, ok := nodes[node]; !ok {
				nodes[node] = newNodeStats()
			}

			nst := nodes[node]

			if len(m.Labels()) == 1 {
				nst.stats[name] += int64(value)
				continue
			}

			if !strings.HasPrefix(name, "mqtt_") && name != metricClientKeepaliveExpired {
				continue
			}

			switch m.Labels().Get("mqtt_version") {
			case "4":
				nst.mqtt4[name] += int64(value)
				m.Labels().Range(func(l labels.Label) {
					if l.Name == "return_code" {
						nst.mqtt4[join(name, l.Name, l.Value)] += int64(value)
					}
				})
			case "5":
				nst.mqtt5[name] += int64(value)
				m.Labels().Range(func(l labels.Label) {
					if l.Name == "reason_code" {
						nst.mqtt5[join(name, l.Name, l.Value)] += int64(value)
					}
				})
			}
		}
	}

	return nodes
}

func (c *Collector) getMetricNamespace(mfs prometheus.MetricFamilies) (string, error) {
	want := metricPUBLISHError
	for _, mf := range mfs {
		if strings.HasSuffix(mf.Name(), want) {
			s := strings.TrimSuffix(mf.Name(), want)
			s = strings.TrimSuffix(s, "_")
			return s, nil
		}
	}

	return "", errors.New("unexpected response: not VerneMQ metrics")
}

func isSchedulerUtilizationMetric(name string) bool {
	return strings.HasPrefix(name, "system_utilization_scheduler_")
}

func join(a, b string, rest ...string) string {
	s := a + "_" + b
	for _, v := range rest {
		s += "_" + v
	}
	return s
}
