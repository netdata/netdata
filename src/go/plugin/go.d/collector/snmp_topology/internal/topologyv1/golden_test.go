// SPDX-License-Identifier: GPL-3.0-or-later

package topologyv1

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/l2topology"
	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyv1test"
	"github.com/stretchr/testify/require"
)

var updateSNMPTopologyV1Golden = flag.Bool(
	"update-snmp-topology-v1-golden",
	false,
	"rewrite snmp_topology normalized topology.v1 golden testdata",
)

const snmpTopologyV1GoldenPayloadPath = "../../testdata/topology_v1_normalized_golden.json"

// Refresh with:
// go test -count=1 ./plugin/go.d/collector/snmp_topology/internal/topologyv1 -run TestSNMPTopologyToV1_NormalizedGolden -update-snmp-topology-v1-golden
func TestSNMPTopologyToV1_NormalizedGolden(t *testing.T) {
	data, err := Render(snmpTopologyV1GoldenInput())
	require.NoError(t, err)
	require.NoError(t, topologyv1test.ValidateData(data))

	normalized := topologyv1test.NormalizeData(t, data)
	actual := topologyv1test.CanonicalJSON(t, normalized)
	if *updateSNMPTopologyV1Golden {
		require.NoError(t, os.MkdirAll(filepath.Dir(snmpTopologyV1GoldenPayloadPath), 0o755))
		require.NoError(t, os.WriteFile(snmpTopologyV1GoldenPayloadPath, []byte(actual), 0o644))
	}

	expected, err := os.ReadFile(snmpTopologyV1GoldenPayloadPath)
	require.NoError(t, err)
	if string(expected) != actual {
		t.Fatalf("normalized golden changed; inspect the payload below and rerun with -update-snmp-topology-v1-golden if intentional:\n%s", actual)
	}
}

