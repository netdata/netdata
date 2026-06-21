// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"fmt"
	"strings"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/l2topology"
)

func topologyActorDetailDisplayName(actor topologyActor) string {
	return firstNonEmptyString(
		actor.Detail.L2.DisplayName,
		actor.Match.SysName,
		firstString(actor.Match.Hostnames),
		firstString(actor.Match.DNSNames),
	)
}

func topologyActorDetailDisplaySource(actor topologyActor) string {
	return actor.Detail.L2.DisplaySource
}

func topologyActorDetailParentDevices(actor topologyActor) []string {
	return actor.Detail.L2.Segment.ParentDevices
}

func topologyActorDetailVendor(actor topologyActor) string {
	return firstNonEmptyString(
		actor.Detail.SNMP.Vendor,
		actor.Detail.L2.Device.Vendor,
		actor.Detail.L2.Device.VendorDerived,
		actor.Detail.L2.Endpoint.Vendor,
		actor.Detail.L2.Endpoint.VendorDerived,
	)
}

func topologyActorDetailVendorDerived(actor topologyActor) string {
	return firstNonEmptyString(actor.Detail.L2.Device.VendorDerived, actor.Detail.L2.Endpoint.VendorDerived)
}

func topologyActorDetailModel(actor topologyActor) string {
	return actor.Detail.SNMP.Model
}

func topologyActorDetailSysDescr(actor topologyActor) string {
	return actor.Detail.SNMP.SysDescr
}

func topologyActorDetailSysLocation(actor topologyActor) string {
	return actor.Detail.SNMP.SysLocation
}

func topologyActorDetailSysContact(actor topologyActor) string {
	return actor.Detail.SNMP.SysContact
}

func topologyActorDetailManagementIP(actor topologyActor) string {
	return firstNonEmptyString(actor.Detail.SNMP.ManagementIP, actor.Detail.L2.Device.ManagementIP)
}

func topologyActorDetailManagementIPs(actor topologyActor) []string {
	out := make([]string, 0, 1+len(actor.Detail.SNMP.ManagementAddresses)+len(actor.Detail.L2.Device.ManagementAddresses))
	if ip := topologyActorDetailManagementIPValue(topologyActorDetailManagementIP(actor)); ip != "" {
		out = append(out, ip)
	}
	for _, address := range actor.Detail.SNMP.ManagementAddresses {
		if ip := topologyActorDetailManagementIPValue(address.Address); ip != "" {
			out = append(out, ip)
		}
	}
	for _, address := range actor.Detail.L2.Device.ManagementAddresses {
		if ip := topologyActorDetailManagementIPValue(address); ip != "" {
			out = append(out, ip)
		}
	}
	return deduplicateSortedStrings(out)
}

func topologyActorDetailManagementIPValue(value string) string {
	return normalizeNonUnspecifiedIPAddress(value)
}

func topologyActorDetailDeviceID(actor topologyActor) string {
	return actor.Detail.L2.Device.DeviceID
}

func topologyActorDetailProtocols(actor topologyActor) []string {
	if len(actor.Detail.L2.Device.Protocols) > 0 {
		return actor.Detail.L2.Device.Protocols
	}
	if len(actor.Detail.L2.Endpoint.LearnedSources) > 0 {
		return actor.Detail.L2.Endpoint.LearnedSources
	}
	return actor.Detail.L2.Segment.LearnedSources
}

func topologyActorDetailCapabilities(actor topologyActor) []string {
	if len(actor.Detail.SNMP.Capabilities) > 0 {
		return actor.Detail.SNMP.Capabilities
	}
	return actor.Detail.L2.Device.Capabilities
}

func topologyActorDetailPortsTotal(actor topologyActor) topologyengine.OptionalValue[int] {
	if actor.Detail.L2.Device.PortsTotal.Has {
		return actor.Detail.L2.Device.PortsTotal
	}
	if actor.Detail.L2.Segment.PortsTotal.Has {
		return actor.Detail.L2.Segment.PortsTotal
	}
	return topologyengine.OptionalValue[int]{}
}

func topologyActorDetailVLANCount(actor topologyActor) topologyengine.OptionalValue[int] {
	return actor.Detail.L2.Device.VLANCount
}

func topologyActorDetailFDBTotalMACs(actor topologyActor) topologyengine.OptionalValue[int] {
	return actor.Detail.L2.Device.FDBTotalMACs
}

func topologyActorDetailLLDPNeighborCount(actor topologyActor) topologyengine.OptionalValue[int] {
	return actor.Detail.L2.Device.LLDPNeighborCount
}

func topologyActorDetailCDPNeighborCount(actor topologyActor) topologyengine.OptionalValue[int] {
	return actor.Detail.L2.Device.CDPNeighborCount
}

func topologyActorDetailEndpointsTotal(actor topologyActor) topologyengine.OptionalValue[int] {
	return actor.Detail.L2.Segment.EndpointsTotal
}

func topologyActorDetailChartIDPrefix(actor topologyActor) string {
	return actor.Detail.SNMP.ChartIDPrefix
}

func topologyActorDetailChartContextPrefix(actor topologyActor) string {
	return actor.Detail.SNMP.ChartContextPrefix
}

func topologyActorDetailNetdataHostID(actor topologyActor) string {
	return actor.Detail.SNMP.NetdataHostID
}

