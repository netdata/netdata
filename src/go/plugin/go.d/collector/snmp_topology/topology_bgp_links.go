// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
)

func applyTopologyBGPAdjacencyEnrichment(data *topologyData, aggregate topologyObservationAggregate) topologyBGPEnrichmentStats {
	var stats topologyBGPEnrichmentStats
	if data == nil || len(aggregate.BGPPeers) == 0 {
		return finishTopologyBGPAdjacencyEnrichment(data, stats)
	}

	resolver := newTopologyBGPActorResolver(data, aggregate)
	seen := existingTopologyBGPLinkKeys(data.Links)
	peerRowsByActor := make(map[string][]topologyBGPPeerDetailRow)

	for _, row := range aggregate.BGPPeers {
		stats.ObservedRows++
		localRef, localOK := resolver.resolveDeviceID(row.DeviceID)
		remoteRef, remoteOK := resolver.resolveBGPPeer(row)
		if localOK {
			modalRow := topologyBGPPeerActorRow(row)
			if remoteOK {
				modalRow.RemoteActorID = remoteRef.actorID
			}
			peerRowsByActor[localRef.actorID] = append(peerRowsByActor[localRef.actorID], modalRow)
			stats.AttachedPeerRows++
		}

		if !isBGPPeerEstablished(row) {
			stats.SuppressedNonEstablished++
			continue
		}
		if !localOK {
			stats.SuppressedUnresolvedLocal++
			continue
		}
		if !remoteOK {
			stats.SuppressedUnresolvedNeighbor++
			continue
		}
		if localRef.actorID == remoteRef.actorID {
			stats.SuppressedSelfActor++
			continue
		}

		key := topologyBGPPeerLinkKeyParts(row, localRef.actorID, remoteRef.actorID)
		if _, exists := seen[key]; exists {
			stats.SuppressedDuplicateLink++
			continue
		}
		seen[key] = struct{}{}
		data.Links = append(data.Links, topologyBGPAdjacencyLink(row, localRef, remoteRef))
		stats.EmittedLinks++
	}

	attachTopologyBGPPeerRows(data, peerRowsByActor)
	sort.Slice(data.Links, func(i, j int) bool {
		return topologymodel.LinkSortKey(data.Links[i]) < topologymodel.LinkSortKey(data.Links[j])
	})
	return finishTopologyBGPAdjacencyEnrichment(data, stats)
}

func finishTopologyBGPAdjacencyEnrichment(data *topologyData, stats topologyBGPEnrichmentStats) topologyBGPEnrichmentStats {
	recordTopologyBGPEnrichmentStats(data, stats)
	topologymodel.RecomputeLinkStats(data)
	return stats
}

type topologyBGPActorResolver struct {
	l3           topologyL3ActorResolver
	byIdentifier map[string]topologyL3ActorRef
}

func newTopologyBGPActorResolver(data *topologyData, aggregate topologyObservationAggregate) topologyBGPActorResolver {
	resolver := topologyBGPActorResolver{
		l3:           newTopologyL3ActorResolver(data, aggregate.Snapshots),
		byIdentifier: make(map[string]topologyL3ActorRef),
	}
	for _, row := range aggregate.L3Interfaces {
		ref, ok := resolver.l3.resolveDeviceID(row.DeviceID)
		if ok {
			resolver.l3.addUniqueIPAddress(row.IP, ref)
		}
	}
	for _, row := range aggregate.BGPPeers {
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
	if ref, ok := r.byIdentifier[topologyutil.NormalizeBGPRouterID(row.PeerIdentifier)]; ok && ref.actorID != "" {
		return ref, true
	}
	return r.l3.resolveRouterEndpoint(row.PeerIdentifier, row.NeighborIP)
}

func (r topologyBGPActorResolver) addUniqueIdentifier(identifier string, ref topologyL3ActorRef) {
	identifier = topologyutil.NormalizeBGPRouterID(identifier)
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
			Match: srcRef.match,
		},
		Dst: topologyLinkEndpoint{
			Match: dstRef.match,
		},
		Inference: &graph.LinkInference{
			Inference:      "bgp_established_adjacency",
			AttachmentMode: "logical_l3_bgp",
		},
		Detail: topologyLinkDetail{
			BGP: &topologyBGPAdjacencyLinkDetail{
				Source:          "bgp_mib",
				RoutingInstance: topologyBGPRoutingInstanceValue(row.RoutingInstance),
				LocalIP:         topologyutil.NormalizeNonUnspecifiedIPAddress(src.ip),
				NeighborIP:      topologyutil.NormalizeNonUnspecifiedIPAddress(dst.ip),
				LocalAS:         strings.TrimSpace(src.asn),
				RemoteAS:        strings.TrimSpace(dst.asn),
				LocalIdentifier: topologyutil.NormalizeBGPRouterID(src.identifier),
				PeerIdentifier:  topologyutil.NormalizeBGPRouterID(dst.identifier),
			},
		},
	}
}

