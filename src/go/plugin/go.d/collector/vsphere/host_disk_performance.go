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
	defaultMaxHostDisks   = 1024
	hostDiskLabel         = "disk"
	hostDiskInstanceLabel = "disk_instance"

	hostDiskDeviceIOContext     = "vsphere.host_disk_device_io"
	hostDiskDeviceIOReadMetric  = "host_disk_device_io_read"
	hostDiskDeviceIOWriteMetric = "host_disk_device_io_write"
	hostDiskDeviceIOReadDim     = "read"
	hostDiskDeviceIOWriteDim    = "write"

	hostDiskDeviceIOPSContext     = "vsphere.host_disk_device_iops"
	hostDiskDeviceIOPSReadMetric  = "host_disk_device_iops_read"
	hostDiskDeviceIOPSWriteMetric = "host_disk_device_iops_write"
	hostDiskDeviceIOPSReadDim     = "read"
	hostDiskDeviceIOPSWriteDim    = "write"

	hostDiskDeviceRequestsContext     = "vsphere.host_disk_device_requests"
	hostDiskDeviceRequestsReadMetric  = "host_disk_device_requests_read"
	hostDiskDeviceRequestsWriteMetric = "host_disk_device_requests_write"
	hostDiskDeviceRequestsReadDim     = "read"
	hostDiskDeviceRequestsWriteDim    = "write"

	hostDiskDeviceLatencyContext     = "vsphere.host_disk_device_latency"
	hostDiskDeviceLatencyTotalMetric = "host_disk_device_latency_total"
	hostDiskDeviceLatencyReadMetric  = "host_disk_device_latency_read"
	hostDiskDeviceLatencyWriteMetric = "host_disk_device_latency_write"
	hostDiskDeviceLatencyTotalDim    = "total"
	hostDiskDeviceLatencyReadDim     = "read"
	hostDiskDeviceLatencyWriteDim    = "write"

	hostDiskDeviceLatencyBreakdownContext = "vsphere.host_disk_device_latency_breakdown"
	hostDiskDeviceLatencyDeviceMetric     = "host_disk_device_latency_breakdown_device"
	hostDiskDeviceLatencyKernelMetric     = "host_disk_device_latency_breakdown_kernel"
	hostDiskDeviceLatencyQueueMetric      = "host_disk_device_latency_breakdown_queue"
	hostDiskDeviceLatencyDeviceDim        = "device"
	hostDiskDeviceLatencyKernelDim        = "kernel"
	hostDiskDeviceLatencyQueueDim         = "queue"

	hostDiskDeviceReadLatencyBreakdownContext = "vsphere.host_disk_device_read_latency_breakdown"
	hostDiskDeviceReadLatencyDeviceMetric     = "host_disk_device_read_latency_breakdown_device"
	hostDiskDeviceReadLatencyKernelMetric     = "host_disk_device_read_latency_breakdown_kernel"
	hostDiskDeviceReadLatencyQueueMetric      = "host_disk_device_read_latency_breakdown_queue"

	hostDiskDeviceWriteLatencyBreakdownContext = "vsphere.host_disk_device_write_latency_breakdown"
	hostDiskDeviceWriteLatencyDeviceMetric     = "host_disk_device_write_latency_breakdown_device"
	hostDiskDeviceWriteLatencyKernelMetric     = "host_disk_device_write_latency_breakdown_kernel"
	hostDiskDeviceWriteLatencyQueueMetric      = "host_disk_device_write_latency_breakdown_queue"

	hostDiskDeviceCommandsContext = "vsphere.host_disk_device_commands"
	hostDiskDeviceCommandsMetric  = "host_disk_device_commands_issued"
	hostDiskDeviceCommandsDim     = "issued"

	hostDiskDeviceCommandEventsContext         = "vsphere.host_disk_device_command_events"
	hostDiskDeviceCommandEventsIssuedMetric    = "host_disk_device_command_events_issued"
	hostDiskDeviceCommandEventsAbortedMetric   = "host_disk_device_command_events_aborted"
	hostDiskDeviceCommandEventsBusResetsMetric = "host_disk_device_command_events_bus_resets"
	hostDiskDeviceCommandEventsIssuedDim       = "issued"
	hostDiskDeviceCommandEventsAbortedDim      = "aborted"
	hostDiskDeviceCommandEventsBusResetsDim    = "bus_resets"

	hostDiskDeviceQueueDepthContext = "vsphere.host_disk_device_queue_depth"
	hostDiskDeviceQueueDepthMetric  = "host_disk_device_queue_depth_max"
	hostDiskDeviceQueueDepthDim     = "max"

	hostDiskDeviceSCSIReservationConflictsContext = "vsphere.host_disk_device_scsi_reservation_conflicts"
	hostDiskDeviceSCSIReservationConflictsMetric  = "host_disk_device_scsi_reservation_conflicts_conflicts"
	hostDiskDeviceSCSIReservationConflictsDim     = "conflicts"

	hostDiskDeviceSCSIReservationConflictsPctContext = "vsphere.host_disk_device_scsi_reservation_conflicts_percentage"
	hostDiskDeviceSCSIReservationConflictsPctMetric  = "host_disk_device_scsi_reservation_conflicts_percentage_conflicts"
	hostDiskDeviceSCSIReservationConflictsPctDim     = "conflicts"
)

