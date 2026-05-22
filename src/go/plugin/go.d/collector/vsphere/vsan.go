// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import (
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	rs "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/resources"
	scrapepkg "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/scrape"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/oldmetrix"
)

const (
	vsanClusterSpaceUsageTotalMetric      = "vsan_cluster_space_usage_total"
	vsanClusterSpaceUsageFreeMetric       = "vsan_cluster_space_usage_free"
	vsanClusterSpaceUsageUsedMetric       = "vsan_cluster_space_usage_used"
	vsanClusterSpaceUtilizationUsedMetric = "vsan_cluster_space_utilization_used"
	vsanClusterHealthStatusGreenMetric    = "vsan_cluster_health_status_green"
	vsanClusterHealthStatusYellowMetric   = "vsan_cluster_health_status_yellow"
	vsanClusterHealthStatusRedMetric      = "vsan_cluster_health_status_red"
	vsanClusterHealthStatusUnknownMetric  = "vsan_cluster_health_status_unknown"
	vsanClusterOperationsReadMetric       = "vsan_cluster_operations_read"
	vsanClusterOperationsWriteMetric      = "vsan_cluster_operations_write"
	vsanClusterThroughputReadMetric       = "vsan_cluster_throughput_read"
	vsanClusterThroughputWriteMetric      = "vsan_cluster_throughput_write"
	vsanClusterLatencyReadMetric          = "vsan_cluster_latency_read"
	vsanClusterLatencyWriteMetric         = "vsan_cluster_latency_write"
	vsanClusterCongestionsMetric          = "vsan_cluster_congestions"
	vsanHostOperationsReadMetric          = "vsan_host_operations_read"
	vsanHostOperationsWriteMetric         = "vsan_host_operations_write"
	vsanHostThroughputReadMetric          = "vsan_host_throughput_read"
	vsanHostThroughputWriteMetric         = "vsan_host_throughput_write"
	vsanHostLatencyReadMetric             = "vsan_host_latency_read"
	vsanHostLatencyWriteMetric            = "vsan_host_latency_write"
	vsanHostCongestionsMetric             = "vsan_host_congestions"
	vsanHostCacheHitRateMetric            = "vsan_host_cache_hit_rate"
	vsanVMOperationsReadMetric            = "vsan_vm_operations_read"
	vsanVMOperationsWriteMetric           = "vsan_vm_operations_write"
	vsanVMThroughputReadMetric            = "vsan_vm_throughput_read"
	vsanVMThroughputWriteMetric           = "vsan_vm_throughput_write"
	vsanVMLatencyReadMetric               = "vsan_vm_latency_read"
	vsanVMLatencyWriteMetric              = "vsan_vm_latency_write"
	vsanUUIDLabel                         = "vsan_uuid"
	vsanNodeUUIDLabel                     = "vsan_node_uuid"
	vmInstanceUUIDLabel                   = "vm_instance_uuid"
)

func (c *Collector) writeVSANMetrics() {
	if !c.CollectVSAN || c.resources == nil || c.vsanMetrics == nil {
		return
	}
	c.writeVSANClusterSpaceMetrics()
	c.writeVSANClusterHealthMetrics()
	c.writeVSANClusterPerformanceMetrics()
	c.writeVSANHostPerformanceMetrics()
	c.writeVSANVMPerformanceMetrics()
}

func (c *Collector) vsanResources() (rs.Clusters, rs.Hosts, rs.VMs) {
	clusters := make(rs.Clusters)
	hosts := make(rs.Hosts)
	vms := make(rs.VMs)

	clusterMatcher := c.vsanClusterMatcher
	hostMatcher := c.vsanHostMatcher
	vmMatcher := c.vsanVMMatcher

	selectedClusters := make(map[string]bool)
	for _, cluster := range sortedClusters(c.resources.Clusters) {
		if !cluster.VSANEnabled || (clusterMatcher != nil && !clusterMatcher.Match(cluster)) {
			continue
		}
		clusters[cluster.ID] = cluster
		selectedClusters[cluster.ID] = true
	}

	for _, host := range sortedHosts(c.resources.Hosts) {
		if !selectedClusters[host.Hier.Cluster.ID] || host.VSANNodeUUID == "" || (hostMatcher != nil && !hostMatcher.Match(host)) {
			continue
		}
		hosts[host.ID] = host
	}

	for _, vm := range sortedVMs(c.resources.VMs) {
		if !selectedClusters[vm.Hier.Cluster.ID] || vm.InstanceUUID == "" || (vmMatcher != nil && !vmMatcher.Match(vm)) {
			continue
		}
		vms[vm.ID] = vm
	}

	return clusters, hosts, vms
}

func (c *Collector) writeVSANClusterSpaceMetrics() {
	for _, id := range sortedMapKeys(c.vsanMetrics.Space) {
		cluster := c.resources.Clusters.Get(id)
		if cluster == nil {
			continue
		}
		space := c.vsanMetrics.Space[id]
		used := max(space.Total-space.Free, 0)
		labels := c.labelSet(c.vsanClusterLabels(cluster))
		c.observeGaugeFloat(vsanClusterSpaceUsageTotalMetric, float64(space.Total), labels)
		c.observeGaugeFloat(vsanClusterSpaceUsageFreeMetric, float64(space.Free), labels)
		c.observeGaugeFloat(vsanClusterSpaceUsageUsedMetric, float64(used), labels)
		if space.Total > 0 {
			c.observeGaugeFloat(vsanClusterSpaceUtilizationUsedMetric, float64(used)/float64(space.Total)*scaledPercent, labels)
		} else {
			c.observeGaugeFloat(vsanClusterSpaceUtilizationUsedMetric, 0, labels)
		}
	}
}

