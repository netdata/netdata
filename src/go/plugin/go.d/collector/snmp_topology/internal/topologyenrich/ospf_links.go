// SPDX-License-Identifier: GPL-3.0-or-later

package topologyenrich

import (
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"

	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
)

func ApplyOSPFAdjacency(data *topologymodel.Data, aggregate topologymodel.ObservationAggregate) topologymodel.OSPFEnrichmentStats {
	var stats topologymodel.OSPFEnrichmentStats
	if data == nil || len(aggregate.OSPFNeighbors) == 0 {
		return finishTopologyOSPFAdjacencyEnrichment(data, stats)
	}

	resolver := newTopologyL3ActorResolver(data, aggregate.Snapshots)
	seen := existingTopologyOSPFLinkKeys(data.Links)
	neighborRowsByActor := make(map[string][]topologymodel.OSPFNeighborDetailRow)

	for _, row := range aggregate.OSPFNeighbors {
		stats.ObservedRows++
		localRef, localOK := resolver.resolveDeviceID(row.DeviceID)
		remoteRef, remoteOK := resolver.resolveRouterEndpoint(row.NeighborRouterID, row.NeighborIP)
		if localOK {
			modalRow := topologyOSPFNeighborActorRow(row)
			if remoteOK {
				row.RemoteActorID = remoteRef.actorID
				modalRow.RemoteActorID = remoteRef.actorID
			}
			neighborRowsByActor[localRef.actorID] = append(neighborRowsByActor[localRef.actorID], modalRow)
			stats.AttachedNeighborRows++
		}

		if !isOSPFNeighborFull(row) {
			stats.SuppressedNonFullState++
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

		link := topologyOSPFAdjacencyLink(row, localRef, remoteRef)
		key := topologyOSPFNeighborLinkKeyParts(row, localRef.actorID, remoteRef.actorID)
		if _, exists := seen[key]; exists {
			stats.SuppressedDuplicateLink++
			continue
		}
		seen[key] = struct{}{}
		data.Links = append(data.Links, link)
		stats.EmittedLinks++
	}

	attachTopologyOSPFNeighborRows(data, neighborRowsByActor)
	sort.Slice(data.Links, func(i, j int) bool {
		return topologymodel.LinkSortKey(data.Links[i]) < topologymodel.LinkSortKey(data.Links[j])
	})
	return finishTopologyOSPFAdjacencyEnrichment(data, stats)
}

func finishTopologyOSPFAdjacencyEnrichment(data *topologymodel.Data, stats topologymodel.OSPFEnrichmentStats) topologymodel.OSPFEnrichmentStats {
	recordTopologyOSPFEnrichmentStats(data, stats)
	topologymodel.RecomputeLinkStats(data)
	return stats
}

func topologyOSPFAdjacencyLink(row topologymodel.OSPFNeighbor, srcRef, dstRef topologyL3ActorRef) topologymodel.Link {
	return topologymodel.Link{
		Layer:      "3",
		Protocol:   topologymodel.OSPFAdjacencyLinkType,
		LinkType:   topologymodel.OSPFAdjacencyLinkType,
		Direction:  "observed",
		State:      "full",
		SrcActorID: srcRef.actorID,
		DstActorID: dstRef.actorID,
		Src: topologymodel.LinkEndpoint{
			Match: srcRef.match,
		},
		Dst: topologymodel.LinkEndpoint{
			Match: dstRef.match,
		},
		Inference: &graph.LinkInference{
			Inference:      "ospf_full_adjacency",
			AttachmentMode: "logical_l3_ospf",
		},
		Detail: topologymodel.LinkDetail{
			OSPF: &topologymodel.OSPFAdjacencyLinkDetail{
				Source:           "ospf_mib",
				LocalRouterID:    topologyutil.NormalizeTopologyRouterID(row.LocalRouterID),
				NeighborRouterID: topologyutil.NormalizeTopologyRouterID(row.NeighborRouterID),
				LocalIP:          topologyutil.NormalizeNonUnspecifiedIPAddress(row.LocalIP),
				NeighborIP:       topologyutil.NormalizeNonUnspecifiedIPAddress(row.NeighborIP),
				AddresslessIndex: strings.TrimSpace(row.AddresslessIndex),
				Subnet:           strings.TrimSpace(row.Subnet),
				Network:          strings.TrimSpace(row.Network),
				Netmask:          strings.TrimSpace(row.Netmask),
				Prefix:           row.Prefix,
			},
		},
	}
}

func topologyOSPFNeighborActorRow(row topologymodel.OSPFNeighbor) topologymodel.OSPFNeighborDetailRow {
	return topologymodel.OSPFNeighborDetailRow{
		LocalRouterID:    topologyutil.NormalizeTopologyRouterID(row.LocalRouterID),
		NeighborRouterID: topologyutil.NormalizeTopologyRouterID(row.NeighborRouterID),
		NeighborIP:       topologyutil.NormalizeNonUnspecifiedIPAddress(row.NeighborIP),
		State:            topologyutil.NormalizeOSPFNeighborState(row.State),
		LocalIP:          topologyutil.NormalizeNonUnspecifiedIPAddress(row.LocalIP),
		Subnet:           row.Subnet,
		AddresslessIndex: row.AddresslessIndex,
		Source:           "ospf_mib",
	}
}

func sortTopologyOSPFNeighborDetailRows(rows []topologymodel.OSPFNeighborDetailRow) {
	sort.Slice(rows, func(i, j int) bool {
		return topologyOSPFNeighborActorRowSortKey(rows[i]) < topologyOSPFNeighborActorRowSortKey(rows[j])
	})
}

func topologyOSPFNeighborActorRowSortKey(row topologymodel.OSPFNeighborDetailRow) string {
	return strings.Join([]string{
		row.NeighborRouterID,
		topologyutil.NormalizeNonUnspecifiedIPAddress(row.NeighborIP),
		row.AddresslessIndex,
		row.State,
	}, "\x00")
}

func existingTopologyOSPFLinkKeys(links []topologymodel.Link) map[string]struct{} {
	seen := make(map[string]struct{})
	for _, link := range links {
		if strings.EqualFold(strings.TrimSpace(topologyutil.FirstNonEmptyString(link.LinkType, link.Protocol)), topologymodel.OSPFAdjacencyLinkType) {
			row := topologymodel.OSPFNeighbor{
				LocalRouterID:    topologyOSPFLocalRouterID(link),
				NeighborRouterID: topologyOSPFNeighborRouterID(link),
				LocalIP:          topologyOSPFLocalIP(link),
				NeighborIP:       topologyOSPFNeighborIP(link),
				AddresslessIndex: topologyOSPFAddresslessIndex(link),
				Subnet:           topologyOSPFSubnet(link),
				Prefix:           topologyOSPFPrefix(link),
			}
			seen[topologyOSPFNeighborLinkKeyParts(row, link.SrcActorID, link.DstActorID)] = struct{}{}
		}
	}
	return seen
}

func topologyOSPFLocalRouterID(link topologymodel.Link) string {
	if link.Detail.OSPF == nil {
		return ""
	}
	return strings.TrimSpace(link.Detail.OSPF.LocalRouterID)
}

func topologyOSPFNeighborRouterID(link topologymodel.Link) string {
	if link.Detail.OSPF == nil {
		return ""
	}
	return strings.TrimSpace(link.Detail.OSPF.NeighborRouterID)
}

func topologyOSPFLocalIP(link topologymodel.Link) string {
	if link.Detail.OSPF == nil {
		return ""
	}
	return strings.TrimSpace(link.Detail.OSPF.LocalIP)
}

func topologyOSPFNeighborIP(link topologymodel.Link) string {
	if link.Detail.OSPF == nil {
		return ""
	}
	return strings.TrimSpace(link.Detail.OSPF.NeighborIP)
}

func topologyOSPFAddresslessIndex(link topologymodel.Link) string {
	if link.Detail.OSPF == nil {
		return ""
	}
	return strings.TrimSpace(link.Detail.OSPF.AddresslessIndex)
}

func topologyOSPFSubnet(link topologymodel.Link) string {
	if link.Detail.OSPF == nil {
		return ""
	}
	return strings.TrimSpace(link.Detail.OSPF.Subnet)
}

func topologyOSPFPrefix(link topologymodel.Link) int {
	if link.Detail.OSPF == nil {
		return 0
	}
	return link.Detail.OSPF.Prefix
}

func recordTopologyOSPFEnrichmentStats(data *topologymodel.Data, stats topologymodel.OSPFEnrichmentStats) {
	if data == nil {
		return
	}
	data.Stats.OSPF = stats
	data.Stats.HasOSPF = true
}
