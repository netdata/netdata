// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"fmt"
	"strings"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/l2topology"
	topologyv1 "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
)

type snmpTopologyV1PortNeighborKey struct {
	actorRef int
	ifIndex  uint64
	portName string
}

type snmpTopologyV1PortNeighborSummary struct {
	remoteActor    any
	remotePortName string
	ambiguous      bool
}

type topologyV1DynamicRow struct {
	actorRef int
	values   map[string]any
}

func snmpTopologyV1PortNeighborKeyFor(actorRef int, ifIndex any, portName string) snmpTopologyV1PortNeighborKey {
	if index, ok := uintValue(ifIndex); ok && index > 0 {
		return snmpTopologyV1PortNeighborKey{actorRef: actorRef, ifIndex: index}
	}
	portName = strings.ToLower(strings.TrimSpace(portName))
	if portName == "" {
		return snmpTopologyV1PortNeighborKey{actorRef: -1}
	}
	return snmpTopologyV1PortNeighborKey{actorRef: actorRef, portName: portName}
}

func buildSNMPTopologyV1PortNeighborSummaries(
	links []topologyLink,
	actorIndex map[string]int,
) map[snmpTopologyV1PortNeighborKey]snmpTopologyV1PortNeighborSummary {
	summaries := make(map[snmpTopologyV1PortNeighborKey]snmpTopologyV1PortNeighborSummary)
	appendSide := func(actorID, remoteActorID string, endpoint, remoteEndpoint topologyLinkEndpoint) {
		actorRef, ok := actorIndex[strings.TrimSpace(actorID)]
		if !ok {
			return
		}
		remoteActorRef, ok := actorIndex[strings.TrimSpace(remoteActorID)]
		if !ok {
			return
		}
		key := snmpTopologyV1PortNeighborKeyFor(actorRef, endpoint.Attributes["if_index"], topologyV1EndpointPortName(endpoint))
		if key.actorRef < 0 {
			return
		}
		remotePortName := topologyV1EndpointPortName(remoteEndpoint)
		if existing, exists := summaries[key]; exists {
			if existing.remoteActor != remoteActorRef || !snmpTopologyV1SameRemotePortName(existing.remotePortName, remotePortName) {
				existing.ambiguous = true
				summaries[key] = existing
			} else if strings.TrimSpace(existing.remotePortName) == "" && strings.TrimSpace(remotePortName) != "" {
				existing.remotePortName = remotePortName
				summaries[key] = existing
			}
			return
		}
		summaries[key] = snmpTopologyV1PortNeighborSummary{
			remoteActor:    remoteActorRef,
			remotePortName: remotePortName,
		}
	}

	for _, link := range links {
		if snmpTopologyV1LinkIsLogicalL3(link) {
			continue
		}
		appendSide(link.SrcActorID, link.DstActorID, link.Src, link.Dst)
		appendSide(link.DstActorID, link.SrcActorID, link.Dst, link.Src)
	}
	return summaries
}

func snmpTopologyV1SameRemotePortName(left, right string) bool {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == "" || right == "" {
		return true
	}
	return strings.EqualFold(left, right)
}

