// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	rs "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/resources"
)

const (
	prioClusterHosts = collectorapi.Priority + iota
	prioClusterCPUCapacity
	prioClusterMemCapacity
	prioClusterCPUTopology
	prioClusterDRSConfig
	prioClusterHAConfig
	prioClusterOverallStatus
	prioClusterVMotions
	prioClusterDRSScore
	prioClusterDRSBalance
	prioClusterVMCount
	prioClusterUsageCPU
	prioClusterUsageMem
	prioClusterCPUUtilization
	prioClusterCPUUsage
	prioClusterMemUtilization
	prioClusterMemUsage
	prioClusterServicesFairness
	prioClusterServicesEffectiveCPU
	prioClusterServicesEffectiveMem
	prioClusterServicesFailover
	prioClusterVMMigrations
	prioClusterVMLifecycle
	prioClusterVMManagement
	prioClusterVMGuestOps
	prioClusterVMColdMigrations

	prioResourcePoolCPUUsage
	prioResourcePoolCPUEntitlement
	prioResourcePoolCPUAllocation
	prioResourcePoolMemUsage
	prioResourcePoolMemEntitlement
	prioResourcePoolMemAllocation
	prioResourcePoolMemBreakdown
	prioResourcePoolCPUConfig
	prioResourcePoolMemConfig
	prioResourcePoolOverallStatus

	prioDatastoreDiskIO
	prioDatastoreDiskIOPS
	prioDatastoreDiskLatency
	prioDatastoreSpaceUtilization
	prioDatastoreSpaceUsage
	prioDatastoreOverallStatus

	prioVMCPUUtilization
	prioVmMemoryUtilization
	prioVmMemoryUsage
	prioVmMemorySwapUsage
	prioVmMemorySwapIO
	prioVmDiskIO
	prioVmDiskMaxLatency
	prioVmNetworkTraffic
	prioVmNetworkPackets
	prioVmNetworkDrops
	prioVmOverallStatus
	prioVmSystemUptime

	prioHostCPUUtilization
	prioHostMemoryUtilization
	prioHostMemoryUsage
	prioHostMemorySwapIO
	prioHostDiskIO
	prioHostDiskMaxLatency
	prioHostNetworkTraffic
	prioHostNetworkPackets
	prioHostNetworkDrops
	prioHostNetworkErrors
	prioHostOverallStatus
	prioHostSystemUptime
)

var (
	vmChartsTmpl = collectorapi.Charts{
		vmCPUUtilizationChartTmpl.Copy(),

		vmMemoryUtilizationChartTmpl.Copy(),
		vmMemoryUsageChartTmpl.Copy(),
		vmMemorySwapUsageChartTmpl.Copy(),
		vmMemorySwapIOChartTmpl.Copy(),

		vmDiskIOChartTmpl.Copy(),
		vmDiskMaxLatencyChartTmpl.Copy(),

		vmNetworkTrafficChartTmpl.Copy(),
		vmNetworkPacketsChartTmpl.Copy(),
		vmNetworkDropsChartTmpl.Copy(),

		vmOverallStatusChartTmpl.Copy(),

		vmSystemUptimeChartTmpl.Copy(),
	}

	vmCPUUtilizationChartTmpl = collectorapi.Chart{
		ID:       "%s_cpu_utilization",
		Title:    "Virtual Machine CPU utilization",
		Units:    "percentage",
		Fam:      "vms cpu",
		Ctx:      "vsphere.vm_cpu_utilization",
		Priority: prioVMCPUUtilization,
		Dims: collectorapi.Dims{
			{ID: "%s_cpu.usage.average", Name: "used", Div: 100},
		},
	}

	// Ref: https://www.vmware.com/support/developer/converter-sdk/conv51_apireference/memory_counters.html
	vmMemoryUtilizationChartTmpl = collectorapi.Chart{
		ID:       "%s_mem_utilization",
		Title:    "Virtual Machine memory utilization",
		Units:    "percentage",
		Fam:      "vms mem",
		Ctx:      "vsphere.vm_mem_utilization",
		Priority: prioVmMemoryUtilization,
		Dims: collectorapi.Dims{
			{ID: "%s_mem.usage.average", Name: "used", Div: 100},
		},
	}
	vmMemoryUsageChartTmpl = collectorapi.Chart{
		ID:       "%s_mem_usage",
		Title:    "Virtual Machine memory usage",
		Units:    "KiB",
		Fam:      "vms mem",
		Ctx:      "vsphere.vm_mem_usage",
		Priority: prioVmMemoryUsage,
		Dims: collectorapi.Dims{
			{ID: "%s_mem.granted.average", Name: "granted"},
			{ID: "%s_mem.consumed.average", Name: "consumed"},
			{ID: "%s_mem.active.average", Name: "active"},
			{ID: "%s_mem.shared.average", Name: "shared"},
		},
	}
	vmMemorySwapUsageChartTmpl = collectorapi.Chart{
		ID:       "%s_mem_swap_usage",
		Title:    "Virtual Machine VMKernel memory swap usage",
		Units:    "KiB",
		Fam:      "vms mem",
		Ctx:      "vsphere.vm_mem_swap_usage",
		Priority: prioVmMemorySwapUsage,
		Dims: collectorapi.Dims{
			{ID: "%s_mem.swapped.average", Name: "swapped"},
		},
	}
	vmMemorySwapIOChartTmpl = collectorapi.Chart{
		ID:       "%s_mem_swap_io_rate",
		Title:    "Virtual Machine VMKernel memory swap IO",
		Units:    "KiB/s",
		Fam:      "vms mem",
		Ctx:      "vsphere.vm_mem_swap_io",
		Type:     collectorapi.Area,
		Priority: prioVmMemorySwapIO,
		Dims: collectorapi.Dims{
			{ID: "%s_mem.swapinRate.average", Name: "in"},
			{ID: "%s_mem.swapoutRate.average", Name: "out"},
		},
	}

	vmDiskIOChartTmpl = collectorapi.Chart{
		ID:       "%s_disk_io",
		Title:    "Virtual Machine disk IO",
		Units:    "KiB/s",
		Fam:      "vms disk",
		Ctx:      "vsphere.vm_disk_io",
		Type:     collectorapi.Area,
		Priority: prioVmDiskIO,
		Dims: collectorapi.Dims{
			{ID: "%s_disk.read.average", Name: "read"},
			{ID: "%s_disk.write.average", Name: "write", Mul: -1},
		},
	}
	vmDiskMaxLatencyChartTmpl = collectorapi.Chart{
		ID:       "%s_disk_max_latency",
		Title:    "Virtual Machine disk max latency",
		Units:    "milliseconds",
		Fam:      "vms disk",
		Ctx:      "vsphere.vm_disk_max_latency",
		Priority: prioVmDiskMaxLatency,
		Dims: collectorapi.Dims{
			{ID: "%s_disk.maxTotalLatency.latest", Name: "latency"},
		},
	}

	vmNetworkTrafficChartTmpl = collectorapi.Chart{
		ID:       "%s_net_traffic",
		Title:    "Virtual Machine network traffic",
		Units:    "KiB/s",
		Fam:      "vms net",
		Ctx:      "vsphere.vm_net_traffic",
		Type:     collectorapi.Area,
		Priority: prioVmNetworkTraffic,
		Dims: collectorapi.Dims{
			{ID: "%s_net.bytesRx.average", Name: "received"},
			{ID: "%s_net.bytesTx.average", Name: "sent", Mul: -1},
		},
	}
	vmNetworkPacketsChartTmpl = collectorapi.Chart{
		ID:       "%s_net_packets",
		Title:    "Virtual Machine network packets",
		Units:    "packets",
		Fam:      "vms net",
		Ctx:      "vsphere.vm_net_packets",
		Priority: prioVmNetworkPackets,
		Dims: collectorapi.Dims{
			{ID: "%s_net.packetsRx.summation", Name: "received"},
			{ID: "%s_net.packetsTx.summation", Name: "sent", Mul: -1},
		},
	}
	vmNetworkDropsChartTmpl = collectorapi.Chart{
		ID:       "%s_net_drops",
		Title:    "Virtual Machine network dropped packets",
		Units:    "drops",
		Fam:      "vms net",
		Ctx:      "vsphere.vm_net_drops",
		Priority: prioVmNetworkDrops,
		Dims: collectorapi.Dims{
			{ID: "%s_net.droppedRx.summation", Name: "received"},
			{ID: "%s_net.droppedTx.summation", Name: "sent", Mul: -1},
		},
	}

	vmOverallStatusChartTmpl = collectorapi.Chart{
		ID:       "%s_overall_status",
		Title:    "Virtual Machine overall alarm status",
		Units:    "status",
		Fam:      "vms status",
		Ctx:      "vsphere.vm_overall_status",
		Priority: prioVmOverallStatus,
		Dims: collectorapi.Dims{
			{ID: "%s_overall.status.green", Name: "green"},
			{ID: "%s_overall.status.red", Name: "red"},
			{ID: "%s_overall.status.yellow", Name: "yellow"},
			{ID: "%s_overall.status.gray", Name: "gray"},
		},
	}

	vmSystemUptimeChartTmpl = collectorapi.Chart{
		ID:       "%s_system_uptime",
		Title:    "Virtual Machine system uptime",
		Units:    "seconds",
		Fam:      "vms uptime",
		Ctx:      "vsphere.vm_system_uptime",
		Priority: prioVmSystemUptime,
		Dims: collectorapi.Dims{
			{ID: "%s_sys.uptime.latest", Name: "uptime"},
		},
	}
)

