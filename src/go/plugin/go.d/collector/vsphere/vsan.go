// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import (
	"sort"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	rs "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/resources"
	scrapepkg "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/scrape"
)

const (
	defaultMaxVSANClusters = 256
	defaultMaxVSANHosts    = 1024
	defaultMaxVSANVMs      = 1024

	vsanClusterSpaceUsageContext       = "vsphere.vsan_cluster_space_usage"
	vsanClusterSpaceUtilizationContext = "vsphere.vsan_cluster_space_utilization"
	vsanClusterHealthStatusContext     = "vsphere.vsan_cluster_health_status"
	vsanClusterOperationsContext       = "vsphere.vsan_cluster_operations"
	vsanClusterThroughputContext       = "vsphere.vsan_cluster_throughput"
	vsanClusterLatencyContext          = "vsphere.vsan_cluster_latency"
	vsanClusterCongestionsContext      = "vsphere.vsan_cluster_congestions"
	vsanHostOperationsContext          = "vsphere.vsan_host_operations"
	vsanHostThroughputContext          = "vsphere.vsan_host_throughput"
	vsanHostLatencyContext             = "vsphere.vsan_host_latency"
	vsanHostCongestionsContext         = "vsphere.vsan_host_congestions"
	vsanHostCacheHitRateContext        = "vsphere.vsan_host_cache_hit_rate"
	vsanVMOperationsContext            = "vsphere.vsan_vm_operations"
	vsanVMThroughputContext            = "vsphere.vsan_vm_throughput"
	vsanVMLatencyContext               = "vsphere.vsan_vm_latency"

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
	vsanSpaceUsageTotalDim                = "total"
	vsanSpaceUsageFreeDim                 = "free"
	vsanSpaceUsageUsedDim                 = "used"
	vsanHealthStatusGreenDim              = "green"
	vsanHealthStatusYellowDim             = "yellow"
	vsanHealthStatusRedDim                = "red"
	vsanHealthStatusUnknownDim            = "unknown"
	vsanOperationsReadDim                 = "read"
	vsanOperationsWriteDim                = "write"
	vsanThroughputReadDim                 = "read"
	vsanThroughputWriteDim                = "write"
	vsanLatencyReadDim                    = "read"
	vsanLatencyWriteDim                   = "write"
	vsanCongestionsDim                    = "congestions"
	vsanCacheHitRateDim                   = "hit_rate"
	vsanUUIDLabel                         = "vsan_uuid"
	vsanNodeUUIDLabel                     = "vsan_node_uuid"
	vmInstanceUUIDLabel                   = "vm_instance_uuid"
)

func vsanOptionalMetricNames() []string {
	return []string{
		vsanClusterSpaceUsageTotalMetric,
		vsanClusterSpaceUsageFreeMetric,
		vsanClusterSpaceUsageUsedMetric,
		vsanClusterSpaceUtilizationUsedMetric,
		vsanClusterHealthStatusGreenMetric,
		vsanClusterHealthStatusYellowMetric,
		vsanClusterHealthStatusRedMetric,
		vsanClusterHealthStatusUnknownMetric,
		vsanClusterOperationsReadMetric,
		vsanClusterOperationsWriteMetric,
		vsanClusterThroughputReadMetric,
		vsanClusterThroughputWriteMetric,
		vsanClusterLatencyReadMetric,
		vsanClusterLatencyWriteMetric,
		vsanClusterCongestionsMetric,
		vsanHostOperationsReadMetric,
		vsanHostOperationsWriteMetric,
		vsanHostThroughputReadMetric,
		vsanHostThroughputWriteMetric,
		vsanHostLatencyReadMetric,
		vsanHostLatencyWriteMetric,
		vsanHostCongestionsMetric,
		vsanHostCacheHitRateMetric,
		vsanVMOperationsReadMetric,
		vsanVMOperationsWriteMetric,
		vsanVMThroughputReadMetric,
		vsanVMThroughputWriteMetric,
		vsanVMLatencyReadMetric,
		vsanVMLatencyWriteMetric,
	}
}

func (c *Collector) writeVSANMetrics(meter metrix.SnapshotMeter) {
	if !c.CollectVSAN || c.resources == nil || c.vsanMetrics == nil {
		return
	}
	c.writeVSANClusterSpaceMetrics(meter)
	c.writeVSANClusterHealthMetrics(meter)
	c.writeVSANClusterPerformanceMetrics(meter)
	c.writeVSANHostPerformanceMetrics(meter)
	c.writeVSANVMPerformanceMetrics(meter)
}

