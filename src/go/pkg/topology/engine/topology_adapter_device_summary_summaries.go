// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"sort"
	"strings"
)

func (b *deviceInterfaceSummaryBuilder) buildSummaries() map[string]topologyDeviceInterfaceSummary {
	out := make(map[string]topologyDeviceInterfaceSummary, len(b.collectors))
	for deviceID, col := range b.collectors {
		sort.Slice(col.portStatuses, func(i, j int) bool {
			left, right := col.portStatuses[i], col.portStatuses[j]
			if left.IfIndex != right.IfIndex {
				return left.IfIndex < right.IfIndex
			}
			return left.IfName < right.IfName
		})

		modeCounts := make(map[string]int)
		roleCounts := make(map[string]int)
		deviceVLANIDs := make(map[string]struct{})
		portsUp := 0
		portsDown := 0
		portsAdminDown := 0
		totalBandwidthBps := int64(0)
		fdbTotalMACs := 0
		lldpNeighborCount := 0
		cdpNeighborCount := 0
		portStatuses := make([]map[string]any, 0, len(col.portStatuses))
		for _, st := range col.portStatuses {
			evidence := col.portEvidence[st.IfIndex]
			mode, confidence, sources, vlans := classifyTopologyPortLinkMode(evidence)
			role, roleConfidence, roleSources := classifyTopologyPortRole(evidence)
			st.LinkMode = mode
			st.ModeConfidence = confidence
			st.ModeSources = sources
			st.VLANIDs = vlans
			st.TopologyRole = role
			st.RoleConfidence = roleConfidence
			st.RoleSources = roleSources
			if evidence != nil {
				st.FDBMACCount = len(evidence.fdbEndpointIDs)
				st.STPState = summarizeTopologySTPState(evidence.stpStates)
				st.VLANs = topologyPortVLANAttributes(st.VLANIDs, evidence.vlanNames, st.LinkMode)
				st.Neighbors = sortedTopologyPortNeighbors(evidence.neighbors)
			}

			for _, vlanID := range st.VLANIDs {
				deviceVLANIDs[vlanID] = struct{}{}
			}
			if strings.EqualFold(strings.TrimSpace(st.OperStatus), "up") {
				portsUp++
				totalBandwidthBps = safeTopologyInt64Add(totalBandwidthBps, st.SpeedBps)
			} else if strings.EqualFold(strings.TrimSpace(st.OperStatus), "down") || strings.EqualFold(strings.TrimSpace(st.OperStatus), "lowerlayerdown") {
				portsDown++
			}
			if strings.EqualFold(strings.TrimSpace(st.AdminStatus), "down") || strings.EqualFold(strings.TrimSpace(st.AdminStatus), "administrativelydown") {
				portsAdminDown++
			}
			fdbTotalMACs += st.FDBMACCount
			for _, neighbor := range st.Neighbors {
				switch strings.ToLower(strings.TrimSpace(neighbor.Protocol)) {
				case "lldp":
					lldpNeighborCount++
				case "cdp":
					cdpNeighborCount++
				}
			}

			modeCounts[mode]++
			roleCounts[role]++
			portStatuses = append(portStatuses, buildTopologyDevicePortStatusAttributes(st))
		}

		out[deviceID] = topologyDeviceInterfaceSummary{
			portsTotal:        len(col.ifIndexes),
			ifIndexes:         sortedTopologySet(col.ifIndexes),
			ifNames:           sortedTopologySet(col.ifNames),
			adminStatusCount:  intCountMapToAny(col.adminCounts),
			operStatusCount:   intCountMapToAny(col.operCounts),
			linkModeCount:     intCountMapToAny(modeCounts),
			roleCount:         intCountMapToAny(roleCounts),
			portsUp:           portsUp,
			portsDown:         portsDown,
			portsAdminDown:    portsAdminDown,
			totalBandwidthBps: totalBandwidthBps,
			fdbTotalMACs:      fdbTotalMACs,
			vlanCount:         len(deviceVLANIDs),
			lldpNeighborCount: lldpNeighborCount,
			cdpNeighborCount:  cdpNeighborCount,
			portStatuses:      portStatuses,
		}
	}
	return out
}
