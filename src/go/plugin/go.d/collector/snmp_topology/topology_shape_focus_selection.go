// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"slices"
	"strings"
)

func topologyManagedFocusSelectedIPs(value string) []string {
	normalized := parseTopologyManagedFocuses(value)
	if len(normalized) == 1 && normalized[0] == topologyManagedFocusAllDevices {
		return nil
	}

	out := make([]string, 0, len(normalized))
	for _, focus := range normalized {
		if len(focus) <= len(topologyManagedFocusIPPrefix) {
			continue
		}
		if !strings.EqualFold(focus[:len(topologyManagedFocusIPPrefix)], topologyManagedFocusIPPrefix) {
			continue
		}
		ip := normalizeIPAddress(strings.TrimSpace(focus[len(topologyManagedFocusIPPrefix):]))
		if ip == "" {
			continue
		}
		out = append(out, ip)
	}
	return out
}

func collectTopologyFocusRoots(graph topologyFocusGraph, focusIPs []string) map[string]struct{} {
	roots := make(map[string]struct{})
	for actorID, actor := range graph.actorByID {
		if _, ok := graph.nonSegmentSet[actorID]; !ok {
			continue
		}
		if !isManagedSNMPDeviceActor(actor) {
			continue
		}
		for _, focusIP := range focusIPs {
			if !topologyActorHasIP(actor, focusIP) {
				continue
			}
			roots[actorID] = struct{}{}
			break
		}
	}
	return roots
}

func topologyActorHasIP(actor topologyActor, ip string) bool {
	ip = normalizeIPAddress(ip)
	if ip == "" {
		return false
	}
	if slices.Contains(normalizedMatchIPs(actor.Match), ip) {
		return true
	}
	return slices.Contains(topologyActorDetailManagementIPs(actor), ip)
}
