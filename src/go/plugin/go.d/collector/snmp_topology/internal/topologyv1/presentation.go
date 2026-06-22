// SPDX-License-Identifier: GPL-3.0-or-later

package topologyv1

import (
	topologyapi "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
)

func snmpTopologyV1ActorTypes() map[string]topologyapi.ActorType {
	types := make(map[string]topologyapi.ActorType)
	addDevice := func(id, label, colorSlot, icon string) {
		types[id] = topologyapi.ActorType{
			Layer:             "network",
			Identity:          []string{"id"},
			MergeIdentity:     []string{"chassis_ids", "mac_addresses", "ip_addresses", "sys_name"},
			AggregationScopes: []string{"device", "network"},
			Search: &topologyapi.ActorSearchPolicy{
				Columns:   []string{"display_name", "sys_name", "management_ip", "vendor", "model"},
				LabelKeys: []string{topologymodel.LabelOSPFRouterID},
			},
			Presentation: &topologyapi.ActorPresentation{
				Label:     label,
				Role:      "actor",
				Icon:      icon,
				ColorSlot: colorSlot,
				Border:    &topologyapi.BorderPresentation{Enabled: new(true)},
				Size:      &topologyapi.ActorSizePresentation{Mode: "link_count", Scale: "emphasized"},
				Layout:    &topologyapi.ActorLayoutPresentation{Repulsion: "stronger"},
				LabelPolicy: &topologyapi.LabelPolicy{
					Columns:   []string{"display_name", "sys_name"},
					Fallback:  "type_label",
					MaxLength: 80,
					Array:     "reject",
				},
				Ports: &topologyapi.ActorPortsPresentation{
					ShowBullets: true,
					Sources: []topologyapi.PortSourcePresentation{
						{
							Source:       "actor_table",
							Table:        "actor_ports",
							ActorColumn:  "actor",
							NameColumn:   "name",
							TypeColumn:   "topology_role",
							DefaultType:  "topology",
							StatusColumn: "oper_status",
							ModeColumn:   "link_mode",
							RoleColumn:   "topology_role",
						},
					},
				},
				Modal: snmpTopologyV1DeviceModal(),
			},
		}
	}

	addDevice("device", "Device", "primary", "server")
	addDevice("router", "Router", "primary", "router")
	addDevice("switch", "Switch", "primary", "switch")
	addDevice("firewall", "Firewall", "warning", "firewall")
	addDevice("access_point", "Access Point", "info", "access_point")
	addDevice("server", "Server", "secondary", "server")
	addDevice("storage", "Storage", "secondary", "storage")
	addDevice("load_balancer", "Load Balancer", "info", "load_balancer")
	addDevice("printer", "Printer", "neutral", "printer")
	addDevice("phone", "Phone", "neutral", "phone")
	addDevice("ups", "UPS", "structural", "ups")
	addDevice("camera", "Camera", "neutral", "camera")

	types[snmpTopologyV1ActorEndpoint] = topologyapi.ActorType{
		Layer:             "network",
		Identity:          []string{"id"},
		MergeIdentity:     []string{"mac_addresses", "ip_addresses"},
		AggregationScopes: []string{"endpoint", "network"},
		Search:            &topologyapi.ActorSearchPolicy{Columns: []string{"display_name"}},
		Presentation: &topologyapi.ActorPresentation{
			Label:     "Inferred endpoint",
			Role:      "endpoint",
			Icon:      "remote-endpoint",
			ColorSlot: "derived",
			Border:    &topologyapi.BorderPresentation{Enabled: new(true)},
			Size:      &topologyapi.ActorSizePresentation{Mode: "fixed", Scale: "compact"},
			Layout:    &topologyapi.ActorLayoutPresentation{Repulsion: "weaker"},
			LabelPolicy: &topologyapi.LabelPolicy{
				Columns:   []string{"display_name"},
				Fallback:  "type_label",
				MaxLength: 80,
				Array:     "reject",
			},
			Modal: snmpTopologyV1EndpointModal(),
		},
	}
	types[snmpTopologyV1ActorSegment] = topologyapi.ActorType{
		Layer:             "network",
		Identity:          []string{"id"},
		MergeIdentity:     []string{"id"},
		ParentIdentity:    []string{"parent_devices"},
		AggregationScopes: []string{"segment", "network"},
		Search:            &topologyapi.ActorSearchPolicy{Enabled: new(false)},
		Presentation: &topologyapi.ActorPresentation{
			Label:     "Network segment",
			Role:      "group",
			Icon:      "segment",
			ColorSlot: "dim",
			Size:      &topologyapi.ActorSizePresentation{Mode: "fixed", Scale: "compact"},
			Layout:    &topologyapi.ActorLayoutPresentation{Repulsion: "weakest"},
			LabelPolicy: &topologyapi.LabelPolicy{
				Columns:   []string{"display_name"},
				Fallback:  "type_label",
				MaxLength: 80,
				Array:     "reject",
			},
			Modal: snmpTopologyV1EndpointModal(),
		},
	}
	types[snmpTopologyV1ActorL3SubnetSegment] = topologyapi.ActorType{
		Layer:             "network",
		Identity:          []string{"id"},
		MergeIdentity:     []string{"id"},
		AggregationScopes: []string{"segment", "network"},
		Search:            &topologyapi.ActorSearchPolicy{Enabled: new(false)},
		Presentation: &topologyapi.ActorPresentation{
			Label:     "L3 subnet",
			Role:      "group",
			Icon:      "segment",
			ColorSlot: "info",
			Size:      &topologyapi.ActorSizePresentation{Mode: "fixed", Scale: "compact"},
			Layout:    &topologyapi.ActorLayoutPresentation{Repulsion: "weakest"},
			LabelPolicy: &topologyapi.LabelPolicy{
				Columns:   []string{"display_name"},
				Fallback:  "type_label",
				MaxLength: 80,
				Array:     "reject",
			},
			Modal: snmpTopologyV1L3SubnetSegmentModal(),
		},
	}
	types["custom"] = topologyapi.ActorType{
		Layer:             "custom",
		Identity:          []string{"id"},
		MergeIdentity:     []string{"id"},
		AggregationScopes: []string{"network"},
		Search:            &topologyapi.ActorSearchPolicy{Columns: []string{"display_name"}},
		Presentation: &topologyapi.ActorPresentation{
			Label:     "Custom",
			Role:      "actor",
			Icon:      "service",
			ColorSlot: "neutral",
			LabelPolicy: &topologyapi.LabelPolicy{
				Columns:   []string{"display_name"},
				Fallback:  "type_label",
				MaxLength: 80,
				Array:     "reject",
			},
			Modal: snmpTopologyV1EndpointModal(),
		},
	}
	return types
}

