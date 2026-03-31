// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"strconv"
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

func deviceToTopologyActor(
	dev Device,
	source, layer, localDeviceID string,
	ifaceSummary topologyDeviceInterfaceSummary,
	reporterAliases []string,
) topology.Actor {
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

	discovered := true
	if strings.TrimSpace(localDeviceID) != "" && dev.ID == localDeviceID {
		discovered = false
	}

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

	var tables map[string][]map[string]any
	if len(ifaceSummary.portStatuses) > 0 {
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
		tables = map[string][]map[string]any{"ports": rows}
	}

	return topology.Actor{
		ActorType:  resolveDeviceActorType(dev.Labels),
		Layer:      layer,
		Source:     source,
		Match:      match,
		Attributes: pruneTopologyAttributes(attrs),
		Labels:     cloneStringMap(dev.Labels),
		Tables:     tables,
	}
}

var deviceCategoryToActorType = map[string]string{
	"router":                  "router",
	"gateway":                 "router",
	"layer 3 switch":          "router",
	"voip gateway":            "router",
	"switch":                  "switch",
	"bridge":                  "switch",
	"hub":                     "switch",
	"sanswitch":               "switch",
	"sanbridge":               "switch",
	"bridge/extender":         "switch",
	"firewall":                "firewall",
	"security":                "firewall",
	"access point":            "access_point",
	"wireless":                "access_point",
	"wireless lan controller": "access_point",
	"extender":                "access_point",
	"radio":                   "access_point",
	"server":                  "server",
	"file server":             "server",
	"application":             "server",
	"desktop":                 "server",
	"blade system":            "server",
	"storage":                 "storage",
	"nas":                     "storage",
	"self-contained nas":      "storage",
	"nas head":                "storage",
	"tape library":            "storage",
	"load balancer":           "load_balancer",
	"wan accelerator":         "load_balancer",
	"web caching":             "load_balancer",
	"proxy server":            "load_balancer",
	"content":                 "load_balancer",
	"printer":                 "printer",
	"ip phone":                "phone",
	"voip":                    "phone",
	"gsm":                     "phone",
	"mobile":                  "phone",
	"ups":                     "ups",
	"pdu":                     "ups",
	"power":                   "ups",
	"video":                   "camera",
	"media":                   "camera",
	"media exchange":          "camera",
	"sensor":                  "camera",
	"other":                   "device",
	"network device":          "device",
	"management":              "server",
	"management controller":   "server",
	"dslam":                   "switch",
	"access server":           "server",
	"pon":                     "switch",
	"console":                 "server",
	"module":                  "device",
	"plc":                     "device",
	"sre module":              "server",
	"chassis manager":         "server",
	"snmp managed device":     "device",
}

var deviceActorTypes = func() map[string]struct{} {
	s := map[string]struct{}{"device": {}}
	for _, v := range deviceCategoryToActorType {
		s[v] = struct{}{}
	}
	return s
}()

func resolveDeviceActorType(labels map[string]string) string {
	cat := strings.TrimSpace(labels["type"])
	if cat == "" {
		return "device"
	}
	if at, ok := deviceCategoryToActorType[strings.ToLower(cat)]; ok {
		return at
	}
	return "device"
}

func IsDeviceActorType(actorType string) bool {
	_, ok := deviceActorTypes[strings.ToLower(strings.TrimSpace(actorType))]
	return ok
}

func adjacencySideToEndpoint(dev Device, port string, ifIndexByDeviceName map[string]int, ifaceByDeviceIndex map[string]Interface) topology.LinkEndpoint {
	match := topology.Match{
		SysObjectID: strings.TrimSpace(dev.SysObject),
		SysName:     strings.TrimSpace(dev.Hostname),
	}
	chassis := strings.TrimSpace(dev.ChassisID)
	if chassis != "" {
		match.ChassisIDs = []string{chassis}
		if mac := normalizeMAC(chassis); mac != "" {
			match.MacAddresses = []string{mac}
		}
	}
	for _, addr := range dev.Addresses {
		if !addr.IsValid() {
			continue
		}
		match.IPAddresses = append(match.IPAddresses, addr.String())
	}
	match.IPAddresses = uniqueTopologyStrings(match.IPAddresses)

	port = strings.TrimSpace(port)
	ifName := ""
	ifDescr := ""
	ifIndex := 0
	var iface Interface
	hasIface := false
	if port != "" {
		ifIndex = resolveIfIndexByPortName(dev.ID, port, ifIndexByDeviceName)
	}
	if ifIndex > 0 {
		if ifaceValue, ok := ifaceByDeviceIndex[deviceIfIndexKey(dev.ID, ifIndex)]; ok {
			iface = ifaceValue
			hasIface = true
			ifName = strings.TrimSpace(iface.IfName)
			ifDescr = strings.TrimSpace(iface.IfDescr)
		}
	}
	if ifName == "" {
		ifName = ifDescr
	}
	if ifName == "" {
		ifName = port
	}
	if ifIndex > 0 && ifName == "" {
		ifName = strconv.Itoa(ifIndex)
	}

	attrs := map[string]any{
		"if_index":      ifIndex,
		"if_name":       ifName,
		"port_id":       port,
		"sys_name":      strings.TrimSpace(dev.Hostname),
		"management_ip": firstAddress(dev.Addresses),
	}
	if ifDescr != "" {
		attrs["if_descr"] = ifDescr
	}
	if ifIndex > 0 && hasIface {
		if admin := strings.TrimSpace(iface.Labels["admin_status"]); admin != "" {
			attrs["if_admin_status"] = admin
		}
		if oper := strings.TrimSpace(iface.Labels["oper_status"]); oper != "" {
			attrs["if_oper_status"] = oper
		}
	}

	return topology.LinkEndpoint{
		Match:      match,
		Attributes: pruneTopologyAttributes(attrs),
	}
}
