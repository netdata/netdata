// SPDX-License-Identifier: GPL-3.0-or-later

package topologyshape

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
)

type deltaActorRef uint64

type topologyLinkDeltaKeyValue struct {
	protocol     string
	direction    string
	srcActor     deltaActorRef
	dstActor     deltaActorRef
	srcIfIndex   int
	srcIfName    string
	srcPortID    string
	dstIfIndex   int
	dstIfName    string
	dstPortID    string
	bridgeDomain string
}

func topologyLinkDeltaKey(link topologymodel.Link, srcActor, dstActor deltaActorRef) topologyLinkDeltaKeyValue {
	return topologyLinkDeltaKeyValue{
		protocol:     strings.ToLower(strings.TrimSpace(link.Protocol)),
		direction:    strings.ToLower(strings.TrimSpace(link.Direction)),
		srcActor:     srcActor,
		dstActor:     dstActor,
		srcIfIndex:   max(link.Src.IfIndex, 0),
		srcIfName:    topologymodel.EndpointKey(link.Src, "if_name"),
		srcPortID:    topologymodel.EndpointKey(link.Src, "port_id"),
		dstIfIndex:   max(link.Dst.IfIndex, 0),
		dstIfName:    topologymodel.EndpointKey(link.Dst, "if_name"),
		dstPortID:    topologymodel.EndpointKey(link.Dst, "port_id"),
		bridgeDomain: topologyL2BridgeDomain(link),
	}
}

func MarkProbableDeltaLinks(strictData, probableData *topologymodel.Data) {
	if strictData == nil || probableData == nil {
		return
	}

	strictActorRefs, probableActorRefs := buildDeltaActorRefs(strictData.Actors, probableData.Actors)
	strictKeys := make(map[topologyLinkDeltaKeyValue]struct{}, len(strictData.Links))
	for _, link := range strictData.Links {
		strictKeys[topologyLinkDeltaKey(link, strictActorRefs[link.SrcActorHandle], strictActorRefs[link.DstActorHandle])] = struct{}{}
	}

	for idx, link := range probableData.Links {
		key := topologyLinkDeltaKey(link, probableActorRefs[link.SrcActorHandle], probableActorRefs[link.DstActorHandle])
		if _, exists := strictKeys[key]; exists {
			continue
		}
		link.State = "probable"
		inference := topologymodel.EnsureLinkInference(&link)
		if inference != nil {
			inference.Inference = "probable"
		}
		if inference != nil && strings.TrimSpace(inference.Confidence) == "" {
			inference.Confidence = "low"
		}
		if inference != nil && strings.TrimSpace(inference.AttachmentMode) == "" {
			if strings.EqualFold(strings.TrimSpace(link.Protocol), "bridge") {
				inference.AttachmentMode = "probable_bridge_anchor"
			} else {
				inference.AttachmentMode = "probable_added"
			}
		}
		probableData.Links[idx] = link
	}
	topologymodel.RecomputeLinkStats(probableData)
}

func buildDeltaActorRefs(strictActors, probableActors []topologymodel.Actor) (map[topologymodel.ActorHandle]deltaActorRef, map[topologymodel.ActorHandle]deltaActorRef) {
	byActorID := make(map[string]deltaActorRef, len(strictActors)+len(probableActors))
	strictRefs := make(map[topologymodel.ActorHandle]deltaActorRef, len(strictActors))
	probableRefs := make(map[topologymodel.ActorHandle]deltaActorRef, len(probableActors))
	next := deltaActorRef(1)
	add := func(actors []topologymodel.Actor, refs map[topologymodel.ActorHandle]deltaActorRef) {
		for _, actor := range actors {
			actorID := strings.TrimSpace(actor.ActorID)
			ref, ok := byActorID[actorID]
			if actorID == "" || !ok {
				ref = next
				next++
				if actorID != "" {
					byActorID[actorID] = ref
				}
			}
			refs[actor.ActorHandle] = ref
		}
	}
	add(strictActors, strictRefs)
	add(probableActors, probableRefs)
	return strictRefs, probableRefs
}

type topologyLinkActorKeyValue struct {
	protocol       string
	direction      string
	srcActor       topologymodel.ActorHandle
	dstActor       topologymodel.ActorHandle
	srcIfIndex     string
	srcIfName      string
	srcPortID      string
	dstIfIndex     string
	dstIfName      string
	dstPortID      string
	state          string
	bridgeDomain   string
	attachmentMode string
	inference      string
}

func topologyLinkActorKey(link topologymodel.Link) topologyLinkActorKeyValue {
	return topologyLinkActorKeyValue{
		protocol:       link.Protocol,
		direction:      link.Direction,
		srcActor:       link.SrcActorHandle,
		dstActor:       link.DstActorHandle,
		srcIfIndex:     topologymodel.EndpointKey(link.Src, "if_index"),
		srcIfName:      topologymodel.EndpointKey(link.Src, "if_name"),
		srcPortID:      topologymodel.EndpointKey(link.Src, "port_id"),
		dstIfIndex:     topologymodel.EndpointKey(link.Dst, "if_index"),
		dstIfName:      topologymodel.EndpointKey(link.Dst, "if_name"),
		dstPortID:      topologymodel.EndpointKey(link.Dst, "port_id"),
		state:          link.State,
		bridgeDomain:   topologyL2BridgeDomain(link),
		attachmentMode: topologymodel.LinkAttachmentModeValue(link),
		inference:      topologymodel.LinkInferenceValue(link),
	}
}

func topologyL2BridgeDomain(link topologymodel.Link) string {
	if link.L2 == nil {
		return ""
	}
	return strings.TrimSpace(link.L2.BridgeDomain)
}