func snmpTopologyV1DeviceModal() *topologyapi.ModalPresentation {
	return &topologyapi.ModalPresentation{
		Labels:       snmpTopologyV1DeviceModalLabels(),
		MiniTopology: &topologyapi.ModalMiniTopologyPresentation{Depth: 1},
		Sections: []topologyapi.ModalSection{
			{
				ID:    "ports",
				Label: "Ports",
				Order: 1,
				Source: topologyapi.ModalSource{
					Kind:  "actor_table",
					Table: "actor_ports",
				},
				OwnerFilter: &topologyapi.ModalOwnerFilter{
					Mode:        "actor_column",
					ActorColumn: "actor",
				},
				Columns: snmpTopologyV1PortModalColumns(),
				Sort:    &topologyapi.ModalSort{Column: "if_index", Direction: "asc"},
			},
			snmpTopologyV1PortLinksSection(2),
			snmpTopologyV1L3SubnetSection(3),
			snmpTopologyV1L3SubnetMembershipSection(4),
			snmpTopologyV1OSPFNeighborsSection(5),
			snmpTopologyV1BGPPeersSection(6),
		},
	}
}

func snmpTopologyV1EndpointModal() *topologyapi.ModalPresentation {
	return &topologyapi.ModalPresentation{
		Labels:       snmpTopologyV1EndpointModalLabels(),
		MiniTopology: &topologyapi.ModalMiniTopologyPresentation{Depth: 1},
		Sections:     []topologyapi.ModalSection{snmpTopologyV1LinksSection(1)},
	}
}

