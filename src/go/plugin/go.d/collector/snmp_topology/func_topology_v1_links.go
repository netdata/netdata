// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"fmt"
	topologyv1 "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
	"strings"
)

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
		subnet := topologyEvidenceSubnet(link)
		if linkType == snmpTopologyV1LinkL3Subnet && subnet == "" {
			return topologyv1.Table{}, nil, fmt.Errorf("l3_subnet link %d is missing subnet", i)
		}

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
		evidenceRows.srcIfIndexes = append(evidenceRows.srcIfIndexes, nullableEndpointIfIndex(link.Src))
		evidenceRows.dstIfIndexes = append(evidenceRows.dstIfIndexes, nullableEndpointIfIndex(link.Dst))
		evidenceRows.srcPortIDs = append(evidenceRows.srcPortIDs, nullableStringRef(stringsDict, topologyV1EndpointString(link.Src, "port_id")))
		evidenceRows.dstPortIDs = append(evidenceRows.dstPortIDs, nullableStringRef(stringsDict, topologyV1EndpointString(link.Dst, "port_id")))
		evidenceRows.srcManagementIPs = append(evidenceRows.srcManagementIPs, nullableStringRef(stringsDict, topologyV1EndpointString(link.Src, "management_ip")))
		evidenceRows.dstManagementIPs = append(evidenceRows.dstManagementIPs, nullableStringRef(stringsDict, topologyV1EndpointString(link.Dst, "management_ip")))
		evidenceRows.confidences = append(evidenceRows.confidences, nullableStringRef(stringsDict, topologyLinkConfidenceValue(link)))
		evidenceRows.inferences = append(evidenceRows.inferences, nullableStringRef(stringsDict, topologyLinkInferenceValue(link)))
		evidenceRows.attachmentModes = append(evidenceRows.attachmentModes, nullableStringRef(stringsDict, topologyLinkAttachmentModeValue(link)))
		if linkType == snmpTopologyV1LinkL3Subnet || linkType == snmpTopologyV1LinkOSPF {
			if linkType == snmpTopologyV1LinkOSPF {
				evidenceRows.srcRouterIDs = append(evidenceRows.srcRouterIDs, nullableStringRef(stringsDict, topologyOSPFLocalRouterID(link)))
				evidenceRows.dstRouterIDs = append(evidenceRows.dstRouterIDs, nullableStringRef(stringsDict, topologyOSPFNeighborRouterID(link)))
			}
			evidenceRows.srcIPs = append(evidenceRows.srcIPs, nullableStringRef(stringsDict, topologyEvidenceSrcIP(link)))
			evidenceRows.dstIPs = append(evidenceRows.dstIPs, nullableStringRef(stringsDict, topologyEvidenceDstIP(link)))
			evidenceRows.subnets = append(evidenceRows.subnets, nullableStringRef(stringsDict, subnet))
			evidenceRows.networks = append(evidenceRows.networks, nullableStringRef(stringsDict, topologyEvidenceNetwork(link)))
			evidenceRows.netmasks = append(evidenceRows.netmasks, nullableStringRef(stringsDict, topologyEvidenceNetmask(link)))
			evidenceRows.prefixes = append(evidenceRows.prefixes, nullableEvidencePrefix(link))
			evidenceRows.sources = append(evidenceRows.sources, nullableStringRef(stringsDict, topologyEvidenceSource(link)))
		}
		if linkType == snmpTopologyV1LinkBGP {
			evidenceRows.routingInstances = append(evidenceRows.routingInstances, nullableStringRef(stringsDict, topologyBGPLinkRoutingInstance(link)))
			evidenceRows.localIdentifiers = append(evidenceRows.localIdentifiers, nullableStringRef(stringsDict, topologyBGPLocalIdentifier(link)))
			evidenceRows.peerIdentifiers = append(evidenceRows.peerIdentifiers, nullableStringRef(stringsDict, topologyBGPPeerIdentifier(link)))
			evidenceRows.localIPs = append(evidenceRows.localIPs, nullableStringRef(stringsDict, topologyBGPLocalIP(link)))
			evidenceRows.neighborIPs = append(evidenceRows.neighborIPs, nullableStringRef(stringsDict, topologyBGPNeighborIP(link)))
			evidenceRows.localASes = append(evidenceRows.localASes, nullableStringRef(stringsDict, topologyBGPLocalAS(link)))
			evidenceRows.remoteASes = append(evidenceRows.remoteASes, nullableStringRef(stringsDict, topologyBGPRemoteAS(link)))
			evidenceRows.sources = append(evidenceRows.sources, nullableStringRef(stringsDict, topologyBGPSource(link)))
		}
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
			Table: rows.table(linkType),
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
	srcPortIDs       []any
	dstPortIDs       []any
	srcManagementIPs []any
	dstManagementIPs []any
	confidences      []any
	inferences       []any
	attachmentModes  []any
	srcRouterIDs     []any
	dstRouterIDs     []any
	srcIPs           []any
	dstIPs           []any
	subnets          []any
	networks         []any
	netmasks         []any
	prefixes         []any
	sources          []any
	routingInstances []any
	localIdentifiers []any
	peerIdentifiers  []any
	localASes        []any
	remoteASes       []any
	localIPs         []any
	neighborIPs      []any
}

