// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"fmt"
	"strings"
)

func topologyLinkDeltaKey(link topologyLink) string {
	return strings.Join([]string{
		strings.ToLower(strings.TrimSpace(link.Protocol)),
		strings.ToLower(strings.TrimSpace(link.Direction)),
		strings.TrimSpace(link.SrcActorID),
		strings.TrimSpace(link.DstActorID),
		attrKey(link.Src.Attributes, "if_index"),
		attrKey(link.Src.Attributes, "if_name"),
		attrKey(link.Src.Attributes, "port_id"),
		attrKey(link.Dst.Attributes, "if_index"),
		attrKey(link.Dst.Attributes, "if_name"),
		attrKey(link.Dst.Attributes, "port_id"),
		fmt.Sprint(link.Metrics["bridge_domain"]),
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
		if link.Metrics == nil {
			link.Metrics = make(map[string]any)
		}
		link.Metrics["inference"] = "probable"
		if topologyMetricValueString(link.Metrics, "confidence") == "" {
			link.Metrics["confidence"] = "low"
		}
		if topologyMetricValueString(link.Metrics, "attachment_mode") == "" {
			if strings.EqualFold(strings.TrimSpace(link.Protocol), "bridge") {
				link.Metrics["attachment_mode"] = "probable_bridge_anchor"
			} else {
				link.Metrics["attachment_mode"] = "probable_added"
			}
		}
		probableData.Links[idx] = link
	}
	recomputeTopologyLinkStats(probableData)
}

func topologyLinkActorKey(link topologyLink) string {
	return strings.Join([]string{
		link.Protocol,
		link.Direction,
		link.SrcActorID,
		link.DstActorID,
		attrKey(link.Src.Attributes, "if_index"),
		attrKey(link.Src.Attributes, "if_name"),
		attrKey(link.Src.Attributes, "port_id"),
		attrKey(link.Dst.Attributes, "if_index"),
		attrKey(link.Dst.Attributes, "if_name"),
		attrKey(link.Dst.Attributes, "port_id"),
		link.State,
		fmt.Sprint(link.Metrics["bridge_domain"]),
		fmt.Sprint(link.Metrics["attachment_mode"]),
		fmt.Sprint(link.Metrics["inference"]),
	}, "|")
}