func snmpTopologyV1L3SubnetSegmentModal() *topologyapi.ModalPresentation {
	return &topologyapi.ModalPresentation{
		MiniTopology: &topologyapi.ModalMiniTopologyPresentation{Depth: 1},
		Sections: []topologyapi.ModalSection{
			snmpTopologyV1L3SubnetSegmentMembersSection(1),
		},
	}
}

func snmpTopologyV1DeviceModalLabels() *topologyapi.ModalLabelsPresentation {
	return &topologyapi.ModalLabelsPresentation{
		Table: "actor_labels",
		Identification: &topologyapi.ModalLabelIdentificationPresentation{
			Fields: []topologyapi.ModalLabelIdentificationField{
				{Key: "display_name", Label: "Name", MaxValues: 1},
				{Key: "management_ip", Label: "Management IP", MaxValues: 1},
				{Key: "vendor", Label: "Vendor", MaxValues: 1},
				{Key: "model", Label: "Model", MaxValues: 1},
				{Key: topologymodel.LabelOSPFRouterID, Label: "OSPF Router ID", MaxValues: 1},
				{Key: "ports_total", Label: "Ports", MaxValues: 1},
				{Key: "lldp_neighbor_count", Label: "LLDP", MaxValues: 1},
				{Key: "cdp_neighbor_count", Label: "CDP", MaxValues: 1},
			},
		},
	}
}

func snmpTopologyV1EndpointModalLabels() *topologyapi.ModalLabelsPresentation {
	return &topologyapi.ModalLabelsPresentation{
		Table: "actor_labels",
		Identification: &topologyapi.ModalLabelIdentificationPresentation{
			Fields: []topologyapi.ModalLabelIdentificationField{
				{Key: "display_name", Label: "Name", MaxValues: 1},
				{Key: "ip_address", Label: "IP", MaxValues: 2},
				{Key: "mac_address", Label: "MAC", MaxValues: 2},
				{Key: "hostname", Label: "Hostname", MaxValues: 2},
			},
		},
	}
}

func snmpTopologyV1LinksSection(order int) topologyapi.ModalSection {
	return topologyapi.ModalSection{
		ID:    "links",
		Label: "Links",
		Order: order,
		Source: topologyapi.ModalSource{
			Kind: "links",
		},
		OwnerFilter: &topologyapi.ModalOwnerFilter{
			Mode:           "incident_link",
			SrcActorColumn: "src_actor",
			DstActorColumn: "dst_actor",
		},
		Columns: []topologyapi.ModalColumn{
			{
				ID:    "remote",
				Label: "Remote Actor",
				Projection: topologyapi.ModalProjection{
					Kind:           "opposite_actor",
					SrcActorColumn: "src_actor",
					DstActorColumn: "dst_actor",
				},
				Cell: "actor_link",
			},
			modalSelectedSidePortColumn("local_port", "Local Port", "src_port_name", "dst_port_name"),
			modalSelectedSidePortColumn("remote_port", "Remote Port", "dst_port_name", "src_port_name"),
			modalDirectColumn("protocol", "Protocol", "protocol", "badge"),
			modalDirectColumn("direction", "Direction", "direction", "text"),
			modalDirectColumn("state", "State", "state", "badge"),
			modalDirectColumn("evidence_count", "Evidence", "evidence_count", "number"),
		},
	}
}

