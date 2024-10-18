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

	seen := make(map[string]bool)
	mx := make(map[string]int64)

	for _, mf := range mfs {
		name, _ := strings.CutPrefix(mf.Name(), v.namespace.name+"_")
		if isSchedulerUtilizationMetric(name) {
			continue
		}

		for _, m := range mf.Metrics() {
			node := m.Labels().Get("node")
			if node == "" {
				continue
			}

			seen[node] = true

			switch mf.Type() {
			case model.MetricTypeGauge:
				v.collectGauge(mx, node, name, m.Labels(), m.Gauge())
			case model.MetricTypeCounter:
				v.collectCounter(mx, node, name, m.Labels(), m.Counter())
			}
		}
	}

	for n := range seen {
		if !v.seenNodes[n] {
			v.seenNodes[n] = true
			v.addNodeCharts(n)
		}
	}

	// 	mx["open_sockets"] = mx[metricSocketOpen] - mx[metricSocketClose]

	//l := make([]string, 0)
	//for k := range mx {
	//	l = append(l, k)
	//}
	//sort.Strings(l)
	//for _, value := range l {
	//	v.Warning(fmt.Sprintf("\"%s\": %d,", value, mx[value]))
	//}

	return mx, nil
}

func (v *VerneMQ) collectGauge(mx map[string]int64, node, metric string, lbs labels.Labels, m *prometheus.Gauge) {
	if len(lbs) == 1 {
		key := join(metric, "node", node)
		mx[key] += int64(m.Value())
		return
	}

	return
}

func (v *VerneMQ) collectCounter(mx map[string]int64, node, metric string, lbs labels.Labels, m *prometheus.Counter) {
	if len(lbs) == 1 {
		key := join(metric, "node", node)
		mx[key] += int64(m.Value())
		return
	}

	if !strings.HasPrefix(metric, "mqtt_") {
		return
	}

	if ver := lbs.Get("mqtt_version"); ver != "" {
		key := join(metric, "node", node, "mqtt_ver", ver)
		mx[key] += int64(m.Value())

		lbs.Range(func(l labels.Label) {
			switch l.Name {
			case "reason_code", "return_code":
				key = join(key, l.Name, l.Value)
				mx[key] += int64(m.Value())
			}
		})
	}
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
