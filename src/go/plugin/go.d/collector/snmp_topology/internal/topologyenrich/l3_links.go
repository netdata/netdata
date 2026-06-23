// SPDX-License-Identifier: GPL-3.0-or-later

package topologyenrich

import (
	"crypto/sha1"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/l2topology"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"

	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
)

const topologyL3SubnetSource = "ip_mib"

func ApplyL3Subnet(data *topologymodel.Data, aggregate topologymodel.ObservationAggregate) topologymodel.L3EnrichmentStats {
	var stats topologymodel.L3EnrichmentStats
	if data == nil || len(aggregate.L3Interfaces) == 0 {
		return finishTopologyL3SubnetEnrichment(data, stats)
	}

	candidates, subnetStats := buildTopologyL3SubnetCandidates(aggregate.L3Interfaces)
	stats.SubnetStats = subnetStats
	if len(candidates.Adjacencies) == 0 && len(candidates.Segments) == 0 {
		return finishTopologyL3SubnetEnrichment(data, stats)
	}

	resolver := newTopologyL3ActorResolver(data, aggregate.Snapshots)
	directSeen := existingTopologyL3LinkKeys(data.Links)
	for _, adjacency := range candidates.Adjacencies {
		srcRef, ok := resolver.resolve(adjacency.A)
		if !ok {
			stats.SuppressedUnresolvedActor++
			continue
		}
		dstRef, ok := resolver.resolve(adjacency.B)
		if !ok {
			stats.SuppressedUnresolvedActor++
			continue
		}
		if srcRef.actorID == dstRef.actorID {
			stats.SuppressedSelfActor++
			continue
		}
		link := topologyL3SubnetLink(adjacency, srcRef, dstRef)
		key := topologyL3SubnetLinkKey(link)
		if _, exists := directSeen[key]; exists {
			stats.SuppressedDuplicateLink++
			continue
		}
		directSeen[key] = struct{}{}
		data.Links = append(data.Links, link)
		stats.EmittedLinks++
	}

	applyTopologyL3SubnetSegments(data, candidates.Segments, resolver, strings.TrimSpace(aggregate.ProducerScopeID), &stats)

	sort.Slice(data.Actors, func(i, j int) bool {
		return strings.TrimSpace(data.Actors[i].ActorID) < strings.TrimSpace(data.Actors[j].ActorID)
	})
	sort.Slice(data.Links, func(i, j int) bool {
		return topologymodel.LinkSortKey(data.Links[i]) < topologymodel.LinkSortKey(data.Links[j])
	})
	return finishTopologyL3SubnetEnrichment(data, stats)
}

func finishTopologyL3SubnetEnrichment(data *topologymodel.Data, stats topologymodel.L3EnrichmentStats) topologymodel.L3EnrichmentStats {
	recordTopologyL3EnrichmentStats(data, stats)
	topologymodel.RecomputeLinkStats(data)
	return stats
}

func topologyL3SubnetLink(adjacency topologyL3SubnetAdjacency, srcRef, dstRef topologyL3ActorRef) topologymodel.Link {
	return topologymodel.Link{
		Layer:      "3",
		Protocol:   topologymodel.L3SubnetLinkType,
		LinkType:   topologymodel.L3SubnetLinkType,
		Direction:  "observed",
		SrcActorID: srcRef.actorID,
		DstActorID: dstRef.actorID,
		Src: topologymodel.LinkEndpoint{
			Match:   srcRef.match,
			IfIndex: topologyutil.ParseIndex(adjacency.A.IfIndex),
			IfName:  strings.TrimSpace(adjacency.A.IfName),
			IfDescr: strings.TrimSpace(adjacency.A.IfDescr),
		},
		Dst: topologymodel.LinkEndpoint{
			Match:   dstRef.match,
			IfIndex: topologyutil.ParseIndex(adjacency.B.IfIndex),
			IfName:  strings.TrimSpace(adjacency.B.IfName),
			IfDescr: strings.TrimSpace(adjacency.B.IfDescr),
		},
		Inference: &graph.LinkInference{
			Inference:      "shared_subnet",
			AttachmentMode: "logical_l3_subnet",
		},
		Detail: topologymodel.LinkDetail{
			L3Subnet: &topologymodel.L3SubnetLinkDetail{
				Source:  topologyL3SubnetSource,
				SrcIP:   topologyutil.NormalizeIPAddress(adjacency.A.IP),
				DstIP:   topologyutil.NormalizeIPAddress(adjacency.B.IP),
				Subnet:  adjacency.Subnet,
				Network: adjacency.Network,
				Netmask: adjacency.Netmask,
				Prefix:  adjacency.Prefix,
			},
		},
	}
}

type topologyL3SubnetMember struct {
	ref        topologyL3ActorRef
	interfaces []topologymodel.L3SubnetMembershipInterface
}

