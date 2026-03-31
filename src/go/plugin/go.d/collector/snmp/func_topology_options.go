// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"sort"
	"strconv"
	"strings"
)

func normalizeTopologyNodesIdentity(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", topologyNodesIdentityIP:
		return topologyNodesIdentityIP
	case topologyNodesIdentityMAC:
		return topologyNodesIdentityMAC
	default:
		return ""
	}
}

func normalizeTopologyMapType(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", topologyMapTypeLLDPCDPManaged:
		return topologyMapTypeLLDPCDPManaged
	case topologyMapTypeHighConfidenceInferred:
		return topologyMapTypeHighConfidenceInferred
	case topologyMapTypeAllDevicesLowConfidence:
		return topologyMapTypeAllDevicesLowConfidence
	default:
		return ""
	}
}

func normalizeTopologyInferenceStrategy(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", topologyInferenceStrategyFDBMinimumKnowledge:
		return topologyInferenceStrategyFDBMinimumKnowledge
	case topologyInferenceStrategySTPParentTree:
		return topologyInferenceStrategySTPParentTree
	case topologyInferenceStrategyFDBPairwise:
		return topologyInferenceStrategyFDBPairwise
	case topologyInferenceStrategySTPFDBCorrelated:
		return topologyInferenceStrategySTPFDBCorrelated
	case topologyInferenceStrategyCDPFDBHybrid:
		return topologyInferenceStrategyCDPFDBHybrid
	default:
		return ""
	}
}

func normalizeTopologyManagedFocus(v string) string {
	value := strings.TrimSpace(v)
	if value == "" {
		return topologyManagedFocusAllDevices
	}
	return normalizeTopologyManagedFocusValue(value)
}

func normalizeTopologyManagedFocusValue(v string) string {
	value := strings.TrimSpace(v)
	switch strings.ToLower(value) {
	case topologyManagedFocusAllDevices:
		return topologyManagedFocusAllDevices
	}
	if len(value) > len(topologyManagedFocusIPPrefix) &&
		strings.EqualFold(value[:len(topologyManagedFocusIPPrefix)], topologyManagedFocusIPPrefix) {
		ip := normalizeIPAddress(strings.TrimSpace(value[len(topologyManagedFocusIPPrefix):]))
		if ip == "" {
			return ""
		}
		return topologyManagedFocusIPPrefix + ip
	}
	return ""
}

func normalizeTopologyManagedFocuses(values []string) []string {
	expanded := splitTopologyManagedFocusValues(values)
	if len(expanded) == 0 {
		return []string{topologyManagedFocusAllDevices}
	}

	seen := make(map[string]struct{}, len(expanded))
	out := make([]string, 0, len(expanded))
	for _, raw := range expanded {
		normalized := normalizeTopologyManagedFocusValue(raw)
		if normalized == "" {
			continue
		}
		if normalized == topologyManagedFocusAllDevices {
			return []string{topologyManagedFocusAllDevices}
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}

	if len(out) == 0 {
		return []string{topologyManagedFocusAllDevices}
	}
	sort.Strings(out)
	return out
}

func splitTopologyManagedFocusValues(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	out := make([]string, 0, len(values))
	for _, raw := range values {
		for _, token := range strings.Split(raw, ",") {
			token = strings.TrimSpace(token)
			if token == "" {
				continue
			}
			out = append(out, token)
		}
	}
	return out
}

func parseTopologyManagedFocuses(value string) []string {
	if strings.TrimSpace(value) == "" {
		return []string{topologyManagedFocusAllDevices}
	}
	return normalizeTopologyManagedFocuses(strings.Split(value, ","))
}

func formatTopologyManagedFocuses(values []string) string {
	normalized := normalizeTopologyManagedFocuses(values)
	if len(normalized) == 0 {
		return topologyManagedFocusAllDevices
	}
	return strings.Join(normalized, ",")
}

func isTopologyManagedFocusAllDevices(value string) bool {
	normalized := parseTopologyManagedFocuses(value)
	return len(normalized) == 1 && normalized[0] == topologyManagedFocusAllDevices
}

func normalizeTopologyDepth(v string) int {
	value := strings.ToLower(strings.TrimSpace(v))
	if value == "" || value == topologyDepthAll {
		return topologyDepthAllInternal
	}
	depth, err := strconv.Atoi(value)
	if err != nil {
		return topologyDepthAllInternal
	}
	if depth < topologyDepthMin {
		return topologyDepthMin
	}
	if depth > topologyDepthMax {
		return topologyDepthMax
	}
	return depth
}

func isTopologyMapTypeProbable(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", topologyMapTypeAllDevicesLowConfidence:
		return true
	default:
		return false
	}
}
