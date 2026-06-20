// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"strings"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/l2topology"
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
			repActor.Detail = mergeTopologyActorDetail(repActor.Detail, data.Actors[idx].Detail)
			keep[idx] = false
		}
		if collapsedCount > 1 {
			repActor.Detail.L2.CollapsedByIP = true
			repActor.Detail.L2.CollapsedCount = collapsedCount
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

func mergeTopologyActorDetail(dst, src topologyActorDetail) topologyActorDetail {
	dst.L2 = mergeTopologyProjectionActorDetail(dst.L2, src.L2)
	dst.SNMP = mergeTopologySNMPActorDetail(dst.SNMP, src.SNMP)
	dst.OSPF = append(dst.OSPF, src.OSPF...)
	dst.BGP = append(dst.BGP, src.BGP...)
	return dst
}

func mergeTopologyProjectionActorDetail(dst, src topologyengine.ProjectionActorDetail) topologyengine.ProjectionActorDetail {
	dst.DisplayName = firstNonEmptyString(dst.DisplayName, src.DisplayName)
	dst.DisplaySource = firstNonEmptyString(dst.DisplaySource, src.DisplaySource)
	dst.Device = mergeTopologyProjectionDeviceDetail(dst.Device, src.Device)
	dst.Endpoint = mergeTopologyProjectionEndpointDetail(dst.Endpoint, src.Endpoint)
	dst.Segment = mergeTopologyProjectionSegmentDetail(dst.Segment, src.Segment)
	if !dst.CollapsedByIP {
		dst.CollapsedByIP = src.CollapsedByIP
	}
	if dst.CollapsedCount == 0 {
		dst.CollapsedCount = src.CollapsedCount
	}
	return dst
}

func mergeTopologyProjectionDeviceDetail(dst, src topologyengine.ProjectionDeviceActorDetail) topologyengine.ProjectionDeviceActorDetail {
	dst.HasInventoryStats = dst.HasInventoryStats || src.HasInventoryStats
	dst.DeviceID = firstNonEmptyString(dst.DeviceID, src.DeviceID)
	dst.Discovered = dst.Discovered || src.Discovered
	dst.Inferred = dst.Inferred || src.Inferred
	dst.ManagementIP = firstNonEmptyString(dst.ManagementIP, src.ManagementIP)
	dst.ManagementAddresses = firstNonEmptyStringSlice(dst.ManagementAddresses, src.ManagementAddresses)
	dst.Protocols = firstNonEmptyStringSlice(dst.Protocols, src.Protocols)
	dst.ProtocolsCollected = firstNonEmptyStringSlice(dst.ProtocolsCollected, src.ProtocolsCollected)
	dst.Capabilities = firstNonEmptyStringSlice(dst.Capabilities, src.Capabilities)
	dst.CapabilitiesSupported = firstNonEmptyStringSlice(dst.CapabilitiesSupported, src.CapabilitiesSupported)
	dst.CapabilitiesEnabled = firstNonEmptyStringSlice(dst.CapabilitiesEnabled, src.CapabilitiesEnabled)
	dst.Vendor = firstNonEmptyString(dst.Vendor, src.Vendor)
	dst.VendorSource = firstNonEmptyString(dst.VendorSource, src.VendorSource)
	dst.VendorConfidence = firstNonEmptyString(dst.VendorConfidence, src.VendorConfidence)
	dst.VendorDerived = firstNonEmptyString(dst.VendorDerived, src.VendorDerived)
	dst.VendorDerivedSource = firstNonEmptyString(dst.VendorDerivedSource, src.VendorDerivedSource)
	dst.VendorDerivedConfidence = firstNonEmptyString(dst.VendorDerivedConfidence, src.VendorDerivedConfidence)
	dst.VendorDerivedMatchPrefix = firstNonEmptyString(dst.VendorDerivedMatchPrefix, src.VendorDerivedMatchPrefix)
	dst.VendorMatchPrefix = firstNonEmptyString(dst.VendorMatchPrefix, src.VendorMatchPrefix)
	dst.PortsTotal = firstNonZeroInt(dst.PortsTotal, src.PortsTotal)
	dst.IfIndexes = firstNonEmptyStringSlice(dst.IfIndexes, src.IfIndexes)
	dst.IfNames = firstNonEmptyStringSlice(dst.IfNames, src.IfNames)
	dst.PortsUp = firstNonZeroInt(dst.PortsUp, src.PortsUp)
	dst.PortsDown = firstNonZeroInt(dst.PortsDown, src.PortsDown)
	dst.PortsAdminDown = firstNonZeroInt(dst.PortsAdminDown, src.PortsAdminDown)
	dst.TotalBandwidthBps = firstNonZeroInt64(dst.TotalBandwidthBps, src.TotalBandwidthBps)
	dst.FDBTotalMACs = firstNonZeroInt(dst.FDBTotalMACs, src.FDBTotalMACs)
	dst.VLANCount = firstNonZeroInt(dst.VLANCount, src.VLANCount)
	dst.LLDPNeighborCount = firstNonZeroInt(dst.LLDPNeighborCount, src.LLDPNeighborCount)
	dst.CDPNeighborCount = firstNonZeroInt(dst.CDPNeighborCount, src.CDPNeighborCount)
	dst.AdminStatusCounts = firstNonEmptyIntMap(dst.AdminStatusCounts, src.AdminStatusCounts)
	dst.OperStatusCounts = firstNonEmptyIntMap(dst.OperStatusCounts, src.OperStatusCounts)
	dst.LinkModeCounts = firstNonEmptyIntMap(dst.LinkModeCounts, src.LinkModeCounts)
	dst.TopologyRoleCounts = firstNonEmptyIntMap(dst.TopologyRoleCounts, src.TopologyRoleCounts)
	if len(dst.Ports) == 0 {
		dst.Ports = src.Ports
	}
	return dst
}

func mergeTopologyProjectionEndpointDetail(dst, src topologyengine.ProjectionEndpointActorDetail) topologyengine.ProjectionEndpointActorDetail {
	dst.Discovered = dst.Discovered || src.Discovered
	dst.LearnedSources = firstNonEmptyStringSlice(dst.LearnedSources, src.LearnedSources)
	dst.LearnedDeviceIDs = firstNonEmptyStringSlice(dst.LearnedDeviceIDs, src.LearnedDeviceIDs)
	dst.LearnedIfIndexes = firstNonEmptyStringSlice(dst.LearnedIfIndexes, src.LearnedIfIndexes)
	dst.LearnedIfNames = firstNonEmptyStringSlice(dst.LearnedIfNames, src.LearnedIfNames)
	dst.Vendor = firstNonEmptyString(dst.Vendor, src.Vendor)
	dst.VendorSource = firstNonEmptyString(dst.VendorSource, src.VendorSource)
	dst.VendorConfidence = firstNonEmptyString(dst.VendorConfidence, src.VendorConfidence)
	dst.VendorMatchPrefix = firstNonEmptyString(dst.VendorMatchPrefix, src.VendorMatchPrefix)
	dst.VendorDerived = firstNonEmptyString(dst.VendorDerived, src.VendorDerived)
	dst.VendorDerivedSource = firstNonEmptyString(dst.VendorDerivedSource, src.VendorDerivedSource)
	dst.VendorDerivedConfidence = firstNonEmptyString(dst.VendorDerivedConfidence, src.VendorDerivedConfidence)
	dst.VendorDerivedMatchPrefix = firstNonEmptyString(dst.VendorDerivedMatchPrefix, src.VendorDerivedMatchPrefix)
	dst.AttachmentSource = firstNonEmptyString(dst.AttachmentSource, src.AttachmentSource)
	dst.AttachedDeviceID = firstNonEmptyString(dst.AttachedDeviceID, src.AttachedDeviceID)
	dst.AttachedDevice = firstNonEmptyString(dst.AttachedDevice, src.AttachedDevice)
	dst.AttachedPort = firstNonEmptyString(dst.AttachedPort, src.AttachedPort)
	dst.AttachedIfName = firstNonEmptyString(dst.AttachedIfName, src.AttachedIfName)
	dst.AttachedIfIndex = firstNonZeroInt(dst.AttachedIfIndex, src.AttachedIfIndex)
	dst.AttachedBridgePort = firstNonEmptyString(dst.AttachedBridgePort, src.AttachedBridgePort)
	dst.AttachedVLAN = firstNonEmptyString(dst.AttachedVLAN, src.AttachedVLAN)
	dst.AttachedVLANID = firstNonEmptyString(dst.AttachedVLANID, src.AttachedVLANID)
	dst.AttachedBy = firstNonEmptyString(dst.AttachedBy, src.AttachedBy)
	return dst
}

func mergeTopologyProjectionSegmentDetail(dst, src topologyengine.ProjectionSegmentActorDetail) topologyengine.ProjectionSegmentActorDetail {
	dst.HasStats = dst.HasStats || src.HasStats
	dst.SegmentID = firstNonEmptyString(dst.SegmentID, src.SegmentID)
	dst.SegmentType = firstNonEmptyString(dst.SegmentType, src.SegmentType)
	dst.ParentDevices = firstNonEmptyStringSlice(dst.ParentDevices, src.ParentDevices)
	dst.IfNames = firstNonEmptyStringSlice(dst.IfNames, src.IfNames)
	dst.IfIndexes = firstNonEmptyStringSlice(dst.IfIndexes, src.IfIndexes)
	dst.BridgePorts = firstNonEmptyStringSlice(dst.BridgePorts, src.BridgePorts)
	dst.VLANIDs = firstNonEmptyStringSlice(dst.VLANIDs, src.VLANIDs)
	dst.LearnedSources = firstNonEmptyStringSlice(dst.LearnedSources, src.LearnedSources)
	dst.PortsTotal = firstNonZeroInt(dst.PortsTotal, src.PortsTotal)
	dst.EndpointsTotal = firstNonZeroInt(dst.EndpointsTotal, src.EndpointsTotal)
	dst.DesignatedPort = firstNonEmptyString(dst.DesignatedPort, src.DesignatedPort)
	dst.SegmentKind = firstNonEmptyString(dst.SegmentKind, src.SegmentKind)
	return dst
}

func mergeTopologySNMPActorDetail(dst, src topologySNMPActorDetail) topologySNMPActorDetail {
	dst.ManagementAddresses = firstNonEmptyManagementAddresses(dst.ManagementAddresses, src.ManagementAddresses)
	dst.Capabilities = firstNonEmptyStringSlice(dst.Capabilities, src.Capabilities)
	dst.CapabilitiesSupported = firstNonEmptyStringSlice(dst.CapabilitiesSupported, src.CapabilitiesSupported)
	dst.CapabilitiesEnabled = firstNonEmptyStringSlice(dst.CapabilitiesEnabled, src.CapabilitiesEnabled)
	dst.SysDescr = firstNonEmptyString(dst.SysDescr, src.SysDescr)
	dst.SysContact = firstNonEmptyString(dst.SysContact, src.SysContact)
	dst.SysLocation = firstNonEmptyString(dst.SysLocation, src.SysLocation)
	dst.SysUptime = firstNonZeroInt64(dst.SysUptime, src.SysUptime)
	dst.Vendor = firstNonEmptyString(dst.Vendor, src.Vendor)
	dst.VendorSource = firstNonEmptyString(dst.VendorSource, src.VendorSource)
	dst.VendorConfidence = firstNonEmptyString(dst.VendorConfidence, src.VendorConfidence)
	dst.Model = firstNonEmptyString(dst.Model, src.Model)
	dst.OSPFRouterID = firstNonEmptyString(dst.OSPFRouterID, src.OSPFRouterID)
	dst.SerialNumber = firstNonEmptyString(dst.SerialNumber, src.SerialNumber)
	dst.SoftwareVersion = firstNonEmptyString(dst.SoftwareVersion, src.SoftwareVersion)
	dst.FirmwareVersion = firstNonEmptyString(dst.FirmwareVersion, src.FirmwareVersion)
	dst.HardwareVersion = firstNonEmptyString(dst.HardwareVersion, src.HardwareVersion)
	dst.ManagementIP = firstNonEmptyString(dst.ManagementIP, src.ManagementIP)
	dst.NetdataHostID = firstNonEmptyString(dst.NetdataHostID, src.NetdataHostID)
	dst.ChartIDPrefix = firstNonEmptyString(dst.ChartIDPrefix, src.ChartIDPrefix)
	dst.ChartContextPrefix = firstNonEmptyString(dst.ChartContextPrefix, src.ChartContextPrefix)
	dst.DeviceCharts = firstNonEmptyStringMap(dst.DeviceCharts, src.DeviceCharts)
	dst.InterfaceCharts = firstNonEmptyInterfaceChartMap(dst.InterfaceCharts, src.InterfaceCharts)
	return dst
}