func snmpTopologyV1PortNeighborSummaryFor(
	row topologyV1DynamicRow,
	portName string,
	summaries map[snmpTopologyV1PortNeighborKey]snmpTopologyV1PortNeighborSummary,
) (snmpTopologyV1PortNeighborSummary, bool) {
	candidates := []string{
		portName,
		topologyV1ScalarLabelValue(row.values["if_name"]),
		topologyV1ScalarLabelValue(row.values["port_name"]),
		topologyV1ScalarLabelValue(row.values["port_id"]),
	}
	seen := make(map[snmpTopologyV1PortNeighborKey]struct{}, len(candidates)+1)
	keys := []snmpTopologyV1PortNeighborKey{
		snmpTopologyV1PortNeighborKeyFor(row.actorRef, row.values["if_index"], ""),
	}
	for _, candidate := range candidates {
		keys = append(keys, snmpTopologyV1PortNeighborKeyFor(row.actorRef, nil, candidate))
	}
	for _, key := range keys {
		if key.actorRef < 0 {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if summary, ok := summaries[key]; ok {
			if summary.ambiguous {
				return snmpTopologyV1PortNeighborSummary{}, false
			}
			return summary, true
		}
	}
	return snmpTopologyV1PortNeighborSummary{}, false
}

func collectSNMPTopologyV1ActorTableRows(actors []topologyActor) map[string][]topologyV1DynamicRow {
	tables := make(map[string][]topologyV1DynamicRow)
	for actorIndex, actor := range actors {
		for _, port := range actor.Detail.L2.Device.Ports {
			tables["ports"] = append(tables["ports"], topologyV1DynamicRow{
				actorRef: actorIndex,
				values:   snmpTopologyV1PortValues(port),
			})
		}
		for _, row := range actor.Detail.OSPF {
			tables["ospf_neighbors"] = append(tables["ospf_neighbors"], topologyV1DynamicRow{
				actorRef: actorIndex,
				values:   snmpTopologyV1OSPFNeighborValues(row),
			})
		}
		for _, row := range actor.Detail.BGP {
			tables["bgp_peers"] = append(tables["bgp_peers"], topologyV1DynamicRow{
				actorRef: actorIndex,
				values:   snmpTopologyV1BGPPeerValues(row),
			})
		}
	}
	return tables
}

func snmpTopologyV1PortValues(port topologyengine.ProjectionPortDetail) map[string]any {
	return pruneNilAttributes(map[string]any{
		"if_index":                 nullableOptionalUintValue(port.IfIndex),
		"port_id":                  port.PortID,
		"name":                     firstNonEmptyString(port.Name, port.IfName, port.PortID),
		"if_name":                  port.IfName,
		"if_descr":                 port.IfDescr,
		"if_alias":                 port.IfAlias,
		"mac":                      port.MAC,
		"speed":                    nullableOptionalUint64Value(port.Speed),
		"topology_role":            port.TopologyRole,
		"oper_status":              port.OperStatus,
		"admin_status":             port.AdminStatus,
		"port_type":                port.PortType,
		"link_mode":                port.LinkMode,
		"stp_state":                port.STPState,
		"vlan_ids":                 port.VLANIDs,
		"fdb_mac_count":            nullableOptionalUintValue(port.FDBMACCount),
		"link_count":               nullableOptionalUintValue(port.LinkCount),
		"neighbor_count":           nullableOptionalUintValue(port.NeighborCount),
		"neighbors":                snmpTopologyV1PortNeighborValues(port.Neighbors),
		"vlans":                    snmpTopologyV1PortVLANValues(port.VLANs),
		"duplex":                   port.Duplex,
		"link_mode_confidence":     port.LinkModeConfidence,
		"topology_role_confidence": port.TopologyRoleConfidence,
		"link_mode_sources":        port.LinkModeSources,
		"topology_role_sources":    port.TopologyRoleSources,
		"last_change":              port.LastChange,
	})
}

func snmpTopologyV1PortNeighborValues(neighbors []topologyengine.ProjectionPortNeighbor) []map[string]any {
	if len(neighbors) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(neighbors))
	for _, neighbor := range neighbors {
		out = append(out, pruneNilAttributes(map[string]any{
			"protocol":            neighbor.Protocol,
			"remote_device":       neighbor.RemoteDevice,
			"remote_port":         neighbor.RemotePort,
			"remote_ip":           neighbor.RemoteIP,
			"remote_chassis_id":   neighbor.RemoteChassisID,
			"remote_capabilities": neighbor.RemoteCapabilities,
		}))
	}
	return out
}

func snmpTopologyV1PortVLANValues(vlans []topologyengine.ProjectionPortVLAN) []map[string]any {
	if len(vlans) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(vlans))
	for _, vlan := range vlans {
		out = append(out, pruneNilAttributes(map[string]any{
			"vlan_id":   vlan.VLANID,
			"vlan_name": vlan.VLANName,
			"tagged":    vlan.Tagged,
		}))
	}
	return out
}

