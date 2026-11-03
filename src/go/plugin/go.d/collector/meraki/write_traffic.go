// SPDX-License-Identifier: GPL-3.0-or-later

package meraki

import "github.com/netdata/netdata/go/plugins/pkg/metrix"

func (c *Collector) writeTrafficMetrics(organizationName string, inventory []device, snapshot trafficSnapshot) {
	if len(snapshot.ports)+len(snapshot.packets)+len(snapshot.uplinks) == 0 {
		return
	}
	base := c.metrics.meter
	orgLabels := c.organizationLabels(organizationName)
	deviceMeters := make(map[string]metrix.SnapshotMeter)
	if len(snapshot.ports)+len(snapshot.packets) > 0 {
		for _, dev := range inventory {
			if dev.productType == "switch" {
				deviceMeters[dev.serial] = c.deviceMeter(dev, orgLabels)
			}
		}
	}
	now := c.now()
	for _, port := range snapshot.ports {
		meter, ok := deviceMeters[port.serial]
		if !ok || port.end.After(now) {
			continue
		}
		meter = meter.WithLabelSet(base.LabelSet(metrix.Label{Key: "port_id", Value: port.portID}))
		observeRate(meter, metricPortTrafficSent, metricPortTrafficReceived, port.directionalRate)
		meter.Gauge(metricPortTrafficIntervalAge, metrix.WithFloat(true)).Observe(metrix.SampleValue(now.Sub(port.end).Seconds()))
	}
	for _, port := range snapshot.packets {
		meter, ok := deviceMeters[port.serial]
		if !ok {
			continue
		}
		meter = meter.WithLabelSet(base.LabelSet(metrix.Label{Key: "port_id", Value: port.portID}))
		observeRate(meter, metricPortPacketsSent, metricPortPacketsReceived, port.directionalRate)
	}
	for _, uplink := range snapshot.uplinks {
		labels := base.LabelSet(
			metrix.Label{Key: "network_id", Value: uplink.networkID},
			metrix.Label{Key: "network_name", Value: uplink.networkName},
			metrix.Label{Key: "interface", Value: uplink.iface},
		)
		observeRate(base.WithLabelSet(orgLabels, labels), metricUplinkTrafficSent, metricUplinkTrafficReceived, uplink.directionalRate)
	}
}

func observeRate(meter metrix.SnapshotMeter, sent, received string, rate directionalRate) {
	if rate.sent != nil {
		meter.Gauge(sent, metrix.WithFloat(true)).Observe(metrix.SampleValue(*rate.sent))
	}
	if rate.received != nil {
		meter.Gauge(received, metrix.WithFloat(true)).Observe(metrix.SampleValue(*rate.received))
	}
}
