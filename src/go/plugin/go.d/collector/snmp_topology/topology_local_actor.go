// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"strings"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/l2topology"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"
)

func augmentLocalActorFromCache(data *topologymodel.Data, local topologymodel.Device) bool {
	if data == nil || len(data.Actors) == 0 {
		return false
	}

	for i := range data.Actors {
		actor := &data.Actors[i]
		if !topologyengine.IsDeviceActorType(actor.ActorType) {
			continue
		}
		if !topologymodel.MatchLocalActor(actor.Match, local) {
			continue
		}

		actor.Detail.SNMP = topologySNMPActorDetailFromDevice(local)
		applyLocalActorLabels(actor, local)
		enrichLocalActorChartReferences(actor, local.InterfaceCharts)
		return true
	}

	return false
}

func addLocalActorFromCache(data *topologymodel.Data, localDeviceID string, local topologymodel.Device) bool {
	if data == nil {
		return false
	}

	actorID := strings.TrimSpace(localDeviceID)
	if actorID == "" {
		actorID = ensureTopologyObservationDeviceID(local, "")
	}
	if actorID == "" || topologyLocalActorExists(data.Actors, actorID) {
		return false
	}

	labels := cloneTopologyLabels(local.Labels)
	actor := topologymodel.Actor{
		ActorID:   actorID,
		ActorType: topologyengine.ResolveDeviceActorType(labels),
		Layer:     "network",
		Source:    "snmp",
		Match:     topologyLocalActorMatch(local),
		Labels:    labels,
		Detail: topologymodel.ActorDetail{
			SNMP: topologySNMPActorDetailFromDevice(local),
		},
	}
	if actor.ActorType == "" {
		actor.ActorType = "device"
	}
	data.Actors = append(data.Actors, actor)
	return true
}

func topologyLocalActorExists(actors []topologymodel.Actor, actorID string) bool {
	for _, actor := range actors {
		if strings.TrimSpace(actor.ActorID) == actorID {
			return true
		}
	}
	return false
}

func topologyLocalActorMatch(local topologymodel.Device) topologymodel.Match {
	match := topologymodel.Match{
		SysObjectID: strings.TrimSpace(local.SysObjectID),
		SysName:     strings.TrimSpace(local.SysName),
	}
	if chassisID := strings.TrimSpace(local.ChassisID); chassisID != "" {
		match.ChassisIDs = []string{chassisID}
		if mac := topologyutil.NormalizeMAC(chassisID); mac != "" {
			match.MacAddresses = []string{mac}
		}
	}

	ips := make([]string, 0, 1+len(local.ManagementAddresses))
	if ip := topologyutil.NormalizeIPAddress(local.ManagementIP); ip != "" {
		ips = append(ips, ip)
	}
	for _, addr := range local.ManagementAddresses {
		if ip := topologyutil.NormalizeIPAddress(addr.Address); ip != "" {
			ips = append(ips, ip)
		}
	}
	match.IPAddresses = topologyutil.DeduplicateSortedStrings(ips)

	return match
}

func topologySNMPActorDetailFromDevice(local topologymodel.Device) topologymodel.SNMPActorDetail {
	return topologymodel.SNMPActorDetail{
		ManagementAddresses:   local.ManagementAddresses,
		Capabilities:          local.Capabilities,
		CapabilitiesSupported: local.CapabilitiesSupported,
		CapabilitiesEnabled:   local.CapabilitiesEnabled,
		SysDescr:              strings.TrimSpace(local.SysDescr),
		SysContact:            strings.TrimSpace(local.SysContact),
		SysLocation:           strings.TrimSpace(local.SysLocation),
		SysUptime:             local.SysUptime,
		Vendor:                strings.TrimSpace(local.Vendor),
		VendorSource:          "snmp",
		VendorConfidence:      "high",
		Model:                 strings.TrimSpace(local.Model),
		OSPFRouterID:          topologyutil.NormalizeTopologyRouterID(local.OSPFRouterID),
		SerialNumber:          strings.TrimSpace(local.SerialNumber),
		SoftwareVersion:       strings.TrimSpace(local.SoftwareVersion),
		FirmwareVersion:       strings.TrimSpace(local.FirmwareVersion),
		HardwareVersion:       strings.TrimSpace(local.HardwareVersion),
		ManagementIP:          topologyutil.NormalizeIPAddress(local.ManagementIP),
		NetdataHostID:         strings.TrimSpace(local.NetdataHostID),
		ChartIDPrefix:         strings.TrimSpace(local.ChartIDPrefix),
		ChartContextPrefix:    strings.TrimSpace(local.ChartContextPrefix),
		DeviceCharts:          local.DeviceCharts,
		InterfaceCharts:       local.InterfaceCharts,
	}
}

func applyLocalActorLabels(actor *topologymodel.Actor, local topologymodel.Device) {
	if actor == nil {
		return
	}
	if actor.Labels == nil {
		actor.Labels = make(map[string]string)
	}
	for key, value := range cloneTopologyLabels(local.Labels) {
		actor.Labels[key] = value
	}
}

func enrichLocalActorChartReferences(actor *topologymodel.Actor, interfaceCharts map[string]topologymodel.InterfaceChartRef) {
	if actor == nil || len(interfaceCharts) == 0 {
		return
	}

	lookup := topologyInterfaceChartLookup(interfaceCharts)
	if len(lookup) == 0 {
		return
	}

	enrichTopologyPortDetailsWithChartRefs(actor.Detail.L2.Device.Ports, lookup)
}

func topologyInterfaceChartLookup(interfaceCharts map[string]topologymodel.InterfaceChartRef) map[string]topologymodel.InterfaceChartRef {
	lookup := make(map[string]topologymodel.InterfaceChartRef, len(interfaceCharts))
	for ifName, ref := range interfaceCharts {
		ifName = strings.ToLower(strings.TrimSpace(ifName))
		if ifName == "" {
			continue
		}
		if strings.TrimSpace(ref.ChartIDSuffix) == "" {
			ref.ChartIDSuffix = ifName
		}
		ref.AvailableMetrics = topologyutil.DeduplicateSortedStrings(ref.AvailableMetrics)
		lookup[ifName] = ref
	}
	return lookup
}

func enrichTopologyPortDetailsWithChartRefs(ports []topologyengine.ProjectionPortDetail, lookup map[string]topologymodel.InterfaceChartRef) {
	if len(lookup) == 0 || len(ports) == 0 {
		return
	}

	for i := range ports {
		name := strings.ToLower(strings.TrimSpace(topologyutil.FirstNonEmptyString(
			ports[i].IfName,
			ports[i].Name,
			ports[i].PortID,
		)))
		if name == "" {
			continue
		}
		ref, ok := lookup[name]
		if !ok {
			continue
		}
		ports[i].ChartIDSuffix = ref.ChartIDSuffix
		if len(ref.AvailableMetrics) > 0 {
			ports[i].AvailableMetrics = ref.AvailableMetrics
		}
	}
}
