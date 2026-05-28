// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import "github.com/netdata/netdata/go/plugins/pkg/topology"

func topologyDeviceSummaryFields() []topology.PresentationSummaryField {
	return []topology.PresentationSummaryField{
		{Key: "actor_type", Label: "Type", Sources: []string{"actor_type"}},
		{Key: "vendor", Label: "Vendor", Sources: []string{"attributes.vendor", "attributes.vendor_derived"}},
		{Key: "model", Label: "Model", Sources: []string{"attributes.model"}},
		{Key: "sys_descr", Label: "Description", Sources: []string{"attributes.sys_descr", "match.sys_name"}},
		{Key: "sys_location", Label: "Location", Sources: []string{"attributes.sys_location"}},
		{Key: "sys_contact", Label: "Contact", Sources: []string{"attributes.sys_contact"}},
		{Key: "protocols", Label: "Protocols", Sources: []string{"attributes.protocols", "attributes.learned_sources"}},
		{Key: "capabilities", Label: "Capabilities", Sources: []string{"attributes.capabilities"}},
		{Key: "ports_total", Label: "Ports", Sources: []string{"attributes.ports_total"}},
		{Key: "vlan_count", Label: "VLANs", Sources: []string{"attributes.vlan_count"}},
		{Key: "fdb_total_macs", Label: "FDB MACs", Sources: []string{"attributes.fdb_total_macs"}},
		{Key: "lldp_neighbor_count", Label: "LLDP Neighbors", Sources: []string{"attributes.lldp_neighbor_count"}},
		{Key: "cdp_neighbor_count", Label: "CDP Neighbors", Sources: []string{"attributes.cdp_neighbor_count"}},
		{Key: "chart_id_prefix", Label: "Chart Prefix", Sources: []string{"attributes.chart_id_prefix"}},
		{Key: "netdata_host_id", Label: "Netdata Host", Sources: []string{"attributes.netdata_host_id"}},
		{Key: "source", Label: "Source", Sources: []string{"source"}},
		{Key: "layer", Label: "Layer", Sources: []string{"layer"}},
	}
}

func topologyDeviceTables() map[string]topology.PresentationTable {
	return map[string]topology.PresentationTable{
		"ports": {
			Label:        "Ports",
			Source:       "data",
			BulletSource: true,
			Order:        1,
			Columns: []topology.PresentationTableColumn{
				{Key: "name", Label: "Port"},
				{Key: "oper_status", Label: "Status", Type: "badge"},
				{Key: "admin_status", Label: "Admin"},
				{Key: "port_type", Label: "Type", Type: "badge"},
				{Key: "link_mode", Label: "Mode", Type: "badge"},
				{Key: "topology_role", Label: "Role", Type: "badge"},
				{Key: "stp_state", Label: "STP", Type: "badge"},
				{Key: "vlan_ids", Label: "VLANs", Type: "count"},
				{Key: "fdb_mac_count", Label: "FDB", Type: "number"},
				{Key: "link_count", Label: "Links", Type: "number"},
				{Key: "neighbor_count", Label: "Neighbors", Type: "number"},
			},
		},
		"links": {
			Label:  "Links",
			Source: "links",
			Order:  2,
			Columns: []topology.PresentationTableColumn{
				{Key: "localPort", Label: "Local Port"},
				{Key: "remoteLabel", Label: "Remote Actor", Type: "actor_link"},
				{Key: "remotePort", Label: "Remote Port"},
				{Key: "protocol", Label: "Protocol"},
				{Key: "direction", Label: "Direction"},
			},
		},
	}
}

func topologySegmentSummaryFields() []topology.PresentationSummaryField {
	return []topology.PresentationSummaryField{
		{Key: "actor_type", Label: "Type", Sources: []string{"actor_type"}},
		{Key: "learned_sources", Label: "Discovered By", Sources: []string{"attributes.learned_sources"}},
		{Key: "ports_total", Label: "Ports", Sources: []string{"attributes.ports_total"}},
		{Key: "endpoints_total", Label: "Endpoints", Sources: []string{"attributes.endpoints_total"}},
		{Key: "source", Label: "Source", Sources: []string{"source"}},
		{Key: "layer", Label: "Layer", Sources: []string{"layer"}},
	}
}

func topologyEndpointSummaryFields() []topology.PresentationSummaryField {
	return []topology.PresentationSummaryField{
		{Key: "actor_type", Label: "Type", Sources: []string{"actor_type"}},
		{Key: "vendor", Label: "Vendor", Sources: []string{"attributes.vendor", "attributes.vendor_derived"}},
		{Key: "learned_sources", Label: "Discovered By", Sources: []string{"attributes.learned_sources"}},
		{Key: "source", Label: "Source", Sources: []string{"source"}},
		{Key: "layer", Label: "Layer", Sources: []string{"layer"}},
	}
}

func topologyInfoOnlyTabs() []topology.PresentationModalTab {
	return []topology.PresentationModalTab{
		{ID: "info", Label: "Info"},
	}
}

func topologyLinkOnlyTables(linkTable topology.PresentationTable) map[string]topology.PresentationTable {
	return map[string]topology.PresentationTable{
		"links": linkTable,
	}
}
