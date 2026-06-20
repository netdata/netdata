// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"sort"
	"strings"
)

const topologyOSPFAdjacencyLinkType = "ospf_adjacency"

type topologyOSPFEnrichmentStats struct {
	observedRows                 int
	emittedLinks                 int
	attachedNeighborRows         int
	suppressedNonFullState       int
	suppressedUnresolvedLocal    int
	suppressedUnresolvedNeighbor int
	suppressedSelfActor          int
	suppressedDuplicateLink      int
}

func applyTopologyOSPFAdjacencyEnrichment(data *topologyData, aggregate topologyObservationAggregate) topologyOSPFEnrichmentStats {
	var stats topologyOSPFEnrichmentStats
	if data == nil || len(aggregate.ospfNeighbors) == 0 {
		return finishTopologyOSPFAdjacencyEnrichment(data, stats)
	}

	resolver := newTopologyL3ActorResolver(data, aggregate.snapshots)
	seen := existingTopologyOSPFLinkKeys(data.Links)
	neighborRowsByActor := make(map[string][]map[string]any)

	for _, row := range aggregate.ospfNeighbors {
		stats.observedRows++
		localRef, localOK := resolver.resolveDeviceID(row.DeviceID)
		remoteRef, remoteOK := resolver.resolveRouterEndpoint(row.NeighborRouterID, row.NeighborIP)
		if localOK {
			modalRow := topologyOSPFNeighborActorRow(row)
			if remoteOK {
				row.RemoteActorID = remoteRef.actorID
				modalRow["remote_actor_id"] = remoteRef.actorID
			}
			neighborRowsByActor[localRef.actorID] = append(neighborRowsByActor[localRef.actorID], modalRow)
			stats.attachedNeighborRows++
		}

		if !isOSPFNeighborFull(row) {
			stats.suppressedNonFullState++
			continue
		}
		if !localOK {
			stats.suppressedUnresolvedLocal++
			continue
		}
		if !remoteOK {
			stats.suppressedUnresolvedNeighbor++
			continue
		}
		if localRef.actorID == remoteRef.actorID {
			stats.suppressedSelfActor++
			continue
		}

		link := topologyOSPFAdjacencyLink(row, localRef, remoteRef)
		key := topologyOSPFNeighborLinkKeyParts(row, localRef.actorID, remoteRef.actorID)
		if _, exists := seen[key]; exists {
			stats.suppressedDuplicateLink++
			continue
		}
		seen[key] = struct{}{}
		data.Links = append(data.Links, link)
		stats.emittedLinks++
	}

	attachTopologyActorTableRows(data, "ospf_neighbors", neighborRowsByActor, sortTopologyOSPFNeighborRows)
	sort.Slice(data.Links, func(i, j int) bool {
		return topologyLinkSortKey(data.Links[i]) < topologyLinkSortKey(data.Links[j])
	})
	return finishTopologyOSPFAdjacencyEnrichment(data, stats)
}

func finishTopologyOSPFAdjacencyEnrichment(data *topologyData, stats topologyOSPFEnrichmentStats) topologyOSPFEnrichmentStats {
	recordTopologyOSPFEnrichmentStats(data, stats)
	recomputeTopologyLinkStats(data)
	return stats
}

func topologyOSPFAdjacencyLink(row topologyOSPFNeighbor, srcRef, dstRef topologyL3ActorRef) topologyLink {
	return topologyLink{
		Layer:      "3",
		Protocol:   topologyOSPFAdjacencyLinkType,
		LinkType:   topologyOSPFAdjacencyLinkType,
		Direction:  "observed",
		State:      "full",
		SrcActorID: srcRef.actorID,
		DstActorID: dstRef.actorID,
		Src: topologyLinkEndpoint{
			Match:      srcRef.match,
			Attributes: topologyOSPFEndpointAttributes(row.LocalRouterID, row.LocalIP, row),
		},
		Dst: topologyLinkEndpoint{
			Match:      dstRef.match,
			Attributes: topologyOSPFEndpointAttributes(row.NeighborRouterID, row.NeighborIP, row),
		},
		Metrics: topologyOSPFLinkMetrics(row),
	}
}

func topologyOSPFEndpointAttributes(routerID, ip string, row topologyOSPFNeighbor) map[string]any {
	attrs := map[string]any{
		"source": "ospf_mib",
	}
	if normalizedRouterID := normalizeTopologyRouterID(routerID); normalizedRouterID != "" {
		attrs["router_id"] = normalizedRouterID
	}
	if normalizedIP := normalizeNonUnspecifiedIPAddress(ip); normalizedIP != "" {
		attrs["ip"] = normalizedIP
	}
	if _, subnet, prefix, ok := topologyOSPFSubnetMatch(row); ok {
		attrs["subnet"] = subnet
		attrs["prefix"] = prefix
	}
	return attrs
}

