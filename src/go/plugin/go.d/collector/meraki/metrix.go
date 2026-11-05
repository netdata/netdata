// SPDX-License-Identifier: GPL-3.0-or-later

package meraki

import "github.com/netdata/netdata/go/plugins/pkg/metrix"

const (
	metricOrganizationAssignedDevices    = "organization_assigned_devices"
	metricDeviceAssigned                 = "device_assigned"
	metricDeviceAvailabilityStatus       = "device_availability_status"
	metricSwitchPortTrafficSent          = "switch_port_traffic_sent"
	metricSwitchPortTrafficReceived      = "switch_port_traffic_received"
	metricSwitchPortTrafficIntervalAge   = "switch_port_traffic_interval_age"
	metricSwitchPortPacketsSent          = "switch_port_packets_sent"
	metricSwitchPortPacketsReceived      = "switch_port_packets_received"
	metricApplianceUplinkTrafficSent     = "appliance_uplink_traffic_sent"
	metricApplianceUplinkTrafficReceived = "appliance_uplink_traffic_received"
	metricApplianceReportAge             = "appliance_report_age"
	metricApplianceUplinkStatus          = "appliance_uplink_status"
	metricApplianceUplinkPacketLoss      = "appliance_uplink_packet_loss"
	metricApplianceUplinkLatency         = "appliance_uplink_latency"
	metricApplianceUplinkSampleAge       = "appliance_uplink_sample_age"
)

// availabilityStates are the device states Meraki documents.
var availabilityStates = []string{"alerting", "dormant", "offline", "online"}

var availabilityStateSet = []metrix.InstrumentOption{
	metrix.WithStateSetMode(metrix.ModeEnum),
	metrix.WithStateSetStates(availabilityStates...),
}

// uplinkStates are the uplink states Meraki documents, with "not connected"
// written as not_connected.
var uplinkStates = []string{"active", "connecting", "failed", "not_connected", "ready"}

var uplinkStateSet = []metrix.InstrumentOption{
	metrix.WithStateSetMode(metrix.ModeEnum),
	metrix.WithStateSetStates(uplinkStates...),
}

// collectorMetrics holds the organization instrument. Device, port and uplink
// instruments are bound to their host scope and labels every cycle instead of
// being kept in a Vec: a Vec caches every label tuple it has seen, and device and
// network names change.
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
