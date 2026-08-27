// SPDX-License-Identifier: GPL-3.0-or-later

package projector

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/l2topology/internal/model"
	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
)

type topologyMatchLookup struct {
	canonical    string
	identityKeys []string
}

type topologyActorRef struct {
	actorID     string
	actorHandle graph.ActorHandle
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
	return canonicalTopologyMatchKeyWithWork(nil, match)
}

func canonicalTopologyMatchKeyWithWork(work *projectionWork, match graph.Match) string {
	if key := canonicalTopologyPrimaryMACKeyWithWork(work, match); key != "" {
		return "mac:" + key
	}
	if key := canonicalTopologyHardwareKeyWithWork(work, match.ChassisIDs); key != "" {
		return "chassis:" + key
	}
	if key := canonicalTopologyIPListKeyWithWork(work, match.IPAddresses); key != "" {
		return "ip:" + key
	}
	if key := canonicalTopologyStringListKeyWithWork(work, match.Hostnames); key != "" {
		return "hostname:" + key
	}
	if key := canonicalTopologyStringListKeyWithWork(work, match.DNSNames); key != "" {
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

func assignTopologyActorIDsAndLinkEndpoints(
	actors []projectedActor,
	links []graph.Link,
) {
	assignTopologyActorIDsAndLinkEndpointsWithWork(nil, actors, links)
}

func assignTopologyActorIDsAndLinkEndpointsWithWork(
	work *projectionWork,
	actors []projectedActor,
	links []graph.Link,
) {
	if len(actors) == 0 {
		return
	}
	if !work.charge(uint64(len(actors))) || !work.charge(uint64(len(links))) {
		return
	}

	usedActorIDs := make(map[string]int, len(actors))
	actorRefByCanonicalMatch := make(map[string]topologyActorRef, len(actors))
	actorRefByIdentityKey := make(map[string]topologyActorRef, len(actors)*4)
	actorLookups := make([]topologyMatchLookup, len(actors))
	handles := graph.NewActorHandleAllocator()

	for i := range actors {
		actorLookups[i] = newTopologyMatchLookup(work, actors[i].Actor.Match)

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
		actors[i].Actor.ActorHandle = handles.Next()
		actorRef := topologyActorRef{actorID: actorID, actorHandle: actors[i].Actor.ActorHandle}

		if actorLookups[i].canonical != "" {
			if _, exists := actorRefByCanonicalMatch[actorLookups[i].canonical]; !exists {
				actorRefByCanonicalMatch[actorLookups[i].canonical] = actorRef
			}
		}
		for _, key := range actorLookups[i].identityKeys {
			if _, exists := actorRefByIdentityKey[key]; !exists {
				actorRefByIdentityKey[key] = actorRef
			}
		}
	}

	for i := range links {
		srcLookup := newTopologyMatchLookup(work, links[i].Src.Match)
		dstLookup := newTopologyMatchLookup(work, links[i].Dst.Match)
		srcRef := resolveTopologyEndpointActorRef(srcLookup, actorRefByCanonicalMatch, actorRefByIdentityKey)
		dstRef := resolveTopologyEndpointActorRef(dstLookup, actorRefByCanonicalMatch, actorRefByIdentityKey)
		links[i].SrcActorID = srcRef.actorID
		links[i].SrcActorHandle = srcRef.actorHandle
		links[i].DstActorID = dstRef.actorID
		links[i].DstActorHandle = dstRef.actorHandle
	}
}

func newTopologyMatchLookup(work *projectionWork, match graph.Match) topologyMatchLookup {
	return topologyMatchLookup{
		canonical:    canonicalTopologyMatchKeyWithWork(work, match),
		identityKeys: topologyMatchIdentityKeysWithWork(work, match),
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

func resolveTopologyEndpointActorRef(lookup topologyMatchLookup, byCanonicalMatch map[string]topologyActorRef, byIdentityKey map[string]topologyActorRef) topologyActorRef {
	if lookup.canonical != "" {
		if ref := byCanonicalMatch[lookup.canonical]; ref.actorID != "" {
			return ref
		}
	}
	for _, key := range lookup.identityKeys {
		if ref := byIdentityKey[key]; ref.actorID != "" {
			return ref
		}
	}
	return topologyActorRef{}
}

func enrichTopologyPortDetailsWithLinkCounts(
	actors []projectedActor,
	links []graph.Link,
) {
	enrichTopologyPortDetailsWithLinkCountsWithWork(nil, actors, links)
}

func enrichTopologyPortDetailsWithLinkCountsWithWork(
	work *projectionWork,
	actors []projectedActor,
	links []graph.Link,
) {
	type actorPort struct {
		actorHandle graph.ActorHandle
		portName    string
	}
	if !work.chargeProduct(uint64(len(links)), 2) {
		return
	}
	counts := make(map[actorPort]int, len(links)*2)

	for _, link := range links {
		if !link.SrcActorHandle.IsZero() {
			name := strings.TrimSpace(link.Src.IfName)
			if name != "" {
				counts[actorPort{link.SrcActorHandle, name}]++
			}
		}
		if !link.DstActorHandle.IsZero() {
			name := strings.TrimSpace(link.Dst.IfName)
			if name != "" {
				counts[actorPort{link.DstActorHandle, name}]++
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
			if c := counts[actorPort{actors[i].Actor.ActorHandle, name}]; c > 0 {
				ports[j].LinkCount = model.OptionalValue[int]{Value: c, Has: true}
			}
		}
		actors[i].Detail.Device.Ports = ports
	}
}

func canonicalTopologyPrimaryMACKey(match graph.Match) string {
	return canonicalTopologyPrimaryMACKeyWithWork(nil, match)
}

func canonicalTopologyPrimaryMACKeyWithWork(work *projectionWork, match graph.Match) string {
	if !work.chargeStrings(match.MacAddresses) || !work.chargeStrings(match.ChassisIDs) {
		return ""
	}
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
	var keys []string
	if work == nil {
		keys = make([]string, 0, len(set))
	}
	keys = sortedProjectionKeys(work, set, keys)
	if len(keys) == 0 {
		return ""
	}
	return strings.Join(keys, ",")
}

func canonicalTopologyHardwareKey(values []string) string {
	return canonicalTopologyHardwareKeyWithWork(nil, values)
}

func canonicalTopologyHardwareKeyWithWork(work *projectionWork, values []string) string {
	if len(values) == 0 {
		return ""
	}
	if !work.chargeStrings(values) {
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
	if !sortProjectionStrings(work, out) {
		return ""
	}
	out = uniqueTopologyStringsWithWork(work, out)
	return strings.Join(out, ",")
}

func canonicalTopologyIPListKey(values []string) string {
	return canonicalTopologyIPListKeyWithWork(nil, values)
}

func canonicalTopologyIPListKeyWithWork(work *projectionWork, values []string) string {
	if len(values) == 0 {
		return ""
	}
	if !work.chargeStrings(values) {
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
	if !sortProjectionStrings(work, out) {
		return ""
	}
	out = uniqueTopologyStringsWithWork(work, out)
	return strings.Join(out, ",")
}

func canonicalTopologyStringListKey(values []string) string {
	return canonicalTopologyStringListKeyWithWork(nil, values)
}

func canonicalTopologyStringListKeyWithWork(work *projectionWork, values []string) string {
	if len(values) == 0 {
		return ""
	}
	if !work.chargeStrings(values) {
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
	if !sortProjectionStrings(work, out) {
		return ""
	}
	out = uniqueTopologyStringsWithWork(work, out)
	return strings.Join(out, ",")
}

func topologyLinkSortKey(link graph.Link) string {
	return topologyLinkSortKeyWithWork(nil, link)
}

func topologyLinkSortKeyWithWork(work *projectionWork, link graph.Link) string {
	parts := []string{
		link.Protocol,
		link.Direction,
		canonicalTopologyMatchKeyWithWork(work, link.Src.Match),
		canonicalTopologyMatchKeyWithWork(work, link.Dst.Match),
		topologyEndpointKey(link.Src, "if_index"),
		topologyEndpointKey(link.Src, "if_name"),
		topologyEndpointKey(link.Src, "port_id"),
		topologyEndpointKey(link.Dst, "if_index"),
		topologyEndpointKey(link.Dst, "if_name"),
		topologyEndpointKey(link.Dst, "port_id"),
		link.State,
	}
	if !work.chargeStrings(parts) {
		return ""
	}
	return strings.Join(parts, keySep)
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
	return topologyActorSortKeyWithWork(nil, actor)
}

func topologyActorSortKeyWithWork(work *projectionWork, actor graph.Actor) string {
	parts := []string{
		actor.ActorType,
		canonicalTopologyMatchKeyWithWork(work, actor.Match),
		actor.Source,
		actor.Layer,
	}
	if !work.chargeStrings(parts) {
		return ""
	}
	return strings.Join(parts, keySep)
}

func sortProjectedTopologyActors(actors []projectedActor) {
	sortProjectedTopologyActorsWithWork(nil, actors)
}

func sortProjectedTopologyActorsWithWork(work *projectionWork, actors []projectedActor) {
	if len(actors) < 2 {
		return
	}
	if !work.charge(uint64(len(actors))) {
		return
	}

	entries := make([]projectedTopologyActorSortEntry, len(actors))
	var maxKeyBytes uint64
	for i := range actors {
		entries[i] = projectedTopologyActorSortEntry{
			actor: actors[i],
			key:   topologyActorSortKeyWithWork(work, actors[i].Actor),
		}
		if size := uint64(len(entries[i].key)); size > maxKeyBytes {
			maxKeyBytes = size
		}
		if work != nil && work.err != nil {
			return
		}
	}

	if !sortProjectionSliceStableWithStringWork(work, entries, maxKeyBytes, func(i, j int) bool {
		return entries[i].key < entries[j].key
	}) {
		return
	}

	for i := range entries {
		actors[i] = entries[i].actor
	}
}

func sortTopologyLinks(links []graph.Link) {
	sortTopologyLinksWithWork(nil, links)
}

func sortTopologyLinksWithWork(work *projectionWork, links []graph.Link) {
	if len(links) < 2 {
		return
	}
	if !work.charge(uint64(len(links))) {
		return
	}

	entries := make([]topologyLinkSortEntry, len(links))
	var maxKeyBytes uint64
	for i := range links {
		entries[i] = topologyLinkSortEntry{
			link: links[i],
			key:  topologyLinkSortKeyWithWork(work, links[i]),
		}
		if size := uint64(len(entries[i].key)); size > maxKeyBytes {
			maxKeyBytes = size
		}
		if work != nil && work.err != nil {
			return
		}
	}

	if !sortProjectionSliceStableWithStringWork(work, entries, maxKeyBytes, func(i, j int) bool {
		return entries[i].key < entries[j].key
	}) {
		return
	}

	for i := range entries {
		links[i] = entries[i].link
	}
}
