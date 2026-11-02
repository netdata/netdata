// SPDX-License-Identifier: GPL-3.0-or-later

package meraki

import "github.com/netdata/netdata/go/plugins/pkg/metrix"

var availabilityStates = []string{"alerting", "dormant", "offline", "online"}

// OrganizationLabelKeys and DeviceLabelKeys describe chart labels documented in metadata.yaml.
var OrganizationLabelKeys = []string{"organization_id", "organization_name"}

var DeviceLabelKeys = []string{
	"organization_id",
	"organization_name",
	"serial",
	"device_name",
	"model",
	"product_type",
	"network_id",
	"network_name",
}

type collectorMetrics struct {
	meter    metrix.SnapshotMeter
	assigned metrix.SnapshotGauge
}

func newCollectorMetrics(store metrix.CollectorStore) *collectorMetrics {
	meter := store.Write().SnapshotMeter("")
	// Fixed descriptors; scoped bindings below live for only the current cycle.
	meter.Gauge("device_assigned")
	meter.StateSet(
		"device_availability_status",
		metrix.WithStateSetMode(metrix.ModeEnum),
		metrix.WithStateSetStates(availabilityStates...),
	)
	return &collectorMetrics{
		meter:    meter,
		assigned: meter.Gauge("organization_assigned_devices"),
	}
}