func topologyActorDetailOSPFRouterID(actor topologyActor) string {
	return actor.Detail.SNMP.OSPFRouterID
}

func topologyActorDetailScalarLabelValues(actor topologyActor) map[string]string {
	out := map[string]string{
		"vendor":                  topologyActorDetailVendor(actor),
		"vendor_derived":          topologyActorDetailVendorDerived(actor),
		"model":                   topologyActorDetailModel(actor),
		"sys_descr":               topologyActorDetailSysDescr(actor),
		"sys_location":            topologyActorDetailSysLocation(actor),
		"sys_contact":             topologyActorDetailSysContact(actor),
		"management_ip":           topologyActorDetailManagementIP(actor),
		tagOSPFRouterID:           topologyActorDetailOSPFRouterID(actor),
		"display_name":            topologyActorDetailDisplayName(actor),
		"display_source":          topologyActorDetailDisplaySource(actor),
		"chart_id_prefix":         topologyActorDetailChartIDPrefix(actor),
		"chart_context_prefix":    topologyActorDetailChartContextPrefix(actor),
		"netdata_host_id":         topologyActorDetailNetdataHostID(actor),
		"ports_total":             topologyScalarOptionalInt(topologyActorDetailPortsTotal(actor)),
		"ports_up":                topologyScalarOptionalInt(actor.Detail.L2.Device.PortsUp),
		"ports_down":              topologyScalarOptionalInt(actor.Detail.L2.Device.PortsDown),
		"vlan_count":              topologyScalarOptionalInt(topologyActorDetailVLANCount(actor)),
		"fdb_total_macs":          topologyScalarOptionalInt(topologyActorDetailFDBTotalMACs(actor)),
		"lldp_neighbor_count":     topologyScalarOptionalInt(topologyActorDetailLLDPNeighborCount(actor)),
		"cdp_neighbor_count":      topologyScalarOptionalInt(topologyActorDetailCDPNeighborCount(actor)),
		"endpoints_total":         topologyScalarOptionalInt(topologyActorDetailEndpointsTotal(actor)),
		"if_admin_status_counts":  topologyIntMapLabel(actor.Detail.L2.Device.AdminStatusCounts),
		"if_oper_status_counts":   topologyIntMapLabel(actor.Detail.L2.Device.OperStatusCounts),
		"if_link_mode_counts":     topologyIntMapLabel(actor.Detail.L2.Device.LinkModeCounts),
		"if_topology_role_counts": topologyIntMapLabel(actor.Detail.L2.Device.TopologyRoleCounts),
	}
	for key, value := range out {
		if strings.TrimSpace(value) == "" {
			delete(out, key)
		}
	}
	return out
}

func topologyActorDetailArrayLabelValues(actor topologyActor) map[string][]string {
	out := map[string][]string{
		"protocols":              actor.Detail.L2.Device.Protocols,
		"protocols_collected":    actor.Detail.L2.Device.ProtocolsCollected,
		"learned_sources":        actor.Detail.L2.Endpoint.LearnedSources,
		"capabilities":           topologyActorDetailCapabilities(actor),
		"capabilities_supported": firstNonEmptyStringSlice(actor.Detail.SNMP.CapabilitiesSupported, actor.Detail.L2.Device.CapabilitiesSupported),
		"capabilities_enabled":   firstNonEmptyStringSlice(actor.Detail.SNMP.CapabilitiesEnabled, actor.Detail.L2.Device.CapabilitiesEnabled),
		"if_names":               firstNonEmptyStringSlice(actor.Detail.L2.Device.IfNames, actor.Detail.L2.Segment.IfNames),
		"if_indexes":             firstNonEmptyStringSlice(actor.Detail.L2.Device.IfIndexes, actor.Detail.L2.Segment.IfIndexes),
	}
	for key, value := range out {
		if len(value) == 0 {
			delete(out, key)
		}
	}
	return out
}

func topologyScalarOptionalInt(value topologyengine.OptionalValue[int]) string {
	if !value.Has {
		return ""
	}
	return fmt.Sprint(value.Value)
}

func topologyIntMapLabel(values map[string]int) string {
	if len(values) == 0 {
		return ""
	}
	parts := make([]string, 0, len(values))
	for _, key := range sortedMapKeys(values) {
		parts = append(parts, fmt.Sprintf("%s:%d", key, values[key]))
	}
	return strings.Join(parts, ",")
}

func firstNonEmptyStringSlice(values ...[]string) []string {
	for _, value := range values {
		if len(value) > 0 {
			return value
		}
	}
	return nil
}

func firstNonZeroInt(values ...int) int {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}

func firstNonZeroInt64(values ...int64) int64 {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}

func firstNonEmptyIntMap(values ...map[string]int) map[string]int {
	for _, value := range values {
		if len(value) > 0 {
			return value
		}
	}
	return nil
}

func firstNonEmptyStringMap(values ...map[string]string) map[string]string {
	for _, value := range values {
		if len(value) > 0 {
			return value
		}
	}
	return nil
}

func firstNonEmptyManagementAddresses(values ...[]topologyManagementAddress) []topologyManagementAddress {
	for _, value := range values {
		if len(value) > 0 {
			return value
		}
	}
	return nil
}

func firstNonEmptyInterfaceChartMap(values ...map[string]topologyInterfaceChartRef) map[string]topologyInterfaceChartRef {
	for _, value := range values {
		if len(value) > 0 {
			return value
		}
	}
	return nil
}
