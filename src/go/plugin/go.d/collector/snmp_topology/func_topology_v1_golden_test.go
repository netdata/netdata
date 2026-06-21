// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/l2topology"
	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
	topologyv1 "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
	"github.com/stretchr/testify/require"
)

var updateSNMPTopologyV1Golden = flag.Bool(
	"update-snmp-topology-v1-golden",
	false,
	"rewrite snmp_topology normalized topology.v1 golden testdata",
)

const snmpTopologyV1GoldenPayloadPath = "testdata/topology_v1_normalized_golden.json"

// Refresh with:
// go test -count=1 ./plugin/go.d/collector/snmp_topology -run TestSNMPTopologyToV1_NormalizedGolden -update-snmp-topology-v1-golden
func TestSNMPTopologyToV1_NormalizedGolden(t *testing.T) {
	data, err := snmpTopologyToV1(snmpTopologyV1GoldenInput())
	require.NoError(t, err)
	require.NoError(t, validateTopologyV1Data(data))

	normalized := normalizeTopologyV1GoldenData(t, data)
	actual := canonicalTopologyV1GoldenJSON(t, normalized)
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

func snmpTopologyV1GoldenInput() topologyData {
	collectedAt := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	discoveredAt := time.Date(2026, time.January, 2, 2, 4, 5, 0, time.UTC)
	lastSeen := time.Date(2026, time.January, 2, 3, 3, 5, 0, time.UTC)

	return topologyData{
		SchemaVersion: topologySchemaVersion,
		Source:        "snmp_topology_test",
		Layer:         "multi",
		AgentID:       "golden-agent",
		CollectedAt:   collectedAt,
		View:          "summary",
		Actors: []topologyActor{
			{
				ActorID:   "switch-a",
				ActorType: "switch",
				Layer:     "2",
				Source:    "snmp",
				Match: topologyMatch{
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
				Detail: topologyActorDetail{
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
					SNMP: topologySNMPActorDetail{
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
				Match: topologyMatch{
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
				Detail: topologyActorDetail{
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
					SNMP: topologySNMPActorDetail{
						Capabilities: []string{"router"},
						ManagementIP: "198.51.100.1",
						Model:        "ASR 1001",
						OSPFRouterID: "10.255.0.1",
						SysDescr:     "Synthetic router A",
						Vendor:       "Cisco",
					},
					BGP: []topologyBGPPeerDetailRow{
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
					OSPF: []topologyOSPFNeighborDetailRow{
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
				Match: topologyMatch{
					ChassisIDs:  []string{"aa:bb:cc:00:00:03"},
					IPAddresses: []string{"198.51.100.2", "203.0.113.2"},
					SysObjectID: "1.3.6.1.4.1.2636.1.1.1.2.131",
					SysName:     "router-b",
				},
				Detail: topologyActorDetail{
					L2: topologyengine.ProjectionActorDetail{
						DisplayName: "Router B",
						Device: topologyengine.ProjectionDeviceActorDetail{
							ManagementIP: "198.51.100.2",
						},
					},
					SNMP: topologySNMPActorDetail{
						ManagementIP: "198.51.100.2",
						Model:        "MX204",
						OSPFRouterID: "10.255.0.2",
						SysDescr:     "Synthetic router B",
						Vendor:       "Juniper",
					},
					BGP: []topologyBGPPeerDetailRow{
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
					OSPF: []topologyOSPFNeighborDetailRow{
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
				Match: topologyMatch{
					MacAddresses: []string{"00:11:22:33:44:55"},
					IPAddresses:  []string{"192.0.2.50"},
					Hostnames:    []string{"server-a"},
				},
				Labels: map[string]string{
					"role": "application",
				},
				Detail: topologyActorDetail{
					L2: topologyengine.ProjectionActorDetail{
						DisplayName: "Server A",
						Endpoint: topologyengine.ProjectionEndpointActorDetail{
							LearnedSources: []string{"fdb", "arp"},
						},
					},
				},
			},
			{
				ActorID:   "segment-198-51-100-0-30",
				ActorType: "segment",
				Layer:     "3",
				Source:    "ip_mib",
				Match: topologyMatch{
					IPAddresses: []string{"198.51.100.0"},
				},
				Detail: topologyActorDetail{
					L2: topologyengine.ProjectionActorDetail{
						DisplayName: "198.51.100.0/30",
					},
				},
			},
		},
		Links: []topologyLink{
			{
				Layer:      "2",
				Protocol:   "lldp",
				LinkType:   "lldp",
				Direction:  "observed",
				State:      "up",
				SrcActorID: "switch-a",
				DstActorID: "router-a",
				Src: topologyLinkEndpoint{
					IfIndex:      101,
					IfName:       "Gi1/0/1",
					ManagementIP: "192.0.2.10",
					PortID:       "Gi1/0/1",
				},
				Dst: topologyLinkEndpoint{
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
				Src: topologyLinkEndpoint{
					IfIndex: 102,
					IfName:  "Gi1/0/2",
					PortID:  "Gi1/0/2",
				},
				Dst: topologyLinkEndpoint{},
				Inference: &graph.LinkInference{
					AttachmentMode: "probable_host",
					Confidence:     "low",
					Inference:      "probable",
				},
			},
			{
				Layer:      "3",
				Protocol:   topologyL3SubnetLinkType,
				LinkType:   topologyL3SubnetLinkType,
				Direction:  "observed",
				SrcActorID: "router-a",
				DstActorID: "router-b",
				Src: topologyLinkEndpoint{
					IfIndex: 301,
					IfName:  "xe-0/0/0",
				},
				Dst: topologyLinkEndpoint{
					IfIndex: 401,
					IfName:  "xe-0/0/1",
				},
				Inference: &graph.LinkInference{
					AttachmentMode: "logical_l3_subnet",
					Inference:      "shared_subnet",
				},
				Detail: topologyLinkDetail{
					L3Subnet: &topologyL3SubnetLinkDetail{
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
				Protocol:   topologyOSPFAdjacencyLinkType,
				LinkType:   topologyOSPFAdjacencyLinkType,
				Direction:  "observed",
				State:      "full",
				SrcActorID: "router-a",
				DstActorID: "router-b",
				Src:        topologyLinkEndpoint{},
				Dst:        topologyLinkEndpoint{},
				Inference: &graph.LinkInference{
					AttachmentMode: "logical_ospf_adjacency",
					Inference:      "ospf_neighbor",
				},
				Detail: topologyLinkDetail{
					OSPF: &topologyOSPFAdjacencyLinkDetail{
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
				Protocol:   topologyBGPAdjacencyLinkType,
				LinkType:   topologyBGPAdjacencyLinkType,
				Direction:  "observed",
				State:      "established",
				SrcActorID: "router-a",
				DstActorID: "router-b",
				Src:        topologyLinkEndpoint{},
				Dst:        topologyLinkEndpoint{},
				Inference: &graph.LinkInference{
					AttachmentMode: "logical_bgp_adjacency",
					Inference:      "bgp_peer",
				},
				Detail: topologyLinkDetail{
					BGP: &topologyBGPAdjacencyLinkDetail{
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
		},
		Stats: topologyStats{
			Shape: topologyShapeStats{
				ActorsCollapsedByIP:     1,
				ActorsMapTypeSuppressed: 2,
				MapType:                 topologyMapTypeHighConfidenceInferred,
			},
			HasShape: true,
			Focus: topologyFocusStats{
				ManagedSNMPDeviceFocus: topologyManagedFocusAllDevices,
				Depth:                  topologyFocusDepth{All: true},
			},
			HasFocus: true,
			L3: topologyL3EnrichmentStats{
				emittedLinks: 1,
			},
			HasL3: true,
			OSPF: topologyOSPFEnrichmentStats{
				emittedLinks: 1,
			},
			HasOSPF: true,
			BGP: topologyBGPEnrichmentStats{
				emittedLinks: 1,
			},
			HasBGP: true,
			Recomputed: topologyRecomputedStats{
				ActorsTotal:               5,
				LinksTotal:                5,
				LinksProbable:             1,
				L3SubnetVisibleLinks:      1,
				OSPFAdjacencyVisibleLinks: 1,
				BGPAdjacencyVisibleLinks:  1,
			},
			HasComputed: true,
		},
	}
}

type normalizedTopologyV1GoldenData struct {
	SchemaVersion string                                   `json:"schema_version"`
	Producer      topologyv1.Producer                      `json:"producer"`
	CollectedAt   string                                   `json:"collected_at"`
	ValidAfter    string                                   `json:"valid_after,omitempty"`
	ValidUntil    string                                   `json:"valid_until,omitempty"`
	View          *topologyv1.View                         `json:"view,omitempty"`
	Types         topologyv1.TypeRegistry                  `json:"types"`
	Presentation  *topologyv1.Presentation                 `json:"presentation,omitempty"`
	Correlation   any                                      `json:"correlation,omitempty"`
	Actors        normalizedTopologyV1GoldenTable          `json:"actors"`
	Links         normalizedTopologyV1GoldenTable          `json:"links"`
	Evidence      map[string]normalizedTopologyV1GoldenRef `json:"evidence,omitempty"`
	Tables        *normalizedTopologyV1GoldenDetailTables  `json:"tables,omitempty"`
	Overlays      any                                      `json:"overlays,omitempty"`
	Stats         any                                      `json:"stats,omitempty"`
	Extensions    any                                      `json:"extensions,omitempty"`
}

type normalizedTopologyV1GoldenDetailTables struct {
	Actor map[string]normalizedTopologyV1GoldenRef `json:"actor,omitempty"`
}

type normalizedTopologyV1GoldenRef struct {
	Type  string                          `json:"type"`
	Table normalizedTopologyV1GoldenTable `json:"table"`
}

type normalizedTopologyV1GoldenTable struct {
	Columns []string         `json:"columns"`
	Rows    []map[string]any `json:"rows"`
}

type topologyV1GoldenRefs struct {
	actors []string
	links  []string
}

func normalizeTopologyV1GoldenData(t *testing.T, data topologyv1.Data) normalizedTopologyV1GoldenData {
	t.Helper()

	refs := topologyV1GoldenRefs{}
	actorRows := topologyV1GoldenTableRows(t, data, data.Actors, refs)
	refs.actors = topologyV1GoldenActorRefs(t, actorRows)
	linkRows := topologyV1GoldenTableRows(t, data, data.Links, refs)
	refs.links = topologyV1GoldenLinkRefs(t, linkRows)

	normalized := normalizedTopologyV1GoldenData{
		SchemaVersion: data.SchemaVersion,
		Producer:      data.Producer,
		CollectedAt:   data.CollectedAt.UTC().Format(time.RFC3339Nano),
		ValidAfter:    topologyV1GoldenTimeString(data.ValidAfter),
		ValidUntil:    topologyV1GoldenTimeString(data.ValidUntil),
		View:          data.View,
		Types:         data.Types,
		Presentation:  data.Presentation,
		Actors: normalizedTopologyV1GoldenTable{
			Columns: topologyV1GoldenColumnSummaries(data.Actors.Columns),
			Rows:    topologyV1GoldenSortedRows(actorRows),
		},
		Links: normalizedTopologyV1GoldenTable{
			Columns: topologyV1GoldenColumnSummaries(data.Links.Columns),
			Rows:    topologyV1GoldenSortedRows(linkRows),
		},
		Stats: topologyV1GoldenNormalizeValue(t, data.Stats),
	}
	if data.Correlation != nil {
		normalized.Correlation = topologyV1GoldenNormalizeValue(t, data.Correlation)
	}
	if data.Overlays != nil {
		normalized.Overlays = topologyV1GoldenNormalizeValue(t, data.Overlays)
	}
	if len(data.Extensions) > 0 {
		normalized.Extensions = topologyV1GoldenNormalizeValue(t, data.Extensions)
	}

	if len(data.Evidence) > 0 {
		normalized.Evidence = make(map[string]normalizedTopologyV1GoldenRef, len(data.Evidence))
		for key, section := range data.Evidence {
			normalized.Evidence[key] = normalizedTopologyV1GoldenRef{
				Type:  section.Type,
				Table: topologyV1GoldenNormalizeTable(t, data, section.Table, refs),
			}
		}
	}

	if data.Tables != nil && len(data.Tables.Actor) > 0 {
		normalized.Tables = &normalizedTopologyV1GoldenDetailTables{
			Actor: make(map[string]normalizedTopologyV1GoldenRef, len(data.Tables.Actor)),
		}
		for key, detail := range data.Tables.Actor {
			normalized.Tables.Actor[key] = normalizedTopologyV1GoldenRef{
				Type:  detail.Type,
				Table: topologyV1GoldenNormalizeTable(t, data, detail.Table, refs),
			}
		}
	}

	return normalized
}

func topologyV1GoldenNormalizeTable(
	t *testing.T,
	data topologyv1.Data,
	table topologyv1.Table,
	refs topologyV1GoldenRefs,
) normalizedTopologyV1GoldenTable {
	t.Helper()

	return normalizedTopologyV1GoldenTable{
		Columns: topologyV1GoldenColumnSummaries(table.Columns),
		Rows:    topologyV1GoldenSortedRows(topologyV1GoldenTableRows(t, data, table, refs)),
	}
}

func topologyV1GoldenTableRows(
	t *testing.T,
	data topologyv1.Data,
	table topologyv1.Table,
	refs topologyV1GoldenRefs,
) []map[string]any {
	t.Helper()

	rows := make([]map[string]any, table.Rows)
	for row := range rows {
		rows[row] = make(map[string]any, len(table.Columns))
	}
	for columnIndex, column := range table.Columns {
		values := topologyV1DecodeColumnValues(t, table, columnIndex)
		require.Len(t, values, table.Rows)
		for rowIndex, value := range values {
			decoded := topologyV1GoldenDecodeValue(t, data, column, value, refs)
			if decoded != nil {
				rows[rowIndex][column.ID] = decoded
			}
		}
	}
	return rows
}

func topologyV1GoldenColumnSummaries(columns []topologyv1.Column) []string {
	out := make([]string, len(columns))
	for i, column := range columns {
		summary := column.ID + ":" + column.Type
		if column.Dictionary != "" {
			summary += ":dict=" + column.Dictionary
		}
		if column.Nullable {
			summary += ":nullable"
		}
		if column.Role != "" {
			summary += ":role=" + column.Role
		}
		if column.Aggregation != "" {
			summary += ":aggregation=" + column.Aggregation
		}
		if column.Unit != "" {
			summary += ":unit=" + column.Unit
		}
		out[i] = summary
	}
	return out
}

func topologyV1GoldenDecodeValue(
	t *testing.T,
	data topologyv1.Data,
	column topologyv1.Column,
	value any,
	refs topologyV1GoldenRefs,
) any {
	t.Helper()

	if value == nil {
		return nil
	}

	switch column.Type {
	case "string_ref", "ip_ref", "mac_ref":
		return topologyV1GoldenDictionaryValue(t, data, column, value)
	case "actor_ref":
		return topologyV1GoldenRefValue(t, refs.actors, value, "actor")
	case "link_ref":
		return topologyV1GoldenRefValue(t, refs.links, value, "link")
	default:
		return topologyV1GoldenNormalizeValue(t, value)
	}
}

func topologyV1GoldenDictionaryValue(t *testing.T, data topologyv1.Data, column topologyv1.Column, value any) any {
	t.Helper()

	ref := topologyV1GoldenIntValue(t, value)
	dict := data.Dictionaries[column.Dictionary]
	require.NotNilf(t, dict, "missing dictionary %q for column %q", column.Dictionary, column.ID)
	require.GreaterOrEqual(t, ref, 0)
	require.Less(t, ref, len(dict))
	return topologyV1GoldenNormalizeValue(t, dict[ref])
}

func topologyV1GoldenRefValue(t *testing.T, values []string, value any, kind string) any {
	t.Helper()

	ref := topologyV1GoldenIntValue(t, value)
	require.GreaterOrEqual(t, ref, 0)
	require.Lessf(t, ref, len(values), "%s ref %d out of bounds", kind, ref)
	return values[ref]
}

func topologyV1GoldenIntValue(t *testing.T, value any) int {
	t.Helper()

	switch v := value.(type) {
	case int:
		return v
	case int8:
		return int(v)
	case int16:
		return int(v)
	case int32:
		return int(v)
	case int64:
		return int(v)
	case uint:
		return int(v)
	case uint8:
		return int(v)
	case uint16:
		return int(v)
	case uint32:
		return int(v)
	case uint64:
		return int(v)
	case float64:
		return int(v)
	default:
		require.Failf(t, "invalid reference value", "got %T", value)
		return 0
	}
}

func topologyV1GoldenActorRefs(t *testing.T, rows []map[string]any) []string {
	t.Helper()

	refs := make([]string, len(rows))
	for i, row := range rows {
		id, ok := row["id"].(string)
		require.Truef(t, ok && id != "", "actor row %d has invalid id %v", i, row["id"])
		refs[i] = id
	}
	return refs
}

func topologyV1GoldenLinkRefs(t *testing.T, rows []map[string]any) []string {
	t.Helper()

	refs := make([]string, len(rows))
	seen := make(map[string]int, len(rows))
	for i, row := range rows {
		key := fmt.Sprintf("%v -> %v | %v | %v | %v | %v | %v",
			row["src_actor"],
			row["dst_actor"],
			row["type"],
			row["protocol"],
			row["state"],
			row["src_port_name"],
			row["dst_port_name"],
		)
		seen[key]++
		if seen[key] > 1 {
			key = fmt.Sprintf("%s #%d", key, seen[key])
		}
		refs[i] = key
	}
	return refs
}

func topologyV1GoldenSortedRows(rows []map[string]any) []map[string]any {
	out := append([]map[string]any(nil), rows...)
	sort.Slice(out, func(i, j int) bool {
		return topologyV1GoldenSortKey(out[i]) < topologyV1GoldenSortKey(out[j])
	})
	return out
}

func topologyV1GoldenSortKey(row map[string]any) string {
	bs, err := json.Marshal(row)
	if err != nil {
		panic(err)
	}
	return string(bs)
}

func topologyV1GoldenNormalizeValue(t *testing.T, value any) any {
	t.Helper()

	if value == nil {
		return nil
	}
	bs, err := json.Marshal(value)
	require.NoError(t, err)
	var normalized any
	require.NoError(t, json.Unmarshal(bs, &normalized))
	return normalized
}

func canonicalTopologyV1GoldenJSON(t *testing.T, value any) string {
	t.Helper()

	bs, err := json.Marshal(value)
	require.NoError(t, err)
	var normalized any
	require.NoError(t, json.Unmarshal(bs, &normalized))

	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetIndent("", "  ")
	require.NoError(t, enc.Encode(normalized))
	return out.String()
}

func topologyV1GoldenTimeString(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}
