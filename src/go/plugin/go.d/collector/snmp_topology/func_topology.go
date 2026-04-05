// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import "github.com/netdata/netdata/go/plugins/pkg/funcapi"

// Compile-time interface check.
var _ funcapi.MethodHandler = (*funcTopology)(nil)

type funcTopology struct{}

const topologyMethodID = "topology:snmp"

const (
	topologyParamNodesIdentity      = "nodes_identity"
	topologyParamMapType            = "map_type"
	topologyParamInferenceStrategy  = "inference_strategy"
	topologyParamManagedDeviceFocus = "managed_snmp_device_focus"
	topologyParamDepth              = "depth"

	topologyNodesIdentityIP  = "ip"
	topologyNodesIdentityMAC = "mac"

	topologyMapTypeLLDPCDPManaged                = "lldp_cdp_managed"
	topologyMapTypeHighConfidenceInferred        = "high_confidence_inferred"
	topologyMapTypeAllDevicesLowConfidence       = "all_devices_low_confidence"
	topologyInferenceStrategyFDBMinimumKnowledge = "fdb_minimum_knowledge"
	topologyInferenceStrategySTPParentTree       = "stp_parent_tree"
	topologyInferenceStrategyFDBPairwise         = "fdb_pairwise_minimum_knowledge"
	topologyInferenceStrategySTPFDBCorrelated    = "stp_fdb_correlated"
	topologyInferenceStrategyCDPFDBHybrid        = "cdp_fdb_hybrid"
	topologyManagedFocusAllDevices               = "all_devices"
	topologyManagedFocusIPPrefix                 = "ip:"
	topologyDepthAll                             = "all"
	topologyDepthMin                             = 0
	topologyDepthMax                             = 10
	topologyDepthAllInternal                     = -1
)
