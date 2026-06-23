// SPDX-License-Identifier: GPL-3.0-or-later

package projector

import (
	"strconv"
	"strings"
)

func buildTopologyDevicePortDetail(st topologyDevicePortStatus) ProjectionPortDetail {
	detail := ProjectionPortDetail{
		Name:                   strings.TrimSpace(st.IfName),
		IfName:                 strings.TrimSpace(st.IfName),
		IfDescr:                strings.TrimSpace(st.IfDescr),
		IfAlias:                strings.TrimSpace(st.IfAlias),
		MAC:                    strings.TrimSpace(st.MAC),
		Duplex:                 strings.TrimSpace(st.Duplex),
		LinkMode:               st.LinkMode,
		LinkModeConfidence:     st.ModeConfidence,
		TopologyRole:           st.TopologyRole,
		TopologyRoleConfidence: st.RoleConfidence,
		AdminStatus:            st.AdminStatus,
		OperStatus:             st.OperStatus,
		PortType:               st.InterfaceType,
		VLANIDs:                st.VLANIDs,
		VLANs:                  st.VLANs,
		STPState:               st.STPState,
		Neighbors:              topologyPortNeighborDetails(st.Neighbors),
	}
	if st.IfIndex > 0 {
		detail.IfIndex = OptionalValue[int]{Value: st.IfIndex, Has: true}
	}
	if st.SpeedBps > 0 {
		detail.Speed = OptionalValue[int64]{Value: st.SpeedBps, Has: true}
	}
	if st.LastChange > 0 {
		detail.LastChange = strings.TrimSpace(formatTopologyLabelInt64(st.LastChange))
	}
	if len(st.ModeSources) > 0 {
		detail.LinkModeSources = st.ModeSources
	}
	if len(st.RoleSources) > 0 {
		detail.TopologyRoleSources = st.RoleSources
	}
	if st.FDBMACCount > 0 {
		detail.FDBMACCount = OptionalValue[int]{Value: st.FDBMACCount, Has: true}
	}
	if len(detail.Neighbors) > 0 {
		detail.NeighborCount = OptionalValue[int]{Value: len(detail.Neighbors), Has: true}
	}
	return detail
}

func formatTopologyLabelInt64(value int64) string {
	if value <= 0 {
		return ""
	}
	return strconv.FormatInt(value, 10)
}

func topologyPortNeighborDetails(statuses []topologyPortNeighborStatus) []ProjectionPortNeighbor {
	if len(statuses) == 0 {
		return nil
	}
	out := make([]ProjectionPortNeighbor, 0, len(statuses))
	for _, status := range statuses {
		neighbor := ProjectionPortNeighbor{
			Protocol:           strings.ToLower(strings.TrimSpace(status.Protocol)),
			RemoteDevice:       strings.TrimSpace(status.RemoteDevice),
			RemotePort:         strings.TrimSpace(status.RemotePort),
			RemoteIP:           strings.TrimSpace(status.RemoteIP),
			RemoteChassisID:    strings.TrimSpace(status.RemoteChassisID),
			RemoteCapabilities: uniqueTopologyStrings(status.RemoteCapabilities),
		}
		if topologyPortNeighborStatusKey(topologyDevicePortNeighborStatus(neighbor)) == "" {
			continue
		}
		out = append(out, neighbor)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func topologyDevicePortNeighborStatus(neighbor ProjectionPortNeighbor) topologyPortNeighborStatus {
	return topologyPortNeighborStatus{
		Protocol:        neighbor.Protocol,
		RemoteDevice:    neighbor.RemoteDevice,
		RemotePort:      neighbor.RemotePort,
		RemoteIP:        neighbor.RemoteIP,
		RemoteChassisID: neighbor.RemoteChassisID,
	}
}
