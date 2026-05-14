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
	defaultMaxHostStoragePaths   = 1024
	hostStoragePathLabel         = "path"
	hostStoragePathInstanceLabel = "path_instance"

	hostStoragePathIOContext     = "vsphere.host_storage_path_io"
	hostStoragePathIOReadMetric  = "host_storage_path_io_read"
	hostStoragePathIOWriteMetric = "host_storage_path_io_write"
	hostStoragePathIOReadDim     = "read"
	hostStoragePathIOWriteDim    = "write"

	hostStoragePathCommandsContext      = "vsphere.host_storage_path_commands"
	hostStoragePathCommandsIssuedMetric = "host_storage_path_commands_issued"
	hostStoragePathCommandsReadMetric   = "host_storage_path_commands_read"
	hostStoragePathCommandsWriteMetric  = "host_storage_path_commands_write"
	hostStoragePathCommandsIssuedDim    = "issued"
	hostStoragePathCommandsReadDim      = "read"
	hostStoragePathCommandsWriteDim     = "write"

	hostStoragePathLatencyContext     = "vsphere.host_storage_path_latency"
	hostStoragePathLatencyReadMetric  = "host_storage_path_latency_read"
	hostStoragePathLatencyWriteMetric = "host_storage_path_latency_write"
	hostStoragePathLatencyReadDim     = "read"
	hostStoragePathLatencyWriteDim    = "write"

	hostStoragePathCommandEventsContext         = "vsphere.host_storage_path_command_events"
	hostStoragePathCommandEventsAbortedMetric   = "host_storage_path_command_events_aborted"
	hostStoragePathCommandEventsBusResetsMetric = "host_storage_path_command_events_bus_resets"
	hostStoragePathCommandEventsAbortedDim      = "aborted"
	hostStoragePathCommandEventsBusResetsDim    = "bus_resets"

	hostStoragePathThroughputContext = "vsphere.host_storage_path_throughput"
	hostStoragePathThroughputMetric  = "host_storage_path_throughput_usage"
	hostStoragePathThroughputDim     = "usage"

	hostStoragePathThroughputContentionContext = "vsphere.host_storage_path_throughput_contention"
	hostStoragePathThroughputContentionMetric  = "host_storage_path_throughput_contention_contention"
	hostStoragePathThroughputContentionDim     = "contention"

	hostStoragePathMaxLatencyContext = "vsphere.host_storage_path_max_latency"
	hostStoragePathMaxLatencyMetric  = "host_storage_path_max_latency_max"
	hostStoragePathMaxLatencyDim     = "max"
)

var hostStoragePathPerfMetricByCounter = map[string]string{
	"storagePath.read.average":                hostStoragePathIOReadMetric,
	"storagePath.write.average":               hostStoragePathIOWriteMetric,
	"storagePath.commandsAveraged.average":    hostStoragePathCommandsIssuedMetric,
	"storagePath.numberReadAveraged.average":  hostStoragePathCommandsReadMetric,
	"storagePath.numberWriteAveraged.average": hostStoragePathCommandsWriteMetric,
	"storagePath.totalReadLatency.average":    hostStoragePathLatencyReadMetric,
	"storagePath.totalWriteLatency.average":   hostStoragePathLatencyWriteMetric,
	"storagePath.commandsAborted.summation":   hostStoragePathCommandEventsAbortedMetric,
	"storagePath.busResets.summation":         hostStoragePathCommandEventsBusResetsMetric,
	"storagePath.throughput.usage.average":    hostStoragePathThroughputMetric,
	"storagePath.throughput.cont.average":     hostStoragePathThroughputContentionMetric,
}

var hostStoragePathAggregateMetricByCounter = map[string]string{
	"storagePath.maxTotalLatency.latest": hostStoragePathMaxLatencyMetric,
}

type hostStoragePathPerfSample struct {
	host     *rs.Host
	instance string
	values   map[string]int64
}

type hostStoragePathAggregatePerfSample struct {
	host   *rs.Host
	values map[string]int64
}

func hostStoragePathPerformanceOptionalMetricNames() []string {
	names := hostStoragePathInstancePerformanceOptionalMetricNames()
	names = append(names, hostStoragePathMaxLatencyMetric)
	return names
}

func hostStoragePathInstancePerformanceOptionalMetricNames() []string {
	return []string{
		hostStoragePathIOReadMetric,
		hostStoragePathIOWriteMetric,
		hostStoragePathCommandsIssuedMetric,
		hostStoragePathCommandsReadMetric,
		hostStoragePathCommandsWriteMetric,
		hostStoragePathLatencyReadMetric,
		hostStoragePathLatencyWriteMetric,
		hostStoragePathCommandEventsAbortedMetric,
		hostStoragePathCommandEventsBusResetsMetric,
		hostStoragePathThroughputMetric,
		hostStoragePathThroughputContentionMetric,
	}
}

func (c *Collector) collectHostStoragePathPerformanceMetrics(host *rs.Host, metrics []performance.MetricSeries) {
	for _, metric := range metrics {
		if len(metric.Value) == 0 || metric.Value[0] == -1 {
			continue
		}
		if metricName, ok := hostStoragePathAggregateMetricByCounter[metric.Name]; ok && metric.Instance == "" {
			c.collectHostStoragePathAggregatePerformanceMetric(host, metricName, metric.Value[0])
			continue
		}
		metricName, ok := hostStoragePathPerfMetricByCounter[metric.Name]
		if !ok || metric.Instance == "" || metric.Instance == "*" {
			continue
		}
		key := host.ID + "\x00" + metric.Instance
		if c.hostStoragePathPerfSamples == nil {
			c.hostStoragePathPerfSamples = make(map[string]*hostStoragePathPerfSample)
		}
		sample := c.hostStoragePathPerfSamples[key]
		if sample == nil {
			sample = &hostStoragePathPerfSample{
				host:     host,
				instance: metric.Instance,
				values:   make(map[string]int64),
			}
			c.hostStoragePathPerfSamples[key] = sample
		}
		sample.values[metricName] = metric.Value[0]
	}
}