var hostDiskPerfMetricByCounter = map[string]string{
	"disk.read.average":                       hostDiskDeviceIOReadMetric,
	"disk.write.average":                      hostDiskDeviceIOWriteMetric,
	"disk.numberReadAveraged.average":         hostDiskDeviceIOPSReadMetric,
	"disk.numberWriteAveraged.average":        hostDiskDeviceIOPSWriteMetric,
	"disk.numberRead.summation":               hostDiskDeviceRequestsReadMetric,
	"disk.numberWrite.summation":              hostDiskDeviceRequestsWriteMetric,
	"disk.totalLatency.average":               hostDiskDeviceLatencyTotalMetric,
	"disk.totalReadLatency.average":           hostDiskDeviceLatencyReadMetric,
	"disk.totalWriteLatency.average":          hostDiskDeviceLatencyWriteMetric,
	"disk.deviceLatency.average":              hostDiskDeviceLatencyDeviceMetric,
	"disk.kernelLatency.average":              hostDiskDeviceLatencyKernelMetric,
	"disk.queueLatency.average":               hostDiskDeviceLatencyQueueMetric,
	"disk.deviceReadLatency.average":          hostDiskDeviceReadLatencyDeviceMetric,
	"disk.kernelReadLatency.average":          hostDiskDeviceReadLatencyKernelMetric,
	"disk.queueReadLatency.average":           hostDiskDeviceReadLatencyQueueMetric,
	"disk.deviceWriteLatency.average":         hostDiskDeviceWriteLatencyDeviceMetric,
	"disk.kernelWriteLatency.average":         hostDiskDeviceWriteLatencyKernelMetric,
	"disk.queueWriteLatency.average":          hostDiskDeviceWriteLatencyQueueMetric,
	"disk.commandsAveraged.average":           hostDiskDeviceCommandsMetric,
	"disk.commands.summation":                 hostDiskDeviceCommandEventsIssuedMetric,
	"disk.commandsAborted.summation":          hostDiskDeviceCommandEventsAbortedMetric,
	"disk.busResets.summation":                hostDiskDeviceCommandEventsBusResetsMetric,
	"disk.maxQueueDepth.average":              hostDiskDeviceQueueDepthMetric,
	"disk.scsiReservationConflicts.summation": hostDiskDeviceSCSIReservationConflictsMetric,
	"disk.scsiReservationCnflctsPct.average":  hostDiskDeviceSCSIReservationConflictsPctMetric,
}

type hostDiskPerfSample struct {
	host     *rs.Host
	instance string
	values   map[string]int64
}