func applyTopologyL3SubnetSegments(
	data *topologymodel.Data,
	segments []topologyL3SubnetSegment,
	resolver topologyL3ActorResolver,
	producerScopeID string,
	stats *topologymodel.L3EnrichmentStats,
) {
	if data == nil || len(segments) == 0 {
		return
	}
	if producerScopeID == "" {
		stats.SuppressedNoProducerScope += len(segments)
		return
	}

	actorSeen := existingTopologyActorIDs(data.Actors)
	membershipSeen := existingTopologyL3MembershipLinkKeys(data.Links)
	for _, segment := range segments {
		members := topologyL3SubnetSegmentMembers(segment, resolver, stats)
		if len(members) < 2 {
			stats.SuppressedMembershipUnmatched++
			continue
		}

		segmentActor := topologyL3SubnetSegmentActor(producerScopeID, segment, members)
		segmentActorID := strings.TrimSpace(segmentActor.ActorID)
		segmentActorEmitted := false
		for _, member := range members {
			link := topologyL3SubnetMembershipLink(segment, segmentActorID, member)
			key := topologyL3SubnetMembershipLinkKey(link)
			if _, exists := membershipSeen[key]; exists {
				stats.SuppressedDuplicateMembershipLink++
				continue
			}
			membershipSeen[key] = struct{}{}
			if _, exists := actorSeen[segmentActorID]; !exists && !segmentActorEmitted {
				data.Actors = append(data.Actors, segmentActor)
				actorSeen[segmentActorID] = struct{}{}
				segmentActorEmitted = true
				stats.EmittedSegments++
			}
			data.Links = append(data.Links, link)
			stats.EmittedMembershipLinks++
		}
	}
}

func topologyL3SubnetSegmentMembers(
	segment topologyL3SubnetSegment,
	resolver topologyL3ActorResolver,
	stats *topologymodel.L3EnrichmentStats,
) []topologyL3SubnetMember {
	byActor := make(map[string]*topologyL3SubnetMember, len(segment.Rows))
	for _, row := range segment.Rows {
		ref, ok := resolver.resolve(row)
		if !ok {
			stats.SuppressedMembershipUnresolvedActor++
			continue
		}
		actorID := strings.TrimSpace(ref.actorID)
		if actorID == "" {
			stats.SuppressedMembershipUnresolvedActor++
			continue
		}
		member := byActor[actorID]
		if member == nil {
			member = &topologyL3SubnetMember{ref: ref}
			byActor[actorID] = member
		}
		member.interfaces = append(member.interfaces, topologyL3SubnetMembershipInterface(row))
	}

	actorIDs := make([]string, 0, len(byActor))
	for actorID := range byActor {
		actorIDs = append(actorIDs, actorID)
	}
	sort.Strings(actorIDs)

	out := make([]topologyL3SubnetMember, 0, len(actorIDs))
	for _, actorID := range actorIDs {
		member := byActor[actorID]
		sort.Slice(member.interfaces, func(i, j int) bool {
			return topologyL3SubnetMembershipInterfaceSortKey(member.interfaces[i]) < topologyL3SubnetMembershipInterfaceSortKey(member.interfaces[j])
		})
		out = append(out, *member)
	}
	return out
}

func topologyL3SubnetMembershipInterface(row topologymodel.L3Interface) topologymodel.L3SubnetMembershipInterface {
	return topologymodel.L3SubnetMembershipInterface{
		MemberIP: topologyutil.NormalizeIPAddress(row.IP),
		IfIndex:  topologyutil.ParseIndex(row.IfIndex),
		IfName:   strings.TrimSpace(row.IfName),
		IfDescr:  strings.TrimSpace(row.IfDescr),
	}
}

func topologyL3SubnetMembershipInterfaceSortKey(row topologymodel.L3SubnetMembershipInterface) string {
	return strings.Join([]string{
		strings.TrimSpace(row.MemberIP),
		strconv.Itoa(row.IfIndex),
		strings.TrimSpace(row.IfName),
		strings.TrimSpace(row.IfDescr),
	}, "\x00")
}

func topologyL3SubnetSegmentActor(producerScopeID string, segment topologyL3SubnetSegment, members []topologyL3SubnetMember) topologymodel.Actor {
	actorID := topologyL3SubnetSegmentActorID(producerScopeID, segment)
	portCount := 0
	for _, member := range members {
		portCount += len(member.interfaces)
	}
	return topologymodel.Actor{
		ActorID:     actorID,
		ActorType:   topologymodel.L3SubnetSegmentActorType,
		SegmentKind: topologymodel.SegmentKindL3Subnet,
		Layer:       "3",
		Source:      topologyL3SubnetSource,
		Detail: topologymodel.ActorDetail{
			L2: topologyengine.ProjectionActorDetail{
				DisplayName: segment.Subnet,
				Segment: topologyengine.ProjectionSegmentActorDetail{
					SegmentID:      actorID,
					SegmentType:    topologymodel.SegmentKindL3Subnet,
					SegmentKind:    topologymodel.SegmentKindL3Subnet,
					PortsTotal:     topologyengine.OptionalValue[int]{Value: portCount, Has: true},
					EndpointsTotal: topologyengine.OptionalValue[int]{Value: len(members), Has: true},
				},
			},
		},
	}
}

