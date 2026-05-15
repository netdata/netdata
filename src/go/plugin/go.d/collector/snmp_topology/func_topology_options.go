// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import "strings"

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
