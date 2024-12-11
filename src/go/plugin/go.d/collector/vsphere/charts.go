// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	rs "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/resources"
)

const (
	prioVMCPUUtilization = module.Priority + iota
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
	vmChartsTmpl = module.Charts{
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

	vmCPUUtilizationChartTmpl = module.Chart{
		ID:       "%s_cpu_utilization",
		Title:    "Virtual Machine CPU utilization",
		Units:    "percentage",
		Fam:      "vms cpu",
		Ctx:      "vsphere.vm_cpu_utilization",
		Priority: prioVMCPUUtilization,
		Dims: module.Dims{
			{ID: "%s_cpu.usage.average", Name: "used", Div: 100},
		},
	}

	// Ref: https://www.vmware.com/support/developer/converter-sdk/conv51_apireference/memory_counters.html
	vmMemoryUtilizationChartTmpl = module.Chart{
		ID:       "%s_mem_utilization",
		Title:    "Virtual Machine memory utilization",
		Units:    "percentage",
		Fam:      "vms mem",
		Ctx:      "vsphere.vm_mem_utilization",
		Priority: prioVmMemoryUtilization,
		Dims: module.Dims{
			{ID: "%s_mem.usage.average", Name: "used", Div: 100},
		},
	}
	vmMemoryUsageChartTmpl = module.Chart{
		ID:       "%s_mem_usage",
		Title:    "Virtual Machine memory usage",
		Units:    "KiB",
		Fam:      "vms mem",
		Ctx:      "vsphere.vm_mem_usage",
		Priority: prioVmMemoryUsage,
		Dims: module.Dims{
			{ID: "%s_mem.granted.average", Name: "granted"},
			{ID: "%s_mem.consumed.average", Name: "consumed"},
			{ID: "%s_mem.active.average", Name: "active"},
			{ID: "%s_mem.shared.average", Name: "shared"},
		},
	}
	vmMemorySwapUsageChartTmpl = module.Chart{
		ID:       "%s_mem_swap_usage",
		Title:    "Virtual Machine VMKernel memory swap usage",
		Units:    "KiB",
		Fam:      "vms mem",
		Ctx:      "vsphere.vm_mem_swap_usage",
		Priority: prioVmMemorySwapUsage,
		Dims: module.Dims{
			{ID: "%s_mem.swapped.average", Name: "swapped"},
		},
	}
	vmMemorySwapIOChartTmpl = module.Chart{
		ID:       "%s_mem_swap_io_rate",
		Title:    "Virtual Machine VMKernel memory swap IO",
		Units:    "KiB/s",
		Fam:      "vms mem",
		Ctx:      "vsphere.vm_mem_swap_io",
		Type:     module.Area,
		Priority: prioVmMemorySwapIO,
		Dims: module.Dims{
			{ID: "%s_mem.swapinRate.average", Name: "in"},
			{ID: "%s_mem.swapoutRate.average", Name: "out"},
		},
	}

	vmDiskIOChartTmpl = module.Chart{
		ID:       "%s_disk_io",
		Title:    "Virtual Machine disk IO",
		Units:    "KiB/s",
		Fam:      "vms disk",
		Ctx:      "vsphere.vm_disk_io",
		Type:     module.Area,
		Priority: prioVmDiskIO,
		Dims: module.Dims{
			{ID: "%s_disk.read.average", Name: "read"},
			{ID: "%s_disk.write.average", Name: "write", Mul: -1},
		},
	}
	vmDiskMaxLatencyChartTmpl = module.Chart{
		ID:       "%s_disk_max_latency",
		Title:    "Virtual Machine disk max latency",
		Units:    "milliseconds",
		Fam:      "vms disk",
		Ctx:      "vsphere.vm_disk_max_latency",
		Priority: prioVmDiskMaxLatency,
		Dims: module.Dims{
			{ID: "%s_disk.maxTotalLatency.latest", Name: "latency"},
		},
	}

	vmNetworkTrafficChartTmpl = module.Chart{
		ID:       "%s_net_traffic",
		Title:    "Virtual Machine network traffic",
		Units:    "KiB/s",
		Fam:      "vms net",
		Ctx:      "vsphere.vm_net_traffic",
		Type:     module.Area,
		Priority: prioVmNetworkTraffic,
		Dims: module.Dims{
			{ID: "%s_net.bytesRx.average", Name: "received"},
			{ID: "%s_net.bytesTx.average", Name: "sent", Mul: -1},
		},
	}
	vmNetworkPacketsChartTmpl = module.Chart{
		ID:       "%s_net_packets",
		Title:    "Virtual Machine network packets",
		Units:    "packets",
		Fam:      "vms net",
		Ctx:      "vsphere.vm_net_packets",
		Priority: prioVmNetworkPackets,
		Dims: module.Dims{
			{ID: "%s_net.packetsRx.summation", Name: "received"},
			{ID: "%s_net.packetsTx.summation", Name: "sent", Mul: -1},
		},
	}
	vmNetworkDropsChartTmpl = module.Chart{
		ID:       "%s_net_drops",
		Title:    "Virtual Machine network dropped packets",
		Units:    "drops",
		Fam:      "vms net",
		Ctx:      "vsphere.vm_net_drops",
		Priority: prioVmNetworkDrops,
		Dims: module.Dims{
			{ID: "%s_net.droppedRx.summation", Name: "received"},
			{ID: "%s_net.droppedTx.summation", Name: "sent", Mul: -1},
		},
	}

	vmOverallStatusChartTmpl = module.Chart{
		ID:       "%s_overall_status",
		Title:    "Virtual Machine overall alarm status",
		Units:    "status",
		Fam:      "vms status",
		Ctx:      "vsphere.vm_overall_status",
		Priority: prioVmOverallStatus,
		Dims: module.Dims{
			{ID: "%s_overall.status.green", Name: "green"},
			{ID: "%s_overall.status.red", Name: "red"},
			{ID: "%s_overall.status.yellow", Name: "yellow"},
			{ID: "%s_overall.status.gray", Name: "gray"},
		},
	}

	vmSystemUptimeChartTmpl = module.Chart{
		ID:       "%s_system_uptime",
		Title:    "Virtual Machine system uptime",
		Units:    "seconds",
		Fam:      "vms uptime",
		Ctx:      "vsphere.vm_system_uptime",
		Priority: prioVmSystemUptime,
		Dims: module.Dims{
			{ID: "%s_sys.uptime.latest", Name: "uptime"},
		},
	}
)

var (
	hostChartsTmpl = module.Charts{
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
	hostCPUUtilizationChartTmpl = module.Chart{
		ID:       "%s_cpu_usage_total",
		Title:    "ESXi Host CPU utilization",
		Units:    "percentage",
		Fam:      "hosts cpu",
		Ctx:      "vsphere.host_cpu_utilization",
		Priority: prioHostCPUUtilization,
		Dims: module.Dims{
			{ID: "%s_cpu.usage.average", Name: "used", Div: 100},
		},
	}
	hostMemUtilizationChartTmpl = module.Chart{
		ID:       "%s_mem_utilization",
		Title:    "ESXi Host memory utilization",
		Units:    "percentage",
		Fam:      "hosts mem",
		Ctx:      "vsphere.host_mem_utilization",
		Priority: prioHostMemoryUtilization,
		Dims: module.Dims{
			{ID: "%s_mem.usage.average", Name: "used", Div: 100},
		},
	}
	hostMemUsageChartTmpl = module.Chart{
		ID:       "%s_mem_usage",
		Title:    "ESXi Host memory usage",
		Units:    "KiB",
		Fam:      "hosts mem",
		Ctx:      "vsphere.host_mem_usage",
		Priority: prioHostMemoryUsage,
		Dims: module.Dims{
			{ID: "%s_mem.granted.average", Name: "granted"},
			{ID: "%s_mem.consumed.average", Name: "consumed"},
			{ID: "%s_mem.active.average", Name: "active"},
			{ID: "%s_mem.shared.average", Name: "shared"},
			{ID: "%s_mem.sharedcommon.average", Name: "sharedcommon"},
		},
	}
	hostMemSwapIOChartTmpl = module.Chart{
		ID:       "%s_mem_swap_rate",
		Title:    "ESXi Host VMKernel memory swap IO",
		Units:    "KiB/s",
		Fam:      "hosts mem",
		Ctx:      "vsphere.host_mem_swap_io",
		Type:     module.Area,
		Priority: prioHostMemorySwapIO,
		Dims: module.Dims{
			{ID: "%s_mem.swapinRate.average", Name: "in"},
			{ID: "%s_mem.swapoutRate.average", Name: "out"},
		},
	}

	hostDiskIOChartTmpl = module.Chart{
		ID:       "%s_disk_io",
		Title:    "ESXi Host disk IO",
		Units:    "KiB/s",
		Fam:      "hosts disk",
		Ctx:      "vsphere.host_disk_io",
		Type:     module.Area,
		Priority: prioHostDiskIO,
		Dims: module.Dims{
			{ID: "%s_disk.read.average", Name: "read"},
			{ID: "%s_disk.write.average", Name: "write", Mul: -1},
		},
	}
	hostDiskMaxLatencyChartTmpl = module.Chart{
		ID:       "%s_disk_max_latency",
		Title:    "ESXi Host disk max latency",
		Units:    "milliseconds",
		Fam:      "hosts disk",
		Ctx:      "vsphere.host_disk_max_latency",
		Priority: prioHostDiskMaxLatency,
		Dims: module.Dims{
			{ID: "%s_disk.maxTotalLatency.latest", Name: "latency"},
		},
	}

	hostNetworkTraffic = module.Chart{
		ID:       "%s_net_traffic",
		Title:    "ESXi Host network traffic",
		Units:    "KiB/s",
		Fam:      "hosts net",
		Ctx:      "vsphere.host_net_traffic",
		Type:     module.Area,
		Priority: prioHostNetworkTraffic,
		Dims: module.Dims{
			{ID: "%s_net.bytesRx.average", Name: "received"},
			{ID: "%s_net.bytesTx.average", Name: "sent", Mul: -1},
		},
	}
	hostNetworkPacketsChartTmpl = module.Chart{
		ID:       "%s_net_packets",
		Title:    "ESXi Host network packets",
		Units:    "packets",
		Fam:      "hosts net",
		Ctx:      "vsphere.host_net_packets",
		Priority: prioHostNetworkPackets,
		Dims: module.Dims{
			{ID: "%s_net.packetsRx.summation", Name: "received"},
			{ID: "%s_net.packetsTx.summation", Name: "sent", Mul: -1},
		},
	}
	hostNetworkDropsChartTmpl = module.Chart{
		ID:       "%s_net_drops_total",
		Title:    "ESXi Host network drops",
		Units:    "drops",
		Fam:      "hosts net",
		Ctx:      "vsphere.host_net_drops",
		Priority: prioHostNetworkDrops,
		Dims: module.Dims{
			{ID: "%s_net.droppedRx.summation", Name: "received"},
			{ID: "%s_net.droppedTx.summation", Name: "sent", Mul: -1},
		},
	}
	hostNetworkErrorsChartTmpl = module.Chart{
		ID:       "%s_net_errors",
		Title:    "ESXi Host network errors",
		Units:    "errors",
		Fam:      "hosts net",
		Ctx:      "vsphere.host_net_errors",
		Priority: prioHostNetworkErrors,
		Dims: module.Dims{
			{ID: "%s_net.errorsRx.summation", Name: "received"},
			{ID: "%s_net.errorsTx.summation", Name: "sent", Mul: -1},
		},
	}

	hostOverallStatusChartTmpl = module.Chart{
		ID:       "%s_overall_status",
		Title:    "ESXi Host overall alarm status",
		Units:    "status",
		Fam:      "hosts status",
		Ctx:      "vsphere.host_overall_status",
		Priority: prioHostOverallStatus,
		Dims: module.Dims{
			{ID: "%s_overall.status.green", Name: "green"},
			{ID: "%s_overall.status.red", Name: "red"},
			{ID: "%s_overall.status.yellow", Name: "yellow"},
			{ID: "%s_overall.status.gray", Name: "gray"},
		},
	}
	hostSystemUptimeChartTmpl = module.Chart{
		ID:       "%s_system_uptime",
		Title:    "ESXi Host system uptime",
		Units:    "seconds",
		Fam:      "hosts uptime",
		Ctx:      "vsphere.host_system_uptime",
		Priority: prioHostSystemUptime,
		Dims: module.Dims{
			{ID: "%s_sys.uptime.latest", Name: "uptime"},
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
}

func newVMCHarts(vm *rs.VM) *module.Charts {
	charts := vmChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, vm.ID)
		chart.Labels = []module.Label{
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

func newHostCharts(host *rs.Host) *module.Charts {
	charts := hostChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, host.ID)
		chart.Labels = []module.Label{
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

func (c *Collector) removeFromCharts(prefix string) {
	for _, c := range *c.Charts() {
		if strings.HasPrefix(c.ID, prefix) {
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
