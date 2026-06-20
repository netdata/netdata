// SPDX-License-Identifier: GPL-3.0-or-later

package l2topology

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
)

const maxProjectionInt64 = int64(1<<63 - 1)

func buildProjectionActorDetails(actors []graph.Actor) map[string]ProjectionActorDetail {
	if len(actors) == 0 {
		return nil
	}
	out := make(map[string]ProjectionActorDetail, len(actors))
	for _, actor := range actors {
		actorID := strings.TrimSpace(actor.ActorID)
		if actorID == "" {
			continue
		}
		out[actorID] = projectionActorDetailFromGraphActor(actor)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func projectionActorDetailFromGraphActor(actor graph.Actor) ProjectionActorDetail {
	attrs := actor.Attributes
	return ProjectionActorDetail{
		DisplayName:    firstNonEmpty(topologyAttrString(attrs, "display_name"), actor.Labels["display_name"]),
		DisplaySource:  firstNonEmpty(topologyAttrString(attrs, "display_source"), actor.Labels["display_source"]),
		Device:         projectionDeviceActorDetail(attrs, actor.Tables["ports"]),
		Endpoint:       projectionEndpointActorDetail(attrs, actor.Labels),
		Segment:        projectionSegmentActorDetail(attrs, actor.Labels),
		CollapsedByIP:  topologyAnyBoolValue(attrs["collapsed_by_ip"]),
		CollapsedCount: topologyAttrInt(attrs, "collapsed_count"),
	}
}

func projectionDeviceActorDetail(attrs map[string]any, portRows []map[string]any) ProjectionDeviceActorDetail {
	return ProjectionDeviceActorDetail{
		HasInventoryStats:        topologyAttrsHaveAny(attrs, "ports_total", "ports_up", "ports_down", "ports_admin_down", "total_bandwidth_bps", "fdb_total_macs", "vlan_count", "lldp_neighbor_count", "cdp_neighbor_count", "if_admin_status_counts", "if_oper_status_counts", "if_link_mode_counts", "if_topology_role_counts"),
		DeviceID:                 topologyAttrString(attrs, "device_id"),
		Discovered:               topologyAnyBoolValue(attrs["discovered"]),
		Inferred:                 topologyAnyBoolValue(attrs["inferred"]),
		ManagementIP:             topologyAttrString(attrs, "management_ip"),
		ManagementAddresses:      topologyAttrStringSlice(attrs, "management_addresses"),
		Protocols:                topologyAttrStringSlice(attrs, "protocols"),
		ProtocolsCollected:       topologyAttrStringSlice(attrs, "protocols_collected"),
		Capabilities:             topologyAttrStringSlice(attrs, "capabilities"),
		CapabilitiesSupported:    topologyAttrStringSlice(attrs, "capabilities_supported"),
		CapabilitiesEnabled:      topologyAttrStringSlice(attrs, "capabilities_enabled"),
		Vendor:                   topologyAttrString(attrs, "vendor"),
		VendorSource:             topologyAttrString(attrs, "vendor_source"),
		VendorConfidence:         topologyAttrString(attrs, "vendor_confidence"),
		VendorDerived:            topologyAttrString(attrs, "vendor_derived"),
		VendorDerivedSource:      topologyAttrString(attrs, "vendor_derived_source"),
		VendorDerivedConfidence:  topologyAttrString(attrs, "vendor_derived_confidence"),
		VendorDerivedMatchPrefix: topologyAttrString(attrs, "vendor_derived_match_prefix"),
		VendorMatchPrefix:        topologyAttrString(attrs, "vendor_match_prefix"),
		PortsTotal:               topologyAttrInt(attrs, "ports_total"),
		IfIndexes:                topologyAttrStringSlice(attrs, "if_indexes"),
		IfNames:                  topologyAttrStringSlice(attrs, "if_names"),
		PortsUp:                  topologyAttrInt(attrs, "ports_up"),
		PortsDown:                topologyAttrInt(attrs, "ports_down"),
		PortsAdminDown:           topologyAttrInt(attrs, "ports_admin_down"),
		TotalBandwidthBps:        topologyAttrInt64(attrs, "total_bandwidth_bps"),
		FDBTotalMACs:             topologyAttrInt(attrs, "fdb_total_macs"),
		VLANCount:                topologyAttrInt(attrs, "vlan_count"),
		LLDPNeighborCount:        topologyAttrInt(attrs, "lldp_neighbor_count"),
		CDPNeighborCount:         topologyAttrInt(attrs, "cdp_neighbor_count"),
		AdminStatusCounts:        topologyAttrIntMap(attrs, "if_admin_status_counts"),
		OperStatusCounts:         topologyAttrIntMap(attrs, "if_oper_status_counts"),
		LinkModeCounts:           topologyAttrIntMap(attrs, "if_link_mode_counts"),
		TopologyRoleCounts:       topologyAttrIntMap(attrs, "if_topology_role_counts"),
		Ports:                    projectionPortDetails(portRows),
	}
}

func projectionEndpointActorDetail(attrs map[string]any, labels map[string]string) ProjectionEndpointActorDetail {
	return ProjectionEndpointActorDetail{
		Discovered:               topologyAnyBoolValue(attrs["discovered"]),
		LearnedSources:           topologyAttrStringSlice(attrs, "learned_sources"),
		LearnedDeviceIDs:         topologyAttrStringSlice(attrs, "learned_device_ids"),
		LearnedIfIndexes:         topologyAttrStringSlice(attrs, "learned_if_indexes"),
		LearnedIfNames:           topologyAttrStringSlice(attrs, "learned_if_names"),
		Vendor:                   topologyAttrString(attrs, "vendor"),
		VendorSource:             topologyAttrString(attrs, "vendor_source"),
		VendorConfidence:         topologyAttrString(attrs, "vendor_confidence"),
		VendorMatchPrefix:        topologyAttrString(attrs, "vendor_match_prefix"),
		VendorDerived:            topologyAttrString(attrs, "vendor_derived"),
		VendorDerivedSource:      topologyAttrString(attrs, "vendor_derived_source"),
		VendorDerivedConfidence:  topologyAttrString(attrs, "vendor_derived_confidence"),
		VendorDerivedMatchPrefix: topologyAttrString(attrs, "vendor_derived_match_prefix"),
		AttachmentSource:         topologyAttrString(attrs, "attachment_source"),
		AttachedDeviceID:         firstNonEmpty(topologyAttrString(attrs, "attached_device_id"), labels["attached_device_id"]),
		AttachedDevice:           firstNonEmpty(topologyAttrString(attrs, "attached_device"), labels["attached_device"]),
		AttachedPort:             firstNonEmpty(topologyAttrString(attrs, "attached_port"), labels["attached_port"]),
		AttachedIfName:           topologyAttrString(attrs, "attached_if_name"),
		AttachedIfIndex:          topologyAttrInt(attrs, "attached_if_index"),
		AttachedBridgePort:       topologyAttrString(attrs, "attached_bridge_port"),
		AttachedVLAN:             topologyAttrString(attrs, "attached_vlan"),
		AttachedVLANID:           topologyAttrString(attrs, "attached_vlan_id"),
		AttachedBy:               labels["attached_by"],
	}
}

func projectionSegmentActorDetail(attrs map[string]any, labels map[string]string) ProjectionSegmentActorDetail {
	return ProjectionSegmentActorDetail{
		HasStats:       topologyAttrsHaveAny(attrs, "ports_total", "endpoints_total"),
		SegmentID:      topologyAttrString(attrs, "segment_id"),
		SegmentType:    topologyAttrString(attrs, "segment_type"),
		ParentDevices:  topologyAttrStringSlice(attrs, "parent_devices"),
		IfNames:        topologyAttrStringSlice(attrs, "if_names"),
		IfIndexes:      topologyAttrStringSlice(attrs, "if_indexes"),
		BridgePorts:    topologyAttrStringSlice(attrs, "bridge_ports"),
		VLANIDs:        topologyAttrStringSlice(attrs, "vlan_ids"),
		LearnedSources: topologyAttrStringSlice(attrs, "learned_sources"),
		PortsTotal:     topologyAttrInt(attrs, "ports_total"),
		EndpointsTotal: topologyAttrInt(attrs, "endpoints_total"),
		DesignatedPort: topologyAttrString(attrs, "designated_port"),
		SegmentKind:    labels["segment_kind"],
	}
}

func topologyAttrsHaveAny(attrs map[string]any, keys ...string) bool {
	if len(attrs) == 0 {
		return false
	}
	for _, key := range keys {
		if _, ok := attrs[key]; ok {
			return true
		}
	}
	return false
}

func projectionPortDetails(rows []map[string]any) []ProjectionPortDetail {
	if len(rows) == 0 {
		return nil
	}
	out := make([]ProjectionPortDetail, 0, len(rows))
	for _, row := range rows {
		if len(row) == 0 {
			continue
		}
		out = append(out, ProjectionPortDetail{
			IfIndex:                topologyAttrInt(row, "if_index"),
			PortID:                 topologyAttrString(row, "port_id"),
			Name:                   topologyAttrString(row, "name"),
			IfName:                 topologyAttrString(row, "if_name"),
			IfDescr:                topologyAttrString(row, "if_descr"),
			IfAlias:                topologyAttrString(row, "if_alias"),
			MAC:                    topologyAttrString(row, "mac"),
			Speed:                  topologyAttrInt64(row, "speed"),
			TopologyRole:           topologyAttrString(row, "topology_role"),
			OperStatus:             firstNonEmpty(topologyAttrString(row, "oper_status"), topologyAttrString(row, "if_oper_status")),
			AdminStatus:            firstNonEmpty(topologyAttrString(row, "admin_status"), topologyAttrString(row, "if_admin_status")),
			PortType:               firstNonEmpty(topologyAttrString(row, "port_type"), topologyAttrString(row, "if_type")),
			LinkMode:               topologyAttrString(row, "link_mode"),
			STPState:               topologyAttrString(row, "stp_state"),
			VLANIDs:                topologyAttrStringSlice(row, "vlan_ids"),
			FDBMACCount:            topologyAttrInt(row, "fdb_mac_count"),
			LinkCount:              topologyAttrInt(row, "link_count"),
			NeighborCount:          topologyAttrInt(row, "neighbor_count"),
			Neighbors:              projectionPortNeighbors(row["neighbors"]),
			VLANs:                  projectionPortVLANs(row["vlans"]),
			Duplex:                 topologyAttrString(row, "duplex"),
			LinkModeConfidence:     topologyAttrString(row, "link_mode_confidence"),
			TopologyRoleConfidence: topologyAttrString(row, "topology_role_confidence"),
			LinkModeSources:        topologyAttrStringSlice(row, "link_mode_sources"),
			TopologyRoleSources:    topologyAttrStringSlice(row, "topology_role_sources"),
			LastChange:             topologyAttrScalarString(row, "last_change"),
			ChartIDSuffix:          topologyAttrString(row, "chart_id_suffix"),
			AvailableMetrics:       topologyAttrStringSlice(row, "available_metrics"),
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func projectionPortNeighbors(value any) []ProjectionPortNeighbor {
	rows, ok := anyMapSlice(value)
	if !ok || len(rows) == 0 {
		return nil
	}
	out := make([]ProjectionPortNeighbor, 0, len(rows))
	for _, row := range rows {
		out = append(out, ProjectionPortNeighbor{
			Protocol:           topologyAttrString(row, "protocol"),
			RemoteDevice:       topologyAttrString(row, "remote_device"),
			RemotePort:         topologyAttrString(row, "remote_port"),
			RemoteIP:           topologyAttrString(row, "remote_ip"),
			RemoteChassisID:    topologyAttrString(row, "remote_chassis_id"),
			RemoteCapabilities: topologyAttrStringSlice(row, "remote_capabilities"),
		})
	}
	return out
}

func projectionPortVLANs(value any) []ProjectionPortVLAN {
	rows, ok := anyMapSlice(value)
	if !ok || len(rows) == 0 {
		return nil
	}
	out := make([]ProjectionPortVLAN, 0, len(rows))
	for _, row := range rows {
		out = append(out, ProjectionPortVLAN{
			VLANID:   topologyAttrString(row, "vlan_id"),
			VLANName: topologyAttrString(row, "vlan_name"),
			Tagged:   topologyAnyBoolValue(row["tagged"]),
		})
	}
	return out
}

func topologyAttrScalarString(attrs map[string]any, key string) string {
	if len(attrs) == 0 {
		return ""
	}
	value, ok := attrs[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case []byte:
		return strings.TrimSpace(string(typed))
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, bool:
		return strings.TrimSpace(fmt.Sprint(typed))
	default:
		return ""
	}
}

func topologyAttrInt64(attrs map[string]any, key string) int64 {
	if len(attrs) == 0 {
		return 0
	}
	value, ok := attrs[key]
	if !ok || value == nil {
		return 0
	}
	switch typed := value.(type) {
	case int:
		return int64(typed)
	case int8:
		return int64(typed)
	case int16:
		return int64(typed)
	case int32:
		return int64(typed)
	case int64:
		return typed
	case uint:
		if uint64(typed) > uint64(maxProjectionInt64) {
			return maxProjectionInt64
		}
		return int64(typed)
	case uint8:
		return int64(typed)
	case uint16:
		return int64(typed)
	case uint32:
		return int64(typed)
	case uint64:
		if typed > uint64(maxProjectionInt64) {
			return maxProjectionInt64
		}
		return int64(typed)
	case float32:
		if float64(typed) > float64(maxProjectionInt64) {
			return maxProjectionInt64
		}
		return int64(typed)
	case float64:
		if typed > float64(maxProjectionInt64) {
			return maxProjectionInt64
		}
		return int64(typed)
	case string:
		return parseTopologyLabelInt64(typed)
	default:
		return 0
	}
}

func topologyAttrIntMap(attrs map[string]any, key string) map[string]int {
	if len(attrs) == 0 {
		return nil
	}
	value, ok := attrs[key]
	if !ok || value == nil {
		return nil
	}
	out := make(map[string]int)
	switch typed := value.(type) {
	case map[string]int:
		for k, v := range typed {
			if k = strings.TrimSpace(k); k != "" {
				out[k] = v
			}
		}
	case map[string]any:
		for k, v := range typed {
			if k = strings.TrimSpace(k); k != "" {
				out[k] = int(topologyAnyInt64Value(v))
			}
		}
	default:
		return nil
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func topologyAnyInt64Value(value any) int64 {
	return topologyAttrInt64(map[string]any{"value": value}, "value")
}

func anyMapSlice(value any) ([]map[string]any, bool) {
	switch typed := value.(type) {
	case []map[string]any:
		return typed, true
	case []any:
		out := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			row, ok := item.(map[string]any)
			if !ok {
				return nil, false
			}
			out = append(out, row)
		}
		return out, true
	default:
		return nil, false
	}
}

func projectionDebugString(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}
