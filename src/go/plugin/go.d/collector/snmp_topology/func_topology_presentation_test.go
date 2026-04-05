// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/topology"
	"github.com/stretchr/testify/require"
)

func TestSNMPTopologyPresentationMatchesExpectedContract(t *testing.T) {
	require.Equal(t, expectedSNMPTopologyPresentation(), snmpTopologyPresentation())
}

func expectedSNMPTopologyPresentation() *topology.Presentation {
	deviceSummaryFields := []topology.PresentationSummaryField{
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

	deviceLinkTable := topology.PresentationTable{
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
	}

	deviceTables := map[string]topology.PresentationTable{
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
		"links": deviceLinkTable,
	}

	linkOnlyTables := map[string]topology.PresentationTable{
		"links": deviceLinkTable,
	}

	infoOnlyTabs := []topology.PresentationModalTab{
		{ID: "info", Label: "Info"},
	}

	segmentSummaryFields := []topology.PresentationSummaryField{
		{Key: "actor_type", Label: "Type", Sources: []string{"actor_type"}},
		{Key: "learned_sources", Label: "Discovered By", Sources: []string{"attributes.learned_sources"}},
		{Key: "ports_total", Label: "Ports", Sources: []string{"attributes.ports_total"}},
		{Key: "endpoints_total", Label: "Endpoints", Sources: []string{"attributes.endpoints_total"}},
		{Key: "source", Label: "Source", Sources: []string{"source"}},
		{Key: "layer", Label: "Layer", Sources: []string{"layer"}},
	}

	endpointSummaryFields := []topology.PresentationSummaryField{
		{Key: "actor_type", Label: "Type", Sources: []string{"actor_type"}},
		{Key: "vendor", Label: "Vendor", Sources: []string{"attributes.vendor", "attributes.vendor_derived"}},
		{Key: "learned_sources", Label: "Discovered By", Sources: []string{"attributes.learned_sources"}},
		{Key: "source", Label: "Source", Sources: []string{"source"}},
		{Key: "layer", Label: "Layer", Sources: []string{"layer"}},
	}

	deviceType := func(label, colorSlot string) topology.PresentationActorType {
		return topology.PresentationActorType{
			Label:           label,
			ColorSlot:       colorSlot,
			Border:          true,
			SizeByLinks:     true,
			ShowPortBullets: true,
			SummaryFields:   deviceSummaryFields,
			Tables:          deviceTables,
			ModalTabs:       infoOnlyTabs,
		}
	}

	return &topology.Presentation{
		ActorTypes: map[string]topology.PresentationActorType{
			"device":        deviceType("Device", "primary"),
			"router":        deviceType("Router", "primary"),
			"switch":        deviceType("Switch", "primary"),
			"firewall":      deviceType("Firewall", "warning"),
			"access_point":  deviceType("Access Point", "info"),
			"server":        deviceType("Server", "secondary"),
			"storage":       deviceType("Storage", "secondary"),
			"load_balancer": deviceType("Load Balancer", "info"),
			"printer":       deviceType("Printer", "neutral"),
			"phone":         deviceType("Phone", "neutral"),
			"ups":           deviceType("UPS", "structural"),
			"camera":        deviceType("Camera", "neutral"),
			"segment": {
				Label:         "Network segment",
				ColorSlot:     "dim",
				SummaryFields: segmentSummaryFields,
				Tables:        linkOnlyTables,
				ModalTabs:     infoOnlyTabs,
			},
			"endpoint": {
				Label:         "Inferred endpoint",
				ColorSlot:     "derived",
				Border:        true,
				SummaryFields: endpointSummaryFields,
				Tables:        linkOnlyTables,
				ModalTabs:     infoOnlyTabs,
			},
		},
		LinkTypes: map[string]topology.PresentationLinkType{
			"lldp":     {Label: "LLDP", ColorSlot: "accent", Width: 2},
			"cdp":      {Label: "CDP", ColorSlot: "accent", Width: 2},
			"bridge":   {Label: "Bridge", ColorSlot: "neutral"},
			"fdb":      {Label: "FDB", ColorSlot: "neutral"},
			"stp":      {Label: "STP", ColorSlot: "muted"},
			"arp":      {Label: "ARP", ColorSlot: "muted"},
			"snmp":     {Label: "SNMP", ColorSlot: "primary"},
			"probable": {Label: "Probable", ColorSlot: "dim"},
		},
		PortFields: []topology.PresentationPortField{
			{Key: "type", Label: "Type"},
			{Key: "role", Label: "Role"},
			{Key: "status", Label: "Status"},
			{Key: "mode", Label: "Mode"},
			{Key: "sources", Label: "Sources"},
		},
		PortTypes: map[string]topology.PresentationPortType{
			"lldp":           {Label: "lldp/cdp", ColorSlot: "accent"},
			"switch_facing":  {Label: "switch-facing", ColorSlot: "primary"},
			"host_facing":    {Label: "host-facing", ColorSlot: "secondary"},
			"host_candidate": {Label: "host-candidate", ColorSlot: "info"},
			"trunk":          {Label: "trunk", ColorSlot: "warning"},
			"access":         {Label: "access", ColorSlot: "derived"},
			"topology":       {Label: "unclassified", ColorSlot: "neutral"},
			"idle":           {Label: "idle", ColorSlot: "muted"},
			"unknown":        {Label: "unknown", ColorSlot: "dim"},
		},
		Legend: topology.PresentationLegend{
			Actors: []topology.PresentationLegendEntry{
				{Type: "router", Label: "Router"},
				{Type: "switch", Label: "Switch"},
				{Type: "firewall", Label: "Firewall"},
				{Type: "access_point", Label: "Access Point"},
				{Type: "server", Label: "Server"},
				{Type: "storage", Label: "Storage"},
				{Type: "load_balancer", Label: "Load Balancer"},
				{Type: "printer", Label: "Printer"},
				{Type: "phone", Label: "IP Phone"},
				{Type: "ups", Label: "UPS / PDU"},
				{Type: "camera", Label: "Camera / Media"},
				{Type: "device", Label: "Other device"},
				{Type: "endpoint", Label: "Inferred endpoint"},
				{Type: "segment", Label: "Network segment"},
			},
			Links: []topology.PresentationLegendEntry{
				{Type: "lldp", Label: "LLDP"},
				{Type: "cdp", Label: "CDP"},
				{Type: "snmp", Label: "SNMP"},
				{Type: "bridge", Label: "Bridge"},
				{Type: "probable", Label: "Probable"},
			},
			Ports: []topology.PresentationLegendEntry{
				{Type: "lldp", Label: "lldp/cdp"},
				{Type: "switch_facing", Label: "switch-facing"},
				{Type: "host_facing", Label: "host-facing"},
				{Type: "host_candidate", Label: "host-candidate"},
				{Type: "trunk", Label: "trunk"},
				{Type: "access", Label: "access"},
				{Type: "topology", Label: "unclassified"},
				{Type: "idle", Label: "idle"},
			},
		},
		ActorClickBehavior: "highlight_connections",
	}
}
