// SPDX-License-Identifier: GPL-3.0-or-later

package projector

import (
	"strings"
)

func inferFDBEndpointOwners(
	observations fdbReporterObservation,
	reporterAliases map[string][]string,
	switchFacingPortKeys map[string]struct{},
) map[string]fdbEndpointOwner {
	return inferFDBEndpointOwnersWithWork(nil, observations, reporterAliases, switchFacingPortKeys)
}

func inferFDBEndpointOwnersWithWork(
	work *projectionWork,
	observations fdbReporterObservation,
	reporterAliases map[string][]string,
	switchFacingPortKeys map[string]struct{},
) map[string]fdbEndpointOwner {
	if len(observations.byEndpoint) == 0 {
		return nil
	}

	owners := make(map[string]fdbEndpointOwner, len(observations.byEndpoint))
	var endpointIDs []string
	if work == nil {
		endpointIDs = make([]string, 0, len(observations.byEndpoint))
	}
	endpointIDs = sortedProjectionKeys(work, observations.byEndpoint, endpointIDs)

	for _, endpointID := range endpointIDs {
		reportersMap := observations.byEndpoint[endpointID]
		if len(reportersMap) < 2 {
			continue
		}

		var reporterIDs []string
		if work == nil {
			reporterIDs = make([]string, 0, len(reportersMap))
		}
		reporterIDs = sortedProjectionKeys(work, reportersMap, reporterIDs)

		validPortsByReporter := make(map[string][]string)
		for _, reporterID := range reporterIDs {
			ports := sortedTopologySetWithWork(work, reportersMap[reporterID])
			if len(ports) == 0 {
				continue
			}

			for _, endpointPort := range ports {
				if _, isSwitchFacingPort := switchFacingPortKeys[endpointPort]; isSwitchFacingPort {
					continue
				}
				if !reporterSatisfiesFDBOwnerRule(endpointPort, reporterID, reporterIDs, observations.byReporter, reporterAliases) {
					continue
				}
				validPortsByReporter[reporterID] = append(validPortsByReporter[reporterID], endpointPort)
			}
		}

		if len(validPortsByReporter) != 1 {
			continue
		}
		for _, ports := range validPortsByReporter {
			ports = uniqueTopologyStringsWithWork(work, ports)
			if len(ports) == 0 {
				continue
			}
			owners[endpointID] = fdbEndpointOwner{
				portKey: ports[0],
				source:  "reporter_matrix",
			}
		}
	}

	if len(owners) == 0 {
		return nil
	}
	return owners
}

func reporterSatisfiesFDBOwnerRule(
	endpointPort string,
	reporterID string,
	reporterIDs []string,
	reporterObservations map[string]map[string]map[string]struct{},
	reporterAliases map[string][]string,
) bool {
	reporterEndpoints := reporterObservations[reporterID]
	if len(reporterEndpoints) == 0 {
		return false
	}

	for _, otherReporterID := range reporterIDs {
		if otherReporterID == reporterID {
			continue
		}
		aliases := reporterAliases[otherReporterID]
		if len(aliases) == 0 {
			return false
		}

		seenOtherOnDifferentPort := false
		for _, alias := range aliases {
			ports := reporterEndpoints[alias]
			if len(ports) == 0 {
				continue
			}
			for observedPort := range ports {
				if observedPort == endpointPort {
					return false
				}
				seenOtherOnDifferentPort = true
			}
		}
		if !seenOtherOnDifferentPort {
			return false
		}
	}

	return true
}

func inferSinglePortEndpointOwners(
	macLinks []bridgeMacLinkRecord,
	switchFacingPortKeys map[string]struct{},
) map[string]fdbEndpointOwner {
	return inferSinglePortEndpointOwnersWithWork(nil, macLinks, switchFacingPortKeys)
}

func inferSinglePortEndpointOwnersWithWork(
	work *projectionWork,
	macLinks []bridgeMacLinkRecord,
	switchFacingPortKeys map[string]struct{},
) map[string]fdbEndpointOwner {
	if len(macLinks) == 0 {
		return nil
	}
	if !work.charge(uint64(len(macLinks))) {
		return nil
	}

	type portScope struct {
		portKey     string
		portVLANKey string
		port        bridgePortRef
		endpointIDs map[string]struct{}
	}

	byPortScope := make(map[string]*portScope)
	for _, link := range macLinks {
		if strings.ToLower(strings.TrimSpace(link.method)) != "fdb" {
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
		if _, isSwitchFacingPort := switchFacingPortKeys[portKey]; isSwitchFacingPort {
			continue
		}
		portVLANKey := bridgePortObservationVLANKey(link.port)
		if portVLANKey == "" {
			portVLANKey = portKey
		}
		if _, isSwitchFacingPort := switchFacingPortKeys[portVLANKey]; isSwitchFacingPort {
			continue
		}

		scope := byPortScope[portVLANKey]
		if scope == nil {
			scope = &portScope{
				portKey:     portKey,
				portVLANKey: portVLANKey,
				port:        link.port,
				endpointIDs: make(map[string]struct{}),
			}
			byPortScope[portVLANKey] = scope
		}
		scope.endpointIDs[endpointID] = struct{}{}
	}

	if len(byPortScope) == 0 {
		return nil
	}

	candidatesByEndpoint := make(map[string]map[string]fdbEndpointOwner)
	var scopeKeys []string
	if work == nil {
		scopeKeys = make([]string, 0, len(byPortScope))
	}
	scopeKeys = sortedProjectionKeys(work, byPortScope, scopeKeys)

	for _, scopeKey := range scopeKeys {
		scope := byPortScope[scopeKey]
		if scope == nil || len(scope.endpointIDs) != 1 {
			continue
		}
		endpointID := ""
		for id := range scope.endpointIDs {
			endpointID = id
			break
		}
		if endpointID == "" {
			continue
		}
		candidates := candidatesByEndpoint[endpointID]
		if candidates == nil {
			candidates = make(map[string]fdbEndpointOwner)
			candidatesByEndpoint[endpointID] = candidates
		}
		candidates[scope.portVLANKey] = fdbEndpointOwner{
			portKey:     scope.portKey,
			portVLANKey: scope.portVLANKey,
			port:        scope.port,
			source:      "single_port_mac",
		}
	}

	if len(candidatesByEndpoint) == 0 {
		return nil
	}

	owners := make(map[string]fdbEndpointOwner)
	var endpointIDs []string
	if work == nil {
		endpointIDs = make([]string, 0, len(candidatesByEndpoint))
	}
	endpointIDs = sortedProjectionKeys(work, candidatesByEndpoint, endpointIDs)

	for _, endpointID := range endpointIDs {
		candidates := candidatesByEndpoint[endpointID]
		if len(candidates) != 1 {
			continue
		}
		for _, owner := range candidates {
			owners[endpointID] = owner
		}
	}

	if len(owners) == 0 {
		return nil
	}
	return owners
}
