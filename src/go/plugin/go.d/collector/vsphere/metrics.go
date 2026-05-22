// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	rs "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/resources"
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
	for _, set := range chartTemplateSets() {
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
		c.writeInventoryMetrics(meter, mx)
		c.writeResourceMetrics(meter, mx)
	}
	c.writeOptionalMetrics(meter)
}

func (mx *collectorMetrics) gauge(name string) metrix.SnapshotGauge {
	return mx.gauges[name]
}

func (c *Collector) writeInventoryMetrics(meter metrix.SnapshotMeter, mx map[string]int64) {
	c.writeChartMetrics(meter, mx, inventoryChartsTmpl, "inventory", nil)
}

func (c *Collector) writeResourceMetrics(meter metrix.SnapshotMeter, mx map[string]int64) {
	if c.resources == nil {
		return
	}

	for _, host := range sortedHosts(c.resources.Hosts) {
		c.writeChartMetrics(meter, mx, hostChartsTmpl, host.ID, c.hostLabels(host))
	}
	for _, vm := range sortedVMs(c.resources.VMs) {
		c.writeChartMetrics(meter, mx, vmChartsTmpl, vm.ID, c.vmLabels(vm))
	}
	for _, ds := range sortedDatastores(c.resources.Datastores) {
		c.writeChartMetrics(meter, mx, datastorePropertyChartsTmpl, ds.ID, c.datastoreLabels(ds))
		c.writeChartMetrics(meter, mx, datastorePerfChartsTmpl, ds.ID, c.datastoreLabels(ds))
	}
	for _, cl := range sortedClusters(c.resources.Clusters) {
		c.writeChartMetrics(meter, mx, clusterPropertyChartsTmpl, cl.ID, c.clusterLabels(cl))
		c.writeChartMetrics(meter, mx, clusterPerfChartsTmpl, cl.ID, c.clusterLabels(cl))
	}
	for _, rp := range sortedResourcePools(c.resources.ResourcePools) {
		c.writeChartMetrics(meter, mx, resourcePoolChartsTmpl, rp.ID, c.resourcePoolLabels(rp))
	}
}

func (c *Collector) writeChartMetrics(meter metrix.SnapshotMeter, mx map[string]int64, charts collectorapi.Charts, id string, labels []metrix.Label) {
	labelSet := meter.LabelSet(c.v2MetricLabels(id, labels)...)
	for _, chart := range charts {
		for _, dim := range chart.Dims {
			value, ok := mx[legacyDimID(dim.ID, id)]
			if !ok {
				continue
			}
			name := v2MetricName(chart.Ctx, dim.Name)
			if gauge := c.mx.gauge(name); gauge != nil {
				gauge.Observe(metrix.SampleValue(value), labelSet)
			}
		}
	}
}

func legacyDimID(tmpl, id string) string {
	if !strings.Contains(tmpl, "%s") {
		return tmpl
	}
	return fmt.Sprintf(tmpl, id)
}

func (c *Collector) v2MetricLabels(id string, base []metrix.Label) []metrix.Label {
	labels := make([]metrix.Label, 0, len(base)+1)
	labels = append(labels, metrix.Label{Key: "id", Value: id})
	labels = append(labels, base...)
	labels = append(labels, c.resourceEnrichmentLabels(id)...)
	return labels
}

func (c *Collector) hostLabels(host *rs.Host) []metrix.Label {
	return []metrix.Label{
		{Key: "datacenter", Value: host.Hier.DC.Name},
		{Key: "cluster", Value: getHostClusterName(host)},
		{Key: "host", Value: host.Name},
	}
}

func (c *Collector) vmLabels(vm *rs.VM) []metrix.Label {
	return []metrix.Label{
		{Key: "datacenter", Value: vm.Hier.DC.Name},
		{Key: "cluster", Value: getVMClusterName(vm)},
		{Key: "host", Value: vm.Hier.Host.Name},
		{Key: "vm", Value: vm.Name},
	}
}

func (c *Collector) datastoreLabels(ds *rs.Datastore) []metrix.Label {
	return []metrix.Label{
		{Key: "datacenter", Value: ds.Hier.DC.Name},
		{Key: "datastore", Value: ds.Name},
		{Key: "type", Value: ds.Type},
	}
}

func (c *Collector) clusterLabels(cl *rs.Cluster) []metrix.Label {
	return []metrix.Label{
		{Key: "datacenter", Value: cl.Hier.DC.Name},
		{Key: "cluster", Value: cl.Name},
	}
}

func (c *Collector) resourcePoolLabels(rp *rs.ResourcePool) []metrix.Label {
	return []metrix.Label{
		{Key: "datacenter", Value: rp.Hier.DC.Name},
		{Key: "cluster", Value: rp.Hier.Cluster.Name},
		{Key: "resource_pool", Value: rp.Name},
	}
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
