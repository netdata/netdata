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
	defaultMaxHostNICs   = 1024
	hostNICLabel         = "interface"
	hostNICInstanceLabel = "interface_instance"

	hostNetInterfaceTrafficContext  = "vsphere.host_net_interface_traffic"
	hostNetInterfaceTrafficRxMetric = "host_net_interface_traffic_received"
	hostNetInterfaceTrafficTxMetric = "host_net_interface_traffic_sent"
	hostNetInterfaceTrafficRxDim    = "received"
	hostNetInterfaceTrafficTxDim    = "sent"

	hostNetInterfacePacketsContext  = "vsphere.host_net_interface_packets"
	hostNetInterfacePacketsRxMetric = "host_net_interface_packets_received"
	hostNetInterfacePacketsTxMetric = "host_net_interface_packets_sent"
	hostNetInterfacePacketsRxDim    = "received"
	hostNetInterfacePacketsTxDim    = "sent"

	hostNetInterfaceDropsContext  = "vsphere.host_net_interface_drops"
	hostNetInterfaceDropsRxMetric = "host_net_interface_drops_received"
	hostNetInterfaceDropsTxMetric = "host_net_interface_drops_sent"
	hostNetInterfaceDropsRxDim    = "received"
	hostNetInterfaceDropsTxDim    = "sent"

	hostNetInterfaceErrorsContext  = "vsphere.host_net_interface_errors"
	hostNetInterfaceErrorsRxMetric = "host_net_interface_errors_received"
	hostNetInterfaceErrorsTxMetric = "host_net_interface_errors_sent"
	hostNetInterfaceErrorsRxDim    = "received"
	hostNetInterfaceErrorsTxDim    = "sent"

	hostNetInterfaceBroadcastPacketsContext  = "vsphere.host_net_interface_broadcast_packets"
	hostNetInterfaceBroadcastPacketsRxMetric = "host_net_interface_broadcast_packets_received"
	hostNetInterfaceBroadcastPacketsTxMetric = "host_net_interface_broadcast_packets_sent"
	hostNetInterfaceBroadcastPacketsRxDim    = "received"
	hostNetInterfaceBroadcastPacketsTxDim    = "sent"

	hostNetInterfaceMulticastPacketsContext  = "vsphere.host_net_interface_multicast_packets"
	hostNetInterfaceMulticastPacketsRxMetric = "host_net_interface_multicast_packets_received"
	hostNetInterfaceMulticastPacketsTxMetric = "host_net_interface_multicast_packets_sent"
	hostNetInterfaceMulticastPacketsRxDim    = "received"
	hostNetInterfaceMulticastPacketsTxDim    = "sent"

	hostNetInterfaceUnknownProtocolFramesContext = "vsphere.host_net_interface_unknown_protocol_frames"
	hostNetInterfaceUnknownProtocolFramesMetric  = "host_net_interface_unknown_protocol_frames_unknown"
	hostNetInterfaceUnknownProtocolFramesDim     = "unknown"

	hostNetInterfaceUsageContext = "vsphere.host_net_interface_usage"
	hostNetInterfaceUsageMetric  = "host_net_interface_usage_usage"
	hostNetInterfaceUsageDim     = "usage"
)

var hostNICPerfMetricByCounter = map[string]string{
	"net.bytesRx.average":         hostNetInterfaceTrafficRxMetric,
	"net.bytesTx.average":         hostNetInterfaceTrafficTxMetric,
	"net.packetsRx.summation":     hostNetInterfacePacketsRxMetric,
	"net.packetsTx.summation":     hostNetInterfacePacketsTxMetric,
	"net.droppedRx.summation":     hostNetInterfaceDropsRxMetric,
	"net.droppedTx.summation":     hostNetInterfaceDropsTxMetric,
	"net.errorsRx.summation":      hostNetInterfaceErrorsRxMetric,
	"net.errorsTx.summation":      hostNetInterfaceErrorsTxMetric,
	"net.broadcastRx.summation":   hostNetInterfaceBroadcastPacketsRxMetric,
	"net.broadcastTx.summation":   hostNetInterfaceBroadcastPacketsTxMetric,
	"net.multicastRx.summation":   hostNetInterfaceMulticastPacketsRxMetric,
	"net.multicastTx.summation":   hostNetInterfaceMulticastPacketsTxMetric,
	"net.unknownProtos.summation": hostNetInterfaceUnknownProtocolFramesMetric,
	"net.usage.average":           hostNetInterfaceUsageMetric,
}

