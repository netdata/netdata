// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"fmt"
	"math"
	"regexp"
	"sort"
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
		},
	}
	return types
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

	actorDetails, tableTypes, err := buildSNMPTopologyV1ActorDetails(data.Actors, stringsDict)
	if err != nil {
		return topologyv1.Data{}, err
	}
	if tableTypes == nil {
		tableTypes = make(map[string]topologyv1.TableType)
	}
	if _, ok := tableTypes["actor_ports"]; !ok {
		tableTypes["actor_ports"] = snmpTopologyV1ActorPortsTableType()
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
		Columns: []topologyv1.Column{
			topologyv1.NewColumn("actor", "actor_ref", topologyv1.WithRole("reference")),
			topologyv1.NewColumn("name", "string_ref", topologyv1.WithNullable(), topologyv1.WithDictionary("strings")),
			topologyv1.NewColumn("topology_role", "string_ref", topologyv1.WithNullable(), topologyv1.WithDictionary("strings")),
			topologyv1.NewColumn("oper_status", "string_ref", topologyv1.WithNullable(), topologyv1.WithDictionary("strings")),
			topologyv1.NewColumn("link_mode", "string_ref", topologyv1.WithNullable(), topologyv1.WithDictionary("strings")),
		},
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
	linkRefs     []any
	srcActors    []any
	dstActors    []any
	protocols    []any
	directions   []any
	states       []any
	srcEndpoints []any
	dstEndpoints []any
	metrics      []any
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
) (map[string]topologyv1.DetailTable, map[string]topologyv1.TableType, error) {
	details := make(map[string]topologyv1.DetailTable)
	tableTypes := make(map[string]topologyv1.TableType)

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
		}
	}

	tableRowsByName := collectSNMPTopologyV1ActorTableRows(actors)
	tableNames := sortedMapKeys(tableRowsByName)
	for _, tableName := range tableNames {
		rows := tableRowsByName[tableName]
		if len(rows) == 0 {
			continue
		}
		tableID := topologyID("actor_"+tableName, "actor_detail")
		table, err := buildSNMPTopologyV1DynamicTable(rows, stringsDict)
		if err != nil {
			return nil, nil, fmt.Errorf("build actor detail table %q: %w", tableName, err)
		}
		details[tableID] = topologyv1.DetailTable{
			Type:  tableID,
			Table: table,
		}
		tableTypes[tableID] = topologyv1.TableType{
			Role:        "actor_detail",
			Owner:       "actor",
			Aggregation: "append",
			Columns:     table.Columns,
		}
	}

	if len(details) == 0 {
		return nil, nil, nil
	}
	return details, tableTypes, nil
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

type topologyV1DynamicRow struct {
	actorRef int
	values   map[string]any
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
		return nil
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