func snmpTopologyV1PortLinksSection(order int) topologyapi.ModalSection {
	return topologyapi.ModalSection{
		ID:    "port_neighbors",
		Label: "Port Neighbors",
		Order: order,
		Source: topologyapi.ModalSource{
			Kind:  "actor_table",
			Table: "actor_port_links",
		},
		OwnerFilter: &topologyapi.ModalOwnerFilter{
			Mode:        "actor_column",
			ActorColumn: "actor",
		},
		Columns:    snmpTopologyV1PortLinkModalColumns(),
		Sort:       &topologyapi.ModalSort{Column: "if_index", Direction: "asc"},
		EmptyLabel: "No port neighbors",
	}
}

func snmpTopologyV1L3SubnetSection(order int) topologyapi.ModalSection {
	return topologyapi.ModalSection{
		ID:    "l3_adjacencies",
		Label: "L3 Adjacencies",
		Order: order,
		Source: topologyapi.ModalSource{
			Kind:     "evidence",
			Evidence: snmpTopologyV1LinkL3Subnet,
		},
		OwnerFilter: &topologyapi.ModalOwnerFilter{
			Mode:           "incident_evidence",
			LinkColumn:     "link",
			SrcActorColumn: "src_actor",
			DstActorColumn: "dst_actor",
		},
		Columns: []topologyapi.ModalColumn{
			{
				ID:    "remote",
				Label: "Remote Actor",
				Projection: topologyapi.ModalProjection{
					Kind:           "opposite_actor",
					SrcActorColumn: "src_actor",
					DstActorColumn: "dst_actor",
				},
				Cell: "actor_link",
			},
			modalSelectedSideEndpointColumn("local_endpoint", "Local Endpoint", "src_ip", "src_port_name", "dst_ip", "dst_port_name"),
			modalSelectedSideEndpointColumn("remote_endpoint", "Remote Endpoint", "dst_ip", "dst_port_name", "src_ip", "src_port_name"),
			modalDirectColumn("subnet", "Subnet", "subnet", "text"),
			modalDirectColumn("prefix", "Prefix", "prefix", "number"),
			modalDirectColumnWithVisibility("network", "Network", "network", "text", "expanded"),
			modalDirectColumnWithVisibility("netmask", "Netmask", "netmask", "text", "expanded"),
			modalDirectColumnWithVisibility("source", "Source", "source", "badge", "expanded"),
			modalDirectColumnWithVisibility("inference", "Inference", "inference", "badge", "expanded"),
			modalDirectColumnWithVisibility("attachment_mode", "Attachment", "attachment_mode", "badge", "expanded"),
		},
		Sort:       &topologyapi.ModalSort{Column: "subnet", Direction: "asc"},
		EmptyLabel: "No L3 adjacencies",
	}
}

func snmpTopologyV1L3SubnetMembershipSection(order int) topologyapi.ModalSection {
	return topologyapi.ModalSection{
		ID:    "l3_subnet_memberships",
		Label: "L3 Subnet Memberships",
		Order: order,
		Source: topologyapi.ModalSource{
			Kind:     "evidence",
			Evidence: snmpTopologyV1LinkL3SubnetMembership,
		},
		OwnerFilter: &topologyapi.ModalOwnerFilter{
			Mode:           "incident_evidence",
			LinkColumn:     "link",
			SrcActorColumn: "src_actor",
			DstActorColumn: "dst_actor",
		},
		Columns: []topologyapi.ModalColumn{
			{
				ID:    "subnet_actor",
				Label: "Subnet",
				Projection: topologyapi.ModalProjection{
					Kind:           "opposite_actor",
					SrcActorColumn: "src_actor",
					DstActorColumn: "dst_actor",
				},
				Cell: "actor_link",
			},
			modalDirectColumn("member_ip", "Local IP", "member_ip", "text"),
			modalDirectColumn("member_if_name", "Interface", "member_if_name", "text"),
			modalDirectColumn("subnet", "Subnet", "subnet", "text"),
			modalDirectColumn("prefix", "Prefix", "prefix", "number"),
			modalDirectColumnWithVisibility("member_if_index", "IfIndex", "member_if_index", "number", "expanded"),
			modalDirectColumnWithVisibility("member_if_descr", "IfDescr", "member_if_descr", "text", "expanded"),
			modalDirectColumnWithVisibility("network", "Network", "network", "text", "expanded"),
			modalDirectColumnWithVisibility("netmask", "Netmask", "netmask", "text", "expanded"),
			modalDirectColumnWithVisibility("source", "Source", "source", "badge", "expanded"),
		},
		Sort:       &topologyapi.ModalSort{Column: "subnet", Direction: "asc"},
		EmptyLabel: "No L3 subnet memberships",
	}
}

