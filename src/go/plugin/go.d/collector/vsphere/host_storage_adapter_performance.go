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
	defaultMaxHostStorageAdapters   = 1024
	hostStorageAdapterLabel         = "adapter"
	hostStorageAdapterInstanceLabel = "adapter_instance"

	hostStorageAdapterIOContext     = "vsphere.host_storage_adapter_io"
	hostStorageAdapterIOReadMetric  = "host_storage_adapter_io_read"
	hostStorageAdapterIOWriteMetric = "host_storage_adapter_io_write"
	hostStorageAdapterIOReadDim     = "read"
	hostStorageAdapterIOWriteDim    = "write"

	hostStorageAdapterCommandsContext      = "vsphere.host_storage_adapter_commands"
	hostStorageAdapterCommandsIssuedMetric = "host_storage_adapter_commands_issued"
	hostStorageAdapterCommandsReadMetric   = "host_storage_adapter_commands_read"
	hostStorageAdapterCommandsWriteMetric  = "host_storage_adapter_commands_write"
	hostStorageAdapterCommandsIssuedDim    = "issued"
	hostStorageAdapterCommandsReadDim      = "read"
	hostStorageAdapterCommandsWriteDim     = "write"

	hostStorageAdapterLatencyContext     = "vsphere.host_storage_adapter_latency"
	hostStorageAdapterLatencyReadMetric  = "host_storage_adapter_latency_read"
	hostStorageAdapterLatencyWriteMetric = "host_storage_adapter_latency_write"
	hostStorageAdapterLatencyQueueMetric = "host_storage_adapter_latency_queue"
	hostStorageAdapterLatencyReadDim     = "read"
	hostStorageAdapterLatencyWriteDim    = "write"
	hostStorageAdapterLatencyQueueDim    = "queue"

	hostStorageAdapterQueueContext           = "vsphere.host_storage_adapter_queue"
	hostStorageAdapterQueueOutstandingMetric = "host_storage_adapter_queue_outstanding"
	hostStorageAdapterQueueQueuedMetric      = "host_storage_adapter_queue_queued"
	hostStorageAdapterQueueDepthMetric       = "host_storage_adapter_queue_depth"
	hostStorageAdapterQueueOutstandingDim    = "outstanding"
	hostStorageAdapterQueueQueuedDim         = "queued"
	hostStorageAdapterQueueDepthDim          = "depth"

	hostStorageAdapterOutstandingIOPctContext = "vsphere.host_storage_adapter_outstanding_io_percentage"
	hostStorageAdapterOutstandingIOPctMetric  = "host_storage_adapter_outstanding_io_percentage_outstanding"
	hostStorageAdapterOutstandingIOPctDim     = "outstanding"

	hostStorageAdapterThroughputContext = "vsphere.host_storage_adapter_throughput"
	hostStorageAdapterThroughputMetric  = "host_storage_adapter_throughput_usage"
	hostStorageAdapterThroughputDim     = "usage"

	hostStorageAdapterThroughputContentionContext = "vsphere.host_storage_adapter_throughput_contention"
	hostStorageAdapterThroughputContentionMetric  = "host_storage_adapter_throughput_contention_contention"
	hostStorageAdapterThroughputContentionDim     = "contention"

	hostStorageAdapterMaxLatencyContext = "vsphere.host_storage_adapter_max_latency"
	hostStorageAdapterMaxLatencyMetric  = "host_storage_adapter_max_latency_max"
	hostStorageAdapterMaxLatencyDim     = "max"
)

