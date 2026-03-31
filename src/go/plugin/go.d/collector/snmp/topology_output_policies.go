// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/topology/engine"
)

func applySNMPTopologyOutputPolicies(data *topologyData, options topologyQueryOptions) {
	if data == nil {
		return
	}
	mapType := normalizeTopologyMapType(options.MapType)
	if mapType == "" {
		mapType = topologyMapTypeAllDevicesLowConfidence
	}
	options.MapType = mapType

	collapsed := 0
	if options.CollapseActorsByIP {
		collapsed = collapseActorsByIP(data)
	}

	removedNonIP := 0
	if options.EliminateNonIPInferred {
		removedNonIP = eliminateNonIPInferredActors(data)
	}

	filterDanglingLinks(data)
	removedByMapType := applyMapTypePolicy(data, options.MapType)
	filterDanglingLinks(data)

	removedSparseSegments := 0
	if options.EliminateNonIPInferred {
		removedSparseSegments = pruneSparseSegments(data, 1)
	}
	filterDanglingLinks(data)

	sort.Slice(data.Actors, func(i, j int) bool {
		return canonicalMatchKey(data.Actors[i].Match) < canonicalMatchKey(data.Actors[j].Match)
	})
	sort.Slice(data.Links, func(i, j int) bool {
		return topologyLinkSortKey(data.Links[i]) < topologyLinkSortKey(data.Links[j])
	})

	if data.Stats == nil {
		data.Stats = make(map[string]any)
	}
	data.Stats["actors_collapsed_by_ip"] = collapsed
	data.Stats["actors_non_ip_inferred_suppressed"] = removedNonIP
	data.Stats["actors_map_type_suppressed"] = removedByMapType
	data.Stats["segments_sparse_suppressed"] = removedSparseSegments
	data.Stats["map_type"] = options.MapType
	if strategy := normalizeTopologyInferenceStrategy(options.InferenceStrategy); strategy != "" {
		data.Stats["inference_strategy"] = strategy
	}
	if removedSparseSegments > 0 {
		data.Stats["segments_suppressed"] = intStatValue(data.Stats["segments_suppressed"]) + removedSparseSegments
	}
	recomputeTopologyLinkStats(data)
}

func applyMapTypePolicy(data *topologyData, mapType string) int {
	switch normalizeTopologyMapType(mapType) {
	case topologyMapTypeLLDPCDPManaged:
		return applyLLDPCDPManagedMapPolicy(data)
	case topologyMapTypeHighConfidenceInferred:
		return suppressUnlinkedInferredEndpoints(data)
	default:
		return 0
	}
}

func applyLLDPCDPManagedMapPolicy(data *topologyData) int {
	if data == nil || len(data.Actors) == 0 {
		return 0
	}

	managedIDs := make(map[string]struct{})
	for _, actor := range data.Actors {
		if !isManagedSNMPDeviceActor(actor) {
			continue
		}
		managedIDs[actor.ActorID] = struct{}{}
	}

	keepLink := func(link topologyLink) bool {
		protocol := strings.ToLower(strings.TrimSpace(link.Protocol))
		return protocol == "lldp" || protocol == "cdp"
	}

	keptLinks := make([]topologyLink, 0, len(data.Links))
	linkedIDs := make(map[string]struct{}, len(managedIDs))
	for managedID := range managedIDs {
		linkedIDs[managedID] = struct{}{}
	}
	for _, link := range data.Links {
		if !keepLink(link) {
			continue
		}
		keptLinks = append(keptLinks, link)
		if strings.TrimSpace(link.SrcActorID) != "" {
			linkedIDs[link.SrcActorID] = struct{}{}
		}
		if strings.TrimSpace(link.DstActorID) != "" {
			linkedIDs[link.DstActorID] = struct{}{}
		}
	}
	data.Links = keptLinks

	keptActors := make([]topologyActor, 0, len(data.Actors))
	removed := 0
	for _, actor := range data.Actors {
		if _, ok := linkedIDs[actor.ActorID]; ok {
			keptActors = append(keptActors, actor)
			continue
		}
		removed++
	}
	data.Actors = keptActors
	return removed
}