func snmpTopologyV1L3SubnetSegmentMembersSection(order int) topologyapi.ModalSection {
	return topologyapi.ModalSection{
		ID:    "l3_subnet_members",
		Label: "Members",
		Order: order,
		Source: topologyapi.ModalSource{
			Kind:     "evidence",
			Evidence: snmpTopologyV1LinkL3SubnetMembership,
		},
		OwnerFilter: &topologyapi.ModalOwnerFilter{
			Mode:           "incident_evidence",
			LinkColumn:     "link",
			SrcActorColumn: "src_actor",
			DstActorColumn: "dst_actor",
		},
		Columns: []topologyapi.ModalColumn{
			modalActorRefColumn("member", "Member", "member_actor"),
			modalDirectColumn("member_ip", "Member IP", "member_ip", "text"),
			modalDirectColumn("member_if_name", "Interface", "member_if_name", "text"),
			modalDirectColumn("subnet", "Subnet", "subnet", "text"),
			modalDirectColumn("prefix", "Prefix", "prefix", "number"),
			modalDirectColumnWithVisibility("member_if_index", "IfIndex", "member_if_index", "number", "expanded"),
			modalDirectColumnWithVisibility("member_if_descr", "IfDescr", "member_if_descr", "text", "expanded"),
			modalDirectColumnWithVisibility("network", "Network", "network", "text", "expanded"),
			modalDirectColumnWithVisibility("netmask", "Netmask", "netmask", "text", "expanded"),
			modalDirectColumnWithVisibility("source", "Source", "source", "badge", "expanded"),
		},
		Sort:       &topologyapi.ModalSort{Column: "member_ip", Direction: "asc"},
		EmptyLabel: "No subnet members",
	}
}

func snmpTopologyV1OSPFNeighborsSection(order int) topologyapi.ModalSection {
	return topologyapi.ModalSection{
		ID:    "ospf_neighbors",
		Label: "OSPF Neighbors",
		Order: order,
		Source: topologyapi.ModalSource{
			Kind:  "actor_table",
			Table: "actor_ospf_neighbors",
		},
		OwnerFilter: &topologyapi.ModalOwnerFilter{
			Mode:        "actor_column",
			ActorColumn: "actor",
		},
		Columns:    snmpTopologyV1OSPFNeighborModalColumns(),
		Sort:       &topologyapi.ModalSort{Column: "neighbor_router_id", Direction: "asc"},
		EmptyLabel: "No OSPF neighbors",
	}
}

func snmpTopologyV1BGPPeersSection(order int) topologyapi.ModalSection {
	return topologyapi.ModalSection{
		ID:    "bgp_peers",
		Label: "BGP Peers",
		Order: order,
		Source: topologyapi.ModalSource{
			Kind:  "actor_table",
			Table: "actor_bgp_peers",
		},
		OwnerFilter: &topologyapi.ModalOwnerFilter{
			Mode:        "actor_column",
			ActorColumn: "actor",
		},
		Columns:    snmpTopologyV1BGPPeerModalColumns(),
		Sort:       &topologyapi.ModalSort{Column: "neighbor_ip", Direction: "asc"},
		EmptyLabel: "No BGP peers",
	}
}

