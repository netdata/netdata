// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"fmt"
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/topology"
)

type topologyMatchLookup struct {
	canonical    string
	identityKeys []string
}

type topologyActorSortEntry struct {
	actor topology.Actor
	key   string
}

type topologyLinkSortEntry struct {
	link topology.Link
	key  string
}

func canonicalTopologyMatchKey(match topology.Match) string {
	if key := canonicalTopologyPrimaryMACKey(match); key != "" {
		return "mac:" + key
	}
	if key := canonicalTopologyHardwareKey(match.ChassisIDs); key != "" {
		return "chassis:" + key
	}
	if key := canonicalTopologyIPListKey(match.IPAddresses); key != "" {
		return "ip:" + key
	}
	if key := canonicalTopologyStringListKey(match.Hostnames); key != "" {
		return "hostname:" + key
	}
	if key := canonicalTopologyStringListKey(match.DNSNames); key != "" {
		return "dns:" + key
	}
	if sysName := strings.ToLower(strings.TrimSpace(match.SysName)); sysName != "" {
		return "sysname:" + sysName
	}
	if match.SysObjectID != "" {
		return "sysobjectid:" + match.SysObjectID
	}
	return ""
}

func assignTopologyActorIDsAndLinkEndpoints(actors []topology.Actor, links []topology.Link) {
	if len(actors) == 0 {
		return
	}

	usedActorIDs := make(map[string]int, len(actors))
	actorIDByCanonicalMatch := make(map[string]string, len(actors))
	actorIDByIdentityKey := make(map[string]string, len(actors)*4)
	actorLookups := make([]topologyMatchLookup, len(actors))

	for i := range actors {
		actorLookups[i] = newTopologyMatchLookup(actors[i].Match)

		baseID := actorLookups[i].canonical
		if baseID == "" {
			actorType := strings.ToLower(strings.TrimSpace(actors[i].ActorType))
			if actorType == "" {
				actorType = "actor"
			}
			baseID = "generated:" + actorType
		}

		actorID := responseScopedActorID(baseID, usedActorIDs)
		actors[i].ActorID = actorID

		if actorLookups[i].canonical != "" {
			if _, exists := actorIDByCanonicalMatch[actorLookups[i].canonical]; !exists {
				actorIDByCanonicalMatch[actorLookups[i].canonical] = actorID
			}
		}
		for _, key := range actorLookups[i].identityKeys {
			if _, exists := actorIDByIdentityKey[key]; !exists {
				actorIDByIdentityKey[key] = actorID
			}
		}
	}

	for i := range links {
		srcLookup := newTopologyMatchLookup(links[i].Src.Match)
		dstLookup := newTopologyMatchLookup(links[i].Dst.Match)
		links[i].SrcActorID = resolveTopologyEndpointActorID(srcLookup, actorIDByCanonicalMatch, actorIDByIdentityKey)
		links[i].DstActorID = resolveTopologyEndpointActorID(dstLookup, actorIDByCanonicalMatch, actorIDByIdentityKey)
	}
}

func newTopologyMatchLookup(match topology.Match) topologyMatchLookup {
	return topologyMatchLookup{
		canonical:    canonicalTopologyMatchKey(match),
		identityKeys: topologyMatchIdentityKeys(match),
	}
}

func responseScopedActorID(base string, used map[string]int) string {
	base = strings.ToLower(strings.TrimSpace(base))
	if base == "" {
		base = "generated:actor"
	}

	count := used[base]
	count++
	used[base] = count
	if count == 1 {
		return base
	}
	return fmt.Sprintf("%s#%d", base, count)
}

func resolveTopologyEndpointActorID(lookup topologyMatchLookup, byCanonicalMatch map[string]string, byIdentityKey map[string]string) string {
	if lookup.canonical != "" {
		if actorID := strings.TrimSpace(byCanonicalMatch[lookup.canonical]); actorID != "" {
			return actorID
		}
	}
	for _, key := range lookup.identityKeys {
		if actorID := strings.TrimSpace(byIdentityKey[key]); actorID != "" {
			return actorID
		}
	}
	return ""
}

