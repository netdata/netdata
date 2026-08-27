// SPDX-License-Identifier: GPL-3.0-or-later

package projector

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
	"github.com/netdata/netdata/go/plugins/pkg/topology/worklimit"
)

type topologyDisplayNameResolver struct {
	lookup func(ip string) string
	cache  map[string]string
	work   *projectionWork
}

type topologyDisplayName struct {
	name   string
	source string
}

func applyTopologyDisplayNames(actors []projectedActor, links []graph.Link, lookup func(ip string) string) {
	applyTopologyDisplayNamesWithWork(nil, actors, links, lookup)
}

func applyTopologyDisplayNamesWithWork(work *projectionWork, actors []projectedActor, links []graph.Link, lookup func(ip string) string) {
	if !work.chargeProduct(uint64(len(actors)), 3) {
		return
	}
	resolver := topologyDisplayNameResolver{
		lookup: lookup,
		cache:  make(map[string]string),
		work:   work,
	}

	deviceDisplayByID := make(map[string]string, len(actors))
	deviceDisplayByActorHandle := make(map[graph.ActorHandle]string, len(actors))
	displayByMatchKey := make(map[string]string, len(actors))

	// First pass: materialize display names for non-segment actors so segment naming can reuse them.
	for i := range actors {
		if actors[i].Actor.ActorType == "segment" {
			continue
		}
		if !work.charge(1) {
			return
		}
		display := topologyActorDisplayNameWithWork(work, actors[i], nil, &resolver)
		if work.failure() != nil {
			return
		}
		if display.name == "" {
			display = topologyFallbackActorDisplayNameWithWork(work, actors[i])
		}
		if !topologySetActorDisplayWithWork(work, &actors[i], display) {
			return
		}
		if IsDeviceActorType(actors[i].Actor.ActorType) {
			if handle := actors[i].Actor.ActorHandle; !handle.IsZero() {
				deviceDisplayByActorHandle[handle] = display.name
			}
			if deviceID := topologyActorDeviceID(actors[i]); deviceID != "" {
				deviceDisplayByID[deviceID] = display.name
			}
		} else {
			if matchKey := canonicalTopologyMatchKeyWithWork(work, actors[i].Actor.Match); matchKey != "" {
				displayByMatchKey[matchKey] = display.name
			}
			if work.failure() != nil {
				return
			}
		}
	}

	// Second pass: segment display names depend on finalized device display names.
	for i := range actors {
		if actors[i].Actor.ActorType != "segment" {
			continue
		}
		if !work.charge(1) {
			return
		}
		display := topologyActorDisplayNameWithWork(work, actors[i], deviceDisplayByID, &resolver)
		if work.failure() != nil {
			return
		}
		if display.name == "" {
			display = topologyFallbackActorDisplayNameWithWork(work, actors[i])
		}
		if !topologySetActorDisplayWithWork(work, &actors[i], display) {
			return
		}
		if matchKey := canonicalTopologyMatchKeyWithWork(work, actors[i].Actor.Match); matchKey != "" {
			displayByMatchKey[matchKey] = display.name
		}
		if work.failure() != nil {
			return
		}
	}

	for i := range links {
		if !work.charge(1) {
			return
		}
		src := topologyEndpointDisplayNameWithWork(work, links[i].Src, links[i].SrcActorHandle, deviceDisplayByActorHandle, displayByMatchKey, &resolver)
		if work.failure() != nil {
			return
		}
		if src.name == "" {
			src = topologyDisplayName{name: "[unset]", source: "fallback"}
		}
		srcPortName := topologySetEndpointDisplayAndCanonicalPortNameWithWork(work, &links[i].Src, src)
		if work.failure() != nil {
			return
		}

		dst := topologyEndpointDisplayNameWithWork(work, links[i].Dst, links[i].DstActorHandle, deviceDisplayByActorHandle, displayByMatchKey, &resolver)
		if work.failure() != nil {
			return
		}
		if dst.name == "" {
			dst = topologyDisplayName{name: "[unset]", source: "fallback"}
		}
		dstPortName := topologySetEndpointDisplayAndCanonicalPortNameWithWork(work, &links[i].Dst, dst)
		if work.failure() != nil {
			return
		}

		linkName := topologyCanonicalLinkName(src.name, srcPortName, dst.name, dstPortName)
		links[i].Display = &graph.LinkDisplay{
			Name:        linkName,
			SrcPortName: srcPortName,
			DstPortName: dstPortName,
		}
	}
}

