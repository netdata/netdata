// SPDX-License-Identifier: GPL-3.0-or-later

package topologyv1

import (
	"fmt"
	"strings"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/l2topology"
	topologyapi "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"
)

func buildSNMPTopologyV1Actors(actors []topologymodel.Actor, stringsDict *topologyapi.StringDictionary) (topologyapi.Table, map[string]int) {
	actorIndex := make(map[string]int, len(actors))
	usedActorIDs := make(map[string]struct{}, len(actors))
	for _, actor := range actors {
		if actorID := strings.TrimSpace(actor.ActorID); actorID != "" {
			usedActorIDs[actorID] = struct{}{}
		}
	}
	ids := make([]any, len(actors))
	types := make([]any, len(actors))
	layers := make([]any, len(actors))
	sources := make([]any, len(actors))
	displayNames := make([]any, len(actors))
	chassisIDs := make([]any, len(actors))
	macAddresses := make([]any, len(actors))
	ipAddresses := make([]any, len(actors))
	hostnames := make([]any, len(actors))
	dnsNames := make([]any, len(actors))
	sysObjectIDs := make([]any, len(actors))
	sysNames := make([]any, len(actors))
	parentDevices := make([]any, len(actors))
	vendors := make([]any, len(actors))
	models := make([]any, len(actors))
	sysDescrs := make([]any, len(actors))
	sysLocations := make([]any, len(actors))
	sysContacts := make([]any, len(actors))
	managementIPs := make([]any, len(actors))
	protocols := make([]any, len(actors))
	capabilities := make([]any, len(actors))
	portsTotal := make([]any, len(actors))
	vlanCounts := make([]any, len(actors))
	fdbTotalMACs := make([]any, len(actors))
	lldpNeighborCounts := make([]any, len(actors))
	cdpNeighborCounts := make([]any, len(actors))
	endpointsTotal := make([]any, len(actors))
	chartIDPrefixes := make([]any, len(actors))
	netdataHostIDs := make([]any, len(actors))

	for i, actor := range actors {
		actorID := strings.TrimSpace(actor.ActorID)
		if actorID == "" {
			actorID = snmpTopologyV1FallbackActorID(actor, i, usedActorIDs)
		}
		actorIndex[actorID] = i
		ids[i] = stringsDict.Ref(actorID)
		types[i] = stringsDict.Ref(snmpTopologyV1ActorType(actor.ActorType))
		layers[i] = stringsDict.Ref(snmpTopologyV1ActorLayer(actor))
		sources[i] = stringsDict.Ref(topologyutil.FirstNonEmptyString(actor.Source, snmpTopologyV1ProducerSource))
		displayNames[i] = nullableStringRef(stringsDict, snmpTopologyV1DisplayName(actor))
		chassisIDs[i] = stringArrayCell(actor.Match.ChassisIDs)
		macAddresses[i] = stringArrayCell(actor.Match.MacAddresses)
		ipAddresses[i] = stringArrayCell(actor.Match.IPAddresses)
		hostnames[i] = stringArrayCell(actor.Match.Hostnames)
		dnsNames[i] = stringArrayCell(actor.Match.DNSNames)
		sysObjectIDs[i] = stringsDict.Ref(actor.Match.SysObjectID)
		sysNames[i] = stringsDict.Ref(actor.Match.SysName)
		parentDevices[i] = stringArrayCell(topologymodel.ActorDetailParentDevices(actor))
		vendors[i] = nullableStringRef(stringsDict, topologymodel.ActorDetailVendor(actor))
		models[i] = nullableStringRef(stringsDict, topologymodel.ActorDetailModel(actor))
		sysDescrs[i] = nullableStringRef(stringsDict, topologymodel.ActorDetailSysDescr(actor))
		sysLocations[i] = nullableStringRef(stringsDict, topologymodel.ActorDetailSysLocation(actor))
		sysContacts[i] = nullableStringRef(stringsDict, topologymodel.ActorDetailSysContact(actor))
		managementIPs[i] = nullableStringRef(stringsDict, topologymodel.ActorDetailManagementIP(actor))
		protocols[i] = stringArrayCell(topologymodel.ActorDetailProtocols(actor))
		if isEmptyArrayCell(protocols[i]) {
			protocols[i] = nil
		}
		capabilities[i] = stringArrayCell(topologymodel.ActorDetailCapabilities(actor))
		if isEmptyArrayCell(capabilities[i]) {
			capabilities[i] = nil
		}
		portsTotal[i] = nullableOptionalUintValue(topologymodel.ActorDetailPortsTotal(actor))
		vlanCounts[i] = nullableOptionalUintValue(topologymodel.ActorDetailVLANCount(actor))
		fdbTotalMACs[i] = nullableOptionalUintValue(topologymodel.ActorDetailFDBTotalMACs(actor))
		lldpNeighborCounts[i] = nullableOptionalUintValue(topologymodel.ActorDetailLLDPNeighborCount(actor))
		cdpNeighborCounts[i] = nullableOptionalUintValue(topologymodel.ActorDetailCDPNeighborCount(actor))
		endpointsTotal[i] = nullableOptionalUintValue(topologymodel.ActorDetailEndpointsTotal(actor))
		chartIDPrefixes[i] = nullableStringRef(stringsDict, topologymodel.ActorDetailChartIDPrefix(actor))
		netdataHostIDs[i] = nullableStringRef(stringsDict, topologymodel.ActorDetailNetdataHostID(actor))
	}

	return topologyapi.MustTable(len(actors),
		[]topologyapi.Column{
			topologyapi.NewColumn("id", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithRole("identity")),
			topologyapi.NewColumn("type", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithRole("group_key")),
			topologyapi.NewColumn("layer", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithRole("group_key")),
			topologyapi.NewColumn("source", "string_ref", topologyapi.WithDictionary("strings")),
			topologyapi.NewColumn("display_name", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithNullable(), topologyapi.WithRole("attribute")),
			topologyapi.NewColumn("chassis_ids", "array", topologyapi.WithRole("merge_identity")),
			topologyapi.NewColumn("mac_addresses", "array", topologyapi.WithRole("merge_identity")),
			topologyapi.NewColumn("ip_addresses", "array", topologyapi.WithRole("merge_identity")),
			topologyapi.NewColumn("hostnames", "array"),
			topologyapi.NewColumn("dns_names", "array"),
			topologyapi.NewColumn("sys_object_id", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithRole("merge_identity")),
			topologyapi.NewColumn("sys_name", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithRole("merge_identity")),
			topologyapi.NewColumn("parent_devices", "array", topologyapi.WithRole("parent_identity")),
			topologyapi.NewColumn("vendor", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithNullable()),
			topologyapi.NewColumn("model", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithNullable()),
			topologyapi.NewColumn("sys_descr", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithNullable()),
			topologyapi.NewColumn("sys_location", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithNullable()),
			topologyapi.NewColumn("sys_contact", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithNullable()),
			topologyapi.NewColumn("management_ip", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithNullable()),
			topologyapi.NewColumn("protocols", "array", topologyapi.WithNullable()),
			topologyapi.NewColumn("capabilities", "array", topologyapi.WithNullable()),
			topologyapi.NewColumn("ports_total", "uint", topologyapi.WithNullable()),
			topologyapi.NewColumn("vlan_count", "uint", topologyapi.WithNullable()),
			topologyapi.NewColumn("fdb_total_macs", "uint", topologyapi.WithNullable()),
			topologyapi.NewColumn("lldp_neighbor_count", "uint", topologyapi.WithNullable()),
			topologyapi.NewColumn("cdp_neighbor_count", "uint", topologyapi.WithNullable()),
			topologyapi.NewColumn("endpoints_total", "uint", topologyapi.WithNullable()),
			topologyapi.NewColumn("chart_id_prefix", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithNullable()),
			topologyapi.NewColumn("netdata_host_id", "string_ref", topologyapi.WithDictionary("strings"), topologyapi.WithNullable()),
		},
		[]topologyapi.ColumnEncoding{
			topologyapi.Values(ids...),
			topologyapi.Values(types...),
			topologyapi.Values(layers...),
			topologyapi.Values(sources...),
			topologyapi.Values(displayNames...),
			topologyapi.Values(chassisIDs...),
			topologyapi.Values(macAddresses...),
			topologyapi.Values(ipAddresses...),
			topologyapi.Values(hostnames...),
			topologyapi.Values(dnsNames...),
			topologyapi.Values(sysObjectIDs...),
			topologyapi.Values(sysNames...),
			topologyapi.Values(parentDevices...),
			topologyapi.Values(vendors...),
			topologyapi.Values(models...),
			topologyapi.Values(sysDescrs...),
			topologyapi.Values(sysLocations...),
			topologyapi.Values(sysContacts...),
			topologyapi.Values(managementIPs...),
			topologyapi.Values(protocols...),
			topologyapi.Values(capabilities...),
			topologyapi.Values(portsTotal...),
			topologyapi.Values(vlanCounts...),
			topologyapi.Values(fdbTotalMACs...),
			topologyapi.Values(lldpNeighborCounts...),
			topologyapi.Values(cdpNeighborCounts...),
			topologyapi.Values(endpointsTotal...),
			topologyapi.Values(chartIDPrefixes...),
			topologyapi.Values(netdataHostIDs...),
		},
	), actorIndex
}

