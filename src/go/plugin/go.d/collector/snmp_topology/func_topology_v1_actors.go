// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"fmt"
	topologyengine "github.com/netdata/netdata/go/plugins/pkg/l2topology"
	topologyv1 "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
	"strings"
)

func buildSNMPTopologyV1Actors(actors []topologyActor, stringsDict *topologyv1.StringDictionary) (topologyv1.Table, map[string]int) {
	actorIndex := make(map[string]int, len(actors))
	usedFallbackActorIDs := make(map[string]struct{})
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
			actorID = snmpTopologyV1FallbackActorID(actor, i, usedFallbackActorIDs)
		}
		actorIndex[actorID] = i
		ids[i] = stringsDict.Ref(actorID)
		types[i] = stringsDict.Ref(snmpTopologyV1ActorType(actor.ActorType))
		layers[i] = stringsDict.Ref(snmpTopologyV1ActorLayer(actor))
		sources[i] = stringsDict.Ref(firstNonEmptyString(actor.Source, snmpTopologyV1ProducerSource))
		displayNames[i] = nullableStringRef(stringsDict, snmpTopologyV1DisplayName(actor))
		chassisIDs[i] = stringArrayCell(actor.Match.ChassisIDs)
		macAddresses[i] = stringArrayCell(actor.Match.MacAddresses)
		ipAddresses[i] = stringArrayCell(actor.Match.IPAddresses)
		hostnames[i] = stringArrayCell(actor.Match.Hostnames)
		dnsNames[i] = stringArrayCell(actor.Match.DNSNames)
		sysObjectIDs[i] = stringsDict.Ref(actor.Match.SysObjectID)
		sysNames[i] = stringsDict.Ref(actor.Match.SysName)
		parentDevices[i] = stringArrayCell(anyStringSlice(actor.Attributes["parent_devices"]))
		vendors[i] = nullableStringRef(stringsDict, firstNonEmptyString(anyStringValue(actor.Attributes["vendor"]), anyStringValue(actor.Attributes["vendor_derived"])))
		models[i] = nullableStringRef(stringsDict, anyStringValue(actor.Attributes["model"]))
		sysDescrs[i] = nullableStringRef(stringsDict, anyStringValue(actor.Attributes["sys_descr"]))
		sysLocations[i] = nullableStringRef(stringsDict, anyStringValue(actor.Attributes["sys_location"]))
		sysContacts[i] = nullableStringRef(stringsDict, anyStringValue(actor.Attributes["sys_contact"]))
		managementIPs[i] = nullableStringRef(stringsDict, anyStringValue(actor.Attributes["management_ip"]))
		protocols[i] = stringArrayCell(anyStringSlice(actor.Attributes["protocols"]))
		if isEmptyArrayCell(protocols[i]) {
			// Older SNMP topology payloads used learned_sources for discovered protocols.
			protocols[i] = stringArrayCell(anyStringSlice(actor.Attributes["learned_sources"]))
		}
		if isEmptyArrayCell(protocols[i]) {
			protocols[i] = nil
		}
		capabilities[i] = stringArrayCell(anyStringSlice(actor.Attributes["capabilities"]))
		if isEmptyArrayCell(capabilities[i]) {
			capabilities[i] = nil
		}
		portsTotal[i] = nullableUintValue(actor.Attributes["ports_total"])
		vlanCounts[i] = nullableUintValue(actor.Attributes["vlan_count"])
		fdbTotalMACs[i] = nullableUintValue(actor.Attributes["fdb_total_macs"])
		lldpNeighborCounts[i] = nullableUintValue(actor.Attributes["lldp_neighbor_count"])
		cdpNeighborCounts[i] = nullableUintValue(actor.Attributes["cdp_neighbor_count"])
		endpointsTotal[i] = nullableUintValue(actor.Attributes["endpoints_total"])
		chartIDPrefixes[i] = nullableStringRef(stringsDict, anyStringValue(actor.Attributes["chart_id_prefix"]))
		netdataHostIDs[i] = nullableStringRef(stringsDict, anyStringValue(actor.Attributes["netdata_host_id"]))
	}

	return topologyv1.MustTable(len(actors),
		[]topologyv1.Column{
			topologyv1.NewColumn("id", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithRole("identity")),
			topologyv1.NewColumn("type", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithRole("group_key")),
			topologyv1.NewColumn("layer", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithRole("group_key")),
			topologyv1.NewColumn("source", "string_ref", topologyv1.WithDictionary("strings")),
			topologyv1.NewColumn("display_name", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable(), topologyv1.WithRole("attribute")),
			topologyv1.NewColumn("chassis_ids", "array", topologyv1.WithRole("merge_identity")),
			topologyv1.NewColumn("mac_addresses", "array", topologyv1.WithRole("merge_identity")),
			topologyv1.NewColumn("ip_addresses", "array", topologyv1.WithRole("merge_identity")),
			topologyv1.NewColumn("hostnames", "array"),
			topologyv1.NewColumn("dns_names", "array"),
			topologyv1.NewColumn("sys_object_id", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithRole("merge_identity")),
			topologyv1.NewColumn("sys_name", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithRole("merge_identity")),
			topologyv1.NewColumn("parent_devices", "array", topologyv1.WithRole("parent_identity")),
			topologyv1.NewColumn("vendor", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable()),
			topologyv1.NewColumn("model", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable()),
			topologyv1.NewColumn("sys_descr", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable()),
			topologyv1.NewColumn("sys_location", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable()),
			topologyv1.NewColumn("sys_contact", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable()),
			topologyv1.NewColumn("management_ip", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable()),
			topologyv1.NewColumn("protocols", "array", topologyv1.WithNullable()),
			topologyv1.NewColumn("capabilities", "array", topologyv1.WithNullable()),
			topologyv1.NewColumn("ports_total", "uint", topologyv1.WithNullable()),
			topologyv1.NewColumn("vlan_count", "uint", topologyv1.WithNullable()),
			topologyv1.NewColumn("fdb_total_macs", "uint", topologyv1.WithNullable()),
			topologyv1.NewColumn("lldp_neighbor_count", "uint", topologyv1.WithNullable()),
			topologyv1.NewColumn("cdp_neighbor_count", "uint", topologyv1.WithNullable()),
			topologyv1.NewColumn("endpoints_total", "uint", topologyv1.WithNullable()),
			topologyv1.NewColumn("chart_id_prefix", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable()),
			topologyv1.NewColumn("netdata_host_id", "string_ref", topologyv1.WithDictionary("strings"), topologyv1.WithNullable()),
		},
		[]topologyv1.ColumnEncoding{
			topologyv1.Values(ids...),
			topologyv1.Values(types...),
			topologyv1.Values(layers...),
			topologyv1.Values(sources...),
			topologyv1.Values(displayNames...),
			topologyv1.Values(chassisIDs...),
			topologyv1.Values(macAddresses...),
			topologyv1.Values(ipAddresses...),
			topologyv1.Values(hostnames...),
			topologyv1.Values(dnsNames...),
			topologyv1.Values(sysObjectIDs...),
			topologyv1.Values(sysNames...),
			topologyv1.Values(parentDevices...),
			topologyv1.Values(vendors...),
			topologyv1.Values(models...),
			topologyv1.Values(sysDescrs...),
			topologyv1.Values(sysLocations...),
			topologyv1.Values(sysContacts...),
			topologyv1.Values(managementIPs...),
			topologyv1.Values(protocols...),
			topologyv1.Values(capabilities...),
			topologyv1.Values(portsTotal...),
			topologyv1.Values(vlanCounts...),
			topologyv1.Values(fdbTotalMACs...),
			topologyv1.Values(lldpNeighborCounts...),
			topologyv1.Values(cdpNeighborCounts...),
			topologyv1.Values(endpointsTotal...),
			topologyv1.Values(chartIDPrefixes...),
			topologyv1.Values(netdataHostIDs...),
		},
	), actorIndex
}

func snmpTopologyV1FallbackActorID(actor topologyActor, index int, used map[string]struct{}) string {
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

func snmpTopologyV1DisplayName(actor topologyActor) string {
	return firstNonEmptyString(
		anyStringValue(actor.Attributes["display_name"]),
		anyStringValue(actor.Attributes["name"]),
		actor.Labels["display_name"],
		actor.Labels["name"],
		actor.Match.SysName,
		firstString(actor.Match.Hostnames),
		firstString(actor.Match.DNSNames),
	)
}

func snmpTopologyV1ActorLayer(actor topologyActor) string {
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
