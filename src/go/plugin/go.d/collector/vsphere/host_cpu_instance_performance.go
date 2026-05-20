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
	defaultMaxHostCPUInstances   = 1024
	hostCPUInstanceLabel         = "cpu"
	hostCPUInstanceInstanceLabel = "cpu_instance"

	hostCPUInstanceUtilizationContext           = "vsphere.host_cpu_instance_utilization"
	hostCPUInstanceUtilizationUsageMetric       = "host_cpu_instance_utilization_usage"
	hostCPUInstanceUtilizationUtilizationMetric = "host_cpu_instance_utilization_utilization"
	hostCPUInstanceUtilizationCoreMetric        = "host_cpu_instance_utilization_core"
	hostCPUInstanceUtilizationUsageDim          = "usage"
	hostCPUInstanceUtilizationUtilizationDim    = "utilization"
	hostCPUInstanceUtilizationCoreDim           = "core_utilization"

	hostCPUInstanceTimeContext    = "vsphere.host_cpu_instance_time"
	hostCPUInstanceTimeUsedMetric = "host_cpu_instance_time_used"
	hostCPUInstanceTimeIdleMetric = "host_cpu_instance_time_idle"
	hostCPUInstanceTimeUsedDim    = "used"
	hostCPUInstanceTimeIdleDim    = "idle"
)

var hostCPUInstancePerfMetricByCounter = map[string]string{
	"cpu.coreUtilization.average": hostCPUInstanceUtilizationCoreMetric,
	"cpu.usage.average":           hostCPUInstanceUtilizationUsageMetric,
	"cpu.utilization.average":     hostCPUInstanceUtilizationUtilizationMetric,
	"cpu.idle.summation":          hostCPUInstanceTimeIdleMetric,
	"cpu.used.summation":          hostCPUInstanceTimeUsedMetric,
}

type hostCPUInstancePerfSample struct {
	host     *rs.Host
	instance string
	values   map[string]int64
}

func hostCPUInstancePerformanceOptionalMetricNames() []string {
	return []string{
		hostCPUInstanceUtilizationUsageMetric,
		hostCPUInstanceUtilizationUtilizationMetric,
		hostCPUInstanceUtilizationCoreMetric,
		hostCPUInstanceTimeUsedMetric,
		hostCPUInstanceTimeIdleMetric,
	}
}

func (c *Collector) collectHostCPUInstancePerformanceMetrics(host *rs.Host, metrics []performance.MetricSeries) {
	for _, metric := range metrics {
		if len(metric.Value) == 0 || metric.Value[0] == -1 {
			continue
		}
		metricName, ok := hostCPUInstancePerfMetricByCounter[metric.Name]
		if !ok || metric.Instance == "" || metric.Instance == "*" {
			continue
		}
		key := host.ID + "\x00" + metric.Instance
		if c.hostCPUInstancePerfSamples == nil {
			c.hostCPUInstancePerfSamples = make(map[string]*hostCPUInstancePerfSample)
		}
		sample := c.hostCPUInstancePerfSamples[key]
		if sample == nil {
			sample = &hostCPUInstancePerfSample{
				host:     host,
				instance: metric.Instance,
				values:   make(map[string]int64),
			}
			c.hostCPUInstancePerfSamples[key] = sample
		}
		sample.values[metricName] = metric.Value[0]
	}
}

func (c *Collector) writeHostCPUInstancePerformanceMetrics(meter metrix.SnapshotMeter) {
	if !c.CollectHostCPUInstancePerformance {
		return
	}
	if len(c.hostCPUInstancePerfSamples) == 0 || c.MaxHostCPUInstances < 1 {
		return
	}

	m := c.hostCPUInstanceMatcher
	if m == nil {
		m = matcher.TRUE()
	}

	count := 0
	for _, sample := range sortedHostCPUInstancePerfSamples(c.hostCPUInstancePerfSamples) {
		if !hostCPUInstanceMatches(m, sample.instance) {
			continue
		}
		if count >= c.MaxHostCPUInstances {
			return
		}
		count++

		labels := meter.LabelSet(c.hostCPUInstancePerformanceLabels(sample.host, sample.instance)...)
		for metricName, value := range sample.values {
			if gauge := c.mx.gauge(metricName); gauge != nil {
				gauge.Observe(metrix.SampleValue(value), labels)
			}
		}
	}
}

func (c *Collector) hostCPUInstancePerformanceLabels(host *rs.Host, instance string) []metrix.Label {
	labels := []metrix.Label{
		{Key: "id", Value: host.ID},
		{Key: "datacenter", Value: host.Hier.DC.Name},
		{Key: "cluster", Value: host.Hier.Cluster.Name},
		{Key: "host", Value: host.Name},
		{Key: hostCPUInstanceLabel, Value: instance},
		{Key: hostCPUInstanceInstanceLabel, Value: instance},
	}
	labels = append(labels, c.resourceEnrichmentLabels(host.ID)...)
	return labels
}

func hostCPUInstanceMatches(m matcher.Matcher, instance string) bool {
	return m.MatchString(instance) || m.MatchString("cpu:"+instance) || m.MatchString("instance:"+instance)
}

func sortedHostCPUInstancePerfSamples(samples map[string]*hostCPUInstancePerfSample) []*hostCPUInstancePerfSample {
	out := make([]*hostCPUInstancePerfSample, 0, len(samples))
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
