// SPDX-License-Identifier: GPL-3.0-or-later

package meraki

import "github.com/netdata/netdata/go/plugins/pkg/metrix"

func (c *Collector) writeMetrics(organizationName string, inventory []device) {
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
}
