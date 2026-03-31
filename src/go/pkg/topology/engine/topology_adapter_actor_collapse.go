// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/topology"
)

func collapseActorsByIP(actors []topology.Actor) []topology.Actor {
	if len(actors) <= 1 {
		return actors
	}

	parent := make([]int, len(actors))
	for i := range parent {
		parent[i] = i
	}
	find := func(x int) int {
		for parent[x] != x {
			parent[x] = parent[parent[x]]
			x = parent[x]
		}
		return x
	}
	union := func(a, b int) {
		ra := find(a)
		rb := find(b)
		if ra == rb {
			return
		}
		if ra < rb {
			parent[rb] = ra
			return
		}
		parent[ra] = rb
	}

	ipOwner := make(map[string]int)
	for idx, actor := range actors {
		if strings.EqualFold(strings.TrimSpace(actor.ActorType), "segment") {
			continue
		}
		ips := normalizedTopologyActorIPs(actor)
		if len(ips) == 0 {
			continue
		}
		for _, ip := range ips {
			if owner, ok := ipOwner[ip]; ok {
				union(idx, owner)
				continue
			}
			ipOwner[ip] = idx
		}
	}

	groups := make(map[int][]int)
	for idx := range actors {
		root := find(idx)
		groups[root] = append(groups[root], idx)
	}

	keep := make([]bool, len(actors))
	for i := range keep {
		keep[i] = true
	}
	for _, members := range groups {
		if len(members) <= 1 {
			continue
		}
		rep := members[0]
		for _, idx := range members[1:] {
			if compareTopologyActorCollapsePriority(actors[idx], actors[rep]) < 0 {
				rep = idx
			}
		}
		merged := actors[rep]
		collapsedCount := 1
		for _, idx := range members {
			if idx == rep {
				continue
			}
			collapsedCount++
			merged.Match = mergeTopologyActorMatch(merged.Match, actors[idx].Match)
			merged.Labels = mergeTopologyActorLabels(merged.Labels, actors[idx].Labels)
			merged.Attributes = mergeTopologyActorAttributes(merged.Attributes, actors[idx].Attributes)
			keep[idx] = false
		}
		if collapsedCount > 1 {
			if merged.Attributes == nil {
				merged.Attributes = make(map[string]any)
			}
			merged.Attributes["collapsed_by_ip"] = true
			merged.Attributes["collapsed_count"] = collapsedCount
		}
		actors[rep] = merged
	}

	out := make([]topology.Actor, 0, len(actors))
	for idx, actor := range actors {
		if !keep[idx] {
			continue
		}
		out = append(out, actor)
	}
	return out
}

func eliminateNonIPInferredActors(actors []topology.Actor, links []topology.Link) ([]topology.Actor, []topology.Link) {
	if len(actors) == 0 {
		return actors, links
	}
	removedIdentityKeys := make(map[string]struct{})
	filteredActors := make([]topology.Actor, 0, len(actors))
	for _, actor := range actors {
		if topologyActorIsInferred(actor) && len(normalizedTopologyActorIPs(actor)) == 0 {
			for _, key := range topologyMatchIdentityKeys(actor.Match) {
				removedIdentityKeys[key] = struct{}{}
			}
			continue
		}
		filteredActors = append(filteredActors, actor)
	}
	if len(removedIdentityKeys) == 0 {
		return actors, links
	}

	filteredLinks := make([]topology.Link, 0, len(links))
	for _, link := range links {
		srcKeys := topologyMatchIdentityKeys(link.Src.Match)
		dstKeys := topologyMatchIdentityKeys(link.Dst.Match)
		if topologyIdentityKeysOverlap(srcKeys, removedIdentityKeys) {
			continue
		}
		if topologyIdentityKeysOverlap(dstKeys, removedIdentityKeys) {
			continue
		}
		filteredLinks = append(filteredLinks, link)
	}
	return filteredActors, filteredLinks
}

func topologyIdentityKeysOverlap(keys []string, set map[string]struct{}) bool {
	if len(keys) == 0 || len(set) == 0 {
		return false
	}
	for _, key := range keys {
		if _, ok := set[key]; ok {
			return true
		}
	}
	return false
}

func normalizedTopologyActorIPs(actor topology.Actor) []string {
	if len(actor.Match.IPAddresses) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(actor.Match.IPAddresses))
	out := make([]string, 0, len(actor.Match.IPAddresses))
	for _, value := range actor.Match.IPAddresses {
		ip := normalizeTopologyIP(value)
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

func compareTopologyActorCollapsePriority(left, right topology.Actor) int {
	leftDevice := IsDeviceActorType(left.ActorType)
	rightDevice := IsDeviceActorType(right.ActorType)
	if leftDevice != rightDevice {
		if leftDevice {
			return -1
		}
		return 1
	}
	leftInferred := topologyActorIsInferred(left)
	rightInferred := topologyActorIsInferred(right)
	if leftInferred != rightInferred {
		if !leftInferred {
			return -1
		}
		return 1
	}
	leftKey := canonicalTopologyMatchKey(left.Match)
	rightKey := canonicalTopologyMatchKey(right.Match)
	return strings.Compare(leftKey, rightKey)
}

func mergeTopologyActorMatch(base, other topology.Match) topology.Match {
	base.ChassisIDs = mergeTopologyStringLists(base.ChassisIDs, other.ChassisIDs)
	base.MacAddresses = mergeTopologyStringLists(base.MacAddresses, other.MacAddresses)
	base.IPAddresses = mergeTopologyStringLists(base.IPAddresses, other.IPAddresses)
	base.Hostnames = mergeTopologyStringLists(base.Hostnames, other.Hostnames)
	base.DNSNames = mergeTopologyStringLists(base.DNSNames, other.DNSNames)
	if strings.TrimSpace(base.SysName) == "" {
		base.SysName = strings.TrimSpace(other.SysName)
	}
	if strings.TrimSpace(base.SysObjectID) == "" {
		base.SysObjectID = strings.TrimSpace(other.SysObjectID)
	}
	return base
}

func mergeTopologyStringLists(base []string, extra []string) []string {
	seen := make(map[string]struct{}, len(base)+len(extra))
	out := make([]string, 0, len(base)+len(extra))
	for _, value := range append(base, extra...) {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	if len(out) == 0 {
		return nil
	}
	return out
}

func mergeTopologyActorLabels(base, extra map[string]string) map[string]string {
	if len(extra) == 0 {
		return base
	}
	if base == nil {
		base = make(map[string]string, len(extra))
	}
	for key, value := range extra {
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

func mergeTopologyActorAttributes(base, extra map[string]any) map[string]any {
	if len(extra) == 0 {
		return base
	}
	if base == nil {
		base = make(map[string]any, len(extra))
	}
	for key, value := range extra {
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

func topologyActorIsInferred(actor topology.Actor) bool {
	if strings.EqualFold(strings.TrimSpace(actor.ActorType), "endpoint") {
		return true
	}
	if topologyAnyBoolValue(actor.Attributes["inferred"]) {
		return true
	}
	if len(actor.Labels) > 0 {
		if topologyAnyBoolValue(actor.Labels["inferred"]) {
			return true
		}
	}
	return false
}

func topologyAnyBoolValue(value any) bool {
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