func (rows *snmpTopologyV1EvidenceRows) table(linkType string) topologyv1.Table {
	return topologyv1.MustTable(len(rows.linkRefs),
		snmpTopologyV1EvidenceColumnsForType(linkType),
		rows.columnEncodingsForType(linkType),
	)
}

func (rows *snmpTopologyV1EvidenceRows) columnEncodingsForType(linkType string) []topologyv1.ColumnEncoding {
	encodings := []topologyv1.ColumnEncoding{
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
		topologyv1.Values(rows.srcPortIDs...),
		topologyv1.Values(rows.dstPortIDs...),
		topologyv1.Values(rows.srcManagementIPs...),
		topologyv1.Values(rows.dstManagementIPs...),
		topologyv1.Values(rows.confidences...),
		topologyv1.Values(rows.inferences...),
		topologyv1.Values(rows.attachmentModes...),
	}
	if linkType == snmpTopologyV1LinkL3Subnet || linkType == snmpTopologyV1LinkOSPF {
		if linkType == snmpTopologyV1LinkOSPF {
			encodings = append(encodings,
				topologyv1.Values(rows.srcRouterIDs...),
				topologyv1.Values(rows.dstRouterIDs...),
			)
		}
		encodings = append(encodings,
			topologyv1.Values(rows.srcIPs...),
			topologyv1.Values(rows.dstIPs...),
			topologyv1.Values(rows.subnets...),
			topologyv1.Values(rows.networks...),
			topologyv1.Values(rows.netmasks...),
			topologyv1.Values(rows.prefixes...),
			topologyv1.Values(rows.sources...),
		)
	}
	if linkType == snmpTopologyV1LinkBGP {
		encodings = append(encodings,
			topologyv1.Values(rows.routingInstances...),
			topologyv1.Values(rows.localIdentifiers...),
			topologyv1.Values(rows.peerIdentifiers...),
			topologyv1.Values(rows.localIPs...),
			topologyv1.Values(rows.neighborIPs...),
			topologyv1.Values(rows.localASes...),
			topologyv1.Values(rows.remoteASes...),
			topologyv1.Values(rows.sources...),
		)
	}
	return encodings
}

func snmpTopologyV1EvidenceColumnsForType(linkType string) []topologyv1.Column {
	columns := snmpTopologyV1EvidenceColumns()
	if linkType == snmpTopologyV1LinkOSPF {
		extras := []topologyv1.Column{
			topologyv1.NewColumn("src_router_id", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable()),
			topologyv1.NewColumn("dst_router_id", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable()),
			topologyv1.NewColumn("src_ip", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable()),
			topologyv1.NewColumn("dst_ip", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable()),
			topologyv1.NewColumn("subnet", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable()),
			topologyv1.NewColumn("network", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable()),
			topologyv1.NewColumn("netmask", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable()),
			topologyv1.NewColumn("prefix", "uint", topologyv1.WithNullable()),
			topologyv1.NewColumn("source", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable()),
		}
		return snmpTopologyV1EvidenceColumnsWithExtras(columns, extras)
	}
	if linkType == snmpTopologyV1LinkBGP {
		extras := []topologyv1.Column{
			topologyv1.NewColumn("routing_instance", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable()),
			topologyv1.NewColumn("local_identifier", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable()),
			topologyv1.NewColumn("peer_identifier", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable()),
			topologyv1.NewColumn("local_ip", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable()),
			topologyv1.NewColumn("neighbor_ip", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable()),
			topologyv1.NewColumn("local_as", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable()),
			topologyv1.NewColumn("remote_as", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable()),
			topologyv1.NewColumn("source", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable()),
		}
		return snmpTopologyV1EvidenceColumnsWithExtras(columns, extras)
	}
	if linkType != snmpTopologyV1LinkL3Subnet {
		return columns
	}
	extras := []topologyv1.Column{
		topologyv1.NewColumn("src_ip", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable()),
		topologyv1.NewColumn("dst_ip", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable()),
		topologyv1.NewColumn("subnet", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithRole("group_key")),
		topologyv1.NewColumn("network", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable()),
		topologyv1.NewColumn("netmask", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable()),
		topologyv1.NewColumn("prefix", "uint", topologyv1.WithNullable()),
		topologyv1.NewColumn("source", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable()),
	}
	return snmpTopologyV1EvidenceColumnsWithExtras(columns, extras)
}

