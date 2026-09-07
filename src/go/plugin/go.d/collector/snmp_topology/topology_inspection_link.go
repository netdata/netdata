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
	if result.srcActors.membership.state == topologyInspectionAbsent ||
		result.dstActors.membership.state == topologyInspectionAbsent {
		result.membership.state = topologyInspectionAbsent
		return result
	}

	srcHandles := make(map[topologymodel.ActorHandle]struct{}, len(result.srcActors.actors))
	for _, actor := range result.srcActors.actors {
		srcHandles[actor.ActorHandle] = struct{}{}
	}
	dstHandles := make(map[topologymodel.ActorHandle]struct{}, len(result.dstActors.actors))
	for _, actor := range result.dstActors.actors {
		dstHandles[actor.ActorHandle] = struct{}{}
	}
	for i := range data.Links {
		if topologyInspectionLinkMatches(data.Links[i], srcHandles, dstHandles, subject) {
			result.links = append(result.links, data.Links[i])
			if result.index == -1 {
				result.index = i
			}
		}
	}
	result.membership.candidates = len(result.links)
	if result.srcActors.membership.state == topologyInspectionUndetermined ||
		result.dstActors.membership.state == topologyInspectionUndetermined {
		result.membership.state = topologyInspectionUndetermined
		result.index = -1
		return result
	}
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

func inspectTopologyGraphLinkAt(
	data topologymodel.Data,
	index int,
) topologyInspectionGraphLinkResult {
	link := data.Links[index]
	return topologyInspectionGraphLinkResult{
		membership: topologyInspectionStage{state: topologyInspectionPresent, candidates: 1},
		srcActors:  inspectTopologyActorHandle(data, link.SrcActorHandle),
		dstActors:  inspectTopologyActorHandle(data, link.DstActorHandle),
		links:      []topologymodel.Link{link},
		index:      index,
	}
}

func inspectTopologyActorHandle(
	data topologymodel.Data,
	handle topologymodel.ActorHandle,
) topologyInspectionActorResult {
	indexes := make([]int, 0, 1)
	for i := range data.Actors {
		if data.Actors[i].ActorHandle == handle {
			indexes = append(indexes, i)
		}
	}
	return topologyInspectionActorsAt(data, indexes)
}

func topologyInspectionLinkMatches(
	link topologymodel.Link,
	srcHandles map[topologymodel.ActorHandle]struct{},
	dstHandles map[topologymodel.ActorHandle]struct{},
	subject topologyInspectionLinkSubject,
) bool {
	if topologyInspectionLinkFamily(link) != subject.family ||
		normalizeTopologyInspectionToken(link.Protocol) != subject.protocol ||
		normalizeTopologyInspectionDirection(link.Direction) != subject.direction {
		return false
	}
	_, srcMatchesSrc := srcHandles[link.SrcActorHandle]
	_, dstMatchesDst := dstHandles[link.DstActorHandle]
	if srcMatchesSrc && dstMatchesDst {
		return true
	}
	if !topologyInspectionLinkSubjectUnordered(subject) {
		return false
	}
	_, srcMatchesDst := dstHandles[link.SrcActorHandle]
	_, dstMatchesSrc := srcHandles[link.DstActorHandle]
	return srcMatchesDst && dstMatchesSrc
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
	membership topologyInspectionStage,
	row int,
	rows int,
) topologyInspectionRowResult {
	result := topologyInspectionRowResult{row: -1}
	if renderState != topologyInspectionPresent {
		return result
	}
	result.candidates = membership.candidates
	switch membership.state {
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