func chargeProjectionStringMapWork(work *projectionWork, labels map[string]string) bool {
	if work == nil || len(labels) == 0 {
		return true
	}
	if !work.charge(uint64(len(labels))) {
		return false
	}
	var bytes uint64
	for key, value := range labels {
		var err error
		bytes, err = worklimit.Sum(bytes, uint64(len(key)), uint64(len(value)))
		if err != nil {
			work.err = err
			return false
		}
	}
	return work.charge(bytes)
}

func topologySetActorDisplay(actor *projectedActor, display topologyDisplayName) {
	topologySetActorDisplayWithWork(nil, actor, display)
}

func topologySetActorDisplayWithWork(work *projectionWork, actor *projectedActor, display topologyDisplayName) bool {
	if actor == nil {
		return true
	}
	if work != nil && (!chargeProjectionStringMapWork(work, actor.Actor.Labels) ||
		!work.chargeStrings([]string{display.name, display.source})) {
		return false
	}
	labels := cloneStringMap(actor.Actor.Labels)
	if labels == nil {
		labels = make(map[string]string)
	}
	labels["display_name"] = display.name
	if display.source != "" {
		labels["display_source"] = display.source
	}
	actor.Actor.Labels = labels
	actor.Detail.DisplayName = strings.TrimSpace(display.name)
	actor.Detail.DisplaySource = strings.TrimSpace(display.source)
	return true
}

func topologySetEndpointDisplayAndCanonicalPortName(endpoint *graph.LinkEndpoint, display topologyDisplayName) string {
	return topologySetEndpointDisplayAndCanonicalPortNameWithWork(nil, endpoint, display)
}

func topologySetEndpointDisplayAndCanonicalPortNameWithWork(
	work *projectionWork,
	endpoint *graph.LinkEndpoint,
	display topologyDisplayName,
) string {
	if endpoint == nil {
		return ""
	}
	if work != nil && !work.chargeStrings([]string{
		display.name, display.source,
		endpoint.IfName, endpoint.PortID, endpoint.PortName,
	}) {
		return ""
	}
	endpoint.DisplayName = strings.TrimSpace(display.name)
	endpoint.DisplaySource = strings.TrimSpace(display.source)
	name := topologyEndpointCanonicalPortName(*endpoint)
	endpoint.PortName = name
	return name
}

func topologyEndpointDisplayName(
	endpoint graph.LinkEndpoint,
	actorHandle graph.ActorHandle,
	deviceDisplayByActorHandle map[graph.ActorHandle]string,
	actorDisplayByMatch map[string]string,
	resolver *topologyDisplayNameResolver,
) topologyDisplayName {
	return topologyEndpointDisplayNameWithWork(nil, endpoint, actorHandle, deviceDisplayByActorHandle, actorDisplayByMatch, resolver)
}

func topologyEndpointDisplayNameWithWork(
	work *projectionWork,
	endpoint graph.LinkEndpoint,
	actorHandle graph.ActorHandle,
	deviceDisplayByActorHandle map[graph.ActorHandle]string,
	actorDisplayByMatch map[string]string,
	resolver *topologyDisplayNameResolver,
) topologyDisplayName {
	if !actorHandle.IsZero() {
		if name := strings.TrimSpace(deviceDisplayByActorHandle[actorHandle]); name != "" {
			return topologyDisplayName{name: name, source: "actor"}
		}
	}
	if key := canonicalTopologyMatchKeyWithWork(work, endpoint.Match); key != "" {
		if name := strings.TrimSpace(actorDisplayByMatch[key]); name != "" {
			return topologyDisplayName{name: name, source: "actor"}
		}
	}
	if work.failure() != nil {
		return topologyDisplayName{}
	}
	return topologyDisplayNameFromMatchWithWork(work, endpoint.Match, resolver)
}

func topologyActorDisplayName(actor projectedActor, deviceDisplayByID map[string]string, resolver *topologyDisplayNameResolver) topologyDisplayName {
	return topologyActorDisplayNameWithWork(nil, actor, deviceDisplayByID, resolver)
}