func snmpTopologyV1EvidenceColumnsWithExtras(columns, extras []topologyv1.Column) []topologyv1.Column {
	insertAt := len(columns)
	out := make([]topologyv1.Column, 0, len(columns)+len(extras))
	out = append(out, columns[:insertAt]...)
	out = append(out, extras...)
	out = append(out, columns[insertAt:]...)
	return out
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
		topologyv1.NewColumn("src_port_id", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable()),
		topologyv1.NewColumn("dst_port_id", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable()),
		topologyv1.NewColumn("src_management_ip", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable()),
		topologyv1.NewColumn("dst_management_ip", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable()),
		topologyv1.NewColumn("confidence", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable()),
		topologyv1.NewColumn("inference", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable()),
		topologyv1.NewColumn("attachment_mode", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable()),
	}
}

func topologyEvidenceSource(link topologyLink) string {
	if link.Detail.L3Subnet != nil {
		return strings.TrimSpace(link.Detail.L3Subnet.Source)
	}
	if link.Detail.OSPF != nil {
		return strings.TrimSpace(link.Detail.OSPF.Source)
	}
	if link.Detail.BGP != nil {
		return strings.TrimSpace(link.Detail.BGP.Source)
	}
	return ""
}

func topologyEvidenceSrcIP(link topologyLink) string {
	if link.Detail.L3Subnet != nil {
		return strings.TrimSpace(link.Detail.L3Subnet.SrcIP)
	}
	if link.Detail.OSPF != nil {
		return strings.TrimSpace(link.Detail.OSPF.LocalIP)
	}
	return ""
}

func topologyEvidenceDstIP(link topologyLink) string {
	if link.Detail.L3Subnet != nil {
		return strings.TrimSpace(link.Detail.L3Subnet.DstIP)
	}
	if link.Detail.OSPF != nil {
		return strings.TrimSpace(link.Detail.OSPF.NeighborIP)
	}
	return ""
}

func topologyEvidenceSubnet(link topologyLink) string {
	if link.Detail.L3Subnet != nil {
		return strings.TrimSpace(link.Detail.L3Subnet.Subnet)
	}
	if link.Detail.OSPF != nil {
		return strings.TrimSpace(link.Detail.OSPF.Subnet)
	}
	return ""
}

func topologyEvidenceNetwork(link topologyLink) string {
	if link.Detail.L3Subnet != nil {
		return strings.TrimSpace(link.Detail.L3Subnet.Network)
	}
	if link.Detail.OSPF != nil {
		return strings.TrimSpace(link.Detail.OSPF.Network)
	}
	return ""
}

func topologyEvidenceNetmask(link topologyLink) string {
	if link.Detail.L3Subnet != nil {
		return strings.TrimSpace(link.Detail.L3Subnet.Netmask)
	}
	if link.Detail.OSPF != nil {
		return strings.TrimSpace(link.Detail.OSPF.Netmask)
	}
	return ""
}

func nullableEvidencePrefix(link topologyLink) any {
	prefix := 0
	if link.Detail.L3Subnet != nil {
		prefix = link.Detail.L3Subnet.Prefix
	} else if link.Detail.OSPF != nil {
		prefix = link.Detail.OSPF.Prefix
	}
	if prefix <= 0 {
		return nil
	}
	return uint64(prefix)
}

func topologyBGPSource(link topologyLink) string {
	if link.Detail.BGP == nil {
		return ""
	}
	return strings.TrimSpace(link.Detail.BGP.Source)
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
	case snmpTopologyV1LinkL3Subnet:
		return snmpTopologyV1LinkL3Subnet
	case snmpTopologyV1LinkOSPF:
		return snmpTopologyV1LinkOSPF
	case snmpTopologyV1LinkBGP:
		return snmpTopologyV1LinkBGP
	default:
		return snmpTopologyV1LinkObservation
	}
}

func snmpTopologyV1LinkIsLogicalL3(link topologyLink) bool {
	switch snmpTopologyV1LinkType(link) {
	case snmpTopologyV1LinkL3Subnet, snmpTopologyV1LinkOSPF, snmpTopologyV1LinkBGP:
		return true
	default:
		return false
	}
}

func snmpTopologyV1LinkIsProbable(link topologyLink) bool {
	if strings.EqualFold(strings.TrimSpace(link.State), snmpTopologyV1LinkProbable) {
		return true
	}
	if strings.EqualFold(topologyLinkInferenceValue(link), snmpTopologyV1LinkProbable) {
		return true
	}
	return strings.HasPrefix(strings.ToLower(topologyLinkAttachmentModeValue(link)), snmpTopologyV1LinkProbable+"_")
}
