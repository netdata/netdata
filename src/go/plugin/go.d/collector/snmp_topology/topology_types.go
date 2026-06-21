// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"time"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/l2topology"
	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
)

const topologySchemaVersion = "2.0"

type topologyMatch = graph.Match
type topologyLinkEndpoint = graph.LinkEndpoint

type topologyLink struct {
	Layer        string
	Protocol     string
	LinkType     string
	Direction    string
	State        string
	SrcActorID   string
	DstActorID   string
	Src          topologyLinkEndpoint
	Dst          topologyLinkEndpoint
	DiscoveredAt *time.Time
	LastSeen     *time.Time
	Display      *graph.LinkDisplay
	L2           *graph.LinkL2
	Inference    *graph.LinkInference
	Detail       topologyLinkDetail
}

type topologyLinkDetail struct {
	L3Subnet *topologyL3SubnetLinkDetail
	OSPF     *topologyOSPFAdjacencyLinkDetail
	BGP      *topologyBGPAdjacencyLinkDetail
}

type topologyL3SubnetLinkDetail struct {
	Source  string
	SrcIP   string
	DstIP   string
	Subnet  string
	Network string
	Netmask string
	Prefix  int
}

type topologyOSPFAdjacencyLinkDetail struct {
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

type topologyBGPAdjacencyLinkDetail struct {
	Source          string
	RoutingInstance string
	LocalIP         string
	NeighborIP      string
	LocalAS         string
	RemoteAS        string
	LocalIdentifier string
	PeerIdentifier  string
}

type topologyActor struct {
	ActorID     string
	ActorType   string
	Layer       string
	Source      string
	Match       topologyMatch
	ParentMatch *topologyMatch
	Labels      map[string]string
	Detail      topologyActorDetail
}

type topologyActorDetail struct {
	L2   topologyengine.ProjectionActorDetail
	SNMP topologySNMPActorDetail
	OSPF []topologyOSPFNeighborDetailRow
	BGP  []topologyBGPPeerDetailRow
}

type topologySNMPActorDetail struct {
	ManagementAddresses   []topologyManagementAddress
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
	InterfaceCharts       map[string]topologyInterfaceChartRef
}

type topologyData struct {
	SchemaVersion string
	Source        string
	Layer         string
	AgentID       string
	CollectedAt   time.Time
	View          string
	Actors        []topologyActor
	Links         []topologyLink
	Stats         topologyStats
}

func topologyActorFromGraph(actor graph.Actor, detail topologyengine.ProjectionActorDetail) topologyActor {
	return topologyActor{
		ActorID:     actor.ActorID,
		ActorType:   actor.ActorType,
		Layer:       actor.Layer,
		Source:      actor.Source,
		Match:       actor.Match,
		ParentMatch: actor.ParentMatch,
		Labels:      actor.Labels,
		Detail:      topologyActorDetail{L2: detail},
	}
}

func topologyLinksFromGraph(links []graph.Link) []topologyLink {
	if len(links) == 0 {
		return nil
	}
	out := make([]topologyLink, len(links))
	for i, link := range links {
		out[i] = topologyLinkFromGraph(link)
	}
	return out
}

func topologyLinkFromGraph(link graph.Link) topologyLink {
	return topologyLink{
		Layer:        link.Layer,
		Protocol:     link.Protocol,
		LinkType:     link.LinkType,
		Direction:    link.Direction,
		State:        link.State,
		SrcActorID:   link.SrcActorID,
		DstActorID:   link.DstActorID,
		Src:          link.Src,
		Dst:          link.Dst,
		DiscoveredAt: link.DiscoveredAt,
		LastSeen:     link.LastSeen,
		Display:      link.Display,
		L2:           link.L2,
		Inference:    link.Inference,
	}
}

type topologyStats struct {
	L2          topologyengine.ProjectionStats
	HasL2       bool
	Shape       topologyShapeStats
	HasShape    bool
	Focus       topologyFocusStats
	HasFocus    bool
	L3          topologyL3EnrichmentStats
	HasL3       bool
	OSPF        topologyOSPFEnrichmentStats
	HasOSPF     bool
	BGP         topologyBGPEnrichmentStats
	HasBGP      bool
	Recomputed  topologyRecomputedStats
	HasComputed bool
}

type topologyShapeStats struct {
	ActorsCollapsedByIP           int
	ActorsNonIPInferredSuppressed int
	ActorsMapTypeSuppressed       int
	SegmentsSparseSuppressed      int
	MapType                       string
	InferenceStrategy             string
}

type topologyFocusStats struct {
	ManagedSNMPDeviceFocus string
	Depth                  topologyFocusDepth
	ActorsDepthFiltered    int
	LinksDepthFiltered     int
}

type topologyFocusDepth struct {
	All   bool
	Value int
}

type topologyRecomputedStats struct {
	ActorsTotal               int
	LinksTotal                int
	LinksProbable             int
	L3SubnetVisibleLinks      int
	OSPFAdjacencyVisibleLinks int
	BGPAdjacencyVisibleLinks  int
}

type topologyManagementAddress struct {
	Address     string `json:"address"`
	AddressType string `json:"address_type,omitempty"`
	IfSubtype   string `json:"if_subtype,omitempty"`
	IfID        string `json:"if_id,omitempty"`
	OID         string `json:"oid,omitempty"`
	Source      string `json:"source,omitempty"`
}

type topologyInterfaceChartRef struct {
	ChartIDSuffix    string   `json:"chart_id_suffix,omitempty"`
	AvailableMetrics []string `json:"available_metrics,omitempty"`
}

type topologyDevice struct {
	ChassisID             string                               `json:"chassis_id"`
	ChassisIDType         string                               `json:"chassis_id_type"`
	SysObjectID           string                               `json:"sys_object_id,omitempty"`
	SysName               string                               `json:"sys_name,omitempty"`
	SysDescr              string                               `json:"sys_descr,omitempty"`
	SysContact            string                               `json:"sys_contact,omitempty"`
	SysLocation           string                               `json:"sys_location,omitempty"`
	SysUptime             int64                                `json:"sys_uptime,omitempty"`
	SerialNumber          string                               `json:"serial_number,omitempty"`
	SoftwareVersion       string                               `json:"software_version,omitempty"`
	FirmwareVersion       string                               `json:"firmware_version,omitempty"`
	HardwareVersion       string                               `json:"hardware_version,omitempty"`
	ManagementIP          string                               `json:"management_ip,omitempty"`
	ManagementAddresses   []topologyManagementAddress          `json:"management_addresses,omitempty"`
	AgentID               string                               `json:"agent_id,omitempty"`
	AgentJobID            string                               `json:"agent_job_id,omitempty"`
	NetdataHostID         string                               `json:"netdata_host_id,omitempty"`
	ChartIDPrefix         string                               `json:"chart_id_prefix,omitempty"`
	ChartContextPrefix    string                               `json:"chart_context_prefix,omitempty"`
	DeviceCharts          map[string]string                    `json:"device_charts,omitempty"`
	InterfaceCharts       map[string]topologyInterfaceChartRef `json:"interface_charts,omitempty"`
	Vendor                string                               `json:"vendor,omitempty"`
	Model                 string                               `json:"model,omitempty"`
	OSPFRouterID          string                               `json:"ospf_router_id,omitempty"`
	Capabilities          []string                             `json:"capabilities,omitempty"`
	CapabilitiesSupported []string                             `json:"capabilities_supported,omitempty"`
	CapabilitiesEnabled   []string                             `json:"capabilities_enabled,omitempty"`
	Labels                map[string]string                    `json:"labels,omitempty"`
	Discovered            bool                                 `json:"discovered,omitempty"`
}

type topologyEndpoint struct {
	ChassisID           string                      `json:"chassis_id"`
	ChassisIDType       string                      `json:"chassis_id_type"`
	IfIndex             int                         `json:"if_index,omitempty"`
	IfName              string                      `json:"if_name,omitempty"`
	PortID              string                      `json:"port_id,omitempty"`
	PortIDType          string                      `json:"port_id_type,omitempty"`
	PortDescr           string                      `json:"port_descr,omitempty"`
	SysName             string                      `json:"sys_name,omitempty"`
	ManagementIP        string                      `json:"management_ip,omitempty"`
	ManagementAddresses []topologyManagementAddress `json:"management_addresses,omitempty"`
	AgentID             string                      `json:"agent_id,omitempty"`
	Labels              map[string]string           `json:"labels,omitempty"`
}
