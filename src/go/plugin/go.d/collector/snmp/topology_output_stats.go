// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

func normalizedMatchIPs(match topologyMatch) []string {
	if len(match.IPAddresses) == 0 {
		return nil
	}
	out := make([]string, 0, len(match.IPAddresses))
	seen := make(map[string]struct{}, len(match.IPAddresses))
	for _, value := range match.IPAddresses {
		ip := normalizeIPAddress(value)
		if ip == "" {
			continue
		}
		if _, ok := seen[ip]; ok {
			continue
		}
		seen[ip] = struct{}{}
		out = append(out, ip)
	}
	sort.Strings(out)
	return out
}

func topologyActorIsInferred(actor topologyActor) bool {
	if strings.EqualFold(strings.TrimSpace(actor.ActorType), "endpoint") {
		return true
	}
	if boolStatValue(actor.Attributes["inferred"]) {
		return true
	}
	if boolStatValue(actor.Labels["inferred"]) {
		return true
	}
	return false
}

func boolStatValue(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "1", "true", "yes", "on":
			return true
		}
	}
	return false
}

func intStatValue(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(typed))
		if err == nil {
			return n
		}
	}
	return 0
}

func topologyMetricValueString(metrics map[string]any, key string) string {
	if metrics == nil {
		return ""
	}
	value, ok := metrics[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

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
}

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