func modalSelectedSidePortColumn(id, label, selectedSrcPortColumn, selectedDstPortColumn string) topologyapi.ModalColumn {
	return topologyapi.ModalColumn{
		ID:    id,
		Label: label,
		Projection: topologyapi.ModalProjection{
			Kind:             "selected_side_endpoint",
			SrcActorColumn:   "src_actor",
			DstActorColumn:   "dst_actor",
			LocalPortColumn:  selectedSrcPortColumn,
			RemotePortColumn: selectedDstPortColumn,
		},
		Cell: "text",
	}
}

func modalSelectedSideEndpointColumn(id, label, selectedSrcIPColumn, selectedSrcPortColumn, selectedDstIPColumn, selectedDstPortColumn string) topologyapi.ModalColumn {
	return topologyapi.ModalColumn{
		ID:    id,
		Label: label,
		Projection: topologyapi.ModalProjection{
			Kind:             "selected_side_endpoint",
			SrcActorColumn:   "src_actor",
			DstActorColumn:   "dst_actor",
			LocalIPColumn:    selectedSrcIPColumn,
			LocalPortColumn:  selectedSrcPortColumn,
			RemoteIPColumn:   selectedDstIPColumn,
			RemotePortColumn: selectedDstPortColumn,
		},
		Cell: "endpoint",
	}
}

func modalDirectColumn(id, label, sourceColumn, cell string) topologyapi.ModalColumn {
	return topologyapi.ModalColumn{
		ID:         id,
		Label:      label,
		Projection: topologyapi.ModalProjection{Kind: "direct", Column: sourceColumn},
		Cell:       cell,
	}
}

func modalDirectColumnWithVisibility(id, label, sourceColumn, cell, visibility string) topologyapi.ModalColumn {
	column := modalDirectColumn(id, label, sourceColumn, cell)
	column.Visibility = visibility
	return column
}

func modalActorRefColumn(id, label, actorColumn string) topologyapi.ModalColumn {
	return topologyapi.ModalColumn{
		ID:    id,
		Label: label,
		Projection: topologyapi.ModalProjection{
			Kind:        "actor_ref_label",
			ActorColumn: actorColumn,
		},
		Cell: "actor_link",
	}
}

func modalActorRefColumnWithVisibility(id, label, actorColumn, visibility string) topologyapi.ModalColumn {
	column := modalActorRefColumn(id, label, actorColumn)
	column.Visibility = visibility
	return column
}

func snmpTopologyV1PortTypes() map[string]topologyapi.PortType {
	return map[string]topologyapi.PortType{
		"lldp":           {Presentation: &topologyapi.PortPresentation{Label: "lldp/cdp", ColorSlot: "accent", Opacity: "normal"}},
		"switch_facing":  {Presentation: &topologyapi.PortPresentation{Label: "switch-facing", ColorSlot: "primary", Opacity: "normal"}},
		"host_facing":    {Presentation: &topologyapi.PortPresentation{Label: "host-facing", ColorSlot: "secondary", Opacity: "normal"}},
		"host_candidate": {Presentation: &topologyapi.PortPresentation{Label: "host-candidate", ColorSlot: "info", Opacity: "normal"}},
		"trunk":          {Presentation: &topologyapi.PortPresentation{Label: "trunk", ColorSlot: "warning", Opacity: "normal"}},
		"access":         {Presentation: &topologyapi.PortPresentation{Label: "access", ColorSlot: "derived", Opacity: "normal"}},
		"topology":       {Presentation: &topologyapi.PortPresentation{Label: "unclassified", ColorSlot: "neutral", Opacity: "normal"}},
		"idle":           {Presentation: &topologyapi.PortPresentation{Label: "idle", ColorSlot: "muted", Opacity: "muted"}},
		"unknown":        {Presentation: &topologyapi.PortPresentation{Label: "unknown", ColorSlot: "dim", Opacity: "muted"}},
	}
}

