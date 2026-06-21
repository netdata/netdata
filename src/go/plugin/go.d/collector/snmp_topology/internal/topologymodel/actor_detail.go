// SPDX-License-Identifier: GPL-3.0-or-later

package topologymodel

import (
	"fmt"
	"strings"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/l2topology"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"
)

const LabelOSPFRouterID = "ospf_router_id"

func ActorDetailDisplayName(actor Actor) string {
	return topologyutil.FirstNonEmptyString(
		actor.Detail.L2.DisplayName,
		actor.Match.SysName,
		firstString(actor.Match.Hostnames),
		firstString(actor.Match.DNSNames),
	)
}

func ActorDetailDisplaySource(actor Actor) string {
	return actor.Detail.L2.DisplaySource
}

func ActorDetailParentDevices(actor Actor) []string {
	return actor.Detail.L2.Segment.ParentDevices
}

func ActorDetailVendor(actor Actor) string {
	return topologyutil.FirstNonEmptyString(
		actor.Detail.SNMP.Vendor,
		actor.Detail.L2.Device.Vendor,
		actor.Detail.L2.Device.VendorDerived,
		actor.Detail.L2.Endpoint.Vendor,
		actor.Detail.L2.Endpoint.VendorDerived,
	)
}

func ActorDetailVendorDerived(actor Actor) string {
	return topologyutil.FirstNonEmptyString(actor.Detail.L2.Device.VendorDerived, actor.Detail.L2.Endpoint.VendorDerived)
}

func ActorDetailModel(actor Actor) string {
	return actor.Detail.SNMP.Model
}

func ActorDetailSysDescr(actor Actor) string {
	return actor.Detail.SNMP.SysDescr
}

func ActorDetailSysLocation(actor Actor) string {
	return actor.Detail.SNMP.SysLocation
}

func ActorDetailSysContact(actor Actor) string {
	return actor.Detail.SNMP.SysContact
}

func ActorDetailManagementIP(actor Actor) string {
	return topologyutil.FirstNonEmptyString(actor.Detail.SNMP.ManagementIP, actor.Detail.L2.Device.ManagementIP)
}

func ActorDetailManagementIPs(actor Actor) []string {
	out := make([]string, 0, 1+len(actor.Detail.SNMP.ManagementAddresses)+len(actor.Detail.L2.Device.ManagementAddresses))
	if ip := ActorDetailManagementIPValue(ActorDetailManagementIP(actor)); ip != "" {
		out = append(out, ip)
	}
	for _, address := range actor.Detail.SNMP.ManagementAddresses {
		if ip := ActorDetailManagementIPValue(address.Address); ip != "" {
			out = append(out, ip)
		}
	}
	for _, address := range actor.Detail.L2.Device.ManagementAddresses {
		if ip := ActorDetailManagementIPValue(address); ip != "" {
			out = append(out, ip)
		}
	}
	return topologyutil.DeduplicateSortedStrings(out)
}

func ActorDetailManagementIPValue(value string) string {
	return topologyutil.NormalizeNonUnspecifiedIPAddress(value)
}

func ActorDetailDeviceID(actor Actor) string {
	return actor.Detail.L2.Device.DeviceID
}

func ActorDetailProtocols(actor Actor) []string {
	if len(actor.Detail.L2.Device.Protocols) > 0 {
		return actor.Detail.L2.Device.Protocols
	}
	if len(actor.Detail.L2.Endpoint.LearnedSources) > 0 {
		return actor.Detail.L2.Endpoint.LearnedSources
	}
	return actor.Detail.L2.Segment.LearnedSources
}

func ActorDetailCapabilities(actor Actor) []string {
	if len(actor.Detail.SNMP.Capabilities) > 0 {
		return actor.Detail.SNMP.Capabilities
	}
	return actor.Detail.L2.Device.Capabilities
}

func ActorDetailPortsTotal(actor Actor) topologyengine.OptionalValue[int] {
	if actor.Detail.L2.Device.PortsTotal.Has {
		return actor.Detail.L2.Device.PortsTotal
	}
	if actor.Detail.L2.Segment.PortsTotal.Has {
		return actor.Detail.L2.Segment.PortsTotal
	}
	return topologyengine.OptionalValue[int]{}
}

func ActorDetailVLANCount(actor Actor) topologyengine.OptionalValue[int] {
	return actor.Detail.L2.Device.VLANCount
}

func ActorDetailFDBTotalMACs(actor Actor) topologyengine.OptionalValue[int] {
	return actor.Detail.L2.Device.FDBTotalMACs
}

func ActorDetailLLDPNeighborCount(actor Actor) topologyengine.OptionalValue[int] {
	return actor.Detail.L2.Device.LLDPNeighborCount
}

func ActorDetailCDPNeighborCount(actor Actor) topologyengine.OptionalValue[int] {
	return actor.Detail.L2.Device.CDPNeighborCount
}

