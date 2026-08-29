// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"
)

func topologyInspectionSubjectFromLink(data topologymodel.Data, index int) (topologyInspectionLinkSubject, bool) {
	if index < 0 || index >= len(data.Links) {
		return topologyInspectionLinkSubject{}, false
	}
	link := data.Links[index]
	actors := make(map[topologymodel.ActorHandle]topologymodel.Actor, len(data.Actors))
	for _, actor := range data.Actors {
		actors[actor.ActorHandle] = actor
	}
	srcIdentity := topologyInspectionPreferredActorIdentity(actors[link.SrcActorHandle])
	dstIdentity := topologyInspectionPreferredActorIdentity(actors[link.DstActorHandle])
	if srcIdentity == "" || dstIdentity == "" {
		return topologyInspectionLinkSubject{}, false
	}
	return normalizeTopologyInspectionLinkSubject(topologyInspectionLinkSubject{
		srcIdentity:   srcIdentity,
		dstIdentity:   dstIdentity,
		family:        topologyInspectionLinkFamily(link),
		protocol:      link.Protocol,
		direction:     link.Direction,
		discriminator: topologyInspectionLinkDiscriminatorForLink(link),
	}), true
}

func inspectTopologyGraphLink(
	data topologymodel.Data,
	subject topologyInspectionLinkSubject,
) topologyInspectionGraphLinkResult {
	subject = normalizeTopologyInspectionLinkSubject(subject)
	result := topologyInspectionGraphLinkResult{
		srcActors: inspectTopologyActorIdentity(data, subject.srcIdentity),
		dstActors: inspectTopologyActorIdentity(data, subject.dstIdentity),
		index:     -1,
	}
	if result.srcActors.membership.state == topologyInspectionUndetermined ||
		result.dstActors.membership.state == topologyInspectionUndetermined {
		return result
	}
	if result.srcActors.membership.state == topologyInspectionAbsent ||
		result.dstActors.membership.state == topologyInspectionAbsent {
		result.membership.state = topologyInspectionAbsent
		return result
	}

	srcHandle := result.srcActors.actors[0].ActorHandle
	dstHandle := result.dstActors.actors[0].ActorHandle
	for i := range data.Links {
		if topologyInspectionLinkMatches(data.Links[i], srcHandle, dstHandle, subject) {
			result.links = append(result.links, data.Links[i])
			if result.index == -1 {
				result.index = i
			}
		}
	}
	result.membership.candidates = len(result.links)
	switch len(result.links) {
	case 0:
		result.membership.state = topologyInspectionAbsent
	case 1:
		result.membership.state = topologyInspectionPresent
	default:
		result.membership.state = topologyInspectionUndetermined
		result.index = -1
	}
	return result
}

func topologyInspectionLinkMatches(
	link topologymodel.Link,
	srcHandle topologymodel.ActorHandle,
	dstHandle topologymodel.ActorHandle,
	subject topologyInspectionLinkSubject,
) bool {
	if topologyInspectionLinkFamily(link) != subject.family ||
		normalizeTopologyInspectionToken(link.Protocol) != subject.protocol ||
		normalizeTopologyInspectionDirection(link.Direction) != subject.direction {
		return false
	}
	actual := topologyInspectionLinkDiscriminatorForLink(link)
	if link.SrcActorHandle == srcHandle && link.DstActorHandle == dstHandle {
		return actual == subject.discriminator
	}
	if topologyInspectionLinkSubjectUnordered(subject) &&
		link.SrcActorHandle == dstHandle && link.DstActorHandle == srcHandle {
		return topologyInspectionSwapLinkDiscriminator(actual) == subject.discriminator
	}
	return false
}

func topologyInspectionLinkSubjectUnordered(subject topologyInspectionLinkSubject) bool {
	if subject.direction == "bidirectional" {
		return true
	}
	switch subject.family {
	case topologymodel.L3SubnetLinkType,
		topologymodel.OSPFAdjacencyLinkType,
		topologymodel.BGPAdjacencyLinkType:
		return true
	default:
		return false
	}
}

func normalizeTopologyInspectionLinkSubject(subject topologyInspectionLinkSubject) topologyInspectionLinkSubject {
	subject.srcIdentity = strings.TrimSpace(subject.srcIdentity)
	subject.dstIdentity = strings.TrimSpace(subject.dstIdentity)
	subject.family = normalizeTopologyInspectionToken(subject.family)
	subject.protocol = normalizeTopologyInspectionToken(subject.protocol)
	if subject.protocol == "" {
		subject.protocol = subject.family
	}
	subject.direction = normalizeTopologyInspectionDirection(subject.direction)
	subject.discriminator = normalizeTopologyInspectionLinkDiscriminator(subject.discriminator)
	return subject
}