type snmpTopologyV1LinkTypeSpec struct {
	id           string
	label        string
	colorSlot    string
	lineStyle    string
	width        string
	semanticRole string
}

func snmpTopologyV1LinkTypeSpecs() []snmpTopologyV1LinkTypeSpec {
	return []snmpTopologyV1LinkTypeSpec{
		{id: snmpTopologyV1LinkLLDP, label: "LLDP", colorSlot: "accent", lineStyle: "solid", width: "thick", semanticRole: "discovery"},
		{id: snmpTopologyV1LinkCDP, label: "CDP", colorSlot: "accent", lineStyle: "solid", width: "thick", semanticRole: "discovery"},
		{id: snmpTopologyV1LinkBridge, label: "Bridge", colorSlot: "neutral", lineStyle: "solid", width: "normal", semanticRole: "normal"},
		{id: snmpTopologyV1LinkFDB, label: "FDB", colorSlot: "neutral", lineStyle: "solid", width: "normal", semanticRole: "normal"},
		{id: snmpTopologyV1LinkSTP, label: "STP", colorSlot: "muted", lineStyle: "solid", width: "normal", semanticRole: "normal"},
		{id: snmpTopologyV1LinkARP, label: "ARP", colorSlot: "muted", lineStyle: "solid", width: "normal", semanticRole: "normal"},
		{id: snmpTopologyV1LinkL3Subnet, label: "L3 subnet", colorSlot: "info", lineStyle: "dashed", width: "normal", semanticRole: "normal"},
		{id: snmpTopologyV1LinkL3SubnetMembership, label: "L3 subnet membership", colorSlot: "info", lineStyle: "dashed", width: "normal", semanticRole: "normal"},
		{id: snmpTopologyV1LinkOSPF, label: "OSPF adjacency", colorSlot: "purple", lineStyle: "dashed", width: "normal", semanticRole: "control"},
		{id: snmpTopologyV1LinkBGP, label: "BGP adjacency", colorSlot: "accent", lineStyle: "dashed", width: "normal", semanticRole: "control"},
		{id: snmpTopologyV1LinkSNMP, label: "SNMP", colorSlot: "primary", lineStyle: "solid", width: "normal", semanticRole: "normal"},
		{id: snmpTopologyV1LinkProbable, label: "Probable", colorSlot: "dim", lineStyle: "solid", width: "normal", semanticRole: "normal"},
		{id: snmpTopologyV1LinkObservation, label: "L2 observation", colorSlot: "neutral", lineStyle: "solid", width: "normal", semanticRole: "normal"},
	}
}

func snmpTopologyV1LinkTypes() map[string]topologyapi.LinkType {
	types := make(map[string]topologyapi.LinkType)
	for _, spec := range snmpTopologyV1LinkTypeSpecs() {
		types[spec.id] = topologyapi.LinkType{
			Orientation:   "observed_bidirectional",
			DirectionRole: "observation",
			SemanticRole:  spec.semanticRole,
			Aggregation: topologyapi.LinkAggregation{
				Direction: "canonicalize_unordered",
				Evidence:  "append",
				Metrics: map[string]string{
					"evidence_count": "sum",
				},
			},
			EvidenceTypes: []string{spec.id},
			Presentation: &topologyapi.LinkPresentation{
				Label:     spec.label,
				ColorSlot: spec.colorSlot,
				LineStyle: spec.lineStyle,
				Width:     spec.width,
				Curve:     "straight",
				Arrow:     "none",
			},
		}
	}
	return types
}

