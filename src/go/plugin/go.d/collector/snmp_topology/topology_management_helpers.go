// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"strings"
)

func normalizeTopologyDevice(dev topologyDevice) topologyDevice {
	if dev.ChartIDPrefix == "" {
		dev.ChartIDPrefix = topologyProfileChartIDPrefix
	}
	if dev.ChartContextPrefix == "" {
		dev.ChartContextPrefix = topologyProfileChartContextPrefix
	}
	if dev.ManagementIP == "" && len(dev.ManagementAddresses) > 0 {
		if ip := pickManagementIP(dev.ManagementAddresses); ip != "" {
			dev.ManagementIP = ip
		}
	}
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
	if value := topologyMetadataValue(dev.Labels, topologyMetadataAliasSysDescr); value != "" && dev.SysDescr == "" {
		dev.SysDescr = value
	}
	if value := topologyMetadataValue(dev.Labels, topologyMetadataAliasSysContact); value != "" && dev.SysContact == "" {
		dev.SysContact = value
	}
	if value := topologyMetadataValue(dev.Labels, topologyMetadataAliasSysLocation); value != "" && dev.SysLocation == "" {
		dev.SysLocation = value
	}
	if value := topologyMetadataValue(dev.Labels, topologyMetadataAliasVendor); value != "" && dev.Vendor == "" {
		dev.Vendor = value
	}
	if value := topologyMetadataValue(dev.Labels, topologyMetadataAliasModel); value != "" && dev.Model == "" {
		dev.Model = value
	}
	if dev.SysUptime <= 0 {
		if value := topologyMetadataValue(dev.Labels, topologyMetadataAliasSysUptime); value != "" {
			dev.SysUptime = parsePositiveInt64(value)
		}
	}
	if value := topologyMetadataValue(dev.Labels, topologyMetadataAliasSerial); value != "" && dev.SerialNumber == "" {
		dev.SerialNumber = value
		setTopologyMetadataLabelIfMissing(dev.Labels, "serial_number", value)
	}
	if value := topologyMetadataValue(dev.Labels, topologyMetadataAliasSoftware); value != "" && dev.SoftwareVersion == "" {
		dev.SoftwareVersion = value
		setTopologyMetadataLabelIfMissing(dev.Labels, "software_version", value)
	}
	if value := topologyMetadataValue(dev.Labels, topologyMetadataAliasFirmware); value != "" && dev.FirmwareVersion == "" {
		dev.FirmwareVersion = value
		setTopologyMetadataLabelIfMissing(dev.Labels, "firmware_version", value)
	}
	if value := topologyMetadataValue(dev.Labels, topologyMetadataAliasHardware); value != "" && dev.HardwareVersion == "" {
		dev.HardwareVersion = value
		setTopologyMetadataLabelIfMissing(dev.Labels, "hardware_version", value)
	}
	return dev
}

func topologyDeviceKey(dev topologyDevice) string {
	if dev.ChassisID == "" {
		return ""
	}
	return dev.ChassisIDType + ":" + dev.ChassisID
}

func normalizeLLDPSubtype(value string, mapping map[string]string) string {
	if v, ok := mapping[value]; ok {
		return v
	}
	return value
}
