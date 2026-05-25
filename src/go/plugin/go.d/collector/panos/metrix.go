// SPDX-License-Identifier: GPL-3.0-or-later

package panos

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

func (c *Collector) writeMetricStore(mx map[string]int64) {
	if len(mx) == 0 || c.store == nil || c.charts == nil {
		return
	}

	meter := c.store.Write().SnapshotMeter("")
	for _, chart := range *c.charts {
		if chart == nil || !chartHasMetrics(chart, mx) {
			continue
		}

		chartMeter := meter
		if labels := metrixLabels(chart.Labels); len(labels) > 0 {
			chartMeter = meter.WithLabels(labels...)
		}

		for _, dim := range chart.Dims {
			value, ok := mx[dim.ID]
			if !ok {
				continue
			}
			name := metricNameForChartDim(chart.Ctx, dim.Name, dim.ID)
			if name == "" {
				continue
			}
			chartMeter.Gauge(name).Observe(float64(value))
		}
	}
}

func chartHasMetrics(chart *collectorapi.Chart, mx map[string]int64) bool {
	for _, dim := range chart.Dims {
		if _, ok := mx[dim.ID]; ok {
			return true
		}
	}
	return false
}

func metrixLabels(labels []collectorapi.Label) []metrix.Label {
	out := make([]metrix.Label, 0, len(labels))
	for _, label := range labels {
		key := strings.TrimSpace(label.Key)
		value := strings.TrimSpace(label.Value)
		if key == "" || value == "" {
			continue
		}
		out = append(out, metrix.Label{Key: key, Value: value})
	}
	return out
}

func metricNameForChartDim(ctx, dimName, dimID string) string {
	base := strings.TrimPrefix(strings.TrimSpace(ctx), "panos.")
	base = cleanID(strings.ReplaceAll(base, ".", "_"))
	if base == "" {
		return ""
	}

	dim := cleanID(firstNonEmpty(dimName, dimID))
	if dim == "" {
		return ""
	}

	return base + "_" + dim
}
