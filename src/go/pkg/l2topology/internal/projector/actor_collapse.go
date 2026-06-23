// SPDX-License-Identifier: GPL-3.0-or-later

package projector

import (
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
)

func collapseActorsByIP(actors []projectedActor) []projectedActor {
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
		if strings.EqualFold(strings.TrimSpace(actor.Actor.ActorType), "segment") {
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
			merged.Actor.Match = mergeTopologyActorMatch(merged.Actor.Match, actors[idx].Actor.Match)
			merged.Actor.Labels = mergeTopologyActorLabels(merged.Actor.Labels, actors[idx].Actor.Labels)
			merged.Detail = mergeProjectionActorDetail(merged.Detail, actors[idx].Detail)
			keep[idx] = false
		}
		if collapsedCount > 1 {
			merged.Detail.CollapsedByIP = true
			merged.Detail.CollapsedCount = collapsedCount
		}
		actors[rep] = merged
	}

	out := make([]projectedActor, 0, len(actors))
	for idx, actor := range actors {
		if !keep[idx] {
			continue
		}
		out = append(out, actor)
	}
	return out
}

func eliminateNonIPInferredActors(actors []projectedActor, links []graph.Link) ([]projectedActor, []graph.Link) {
	if len(actors) == 0 {
		return actors, links
	}
	removedIdentityKeys := make(map[string]struct{})
	filteredActors := make([]projectedActor, 0, len(actors))
	for _, actor := range actors {
		if topologyActorIsInferred(actor) && len(normalizedTopologyActorIPs(actor)) == 0 {
			for _, key := range topologyMatchIdentityKeys(actor.Actor.Match) {
				removedIdentityKeys[key] = struct{}{}
			}
			continue
		}
		filteredActors = append(filteredActors, actor)
	}
	if len(removedIdentityKeys) == 0 {
		return actors, links
	}

	filteredLinks := make([]graph.Link, 0, len(links))
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

func normalizedTopologyActorIPs(actor projectedActor) []string {
	if len(actor.Actor.Match.IPAddresses) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(actor.Actor.Match.IPAddresses))
	out := make([]string, 0, len(actor.Actor.Match.IPAddresses))
	for _, value := range actor.Actor.Match.IPAddresses {
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

func compareTopologyActorCollapsePriority(left, right projectedActor) int {
	leftDevice := IsDeviceActorType(left.Actor.ActorType)
	rightDevice := IsDeviceActorType(right.Actor.ActorType)
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
	leftKey := canonicalTopologyMatchKey(left.Actor.Match)
	rightKey := canonicalTopologyMatchKey(right.Actor.Match)
	return strings.Compare(leftKey, rightKey)
}

func mergeTopologyActorMatch(base, other graph.Match) graph.Match {
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

func mergeProjectionActorDetail(base, extra ProjectionActorDetail) ProjectionActorDetail {
	base.Device = mergeProjectionDeviceActorDetail(base.Device, extra.Device)
	base.Endpoint = mergeProjectionEndpointActorDetail(base.Endpoint, extra.Endpoint)
	base.Segment = mergeProjectionSegmentActorDetail(base.Segment, extra.Segment)
	if strings.TrimSpace(base.DisplayName) == "" {
		base.DisplayName = strings.TrimSpace(extra.DisplayName)
	}
	if strings.TrimSpace(base.DisplaySource) == "" {
		base.DisplaySource = strings.TrimSpace(extra.DisplaySource)
	}
	return base
}

func mergeProjectionDeviceActorDetail(base, extra ProjectionDeviceActorDetail) ProjectionDeviceActorDetail {
	if strings.TrimSpace(base.DeviceID) == "" {
		base.DeviceID = strings.TrimSpace(extra.DeviceID)
	}
	base.Discovered = base.Discovered || extra.Discovered
	base.Inferred = base.Inferred || extra.Inferred
	if strings.TrimSpace(base.ManagementIP) == "" {
		base.ManagementIP = strings.TrimSpace(extra.ManagementIP)
	}
	base.ManagementAddresses = mergeTopologyStringLists(base.ManagementAddresses, extra.ManagementAddresses)
	base.Protocols = mergeTopologyStringLists(base.Protocols, extra.Protocols)
	base.ProtocolsCollected = mergeTopologyStringLists(base.ProtocolsCollected, extra.ProtocolsCollected)
	base.Capabilities = mergeTopologyStringLists(base.Capabilities, extra.Capabilities)
	base.CapabilitiesSupported = mergeTopologyStringLists(base.CapabilitiesSupported, extra.CapabilitiesSupported)
	base.CapabilitiesEnabled = mergeTopologyStringLists(base.CapabilitiesEnabled, extra.CapabilitiesEnabled)
	if strings.TrimSpace(base.Vendor) == "" {
		base.Vendor = strings.TrimSpace(extra.Vendor)
	}
	if strings.TrimSpace(base.VendorSource) == "" {
		base.VendorSource = strings.TrimSpace(extra.VendorSource)
	}
	if strings.TrimSpace(base.VendorConfidence) == "" {
		base.VendorConfidence = strings.TrimSpace(extra.VendorConfidence)
	}
	if strings.TrimSpace(base.VendorDerived) == "" {
		base.VendorDerived = strings.TrimSpace(extra.VendorDerived)
	}
	if strings.TrimSpace(base.VendorDerivedSource) == "" {
		base.VendorDerivedSource = strings.TrimSpace(extra.VendorDerivedSource)
	}
	if strings.TrimSpace(base.VendorDerivedConfidence) == "" {
		base.VendorDerivedConfidence = strings.TrimSpace(extra.VendorDerivedConfidence)
	}
	if strings.TrimSpace(base.VendorDerivedMatchPrefix) == "" {
		base.VendorDerivedMatchPrefix = strings.TrimSpace(extra.VendorDerivedMatchPrefix)
	}
	if strings.TrimSpace(base.VendorMatchPrefix) == "" {
		base.VendorMatchPrefix = strings.TrimSpace(extra.VendorMatchPrefix)
	}
	base.IfIndexes = mergeTopologyStringLists(base.IfIndexes, extra.IfIndexes)
	base.IfNames = mergeTopologyStringLists(base.IfNames, extra.IfNames)
	if !base.PortsTotal.Has {
		base.PortsTotal = extra.PortsTotal
	}
	if !base.PortsUp.Has {
		base.PortsUp = extra.PortsUp
	}
	if !base.PortsDown.Has {
		base.PortsDown = extra.PortsDown
	}
	if !base.PortsAdminDown.Has {
		base.PortsAdminDown = extra.PortsAdminDown
	}
	if !base.TotalBandwidthBps.Has {
		base.TotalBandwidthBps = extra.TotalBandwidthBps
	}
	if !base.FDBTotalMACs.Has {
		base.FDBTotalMACs = extra.FDBTotalMACs
	}
	if !base.VLANCount.Has {
		base.VLANCount = extra.VLANCount
	}
	if !base.LLDPNeighborCount.Has {
		base.LLDPNeighborCount = extra.LLDPNeighborCount
	}
	if !base.CDPNeighborCount.Has {
		base.CDPNeighborCount = extra.CDPNeighborCount
	}
	base.AdminStatusCounts = mergeTopologyIntMaps(base.AdminStatusCounts, extra.AdminStatusCounts)
	base.OperStatusCounts = mergeTopologyIntMaps(base.OperStatusCounts, extra.OperStatusCounts)
	base.LinkModeCounts = mergeTopologyIntMaps(base.LinkModeCounts, extra.LinkModeCounts)
	base.TopologyRoleCounts = mergeTopologyIntMaps(base.TopologyRoleCounts, extra.TopologyRoleCounts)
	if len(base.Ports) == 0 {
		base.Ports = cloneProjectionPortDetails(extra.Ports)
	}
	return base
}

func mergeProjectionEndpointActorDetail(base, extra ProjectionEndpointActorDetail) ProjectionEndpointActorDetail {
	base.LearnedSources = mergeTopologyStringLists(base.LearnedSources, extra.LearnedSources)
	base.LearnedDeviceIDs = mergeTopologyStringLists(base.LearnedDeviceIDs, extra.LearnedDeviceIDs)
	base.LearnedIfIndexes = mergeTopologyStringLists(base.LearnedIfIndexes, extra.LearnedIfIndexes)
	base.LearnedIfNames = mergeTopologyStringLists(base.LearnedIfNames, extra.LearnedIfNames)
	if !base.Discovered {
		base.Discovered = extra.Discovered
	}
	if strings.TrimSpace(base.Vendor) == "" {
		base.Vendor = strings.TrimSpace(extra.Vendor)
	}
	if strings.TrimSpace(base.VendorSource) == "" {
		base.VendorSource = strings.TrimSpace(extra.VendorSource)
	}
	if strings.TrimSpace(base.VendorConfidence) == "" {
		base.VendorConfidence = strings.TrimSpace(extra.VendorConfidence)
	}
	if strings.TrimSpace(base.VendorMatchPrefix) == "" {
		base.VendorMatchPrefix = strings.TrimSpace(extra.VendorMatchPrefix)
	}
	if strings.TrimSpace(base.VendorDerived) == "" {
		base.VendorDerived = strings.TrimSpace(extra.VendorDerived)
	}
	if strings.TrimSpace(base.VendorDerivedSource) == "" {
		base.VendorDerivedSource = strings.TrimSpace(extra.VendorDerivedSource)
	}
	if strings.TrimSpace(base.VendorDerivedConfidence) == "" {
		base.VendorDerivedConfidence = strings.TrimSpace(extra.VendorDerivedConfidence)
	}
	if strings.TrimSpace(base.VendorDerivedMatchPrefix) == "" {
		base.VendorDerivedMatchPrefix = strings.TrimSpace(extra.VendorDerivedMatchPrefix)
	}
	if strings.TrimSpace(base.AttachmentSource) == "" {
		base.AttachmentSource = strings.TrimSpace(extra.AttachmentSource)
		base.AttachedDeviceID = strings.TrimSpace(extra.AttachedDeviceID)
		base.AttachedDevice = strings.TrimSpace(extra.AttachedDevice)
		base.AttachedPort = strings.TrimSpace(extra.AttachedPort)
		base.AttachedIfName = strings.TrimSpace(extra.AttachedIfName)
		base.AttachedIfIndex = extra.AttachedIfIndex
		base.AttachedBridgePort = strings.TrimSpace(extra.AttachedBridgePort)
		base.AttachedVLAN = strings.TrimSpace(extra.AttachedVLAN)
		base.AttachedVLANID = strings.TrimSpace(extra.AttachedVLANID)
		base.AttachedBy = strings.TrimSpace(extra.AttachedBy)
	}
	return base
}

func mergeProjectionSegmentActorDetail(base, extra ProjectionSegmentActorDetail) ProjectionSegmentActorDetail {
	if strings.TrimSpace(base.SegmentID) == "" {
		base.SegmentID = strings.TrimSpace(extra.SegmentID)
	}
	if strings.TrimSpace(base.SegmentType) == "" {
		base.SegmentType = strings.TrimSpace(extra.SegmentType)
	}
	if strings.TrimSpace(base.SegmentKind) == "" {
		base.SegmentKind = strings.TrimSpace(extra.SegmentKind)
	}
	base.ParentDevices = mergeTopologyStringLists(base.ParentDevices, extra.ParentDevices)
	base.IfNames = mergeTopologyStringLists(base.IfNames, extra.IfNames)
	base.IfIndexes = mergeTopologyStringLists(base.IfIndexes, extra.IfIndexes)
	base.BridgePorts = mergeTopologyStringLists(base.BridgePorts, extra.BridgePorts)
	base.VLANIDs = mergeTopologyStringLists(base.VLANIDs, extra.VLANIDs)
	base.LearnedSources = mergeTopologyStringLists(base.LearnedSources, extra.LearnedSources)
	if !base.PortsTotal.Has {
		base.PortsTotal = extra.PortsTotal
	}
	if !base.EndpointsTotal.Has {
		base.EndpointsTotal = extra.EndpointsTotal
	}
	if strings.TrimSpace(base.DesignatedPort) == "" {
		base.DesignatedPort = strings.TrimSpace(extra.DesignatedPort)
	}
	return base
}

func mergeTopologyIntMaps(base, extra map[string]int) map[string]int {
	if len(extra) == 0 {
		return base
	}
	if base == nil {
		base = make(map[string]int, len(extra))
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

func topologyActorIsInferred(actor projectedActor) bool {
	if strings.EqualFold(strings.TrimSpace(actor.Actor.ActorType), "endpoint") {
		return true
	}
	if actor.Detail.Device.Inferred {
		return true
	}
	return false
}
