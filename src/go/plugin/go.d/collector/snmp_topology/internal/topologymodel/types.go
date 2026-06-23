// SPDX-License-Identifier: GPL-3.0-or-later

package topologymodel

import (
	"time"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/l2topology"
	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
)

const SchemaVersion = "2.0"

const (
	L3SubnetSegmentActorType   = "l3_subnet_segment"
	L3SubnetLinkType           = "l3_subnet"
	L3SubnetMembershipLinkType = "l3_subnet_membership"
	OSPFAdjacencyLinkType      = "ospf_adjacency"
	BGPAdjacencyLinkType       = "bgp_adjacency"
)

const (
	SegmentKindBroadcastDomain = "broadcast_domain"
	SegmentKindL3Subnet        = "l3_subnet"
)

type Match = graph.Match
type LinkEndpoint = graph.LinkEndpoint

type Link struct {
	Layer        string
	Protocol     string
	LinkType     string
	Direction    string
	State        string
	SrcActorID   string
	DstActorID   string
	Src          LinkEndpoint
	Dst          LinkEndpoint
	DiscoveredAt *time.Time
	LastSeen     *time.Time
	Display      *graph.LinkDisplay
	L2           *graph.LinkL2
	Inference    *graph.LinkInference
	Detail       LinkDetail
}

type LinkDetail struct {
	L3Subnet           *L3SubnetLinkDetail
	L3SubnetMembership *L3SubnetMembershipLinkDetail
	OSPF               *OSPFAdjacencyLinkDetail
	BGP                *BGPAdjacencyLinkDetail
}

type L3SubnetLinkDetail struct {
	Source  string
	SrcIP   string
	DstIP   string
	Subnet  string
	Network string
	Netmask string
	Prefix  int
}

type L3SubnetMembershipLinkDetail struct {
	Source     string
	Subnet     string
	Network    string
	Netmask    string
	Prefix     int
	Interfaces []L3SubnetMembershipInterface
}

type L3SubnetMembershipInterface struct {
	MemberIP string
	IfIndex  int
	IfName   string
	IfDescr  string
}

type OSPFAdjacencyLinkDetail struct {
	Source           string
	LocalRouterID    string
	NeighborRouterID string
	LocalIP          string
	NeighborIP       string
	AddresslessIndex string
	Subnet           string
	Network          string
	Netmask          string
	Prefix           int
}

type BGPAdjacencyLinkDetail struct {
	Source          string
	RoutingInstance string
	LocalIP         string
	NeighborIP      string
	LocalAS         string
	RemoteAS        string
	LocalIdentifier string
	PeerIdentifier  string
}

type Actor struct {
	ActorID     string
	ActorType   string
	SegmentKind string
	Layer       string
	Source      string
	Match       Match
	ParentMatch *Match
	Labels      map[string]string
	Detail      ActorDetail
}

type ActorDetail struct {
	L2   topologyengine.ProjectionActorDetail
	SNMP SNMPActorDetail
	OSPF []OSPFNeighborDetailRow
	BGP  []BGPPeerDetailRow
}

type SNMPActorDetail struct {
	ManagementAddresses   []ManagementAddress
	Capabilities          []string
	CapabilitiesSupported []string
	CapabilitiesEnabled   []string
	SysDescr              string
	SysContact            string
	SysLocation           string
	SysUptime             int64
	Vendor                string
	VendorSource          string
	VendorConfidence      string
	Model                 string
	OSPFRouterID          string
	SerialNumber          string
	SoftwareVersion       string
	FirmwareVersion       string
	HardwareVersion       string
	ManagementIP          string
	NetdataHostID         string
	ChartIDPrefix         string
	ChartContextPrefix    string
	DeviceCharts          map[string]string
	InterfaceCharts       map[string]InterfaceChartRef
}

type Data struct {
	SchemaVersion string
	Source        string
	Layer         string
	AgentID       string
	CollectedAt   time.Time
	View          string
	Actors        []Actor
	Links         []Link
	Stats         Stats
}

type ManagementAddress struct {
	Address     string `json:"address"`
	AddressType string `json:"address_type,omitempty"`
	IfSubtype   string `json:"if_subtype,omitempty"`
	IfID        string `json:"if_id,omitempty"`
	OID         string `json:"oid,omitempty"`
	Source      string `json:"source,omitempty"`
}

type InterfaceChartRef struct {
	ChartIDSuffix    string   `json:"chart_id_suffix,omitempty"`
	AvailableMetrics []string `json:"available_metrics,omitempty"`
}

type Device struct {
	ChassisID             string                       `json:"chassis_id"`
	ChassisIDType         string                       `json:"chassis_id_type"`
	SysObjectID           string                       `json:"sys_object_id,omitempty"`
	SysName               string                       `json:"sys_name,omitempty"`
	SysDescr              string                       `json:"sys_descr,omitempty"`
	SysContact            string                       `json:"sys_contact,omitempty"`
	SysLocation           string                       `json:"sys_location,omitempty"`
	SysUptime             int64                        `json:"sys_uptime,omitempty"`
	SerialNumber          string                       `json:"serial_number,omitempty"`
	SoftwareVersion       string                       `json:"software_version,omitempty"`
	FirmwareVersion       string                       `json:"firmware_version,omitempty"`
	HardwareVersion       string                       `json:"hardware_version,omitempty"`
	ManagementIP          string                       `json:"management_ip,omitempty"`
	ManagementAddresses   []ManagementAddress          `json:"management_addresses,omitempty"`
	AgentID               string                       `json:"agent_id,omitempty"`
	AgentJobID            string                       `json:"agent_job_id,omitempty"`
	NetdataHostID         string                       `json:"netdata_host_id,omitempty"`
	ChartIDPrefix         string                       `json:"chart_id_prefix,omitempty"`
	ChartContextPrefix    string                       `json:"chart_context_prefix,omitempty"`
	DeviceCharts          map[string]string            `json:"device_charts,omitempty"`
	InterfaceCharts       map[string]InterfaceChartRef `json:"interface_charts,omitempty"`
	Vendor                string                       `json:"vendor,omitempty"`
	Model                 string                       `json:"model,omitempty"`
	OSPFRouterID          string                       `json:"ospf_router_id,omitempty"`
	Capabilities          []string                     `json:"capabilities,omitempty"`
	CapabilitiesSupported []string                     `json:"capabilities_supported,omitempty"`
	CapabilitiesEnabled   []string                     `json:"capabilities_enabled,omitempty"`
	Labels                map[string]string            `json:"labels,omitempty"`
	Discovered            bool                         `json:"discovered,omitempty"`
}

type Endpoint struct {
	ChassisID           string              `json:"chassis_id"`
	ChassisIDType       string              `json:"chassis_id_type"`
	IfIndex             int                 `json:"if_index,omitempty"`
	IfName              string              `json:"if_name,omitempty"`
	PortID              string              `json:"port_id,omitempty"`
	PortIDType          string              `json:"port_id_type,omitempty"`
	PortDescr           string              `json:"port_descr,omitempty"`
	SysName             string              `json:"sys_name,omitempty"`
	ManagementIP        string              `json:"management_ip,omitempty"`
	ManagementAddresses []ManagementAddress `json:"management_addresses,omitempty"`
	AgentID             string              `json:"agent_id,omitempty"`
	Labels              map[string]string   `json:"labels,omitempty"`
}
