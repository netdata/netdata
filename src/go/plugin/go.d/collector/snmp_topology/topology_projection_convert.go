// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	topologyengine "github.com/netdata/netdata/go/plugins/pkg/l2topology"
	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
)

func topologyActorFromGraph(actor graph.Actor, detail topologyengine.ProjectionActorDetail) topologymodel.Actor {
	return topologymodel.Actor{
		ActorID:     actor.ActorID,
		ActorType:   actor.ActorType,
		Layer:       actor.Layer,
		Source:      actor.Source,
		Match:       actor.Match,
		ParentMatch: actor.ParentMatch,
		Labels:      actor.Labels,
		Detail:      topologymodel.ActorDetail{L2: detail},
	}
}

func topologyLinksFromGraph(links []graph.Link) []topologymodel.Link {
	if len(links) == 0 {
		return nil
	}
	out := make([]topologymodel.Link, len(links))
	for i, link := range links {
		out[i] = topologyLinkFromGraph(link)
	}
	return out
}

func topologyLinkFromGraph(link graph.Link) topologymodel.Link {
	return topologymodel.Link{
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
