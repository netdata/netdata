// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
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
		srcIdentity: srcIdentity,
		dstIdentity: dstIdentity,
		family:      topologyInspectionLinkFamily(link),
		protocol:    link.Protocol,
		direction:   link.Direction,
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
	if link.SrcActorHandle == srcHandle && link.DstActorHandle == dstHandle {
		return true
	}
	if topologyInspectionLinkSubjectUnordered(subject) &&
		link.SrcActorHandle == dstHandle && link.DstActorHandle == srcHandle {
		return true
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
	return subject
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
