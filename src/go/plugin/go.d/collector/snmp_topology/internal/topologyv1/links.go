// SPDX-License-Identifier: GPL-3.0-or-later

package topologyv1

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"

	topologyapi "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
)

func buildSNMPTopologyV1Links(
	links []topologymodel.Link,
	actorIndex map[string]int,
	stringsDict *topologyapi.StringDictionary,
) (topologyapi.Table, topologyapi.EvidenceMap, error) {
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
			return topologyapi.Table{}, nil, fmt.Errorf("link %d references unknown source actor %q", i, link.SrcActorID)
		}
		dst, ok := actorIndex[strings.TrimSpace(link.DstActorID)]
		if !ok {
			return topologyapi.Table{}, nil, fmt.Errorf("link %d references unknown destination actor %q", i, link.DstActorID)
		}
		protocol := topologyutil.FirstNonEmptyString(link.Protocol, link.LinkType, "l2")
		linkType := snmpTopologyV1LinkType(link)
		srcActors[i] = src
		dstActors[i] = dst
		linkTypes[i] = stringsDict.Ref(linkType)
		protocols[i] = stringsDict.Ref(protocol)
		directions[i] = stringsDict.Ref(topologyutil.FirstNonEmptyString(link.Direction, "observed"))
		states[i] = nullableStringRef(stringsDict, link.State)
		srcPortNames[i] = nullableStringRef(stringsDict, topologyV1EndpointPortName(link.Src))
		dstPortNames[i] = nullableStringRef(stringsDict, topologyV1EndpointPortName(link.Dst))
		discoveredAt[i] = nullableTime(link.DiscoveredAt)
		lastSeen[i] = nullableTime(link.LastSeen)
		subnet := topologyEvidenceSubnet(link)
		if (linkType == snmpTopologyV1LinkL3Subnet || linkType == snmpTopologyV1LinkL3SubnetMembership) && subnet == "" {
			return topologyapi.Table{}, nil, fmt.Errorf("%s link %d is missing subnet", linkType, i)
		}

		evidenceRows := evidenceRowsByType[linkType]
		if evidenceRows == nil {
			evidenceRows = &snmpTopologyV1EvidenceRows{}
			evidenceRowsByType[linkType] = evidenceRows
		}
		if linkType == snmpTopologyV1LinkL3SubnetMembership {
			count, err := appendSNMPTopologyV1L3SubnetMembershipEvidenceRows(evidenceRows, i, src, dst, link, protocol, stringsDict)
			if err != nil {
				return topologyapi.Table{}, nil, err
			}
			evidenceCounts[i] = count
			continue
		}
		evidenceCounts[i] = 1
		evidenceRows.linkRefs = append(evidenceRows.linkRefs, i)
		evidenceRows.srcActors = append(evidenceRows.srcActors, src)
		evidenceRows.dstActors = append(evidenceRows.dstActors, dst)
		evidenceRows.protocols = append(evidenceRows.protocols, stringsDict.Ref(protocol))
		evidenceRows.directions = append(evidenceRows.directions, stringsDict.Ref(topologyutil.FirstNonEmptyString(link.Direction, "observed")))
		evidenceRows.states = append(evidenceRows.states, nullableStringRef(stringsDict, link.State))
		evidenceRows.srcPortNames = append(evidenceRows.srcPortNames, nullableStringRef(stringsDict, topologyV1EndpointPortName(link.Src)))
		evidenceRows.dstPortNames = append(evidenceRows.dstPortNames, nullableStringRef(stringsDict, topologyV1EndpointPortName(link.Dst)))
		evidenceRows.srcIfIndexes = append(evidenceRows.srcIfIndexes, nullableEndpointIfIndex(link.Src))
		evidenceRows.dstIfIndexes = append(evidenceRows.dstIfIndexes, nullableEndpointIfIndex(link.Dst))
		evidenceRows.srcPortIDs = append(evidenceRows.srcPortIDs, nullableStringRef(stringsDict, topologyV1EndpointString(link.Src, "port_id")))
		evidenceRows.dstPortIDs = append(evidenceRows.dstPortIDs, nullableStringRef(stringsDict, topologyV1EndpointString(link.Dst, "port_id")))
		evidenceRows.srcManagementIPs = append(evidenceRows.srcManagementIPs, nullableStringRef(stringsDict, topologyV1EndpointString(link.Src, "management_ip")))
		evidenceRows.dstManagementIPs = append(evidenceRows.dstManagementIPs, nullableStringRef(stringsDict, topologyV1EndpointString(link.Dst, "management_ip")))
		evidenceRows.confidences = append(evidenceRows.confidences, nullableStringRef(stringsDict, topologymodel.LinkConfidenceValue(link)))
		evidenceRows.inferences = append(evidenceRows.inferences, nullableStringRef(stringsDict, topologymodel.LinkInferenceValue(link)))
		evidenceRows.attachmentModes = append(evidenceRows.attachmentModes, nullableStringRef(stringsDict, topologymodel.LinkAttachmentModeValue(link)))
		if linkType == snmpTopologyV1LinkL3Subnet || linkType == snmpTopologyV1LinkOSPF {
			if linkType == snmpTopologyV1LinkOSPF {
				evidenceRows.srcRouterIDs = append(evidenceRows.srcRouterIDs, nullableStringRef(stringsDict, topologymodel.LinkOSPFLocalRouterID(link)))
				evidenceRows.dstRouterIDs = append(evidenceRows.dstRouterIDs, nullableStringRef(stringsDict, topologymodel.LinkOSPFNeighborRouterID(link)))
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
			evidenceRows.routingInstances = append(evidenceRows.routingInstances, nullableStringRef(stringsDict, topologymodel.LinkBGPRoutingInstance(link)))
			evidenceRows.localIdentifiers = append(evidenceRows.localIdentifiers, nullableStringRef(stringsDict, topologymodel.LinkBGPLocalIdentifier(link)))
			evidenceRows.peerIdentifiers = append(evidenceRows.peerIdentifiers, nullableStringRef(stringsDict, topologymodel.LinkBGPPeerIdentifier(link)))
			evidenceRows.localIPs = append(evidenceRows.localIPs, nullableStringRef(stringsDict, topologymodel.LinkBGPLocalIP(link)))
			evidenceRows.neighborIPs = append(evidenceRows.neighborIPs, nullableStringRef(stringsDict, topologymodel.LinkBGPNeighborIP(link)))
			evidenceRows.localASes = append(evidenceRows.localASes, nullableStringRef(stringsDict, topologymodel.LinkBGPLocalAS(link)))
			evidenceRows.remoteASes = append(evidenceRows.remoteASes, nullableStringRef(stringsDict, topologymodel.LinkBGPRemoteAS(link)))
			evidenceRows.sources = append(evidenceRows.sources, nullableStringRef(stringsDict, topologymodel.LinkBGPSource(link)))
		}
	}

	linkTable := topologyapi.MustTable(len(links),
		[]topologyapi.Column{
			topologyapi.NewColumn("src_actor", "actor_ref", topologyapi.WithRole("reference")),
			topologyapi.NewColumn("dst_actor", "actor_ref", topologyapi.WithRole("reference")),
			topologyapi.NewColumn("type", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithRole("group_key")),
			topologyapi.NewColumn("protocol", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithRole("group_key")),
			topologyapi.NewColumn("direction", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithRole("group_key")),
			topologyapi.NewColumn("state", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithNullable()),
			topologyapi.NewColumn("src_port_name", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithNullable()),
			topologyapi.NewColumn("dst_port_name", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithNullable()),
			topologyapi.NewColumn("evidence_count", "uint", topologyapi.WithAggregation("sum")),
			topologyapi.NewColumn("discovered_at", "timestamp", topologyapi.WithNullable(), topologyapi.WithRole("timestamp")),
			topologyapi.NewColumn("last_seen", "timestamp", topologyapi.WithNullable(), topologyapi.WithRole("timestamp")),
		},
		[]topologyapi.ColumnEncoding{
			topologyapi.Values(srcActors...),
			topologyapi.Values(dstActors...),
			topologyapi.Values(linkTypes...),
			topologyapi.Values(protocols...),
			topologyapi.Values(directions...),
			topologyapi.Values(states...),
			topologyapi.Values(srcPortNames...),
			topologyapi.Values(dstPortNames...),
			topologyapi.Values(evidenceCounts...),
			topologyapi.Values(discoveredAt...),
			topologyapi.Values(lastSeen...),
		},
	)

	evidenceSections := make(topologyapi.EvidenceMap, len(evidenceRowsByType))
	for linkType, rows := range evidenceRowsByType {
		evidenceSections[linkType] = topologyapi.EvidenceSection{
			Type:  linkType,
			Table: rows.table(linkType),
		}
	}

	return linkTable, evidenceSections, nil
}

