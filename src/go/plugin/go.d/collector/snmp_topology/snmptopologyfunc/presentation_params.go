// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopologyfunc

import (
	"strconv"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
)

const (
	ParamNodesIdentity      = "nodes_identity"
	ParamMapType            = "map_type"
	ParamInferenceStrategy  = "inference_strategy"
	ParamManagedDeviceFocus = "managed_snmp_device_focus"
	ParamDepth              = "depth"

	NodesIdentityIP  = "ip"
	NodesIdentityMAC = "mac"
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
				ID:      topologyoptions.MapTypeLLDPCDPManaged,
				Name:    "LLDP/CDP/Managed Devices Map",
				Default: true,
			},
			{ID: topologyoptions.MapTypeHighConfidenceInferred, Name: "High Confidence Inferred Map"},
			{
				ID:   topologyoptions.MapTypeAllDevicesLowConfidence,
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
				ID:      topologyoptions.InferenceStrategyFDBMinimumKnowledge,
				Name:    "FDB Minimum-Knowledge (Baseline)",
				Default: true,
			},
			{
				ID:   topologyoptions.InferenceStrategySTPParentTree,
				Name: "STP Parent Tree",
			},
			{
				ID:   topologyoptions.InferenceStrategyFDBPairwise,
				Name: "FDB Pairwise Minimum-Knowledge",
			},
			{
				ID:   topologyoptions.InferenceStrategySTPFDBCorrelated,
				Name: "STP + FDB Correlated",
			},
			{
				ID:   topologyoptions.InferenceStrategyCDPFDBHybrid,
				Name: "CDP + FDB Hybrid",
			},
		},
	}
}

func managedFocusParamConfig(extraOptions []funcapi.ParamOption) funcapi.ParamConfig {
	options := make([]funcapi.ParamOption, 0, 1+len(extraOptions))
	options = append(options, funcapi.ParamOption{
		ID:      topologyoptions.ManagedFocusAllDevices,
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
	options := make([]funcapi.ParamOption, 0, 1+(topologyoptions.DepthMax-topologyoptions.DepthMin+1))
	options = append(options, funcapi.ParamOption{
		ID:      topologyoptions.DepthAll,
		Name:    "All",
		Default: true,
	})
	for depth := topologyoptions.DepthMin; depth <= topologyoptions.DepthMax; depth++ {
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