func snmpTopologyV1FallbackActorID(actor topologymodel.Actor, index int, used map[string]struct{}) string {
	for _, candidate := range []struct {
		kind  string
		value string
	}{
		{kind: "netdata_node", value: actor.Match.NetdataNodeID},
		{kind: "machine", value: actor.Match.NetdataMachineGUID},
		{kind: "cloud_instance", value: actor.Match.CloudInstanceID},
		{kind: "chassis", value: firstString(actor.Match.ChassisIDs)},
		{kind: "mac", value: firstString(actor.Match.MacAddresses)},
		{kind: "ip", value: firstString(actor.Match.IPAddresses)},
		{kind: "sys_name", value: actor.Match.SysName},
		{kind: "hostname", value: firstString(actor.Match.Hostnames)},
		{kind: "dns", value: firstString(actor.Match.DNSNames)},
		{kind: "sys_object_id", value: actor.Match.SysObjectID},
		{kind: "display_name", value: snmpTopologyV1DisplayName(actor)},
	} {
		if value := strings.TrimSpace(candidate.value); value != "" {
			return snmpTopologyV1UniqueFallbackActorID(
				fmt.Sprintf("generated:%s:%s:%s", snmpTopologyV1ActorType(actor.ActorType), candidate.kind, topologyID(value, "actor")),
				used,
			)
		}
	}

	return snmpTopologyV1UniqueFallbackActorID(fmt.Sprintf("generated:%d", index), used)
}

