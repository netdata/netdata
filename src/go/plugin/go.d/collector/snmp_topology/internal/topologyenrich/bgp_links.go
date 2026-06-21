// SPDX-License-Identifier: GPL-3.0-or-later

package topologyenrich

import (
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"

	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
)

func ApplyBGPAdjacency(data *topologymodel.Data, aggregate topologymodel.ObservationAggregate) topologymodel.BGPEnrichmentStats {
	var stats topologymodel.BGPEnrichmentStats
	if data == nil || len(aggregate.BGPPeers) == 0 {
		return finishTopologyBGPAdjacencyEnrichment(data, stats)
	}

	resolver := newTopologyBGPActorResolver(data, aggregate)
	seen := existingTopologyBGPLinkKeys(data.Links)
	peerRowsByActor := make(map[string][]topologymodel.BGPPeerDetailRow)

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

func finishTopologyBGPAdjacencyEnrichment(data *topologymodel.Data, stats topologymodel.BGPEnrichmentStats) topologymodel.BGPEnrichmentStats {
	recordTopologyBGPEnrichmentStats(data, stats)
	topologymodel.RecomputeLinkStats(data)
	return stats
}

type topologyBGPActorResolver struct {
	l3           topologyL3ActorResolver
	byIdentifier map[string]topologyL3ActorRef
}

func newTopologyBGPActorResolver(data *topologymodel.Data, aggregate topologymodel.ObservationAggregate) topologyBGPActorResolver {
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

func (r topologyBGPActorResolver) resolveBGPPeer(row topologymodel.BGPPeer) (topologyL3ActorRef, bool) {
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

func topologyBGPAdjacencyLink(row topologymodel.BGPPeer, srcRef, dstRef topologyL3ActorRef) topologymodel.Link {
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

	return topologymodel.Link{
		Layer:      "3",
		Protocol:   topologymodel.BGPAdjacencyLinkType,
		LinkType:   topologymodel.BGPAdjacencyLinkType,
		Direction:  "observed",
		State:      "established",
		SrcActorID: srcRef.actorID,
		DstActorID: dstRef.actorID,
		Src: topologymodel.LinkEndpoint{
			Match: srcRef.match,
		},
		Dst: topologymodel.LinkEndpoint{
			Match: dstRef.match,
		},
		Inference: &graph.LinkInference{
			Inference:      "bgp_established_adjacency",
			AttachmentMode: "logical_l3_bgp",
		},
		Detail: topologymodel.LinkDetail{
			BGP: &topologymodel.BGPAdjacencyLinkDetail{
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

func topologyBGPPeerActorRow(row topologymodel.BGPPeer) topologymodel.BGPPeerDetailRow {
	return topologymodel.BGPPeerDetailRow{
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

func existingTopologyBGPLinkKeys(links []topologymodel.Link) map[string]struct{} {
	seen := make(map[string]struct{})
	for _, link := range links {
		if strings.EqualFold(strings.TrimSpace(topologyutil.FirstNonEmptyString(link.LinkType, link.Protocol)), topologymodel.BGPAdjacencyLinkType) {
			// Seed the key set so repeated enrichment over already-shaped data remains idempotent.
			row := topologymodel.BGPPeer{
				RoutingInstance: topologyBGPLinkRoutingInstance(link),
			}
			seen[topologyBGPPeerLinkKeyParts(row, link.SrcActorID, link.DstActorID)] = struct{}{}
		}
	}
	return seen
}

func topologyBGPPeerLinkKeyParts(row topologymodel.BGPPeer, srcActorID, dstActorID string) string {
	srcActorID = strings.TrimSpace(srcActorID)
	dstActorID = strings.TrimSpace(dstActorID)
	if srcActorID > dstActorID {
		srcActorID, dstActorID = dstActorID, srcActorID
	}

	return topologyutil.JoinKeyParts(
		srcActorID,
		dstActorID,
		topologyBGPRoutingInstanceValue(row.RoutingInstance),
	)
}

func topologyBGPRoutingInstanceValue(routingInstance string) string {
	return topologyutil.FirstNonEmptyString(routingInstance, "default")
}

func topologyBGPLinkRoutingInstance(link topologymodel.Link) string {
	if link.Detail.BGP == nil {
		return ""
	}
	return strings.TrimSpace(link.Detail.BGP.RoutingInstance)
}

func topologyBGPLocalIP(link topologymodel.Link) string {
	if link.Detail.BGP == nil {
		return ""
	}
	return strings.TrimSpace(link.Detail.BGP.LocalIP)
}

func topologyBGPNeighborIP(link topologymodel.Link) string {
	if link.Detail.BGP == nil {
		return ""
	}
	return strings.TrimSpace(link.Detail.BGP.NeighborIP)
}

func topologyBGPLocalAS(link topologymodel.Link) string {
	if link.Detail.BGP == nil {
		return ""
	}
	return strings.TrimSpace(link.Detail.BGP.LocalAS)
}

func topologyBGPRemoteAS(link topologymodel.Link) string {
	if link.Detail.BGP == nil {
		return ""
	}
	return strings.TrimSpace(link.Detail.BGP.RemoteAS)
}

func topologyBGPLocalIdentifier(link topologymodel.Link) string {
	if link.Detail.BGP == nil {
		return ""
	}
	return strings.TrimSpace(link.Detail.BGP.LocalIdentifier)
}

func topologyBGPPeerIdentifier(link topologymodel.Link) string {
	if link.Detail.BGP == nil {
		return ""
	}
	return strings.TrimSpace(link.Detail.BGP.PeerIdentifier)
}

func recordTopologyBGPEnrichmentStats(data *topologymodel.Data, stats topologymodel.BGPEnrichmentStats) {
	if data == nil {
		return
	}
	data.Stats.BGP = stats
	data.Stats.HasBGP = true
}
