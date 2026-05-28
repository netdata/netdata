// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"fmt"
	"slices"
	"strings"
)

func recordTopologyFocusAllDevicesStats(data *topologyData, options topologyQueryOptions) {
	if data == nil {
		return
	}
	if data.Stats == nil {
		data.Stats = make(map[string]any)
	}
	data.Stats["managed_snmp_device_focus"] = options.ManagedDeviceFocus
	data.Stats["depth"] = topologyDepthAll
	data.Stats["actors_focus_depth_filtered"] = 0
	data.Stats["links_focus_depth_filtered"] = 0
	recomputeTopologyLinkStats(data)
}

func recordTopologyFocusStats(data *topologyData, options topologyQueryOptions, beforeActors, beforeLinks int) {
	if data == nil {
		return
	}
	if data.Stats == nil {
		data.Stats = make(map[string]any)
	}
	data.Stats["managed_snmp_device_focus"] = options.ManagedDeviceFocus
	if options.Depth == topologyDepthAllInternal {
		data.Stats["depth"] = topologyDepthAll
	} else {
		data.Stats["depth"] = options.Depth
	}
	data.Stats["actors_focus_depth_filtered"] = beforeActors - len(data.Actors)
	data.Stats["links_focus_depth_filtered"] = beforeLinks - len(data.Links)
	recomputeTopologyLinkStats(data)
}

func topologyManagedFocusSelectedIP(value string) string {
	ips := topologyManagedFocusSelectedIPs(value)
	if len(ips) == 0 {
		return ""
	}
	return ips[0]
}

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
	if ip == normalizeIPAddress(topologyMetricValueString(actor.Attributes, "management_ip")) {
		return true
	}
	if raw, ok := actor.Attributes["management_addresses"]; ok {
		switch values := raw.(type) {
		case []string:
			for _, value := range values {
				if ip == normalizeIPAddress(value) {
					return true
				}
			}
		case []any:
			for _, value := range values {
				if ip == normalizeIPAddress(fmt.Sprint(value)) {
					return true
				}
			}
		}
	}
	return false
}
