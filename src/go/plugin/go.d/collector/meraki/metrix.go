// SPDX-License-Identifier: GPL-3.0-or-later

package meraki

import "github.com/netdata/netdata/go/plugins/pkg/metrix"

const (
	metricOrganizationAssignedDevices = "organization_assigned_devices"
	metricDeviceAssigned              = "device_assigned"
	metricDeviceAvailabilityStatus    = "device_availability_status"
)

// availabilityStates are the device states Meraki documents.
var availabilityStates = []string{"alerting", "dormant", "offline", "online"}

var availabilityStateSet = []metrix.InstrumentOption{
	metrix.WithStateSetMode(metrix.ModeEnum),
	metrix.WithStateSetStates(availabilityStates...),
}

// collectorMetrics holds the organization instrument. Device instruments are
// bound to a device's host scope and labels every cycle instead of being kept in
// a Vec: a Vec caches every label tuple it has seen, and device and network
// names change.
type collectorMetrics struct {
	meter                metrix.SnapshotMeter
	organizationAssigned metrix.SnapshotGauge
}

func newCollectorMetrics(store metrix.CollectorStore) *collectorMetrics {
	meter := store.Write().SnapshotMeter("")
	return &collectorMetrics{
		meter:                meter,
		organizationAssigned: meter.Gauge(metricOrganizationAssignedDevices),
	}
}
