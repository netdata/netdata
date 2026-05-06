// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"sort"
	"strconv"
	"strings"
)

func selectProbableEndpointReporterHint(
	endpointLabels map[string]string,
	reporterHints map[string][]bridgePortRef,
	owner fdbEndpointOwner,
	aliasOwnerIDs map[string]map[string]struct{},
	managedDeviceIDs map[string]struct{},
) probableEndpointReporterHint {
	ownerDeviceID := strings.TrimSpace(owner.port.deviceID)
	if ownerDeviceID != "" {
		if len(managedDeviceIDs) == 0 {
			return probableEndpointReporterHint{
				deviceID: ownerDeviceID,
				ifIndex:  owner.port.ifIndex,
				ifName:   strings.TrimSpace(owner.port.ifName),
			}
		}
		if _, ok := managedDeviceIDs[ownerDeviceID]; ok {
			return probableEndpointReporterHint{
				deviceID: ownerDeviceID,
				ifIndex:  owner.port.ifIndex,
				ifName:   strings.TrimSpace(owner.port.ifName),
			}
		}
	}

	deviceIDs := resolveTopologyEndpointDeviceHints(
		topologyEndpointLabelDeviceIDs(endpointLabels),
		aliasOwnerIDs,
	)
	if len(deviceIDs) == 0 {
		for reporterID := range reporterHints {
			reporterID = strings.TrimSpace(reporterID)
			if reporterID == "" {
				continue
			}
			deviceIDs = append(deviceIDs, reporterID)
		}
		deviceIDs = resolveTopologyEndpointDeviceHints(deviceIDs, aliasOwnerIDs)
	}
	deviceIDs = filterManagedDeviceHints(deviceIDs, managedDeviceIDs)
	if len(deviceIDs) == 0 {
		return probableEndpointReporterHint{}
	}

	ifIndexes := labelsCSVToSlice(endpointLabels, "learned_if_indexes")
	ifNames := labelsCSVToSlice(endpointLabels, "learned_if_names")
	parsedIfIndex := 0
	for _, value := range ifIndexes {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if n, err := strconv.Atoi(value); err == nil && n > 0 {
			parsedIfIndex = n
			break
		}
	}
	parsedIfName := ""
	for _, value := range ifNames {
		value = strings.TrimSpace(value)
		if value != "" {
			parsedIfName = value
			break
		}
	}

	selectedDeviceID := deviceIDs[0]
	for _, deviceID := range deviceIDs {
		if len(reporterHints[deviceID]) > 0 {
			selectedDeviceID = deviceID
			break
		}
	}
	hint := probableEndpointReporterHint{
		deviceID: selectedDeviceID,
		ifIndex:  parsedIfIndex,
		ifName:   parsedIfName,
	}
	if ports := reporterHints[selectedDeviceID]; len(ports) > 0 {
		sort.SliceStable(ports, func(i, j int) bool {
			return bridgePortRefSortKey(ports[i]) < bridgePortRefSortKey(ports[j])
		})
		port := ports[0]
		if hint.ifIndex == 0 && port.ifIndex > 0 {
			hint.ifIndex = port.ifIndex
		}
		if strings.TrimSpace(hint.ifName) == "" && strings.TrimSpace(port.ifName) != "" {
			hint.ifName = strings.TrimSpace(port.ifName)
		}
	}

	if hint.ifIndex == 0 && strings.TrimSpace(hint.ifName) == "" {
		hint.ifName = "0"
	}
	return hint
}

func ensureManagedProbableReporterHint(
	hint probableEndpointReporterHint,
	endpointLabels map[string]string,
	reporterHints map[string][]bridgePortRef,
	aliasOwnerIDs map[string]map[string]struct{},
	managedDeviceIDs map[string]struct{},
	managedDeviceIDList []string,
) probableEndpointReporterHint {
	if len(managedDeviceIDs) == 0 {
		return hint
	}
	deviceID := strings.TrimSpace(hint.deviceID)
	if deviceID != "" {
		if _, ok := managedDeviceIDs[deviceID]; ok {
			return hint
		}
	}

	deviceIDs := resolveTopologyEndpointDeviceHints(
		topologyEndpointLabelDeviceIDs(endpointLabels),
		aliasOwnerIDs,
	)
	if len(deviceIDs) == 0 {
		for reporterID := range reporterHints {
			reporterID = strings.TrimSpace(reporterID)
			if reporterID == "" {
				continue
			}
			deviceIDs = append(deviceIDs, reporterID)
		}
		deviceIDs = resolveTopologyEndpointDeviceHints(deviceIDs, aliasOwnerIDs)
	}
	deviceIDs = filterManagedDeviceHints(deviceIDs, managedDeviceIDs)
	if len(deviceIDs) == 0 {
		deviceIDs = managedDeviceIDList
	}
	if len(deviceIDs) == 0 {
		return probableEndpointReporterHint{}
	}

	hint.deviceID = strings.TrimSpace(deviceIDs[0])

	ports := reporterHints[hint.deviceID]
	if len(ports) > 0 {
		sort.SliceStable(ports, func(i, j int) bool {
			return bridgePortRefSortKey(ports[i]) < bridgePortRefSortKey(ports[j])
		})
		port := ports[0]
		if hint.ifIndex == 0 && port.ifIndex > 0 {
			hint.ifIndex = port.ifIndex
		}
		if strings.TrimSpace(hint.ifName) == "" && strings.TrimSpace(port.ifName) != "" {
			hint.ifName = strings.TrimSpace(port.ifName)
		}
	}
	if hint.ifIndex == 0 {
		for _, value := range labelsCSVToSlice(endpointLabels, "learned_if_indexes") {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			if n, err := strconv.Atoi(value); err == nil && n > 0 {
				hint.ifIndex = n
				break
			}
		}
	}
	if strings.TrimSpace(hint.ifName) == "" {
		for _, value := range labelsCSVToSlice(endpointLabels, "learned_if_names") {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			hint.ifName = value
			break
		}
	}
	if hint.ifIndex == 0 && strings.TrimSpace(hint.ifName) == "" {
		hint.ifName = "0"
	}
	return hint
}

func ensureProbablePortlessSegment(
	segmentByID map[string]*bridgeDomainSegment,
	hint probableEndpointReporterHint,
) (string, bool) {
	deviceID := strings.TrimSpace(hint.deviceID)
	if deviceID == "" {
		return "", false
	}
	port := bridgePortRef{
		deviceID: deviceID,
		ifIndex:  hint.ifIndex,
		ifName:   strings.TrimSpace(hint.ifName),
	}
	if port.ifIndex == 0 && strings.TrimSpace(port.ifName) == "" {
		port.ifName = "0"
	}

	segmentID := "bridge-domain:probable:" + bridgePortRefSortKey(port)
	if _, ok := segmentByID[segmentID]; ok {
		return segmentID, false
	}
	segment := newBridgeDomainSegment(port)
	segment.methods["probable"] = struct{}{}
	segmentByID[segmentID] = segment
	return segmentID, true
}