var (
	hostChartsTmpl = collectorapi.Charts{
		hostCPUUtilizationChartTmpl.Copy(),

		hostMemUtilizationChartTmpl.Copy(),
		hostMemUsageChartTmpl.Copy(),
		hostMemSwapIOChartTmpl.Copy(),

		hostDiskIOChartTmpl.Copy(),
		hostDiskMaxLatencyChartTmpl.Copy(),

		hostNetworkTraffic.Copy(),
		hostNetworkPacketsChartTmpl.Copy(),
		hostNetworkDropsChartTmpl.Copy(),
		hostNetworkErrorsChartTmpl.Copy(),

		hostOverallStatusChartTmpl.Copy(),

		hostSystemUptimeChartTmpl.Copy(),
	}
	hostCPUUtilizationChartTmpl = collectorapi.Chart{
		ID:       "%s_cpu_usage_total",
		Title:    "ESXi Host CPU utilization",
		Units:    "percentage",
		Fam:      "hosts cpu",
		Ctx:      "vsphere.host_cpu_utilization",
		Priority: prioHostCPUUtilization,
		Dims: collectorapi.Dims{
			{ID: "%s_cpu.usage.average", Name: "used", Div: 100},
		},
	}
	hostMemUtilizationChartTmpl = collectorapi.Chart{
		ID:       "%s_mem_utilization",
		Title:    "ESXi Host memory utilization",
		Units:    "percentage",
		Fam:      "hosts mem",
		Ctx:      "vsphere.host_mem_utilization",
		Priority: prioHostMemoryUtilization,
		Dims: collectorapi.Dims{
			{ID: "%s_mem.usage.average", Name: "used", Div: 100},
		},
	}
	hostMemUsageChartTmpl = collectorapi.Chart{
		ID:       "%s_mem_usage",
		Title:    "ESXi Host memory usage",
		Units:    "KiB",
		Fam:      "hosts mem",
		Ctx:      "vsphere.host_mem_usage",
		Priority: prioHostMemoryUsage,
		Dims: collectorapi.Dims{
			{ID: "%s_mem.granted.average", Name: "granted"},
			{ID: "%s_mem.consumed.average", Name: "consumed"},
			{ID: "%s_mem.active.average", Name: "active"},
			{ID: "%s_mem.shared.average", Name: "shared"},
			{ID: "%s_mem.sharedcommon.average", Name: "sharedcommon"},
		},
	}
	hostMemSwapIOChartTmpl = collectorapi.Chart{
		ID:       "%s_mem_swap_rate",
		Title:    "ESXi Host VMKernel memory swap IO",
		Units:    "KiB/s",
		Fam:      "hosts mem",
		Ctx:      "vsphere.host_mem_swap_io",
		Type:     collectorapi.Area,
		Priority: prioHostMemorySwapIO,
		Dims: collectorapi.Dims{
			{ID: "%s_mem.swapinRate.average", Name: "in"},
			{ID: "%s_mem.swapoutRate.average", Name: "out"},
		},
	}

	hostDiskIOChartTmpl = collectorapi.Chart{
		ID:       "%s_disk_io",
		Title:    "ESXi Host disk IO",
		Units:    "KiB/s",
		Fam:      "hosts disk",
		Ctx:      "vsphere.host_disk_io",
		Type:     collectorapi.Area,
		Priority: prioHostDiskIO,
		Dims: collectorapi.Dims{
			{ID: "%s_disk.read.average", Name: "read"},
			{ID: "%s_disk.write.average", Name: "write", Mul: -1},
		},
	}
	hostDiskMaxLatencyChartTmpl = collectorapi.Chart{
		ID:       "%s_disk_max_latency",
		Title:    "ESXi Host disk max latency",
		Units:    "milliseconds",
		Fam:      "hosts disk",
		Ctx:      "vsphere.host_disk_max_latency",
		Priority: prioHostDiskMaxLatency,
		Dims: collectorapi.Dims{
			{ID: "%s_disk.maxTotalLatency.latest", Name: "latency"},
		},
	}

	hostNetworkTraffic = collectorapi.Chart{
		ID:       "%s_net_traffic",
		Title:    "ESXi Host network traffic",
		Units:    "KiB/s",
		Fam:      "hosts net",
		Ctx:      "vsphere.host_net_traffic",
		Type:     collectorapi.Area,
		Priority: prioHostNetworkTraffic,
		Dims: collectorapi.Dims{
			{ID: "%s_net.bytesRx.average", Name: "received"},
			{ID: "%s_net.bytesTx.average", Name: "sent", Mul: -1},
		},
	}
	hostNetworkPacketsChartTmpl = collectorapi.Chart{
		ID:       "%s_net_packets",
		Title:    "ESXi Host network packets",
		Units:    "packets",
		Fam:      "hosts net",
		Ctx:      "vsphere.host_net_packets",
		Priority: prioHostNetworkPackets,
		Dims: collectorapi.Dims{
			{ID: "%s_net.packetsRx.summation", Name: "received"},
			{ID: "%s_net.packetsTx.summation", Name: "sent", Mul: -1},
		},
	}
	hostNetworkDropsChartTmpl = collectorapi.Chart{
		ID:       "%s_net_drops_total",
		Title:    "ESXi Host network drops",
		Units:    "drops",
		Fam:      "hosts net",
		Ctx:      "vsphere.host_net_drops",
		Priority: prioHostNetworkDrops,
		Dims: collectorapi.Dims{
			{ID: "%s_net.droppedRx.summation", Name: "received"},
			{ID: "%s_net.droppedTx.summation", Name: "sent", Mul: -1},
		},
	}
	hostNetworkErrorsChartTmpl = collectorapi.Chart{
		ID:       "%s_net_errors",
		Title:    "ESXi Host network errors",
		Units:    "errors",
		Fam:      "hosts net",
		Ctx:      "vsphere.host_net_errors",
		Priority: prioHostNetworkErrors,
		Dims: collectorapi.Dims{
			{ID: "%s_net.errorsRx.summation", Name: "received"},
			{ID: "%s_net.errorsTx.summation", Name: "sent", Mul: -1},
		},
	}

	hostOverallStatusChartTmpl = collectorapi.Chart{
		ID:       "%s_overall_status",
		Title:    "ESXi Host overall alarm status",
		Units:    "status",
		Fam:      "hosts status",
		Ctx:      "vsphere.host_overall_status",
		Priority: prioHostOverallStatus,
		Dims: collectorapi.Dims{
			{ID: "%s_overall.status.green", Name: "green"},
			{ID: "%s_overall.status.red", Name: "red"},
			{ID: "%s_overall.status.yellow", Name: "yellow"},
			{ID: "%s_overall.status.gray", Name: "gray"},
		},
	}
	hostSystemUptimeChartTmpl = collectorapi.Chart{
		ID:       "%s_system_uptime",
		Title:    "ESXi Host system uptime",
		Units:    "seconds",
		Fam:      "hosts uptime",
		Ctx:      "vsphere.host_system_uptime",
		Priority: prioHostSystemUptime,
		Dims: collectorapi.Dims{
			{ID: "%s_sys.uptime.latest", Name: "uptime"},
		},
	}
)

