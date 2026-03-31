// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/topology"
)

type topologyDisplayNameResolver struct {
	lookup func(ip string) string
	cache  map[string]string
}

type topologyDisplayName struct {
	name   string
	source string
}

func applyTopologyDisplayNames(actors []topology.Actor, links []topology.Link, lookup func(ip string) string) {
	resolver := topologyDisplayNameResolver{
		lookup: lookup,
		cache:  make(map[string]string),
	}

	deviceDisplayByID := make(map[string]string, len(actors))
	displayByMatchKey := make(map[string]string, len(actors))

	// First pass: materialize display names for non-segment actors so segment naming can reuse them.
	for i := range actors {
		if actors[i].ActorType == "segment" {
			continue
		}
		display := topologyActorDisplayName(actors[i], nil, &resolver)
		if display.name == "" {
			display = topologyFallbackActorDisplayName(actors[i])
		}
		topologySetActorDisplay(&actors[i], display)
		if matchKey := canonicalTopologyMatchKey(actors[i].Match); matchKey != "" {
			displayByMatchKey[matchKey] = display.name
		}
		if IsDeviceActorType(actors[i].ActorType) {
			if deviceID := topologyActorDeviceID(actors[i]); deviceID != "" {
				deviceDisplayByID[deviceID] = display.name
			}
		}
	}

	// Second pass: segment display names depend on finalized device display names.
	for i := range actors {
		if actors[i].ActorType != "segment" {
			continue
		}
		display := topologyActorDisplayName(actors[i], deviceDisplayByID, &resolver)
		if display.name == "" {
			display = topologyFallbackActorDisplayName(actors[i])
		}
		topologySetActorDisplay(&actors[i], display)
		if matchKey := canonicalTopologyMatchKey(actors[i].Match); matchKey != "" {
			displayByMatchKey[matchKey] = display.name
		}
	}

	for i := range links {
		src := topologyEndpointDisplayName(links[i].Src, displayByMatchKey, &resolver)
		if src.name == "" {
			src = topologyDisplayName{name: "[unset]", source: "fallback"}
		}
		topologySetEndpointDisplay(&links[i].Src, src)
		srcPortName := topologySetEndpointCanonicalPortName(&links[i].Src)

		dst := topologyEndpointDisplayName(links[i].Dst, displayByMatchKey, &resolver)
		if dst.name == "" {
			dst = topologyDisplayName{name: "[unset]", source: "fallback"}
		}
		topologySetEndpointDisplay(&links[i].Dst, dst)
		dstPortName := topologySetEndpointCanonicalPortName(&links[i].Dst)

		linkName := topologyCanonicalLinkName(src.name, srcPortName, dst.name, dstPortName)
		if links[i].Metrics == nil {
			links[i].Metrics = make(map[string]any)
		}
		links[i].Metrics["display_name"] = linkName
		links[i].Metrics["src_port_name"] = srcPortName
		links[i].Metrics["dst_port_name"] = dstPortName
	}
}

func topologySetActorDisplay(actor *topology.Actor, display topologyDisplayName) {
	if actor == nil {
		return
	}
	labels := cloneStringMap(actor.Labels)
	if labels == nil {
		labels = make(map[string]string)
	}
	labels["display_name"] = display.name
	if display.source != "" {
		labels["display_source"] = display.source
	}
	actor.Labels = labels

	attrs := cloneAnyMap(actor.Attributes)
	if attrs == nil {
		attrs = make(map[string]any)
	}
	attrs["display_name"] = display.name
	if display.source != "" {
		attrs["display_source"] = display.source
	}
	actor.Attributes = pruneTopologyAttributes(attrs)
}

func topologySetEndpointDisplay(endpoint *topology.LinkEndpoint, display topologyDisplayName) {
	if endpoint == nil {
		return
	}
	attrs := cloneAnyMap(endpoint.Attributes)
	if attrs == nil {
		attrs = make(map[string]any)
	}
	attrs["display_name"] = display.name
	if display.source != "" {
		attrs["display_source"] = display.source
	}
	endpoint.Attributes = pruneTopologyAttributes(attrs)
}