func enrichTopologyPortTablesWithLinkCounts(actors []topology.Actor, links []topology.Link) {
	type actorPort struct {
		actorID  string
		portName string
	}
	counts := make(map[actorPort]int, len(links)*2)

	for _, link := range links {
		if link.SrcActorID != "" {
			if ifName, ok := link.Src.Attributes["if_name"]; ok {
				name := strings.TrimSpace(fmt.Sprintf("%v", ifName))
				if name != "" {
					counts[actorPort{link.SrcActorID, name}]++
				}
			}
		}
		if link.DstActorID != "" {
			if ifName, ok := link.Dst.Attributes["if_name"]; ok {
				name := strings.TrimSpace(fmt.Sprintf("%v", ifName))
				if name != "" {
					counts[actorPort{link.DstActorID, name}]++
				}
			}
		}
	}

	for i := range actors {
		portRows := actors[i].Tables["ports"]
		if len(portRows) == 0 {
			continue
		}
		for j := range portRows {
			name := strings.TrimSpace(fmt.Sprintf("%v", portRows[j]["name"]))
			if name == "" {
				continue
			}
			if c := counts[actorPort{actors[i].ActorID, name}]; c > 0 {
				portRows[j]["link_count"] = c
			}
		}
	}
}

func canonicalTopologyPrimaryMACKey(match topology.Match) string {
	set := make(map[string]struct{}, len(match.MacAddresses)+len(match.ChassisIDs))
	for _, value := range match.MacAddresses {
		if mac := normalizeMAC(value); mac != "" {
			set[mac] = struct{}{}
		}
	}
	for _, value := range match.ChassisIDs {
		if mac := normalizeMAC(value); mac != "" {
			set[mac] = struct{}{}
		}
	}
	if len(set) == 0 {
		return ""
	}
	keys := sortedTopologySet(set)
	if len(keys) == 0 {
		return ""
	}
	return strings.Join(keys, ",")
}

func canonicalTopologyHardwareKey(values []string) string {
	if len(values) == 0 {
		return ""
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if mac := normalizeMAC(value); mac != "" {
			out = append(out, mac)
			continue
		}
		if ip := normalizeTopologyIP(value); ip != "" {
			out = append(out, ip)
			continue
		}
		out = append(out, strings.ToLower(value))
	}
	if len(out) == 0 {
		return ""
	}
	sort.Strings(out)
	out = uniqueTopologyStrings(out)
	return strings.Join(out, ",")
}

func canonicalTopologyIPListKey(values []string) string {
	if len(values) == 0 {
		return ""
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if ip := normalizeTopologyIP(value); ip != "" {
			out = append(out, ip)
			continue
		}
		out = append(out, strings.ToLower(value))
	}
	if len(out) == 0 {
		return ""
	}
	sort.Strings(out)
	out = uniqueTopologyStrings(out)
	return strings.Join(out, ",")
}

func canonicalTopologyStringListKey(values []string) string {
	if len(values) == 0 {
		return ""
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	if len(out) == 0 {
		return ""
	}
	sort.Strings(out)
	out = uniqueTopologyStrings(out)
	return strings.Join(out, ",")
}

func topologyLinkSortKey(link topology.Link) string {
	return strings.Join([]string{
		link.Protocol,
		link.Direction,
		canonicalTopologyMatchKey(link.Src.Match),
		canonicalTopologyMatchKey(link.Dst.Match),
		topologyAttrKey(link.Src.Attributes, "if_index"),
		topologyAttrKey(link.Src.Attributes, "if_name"),
		topologyAttrKey(link.Src.Attributes, "port_id"),
		topologyAttrKey(link.Dst.Attributes, "if_index"),
		topologyAttrKey(link.Dst.Attributes, "if_name"),
		topologyAttrKey(link.Dst.Attributes, "port_id"),
		link.State,
	}, keySep)
}

func topologyActorSortKey(actor topology.Actor) string {
	return strings.Join([]string{
		actor.ActorType,
		canonicalTopologyMatchKey(actor.Match),
		actor.Source,
		actor.Layer,
	}, keySep)
}

func topologyAttrKey(attrs map[string]any, key string) string {
	if len(attrs) == 0 {
		return ""
	}
	value, ok := attrs[key]
	if !ok || value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

func sortTopologyActors(actors []topology.Actor) {
	if len(actors) < 2 {
		return
	}

	entries := make([]topologyActorSortEntry, len(actors))
	for i := range actors {
		entries[i] = topologyActorSortEntry{
			actor: actors[i],
			key:   topologyActorSortKey(actors[i]),
		}
	}

	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].key < entries[j].key
	})

	for i := range entries {
		actors[i] = entries[i].actor
	}
}

func sortTopologyLinks(links []topology.Link) {
	if len(links) < 2 {
		return
	}

	entries := make([]topologyLinkSortEntry, len(links))
	for i := range links {
		entries[i] = topologyLinkSortEntry{
			link: links[i],
			key:  topologyLinkSortKey(links[i]),
		}
	}

	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].key < entries[j].key
	})

	for i := range entries {
		links[i] = entries[i].link
	}
}
