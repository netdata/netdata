// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopologyfunc

import (
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
)

func resolveQueryOptions(params funcapi.ResolvedParams) QueryOptions {
	options := QueryOptions{
		CollapseActorsByIP:     true,
		EliminateNonIPInferred: true,
		MapType:                MapTypeLLDPCDPManaged,
		InferenceStrategy:      InferenceStrategyFDBMinimumKnowledge,
		ManagedDeviceFocus:     ManagedFocusAllDevices,
		Depth:                  DepthAllInternal,
	}

	if identity := normalizeNodesIdentity(params.GetOne(ParamNodesIdentity)); identity == NodesIdentityMAC {
		options.CollapseActorsByIP = false
		options.EliminateNonIPInferred = false
	}
	if mapType := normalizeMapType(params.GetOne(ParamMapType)); mapType != "" {
		options.MapType = mapType
	}
	if strategy := normalizeInferenceStrategy(params.GetOne(ParamInferenceStrategy)); strategy != "" {
		options.InferenceStrategy = strategy
	}
	if focuses := normalizeManagedFocuses(params.Get(ParamManagedDeviceFocus)); len(focuses) > 0 {
		options.ManagedDeviceFocus = formatManagedFocuses(focuses)
	}
	options.Depth = normalizeDepth(params.GetOne(ParamDepth))

	return options
}

func normalizeNodesIdentity(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", NodesIdentityIP:
		return NodesIdentityIP
	case NodesIdentityMAC:
		return NodesIdentityMAC
	default:
		return ""
	}
}

func normalizeMapType(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", MapTypeLLDPCDPManaged:
		return MapTypeLLDPCDPManaged
	case MapTypeHighConfidenceInferred:
		return MapTypeHighConfidenceInferred
	case MapTypeAllDevicesLowConfidence:
		return MapTypeAllDevicesLowConfidence
	default:
		return ""
	}
}

func normalizeInferenceStrategy(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", InferenceStrategyFDBMinimumKnowledge:
		return InferenceStrategyFDBMinimumKnowledge
	case InferenceStrategySTPParentTree:
		return InferenceStrategySTPParentTree
	case InferenceStrategyFDBPairwise:
		return InferenceStrategyFDBPairwise
	case InferenceStrategySTPFDBCorrelated:
		return InferenceStrategySTPFDBCorrelated
	case InferenceStrategyCDPFDBHybrid:
		return InferenceStrategyCDPFDBHybrid
	default:
		return ""
	}
}

func normalizeDepth(v string) int {
	value := strings.ToLower(strings.TrimSpace(v))
	if value == "" || value == DepthAll {
		return DepthAllInternal
	}
	depth, err := strconv.Atoi(value)
	if err != nil {
		return DepthAllInternal
	}
	if depth < DepthMin {
		return DepthMin
	}
	if depth > DepthMax {
		return DepthMax
	}
	return depth
}
