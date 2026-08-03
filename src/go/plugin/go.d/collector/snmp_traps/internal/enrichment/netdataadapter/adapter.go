// SPDX-License-Identifier: GPL-3.0-or-later

package netdataadapter

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	snmptopology "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/enrichment"
)

func RegistryLookup(store *ddsnmp.DeviceStore) enrichment.RegistryLookup {
	if store == nil {
		return nil
	}
	return func(sourceIP string) enrichment.RegistryResult {
		devices := store.DevicesByHostname(sourceIP)
		result := enrichment.RegistryResult{Matches: len(devices)}
		if len(devices) != 1 {
			return result
		}

		device := devices[0]
		if !isUnresolvedSysName(device.VnodeHostname) {
			result.Hostname = device.VnodeHostname
		} else if !isUnresolvedSysName(device.SysName) {
			result.Hostname = device.SysName
		}
		result.Vendor = device.Vendor
		result.VnodeID = device.VnodeGUID
		return result
	}
}

func TopologyLookup(handle *snmptopology.TrapEnrichmentHandle) enrichment.TopologyLookup {
	if handle == nil {
		return nil
	}
	return func(sourceIP, trapIfIndex string) enrichment.TopologyResult {
		return topologyResult(handle.EnrichmentForSource(sourceIP, trapIfIndex))
	}
}

func topologyResult(result *snmptopology.TrapTopologyEnrichment) enrichment.TopologyResult {
	if result == nil {
		return enrichment.TopologyResult{}
	}
	return enrichment.TopologyResult{
		Status:          result.DeviceStatus,
		Method:          result.DeviceMethod,
		Matches:         result.DeviceMatches,
		Hostname:        result.DeviceHostname,
		Vendor:          result.DeviceVendor,
		VnodeID:         result.SourceVnodeID,
		InterfaceIndex:  result.InterfaceIndex,
		InterfaceStatus: result.InterfaceStatus,
		InterfaceName:   result.Interface,
		NeighborStatus:  result.NeighborStatus,
		NeighborNames:   result.Neighbors,
	}
}

func isUnresolvedSysName(name string) bool {
	s := strings.TrimSpace(name)
	return s == "" || strings.EqualFold(s, "unknown")
}