func (c *Collector) collectHostStoragePathAggregatePerformanceMetric(host *rs.Host, metricName string, value int64) {
	if c.hostStoragePathAggregatePerfSamples == nil {
		c.hostStoragePathAggregatePerfSamples = make(map[string]*hostStoragePathAggregatePerfSample)
	}
	sample := c.hostStoragePathAggregatePerfSamples[host.ID]
	if sample == nil {
		sample = &hostStoragePathAggregatePerfSample{
			host:   host,
			values: make(map[string]int64),
		}
		c.hostStoragePathAggregatePerfSamples[host.ID] = sample
	}
	sample.values[metricName] = value
}

func (c *Collector) writeHostStoragePathPerformanceMetrics(meter metrix.SnapshotMeter) {
	if !c.CollectHostStoragePathPerformance {
		return
	}
	c.writeHostStoragePathInstancePerformanceMetrics(meter)
	c.writeHostStoragePathAggregatePerformanceMetrics(meter)
}

func (c *Collector) writeHostStoragePathInstancePerformanceMetrics(meter metrix.SnapshotMeter) {
	if len(c.hostStoragePathPerfSamples) == 0 || c.MaxHostStoragePaths < 1 {
		return
	}

	m := c.hostStoragePathMatcher
	if m == nil {
		m = matcher.TRUE()
	}

	count := 0
	for _, sample := range sortedHostStoragePathPerfSamples(c.hostStoragePathPerfSamples) {
		if !hostStoragePathInstanceMatches(m, sample.instance) {
			continue
		}
		if count >= c.MaxHostStoragePaths {
			return
		}
		count++

		scope := c.resourceHostScope(sample.host.ID)
		writeMeter := meter
		scoped := !scope.IsDefault()
		if scoped {
			writeMeter = meter.WithHostScope(scope)
		}
		labels := writeMeter.LabelSet(c.hostStoragePathPerformanceLabels(sample.host, sample.instance)...)
		for metricName, value := range sample.values {
			if gauge := c.mx.gauge(writeMeter, metricName, scoped); gauge != nil {
				gauge.Observe(metrix.SampleValue(value), labels)
			}
		}
	}
}

func (c *Collector) writeHostStoragePathAggregatePerformanceMetrics(meter metrix.SnapshotMeter) {
	if len(c.hostStoragePathAggregatePerfSamples) == 0 {
		return
	}

	for _, sample := range sortedHostStoragePathAggregatePerfSamples(c.hostStoragePathAggregatePerfSamples) {
		scope := c.resourceHostScope(sample.host.ID)
		writeMeter := meter
		scoped := !scope.IsDefault()
		if scoped {
			writeMeter = meter.WithHostScope(scope)
		}
		labels := writeMeter.LabelSet(c.hostStoragePathAggregatePerformanceLabels(sample.host)...)
		for metricName, value := range sample.values {
			if gauge := c.mx.gauge(writeMeter, metricName, scoped); gauge != nil {
				gauge.Observe(metrix.SampleValue(value), labels)
			}
		}
	}
}

func (c *Collector) hostStoragePathPerformanceLabels(host *rs.Host, instance string) []metrix.Label {
	labels := []metrix.Label{
		{Key: "id", Value: host.ID},
		{Key: "datacenter", Value: host.Hier.DC.Name},
		{Key: "cluster", Value: host.Hier.Cluster.Name},
		{Key: "host", Value: host.Name},
		{Key: hostStoragePathLabel, Value: instance},
		{Key: hostStoragePathInstanceLabel, Value: instance},
	}
	labels = append(labels, c.resourceEnrichmentLabels(host.ID)...)
	return labels
}

func (c *Collector) hostStoragePathAggregatePerformanceLabels(host *rs.Host) []metrix.Label {
	labels := []metrix.Label{
		{Key: "id", Value: host.ID},
		{Key: "datacenter", Value: host.Hier.DC.Name},
		{Key: "cluster", Value: host.Hier.Cluster.Name},
		{Key: "host", Value: host.Name},
	}
	labels = append(labels, c.resourceEnrichmentLabels(host.ID)...)
	return labels
}

func hostStoragePathInstanceMatches(m matcher.Matcher, instance string) bool {
	return m.MatchString(instance) || m.MatchString("path:"+instance) || m.MatchString("instance:"+instance)
}

func sortedHostStoragePathPerfSamples(samples map[string]*hostStoragePathPerfSample) []*hostStoragePathPerfSample {
	out := make([]*hostStoragePathPerfSample, 0, len(samples))
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

func sortedHostStoragePathAggregatePerfSamples(samples map[string]*hostStoragePathAggregatePerfSample) []*hostStoragePathAggregatePerfSample {
	out := make([]*hostStoragePathAggregatePerfSample, 0, len(samples))
	for _, sample := range samples {
		out = append(out, sample)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].host.ID < out[j].host.ID
	})
	return out
}