var (
	datastorePropertyChartsTmpl = collectorapi.Charts{
		datastoreSpaceUtilizationChartTmpl.Copy(),
		datastoreSpaceUsageChartTmpl.Copy(),
		datastoreOverallStatusChartTmpl.Copy(),
	}
	datastorePerfChartsTmpl = collectorapi.Charts{
		datastoreDiskIOChartTmpl.Copy(),
		datastoreDiskIOPSChartTmpl.Copy(),
		datastoreDiskLatencyChartTmpl.Copy(),
	}

	datastoreDiskIOChartTmpl = collectorapi.Chart{
		ID:       "%s_disk_io",
		Title:    "Datastore disk IO",
		Units:    "KiB/s",
		Fam:      "datastores disk",
		Ctx:      "vsphere.datastore_disk_io",
		Type:     collectorapi.Area,
		Priority: prioDatastoreDiskIO,
		Dims: collectorapi.Dims{
			{ID: "%s_datastore.read.average", Name: "read"},
			{ID: "%s_datastore.write.average", Name: "write", Mul: -1},
		},
	}
	datastoreDiskIOPSChartTmpl = collectorapi.Chart{
		ID:       "%s_disk_iops",
		Title:    "Datastore disk IOPS",
		Units:    "operations/s",
		Fam:      "datastores disk",
		Ctx:      "vsphere.datastore_disk_iops",
		Priority: prioDatastoreDiskIOPS,
		Dims: collectorapi.Dims{
			{ID: "%s_datastore.numberReadAveraged.average", Name: "reads"},
			{ID: "%s_datastore.numberWriteAveraged.average", Name: "writes", Mul: -1},
		},
	}
	datastoreDiskLatencyChartTmpl = collectorapi.Chart{
		ID:       "%s_disk_latency",
		Title:    "Datastore disk latency",
		Units:    "milliseconds",
		Fam:      "datastores disk",
		Ctx:      "vsphere.datastore_disk_latency",
		Priority: prioDatastoreDiskLatency,
		Dims: collectorapi.Dims{
			{ID: "%s_datastore.totalReadLatency.average", Name: "read"},
			{ID: "%s_datastore.totalWriteLatency.average", Name: "write"},
		},
	}

	datastoreSpaceUtilizationChartTmpl = collectorapi.Chart{
		ID:       "%s_space_utilization",
		Title:    "Datastore space utilization",
		Units:    "percentage",
		Fam:      "datastores space",
		Ctx:      "vsphere.datastore_space_utilization",
		Priority: prioDatastoreSpaceUtilization,
		Dims: collectorapi.Dims{
			{ID: "%s_used_space_pct", Name: "used", Div: 100},
		},
	}
	datastoreSpaceUsageChartTmpl = collectorapi.Chart{
		ID:       "%s_space_usage",
		Title:    "Datastore space usage",
		Units:    "bytes",
		Fam:      "datastores space",
		Ctx:      "vsphere.datastore_space_usage",
		Priority: prioDatastoreSpaceUsage,
		Dims: collectorapi.Dims{
			{ID: "%s_capacity", Name: "capacity"},
			{ID: "%s_free_space", Name: "free"},
			{ID: "%s_used_space", Name: "used"},
		},
	}

	datastoreOverallStatusChartTmpl = collectorapi.Chart{
		ID:       "%s_overall_status",
		Title:    "Datastore overall alarm status",
		Units:    "status",
		Fam:      "datastores status",
		Ctx:      "vsphere.datastore_overall_status",
		Priority: prioDatastoreOverallStatus,
		Dims: collectorapi.Dims{
			{ID: "%s_overall.status.green", Name: "green"},
			{ID: "%s_overall.status.red", Name: "red"},
			{ID: "%s_overall.status.yellow", Name: "yellow"},
			{ID: "%s_overall.status.gray", Name: "gray"},
		},
	}
)

