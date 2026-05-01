// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"sort"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/topology"
)

func collectTopologyEndpointIDs(
	endpointMatchByID map[string]topology.Match,
	endpointLabelsByID map[string]map[string]string,
	endpointSegmentCandidates map[string][]string,
	rawFDBObservations fdbReporterObservation,
	filteredFDBObservations fdbReporterObservation,
) []string {
	set := make(map[string]struct{})
	for endpointID := range endpointMatchByID {
		endpointID = strings.TrimSpace(endpointID)
		if endpointID != "" {
			set[endpointID] = struct{}{}
		}
	}
	for endpointID := range endpointLabelsByID {
		endpointID = strings.TrimSpace(endpointID)
		if endpointID != "" {
			set[endpointID] = struct{}{}
		}
	}
	for endpointID := range endpointSegmentCandidates {
		endpointID = strings.TrimSpace(endpointID)
		if endpointID != "" {
			set[endpointID] = struct{}{}
		}
	}
	for endpointID := range rawFDBObservations.byEndpoint {
		endpointID = strings.TrimSpace(endpointID)
		if endpointID != "" {
			set[endpointID] = struct{}{}
		}
	}
	for endpointID := range filteredFDBObservations.byEndpoint {
		endpointID = strings.TrimSpace(endpointID)
		if endpointID != "" {
			set[endpointID] = struct{}{}
		}
	}
	return sortedTopologySet(set)
}

func buildFDBEndpointReporterHints(macLinks []bridgeMacLinkRecord) map[string]map[string][]bridgePortRef {
	if len(macLinks) == 0 {
		return nil
	}

	byEndpointReporterPorts := make(map[string]map[string]map[string]bridgePortRef)
	for _, link := range macLinks {
		if strings.ToLower(strings.TrimSpace(link.method)) != "fdb" {
			continue
		}
		endpointID := normalizeFDBEndpointID(link.endpointID)
		reporterID := strings.TrimSpace(link.port.deviceID)
		portKey := bridgePortObservationVLANKey(link.port)
		if endpointID == "" || reporterID == "" || portKey == "" {
			continue
		}
		reporters := byEndpointReporterPorts[endpointID]
		if reporters == nil {
			reporters = make(map[string]map[string]bridgePortRef)
			byEndpointReporterPorts[endpointID] = reporters
		}
		ports := reporters[reporterID]
		if ports == nil {
			ports = make(map[string]bridgePortRef)
			reporters[reporterID] = ports
		}
		ports[portKey] = link.port
	}

	out := make(map[string]map[string][]bridgePortRef, len(byEndpointReporterPorts))
	for endpointID, reporters := range byEndpointReporterPorts {
		reporterHints := make(map[string][]bridgePortRef, len(reporters))
		reporterIDs := make([]string, 0, len(reporters))
		for reporterID := range reporters {
			reporterIDs = append(reporterIDs, reporterID)
		}
		sort.Strings(reporterIDs)
		for _, reporterID := range reporterIDs {
			portsMap := reporters[reporterID]
			if len(portsMap) == 0 {
				continue
			}
			portKeys := make([]string, 0, len(portsMap))
			for key := range portsMap {
				portKeys = append(portKeys, key)
			}
			sort.Strings(portKeys)
			ports := make([]bridgePortRef, 0, len(portKeys))
			for _, key := range portKeys {
				ports = append(ports, portsMap[key])
			}
			reporterHints[reporterID] = ports
		}
		if len(reporterHints) > 0 {
			out[endpointID] = reporterHints
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func buildSegmentReporterIndex(
	segmentIDs []string,
	segmentByID map[string]*bridgeDomainSegment,
) segmentReporterIndex {
	index := segmentReporterIndex{
		byDevice:        make(map[string]map[string]struct{}),
		byDeviceIfIndex: make(map[string]map[string]struct{}),
		byDeviceIfName:  make(map[string]map[string]struct{}),
	}
	for _, segmentID := range segmentIDs {
		segment := segmentByID[segmentID]
		if segment == nil {
			continue
		}
		for _, port := range segment.ports {
			deviceID := strings.TrimSpace(port.deviceID)
			if deviceID == "" {
				continue
			}
			addStringSet(index.byDevice, deviceID, segmentID)
			if port.ifIndex > 0 {
				addStringSet(index.byDeviceIfIndex, deviceID+keySep+strconv.Itoa(port.ifIndex), segmentID)
			}
			if ifName := strings.ToLower(strings.TrimSpace(port.ifName)); ifName != "" {
				addStringSet(index.byDeviceIfName, deviceID+keySep+ifName, segmentID)
			}
		}
	}
	return index
}

func addStringSet(out map[string]map[string]struct{}, key string, value string) {
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	if key == "" || value == "" {
		return
	}
	set := out[key]
	if set == nil {
		set = make(map[string]struct{})
		out[key] = set
	}
	set[value] = struct{}{}
}

func probableCandidateSegmentsFromReporterHints(
	endpointLabels map[string]string,
	fdbReporters map[string]map[string]struct{},
	reporterSegmentIndex segmentReporterIndex,
	aliasOwnerIDs map[string]map[string]struct{},
	managedDeviceIDs map[string]struct{},
) []string {
	deviceIDs := resolveTopologyEndpointDeviceHints(
		topologyEndpointLabelDeviceIDs(endpointLabels),
		aliasOwnerIDs,
	)
	if len(deviceIDs) == 0 {
		for reporterID := range fdbReporters {
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
		return nil
	}

	ifIndexes := labelsCSVToSlice(endpointLabels, "learned_if_indexes")
	ifNames := labelsCSVToSlice(endpointLabels, "learned_if_names")
	hasPortHints := false
	for _, ifIndex := range ifIndexes {
		if strings.TrimSpace(ifIndex) != "" {
			hasPortHints = true
			break
		}
	}
	if !hasPortHints {
		for _, ifName := range ifNames {
			if strings.TrimSpace(ifName) != "" {
				hasPortHints = true
				break
			}
		}
	}
	candidateSet := make(map[string]struct{})

	for _, deviceID := range deviceIDs {
		for _, ifIndex := range ifIndexes {
			ifIndex = strings.TrimSpace(ifIndex)
			if ifIndex == "" {
				continue
			}
			for segmentID := range reporterSegmentIndex.byDeviceIfIndex[deviceID+keySep+ifIndex] {
				candidateSet[segmentID] = struct{}{}
			}
		}
		for _, ifName := range ifNames {
			ifName = strings.ToLower(strings.TrimSpace(ifName))
			if ifName == "" {
				continue
			}
			for segmentID := range reporterSegmentIndex.byDeviceIfName[deviceID+keySep+ifName] {
				candidateSet[segmentID] = struct{}{}
			}
		}
	}

	if len(candidateSet) == 0 && !hasPortHints {
		for _, deviceID := range deviceIDs {
			for segmentID := range reporterSegmentIndex.byDevice[deviceID] {
				candidateSet[segmentID] = struct{}{}
			}
		}
	}
	if len(candidateSet) == 0 {
		return nil
	}
	return sortedTopologySet(candidateSet)
}