func snmpTopologyV1GoldenInput() topologymodel.Data {
	collectedAt := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	discoveredAt := time.Date(2026, time.January, 2, 2, 4, 5, 0, time.UTC)
	lastSeen := time.Date(2026, time.January, 2, 3, 3, 5, 0, time.UTC)

	return topologymodel.Data{
		SchemaVersion: topologymodel.SchemaVersion,
		Source:        "snmp_topology_test",
		Layer:         "multi",
		AgentID:       "golden-agent",
		CollectedAt:   collectedAt,
		View:          "summary",
		Actors: []topologymodel.Actor{
			{
				ActorID:   "switch-a",
				ActorType: "switch",
				Layer:     "2",
				Source:    "snmp",
				Match: topologymodel.Match{
					ChassisIDs:   []string{"aa:bb:cc:00:00:01"},
					MacAddresses: []string{"aa:bb:cc:00:00:01"},
					IPAddresses:  []string{"192.0.2.10"},
					Hostnames:    []string{"switch-a.example.test"},
					DNSNames:     []string{"switch-a.example.test"},
					SysObjectID:  "1.3.6.1.4.1.9.1.1208",
					SysName:      "switch-a",
				},
				Labels: map[string]string{
					"role": "distribution",
					"site": "lab",
				},
				Detail: topologymodel.ActorDetail{
					L2: topologyengine.ProjectionActorDetail{
						DisplayName: "Switch A",
						Device: topologyengine.ProjectionDeviceActorDetail{
							ManagementIP:      "192.0.2.10",
							Protocols:         []string{"lldp", "cdp", "snmp"},
							PortsTotal:        topologyengine.OptionalValue[int]{Value: 2, Has: true},
							FDBTotalMACs:      topologyengine.OptionalValue[int]{Value: 42, Has: true},
							VLANCount:         topologyengine.OptionalValue[int]{Value: 3, Has: true},
							LLDPNeighborCount: topologyengine.OptionalValue[int]{Value: 1, Has: true},
							CDPNeighborCount:  topologyengine.OptionalValue[int]{Value: 1, Has: true},
							Ports: []topologyengine.ProjectionPortDetail{
								{
									AdminStatus:            "up",
									Duplex:                 "full",
									FDBMACCount:            topologyengine.OptionalValue[int]{Value: 12, Has: true},
									IfAlias:                "to router-a",
									IfDescr:                "GigabitEthernet1/0/1",
									IfIndex:                topologyengine.OptionalValue[int]{Value: 101, Has: true},
									IfName:                 "Gi1/0/1",
									LastChange:             "2026-01-02T01:02:03Z",
									LinkCount:              topologyengine.OptionalValue[int]{Value: 1, Has: true},
									LinkMode:               "trunk",
									LinkModeConfidence:     "high",
									LinkModeSources:        []string{"if_mib", "lldp"},
									MAC:                    "aa:bb:cc:00:01:01",
									NeighborCount:          topologyengine.OptionalValue[int]{Value: 1, Has: true},
									OperStatus:             "up",
									PortID:                 "Gi1/0/1",
									PortType:               "ethernet",
									Speed:                  topologyengine.OptionalValue[int64]{Value: 1000000000, Has: true},
									STPState:               "forwarding",
									TopologyRole:           "uplink",
									TopologyRoleConfidence: "medium",
									TopologyRoleSources:    []string{"stp", "fdb"},
									VLANIDs:                []string{"10", "20"},
									Neighbors: []topologyengine.ProjectionPortNeighbor{
										{
											Protocol:        "lldp",
											RemoteChassisID: "aa:bb:cc:00:00:02",
											RemotePort:      "Gi0/0",
										},
									},
									VLANs: []topologyengine.ProjectionPortVLAN{
										{VLANID: "10", VLANName: "users"},
										{VLANID: "20", VLANName: "servers"},
									},
								},
							},
						},
					},
					SNMP: topologymodel.SNMPActorDetail{
						Capabilities:       []string{"bridge", "router"},
						ChartIDPrefix:      "snmp_switch_a",
						ChartContextPrefix: "snmp.switch_a",
						ManagementIP:       "192.0.2.10",
						Model:              "Catalyst 9300",
						NetdataHostID:      "host-switch-a",
						OSPFRouterID:       "10.255.0.1",
						SysContact:         "noc@example.test",
						SysDescr:           "Synthetic switch fixture",
						SysLocation:        "lab-rack-1",
						Vendor:             "Cisco",
					},
				},
			},
			{
				ActorID:   "router-a",
				ActorType: "router",
				Layer:     "3",
				Source:    "snmp",
				Match: topologymodel.Match{
					ChassisIDs: []string{"aa:bb:cc:00:00:02"},
					IPAddresses: []string{
						"198.51.100.1",
						"203.0.113.1",
					},
					SysObjectID: "1.3.6.1.4.1.9.1.1745",
					SysName:     "router-a",
				},
				Labels: map[string]string{
					"role": "edge",
				},
				Detail: topologymodel.ActorDetail{
					L2: topologyengine.ProjectionActorDetail{
						DisplayName: "Router A",
						Device: topologyengine.ProjectionDeviceActorDetail{
							ManagementIP:       "198.51.100.1",
							PortsTotal:         topologyengine.OptionalValue[int]{Value: 4, Has: true},
							ProtocolsCollected: []string{"ip_mib", "ospf_mib", "bgp_mib"},
							Ports: []topologyengine.ProjectionPortDetail{
								{
									IfAlias:      "to switch-a",
									IfIndex:      topologyengine.OptionalValue[int]{Value: 201, Has: true},
									IfName:       "Gi0/0",
									LinkMode:     "trunk",
									OperStatus:   "up",
									PortID:       "Gi0/0",
									TopologyRole: "uplink",
								},
							},
						},
					},
					SNMP: topologymodel.SNMPActorDetail{
						Capabilities: []string{"router"},
						ManagementIP: "198.51.100.1",
						Model:        "ASR 1001",
						OSPFRouterID: "10.255.0.1",
						SysDescr:     "Synthetic router A",
						Vendor:       "Cisco",
					},
					BGP: []topologymodel.BGPPeerDetailRow{
						{
							AdminStatus:           "enabled",
							BGPVersion:            "4",
							Description:           "router-b transit",
							EstablishedUptime:     new(int64(7200)),
							LastReceivedUpdateAge: new(int64(15)),
							LocalAS:               "64512",
							LocalIdentifier:       "10.255.0.1",
							LocalIP:               "203.0.113.1",
							NeighborIP:            "203.0.113.2",
							PeerIdentifier:        "10.255.0.2",
							PeerType:              "external",
							RemoteActorID:         "router-b",
							RemoteAS:              "64513",
							RoutingInstance:       "default",
							Source:                "bgp_mib",
							State:                 "established",
						},
					},
					OSPF: []topologymodel.OSPFNeighborDetailRow{
						{
							AddresslessIndex: "0",
							LocalIP:          "198.51.100.1",
							LocalRouterID:    "10.255.0.1",
							NeighborIP:       "198.51.100.2",
							NeighborRouterID: "10.255.0.2",
							RemoteActorID:    "router-b",
							Source:           "ospf_mib",
							State:            "full",
							Subnet:           "198.51.100.0/30",
						},
					},
				},
			},
			{
				ActorID:   "router-b",
				ActorType: "router",
				Layer:     "3",
				Source:    "snmp",
				Match: topologymodel.Match{
					ChassisIDs:  []string{"aa:bb:cc:00:00:03"},
					IPAddresses: []string{"198.51.100.2", "203.0.113.2"},
					SysObjectID: "1.3.6.1.4.1.2636.1.1.1.2.131",
					SysName:     "router-b",
				},
				Detail: topologymodel.ActorDetail{
					L2: topologyengine.ProjectionActorDetail{
						DisplayName: "Router B",
						Device: topologyengine.ProjectionDeviceActorDetail{
							ManagementIP: "198.51.100.2",
						},
					},
					SNMP: topologymodel.SNMPActorDetail{
						ManagementIP: "198.51.100.2",
						Model:        "MX204",
						OSPFRouterID: "10.255.0.2",
						SysDescr:     "Synthetic router B",
						Vendor:       "Juniper",
					},
					BGP: []topologymodel.BGPPeerDetailRow{
						{
							AdminStatus:           "enabled",
							BGPVersion:            "4",
							Description:           "router-a transit",
							EstablishedUptime:     new(int64(7100)),
							LastReceivedUpdateAge: new(int64(20)),
							LocalAS:               "64513",
							LocalIdentifier:       "10.255.0.2",
							LocalIP:               "203.0.113.2",
							NeighborIP:            "203.0.113.1",
							PeerIdentifier:        "10.255.0.1",
							PeerType:              "external",
							RemoteActorID:         "router-a",
							RemoteAS:              "64512",
							RoutingInstance:       "default",
							Source:                "bgp_mib",
							State:                 "established",
						},
					},
					OSPF: []topologymodel.OSPFNeighborDetailRow{
						{
							AddresslessIndex: "0",
							LocalIP:          "198.51.100.2",
							LocalRouterID:    "10.255.0.2",
							NeighborIP:       "198.51.100.1",
							NeighborRouterID: "10.255.0.1",
							RemoteActorID:    "router-a",
							Source:           "ospf_mib",
							State:            "full",
							Subnet:           "198.51.100.0/30",
						},
					},
				},
			},
			{
				ActorID:   "server-a",
				ActorType: "endpoint",
				Layer:     "2",
				Source:    "fdb",
				Match: topologymodel.Match{
					MacAddresses: []string{"00:11:22:33:44:55"},
					IPAddresses:  []string{"192.0.2.50"},
					Hostnames:    []string{"server-a"},
				},
				Labels: map[string]string{
					"role": "application",
				},
				Detail: topologymodel.ActorDetail{
					L2: topologyengine.ProjectionActorDetail{
						DisplayName: "Server A",
						Endpoint: topologyengine.ProjectionEndpointActorDetail{
							LearnedSources: []string{"fdb", "arp"},
						},
					},
				},
			},
			{
				ActorID:     "segment-198-51-100-0-30",
				ActorType:   "segment",
				SegmentKind: topologymodel.SegmentKindBroadcastDomain,
				Layer:       "3",
				Source:      "ip_mib",
				Match: topologymodel.Match{
					IPAddresses: []string{"198.51.100.0"},
				},
				Detail: topologymodel.ActorDetail{
					L2: topologyengine.ProjectionActorDetail{
						DisplayName: "198.51.100.0/30",
					},
				},
			},
			{
				ActorID:     "l3-subnet-segment-203-0-113-0-24",
				ActorType:   topologymodel.L3SubnetSegmentActorType,
				SegmentKind: topologymodel.SegmentKindL3Subnet,
				Layer:       "3",
				Source:      "ip_mib",
				Detail: topologymodel.ActorDetail{
					L2: topologyengine.ProjectionActorDetail{
						DisplayName: "203.0.113.0/24",
						Segment: topologyengine.ProjectionSegmentActorDetail{
							SegmentID:      "l3-subnet-segment-203-0-113-0-24",
							SegmentType:    topologymodel.SegmentKindL3Subnet,
							SegmentKind:    topologymodel.SegmentKindL3Subnet,
							PortsTotal:     topologyengine.OptionalValue[int]{Value: 2, Has: true},
							EndpointsTotal: topologyengine.OptionalValue[int]{Value: 2, Has: true},
						},
					},
				},
			},
		},
		Links: []topologymodel.Link{
			{
				Layer:      "2",
				Protocol:   "lldp",
				LinkType:   "lldp",
				Direction:  "observed",
				State:      "up",
				SrcActorID: "switch-a",
				DstActorID: "router-a",
				Src: topologymodel.LinkEndpoint{
					IfIndex:      101,
					IfName:       "Gi1/0/1",
					ManagementIP: "192.0.2.10",
					PortID:       "Gi1/0/1",
				},
				Dst: topologymodel.LinkEndpoint{
					IfIndex:      201,
					IfName:       "Gi0/0",
					ManagementIP: "198.51.100.1",
					PortID:       "Gi0/0",
				},
				DiscoveredAt: &discoveredAt,
				LastSeen:     &lastSeen,
				Inference: &graph.LinkInference{
					AttachmentMode: "direct",
					Confidence:     "high",
					Inference:      "lldp",
				},
			},
			{
				Layer:      "2",
				Protocol:   "fdb",
				LinkType:   "bridge",
				Direction:  "observed",
				State:      "probable",
				SrcActorID: "switch-a",
				DstActorID: "server-a",
				Src: topologymodel.LinkEndpoint{
					IfIndex: 102,
					IfName:  "Gi1/0/2",
					PortID:  "Gi1/0/2",
				},
				Dst: topologymodel.LinkEndpoint{},
				Inference: &graph.LinkInference{
					AttachmentMode: "probable_host",
					Confidence:     "low",
					Inference:      "probable",
				},
			},
			{
				Layer:      "3",
				Protocol:   topologymodel.L3SubnetLinkType,
				LinkType:   topologymodel.L3SubnetLinkType,
				Direction:  "observed",
				SrcActorID: "router-a",
				DstActorID: "router-b",
				Src: topologymodel.LinkEndpoint{
					IfIndex: 301,
					IfName:  "xe-0/0/0",
				},
				Dst: topologymodel.LinkEndpoint{
					IfIndex: 401,
					IfName:  "xe-0/0/1",
				},
				Inference: &graph.LinkInference{
					AttachmentMode: "logical_l3_subnet",
					Inference:      "shared_subnet",
				},
				Detail: topologymodel.LinkDetail{
					L3Subnet: &topologymodel.L3SubnetLinkDetail{
						Source:  "ip_mib",
						SrcIP:   "198.51.100.1",
						DstIP:   "198.51.100.2",
						Subnet:  "198.51.100.0/30",
						Network: "198.51.100.0",
						Netmask: "255.255.255.252",
						Prefix:  30,
					},
				},
			},
			{
				Layer:      "3",
				Protocol:   topologymodel.OSPFAdjacencyLinkType,
				LinkType:   topologymodel.OSPFAdjacencyLinkType,
				Direction:  "observed",
				State:      "full",
				SrcActorID: "router-a",
				DstActorID: "router-b",
				Src:        topologymodel.LinkEndpoint{},
				Dst:        topologymodel.LinkEndpoint{},
				Inference: &graph.LinkInference{
					AttachmentMode: "logical_ospf_adjacency",
					Inference:      "ospf_neighbor",
				},
				Detail: topologymodel.LinkDetail{
					OSPF: &topologymodel.OSPFAdjacencyLinkDetail{
						Source:           "ospf_mib",
						LocalRouterID:    "10.255.0.1",
						NeighborRouterID: "10.255.0.2",
						LocalIP:          "198.51.100.1",
						NeighborIP:       "198.51.100.2",
						Subnet:           "198.51.100.0/30",
						Network:          "198.51.100.0",
						Netmask:          "255.255.255.252",
						Prefix:           30,
					},
				},
			},
			{
				Layer:      "3",
				Protocol:   topologymodel.BGPAdjacencyLinkType,
				LinkType:   topologymodel.BGPAdjacencyLinkType,
				Direction:  "observed",
				State:      "established",
				SrcActorID: "router-a",
				DstActorID: "router-b",
				Src:        topologymodel.LinkEndpoint{},
				Dst:        topologymodel.LinkEndpoint{},
				Inference: &graph.LinkInference{
					AttachmentMode: "logical_bgp_adjacency",
					Inference:      "bgp_peer",
				},
				Detail: topologymodel.LinkDetail{
					BGP: &topologymodel.BGPAdjacencyLinkDetail{
						Source:          "bgp_mib",
						RoutingInstance: "default",
						LocalIdentifier: "10.255.0.1",
						PeerIdentifier:  "10.255.0.2",
						LocalIP:         "203.0.113.1",
						NeighborIP:      "203.0.113.2",
						LocalAS:         "64512",
						RemoteAS:        "64513",
					},
				},
			},
			{
				Layer:      "3",
				Protocol:   topologymodel.L3SubnetMembershipLinkType,
				LinkType:   topologymodel.L3SubnetMembershipLinkType,
				Direction:  "observed",
				SrcActorID: "router-a",
				DstActorID: "l3-subnet-segment-203-0-113-0-24",
				Src: topologymodel.LinkEndpoint{
					IfIndex: 302,
					IfName:  "xe-0/0/2",
				},
				Dst: topologymodel.LinkEndpoint{},
				Inference: &graph.LinkInference{
					AttachmentMode: "logical_l3_subnet_membership",
					Inference:      "shared_subnet_membership",
				},
				Detail: topologymodel.LinkDetail{
					L3SubnetMembership: &topologymodel.L3SubnetMembershipLinkDetail{
						Source:  "ip_mib",
						Subnet:  "203.0.113.0/24",
						Network: "203.0.113.0",
						Netmask: "255.255.255.0",
						Prefix:  24,
						Interfaces: []topologymodel.L3SubnetMembershipInterface{
							{MemberIP: "203.0.113.1", IfIndex: 302, IfName: "xe-0/0/2", IfDescr: "Transit VLAN"},
						},
					},
				},
			},
			{
				Layer:      "3",
				Protocol:   topologymodel.L3SubnetMembershipLinkType,
				LinkType:   topologymodel.L3SubnetMembershipLinkType,
				Direction:  "observed",
				SrcActorID: "router-b",
				DstActorID: "l3-subnet-segment-203-0-113-0-24",
				Src: topologymodel.LinkEndpoint{
					IfIndex: 402,
					IfName:  "xe-0/0/2",
				},
				Dst: topologymodel.LinkEndpoint{},
				Inference: &graph.LinkInference{
					AttachmentMode: "logical_l3_subnet_membership",
					Inference:      "shared_subnet_membership",
				},
				Detail: topologymodel.LinkDetail{
					L3SubnetMembership: &topologymodel.L3SubnetMembershipLinkDetail{
						Source:  "ip_mib",
						Subnet:  "203.0.113.0/24",
						Network: "203.0.113.0",
						Netmask: "255.255.255.0",
						Prefix:  24,
						Interfaces: []topologymodel.L3SubnetMembershipInterface{
							{MemberIP: "203.0.113.2", IfIndex: 402, IfName: "xe-0/0/2", IfDescr: "Transit VLAN"},
						},
					},
				},
			},
		},
		Stats: topologymodel.Stats{
			Shape: topologymodel.ShapeStats{
				ActorsCollapsedByIP:     1,
				ActorsMapTypeSuppressed: 2,
				MapType:                 topologyoptions.MapTypeHighConfidenceInferred,
			},
			HasShape: true,
			Focus: topologymodel.FocusStats{
				ManagedSNMPDeviceFocus: topologyoptions.ManagedFocusAllDevices,
				Depth:                  topologymodel.FocusDepth{All: true},
			},
			HasFocus: true,
			L3: topologymodel.L3EnrichmentStats{
				SubnetStats: topologymodel.L3SubnetBuildStats{
					CandidateLinks:       1,
					CandidateSegments:    1,
					CandidateMemberships: 2,
				},
				EmittedLinks:           1,
				EmittedSegments:        1,
				EmittedMembershipLinks: 2,
			},
			HasL3: true,
			OSPF: topologymodel.OSPFEnrichmentStats{
				EmittedLinks: 1,
			},
			HasOSPF: true,
			BGP: topologymodel.BGPEnrichmentStats{
				EmittedLinks: 1,
			},
			HasBGP: true,
			Recomputed: topologymodel.RecomputedStats{
				ActorsTotal:                    6,
				LinksTotal:                     7,
				LinksProbable:                  1,
				L3SubnetVisibleLinks:           1,
				L3SubnetMembershipVisibleLinks: 2,
				OSPFAdjacencyVisibleLinks:      1,
				BGPAdjacencyVisibleLinks:       1,
			},
			HasComputed: true,
		},
	}
}
