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

func collapseActorsByIP(data *topologyData) int {
	if data == nil || len(data.Actors) <= 1 {
		return 0
	}

	type dsu struct {
		parent []int
	}
	find := func(d *dsu, x int) int {
		for d.parent[x] != x {
			d.parent[x] = d.parent[d.parent[x]]
			x = d.parent[x]
		}
		return x
	}
	union := func(d *dsu, a, b int) {
		ra := find(d, a)
		rb := find(d, b)
		if ra == rb {
			return
		}
		if ra < rb {
			d.parent[rb] = ra
			return
		}
		d.parent[ra] = rb
	}

	d := &dsu{parent: make([]int, len(data.Actors))}
	for i := range d.parent {
		d.parent[i] = i
	}

	ipOwner := make(map[string]int)
	for idx, actor := range data.Actors {
		if strings.EqualFold(strings.TrimSpace(actor.ActorType), "segment") {
			continue
		}
		ips := normalizedMatchIPs(actor.Match)
		if len(ips) == 0 {
			continue
		}
		for _, ip := range ips {
			if owner, ok := ipOwner[ip]; ok {
				union(d, idx, owner)
				continue
			}
			ipOwner[ip] = idx
		}
	}

	groupMembers := make(map[int][]int)
	for idx := range data.Actors {
		root := find(d, idx)
		groupMembers[root] = append(groupMembers[root], idx)
	}

	replaceActorID := make(map[string]string)
	keep := make([]bool, len(data.Actors))
	for i := range keep {
		keep[i] = true
	}

	collapsed := 0
	for _, members := range groupMembers {
		if len(members) <= 1 {
			continue
		}
		rep := members[0]
		for _, idx := range members[1:] {
			if compareCollapseActorPriority(data.Actors[idx], data.Actors[rep]) < 0 {
				rep = idx
			}
		}

		repActor := data.Actors[rep]
		collapsedCount := 1
		for _, idx := range members {
			if idx == rep {
				continue
			}
			collapsedCount++
			collapsed++
			replaceActorID[data.Actors[idx].ActorID] = repActor.ActorID
			repActor.Match = mergeTopologyMatch(repActor.Match, data.Actors[idx].Match)
			repActor.Labels = mergeTopologyStringMap(repActor.Labels, data.Actors[idx].Labels)
			repActor.Attributes = mergeTopologyAnyMap(repActor.Attributes, data.Actors[idx].Attributes)
			keep[idx] = false
		}
		if repActor.Attributes == nil {
			repActor.Attributes = make(map[string]any)
		}
		if collapsedCount > 1 {
			repActor.Attributes["collapsed_by_ip"] = true
			repActor.Attributes["collapsed_count"] = collapsedCount
		}
		data.Actors[rep] = repActor
	}

	if collapsed == 0 {
		return 0
	}

	actors := make([]topologyActor, 0, len(data.Actors)-collapsed)
	for idx, actor := range data.Actors {
		if !keep[idx] {
			continue
		}
		actors = append(actors, actor)
	}
	data.Actors = actors

	links := make([]topologyLink, 0, len(data.Links))
	seen := make(map[string]struct{}, len(data.Links))
	for _, link := range data.Links {
		if replacement, ok := replaceActorID[link.SrcActorID]; ok && replacement != "" {
			link.SrcActorID = replacement
		}
		if replacement, ok := replaceActorID[link.DstActorID]; ok && replacement != "" {
			link.DstActorID = replacement
		}
		if strings.TrimSpace(link.SrcActorID) == "" || strings.TrimSpace(link.DstActorID) == "" {
			continue
		}
		if link.SrcActorID == link.DstActorID {
			continue
		}
		key := topologyLinkActorKey(link)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		links = append(links, link)
	}
	data.Links = links
	return collapsed
}