const failedUpdatesLimit = 10

func (c *Collector) updateCharts() {
	for id, fails := range c.discoveredHosts {
		if fails >= failedUpdatesLimit {
			c.removeFromCharts(id)
			delete(c.charted, id)
			delete(c.discoveredHosts, id)
			continue
		}

		host := c.resources.Hosts.Get(id)
		if host == nil || c.charted[id] || fails != 0 {
			continue
		}

		c.charted[id] = true
		charts := newHostCharts(host)
		if err := c.Charts().Add(*charts...); err != nil {
			c.Error(err)
		}
	}

	for id, fails := range c.discoveredVMs {
		if fails >= failedUpdatesLimit {
			c.removeFromCharts(id)
			delete(c.charted, id)
			delete(c.discoveredVMs, id)
			continue
		}

		vm := c.resources.VMs.Get(id)
		if vm == nil || c.charted[id] || fails != 0 {
			continue
		}

		c.charted[id] = true
		charts := newVMCHarts(vm)
		if err := c.Charts().Add(*charts...); err != nil {
			c.Error(err)
		}
	}

	for id, fails := range c.discoveredDatastores {
		if fails >= failedUpdatesLimit {
			c.removeFromCharts(id)
			delete(c.charted, id)
			delete(c.discoveredDatastores, id)
			delete(c.datastorePerfReceived, id)
			delete(c.datastorePerfCharted, id)
			continue
		}

		ds := c.resources.Datastores.Get(id)
		if ds == nil || fails != 0 {
			continue
		}

		if !c.charted[id] {
			c.charted[id] = true
			charts := newDatastorePropertyCharts(ds)
			if err := c.Charts().Add(*charts...); err != nil {
				c.Error(err)
			}
		}

		if c.datastorePerfReceived[id] && !c.datastorePerfCharted[id] {
			c.datastorePerfCharted[id] = true
			charts := newDatastorePerfCharts(ds)
			if err := c.Charts().Add(*charts...); err != nil {
				c.Error(err)
			}
		}
	}

	for id, fails := range c.discoveredClusters {
		if fails >= failedUpdatesLimit {
			c.removeFromCharts(id)
			delete(c.charted, id)
			delete(c.discoveredClusters, id)
			delete(c.clusterPerfReceived, id)
			delete(c.clusterPerfCharted, id)
			continue
		}

		cl := c.resources.Clusters.Get(id)
		if cl == nil || fails != 0 {
			continue
		}

		if !c.charted[id] {
			c.charted[id] = true
			charts := newClusterPropertyCharts(cl)
			if err := c.Charts().Add(*charts...); err != nil {
				c.Error(err)
			}
		}

		if c.clusterPerfReceived[id] && !c.clusterPerfCharted[id] {
			c.clusterPerfCharted[id] = true
			charts := newClusterPerfCharts(cl)
			if err := c.Charts().Add(*charts...); err != nil {
				c.Error(err)
			}
		}
	}

	for id, fails := range c.discoveredResourcePools {
		if fails >= failedUpdatesLimit {
			c.removeFromCharts(id)
			delete(c.charted, id)
			delete(c.discoveredResourcePools, id)
			continue
		}

		rp := c.resources.ResourcePools.Get(id)
		if rp == nil || c.charted[id] || fails != 0 {
			continue
		}

		c.charted[id] = true
		charts := newResourcePoolCharts(rp)
		if err := c.Charts().Add(*charts...); err != nil {
			c.Error(err)
		}
	}
}

func newVMCHarts(vm *rs.VM) *collectorapi.Charts {
	charts := vmChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, vm.ID)
		chart.Labels = []collectorapi.Label{
			{Key: "datacenter", Value: vm.Hier.DC.Name},
			{Key: "cluster", Value: getVMClusterName(vm)},
			{Key: "host", Value: vm.Hier.Host.Name},
			{Key: "vm", Value: vm.Name},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, vm.ID)
		}
	}

	return charts
}

func getVMClusterName(vm *rs.VM) string {
	if vm.Hier.Cluster.Name == vm.Hier.Host.Name {
		return ""
	}
	return vm.Hier.Cluster.Name
}

func newHostCharts(host *rs.Host) *collectorapi.Charts {
	charts := hostChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, host.ID)
		chart.Labels = []collectorapi.Label{
			{Key: "datacenter", Value: host.Hier.DC.Name},
			{Key: "cluster", Value: getHostClusterName(host)},
			{Key: "host", Value: host.Name},
		}

		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, host.ID)
		}
	}

	return charts
}

func getHostClusterName(host *rs.Host) string {
	if host.Hier.Cluster.Name == host.Name {
		return ""
	}
	return host.Hier.Cluster.Name
}

func newDatastorePropertyCharts(ds *rs.Datastore) *collectorapi.Charts {
	charts := datastorePropertyChartsTmpl.Copy()
	applyDatastoreChartLabels(charts, ds)
	return charts
}