func topologyBGPPeerActorRow(row topologyBGPPeer) topologyBGPPeerDetailRow {
	return topologyBGPPeerDetailRow{
		RoutingInstance:       row.RoutingInstance,
		NeighborIP:            strings.TrimSpace(row.NeighborIP),
		RemoteAS:              strings.TrimSpace(row.RemoteAS),
		State:                 strings.TrimSpace(row.State),
		AdminStatus:           strings.TrimSpace(row.AdminStatus),
		LocalIP:               strings.TrimSpace(row.LocalIP),
		LocalAS:               strings.TrimSpace(row.LocalAS),
		LocalIdentifier:       strings.TrimSpace(row.LocalIdentifier),
		PeerIdentifier:        strings.TrimSpace(row.PeerIdentifier),
		PeerType:              strings.TrimSpace(row.PeerType),
		BGPVersion:            strings.TrimSpace(row.BGPVersion),
		Description:           strings.TrimSpace(row.Description),
		EstablishedUptime:     row.EstablishedUptime,
		LastReceivedUpdateAge: row.LastReceivedUpdateAge,
		Source:                "bgp_mib",
	}
}

func existingTopologyBGPLinkKeys(links []topologyLink) map[string]struct{} {
	seen := make(map[string]struct{})
	for _, link := range links {
		if strings.EqualFold(strings.TrimSpace(topologyutil.FirstNonEmptyString(link.LinkType, link.Protocol)), topologyBGPAdjacencyLinkType) {
			// Seed the key set so repeated enrichment over already-shaped data remains idempotent.
			row := topologyBGPPeer{
				RoutingInstance: topologyBGPLinkRoutingInstance(link),
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
	return topologyutil.FirstNonEmptyString(routingInstance, "default")
}

func topologyBGPLinkRoutingInstance(link topologyLink) string {
	if link.Detail.BGP == nil {
		return ""
	}
	return strings.TrimSpace(link.Detail.BGP.RoutingInstance)
}

func topologyBGPLocalIP(link topologyLink) string {
	if link.Detail.BGP == nil {
		return ""
	}
	return strings.TrimSpace(link.Detail.BGP.LocalIP)
}

func topologyBGPNeighborIP(link topologyLink) string {
	if link.Detail.BGP == nil {
		return ""
	}
	return strings.TrimSpace(link.Detail.BGP.NeighborIP)
}

func topologyBGPLocalAS(link topologyLink) string {
	if link.Detail.BGP == nil {
		return ""
	}
	return strings.TrimSpace(link.Detail.BGP.LocalAS)
}

func topologyBGPRemoteAS(link topologyLink) string {
	if link.Detail.BGP == nil {
		return ""
	}
	return strings.TrimSpace(link.Detail.BGP.RemoteAS)
}

func topologyBGPLocalIdentifier(link topologyLink) string {
	if link.Detail.BGP == nil {
		return ""
	}
	return strings.TrimSpace(link.Detail.BGP.LocalIdentifier)
}

func topologyBGPPeerIdentifier(link topologyLink) string {
	if link.Detail.BGP == nil {
		return ""
	}
	return strings.TrimSpace(link.Detail.BGP.PeerIdentifier)
}

func recordTopologyBGPEnrichmentStats(data *topologyData, stats topologyBGPEnrichmentStats) {
	if data == nil {
		return
	}
	data.Stats.BGP = stats
	data.Stats.HasBGP = true
}
