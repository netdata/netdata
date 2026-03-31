// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"sort"
	"strings"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/topology/engine"
)

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