func appendSNMPTopologyV1L3SubnetMembershipEvidenceRows(
	rows *snmpTopologyV1EvidenceRows,
	linkIndex int,
	srcActor, dstActor int,
	link topologymodel.Link,
	protocol string,
	stringsDict *topologyapi.StringDictionary,
) (any, error) {
	detail := link.Detail.L3SubnetMembership
	if detail == nil {
		return nil, fmt.Errorf("l3_subnet_membership link %d is missing detail", linkIndex)
	}
	if len(detail.Interfaces) == 0 {
		return nil, fmt.Errorf("l3_subnet_membership link %d is missing member interfaces", linkIndex)
	}
	for _, iface := range detail.Interfaces {
		rows.linkRefs = append(rows.linkRefs, linkIndex)
		rows.srcActors = append(rows.srcActors, srcActor)
		rows.dstActors = append(rows.dstActors, dstActor)
		rows.protocols = append(rows.protocols, stringsDict.Ref(protocol))
		rows.directions = append(rows.directions, stringsDict.Ref(topologyutil.FirstNonEmptyString(link.Direction, "observed")))
		rows.states = append(rows.states, nullableStringRef(stringsDict, link.State))
		rows.srcPortNames = append(rows.srcPortNames, nullableStringRef(stringsDict, topologyV1L3SubnetMembershipPortName(iface)))
		rows.dstPortNames = append(rows.dstPortNames, nil)
		rows.srcIfIndexes = append(rows.srcIfIndexes, nullableUintFromInt(iface.IfIndex))
		rows.dstIfIndexes = append(rows.dstIfIndexes, nil)
		rows.srcPortIDs = append(rows.srcPortIDs, nil)
		rows.dstPortIDs = append(rows.dstPortIDs, nil)
		rows.srcManagementIPs = append(rows.srcManagementIPs, nullableStringRef(stringsDict, topologyV1EndpointString(link.Src, "management_ip")))
		rows.dstManagementIPs = append(rows.dstManagementIPs, nil)
		rows.confidences = append(rows.confidences, nullableStringRef(stringsDict, topologymodel.LinkConfidenceValue(link)))
		rows.inferences = append(rows.inferences, nullableStringRef(stringsDict, topologymodel.LinkInferenceValue(link)))
		rows.attachmentModes = append(rows.attachmentModes, nullableStringRef(stringsDict, topologymodel.LinkAttachmentModeValue(link)))
		rows.memberActors = append(rows.memberActors, srcActor)
		rows.segmentActors = append(rows.segmentActors, dstActor)
		rows.memberIPs = append(rows.memberIPs, nullableStringRef(stringsDict, iface.MemberIP))
		rows.memberIfIndexes = append(rows.memberIfIndexes, nullableUintFromInt(iface.IfIndex))
		rows.memberIfNames = append(rows.memberIfNames, nullableStringRef(stringsDict, iface.IfName))
		rows.memberIfDescrs = append(rows.memberIfDescrs, nullableStringRef(stringsDict, iface.IfDescr))
		rows.subnets = append(rows.subnets, nullableStringRef(stringsDict, detail.Subnet))
		rows.networks = append(rows.networks, nullableStringRef(stringsDict, detail.Network))
		rows.netmasks = append(rows.netmasks, nullableStringRef(stringsDict, detail.Netmask))
		rows.prefixes = append(rows.prefixes, nullableUintFromInt(detail.Prefix))
		rows.sources = append(rows.sources, nullableStringRef(stringsDict, detail.Source))
	}
	return len(detail.Interfaces), nil
}