func buildSNMPTopologyV1ActorPortsTable(
	rows []topologyV1DynamicRow,
	stringsDict *topologyv1.StringDictionary,
	portNeighborSummaries map[snmpTopologyV1PortNeighborKey]snmpTopologyV1PortNeighborSummary,
) topologyv1.Table {
	actorRefs := make([]any, len(rows))
	ifIndexes := make([]any, len(rows))
	portIDs := make([]any, len(rows))
	names := make([]any, len(rows))
	ifNames := make([]any, len(rows))
	ifDescrs := make([]any, len(rows))
	ifAliases := make([]any, len(rows))
	macs := make([]any, len(rows))
	speeds := make([]any, len(rows))
	topologyRoles := make([]any, len(rows))
	operStatuses := make([]any, len(rows))
	adminStatuses := make([]any, len(rows))
	portTypes := make([]any, len(rows))
	linkModes := make([]any, len(rows))
	stpStates := make([]any, len(rows))
	vlanIDs := make([]any, len(rows))
	fdbMACCounts := make([]any, len(rows))
	linkCounts := make([]any, len(rows))
	neighborCounts := make([]any, len(rows))
	neighborActors := make([]any, len(rows))
	neighborPortNames := make([]any, len(rows))
	neighbors := make([]any, len(rows))
	vlans := make([]any, len(rows))
	duplexes := make([]any, len(rows))
	linkModeConfidences := make([]any, len(rows))
	topologyRoleConfidences := make([]any, len(rows))
	linkModeSources := make([]any, len(rows))
	topologyRoleSources := make([]any, len(rows))
	lastChanges := make([]any, len(rows))

	for i, row := range rows {
		actorRefs[i] = row.actorRef
		ifIndexes[i] = nullableUintValue(row.values["if_index"])
		portIDs[i] = nullableStringRef(stringsDict, topologyV1ScalarLabelValue(row.values["port_id"]))
		portName := firstNonEmptyString(
			topologyV1ScalarLabelValue(row.values["name"]),
			topologyV1ScalarLabelValue(row.values["if_name"]),
			topologyV1ScalarLabelValue(row.values["port_name"]),
			topologyV1ScalarLabelValue(row.values["port_id"]),
		)
		names[i] = nullableStringRef(stringsDict, portName)
		ifNames[i] = nullableStringRef(stringsDict, topologyV1ScalarLabelValue(row.values["if_name"]))
		ifDescrs[i] = nullableStringRef(stringsDict, topologyV1ScalarLabelValue(row.values["if_descr"]))
		ifAliases[i] = nullableStringRef(stringsDict, topologyV1ScalarLabelValue(row.values["if_alias"]))
		macs[i] = nullableStringRef(stringsDict, topologyV1ScalarLabelValue(row.values["mac"]))
		speeds[i] = nullableUintValue(row.values["speed"])
		topologyRoles[i] = nullableStringRef(stringsDict, topologyV1ScalarLabelValue(row.values["topology_role"]))
		operStatuses[i] = nullableStringRef(stringsDict, firstNonEmptyString(
			topologyV1ScalarLabelValue(row.values["oper_status"]),
			topologyV1ScalarLabelValue(row.values["if_oper_status"]),
		))
		adminStatuses[i] = nullableStringRef(stringsDict, firstNonEmptyString(
			topologyV1ScalarLabelValue(row.values["admin_status"]),
			topologyV1ScalarLabelValue(row.values["if_admin_status"]),
		))
		portTypes[i] = nullableStringRef(stringsDict, firstNonEmptyString(
			topologyV1ScalarLabelValue(row.values["port_type"]),
			topologyV1ScalarLabelValue(row.values["if_type"]),
		))
		linkModes[i] = nullableStringRef(stringsDict, topologyV1ScalarLabelValue(row.values["link_mode"]))
		stpStates[i] = nullableStringRef(stringsDict, topologyV1ScalarLabelValue(row.values["stp_state"]))
		vlanIDs[i] = stringArrayCell(anyStringSlice(row.values["vlan_ids"]))
		if isEmptyArrayCell(vlanIDs[i]) {
			vlanIDs[i] = nil
		}
		fdbMACCounts[i] = nullableUintValue(row.values["fdb_mac_count"])
		linkCounts[i] = nullableUintValue(row.values["link_count"])
		neighborCounts[i] = nullableUintValue(row.values["neighbor_count"])
		if neighborCounts[i] == nil {
			if values, ok := anyMapSlice(row.values["neighbors"]); ok && len(values) > 0 {
				neighborCounts[i] = uint64(len(values))
			}
		}
		if summary, ok := snmpTopologyV1PortNeighborSummaryFor(row, portName, portNeighborSummaries); ok {
			neighborActors[i] = summary.remoteActor
			neighborPortNames[i] = nullableStringRef(stringsDict, summary.remotePortName)
		}
		neighbors[i] = nullableJSON(row.values["neighbors"])
		vlans[i] = nullableJSON(row.values["vlans"])
		duplexes[i] = nullableStringRef(stringsDict, topologyV1ScalarLabelValue(row.values["duplex"]))
		linkModeConfidences[i] = nullableStringRef(stringsDict, topologyV1ScalarLabelValue(row.values["link_mode_confidence"]))
		topologyRoleConfidences[i] = nullableStringRef(stringsDict, topologyV1ScalarLabelValue(row.values["topology_role_confidence"]))
		linkModeSources[i] = stringArrayCell(anyStringSlice(row.values["link_mode_sources"]))
		if isEmptyArrayCell(linkModeSources[i]) {
			linkModeSources[i] = nil
		}
		topologyRoleSources[i] = stringArrayCell(anyStringSlice(row.values["topology_role_sources"]))
		if isEmptyArrayCell(topologyRoleSources[i]) {
			topologyRoleSources[i] = nil
		}
		lastChanges[i] = nullableStringRef(stringsDict, topologyV1ScalarLabelValue(row.values["last_change"]))
	}

	return topologyv1.MustTable(len(rows), snmpTopologyV1ActorPortsColumns(), []topologyv1.ColumnEncoding{
		topologyv1.Values(actorRefs...),
		topologyv1.Values(ifIndexes...),
		topologyv1.Values(portIDs...),
		topologyv1.Values(names...),
		topologyv1.Values(ifNames...),
		topologyv1.Values(ifDescrs...),
		topologyv1.Values(ifAliases...),
		topologyv1.Values(macs...),
		topologyv1.Values(speeds...),
		topologyv1.Values(topologyRoles...),
		topologyv1.Values(operStatuses...),
		topologyv1.Values(adminStatuses...),
		topologyv1.Values(portTypes...),
		topologyv1.Values(linkModes...),
		topologyv1.Values(stpStates...),
		topologyv1.Values(vlanIDs...),
		topologyv1.Values(fdbMACCounts...),
		topologyv1.Values(linkCounts...),
		topologyv1.Values(neighborCounts...),
		topologyv1.Values(neighborActors...),
		topologyv1.Values(neighborPortNames...),
		topologyv1.Values(neighbors...),
		topologyv1.Values(vlans...),
		topologyv1.Values(duplexes...),
		topologyv1.Values(linkModeConfidences...),
		topologyv1.Values(topologyRoleConfidences...),
		topologyv1.Values(linkModeSources...),
		topologyv1.Values(topologyRoleSources...),
		topologyv1.Values(lastChanges...),
	})
}