func newDatastorePerfCharts(ds *rs.Datastore) *collectorapi.Charts {
	charts := datastorePerfChartsTmpl.Copy()
	applyDatastoreChartLabels(charts, ds)
	return charts
}

func applyDatastoreChartLabels(charts *collectorapi.Charts, ds *rs.Datastore) {
	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, ds.ID)
		chart.Labels = []collectorapi.Label{
			{Key: "datacenter", Value: ds.Hier.DC.Name},
			{Key: "datastore", Value: ds.Name},
			{Key: "type", Value: ds.Type},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, ds.ID)
		}
	}
}

// --- Cluster chart templates ---

var (
	clusterPropertyChartsTmpl = collectorapi.Charts{
		clusterHostsChartTmpl.Copy(),
		clusterCPUCapacityChartTmpl.Copy(),
		clusterMemCapacityChartTmpl.Copy(),
		clusterCPUTopologyChartTmpl.Copy(),
		clusterDRSConfigChartTmpl.Copy(),
		clusterHAConfigChartTmpl.Copy(),
		clusterOverallStatusChartTmpl.Copy(),
		clusterVMotionsChartTmpl.Copy(),
		clusterDRSScoreChartTmpl.Copy(),
		clusterDRSBalanceChartTmpl.Copy(),
		clusterVMCountChartTmpl.Copy(),
		clusterUsageCPUChartTmpl.Copy(),
		clusterUsageMemChartTmpl.Copy(),
	}
	clusterPerfChartsTmpl = collectorapi.Charts{
		clusterCPUUtilizationChartTmpl.Copy(),
		clusterCPUUsageChartTmpl.Copy(),
		clusterMemUtilizationChartTmpl.Copy(),
		clusterMemUsageChartTmpl.Copy(),
		clusterServicesFairnessChartTmpl.Copy(),
		clusterServicesEffectiveCPUChartTmpl.Copy(),
		clusterServicesEffectiveMemChartTmpl.Copy(),
		clusterServicesFailoverChartTmpl.Copy(),
		clusterVMMigrationsChartTmpl.Copy(),
		clusterVMLifecycleChartTmpl.Copy(),
		clusterVMManagementChartTmpl.Copy(),
		clusterVMGuestOpsChartTmpl.Copy(),
		clusterVMColdMigrationsChartTmpl.Copy(),
	}

	// Property charts
	clusterHostsChartTmpl = collectorapi.Chart{
		ID:       "%s_hosts",
		Title:    "Cluster host count",
		Units:    "hosts",
		Fam:      "clusters hosts",
		Ctx:      "vsphere.cluster_hosts",
		Priority: prioClusterHosts,
		Dims: collectorapi.Dims{
			{ID: "%s_num_hosts", Name: "total"},
			{ID: "%s_num_effective_hosts", Name: "effective"},
		},
	}
	clusterCPUCapacityChartTmpl = collectorapi.Chart{
		ID:       "%s_cpu_capacity",
		Title:    "Cluster CPU capacity",
		Units:    "MHz",
		Fam:      "clusters cpu",
		Ctx:      "vsphere.cluster_cpu_capacity",
		Priority: prioClusterCPUCapacity,
		Dims: collectorapi.Dims{
			{ID: "%s_total_cpu", Name: "total"},
			{ID: "%s_effective_cpu", Name: "effective"},
		},
	}
	clusterMemCapacityChartTmpl = collectorapi.Chart{
		ID:       "%s_mem_capacity",
		Title:    "Cluster memory capacity",
		Units:    "bytes",
		Fam:      "clusters mem",
		Ctx:      "vsphere.cluster_mem_capacity",
		Priority: prioClusterMemCapacity,
		Dims: collectorapi.Dims{
			{ID: "%s_total_memory", Name: "total"},
			{ID: "%s_effective_memory", Name: "effective"},
		},
	}
	clusterCPUTopologyChartTmpl = collectorapi.Chart{
		ID:       "%s_cpu_topology",
		Title:    "Cluster CPU topology",
		Units:    "count",
		Fam:      "clusters cpu",
		Ctx:      "vsphere.cluster_cpu_topology",
		Priority: prioClusterCPUTopology,
		Dims: collectorapi.Dims{
			{ID: "%s_num_cpu_cores", Name: "cores"},
			{ID: "%s_num_cpu_threads", Name: "threads"},
		},
	}
	clusterDRSConfigChartTmpl = collectorapi.Chart{
		ID:       "%s_drs_config",
		Title:    "Cluster DRS enabled",
		Units:    "status",
		Fam:      "clusters config",
		Ctx:      "vsphere.cluster_drs_config",
		Priority: prioClusterDRSConfig,
		Dims: collectorapi.Dims{
			{ID: "%s_drs_enabled", Name: "enabled"},
		},
	}
	clusterHAConfigChartTmpl = collectorapi.Chart{
		ID:       "%s_ha_config",
		Title:    "Cluster HA configuration",
		Units:    "status",
		Fam:      "clusters config",
		Ctx:      "vsphere.cluster_ha_config",
		Priority: prioClusterHAConfig,
		Dims: collectorapi.Dims{
			{ID: "%s_ha_enabled", Name: "enabled"},
			{ID: "%s_ha_adm_ctrl_enabled", Name: "admission_control"},
		},
	}
	clusterOverallStatusChartTmpl = collectorapi.Chart{
		ID:       "%s_overall_status",
		Title:    "Cluster overall alarm status",
		Units:    "status",
		Fam:      "clusters status",
		Ctx:      "vsphere.cluster_overall_status",
		Priority: prioClusterOverallStatus,
		Dims: collectorapi.Dims{
			{ID: "%s_overall.status.green", Name: "green"},
			{ID: "%s_overall.status.red", Name: "red"},
			{ID: "%s_overall.status.yellow", Name: "yellow"},
			{ID: "%s_overall.status.gray", Name: "gray"},
		},
	}
	clusterVMotionsChartTmpl = collectorapi.Chart{
		ID:       "%s_vmotions",
		Title:    "Cluster cumulative vMotion count",
		Units:    "migrations",
		Fam:      "clusters migrations",
		Ctx:      "vsphere.cluster_vmotions",
		Type:     collectorapi.Line,
		Priority: prioClusterVMotions,
		Dims: collectorapi.Dims{
			{ID: "%s_num_vmotions", Name: "vmotions", Algo: collectorapi.Incremental},
		},
	}
	clusterDRSScoreChartTmpl = collectorapi.Chart{
		ID:       "%s_drs_score",
		Title:    "Cluster DRS score",
		Units:    "percentage",
		Fam:      "clusters drs",
		Ctx:      "vsphere.cluster_drs_score",
		Priority: prioClusterDRSScore,
		Dims: collectorapi.Dims{
			{ID: "%s_drs_score", Name: "score"},
		},
	}
	clusterDRSBalanceChartTmpl = collectorapi.Chart{
		ID:       "%s_drs_balance",
		Title:    "Cluster DRS load balance",
		Units:    "score",
		Fam:      "clusters drs",
		Ctx:      "vsphere.cluster_drs_balance",
		Priority: prioClusterDRSBalance,
		Dims: collectorapi.Dims{
			{ID: "%s_current_balance", Name: "current", Div: 1000},
			{ID: "%s_target_balance", Name: "target", Div: 1000},
		},
	}
	clusterVMCountChartTmpl = collectorapi.Chart{
		ID:       "%s_vm_count",
		Title:    "Cluster VM count",
		Units:    "VMs",
		Fam:      "clusters vms",
		Ctx:      "vsphere.cluster_vm_count",
		Priority: prioClusterVMCount,
		Dims: collectorapi.Dims{
			{ID: "%s_usage_total_vm_count", Name: "total"},
			{ID: "%s_usage_powered_off_vm_count", Name: "powered_off"},
		},
	}
	clusterUsageCPUChartTmpl = collectorapi.Chart{
		ID:       "%s_usage_cpu",
		Title:    "Cluster DRS CPU usage summary",
		Units:    "MHz",
		Fam:      "clusters cpu",
		Ctx:      "vsphere.cluster_usage_cpu",
		Priority: prioClusterUsageCPU,
		Dims: collectorapi.Dims{
			{ID: "%s_usage_cpu_demand_mhz", Name: "demand"},
			{ID: "%s_usage_cpu_entitled_mhz", Name: "entitled"},
			{ID: "%s_usage_cpu_reservation_mhz", Name: "reserved"},
		},
	}
	clusterUsageMemChartTmpl = collectorapi.Chart{
		ID:       "%s_usage_mem",
		Title:    "Cluster DRS memory usage summary",
		Units:    "MB",
		Fam:      "clusters mem",
		Ctx:      "vsphere.cluster_usage_mem",
		Priority: prioClusterUsageMem,
		Dims: collectorapi.Dims{
			{ID: "%s_usage_mem_demand_mb", Name: "demand"},
			{ID: "%s_usage_mem_entitled_mb", Name: "entitled"},
			{ID: "%s_usage_mem_reservation_mb", Name: "reserved"},
		},
	}

	// Perf charts (created only when perf data arrives)
	clusterCPUUtilizationChartTmpl = collectorapi.Chart{
		ID:       "%s_cpu_utilization",
		Title:    "Cluster CPU utilization",
		Units:    "percentage",
		Fam:      "clusters cpu",
		Ctx:      "vsphere.cluster_cpu_utilization",
		Priority: prioClusterCPUUtilization,
		Dims: collectorapi.Dims{
			{ID: "%s_cpu.usage.average", Name: "used", Div: 100},
		},
	}
	clusterCPUUsageChartTmpl = collectorapi.Chart{
		ID:       "%s_cpu_usage_mhz",
		Title:    "Cluster CPU usage",
		Units:    "MHz",
		Fam:      "clusters cpu",
		Ctx:      "vsphere.cluster_cpu_usage",
		Priority: prioClusterCPUUsage,
		Dims: collectorapi.Dims{
			{ID: "%s_cpu.usagemhz.average", Name: "used"},
			{ID: "%s_cpu.totalmhz.average", Name: "total"},
		},
	}
	clusterMemUtilizationChartTmpl = collectorapi.Chart{
		ID:       "%s_mem_utilization",
		Title:    "Cluster memory utilization",
		Units:    "percentage",
		Fam:      "clusters mem",
		Ctx:      "vsphere.cluster_mem_utilization",
		Priority: prioClusterMemUtilization,
		Dims: collectorapi.Dims{
			{ID: "%s_mem.usage.average", Name: "used", Div: 100},
		},
	}
	clusterMemUsageChartTmpl = collectorapi.Chart{
		ID:       "%s_mem_usage",
		Title:    "Cluster memory usage",
		Units:    "KiB",
		Fam:      "clusters mem",
		Ctx:      "vsphere.cluster_mem_usage",
		Priority: prioClusterMemUsage,
		Dims: collectorapi.Dims{
			{ID: "%s_mem.consumed.average", Name: "consumed"},
			{ID: "%s_mem.active.average", Name: "active"},
			{ID: "%s_mem.granted.average", Name: "granted"},
			{ID: "%s_mem.shared.average", Name: "shared"},
			{ID: "%s_mem.overhead.average", Name: "overhead"},
			{ID: "%s_mem.swapused.average", Name: "swap_used"},
		},
	}
	clusterServicesFairnessChartTmpl = collectorapi.Chart{
		ID:       "%s_services_fairness",
		Title:    "Cluster DRS resource distribution fairness",
		Units:    "score",
		Fam:      "clusters drs",
		Ctx:      "vsphere.cluster_services_fairness",
		Priority: prioClusterServicesFairness,
		Dims: collectorapi.Dims{
			{ID: "%s_clusterServices.cpufairness.latest", Name: "cpu"},
			{ID: "%s_clusterServices.memfairness.latest", Name: "memory"},
		},
	}
	clusterServicesEffectiveCPUChartTmpl = collectorapi.Chart{
		ID:       "%s_services_effective_cpu",
		Title:    "Cluster effective CPU capacity",
		Units:    "MHz",
		Fam:      "clusters cpu",
		Ctx:      "vsphere.cluster_services_effective_cpu",
		Priority: prioClusterServicesEffectiveCPU,
		Dims: collectorapi.Dims{
			{ID: "%s_clusterServices.effectivecpu.average", Name: "effective_cpu"},
		},
	}
	clusterServicesEffectiveMemChartTmpl = collectorapi.Chart{
		ID:       "%s_services_effective_mem",
		Title:    "Cluster effective memory capacity",
		Units:    "MB",
		Fam:      "clusters mem",
		Ctx:      "vsphere.cluster_services_effective_mem",
		Priority: prioClusterServicesEffectiveMem,
		Dims: collectorapi.Dims{
			{ID: "%s_clusterServices.effectivemem.average", Name: "effective_mem"},
		},
	}
	clusterServicesFailoverChartTmpl = collectorapi.Chart{
		ID:       "%s_services_failover",
		Title:    "Cluster HA failover capacity",
		Units:    "failures",
		Fam:      "clusters ha",
		Ctx:      "vsphere.cluster_services_failover",
		Priority: prioClusterServicesFailover,
		Dims: collectorapi.Dims{
			{ID: "%s_clusterServices.failover.latest", Name: "failures_tolerable"},
		},
	}
	clusterVMMigrationsChartTmpl = collectorapi.Chart{
		ID:       "%s_vm_migrations",
		Title:    "Cluster VM migration operations",
		Units:    "operations",
		Fam:      "clusters vmop",
		Ctx:      "vsphere.cluster_vm_migrations",
		Priority: prioClusterVMMigrations,
		Dims: collectorapi.Dims{
			{ID: "%s_vmop.numVMotion.latest", Name: "vmotion"},
			{ID: "%s_vmop.numSVMotion.latest", Name: "svmotion"},
			{ID: "%s_vmop.numXVMotion.latest", Name: "xvmotion"},
		},
	}
	clusterVMLifecycleChartTmpl = collectorapi.Chart{
		ID:       "%s_vm_lifecycle",
		Title:    "Cluster VM lifecycle operations",
		Units:    "operations",
		Fam:      "clusters vmop",
		Ctx:      "vsphere.cluster_vm_lifecycle",
		Priority: prioClusterVMLifecycle,
		Dims: collectorapi.Dims{
			{ID: "%s_vmop.numPoweron.latest", Name: "poweron"},
			{ID: "%s_vmop.numPoweroff.latest", Name: "poweroff"},
			{ID: "%s_vmop.numCreate.latest", Name: "create"},
			{ID: "%s_vmop.numDestroy.latest", Name: "destroy"},
			{ID: "%s_vmop.numClone.latest", Name: "clone"},
			{ID: "%s_vmop.numDeploy.latest", Name: "deploy"},
		},
	}
	clusterVMManagementChartTmpl = collectorapi.Chart{
		ID:       "%s_vm_management",
		Title:    "Cluster VM management operations",
		Units:    "operations",
		Fam:      "clusters vmop",
		Ctx:      "vsphere.cluster_vm_management",
		Priority: prioClusterVMManagement,
		Dims: collectorapi.Dims{
			{ID: "%s_vmop.numReconfigure.latest", Name: "reconfigure"},
			{ID: "%s_vmop.numReset.latest", Name: "reset"},
			{ID: "%s_vmop.numSuspend.latest", Name: "suspend"},
			{ID: "%s_vmop.numRegister.latest", Name: "register"},
			{ID: "%s_vmop.numUnregister.latest", Name: "unregister"},
		},
	}
	clusterVMGuestOpsChartTmpl = collectorapi.Chart{
		ID:       "%s_vm_guest_ops",
		Title:    "Cluster VM guest operations",
		Units:    "operations",
		Fam:      "clusters vmop",
		Ctx:      "vsphere.cluster_vm_guest_ops",
		Priority: prioClusterVMGuestOps,
		Dims: collectorapi.Dims{
			{ID: "%s_vmop.numRebootGuest.latest", Name: "reboot"},
			{ID: "%s_vmop.numShutdownGuest.latest", Name: "shutdown"},
			{ID: "%s_vmop.numStandbyGuest.latest", Name: "standby"},
		},
	}
	clusterVMColdMigrationsChartTmpl = collectorapi.Chart{
		ID:       "%s_vm_cold_migrations",
		Title:    "Cluster VM cold migration operations",
		Units:    "operations",
		Fam:      "clusters vmop",
		Ctx:      "vsphere.cluster_vm_cold_migrations",
		Priority: prioClusterVMColdMigrations,
		Dims: collectorapi.Dims{
			{ID: "%s_vmop.numChangeDS.latest", Name: "change_ds"},
			{ID: "%s_vmop.numChangeHost.latest", Name: "change_host"},
			{ID: "%s_vmop.numChangeHostDS.latest", Name: "change_host_ds"},
		},
	}
)

