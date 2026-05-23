// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"net/netip"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/topology"
)

// TopologyDataOptions controls conversion from Result to topology.Data.
type TopologyDataOptions struct {
	SchemaVersion             string
	Source                    string
	Layer                     string
	View                      string
	AgentID                   string
	LocalDeviceID             string
	CollectedAt               time.Time
	ResolveDNSName            func(ip string) string
	CollapseActorsByIP        bool
	EliminateNonIPInferred    bool
	ProbabilisticConnectivity bool
	InferenceStrategy         string
}

const (
	topologyInferenceStrategyFDBMinimumKnowledge = "fdb_minimum_knowledge"
	topologyInferenceStrategySTPParentTree       = "stp_parent_tree"
	topologyInferenceStrategyFDBPairwise         = "fdb_pairwise_minimum_knowledge"
	topologyInferenceStrategySTPFDBCorrelated    = "stp_fdb_correlated"
	topologyInferenceStrategyCDPFDBHybrid        = "cdp_fdb_hybrid"
)

type topologyInferenceStrategyConfig struct {
	id                               string
	includeLLDPBridgeLinks           bool
	includeCDPBridgeLinks            bool
	includeSTPBridgeLinks            bool
	useSTPDesignatedParent           bool
	enableFDBPairwiseLinks           bool
	enableSTPManagedAliasCorrelation bool
	filterSwitchFacingAttachments    bool
}

type endpointActorAccumulator struct {
	endpointID string
	mac        string
	ips        map[string]netip.Addr
	sources    map[string]struct{}
	deviceIDs  map[string]struct{}
	ifIndexes  map[string]struct{}
	ifNames    map[string]struct{}
}

type projectedSegments struct {
	actors                        []topology.Actor
	links                         []topology.Link
	linksFdb                      int
	bidirectionalCount            int
	endpointLinksCandidates       int
	endpointLinksEmitted          int
	endpointLinksSuppressed       int
	endpointsWithAmbiguousSegment int
	endpointDirectOwners          map[string]fdbEndpointOwner
	suppressedManagedOverlapIDs   map[string]struct{}
}

type fdbReporterObservation struct {
	byEndpoint map[string]map[string]map[string]struct{}
	byReporter map[string]map[string]map[string]struct{}
}

type fdbEndpointOwner struct {
	portKey     string
	portVLANKey string
	port        bridgePortRef
	source      string
}

type probableEndpointReporterHint struct {
	deviceID string
	ifIndex  int
	ifName   string
}

type segmentReporterIndex struct {
	byDevice        map[string]map[string]struct{}
	byDeviceIfIndex map[string]map[string]struct{}
	byDeviceIfName  map[string]map[string]struct{}
}

type topologyIdentityKeySet map[string]struct{}

type topologyDevicePortStatus struct {
	IfIndex        int
	IfName         string
	IfDescr        string
	IfAlias        string
	MAC            string
	SpeedBps       int64
	LastChange     int64
	Duplex         string
	InterfaceType  string
	AdminStatus    string
	OperStatus     string
	LinkMode       string
	ModeConfidence string
	ModeSources    []string
	VLANIDs        []string
	TopologyRole   string
	RoleConfidence string
	RoleSources    []string
	FDBMACCount    int
	STPState       string
	VLANs          []map[string]any
	Neighbors      []topologyPortNeighborStatus
}

type topologyPortNeighborStatus struct {
	Protocol           string
	RemoteDevice       string
	RemotePort         string
	RemoteIP           string
	RemoteChassisID    string
	RemoteCapabilities []string
}

type topologyDeviceInterfaceSummary struct {
	portsTotal        int
	ifIndexes         []string
	ifNames           []string
	adminStatusCount  map[string]any
	operStatusCount   map[string]any
	linkModeCount     map[string]any
	roleCount         map[string]any
	portsUp           int
	portsDown         int
	portsAdminDown    int
	totalBandwidthBps int64
	fdbTotalMACs      int
	vlanCount         int
	lldpNeighborCount int
	cdpNeighborCount  int
	portStatuses      []map[string]any
}

type topologyDevicePortEvidence struct {
	vlanIDs            map[string]struct{}
	vlanNames          map[string]string
	fdbEndpointIDs     map[string]struct{}
	hasFDB             bool
	hasFDBManagedAlias bool
	hasSTP             bool
	hasPeer            bool
	hasBridgeLink      bool
	isLAG              bool
	stpStates          map[string]struct{}
	neighbors          map[string]topologyPortNeighborStatus
}

func normalizeTopologyInferenceStrategy(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
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
		return topologyInferenceStrategyFDBMinimumKnowledge
	}
}

func topologyInferenceStrategyConfigFor(strategy string) topologyInferenceStrategyConfig {
	switch normalizeTopologyInferenceStrategy(strategy) {
	case topologyInferenceStrategySTPParentTree:
		return topologyInferenceStrategyConfig{
			id:                            topologyInferenceStrategySTPParentTree,
			includeSTPBridgeLinks:         true,
			useSTPDesignatedParent:        true,
			filterSwitchFacingAttachments: true,
		}
	case topologyInferenceStrategyFDBPairwise:
		return topologyInferenceStrategyConfig{
			id:                            topologyInferenceStrategyFDBPairwise,
			enableFDBPairwiseLinks:        true,
			filterSwitchFacingAttachments: true,
		}
	case topologyInferenceStrategySTPFDBCorrelated:
		return topologyInferenceStrategyConfig{
			id:                               topologyInferenceStrategySTPFDBCorrelated,
			includeLLDPBridgeLinks:           true,
			includeCDPBridgeLinks:            true,
			includeSTPBridgeLinks:            true,
			useSTPDesignatedParent:           true,
			enableFDBPairwiseLinks:           true,
			enableSTPManagedAliasCorrelation: true,
			filterSwitchFacingAttachments:    true,
		}
	case topologyInferenceStrategyCDPFDBHybrid:
		return topologyInferenceStrategyConfig{
			id:                            topologyInferenceStrategyCDPFDBHybrid,
			includeCDPBridgeLinks:         true,
			enableFDBPairwiseLinks:        true,
			filterSwitchFacingAttachments: true,
		}
	default:
		return topologyInferenceStrategyConfig{
			id:                            topologyInferenceStrategyFDBMinimumKnowledge,
			includeLLDPBridgeLinks:        true,
			includeCDPBridgeLinks:         true,
			filterSwitchFacingAttachments: true,
		}
	}
}

// ToTopologyData converts an engine result to the shared topology schema.
func ToTopologyData(result Result, opts TopologyDataOptions) topology.Data {
	builder := newTopologyDataBuilder(result, opts)
	builder.prepareIndexes()
	builder.collectBridgeTopologyInputs()
	builder.buildDeviceActors()
	builder.projectAdjacencyTopology()
	builder.buildEndpointTopology()
	builder.buildSegmentTopology()
	builder.finalizeGraph()
	builder.buildStats()
	return builder.data()
}