func buildSNMPTopologyV1ActorPortLinksTable(
	links []topologyLink,
	actorIndex map[string]int,
	stringsDict *topologyv1.StringDictionary,
) (topologyv1.Table, error) {
	rows := &snmpTopologyV1ActorPortLinkRows{}
	appendSide := func(linkIndex int, link topologyLink, actorID, remoteActorID string, endpoint, remoteEndpoint topologyLinkEndpoint) error {
		actorRef, ok := actorIndex[strings.TrimSpace(actorID)]
		if !ok {
			return fmt.Errorf("link %d references unknown actor %q", linkIndex, actorID)
		}
		remoteActorRef, ok := actorIndex[strings.TrimSpace(remoteActorID)]
		if !ok {
			return fmt.Errorf("link %d references unknown remote actor %q", linkIndex, remoteActorID)
		}
		protocol := firstNonEmptyString(link.Protocol, link.LinkType, "l2")
		linkType := snmpTopologyV1LinkType(link)

		rows.actors = append(rows.actors, actorRef)
		rows.links = append(rows.links, linkIndex)
		rows.remoteActors = append(rows.remoteActors, remoteActorRef)
		rows.ifIndexes = append(rows.ifIndexes, nullableUintValue(endpoint.Attributes["if_index"]))
		rows.portIDs = append(rows.portIDs, nullableStringRef(stringsDict, topologyV1EndpointString(endpoint, "port_id")))
		rows.portNames = append(rows.portNames, nullableStringRef(stringsDict, topologyV1EndpointPortName(endpoint)))
		rows.remoteIfIndexes = append(rows.remoteIfIndexes, nullableUintValue(remoteEndpoint.Attributes["if_index"]))
		rows.remotePortIDs = append(rows.remotePortIDs, nullableStringRef(stringsDict, topologyV1EndpointString(remoteEndpoint, "port_id")))
		rows.remotePortNames = append(rows.remotePortNames, nullableStringRef(stringsDict, topologyV1EndpointPortName(remoteEndpoint)))
		rows.types = append(rows.types, stringsDict.Ref(linkType))
		rows.protocols = append(rows.protocols, stringsDict.Ref(protocol))
		rows.states = append(rows.states, nullableStringRef(stringsDict, link.State))
		rows.evidenceCounts = append(rows.evidenceCounts, uint64(1))
		rows.confidences = append(rows.confidences, nullableStringRef(stringsDict, topologyMetricValueString(link.Metrics, "confidence")))
		rows.inferences = append(rows.inferences, nullableStringRef(stringsDict, topologyMetricValueString(link.Metrics, "inference")))
		rows.attachmentModes = append(rows.attachmentModes, nullableStringRef(stringsDict, topologyMetricValueString(link.Metrics, "attachment_mode")))
		rows.discoveredAt = append(rows.discoveredAt, nullableTime(link.DiscoveredAt))
		rows.lastSeen = append(rows.lastSeen, nullableTime(link.LastSeen))
		return nil
	}

	for i, link := range links {
		if snmpTopologyV1LinkIsLogicalL3(link) {
			continue
		}
		if err := appendSide(i, link, link.SrcActorID, link.DstActorID, link.Src, link.Dst); err != nil {
			return topologyv1.Table{}, err
		}
		if err := appendSide(i, link, link.DstActorID, link.SrcActorID, link.Dst, link.Src); err != nil {
			return topologyv1.Table{}, err
		}
	}

	return rows.table(), nil
}

