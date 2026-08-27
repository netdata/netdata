// SPDX-License-Identifier: GPL-3.0-or-later

package topologyshape

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/topology/worklimit"
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
	key, _ := topologyLinkDeltaKeyWithLimiter(link, srcActor, dstActor, nil)
	return key
}

func topologyLinkDeltaKeyWithLimiter(
	link topologymodel.Link,
	srcActor, dstActor deltaActorRef,
	limiter worklimit.Limiter,
) (topologyLinkDeltaKeyValue, error) {
	srcIfName := topologymodel.EndpointKey(link.Src, "if_name")
	srcPortID := topologymodel.EndpointKey(link.Src, "port_id")
	dstIfName := topologymodel.EndpointKey(link.Dst, "if_name")
	dstPortID := topologymodel.EndpointKey(link.Dst, "port_id")
	bridgeDomain := topologyL2BridgeDomain(link)
	if err := worklimit.ChargeStrings(limiter, []string{
		link.Protocol, link.Direction, srcIfName, srcPortID, dstIfName, dstPortID, bridgeDomain,
	}); err != nil {
		return topologyLinkDeltaKeyValue{}, err
	}
	return topologyLinkDeltaKeyValue{
		protocol:     strings.ToLower(strings.TrimSpace(link.Protocol)),
		direction:    strings.ToLower(strings.TrimSpace(link.Direction)),
		srcActor:     srcActor,
		dstActor:     dstActor,
		srcIfIndex:   max(link.Src.IfIndex, 0),
		srcIfName:    srcIfName,
		srcPortID:    srcPortID,
		dstIfIndex:   max(link.Dst.IfIndex, 0),
		dstIfName:    dstIfName,
		dstPortID:    dstPortID,
		bridgeDomain: bridgeDomain,
	}, nil
}

func MarkProbableDeltaLinks(strictData, probableData *topologymodel.Data) {
	_ = MarkProbableDeltaLinksWithLimiter(strictData, probableData, nil)
}

func MarkProbableDeltaLinksWithLimiter(
	strictData, probableData *topologymodel.Data,
	limiter worklimit.Limiter,
) error {
	if strictData == nil || probableData == nil {
		return nil
	}

	strictActorRefs, probableActorRefs, err := buildDeltaActorRefsWithLimiter(strictData.Actors, probableData.Actors, limiter)
	if err != nil {
		return err
	}
	if err := limiter.Charge(uint64(len(strictData.Links))); err != nil {
		return err
	}
	strictKeys := make(map[topologyLinkDeltaKeyValue]struct{}, len(strictData.Links))
	for _, link := range strictData.Links {
		key, err := topologyLinkDeltaKeyWithLimiter(link, strictActorRefs[link.SrcActorHandle], strictActorRefs[link.DstActorHandle], limiter)
		if err != nil {
			return err
		}
		strictKeys[key] = struct{}{}
	}

	if err := limiter.Charge(uint64(len(probableData.Links))); err != nil {
		return err
	}
	for idx, link := range probableData.Links {
		key, err := topologyLinkDeltaKeyWithLimiter(link, probableActorRefs[link.SrcActorHandle], probableActorRefs[link.DstActorHandle], limiter)
		if err != nil {
			return err
		}
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
	return topologymodel.RecomputeLinkStatsWithLimiter(probableData, limiter)
}

func buildDeltaActorRefs(strictActors, probableActors []topologymodel.Actor) (map[topologymodel.ActorHandle]deltaActorRef, map[topologymodel.ActorHandle]deltaActorRef) {
	strictRefs, probableRefs, _ := buildDeltaActorRefsWithLimiter(strictActors, probableActors, nil)
	return strictRefs, probableRefs
}

func buildDeltaActorRefsWithLimiter(
	strictActors, probableActors []topologymodel.Actor,
	limiter worklimit.Limiter,
) (map[topologymodel.ActorHandle]deltaActorRef, map[topologymodel.ActorHandle]deltaActorRef, error) {
	actorCount, err := worklimit.Sum(uint64(len(strictActors)), uint64(len(probableActors)))
	if err != nil {
		return nil, nil, err
	}
	if err := limiter.Charge(actorCount); err != nil {
		return nil, nil, err
	}
	byActorID := make(map[string]deltaActorRef, len(strictActors)+len(probableActors))
	strictRefs := make(map[topologymodel.ActorHandle]deltaActorRef, len(strictActors))
	probableRefs := make(map[topologymodel.ActorHandle]deltaActorRef, len(probableActors))
	next := deltaActorRef(1)
	add := func(actors []topologymodel.Actor, refs map[topologymodel.ActorHandle]deltaActorRef) error {
		for _, actor := range actors {
			if err := worklimit.ChargeStrings(limiter, []string{actor.ActorID}); err != nil {
				return err
			}
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
		return nil
	}
	if err := add(strictActors, strictRefs); err != nil {
		return nil, nil, err
	}
	if err := add(probableActors, probableRefs); err != nil {
		return nil, nil, err
	}
	return strictRefs, probableRefs, nil
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
	key, _ := topologyLinkActorKeyWithLimiter(link, nil)
	return key
}

func topologyLinkActorKeyWithLimiter(link topologymodel.Link, limiter worklimit.Limiter) (topologyLinkActorKeyValue, error) {
	srcIfIndex := topologymodel.EndpointKey(link.Src, "if_index")
	srcIfName := topologymodel.EndpointKey(link.Src, "if_name")
	srcPortID := topologymodel.EndpointKey(link.Src, "port_id")
	dstIfIndex := topologymodel.EndpointKey(link.Dst, "if_index")
	dstIfName := topologymodel.EndpointKey(link.Dst, "if_name")
	dstPortID := topologymodel.EndpointKey(link.Dst, "port_id")
	bridgeDomain := topologyL2BridgeDomain(link)
	attachmentMode := topologymodel.LinkAttachmentModeValue(link)
	inference := topologymodel.LinkInferenceValue(link)
	if err := worklimit.ChargeStrings(limiter, []string{
		link.Protocol, link.Direction, srcIfIndex, srcIfName, srcPortID,
		dstIfIndex, dstIfName, dstPortID, link.State, bridgeDomain, attachmentMode, inference,
	}); err != nil {
		return topologyLinkActorKeyValue{}, err
	}
	return topologyLinkActorKeyValue{
		protocol:       link.Protocol,
		direction:      link.Direction,
		srcActor:       link.SrcActorHandle,
		dstActor:       link.DstActorHandle,
		srcIfIndex:     srcIfIndex,
		srcIfName:      srcIfName,
		srcPortID:      srcPortID,
		dstIfIndex:     dstIfIndex,
		dstIfName:      dstIfName,
		dstPortID:      dstPortID,
		state:          link.State,
		bridgeDomain:   bridgeDomain,
		attachmentMode: attachmentMode,
		inference:      inference,
	}, nil
}

func topologyL2BridgeDomain(link topologymodel.Link) string {
	if link.L2 == nil {
		return ""
	}
	return strings.TrimSpace(link.L2.BridgeDomain)
}