func topologySetEndpointCanonicalPortName(endpoint *topology.LinkEndpoint) string {
	if endpoint == nil {
		return ""
	}
	attrs := cloneAnyMap(endpoint.Attributes)
	if attrs == nil {
		attrs = make(map[string]any)
	}
	name := topologyCanonicalPortName(attrs)
	attrs["port_name"] = name
	endpoint.Attributes = pruneTopologyAttributes(attrs)
	return name
}

func topologyEndpointDisplayName(endpoint topology.LinkEndpoint, actorDisplayByMatch map[string]string, resolver *topologyDisplayNameResolver) topologyDisplayName {
	if key := canonicalTopologyMatchKey(endpoint.Match); key != "" {
		if name := strings.TrimSpace(actorDisplayByMatch[key]); name != "" {
			return topologyDisplayName{name: name, source: "actor"}
		}
	}
	return topologyDisplayNameFromMatch(endpoint.Match, resolver)
}

func topologyActorDisplayName(actor topology.Actor, deviceDisplayByID map[string]string, resolver *topologyDisplayNameResolver) topologyDisplayName {
	if actor.ActorType == "segment" {
		if name := topologySegmentDisplayName(actor, deviceDisplayByID); name != "" {
			return topologyDisplayName{name: name, source: "segment"}
		}
	}

	display := topologyDisplayNameFromMatch(actor.Match, resolver)
	if display.name != "" {
		return display
	}

	if segmentID := topologyAttrString(actor.Attributes, "segment_id"); segmentID != "" {
		return topologyDisplayName{name: topologyCompactSegmentID(segmentID), source: "segment_id"}
	}
	return topologyDisplayName{}
}

func topologyFallbackActorDisplayName(actor topology.Actor) topologyDisplayName {
	if matchKey := canonicalTopologyMatchKey(actor.Match); matchKey != "" {
		return topologyDisplayName{name: matchKey, source: "fallback_match"}
	}
	if segmentID := topologyAttrString(actor.Attributes, "segment_id"); segmentID != "" {
		return topologyDisplayName{name: topologyCompactSegmentID(segmentID), source: "segment_id"}
	}
	actorType := strings.TrimSpace(actor.ActorType)
	if actorType == "" {
		actorType = "actor"
	}
	return topologyDisplayName{name: actorType + ":[unset]", source: "fallback"}
}

func topologyActorDeviceID(actor topology.Actor) string {
	return topologyAttrString(actor.Attributes, "device_id")
}

func topologyDisplayNameFromMatch(match topology.Match, resolver *topologyDisplayNameResolver) topologyDisplayName {
	if dns := topologyMatchPreferredDNSName(match, resolver); dns != "" {
		return topologyDisplayName{name: dns, source: "dns"}
	}
	if sysName := topologyMatchPreferredSysName(match); sysName != "" {
		return topologyDisplayName{name: sysName, source: "sys_name"}
	}
	if hostname := topologyMatchPreferredHostname(match); hostname != "" {
		return topologyDisplayName{name: hostname, source: "hostname"}
	}
	if ip := topologyMatchPreferredIP(match); ip != "" {
		return topologyDisplayName{name: ip, source: "ip"}
	}
	if mac := topologyMatchPreferredMAC(match); mac != "" {
		return topologyDisplayName{name: mac, source: "mac"}
	}
	return topologyDisplayName{}
}

func topologyMatchPreferredDNSName(match topology.Match, resolver *topologyDisplayNameResolver) string {
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
	names := sortedTopologySet(candidates)
	if len(names) == 0 {
		return ""
	}
	return names[0]
}

func topologyMatchPreferredSysName(match topology.Match) string {
	return strings.TrimSpace(match.SysName)
}

func topologyMatchPreferredHostname(match topology.Match) string {
	hostnames := uniqueTopologyStrings(match.Hostnames)
	if len(hostnames) == 0 {
		return ""
	}
	return hostnames[0]
}

func topologyMatchPreferredIP(match topology.Match) string {
	ips := make([]string, 0, len(match.IPAddresses))
	for _, value := range match.IPAddresses {
		if ip := normalizeTopologyIP(value); ip != "" {
			ips = append(ips, ip)
		}
	}
	ips = uniqueTopologyStrings(ips)
	if len(ips) == 0 {
		return ""
	}
	return ips[0]
}

func topologyMatchPreferredMAC(match topology.Match) string {
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
	macs = uniqueTopologyStrings(macs)
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