func normalizeTopologyInspectionLinkDiscriminator(value topologyInspectionLinkDiscriminator) topologyInspectionLinkDiscriminator {
	value.srcIfIndex = max(value.srcIfIndex, 0)
	value.dstIfIndex = max(value.dstIfIndex, 0)
	value.srcIfName = strings.TrimSpace(value.srcIfName)
	value.dstIfName = strings.TrimSpace(value.dstIfName)
	value.srcPortID = strings.TrimSpace(value.srcPortID)
	value.dstPortID = strings.TrimSpace(value.dstPortID)
	value.bridgeDomain = strings.TrimSpace(value.bridgeDomain)
	value.subnet = strings.TrimSpace(value.subnet)
	value.ospfRouterA = topologyutil.NormalizeTopologyRouterID(value.ospfRouterA)
	value.ospfRouterB = topologyutil.NormalizeTopologyRouterID(value.ospfRouterB)
	if value.ospfRouterA > value.ospfRouterB {
		value.ospfRouterA, value.ospfRouterB = value.ospfRouterB, value.ospfRouterA
	}
	value.ospfAdjacency = strings.TrimSpace(value.ospfAdjacency)
	value.bgpRoutingInstance = strings.TrimSpace(value.bgpRoutingInstance)
	return value
}

func topologyInspectionLinkDiscriminatorForLink(link topologymodel.Link) topologyInspectionLinkDiscriminator {
	var result topologyInspectionLinkDiscriminator
	switch topologyInspectionLinkFamily(link) {
	case topologymodel.L3SubnetLinkType:
		if link.Detail.L3Subnet != nil {
			result.subnet = link.Detail.L3Subnet.Subnet
			result.prefix = link.Detail.L3Subnet.Prefix
		}
	case topologymodel.L3SubnetMembershipLinkType:
		if link.Detail.L3SubnetMembership != nil {
			result.subnet = link.Detail.L3SubnetMembership.Subnet
			result.prefix = link.Detail.L3SubnetMembership.Prefix
		}
	case topologymodel.OSPFAdjacencyLinkType:
		if detail := link.Detail.OSPF; detail != nil {
			result.ospfRouterA = detail.LocalRouterID
			result.ospfRouterB = detail.NeighborRouterID
			result.ospfAdjacency = topologyInspectionOSPFAdjacency(
				detail.Subnet,
				detail.Prefix,
				detail.LocalIP,
				detail.NeighborIP,
			)
		}
	case topologymodel.BGPAdjacencyLinkType:
		if detail := link.Detail.BGP; detail != nil {
			result.bgpRoutingInstance = topologyutil.FirstNonEmptyString(detail.RoutingInstance, "default")
		}
	default:
		result.srcIfIndex = max(link.Src.IfIndex, 0)
		result.srcIfName = topologymodel.EndpointKey(link.Src, "if_name")
		result.srcPortID = topologymodel.EndpointKey(link.Src, "port_id")
		result.dstIfIndex = max(link.Dst.IfIndex, 0)
		result.dstIfName = topologymodel.EndpointKey(link.Dst, "if_name")
		result.dstPortID = topologymodel.EndpointKey(link.Dst, "port_id")
		if link.L2 != nil {
			result.bridgeDomain = link.L2.BridgeDomain
		}
	}
	return normalizeTopologyInspectionLinkDiscriminator(result)
}

func topologyInspectionSwapLinkDiscriminator(value topologyInspectionLinkDiscriminator) topologyInspectionLinkDiscriminator {
	value.srcIfIndex, value.dstIfIndex = value.dstIfIndex, value.srcIfIndex
	value.srcIfName, value.dstIfName = value.dstIfName, value.srcIfName
	value.srcPortID, value.dstPortID = value.dstPortID, value.srcPortID
	return value
}

func topologyInspectionOSPFAdjacency(subnet string, prefix int, localIP, neighborIP string) string {
	if subnet = strings.TrimSpace(subnet); subnet != "" && prefix > 0 {
		return topologyutil.JoinKeyParts("subnet", subnet, strconv.Itoa(prefix))
	}
	localIP = topologyutil.NormalizeNonUnspecifiedIPAddress(localIP)
	neighborIP = topologyutil.NormalizeNonUnspecifiedIPAddress(neighborIP)
	if localIP != "" && neighborIP != "" {
		if localIP > neighborIP {
			localIP, neighborIP = neighborIP, localIP
		}
		return topologyutil.JoinKeyParts("ip_pair", localIP, neighborIP)
	}
	return "router_id"
}

func topologyInspectionLinkFamily(link topologymodel.Link) string {
	return normalizeTopologyInspectionToken(topologyutil.FirstNonEmptyString(link.LinkType, link.Protocol))
}

func normalizeTopologyInspectionToken(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeTopologyInspectionDirection(value string) string {
	return normalizeTopologyInspectionToken(topologyutil.FirstNonEmptyString(value, "observed"))
}

func topologyInspectionRenderedRow(
	renderState topologyInspectionState,
	membership topologyInspectionState,
	row int,
	rows int,
) topologyInspectionRowResult {
	result := topologyInspectionRowResult{row: -1}
	if renderState != topologyInspectionPresent {
		return result
	}
	switch membership {
	case topologyInspectionAbsent:
		result.state = topologyInspectionAbsent
	case topologyInspectionPresent:
		if row >= 0 && row < rows {
			result.state = topologyInspectionPresent
			result.row = row
		}
	}
	return result
}
