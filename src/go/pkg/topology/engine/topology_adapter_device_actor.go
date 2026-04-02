// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/topology"
)

func topologyDeviceInferred(dev Device) bool {
	if len(dev.Labels) == 0 {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(dev.Labels["inferred"])) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func buildDeviceActorMatch(dev Device, reporterAliases []string) topology.Match {
	match := topology.Match{
		SysObjectID: strings.TrimSpace(dev.SysObject),
		SysName:     strings.TrimSpace(dev.Hostname),
	}

	macSet := make(map[string]struct{}, 1+len(reporterAliases))
	chassis := strings.TrimSpace(dev.ChassisID)
	if chassis != "" {
		match.ChassisIDs = []string{chassis}
		if mac := normalizeMAC(chassis); mac != "" {
			macSet[mac] = struct{}{}
		}
	}
	for _, alias := range reporterAliases {
		alias = strings.TrimSpace(alias)
		if alias == "" {
			continue
		}
		if strings.HasPrefix(alias, "mac:") {
			if mac := normalizeMAC(strings.TrimPrefix(alias, "mac:")); mac != "" {
				macSet[mac] = struct{}{}
			}
			continue
		}
		if mac := normalizeMAC(alias); mac != "" {
			macSet[mac] = struct{}{}
		}
	}
	if len(macSet) > 0 {
		match.MacAddresses = sortedTopologySet(macSet)
	}

	if len(dev.Addresses) > 0 {
		ips := make([]string, 0, len(dev.Addresses))
		for _, addr := range dev.Addresses {
			if !addr.IsValid() {
				continue
			}
			ips = append(ips, addr.String())
		}
		match.IPAddresses = uniqueTopologyStrings(ips)
	}

	return match
}

func buildDeviceActorAttributes(
	dev Device,
	localDeviceID string,
	ifaceSummary topologyDeviceInterfaceSummary,
	match topology.Match,
) map[string]any {
	discovered := strings.TrimSpace(localDeviceID) == "" || dev.ID != localDeviceID

	attrs := map[string]any{
		"device_id":              dev.ID,
		"discovered":             discovered,
		"inferred":               topologyDeviceInferred(dev),
		"management_ip":          firstAddress(dev.Addresses),
		"management_addresses":   addressStrings(dev.Addresses),
		"protocols":              labelsCSVToSlice(dev.Labels, "protocols_observed"),
		"protocols_collected":    labelsCSVToSlice(dev.Labels, "protocols_observed"),
		"capabilities":           labelsCSVToSlice(dev.Labels, "capabilities"),
		"capabilities_supported": labelsCSVToSlice(dev.Labels, "capabilities_supported"),
		"capabilities_enabled":   labelsCSVToSlice(dev.Labels, "capabilities_enabled"),
	}
	derivedVendor, derivedPrefix := inferTopologyVendorFromMatch(match)
	if derivedVendor != "" {
		attrs["vendor_derived"] = derivedVendor
		attrs["vendor_derived_source"] = "mac_oui"
		attrs["vendor_derived_confidence"] = "low"
		attrs["vendor_derived_match_prefix"] = derivedPrefix
	}
	if vendor := strings.TrimSpace(dev.Labels["vendor"]); vendor != "" {
		attrs["vendor"] = vendor
		attrs["vendor_source"] = "labels"
		attrs["vendor_confidence"] = "high"
	} else if derivedVendor != "" {
		attrs["vendor"] = derivedVendor
		attrs["vendor_source"] = "mac_oui"
		attrs["vendor_confidence"] = "low"
		attrs["vendor_match_prefix"] = derivedPrefix
	}
	if ifaceSummary.portsTotal > 0 {
		attrs["ports_total"] = ifaceSummary.portsTotal
	}
	if len(ifaceSummary.ifIndexes) > 0 {
		attrs["if_indexes"] = ifaceSummary.ifIndexes
	}
	if len(ifaceSummary.ifNames) > 0 {
		attrs["if_names"] = ifaceSummary.ifNames
	}
	if ifaceSummary.portsUp > 0 {
		attrs["ports_up"] = ifaceSummary.portsUp
	}
	if ifaceSummary.portsDown > 0 {
		attrs["ports_down"] = ifaceSummary.portsDown
	}
	if ifaceSummary.portsAdminDown > 0 {
		attrs["ports_admin_down"] = ifaceSummary.portsAdminDown
	}
	if ifaceSummary.totalBandwidthBps > 0 {
		attrs["total_bandwidth_bps"] = ifaceSummary.totalBandwidthBps
	}
	if ifaceSummary.fdbTotalMACs > 0 {
		attrs["fdb_total_macs"] = ifaceSummary.fdbTotalMACs
	}
	if ifaceSummary.vlanCount > 0 {
		attrs["vlan_count"] = ifaceSummary.vlanCount
	}
	if ifaceSummary.lldpNeighborCount > 0 {
		attrs["lldp_neighbor_count"] = ifaceSummary.lldpNeighborCount
	}
	if ifaceSummary.cdpNeighborCount > 0 {
		attrs["cdp_neighbor_count"] = ifaceSummary.cdpNeighborCount
	}
	if len(ifaceSummary.adminStatusCount) > 0 {
		attrs["if_admin_status_counts"] = ifaceSummary.adminStatusCount
	}
	if len(ifaceSummary.operStatusCount) > 0 {
		attrs["if_oper_status_counts"] = ifaceSummary.operStatusCount
	}
	if len(ifaceSummary.linkModeCount) > 0 {
		attrs["if_link_mode_counts"] = ifaceSummary.linkModeCount
	}
	if len(ifaceSummary.roleCount) > 0 {
		attrs["if_topology_role_counts"] = ifaceSummary.roleCount
	}
	if len(ifaceSummary.portStatuses) > 0 {
		attrs["if_statuses"] = ifaceSummary.portStatuses
	}
	return attrs
}

func buildDeviceActorTables(ifaceSummary topologyDeviceInterfaceSummary) map[string][]map[string]any {
	if len(ifaceSummary.portStatuses) == 0 {
		return nil
	}

	rows := make([]map[string]any, 0, len(ifaceSummary.portStatuses))
	for _, ps := range ifaceSummary.portStatuses {
		row := make(map[string]any, len(ps)+2)
		for k, v := range ps {
			switch k {
			case "if_name":
				row["name"] = v
			case "if_type":
				row["port_type"] = v
			default:
				row[k] = v
			}
		}
		if neighbors, ok := ps["neighbors"]; ok {
			switch nb := neighbors.(type) {
			case []map[string]any:
				row["neighbor_count"] = len(nb)
			case []any:
				row["neighbor_count"] = len(nb)
			}
		}
		rows = append(rows, row)
	}

	return map[string][]map[string]any{"ports": rows}
}
