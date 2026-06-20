// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"fmt"
	"strings"
)

func topologyActorDetailDisplayName(actor topologyActor) string {
	return firstNonEmptyString(
		actor.Detail.L2.DisplayName,
		actor.Labels["display_name"],
		actor.Labels["name"],
		actor.Match.SysName,
		firstString(actor.Match.Hostnames),
		firstString(actor.Match.DNSNames),
	)
}

func topologyActorDetailDisplaySource(actor topologyActor) string {
	return firstNonEmptyString(actor.Detail.L2.DisplaySource, actor.Labels["display_source"])
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
	if ip := topologyActorDetailManagementIP(actor); ip != "" {
		out = append(out, ip)
	}
	for _, address := range actor.Detail.SNMP.ManagementAddresses {
		if ip := normalizeIPAddress(address.Address); ip != "" {
			out = append(out, ip)
		}
	}
	for _, address := range actor.Detail.L2.Device.ManagementAddresses {
		if ip := normalizeIPAddress(address); ip != "" {
			out = append(out, ip)
		}
	}
	return deduplicateSortedStrings(out)
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

func topologyActorDetailPortsTotal(actor topologyActor) (int, bool) {
	if actor.Detail.L2.Device.HasPortsTotal {
		return actor.Detail.L2.Device.PortsTotal, true
	}
	if actor.Detail.L2.Segment.HasPortsTotal {
		return actor.Detail.L2.Segment.PortsTotal, true
	}
	return 0, false
}

func topologyActorDetailVLANCount(actor topologyActor) (int, bool) {
	if actor.Detail.L2.Device.HasVLANCount {
		return actor.Detail.L2.Device.VLANCount, true
	}
	return 0, false
}

func topologyActorDetailFDBTotalMACs(actor topologyActor) (int, bool) {
	if actor.Detail.L2.Device.HasFDBTotalMACs {
		return actor.Detail.L2.Device.FDBTotalMACs, true
	}
	return 0, false
}

func topologyActorDetailLLDPNeighborCount(actor topologyActor) (int, bool) {
	if actor.Detail.L2.Device.HasLLDPNeighborCount {
		return actor.Detail.L2.Device.LLDPNeighborCount, true
	}
	return 0, false
}

func topologyActorDetailCDPNeighborCount(actor topologyActor) (int, bool) {
	if actor.Detail.L2.Device.HasCDPNeighborCount {
		return actor.Detail.L2.Device.CDPNeighborCount, true
	}
	return 0, false
}

func topologyActorDetailEndpointsTotal(actor topologyActor) (int, bool) {
	if actor.Detail.L2.Segment.HasEndpointsTotal {
		return actor.Detail.L2.Segment.EndpointsTotal, true
	}
	return 0, false
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
		"ports_up":                topologyScalarOptionalInt(actor.Detail.L2.Device.PortsUp, actor.Detail.L2.Device.HasPortsUp),
		"ports_down":              topologyScalarOptionalInt(actor.Detail.L2.Device.PortsDown, actor.Detail.L2.Device.HasPortsDown),
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

func topologyScalarOptionalInt(value int, ok bool) string {
	if !ok {
		return ""
	}
	return fmt.Sprint(value)
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

func firstPresentInt(dstValue int, dstOK bool, srcValue int, srcOK bool) (int, bool) {
	if dstOK {
		return dstValue, true
	}
	if srcOK {
		return srcValue, true
	}
	return dstValue, false
}

func firstPresentInt64(dstValue int64, dstOK bool, srcValue int64, srcOK bool) (int64, bool) {
	if dstOK {
		return dstValue, true
	}
	if srcOK {
		return srcValue, true
	}
	return dstValue, false
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