func snmpTopologyV1UniqueFallbackActorID(actorID string, used map[string]struct{}) string {
	if _, ok := used[actorID]; !ok {
		used[actorID] = struct{}{}
		return actorID
	}
	for suffix := 2; ; suffix++ {
		candidate := fmt.Sprintf("%s_%d", actorID, suffix)
		if _, ok := used[candidate]; !ok {
			used[candidate] = struct{}{}
			return candidate
		}
	}
}

func snmpTopologyV1ActorType(actorType string) string {
	normalized := strings.ToLower(strings.TrimSpace(actorType))
	if topologyengine.IsDeviceActorType(normalized) {
		return normalized
	}
	switch normalized {
	case snmpTopologyV1ActorDevice:
		return snmpTopologyV1ActorDevice
	case snmpTopologyV1ActorEndpoint:
		return snmpTopologyV1ActorEndpoint
	case snmpTopologyV1ActorSegment:
		return snmpTopologyV1ActorSegment
	default:
		return "custom"
	}
}

func snmpTopologyV1DisplayName(actor topologymodel.Actor) string {
	return topologymodel.ActorDetailDisplayName(actor)
}

func snmpTopologyV1ActorLayer(actor topologymodel.Actor) string {
	switch snmpTopologyV1ActorType(actor.ActorType) {
	case snmpTopologyV1ActorEndpoint, snmpTopologyV1ActorSegment:
		return "network"
	default:
		if topologyengine.IsDeviceActorType(actor.ActorType) {
			return "network"
		}
		return "custom"
	}
}
