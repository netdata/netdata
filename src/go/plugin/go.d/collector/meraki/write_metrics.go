// SPDX-License-Identifier: GPL-3.0-or-later

package meraki

import "github.com/netdata/netdata/go/plugins/pkg/metrix"

func (c *Collector) writeMetrics(organizationName string, devices []deviceSnapshot) {
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
	m.assigned.Observe(metrix.SampleValue(len(devices)), orgLabels)
	missing := 0
	for _, d := range devices {
		labels := m.meter.LabelSet(
			metrix.Label{
				Key:   "serial",
				Value: d.serial,
			},
			metrix.Label{
				Key:   "device_name",
				Value: d.name,
			},
			metrix.Label{
				Key:   "model",
				Value: d.model,
			},
			metrix.Label{
				Key:   "product_type",
				Value: d.productType,
			},
			metrix.Label{
				Key:   "network_id",
				Value: d.networkID,
			},
			metrix.Label{
				Key:   "network_name",
				Value: d.networkName,
			},
		)
		meter := m.meter.WithHostScope(deviceHostScope(c.OrganizationID, d)).WithLabelSet(orgLabels, labels)
		meter.Gauge("device_assigned").Observe(1)
		if d.status == "" {
			missing++
			continue
		}
		meter.StateSet("device_availability_status", metrix.WithStateSetMode(metrix.ModeEnum), metrix.WithStateSetStates(availabilityStates...)).
			Enable(d.status)
	}
	if missing > 0 {
		c.Debugf(
			"Meraki availability omitted for %d assigned devices with missing, unknown or inconsistent status",
			missing,
		)
	}
}
