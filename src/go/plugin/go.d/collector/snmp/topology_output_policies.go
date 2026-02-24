// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

func applySNMPTopologyOutputPolicies(data *topologyData, options topologyQueryOptions) {
	if data == nil {
		return
	}

	collapsed := 0
	if options.CollapseActorsByIP {
		collapsed = collapseActorsByIP(data)
	}

	removedNonIP := 0
	removedSparseSegments := 0
	if options.EliminateNonIPInferred {
		removedNonIP = eliminateNonIPInferredActors(data)
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
	data.Stats["actors_total"] = len(data.Actors)
	data.Stats["links_total"] = len(data.Links)
	data.Stats["actors_collapsed_by_ip"] = collapsed
	data.Stats["actors_non_ip_inferred_suppressed"] = removedNonIP
	data.Stats["segments_sparse_suppressed"] = removedSparseSegments
	if removedSparseSegments > 0 {
		data.Stats["segments_suppressed"] = intStatValue(data.Stats["segments_suppressed"]) + removedSparseSegments
	}
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
	if leftDevice, rightDevice := strings.EqualFold(strings.TrimSpace(left.ActorType), "device"), strings.EqualFold(strings.TrimSpace(right.ActorType), "device"); leftDevice != rightDevice {
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