func topologyActorDisplayNameWithWork(
	work *projectionWork,
	actor projectedActor,
	deviceDisplayByID map[string]string,
	resolver *topologyDisplayNameResolver,
) topologyDisplayName {
	if work != nil && !work.chargeStrings([]string{
		actor.Actor.ActorType,
		actor.Detail.Device.ManagementIP,
		actor.Detail.Segment.SegmentID,
	}) {
		return topologyDisplayName{}
	}
	if actor.Actor.ActorType == "segment" {
		if name := topologySegmentDisplayNameWithWork(work, actor, deviceDisplayByID); name != "" {
			return topologyDisplayName{name: name, source: "segment"}
		}
		if work.failure() != nil {
			return topologyDisplayName{}
		}
	}

	displayMatch := actor.Actor.Match
	if IsDeviceActorType(actor.Actor.ActorType) {
		displayMatch = topologyDeviceDisplayMatch(displayMatch, actor.Detail.Device.ManagementIP)
	}
	display := topologyDisplayNameFromMatchWithWork(work, displayMatch, resolver)
	if display.name != "" {
		return display
	}
	if work.failure() != nil {
		return topologyDisplayName{}
	}

	if segmentID := strings.TrimSpace(actor.Detail.Segment.SegmentID); segmentID != "" {
		return topologyDisplayName{name: topologyCompactSegmentID(segmentID), source: "segment_id"}
	}
	return topologyDisplayName{}
}

func topologyDeviceDisplayMatch(match graph.Match, managementIP string) graph.Match {
	match.IPAddresses = nil
	if ip := normalizeTopologyIP(managementIP); ip != "" {
		match.IPAddresses = []string{ip}
	}
	return match
}

func topologyFallbackActorDisplayName(actor projectedActor) topologyDisplayName {
	return topologyFallbackActorDisplayNameWithWork(nil, actor)
}

func topologyFallbackActorDisplayNameWithWork(work *projectionWork, actor projectedActor) topologyDisplayName {
	if work != nil && !work.chargeStrings([]string{
		actor.Actor.ActorType,
		actor.Detail.Device.ManagementIP,
		actor.Detail.Segment.SegmentID,
	}) {
		return topologyDisplayName{}
	}
	match := actor.Actor.Match
	if IsDeviceActorType(actor.Actor.ActorType) {
		match = topologyDeviceDisplayMatch(match, actor.Detail.Device.ManagementIP)
	}
	if matchKey := canonicalTopologyMatchKeyWithWork(work, match); matchKey != "" {
		return topologyDisplayName{name: matchKey, source: "fallback_match"}
	}
	if work.failure() != nil {
		return topologyDisplayName{}
	}
	if segmentID := strings.TrimSpace(actor.Detail.Segment.SegmentID); segmentID != "" {
		return topologyDisplayName{name: topologyCompactSegmentID(segmentID), source: "segment_id"}
	}
	actorType := strings.TrimSpace(actor.Actor.ActorType)
	if actorType == "" {
		actorType = "actor"
	}
	return topologyDisplayName{name: actorType + ":[unset]", source: "fallback"}
}

func topologyActorDeviceID(actor projectedActor) string {
	return strings.TrimSpace(actor.Detail.Device.DeviceID)
}

func topologyDisplayNameFromMatch(match graph.Match, resolver *topologyDisplayNameResolver) topologyDisplayName {
	return topologyDisplayNameFromMatchWithWork(nil, match, resolver)
}

func topologyDisplayNameFromMatchWithWork(
	work *projectionWork,
	match graph.Match,
	resolver *topologyDisplayNameResolver,
) topologyDisplayName {
	if dns := topologyMatchPreferredDNSNameWithWork(work, match, resolver); dns != "" {
		return topologyDisplayName{name: dns, source: "dns"}
	}
	if work.failure() != nil {
		return topologyDisplayName{}
	}
	if work != nil && !work.chargeStrings([]string{match.SysName}) {
		return topologyDisplayName{}
	}
	if sysName := topologyMatchPreferredSysName(match); sysName != "" {
		return topologyDisplayName{name: sysName, source: "sys_name"}
	}
	if hostname := topologyMatchPreferredHostnameWithWork(work, match); hostname != "" {
		return topologyDisplayName{name: hostname, source: "hostname"}
	}
	if work.failure() != nil {
		return topologyDisplayName{}
	}
	if ip := topologyMatchPreferredIPWithWork(work, match); ip != "" {
		return topologyDisplayName{name: ip, source: "ip"}
	}
	if work.failure() != nil {
		return topologyDisplayName{}
	}
	if mac := topologyMatchPreferredMACWithWork(work, match); mac != "" {
		return topologyDisplayName{name: mac, source: "mac"}
	}
	return topologyDisplayName{}
}