func (c *Collector) writeVSANClusterHealthMetrics() {
	for _, id := range sortedMapKeys(c.vsanMetrics.Health) {
		cluster := c.resources.Clusters.Get(id)
		if cluster == nil {
			continue
		}
		health := c.vsanMetrics.Health[id]
		labels := c.labelSet(c.vsanClusterLabels(cluster))
		c.observeGaugeFloat(vsanClusterHealthStatusGreenMetric, float64(oldmetrix.Bool(health == "green")), labels)
		c.observeGaugeFloat(vsanClusterHealthStatusYellowMetric, float64(oldmetrix.Bool(health == "yellow")), labels)
		c.observeGaugeFloat(vsanClusterHealthStatusRedMetric, float64(oldmetrix.Bool(health == "red")), labels)
		c.observeGaugeFloat(vsanClusterHealthStatusUnknownMetric, float64(oldmetrix.Bool(health != "green" && health != "yellow" && health != "red")), labels)
	}
}

func (c *Collector) writeVSANClusterPerformanceMetrics() {
	for _, id := range sortedMapKeys(c.vsanMetrics.Clusters) {
		cluster := c.resources.Clusters.Get(id)
		if cluster == nil {
			continue
		}
		labels := c.labelSet(c.vsanClusterLabels(cluster))
		writeVSANPerformanceValues(c, labels, c.vsanMetrics.Clusters[id], vsanClusterPerfMetricByName)
	}
}

func (c *Collector) writeVSANHostPerformanceMetrics() {
	for _, id := range sortedMapKeys(c.vsanMetrics.Hosts) {
		host := c.resources.Hosts.Get(id)
		if host == nil {
			continue
		}
		labels := c.labelSet(c.vsanHostLabels(host))
		writeVSANPerformanceValues(c, labels, c.vsanMetrics.Hosts[id], vsanHostPerfMetricByName)
	}
}

func (c *Collector) writeVSANVMPerformanceMetrics() {
	for _, id := range sortedMapKeys(c.vsanMetrics.VMs) {
		vm := c.resources.VMs.Get(id)
		if vm == nil {
			continue
		}
		labels := c.labelSet(c.vsanVMLabels(vm))
		writeVSANPerformanceValues(c, labels, c.vsanMetrics.VMs[id], vsanVMPerfMetricByName)
	}
}

func writeVSANPerformanceValues(c *Collector, labels metrix.LabelSet, values scrapepkg.VSANEntityMetrics, metricByName map[string]string) {
	for _, name := range sortedMapKeys(values) {
		metricName := metricByName[name]
		if metricName == "" {
			continue
		}
		c.observeGaugeFloat(metricName, values[name], labels)
	}
}

var vsanClusterPerfMetricByName = map[string]string{
	"read_operations":  vsanClusterOperationsReadMetric,
	"write_operations": vsanClusterOperationsWriteMetric,
	"read_throughput":  vsanClusterThroughputReadMetric,
	"write_throughput": vsanClusterThroughputWriteMetric,
	"read_latency":     vsanClusterLatencyReadMetric,
	"write_latency":    vsanClusterLatencyWriteMetric,
	"congestions":      vsanClusterCongestionsMetric,
}

var vsanHostPerfMetricByName = map[string]string{
	"read_operations":  vsanHostOperationsReadMetric,
	"write_operations": vsanHostOperationsWriteMetric,
	"read_throughput":  vsanHostThroughputReadMetric,
	"write_throughput": vsanHostThroughputWriteMetric,
	"read_latency":     vsanHostLatencyReadMetric,
	"write_latency":    vsanHostLatencyWriteMetric,
	"congestions":      vsanHostCongestionsMetric,
	"cache_hit_rate":   vsanHostCacheHitRateMetric,
}

var vsanVMPerfMetricByName = map[string]string{
	"read_operations":  vsanVMOperationsReadMetric,
	"write_operations": vsanVMOperationsWriteMetric,
	"read_throughput":  vsanVMThroughputReadMetric,
	"write_throughput": vsanVMThroughputWriteMetric,
	"read_latency":     vsanVMLatencyReadMetric,
	"write_latency":    vsanVMLatencyWriteMetric,
}

func (c *Collector) vsanClusterLabels(cluster *rs.Cluster) []metrix.Label {
	return c.v2MetricLabels(cluster.ID, []metrix.Label{
		{Key: "datacenter", Value: cluster.Hier.DC.Name},
		{Key: "cluster", Value: cluster.Name},
		{Key: vsanUUIDLabel, Value: cluster.VSANUUID},
	}, cluster.Labels)
}

func (c *Collector) vsanHostLabels(host *rs.Host) []metrix.Label {
	return c.v2MetricLabels(host.ID, []metrix.Label{
		{Key: "datacenter", Value: host.Hier.DC.Name},
		{Key: "cluster", Value: getHostClusterName(host)},
		{Key: "host", Value: host.Name},
		{Key: vsanNodeUUIDLabel, Value: host.VSANNodeUUID},
	}, host.Labels)
}

func (c *Collector) vsanVMLabels(vm *rs.VM) []metrix.Label {
	return c.v2MetricLabels(vm.ID, []metrix.Label{
		{Key: "datacenter", Value: vm.Hier.DC.Name},
		{Key: "cluster", Value: getVMClusterName(vm)},
		{Key: "host", Value: vm.Hier.Host.Name},
		{Key: "vm", Value: vm.Name},
		{Key: vmInstanceUUIDLabel, Value: vm.InstanceUUID},
	}, vm.Labels)
}