func topologyOSPFLinkMetrics(row topologyOSPFNeighbor) map[string]any {
	metrics := map[string]any{
		"source":          "ospf_mib",
		"inference":       "ospf_full_adjacency",
		"attachment_mode": "logical_l3_ospf",
		"state":           "full",
	}
	if routerID := normalizeTopologyRouterID(row.LocalRouterID); routerID != "" {
		metrics["local_router_id"] = routerID
	}
	if routerID := normalizeTopologyRouterID(row.NeighborRouterID); routerID != "" {
		metrics["neighbor_router_id"] = routerID
	}
	if ip := normalizeNonUnspecifiedIPAddress(row.LocalIP); ip != "" {
		metrics["local_ip"] = ip
	}
	if ip := normalizeNonUnspecifiedIPAddress(row.NeighborIP); ip != "" {
		metrics["neighbor_ip"] = ip
	}
	if row.AddresslessIndex != "" {
		metrics["addressless_index"] = row.AddresslessIndex
	}
	if row.Subnet != "" {
		metrics["subnet"] = row.Subnet
		metrics["network"] = row.Network
		metrics["netmask"] = row.Netmask
		metrics["prefix"] = row.Prefix
	}
	return metrics
}

func topologyOSPFNeighborActorRow(row topologyOSPFNeighbor) map[string]any {
	out := map[string]any{
		"state":  normalizeOSPFNeighborState(row.State),
		"source": "ospf_mib",
	}
	if routerID := normalizeTopologyRouterID(row.LocalRouterID); routerID != "" {
		out["local_router_id"] = routerID
	}
	if routerID := normalizeTopologyRouterID(row.NeighborRouterID); routerID != "" {
		out["neighbor_router_id"] = routerID
	}
	if ip := normalizeNonUnspecifiedIPAddress(row.NeighborIP); ip != "" {
		out["neighbor_ip"] = ip
	}
	if ip := normalizeNonUnspecifiedIPAddress(row.LocalIP); ip != "" {
		out["local_ip"] = ip
	}
	if row.AddresslessIndex != "" {
		out["addressless_index"] = row.AddresslessIndex
	}
	if row.Subnet != "" {
		out["subnet"] = row.Subnet
	}
	return out
}

func sortTopologyOSPFNeighborRows(rows []map[string]any) {
	sort.Slice(rows, func(i, j int) bool {
		return topologyOSPFNeighborActorRowSortKey(rows[i]) < topologyOSPFNeighborActorRowSortKey(rows[j])
	})
}

func topologyOSPFNeighborActorRowSortKey(row map[string]any) string {
	return strings.Join([]string{
		anyStringValue(row["neighbor_router_id"]),
		normalizeNonUnspecifiedIPAddress(anyStringValue(row["neighbor_ip"])),
		anyStringValue(row["addressless_index"]),
		anyStringValue(row["state"]),
	}, "\x00")
}

func existingTopologyOSPFLinkKeys(links []topologyLink) map[string]struct{} {
	seen := make(map[string]struct{})
	for _, link := range links {
		if strings.EqualFold(strings.TrimSpace(firstNonEmptyString(link.LinkType, link.Protocol)), topologyOSPFAdjacencyLinkType) {
			row := topologyOSPFNeighbor{
				LocalRouterID:    topologyV1EndpointString(link.Src, "router_id"),
				NeighborRouterID: topologyV1EndpointString(link.Dst, "router_id"),
				LocalIP:          topologyV1EndpointString(link.Src, "ip"),
				NeighborIP:       topologyV1EndpointString(link.Dst, "ip"),
				AddresslessIndex: topologyMetricValueString(link.Metrics, "addressless_index"),
				Subnet:           topologyMetricValueString(link.Metrics, "subnet"),
			}
			if prefix, ok := uintValue(link.Metrics["prefix"]); ok {
				row.Prefix = int(prefix)
			}
			seen[topologyOSPFNeighborLinkKeyParts(row, link.SrcActorID, link.DstActorID)] = struct{}{}
		}
	}
	return seen
}

func recordTopologyOSPFEnrichmentStats(data *topologyData, stats topologyOSPFEnrichmentStats) {
	if data == nil {
		return
	}
	data.Stats.OSPF = stats
	data.Stats.HasOSPF = true
}
