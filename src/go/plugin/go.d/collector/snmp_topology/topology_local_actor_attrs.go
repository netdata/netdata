// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import "strings"

func populateLocalActorAttributes(attrs map[string]any, local topologyDevice) map[string]any {
	if attrs == nil {
		attrs = make(map[string]any)
	}
	if len(local.ManagementAddresses) > 0 {
		attrs["management_addresses"] = local.ManagementAddresses
	}
	if len(local.Capabilities) > 0 {
		attrs["capabilities"] = local.Capabilities
	}
	if len(local.CapabilitiesSupported) > 0 {
		attrs["capabilities_supported"] = local.CapabilitiesSupported
	}
	if len(local.CapabilitiesEnabled) > 0 {
		attrs["capabilities_enabled"] = local.CapabilitiesEnabled
	}
	if sysDescr := strings.TrimSpace(local.SysDescr); sysDescr != "" {
		attrs["sys_descr"] = sysDescr
	}
	if sysContact := strings.TrimSpace(local.SysContact); sysContact != "" {
		attrs["sys_contact"] = sysContact
	}
	if sysLocation := strings.TrimSpace(local.SysLocation); sysLocation != "" {
		attrs["sys_location"] = sysLocation
	}
	if local.SysUptime > 0 {
		attrs["sys_uptime"] = local.SysUptime
	}
	if vendor := strings.TrimSpace(local.Vendor); vendor != "" {
		attrs["vendor"] = vendor
		attrs["vendor_source"] = "snmp"
		attrs["vendor_confidence"] = "high"
	}
	if model := strings.TrimSpace(local.Model); model != "" {
		attrs["model"] = model
	}
	if serial := strings.TrimSpace(local.SerialNumber); serial != "" {
		attrs["serial_number"] = serial
	}
	if software := strings.TrimSpace(local.SoftwareVersion); software != "" {
		attrs["software_version"] = software
	}
	if firmware := strings.TrimSpace(local.FirmwareVersion); firmware != "" {
		attrs["firmware_version"] = firmware
	}
	if hardware := strings.TrimSpace(local.HardwareVersion); hardware != "" {
		attrs["hardware_version"] = hardware
	}
	if managementIP := normalizeIPAddress(local.ManagementIP); managementIP != "" {
		attrs["management_ip"] = managementIP
	}
	if netdataHostID := strings.TrimSpace(local.NetdataHostID); netdataHostID != "" {
		attrs["netdata_host_id"] = netdataHostID
	}
	if chartIDPrefix := strings.TrimSpace(local.ChartIDPrefix); chartIDPrefix != "" {
		attrs["chart_id_prefix"] = chartIDPrefix
	}
	if chartContextPrefix := strings.TrimSpace(local.ChartContextPrefix); chartContextPrefix != "" {
		attrs["chart_context_prefix"] = chartContextPrefix
	}
	if len(local.DeviceCharts) > 0 {
		attrs["device_charts"] = mapStringStringToAny(local.DeviceCharts)
	}
	return attrs
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
