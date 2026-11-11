// SPDX-License-Identifier: GPL-3.0-or-later

package meraki

import "github.com/netdata/netdata/go/plugins/pkg/metrix"

func (c *Collector) writeMetrics(organizationName string, inventory []device, telemetry telemetrySnapshot) {
	m := c.metrics
	orgLabels := m.meter.LabelSet(
		metrix.Label{
			Key:   "organization_id",
			Value: c.OrganizationID,
		},
		metrix.Label{
			Key:   "organization_name",
			Value: organizationName,
		},
	)
	m.organizationAssigned.Observe(metrix.SampleValue(len(inventory)), orgLabels)

	deviceMeters := make(map[string]metrix.SnapshotMeter, len(inventory))
	unknownStatus := 0
	for _, dev := range inventory {
		devLabels := m.meter.LabelSet(
			metrix.Label{
				Key:   "serial",
				Value: dev.serial,
			},
			metrix.Label{
				Key:   "device_name",
				Value: dev.name,
			},
			metrix.Label{
				Key:   "model",
				Value: dev.model,
			},
			metrix.Label{
				Key:   "product_type",
				Value: dev.productType,
			},
			metrix.Label{
				Key:   "network_id",
				Value: dev.networkID,
			},
			metrix.Label{
				Key:   "network_name",
				Value: dev.networkName,
			},
		)
		meter := m.meter.WithHostScope(deviceHostScope(c.OrganizationID, dev)).WithLabelSet(orgLabels, devLabels)
		deviceMeters[dev.serial] = meter

		// Always written, so the device node exists even without a status.
		meter.Gauge(metricDeviceAssigned).Observe(1)
		if dev.status == "" {
			unknownStatus++
			continue
		}
		meter.StateSet(metricDeviceAvailabilityStatus, availabilityStateSet...).Enable(dev.status)
	}
	if unknownStatus > 0 {
		c.Debugf(
			"Meraki availability omitted for %d assigned devices with missing, unknown or inconsistent status",
			unknownStatus,
		)
	}

	c.writeTrafficMetrics(orgLabels, deviceMeters, telemetry.traffic)
	c.writeHealthMetrics(deviceMeters, telemetry.health)
}

// writeTrafficMetrics writes switch port series on their switch's node and
// uplink series on the job's node. Uplinks carry no device labels: their bytes
// can come from more than one appliance.
func (c *Collector) writeTrafficMetrics(
	orgLabels metrix.LabelSet,
	deviceMeters map[string]metrix.SnapshotMeter,
	traffic trafficSnapshot,
) {
	now := c.now()
	for _, port := range traffic.ports {
		meter, ok := deviceMeters[port.serial]
		age := now.Sub(port.end)
		if !ok || age < 0 {
			continue
		}
		meter = meter.WithLabels(metrix.Label{
			Key:   "port_id",
			Value: port.portID,
		})
		writeRates(meter, metricSwitchPortTrafficSent, metricSwitchPortTrafficReceived, port.rates)
		meter.Gauge(metricSwitchPortTrafficIntervalAge, metrix.WithFloat(true)).
			Observe(metrix.SampleValue(age.Seconds()))
	}
	for _, port := range traffic.packets {
		meter, ok := deviceMeters[port.serial]
		if !ok {
			continue
		}
		meter = meter.WithLabels(metrix.Label{
			Key:   "port_id",
			Value: port.portID,
		})
		writeRates(meter, metricSwitchPortPacketsSent, metricSwitchPortPacketsReceived, port.rates)
	}
	for _, uplink := range traffic.uplinks {
		meter := c.metrics.meter.WithLabelSet(orgLabels).WithLabels(
			metrix.Label{
				Key:   "network_id",
				Value: uplink.networkID,
			},
			metrix.Label{
				Key:   "network_name",
				Value: uplink.networkName,
			},
			metrix.Label{
				Key:   "interface",
				Value: uplink.iface,
			},
		)
		writeRates(meter, metricApplianceUplinkTrafficSent, metricApplianceUplinkTrafficReceived, uplink.rates)
	}
}

// writeRates writes each side of rates that holds a value.
func writeRates(meter metrix.SnapshotMeter, sentMetric, receivedMetric string, rates directionalRate) {
	if rates.sent.ok {
		meter.Gauge(sentMetric, metrix.WithFloat(true)).Observe(metrix.SampleValue(rates.sent.value))
	}
	if rates.received.ok {
		meter.Gauge(receivedMetric, metrix.WithFloat(true)).Observe(metrix.SampleValue(rates.received.value))
	}
}
