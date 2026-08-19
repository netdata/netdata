// SPDX-License-Identifier: GPL-3.0-or-later

package model

import "github.com/netdata/netdata/go/plugins/pkg/topology/graph"

// Projection is the L2 graph output consumed by topology producers.
type Projection struct {
	Graph        graph.Graph
	Stats        ProjectionStats
	ActorDetails map[string]ProjectionActorDetail
}

// OptionalValue keeps value presence explicit when a zero value is a real value.
type OptionalValue[T any] struct {
	Value T
	Has   bool
}

// ProjectionActorDetail carries typed L2-owned actor facts keyed by graph actor id.
type ProjectionActorDetail struct {
	DisplayName   string
	DisplaySource string

	Device   ProjectionDeviceActorDetail
	Endpoint ProjectionEndpointActorDetail
	Segment  ProjectionSegmentActorDetail

	CollapsedByIP  bool
	CollapsedCount int
}

type ProjectionDeviceActorDetail struct {
	DeviceID                 string
	Discovered               bool
	Inferred                 bool
	ManagementIP             string
	ManagementAddresses      []string
	Protocols                []string
	ProtocolsCollected       []string
	Capabilities             []string
	CapabilitiesSupported    []string
	CapabilitiesEnabled      []string
	Vendor                   string
	VendorSource             string
	VendorConfidence         string
	VendorDerived            string
	VendorDerivedSource      string
	VendorDerivedConfidence  string
	VendorDerivedMatchPrefix string
	VendorMatchPrefix        string
	PortsTotal               OptionalValue[int]
	IfIndexes                []string
	IfNames                  []string
	PortsUp                  OptionalValue[int]
	PortsDown                OptionalValue[int]
	PortsAdminDown           OptionalValue[int]
	TotalBandwidthBps        OptionalValue[int64]
	FDBTotalMACs             OptionalValue[int]
	VLANCount                OptionalValue[int]
	LLDPNeighborCount        OptionalValue[int]
	CDPNeighborCount         OptionalValue[int]
	AdminStatusCounts        map[string]int
	OperStatusCounts         map[string]int
	LinkModeCounts           map[string]int
	TopologyRoleCounts       map[string]int
	Ports                    []ProjectionPortDetail
}

type ProjectionEndpointActorDetail struct {
	Discovered               bool
	LearnedSources           []string
	LearnedDeviceIDs         []string
	LearnedIfIndexes         []string
	LearnedIfNames           []string
	Vendor                   string
	VendorSource             string
	VendorConfidence         string
	VendorMatchPrefix        string
	VendorDerived            string
	VendorDerivedSource      string
	VendorDerivedConfidence  string
	VendorDerivedMatchPrefix string
	AttachmentSource         string
	AttachedDeviceID         string
	AttachedDevice           string
	AttachedPort             string
	AttachedIfName           string
	AttachedIfIndex          int
	AttachedBridgePort       string
	AttachedVLAN             string
	AttachedVLANID           string
	AttachedBy               string
}

type ProjectionSegmentActorDetail struct {
	SegmentID      string
	SegmentType    string
	ParentDevices  []string
	IfNames        []string
	IfIndexes      []string
	BridgePorts    []string
	VLANIDs        []string
	LearnedSources []string
	PortsTotal     OptionalValue[int]
	EndpointsTotal OptionalValue[int]
	DesignatedPort string
	SegmentKind    string
}

type ProjectionPortDetail struct {
	IfIndex                OptionalValue[int]
	PortID                 string
	Name                   string
	IfName                 string
	IfDescr                string
	IfAlias                string
	MAC                    string
	Speed                  OptionalValue[int64]
	TopologyRole           string
	OperStatus             string
	AdminStatus            string
	PortType               string
	LinkMode               string
	STPState               string
	VLANIDs                []string
	FDBMACCount            OptionalValue[int]
	LinkCount              OptionalValue[int]
	NeighborCount          OptionalValue[int]
	Neighbors              []ProjectionPortNeighbor
	VLANs                  []ProjectionPortVLAN
	Duplex                 string
	LinkModeConfidence     string
	TopologyRoleConfidence string
	LinkModeSources        []string
	TopologyRoleSources    []string
	LastChange             string
	ChartIDSuffix          string
	AvailableMetrics       []string
}

type ProjectionPortNeighbor struct {
	Protocol           string
	RemoteDevice       string
	RemotePort         string
	RemoteIP           string
	RemoteChassisID    string
	RemoteCapabilities []string
}

type ProjectionPortVLAN struct {
	VLANID   string
	VLANName string
	Tagged   bool
}
