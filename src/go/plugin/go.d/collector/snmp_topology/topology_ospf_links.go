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
	suppressedL3SubnetOverlap    int
}

func applyTopologyOSPFAdjacencyEnrichment(data *topologyData, aggregate topologyObservationAggregate) topologyOSPFEnrichmentStats {
	var stats topologyOSPFEnrichmentStats
	if data == nil || len(aggregate.ospfNeighbors) == 0 {
		recordTopologyOSPFEnrichmentStats(data, stats)
		return stats
	}

	resolver := newTopologyL3ActorResolver(data, aggregate.snapshots)
	seen := existingTopologyOSPFLinkKeys(data.Links)
	neighborRowsByActor := make(map[string][]map[string]any)

	for _, row := range aggregate.ospfNeighbors {
		stats.observedRows++
		localRef, localOK := resolver.resolveDeviceID(row.DeviceID)
		remoteRef, remoteOK := resolver.resolveOSPFNeighbor(row)
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
		stats.suppressedL3SubnetOverlap += suppressMatchingTopologyL3SubnetLinks(data, link)
		data.Links = append(data.Links, link)
		stats.emittedLinks++
	}

	attachTopologyOSPFNeighborRows(data, neighborRowsByActor)
	sort.Slice(data.Links, func(i, j int) bool {
		return topologyLinkSortKey(data.Links[i]) < topologyLinkSortKey(data.Links[j])
	})
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
	if normalizedRouterID := normalizeOSPFRouterID(routerID); normalizedRouterID != "" {
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
	if routerID := normalizeOSPFRouterID(row.LocalRouterID); routerID != "" {
		metrics["local_router_id"] = routerID
	}
	if routerID := normalizeOSPFRouterID(row.NeighborRouterID); routerID != "" {
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
	if routerID := normalizeOSPFRouterID(row.LocalRouterID); routerID != "" {
		out["local_router_id"] = routerID
	}
	if routerID := normalizeOSPFRouterID(row.NeighborRouterID); routerID != "" {
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

func attachTopologyOSPFNeighborRows(data *topologyData, rowsByActor map[string][]map[string]any) {
	if data == nil || len(rowsByActor) == 0 {
		return
	}
	for i := range data.Actors {
		actor := &data.Actors[i]
		rows := rowsByActor[strings.TrimSpace(actor.ActorID)]
		if len(rows) == 0 {
			continue
		}
		sort.Slice(rows, func(i, j int) bool {
			return topologyOSPFNeighborActorRowSortKey(rows[i]) < topologyOSPFNeighborActorRowSortKey(rows[j])
		})
		if actor.Tables == nil {
			actor.Tables = make(map[string][]map[string]any)
		}
		actor.Tables["ospf_neighbors"] = rows
	}
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

func suppressMatchingTopologyL3SubnetLinks(data *topologyData, ospfLink topologyLink) int {
	if data == nil || len(data.Links) == 0 {
		return 0
	}

	dst := data.Links[:0]
	removed := 0
	for _, link := range data.Links {
		if topologyLinkMatchesOSPFAdjacencyL3Subnet(link, ospfLink) {
			removed++
			continue
		}
		dst = append(dst, link)
	}
	data.Links = dst
	return removed
}

func topologyLinkMatchesOSPFAdjacencyL3Subnet(link, ospfLink topologyLink) bool {
	if !strings.EqualFold(strings.TrimSpace(firstNonEmptyString(link.LinkType, link.Protocol)), topologyL3SubnetLinkType) {
		return false
	}
	if !sameTopologyActorPair(link, ospfLink) {
		return false
	}
	ospfSubnet := topologyMetricValueString(ospfLink.Metrics, "subnet")
	if ospfSubnet != "" && ospfSubnet == topologyMetricValueString(link.Metrics, "subnet") {
		return true
	}
	return sameTopologyEndpointIPPair(
		topologyV1EndpointString(link.Src, "ip"),
		topologyV1EndpointString(link.Dst, "ip"),
		topologyV1EndpointString(ospfLink.Src, "ip"),
		topologyV1EndpointString(ospfLink.Dst, "ip"),
	)
}

func sameTopologyActorPair(a, b topologyLink) bool {
	aSrc, aDst := strings.TrimSpace(a.SrcActorID), strings.TrimSpace(a.DstActorID)
	bSrc, bDst := strings.TrimSpace(b.SrcActorID), strings.TrimSpace(b.DstActorID)
	return (aSrc == bSrc && aDst == bDst) || (aSrc == bDst && aDst == bSrc)
}

func sameTopologyEndpointIPPair(aSrc, aDst, bSrc, bDst string) bool {
	aSrc, aDst = normalizeNonUnspecifiedIPAddress(aSrc), normalizeNonUnspecifiedIPAddress(aDst)
	bSrc, bDst = normalizeNonUnspecifiedIPAddress(bSrc), normalizeNonUnspecifiedIPAddress(bDst)
	if aSrc == "" || aDst == "" || bSrc == "" || bDst == "" {
		return false
	}
	return (aSrc == bSrc && aDst == bDst) || (aSrc == bDst && aDst == bSrc)
}

func recordTopologyOSPFEnrichmentStats(data *topologyData, stats topologyOSPFEnrichmentStats) {
	if data == nil {
		return
	}
	if data.Stats == nil {
		data.Stats = make(map[string]any)
	}
	data.Stats["ospf_neighbor_rows"] = stats.observedRows
	data.Stats["ospf_neighbor_detail_rows"] = stats.attachedNeighborRows
	data.Stats["ospf_adjacency_emitted_links"] = stats.emittedLinks
	data.Stats["ospf_adjacency_suppressed_non_full_state"] = stats.suppressedNonFullState
	data.Stats["ospf_adjacency_suppressed_unresolved_local"] = stats.suppressedUnresolvedLocal
	data.Stats["ospf_adjacency_suppressed_unresolved_neighbor"] = stats.suppressedUnresolvedNeighbor
	data.Stats["ospf_adjacency_suppressed_self_actor"] = stats.suppressedSelfActor
	data.Stats["ospf_adjacency_suppressed_duplicate_link"] = stats.suppressedDuplicateLink
	data.Stats["ospf_adjacency_suppressed_l3_subnet_overlap"] = stats.suppressedL3SubnetOverlap
	recomputeTopologyOSPFVisibleLinkStats(data)
}

func recomputeTopologyOSPFVisibleLinkStats(data *topologyData) {
	if data == nil || data.Stats == nil {
		return
	}
	if _, ok := data.Stats["ospf_adjacency_emitted_links"]; !ok {
		return
	}
	count := 0
	for _, link := range data.Links {
		if strings.EqualFold(strings.TrimSpace(firstNonEmptyString(link.LinkType, link.Protocol)), topologyOSPFAdjacencyLinkType) {
			count++
		}
	}
	data.Stats["ospf_adjacency_visible_links"] = count
}
