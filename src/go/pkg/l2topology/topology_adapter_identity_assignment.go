// SPDX-License-Identifier: GPL-3.0-or-later

package l2topology

import (
	"fmt"
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
)

type topologyMatchLookup struct {
	canonical    string
	identityKeys []string
}

type projectedTopologyActorSortEntry struct {
	actor projectedActor
	key   string
}

type topologyLinkSortEntry struct {
	link graph.Link
	key  string
}

func canonicalTopologyMatchKey(match graph.Match) string {
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

func assignTopologyActorIDsAndLinkEndpoints(actors []projectedActor, links []graph.Link) {
	if len(actors) == 0 {
		return
	}

	usedActorIDs := make(map[string]int, len(actors))
	actorIDByCanonicalMatch := make(map[string]string, len(actors))
	actorIDByIdentityKey := make(map[string]string, len(actors)*4)
	actorLookups := make([]topologyMatchLookup, len(actors))

	for i := range actors {
		actorLookups[i] = newTopologyMatchLookup(actors[i].Actor.Match)

		baseID := actorLookups[i].canonical
		if baseID == "" {
			actorType := strings.ToLower(strings.TrimSpace(actors[i].Actor.ActorType))
			if actorType == "" {
				actorType = "actor"
			}
			baseID = "generated:" + actorType
		}

		actorID := responseScopedActorID(baseID, usedActorIDs)
		actors[i].Actor.ActorID = actorID

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

func newTopologyMatchLookup(match graph.Match) topologyMatchLookup {
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

func enrichTopologyPortDetailsWithLinkCounts(actors []projectedActor, links []graph.Link) {
	type actorPort struct {
		actorID  string
		portName string
	}
	counts := make(map[actorPort]int, len(links)*2)

	for _, link := range links {
		if link.SrcActorID != "" {
			name := strings.TrimSpace(link.Src.IfName)
			if name != "" {
				counts[actorPort{link.SrcActorID, name}]++
			}
		}
		if link.DstActorID != "" {
			name := strings.TrimSpace(link.Dst.IfName)
			if name != "" {
				counts[actorPort{link.DstActorID, name}]++
			}
		}
	}

	for i := range actors {
		ports := actors[i].Detail.Device.Ports
		if len(ports) == 0 {
			continue
		}
		for j := range ports {
			name := strings.TrimSpace(firstNonEmpty(ports[j].Name, ports[j].IfName, ports[j].PortID))
			if name == "" {
				continue
			}
			if c := counts[actorPort{actors[i].Actor.ActorID, name}]; c > 0 {
				ports[j].LinkCount = OptionalValue[int]{Value: c, Has: true}
			}
		}
		actors[i].Detail.Device.Ports = ports
	}
}

func canonicalTopologyPrimaryMACKey(match graph.Match) string {
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

func topologyLinkSortKey(link graph.Link) string {
	return strings.Join([]string{
		link.Protocol,
		link.Direction,
		canonicalTopologyMatchKey(link.Src.Match),
		canonicalTopologyMatchKey(link.Dst.Match),
		topologyEndpointKey(link.Src, "if_index"),
		topologyEndpointKey(link.Src, "if_name"),
		topologyEndpointKey(link.Src, "port_id"),
		topologyEndpointKey(link.Dst, "if_index"),
		topologyEndpointKey(link.Dst, "if_name"),
		topologyEndpointKey(link.Dst, "port_id"),
		link.State,
	}, keySep)
}

func topologyEndpointKey(endpoint graph.LinkEndpoint, key string) string {
	switch key {
	case "if_index":
		if endpoint.IfIndex > 0 {
			return fmt.Sprint(endpoint.IfIndex)
		}
	case "if_name":
		return strings.TrimSpace(endpoint.IfName)
	case "port_id":
		return strings.TrimSpace(endpoint.PortID)
	}
	return ""
}

func topologyActorSortKey(actor graph.Actor) string {
	return strings.Join([]string{
		actor.ActorType,
		canonicalTopologyMatchKey(actor.Match),
		actor.Source,
		actor.Layer,
	}, keySep)
}

func sortProjectedTopologyActors(actors []projectedActor) {
	if len(actors) < 2 {
		return
	}

	entries := make([]projectedTopologyActorSortEntry, len(actors))
	for i := range actors {
		entries[i] = projectedTopologyActorSortEntry{
			actor: actors[i],
			key:   topologyActorSortKey(actors[i].Actor),
		}
	}

	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].key < entries[j].key
	})

	for i := range entries {
		actors[i] = entries[i].actor
	}
}

func sortTopologyLinks(links []graph.Link) {
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
