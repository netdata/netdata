// SPDX-License-Identifier: GPL-3.0-or-later

package merakiapi

// The response types keep only the fields the collector reads. IDs stay opaque
// strings, and missing fields decode as empty values: the provider does not mark
// them required, so callers validate what they rely on. Measurements are
// pointers, so a missing or null value stays apart from a measured zero.

// Organization is an organization visible to the credential.
type Organization struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Network is a network of an organization.
type Network struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organizationId"`
	Name           string `json:"name"`
}

// Device is a device assigned to a network. Unused organization inventory is
// not listed.
type Device struct {
	Serial      string `json:"serial"`
	NetworkID   string `json:"networkId"`
	Name        string `json:"name"`
	Model       string `json:"model"`
	ProductType string `json:"productType"`
}

// DeviceAvailability is the provider-reported status of a device. Status keeps
// the provider string as-is, including unknown or empty values. Meraki refreshes
// availability every five minutes and supplies no sample timestamp.
type DeviceAvailability struct {
	Serial  string     `json:"serial"`
	Status  string     `json:"status"`
	Network NetworkRef `json:"network"`
}

// NetworkRef references a network by ID.
type NetworkRef struct {
	ID string `json:"id"`
}

// SwitchPortUsageHistory is the port usage of one switch, interval by interval.
type SwitchPortUsageHistory struct {
	Serial  string            `json:"serial"`
	Network NetworkRef        `json:"network"`
	Ports   []SwitchPortUsage `json:"ports"`
}

// SwitchPortUsage is the usage of one port. PortID is opaque: usually the port
// number, with the slot and module type for a port on a module.
type SwitchPortUsage struct {
	PortID    string                    `json:"portId"`
	Intervals []SwitchPortUsageInterval `json:"intervals"`
}

// SwitchPortUsageInterval is the average bandwidth of one port during one
// interval, in kilobits/s: upstream is the data the port sent, downstream the
// data it received. Start and End are RFC 3339 timestamps.
type SwitchPortUsageInterval struct {
	Start     string `json:"startTs"`
	End       string `json:"endTs"`
	Bandwidth struct {
		Usage struct {
			Upstream   *float64 `json:"upstream"`
			Downstream *float64 `json:"downstream"`
		} `json:"usage"`
	} `json:"bandwidth"`
}

// SwitchPortPackets is the packet statistics of one port over the requested
// timespan, by category ("Total", "Broadcast", "CRC align errors", ...).
type SwitchPortPackets struct {
	PortID     string           `json:"portId"`
	Categories []PacketCategory `json:"packets"`
}

// PacketCategory is the average packet rate of one category, in packets/s.
type PacketCategory struct {
	Description string `json:"desc"`
	RatePerSec  struct {
		Sent     *float64 `json:"sent"`
		Received *float64 `json:"recv"`
	} `json:"ratePerSec"`
}

// ApplianceUplinkUsage is the uplink usage of one MX or Z network. When more
// than one appliance was active during the timespan, Meraki aggregates their
// bytes by interface, so the serial it reports is not kept.
type ApplianceUplinkUsage struct {
	NetworkID string        `json:"networkId"`
	ByUplink  []UplinkUsage `json:"byUplink"`
}

// UplinkUsage is the bytes one uplink interface sent and received during the
// timespan.
type UplinkUsage struct {
	Interface string   `json:"interface"`
	Sent      *float64 `json:"sent"`
	Received  *float64 `json:"received"`
}

// ApplianceUplinkStatus reports one appliance's uplinks. LastReportedAt is the
// device's last report time, not the time an interface changed state.
type ApplianceUplinkStatus struct {
	Serial         string                 `json:"serial"`
	NetworkID      string                 `json:"networkId"`
	LastReportedAt string                 `json:"lastReportedAt"`
	Uplinks        []ApplianceUplinkState `json:"uplinks"`
}

// ApplianceUplinkState keeps the provider's interface and status strings,
// including unknown values, for the collector's trust checks.
type ApplianceUplinkState struct {
	Interface string `json:"interface"`
	Status    string `json:"status"`
}

// UplinkLossAndLatency is one MX uplink's ICMP measurements to a destination.
// The API's ip query filter identifies a destination, despite its response
// schema describing that same field as the uplink IP.
type UplinkLossAndLatency struct {
	Serial        string               `json:"serial"`
	NetworkID     string               `json:"networkId"`
	Uplink        string               `json:"uplink"`
	DestinationIP string               `json:"ip"`
	Samples       []UplinkHealthSample `json:"timeSeries"`
}

// UplinkHealthSample is one provider-timestamped observation, with no supplied
// packet counts or interval duration from which to derive weighted averages.
type UplinkHealthSample struct {
	Timestamp   string   `json:"ts"`
	LossPercent *float64 `json:"lossPercent"`
	LatencyMS   *float64 `json:"latencyMs"`
}
