// SPDX-License-Identifier: GPL-3.0-or-later

package vernemq

import (
	"errors"
	"strings"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/prometheus"
)

func (v *VerneMQ) collect() (map[string]int64, error) {
	mfs, err := v.prom.Scrape()
	if err != nil {
		return nil, err
	}

	if !v.namespace.found {
		name, err := v.getMetricNamespace(mfs)
		if err != nil {
			return nil, err
		}
		v.namespace.found = true
		v.namespace.name = name
	}

	mx := make(map[string]int64)

	v.collectMetrics(mx, mfs)

	return mx, nil
}

func (v *VerneMQ) collectMetrics(mx map[string]int64, mfs prometheus.MetricFamilies) {
	nodes := v.getNodesStats(mfs)

	for node, st := range nodes {
		if !v.seenNodes[node] {
			v.seenNodes[node] = true
			v.addNodeCharts(node, st)
		}

		st.stats["open_sockets"] = st.stats[metricSocketOpen] - st.stats[metricSocketClose]
		st.stats["netsplit_unresolved"] = st.stats[metricNetSplitDetected] - st.stats[metricNetSplitResolved]

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

	for node := range v.seenNodes {
		if _, ok := nodes[node]; !ok {
			delete(v.seenNodes, node)
			v.removeNodeCharts(node)
		}
	}

}

func (v *VerneMQ) getNodesStats(mfs prometheus.MetricFamilies) map[string]*nodeStats {
	nodes := make(map[string]*nodeStats)

	for _, mf := range mfs {
		name, _ := strings.CutPrefix(mf.Name(), v.namespace.name+"_")
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

func (v *VerneMQ) getMetricNamespace(mfs prometheus.MetricFamilies) (string, error) {
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
