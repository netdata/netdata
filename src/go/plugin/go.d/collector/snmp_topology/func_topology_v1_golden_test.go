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
				Attributes: map[string]any{
					"capabilities":         []string{"bridge", "router"},
					"cdp_neighbor_count":   uint64(1),
					"chart_id_prefix":      "snmp_switch_a",
					"display_name":         "Switch A",
					"endpoints_total":      uint64(12),
					"fdb_total_macs":       uint64(42),
					"lldp_neighbor_count":  uint64(1),
					"management_ip":        "192.0.2.10",
					"model":                "Catalyst 9300",
					"netdata_host_id":      "host-switch-a",
					"ports_total":          uint64(2),
					"protocols":            []string{"lldp", "cdp", "snmp"},
					"sys_contact":          "noc@example.test",
					"sys_descr":            "Synthetic switch fixture",
					"sys_location":         "lab-rack-1",
					"vendor":               "Cisco",
					"vlan_count":           uint64(3),
					tagOSPFRouterID:        "10.255.0.1",
					"chart_context_prefix": "snmp.switch_a",
				},
				Labels: map[string]string{
					"role": "distribution",
					"site": "lab",
				},
				Tables: map[string][]map[string]any{
					"ports": {
						{
							"admin_status":         "up",
							"duplex":               "full",
							"fdb_mac_count":        uint64(12),
							"if_alias":             "to router-a",
							"if_descr":             "GigabitEthernet1/0/1",
							"if_index":             uint64(101),
							"if_name":              "Gi1/0/1",
							"last_change":          "2026-01-02T01:02:03Z",
							"link_count":           uint64(1),
							"link_mode":            "trunk",
							"link_mode_confidence": "high",
							"link_mode_sources":    []string{"if_mib", "lldp"},
							"mac":                  "aa:bb:cc:00:01:01",
							"neighbor_count":       uint64(1),
							"neighbors": []map[string]any{
								{
									"protocol":          "lldp",
									"remote_chassis_id": "aa:bb:cc:00:00:02",
									"remote_port":       "Gi0/0",
								},
							},
							"oper_status":              "up",
							"port_id":                  "Gi1/0/1",
							"port_type":                "ethernet",
							"speed":                    uint64(1000000000),
							"stp_state":                "forwarding",
							"topology_role":            "uplink",
							"topology_role_confidence": "medium",
							"topology_role_sources":    []string{"stp", "fdb"},
							"vendor_note":              "kept in actor_ports.extra until typed ports replace it",
							"vlan_ids":                 []string{"10", "20"},
							"vlans": []map[string]any{
								{"id": "10", "name": "users"},
								{"id": "20", "name": "servers"},
							},
						},
					},
					"inventory": {
						{
							"enabled":       true,
							"sensor_labels": []any{"ambient", "rack"},
							"slot":          "1",
							"temperature_c": 42.5,
						},
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
				Attributes: map[string]any{
					"capabilities":        []string{"router"},
					"display_name":        "Router A",
					"management_ip":       "198.51.100.1",
					"model":               "ASR 1001",
					"ports_total":         uint64(4),
					"protocols_collected": []string{"ip_mib", "ospf_mib", "bgp_mib"},
					"sys_descr":           "Synthetic router A",
					"vendor":              "Cisco",
					tagOSPFRouterID:       "10.255.0.1",
				},
				Labels: map[string]string{
					"role": "edge",
				},
				Tables: map[string][]map[string]any{
					"bgp_peers": {
						{
							"admin_status":             "enabled",
							"bgp_version":              "4",
							"description":              "router-b transit",
							"established_uptime":       uint64(7200),
							"last_received_update_age": uint64(15),
							"local_as":                 "64512",
							"local_identifier":         "10.255.0.1",
							"local_ip":                 "203.0.113.1",
							"neighbor_ip":              "203.0.113.2",
							"peer_identifier":          "10.255.0.2",
							"peer_type":                "external",
							"remote_actor_id":          "router-b",
							"remote_as":                "64513",
							"routing_instance":         "default",
							"source":                   "bgp_mib",
							"state":                    "established",
						},
					},
					"ospf_neighbors": {
						{
							"addressless_index":  "0",
							"local_ip":           "198.51.100.1",
							"local_router_id":    "10.255.0.1",
							"neighbor_ip":        "198.51.100.2",
							"neighbor_router_id": "10.255.0.2",
							"remote_actor_id":    "router-b",
							"source":             "ospf_mib",
							"state":              "full",
							"subnet":             "198.51.100.0/30",
						},
					},
					"ports": {
						{
							"if_alias":       "to switch-a",
							"if_index":       uint64(201),
							"if_name":        "Gi0/0",
							"link_mode":      "trunk",
							"oper_status":    "up",
							"port_id":        "Gi0/0",
							"topology_role":  "uplink",
							"vendor_context": "router uplink debug field",
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
				Attributes: map[string]any{
					"display_name":  "Router B",
					"management_ip": "198.51.100.2",
					"model":         "MX204",
					"sys_descr":     "Synthetic router B",
					"vendor":        "Juniper",
					tagOSPFRouterID: "10.255.0.2",
				},
				Tables: map[string][]map[string]any{
					"bgp_peers": {
						{
							"admin_status":             "enabled",
							"bgp_version":              "4",
							"description":              "router-a transit",
							"established_uptime":       uint64(7100),
							"last_received_update_age": uint64(20),
							"local_as":                 "64513",
							"local_identifier":         "10.255.0.2",
							"local_ip":                 "203.0.113.2",
							"neighbor_ip":              "203.0.113.1",
							"peer_identifier":          "10.255.0.1",
							"peer_type":                "external",
							"remote_actor_id":          "router-a",
							"remote_as":                "64512",
							"routing_instance":         "default",
							"source":                   "bgp_mib",
							"state":                    "established",
						},
					},
					"ospf_neighbors": {
						{
							"addressless_index":  "0",
							"local_ip":           "198.51.100.2",
							"local_router_id":    "10.255.0.2",
							"neighbor_ip":        "198.51.100.1",
							"neighbor_router_id": "10.255.0.1",
							"remote_actor_id":    "router-a",
							"source":             "ospf_mib",
							"state":              "full",
							"subnet":             "198.51.100.0/30",
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
				Attributes: map[string]any{
					"display_name":    "Server A",
					"learned_sources": []string{"fdb", "arp"},
				},
				Labels: map[string]string{
					"role": "application",
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
				Attributes: map[string]any{
					"display_name": "198.51.100.0/30",
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
					Attributes: map[string]any{
						"if_index":      uint64(101),
						"if_name":       "Gi1/0/1",
						"management_ip": "192.0.2.10",
						"port_id":       "Gi1/0/1",
					},
				},
				Dst: topologyLinkEndpoint{
					Attributes: map[string]any{
						"if_index":      uint64(201),
						"if_name":       "Gi0/0",
						"management_ip": "198.51.100.1",
						"port_id":       "Gi0/0",
					},
				},
				DiscoveredAt: &discoveredAt,
				LastSeen:     &lastSeen,
				Metrics: map[string]any{
					"attachment_mode": "direct",
					"confidence":      "high",
					"inference":       "lldp",
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
					Attributes: map[string]any{
						"if_index": uint64(102),
						"if_name":  "Gi1/0/2",
						"port_id":  "Gi1/0/2",
					},
				},
				Dst: topologyLinkEndpoint{
					Attributes: map[string]any{
						"mac": "00:11:22:33:44:55",
					},
				},
				Metrics: map[string]any{
					"attachment_mode": "probable_host",
					"confidence":      "low",
					"inference":       "probable",
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
					Attributes: map[string]any{
						"if_index": uint64(301),
						"if_name":  "xe-0/0/0",
						"ip":       "198.51.100.1",
						"netmask":  "255.255.255.252",
						"network":  "198.51.100.0",
						"prefix":   uint64(30),
						"source":   "ip_mib",
						"subnet":   "198.51.100.0/30",
					},
				},
				Dst: topologyLinkEndpoint{
					Attributes: map[string]any{
						"if_index": uint64(401),
						"if_name":  "xe-0/0/1",
						"ip":       "198.51.100.2",
						"netmask":  "255.255.255.252",
						"network":  "198.51.100.0",
						"prefix":   uint64(30),
						"source":   "ip_mib",
						"subnet":   "198.51.100.0/30",
					},
				},
				Metrics: map[string]any{
					"attachment_mode": "logical_l3_subnet",
					"inference":       "shared_subnet",
					"netmask":         "255.255.255.252",
					"network":         "198.51.100.0",
					"prefix":          uint64(30),
					"source":          "ip_mib",
					"subnet":          "198.51.100.0/30",
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
				Src: topologyLinkEndpoint{
					Attributes: map[string]any{
						"ip":        "198.51.100.1",
						"router_id": "10.255.0.1",
						"source":    "ospf_mib",
						"subnet":    "198.51.100.0/30",
					},
				},
				Dst: topologyLinkEndpoint{
					Attributes: map[string]any{
						"ip":        "198.51.100.2",
						"router_id": "10.255.0.2",
						"source":    "ospf_mib",
						"subnet":    "198.51.100.0/30",
					},
				},
				Metrics: map[string]any{
					"attachment_mode": "logical_ospf_adjacency",
					"inference":       "ospf_neighbor",
					"netmask":         "255.255.255.252",
					"network":         "198.51.100.0",
					"prefix":          uint64(30),
					"source":          "ospf_mib",
					"subnet":          "198.51.100.0/30",
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
				Src: topologyLinkEndpoint{
					Attributes: map[string]any{
						"as":             "64512",
						"bgp_identifier": "10.255.0.1",
						"ip":             "203.0.113.1",
						"source":         "bgp_mib",
					},
				},
				Dst: topologyLinkEndpoint{
					Attributes: map[string]any{
						"as":             "64513",
						"bgp_identifier": "10.255.0.2",
						"ip":             "203.0.113.2",
						"source":         "bgp_mib",
					},
				},
				Metrics: map[string]any{
					"attachment_mode":  "logical_bgp_adjacency",
					"inference":        "bgp_peer",
					"routing_instance": "default",
					"source":           "bgp_mib",
				},
			},
		},
		Stats: map[string]any{
			"actors_collapsed_by_ip":     uint64(1),
			"actors_map_type_suppressed": uint64(2),
			"actors_total":               uint64(5),
			"bgp_adjacency_visible":      uint64(1),
			"custom_debug_stat":          "kept",
			"depth":                      topologyDepthAll,
			"l3_subnet_visible_links":    uint64(1),
			"links_total":                uint64(5),
			"map_type":                   topologyMapTypeHighConfidenceInferred,
			"ospf_adjacency_visible":     uint64(1),
			"probable_visible_links":     uint64(1),
			"renderer_stats_are_open":    true,
			"suppressed_duplicate_link":  uint64(0),
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
