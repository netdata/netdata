// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"sort"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
)

const topologyL3SubnetLinkType = "l3_subnet"

type topologyL3EnrichmentStats struct {
	subnetStats               topologyL3SubnetBuildStats
	emittedLinks              int
	suppressedUnresolvedActor int
	suppressedSelfActor       int
	suppressedDuplicateLink   int
}

func applyTopologyL3SubnetEnrichment(data *topologyData, aggregate topologyObservationAggregate) topologyL3EnrichmentStats {
	var stats topologyL3EnrichmentStats
	if data == nil || len(aggregate.l3Interfaces) == 0 {
		return finishTopologyL3SubnetEnrichment(data, stats)
	}

	adjacencies, subnetStats := buildTopologyL3SubnetAdjacencies(aggregate.l3Interfaces)
	stats.subnetStats = subnetStats
	if len(adjacencies) == 0 {
		return finishTopologyL3SubnetEnrichment(data, stats)
	}

	resolver := newTopologyL3ActorResolver(data, aggregate.snapshots)
	seen := existingTopologyL3LinkKeys(data.Links)
	for _, adjacency := range adjacencies {
		srcRef, ok := resolver.resolve(adjacency.A)
		if !ok {
			stats.suppressedUnresolvedActor++
			continue
		}
		dstRef, ok := resolver.resolve(adjacency.B)
		if !ok {
			stats.suppressedUnresolvedActor++
			continue
		}
		if srcRef.actorID == dstRef.actorID {
			stats.suppressedSelfActor++
			continue
		}
		link := topologyL3SubnetLink(adjacency, srcRef, dstRef)
		key := topologyL3SubnetLinkKey(link)
		if _, exists := seen[key]; exists {
			stats.suppressedDuplicateLink++
			continue
		}
		seen[key] = struct{}{}
		data.Links = append(data.Links, link)
		stats.emittedLinks++
	}

	sort.Slice(data.Links, func(i, j int) bool {
		return topologyLinkSortKey(data.Links[i]) < topologyLinkSortKey(data.Links[j])
	})
	return finishTopologyL3SubnetEnrichment(data, stats)
}

func finishTopologyL3SubnetEnrichment(data *topologyData, stats topologyL3EnrichmentStats) topologyL3EnrichmentStats {
	recordTopologyL3EnrichmentStats(data, stats)
	recomputeTopologyLinkStats(data)
	return stats
}

func topologyL3SubnetLink(adjacency topologyL3SubnetAdjacency, srcRef, dstRef topologyL3ActorRef) topologyLink {
	return topologyLink{
		Layer:      "3",
		Protocol:   topologyL3SubnetLinkType,
		LinkType:   topologyL3SubnetLinkType,
		Direction:  "observed",
		SrcActorID: srcRef.actorID,
		DstActorID: dstRef.actorID,
		Src: topologyLinkEndpoint{
			Match:   srcRef.match,
			IfIndex: parseIndex(adjacency.A.IfIndex),
			IfName:  strings.TrimSpace(adjacency.A.IfName),
			IfDescr: strings.TrimSpace(adjacency.A.IfDescr),
		},
		Dst: topologyLinkEndpoint{
			Match:   dstRef.match,
			IfIndex: parseIndex(adjacency.B.IfIndex),
			IfName:  strings.TrimSpace(adjacency.B.IfName),
			IfDescr: strings.TrimSpace(adjacency.B.IfDescr),
		},
		Inference: &graph.LinkInference{
			Inference:      "shared_subnet",
			AttachmentMode: "logical_l3_subnet",
		},
		Detail: topologyLinkDetail{
			L3Subnet: &topologyL3SubnetLinkDetail{
				Source:  "ip_mib",
				SrcIP:   normalizeIPAddress(adjacency.A.IP),
				DstIP:   normalizeIPAddress(adjacency.B.IP),
				Subnet:  adjacency.Subnet,
				Network: adjacency.Network,
				Netmask: adjacency.Netmask,
				Prefix:  adjacency.Prefix,
			},
		},
	}
}

func existingTopologyL3LinkKeys(links []topologyLink) map[string]struct{} {
	seen := make(map[string]struct{})
	for _, link := range links {
		if strings.EqualFold(strings.TrimSpace(firstNonEmptyString(link.LinkType, link.Protocol)), topologyL3SubnetLinkType) {
			seen[topologyL3SubnetLinkKey(link)] = struct{}{}
		}
	}
	return seen
}

func topologyL3SubnetLinkKey(link topologyLink) string {
	src := strings.TrimSpace(link.SrcActorID)
	dst := strings.TrimSpace(link.DstActorID)
	if src > dst {
		src, dst = dst, src
	}
	return topologyL3SubnetLinkKeyParts(
		src,
		dst,
		topologyL3Subnet(link),
		strconv.Itoa(topologyL3SubnetPrefix(link)),
	)
}

func topologyL3Subnet(link topologyLink) string {
	if link.Detail.L3Subnet == nil {
		return ""
	}
	return strings.TrimSpace(link.Detail.L3Subnet.Subnet)
}

func topologyL3SubnetPrefix(link topologyLink) int {
	if link.Detail.L3Subnet == nil {
		return 0
	}
	return link.Detail.L3Subnet.Prefix
}

func topologyL3SubnetLinkKeyParts(parts ...string) string {
	var b strings.Builder
	for _, part := range parts {
		b.WriteString(strconv.Itoa(len(part)))
		b.WriteByte(':')
		b.WriteString(part)
	}
	return b.String()
}

func recordTopologyL3EnrichmentStats(data *topologyData, stats topologyL3EnrichmentStats) {
	if data == nil {
		return
	}
	data.Stats.L3 = stats
	data.Stats.HasL3 = true
}
