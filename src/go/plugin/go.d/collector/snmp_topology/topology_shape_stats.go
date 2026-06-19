// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"strings"
)

func recomputeTopologyLinkStats(data *topologyData) {
	if data == nil {
		return
	}
	if data.Stats == nil {
		data.Stats = make(map[string]any)
	}
	data.Stats["actors_total"] = len(data.Actors)
	data.Stats["links_total"] = len(data.Links)

	probable := 0
	for _, link := range data.Links {
		state := strings.ToLower(strings.TrimSpace(link.State))
		inference := strings.ToLower(topologyMetricValueString(link.Metrics, "inference"))
		attachment := strings.ToLower(topologyMetricValueString(link.Metrics, "attachment_mode"))
		if state == "probable" || inference == "probable" || strings.HasPrefix(attachment, "probable_") {
			probable++
		}
	}
	data.Stats["links_probable"] = probable
	recomputeTopologyL3VisibleLinkStats(data)
}

func recomputeTopologyL3VisibleLinkStats(data *topologyData) {
	if data == nil || data.Stats == nil {
		return
	}
	if _, ok := data.Stats["l3_subnet_emitted_links"]; !ok {
		return
	}
	count := 0
	for _, link := range data.Links {
		if strings.EqualFold(strings.TrimSpace(firstNonEmptyString(link.LinkType, link.Protocol)), topologyL3SubnetLinkType) {
			count++
		}
	}
	data.Stats["l3_subnet_visible_links"] = count
}
