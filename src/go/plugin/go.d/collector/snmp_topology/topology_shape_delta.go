// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
)

func topologyLinkDeltaKey(link topologyLink) string {
	return strings.Join([]string{
		strings.ToLower(strings.TrimSpace(link.Protocol)),
		strings.ToLower(strings.TrimSpace(link.Direction)),
		strings.TrimSpace(link.SrcActorID),
		strings.TrimSpace(link.DstActorID),
		topologymodel.EndpointKey(link.Src, "if_index"),
		topologymodel.EndpointKey(link.Src, "if_name"),
		topologymodel.EndpointKey(link.Src, "port_id"),
		topologymodel.EndpointKey(link.Dst, "if_index"),
		topologymodel.EndpointKey(link.Dst, "if_name"),
		topologymodel.EndpointKey(link.Dst, "port_id"),
		topologyL2BridgeDomain(link),
	}, "|")
}

func markProbableDeltaLinks(strictData, probableData *topologyData) {
	if strictData == nil || probableData == nil {
		return
	}

	strictKeys := make(map[string]struct{}, len(strictData.Links))
	for _, link := range strictData.Links {
		strictKeys[topologyLinkDeltaKey(link)] = struct{}{}
	}

	for idx, link := range probableData.Links {
		key := topologyLinkDeltaKey(link)
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

func topologyLinkActorKey(link topologyLink) string {
	return strings.Join([]string{
		link.Protocol,
		link.Direction,
		link.SrcActorID,
		link.DstActorID,
		topologymodel.EndpointKey(link.Src, "if_index"),
		topologymodel.EndpointKey(link.Src, "if_name"),
		topologymodel.EndpointKey(link.Src, "port_id"),
		topologymodel.EndpointKey(link.Dst, "if_index"),
		topologymodel.EndpointKey(link.Dst, "if_name"),
		topologymodel.EndpointKey(link.Dst, "port_id"),
		link.State,
		topologyL2BridgeDomain(link),
		topologymodel.LinkAttachmentModeValue(link),
		topologymodel.LinkInferenceValue(link),
	}, "|")
}

func topologyL2BridgeDomain(link topologyLink) string {
	if link.L2 == nil {
		return ""
	}
	return strings.TrimSpace(link.L2.BridgeDomain)
}