func topologyMatchPreferredDNSName(match graph.Match, resolver *topologyDisplayNameResolver) string {
	return topologyMatchPreferredDNSNameWithWork(nil, match, resolver)
}

func topologyMatchPreferredDNSNameWithWork(
	work *projectionWork,
	match graph.Match,
	resolver *topologyDisplayNameResolver,
) string {
	if !work.chargeStrings(match.DNSNames) || !work.chargeStrings(match.IPAddresses) ||
		!work.chargeProduct(uint64(len(match.DNSNames)+len(match.IPAddresses)), 2) {
		return ""
	}
	candidates := make(map[string]struct{})
	for _, value := range match.DNSNames {
		if normalized := normalizeDNSName(value); normalized != "" {
			candidates[normalized] = struct{}{}
		}
	}
	for _, value := range match.IPAddresses {
		if resolver == nil {
			continue
		}
		if ip := normalizeTopologyIP(value); ip != "" {
			if resolved := resolver.resolve(ip); resolved != "" {
				candidates[resolved] = struct{}{}
			}
		}
	}
	names := sortedTopologySetWithWork(work, candidates)
	if len(names) == 0 {
		return ""
	}
	return names[0]
}

func topologyMatchPreferredSysName(match graph.Match) string {
	return strings.TrimSpace(match.SysName)
}

func topologyMatchPreferredHostname(match graph.Match) string {
	return topologyMatchPreferredHostnameWithWork(nil, match)
}

func topologyMatchPreferredHostnameWithWork(work *projectionWork, match graph.Match) string {
	hostnames := uniqueTopologyStringsWithWork(work, match.Hostnames)
	if len(hostnames) == 0 {
		return ""
	}
	return hostnames[0]
}

func topologyMatchPreferredIP(match graph.Match) string {
	return topologyMatchPreferredIPWithWork(nil, match)
}

func topologyMatchPreferredIPWithWork(work *projectionWork, match graph.Match) string {
	if !work.chargeStrings(match.IPAddresses) {
		return ""
	}
	ips := make([]string, 0, len(match.IPAddresses))
	for _, value := range match.IPAddresses {
		if ip := normalizeTopologyIP(value); ip != "" {
			ips = append(ips, ip)
		}
	}
	ips = uniqueTopologyStringsWithWork(work, ips)
	if len(ips) == 0 {
		return ""
	}
	return ips[0]
}

func topologyMatchPreferredMAC(match graph.Match) string {
	return topologyMatchPreferredMACWithWork(nil, match)
}

func topologyMatchPreferredMACWithWork(work *projectionWork, match graph.Match) string {
	if !work.chargeStrings(match.MacAddresses) || !work.chargeStrings(match.ChassisIDs) {
		return ""
	}
	macs := make([]string, 0, len(match.MacAddresses)+len(match.ChassisIDs))
	for _, value := range match.MacAddresses {
		if mac := normalizeMAC(value); mac != "" {
			macs = append(macs, mac)
		}
	}
	for _, value := range match.ChassisIDs {
		if mac := normalizeMAC(value); mac != "" {
			macs = append(macs, mac)
		}
	}
	macs = uniqueTopologyStringsWithWork(work, macs)
	if len(macs) == 0 {
		return ""
	}
	return macs[0]
}

func normalizeDNSName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.TrimSuffix(name, ".")
	if name == "" {
		return ""
	}
	return strings.ToLower(name)
}

func (r *topologyDisplayNameResolver) resolve(ip string) string {
	if r == nil || r.lookup == nil {
		return ""
	}
	if r.work != nil && (!r.work.chargeStrings([]string{ip}) || !r.work.charge(1)) {
		return ""
	}
	ip = normalizeTopologyIP(ip)
	if ip == "" {
		return ""
	}
	if name, ok := r.cache[ip]; ok {
		return name
	}
	name := normalizeDNSName(r.lookup(ip))
	r.cache[ip] = name
	return name
}
