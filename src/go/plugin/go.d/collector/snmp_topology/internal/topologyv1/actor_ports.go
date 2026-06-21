// SPDX-License-Identifier: GPL-3.0-or-later

package topologyv1

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/l2topology"
	topologyapi "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
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
	links []topologymodel.Link,
	actorIndex map[string]int,
) map[snmpTopologyV1PortNeighborKey]snmpTopologyV1PortNeighborSummary {
	summaries := make(map[snmpTopologyV1PortNeighborKey]snmpTopologyV1PortNeighborSummary)
	appendSide := func(actorID, remoteActorID string, endpoint, remoteEndpoint topologymodel.LinkEndpoint) {
		actorRef, ok := actorIndex[strings.TrimSpace(actorID)]
		if !ok {
			return
		}
		remoteActorRef, ok := actorIndex[strings.TrimSpace(remoteActorID)]
		if !ok {
			return
		}
		key := snmpTopologyV1PortNeighborKeyFor(actorRef, nullableEndpointIfIndex(endpoint), topologyV1EndpointPortName(endpoint))
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

func collectSNMPTopologyV1ActorTableRows(actors []topologymodel.Actor) map[string][]topologyV1DynamicRow {
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
		"name":                     topologyutil.FirstNonEmptyString(port.Name, port.IfName, port.PortID),
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
	stringsDict *topologyapi.StringDictionary,
	portNeighborSummaries map[snmpTopologyV1PortNeighborKey]snmpTopologyV1PortNeighborSummary,
) topologyapi.Table {
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
		portName := topologyutil.FirstNonEmptyString(
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
		operStatuses[i] = nullableStringRef(stringsDict, topologyutil.FirstNonEmptyString(
			topologyV1ScalarLabelValue(row.values["oper_status"]),
			topologyV1ScalarLabelValue(row.values["if_oper_status"]),
		))
		adminStatuses[i] = nullableStringRef(stringsDict, topologyutil.FirstNonEmptyString(
			topologyV1ScalarLabelValue(row.values["admin_status"]),
			topologyV1ScalarLabelValue(row.values["if_admin_status"]),
		))
		portTypes[i] = nullableStringRef(stringsDict, topologyutil.FirstNonEmptyString(
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

	return topologyapi.MustTable(len(rows), snmpTopologyV1ActorPortsColumns(), []topologyapi.ColumnEncoding{
		topologyapi.Values(actorRefs...),
		topologyapi.Values(ifIndexes...),
		topologyapi.Values(portIDs...),
		topologyapi.Values(names...),
		topologyapi.Values(ifNames...),
		topologyapi.Values(ifDescrs...),
		topologyapi.Values(ifAliases...),
		topologyapi.Values(macs...),
		topologyapi.Values(speeds...),
		topologyapi.Values(topologyRoles...),
		topologyapi.Values(operStatuses...),
		topologyapi.Values(adminStatuses...),
		topologyapi.Values(portTypes...),
		topologyapi.Values(linkModes...),
		topologyapi.Values(stpStates...),
		topologyapi.Values(vlanIDs...),
		topologyapi.Values(fdbMACCounts...),
		topologyapi.Values(linkCounts...),
		topologyapi.Values(neighborCounts...),
		topologyapi.Values(neighborActors...),
		topologyapi.Values(neighborPortNames...),
		topologyapi.Values(neighbors...),
		topologyapi.Values(vlans...),
		topologyapi.Values(duplexes...),
		topologyapi.Values(linkModeConfidences...),
		topologyapi.Values(topologyRoleConfidences...),
		topologyapi.Values(linkModeSources...),
		topologyapi.Values(topologyRoleSources...),
		topologyapi.Values(lastChanges...),
	})
}

func buildSNMPTopologyV1ActorPortLinksTable(
	links []topologymodel.Link,
	actorIndex map[string]int,
	stringsDict *topologyapi.StringDictionary,
) (topologyapi.Table, error) {
	rows := &snmpTopologyV1ActorPortLinkRows{}
	appendSide := func(linkIndex int, link topologymodel.Link, actorID, remoteActorID string, endpoint, remoteEndpoint topologymodel.LinkEndpoint) error {
		actorRef, ok := actorIndex[strings.TrimSpace(actorID)]
		if !ok {
			return fmt.Errorf("link %d references unknown actor %q", linkIndex, actorID)
		}
		remoteActorRef, ok := actorIndex[strings.TrimSpace(remoteActorID)]
		if !ok {
			return fmt.Errorf("link %d references unknown remote actor %q", linkIndex, remoteActorID)
		}
		protocol := topologyutil.FirstNonEmptyString(link.Protocol, link.LinkType, "l2")
		linkType := snmpTopologyV1LinkType(link)

		rows.actors = append(rows.actors, actorRef)
		rows.links = append(rows.links, linkIndex)
		rows.remoteActors = append(rows.remoteActors, remoteActorRef)
		rows.ifIndexes = append(rows.ifIndexes, nullableEndpointIfIndex(endpoint))
		rows.portIDs = append(rows.portIDs, nullableStringRef(stringsDict, topologyV1EndpointString(endpoint, "port_id")))
		rows.portNames = append(rows.portNames, nullableStringRef(stringsDict, topologyV1EndpointPortName(endpoint)))
		rows.remoteIfIndexes = append(rows.remoteIfIndexes, nullableEndpointIfIndex(remoteEndpoint))
		rows.remotePortIDs = append(rows.remotePortIDs, nullableStringRef(stringsDict, topologyV1EndpointString(remoteEndpoint, "port_id")))
		rows.remotePortNames = append(rows.remotePortNames, nullableStringRef(stringsDict, topologyV1EndpointPortName(remoteEndpoint)))
		rows.types = append(rows.types, stringsDict.Ref(linkType))
		rows.protocols = append(rows.protocols, stringsDict.Ref(protocol))
		rows.states = append(rows.states, nullableStringRef(stringsDict, link.State))
		rows.evidenceCounts = append(rows.evidenceCounts, uint64(1))
		rows.confidences = append(rows.confidences, nullableStringRef(stringsDict, topologymodel.LinkConfidenceValue(link)))
		rows.inferences = append(rows.inferences, nullableStringRef(stringsDict, topologymodel.LinkInferenceValue(link)))
		rows.attachmentModes = append(rows.attachmentModes, nullableStringRef(stringsDict, topologymodel.LinkAttachmentModeValue(link)))
		rows.discoveredAt = append(rows.discoveredAt, nullableTime(link.DiscoveredAt))
		rows.lastSeen = append(rows.lastSeen, nullableTime(link.LastSeen))
		return nil
	}

	for i, link := range links {
		if snmpTopologyV1LinkIsLogicalL3(link) {
			continue
		}
		if err := appendSide(i, link, link.SrcActorID, link.DstActorID, link.Src, link.Dst); err != nil {
			return topologyapi.Table{}, err
		}
		if err := appendSide(i, link, link.DstActorID, link.SrcActorID, link.Dst, link.Src); err != nil {
			return topologyapi.Table{}, err
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

func (rows *snmpTopologyV1ActorPortLinkRows) table() topologyapi.Table {
	return topologyapi.MustTable(len(rows.actors),
		snmpTopologyV1ActorPortLinksColumns(),
		[]topologyapi.ColumnEncoding{
			topologyapi.Values(rows.actors...),
			topologyapi.Values(rows.links...),
			topologyapi.Values(rows.remoteActors...),
			topologyapi.Values(rows.ifIndexes...),
			topologyapi.Values(rows.portIDs...),
			topologyapi.Values(rows.portNames...),
			topologyapi.Values(rows.remoteIfIndexes...),
			topologyapi.Values(rows.remotePortIDs...),
			topologyapi.Values(rows.remotePortNames...),
			topologyapi.Values(rows.types...),
			topologyapi.Values(rows.protocols...),
			topologyapi.Values(rows.states...),
			topologyapi.Values(rows.evidenceCounts...),
			topologyapi.Values(rows.confidences...),
			topologyapi.Values(rows.inferences...),
			topologyapi.Values(rows.attachmentModes...),
			topologyapi.Values(rows.discoveredAt...),
			topologyapi.Values(rows.lastSeen...),
		},
	)
}
