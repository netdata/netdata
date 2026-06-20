// SPDX-License-Identifier: GPL-3.0-or-later

package l2topology

import "github.com/netdata/netdata/go/plugins/pkg/topology/graph"

// Projection is the L2 engine output consumed by topology producers.
type Projection struct {
	Graph        graph.Graph
	Stats        ProjectionStats
	ActorDetails map[string]ProjectionActorDetail
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
	HasInventoryStats        bool
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
	PortsTotal               int
	IfIndexes                []string
	IfNames                  []string
	PortsUp                  int
	PortsDown                int
	PortsAdminDown           int
	TotalBandwidthBps        int64
	FDBTotalMACs             int
	VLANCount                int
	LLDPNeighborCount        int
	CDPNeighborCount         int
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
	HasStats       bool
	SegmentID      string
	SegmentType    string
	ParentDevices  []string
	IfNames        []string
	IfIndexes      []string
	BridgePorts    []string
	VLANIDs        []string
	LearnedSources []string
	PortsTotal     int
	EndpointsTotal int
	DesignatedPort string
	SegmentKind    string
}

type ProjectionPortDetail struct {
	IfIndex                int
	PortID                 string
	Name                   string
	IfName                 string
	IfDescr                string
	IfAlias                string
	MAC                    string
	Speed                  int64
	TopologyRole           string
	OperStatus             string
	AdminStatus            string
	PortType               string
	LinkMode               string
	STPState               string
	VLANIDs                []string
	FDBMACCount            int
	LinkCount              int
	NeighborCount          int
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
