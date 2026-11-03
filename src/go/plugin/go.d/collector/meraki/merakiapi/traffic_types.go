// SPDX-License-Identifier: GPL-3.0-or-later

package merakiapi

// SwitchPortUsageHistory contains a switch's interval observations, not inventory.
type SwitchPortUsageHistory struct {
	Serial  string            `json:"serial"`
	Network NetworkRef        `json:"network"`
	Ports   []SwitchPortUsage `json:"ports"`
}

// SwitchPortUsage identifies a port by its opaque local identifier.
type SwitchPortUsage struct {
	PortID    string                    `json:"portId"`
	Intervals []SwitchPortUsageInterval `json:"intervals"`
}

// SwitchPortUsageInterval carries source timestamps and average kilobits/second.
type SwitchPortUsageInterval struct {
	StartTs   string `json:"startTs"`
	EndTs     string `json:"endTs"`
	Bandwidth struct {
		Usage struct {
			Upstream   *float64 `json:"upstream"`
			Downstream *float64 `json:"downstream"`
		} `json:"usage"`
	} `json:"bandwidth"`
}

// SwitchPortPacketStatus contains packet rates for a port over a requested window.
type SwitchPortPacketStatus struct {
	PortID  string             `json:"portId"`
	Packets []SwitchPacketRate `json:"packets"`
}

// SwitchPacketRate preserves the provider's packet category and packets/second.
type SwitchPacketRate struct {
	Description string `json:"desc"`
	RatePerSec  struct {
		Sent *float64 `json:"sent"`
		Recv *float64 `json:"recv"`
	} `json:"ratePerSec"`
}

// ApplianceUplinkUsage is network usage aggregated across active appliances.
// A response serial cannot establish per-device ownership and is not retained.
type ApplianceUplinkUsage struct {
	NetworkID string        `json:"networkId"`
	ByUplink  []UplinkUsage `json:"byUplink"`
}

// UplinkUsage contains sent and received bytes during the requested window.
type UplinkUsage struct {
	Interface string   `json:"interface"`
	Sent      *float64 `json:"sent"`
	Received  *float64 `json:"received"`
}
