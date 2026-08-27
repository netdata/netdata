// SPDX-License-Identifier: GPL-3.0-or-later

package projector

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/l2topology/internal/model"
)

func (b *deviceInterfaceSummaryBuilder) buildSummaries() map[string]topologyDeviceInterfaceSummary {
	out := make(map[string]topologyDeviceInterfaceSummary, len(b.collectors))
	for deviceID, col := range b.collectors {
		maxIfNameBytes, ok := chargeProjectionStringValues(b.work, col.portStatuses, func(status topologyDevicePortStatus) (uint64, error) {
			return uint64(len(status.IfName)), nil
		})
		if !ok {
			return nil
		}
		if !sortProjectionSliceStableWithStringWork(b.work, col.portStatuses, maxIfNameBytes, func(i, j int) bool {
			left, right := col.portStatuses[i], col.portStatuses[j]
			if left.IfIndex != right.IfIndex {
				return left.IfIndex < right.IfIndex
			}
			return left.IfName < right.IfName
		}) {
			return nil
		}

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
		portStatuses := make([]model.ProjectionPortDetail, 0, len(col.portStatuses))
		for _, st := range col.portStatuses {
			evidence := col.portEvidence[st.IfIndex]
			mode, confidence, sources, vlans := classifyTopologyPortLinkModeWithWork(b.work, evidence)
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
				st.VLANs = topologyPortVLANDetailsWithWork(b.work, st.VLANIDs, evidence.vlanNames, st.LinkMode)
				st.Neighbors = sortedTopologyPortNeighborsWithWork(b.work, evidence.neighbors)
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
			portStatuses = append(portStatuses, buildTopologyDevicePortDetail(st))
		}

		out[deviceID] = topologyDeviceInterfaceSummary{
			portsTotal:        len(col.ifIndexes),
			ifIndexes:         sortedTopologySetWithWork(b.work, col.ifIndexes),
			ifNames:           sortedTopologySetWithWork(b.work, col.ifNames),
			adminStatusCount:  normalizedIntCountMapWithWork(b.work, col.adminCounts),
			operStatusCount:   normalizedIntCountMapWithWork(b.work, col.operCounts),
			linkModeCount:     normalizedIntCountMapWithWork(b.work, modeCounts),
			roleCount:         normalizedIntCountMapWithWork(b.work, roleCounts),
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
