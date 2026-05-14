// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import (
	"sort"

	"github.com/vmware/govmomi/performance"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	rs "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/resources"
)

const (
	hostPowerUsageContext     = "vsphere.host_power_usage"
	hostPowerUsagePowerMetric = "host_power_usage_power"
	hostPowerUsageCapMetric   = "host_power_usage_cap"
	hostPowerUsagePowerDim    = "power"
	hostPowerUsageCapDim      = "cap"

	hostPowerCapacityUsageContext      = "vsphere.host_power_capacity_usage"
	hostPowerCapacityUsageUsedMetric   = "host_power_capacity_usage_used"
	hostPowerCapacityUsageUsableMetric = "host_power_capacity_usage_usable"
	hostPowerCapacityUsageIdleMetric   = "host_power_capacity_usage_idle"
	hostPowerCapacityUsageSystemMetric = "host_power_capacity_usage_system"
	hostPowerCapacityUsageVMMetric     = "host_power_capacity_usage_vm"
	hostPowerCapacityUsageUsedDim      = "used"
	hostPowerCapacityUsageUsableDim    = "usable"
	hostPowerCapacityUsageIdleDim      = "idle"
	hostPowerCapacityUsageSystemDim    = "system"
	hostPowerCapacityUsageVMDim        = "vm"

	hostPowerCapacityUtilizationContext = "vsphere.host_power_capacity_utilization"
	hostPowerCapacityUtilizationMetric  = "host_power_capacity_utilization_used"
	hostPowerCapacityUtilizationDim     = "used"

	hostEnergyUsageContext = "vsphere.host_energy_usage"
	hostEnergyUsageMetric  = "host_energy_usage_energy"
	hostEnergyUsageDim     = "energy"

	vmPowerUsageContext     = "vsphere.vm_power_usage"
	vmPowerUsagePowerMetric = "vm_power_usage_power"
	vmPowerUsagePowerDim    = "power"

	vmEnergyUsageContext = "vsphere.vm_energy_usage"
	vmEnergyUsageMetric  = "vm_energy_usage_energy"
	vmEnergyUsageDim     = "energy"
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

func powerMetricsOptionalMetricNames() []string {
	return []string{
		hostPowerUsagePowerMetric,
		hostPowerUsageCapMetric,
		hostPowerCapacityUsageUsedMetric,
		hostPowerCapacityUsageUsableMetric,
		hostPowerCapacityUsageIdleMetric,
		hostPowerCapacityUsageSystemMetric,
		hostPowerCapacityUsageVMMetric,
		hostPowerCapacityUtilizationMetric,
		hostEnergyUsageMetric,
		vmPowerUsagePowerMetric,
		vmEnergyUsageMetric,
	}
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

func (c *Collector) writePowerMetrics(meter metrix.SnapshotMeter) {
	if !c.CollectPowerMetrics {
		return
	}
	c.writeHostPowerMetrics(meter)
	c.writeVMPowerMetrics(meter)
}

func (c *Collector) writeHostPowerMetrics(meter metrix.SnapshotMeter) {
	if len(c.hostPowerPerfSamples) == 0 {
		return
	}

	for _, sample := range sortedHostPowerPerfSamples(c.hostPowerPerfSamples) {
		scope := c.resourceHostScope(sample.host.ID)
		writeMeter := meter
		scoped := !scope.IsDefault()
		if scoped {
			writeMeter = meter.WithHostScope(scope)
		}
		labels := writeMeter.LabelSet(c.hostPowerLabels(sample.host)...)
		for metricName, value := range sample.values {
			if gauge := c.mx.gauge(writeMeter, metricName, scoped); gauge != nil {
				gauge.Observe(metrix.SampleValue(value), labels)
			}
		}
	}
}

func (c *Collector) writeVMPowerMetrics(meter metrix.SnapshotMeter) {
	if len(c.vmPowerPerfSamples) == 0 {
		return
	}

	for _, sample := range sortedVMPowerPerfSamples(c.vmPowerPerfSamples) {
		scope := c.resourceHostScope(sample.vm.ID)
		writeMeter := meter
		scoped := !scope.IsDefault()
		if scoped {
			writeMeter = meter.WithHostScope(scope)
		}
		labels := writeMeter.LabelSet(c.vmPowerLabels(sample.vm)...)
		for metricName, value := range sample.values {
			if gauge := c.mx.gauge(writeMeter, metricName, scoped); gauge != nil {
				gauge.Observe(metrix.SampleValue(value), labels)
			}
		}
	}
}

func (c *Collector) hostPowerLabels(host *rs.Host) []metrix.Label {
	labels := []metrix.Label{
		{Key: "id", Value: host.ID},
		{Key: "datacenter", Value: host.Hier.DC.Name},
		{Key: "cluster", Value: host.Hier.Cluster.Name},
		{Key: "host", Value: host.Name},
	}
	labels = append(labels, c.resourceEnrichmentLabels(host.ID)...)
	return labels
}

func (c *Collector) vmPowerLabels(vm *rs.VM) []metrix.Label {
	labels := []metrix.Label{
		{Key: "id", Value: vm.ID},
		{Key: "datacenter", Value: vm.Hier.DC.Name},
		{Key: "cluster", Value: vm.Hier.Cluster.Name},
		{Key: "host", Value: vm.Hier.Host.Name},
		{Key: "vm", Value: vm.Name},
	}
	labels = append(labels, c.resourceEnrichmentLabels(vm.ID)...)
	return labels
}

func sortedHostPowerPerfSamples(samples map[string]*hostPowerPerfSample) []*hostPowerPerfSample {
	out := make([]*hostPowerPerfSample, 0, len(samples))
	for _, sample := range samples {
		out = append(out, sample)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].host.ID < out[j].host.ID
	})
	return out
}

func sortedVMPowerPerfSamples(samples map[string]*vmPowerPerfSample) []*vmPowerPerfSample {
	out := make([]*vmPowerPerfSample, 0, len(samples))
	for _, sample := range samples {
		out = append(out, sample)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].vm.ID < out[j].vm.ID
	})
	return out
}