// --- Resource Pool chart templates ---

var (
	resourcePoolChartsTmpl = collectorapi.Charts{
		rpCPUUsageChartTmpl.Copy(),
		rpCPUEntitlementChartTmpl.Copy(),
		rpCPUAllocationChartTmpl.Copy(),
		rpMemUsageChartTmpl.Copy(),
		rpMemEntitlementChartTmpl.Copy(),
		rpMemAllocationChartTmpl.Copy(),
		rpMemBreakdownChartTmpl.Copy(),
		rpCPUConfigChartTmpl.Copy(),
		rpMemConfigChartTmpl.Copy(),
		rpOverallStatusChartTmpl.Copy(),
	}

	rpCPUUsageChartTmpl = collectorapi.Chart{
		ID:       "%s_cpu_usage",
		Title:    "Resource Pool CPU usage vs demand",
		Units:    "MHz",
		Fam:      "resource pools cpu",
		Ctx:      "vsphere.resource_pool_cpu_usage",
		Priority: prioResourcePoolCPUUsage,
		Dims: collectorapi.Dims{
			{ID: "%s_cpu_usage", Name: "usage"},
			{ID: "%s_cpu_demand", Name: "demand"},
		},
	}
	rpCPUEntitlementChartTmpl = collectorapi.Chart{
		ID:       "%s_cpu_entitlement",
		Title:    "Resource Pool CPU entitlement",
		Units:    "MHz",
		Fam:      "resource pools cpu",
		Ctx:      "vsphere.resource_pool_cpu_entitlement",
		Priority: prioResourcePoolCPUEntitlement,
		Dims: collectorapi.Dims{
			{ID: "%s_cpu_entitlement_distributed", Name: "distributed"},
		},
	}
	rpCPUAllocationChartTmpl = collectorapi.Chart{
		ID:       "%s_cpu_allocation",
		Title:    "Resource Pool CPU allocation",
		Units:    "MHz",
		Fam:      "resource pools cpu",
		Ctx:      "vsphere.resource_pool_cpu_allocation",
		Priority: prioResourcePoolCPUAllocation,
		Dims: collectorapi.Dims{
			{ID: "%s_cpu_reservation_used", Name: "reservation_used"},
			{ID: "%s_cpu_unreserved_for_vm", Name: "unreserved_for_vm"},
			{ID: "%s_cpu_max_usage", Name: "max_usage"},
		},
	}
	rpMemUsageChartTmpl = collectorapi.Chart{
		ID:       "%s_mem_usage",
		Title:    "Resource Pool memory usage",
		Units:    "MB",
		Fam:      "resource pools mem",
		Ctx:      "vsphere.resource_pool_mem_usage",
		Priority: prioResourcePoolMemUsage,
		Dims: collectorapi.Dims{
			{ID: "%s_mem_usage_host", Name: "host"},
			{ID: "%s_mem_usage_guest", Name: "guest"},
		},
	}
	rpMemEntitlementChartTmpl = collectorapi.Chart{
		ID:       "%s_mem_entitlement",
		Title:    "Resource Pool memory entitlement",
		Units:    "MB",
		Fam:      "resource pools mem",
		Ctx:      "vsphere.resource_pool_mem_entitlement",
		Priority: prioResourcePoolMemEntitlement,
		Dims: collectorapi.Dims{
			{ID: "%s_mem_entitlement_distributed", Name: "distributed"},
		},
	}
	rpMemAllocationChartTmpl = collectorapi.Chart{
		ID:       "%s_mem_allocation",
		Title:    "Resource Pool memory allocation",
		Units:    "bytes",
		Fam:      "resource pools mem",
		Ctx:      "vsphere.resource_pool_mem_allocation",
		Priority: prioResourcePoolMemAllocation,
		Dims: collectorapi.Dims{
			{ID: "%s_mem_reservation_used", Name: "reservation_used"},
			{ID: "%s_mem_unreserved_for_vm", Name: "unreserved_for_vm"},
			{ID: "%s_mem_max_usage", Name: "max_usage"},
		},
	}
	rpMemBreakdownChartTmpl = collectorapi.Chart{
		ID:       "%s_mem_breakdown",
		Title:    "Resource Pool memory state breakdown",
		Units:    "MB",
		Fam:      "resource pools mem",
		Ctx:      "vsphere.resource_pool_mem_breakdown",
		Priority: prioResourcePoolMemBreakdown,
		Dims: collectorapi.Dims{
			{ID: "%s_mem_private", Name: "private"},
			{ID: "%s_mem_shared", Name: "shared"},
			{ID: "%s_mem_swapped", Name: "swapped"},
			{ID: "%s_mem_ballooned", Name: "ballooned"},
			{ID: "%s_mem_overhead", Name: "overhead"},
			{ID: "%s_mem_consumed_overhead", Name: "consumed_overhead"},
			{ID: "%s_mem_compressed", Name: "compressed", Div: 1024},
		},
	}
	rpCPUConfigChartTmpl = collectorapi.Chart{
		ID:       "%s_cpu_config",
		Title:    "Resource Pool CPU configured reservation and limit",
		Units:    "MHz",
		Fam:      "resource pools cpu",
		Ctx:      "vsphere.resource_pool_cpu_config",
		Priority: prioResourcePoolCPUConfig,
		Dims: collectorapi.Dims{
			{ID: "%s_cpu_reservation", Name: "reservation"},
			{ID: "%s_cpu_limit", Name: "limit"},
		},
	}
	rpMemConfigChartTmpl = collectorapi.Chart{
		ID:       "%s_mem_config",
		Title:    "Resource Pool memory configured reservation and limit",
		Units:    "MB",
		Fam:      "resource pools mem",
		Ctx:      "vsphere.resource_pool_mem_config",
		Priority: prioResourcePoolMemConfig,
		Dims: collectorapi.Dims{
			{ID: "%s_mem_reservation", Name: "reservation"},
			{ID: "%s_mem_limit", Name: "limit"},
		},
	}
	rpOverallStatusChartTmpl = collectorapi.Chart{
		ID:       "%s_overall_status",
		Title:    "Resource Pool overall alarm status",
		Units:    "status",
		Fam:      "resource pools status",
		Ctx:      "vsphere.resource_pool_overall_status",
		Priority: prioResourcePoolOverallStatus,
		Dims: collectorapi.Dims{
			{ID: "%s_overall.status.green", Name: "green"},
			{ID: "%s_overall.status.red", Name: "red"},
			{ID: "%s_overall.status.yellow", Name: "yellow"},
			{ID: "%s_overall.status.gray", Name: "gray"},
		},
	}
)

