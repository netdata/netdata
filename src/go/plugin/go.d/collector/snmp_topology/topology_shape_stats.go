// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"strings"
)

func recomputeTopologyLinkStats(data *topologyData) {
	if data == nil {
		return
	}
	data.Stats.Recomputed.ActorsTotal = len(data.Actors)
	data.Stats.Recomputed.LinksTotal = len(data.Links)

	probable := 0
	for _, link := range data.Links {
		state := strings.ToLower(strings.TrimSpace(link.State))
		inference := strings.ToLower(topologyMetricValueString(link.Metrics, "inference"))
		attachment := strings.ToLower(topologyMetricValueString(link.Metrics, "attachment_mode"))
		if state == "probable" || inference == "probable" || strings.HasPrefix(attachment, "probable_") {
			probable++
		}
	}
	data.Stats.Recomputed.LinksProbable = probable
	data.Stats.HasComputed = true
	recomputeTopologyL3VisibleLinkStats(data)
	recomputeTopologyOSPFVisibleLinkStats(data)
	recomputeTopologyBGPVisibleLinkStats(data)
}

func recomputeTopologyL3VisibleLinkStats(data *topologyData) {
	if data == nil || !data.Stats.HasL3 {
		return
	}
	count := 0
	for _, link := range data.Links {
		if strings.EqualFold(strings.TrimSpace(firstNonEmptyString(link.LinkType, link.Protocol)), topologyL3SubnetLinkType) {
			count++
		}
	}
	data.Stats.Recomputed.L3SubnetVisibleLinks = count
}

func recomputeTopologyOSPFVisibleLinkStats(data *topologyData) {
	if data == nil || !data.Stats.HasOSPF {
		return
	}
	count := 0
	for _, link := range data.Links {
		if strings.EqualFold(strings.TrimSpace(firstNonEmptyString(link.LinkType, link.Protocol)), topologyOSPFAdjacencyLinkType) {
			count++
		}
	}
	data.Stats.Recomputed.OSPFAdjacencyVisibleLinks = count
}

func recomputeTopologyBGPVisibleLinkStats(data *topologyData) {
	if data == nil || !data.Stats.HasBGP {
		return
	}
	count := 0
	for _, link := range data.Links {
		if strings.EqualFold(strings.TrimSpace(firstNonEmptyString(link.LinkType, link.Protocol)), topologyBGPAdjacencyLinkType) {
			count++
		}
	}
	data.Stats.Recomputed.BGPAdjacencyVisibleLinks = count
}
