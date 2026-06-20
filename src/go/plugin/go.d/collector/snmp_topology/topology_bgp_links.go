// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"sort"
	"strings"
)

const topologyBGPAdjacencyLinkType = "bgp_adjacency"

type topologyBGPEnrichmentStats struct {
	observedRows                 int
	emittedLinks                 int
	attachedPeerRows             int
	suppressedNonEstablished     int
	suppressedUnresolvedLocal    int
	suppressedUnresolvedNeighbor int
	suppressedSelfActor          int
	suppressedDuplicateLink      int
}

func applyTopologyBGPAdjacencyEnrichment(data *topologyData, aggregate topologyObservationAggregate) topologyBGPEnrichmentStats {
	var stats topologyBGPEnrichmentStats
	if data == nil || len(aggregate.bgpPeers) == 0 {
		return finishTopologyBGPAdjacencyEnrichment(data, stats)
	}

	resolver := newTopologyBGPActorResolver(data, aggregate)
	seen := existingTopologyBGPLinkKeys(data.Links)
	peerRowsByActor := make(map[string][]map[string]any)

	for _, row := range aggregate.bgpPeers {
		stats.observedRows++
		localRef, localOK := resolver.resolveDeviceID(row.DeviceID)
		remoteRef, remoteOK := resolver.resolveBGPPeer(row)
		if localOK {
			modalRow := topologyBGPPeerActorRow(row)
			if remoteOK {
				modalRow["remote_actor_id"] = remoteRef.actorID
			}
			peerRowsByActor[localRef.actorID] = append(peerRowsByActor[localRef.actorID], modalRow)
			stats.attachedPeerRows++
		}

		if !isBGPPeerEstablished(row) {
			stats.suppressedNonEstablished++
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

		key := topologyBGPPeerLinkKeyParts(row, localRef.actorID, remoteRef.actorID)
		if _, exists := seen[key]; exists {
			stats.suppressedDuplicateLink++
			continue
		}
		seen[key] = struct{}{}
		data.Links = append(data.Links, topologyBGPAdjacencyLink(row, localRef, remoteRef))
		stats.emittedLinks++
	}

	attachTopologyActorTableRows(data, "bgp_peers", peerRowsByActor, sortTopologyBGPPeerRows)
	sort.Slice(data.Links, func(i, j int) bool {
		return topologyLinkSortKey(data.Links[i]) < topologyLinkSortKey(data.Links[j])
	})
	return finishTopologyBGPAdjacencyEnrichment(data, stats)
}

func finishTopologyBGPAdjacencyEnrichment(data *topologyData, stats topologyBGPEnrichmentStats) topologyBGPEnrichmentStats {
	recordTopologyBGPEnrichmentStats(data, stats)
	recomputeTopologyLinkStats(data)
	return stats
}

type topologyBGPActorResolver struct {
	l3           topologyL3ActorResolver
	byIdentifier map[string]topologyL3ActorRef
}

func newTopologyBGPActorResolver(data *topologyData, aggregate topologyObservationAggregate) topologyBGPActorResolver {
	resolver := topologyBGPActorResolver{
		l3:           newTopologyL3ActorResolver(data, aggregate.snapshots),
		byIdentifier: make(map[string]topologyL3ActorRef),
	}
	for _, row := range aggregate.l3Interfaces {
		ref, ok := resolver.l3.resolveDeviceID(row.DeviceID)
		if ok {
			resolver.l3.addUniqueIPAddress(row.IP, ref)
		}
	}
	for _, row := range aggregate.bgpPeers {
		ref, ok := resolver.l3.resolveDeviceID(row.DeviceID)
		if !ok {
			continue
		}
		resolver.addUniqueIdentifier(row.LocalIdentifier, ref)
		resolver.l3.addUniqueIPAddress(row.LocalIP, ref)
	}
	return resolver
}

func (r topologyBGPActorResolver) resolveDeviceID(deviceID string) (topologyL3ActorRef, bool) {
	return r.l3.resolveDeviceID(deviceID)
}

func (r topologyBGPActorResolver) resolveBGPPeer(row topologyBGPPeer) (topologyL3ActorRef, bool) {
	if ref, ok := r.byIdentifier[normalizeBGPRouterID(row.PeerIdentifier)]; ok && ref.actorID != "" {
		return ref, true
	}
	return r.l3.resolveRouterEndpoint(row.PeerIdentifier, row.NeighborIP)
}

func (r topologyBGPActorResolver) addUniqueIdentifier(identifier string, ref topologyL3ActorRef) {
	identifier = normalizeBGPRouterID(identifier)
	if identifier == "" || ref.actorID == "" {
		return
	}
	existing, ok := r.byIdentifier[identifier]
	if !ok {
		r.byIdentifier[identifier] = ref
		return
	}
	if existing.actorID != "" && existing.actorID != ref.actorID {
		r.byIdentifier[identifier] = topologyL3ActorRef{}
	}
}

type topologyBGPEndpoint struct {
	identifier string
	ip         string
	asn        string
}

func topologyBGPAdjacencyLink(row topologyBGPPeer, srcRef, dstRef topologyL3ActorRef) topologyLink {
	src := topologyBGPEndpoint{
		identifier: row.LocalIdentifier,
		ip:         row.LocalIP,
		asn:        row.LocalAS,
	}
	dst := topologyBGPEndpoint{
		identifier: row.PeerIdentifier,
		ip:         row.NeighborIP,
		asn:        row.RemoteAS,
	}
	if srcRef.actorID > dstRef.actorID {
		srcRef, dstRef = dstRef, srcRef
		src, dst = dst, src
	}

	return topologyLink{
		Layer:      "3",
		Protocol:   topologyBGPAdjacencyLinkType,
		LinkType:   topologyBGPAdjacencyLinkType,
		Direction:  "observed",
		State:      "established",
		SrcActorID: srcRef.actorID,
		DstActorID: dstRef.actorID,
		Src: topologyLinkEndpoint{
			Match:      srcRef.match,
			Attributes: topologyBGPEndpointAttributes(src),
		},
		Dst: topologyLinkEndpoint{
			Match:      dstRef.match,
			Attributes: topologyBGPEndpointAttributes(dst),
		},
		Metrics: topologyBGPLinkMetrics(row, src, dst),
	}
}

func topologyBGPEndpointAttributes(endpoint topologyBGPEndpoint) map[string]any {
	attrs := map[string]any{
		"source": "bgp_mib",
	}
	if identifier := normalizeBGPRouterID(endpoint.identifier); identifier != "" {
		attrs["bgp_identifier"] = identifier
	}
	if ip := normalizeNonUnspecifiedIPAddress(endpoint.ip); ip != "" {
		attrs["ip"] = ip
	}
	if asn := strings.TrimSpace(endpoint.asn); asn != "" {
		attrs["as"] = asn
	}
	return attrs
}

func topologyBGPLinkMetrics(row topologyBGPPeer, src, dst topologyBGPEndpoint) map[string]any {
	metrics := map[string]any{
		"source":           "bgp_mib",
		"inference":        "bgp_established_adjacency",
		"attachment_mode":  "logical_l3_bgp",
		"state":            "established",
		"routing_instance": row.RoutingInstance,
	}
	addStringMetric := func(key, value string) {
		if value = strings.TrimSpace(value); value != "" {
			metrics[key] = value
		}
	}
	addStringMetric("local_ip", normalizeNonUnspecifiedIPAddress(src.ip))
	addStringMetric("neighbor_ip", normalizeNonUnspecifiedIPAddress(dst.ip))
	addStringMetric("local_as", src.asn)
	addStringMetric("remote_as", dst.asn)
	addStringMetric("local_identifier", normalizeBGPRouterID(src.identifier))
	addStringMetric("peer_identifier", normalizeBGPRouterID(dst.identifier))
	addStringMetric("peer_type", row.PeerType)
	addStringMetric("bgp_version", row.BGPVersion)
	return metrics
}

func topologyBGPPeerActorRow(row topologyBGPPeer) map[string]any {
	out := map[string]any{
		"routing_instance": row.RoutingInstance,
		"remote_as":        row.RemoteAS,
		"state":            row.State,
		"source":           "bgp_mib",
	}
	add := func(key, value string) {
		if value = strings.TrimSpace(value); value != "" {
			out[key] = value
		}
	}
	add("neighbor_ip", row.NeighborIP)
	add("local_ip", row.LocalIP)
	add("local_as", row.LocalAS)
	add("local_identifier", row.LocalIdentifier)
	add("peer_identifier", row.PeerIdentifier)
	add("peer_type", row.PeerType)
	add("bgp_version", row.BGPVersion)
	add("description", row.Description)
	add("admin_status", row.AdminStatus)
	if row.EstablishedUptime != nil {
		out["established_uptime"] = *row.EstablishedUptime
	}
	if row.LastReceivedUpdateAge != nil {
		out["last_received_update_age"] = *row.LastReceivedUpdateAge
	}
	return out
}

func existingTopologyBGPLinkKeys(links []topologyLink) map[string]struct{} {
	seen := make(map[string]struct{})
	for _, link := range links {
		if strings.EqualFold(strings.TrimSpace(firstNonEmptyString(link.LinkType, link.Protocol)), topologyBGPAdjacencyLinkType) {
			// Seed the key set so repeated enrichment over already-shaped data remains idempotent.
			row := topologyBGPPeer{
				RoutingInstance: topologyMetricValueString(link.Metrics, "routing_instance"),
			}
			seen[topologyBGPPeerLinkKeyParts(row, link.SrcActorID, link.DstActorID)] = struct{}{}
		}
	}
	return seen
}

func topologyBGPPeerLinkKeyParts(row topologyBGPPeer, srcActorID, dstActorID string) string {
	srcActorID = strings.TrimSpace(srcActorID)
	dstActorID = strings.TrimSpace(dstActorID)
	if srcActorID > dstActorID {
		srcActorID, dstActorID = dstActorID, srcActorID
	}

	return topologyL3SubnetLinkKeyParts(
		srcActorID,
		dstActorID,
		topologyBGPRoutingInstanceValue(row.RoutingInstance),
	)
}

func topologyBGPRoutingInstanceValue(routingInstance string) string {
	return firstNonEmptyString(routingInstance, "default")
}

func recordTopologyBGPEnrichmentStats(data *topologyData, stats topologyBGPEnrichmentStats) {
	if data == nil {
		return
	}
	data.Stats.BGP = stats
	data.Stats.HasBGP = true
}