func topologyV1L3SubnetMembershipPortName(iface topologymodel.L3SubnetMembershipInterface) string {
	return topologyutil.FirstNonEmptyString(iface.IfName, iface.IfDescr)
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
	memberActors     []any
	segmentActors    []any
	memberIPs        []any
	memberIfIndexes  []any
	memberIfNames    []any
	memberIfDescrs   []any
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

func (rows *snmpTopologyV1EvidenceRows) table(linkType string) topologyapi.Table {
	return topologyapi.MustTable(len(rows.linkRefs),
		snmpTopologyV1EvidenceColumnsForType(linkType),
		rows.columnEncodingsForType(linkType),
	)
}

func (rows *snmpTopologyV1EvidenceRows) columnEncodingsForType(linkType string) []topologyapi.ColumnEncoding {
	encodings := []topologyapi.ColumnEncoding{
		topologyapi.Values(rows.linkRefs...),
		topologyapi.Values(rows.srcActors...),
		topologyapi.Values(rows.dstActors...),
		topologyapi.Values(rows.protocols...),
		topologyapi.Values(rows.directions...),
		topologyapi.Values(rows.states...),
		topologyapi.Values(rows.srcPortNames...),
		topologyapi.Values(rows.dstPortNames...),
		topologyapi.Values(rows.srcIfIndexes...),
		topologyapi.Values(rows.dstIfIndexes...),
		topologyapi.Values(rows.srcPortIDs...),
		topologyapi.Values(rows.dstPortIDs...),
		topologyapi.Values(rows.srcManagementIPs...),
		topologyapi.Values(rows.dstManagementIPs...),
		topologyapi.Values(rows.confidences...),
		topologyapi.Values(rows.inferences...),
		topologyapi.Values(rows.attachmentModes...),
	}
	if linkType == snmpTopologyV1LinkL3Subnet || linkType == snmpTopologyV1LinkOSPF {
		if linkType == snmpTopologyV1LinkOSPF {
			encodings = append(encodings,
				topologyapi.Values(rows.srcRouterIDs...),
				topologyapi.Values(rows.dstRouterIDs...),
			)
		}
		encodings = append(encodings,
			topologyapi.Values(rows.srcIPs...),
			topologyapi.Values(rows.dstIPs...),
			topologyapi.Values(rows.subnets...),
			topologyapi.Values(rows.networks...),
			topologyapi.Values(rows.netmasks...),
			topologyapi.Values(rows.prefixes...),
			topologyapi.Values(rows.sources...),
		)
	}
	if linkType == snmpTopologyV1LinkBGP {
		encodings = append(encodings,
			topologyapi.Values(rows.routingInstances...),
			topologyapi.Values(rows.localIdentifiers...),
			topologyapi.Values(rows.peerIdentifiers...),
			topologyapi.Values(rows.localIPs...),
			topologyapi.Values(rows.neighborIPs...),
			topologyapi.Values(rows.localASes...),
			topologyapi.Values(rows.remoteASes...),
			topologyapi.Values(rows.sources...),
		)
	}
	if linkType == snmpTopologyV1LinkL3SubnetMembership {
		encodings = append(encodings,
			topologyapi.Values(rows.memberActors...),
			topologyapi.Values(rows.segmentActors...),
			topologyapi.Values(rows.memberIPs...),
			topologyapi.Values(rows.memberIfIndexes...),
			topologyapi.Values(rows.memberIfNames...),
			topologyapi.Values(rows.memberIfDescrs...),
			topologyapi.Values(rows.subnets...),
			topologyapi.Values(rows.networks...),
			topologyapi.Values(rows.netmasks...),
			topologyapi.Values(rows.prefixes...),
			topologyapi.Values(rows.sources...),
		)
	}
	return encodings
}

func snmpTopologyV1EvidenceColumnsForType(linkType string) []topologyapi.Column {
	columns := snmpTopologyV1EvidenceColumns()
	if linkType == snmpTopologyV1LinkOSPF {
		extras := []topologyapi.Column{
			topologyapi.NewColumn("src_router_id", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithNullable()),
			topologyapi.NewColumn("dst_router_id", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithNullable()),
			topologyapi.NewColumn("src_ip", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithNullable()),
			topologyapi.NewColumn("dst_ip", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithNullable()),
			topologyapi.NewColumn("subnet", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithNullable()),
			topologyapi.NewColumn("network", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithNullable()),
			topologyapi.NewColumn("netmask", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithNullable()),
			topologyapi.NewColumn("prefix", "uint", topologyapi.WithNullable()),
			topologyapi.NewColumn("source", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithNullable()),
		}
		return snmpTopologyV1EvidenceColumnsWithExtras(columns, extras)
	}
	if linkType == snmpTopologyV1LinkBGP {
		extras := []topologyapi.Column{
			topologyapi.NewColumn("routing_instance", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithNullable()),
			topologyapi.NewColumn("local_identifier", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithNullable()),
			topologyapi.NewColumn("peer_identifier", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithNullable()),
			topologyapi.NewColumn("local_ip", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithNullable()),
			topologyapi.NewColumn("neighbor_ip", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithNullable()),
			topologyapi.NewColumn("local_as", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithNullable()),
			topologyapi.NewColumn("remote_as", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithNullable()),
			topologyapi.NewColumn("source", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithNullable()),
		}
		return snmpTopologyV1EvidenceColumnsWithExtras(columns, extras)
	}
	if linkType == snmpTopologyV1LinkL3SubnetMembership {
		extras := []topologyapi.Column{
			topologyapi.NewColumn("member_actor", "actor_ref", topologyapi.WithRole("reference")),
			topologyapi.NewColumn("segment_actor", "actor_ref", topologyapi.WithRole("reference")),
			topologyapi.NewColumn("member_ip", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithNullable()),
			topologyapi.NewColumn("member_if_index", "uint", topologyapi.WithNullable()),
			topologyapi.NewColumn("member_if_name", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithNullable()),
			topologyapi.NewColumn("member_if_descr", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithNullable()),
			topologyapi.NewColumn("subnet", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithRole("group_key")),
			topologyapi.NewColumn("network", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithNullable()),
			topologyapi.NewColumn("netmask", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithNullable()),
			topologyapi.NewColumn("prefix", "uint", topologyapi.WithNullable()),
			topologyapi.NewColumn("source", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithNullable()),
		}
		return snmpTopologyV1EvidenceColumnsWithExtras(columns, extras)
	}
	if linkType != snmpTopologyV1LinkL3Subnet {
		return columns
	}
	extras := []topologyapi.Column{
		topologyapi.NewColumn("src_ip", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithNullable()),
		topologyapi.NewColumn("dst_ip", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithNullable()),
		topologyapi.NewColumn("subnet", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithRole("group_key")),
		topologyapi.NewColumn("network", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithNullable()),
		topologyapi.NewColumn("netmask", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithNullable()),
		topologyapi.NewColumn("prefix", "uint", topologyapi.WithNullable()),
		topologyapi.NewColumn("source", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithNullable()),
	}
	return snmpTopologyV1EvidenceColumnsWithExtras(columns, extras)
}

func snmpTopologyV1EvidenceColumnsWithExtras(columns, extras []topologyapi.Column) []topologyapi.Column {
	insertAt := len(columns)
	out := make([]topologyapi.Column, 0, len(columns)+len(extras))
	out = append(out, columns[:insertAt]...)
	out = append(out, extras...)
	out = append(out, columns[insertAt:]...)
	return out
}

func snmpTopologyV1EvidenceColumns() []topologyapi.Column {
	return []topologyapi.Column{
		topologyapi.NewColumn("link", "link_ref", topologyapi.WithRole("reference")),
		topologyapi.NewColumn("src_actor", "actor_ref", topologyapi.WithRole("reference")),
		topologyapi.NewColumn("dst_actor", "actor_ref", topologyapi.WithRole("reference")),
		topologyapi.NewColumn("protocol", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithRole("group_key")),
		topologyapi.NewColumn("direction", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithRole("group_key")),
		topologyapi.NewColumn("state", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithNullable()),
		topologyapi.NewColumn("src_port_name", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithNullable()),
		topologyapi.NewColumn("dst_port_name", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithNullable()),
		topologyapi.NewColumn("src_if_index", "uint", topologyapi.WithNullable()),
		topologyapi.NewColumn("dst_if_index", "uint", topologyapi.WithNullable()),
		topologyapi.NewColumn("src_port_id", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithNullable()),
		topologyapi.NewColumn("dst_port_id", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithNullable()),
		topologyapi.NewColumn("src_management_ip", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithNullable()),
		topologyapi.NewColumn("dst_management_ip", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithNullable()),
		topologyapi.NewColumn("confidence", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithNullable()),
		topologyapi.NewColumn("inference", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithNullable()),
		topologyapi.NewColumn("attachment_mode", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithNullable()),
	}
}

func topologyEvidenceSource(link topologymodel.Link) string {
	if link.Detail.L3Subnet != nil {
		return strings.TrimSpace(link.Detail.L3Subnet.Source)
	}
	if link.Detail.L3SubnetMembership != nil {
		return strings.TrimSpace(link.Detail.L3SubnetMembership.Source)
	}
	if link.Detail.OSPF != nil {
		return strings.TrimSpace(link.Detail.OSPF.Source)
	}
	if link.Detail.BGP != nil {
		return strings.TrimSpace(link.Detail.BGP.Source)
	}
	return ""
}

func topologyEvidenceSrcIP(link topologymodel.Link) string {
	if link.Detail.L3Subnet != nil {
		return strings.TrimSpace(link.Detail.L3Subnet.SrcIP)
	}
	if link.Detail.OSPF != nil {
		return strings.TrimSpace(link.Detail.OSPF.LocalIP)
	}
	return ""
}

func topologyEvidenceDstIP(link topologymodel.Link) string {
	if link.Detail.L3Subnet != nil {
		return strings.TrimSpace(link.Detail.L3Subnet.DstIP)
	}
	if link.Detail.OSPF != nil {
		return strings.TrimSpace(link.Detail.OSPF.NeighborIP)
	}
	return ""
}

func topologyEvidenceSubnet(link topologymodel.Link) string {
	if link.Detail.L3Subnet != nil {
		return strings.TrimSpace(link.Detail.L3Subnet.Subnet)
	}
	if link.Detail.L3SubnetMembership != nil {
		return strings.TrimSpace(link.Detail.L3SubnetMembership.Subnet)
	}
	if link.Detail.OSPF != nil {
		return strings.TrimSpace(link.Detail.OSPF.Subnet)
	}
	return ""
}

func topologyEvidenceNetwork(link topologymodel.Link) string {
	if link.Detail.L3Subnet != nil {
		return strings.TrimSpace(link.Detail.L3Subnet.Network)
	}
	if link.Detail.L3SubnetMembership != nil {
		return strings.TrimSpace(link.Detail.L3SubnetMembership.Network)
	}
	if link.Detail.OSPF != nil {
		return strings.TrimSpace(link.Detail.OSPF.Network)
	}
	return ""
}

func topologyEvidenceNetmask(link topologymodel.Link) string {
	if link.Detail.L3Subnet != nil {
		return strings.TrimSpace(link.Detail.L3Subnet.Netmask)
	}
	if link.Detail.L3SubnetMembership != nil {
		return strings.TrimSpace(link.Detail.L3SubnetMembership.Netmask)
	}
	if link.Detail.OSPF != nil {
		return strings.TrimSpace(link.Detail.OSPF.Netmask)
	}
	return ""
}

func nullableEvidencePrefix(link topologymodel.Link) any {
	prefix := 0
	if link.Detail.L3Subnet != nil {
		prefix = link.Detail.L3Subnet.Prefix
	} else if link.Detail.L3SubnetMembership != nil {
		prefix = link.Detail.L3SubnetMembership.Prefix
	} else if link.Detail.OSPF != nil {
		prefix = link.Detail.OSPF.Prefix
	}
	if prefix <= 0 {
		return nil
	}
	return uint64(prefix)
}

func snmpTopologyV1LinkType(link topologymodel.Link) string {
	if snmpTopologyV1LinkIsProbable(link) {
		return snmpTopologyV1LinkProbable
	}

	switch strings.ToLower(strings.TrimSpace(topologyutil.FirstNonEmptyString(link.Protocol, link.LinkType))) {
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
	case snmpTopologyV1LinkL3SubnetMembership:
		return snmpTopologyV1LinkL3SubnetMembership
	case snmpTopologyV1LinkOSPF:
		return snmpTopologyV1LinkOSPF
	case snmpTopologyV1LinkBGP:
		return snmpTopologyV1LinkBGP
	default:
		return snmpTopologyV1LinkObservation
	}
}

func snmpTopologyV1LinkIsLogicalL3(link topologymodel.Link) bool {
	switch snmpTopologyV1LinkType(link) {
	case snmpTopologyV1LinkL3Subnet, snmpTopologyV1LinkL3SubnetMembership, snmpTopologyV1LinkOSPF, snmpTopologyV1LinkBGP:
		return true
	default:
		return false
	}
}

func snmpTopologyV1LinkIsProbable(link topologymodel.Link) bool {
	if strings.EqualFold(strings.TrimSpace(link.State), snmpTopologyV1LinkProbable) {
		return true
	}
	if strings.EqualFold(topologymodel.LinkInferenceValue(link), snmpTopologyV1LinkProbable) {
		return true
	}
	return strings.HasPrefix(strings.ToLower(topologymodel.LinkAttachmentModeValue(link)), snmpTopologyV1LinkProbable+"_")
}