func hostDiskPerformanceOptionalMetricNames() []string {
	return []string{
		hostDiskDeviceIOReadMetric,
		hostDiskDeviceIOWriteMetric,
		hostDiskDeviceIOPSReadMetric,
		hostDiskDeviceIOPSWriteMetric,
		hostDiskDeviceRequestsReadMetric,
		hostDiskDeviceRequestsWriteMetric,
		hostDiskDeviceLatencyTotalMetric,
		hostDiskDeviceLatencyReadMetric,
		hostDiskDeviceLatencyWriteMetric,
		hostDiskDeviceLatencyDeviceMetric,
		hostDiskDeviceLatencyKernelMetric,
		hostDiskDeviceLatencyQueueMetric,
		hostDiskDeviceReadLatencyDeviceMetric,
		hostDiskDeviceReadLatencyKernelMetric,
		hostDiskDeviceReadLatencyQueueMetric,
		hostDiskDeviceWriteLatencyDeviceMetric,
		hostDiskDeviceWriteLatencyKernelMetric,
		hostDiskDeviceWriteLatencyQueueMetric,
		hostDiskDeviceCommandsMetric,
		hostDiskDeviceCommandEventsIssuedMetric,
		hostDiskDeviceCommandEventsAbortedMetric,
		hostDiskDeviceCommandEventsBusResetsMetric,
		hostDiskDeviceQueueDepthMetric,
		hostDiskDeviceSCSIReservationConflictsMetric,
		hostDiskDeviceSCSIReservationConflictsPctMetric,
	}
}

func (c *Collector) collectHostDiskPerformanceMetrics(host *rs.Host, metrics []performance.MetricSeries) {
	for _, metric := range metrics {
		metricName, ok := hostDiskPerfMetricByCounter[metric.Name]
		if !ok || metric.Instance == "" || metric.Instance == "*" || len(metric.Value) == 0 || metric.Value[0] == -1 {
			continue
		}
		key := host.ID + "\x00" + metric.Instance
		if c.hostDiskPerfSamples == nil {
			c.hostDiskPerfSamples = make(map[string]*hostDiskPerfSample)
		}
		sample := c.hostDiskPerfSamples[key]
		if sample == nil {
			sample = &hostDiskPerfSample{
				host:     host,
				instance: metric.Instance,
				values:   make(map[string]int64),
			}
			c.hostDiskPerfSamples[key] = sample
		}
		sample.values[metricName] = metric.Value[0]
	}
}

func (c *Collector) writeHostDiskPerformanceMetrics(meter metrix.SnapshotMeter) {
	if !c.CollectHostDiskPerformance || len(c.hostDiskPerfSamples) == 0 || c.MaxHostDisks < 1 {
		return
	}

	m := c.hostDiskMatcher
	if m == nil {
		m = matcher.TRUE()
	}

	count := 0
	for _, sample := range sortedHostDiskPerfSamples(c.hostDiskPerfSamples) {
		if !hostDiskInstanceMatches(m, sample.instance) {
			continue
		}
		if count >= c.MaxHostDisks {
			return
		}
		count++

		labels := meter.LabelSet(c.hostDiskPerformanceLabels(sample.host, sample.instance)...)
		for metricName, value := range sample.values {
			if gauge := c.mx.gauge(metricName); gauge != nil {
				gauge.Observe(metrix.SampleValue(value), labels)
			}
		}
	}
}

func (c *Collector) hostDiskPerformanceLabels(host *rs.Host, instance string) []metrix.Label {
	labels := []metrix.Label{
		{Key: "id", Value: host.ID},
		{Key: "datacenter", Value: host.Hier.DC.Name},
		{Key: "cluster", Value: host.Hier.Cluster.Name},
		{Key: "host", Value: host.Name},
		{Key: hostDiskLabel, Value: instance},
		{Key: hostDiskInstanceLabel, Value: instance},
	}
	labels = append(labels, c.resourceEnrichmentLabels(host.ID)...)
	return labels
}

func hostDiskInstanceMatches(m matcher.Matcher, instance string) bool {
	return m.MatchString(instance) || m.MatchString("instance:"+instance)
}

func sortedHostDiskPerfSamples(samples map[string]*hostDiskPerfSample) []*hostDiskPerfSample {
	out := make([]*hostDiskPerfSample, 0, len(samples))
	for _, sample := range samples {
		out = append(out, sample)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].host.ID != out[j].host.ID {
			return out[i].host.ID < out[j].host.ID
		}
		return out[i].instance < out[j].instance
	})
	return out
}
