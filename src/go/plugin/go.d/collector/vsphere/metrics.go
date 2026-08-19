// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import (
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	rs "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/resources"
)

type collectorMetrics struct {
	meter  metrix.SnapshotMeter
	gauges map[string]metrix.SnapshotGauge
}

const scaledPercent = 10000

func newCollectorMetrics(store metrix.CollectorStore) *collectorMetrics {
	return &collectorMetrics{
		meter:  store.Write().SnapshotMeter(""),
		gauges: make(map[string]metrix.SnapshotGauge),
	}
}

func (mx *collectorMetrics) gauge(name string) metrix.SnapshotGauge {
	// The gauge cache is mutated only from Collect while collectionLock is held.
	gauge := mx.gauges[name]
	if gauge == nil {
		gauge = mx.meter.Gauge(name)
		mx.gauges[name] = gauge
	}
	return gauge
}

func (c *Collector) observeGauge(name string, value int64, labels metrix.LabelSet) {
	c.observeGaugeFloat(name, float64(value), labels)
}

func (c *Collector) observeGaugeFloat(name string, value float64, labels metrix.LabelSet) {
	c.mx.gauge(name).Observe(metrix.SampleValue(value), labels)
}

func (c *Collector) labelSet(labels []metrix.Label) metrix.LabelSet {
	return c.mx.meter.LabelSet(labels...)
}

func (c *Collector) v2MetricLabels(id string, base []metrix.Label, enrichment map[string]string) []metrix.Label {
	labels := make([]metrix.Label, 0, len(base)+1)
	labels = append(labels, metrix.Label{Key: "id", Value: id})
	labels = append(labels, base...)
	labels = append(labels, resourceEnrichmentLabels(enrichment)...)
	return labels
}

func (c *Collector) inventoryLabelSet() metrix.LabelSet {
	return c.labelSet(c.v2MetricLabels("inventory", nil, nil))
}

func (c *Collector) hostLabelSet(host *rs.Host) metrix.LabelSet {
	return c.labelSet(c.v2MetricLabels(host.ID, c.hostLabels(host), host.Labels))
}

func (c *Collector) vmLabelSet(vm *rs.VM) metrix.LabelSet {
	return c.labelSet(c.v2MetricLabels(vm.ID, c.vmLabels(vm), vm.Labels))
}

func (c *Collector) datastoreLabelSet(ds *rs.Datastore) metrix.LabelSet {
	return c.labelSet(c.v2MetricLabels(ds.ID, c.datastoreLabels(ds), ds.Labels))
}

func (c *Collector) clusterLabelSet(cl *rs.Cluster) metrix.LabelSet {
	return c.labelSet(c.v2MetricLabels(cl.ID, c.clusterLabels(cl), cl.Labels))
}

func (c *Collector) resourcePoolLabelSet(rp *rs.ResourcePool) metrix.LabelSet {
	return c.labelSet(c.v2MetricLabels(rp.ID, c.resourcePoolLabels(rp), rp.Labels))
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