func topologyL3SubnetSegmentActorID(producerScopeID string, segment topologyL3SubnetSegment) string {
	sum := sha1.Sum([]byte(topologyutil.JoinKeyParts(
		strings.TrimSpace(producerScopeID),
		strings.TrimSpace(segment.Network),
		strconv.Itoa(segment.Prefix),
	)))
	return "l3_subnet_segment:" + hex.EncodeToString(sum[:8])
}

func topologyL3SubnetMembershipLink(segment topologyL3SubnetSegment, segmentActorID string, member topologyL3SubnetMember) topologymodel.Link {
	src := topologymodel.LinkEndpoint{Match: member.ref.match}
	if len(member.interfaces) > 0 {
		src.IfIndex = member.interfaces[0].IfIndex
		src.IfName = member.interfaces[0].IfName
		src.IfDescr = member.interfaces[0].IfDescr
	}
	return topologymodel.Link{
		Layer:      "3",
		Protocol:   topologymodel.L3SubnetMembershipLinkType,
		LinkType:   topologymodel.L3SubnetMembershipLinkType,
		Direction:  "observed",
		SrcActorID: member.ref.actorID,
		DstActorID: segmentActorID,
		Src:        src,
		Dst:        topologymodel.LinkEndpoint{},
		Inference: &graph.LinkInference{
			Inference:      "shared_subnet_membership",
			AttachmentMode: "logical_l3_subnet_membership",
		},
		Detail: topologymodel.LinkDetail{
			L3SubnetMembership: &topologymodel.L3SubnetMembershipLinkDetail{
				Source:     topologyL3SubnetSource,
				Subnet:     segment.Subnet,
				Network:    segment.Network,
				Netmask:    segment.Netmask,
				Prefix:     segment.Prefix,
				Interfaces: append([]topologymodel.L3SubnetMembershipInterface(nil), member.interfaces...),
			},
		},
	}
}

func existingTopologyL3LinkKeys(links []topologymodel.Link) map[string]struct{} {
	seen := make(map[string]struct{})
	for _, link := range links {
		if strings.EqualFold(strings.TrimSpace(topologyutil.FirstNonEmptyString(link.LinkType, link.Protocol)), topologymodel.L3SubnetLinkType) {
			seen[topologyL3SubnetLinkKey(link)] = struct{}{}
		}
	}
	return seen
}

func existingTopologyL3MembershipLinkKeys(links []topologymodel.Link) map[string]struct{} {
	seen := make(map[string]struct{})
	for _, link := range links {
		if strings.EqualFold(strings.TrimSpace(topologyutil.FirstNonEmptyString(link.LinkType, link.Protocol)), topologymodel.L3SubnetMembershipLinkType) {
			seen[topologyL3SubnetMembershipLinkKey(link)] = struct{}{}
		}
	}
	return seen
}

func existingTopologyActorIDs(actors []topologymodel.Actor) map[string]struct{} {
	seen := make(map[string]struct{}, len(actors))
	for _, actor := range actors {
		if actorID := strings.TrimSpace(actor.ActorID); actorID != "" {
			seen[actorID] = struct{}{}
		}
	}
	return seen
}

func topologyL3SubnetLinkKey(link topologymodel.Link) string {
	src := strings.TrimSpace(link.SrcActorID)
	dst := strings.TrimSpace(link.DstActorID)
	if src > dst {
		src, dst = dst, src
	}
	return topologyutil.JoinKeyParts(
		src,
		dst,
		topologyL3Subnet(link),
		strconv.Itoa(topologyL3SubnetPrefix(link)),
	)
}

func topologyL3SubnetMembershipLinkKey(link topologymodel.Link) string {
	return topologyutil.JoinKeyParts(
		strings.TrimSpace(link.SrcActorID),
		strings.TrimSpace(link.DstActorID),
		topologyL3SubnetMembershipSubnet(link),
		strconv.Itoa(topologyL3SubnetMembershipPrefix(link)),
	)
}

func topologyL3Subnet(link topologymodel.Link) string {
	if link.Detail.L3Subnet == nil {
		return ""
	}
	return strings.TrimSpace(link.Detail.L3Subnet.Subnet)
}

func topologyL3SubnetPrefix(link topologymodel.Link) int {
	if link.Detail.L3Subnet == nil {
		return 0
	}
	return link.Detail.L3Subnet.Prefix
}

func topologyL3SubnetMembershipSubnet(link topologymodel.Link) string {
	if link.Detail.L3SubnetMembership == nil {
		return ""
	}
	return strings.TrimSpace(link.Detail.L3SubnetMembership.Subnet)
}

func topologyL3SubnetMembershipPrefix(link topologymodel.Link) int {
	if link.Detail.L3SubnetMembership == nil {
		return 0
	}
	return link.Detail.L3SubnetMembership.Prefix
}

func recordTopologyL3EnrichmentStats(data *topologymodel.Data, stats topologymodel.L3EnrichmentStats) {
	if data == nil {
		return
	}
	data.Stats.L3 = stats
	data.Stats.HasL3 = true
}