func (c *Collector) vsanResources() (rs.Clusters, rs.Hosts, rs.VMs) {
	clusters := make(rs.Clusters)
	hosts := make(rs.Hosts)
	vms := make(rs.VMs)

	clusterMatcher := c.vsanClusterMatcher
	if clusterMatcher == nil {
		clusterMatcher = matcher.TRUE()
	}
	hostMatcher := c.vsanHostMatcher
	if hostMatcher == nil {
		hostMatcher = matcher.TRUE()
	}
	vmMatcher := c.vsanVMMatcher
	if vmMatcher == nil {
		vmMatcher = matcher.TRUE()
	}

	selectedClusters := make(map[string]bool)
	for _, cluster := range sortedClusters(c.resources.Clusters) {
		if len(clusters) >= c.MaxVSANClusters {
			break
		}
		if !cluster.VSANEnabled || !vsanClusterMatches(clusterMatcher, cluster) {
			continue
		}
		clusters[cluster.ID] = cluster
		selectedClusters[cluster.ID] = true
	}

	for _, host := range sortedHosts(c.resources.Hosts) {
		if len(hosts) >= c.MaxVSANHosts {
			break
		}
		if !selectedClusters[host.Hier.Cluster.ID] || host.VSANNodeUUID == "" || !vsanHostMatches(hostMatcher, host) {
			continue
		}
		hosts[host.ID] = host
	}

	for _, vm := range sortedVMs(c.resources.VMs) {
		if len(vms) >= c.MaxVSANVMs {
			break
		}
		if !selectedClusters[vm.Hier.Cluster.ID] || vm.InstanceUUID == "" || !vsanVMMatches(vmMatcher, vm) {
			continue
		}
		vms[vm.ID] = vm
	}

	return clusters, hosts, vms
}

func vsanClusterMatches(m matcher.Matcher, cluster *rs.Cluster) bool {
	return m.MatchString(vsanClusterPath(cluster)) ||
		m.MatchString(cluster.Name) ||
		m.MatchString(cluster.ID) ||
		m.MatchString("vsan_uuid:"+cluster.VSANUUID)
}

func vsanHostMatches(m matcher.Matcher, host *rs.Host) bool {
	return m.MatchString(vsanHostPath(host)) ||
		m.MatchString(host.Name) ||
		m.MatchString(host.ID) ||
		m.MatchString("vsan_node_uuid:"+host.VSANNodeUUID)
}

func vsanVMMatches(m matcher.Matcher, vm *rs.VM) bool {
	return m.MatchString(vsanVMPath(vm)) ||
		m.MatchString(vm.Name) ||
		m.MatchString(vm.ID) ||
		m.MatchString("instance_uuid:"+vm.InstanceUUID)
}

func vsanClusterPath(cluster *rs.Cluster) string {
	if cluster.Hier.DC.Name == "" {
		return "/" + cluster.Name
	}
	return "/" + cluster.Hier.DC.Name + "/" + cluster.Name
}

func vsanHostPath(host *rs.Host) string {
	return "/" + host.Hier.DC.Name + "/" + host.Hier.Cluster.Name + "/" + host.Name
}

func vsanVMPath(vm *rs.VM) string {
	return "/" + vm.Hier.DC.Name + "/" + vm.Hier.Cluster.Name + "/" + vm.Hier.Host.Name + "/" + vm.Name
}

func (c *Collector) writeVSANClusterSpaceMetrics(meter metrix.SnapshotMeter) {
	for _, id := range sortedMapKeys(c.vsanMetrics.Space) {
		cluster := c.resources.Clusters.Get(id)
		if cluster == nil {
			continue
		}
		space := c.vsanMetrics.Space[id]
		used := max(space.Total-space.Free, 0)
		labels := meter.LabelSet(c.vsanClusterLabels(cluster)...)
		c.observeGaugeFloat(meter, vsanClusterSpaceUsageTotalMetric, float64(space.Total), labels, false)
		c.observeGaugeFloat(meter, vsanClusterSpaceUsageFreeMetric, float64(space.Free), labels, false)
		c.observeGaugeFloat(meter, vsanClusterSpaceUsageUsedMetric, float64(used), labels, false)
		if space.Total > 0 {
			c.observeGaugeFloat(meter, vsanClusterSpaceUtilizationUsedMetric, float64(used)/float64(space.Total)*10000, labels, false)
		} else {
			c.observeGaugeFloat(meter, vsanClusterSpaceUtilizationUsedMetric, 0, labels, false)
		}
	}
}

func (c *Collector) writeVSANClusterHealthMetrics(meter metrix.SnapshotMeter) {
	for _, id := range sortedMapKeys(c.vsanMetrics.Health) {
		cluster := c.resources.Clusters.Get(id)
		if cluster == nil {
			continue
		}
		health := c.vsanMetrics.Health[id]
		labels := meter.LabelSet(c.vsanClusterLabels(cluster)...)
		c.observeGaugeFloat(meter, vsanClusterHealthStatusGreenMetric, boolFloat(health == "green"), labels, false)
		c.observeGaugeFloat(meter, vsanClusterHealthStatusYellowMetric, boolFloat(health == "yellow"), labels, false)
		c.observeGaugeFloat(meter, vsanClusterHealthStatusRedMetric, boolFloat(health == "red"), labels, false)
		c.observeGaugeFloat(meter, vsanClusterHealthStatusUnknownMetric, boolFloat(health != "green" && health != "yellow" && health != "red"), labels, false)
	}
}

