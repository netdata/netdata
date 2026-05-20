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
	defaultMaxVMNICs   = 1024
	vmNICLabel         = "interface"
	vmNICInstanceLabel = "interface_instance"

	vmNetInterfaceTrafficContext  = "vsphere.vm_net_interface_traffic"
	vmNetInterfaceTrafficRxMetric = "vm_net_interface_traffic_received"
	vmNetInterfaceTrafficTxMetric = "vm_net_interface_traffic_sent"
	vmNetInterfaceTrafficRxDim    = "received"
	vmNetInterfaceTrafficTxDim    = "sent"

	vmNetInterfacePacketsContext  = "vsphere.vm_net_interface_packets"
	vmNetInterfacePacketsRxMetric = "vm_net_interface_packets_received"
	vmNetInterfacePacketsTxMetric = "vm_net_interface_packets_sent"
	vmNetInterfacePacketsRxDim    = "received"
	vmNetInterfacePacketsTxDim    = "sent"

	vmNetInterfaceDropsContext  = "vsphere.vm_net_interface_drops"
	vmNetInterfaceDropsRxMetric = "vm_net_interface_drops_received"
	vmNetInterfaceDropsTxMetric = "vm_net_interface_drops_sent"
	vmNetInterfaceDropsRxDim    = "received"
	vmNetInterfaceDropsTxDim    = "sent"

	vmNetInterfaceBroadcastPacketsContext  = "vsphere.vm_net_interface_broadcast_packets"
	vmNetInterfaceBroadcastPacketsRxMetric = "vm_net_interface_broadcast_packets_received"
	vmNetInterfaceBroadcastPacketsTxMetric = "vm_net_interface_broadcast_packets_sent"
	vmNetInterfaceBroadcastPacketsRxDim    = "received"
	vmNetInterfaceBroadcastPacketsTxDim    = "sent"

	vmNetInterfaceMulticastPacketsContext  = "vsphere.vm_net_interface_multicast_packets"
	vmNetInterfaceMulticastPacketsRxMetric = "vm_net_interface_multicast_packets_received"
	vmNetInterfaceMulticastPacketsTxMetric = "vm_net_interface_multicast_packets_sent"
	vmNetInterfaceMulticastPacketsRxDim    = "received"
	vmNetInterfaceMulticastPacketsTxDim    = "sent"
)

var vmNICPerfMetricByCounter = map[string]string{
	"net.bytesRx.average":       vmNetInterfaceTrafficRxMetric,
	"net.bytesTx.average":       vmNetInterfaceTrafficTxMetric,
	"net.packetsRx.summation":   vmNetInterfacePacketsRxMetric,
	"net.packetsTx.summation":   vmNetInterfacePacketsTxMetric,
	"net.droppedRx.summation":   vmNetInterfaceDropsRxMetric,
	"net.droppedTx.summation":   vmNetInterfaceDropsTxMetric,
	"net.broadcastRx.summation": vmNetInterfaceBroadcastPacketsRxMetric,
	"net.broadcastTx.summation": vmNetInterfaceBroadcastPacketsTxMetric,
	"net.multicastRx.summation": vmNetInterfaceMulticastPacketsRxMetric,
	"net.multicastTx.summation": vmNetInterfaceMulticastPacketsTxMetric,
}

type vmNICPerfSample struct {
	vm       *rs.VM
	instance string
	values   map[string]int64
}

func vmNICPerformanceOptionalMetricNames() []string {
	return []string{
		vmNetInterfaceTrafficRxMetric,
		vmNetInterfaceTrafficTxMetric,
		vmNetInterfacePacketsRxMetric,
		vmNetInterfacePacketsTxMetric,
		vmNetInterfaceDropsRxMetric,
		vmNetInterfaceDropsTxMetric,
		vmNetInterfaceBroadcastPacketsRxMetric,
		vmNetInterfaceBroadcastPacketsTxMetric,
		vmNetInterfaceMulticastPacketsRxMetric,
		vmNetInterfaceMulticastPacketsTxMetric,
	}
}

func isVMNICPerformanceMetric(name string) bool {
	_, ok := vmNICPerfMetricByCounter[name]
	return ok
}

func (c *Collector) collectVMNICPerformanceMetrics(vm *rs.VM, metrics []performance.MetricSeries) {
	for _, metric := range metrics {
		metricName, ok := vmNICPerfMetricByCounter[metric.Name]
		if !ok || metric.Instance == "" || metric.Instance == "*" || len(metric.Value) == 0 || metric.Value[0] == -1 {
			continue
		}
		key := vm.ID + "\x00" + metric.Instance
		if c.vmNICPerfSamples == nil {
			c.vmNICPerfSamples = make(map[string]*vmNICPerfSample)
		}
		sample := c.vmNICPerfSamples[key]
		if sample == nil {
			sample = &vmNICPerfSample{
				vm:       vm,
				instance: metric.Instance,
				values:   make(map[string]int64),
			}
			c.vmNICPerfSamples[key] = sample
		}
		sample.values[metricName] = metric.Value[0]
	}
}

func (c *Collector) writeVMNICPerformanceMetrics(meter metrix.SnapshotMeter) {
	if !c.CollectVMNICPerformance || len(c.vmNICPerfSamples) == 0 || c.MaxVMNICs < 1 {
		return
	}

	m := c.vmNICMatcher
	if m == nil {
		m = matcher.TRUE()
	}

	count := 0
	for _, sample := range sortedVMNICPerfSamples(c.vmNICPerfSamples) {
		if !vmNICInstanceMatches(m, sample.instance) {
			continue
		}
		if count >= c.MaxVMNICs {
			return
		}
		count++

		labels := meter.LabelSet(c.vmNICPerformanceLabels(sample.vm, sample.instance)...)
		for metricName, value := range sample.values {
			if gauge := c.mx.gauge(metricName); gauge != nil {
				gauge.Observe(metrix.SampleValue(value), labels)
			}
		}
	}
}

func (c *Collector) vmNICPerformanceLabels(vm *rs.VM, instance string) []metrix.Label {
	labels := []metrix.Label{
		{Key: "id", Value: vm.ID},
		{Key: "datacenter", Value: vm.Hier.DC.Name},
		{Key: "cluster", Value: vm.Hier.Cluster.Name},
		{Key: "host", Value: vm.Hier.Host.Name},
		{Key: "vm", Value: vm.Name},
		{Key: vmNICLabel, Value: instance},
		{Key: vmNICInstanceLabel, Value: instance},
	}
	labels = append(labels, c.resourceEnrichmentLabels(vm.ID)...)
	return labels
}

func vmNICInstanceMatches(m matcher.Matcher, instance string) bool {
	return m.MatchString(instance) || m.MatchString("interface:"+instance) || m.MatchString("instance:"+instance)
}

func sortedVMNICPerfSamples(samples map[string]*vmNICPerfSample) []*vmNICPerfSample {
	out := make([]*vmNICPerfSample, 0, len(samples))
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
