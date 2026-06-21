// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"
	"strings"
)

func topologySNMPActorDetailFromDevice(local topologyDevice) topologySNMPActorDetail {
	return topologySNMPActorDetail{
		ManagementAddresses:   local.ManagementAddresses,
		Capabilities:          local.Capabilities,
		CapabilitiesSupported: local.CapabilitiesSupported,
		CapabilitiesEnabled:   local.CapabilitiesEnabled,
		SysDescr:              strings.TrimSpace(local.SysDescr),
		SysContact:            strings.TrimSpace(local.SysContact),
		SysLocation:           strings.TrimSpace(local.SysLocation),
		SysUptime:             local.SysUptime,
		Vendor:                strings.TrimSpace(local.Vendor),
		VendorSource:          "snmp",
		VendorConfidence:      "high",
		Model:                 strings.TrimSpace(local.Model),
		OSPFRouterID:          topologyutil.NormalizeTopologyRouterID(local.OSPFRouterID),
		SerialNumber:          strings.TrimSpace(local.SerialNumber),
		SoftwareVersion:       strings.TrimSpace(local.SoftwareVersion),
		FirmwareVersion:       strings.TrimSpace(local.FirmwareVersion),
		HardwareVersion:       strings.TrimSpace(local.HardwareVersion),
		ManagementIP:          topologyutil.NormalizeIPAddress(local.ManagementIP),
		NetdataHostID:         strings.TrimSpace(local.NetdataHostID),
		ChartIDPrefix:         strings.TrimSpace(local.ChartIDPrefix),
		ChartContextPrefix:    strings.TrimSpace(local.ChartContextPrefix),
		DeviceCharts:          local.DeviceCharts,
		InterfaceCharts:       local.InterfaceCharts,
	}
}

func applyLocalActorLabels(actor *topologyActor, local topologyDevice) {
	if actor == nil {
		return
	}
	if actor.Labels == nil {
		actor.Labels = make(map[string]string)
	}
	for key, value := range local.Labels {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		actor.Labels[key] = value
	}
}
