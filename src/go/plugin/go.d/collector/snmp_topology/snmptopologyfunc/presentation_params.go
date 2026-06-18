// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopologyfunc

import (
	"strconv"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
)

const (
	ParamNodesIdentity      = "nodes_identity"
	ParamMapType            = "map_type"
	ParamInferenceStrategy  = "inference_strategy"
	ParamManagedDeviceFocus = "managed_snmp_device_focus"
	ParamDepth              = "depth"

	NodesIdentityIP  = "ip"
	NodesIdentityMAC = "mac"

	MapTypeLLDPCDPManaged                = "lldp_cdp_managed"
	MapTypeHighConfidenceInferred        = "high_confidence_inferred"
	MapTypeAllDevicesLowConfidence       = "all_devices_low_confidence"
	InferenceStrategyFDBMinimumKnowledge = "fdb_minimum_knowledge"
	InferenceStrategySTPParentTree       = "stp_parent_tree"
	InferenceStrategyFDBPairwise         = "fdb_pairwise_minimum_knowledge"
	InferenceStrategySTPFDBCorrelated    = "stp_fdb_correlated"
	InferenceStrategyCDPFDBHybrid        = "cdp_fdb_hybrid"
	ManagedFocusAllDevices               = "all_devices"
	ManagedFocusIPPrefix                 = "ip:"
	DepthAll                             = "all"
	DepthMin                             = 0
	DepthMax                             = 10
	DepthAllInternal                     = -1
)

func nodesIdentityParamConfig() funcapi.ParamConfig {
	return funcapi.ParamConfig{
		ID:        ParamNodesIdentity,
		Name:      "Nodes Identity",
		Help:      "Choose actor identity strategy: ip (collapse by IP, remove non-IP inferred) or mac",
		Selection: funcapi.ParamSelect,
		Options: []funcapi.ParamOption{
			{ID: NodesIdentityIP, Name: "IP", Default: true},
			{ID: NodesIdentityMAC, Name: "MAC"},
		},
	}
}

func mapTypeParamConfig() funcapi.ParamConfig {
	return funcapi.ParamConfig{
		ID:        ParamMapType,
		Name:      "Map",
		Help:      "Choose topology map mode",
		Selection: funcapi.ParamSelect,
		Options: []funcapi.ParamOption{
			{
				ID:      MapTypeLLDPCDPManaged,
				Name:    "LLDP/CDP/Managed Devices Map",
				Default: true,
			},
			{ID: MapTypeHighConfidenceInferred, Name: "High Confidence Inferred Map"},
			{
				ID:   MapTypeAllDevicesLowConfidence,
				Name: "All Devices (Low Confidence)",
			},
		},
	}
}

func inferenceStrategyParamConfig() funcapi.ParamConfig {
	return funcapi.ParamConfig{
		ID:        ParamInferenceStrategy,
		Name:      "Infer Strategy",
		Help:      "Choose the topology inference strategy",
		Selection: funcapi.ParamSelect,
		Options: []funcapi.ParamOption{
			{
				ID:      InferenceStrategyFDBMinimumKnowledge,
				Name:    "FDB Minimum-Knowledge (Baseline)",
				Default: true,
			},
			{
				ID:   InferenceStrategySTPParentTree,
				Name: "STP Parent Tree",
			},
			{
				ID:   InferenceStrategyFDBPairwise,
				Name: "FDB Pairwise Minimum-Knowledge",
			},
			{
				ID:   InferenceStrategySTPFDBCorrelated,
				Name: "STP + FDB Correlated",
			},
			{
				ID:   InferenceStrategyCDPFDBHybrid,
				Name: "CDP + FDB Hybrid",
			},
		},
	}
}

func managedFocusParamConfig(extraOptions []funcapi.ParamOption) funcapi.ParamConfig {
	options := make([]funcapi.ParamOption, 0, 1+len(extraOptions))
	options = append(options, funcapi.ParamOption{
		ID:      ManagedFocusAllDevices,
		Name:    "All Devices",
		Default: true,
	})
	options = append(options, extraOptions...)

	return funcapi.ParamConfig{
		ID:        ParamManagedDeviceFocus,
		Name:      "Focus On",
		Help:      "Choose focus root set for depth filtering",
		Selection: funcapi.ParamMultiSelect,
		Options:   options,
	}
}

func depthParamConfig() funcapi.ParamConfig {
	options := make([]funcapi.ParamOption, 0, 1+(DepthMax-DepthMin+1))
	options = append(options, funcapi.ParamOption{
		ID:      DepthAll,
		Name:    "All",
		Default: true,
	})
	for depth := DepthMin; depth <= DepthMax; depth++ {
		value := strconv.Itoa(depth)
		options = append(options, funcapi.ParamOption{
			ID:   value,
			Name: value,
		})
	}

	return funcapi.ParamConfig{
		ID:        ParamDepth,
		Name:      "Focus Depth",
		Help:      "Limit topology expansion hops from focus roots",
		Selection: funcapi.ParamSelect,
		Options:   options,
	}
}