type hostNICPerfSample struct {
	host     *rs.Host
	instance string
	values   map[string]int64
}

func hostNICPerformanceOptionalMetricNames() []string {
	return []string{
		hostNetInterfaceTrafficRxMetric,
		hostNetInterfaceTrafficTxMetric,
		hostNetInterfacePacketsRxMetric,
		hostNetInterfacePacketsTxMetric,
		hostNetInterfaceDropsRxMetric,
		hostNetInterfaceDropsTxMetric,
		hostNetInterfaceErrorsRxMetric,
		hostNetInterfaceErrorsTxMetric,
		hostNetInterfaceBroadcastPacketsRxMetric,
		hostNetInterfaceBroadcastPacketsTxMetric,
		hostNetInterfaceMulticastPacketsRxMetric,
		hostNetInterfaceMulticastPacketsTxMetric,
		hostNetInterfaceUnknownProtocolFramesMetric,
		hostNetInterfaceUsageMetric,
	}
}

func (c *Collector) collectHostNICPerformanceMetrics(host *rs.Host, metrics []performance.MetricSeries) {
	for _, metric := range metrics {
		metricName, ok := hostNICPerfMetricByCounter[metric.Name]
		if !ok || metric.Instance == "" || metric.Instance == "*" || len(metric.Value) == 0 || metric.Value[0] == -1 {
			continue
		}
		key := host.ID + "\x00" + metric.Instance
		if c.hostNICPerfSamples == nil {
			c.hostNICPerfSamples = make(map[string]*hostNICPerfSample)
		}
		sample := c.hostNICPerfSamples[key]
		if sample == nil {
			sample = &hostNICPerfSample{
				host:     host,
				instance: metric.Instance,
				values:   make(map[string]int64),
			}
			c.hostNICPerfSamples[key] = sample
		}
		sample.values[metricName] = metric.Value[0]
	}
}

func (c *Collector) writeHostNICPerformanceMetrics(meter metrix.SnapshotMeter) {
	if !c.CollectHostNICPerformance || len(c.hostNICPerfSamples) == 0 || c.MaxHostNICs < 1 {
		return
	}

	m := c.hostNICMatcher
	if m == nil {
		m = matcher.TRUE()
	}

	count := 0
	for _, sample := range sortedHostNICPerfSamples(c.hostNICPerfSamples) {
		if !hostNICInstanceMatches(m, sample.instance) {
			continue
		}
		if count >= c.MaxHostNICs {
			return
		}
		count++

		labels := meter.LabelSet(c.hostNICPerformanceLabels(sample.host, sample.instance)...)
		for metricName, value := range sample.values {
			if gauge := c.mx.gauge(metricName); gauge != nil {
				gauge.Observe(metrix.SampleValue(value), labels)
			}
		}
	}
}

func (c *Collector) hostNICPerformanceLabels(host *rs.Host, instance string) []metrix.Label {
	labels := []metrix.Label{
		{Key: "id", Value: host.ID},
		{Key: "datacenter", Value: host.Hier.DC.Name},
		{Key: "cluster", Value: host.Hier.Cluster.Name},
		{Key: "host", Value: host.Name},
		{Key: hostNICLabel, Value: instance},
		{Key: hostNICInstanceLabel, Value: instance},
	}
	labels = append(labels, c.resourceEnrichmentLabels(host.ID)...)
	return labels
}

func hostNICInstanceMatches(m matcher.Matcher, instance string) bool {
	return m.MatchString(instance) || m.MatchString("interface:"+instance) || m.MatchString("instance:"+instance)
}

func sortedHostNICPerfSamples(samples map[string]*hostNICPerfSample) []*hostNICPerfSample {
	out := make([]*hostNICPerfSample, 0, len(samples))
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
