// SPDX-License-Identifier: GPL-3.0-or-later

package topologyenrich

import (
	"sort"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"

	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
)

func ApplyL3Subnet(data *topologymodel.Data, aggregate topologymodel.ObservationAggregate) topologymodel.L3EnrichmentStats {
	var stats topologymodel.L3EnrichmentStats
	if data == nil || len(aggregate.L3Interfaces) == 0 {
		return finishTopologyL3SubnetEnrichment(data, stats)
	}

	adjacencies, subnetStats := buildTopologyL3SubnetAdjacencies(aggregate.L3Interfaces)
	stats.SubnetStats = subnetStats
	if len(adjacencies) == 0 {
		return finishTopologyL3SubnetEnrichment(data, stats)
	}

	resolver := newTopologyL3ActorResolver(data, aggregate.Snapshots)
	seen := existingTopologyL3LinkKeys(data.Links)
	for _, adjacency := range adjacencies {
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
		if _, exists := seen[key]; exists {
			stats.SuppressedDuplicateLink++
			continue
		}
		seen[key] = struct{}{}
		data.Links = append(data.Links, link)
		stats.EmittedLinks++
	}

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
				Source:  "ip_mib",
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

func existingTopologyL3LinkKeys(links []topologymodel.Link) map[string]struct{} {
	seen := make(map[string]struct{})
	for _, link := range links {
		if strings.EqualFold(strings.TrimSpace(topologyutil.FirstNonEmptyString(link.LinkType, link.Protocol)), topologymodel.L3SubnetLinkType) {
			seen[topologyL3SubnetLinkKey(link)] = struct{}{}
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

func recordTopologyL3EnrichmentStats(data *topologymodel.Data, stats topologymodel.L3EnrichmentStats) {
	if data == nil {
		return
	}
	data.Stats.L3 = stats
	data.Stats.HasL3 = true
}
