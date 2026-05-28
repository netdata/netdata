// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"strconv"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
)

func topologyNodesIdentityParamConfig() funcapi.ParamConfig {
	return funcapi.ParamConfig{
		ID:        topologyParamNodesIdentity,
		Name:      "Nodes Identity",
		Help:      "Choose actor identity strategy: ip (collapse by IP, remove non-IP inferred) or mac",
		Selection: funcapi.ParamSelect,
		Options: []funcapi.ParamOption{
			{ID: topologyNodesIdentityIP, Name: "IP", Default: true},
			{ID: topologyNodesIdentityMAC, Name: "MAC"},
		},
	}
}

func topologyMapTypeParamConfig() funcapi.ParamConfig {
	return funcapi.ParamConfig{
		ID:        topologyParamMapType,
		Name:      "Map",
		Help:      "Choose topology map mode",
		Selection: funcapi.ParamSelect,
		Options: []funcapi.ParamOption{
			{
				ID:      topologyMapTypeLLDPCDPManaged,
				Name:    "LLDP/CDP/Managed Devices Map",
				Default: true,
			},
			{ID: topologyMapTypeHighConfidenceInferred, Name: "High Confidence Inferred Map"},
			{
				ID:   topologyMapTypeAllDevicesLowConfidence,
				Name: "All Devices (Low Confidence)",
			},
		},
	}
}

func topologyInferenceStrategyParamConfig() funcapi.ParamConfig {
	return funcapi.ParamConfig{
		ID:        topologyParamInferenceStrategy,
		Name:      "Infer Strategy",
		Help:      "Choose the topology inference strategy",
		Selection: funcapi.ParamSelect,
		Options: []funcapi.ParamOption{
			{
				ID:      topologyInferenceStrategyFDBMinimumKnowledge,
				Name:    "FDB Minimum-Knowledge (Baseline)",
				Default: true,
			},
			{
				ID:   topologyInferenceStrategySTPParentTree,
				Name: "STP Parent Tree",
			},
			{
				ID:   topologyInferenceStrategyFDBPairwise,
				Name: "FDB Pairwise Minimum-Knowledge",
			},
			{
				ID:   topologyInferenceStrategySTPFDBCorrelated,
				Name: "STP + FDB Correlated",
			},
			{
				ID:   topologyInferenceStrategyCDPFDBHybrid,
				Name: "CDP + FDB Hybrid",
			},
		},
	}
}

func topologyManagedFocusParamConfig(extraOptions []funcapi.ParamOption) funcapi.ParamConfig {
	options := make([]funcapi.ParamOption, 0, 1+len(extraOptions))
	options = append(options, funcapi.ParamOption{
		ID:      topologyManagedFocusAllDevices,
		Name:    "All Devices",
		Default: true,
	})
	options = append(options, extraOptions...)

	return funcapi.ParamConfig{
		ID:        topologyParamManagedDeviceFocus,
		Name:      "Focus On",
		Help:      "Choose focus root set for depth filtering",
		Selection: funcapi.ParamMultiSelect,
		Options:   options,
	}
}

func topologyDepthParamConfig() funcapi.ParamConfig {
	options := make([]funcapi.ParamOption, 0, 1+(topologyDepthMax-topologyDepthMin+1))
	options = append(options, funcapi.ParamOption{
		ID:      topologyDepthAll,
		Name:    "All",
		Default: true,
	})
	for depth := topologyDepthMin; depth <= topologyDepthMax; depth++ {
		value := strconv.Itoa(depth)
		options = append(options, funcapi.ParamOption{
			ID:   value,
			Name: value,
		})
	}

	return funcapi.ParamConfig{
		ID:        topologyParamDepth,
		Name:      "Focus Depth",
		Help:      "Limit topology expansion hops from focus roots",
		Selection: funcapi.ParamSelect,
		Options:   options,
	}
}