type snmpTopologyV1ActorPortLinkRows struct {
	actors          []any
	links           []any
	remoteActors    []any
	ifIndexes       []any
	portIDs         []any
	portNames       []any
	remoteIfIndexes []any
	remotePortIDs   []any
	remotePortNames []any
	types           []any
	protocols       []any
	states          []any
	evidenceCounts  []any
	confidences     []any
	inferences      []any
	attachmentModes []any
	discoveredAt    []any
	lastSeen        []any
}

func (rows *snmpTopologyV1ActorPortLinkRows) table() topologyv1.Table {
	return topologyv1.MustTable(len(rows.actors),
		snmpTopologyV1ActorPortLinksColumns(),
		[]topologyv1.ColumnEncoding{
			topologyv1.Values(rows.actors...),
			topologyv1.Values(rows.links...),
			topologyv1.Values(rows.remoteActors...),
			topologyv1.Values(rows.ifIndexes...),
			topologyv1.Values(rows.portIDs...),
			topologyv1.Values(rows.portNames...),
			topologyv1.Values(rows.remoteIfIndexes...),
			topologyv1.Values(rows.remotePortIDs...),
			topologyv1.Values(rows.remotePortNames...),
			topologyv1.Values(rows.types...),
			topologyv1.Values(rows.protocols...),
			topologyv1.Values(rows.states...),
			topologyv1.Values(rows.evidenceCounts...),
			topologyv1.Values(rows.confidences...),
			topologyv1.Values(rows.inferences...),
			topologyv1.Values(rows.attachmentModes...),
			topologyv1.Values(rows.discoveredAt...),
			topologyv1.Values(rows.lastSeen...),
		},
	)
}