func snmpTopologyV1EvidenceTypes() map[string]topologyapi.EvidenceType {
	types := make(map[string]topologyapi.EvidenceType)
	for _, spec := range snmpTopologyV1LinkTypeSpecs() {
		types[spec.id] = topologyapi.EvidenceType{
			LinkType: spec.id,
			Role:     "observation_evidence",
			Columns:  snmpTopologyV1EvidenceColumnsForType(spec.id),
			MatchColumns: snmpTopologyV1EvidenceMatchColumnsForType(
				spec.id,
			),
		}
	}
	return types
}

func snmpTopologyV1EvidenceMatchColumnsForType(linkType string) []string {
	if linkType == snmpTopologyV1LinkL3Subnet {
		return []string{
			"src_actor",
			"dst_actor",
			"subnet",
			"src_ip",
			"dst_ip",
		}
	}
	if linkType == snmpTopologyV1LinkL3SubnetMembership {
		return []string{
			"member_actor",
			"segment_actor",
			"member_ip",
			"subnet",
		}
	}
	if linkType == snmpTopologyV1LinkOSPF {
		return []string{
			"src_actor",
			"dst_actor",
			"src_router_id",
			"dst_router_id",
			"src_ip",
			"dst_ip",
		}
	}
	if linkType == snmpTopologyV1LinkBGP {
		return []string{
			"src_actor",
			"dst_actor",
			"routing_instance",
		}
	}
	return []string{
		"src_actor",
		"dst_actor",
		"protocol",
		"src_if_index",
		"src_port_name",
		"src_port_id",
		"dst_if_index",
		"dst_port_name",
		"dst_port_id",
	}
}

func snmpTopologyV1Presentation() *topologyapi.Presentation {
	return &topologyapi.Presentation{
		ProfileVersion: "snmp-l2.v2",
		Selection: &topologyapi.SelectionPresentation{
			ActorClick: &topologyapi.ActorClickPresentation{Mode: "highlight_connections"},
		},
		Legend: &topologyapi.PresentationLegend{
			Actors: []topologyapi.LegendEntry{
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
				{Type: "custom", Label: "Other"},
				{Type: "endpoint", Label: "Inferred endpoint"},
				{Type: "segment", Label: "Network segment"},
				{Type: snmpTopologyV1ActorL3SubnetSegment, Label: "L3 subnet"},
			},
			Links: []topologyapi.LegendEntry{
				{Type: snmpTopologyV1LinkLLDP, Label: "LLDP"},
				{Type: snmpTopologyV1LinkCDP, Label: "CDP"},
				{Type: snmpTopologyV1LinkSNMP, Label: "SNMP"},
				{Type: snmpTopologyV1LinkBridge, Label: "Bridge"},
				{Type: snmpTopologyV1LinkL3Subnet, Label: "L3 subnet"},
				{Type: snmpTopologyV1LinkL3SubnetMembership, Label: "L3 subnet membership"},
				{Type: snmpTopologyV1LinkOSPF, Label: "OSPF adjacency"},
				{Type: snmpTopologyV1LinkBGP, Label: "BGP adjacency"},
				{Type: snmpTopologyV1LinkProbable, Label: "Probable"},
			},
			Ports: []topologyapi.LegendEntry{
				{Type: "lldp", Label: "lldp/cdp"},
				{Type: "switch_facing", Label: "switch-facing"},
				{Type: "host_facing", Label: "host-facing"},
				{Type: "host_candidate", Label: "host-candidate"},
				{Type: "trunk", Label: "trunk"},
				{Type: "access", Label: "access"},
				{Type: "topology", Label: "unclassified"},
				{Type: "idle", Label: "idle"},
				{Type: "unknown", Label: "unknown"},
			},
		},
		PortFields: []topologyapi.PresentationField{
			{Key: "type", Label: "Type"},
			{Key: "role", Label: "Role"},
			{Key: "status", Label: "Status"},
			{Key: "mode", Label: "Mode"},
			{Key: "sources", Label: "Sources"},
		},
	}
}