func newClusterPropertyCharts(cl *rs.Cluster) *collectorapi.Charts {
	charts := clusterPropertyChartsTmpl.Copy()
	applyClusterChartLabels(charts, cl)
	return charts
}

func newClusterPerfCharts(cl *rs.Cluster) *collectorapi.Charts {
	charts := clusterPerfChartsTmpl.Copy()
	applyClusterChartLabels(charts, cl)
	return charts
}

func applyClusterChartLabels(charts *collectorapi.Charts, cl *rs.Cluster) {
	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, cl.ID)
		chart.Labels = []collectorapi.Label{
			{Key: "datacenter", Value: cl.Hier.DC.Name},
			{Key: "cluster", Value: cl.Name},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, cl.ID)
		}
	}
}

func newResourcePoolCharts(rp *rs.ResourcePool) *collectorapi.Charts {
	charts := resourcePoolChartsTmpl.Copy()
	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, rp.ID)
		chart.Labels = []collectorapi.Label{
			{Key: "datacenter", Value: rp.Hier.DC.Name},
			{Key: "cluster", Value: rp.Hier.Cluster.Name},
			{Key: "resource_pool", Value: rp.Name},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, rp.ID)
		}
	}
	return charts
}

func (c *Collector) removeFromCharts(prefix string) {
	for _, c := range *c.Charts() {
		if strings.HasPrefix(c.ID, prefix+"_") {
			c.MarkRemove()
			c.MarkNotCreated()
		}
	}
}

//func findMetricSeriesByPrefix(ms []performance.MetricSeries, prefix string) []performance.MetricSeries {
//	from := sort.Search(len(ms), func(i int) bool { return ms[i].Name >= prefix })
//
//	if from == len(ms) || !strings.HasPrefix(ms[from].Name, prefix) {
//		return nil
//	}
//
//	until := from + 1
//	for until < len(ms) && strings.HasPrefix(ms[until].Name, prefix) {
//		until++
//	}
//	return ms[from:until]
//}
