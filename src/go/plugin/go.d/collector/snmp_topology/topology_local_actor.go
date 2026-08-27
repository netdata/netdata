// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"maps"
	"strings"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/l2topology"
	"github.com/netdata/netdata/go/plugins/pkg/topology/worklimit"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"
)

func augmentLocalActor(actor *topologymodel.Actor, local topologymodel.Device, limiter worklimit.Limiter) error {
	if actor == nil {
		return nil
	}
	actor.Detail.SNMP = topologySNMPActorDetailFromDevice(local)
	if err := applyLocalActorLabelsWithLimiter(actor, local, limiter); err != nil {
		return err
	}
	return enrichLocalActorChartReferences(actor, local.InterfaceCharts, limiter)
}

func topologyLocalActorFromCache(localDeviceID string, local topologymodel.Device) (topologymodel.Actor, bool) {
	actor, ok, _ := topologyLocalActorFromCacheWithLimiter(localDeviceID, local, nil)
	return actor, ok
}

func topologyLocalActorFromCacheWithLimiter(
	localDeviceID string,
	local topologymodel.Device,
	limiter worklimit.Limiter,
) (topologymodel.Actor, bool, error) {
	if err := worklimit.ChargeStrings(limiter, []string{localDeviceID}); err != nil {
		return topologymodel.Actor{}, false, err
	}
	actorID := strings.TrimSpace(localDeviceID)
	if actorID == "" {
		actorID = ensureTopologyObservationDeviceID(local, "")
	}
	if actorID == "" {
		return topologymodel.Actor{}, false, nil
	}
	if err := limiter.Charge(uint64(len(local.Labels))); err != nil {
		return topologymodel.Actor{}, false, err
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
	if err := topologymodel.ChargeMatch(limiter, actor.Match); err != nil {
		return topologymodel.Actor{}, false, err
	}
	return actor, true, nil
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
	_ = applyLocalActorLabelsWithLimiter(actor, local, nil)
}

func applyLocalActorLabelsWithLimiter(
	actor *topologymodel.Actor,
	local topologymodel.Device,
	limiter worklimit.Limiter,
) error {
	if actor == nil {
		return nil
	}
	if err := limiter.Charge(uint64(len(local.Labels))); err != nil {
		return err
	}
	if actor.Labels == nil {
		actor.Labels = make(map[string]string)
	}
	maps.Copy(actor.Labels, cloneTopologyLabels(local.Labels))
	return nil
}

func enrichLocalActorChartReferences(
	actor *topologymodel.Actor,
	interfaceCharts map[string]topologymodel.InterfaceChartRef,
	limiter worklimit.Limiter,
) error {
	if actor == nil || len(interfaceCharts) == 0 {
		return nil
	}

	lookup, err := topologyInterfaceChartLookup(interfaceCharts, limiter)
	if err != nil {
		return err
	}
	if len(lookup) == 0 {
		return nil
	}

	return enrichTopologyPortDetailsWithChartRefs(actor.Detail.L2.Device.Ports, lookup, limiter)
}

func topologyInterfaceChartLookup(
	interfaceCharts map[string]topologymodel.InterfaceChartRef,
	limiter worklimit.Limiter,
) (map[string]topologymodel.InterfaceChartRef, error) {
	keys, err := worklimit.SortedStringKeys(limiter, interfaceCharts)
	if err != nil {
		return nil, err
	}
	lookup := make(map[string]topologymodel.InterfaceChartRef, len(interfaceCharts))
	for _, rawIfName := range keys {
		ifName := rawIfName
		ref := interfaceCharts[rawIfName]
		ifName = strings.ToLower(strings.TrimSpace(ifName))
		if ifName == "" {
			continue
		}
		if strings.TrimSpace(ref.ChartIDSuffix) == "" {
			ref.ChartIDSuffix = ifName
		}
		ref.AvailableMetrics, err = topologyutil.DeduplicateSortedStringsWithLimiter(limiter, ref.AvailableMetrics)
		if err != nil {
			return nil, err
		}
		lookup[ifName] = ref
	}
	return lookup, nil
}

func enrichTopologyPortDetailsWithChartRefs(
	ports []topologyengine.ProjectionPortDetail,
	lookup map[string]topologymodel.InterfaceChartRef,
	limiter worklimit.Limiter,
) error {
	if len(lookup) == 0 || len(ports) == 0 {
		return nil
	}
	if err := limiter.Charge(uint64(len(ports))); err != nil {
		return err
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
	return nil
}
