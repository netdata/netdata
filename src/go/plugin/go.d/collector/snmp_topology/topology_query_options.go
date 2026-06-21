// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"

const (
	topologyMapTypeLLDPCDPManaged                = topologyoptions.MapTypeLLDPCDPManaged
	topologyMapTypeHighConfidenceInferred        = topologyoptions.MapTypeHighConfidenceInferred
	topologyMapTypeAllDevicesLowConfidence       = topologyoptions.MapTypeAllDevicesLowConfidence
	topologyInferenceStrategyFDBMinimumKnowledge = topologyoptions.InferenceStrategyFDBMinimumKnowledge
	topologyInferenceStrategySTPParentTree       = topologyoptions.InferenceStrategySTPParentTree
	topologyInferenceStrategyFDBPairwise         = topologyoptions.InferenceStrategyFDBPairwise
	topologyInferenceStrategySTPFDBCorrelated    = topologyoptions.InferenceStrategySTPFDBCorrelated
	topologyInferenceStrategyCDPFDBHybrid        = topologyoptions.InferenceStrategyCDPFDBHybrid
	topologyManagedFocusAllDevices               = topologyoptions.ManagedFocusAllDevices
	topologyManagedFocusIPPrefix                 = topologyoptions.ManagedFocusIPPrefix
	topologyDepthAll                             = topologyoptions.DepthAll
	topologyDepthMin                             = topologyoptions.DepthMin
	topologyDepthMax                             = topologyoptions.DepthMax
	topologyDepthAllInternal                     = topologyoptions.DepthAllInternal
)

type topologyQueryOptions = topologyoptions.QueryOptions
type topologyManagedFocusTarget = topologyoptions.ManagedFocusTarget
