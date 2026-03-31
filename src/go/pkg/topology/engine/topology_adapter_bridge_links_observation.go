// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"strconv"
	"strings"
)

func addBridgePortObservationKeys(out map[string]struct{}, port bridgePortRef) {
	if out == nil {
		return
	}
	if key := bridgePortObservationKey(port); key != "" {
		out[key] = struct{}{}
	}
	if key := bridgePortObservationVLANKey(port); key != "" {
		out[key] = struct{}{}
	}
}

func buildSwitchFacingPortKeySet(bridgeLinks []bridgeBridgeLinkRecord) map[string]struct{} {
	if len(bridgeLinks) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(bridgeLinks)*4)
	for _, link := range bridgeLinks {
		addBridgePortObservationKeys(out, link.designatedPort)
		addBridgePortObservationKeys(out, link.port)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func augmentSwitchFacingPortKeySetFromManagedAliases(
	switchFacingPortKeys map[string]struct{},
	observations fdbReporterObservation,
	reporterAliases map[string][]string,
) map[string]struct{} {
	if len(observations.byReporter) == 0 || len(reporterAliases) == 0 {
		return switchFacingPortKeys
	}

	aliasOwnerIDs := buildFDBAliasOwnerMap(reporterAliases)
	if len(aliasOwnerIDs) == 0 {
		return switchFacingPortKeys
	}

	updated := switchFacingPortKeys
	for reporterID, endpoints := range observations.byReporter {
		reporterID = strings.TrimSpace(reporterID)
		if reporterID == "" {
			continue
		}
		for endpointID, ports := range endpoints {
			owners := aliasOwnerIDs[normalizeFDBEndpointID(endpointID)]
			if len(owners) == 0 {
				continue
			}
			otherManagedOwner := false
			for ownerID := range owners {
				if !strings.EqualFold(ownerID, reporterID) {
					otherManagedOwner = true
					break
				}
			}
			if !otherManagedOwner {
				continue
			}
			for portKey := range ports {
				portKey = strings.TrimSpace(portKey)
				if portKey == "" {
					continue
				}
				if updated == nil {
					updated = make(map[string]struct{})
				}
				updated[portKey] = struct{}{}
			}
		}
	}
	return updated
}

func augmentSwitchFacingPortKeySetFromSTPManagedAliasCorrelation(
	switchFacingPortKeys map[string]struct{},
	adjacencies []Adjacency,
	ifIndexByDeviceName map[string]int,
	observations fdbReporterObservation,
	reporterAliases map[string][]string,
) map[string]struct{} {
	if len(adjacencies) == 0 || len(observations.byReporter) == 0 || len(reporterAliases) == 0 {
		return switchFacingPortKeys
	}

	updated := switchFacingPortKeys
	for _, adj := range adjacencies {
		if !strings.EqualFold(strings.TrimSpace(adj.Protocol), "stp") {
			continue
		}

		srcReporterID := strings.TrimSpace(adj.SourceID)
		dstReporterID := strings.TrimSpace(adj.TargetID)
		if srcReporterID == "" || dstReporterID == "" || strings.EqualFold(srcReporterID, dstReporterID) {
			continue
		}

		srcPort := bridgePortFromAdjacencySide(srcReporterID, adj.SourcePort, ifIndexByDeviceName)
		dstPort := bridgePortFromAdjacencySide(dstReporterID, adj.TargetPort, ifIndexByDeviceName)

		if stpPortSeesManagedAlias(srcReporterID, srcPort, dstReporterID, observations, reporterAliases) {
			if updated == nil {
				updated = make(map[string]struct{})
			}
			addBridgePortObservationKeys(updated, srcPort)
		}
		if stpPortSeesManagedAlias(dstReporterID, dstPort, srcReporterID, observations, reporterAliases) {
			if updated == nil {
				updated = make(map[string]struct{})
			}
			addBridgePortObservationKeys(updated, dstPort)
		}
	}
	return updated
}

func stpPortSeesManagedAlias(
	reporterID string,
	port bridgePortRef,
	peerDeviceID string,
	observations fdbReporterObservation,
	reporterAliases map[string][]string,
) bool {
	reporterID = strings.TrimSpace(reporterID)
	peerDeviceID = strings.TrimSpace(peerDeviceID)
	if reporterID == "" || peerDeviceID == "" {
		return false
	}
	portKey := bridgePortObservationKey(port)
	if portKey == "" {
		return false
	}
	endpointsByReporter := observations.byReporter[reporterID]
	if len(endpointsByReporter) == 0 {
		return false
	}
	aliases := reporterAliases[peerDeviceID]
	if len(aliases) == 0 {
		return false
	}
	for _, alias := range aliases {
		alias = normalizeFDBEndpointID(alias)
		if alias == "" {
			continue
		}
		ports := endpointsByReporter[alias]
		if len(ports) == 0 {
			continue
		}
		if _, ok := ports[portKey]; ok {
			return true
		}
	}
	return false
}

func bridgePortObservationKey(port bridgePortRef) string {
	base := bridgePortObservationBaseKey(port)
	if base == "" {
		return ""
	}
	return base + "|vlan:"
}

func bridgePortObservationVLANKey(port bridgePortRef) string {
	base := bridgePortObservationBaseKey(port)
	if base == "" {
		return ""
	}
	return base + "|vlan:" + strings.ToLower(strings.TrimSpace(port.vlanID))
}

func bridgePortObservationBaseKey(port bridgePortRef) string {
	deviceID := strings.TrimSpace(port.deviceID)
	if deviceID == "" {
		return ""
	}
	if port.ifIndex > 0 {
		return deviceID + "|if:" + strconv.Itoa(port.ifIndex)
	}
	name := firstNonEmpty(port.ifName, port.bridgePort)
	name = normalizeInterfaceNameForLookup(name)
	if name == "" {
		return ""
	}
	return deviceID + "|name:" + name
}
