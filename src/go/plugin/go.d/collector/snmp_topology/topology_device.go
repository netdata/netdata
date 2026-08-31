// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"maps"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"
)

func normalizeTopologyDevice(dev topologymodel.Device) topologymodel.Device {
	// dev is a shallow copy of the caller's value, so dev.Labels still aliases
	// the builder's map. Clone it before normalization mutates labels so the
	// immutable generation never shares writable label state with its builder.
	dev.Labels = maps.Clone(dev.Labels)

	if dev.ChartIDPrefix == "" {
		dev.ChartIDPrefix = topologyProfileChartIDPrefix
	}
	if dev.ChartContextPrefix == "" {
		dev.ChartContextPrefix = topologyProfileChartContextPrefix
	}
	dev.ManagementIP = normalizeEligibleManagementIP(dev.ManagementIP)
	if len(dev.Capabilities) == 0 {
		if len(dev.CapabilitiesEnabled) > 0 {
			dev.Capabilities = dev.CapabilitiesEnabled
		} else if len(dev.CapabilitiesSupported) > 0 {
			dev.Capabilities = dev.CapabilitiesSupported
		}
	}
	if dev.Labels == nil {
		dev.Labels = make(map[string]string)
	}
	if strings.TrimSpace(dev.Labels["type"]) == "" && len(dev.Capabilities) > 0 {
		dev.Labels["type"] = inferCategoryFromCapabilities(dev.Capabilities)
	}
	if dev.ChassisID == "" && dev.ManagementIP != "" {
		dev.ChassisID = dev.ManagementIP
		dev.ChassisIDType = "management_ip"
	}
	if dev.ChassisID != "" && dev.ChassisIDType == "" {
		dev.ChassisIDType = "unknown"
	}
	dev.Vendor, dev.Model = ddsnmp.ResolveDeviceIdentity(dev.Vendor, dev.Model, nil, dev.Labels)
	metadata := newTopologyMetadataIndex(dev.Labels)
	if value := metadata.value(topologyMetadataAliasSysDescr); value != "" && dev.SysDescr == "" {
		dev.SysDescr = value
	}
	if value := metadata.value(topologyMetadataAliasSysContact); value != "" && dev.SysContact == "" {
		dev.SysContact = value
	}
	if value := metadata.value(topologyMetadataAliasSysLocation); value != "" && dev.SysLocation == "" {
		dev.SysLocation = value
	}
	if value := metadata.value(topologyMetadataAliasVendor); value != "" && dev.Vendor == "" {
		dev.Vendor = value
	}
	if value := metadata.value(topologyMetadataAliasModel); value != "" && dev.Model == "" {
		dev.Model = value
	}
	if value := topologyutil.NormalizeTopologyRouterID(dev.Labels[tagOSPFRouterID]); value != "" && dev.OSPFRouterID == "" {
		dev.OSPFRouterID = value
	}
	if value := topologyutil.NormalizeTopologyRouterID(dev.OSPFRouterID); value != "" {
		dev.OSPFRouterID = value
		setTopologyMetadataLabelIfMissing(dev.Labels, tagOSPFRouterID, value)
	}
	if dev.SysUptime <= 0 {
		if value := metadata.value(topologyMetadataAliasSysUptime); value != "" {
			dev.SysUptime = topologyutil.ParsePositiveInt64(value)
		}
	}
	if value := metadata.value(topologyMetadataAliasSerial); value != "" && dev.SerialNumber == "" {
		dev.SerialNumber = value
		setTopologyMetadataLabelIfMissing(dev.Labels, "serial_number", value)
	}
	if value := metadata.value(topologyMetadataAliasSoftware); value != "" && dev.SoftwareVersion == "" {
		dev.SoftwareVersion = value
		setTopologyMetadataLabelIfMissing(dev.Labels, "software_version", value)
	}
	if value := metadata.value(topologyMetadataAliasFirmware); value != "" && dev.FirmwareVersion == "" {
		dev.FirmwareVersion = value
		setTopologyMetadataLabelIfMissing(dev.Labels, "firmware_version", value)
	}
	if value := metadata.value(topologyMetadataAliasHardware); value != "" && dev.HardwareVersion == "" {
		dev.HardwareVersion = value
		setTopologyMetadataLabelIfMissing(dev.Labels, "hardware_version", value)
	}
	return dev
}

func topologyDeviceKey(dev topologymodel.Device) string {
	if dev.ChassisID == "" {
		return ""
	}
	return dev.ChassisIDType + ":" + dev.ChassisID
}