func ActorDetailEndpointsTotal(actor Actor) topologyengine.OptionalValue[int] {
	return actor.Detail.L2.Segment.EndpointsTotal
}

func ActorDetailChartIDPrefix(actor Actor) string {
	return actor.Detail.SNMP.ChartIDPrefix
}

func ActorDetailChartContextPrefix(actor Actor) string {
	return actor.Detail.SNMP.ChartContextPrefix
}

func ActorDetailNetdataHostID(actor Actor) string {
	return actor.Detail.SNMP.NetdataHostID
}

func ActorDetailOSPFRouterID(actor Actor) string {
	return actor.Detail.SNMP.OSPFRouterID
}

func ActorDetailScalarLabelValues(actor Actor) map[string]string {
	out := map[string]string{
		"vendor":                  ActorDetailVendor(actor),
		"vendor_derived":          ActorDetailVendorDerived(actor),
		"model":                   ActorDetailModel(actor),
		"sys_descr":               ActorDetailSysDescr(actor),
		"sys_location":            ActorDetailSysLocation(actor),
		"sys_contact":             ActorDetailSysContact(actor),
		"management_ip":           ActorDetailManagementIP(actor),
		LabelOSPFRouterID:         ActorDetailOSPFRouterID(actor),
		"display_name":            ActorDetailDisplayName(actor),
		"display_source":          ActorDetailDisplaySource(actor),
		"chart_id_prefix":         ActorDetailChartIDPrefix(actor),
		"chart_context_prefix":    ActorDetailChartContextPrefix(actor),
		"netdata_host_id":         ActorDetailNetdataHostID(actor),
		"ports_total":             scalarOptionalInt(ActorDetailPortsTotal(actor)),
		"ports_up":                scalarOptionalInt(actor.Detail.L2.Device.PortsUp),
		"ports_down":              scalarOptionalInt(actor.Detail.L2.Device.PortsDown),
		"vlan_count":              scalarOptionalInt(ActorDetailVLANCount(actor)),
		"fdb_total_macs":          scalarOptionalInt(ActorDetailFDBTotalMACs(actor)),
		"lldp_neighbor_count":     scalarOptionalInt(ActorDetailLLDPNeighborCount(actor)),
		"cdp_neighbor_count":      scalarOptionalInt(ActorDetailCDPNeighborCount(actor)),
		"endpoints_total":         scalarOptionalInt(ActorDetailEndpointsTotal(actor)),
		"if_admin_status_counts":  intMapLabel(actor.Detail.L2.Device.AdminStatusCounts),
		"if_oper_status_counts":   intMapLabel(actor.Detail.L2.Device.OperStatusCounts),
		"if_link_mode_counts":     intMapLabel(actor.Detail.L2.Device.LinkModeCounts),
		"if_topology_role_counts": intMapLabel(actor.Detail.L2.Device.TopologyRoleCounts),
	}
	for key, value := range out {
		if strings.TrimSpace(value) == "" {
			delete(out, key)
		}
	}
	return out
}

func ActorDetailArrayLabelValues(actor Actor) map[string][]string {
	out := map[string][]string{
		"protocols":              actor.Detail.L2.Device.Protocols,
		"protocols_collected":    actor.Detail.L2.Device.ProtocolsCollected,
		"learned_sources":        actor.Detail.L2.Endpoint.LearnedSources,
		"capabilities":           ActorDetailCapabilities(actor),
		"capabilities_supported": topologyutil.FirstNonEmptySlice(actor.Detail.SNMP.CapabilitiesSupported, actor.Detail.L2.Device.CapabilitiesSupported),
		"capabilities_enabled":   topologyutil.FirstNonEmptySlice(actor.Detail.SNMP.CapabilitiesEnabled, actor.Detail.L2.Device.CapabilitiesEnabled),
		"if_names":               topologyutil.FirstNonEmptySlice(actor.Detail.L2.Device.IfNames, actor.Detail.L2.Segment.IfNames),
		"if_indexes":             topologyutil.FirstNonEmptySlice(actor.Detail.L2.Device.IfIndexes, actor.Detail.L2.Segment.IfIndexes),
	}
	for key, value := range out {
		if len(value) == 0 {
			delete(out, key)
		}
	}
	return out
}

func firstString(values []string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func scalarOptionalInt(value topologyengine.OptionalValue[int]) string {
	if !value.Has {
		return ""
	}
	return fmt.Sprint(value.Value)
}

func intMapLabel(values map[string]int) string {
	if len(values) == 0 {
		return ""
	}
	parts := make([]string, 0, len(values))
	for _, key := range topologyutil.SortedMapKeys(values) {
		parts = append(parts, fmt.Sprintf("%s:%d", key, values[key]))
	}
	return strings.Join(parts, ",")
}
