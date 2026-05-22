// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import (
	"github.com/vmware/govmomi/performance"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	rs "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/resources"
)

const (
	hostPowerUsagePowerMetric = "host_power_usage_power"
	hostPowerUsageCapMetric   = "host_power_usage_cap"

	hostPowerCapacityUsageUsedMetric   = "host_power_capacity_usage_used"
	hostPowerCapacityUsageUsableMetric = "host_power_capacity_usage_usable"
	hostPowerCapacityUsageIdleMetric   = "host_power_capacity_usage_idle"
	hostPowerCapacityUsageSystemMetric = "host_power_capacity_usage_system"
	hostPowerCapacityUsageVMMetric     = "host_power_capacity_usage_vm"

	hostPowerCapacityUtilizationMetric = "host_power_capacity_utilization_used"

	hostEnergyUsageMetric = "host_energy_usage_energy"

	vmPowerUsagePowerMetric = "vm_power_usage_power"

	vmEnergyUsageMetric = "vm_energy_usage_energy"
)

var hostPowerMetricByCounter = map[string]string{
	"power.power.average":                hostPowerUsagePowerMetric,
	"power.powerCap.average":             hostPowerUsageCapMetric,
	"power.capacity.usage.average":       hostPowerCapacityUsageUsedMetric,
	"power.capacity.usable.average":      hostPowerCapacityUsageUsableMetric,
	"power.capacity.usageIdle.average":   hostPowerCapacityUsageIdleMetric,
	"power.capacity.usageSystem.average": hostPowerCapacityUsageSystemMetric,
	"power.capacity.usageVm.average":     hostPowerCapacityUsageVMMetric,
	"power.capacity.usagePct.average":    hostPowerCapacityUtilizationMetric,
	"power.energy.summation":             hostEnergyUsageMetric,
}

var vmPowerMetricByCounter = map[string]string{
	"power.power.average":    vmPowerUsagePowerMetric,
	"power.energy.summation": vmEnergyUsageMetric,
}

type hostPowerPerfSample struct {
	host   *rs.Host
	values map[string]int64
}

type vmPowerPerfSample struct {
	vm     *rs.VM
	values map[string]int64
}

func (c *Collector) collectHostPowerMetrics(host *rs.Host, metrics []performance.MetricSeries) {
	for _, metric := range metrics {
		if len(metric.Value) == 0 || metric.Value[0] == -1 || metric.Instance != "" {
			continue
		}
		metricName, ok := hostPowerMetricByCounter[metric.Name]
		if !ok {
			continue
		}
		if c.hostPowerPerfSamples == nil {
			c.hostPowerPerfSamples = make(map[string]*hostPowerPerfSample)
		}
		sample := c.hostPowerPerfSamples[host.ID]
		if sample == nil {
			sample = &hostPowerPerfSample{
				host:   host,
				values: make(map[string]int64),
			}
			c.hostPowerPerfSamples[host.ID] = sample
		}
		sample.values[metricName] = metric.Value[0]
	}
}

func (c *Collector) collectVMPowerMetrics(vm *rs.VM, metrics []performance.MetricSeries) {
	for _, metric := range metrics {
		if len(metric.Value) == 0 || metric.Value[0] == -1 || metric.Instance != "" {
			continue
		}
		metricName, ok := vmPowerMetricByCounter[metric.Name]
		if !ok {
			continue
		}
		if c.vmPowerPerfSamples == nil {
			c.vmPowerPerfSamples = make(map[string]*vmPowerPerfSample)
		}
		sample := c.vmPowerPerfSamples[vm.ID]
		if sample == nil {
			sample = &vmPowerPerfSample{
				vm:     vm,
				values: make(map[string]int64),
			}
			c.vmPowerPerfSamples[vm.ID] = sample
		}
		sample.values[metricName] = metric.Value[0]
	}
}

func (c *Collector) writePowerMetrics() {
	c.writeHostPowerMetrics()
	c.writeVMPowerMetrics()
}

func (c *Collector) writeHostPowerMetrics() {
	if len(c.hostPowerPerfSamples) == 0 {
		return
	}

	for _, sample := range sortedHostPowerPerfSamples(c.hostPowerPerfSamples) {
		labels := c.labelSet(c.hostPowerLabels(sample.host))
		for metricName, value := range sample.values {
			c.observeGauge(metricName, value, labels)
		}
	}
}

func (c *Collector) writeVMPowerMetrics() {
	if len(c.vmPowerPerfSamples) == 0 {
		return
	}

	for _, sample := range sortedVMPowerPerfSamples(c.vmPowerPerfSamples) {
		labels := c.labelSet(c.vmPowerLabels(sample.vm))
		for metricName, value := range sample.values {
			c.observeGauge(metricName, value, labels)
		}
	}
}

func (c *Collector) hostPowerLabels(host *rs.Host) []metrix.Label {
	return c.v2MetricLabels(host.ID, []metrix.Label{
		{Key: "datacenter", Value: host.Hier.DC.Name},
		{Key: "cluster", Value: getHostClusterName(host)},
		{Key: "host", Value: host.Name},
	}, host.Labels)
}

func (c *Collector) vmPowerLabels(vm *rs.VM) []metrix.Label {
	return c.v2MetricLabels(vm.ID, []metrix.Label{
		{Key: "datacenter", Value: vm.Hier.DC.Name},
		{Key: "cluster", Value: getVMClusterName(vm)},
		{Key: "host", Value: vm.Hier.Host.Name},
		{Key: "vm", Value: vm.Name},
	}, vm.Labels)
}
