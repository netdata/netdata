// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import (
	"sort"

	"github.com/vmware/govmomi/performance"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	rs "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/resources"
)

const (
	vmDiskInstanceLabel = "disk_instance"

	vmDiskDeviceIOContext            = "vsphere.vm_disk_device_io"
	vmDiskDeviceIOReadMetric         = "vm_disk_device_io_read"
	vmDiskDeviceIOWriteMetric        = "vm_disk_device_io_write"
	vmDiskDeviceIOReadDim            = "read"
	vmDiskDeviceIOWriteDim           = "write"
	vmDiskDeviceIOPSContext          = "vsphere.vm_disk_device_iops"
	vmDiskDeviceIOPSReadMetric       = "vm_disk_device_iops_read"
	vmDiskDeviceIOPSWriteMetric      = "vm_disk_device_iops_write"
	vmDiskDeviceIOPSReadDim          = "read"
	vmDiskDeviceIOPSWriteDim         = "write"
	vmDiskDeviceLatencyContext       = "vsphere.vm_disk_device_latency"
	vmDiskDeviceLatencyReadMetric    = "vm_disk_device_latency_read"
	vmDiskDeviceLatencyWriteMetric   = "vm_disk_device_latency_write"
	vmDiskDeviceLatencyReadDim       = "read"
	vmDiskDeviceLatencyWriteDim      = "write"
	vmDiskDeviceOutstandingIOContext = "vsphere.vm_disk_device_outstanding_io"
	vmDiskDeviceOIOReadMetric        = "vm_disk_device_outstanding_io_read"
	vmDiskDeviceOIOWriteMetric       = "vm_disk_device_outstanding_io_write"
	vmDiskDeviceOIOReadDim           = "read"
	vmDiskDeviceOIOWriteDim          = "write"
)

var vmDiskPerfMetricByCounter = map[string]string{
	"virtualDisk.read.average":                vmDiskDeviceIOReadMetric,
	"virtualDisk.write.average":               vmDiskDeviceIOWriteMetric,
	"virtualDisk.numberReadAveraged.average":  vmDiskDeviceIOPSReadMetric,
	"virtualDisk.numberWriteAveraged.average": vmDiskDeviceIOPSWriteMetric,
	"virtualDisk.totalReadLatency.average":    vmDiskDeviceLatencyReadMetric,
	"virtualDisk.totalWriteLatency.average":   vmDiskDeviceLatencyWriteMetric,
	"virtualDisk.readOIO.latest":              vmDiskDeviceOIOReadMetric,
	"virtualDisk.writeOIO.latest":             vmDiskDeviceOIOWriteMetric,
}

type vmDiskPerfSample struct {
	vm       *rs.VM
	instance string
	values   map[string]int64
}

func vmDiskPerformanceOptionalMetricNames() []string {
	return []string{
		vmDiskDeviceIOReadMetric,
		vmDiskDeviceIOWriteMetric,
		vmDiskDeviceIOPSReadMetric,
		vmDiskDeviceIOPSWriteMetric,
		vmDiskDeviceLatencyReadMetric,
		vmDiskDeviceLatencyWriteMetric,
		vmDiskDeviceOIOReadMetric,
		vmDiskDeviceOIOWriteMetric,
	}
}

func isVMDiskPerformanceMetric(name string) bool {
	_, ok := vmDiskPerfMetricByCounter[name]
	return ok
}

func (c *Collector) collectVMDiskPerformanceMetrics(vm *rs.VM, metrics []performance.MetricSeries) {
	for _, metric := range metrics {
		metricName, ok := vmDiskPerfMetricByCounter[metric.Name]
		if !ok || metric.Instance == "" || metric.Instance == "*" || len(metric.Value) == 0 || metric.Value[0] == -1 {
			continue
		}
		key := vm.ID + "\x00" + metric.Instance
		if c.vmDiskPerfSamples == nil {
			c.vmDiskPerfSamples = make(map[string]*vmDiskPerfSample)
		}
		sample := c.vmDiskPerfSamples[key]
		if sample == nil {
			sample = &vmDiskPerfSample{
				vm:       vm,
				instance: metric.Instance,
				values:   make(map[string]int64),
			}
			c.vmDiskPerfSamples[key] = sample
		}
		sample.values[metricName] = metric.Value[0]
	}
}

func (c *Collector) writeVMDiskPerformanceMetrics(meter metrix.SnapshotMeter) {
	if !c.CollectVMDiskPerformance || len(c.vmDiskPerfSamples) == 0 {
		return
	}

	m := c.vmDiskMatcher
	if m == nil {
		m = matcher.TRUE()
	}

	for _, sample := range sortedVMDiskPerfSamples(c.vmDiskPerfSamples) {
		if !vmDiskInstanceMatches(m, sample.instance) {
			continue
		}

		labels := meter.LabelSet(c.vmDiskPerformanceLabels(sample.vm, sample.instance)...)
		for metricName, value := range sample.values {
			if gauge := c.mx.gauge(metricName); gauge != nil {
				gauge.Observe(metrix.SampleValue(value), labels)
			}
		}
	}
}

func (c *Collector) vmDiskPerformanceLabels(vm *rs.VM, instance string) []metrix.Label {
	labels := []metrix.Label{
		{Key: "id", Value: vm.ID},
		{Key: "datacenter", Value: vm.Hier.DC.Name},
		{Key: "cluster", Value: vm.Hier.Cluster.Name},
		{Key: "host", Value: vm.Hier.Host.Name},
		{Key: "vm", Value: vm.Name},
		{Key: vmDiskDisplayNameLabel, Value: instance},
		{Key: vmDiskInstanceLabel, Value: instance},
	}
	labels = append(labels, c.resourceEnrichmentLabels(vm.ID)...)
	return labels
}

func vmDiskInstanceMatches(m matcher.Matcher, instance string) bool {
	return m.MatchString(instance) || m.MatchString("instance:"+instance)
}

func sortedVMDiskPerfSamples(samples map[string]*vmDiskPerfSample) []*vmDiskPerfSample {
	out := make([]*vmDiskPerfSample, 0, len(samples))
	for _, sample := range samples {
		out = append(out, sample)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].vm.ID != out[j].vm.ID {
			return out[i].vm.ID < out[j].vm.ID
		}
		return out[i].instance < out[j].instance
	})
	return out
}
