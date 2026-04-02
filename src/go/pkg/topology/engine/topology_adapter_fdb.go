// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"sort"
	"strings"
)

func buildFDBReporterAliases(
	deviceByID map[string]Device,
	ifaceByDeviceIndex map[string]Interface,
) map[string][]string {
	aliases := make(map[string]map[string]struct{}, len(deviceByID))

	for _, device := range deviceByID {
		deviceID := strings.TrimSpace(device.ID)
		if deviceID == "" {
			continue
		}
		chassisMAC := normalizeMAC(device.ChassisID)
		if chassisMAC == "" {
			continue
		}
		if aliases[deviceID] == nil {
			aliases[deviceID] = make(map[string]struct{})
		}
		aliases[deviceID]["mac:"+chassisMAC] = struct{}{}
	}

	for _, iface := range ifaceByDeviceIndex {
		deviceID := strings.TrimSpace(iface.DeviceID)
		if deviceID == "" {
			continue
		}
		ifaceMAC := normalizeMAC(iface.MAC)
		if ifaceMAC == "" {
			continue
		}
		if aliases[deviceID] == nil {
			aliases[deviceID] = make(map[string]struct{})
		}
		aliases[deviceID]["mac:"+ifaceMAC] = struct{}{}
	}

	out := make(map[string][]string, len(aliases))
	for deviceID, set := range aliases {
		values := sortedTopologySet(set)
		if len(values) == 0 {
			continue
		}
		out[deviceID] = values
	}

	return out
}

func buildFDBReporterObservations(macLinks []bridgeMacLinkRecord) fdbReporterObservation {
	obs := fdbReporterObservation{
		byEndpoint: make(map[string]map[string]map[string]struct{}),
		byReporter: make(map[string]map[string]map[string]struct{}),
	}
	for _, link := range macLinks {
		if strings.ToLower(strings.TrimSpace(link.method)) != "fdb" {
			continue
		}
		reporterID := strings.TrimSpace(link.port.deviceID)
		if reporterID == "" {
			continue
		}
		endpointID := normalizeFDBEndpointID(link.endpointID)
		if endpointID == "" {
			continue
		}
		portKey := bridgePortObservationKey(link.port)
		if portKey == "" {
			continue
		}

		byEndpointReporter := obs.byEndpoint[endpointID]
		if byEndpointReporter == nil {
			byEndpointReporter = make(map[string]map[string]struct{})
			obs.byEndpoint[endpointID] = byEndpointReporter
		}
		if byEndpointReporter[reporterID] == nil {
			byEndpointReporter[reporterID] = make(map[string]struct{})
		}
		byEndpointReporter[reporterID][portKey] = struct{}{}

		byReporterEndpoint := obs.byReporter[reporterID]
		if byReporterEndpoint == nil {
			byReporterEndpoint = make(map[string]map[string]struct{})
			obs.byReporter[reporterID] = byReporterEndpoint
		}
		if byReporterEndpoint[endpointID] == nil {
			byReporterEndpoint[endpointID] = make(map[string]struct{})
		}
		byReporterEndpoint[endpointID][portKey] = struct{}{}
	}
	return obs
}

func normalizeFDBEndpointID(endpointID string) string {
	kind, value, ok := strings.Cut(strings.TrimSpace(endpointID), ":")
	if !ok {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "mac":
		if mac := normalizeMAC(value); mac != "" {
			return "mac:" + mac
		}
	}
	return ""
}

func buildFDBAliasOwnerMap(reporterAliases map[string][]string) map[string]map[string]struct{} {
	if len(reporterAliases) == 0 {
		return nil
	}
	aliasOwners := make(map[string]map[string]struct{})
	reporterIDs := make([]string, 0, len(reporterAliases))
	for reporterID := range reporterAliases {
		reporterID = strings.TrimSpace(reporterID)
		if reporterID == "" {
			continue
		}
		reporterIDs = append(reporterIDs, reporterID)
	}
	sort.Strings(reporterIDs)
	for _, reporterID := range reporterIDs {
		aliases := uniqueTopologyStrings(reporterAliases[reporterID])
		for _, alias := range aliases {
			alias = normalizeFDBEndpointID(alias)
			if alias == "" {
				continue
			}
			owners := aliasOwners[alias]
			if owners == nil {
				owners = make(map[string]struct{})
				aliasOwners[alias] = owners
			}
			owners[reporterID] = struct{}{}
		}
	}
	if len(aliasOwners) == 0 {
		return nil
	}
	return aliasOwners
}
