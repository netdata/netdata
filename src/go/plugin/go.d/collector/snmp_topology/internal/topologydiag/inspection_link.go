// SPDX-License-Identifier: GPL-3.0-or-later

package topologydiag

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"
)

func inspectionSubjectFromLink(data topologymodel.Data, index int) (inspectionLinkSubject, bool) {
	if index < 0 || index >= len(data.Links) {
		return inspectionLinkSubject{}, false
	}
	link := data.Links[index]
	actors := make(map[topologymodel.ActorHandle]topologymodel.Actor, len(data.Actors))
	for _, actor := range data.Actors {
		actors[actor.ActorHandle] = actor
	}
	srcIdentity := inspectionPreferredActorIdentity(actors[link.SrcActorHandle])
	dstIdentity := inspectionPreferredActorIdentity(actors[link.DstActorHandle])
	if srcIdentity == "" || dstIdentity == "" {
		return inspectionLinkSubject{}, false
	}
	return normalizeInspectionLinkSubject(inspectionLinkSubject{
		srcIdentity: srcIdentity,
		dstIdentity: dstIdentity,
		family:      inspectionLinkFamily(link),
		protocol:    link.Protocol,
		direction:   link.Direction,
	}), true
}

func inspectGraphLink(
	data topologymodel.Data,
	subject inspectionLinkSubject,
) inspectionGraphLinkResult {
	subject = normalizeInspectionLinkSubject(subject)
	result := inspectionGraphLinkResult{
		srcActors: inspectActorIdentity(data, subject.srcIdentity),
		dstActors: inspectActorIdentity(data, subject.dstIdentity),
		index:     -1,
	}
	if result.srcActors.membership.state == inspectionAbsent ||
		result.dstActors.membership.state == inspectionAbsent {
		result.membership.state = inspectionAbsent
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
		if inspectionLinkMatches(data.Links[i], srcHandles, dstHandles, subject) {
			result.links = append(result.links, data.Links[i])
			if result.index == -1 {
				result.index = i
			}
		}
	}
	result.membership.candidates = len(result.links)
	if result.srcActors.membership.state == inspectionUndetermined ||
		result.dstActors.membership.state == inspectionUndetermined {
		result.membership.state = inspectionUndetermined
		result.index = -1
		return result
	}
	switch len(result.links) {
	case 0:
		result.membership.state = inspectionAbsent
	case 1:
		result.membership.state = inspectionPresent
	default:
		result.membership.state = inspectionUndetermined
		result.index = -1
	}
	return result
}

func inspectGraphLinkAt(
	data topologymodel.Data,
	index int,
) inspectionGraphLinkResult {
	link := data.Links[index]
	return inspectionGraphLinkResult{
		membership: inspectionStage{state: inspectionPresent, candidates: 1},
		srcActors:  inspectActorHandle(data, link.SrcActorHandle),
		dstActors:  inspectActorHandle(data, link.DstActorHandle),
		links:      []topologymodel.Link{link},
		index:      index,
	}
}

func inspectActorHandle(
	data topologymodel.Data,
	handle topologymodel.ActorHandle,
) inspectionActorResult {
	indexes := make([]int, 0, 1)
	for i := range data.Actors {
		if data.Actors[i].ActorHandle == handle {
			indexes = append(indexes, i)
		}
	}
	return inspectionActorsAt(data, indexes)
}

func inspectionLinkMatches(
	link topologymodel.Link,
	srcHandles map[topologymodel.ActorHandle]struct{},
	dstHandles map[topologymodel.ActorHandle]struct{},
	subject inspectionLinkSubject,
) bool {
	if inspectionLinkFamily(link) != subject.family ||
		normalizeInspectionToken(link.Protocol) != subject.protocol ||
		normalizeInspectionDirection(link.Direction) != subject.direction {
		return false
	}
	_, srcMatchesSrc := srcHandles[link.SrcActorHandle]
	_, dstMatchesDst := dstHandles[link.DstActorHandle]
	if srcMatchesSrc && dstMatchesDst {
		return true
	}
	if !inspectionLinkSubjectUnordered(subject) {
		return false
	}
	_, srcMatchesDst := dstHandles[link.SrcActorHandle]
	_, dstMatchesSrc := srcHandles[link.DstActorHandle]
	return srcMatchesDst && dstMatchesSrc
}

func inspectionLinkSubjectUnordered(subject inspectionLinkSubject) bool {
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

func normalizeInspectionLinkSubject(subject inspectionLinkSubject) inspectionLinkSubject {
	subject.srcIdentity = strings.TrimSpace(subject.srcIdentity)
	subject.dstIdentity = strings.TrimSpace(subject.dstIdentity)
	subject.family = normalizeInspectionToken(subject.family)
	subject.protocol = normalizeInspectionToken(subject.protocol)
	if subject.protocol == "" {
		subject.protocol = subject.family
	}
	subject.direction = normalizeInspectionDirection(subject.direction)
	return subject
}

func inspectionLinkFamily(link topologymodel.Link) string {
	return normalizeInspectionToken(topologyutil.FirstNonEmptyString(link.LinkType, link.Protocol))
}

func normalizeInspectionToken(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeInspectionDirection(value string) string {
	return normalizeInspectionToken(topologyutil.FirstNonEmptyString(value, "observed"))
}

func inspectionRenderedRow(
	renderState inspectionState,
	membership inspectionStage,
	row int,
	rows int,
) inspectionRowResult {
	result := inspectionRowResult{row: -1}
	if renderState != inspectionPresent {
		return result
	}
	result.candidates = membership.candidates
	switch membership.state {
	case inspectionAbsent:
		result.state = inspectionAbsent
	case inspectionPresent:
		if row >= 0 && row < rows {
			result.state = inspectionPresent
			result.row = row
		}
	}
	return result
}