func compareCollapseActorPriority(left, right topologyActor) int {
	if leftDevice, rightDevice := topologyengine.IsDeviceActorType(left.ActorType), topologyengine.IsDeviceActorType(right.ActorType); leftDevice != rightDevice {
		if leftDevice {
			return -1
		}
		return 1
	}
	if leftInferred, rightInferred := topologyActorIsInferred(left), topologyActorIsInferred(right); leftInferred != rightInferred {
		if !leftInferred {
			return -1
		}
		return 1
	}
	leftID := strings.ToLower(strings.TrimSpace(left.ActorID))
	rightID := strings.ToLower(strings.TrimSpace(right.ActorID))
	if (leftID == "") != (rightID == "") {
		if leftID != "" {
			return -1
		}
		return 1
	}
	return strings.Compare(leftID, rightID)
}

func mergeTopologyMatch(base, other topologyMatch) topologyMatch {
	base.ChassisIDs = appendUniqueTopologyStrings(base.ChassisIDs, other.ChassisIDs...)
	base.MacAddresses = appendUniqueTopologyStrings(base.MacAddresses, other.MacAddresses...)
	base.IPAddresses = appendUniqueTopologyStrings(base.IPAddresses, other.IPAddresses...)
	base.Hostnames = appendUniqueTopologyStrings(base.Hostnames, other.Hostnames...)
	base.DNSNames = appendUniqueTopologyStrings(base.DNSNames, other.DNSNames...)
	if strings.TrimSpace(base.SysName) == "" {
		base.SysName = strings.TrimSpace(other.SysName)
	}
	if strings.TrimSpace(base.SysObjectID) == "" {
		base.SysObjectID = strings.TrimSpace(other.SysObjectID)
	}
	return base
}

func mergeTopologyStringMap(base, other map[string]string) map[string]string {
	if len(other) == 0 {
		return base
	}
	if base == nil {
		base = make(map[string]string, len(other))
	}
	for key, value := range other {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		if _, exists := base[key]; exists {
			continue
		}
		base[key] = value
	}
	return base
}

func mergeTopologyAnyMap(base, other map[string]any) map[string]any {
	if len(other) == 0 {
		return base
	}
	if base == nil {
		base = make(map[string]any, len(other))
	}
	for key, value := range other {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, exists := base[key]; exists {
			continue
		}
		base[key] = value
	}
	return base
}

