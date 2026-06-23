// SPDX-License-Identifier: GPL-3.0-or-later

package projector

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/l2topology/internal/model"
	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
)

func topologyDeviceInferred(dev model.Device) bool {
	if len(dev.Labels) == 0 {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(dev.Labels["inferred"])) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func buildDeviceActorMatch(dev model.Device, reporterAliases []string) graph.Match {
	match := graph.Match{
		SysObjectID: strings.TrimSpace(dev.SysObject),
		SysName:     strings.TrimSpace(dev.Hostname),
	}

	macSet := make(map[string]struct{}, 1+len(reporterAliases))
	chassis := strings.TrimSpace(dev.ChassisID)
	if chassis != "" {
		match.ChassisIDs = []string{chassis}
		if mac := normalizeMAC(chassis); mac != "" {
			macSet[mac] = struct{}{}
		}
	}
	for _, alias := range reporterAliases {
		alias = strings.TrimSpace(alias)
		if alias == "" {
			continue
		}
		if after, ok := strings.CutPrefix(alias, "mac:"); ok {
			if mac := normalizeMAC(after); mac != "" {
				macSet[mac] = struct{}{}
			}
			continue
		}
		if mac := normalizeMAC(alias); mac != "" {
			macSet[mac] = struct{}{}
		}
	}
	if len(macSet) > 0 {
		match.MacAddresses = sortedTopologySet(macSet)
	}

	if len(dev.Addresses) > 0 {
		ips := make([]string, 0, len(dev.Addresses))
		for _, addr := range dev.Addresses {
			if !addr.IsValid() {
				continue
			}
			ips = append(ips, addr.String())
		}
		match.IPAddresses = uniqueTopologyStrings(ips)
	}

	return match
}

func buildDeviceActorDetail(
	dev model.Device,
	localDeviceID string,
	ifaceSummary topologyDeviceInterfaceSummary,
	match graph.Match,
) model.ProjectionDeviceActorDetail {
	discovered := strings.TrimSpace(localDeviceID) == "" || dev.ID != localDeviceID

	detail := model.ProjectionDeviceActorDetail{
		DeviceID:              strings.TrimSpace(dev.ID),
		Discovered:            discovered,
		Inferred:              topologyDeviceInferred(dev),
		ManagementIP:          firstAddress(dev.Addresses),
		ManagementAddresses:   addressStrings(dev.Addresses),
		Protocols:             labelsCSVToSlice(dev.Labels, "protocols_observed"),
		ProtocolsCollected:    labelsCSVToSlice(dev.Labels, "protocols_observed"),
		Capabilities:          labelsCSVToSlice(dev.Labels, "capabilities"),
		CapabilitiesSupported: labelsCSVToSlice(dev.Labels, "capabilities_supported"),
		CapabilitiesEnabled:   labelsCSVToSlice(dev.Labels, "capabilities_enabled"),
		IfIndexes:             ifaceSummary.ifIndexes,
		IfNames:               ifaceSummary.ifNames,
		AdminStatusCounts:     cloneIntMap(ifaceSummary.adminStatusCount),
		OperStatusCounts:      cloneIntMap(ifaceSummary.operStatusCount),
		LinkModeCounts:        cloneIntMap(ifaceSummary.linkModeCount),
		TopologyRoleCounts:    cloneIntMap(ifaceSummary.roleCount),
		Ports:                 cloneProjectionPortDetails(ifaceSummary.portStatuses),
	}
	derivedVendor, derivedPrefix := inferTopologyVendorFromMatch(match)
	if derivedVendor != "" {
		detail.VendorDerived = derivedVendor
		detail.VendorDerivedSource = "mac_oui"
		detail.VendorDerivedConfidence = "low"
		detail.VendorDerivedMatchPrefix = derivedPrefix
	}
	if vendor := strings.TrimSpace(dev.Labels["vendor"]); vendor != "" {
		detail.Vendor = vendor
		detail.VendorSource = "labels"
		detail.VendorConfidence = "high"
	} else if derivedVendor != "" {
		detail.Vendor = derivedVendor
		detail.VendorSource = "mac_oui"
		detail.VendorConfidence = "low"
		detail.VendorMatchPrefix = derivedPrefix
	}
	if ifaceSummary.portsTotal > 0 {
		detail.PortsTotal = model.OptionalValue[int]{Value: ifaceSummary.portsTotal, Has: true}
	}
	if ifaceSummary.portsUp > 0 {
		detail.PortsUp = model.OptionalValue[int]{Value: ifaceSummary.portsUp, Has: true}
	}
	if ifaceSummary.portsDown > 0 {
		detail.PortsDown = model.OptionalValue[int]{Value: ifaceSummary.portsDown, Has: true}
	}
	if ifaceSummary.portsAdminDown > 0 {
		detail.PortsAdminDown = model.OptionalValue[int]{Value: ifaceSummary.portsAdminDown, Has: true}
	}
	if ifaceSummary.totalBandwidthBps > 0 {
		detail.TotalBandwidthBps = model.OptionalValue[int64]{Value: ifaceSummary.totalBandwidthBps, Has: true}
	}
	if ifaceSummary.fdbTotalMACs > 0 {
		detail.FDBTotalMACs = model.OptionalValue[int]{Value: ifaceSummary.fdbTotalMACs, Has: true}
	}
	if ifaceSummary.vlanCount > 0 {
		detail.VLANCount = model.OptionalValue[int]{Value: ifaceSummary.vlanCount, Has: true}
	}
	if ifaceSummary.lldpNeighborCount > 0 {
		detail.LLDPNeighborCount = model.OptionalValue[int]{Value: ifaceSummary.lldpNeighborCount, Has: true}
	}
	if ifaceSummary.cdpNeighborCount > 0 {
		detail.CDPNeighborCount = model.OptionalValue[int]{Value: ifaceSummary.cdpNeighborCount, Has: true}
	}
	return detail
}

func cloneIntMap(in map[string]int) map[string]int {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]int, len(in))
	for key, value := range in {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func cloneProjectionPortDetails(in []model.ProjectionPortDetail) []model.ProjectionPortDetail {
	if len(in) == 0 {
		return nil
	}
	out := make([]model.ProjectionPortDetail, len(in))
	copy(out, in)
	return out
}