var hostStorageAdapterPerfMetricByCounter = map[string]string{
	"storageAdapter.read.average":                hostStorageAdapterIOReadMetric,
	"storageAdapter.write.average":               hostStorageAdapterIOWriteMetric,
	"storageAdapter.commandsAveraged.average":    hostStorageAdapterCommandsIssuedMetric,
	"storageAdapter.numberReadAveraged.average":  hostStorageAdapterCommandsReadMetric,
	"storageAdapter.numberWriteAveraged.average": hostStorageAdapterCommandsWriteMetric,
	"storageAdapter.totalReadLatency.average":    hostStorageAdapterLatencyReadMetric,
	"storageAdapter.totalWriteLatency.average":   hostStorageAdapterLatencyWriteMetric,
	"storageAdapter.queueLatency.average":        hostStorageAdapterLatencyQueueMetric,
	"storageAdapter.outstandingIOs.average":      hostStorageAdapterQueueOutstandingMetric,
	"storageAdapter.queued.average":              hostStorageAdapterQueueQueuedMetric,
	"storageAdapter.queueDepth.average":          hostStorageAdapterQueueDepthMetric,
	"storageAdapter.OIOsPct.average":             hostStorageAdapterOutstandingIOPctMetric,
	"storageAdapter.throughput.usag.average":     hostStorageAdapterThroughputMetric,
	"storageAdapter.throughput.cont.average":     hostStorageAdapterThroughputContentionMetric,
}

var hostStorageAdapterAggregateMetricByCounter = map[string]string{
	"storageAdapter.maxTotalLatency.latest": hostStorageAdapterMaxLatencyMetric,
}

type hostStorageAdapterPerfSample struct {
	host     *rs.Host
	instance string
	values   map[string]int64
}

type hostStorageAdapterAggregatePerfSample struct {
	host   *rs.Host
	values map[string]int64
}

func hostStorageAdapterPerformanceOptionalMetricNames() []string {
	names := hostStorageAdapterInstancePerformanceOptionalMetricNames()
	names = append(names, hostStorageAdapterMaxLatencyMetric)
	return names
}

func hostStorageAdapterInstancePerformanceOptionalMetricNames() []string {
	return []string{
		hostStorageAdapterIOReadMetric,
		hostStorageAdapterIOWriteMetric,
		hostStorageAdapterCommandsIssuedMetric,
		hostStorageAdapterCommandsReadMetric,
		hostStorageAdapterCommandsWriteMetric,
		hostStorageAdapterLatencyReadMetric,
		hostStorageAdapterLatencyWriteMetric,
		hostStorageAdapterLatencyQueueMetric,
		hostStorageAdapterQueueOutstandingMetric,
		hostStorageAdapterQueueQueuedMetric,
		hostStorageAdapterQueueDepthMetric,
		hostStorageAdapterOutstandingIOPctMetric,
		hostStorageAdapterThroughputMetric,
		hostStorageAdapterThroughputContentionMetric,
	}
}

func (c *Collector) collectHostStorageAdapterPerformanceMetrics(host *rs.Host, metrics []performance.MetricSeries) {
	for _, metric := range metrics {
		if len(metric.Value) == 0 || metric.Value[0] == -1 {
			continue
		}
		if metricName, ok := hostStorageAdapterAggregateMetricByCounter[metric.Name]; ok && metric.Instance == "" {
			c.collectHostStorageAdapterAggregatePerformanceMetric(host, metricName, metric.Value[0])
			continue
		}
		metricName, ok := hostStorageAdapterPerfMetricByCounter[metric.Name]
		if !ok || metric.Instance == "" || metric.Instance == "*" {
			continue
		}
		key := host.ID + "\x00" + metric.Instance
		if c.hostStorageAdapterPerfSamples == nil {
			c.hostStorageAdapterPerfSamples = make(map[string]*hostStorageAdapterPerfSample)
		}
		sample := c.hostStorageAdapterPerfSamples[key]
		if sample == nil {
			sample = &hostStorageAdapterPerfSample{
				host:     host,
				instance: metric.Instance,
				values:   make(map[string]int64),
			}
			c.hostStorageAdapterPerfSamples[key] = sample
		}
		sample.values[metricName] = metric.Value[0]
	}
}

func (c *Collector) collectHostStorageAdapterAggregatePerformanceMetric(host *rs.Host, metricName string, value int64) {
	if c.hostStorageAdapterAggregatePerfSamples == nil {
		c.hostStorageAdapterAggregatePerfSamples = make(map[string]*hostStorageAdapterAggregatePerfSample)
	}
	sample := c.hostStorageAdapterAggregatePerfSamples[host.ID]
	if sample == nil {
		sample = &hostStorageAdapterAggregatePerfSample{
			host:   host,
			values: make(map[string]int64),
		}
		c.hostStorageAdapterAggregatePerfSamples[host.ID] = sample
	}
	sample.values[metricName] = value
}

