// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"fmt"
	"math"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/topology/engine"
	topologyv1 "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
)

const (
	snmpTopologyV1ProducerSource = "snmp-l2"
	snmpTopologyV1Instance       = "local"

	snmpTopologyV1ActorDevice   = "device"
	snmpTopologyV1ActorEndpoint = "endpoint"
	snmpTopologyV1ActorSegment  = "segment"

	snmpTopologyV1LinkObservation = "l2_observation"
	snmpTopologyV1LinkLLDP        = "lldp"
	snmpTopologyV1LinkCDP         = "cdp"
	snmpTopologyV1LinkBridge      = "bridge"
	snmpTopologyV1LinkFDB         = "fdb"
	snmpTopologyV1LinkSTP         = "stp"
	snmpTopologyV1LinkARP         = "arp"
	snmpTopologyV1LinkSNMP        = "snmp"
	snmpTopologyV1LinkProbable    = "probable"
)

var topologyV1IDInvalidChars = regexp.MustCompile(`[^A-Za-z0-9_.:-]+`)

func snmpTopologyV1ActorTypes() map[string]topologyv1.ActorType {
	types := make(map[string]topologyv1.ActorType)
	addDevice := func(id, label, colorSlot, icon string) {
		types[id] = topologyv1.ActorType{
			Layer:             "network",
			Identity:          []string{"id"},
			MergeIdentity:     []string{"chassis_ids", "mac_addresses", "ip_addresses", "sys_name"},
			AggregationScopes: []string{"device", "network"},
			Presentation: &topologyv1.ActorPresentation{
				Label:     label,
				Role:      "actor",
				Icon:      icon,
				ColorSlot: colorSlot,
				Border:    &topologyv1.BorderPresentation{Enabled: topologyv1.Bool(true)},
				Size:      &topologyv1.ActorSizePresentation{Mode: "link_count"},
				LabelPolicy: &topologyv1.LabelPolicy{
					Columns:   []string{"display_name", "sys_name"},
					Fallback:  "type_label",
					MaxLength: 80,
					Array:     "reject",
				},
				Ports: &topologyv1.ActorPortsPresentation{
					ShowBullets: true,
					Sources: []topologyv1.PortSourcePresentation{
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

	types[snmpTopologyV1ActorEndpoint] = topologyv1.ActorType{
		Layer:             "network",
		Identity:          []string{"id"},
		MergeIdentity:     []string{"mac_addresses", "ip_addresses"},
		AggregationScopes: []string{"endpoint", "network"},
		Presentation: &topologyv1.ActorPresentation{
			Label:     "Inferred endpoint",
			Role:      "endpoint",
			Icon:      "remote-endpoint",
			ColorSlot: "derived",
			Border:    &topologyv1.BorderPresentation{Enabled: topologyv1.Bool(true)},
			LabelPolicy: &topologyv1.LabelPolicy{
				Columns:   []string{"display_name"},
				Fallback:  "type_label",
				MaxLength: 80,
				Array:     "reject",
			},
			Modal: snmpTopologyV1EndpointModal(),
		},
	}
	types[snmpTopologyV1ActorSegment] = topologyv1.ActorType{
		Layer:             "network",
		Identity:          []string{"id"},
		MergeIdentity:     []string{"id"},
		ParentIdentity:    []string{"parent_devices"},
		AggregationScopes: []string{"segment", "network"},
		Presentation: &topologyv1.ActorPresentation{
			Label:     "Network segment",
			Role:      "group",
			Icon:      "segment",
			ColorSlot: "dim",
			LabelPolicy: &topologyv1.LabelPolicy{
				Columns:   []string{"display_name"},
				Fallback:  "type_label",
				MaxLength: 80,
				Array:     "reject",
			},
			Modal: snmpTopologyV1EndpointModal(),
		},
	}
	types["custom"] = topologyv1.ActorType{
		Layer:             "custom",
		Identity:          []string{"id"},
		MergeIdentity:     []string{"id"},
		AggregationScopes: []string{"network"},
		Presentation: &topologyv1.ActorPresentation{
			Label:     "Custom",
			Role:      "actor",
			Icon:      "service",
			ColorSlot: "neutral",
			LabelPolicy: &topologyv1.LabelPolicy{
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

func snmpTopologyV1DeviceModal() *topologyv1.ModalPresentation {
	return &topologyv1.ModalPresentation{
		Labels:       snmpTopologyV1DeviceModalLabels(),
		MiniTopology: &topologyv1.ModalMiniTopologyPresentation{Depth: 1},
		Sections: []topologyv1.ModalSection{
			{
				ID:    "ports",
				Label: "Ports",
				Order: 1,
				Source: topologyv1.ModalSource{
					Kind:  "actor_table",
					Table: "actor_ports",
				},
				OwnerFilter: &topologyv1.ModalOwnerFilter{
					Mode:        "actor_column",
					ActorColumn: "actor",
				},
				Columns: snmpTopologyV1PortModalColumns(),
				Sort:    &topologyv1.ModalSort{Column: "if_index", Direction: "asc"},
			},
			snmpTopologyV1PortLinksSection(2),
		},
	}
}

func snmpTopologyV1EndpointModal() *topologyv1.ModalPresentation {
	return &topologyv1.ModalPresentation{
		Labels:       snmpTopologyV1EndpointModalLabels(),
		MiniTopology: &topologyv1.ModalMiniTopologyPresentation{Depth: 1},
		Sections:     []topologyv1.ModalSection{snmpTopologyV1LinksSection(1)},
	}
}

func snmpTopologyV1DeviceModalLabels() *topologyv1.ModalLabelsPresentation {
	return &topologyv1.ModalLabelsPresentation{
		Table: "actor_labels",
		Identification: &topologyv1.ModalLabelIdentificationPresentation{
			Fields: []topologyv1.ModalLabelIdentificationField{
				{Key: "display_name", Label: "Name", MaxValues: 1},
				{Key: "management_ip", Label: "Management IP", MaxValues: 1},
				{Key: "vendor", Label: "Vendor", MaxValues: 1},
				{Key: "model", Label: "Model", MaxValues: 1},
				{Key: "ports_total", Label: "Ports", MaxValues: 1},
				{Key: "lldp_neighbor_count", Label: "LLDP", MaxValues: 1},
				{Key: "cdp_neighbor_count", Label: "CDP", MaxValues: 1},
			},
		},
	}
}

func snmpTopologyV1EndpointModalLabels() *topologyv1.ModalLabelsPresentation {
	return &topologyv1.ModalLabelsPresentation{
		Table: "actor_labels",
		Identification: &topologyv1.ModalLabelIdentificationPresentation{
			Fields: []topologyv1.ModalLabelIdentificationField{
				{Key: "display_name", Label: "Name", MaxValues: 1},
				{Key: "ip_address", Label: "IP", MaxValues: 2},
				{Key: "mac_address", Label: "MAC", MaxValues: 2},
				{Key: "hostname", Label: "Hostname", MaxValues: 2},
			},
		},
	}
}

func snmpTopologyV1LinksSection(order int) topologyv1.ModalSection {
	return topologyv1.ModalSection{
		ID:    "links",
		Label: "Links",
		Order: order,
		Source: topologyv1.ModalSource{
			Kind: "links",
		},
		OwnerFilter: &topologyv1.ModalOwnerFilter{
			Mode:           "incident_link",
			SrcActorColumn: "src_actor",
			DstActorColumn: "dst_actor",
		},
		Columns: []topologyv1.ModalColumn{
			{
				ID:    "remote",
				Label: "Remote Actor",
				Projection: topologyv1.ModalProjection{
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

func snmpTopologyV1PortLinksSection(order int) topologyv1.ModalSection {
	return topologyv1.ModalSection{
		ID:    "port_neighbors",
		Label: "Port Neighbors",
		Order: order,
		Source: topologyv1.ModalSource{
			Kind:  "actor_table",
			Table: "actor_port_links",
		},
		OwnerFilter: &topologyv1.ModalOwnerFilter{
			Mode:        "actor_column",
			ActorColumn: "actor",
		},
		Columns:    snmpTopologyV1PortLinkModalColumns(),
		Sort:       &topologyv1.ModalSort{Column: "if_index", Direction: "asc"},
		EmptyLabel: "No port neighbors",
	}
}

func modalSelectedSidePortColumn(id, label, selectedSrcPortColumn, selectedDstPortColumn string) topologyv1.ModalColumn {
	return topologyv1.ModalColumn{
		ID:    id,
		Label: label,
		Projection: topologyv1.ModalProjection{
			Kind:             "selected_side_endpoint",
			SrcActorColumn:   "src_actor",
			DstActorColumn:   "dst_actor",
			LocalPortColumn:  selectedSrcPortColumn,
			RemotePortColumn: selectedDstPortColumn,
		},
		Cell: "text",
	}
}

func modalDirectColumn(id, label, sourceColumn, cell string) topologyv1.ModalColumn {
	return topologyv1.ModalColumn{
		ID:         id,
		Label:      label,
		Projection: topologyv1.ModalProjection{Kind: "direct", Column: sourceColumn},
		Cell:       cell,
	}
}

func modalDirectColumnWithVisibility(id, label, sourceColumn, cell, visibility string) topologyv1.ModalColumn {
	column := modalDirectColumn(id, label, sourceColumn, cell)
	column.Visibility = visibility
	return column
}

func modalActorRefColumn(id, label, actorColumn string) topologyv1.ModalColumn {
	return topologyv1.ModalColumn{
		ID:    id,
		Label: label,
		Projection: topologyv1.ModalProjection{
			Kind:        "actor_ref_label",
			ActorColumn: actorColumn,
		},
		Cell: "actor_link",
	}
}

func modalActorRefColumnWithVisibility(id, label, actorColumn, visibility string) topologyv1.ModalColumn {
	column := modalActorRefColumn(id, label, actorColumn)
	column.Visibility = visibility
	return column
}

func snmpTopologyV1PortTypes() map[string]topologyv1.PortType {
	return map[string]topologyv1.PortType{
		"lldp":           {Presentation: &topologyv1.PortPresentation{Label: "lldp/cdp", ColorSlot: "accent", Opacity: "normal"}},
		"switch_facing":  {Presentation: &topologyv1.PortPresentation{Label: "switch-facing", ColorSlot: "primary", Opacity: "normal"}},
		"host_facing":    {Presentation: &topologyv1.PortPresentation{Label: "host-facing", ColorSlot: "secondary", Opacity: "normal"}},
		"host_candidate": {Presentation: &topologyv1.PortPresentation{Label: "host-candidate", ColorSlot: "info", Opacity: "normal"}},
		"trunk":          {Presentation: &topologyv1.PortPresentation{Label: "trunk", ColorSlot: "warning", Opacity: "normal"}},
		"access":         {Presentation: &topologyv1.PortPresentation{Label: "access", ColorSlot: "derived", Opacity: "normal"}},
		"topology":       {Presentation: &topologyv1.PortPresentation{Label: "unclassified", ColorSlot: "neutral", Opacity: "normal"}},
		"idle":           {Presentation: &topologyv1.PortPresentation{Label: "idle", ColorSlot: "muted", Opacity: "muted"}},
		"unknown":        {Presentation: &topologyv1.PortPresentation{Label: "unknown", ColorSlot: "dim", Opacity: "muted"}},
	}
}

type snmpTopologyV1LinkTypeSpec struct {
	id        string
	label     string
	colorSlot string
	lineStyle string
	width     string
}

func snmpTopologyV1LinkTypeSpecs() []snmpTopologyV1LinkTypeSpec {
	return []snmpTopologyV1LinkTypeSpec{
		{id: snmpTopologyV1LinkLLDP, label: "LLDP", colorSlot: "accent", lineStyle: "solid", width: "thick"},
		{id: snmpTopologyV1LinkCDP, label: "CDP", colorSlot: "accent", lineStyle: "solid", width: "thick"},
		{id: snmpTopologyV1LinkBridge, label: "Bridge", colorSlot: "neutral", lineStyle: "solid", width: "normal"},
		{id: snmpTopologyV1LinkFDB, label: "FDB", colorSlot: "neutral", lineStyle: "solid", width: "normal"},
		{id: snmpTopologyV1LinkSTP, label: "STP", colorSlot: "muted", lineStyle: "solid", width: "normal"},
		{id: snmpTopologyV1LinkARP, label: "ARP", colorSlot: "muted", lineStyle: "solid", width: "normal"},
		{id: snmpTopologyV1LinkSNMP, label: "SNMP", colorSlot: "primary", lineStyle: "solid", width: "normal"},
		{id: snmpTopologyV1LinkProbable, label: "Probable", colorSlot: "dim", lineStyle: "solid", width: "normal"},
		{id: snmpTopologyV1LinkObservation, label: "L2 observation", colorSlot: "neutral", lineStyle: "solid", width: "normal"},
	}
}

func snmpTopologyV1LinkTypes() map[string]topologyv1.LinkType {
	types := make(map[string]topologyv1.LinkType)
	for _, spec := range snmpTopologyV1LinkTypeSpecs() {
		types[spec.id] = topologyv1.LinkType{
			Orientation:   "observed_bidirectional",
			DirectionRole: "observation",
			Aggregation: topologyv1.LinkAggregation{
				Direction: "canonicalize_unordered",
				Evidence:  "append",
				Metrics: map[string]string{
					"evidence_count": "sum",
				},
			},
			EvidenceTypes: []string{spec.id},
			Presentation: &topologyv1.LinkPresentation{
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

func snmpTopologyV1EvidenceTypes() map[string]topologyv1.EvidenceType {
	types := make(map[string]topologyv1.EvidenceType)
	for _, spec := range snmpTopologyV1LinkTypeSpecs() {
		types[spec.id] = topologyv1.EvidenceType{
			LinkType: spec.id,
			Role:     "observation_evidence",
			Columns:  snmpTopologyV1EvidenceColumns(),
			MatchColumns: []string{
				"src_actor",
				"dst_actor",
				"protocol",
				"src_endpoint",
				"dst_endpoint",
			},
		}
	}
	return types
}

func snmpTopologyV1Presentation() *topologyv1.Presentation {
	return &topologyv1.Presentation{
		ProfileVersion: "snmp-l2.v1",
		Selection: &topologyv1.SelectionPresentation{
			ActorClick: &topologyv1.ActorClickPresentation{Mode: "highlight_connections"},
		},
		Legend: &topologyv1.PresentationLegend{
			Actors: []topologyv1.LegendEntry{
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
			},
			Links: []topologyv1.LegendEntry{
				{Type: snmpTopologyV1LinkLLDP, Label: "LLDP"},
				{Type: snmpTopologyV1LinkCDP, Label: "CDP"},
				{Type: snmpTopologyV1LinkSNMP, Label: "SNMP"},
				{Type: snmpTopologyV1LinkBridge, Label: "Bridge"},
				{Type: snmpTopologyV1LinkProbable, Label: "Probable"},
			},
			Ports: []topologyv1.LegendEntry{
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
		PortFields: []topologyv1.PresentationField{
			{Key: "type", Label: "Type"},
			{Key: "role", Label: "Role"},
			{Key: "status", Label: "Status"},
			{Key: "mode", Label: "Mode"},
			{Key: "sources", Label: "Sources"},
		},
	}
}

func snmpTopologyToV1(data topologyData) (topologyv1.Data, error) {
	stringsDict := topologyv1.NewStringDictionary("")
	actorRows, actorIndex := buildSNMPTopologyV1Actors(data.Actors, stringsDict)

	linkRows, evidenceSections, err := buildSNMPTopologyV1Links(data.Links, actorIndex, stringsDict)
	if err != nil {
		return topologyv1.Data{}, err
	}

	portNeighborSummaries := buildSNMPTopologyV1PortNeighborSummaries(data.Links, actorIndex)
	actorDetails, tableTypes, err := buildSNMPTopologyV1ActorDetails(data.Actors, stringsDict, portNeighborSummaries)
	if err != nil {
		return topologyv1.Data{}, err
	}
	if tableTypes == nil {
		tableTypes = make(map[string]topologyv1.TableType)
	}
	if _, ok := tableTypes["actor_labels"]; !ok {
		tableTypes["actor_labels"] = snmpTopologyV1ActorLabelsTableType()
	}
	if _, ok := tableTypes["actor_ports"]; !ok {
		tableTypes["actor_ports"] = snmpTopologyV1ActorPortsTableType()
	}
	tableTypes["actor_port_links"] = snmpTopologyV1ActorPortLinksTableType()
	portLinksTable, err := buildSNMPTopologyV1ActorPortLinksTable(data.Links, actorIndex, stringsDict)
	if err != nil {
		return topologyv1.Data{}, err
	}
	if portLinksTable.Rows > 0 {
		if actorDetails == nil {
			actorDetails = make(map[string]topologyv1.DetailTable)
		}
		actorDetails["actor_port_links"] = topologyv1.DetailTable{
			Type:  "actor_port_links",
			Table: portLinksTable,
		}
	}

	types := topologyv1.TypeRegistry{
		ActorTypes:    snmpTopologyV1ActorTypes(),
		LinkTypes:     snmpTopologyV1LinkTypes(),
		PortTypes:     snmpTopologyV1PortTypes(),
		EvidenceTypes: snmpTopologyV1EvidenceTypes(),
		TableTypes:    tableTypes,
		AggregationScopes: map[string]topologyv1.AggregationScope{
			"device": {
				Columns:        []string{"id"},
				EvidencePolicy: "preserve",
			},
			"network": {
				Columns:        []string{"type"},
				EvidencePolicy: "preserve",
			},
			"segment": {
				Columns:        []string{"id"},
				EvidencePolicy: "preserve",
			},
			"endpoint": {
				Columns:        []string{"id"},
				EvidencePolicy: "preserve",
			},
		},
	}

	if len(types.TableTypes) == 0 {
		types.TableTypes = nil
	}

	payload := topologyv1.Data{
		SchemaVersion: topologyv1.SchemaVersion,
		Producer: topologyv1.Producer{
			Source:   snmpTopologyV1ProducerSource,
			Instance: firstNonEmptyString(data.AgentID, snmpTopologyV1Instance),
			Plugin:   "go.d/snmp_topology",
			Capabilities: []string{
				"lldp",
				"cdp",
				"fdb",
				"stp",
			},
		},
		CollectedAt: data.CollectedAt,
		View: &topologyv1.View{
			ID:    firstNonEmptyString(data.View, "summary"),
			Scope: "network",
			Mode:  "detailed",
		},
		Dictionaries: topologyv1.Dictionaries{
			"strings": stringsDict.Values(),
		},
		Types:        types,
		Presentation: snmpTopologyV1Presentation(),
		Actors:       actorRows,
		Links:        linkRows,
		Evidence:     evidenceSections,
		Stats:        cloneAnyMapForTopologyV1(data.Stats),
	}
	if payload.CollectedAt.IsZero() {
		payload.CollectedAt = time.Now().UTC()
	}
	if actorDetails != nil {
		payload.Tables = &topologyv1.DetailTables{
			Actor: actorDetails,
		}
	}
	return payload, nil
}

func snmpTopologyV1ActorPortsTableType() topologyv1.TableType {
	return topologyv1.TableType{
		Role:        "actor_detail",
		Owner:       "actor",
		Aggregation: "append",
		Columns:     snmpTopologyV1ActorPortsColumns(),
		Presentation: &topologyv1.TableTypePresentation{
			Label:   "Ports",
			Order:   1,
			Columns: snmpTopologyV1PortModalColumns(),
		},
	}
}

func snmpTopologyV1PortModalColumns() []topologyv1.ModalColumn {
	return []topologyv1.ModalColumn{
		modalDirectColumn("port_number", "Port #", "port_number", "number"),
		modalDirectColumn("name", "Port", "name", "text"),
		modalDirectColumn("oper_status", "Status", "oper_status", "badge"),
		modalDirectColumn("admin_status", "Admin", "admin_status", "badge"),
		modalDirectColumn("port_type", "Type", "port_type", "badge"),
		modalDirectColumn("link_mode", "Mode", "link_mode", "badge"),
		modalDirectColumn("topology_role", "Role", "topology_role", "badge"),
		modalDirectColumn("vlan_ids", "VLANs", "vlan_ids", "array_count"),
		modalDirectColumn("fdb_mac_count", "FDB", "fdb_mac_count", "number"),
		modalDirectColumn("link_count", "Links", "link_count", "number"),
		modalDirectColumn("neighbor_count", "Neighbors", "neighbor_count", "number"),
		modalActorRefColumnWithVisibility("neighbor_actor", "Neighbor", "neighbor_actor", "expanded"),
		modalDirectColumnWithVisibility("neighbor_port_name", "Neighbor Port", "neighbor_port_name", "text", "expanded"),
		modalDirectColumnWithVisibility("if_index", "SNMP ifIndex", "if_index", "number", "expanded"),
		modalDirectColumnWithVisibility("if_name", "ifName", "if_name", "text", "expanded"),
		modalDirectColumnWithVisibility("if_descr", "ifDescr", "if_descr", "text", "expanded"),
		modalDirectColumnWithVisibility("if_alias", "Alias", "if_alias", "text", "expanded"),
		modalDirectColumnWithVisibility("port_id", "Source Port ID", "port_id", "text", "expanded"),
		modalDirectColumnWithVisibility("mac", "MAC", "mac", "text", "expanded"),
		modalDirectColumnWithVisibility("speed", "Speed", "speed", "number", "expanded"),
		modalDirectColumnWithVisibility("stp_state", "STP", "stp_state", "badge", "expanded"),
		modalDirectColumnWithVisibility("neighbors", "Neighbor Data", "neighbors", "debug_json", "debug"),
		modalDirectColumnWithVisibility("vlans", "VLAN Data", "vlans", "debug_json", "debug"),
		modalDirectColumnWithVisibility("extra", "Extra", "extra", "debug_json", "debug"),
	}
}

func snmpTopologyV1ActorPortLinksTableType() topologyv1.TableType {
	return topologyv1.TableType{
		Role:        "actor_detail",
		Owner:       "actor",
		Aggregation: "append",
		Columns:     snmpTopologyV1ActorPortLinksColumns(),
		Presentation: &topologyv1.TableTypePresentation{
			Label:   "Port Neighbors",
			Order:   2,
			Columns: snmpTopologyV1PortLinkModalColumns(),
		},
	}
}

func snmpTopologyV1PortLinkModalColumns() []topologyv1.ModalColumn {
	return []topologyv1.ModalColumn{
		modalDirectColumn("if_index", "Port ID", "if_index", "number"),
		modalDirectColumn("port_name", "Port", "port_name", "text"),
		modalActorRefColumn("remote_actor", "Remote Actor", "remote_actor"),
		modalDirectColumn("remote_port_name", "Remote Port", "remote_port_name", "text"),
		modalDirectColumn("type", "Type", "type", "badge"),
		modalDirectColumn("state", "State", "state", "badge"),
		modalDirectColumn("evidence_count", "Evidence", "evidence_count", "number"),
		modalDirectColumnWithVisibility("protocol", "Protocol", "protocol", "badge", "expanded"),
		modalDirectColumnWithVisibility("remote_if_index", "Remote Port ID", "remote_if_index", "number", "expanded"),
		modalDirectColumnWithVisibility("port_id", "Source Port ID", "port_id", "text", "expanded"),
		modalDirectColumnWithVisibility("remote_port_id", "Remote Source Port ID", "remote_port_id", "text", "expanded"),
		modalDirectColumnWithVisibility("confidence", "Confidence", "confidence", "badge", "expanded"),
		modalDirectColumnWithVisibility("inference", "Inference", "inference", "badge", "expanded"),
		modalDirectColumnWithVisibility("attachment_mode", "Attachment", "attachment_mode", "badge", "expanded"),
		modalDirectColumnWithVisibility("discovered_at", "Discovered", "discovered_at", "timestamp", "expanded"),
		modalDirectColumnWithVisibility("last_seen", "Last Seen", "last_seen", "timestamp", "expanded"),
	}
}

func snmpTopologyV1ActorLabelsTableType() topologyv1.TableType {
	return topologyv1.TableType{
		Role:        "actor_inventory",
		Owner:       "actor",
		Aggregation: "set",
		Columns: []topologyv1.Column{
			topologyv1.NewColumn("actor", "actor_ref", topologyv1.WithRole("reference")),
			topologyv1.NewColumn("key", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithRole("attribute")),
			topologyv1.NewColumn("value", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithRole("attribute")),
			topologyv1.NewColumn("source", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable(), topologyv1.WithRole("attribute")),
			topologyv1.NewColumn("kind", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable(), topologyv1.WithRole("attribute")),
			topologyv1.NewColumn("value_index", "uint", topologyv1.WithNullable(), topologyv1.WithRole("attribute")),
		},
		Presentation: &topologyv1.TableTypePresentation{
			Label: "Labels",
			Order: 0,
			Columns: []topologyv1.ModalColumn{
				modalDirectColumn("key", "Label", "key", "text"),
				modalDirectColumn("value", "Value", "value", "text"),
				modalDirectColumn("source", "Source", "source", "badge"),
				modalDirectColumn("kind", "Kind", "kind", "badge"),
			},
		},
	}
}

func snmpTopologyV1ActorPortsColumns() []topologyv1.Column {
	return []topologyv1.Column{
		topologyv1.NewColumn("actor", "actor_ref", topologyv1.WithRole("reference")),
		topologyv1.NewColumn("port_number", "uint", topologyv1.WithNullable()),
		topologyv1.NewColumn("if_index", "uint", topologyv1.WithNullable()),
		topologyv1.NewColumn("port_id", "string_ref", topologyv1.WithNullable(), topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("name", "string_ref", topologyv1.WithNullable(), topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("if_name", "string_ref", topologyv1.WithNullable(), topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("if_descr", "string_ref", topologyv1.WithNullable(), topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("if_alias", "string_ref", topologyv1.WithNullable(), topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("mac", "string_ref", topologyv1.WithNullable(), topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("speed", "uint", topologyv1.WithNullable()),
		topologyv1.NewColumn("topology_role", "string_ref", topologyv1.WithNullable(), topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("oper_status", "string_ref", topologyv1.WithNullable(), topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("admin_status", "string_ref", topologyv1.WithNullable(), topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("port_type", "string_ref", topologyv1.WithNullable(), topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("link_mode", "string_ref", topologyv1.WithNullable(), topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("stp_state", "string_ref", topologyv1.WithNullable(), topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("vlan_ids", "array", topologyv1.WithNullable()),
		topologyv1.NewColumn("fdb_mac_count", "uint", topologyv1.WithNullable()),
		topologyv1.NewColumn("link_count", "uint", topologyv1.WithNullable()),
		topologyv1.NewColumn("neighbor_count", "uint", topologyv1.WithNullable()),
		topologyv1.NewColumn("neighbor_actor", "actor_ref", topologyv1.WithNullable(), topologyv1.WithRole("reference")),
		topologyv1.NewColumn("neighbor_port_name", "string_ref", topologyv1.WithNullable(), topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("neighbors", "json", topologyv1.WithNullable()),
		topologyv1.NewColumn("vlans", "json", topologyv1.WithNullable()),
		topologyv1.NewColumn("extra", "json", topologyv1.WithNullable()),
	}
}

func snmpTopologyV1ActorPortLinksColumns() []topologyv1.Column {
	return []topologyv1.Column{
		topologyv1.NewColumn("actor", "actor_ref", topologyv1.WithRole("reference")),
		topologyv1.NewColumn("link", "link_ref", topologyv1.WithRole("reference")),
		topologyv1.NewColumn("remote_actor", "actor_ref", topologyv1.WithRole("reference")),
		topologyv1.NewColumn("if_index", "uint", topologyv1.WithNullable()),
		topologyv1.NewColumn("port_id", "string_ref", topologyv1.WithNullable(), topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("port_name", "string_ref", topologyv1.WithNullable(), topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("remote_if_index", "uint", topologyv1.WithNullable()),
		topologyv1.NewColumn("remote_port_id", "string_ref", topologyv1.WithNullable(), topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("remote_port_name", "string_ref", topologyv1.WithNullable(), topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("type", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithRole("group_key")),
		topologyv1.NewColumn("protocol", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithRole("group_key")),
		topologyv1.NewColumn("state", "string_ref", topologyv1.WithNullable(), topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("evidence_count", "uint", topologyv1.WithAggregation("sum")),
		topologyv1.NewColumn("confidence", "string_ref", topologyv1.WithNullable(), topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("inference", "string_ref", topologyv1.WithNullable(), topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("attachment_mode", "string_ref", topologyv1.WithNullable(), topologyv1.WithDictionary("strings")),
		topologyv1.NewColumn("discovered_at", "timestamp", topologyv1.WithNullable(), topologyv1.WithRole("timestamp")),
		topologyv1.NewColumn("last_seen", "timestamp", topologyv1.WithNullable(), topologyv1.WithRole("timestamp")),
	}
}

func buildSNMPTopologyV1Actors(actors []topologyActor, stringsDict *topologyv1.StringDictionary) (topologyv1.Table, map[string]int) {
	actorIndex := make(map[string]int, len(actors))
	ids := make([]any, len(actors))
	types := make([]any, len(actors))
	layers := make([]any, len(actors))
	sources := make([]any, len(actors))
	displayNames := make([]any, len(actors))
	chassisIDs := make([]any, len(actors))
	macAddresses := make([]any, len(actors))
	ipAddresses := make([]any, len(actors))
	hostnames := make([]any, len(actors))
	dnsNames := make([]any, len(actors))
	sysObjectIDs := make([]any, len(actors))
	sysNames := make([]any, len(actors))
	parentDevices := make([]any, len(actors))
	vendors := make([]any, len(actors))
	models := make([]any, len(actors))
	sysDescrs := make([]any, len(actors))
	sysLocations := make([]any, len(actors))
	sysContacts := make([]any, len(actors))
	managementIPs := make([]any, len(actors))
	protocols := make([]any, len(actors))
	capabilities := make([]any, len(actors))
	portsTotal := make([]any, len(actors))
	vlanCounts := make([]any, len(actors))
	fdbTotalMACs := make([]any, len(actors))
	lldpNeighborCounts := make([]any, len(actors))
	cdpNeighborCounts := make([]any, len(actors))
	endpointsTotal := make([]any, len(actors))
	chartIDPrefixes := make([]any, len(actors))
	netdataHostIDs := make([]any, len(actors))

	for i, actor := range actors {
		actorID := strings.TrimSpace(actor.ActorID)
		if actorID == "" {
			actorID = fmt.Sprintf("generated:%d", i)
		}
		actorIndex[actorID] = i
		ids[i] = stringsDict.Ref(actorID)
		types[i] = stringsDict.Ref(snmpTopologyV1ActorType(actor.ActorType))
		layers[i] = stringsDict.Ref(snmpTopologyV1ActorLayer(actor))
		sources[i] = stringsDict.Ref(firstNonEmptyString(actor.Source, snmpTopologyV1ProducerSource))
		displayNames[i] = nullableStringRef(stringsDict, snmpTopologyV1DisplayName(actor))
		chassisIDs[i] = stringArrayCell(actor.Match.ChassisIDs)
		macAddresses[i] = stringArrayCell(actor.Match.MacAddresses)
		ipAddresses[i] = stringArrayCell(actor.Match.IPAddresses)
		hostnames[i] = stringArrayCell(actor.Match.Hostnames)
		dnsNames[i] = stringArrayCell(actor.Match.DNSNames)
		sysObjectIDs[i] = stringsDict.Ref(actor.Match.SysObjectID)
		sysNames[i] = stringsDict.Ref(actor.Match.SysName)
		parentDevices[i] = stringArrayCell(anyStringSlice(actor.Attributes["parent_devices"]))
		vendors[i] = nullableStringRef(stringsDict, firstNonEmptyString(anyStringValue(actor.Attributes["vendor"]), anyStringValue(actor.Attributes["vendor_derived"])))
		models[i] = nullableStringRef(stringsDict, anyStringValue(actor.Attributes["model"]))
		sysDescrs[i] = nullableStringRef(stringsDict, anyStringValue(actor.Attributes["sys_descr"]))
		sysLocations[i] = nullableStringRef(stringsDict, anyStringValue(actor.Attributes["sys_location"]))
		sysContacts[i] = nullableStringRef(stringsDict, anyStringValue(actor.Attributes["sys_contact"]))
		managementIPs[i] = nullableStringRef(stringsDict, anyStringValue(actor.Attributes["management_ip"]))
		protocols[i] = stringArrayCell(anyStringSlice(actor.Attributes["protocols"]))
		if isEmptyArrayCell(protocols[i]) {
			// Older SNMP topology payloads used learned_sources for discovered protocols.
			protocols[i] = stringArrayCell(anyStringSlice(actor.Attributes["learned_sources"]))
		}
		if isEmptyArrayCell(protocols[i]) {
			protocols[i] = nil
		}
		capabilities[i] = stringArrayCell(anyStringSlice(actor.Attributes["capabilities"]))
		if isEmptyArrayCell(capabilities[i]) {
			capabilities[i] = nil
		}
		portsTotal[i] = nullableUintValue(actor.Attributes["ports_total"])
		vlanCounts[i] = nullableUintValue(actor.Attributes["vlan_count"])
		fdbTotalMACs[i] = nullableUintValue(actor.Attributes["fdb_total_macs"])
		lldpNeighborCounts[i] = nullableUintValue(actor.Attributes["lldp_neighbor_count"])
		cdpNeighborCounts[i] = nullableUintValue(actor.Attributes["cdp_neighbor_count"])
		endpointsTotal[i] = nullableUintValue(actor.Attributes["endpoints_total"])
		chartIDPrefixes[i] = nullableStringRef(stringsDict, anyStringValue(actor.Attributes["chart_id_prefix"]))
		netdataHostIDs[i] = nullableStringRef(stringsDict, anyStringValue(actor.Attributes["netdata_host_id"]))
	}

	return topologyv1.MustTable(len(actors),
		[]topologyv1.Column{
			topologyv1.NewColumn("id", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithRole("identity")),
			topologyv1.NewColumn("type", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithRole("group_key")),
			topologyv1.NewColumn("layer", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithRole("group_key")),
			topologyv1.NewColumn("source", "string_ref", topologyv1.WithDictionary("strings")),
			topologyv1.NewColumn("display_name", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable(), topologyv1.WithRole("attribute")),
			topologyv1.NewColumn("chassis_ids", "array", topologyv1.WithRole("merge_identity")),
			topologyv1.NewColumn("mac_addresses", "array", topologyv1.WithRole("merge_identity")),
			topologyv1.NewColumn("ip_addresses", "array", topologyv1.WithRole("merge_identity")),
			topologyv1.NewColumn("hostnames", "array"),
			topologyv1.NewColumn("dns_names", "array"),
			topologyv1.NewColumn("sys_object_id", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithRole("merge_identity")),
			topologyv1.NewColumn("sys_name", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithRole("merge_identity")),
			topologyv1.NewColumn("parent_devices", "array", topologyv1.WithRole("parent_identity")),
			topologyv1.NewColumn("vendor", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable()),
			topologyv1.NewColumn("model", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable()),
			topologyv1.NewColumn("sys_descr", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable()),
			topologyv1.NewColumn("sys_location", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable()),
			topologyv1.NewColumn("sys_contact", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable()),
			topologyv1.NewColumn("management_ip", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable()),
			topologyv1.NewColumn("protocols", "array", topologyv1.WithNullable()),
			topologyv1.NewColumn("capabilities", "array", topologyv1.WithNullable()),
			topologyv1.NewColumn("ports_total", "uint", topologyv1.WithNullable()),
			topologyv1.NewColumn("vlan_count", "uint", topologyv1.WithNullable()),
			topologyv1.NewColumn("fdb_total_macs", "uint", topologyv1.WithNullable()),
			topologyv1.NewColumn("lldp_neighbor_count", "uint", topologyv1.WithNullable()),
			topologyv1.NewColumn("cdp_neighbor_count", "uint", topologyv1.WithNullable()),
			topologyv1.NewColumn("endpoints_total", "uint", topologyv1.WithNullable()),
			topologyv1.NewColumn("chart_id_prefix", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable()),
			topologyv1.NewColumn("netdata_host_id", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable()),
		},
		[]topologyv1.ColumnEncoding{
			topologyv1.Values(ids...),
			topologyv1.Values(types...),
			topologyv1.Values(layers...),
			topologyv1.Values(sources...),
			topologyv1.Values(displayNames...),
			topologyv1.Values(chassisIDs...),
			topologyv1.Values(macAddresses...),
			topologyv1.Values(ipAddresses...),
			topologyv1.Values(hostnames...),
			topologyv1.Values(dnsNames...),
			topologyv1.Values(sysObjectIDs...),
			topologyv1.Values(sysNames...),
			topologyv1.Values(parentDevices...),
			topologyv1.Values(vendors...),
			topologyv1.Values(models...),
			topologyv1.Values(sysDescrs...),
			topologyv1.Values(sysLocations...),
			topologyv1.Values(sysContacts...),
			topologyv1.Values(managementIPs...),
			topologyv1.Values(protocols...),
			topologyv1.Values(capabilities...),
			topologyv1.Values(portsTotal...),
			topologyv1.Values(vlanCounts...),
			topologyv1.Values(fdbTotalMACs...),
			topologyv1.Values(lldpNeighborCounts...),
			topologyv1.Values(cdpNeighborCounts...),
			topologyv1.Values(endpointsTotal...),
			topologyv1.Values(chartIDPrefixes...),
			topologyv1.Values(netdataHostIDs...),
		},
	), actorIndex
}

func buildSNMPTopologyV1Links(
	links []topologyLink,
	actorIndex map[string]int,
	stringsDict *topologyv1.StringDictionary,
) (topologyv1.Table, topologyv1.EvidenceMap, error) {
	srcActors := make([]any, len(links))
	dstActors := make([]any, len(links))
	linkTypes := make([]any, len(links))
	protocols := make([]any, len(links))
	directions := make([]any, len(links))
	states := make([]any, len(links))
	srcPortNames := make([]any, len(links))
	dstPortNames := make([]any, len(links))
	evidenceCounts := make([]any, len(links))
	discoveredAt := make([]any, len(links))
	lastSeen := make([]any, len(links))
	evidenceRowsByType := make(map[string]*snmpTopologyV1EvidenceRows)

	for i, link := range links {
		src, ok := actorIndex[strings.TrimSpace(link.SrcActorID)]
		if !ok {
			return topologyv1.Table{}, nil, fmt.Errorf("link %d references unknown source actor %q", i, link.SrcActorID)
		}
		dst, ok := actorIndex[strings.TrimSpace(link.DstActorID)]
		if !ok {
			return topologyv1.Table{}, nil, fmt.Errorf("link %d references unknown destination actor %q", i, link.DstActorID)
		}
		protocol := firstNonEmptyString(link.Protocol, link.LinkType, "l2")
		linkType := snmpTopologyV1LinkType(link)
		srcActors[i] = src
		dstActors[i] = dst
		linkTypes[i] = stringsDict.Ref(linkType)
		protocols[i] = stringsDict.Ref(protocol)
		directions[i] = stringsDict.Ref(firstNonEmptyString(link.Direction, "observed"))
		states[i] = nullableStringRef(stringsDict, link.State)
		srcPortNames[i] = nullableStringRef(stringsDict, topologyV1EndpointPortName(link.Src))
		dstPortNames[i] = nullableStringRef(stringsDict, topologyV1EndpointPortName(link.Dst))
		evidenceCounts[i] = 1
		discoveredAt[i] = nullableTime(link.DiscoveredAt)
		lastSeen[i] = nullableTime(link.LastSeen)
		srcEndpoint := nullableJSON(link.Src.Attributes)
		dstEndpoint := nullableJSON(link.Dst.Attributes)
		metrics := nullableJSON(link.Metrics)

		evidenceRows := evidenceRowsByType[linkType]
		if evidenceRows == nil {
			evidenceRows = &snmpTopologyV1EvidenceRows{}
			evidenceRowsByType[linkType] = evidenceRows
		}
		evidenceRows.linkRefs = append(evidenceRows.linkRefs, i)
		evidenceRows.srcActors = append(evidenceRows.srcActors, src)
		evidenceRows.dstActors = append(evidenceRows.dstActors, dst)
		evidenceRows.protocols = append(evidenceRows.protocols, stringsDict.Ref(protocol))
		evidenceRows.directions = append(evidenceRows.directions, stringsDict.Ref(firstNonEmptyString(link.Direction, "observed")))
		evidenceRows.states = append(evidenceRows.states, nullableStringRef(stringsDict, link.State))
		evidenceRows.srcPortNames = append(evidenceRows.srcPortNames, nullableStringRef(stringsDict, topologyV1EndpointPortName(link.Src)))
		evidenceRows.dstPortNames = append(evidenceRows.dstPortNames, nullableStringRef(stringsDict, topologyV1EndpointPortName(link.Dst)))
		evidenceRows.srcIfIndexes = append(evidenceRows.srcIfIndexes, nullableUintValue(link.Src.Attributes["if_index"]))
		evidenceRows.dstIfIndexes = append(evidenceRows.dstIfIndexes, nullableUintValue(link.Dst.Attributes["if_index"]))
		evidenceRows.srcManagementIPs = append(evidenceRows.srcManagementIPs, nullableStringRef(stringsDict, topologyV1EndpointString(link.Src, "management_ip")))
		evidenceRows.dstManagementIPs = append(evidenceRows.dstManagementIPs, nullableStringRef(stringsDict, topologyV1EndpointString(link.Dst, "management_ip")))
		evidenceRows.confidences = append(evidenceRows.confidences, nullableStringRef(stringsDict, topologyMetricValueString(link.Metrics, "confidence")))
		evidenceRows.inferences = append(evidenceRows.inferences, nullableStringRef(stringsDict, topologyMetricValueString(link.Metrics, "inference")))
		evidenceRows.attachmentModes = append(evidenceRows.attachmentModes, nullableStringRef(stringsDict, topologyMetricValueString(link.Metrics, "attachment_mode")))
		evidenceRows.srcEndpoints = append(evidenceRows.srcEndpoints, srcEndpoint)
		evidenceRows.dstEndpoints = append(evidenceRows.dstEndpoints, dstEndpoint)
		evidenceRows.metrics = append(evidenceRows.metrics, metrics)
	}

	linkTable := topologyv1.MustTable(len(links),
		[]topologyv1.Column{
			topologyv1.NewColumn("src_actor", "actor_ref", topologyv1.WithRole("reference")),
			topologyv1.NewColumn("dst_actor", "actor_ref", topologyv1.WithRole("reference")),
			topologyv1.NewColumn("type", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithRole("group_key")),
			topologyv1.NewColumn("protocol", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithRole("group_key")),
			topologyv1.NewColumn("direction", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithRole("group_key")),
			topologyv1.NewColumn("state", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable()),
			topologyv1.NewColumn("src_port_name", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable()),
			topologyv1.NewColumn("dst_port_name", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable()),
			topologyv1.NewColumn("evidence_count", "uint", topologyv1.WithAggregation("sum")),
			topologyv1.NewColumn("discovered_at", "timestamp", topologyv1.WithNullable(), topologyv1.WithRole("timestamp")),
			topologyv1.NewColumn("last_seen", "timestamp", topologyv1.WithNullable(), topologyv1.WithRole("timestamp")),
		},
		[]topologyv1.ColumnEncoding{
			topologyv1.Values(srcActors...),
			topologyv1.Values(dstActors...),
			topologyv1.Values(linkTypes...),
			topologyv1.Values(protocols...),
			topologyv1.Values(directions...),
			topologyv1.Values(states...),
			topologyv1.Values(srcPortNames...),
			topologyv1.Values(dstPortNames...),
			topologyv1.Values(evidenceCounts...),
			topologyv1.Values(discoveredAt...),
			topologyv1.Values(lastSeen...),
		},
	)

	evidenceSections := make(topologyv1.EvidenceMap, len(evidenceRowsByType))
	for linkType, rows := range evidenceRowsByType {
		evidenceSections[linkType] = topologyv1.EvidenceSection{
			Type:  linkType,
			Table: rows.table(),
		}
	}

	return linkTable, evidenceSections, nil
}

type snmpTopologyV1EvidenceRows struct {
	linkRefs         []any
	srcActors        []any
	dstActors        []any
	protocols        []any
	directions       []any
	states           []any
	srcPortNames     []any
	dstPortNames     []any
	srcIfIndexes     []any
	dstIfIndexes     []any
	srcManagementIPs []any
	dstManagementIPs []any
	confidences      []any
	inferences       []any
	attachmentModes  []any
	srcEndpoints     []any
	dstEndpoints     []any
	metrics          []any
}

func (rows *snmpTopologyV1EvidenceRows) table() topologyv1.Table {
	return topologyv1.MustTable(len(rows.linkRefs),
		snmpTopologyV1EvidenceColumns(),
		[]topologyv1.ColumnEncoding{
			topologyv1.Values(rows.linkRefs...),
			topologyv1.Values(rows.srcActors...),
			topologyv1.Values(rows.dstActors...),
			topologyv1.Values(rows.protocols...),
			topologyv1.Values(rows.directions...),
			topologyv1.Values(rows.states...),
			topologyv1.Values(rows.srcPortNames...),
			topologyv1.Values(rows.dstPortNames...),
			topologyv1.Values(rows.srcIfIndexes...),
			topologyv1.Values(rows.dstIfIndexes...),
			topologyv1.Values(rows.srcManagementIPs...),
			topologyv1.Values(rows.dstManagementIPs...),
			topologyv1.Values(rows.confidences...),
			topologyv1.Values(rows.inferences...),
			topologyv1.Values(rows.attachmentModes...),
			topologyv1.Values(rows.srcEndpoints...),
			topologyv1.Values(rows.dstEndpoints...),
			topologyv1.Values(rows.metrics...),
		},
	)
}

func snmpTopologyV1EvidenceColumns() []topologyv1.Column {
	return []topologyv1.Column{
		topologyv1.NewColumn("link", "link_ref", topologyv1.WithRole("reference")),
		topologyv1.NewColumn("src_actor", "actor_ref", topologyv1.WithRole("reference")),
		topologyv1.NewColumn("dst_actor", "actor_ref", topologyv1.WithRole("reference")),
		topologyv1.NewColumn("protocol", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithRole("group_key")),
		topologyv1.NewColumn("direction", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithRole("group_key")),
		topologyv1.NewColumn("state", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable()),
		topologyv1.NewColumn("src_port_name", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable()),
		topologyv1.NewColumn("dst_port_name", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable()),
		topologyv1.NewColumn("src_if_index", "uint", topologyv1.WithNullable()),
		topologyv1.NewColumn("dst_if_index", "uint", topologyv1.WithNullable()),
		topologyv1.NewColumn("src_management_ip", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable()),
		topologyv1.NewColumn("dst_management_ip", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable()),
		topologyv1.NewColumn("confidence", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable()),
		topologyv1.NewColumn("inference", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable()),
		topologyv1.NewColumn("attachment_mode", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable()),
		topologyv1.NewColumn("src_endpoint", "json", topologyv1.WithNullable()),
		topologyv1.NewColumn("dst_endpoint", "json", topologyv1.WithNullable()),
		topologyv1.NewColumn("metrics", "json", topologyv1.WithNullable()),
	}
}

func snmpTopologyV1LinkType(link topologyLink) string {
	if snmpTopologyV1LinkIsProbable(link) {
		return snmpTopologyV1LinkProbable
	}

	switch strings.ToLower(strings.TrimSpace(firstNonEmptyString(link.Protocol, link.LinkType))) {
	case snmpTopologyV1LinkLLDP:
		return snmpTopologyV1LinkLLDP
	case snmpTopologyV1LinkCDP:
		return snmpTopologyV1LinkCDP
	case snmpTopologyV1LinkBridge:
		return snmpTopologyV1LinkBridge
	case snmpTopologyV1LinkFDB:
		return snmpTopologyV1LinkFDB
	case snmpTopologyV1LinkSTP:
		return snmpTopologyV1LinkSTP
	case snmpTopologyV1LinkARP:
		return snmpTopologyV1LinkARP
	case snmpTopologyV1LinkSNMP:
		return snmpTopologyV1LinkSNMP
	default:
		return snmpTopologyV1LinkObservation
	}
}

func snmpTopologyV1LinkIsProbable(link topologyLink) bool {
	if strings.EqualFold(strings.TrimSpace(link.State), snmpTopologyV1LinkProbable) {
		return true
	}
	if len(link.Metrics) == 0 {
		return false
	}
	if strings.EqualFold(topologyMetricValueString(link.Metrics, "inference"), snmpTopologyV1LinkProbable) {
		return true
	}
	return strings.HasPrefix(strings.ToLower(topologyMetricValueString(link.Metrics, "attachment_mode")), snmpTopologyV1LinkProbable+"_")
}

func buildSNMPTopologyV1ActorDetails(
	actors []topologyActor,
	stringsDict *topologyv1.StringDictionary,
	portNeighborSummaries map[snmpTopologyV1PortNeighborKey]snmpTopologyV1PortNeighborSummary,
) (map[string]topologyv1.DetailTable, map[string]topologyv1.TableType, error) {
	details := make(map[string]topologyv1.DetailTable)
	tableTypes := make(map[string]topologyv1.TableType)

	labelsTable := buildSNMPTopologyV1ActorLabelsTable(actors, stringsDict)
	details["actor_labels"] = topologyv1.DetailTable{
		Type:  "actor_labels",
		Table: labelsTable,
	}
	tableTypes["actor_labels"] = snmpTopologyV1ActorLabelsTableType()
	usedTableIDs := map[string]struct{}{
		"actor_labels":     {},
		"actor_port_links": {},
	}

	metadataTable := buildSNMPTopologyV1ActorMetadataTable(actors)
	if metadataTable.Rows > 0 {
		tableID := "actor_metadata"
		details[tableID] = topologyv1.DetailTable{
			Type:  tableID,
			Table: metadataTable,
		}
		tableTypes[tableID] = topologyv1.TableType{
			Role:        "actor_detail",
			Owner:       "actor",
			Aggregation: "append",
			Columns:     metadataTable.Columns,
			Presentation: &topologyv1.TableTypePresentation{
				Label:             "Debug metadata",
				DefaultVisibility: "debug",
				Columns: []topologyv1.ModalColumn{
					modalDirectColumn("attributes", "Attributes", "attributes", "debug_json"),
					modalDirectColumn("labels", "Labels", "labels", "debug_json"),
				},
			},
		}
		usedTableIDs[tableID] = struct{}{}
	}

	tableRowsByName := collectSNMPTopologyV1ActorTableRows(actors)
	tableNames := sortedMapKeys(tableRowsByName)
	reservedCustomTableIDs := snmpTopologyV1ReservedCustomTableIDs(tableNames)
	for _, tableName := range tableNames {
		rows := tableRowsByName[tableName]
		if len(rows) == 0 {
			continue
		}
		tableID := snmpTopologyV1ActorDetailTableID(tableName, usedTableIDs, reservedCustomTableIDs)
		var table topologyv1.Table
		var err error
		if tableID == "actor_ports" {
			table = buildSNMPTopologyV1ActorPortsTable(rows, stringsDict, portNeighborSummaries)
		} else {
			table, err = buildSNMPTopologyV1DynamicTable(rows, stringsDict)
		}
		if err != nil {
			return nil, nil, fmt.Errorf("build actor detail table %q: %w", tableName, err)
		}
		details[tableID] = topologyv1.DetailTable{
			Type:  tableID,
			Table: table,
		}
		if tableID == "actor_ports" {
			tableTypes[tableID] = snmpTopologyV1ActorPortsTableType()
		} else {
			tableTypes[tableID] = topologyv1.TableType{
				Role:        "actor_detail",
				Owner:       "actor",
				Aggregation: "append",
				Columns:     table.Columns,
			}
		}
		usedTableIDs[tableID] = struct{}{}
	}
	return details, tableTypes, nil
}

func snmpTopologyV1ReservedCustomTableIDs(tableNames []string) map[string]struct{} {
	reserved := make(map[string]struct{})
	for _, tableName := range tableNames {
		tableID := topologyID("actor_"+tableName, "actor_detail")
		switch tableID {
		case "actor_labels", "actor_metadata", "actor_port_links":
			reserved[topologyID("actor_custom_"+tableName, "actor_detail")] = struct{}{}
		}
	}
	return reserved
}

func snmpTopologyV1ActorDetailTableID(tableName string, usedTableIDs, reservedCustomTableIDs map[string]struct{}) string {
	tableID := topologyID("actor_"+tableName, "actor_detail")
	switch tableID {
	case "actor_labels", "actor_metadata", "actor_port_links":
		return snmpTopologyV1UniqueActorDetailTableID(topologyID("actor_custom_"+tableName, "actor_detail"), usedTableIDs)
	default:
		if _, reserved := reservedCustomTableIDs[tableID]; reserved {
			return snmpTopologyV1UniqueActorDetailTableID(topologyID("actor_detail_"+tableName, "actor_detail"), usedTableIDs)
		}
		return snmpTopologyV1UniqueActorDetailTableID(tableID, usedTableIDs)
	}
}

func snmpTopologyV1UniqueActorDetailTableID(tableID string, usedTableIDs map[string]struct{}) string {
	if _, ok := usedTableIDs[tableID]; !ok {
		return tableID
	}
	for suffix := 2; ; suffix++ {
		candidate := fmt.Sprintf("%s_%d", tableID, suffix)
		if _, ok := usedTableIDs[candidate]; !ok {
			return candidate
		}
	}
}

func buildSNMPTopologyV1ActorMetadataTable(actors []topologyActor) topologyv1.Table {
	actorRefs := make([]any, 0, len(actors))
	attributes := make([]any, 0, len(actors))
	labels := make([]any, 0, len(actors))
	for i, actor := range actors {
		if len(actor.Attributes) == 0 && len(actor.Labels) == 0 {
			continue
		}
		actorRefs = append(actorRefs, i)
		attributes = append(attributes, nullableJSON(actor.Attributes))
		labels = append(labels, nullableJSON(actor.Labels))
	}
	if len(actorRefs) == 0 {
		return topologyv1.EmptyTable()
	}
	return topologyv1.MustTable(len(actorRefs),
		[]topologyv1.Column{
			topologyv1.NewColumn("actor", "actor_ref", topologyv1.WithRole("reference")),
			topologyv1.NewColumn("attributes", "json", topologyv1.WithNullable()),
			topologyv1.NewColumn("labels", "json", topologyv1.WithNullable()),
		},
		[]topologyv1.ColumnEncoding{
			topologyv1.Values(actorRefs...),
			topologyv1.Values(attributes...),
			topologyv1.Values(labels...),
		},
	)
}

func buildSNMPTopologyV1ActorLabelsTable(
	actors []topologyActor,
	stringsDict *topologyv1.StringDictionary,
) topologyv1.Table {
	type labelRow struct {
		actor      int
		key        string
		value      string
		source     string
		kind       string
		valueIndex any
	}

	rows := make([]labelRow, 0, len(actors)*8)
	add := func(actor int, key, value, source, kind string, valueIndex any) {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			return
		}
		rows = append(rows, labelRow{
			actor:      actor,
			key:        key,
			value:      value,
			source:     source,
			kind:       kind,
			valueIndex: valueIndex,
		})
	}
	addSlice := func(actor int, key string, values []string, source, kind string) {
		index := 0
		for _, value := range values {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			add(actor, key, value, source, kind, index)
			index++
		}
	}

	scalarAttributeKeys := []string{
		"vendor", "vendor_derived", "model", "sys_descr", "sys_location", "sys_contact",
		"management_ip", "display_name", "display_source", "chart_id_prefix", "chart_context_prefix",
		"netdata_host_id", "ports_total", "ports_up", "ports_down", "vlan_count", "fdb_total_macs",
		"lldp_neighbor_count", "cdp_neighbor_count", "endpoints_total", "if_admin_status_counts",
		"if_oper_status_counts", "if_link_mode_counts", "if_topology_role_counts",
	}
	arrayAttributeKeys := []string{
		"protocols", "protocols_collected", "learned_sources", "capabilities",
		"capabilities_supported", "capabilities_enabled", "if_names", "if_indexes",
	}

	for actorIndex, actor := range actors {
		add(actorIndex, "actor_type", snmpTopologyV1ActorType(actor.ActorType), snmpTopologyV1ProducerSource, "identity", nil)
		add(actorIndex, "layer", snmpTopologyV1ActorLayer(actor), snmpTopologyV1ProducerSource, "identity", nil)
		add(actorIndex, "source", firstNonEmptyString(actor.Source, snmpTopologyV1ProducerSource), snmpTopologyV1ProducerSource, "identity", nil)
		add(actorIndex, "display_name", snmpTopologyV1DisplayName(actor), snmpTopologyV1ProducerSource, "attribute", nil)
		add(actorIndex, "sys_name", actor.Match.SysName, snmpTopologyV1ProducerSource, "match", nil)
		add(actorIndex, "sys_object_id", actor.Match.SysObjectID, snmpTopologyV1ProducerSource, "match", nil)
		addSlice(actorIndex, "chassis_id", actor.Match.ChassisIDs, snmpTopologyV1ProducerSource, "match")
		addSlice(actorIndex, "mac_address", actor.Match.MacAddresses, snmpTopologyV1ProducerSource, "match")
		addSlice(actorIndex, "ip_address", actor.Match.IPAddresses, snmpTopologyV1ProducerSource, "match")
		addSlice(actorIndex, "hostname", actor.Match.Hostnames, snmpTopologyV1ProducerSource, "match")
		addSlice(actorIndex, "dns_name", actor.Match.DNSNames, snmpTopologyV1ProducerSource, "match")

		for key, value := range actor.Labels {
			add(actorIndex, key, value, "producer_label", "label", nil)
		}
		for _, key := range scalarAttributeKeys {
			if value := topologyV1ScalarLabelValue(actor.Attributes[key]); value != "" {
				add(actorIndex, key, value, snmpTopologyV1ProducerSource, "attribute", nil)
			}
		}
		for _, key := range arrayAttributeKeys {
			addSlice(actorIndex, key, anyStringSlice(actor.Attributes[key]), snmpTopologyV1ProducerSource, "attribute")
		}
	}

	actorRefs := make([]any, len(rows))
	keys := make([]any, len(rows))
	values := make([]any, len(rows))
	sources := make([]any, len(rows))
	kinds := make([]any, len(rows))
	valueIndexes := make([]any, len(rows))
	for i, row := range rows {
		actorRefs[i] = row.actor
		keys[i] = stringsDict.Ref(row.key)
		values[i] = stringsDict.Ref(row.value)
		sources[i] = nullableStringRef(stringsDict, row.source)
		kinds[i] = nullableStringRef(stringsDict, row.kind)
		valueIndexes[i] = row.valueIndex
	}

	return topologyv1.MustTable(len(rows), snmpTopologyV1ActorLabelsTableType().Columns, []topologyv1.ColumnEncoding{
		topologyv1.Values(actorRefs...),
		topologyv1.Values(keys...),
		topologyv1.Values(values...),
		topologyv1.Values(sources...),
		topologyv1.Values(kinds...),
		topologyv1.Values(valueIndexes...),
	})
}

type topologyV1DynamicRow struct {
	actorRef int
	values   map[string]any
}

type snmpTopologyV1PortNeighborKey struct {
	actorRef int
	ifIndex  uint64
	portName string
}

type snmpTopologyV1PortNeighborSummary struct {
	remoteActor    any
	remotePortName string
	ambiguous      bool
}

func snmpTopologyV1PortNeighborKeyFor(actorRef int, ifIndex any, portName string) snmpTopologyV1PortNeighborKey {
	if index, ok := uintValue(ifIndex); ok && index > 0 {
		return snmpTopologyV1PortNeighborKey{actorRef: actorRef, ifIndex: index}
	}
	portName = strings.ToLower(strings.TrimSpace(portName))
	if portName == "" {
		return snmpTopologyV1PortNeighborKey{actorRef: -1}
	}
	return snmpTopologyV1PortNeighborKey{actorRef: actorRef, portName: portName}
}

func buildSNMPTopologyV1PortNeighborSummaries(
	links []topologyLink,
	actorIndex map[string]int,
) map[snmpTopologyV1PortNeighborKey]snmpTopologyV1PortNeighborSummary {
	summaries := make(map[snmpTopologyV1PortNeighborKey]snmpTopologyV1PortNeighborSummary)
	appendSide := func(actorID, remoteActorID string, endpoint, remoteEndpoint topologyLinkEndpoint) {
		actorRef, ok := actorIndex[strings.TrimSpace(actorID)]
		if !ok {
			return
		}
		remoteActorRef, ok := actorIndex[strings.TrimSpace(remoteActorID)]
		if !ok {
			return
		}
		key := snmpTopologyV1PortNeighborKeyFor(actorRef, endpoint.Attributes["if_index"], topologyV1EndpointPortName(endpoint))
		if key.actorRef < 0 {
			return
		}
		if existing, exists := summaries[key]; exists {
			if existing.remoteActor != remoteActorRef || strings.TrimSpace(existing.remotePortName) != strings.TrimSpace(topologyV1EndpointPortName(remoteEndpoint)) {
				existing.ambiguous = true
				summaries[key] = existing
			}
			return
		}
		summaries[key] = snmpTopologyV1PortNeighborSummary{
			remoteActor:    remoteActorRef,
			remotePortName: topologyV1EndpointPortName(remoteEndpoint),
		}
	}

	for _, link := range links {
		appendSide(link.SrcActorID, link.DstActorID, link.Src, link.Dst)
		appendSide(link.DstActorID, link.SrcActorID, link.Dst, link.Src)
	}
	return summaries
}

func snmpTopologyV1PortNeighborSummaryFor(
	row topologyV1DynamicRow,
	portName string,
	summaries map[snmpTopologyV1PortNeighborKey]snmpTopologyV1PortNeighborSummary,
) (snmpTopologyV1PortNeighborSummary, bool) {
	candidates := []string{
		portName,
		topologyV1ScalarLabelValue(row.values["if_name"]),
		topologyV1ScalarLabelValue(row.values["port_name"]),
		topologyV1ScalarLabelValue(row.values["port_id"]),
	}
	seen := make(map[snmpTopologyV1PortNeighborKey]struct{}, len(candidates)+1)
	keys := []snmpTopologyV1PortNeighborKey{
		snmpTopologyV1PortNeighborKeyFor(row.actorRef, row.values["if_index"], ""),
	}
	for _, candidate := range candidates {
		keys = append(keys, snmpTopologyV1PortNeighborKeyFor(row.actorRef, nil, candidate))
	}
	for _, key := range keys {
		if key.actorRef < 0 {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if summary, ok := summaries[key]; ok {
			if summary.ambiguous {
				return snmpTopologyV1PortNeighborSummary{}, false
			}
			return summary, true
		}
	}
	return snmpTopologyV1PortNeighborSummary{}, false
}

func collectSNMPTopologyV1ActorTableRows(actors []topologyActor) map[string][]topologyV1DynamicRow {
	tables := make(map[string][]topologyV1DynamicRow)
	for actorIndex, actor := range actors {
		for tableName, rows := range actor.Tables {
			tableName = strings.TrimSpace(tableName)
			if tableName == "" || len(rows) == 0 {
				continue
			}
			for _, row := range rows {
				if len(row) == 0 {
					continue
				}
				tables[tableName] = append(tables[tableName], topologyV1DynamicRow{
					actorRef: actorIndex,
					values:   row,
				})
			}
		}
	}
	return tables
}

func buildSNMPTopologyV1ActorPortsTable(
	rows []topologyV1DynamicRow,
	stringsDict *topologyv1.StringDictionary,
	portNeighborSummaries map[snmpTopologyV1PortNeighborKey]snmpTopologyV1PortNeighborSummary,
) topologyv1.Table {
	actorRefs := make([]any, len(rows))
	portNumbers := make([]any, len(rows))
	ifIndexes := make([]any, len(rows))
	portIDs := make([]any, len(rows))
	names := make([]any, len(rows))
	ifNames := make([]any, len(rows))
	ifDescrs := make([]any, len(rows))
	ifAliases := make([]any, len(rows))
	macs := make([]any, len(rows))
	speeds := make([]any, len(rows))
	topologyRoles := make([]any, len(rows))
	operStatuses := make([]any, len(rows))
	adminStatuses := make([]any, len(rows))
	portTypes := make([]any, len(rows))
	linkModes := make([]any, len(rows))
	stpStates := make([]any, len(rows))
	vlanIDs := make([]any, len(rows))
	fdbMACCounts := make([]any, len(rows))
	linkCounts := make([]any, len(rows))
	neighborCounts := make([]any, len(rows))
	neighborActors := make([]any, len(rows))
	neighborPortNames := make([]any, len(rows))
	neighbors := make([]any, len(rows))
	vlans := make([]any, len(rows))
	extra := make([]any, len(rows))

	for i, row := range rows {
		actorRefs[i] = row.actorRef
		portNumbers[i] = nullableSNMPTopologyV1PortNumber(row.values)
		ifIndexes[i] = nullableUintValue(row.values["if_index"])
		portIDs[i] = nullableStringRef(stringsDict, topologyV1ScalarLabelValue(row.values["port_id"]))
		portName := firstNonEmptyString(
			topologyV1ScalarLabelValue(row.values["name"]),
			topologyV1ScalarLabelValue(row.values["if_name"]),
			topologyV1ScalarLabelValue(row.values["port_name"]),
			topologyV1ScalarLabelValue(row.values["port_id"]),
		)
		names[i] = nullableStringRef(stringsDict, portName)
		ifNames[i] = nullableStringRef(stringsDict, topologyV1ScalarLabelValue(row.values["if_name"]))
		ifDescrs[i] = nullableStringRef(stringsDict, topologyV1ScalarLabelValue(row.values["if_descr"]))
		ifAliases[i] = nullableStringRef(stringsDict, topologyV1ScalarLabelValue(row.values["if_alias"]))
		macs[i] = nullableStringRef(stringsDict, topologyV1ScalarLabelValue(row.values["mac"]))
		speeds[i] = nullableUintValue(row.values["speed"])
		topologyRoles[i] = nullableStringRef(stringsDict, topologyV1ScalarLabelValue(row.values["topology_role"]))
		operStatuses[i] = nullableStringRef(stringsDict, firstNonEmptyString(
			topologyV1ScalarLabelValue(row.values["oper_status"]),
			topologyV1ScalarLabelValue(row.values["if_oper_status"]),
		))
		adminStatuses[i] = nullableStringRef(stringsDict, firstNonEmptyString(
			topologyV1ScalarLabelValue(row.values["admin_status"]),
			topologyV1ScalarLabelValue(row.values["if_admin_status"]),
		))
		portTypes[i] = nullableStringRef(stringsDict, firstNonEmptyString(
			topologyV1ScalarLabelValue(row.values["port_type"]),
			topologyV1ScalarLabelValue(row.values["if_type"]),
		))
		linkModes[i] = nullableStringRef(stringsDict, topologyV1ScalarLabelValue(row.values["link_mode"]))
		stpStates[i] = nullableStringRef(stringsDict, topologyV1ScalarLabelValue(row.values["stp_state"]))
		vlanIDs[i] = stringArrayCell(anyStringSlice(row.values["vlan_ids"]))
		if isEmptyArrayCell(vlanIDs[i]) {
			vlanIDs[i] = nil
		}
		fdbMACCounts[i] = nullableUintValue(row.values["fdb_mac_count"])
		linkCounts[i] = nullableUintValue(row.values["link_count"])
		neighborCounts[i] = nullableUintValue(row.values["neighbor_count"])
		if neighborCounts[i] == nil {
			if values, ok := anyMapSlice(row.values["neighbors"]); ok {
				neighborCounts[i] = uint64(len(values))
			}
		}
		if summary, ok := snmpTopologyV1PortNeighborSummaryFor(row, portName, portNeighborSummaries); ok {
			neighborActors[i] = summary.remoteActor
			neighborPortNames[i] = nullableStringRef(stringsDict, summary.remotePortName)
		}
		neighbors[i] = nullableJSON(row.values["neighbors"])
		vlans[i] = nullableJSON(row.values["vlans"])
		extra[i] = nullableJSON(snmpTopologyV1ExtraPortValues(row.values))
	}

	return topologyv1.MustTable(len(rows), snmpTopologyV1ActorPortsColumns(), []topologyv1.ColumnEncoding{
		topologyv1.Values(actorRefs...),
		topologyv1.Values(portNumbers...),
		topologyv1.Values(ifIndexes...),
		topologyv1.Values(portIDs...),
		topologyv1.Values(names...),
		topologyv1.Values(ifNames...),
		topologyv1.Values(ifDescrs...),
		topologyv1.Values(ifAliases...),
		topologyv1.Values(macs...),
		topologyv1.Values(speeds...),
		topologyv1.Values(topologyRoles...),
		topologyv1.Values(operStatuses...),
		topologyv1.Values(adminStatuses...),
		topologyv1.Values(portTypes...),
		topologyv1.Values(linkModes...),
		topologyv1.Values(stpStates...),
		topologyv1.Values(vlanIDs...),
		topologyv1.Values(fdbMACCounts...),
		topologyv1.Values(linkCounts...),
		topologyv1.Values(neighborCounts...),
		topologyv1.Values(neighborActors...),
		topologyv1.Values(neighborPortNames...),
		topologyv1.Values(neighbors...),
		topologyv1.Values(vlans...),
		topologyv1.Values(extra...),
	})
}

func buildSNMPTopologyV1ActorPortLinksTable(
	links []topologyLink,
	actorIndex map[string]int,
	stringsDict *topologyv1.StringDictionary,
) (topologyv1.Table, error) {
	rows := &snmpTopologyV1ActorPortLinkRows{}
	appendSide := func(linkIndex int, link topologyLink, actorID, remoteActorID string, endpoint, remoteEndpoint topologyLinkEndpoint) error {
		actorRef, ok := actorIndex[strings.TrimSpace(actorID)]
		if !ok {
			return fmt.Errorf("link %d references unknown actor %q", linkIndex, actorID)
		}
		remoteActorRef, ok := actorIndex[strings.TrimSpace(remoteActorID)]
		if !ok {
			return fmt.Errorf("link %d references unknown remote actor %q", linkIndex, remoteActorID)
		}
		protocol := firstNonEmptyString(link.Protocol, link.LinkType, "l2")
		linkType := snmpTopologyV1LinkType(link)

		rows.actors = append(rows.actors, actorRef)
		rows.links = append(rows.links, linkIndex)
		rows.remoteActors = append(rows.remoteActors, remoteActorRef)
		rows.ifIndexes = append(rows.ifIndexes, nullableUintValue(endpoint.Attributes["if_index"]))
		rows.portIDs = append(rows.portIDs, nullableStringRef(stringsDict, topologyV1EndpointString(endpoint, "port_id")))
		rows.portNames = append(rows.portNames, nullableStringRef(stringsDict, topologyV1EndpointPortName(endpoint)))
		rows.remoteIfIndexes = append(rows.remoteIfIndexes, nullableUintValue(remoteEndpoint.Attributes["if_index"]))
		rows.remotePortIDs = append(rows.remotePortIDs, nullableStringRef(stringsDict, topologyV1EndpointString(remoteEndpoint, "port_id")))
		rows.remotePortNames = append(rows.remotePortNames, nullableStringRef(stringsDict, topologyV1EndpointPortName(remoteEndpoint)))
		rows.types = append(rows.types, stringsDict.Ref(linkType))
		rows.protocols = append(rows.protocols, stringsDict.Ref(protocol))
		rows.states = append(rows.states, nullableStringRef(stringsDict, link.State))
		rows.evidenceCounts = append(rows.evidenceCounts, uint64(1))
		rows.confidences = append(rows.confidences, nullableStringRef(stringsDict, topologyMetricValueString(link.Metrics, "confidence")))
		rows.inferences = append(rows.inferences, nullableStringRef(stringsDict, topologyMetricValueString(link.Metrics, "inference")))
		rows.attachmentModes = append(rows.attachmentModes, nullableStringRef(stringsDict, topologyMetricValueString(link.Metrics, "attachment_mode")))
		rows.discoveredAt = append(rows.discoveredAt, nullableTime(link.DiscoveredAt))
		rows.lastSeen = append(rows.lastSeen, nullableTime(link.LastSeen))
		return nil
	}

	for i, link := range links {
		if err := appendSide(i, link, link.SrcActorID, link.DstActorID, link.Src, link.Dst); err != nil {
			return topologyv1.Table{}, err
		}
		if err := appendSide(i, link, link.DstActorID, link.SrcActorID, link.Dst, link.Src); err != nil {
			return topologyv1.Table{}, err
		}
	}

	return rows.table(), nil
}

type snmpTopologyV1ActorPortLinkRows struct {
	actors          []any
	links           []any
	remoteActors    []any
	ifIndexes       []any
	portIDs         []any
	portNames       []any
	remoteIfIndexes []any
	remotePortIDs   []any
	remotePortNames []any
	types           []any
	protocols       []any
	states          []any
	evidenceCounts  []any
	confidences     []any
	inferences      []any
	attachmentModes []any
	discoveredAt    []any
	lastSeen        []any
}

func (rows *snmpTopologyV1ActorPortLinkRows) table() topologyv1.Table {
	return topologyv1.MustTable(len(rows.actors),
		snmpTopologyV1ActorPortLinksColumns(),
		[]topologyv1.ColumnEncoding{
			topologyv1.Values(rows.actors...),
			topologyv1.Values(rows.links...),
			topologyv1.Values(rows.remoteActors...),
			topologyv1.Values(rows.ifIndexes...),
			topologyv1.Values(rows.portIDs...),
			topologyv1.Values(rows.portNames...),
			topologyv1.Values(rows.remoteIfIndexes...),
			topologyv1.Values(rows.remotePortIDs...),
			topologyv1.Values(rows.remotePortNames...),
			topologyv1.Values(rows.types...),
			topologyv1.Values(rows.protocols...),
			topologyv1.Values(rows.states...),
			topologyv1.Values(rows.evidenceCounts...),
			topologyv1.Values(rows.confidences...),
			topologyv1.Values(rows.inferences...),
			topologyv1.Values(rows.attachmentModes...),
			topologyv1.Values(rows.discoveredAt...),
			topologyv1.Values(rows.lastSeen...),
		},
	)
}

var snmpTopologyV1ActorPortCanonicalKeys = map[string]struct{}{
	"admin_status":       {},
	"fdb_mac_count":      {},
	"if_admin_status":    {},
	"if_alias":           {},
	"if_descr":           {},
	"if_index":           {},
	"if_name":            {},
	"if_oper_status":     {},
	"if_type":            {},
	"link_count":         {},
	"link_mode":          {},
	"mac":                {},
	"name":               {},
	"neighbor_actor":     {},
	"neighbor_count":     {},
	"neighbor_port_name": {},
	"neighbors":          {},
	"oper_status":        {},
	"port_id":            {},
	"port_name":          {},
	"port_number":        {},
	"port_type":          {},
	"speed":              {},
	"stp_state":          {},
	"topology_role":      {},
	"vlan_ids":           {},
	"vlans":              {},
}

func snmpTopologyV1ExtraPortValues(values map[string]any) map[string]any {
	extra := make(map[string]any)
	for key, value := range values {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, ok := snmpTopologyV1ActorPortCanonicalKeys[key]; ok {
			continue
		}
		extra[key] = value
	}
	if len(extra) == 0 {
		return nil
	}
	return extra
}

func buildSNMPTopologyV1DynamicTable(rows []topologyV1DynamicRow, stringsDict *topologyv1.StringDictionary) (topologyv1.Table, error) {
	keysSet := make(map[string]struct{})
	for _, row := range rows {
		for key := range row.values {
			key = strings.TrimSpace(key)
			if key != "" {
				keysSet[key] = struct{}{}
			}
		}
	}
	keys := sortedMapKeys(keysSet)

	columns := make([]topologyv1.Column, 0, len(keys)+1)
	values := make([]topologyv1.ColumnEncoding, 0, len(keys)+1)
	actorRefs := make([]any, len(rows))
	for i, row := range rows {
		actorRefs[i] = row.actorRef
	}
	columns = append(columns, topologyv1.NewColumn("actor", "actor_ref", topologyv1.WithRole("reference")))
	values = append(values, topologyv1.Values(actorRefs...))

	for _, key := range keys {
		columnValues := make([]any, len(rows))
		for i, row := range rows {
			if value, ok := row.values[key]; ok {
				columnValues[i] = value
			}
		}
		columnType := inferTopologyV1ColumnType(columnValues)
		columnID := topologyID(key, "field")
		column := topologyv1.NewColumn(columnID, columnType, topologyv1.WithNullable())
		if columnType == "string_ref" {
			column = topologyv1.NewColumn(columnID, columnType, topologyv1.WithNullable(), topologyv1.WithDictionary("strings"))
			for i, value := range columnValues {
				if value == nil {
					continue
				}
				columnValues[i] = stringsDict.Ref(fmt.Sprint(value))
			}
		}
		columns = append(columns, column)
		values = append(values, topologyv1.Values(columnValues...))
	}

	return topologyv1.NewTable(len(rows), columns, values)
}

func inferTopologyV1ColumnType(values []any) string {
	typ := ""
	for _, value := range values {
		if value == nil {
			continue
		}
		valueType := topologyV1ValueType(value)
		if typ == "" {
			typ = valueType
			continue
		}
		if typ != valueType {
			return "json"
		}
	}
	if typ == "" {
		return "json"
	}
	return typ
}

func topologyV1ValueType(value any) string {
	switch typed := value.(type) {
	case bool:
		return "bool"
	case int, int8, int16, int32, int64:
		return "int"
	case uint, uint8, uint16, uint32, uint64:
		return "uint"
	case float32:
		if math.Trunc(float64(typed)) == float64(typed) {
			return "int"
		}
		return "float"
	case float64:
		if math.Trunc(typed) == typed {
			return "int"
		}
		return "float"
	case string:
		return "string_ref"
	case []string, []int, []int64, []uint, []uint64, []float64, []bool:
		return "array"
	case []any:
		if scalarArray(typed) {
			return "array"
		}
		return "json"
	default:
		return "json"
	}
}

func scalarArray(values []any) bool {
	for _, value := range values {
		switch value.(type) {
		case nil, bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, string:
		default:
			return false
		}
	}
	return true
}

func snmpTopologyV1ActorType(actorType string) string {
	normalized := strings.ToLower(strings.TrimSpace(actorType))
	if topologyengine.IsDeviceActorType(normalized) {
		return normalized
	}
	switch normalized {
	case snmpTopologyV1ActorDevice:
		return snmpTopologyV1ActorDevice
	case snmpTopologyV1ActorEndpoint:
		return snmpTopologyV1ActorEndpoint
	case snmpTopologyV1ActorSegment:
		return snmpTopologyV1ActorSegment
	default:
		return "custom"
	}
}

func snmpTopologyV1DisplayName(actor topologyActor) string {
	return firstNonEmptyString(
		anyStringValue(actor.Attributes["display_name"]),
		anyStringValue(actor.Attributes["name"]),
		actor.Labels["display_name"],
		actor.Labels["name"],
		actor.Match.SysName,
		firstString(actor.Match.Hostnames),
		firstString(actor.Match.DNSNames),
	)
}

func snmpTopologyV1ActorLayer(actor topologyActor) string {
	switch snmpTopologyV1ActorType(actor.ActorType) {
	case snmpTopologyV1ActorEndpoint, snmpTopologyV1ActorSegment:
		return "network"
	default:
		if topologyengine.IsDeviceActorType(actor.ActorType) {
			return "network"
		}
		return "custom"
	}
}

func anyStringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		return ""
	}
}

func topologyV1ScalarLabelValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case bool:
		if typed {
			return "true"
		}
		return "false"
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return strings.TrimSpace(fmt.Sprint(typed))
	default:
		return ""
	}
}

func nullableUintValue(value any) any {
	out, ok := uintValue(value)
	if !ok {
		return nil
	}
	return out
}

func nullableSNMPTopologyV1PortNumber(values map[string]any) any {
	if value, ok := uintValue(values["port_number"]); ok {
		return value
	}
	portID := strings.TrimSpace(topologyV1ScalarLabelValue(values["port_id"]))
	if portID == "" {
		return nil
	}
	value, err := strconv.ParseUint(portID, 10, 64)
	if err != nil {
		return nil
	}
	return value
}

func uintValue(value any) (uint64, bool) {
	switch typed := value.(type) {
	case int:
		if typed >= 0 {
			return uint64(typed), true
		}
	case int8:
		if typed >= 0 {
			return uint64(typed), true
		}
	case int16:
		if typed >= 0 {
			return uint64(typed), true
		}
	case int32:
		if typed >= 0 {
			return uint64(typed), true
		}
	case int64:
		if typed >= 0 {
			return uint64(typed), true
		}
	case uint:
		return uint64(typed), true
	case uint8:
		return uint64(typed), true
	case uint16:
		return uint64(typed), true
	case uint32:
		return uint64(typed), true
	case uint64:
		return typed, true
	case float32:
		if typed >= 0 && math.Trunc(float64(typed)) == float64(typed) {
			return uint64(typed), true
		}
	case float64:
		if typed >= 0 && math.Trunc(typed) == typed {
			return uint64(typed), true
		}
	}
	return 0, false
}

func topologyV1EndpointString(endpoint topologyLinkEndpoint, key string) string {
	return firstNonEmptyString(
		anyStringValue(endpoint.Attributes[key]),
		topologyV1MatchString(endpoint.Match, key),
	)
}

func topologyV1EndpointPortName(endpoint topologyLinkEndpoint) string {
	return firstNonEmptyString(
		topologyV1EndpointString(endpoint, "port_name"),
		topologyV1EndpointString(endpoint, "if_name"),
		topologyV1EndpointString(endpoint, "if_descr"),
		topologyV1EndpointString(endpoint, "port_id"),
	)
}

func topologyV1MatchString(match topologyMatch, key string) string {
	switch key {
	case "sys_name":
		return match.SysName
	case "sys_object_id":
		return match.SysObjectID
	default:
		return ""
	}
}

func firstString(values []string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func nullableTime(value *time.Time) any {
	if value == nil || value.IsZero() {
		return nil
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func nullableStringRef(dict *topologyv1.StringDictionary, value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return dict.Ref(value)
}

func nullableJSON(value any) any {
	switch typed := value.(type) {
	case nil:
		return nil
	case map[string]any:
		if len(typed) == 0 {
			return nil
		}
	case map[string]string:
		if len(typed) == 0 {
			return nil
		}
	case []any:
		if len(typed) == 0 {
			return nil
		}
	case []map[string]any:
		if len(typed) == 0 {
			return nil
		}
	}
	return value
}

func stringArrayCell(values []string) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func isEmptyArrayCell(value any) bool {
	values, ok := value.([]any)
	return ok && len(values) == 0
}

func anyStringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if s := strings.TrimSpace(fmt.Sprint(item)); s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		rv := reflect.ValueOf(value)
		if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
			return nil
		}
		out := make([]string, 0, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			if s := topologyV1ScalarLabelValue(rv.Index(i).Interface()); s != "" {
				out = append(out, s)
			}
		}
		return out
	}
}

func anyMapSlice(value any) ([]map[string]any, bool) {
	switch typed := value.(type) {
	case []map[string]any:
		return typed, true
	case []any:
		out := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			row, ok := item.(map[string]any)
			if !ok {
				return nil, false
			}
			out = append(out, row)
		}
		return out, true
	default:
		return nil, false
	}
}

func sortedMapKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func topologyID(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		value = fallback
	}
	value = topologyV1IDInvalidChars.ReplaceAllString(value, "_")
	value = strings.Trim(value, "_.:-")
	if value == "" {
		value = fallback
	}
	first := value[0]
	if (first >= 'A' && first <= 'Z') || (first >= 'a' && first <= 'z') {
		return value
	}
	return "x_" + value
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func cloneAnyMapForTopologyV1(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
