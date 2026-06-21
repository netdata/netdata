// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	topologyengine "github.com/netdata/netdata/go/plugins/pkg/l2topology"
	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
)

const topologySchemaVersion = topologymodel.SchemaVersion

const (
	topologyL3SubnetLinkType      = topologymodel.L3SubnetLinkType
	topologyOSPFAdjacencyLinkType = topologymodel.OSPFAdjacencyLinkType
	topologyBGPAdjacencyLinkType  = topologymodel.BGPAdjacencyLinkType
)

type topologyMatch = topologymodel.Match
type topologyLinkEndpoint = topologymodel.LinkEndpoint
type topologyLink = topologymodel.Link
type topologyLinkDetail = topologymodel.LinkDetail
type topologyL3SubnetLinkDetail = topologymodel.L3SubnetLinkDetail
type topologyOSPFAdjacencyLinkDetail = topologymodel.OSPFAdjacencyLinkDetail
type topologyBGPAdjacencyLinkDetail = topologymodel.BGPAdjacencyLinkDetail
type topologyActor = topologymodel.Actor
type topologyActorDetail = topologymodel.ActorDetail
type topologySNMPActorDetail = topologymodel.SNMPActorDetail
type topologyData = topologymodel.Data
type topologyStats = topologymodel.Stats
type topologyShapeStats = topologymodel.ShapeStats
type topologyFocusStats = topologymodel.FocusStats
type topologyFocusDepth = topologymodel.FocusDepth
type topologyRecomputedStats = topologymodel.RecomputedStats
type topologyL3EnrichmentStats = topologymodel.L3EnrichmentStats
type topologyL3SubnetBuildStats = topologymodel.L3SubnetBuildStats
type topologyOSPFEnrichmentStats = topologymodel.OSPFEnrichmentStats
type topologyBGPEnrichmentStats = topologymodel.BGPEnrichmentStats
type topologyManagementAddress = topologymodel.ManagementAddress
type topologyInterfaceChartRef = topologymodel.InterfaceChartRef
type topologyDevice = topologymodel.Device
type topologyEndpoint = topologymodel.Endpoint
type topologyObservationSnapshot = topologymodel.ObservationSnapshot
type topologyObservationAggregate = topologymodel.ObservationAggregate
type topologyL3Interface = topologymodel.L3Interface
type topologyOSPFNeighbor = topologymodel.OSPFNeighbor
type topologyBGPPeer = topologymodel.BGPPeer
type topologyOSPFNeighborDetailRow = topologymodel.OSPFNeighborDetailRow
type topologyBGPPeerDetailRow = topologymodel.BGPPeerDetailRow

func topologyActorFromGraph(actor graph.Actor, detail topologyengine.ProjectionActorDetail) topologyActor {
	return topologyActor{
		ActorID:     actor.ActorID,
		ActorType:   actor.ActorType,
		Layer:       actor.Layer,
		Source:      actor.Source,
		Match:       actor.Match,
		ParentMatch: actor.ParentMatch,
		Labels:      actor.Labels,
		Detail:      topologyActorDetail{L2: detail},
	}
}

func topologyLinksFromGraph(links []graph.Link) []topologyLink {
	if len(links) == 0 {
		return nil
	}
	out := make([]topologyLink, len(links))
	for i, link := range links {
		out[i] = topologyLinkFromGraph(link)
	}
	return out
}

func topologyLinkFromGraph(link graph.Link) topologyLink {
	return topologyLink{
		Layer:        link.Layer,
		Protocol:     link.Protocol,
		LinkType:     link.LinkType,
		Direction:    link.Direction,
		State:        link.State,
		SrcActorID:   link.SrcActorID,
		DstActorID:   link.DstActorID,
		Src:          link.Src,
		Dst:          link.Dst,
		DiscoveredAt: link.DiscoveredAt,
		LastSeen:     link.LastSeen,
		Display:      link.Display,
		L2:           link.L2,
		Inference:    link.Inference,
	}
}
