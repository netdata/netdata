// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"sort"
	"strings"
)

func inferFDBEndpointOwners(
	observations fdbReporterObservation,
	reporterAliases map[string][]string,
	switchFacingPortKeys map[string]struct{},
) map[string]fdbEndpointOwner {
	if len(observations.byEndpoint) == 0 {
		return nil
	}

	owners := make(map[string]fdbEndpointOwner, len(observations.byEndpoint))
	endpointIDs := make([]string, 0, len(observations.byEndpoint))
	for endpointID := range observations.byEndpoint {
		endpointIDs = append(endpointIDs, endpointID)
	}
	sort.Strings(endpointIDs)

	for _, endpointID := range endpointIDs {
		reportersMap := observations.byEndpoint[endpointID]
		if len(reportersMap) < 2 {
			continue
		}

		reporterIDs := make([]string, 0, len(reportersMap))
		for reporterID := range reportersMap {
			reporterIDs = append(reporterIDs, reporterID)
		}
		sort.Strings(reporterIDs)

		validPortsByReporter := make(map[string][]string)
		for _, reporterID := range reporterIDs {
			ports := sortedTopologySet(reportersMap[reporterID])
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
			ports = uniqueTopologyStrings(ports)
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
	if len(macLinks) == 0 {
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
	scopeKeys := make([]string, 0, len(byPortScope))
	for key := range byPortScope {
		scopeKeys = append(scopeKeys, key)
	}
	sort.Strings(scopeKeys)

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
	endpointIDs := make([]string, 0, len(candidatesByEndpoint))
	for endpointID := range candidatesByEndpoint {
		endpointIDs = append(endpointIDs, endpointID)
	}
	sort.Strings(endpointIDs)

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
