// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import "strings"

func buildTopologyDevicePortStatusAttributes(st topologyDevicePortStatus) map[string]any {
	portStatus := map[string]any{
		"if_index":                 st.IfIndex,
		"if_name":                  strings.TrimSpace(st.IfName),
		"if_descr":                 strings.TrimSpace(st.IfDescr),
		"if_alias":                 strings.TrimSpace(st.IfAlias),
		"mac":                      strings.TrimSpace(st.MAC),
		"duplex":                   strings.TrimSpace(st.Duplex),
		"link_mode":                st.LinkMode,
		"link_mode_confidence":     st.ModeConfidence,
		"topology_role":            st.TopologyRole,
		"topology_role_confidence": st.RoleConfidence,
	}
	if st.SpeedBps > 0 {
		portStatus["speed"] = st.SpeedBps
	}
	if st.LastChange > 0 {
		portStatus["last_change"] = st.LastChange
	}
	if len(st.ModeSources) > 0 {
		portStatus["link_mode_sources"] = st.ModeSources
	}
	if len(st.RoleSources) > 0 {
		portStatus["topology_role_sources"] = st.RoleSources
	}
	if len(st.VLANIDs) > 0 {
		portStatus["vlan_ids"] = st.VLANIDs
	}
	if len(st.VLANs) > 0 {
		portStatus["vlans"] = st.VLANs
	}
	if st.FDBMACCount > 0 {
		portStatus["fdb_mac_count"] = st.FDBMACCount
	}
	if st.STPState != "" {
		portStatus["stp_state"] = st.STPState
	}
	if len(st.Neighbors) > 0 {
		neighbors := make([]map[string]any, 0, len(st.Neighbors))
		for _, neighbor := range st.Neighbors {
			if attrs := topologyPortNeighborStatusToAttributes(neighbor); len(attrs) > 0 {
				neighbors = append(neighbors, attrs)
			}
		}
		if len(neighbors) > 0 {
			portStatus["neighbors"] = neighbors
		}
	}
	if st.AdminStatus != "" {
		portStatus["admin_status"] = st.AdminStatus
	}
	if st.OperStatus != "" {
		portStatus["oper_status"] = st.OperStatus
	}
	if st.InterfaceType != "" {
		portStatus["if_type"] = st.InterfaceType
	}
	return pruneTopologyAttributes(portStatus)
}