func appendUniqueTopologyStrings(base []string, values ...string) []string {
	seen := make(map[string]struct{}, len(base)+len(values))
	out := make([]string, 0, len(base)+len(values))
	for _, value := range append(base, values...) {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
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

func applyTopologyDepthFocusFilter(data *topologyData, options topologyQueryOptions) {
	if data == nil || len(data.Actors) == 0 {
		return
	}
	options = normalizeTopologyQueryOptions(options)
	focusIPs := topologyManagedFocusSelectedIPs(options.ManagedDeviceFocus)

	beforeActors := len(data.Actors)
	beforeLinks := len(data.Links)

	if isTopologyManagedFocusAllDevices(options.ManagedDeviceFocus) {
		if data.Stats == nil {
			data.Stats = make(map[string]any)
		}
		data.Stats["managed_snmp_device_focus"] = options.ManagedDeviceFocus
		data.Stats["depth"] = topologyDepthAll
		data.Stats["actors_focus_depth_filtered"] = 0
		data.Stats["links_focus_depth_filtered"] = 0
		recomputeTopologyLinkStats(data)
		return
	}

	actorByID := make(map[string]topologyActor, len(data.Actors))
	segmentSet := make(map[string]struct{})
	nonSegmentSet := make(map[string]struct{})
	for _, actor := range data.Actors {
		id := strings.TrimSpace(actor.ActorID)
		if id == "" {
			continue
		}
		actorByID[id] = actor
		if strings.EqualFold(strings.TrimSpace(actor.ActorType), "segment") {
			segmentSet[id] = struct{}{}
		} else {
			nonSegmentSet[id] = struct{}{}
		}
	}
	if len(nonSegmentSet) == 0 {
		recomputeTopologyLinkStats(data)
		return
	}

	if len(focusIPs) == 0 {
		recomputeTopologyLinkStats(data)
		return
	}

	roots := make(map[string]struct{})
	for actorID, actor := range actorByID {
		if _, ok := nonSegmentSet[actorID]; !ok {
			continue
		}
		if !isManagedSNMPDeviceActor(actor) {
			continue
		}
		for _, focusIP := range focusIPs {
			if !topologyActorHasIP(actor, focusIP) {
				continue
			}
			roots[actorID] = struct{}{}
			break
		}
	}
	if len(roots) == 0 {
		if data.Stats == nil {
			data.Stats = make(map[string]any)
		}
		data.Stats["managed_snmp_device_focus"] = options.ManagedDeviceFocus
		if options.Depth == topologyDepthAllInternal {
			data.Stats["depth"] = topologyDepthAll
		} else {
			data.Stats["depth"] = options.Depth
		}
		data.Stats["actors_focus_depth_filtered"] = 0
		data.Stats["links_focus_depth_filtered"] = 0
		recomputeTopologyLinkStats(data)
		return
	}

	nonSegmentAdj := make(map[string]map[string]struct{}, len(nonSegmentSet))
	nodeSegments := make(map[string]map[string]struct{}, len(nonSegmentSet))
	segmentNeighbors := make(map[string]map[string]struct{}, len(segmentSet))
	for actorID := range nonSegmentSet {
		nonSegmentAdj[actorID] = make(map[string]struct{})
		nodeSegments[actorID] = make(map[string]struct{})
	}
	for segmentID := range segmentSet {
		segmentNeighbors[segmentID] = make(map[string]struct{})
	}

	for _, link := range data.Links {
		src := strings.TrimSpace(link.SrcActorID)
		dst := strings.TrimSpace(link.DstActorID)
		if src == "" || dst == "" || src == dst {
			continue
		}
		_, srcSegment := segmentSet[src]
		_, dstSegment := segmentSet[dst]
		_, srcNonSegment := nonSegmentSet[src]
		_, dstNonSegment := nonSegmentSet[dst]

		switch {
		case srcNonSegment && dstNonSegment:
			nonSegmentAdj[src][dst] = struct{}{}
			nonSegmentAdj[dst][src] = struct{}{}
		case srcSegment && dstNonSegment:
			segmentNeighbors[src][dst] = struct{}{}
			nodeSegments[dst][src] = struct{}{}
		case dstSegment && srcNonSegment:
			segmentNeighbors[dst][src] = struct{}{}
			nodeSegments[src][dst] = struct{}{}
		}
	}

	distance := make(map[string]int, len(nonSegmentSet))
	queue := make([]string, 0, len(roots))
	for root := range roots {
		distance[root] = 0
		queue = append(queue, root)
	}
	segmentExpandedDepth := make(map[string]int)

	for head := 0; head < len(queue); head++ {
		current := queue[head]
		currentDepth := distance[current]
		if options.Depth != topologyDepthAllInternal && currentDepth >= options.Depth {
			continue
		}

		for neighbor := range nonSegmentAdj[current] {
			if _, seen := distance[neighbor]; seen {
				continue
			}
			distance[neighbor] = currentDepth + 1
			queue = append(queue, neighbor)
		}

		for segmentID := range nodeSegments[current] {
			if expandedAt, ok := segmentExpandedDepth[segmentID]; ok && expandedAt <= currentDepth {
				continue
			}
			segmentExpandedDepth[segmentID] = currentDepth
			for neighbor := range segmentNeighbors[segmentID] {
				if _, seen := distance[neighbor]; seen {
					continue
				}
				distance[neighbor] = currentDepth + 1
				queue = append(queue, neighbor)
			}
		}
	}

	includedNonSegment := make(map[string]struct{}, len(distance))
	for actorID, depth := range distance {
		if options.Depth == topologyDepthAllInternal || depth <= options.Depth {
			includedNonSegment[actorID] = struct{}{}
		}
	}
	if len(includedNonSegment) == 0 {
		recomputeTopologyLinkStats(data)
		return
	}

	includedActorsByDepth := make(map[string]struct{}, len(includedNonSegment)+len(segmentSet))
	for actorID := range includedNonSegment {
		includedActorsByDepth[actorID] = struct{}{}
	}
	if options.Depth == topologyDepthAllInternal || options.Depth > 0 {
		for segmentID, neighbors := range segmentNeighbors {
			for actorID := range neighbors {
				if _, ok := includedNonSegment[actorID]; ok {
					includedActorsByDepth[segmentID] = struct{}{}
					break
				}
			}
		}
	}

	shortestPathActors, shortestPathPairs := topologyShortestPathUnion(data, roots)
	includedActors := make(map[string]struct{}, len(includedActorsByDepth)+len(shortestPathActors))
	for actorID := range includedActorsByDepth {
		includedActors[actorID] = struct{}{}
	}
	for actorID := range shortestPathActors {
		includedActors[actorID] = struct{}{}
	}

	filteredLinks := make([]topologyLink, 0, len(data.Links))
	linkActors := make(map[string]struct{})
	for _, link := range data.Links {
		srcActorID := strings.TrimSpace(link.SrcActorID)
		dstActorID := strings.TrimSpace(link.DstActorID)
		if srcActorID == "" || dstActorID == "" {
			continue
		}

		_, srcInDepth := includedActorsByDepth[srcActorID]
		_, dstInDepth := includedActorsByDepth[dstActorID]
		_, inShortestPath := shortestPathPairs[topologyActorPairKey(srcActorID, dstActorID)]
		if !(srcInDepth && dstInDepth) && !inShortestPath {
			continue
		}

		filteredLinks = append(filteredLinks, link)
		linkActors[srcActorID] = struct{}{}
		linkActors[dstActorID] = struct{}{}
	}
	data.Links = filteredLinks

	for actorID := range linkActors {
		includedActors[actorID] = struct{}{}
	}

	filteredActors := make([]topologyActor, 0, len(data.Actors))
	for _, actor := range data.Actors {
		if _, ok := includedActors[actor.ActorID]; ok {
			filteredActors = append(filteredActors, actor)
		}
	}
	data.Actors = filteredActors

	filterDanglingLinks(data)
	if options.EliminateNonIPInferred {
		pruneSparseSegments(data, 1)
		filterDanglingLinks(data)
	}

	sort.Slice(data.Actors, func(i, j int) bool {
		return canonicalMatchKey(data.Actors[i].Match) < canonicalMatchKey(data.Actors[j].Match)
	})
	sort.Slice(data.Links, func(i, j int) bool {
		return topologyLinkSortKey(data.Links[i]) < topologyLinkSortKey(data.Links[j])
	})

	if data.Stats == nil {
		data.Stats = make(map[string]any)
	}
	data.Stats["managed_snmp_device_focus"] = options.ManagedDeviceFocus
	if options.Depth == topologyDepthAllInternal {
		data.Stats["depth"] = topologyDepthAll
	} else {
		data.Stats["depth"] = options.Depth
	}
	data.Stats["actors_focus_depth_filtered"] = beforeActors - len(data.Actors)
	data.Stats["links_focus_depth_filtered"] = beforeLinks - len(data.Links)
	recomputeTopologyLinkStats(data)
}

func topologyManagedFocusSelectedIP(value string) string {
	ips := topologyManagedFocusSelectedIPs(value)
	if len(ips) == 0 {
		return ""
	}
	return ips[0]
}

func topologyManagedFocusSelectedIPs(value string) []string {
	normalized := parseTopologyManagedFocuses(value)
	if len(normalized) == 1 && normalized[0] == topologyManagedFocusAllDevices {
		return nil
	}

	out := make([]string, 0, len(normalized))
	for _, focus := range normalized {
		if len(focus) <= len(topologyManagedFocusIPPrefix) {
			continue
		}
		if !strings.EqualFold(focus[:len(topologyManagedFocusIPPrefix)], topologyManagedFocusIPPrefix) {
			continue
		}
		ip := normalizeIPAddress(strings.TrimSpace(focus[len(topologyManagedFocusIPPrefix):]))
		if ip == "" {
			continue
		}
		out = append(out, ip)
	}
	return out
}

func topologyShortestPathUnion(
	data *topologyData,
	roots map[string]struct{},
) (map[string]struct{}, map[string]struct{}) {
	includedActors := make(map[string]struct{})
	includedPairs := make(map[string]struct{})
	if data == nil || len(roots) < 2 {
		return includedActors, includedPairs
	}

	adjacency := make(map[string]map[string]struct{})
	for _, link := range data.Links {
		src := strings.TrimSpace(link.SrcActorID)
		dst := strings.TrimSpace(link.DstActorID)
		if src == "" || dst == "" || src == dst {
			continue
		}
		if _, ok := adjacency[src]; !ok {
			adjacency[src] = make(map[string]struct{})
		}
		if _, ok := adjacency[dst]; !ok {
			adjacency[dst] = make(map[string]struct{})
		}
		adjacency[src][dst] = struct{}{}
		adjacency[dst][src] = struct{}{}
	}

	rootIDs := make([]string, 0, len(roots))
	for actorID := range roots {
		rootIDs = append(rootIDs, actorID)
	}
	sort.Strings(rootIDs)

	for i := 0; i < len(rootIDs); i++ {
		source := rootIDs[i]
		if _, ok := adjacency[source]; !ok {
			continue
		}

		parents, distance := topologyShortestParents(adjacency, source)
		for j := i + 1; j < len(rootIDs); j++ {
			target := rootIDs[j]
			if _, ok := distance[target]; !ok {
				continue
			}

			visited := make(map[string]struct{})
			stack := []string{target}
			for len(stack) > 0 {
				node := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				if _, seen := visited[node]; seen {
					continue
				}
				visited[node] = struct{}{}
				includedActors[node] = struct{}{}
				if node == source {
					continue
				}

				for _, parent := range parents[node] {
					includedActors[parent] = struct{}{}
					includedPairs[topologyActorPairKey(node, parent)] = struct{}{}
					stack = append(stack, parent)
				}
			}
		}
	}

	return includedActors, includedPairs
}

func topologyShortestParents(
	adjacency map[string]map[string]struct{},
	source string,
) (map[string][]string, map[string]int) {
	parents := make(map[string][]string)
	distance := map[string]int{source: 0}
	queue := []string{source}

	for head := 0; head < len(queue); head++ {
		current := queue[head]
		neighbors := make([]string, 0, len(adjacency[current]))
		for neighbor := range adjacency[current] {
			neighbors = append(neighbors, neighbor)
		}
		sort.Strings(neighbors)
		for _, neighbor := range neighbors {
			nextDepth := distance[current] + 1
			currentDepth, seen := distance[neighbor]
			if !seen {
				distance[neighbor] = nextDepth
				parents[neighbor] = []string{current}
				queue = append(queue, neighbor)
				continue
			}
			if nextDepth == currentDepth {
				parents[neighbor] = append(parents[neighbor], current)
			}
		}
	}

	for node := range parents {
		sort.Strings(parents[node])
	}

	return parents, distance
}

func topologyActorPairKey(left, right string) string {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == "" || right == "" {
		return ""
	}
	if left > right {
		left, right = right, left
	}
	return left + "|" + right
}

func topologyActorHasIP(actor topologyActor, ip string) bool {
	ip = normalizeIPAddress(ip)
	if ip == "" {
		return false
	}
	for _, candidate := range normalizedMatchIPs(actor.Match) {
		if candidate == ip {
			return true
		}
	}
	if ip == normalizeIPAddress(topologyMetricValueString(actor.Attributes, "management_ip")) {
		return true
	}
	if raw, ok := actor.Attributes["management_addresses"]; ok {
		switch values := raw.(type) {
		case []string:
			for _, value := range values {
				if ip == normalizeIPAddress(value) {
					return true
				}
			}
		case []any:
			for _, value := range values {
				if ip == normalizeIPAddress(fmt.Sprint(value)) {
					return true
				}
			}
		}
	}
	return false
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