func (c *Collector) writeHostStorageAdapterPerformanceMetrics(meter metrix.SnapshotMeter) {
	if !c.CollectHostStorageAdapterPerformance {
		return
	}
	c.writeHostStorageAdapterInstancePerformanceMetrics(meter)
	c.writeHostStorageAdapterAggregatePerformanceMetrics(meter)
}

func (c *Collector) writeHostStorageAdapterInstancePerformanceMetrics(meter metrix.SnapshotMeter) {
	if len(c.hostStorageAdapterPerfSamples) == 0 || c.MaxHostStorageAdapters < 1 {
		return
	}

	m := c.hostStorageAdapterMatcher
	if m == nil {
		m = matcher.TRUE()
	}

	count := 0
	for _, sample := range sortedHostStorageAdapterPerfSamples(c.hostStorageAdapterPerfSamples) {
		if !hostStorageAdapterInstanceMatches(m, sample.instance) {
			continue
		}
		if count >= c.MaxHostStorageAdapters {
			return
		}
		count++

		labels := meter.LabelSet(c.hostStorageAdapterPerformanceLabels(sample.host, sample.instance)...)
		for metricName, value := range sample.values {
			if gauge := c.mx.gauge(metricName); gauge != nil {
				gauge.Observe(metrix.SampleValue(value), labels)
			}
		}
	}
}

func (c *Collector) writeHostStorageAdapterAggregatePerformanceMetrics(meter metrix.SnapshotMeter) {
	if len(c.hostStorageAdapterAggregatePerfSamples) == 0 {
		return
	}

	for _, sample := range sortedHostStorageAdapterAggregatePerfSamples(c.hostStorageAdapterAggregatePerfSamples) {
		labels := meter.LabelSet(c.hostStorageAdapterAggregatePerformanceLabels(sample.host)...)
		for metricName, value := range sample.values {
			if gauge := c.mx.gauge(metricName); gauge != nil {
				gauge.Observe(metrix.SampleValue(value), labels)
			}
		}
	}
}

func (c *Collector) hostStorageAdapterPerformanceLabels(host *rs.Host, instance string) []metrix.Label {
	labels := []metrix.Label{
		{Key: "id", Value: host.ID},
		{Key: "datacenter", Value: host.Hier.DC.Name},
		{Key: "cluster", Value: host.Hier.Cluster.Name},
		{Key: "host", Value: host.Name},
		{Key: hostStorageAdapterLabel, Value: instance},
		{Key: hostStorageAdapterInstanceLabel, Value: instance},
	}
	labels = append(labels, c.resourceEnrichmentLabels(host.ID)...)
	return labels
}

func (c *Collector) hostStorageAdapterAggregatePerformanceLabels(host *rs.Host) []metrix.Label {
	labels := []metrix.Label{
		{Key: "id", Value: host.ID},
		{Key: "datacenter", Value: host.Hier.DC.Name},
		{Key: "cluster", Value: host.Hier.Cluster.Name},
		{Key: "host", Value: host.Name},
	}
	labels = append(labels, c.resourceEnrichmentLabels(host.ID)...)
	return labels
}

func hostStorageAdapterInstanceMatches(m matcher.Matcher, instance string) bool {
	return m.MatchString(instance) || m.MatchString("adapter:"+instance) || m.MatchString("instance:"+instance)
}

func sortedHostStorageAdapterPerfSamples(samples map[string]*hostStorageAdapterPerfSample) []*hostStorageAdapterPerfSample {
	out := make([]*hostStorageAdapterPerfSample, 0, len(samples))
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

func sortedHostStorageAdapterAggregatePerfSamples(samples map[string]*hostStorageAdapterAggregatePerfSample) []*hostStorageAdapterAggregatePerfSample {
	out := make([]*hostStorageAdapterAggregatePerfSample, 0, len(samples))
	for _, sample := range samples {
		out = append(out, sample)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].host.ID < out[j].host.ID
	})
	return out
}
