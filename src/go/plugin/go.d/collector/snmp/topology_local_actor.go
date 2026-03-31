// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/topology"
	topologyengine "github.com/netdata/netdata/go/plugins/pkg/topology/engine"
)

func augmentLocalActorFromCache(data *topologyData, local topologyDevice) {
	if data == nil || len(data.Actors) == 0 {
		return
	}

	for i := range data.Actors {
		actor := &data.Actors[i]
		if !topologyengine.IsDeviceActorType(actor.ActorType) {
			continue
		}
		if !matchLocalTopologyActor(actor.Match, local) {
			continue
		}

		attrs := actor.Attributes
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
		if len(local.InterfaceCharts) > 0 {
			if statuses, ok := attrs["if_statuses"]; ok && statuses != nil {
				attrs["if_statuses"] = enrichTopologyInterfaceStatusesWithChartRefs(statuses, local.InterfaceCharts)
			}
			if actor.Tables != nil {
				if portRows, ok := actor.Tables["ports"]; ok && len(portRows) > 0 {
					enrichTopologyTableRowsWithChartRefs(portRows, local.InterfaceCharts)
				}
			}
		}

		actor.Attributes = pruneNilAttributes(attrs)
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
		return
	}
}

func matchLocalTopologyActor(match topology.Match, local topologyDevice) bool {
	localChassisID := strings.TrimSpace(local.ChassisID)
	if localChassisID != "" {
		for _, chassisID := range match.ChassisIDs {
			if strings.EqualFold(strings.TrimSpace(chassisID), localChassisID) {
				return true
			}
		}
	}

	localSysName := strings.TrimSpace(local.SysName)
	if localSysName != "" && strings.EqualFold(strings.TrimSpace(match.SysName), localSysName) {
		return true
	}

	localIP := normalizeIPAddress(local.ManagementIP)
	if localIP != "" {
		for _, ip := range match.IPAddresses {
			if normalizeIPAddress(ip) == localIP {
				return true
			}
		}
	}

	return false
}

func enrichTopologyInterfaceStatusesWithChartRefs(
	statuses any,
	interfaceCharts map[string]topologyInterfaceChartRef,
) any {
	if len(interfaceCharts) == 0 || statuses == nil {
		return statuses
	}

	lookup := make(map[string]topologyInterfaceChartRef, len(interfaceCharts))
	for ifName, ref := range interfaceCharts {
		ifName = strings.ToLower(strings.TrimSpace(ifName))
		if ifName == "" {
			continue
		}
		if strings.TrimSpace(ref.ChartIDSuffix) == "" {
			ref.ChartIDSuffix = ifName
		}
		ref.AvailableMetrics = deduplicateSortedStrings(ref.AvailableMetrics)
		lookup[ifName] = ref
	}
	if len(lookup) == 0 {
		return statuses
	}

	switch typed := statuses.(type) {
	case []map[string]any:
		for _, status := range typed {
			ifName := strings.ToLower(strings.TrimSpace(fmt.Sprint(status["if_name"])))
			if ifName == "" {
				continue
			}
			ref, ok := lookup[ifName]
			if !ok {
				continue
			}
			status["chart_id_suffix"] = ref.ChartIDSuffix
			if len(ref.AvailableMetrics) > 0 {
				status["available_metrics"] = ref.AvailableMetrics
			}
		}
		return typed
	case []any:
		for i := range typed {
			status, ok := typed[i].(map[string]any)
			if !ok || status == nil {
				continue
			}
			ifName := strings.ToLower(strings.TrimSpace(fmt.Sprint(status["if_name"])))
			if ifName == "" {
				continue
			}
			ref, ok := lookup[ifName]
			if !ok {
				continue
			}
			status["chart_id_suffix"] = ref.ChartIDSuffix
			if len(ref.AvailableMetrics) > 0 {
				status["available_metrics"] = ref.AvailableMetrics
			}
			typed[i] = status
		}
		return typed
	default:
		return statuses
	}
}

func enrichTopologyTableRowsWithChartRefs(rows []map[string]any, interfaceCharts map[string]topologyInterfaceChartRef) {
	if len(interfaceCharts) == 0 || len(rows) == 0 {
		return
	}

	lookup := make(map[string]topologyInterfaceChartRef, len(interfaceCharts))
	for ifName, ref := range interfaceCharts {
		ifName = strings.ToLower(strings.TrimSpace(ifName))
		if ifName == "" {
			continue
		}
		if strings.TrimSpace(ref.ChartIDSuffix) == "" {
			ref.ChartIDSuffix = ifName
		}
		ref.AvailableMetrics = deduplicateSortedStrings(ref.AvailableMetrics)
		lookup[ifName] = ref
	}
	if len(lookup) == 0 {
		return
	}

	for _, row := range rows {
		name := strings.ToLower(strings.TrimSpace(fmt.Sprint(row["name"])))
		if name == "" {
			continue
		}
		ref, ok := lookup[name]
		if !ok {
			continue
		}
		row["chart_id_suffix"] = ref.ChartIDSuffix
		if len(ref.AvailableMetrics) > 0 {
			row["available_metrics"] = ref.AvailableMetrics
		}
	}
}
