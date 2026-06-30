// SPDX-License-Identifier: GPL-3.0-or-later

package topologyshape

import (
	"strings"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/l2topology"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"
)

func collapseActorsByIP(data *topologymodel.Data) int {
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
		if topologymodel.ActorIsSegment(actor) {
			continue
		}
		ips := topologymodel.NormalizedMatchIPs(actor.Match)
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
			repActor.SegmentKind = topologyutil.FirstNonEmptyString(repActor.SegmentKind, data.Actors[idx].SegmentKind)
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

	actors := make([]topologymodel.Actor, 0, len(data.Actors)-collapsed)
	for idx, actor := range data.Actors {
		if !keep[idx] {
			continue
		}
		actors = append(actors, actor)
	}
	data.Actors = actors

	links := make([]topologymodel.Link, 0, len(data.Links))
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

func compareCollapseActorPriority(left, right topologymodel.Actor) int {
	if leftDevice, rightDevice := topologyengine.IsDeviceActorType(left.ActorType), topologyengine.IsDeviceActorType(right.ActorType); leftDevice != rightDevice {
		if leftDevice {
			return -1
		}
		return 1
	}
	if leftInferred, rightInferred := topologymodel.ActorIsInferred(left), topologymodel.ActorIsInferred(right); leftInferred != rightInferred {
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

func mergeTopologyActorDetail(dst, src topologymodel.ActorDetail) topologymodel.ActorDetail {
	dst.L2 = mergeTopologyProjectionActorDetail(dst.L2, src.L2)
	dst.SNMP = mergeTopologySNMPActorDetail(dst.SNMP, src.SNMP)
	dst.OSPF = append(dst.OSPF, src.OSPF...)
	dst.BGP = append(dst.BGP, src.BGP...)
	return dst
}

func mergeTopologyProjectionActorDetail(dst, src topologyengine.ProjectionActorDetail) topologyengine.ProjectionActorDetail {
	dst.DisplayName = topologyutil.FirstNonEmptyString(dst.DisplayName, src.DisplayName)
	dst.DisplaySource = topologyutil.FirstNonEmptyString(dst.DisplaySource, src.DisplaySource)
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
	dst.DeviceID = topologyutil.FirstNonEmptyString(dst.DeviceID, src.DeviceID)
	dst.Discovered = dst.Discovered || src.Discovered
	dst.Inferred = dst.Inferred || src.Inferred
	dst.ManagementIP = topologyutil.FirstNonEmptyString(dst.ManagementIP, src.ManagementIP)
	dst.ManagementAddresses = topologyutil.FirstNonEmptySlice(dst.ManagementAddresses, src.ManagementAddresses)
	dst.Protocols = topologyutil.FirstNonEmptySlice(dst.Protocols, src.Protocols)
	dst.ProtocolsCollected = topologyutil.FirstNonEmptySlice(dst.ProtocolsCollected, src.ProtocolsCollected)
	dst.Capabilities = topologyutil.FirstNonEmptySlice(dst.Capabilities, src.Capabilities)
	dst.CapabilitiesSupported = topologyutil.FirstNonEmptySlice(dst.CapabilitiesSupported, src.CapabilitiesSupported)
	dst.CapabilitiesEnabled = topologyutil.FirstNonEmptySlice(dst.CapabilitiesEnabled, src.CapabilitiesEnabled)
	dst.Vendor = topologyutil.FirstNonEmptyString(dst.Vendor, src.Vendor)
	dst.VendorSource = topologyutil.FirstNonEmptyString(dst.VendorSource, src.VendorSource)
	dst.VendorConfidence = topologyutil.FirstNonEmptyString(dst.VendorConfidence, src.VendorConfidence)
	dst.VendorDerived = topologyutil.FirstNonEmptyString(dst.VendorDerived, src.VendorDerived)
	dst.VendorDerivedSource = topologyutil.FirstNonEmptyString(dst.VendorDerivedSource, src.VendorDerivedSource)
	dst.VendorDerivedConfidence = topologyutil.FirstNonEmptyString(dst.VendorDerivedConfidence, src.VendorDerivedConfidence)
	dst.VendorDerivedMatchPrefix = topologyutil.FirstNonEmptyString(dst.VendorDerivedMatchPrefix, src.VendorDerivedMatchPrefix)
	dst.VendorMatchPrefix = topologyutil.FirstNonEmptyString(dst.VendorMatchPrefix, src.VendorMatchPrefix)
	dst.PortsTotal = firstPresentValue(dst.PortsTotal, src.PortsTotal)
	dst.IfIndexes = topologyutil.FirstNonEmptySlice(dst.IfIndexes, src.IfIndexes)
	dst.IfNames = topologyutil.FirstNonEmptySlice(dst.IfNames, src.IfNames)
	dst.PortsUp = firstPresentValue(dst.PortsUp, src.PortsUp)
	dst.PortsDown = firstPresentValue(dst.PortsDown, src.PortsDown)
	dst.PortsAdminDown = firstPresentValue(dst.PortsAdminDown, src.PortsAdminDown)
	dst.TotalBandwidthBps = firstPresentValue(dst.TotalBandwidthBps, src.TotalBandwidthBps)
	dst.FDBTotalMACs = firstPresentValue(dst.FDBTotalMACs, src.FDBTotalMACs)
	dst.VLANCount = firstPresentValue(dst.VLANCount, src.VLANCount)
	dst.LLDPNeighborCount = firstPresentValue(dst.LLDPNeighborCount, src.LLDPNeighborCount)
	dst.CDPNeighborCount = firstPresentValue(dst.CDPNeighborCount, src.CDPNeighborCount)
	dst.AdminStatusCounts = topologyutil.FirstNonEmptyMap(dst.AdminStatusCounts, src.AdminStatusCounts)
	dst.OperStatusCounts = topologyutil.FirstNonEmptyMap(dst.OperStatusCounts, src.OperStatusCounts)
	dst.LinkModeCounts = topologyutil.FirstNonEmptyMap(dst.LinkModeCounts, src.LinkModeCounts)
	dst.TopologyRoleCounts = topologyutil.FirstNonEmptyMap(dst.TopologyRoleCounts, src.TopologyRoleCounts)
	if len(dst.Ports) == 0 {
		dst.Ports = src.Ports
	}
	return dst
}

func mergeTopologyProjectionEndpointDetail(dst, src topologyengine.ProjectionEndpointActorDetail) topologyengine.ProjectionEndpointActorDetail {
	dst.Discovered = dst.Discovered || src.Discovered
	dst.LearnedSources = topologyutil.FirstNonEmptySlice(dst.LearnedSources, src.LearnedSources)
	dst.LearnedDeviceIDs = topologyutil.FirstNonEmptySlice(dst.LearnedDeviceIDs, src.LearnedDeviceIDs)
	dst.LearnedIfIndexes = topologyutil.FirstNonEmptySlice(dst.LearnedIfIndexes, src.LearnedIfIndexes)
	dst.LearnedIfNames = topologyutil.FirstNonEmptySlice(dst.LearnedIfNames, src.LearnedIfNames)
	dst.Vendor = topologyutil.FirstNonEmptyString(dst.Vendor, src.Vendor)
	dst.VendorSource = topologyutil.FirstNonEmptyString(dst.VendorSource, src.VendorSource)
	dst.VendorConfidence = topologyutil.FirstNonEmptyString(dst.VendorConfidence, src.VendorConfidence)
	dst.VendorMatchPrefix = topologyutil.FirstNonEmptyString(dst.VendorMatchPrefix, src.VendorMatchPrefix)
	dst.VendorDerived = topologyutil.FirstNonEmptyString(dst.VendorDerived, src.VendorDerived)
	dst.VendorDerivedSource = topologyutil.FirstNonEmptyString(dst.VendorDerivedSource, src.VendorDerivedSource)
	dst.VendorDerivedConfidence = topologyutil.FirstNonEmptyString(dst.VendorDerivedConfidence, src.VendorDerivedConfidence)
	dst.VendorDerivedMatchPrefix = topologyutil.FirstNonEmptyString(dst.VendorDerivedMatchPrefix, src.VendorDerivedMatchPrefix)
	dst.AttachmentSource = topologyutil.FirstNonEmptyString(dst.AttachmentSource, src.AttachmentSource)
	dst.AttachedDeviceID = topologyutil.FirstNonEmptyString(dst.AttachedDeviceID, src.AttachedDeviceID)
	dst.AttachedDevice = topologyutil.FirstNonEmptyString(dst.AttachedDevice, src.AttachedDevice)
	dst.AttachedPort = topologyutil.FirstNonEmptyString(dst.AttachedPort, src.AttachedPort)
	dst.AttachedIfName = topologyutil.FirstNonEmptyString(dst.AttachedIfName, src.AttachedIfName)
	dst.AttachedIfIndex = topologyutil.FirstNonZeroInt(dst.AttachedIfIndex, src.AttachedIfIndex)
	dst.AttachedBridgePort = topologyutil.FirstNonEmptyString(dst.AttachedBridgePort, src.AttachedBridgePort)
	dst.AttachedVLAN = topologyutil.FirstNonEmptyString(dst.AttachedVLAN, src.AttachedVLAN)
	dst.AttachedVLANID = topologyutil.FirstNonEmptyString(dst.AttachedVLANID, src.AttachedVLANID)
	dst.AttachedBy = topologyutil.FirstNonEmptyString(dst.AttachedBy, src.AttachedBy)
	return dst
}

func mergeTopologyProjectionSegmentDetail(dst, src topologyengine.ProjectionSegmentActorDetail) topologyengine.ProjectionSegmentActorDetail {
	dst.SegmentID = topologyutil.FirstNonEmptyString(dst.SegmentID, src.SegmentID)
	dst.SegmentType = topologyutil.FirstNonEmptyString(dst.SegmentType, src.SegmentType)
	dst.ParentDevices = topologyutil.FirstNonEmptySlice(dst.ParentDevices, src.ParentDevices)
	dst.IfNames = topologyutil.FirstNonEmptySlice(dst.IfNames, src.IfNames)
	dst.IfIndexes = topologyutil.FirstNonEmptySlice(dst.IfIndexes, src.IfIndexes)
	dst.BridgePorts = topologyutil.FirstNonEmptySlice(dst.BridgePorts, src.BridgePorts)
	dst.VLANIDs = topologyutil.FirstNonEmptySlice(dst.VLANIDs, src.VLANIDs)
	dst.LearnedSources = topologyutil.FirstNonEmptySlice(dst.LearnedSources, src.LearnedSources)
	dst.PortsTotal = firstPresentValue(dst.PortsTotal, src.PortsTotal)
	dst.EndpointsTotal = firstPresentValue(dst.EndpointsTotal, src.EndpointsTotal)
	dst.DesignatedPort = topologyutil.FirstNonEmptyString(dst.DesignatedPort, src.DesignatedPort)
	dst.SegmentKind = topologyutil.FirstNonEmptyString(dst.SegmentKind, src.SegmentKind)
	return dst
}

func mergeTopologySNMPActorDetail(dst, src topologymodel.SNMPActorDetail) topologymodel.SNMPActorDetail {
	dst.ManagementAddresses = topologyutil.FirstNonEmptySlice(dst.ManagementAddresses, src.ManagementAddresses)
	dst.Capabilities = topologyutil.FirstNonEmptySlice(dst.Capabilities, src.Capabilities)
	dst.CapabilitiesSupported = topologyutil.FirstNonEmptySlice(dst.CapabilitiesSupported, src.CapabilitiesSupported)
	dst.CapabilitiesEnabled = topologyutil.FirstNonEmptySlice(dst.CapabilitiesEnabled, src.CapabilitiesEnabled)
	dst.SysDescr = topologyutil.FirstNonEmptyString(dst.SysDescr, src.SysDescr)
	dst.SysContact = topologyutil.FirstNonEmptyString(dst.SysContact, src.SysContact)
	dst.SysLocation = topologyutil.FirstNonEmptyString(dst.SysLocation, src.SysLocation)
	dst.SysUptime = topologyutil.FirstNonZeroInt64(dst.SysUptime, src.SysUptime)
	dst.Vendor = topologyutil.FirstNonEmptyString(dst.Vendor, src.Vendor)
	dst.VendorSource = topologyutil.FirstNonEmptyString(dst.VendorSource, src.VendorSource)
	dst.VendorConfidence = topologyutil.FirstNonEmptyString(dst.VendorConfidence, src.VendorConfidence)
	dst.Model = topologyutil.FirstNonEmptyString(dst.Model, src.Model)
	dst.OSPFRouterID = topologyutil.FirstNonEmptyString(dst.OSPFRouterID, src.OSPFRouterID)
	dst.SerialNumber = topologyutil.FirstNonEmptyString(dst.SerialNumber, src.SerialNumber)
	dst.SoftwareVersion = topologyutil.FirstNonEmptyString(dst.SoftwareVersion, src.SoftwareVersion)
	dst.FirmwareVersion = topologyutil.FirstNonEmptyString(dst.FirmwareVersion, src.FirmwareVersion)
	dst.HardwareVersion = topologyutil.FirstNonEmptyString(dst.HardwareVersion, src.HardwareVersion)
	dst.ManagementIP = topologyutil.FirstNonEmptyString(dst.ManagementIP, src.ManagementIP)
	dst.NetdataHostID = topologyutil.FirstNonEmptyString(dst.NetdataHostID, src.NetdataHostID)
	dst.ChartIDPrefix = topologyutil.FirstNonEmptyString(dst.ChartIDPrefix, src.ChartIDPrefix)
	dst.ChartContextPrefix = topologyutil.FirstNonEmptyString(dst.ChartContextPrefix, src.ChartContextPrefix)
	dst.DeviceCharts = topologyutil.FirstNonEmptyMap(dst.DeviceCharts, src.DeviceCharts)
	dst.InterfaceCharts = topologyutil.FirstNonEmptyMap(dst.InterfaceCharts, src.InterfaceCharts)
	return dst
}

func firstPresentValue[T any](dst, src topologyengine.OptionalValue[T]) topologyengine.OptionalValue[T] {
	if dst.Has {
		return dst
	}
	if src.Has {
		return src
	}
	return dst
}
