// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import (
	"strings"
	"unicode"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

const chartContextPrefix = "vsphere."

type collectorMetrics struct {
	gauges map[string]metrix.SnapshotGauge
}

func newCollectorMetrics(store metrix.CollectorStore) *collectorMetrics {
	meter := store.Write().SnapshotMeter("")
	mx := &collectorMetrics{
		gauges: make(map[string]metrix.SnapshotGauge),
	}
	for _, set := range legacyChartTemplateSets() {
		for _, chart := range set.charts {
			for _, dim := range chart.Dims {
				name := v2MetricName(chart.Ctx, dim.Name)
				if _, ok := mx.gauges[name]; ok {
					continue
				}
				mx.gauges[name] = meter.Gauge(name)
			}
		}
	}
	for _, name := range optionalMetricNames() {
		if _, ok := mx.gauges[name]; ok {
			continue
		}
		mx.gauges[name] = meter.Gauge(name)
	}
	return mx
}

func (c *Collector) writeMetrics(mx map[string]int64) {
	meter := c.store.Write().SnapshotMeter("")
	if len(mx) > 0 {
		for _, chart := range *c.Charts() {
			if chart.Obsolete {
				continue
			}

			resourceID, ok := chartResourceID(chart)
			if !ok {
				continue
			}
			labels := c.v2ChartLabels(chart, resourceID)
			scope := c.resourceHostScope(resourceID)
			writeMeter := meter
			scoped := !scope.IsDefault()
			if scoped {
				writeMeter = meter.WithHostScope(scope)
			}
			labelSet := writeMeter.LabelSet(labels...)

			for _, dim := range chart.Dims {
				value, ok := mx[dim.ID]
				if !ok {
					continue
				}
				name := v2MetricName(chart.Ctx, dim.Name)
				if gauge := c.mx.gauge(writeMeter, name, scoped); gauge != nil {
					gauge.Observe(metrix.SampleValue(value), labelSet)
				}
			}
		}
	}
	c.writeOptionalMetrics(meter)
}

func (mx *collectorMetrics) gauge(meter metrix.SnapshotMeter, name string, scoped bool) metrix.SnapshotGauge {
	if !scoped {
		if gauge := mx.gauges[name]; gauge != nil {
			return gauge
		}
	}
	if gauge := meter.Gauge(name); gauge != nil {
		return gauge
	}
	return nil
}

func (c *Collector) v2ChartLabels(chart *collectorapi.Chart, id string) []metrix.Label {
	labels := make([]metrix.Label, 0, len(chart.Labels)+1)
	labels = append(labels, metrix.Label{Key: "id", Value: id})
	for _, label := range chart.Labels {
		labels = append(labels, metrix.Label{Key: label.Key, Value: label.Value})
	}
	labels = append(labels, c.resourceEnrichmentLabels(id)...)
	return labels
}

func chartResourceID(chart *collectorapi.Chart) (string, bool) {
	if len(chart.Dims) == 0 {
		return "", false
	}
	// Legacy vSphere chart dimensions are generated as "<managed-object-id>_<counter>".
	id, _, ok := strings.Cut(chart.Dims[0].ID, "_")
	return id, ok && id != ""
}

func v2ChartTemplateID(ctx string) string {
	return sanitizeMetricName(strings.TrimPrefix(ctx, chartContextPrefix))
}

func v2MetricName(ctx, dimName string) string {
	return sanitizeMetricName(v2ChartTemplateID(ctx) + "_" + dimName)
}

func sanitizeMetricName(name string) string {
	var b strings.Builder
	b.Grow(len(name))
	lastUnderscore := false
	for _, r := range name {
		ok := r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
		if ok {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	return strings.Trim(b.String(), "_")
}
