// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"maps"
	"strings"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/l2topology"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"
)

func augmentLocalActor(actor *topologymodel.Actor, local topologymodel.Device) {
	if actor == nil {
		return
	}
	actor.Detail.SNMP = topologySNMPActorDetailFromDevice(local)
	applyLocalActorLabels(actor, local)
	enrichLocalActorChartReferences(actor, local.InterfaceCharts)
}

func topologyLocalActorFromCache(localDeviceID string, local topologymodel.Device) (topologymodel.Actor, bool) {
	actorID := strings.TrimSpace(localDeviceID)
	if actorID == "" {
		actorID = ensureTopologyObservationDeviceID(local, "")
	}
	if actorID == "" {
		return topologymodel.Actor{}, false
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
	return actor, true
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

	if ip := topologyutil.NormalizeIPAddress(local.ManagementIP); ip != "" {
		match.IPAddresses = []string{ip}
	}

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
	maps.Copy(actor.Labels, cloneTopologyLabels(local.Labels))
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