func suppressUnlinkedInferredEndpoints(data *topologyData) int {
	if data == nil || len(data.Actors) == 0 {
		return 0
	}

	linked := make(map[string]struct{}, len(data.Links)*2)
	for _, link := range data.Links {
		if strings.TrimSpace(link.SrcActorID) != "" {
			linked[link.SrcActorID] = struct{}{}
		}
		if strings.TrimSpace(link.DstActorID) != "" {
			linked[link.DstActorID] = struct{}{}
		}
	}

	removed := 0
	removedIDs := make(map[string]struct{})
	kept := make([]topologyActor, 0, len(data.Actors))
	for _, actor := range data.Actors {
		if !strings.EqualFold(strings.TrimSpace(actor.ActorType), "endpoint") {
			kept = append(kept, actor)
			continue
		}
		if !topologyActorIsInferred(actor) {
			kept = append(kept, actor)
			continue
		}
		if _, ok := linked[actor.ActorID]; ok {
			kept = append(kept, actor)
			continue
		}
		removed++
		removedIDs[actor.ActorID] = struct{}{}
	}
	if removed == 0 {
		return 0
	}
	data.Actors = kept
	if len(data.Links) == 0 {
		return removed
	}
	filtered := make([]topologyLink, 0, len(data.Links))
	for _, link := range data.Links {
		if _, drop := removedIDs[link.SrcActorID]; drop {
			continue
		}
		if _, drop := removedIDs[link.DstActorID]; drop {
			continue
		}
		filtered = append(filtered, link)
	}
	data.Links = filtered
	return removed
}

func isManagedSNMPDeviceActor(actor topologyActor) bool {
	if !topologyengine.IsDeviceActorType(actor.ActorType) {
		return false
	}
	if strings.ToLower(strings.TrimSpace(actor.Source)) != "snmp" {
		return false
	}
	return !topologyActorIsInferred(actor)
}

func eliminateNonIPInferredActors(data *topologyData) int {
	if data == nil || len(data.Actors) == 0 {
		return 0
	}

	removedIDs := make(map[string]struct{})
	keptActors := make([]topologyActor, 0, len(data.Actors))
	for _, actor := range data.Actors {
		if topologyActorIsInferred(actor) && len(normalizedMatchIPs(actor.Match)) == 0 {
			removedIDs[actor.ActorID] = struct{}{}
			continue
		}
		keptActors = append(keptActors, actor)
	}

	if len(removedIDs) == 0 {
		return 0
	}

	data.Actors = keptActors
	links := make([]topologyLink, 0, len(data.Links))
	for _, link := range data.Links {
		if _, removed := removedIDs[link.SrcActorID]; removed {
			continue
		}
		if _, removed := removedIDs[link.DstActorID]; removed {
			continue
		}
		links = append(links, link)
	}
	data.Links = links
	return len(removedIDs)
}

func pruneSparseSegments(data *topologyData, threshold int) int {
	if data == nil || len(data.Actors) == 0 {
		return 0
	}

	removedTotal := 0
	for {
		segmentSet := make(map[string]struct{})
		for _, actor := range data.Actors {
			if strings.EqualFold(strings.TrimSpace(actor.ActorType), "segment") {
				segmentSet[actor.ActorID] = struct{}{}
			}
		}
		if len(segmentSet) == 0 {
			return removedTotal
		}

		neighborSet := make(map[string]map[string]struct{}, len(segmentSet))
		for segmentID := range segmentSet {
			neighborSet[segmentID] = make(map[string]struct{})
		}
		for _, link := range data.Links {
			if _, ok := segmentSet[link.SrcActorID]; ok {
				neighborSet[link.SrcActorID][link.DstActorID] = struct{}{}
			}
			if _, ok := segmentSet[link.DstActorID]; ok {
				neighborSet[link.DstActorID][link.SrcActorID] = struct{}{}
			}
		}

		removeSegments := make(map[string]struct{})
		for segmentID, neighbors := range neighborSet {
			if len(neighbors) <= threshold {
				removeSegments[segmentID] = struct{}{}
			}
		}
		if len(removeSegments) == 0 {
			return removedTotal
		}
		removedTotal += len(removeSegments)

		filteredActors := make([]topologyActor, 0, len(data.Actors)-len(removeSegments))
		for _, actor := range data.Actors {
			if _, drop := removeSegments[actor.ActorID]; drop {
				continue
			}
			filteredActors = append(filteredActors, actor)
		}
		data.Actors = filteredActors

		filteredLinks := make([]topologyLink, 0, len(data.Links))
		for _, link := range data.Links {
			if _, drop := removeSegments[link.SrcActorID]; drop {
				continue
			}
			if _, drop := removeSegments[link.DstActorID]; drop {
				continue
			}
			filteredLinks = append(filteredLinks, link)
		}
		data.Links = filteredLinks
	}
}

func filterDanglingLinks(data *topologyData) {
	if data == nil || len(data.Links) == 0 {
		return
	}
	actorSet := make(map[string]struct{}, len(data.Actors))
	for _, actor := range data.Actors {
		if id := strings.TrimSpace(actor.ActorID); id != "" {
			actorSet[id] = struct{}{}
		}
	}
	if len(actorSet) == 0 {
		data.Links = nil
		return
	}
	filtered := make([]topologyLink, 0, len(data.Links))
	for _, link := range data.Links {
		if _, ok := actorSet[link.SrcActorID]; !ok {
			continue
		}
		if _, ok := actorSet[link.DstActorID]; !ok {
			continue
		}
		filtered = append(filtered, link)
	}
	data.Links = filtered
}

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