func (c *Collector) writeVSANClusterPerformanceMetrics(meter metrix.SnapshotMeter) {
	for _, id := range sortedMapKeys(c.vsanMetrics.Clusters) {
		cluster := c.resources.Clusters.Get(id)
		if cluster == nil {
			continue
		}
		labels := meter.LabelSet(c.vsanClusterLabels(cluster)...)
		writeVSANPerformanceValues(c, meter, labels, c.vsanMetrics.Clusters[id], vsanClusterPerfMetricByName, false)
	}
}

func (c *Collector) writeVSANHostPerformanceMetrics(meter metrix.SnapshotMeter) {
	for _, id := range sortedMapKeys(c.vsanMetrics.Hosts) {
		host := c.resources.Hosts.Get(id)
		if host == nil {
			continue
		}
		scope := c.resourceHostScope(host.ID)
		writeMeter := meter
		scoped := !scope.IsDefault()
		if scoped {
			writeMeter = meter.WithHostScope(scope)
		}
		labels := writeMeter.LabelSet(c.vsanHostLabels(host)...)
		writeVSANPerformanceValues(c, writeMeter, labels, c.vsanMetrics.Hosts[id], vsanHostPerfMetricByName, scoped)
	}
}

func (c *Collector) writeVSANVMPerformanceMetrics(meter metrix.SnapshotMeter) {
	for _, id := range sortedMapKeys(c.vsanMetrics.VMs) {
		vm := c.resources.VMs.Get(id)
		if vm == nil {
			continue
		}
		scope := c.resourceHostScope(vm.ID)
		writeMeter := meter
		scoped := !scope.IsDefault()
		if scoped {
			writeMeter = meter.WithHostScope(scope)
		}
		labels := writeMeter.LabelSet(c.vsanVMLabels(vm)...)
		writeVSANPerformanceValues(c, writeMeter, labels, c.vsanMetrics.VMs[id], vsanVMPerfMetricByName, scoped)
	}
}

func writeVSANPerformanceValues(c *Collector, meter metrix.SnapshotMeter, labels metrix.LabelSet, values scrapepkg.VSANEntityMetrics, metricByName map[string]string, scoped bool) {
	for _, name := range sortedMapKeys(values) {
		metricName := metricByName[name]
		if metricName == "" {
			continue
		}
		c.observeGaugeFloat(meter, metricName, values[name], labels, scoped)
	}
}

func (c *Collector) observeGaugeFloat(meter metrix.SnapshotMeter, name string, value float64, labels metrix.LabelSet, scoped bool) {
	if gauge := c.mx.gauge(meter, name, scoped); gauge != nil {
		gauge.Observe(metrix.SampleValue(value), labels)
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
	labels := []metrix.Label{
		{Key: "id", Value: cluster.ID},
		{Key: "datacenter", Value: cluster.Hier.DC.Name},
		{Key: "cluster", Value: cluster.Name},
		{Key: vsanUUIDLabel, Value: cluster.VSANUUID},
	}
	labels = append(labels, c.resourceEnrichmentLabels(cluster.ID)...)
	return labels
}

func (c *Collector) vsanHostLabels(host *rs.Host) []metrix.Label {
	labels := []metrix.Label{
		{Key: "id", Value: host.ID},
		{Key: "datacenter", Value: host.Hier.DC.Name},
		{Key: "cluster", Value: getHostClusterName(host)},
		{Key: "host", Value: host.Name},
		{Key: vsanNodeUUIDLabel, Value: host.VSANNodeUUID},
	}
	labels = append(labels, c.resourceEnrichmentLabels(host.ID)...)
	return labels
}

func (c *Collector) vsanVMLabels(vm *rs.VM) []metrix.Label {
	labels := []metrix.Label{
		{Key: "id", Value: vm.ID},
		{Key: "datacenter", Value: vm.Hier.DC.Name},
		{Key: "cluster", Value: getVMClusterName(vm)},
		{Key: "host", Value: vm.Hier.Host.Name},
		{Key: "vm", Value: vm.Name},
		{Key: vmInstanceUUIDLabel, Value: vm.InstanceUUID},
	}
	labels = append(labels, c.resourceEnrichmentLabels(vm.ID)...)
	return labels
}

func sortedMapKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func boolFloat(v bool) float64 {
	if v {
		return 1
	}
	return 0
}
