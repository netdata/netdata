// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/topology"
	"github.com/stretchr/testify/require"
)

func TestToTopologyData_ProjectsResult(t *testing.T) {
	collectedAt := time.Date(2026, time.February, 20, 4, 5, 6, 0, time.UTC)

	result := Result{
		CollectedAt: collectedAt,
		Devices: []Device{
			{
				ID:        "local-device",
				Hostname:  "sw1",
				ChassisID: "00:11:22:33:44:55",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.1")},
				Labels:    map[string]string{"protocols_observed": "bridge,fdb,stp"},
			},
			{
				ID:        "remote-device",
				Hostname:  "sw2",
				ChassisID: "aa:bb:cc:dd:ee:ff",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.2")},
				Labels:    map[string]string{"inferred": "true"},
			},
		},
		Interfaces: []Interface{
			{DeviceID: "local-device", IfIndex: 3, IfName: "Gi0/3", IfDescr: "Gi0/3", Labels: map[string]string{"admin_status": "up", "oper_status": "up"}},
			{DeviceID: "local-device", IfIndex: 4, IfName: "Gi0/4", IfDescr: "Gi0/4", Labels: map[string]string{"admin_status": "up", "oper_status": "lowerLayerDown"}},
		},
		Adjacencies: []Adjacency{
			{
				Protocol:   "lldp",
				SourceID:   "local-device",
				SourcePort: "Gi0/3",
				TargetID:   "remote-device",
				TargetPort: "Gi0/1",
			},
		},
		Attachments: []Attachment{
			{DeviceID: "local-device", IfIndex: 4, EndpointID: "mac:70:49:a2:65:72:cd", Method: "fdb"},
		},
		Enrichments: []Enrichment{
			{
				EndpointID: "mac:70:49:a2:65:72:cd",
				MAC:        "70:49:a2:65:72:cd",
				IPs:        []netip.Addr{netip.MustParseAddr("10.20.4.84")},
				Labels: map[string]string{
					"sources":    "arp",
					"if_indexes": "4",
					"if_names":   "Gi0/4",
				},
			},
		},
	}

	data := ToTopologyData(result, TopologyDataOptions{
		SchemaVersion: "2.0",
		Source:        "snmp",
		Layer:         "2",
		View:          "summary",
		AgentID:       "agent-1",
		LocalDeviceID: "local-device",
	})

	require.Equal(t, "2.0", data.SchemaVersion)
	require.Equal(t, "snmp", data.Source)
	require.Equal(t, "2", data.Layer)
	require.Equal(t, "agent-1", data.AgentID)
	require.Equal(t, collectedAt, data.CollectedAt)

	require.Len(t, data.Actors, 3)
	require.Len(t, data.Links, 2)
	lldpLink := findLinkByProtocol(data.Links, "lldp")
	require.NotNil(t, lldpLink)
	require.Equal(t, "Gi0/3", lldpLink.Src.Attributes["if_name"])
	require.Equal(t, "Gi0/3", lldpLink.Src.Attributes["port_id"])
	require.Equal(t, "up", lldpLink.Src.Attributes["if_admin_status"])
	require.Equal(t, "up", lldpLink.Src.Attributes["if_oper_status"])
	require.Equal(t, "sw2", lldpLink.Dst.Attributes["sys_name"])

	localActor := findActorBySysName(data.Actors, "sw1")
	require.NotNil(t, localActor)
	require.Equal(t, false, localActor.Attributes["discovered"])
	require.Equal(t, false, localActor.Attributes["inferred"])
	require.Equal(t, []string{"bridge", "fdb", "stp"}, localActor.Attributes["protocols"])
	require.Equal(t, []string{"bridge", "fdb", "stp"}, localActor.Attributes["protocols_collected"])
	require.Equal(t, 2, localActor.Attributes["ports_total"])
	require.NotNil(t, localActor.Attributes["if_admin_status_counts"])
	require.NotNil(t, localActor.Attributes["if_oper_status_counts"])
	require.NotNil(t, localActor.Attributes["if_link_mode_counts"])
	require.NotNil(t, localActor.Attributes["if_topology_role_counts"])
	require.NotNil(t, localActor.Attributes["if_statuses"])
	remoteActor := findActorBySysName(data.Actors, "sw2")
	require.NotNil(t, remoteActor)
	require.Equal(t, true, remoteActor.Attributes["inferred"])

	endpointActor := findActorByMAC(data.Actors, "70:49:a2:65:72:cd")
	require.NotNil(t, endpointActor)
	require.Equal(t, "endpoint", endpointActor.ActorType)
	require.Equal(t, []string{"10.20.4.84"}, endpointActor.Match.IPAddresses)
	require.Equal(t, []string{"arp", "fdb"}, endpointActor.Attributes["learned_sources"])
	require.Equal(t, "single_port_mac", endpointActor.Attributes["attachment_source"])
	require.Equal(t, "sw1", endpointActor.Attributes["attached_device"])
	require.Equal(t, "Gi0/4", endpointActor.Attributes["attached_port"])

	require.Equal(t, 2, data.Stats["devices_total"])
	require.Equal(t, 1, data.Stats["devices_discovered"])
	require.Equal(t, 2, data.Stats["links_total"])
	require.Equal(t, 1, data.Stats["links_lldp"])
	require.Equal(t, 0, data.Stats["links_cdp"])
	require.Equal(t, 1, data.Stats["links_fdb"])
	require.Equal(t, 0, data.Stats["links_arp"])
	require.Equal(t, 1, data.Stats["links_bidirectional"])
	require.Equal(t, 1, data.Stats["links_unidirectional"])
	require.Equal(t, 3, data.Stats["actors_total"])
	require.Equal(t, 1, data.Stats["endpoints_total"])
}

func TestToTopologyData_ClassifiesPortLinkModesFromFDBAndSTPEvidence(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:        "sw1",
				Hostname:  "sw1",
				ChassisID: "00:11:22:33:44:55",
			},
			{
				ID:        "sw2",
				Hostname:  "sw2",
				ChassisID: "00:11:22:33:44:66",
			},
		},
		Interfaces: []Interface{
			{DeviceID: "sw1", IfIndex: 1, IfName: "Gi0/1", IfDescr: "Gi0/1"},
			{DeviceID: "sw1", IfIndex: 2, IfName: "Gi0/2", IfDescr: "Gi0/2"},
			{DeviceID: "sw1", IfIndex: 3, IfName: "Gi0/3", IfDescr: "Gi0/3"},
			{DeviceID: "sw1", IfIndex: 4, IfName: "Gi0/4", IfDescr: "Gi0/4"},
			{DeviceID: "sw2", IfIndex: 1, IfName: "Gi0/1", IfDescr: "Gi0/1"},
		},
		Adjacencies: []Adjacency{
			{
				Protocol:   "lldp",
				SourceID:   "sw1",
				SourcePort: "Gi0/3",
				TargetID:   "sw2",
				TargetPort: "Gi0/1",
			},
			{
				Protocol:   "stp",
				SourceID:   "sw1",
				SourcePort: "Gi0/1",
				TargetID:   "sw2",
				TargetPort: "Gi0/1",
				Labels: map[string]string{
					"vlan_id": "20",
				},
			},
		},
		Attachments: []Attachment{
			{
				DeviceID:   "sw1",
				IfIndex:    1,
				EndpointID: "mac:00:00:00:00:10:01",
				Method:     "fdb",
				Labels: map[string]string{
					"fdb_status": "learned",
					"vlan_id":    "10",
				},
			},
			{
				DeviceID:   "sw1",
				IfIndex:    1,
				EndpointID: "mac:00:00:00:00:20:01",
				Method:     "fdb",
				Labels: map[string]string{
					"fdb_status": "learned",
					"vlan_id":    "20",
				},
			},
			{
				DeviceID:   "sw1",
				IfIndex:    2,
				EndpointID: "mac:00:00:00:00:30:01",
				Method:     "fdb",
				Labels: map[string]string{
					"fdb_status": "learned",
					"vlan_id":    "30",
				},
			},
			{
				DeviceID:   "sw1",
				IfIndex:    3,
				EndpointID: "mac:00:00:00:00:40:01",
				Method:     "fdb",
				Labels: map[string]string{
					"fdb_status": "learned",
					"vlan_id":    "40",
				},
			},
			{
				DeviceID:   "sw1",
				IfIndex:    4,
				EndpointID: "mac:00:00:00:00:50:01",
				Method:     "fdb",
				Labels: map[string]string{
					"fdb_status": "learned",
				},
			},
			{
				DeviceID:   "sw1",
				IfIndex:    4,
				EndpointID: "mac:00:00:00:00:50:02",
				Method:     "fdb",
				Labels: map[string]string{
					"fdb_status": "learned",
				},
			},
		},
	}

	data := ToTopologyData(result, TopologyDataOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})

	actor := findActorBySysName(data.Actors, "sw1")
	require.NotNil(t, actor)

	modeCounts, ok := actor.Attributes["if_link_mode_counts"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, 1, modeCounts["trunk"])
	require.Equal(t, 1, modeCounts["access"])
	require.Equal(t, 2, modeCounts["unknown"])

	roleCounts, ok := actor.Attributes["if_topology_role_counts"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, 1, roleCounts["switch_facing"])
	require.Equal(t, 1, roleCounts["host_facing"])
	require.Equal(t, 1, roleCounts["host_candidate"])
	require.Equal(t, 1, roleCounts["unknown"])

	statuses, ok := actor.Attributes["if_statuses"].([]map[string]any)
	require.True(t, ok)

	port1 := findInterfaceStatusByIndex(statuses, 1)
	require.Equal(t, "trunk", port1["link_mode"])
	require.Equal(t, "high", port1["link_mode_confidence"])
	require.Equal(t, []string{"fdb", "stp"}, port1["link_mode_sources"])
	require.Equal(t, []string{"10", "20"}, port1["vlan_ids"])
	require.Equal(t, "unknown", port1["topology_role"])
	require.Equal(t, "low", port1["topology_role_confidence"])
	require.Equal(t, []string{"stp", "fdb"}, port1["topology_role_sources"])

	port2 := findInterfaceStatusByIndex(statuses, 2)
	require.Equal(t, "access", port2["link_mode"])
	require.Equal(t, "medium", port2["link_mode_confidence"])
	require.Equal(t, []string{"fdb"}, port2["link_mode_sources"])
	require.Equal(t, []string{"30"}, port2["vlan_ids"])
	require.Equal(t, "host_facing", port2["topology_role"])
	require.Equal(t, "medium", port2["topology_role_confidence"])
	require.Equal(t, []string{"fdb"}, port2["topology_role_sources"])

	port3 := findInterfaceStatusByIndex(statuses, 3)
	require.Equal(t, "unknown", port3["link_mode"])
	require.Equal(t, "low", port3["link_mode_confidence"])
	require.Equal(t, []string{"fdb", "peer_link"}, port3["link_mode_sources"])
	require.Equal(t, []string{"40"}, port3["vlan_ids"])
	require.Equal(t, "switch_facing", port3["topology_role"])
	require.Equal(t, "high", port3["topology_role_confidence"])
	require.Equal(t, []string{"peer_link", "bridge_link", "fdb"}, port3["topology_role_sources"])

	port4 := findInterfaceStatusByIndex(statuses, 4)
	require.Equal(t, "unknown", port4["link_mode"])
	require.Equal(t, "low", port4["link_mode_confidence"])
	require.Equal(t, []string{"fdb"}, port4["link_mode_sources"])
	_, hasVLANs := port4["vlan_ids"]
	require.False(t, hasVLANs)
	require.Equal(t, "host_candidate", port4["topology_role"])
	require.Equal(t, "low", port4["topology_role_confidence"])
	require.Equal(t, []string{"fdb"}, port4["topology_role_sources"])
}

func TestToTopologyData_IgnoresIgnoredFDBStatusForLinkModeClassification(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:       "sw1",
				Hostname: "sw1",
			},
		},
		Interfaces: []Interface{
			{DeviceID: "sw1", IfIndex: 10, IfName: "Gi0/10", IfDescr: "Gi0/10"},
		},
		Attachments: []Attachment{
			{
				DeviceID:   "sw1",
				IfIndex:    10,
				EndpointID: "mac:00:00:00:00:50:01",
				Method:     "fdb",
				Labels: map[string]string{
					"fdb_status": "ignored",
					"vlan_id":    "50",
				},
			},
		},
	}

	data := ToTopologyData(result, TopologyDataOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})

	actor := findActorBySysName(data.Actors, "sw1")
	require.NotNil(t, actor)
	statuses, ok := actor.Attributes["if_statuses"].([]map[string]any)
	require.True(t, ok)
	port := findInterfaceStatusByIndex(statuses, 10)
	require.Equal(t, "unknown", port["link_mode"])
	require.Equal(t, "low", port["link_mode_confidence"])
	_, hasSources := port["link_mode_sources"]
	require.False(t, hasSources)
	_, hasVLANs := port["vlan_ids"]
	require.False(t, hasVLANs)
	require.Equal(t, "unknown", port["topology_role"])
	require.Equal(t, "low", port["topology_role_confidence"])
	_, hasRoleSources := port["topology_role_sources"]
	require.False(t, hasRoleSources)
}

func TestToTopologyData_ClassifiesSTPCorroboratedManagedAliasAsSwitchFacing(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:        "sw1",
				Hostname:  "sw1",
				ChassisID: "00:11:22:33:44:55",
			},
			{
				ID:        "sw2",
				Hostname:  "sw2",
				ChassisID: "00:11:22:33:44:66",
			},
		},
		Interfaces: []Interface{
			{DeviceID: "sw1", IfIndex: 1, IfName: "Gi0/1", IfDescr: "Gi0/1"},
		},
		Adjacencies: []Adjacency{
			{
				Protocol:   "stp",
				SourceID:   "sw1",
				SourcePort: "Gi0/1",
				TargetID:   "stp-root",
				Labels: map[string]string{
					"vlan_id": "10",
				},
			},
		},
		Attachments: []Attachment{
			{
				DeviceID:   "sw1",
				IfIndex:    1,
				EndpointID: "mac:00:11:22:33:44:66",
				Method:     "fdb",
				Labels: map[string]string{
					"fdb_status": "learned",
					"vlan_id":    "10",
				},
			},
		},
	}

	data := ToTopologyData(result, TopologyDataOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})

	actor := findActorBySysName(data.Actors, "sw1")
	require.NotNil(t, actor)

	statuses, ok := actor.Attributes["if_statuses"].([]map[string]any)
	require.True(t, ok)
	port1 := findInterfaceStatusByIndex(statuses, 1)
	require.Equal(t, "switch_facing", port1["topology_role"])
	require.Equal(t, "medium", port1["topology_role_confidence"])
	require.Equal(t, []string{"stp", "fdb", "fdb_managed_alias"}, port1["topology_role_sources"])
}

func TestToTopologyData_EnrichesPortStatusesWithNeighborsFDBAndSTP(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:        "sw1",
				Hostname:  "sw1",
				ChassisID: "00:11:22:33:44:55",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.1")},
			},
			{
				ID:        "sw2",
				Hostname:  "sw2",
				ChassisID: "aa:bb:cc:dd:ee:ff",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.2")},
				Labels: map[string]string{
					"capabilities_enabled": "bridge,router",
				},
			},
		},
		Interfaces: []Interface{
			{
				DeviceID: "sw1",
				IfIndex:  1,
				IfName:   "Gi0/1",
				IfDescr:  "Gi0/1",
				MAC:      "00:11:22:33:44:55",
				Labels: map[string]string{
					"admin_status": "up",
					"oper_status":  "up",
					"if_alias":     "uplink-core",
					"speed_bps":    "1000000000",
					"last_change":  "12345",
					"duplex":       "full",
				},
			},
			{
				DeviceID: "sw1",
				IfIndex:  2,
				IfName:   "Gi0/2",
				IfDescr:  "Gi0/2",
				MAC:      "00:11:22:33:44:66",
				Labels: map[string]string{
					"admin_status": "down",
					"oper_status":  "down",
					"if_alias":     "server-a",
					"speed_bps":    "100000000",
					"last_change":  "54321",
					"duplex":       "half",
				},
			},
		},
		Adjacencies: []Adjacency{
			{
				Protocol:   "lldp",
				SourceID:   "sw1",
				SourcePort: "Gi0/1",
				TargetID:   "sw2",
				TargetPort: "Gi0/24",
			},
			{
				Protocol:   "cdp",
				SourceID:   "sw1",
				SourcePort: "Gi0/1",
				TargetID:   "sw2",
				TargetPort: "Gi0/24",
			},
			{
				Protocol:   "stp",
				SourceID:   "sw1",
				SourcePort: "Gi0/1",
				TargetID:   "sw2",
				TargetPort: "Gi0/24",
				Labels: map[string]string{
					"stp_state": "forwarding",
					"vlan_id":   "10",
				},
			},
			{
				Protocol:   "stp",
				SourceID:   "sw1",
				SourcePort: "Gi0/1",
				TargetID:   "sw2",
				TargetPort: "Gi0/24",
				Labels: map[string]string{
					"stp_state": "blocking",
					"vlan_id":   "20",
				},
			},
		},
		Attachments: []Attachment{
			{
				DeviceID:   "sw1",
				IfIndex:    1,
				EndpointID: "mac:00:00:00:00:10:01",
				Method:     "fdb",
				Labels: map[string]string{
					"fdb_status": "learned",
					"vlan_id":    "10",
				},
			},
			{
				DeviceID:   "sw1",
				IfIndex:    1,
				EndpointID: "mac:00:00:00:00:20:01",
				Method:     "fdb",
				Labels: map[string]string{
					"fdb_status": "learned",
					"vlan_id":    "20",
				},
			},
			{
				DeviceID:   "sw1",
				IfIndex:    2,
				EndpointID: "mac:00:00:00:00:30:01",
				Method:     "fdb",
				Labels: map[string]string{
					"fdb_status": "learned",
				},
			},
		},
	}

	data := ToTopologyData(result, TopologyDataOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})

	actor := findActorBySysName(data.Actors, "sw1")
	require.NotNil(t, actor)
	require.Equal(t, 1, actor.Attributes["ports_up"])
	require.Equal(t, 1, actor.Attributes["ports_down"])
	require.Equal(t, 1, actor.Attributes["ports_admin_down"])
	require.EqualValues(t, 1_000_000_000, actor.Attributes["total_bandwidth_bps"])
	require.Equal(t, 3, actor.Attributes["fdb_total_macs"])
	require.Equal(t, 2, actor.Attributes["vlan_count"])
	require.Equal(t, 1, actor.Attributes["lldp_neighbor_count"])
	require.Equal(t, 1, actor.Attributes["cdp_neighbor_count"])

	statuses, ok := actor.Attributes["if_statuses"].([]map[string]any)
	require.True(t, ok)

	port1 := findInterfaceStatusByIndex(statuses, 1)
	require.Equal(t, "Gi0/1", port1["if_descr"])
	require.Equal(t, "uplink-core", port1["if_alias"])
	require.Equal(t, "00:11:22:33:44:55", port1["mac"])
	require.EqualValues(t, 1_000_000_000, port1["speed"])
	require.EqualValues(t, 12345, port1["last_change"])
	require.Equal(t, "full", port1["duplex"])
	require.Equal(t, 2, port1["fdb_mac_count"])
	require.Equal(t, "blocking", port1["stp_state"])
	vlans, ok := port1["vlans"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, vlans, 2)
	require.Equal(t, "10", vlans[0]["vlan_id"])
	require.Equal(t, true, vlans[0]["tagged"])
	require.Equal(t, "20", vlans[1]["vlan_id"])
	require.Equal(t, true, vlans[1]["tagged"])
	neighbors, ok := port1["neighbors"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, neighbors, 2)

	cdpNeighbor := findNeighborByProtocol(neighbors, "cdp")
	require.NotNil(t, cdpNeighbor)
	require.Equal(t, "sw2", cdpNeighbor["remote_device"])
	require.Equal(t, "Gi0/24", cdpNeighbor["remote_port"])
	require.Equal(t, "10.0.0.2", cdpNeighbor["remote_ip"])
	require.Equal(t, "aa:bb:cc:dd:ee:ff", cdpNeighbor["remote_chassis_id"])
	require.Equal(t, []string{"bridge", "router"}, cdpNeighbor["remote_capabilities"])

	lldpNeighbor := findNeighborByProtocol(neighbors, "lldp")
	require.NotNil(t, lldpNeighbor)
	require.Equal(t, "sw2", lldpNeighbor["remote_device"])
	require.Equal(t, "Gi0/24", lldpNeighbor["remote_port"])
	require.Equal(t, "10.0.0.2", lldpNeighbor["remote_ip"])
	require.Equal(t, "aa:bb:cc:dd:ee:ff", lldpNeighbor["remote_chassis_id"])
	require.Equal(t, []string{"bridge", "router"}, lldpNeighbor["remote_capabilities"])

	port2 := findInterfaceStatusByIndex(statuses, 2)
	require.Equal(t, "server-a", port2["if_alias"])
	require.Equal(t, "00:11:22:33:44:66", port2["mac"])
	require.EqualValues(t, 100_000_000, port2["speed"])
	require.EqualValues(t, 54321, port2["last_change"])
	require.Equal(t, "half", port2["duplex"])
	require.Equal(t, 1, port2["fdb_mac_count"])
	_, hasNeighbors := port2["neighbors"]
	require.False(t, hasNeighbors)
}

func TestToTopologyData_InfersVendorFromMACOUI(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:        "sw1",
				Hostname:  "sw1",
				ChassisID: "00:11:22:33:44:55",
			},
			{
				ID:        "remote-device",
				Hostname:  "edge-remote",
				ChassisID: "28:6f:b9:00:00:22",
				Labels:    map[string]string{"inferred": "true"},
			},
		},
		Interfaces: []Interface{
			{DeviceID: "sw1", IfIndex: 1, IfName: "Gi0/1", IfDescr: "Gi0/1"},
		},
		Attachments: []Attachment{
			{DeviceID: "sw1", IfIndex: 1, EndpointID: "mac:08:ea:44:11:22:33", Method: "fdb"},
		},
		Enrichments: []Enrichment{
			{EndpointID: "mac:08:ea:44:11:22:33", MAC: "08:ea:44:11:22:33"},
		},
	}

	data := ToTopologyData(result, TopologyDataOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})

	remote := findActorBySysName(data.Actors, "edge-remote")
	require.NotNil(t, remote)
	require.Equal(t, "Nokia Shanghai Bell Co., Ltd.", remote.Attributes["vendor"])
	require.Equal(t, "mac_oui", remote.Attributes["vendor_source"])
	require.Equal(t, "low", remote.Attributes["vendor_confidence"])
	require.Equal(t, "Nokia Shanghai Bell Co., Ltd.", remote.Attributes["vendor_derived"])
	require.Equal(t, "mac_oui", remote.Attributes["vendor_derived_source"])
	require.Equal(t, "low", remote.Attributes["vendor_derived_confidence"])
	require.NotEmpty(t, remote.Attributes["vendor_derived_match_prefix"])

	endpoint := findActorByMAC(data.Actors, "08:ea:44:11:22:33")
	require.NotNil(t, endpoint)
	require.Equal(t, "endpoint", endpoint.ActorType)
	require.Equal(t, "Extreme Networks Headquarters", endpoint.Attributes["vendor"])
	require.Equal(t, "mac_oui", endpoint.Attributes["vendor_source"])
	require.Equal(t, "low", endpoint.Attributes["vendor_confidence"])
	require.Equal(t, "Extreme Networks Headquarters", endpoint.Attributes["vendor_derived"])
	require.Equal(t, "mac_oui", endpoint.Attributes["vendor_derived_source"])
	require.Equal(t, "low", endpoint.Attributes["vendor_derived_confidence"])
	require.NotEmpty(t, endpoint.Attributes["vendor_derived_match_prefix"])
}

func TestDeviceToTopologyActor_DoesNotOverrideExplicitVendor(t *testing.T) {
	actor := deviceToTopologyActor(
		Device{
			ID:        "switch-a",
			Hostname:  "switch-a",
			ChassisID: "08:ea:44:99:88:77",
			Labels: map[string]string{
				"vendor": "Explicit Vendor",
			},
		},
		"snmp",
		"2",
		"",
		topologyDeviceInterfaceSummary{},
		nil,
	)

	require.Equal(t, "Explicit Vendor", actor.Attributes["vendor"])
	require.Equal(t, "labels", actor.Attributes["vendor_source"])
	require.Equal(t, "high", actor.Attributes["vendor_confidence"])
	require.Equal(t, "Extreme Networks Headquarters", actor.Attributes["vendor_derived"])
	require.Equal(t, "mac_oui", actor.Attributes["vendor_derived_source"])
	require.Equal(t, "low", actor.Attributes["vendor_derived_confidence"])
	require.NotEmpty(t, actor.Attributes["vendor_derived_match_prefix"])
}

func TestToTopologyData_DefaultDiscoveredCountWithoutLocalID(t *testing.T) {
	result := Result{
		Devices: []Device{
			{ID: "a", Hostname: "a"},
			{ID: "b", Hostname: "b"},
			{ID: "c", Hostname: "c"},
		},
	}

	data := ToTopologyData(result, TopologyDataOptions{})
	require.Equal(t, 2, data.Stats["devices_discovered"])
}

func TestToTopologyData_AssignsDeterministicActorIDsAndLinkActorIDs(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:        "sw1",
				Hostname:  "sw1",
				ChassisID: "00:11:22:33:44:55",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.1")},
			},
			{
				ID:        "sw2",
				Hostname:  "sw2",
				ChassisID: "00:11:22:33:44:66",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.2")},
			},
		},
		Adjacencies: []Adjacency{
			{
				Protocol:   "lldp",
				SourceID:   "sw1",
				SourcePort: "Gi0/1",
				TargetID:   "sw2",
				TargetPort: "Gi0/2",
			},
		},
	}

	data := ToTopologyData(result, TopologyDataOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})

	require.Len(t, data.Actors, 2)
	actorIDs := make(map[string]struct{}, len(data.Actors))
	for _, actor := range data.Actors {
		require.NotEmpty(t, actor.ActorID)
		_, exists := actorIDs[actor.ActorID]
		require.False(t, exists, "duplicate actor_id %q", actor.ActorID)
		actorIDs[actor.ActorID] = struct{}{}
	}

	require.Len(t, data.Links, 1)
	require.NotEmpty(t, data.Links[0].SrcActorID)
	require.NotEmpty(t, data.Links[0].DstActorID)
	_, srcExists := actorIDs[data.Links[0].SrcActorID]
	_, dstExists := actorIDs[data.Links[0].DstActorID]
	require.True(t, srcExists)
	require.True(t, dstExists)
}

func TestToTopologyData_DeterministicAcrossRepeatedCalls(t *testing.T) {
	collectedAt := time.Date(2026, time.February, 20, 4, 5, 6, 0, time.UTC)

	result := Result{
		CollectedAt: collectedAt,
		Devices: []Device{
			{
				ID:        "local-device",
				Hostname:  "sw1",
				ChassisID: "00:11:22:33:44:55",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.1")},
				Labels:    map[string]string{"protocols_observed": "bridge,fdb,stp"},
			},
			{
				ID:        "remote-device",
				Hostname:  "sw2",
				ChassisID: "aa:bb:cc:dd:ee:ff",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.2")},
				Labels:    map[string]string{"inferred": "true"},
			},
		},
		Interfaces: []Interface{
			{DeviceID: "local-device", IfIndex: 3, IfName: "Gi0/3", IfDescr: "Gi0/3", Labels: map[string]string{"admin_status": "up", "oper_status": "up"}},
			{DeviceID: "local-device", IfIndex: 4, IfName: "Gi0/4", IfDescr: "Gi0/4", Labels: map[string]string{"admin_status": "up", "oper_status": "lowerLayerDown"}},
		},
		Adjacencies: []Adjacency{
			{
				Protocol:   "lldp",
				SourceID:   "local-device",
				SourcePort: "Gi0/3",
				TargetID:   "remote-device",
				TargetPort: "Gi0/1",
			},
		},
		Attachments: []Attachment{
			{DeviceID: "local-device", IfIndex: 4, EndpointID: "mac:70:49:a2:65:72:cd", Method: "fdb"},
		},
		Enrichments: []Enrichment{
			{
				EndpointID: "mac:70:49:a2:65:72:cd",
				MAC:        "70:49:a2:65:72:cd",
				IPs:        []netip.Addr{netip.MustParseAddr("10.20.4.84")},
				Labels: map[string]string{
					"sources":    "arp",
					"if_indexes": "4",
					"if_names":   "Gi0/4",
				},
			},
		},
	}

	opts := TopologyDataOptions{
		SchemaVersion: "2.0",
		Source:        "snmp",
		Layer:         "2",
		View:          "summary",
		AgentID:       "agent-1",
		LocalDeviceID: "local-device",
	}

	baseline := ToTopologyData(result, opts)
	for range 10 {
		next := ToTopologyData(result, opts)
		require.Equal(t, baseline, next)
	}
}

func TestToTopologyData_DeduplicatesEndpointActorOverlappingManagedDevice(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:        "sw1",
				Hostname:  "sw1",
				ChassisID: "7049a26572cd",
				Addresses: []netip.Addr{netip.MustParseAddr("10.20.4.84")},
			},
		},
		Attachments: []Attachment{
			{
				DeviceID:   "sw1",
				IfIndex:    1,
				EndpointID: "mac:70:49:a2:65:72:cd",
				Method:     "fdb",
			},
		},
		Enrichments: []Enrichment{
			{
				EndpointID: "mac:70:49:a2:65:72:cd",
				MAC:        "70:49:a2:65:72:cd",
				IPs:        []netip.Addr{netip.MustParseAddr("10.20.4.84")},
			},
		},
	}

	data := ToTopologyData(result, TopologyDataOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})

	require.Len(t, data.Actors, 1)
	require.Equal(t, "device", data.Actors[0].ActorType)
	require.Equal(t, 0, data.Stats["endpoints_total"])
	require.Equal(t, 1, data.Stats["actors_total"])
	require.Equal(t, 0, data.Stats["links_total"])
	require.Equal(t, 0, data.Stats["links_fdb_endpoint_emitted"])
	require.Equal(t, 1, data.Stats["segments_suppressed"])
}

func TestCanonicalTopologyMatchKey_NormalizesEquivalentMACRepresentations(t *testing.T) {
	raw := topology.Match{
		ChassisIDs: []string{"7049a26572cd"},
	}
	colon := topology.Match{
		ChassisIDs: []string{"70:49:A2:65:72:CD"},
	}

	require.Equal(t, canonicalTopologyMatchKey(raw), canonicalTopologyMatchKey(colon))
	require.Equal(t, "mac:70:49:a2:65:72:cd", canonicalTopologyMatchKey(raw))
}

func TestToTopologyData_UsesDeterministicPrimaryManagementIP(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:        "device-a",
				Hostname:  "device-a",
				ChassisID: "aa:bb:cc:dd:ee:ff",
				Addresses: []netip.Addr{
					netip.MustParseAddr("10.0.0.9"),
					netip.MustParseAddr("10.0.0.2"),
					netip.MustParseAddr("10.0.0.9"),
				},
			},
		},
	}

	data := ToTopologyData(result, TopologyDataOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})

	actor := findActorBySysName(data.Actors, "device-a")
	require.NotNil(t, actor)
	require.Equal(t, "10.0.0.2", actor.Attributes["management_ip"])
	require.Equal(t, []string{"10.0.0.2", "10.0.0.9"}, actor.Attributes["management_addresses"])
}

func TestToTopologyData_KeepsDistinctActorsWhenMACDiffersDespiteSameSecondaryIdentity(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:        "device-a",
				Hostname:  "shared-name",
				ChassisID: "00:11:22:33:44:55",
				Addresses: []netip.Addr{netip.MustParseAddr("10.20.30.40")},
			},
			{
				ID:        "device-b",
				Hostname:  "shared-name",
				ChassisID: "00:11:22:33:44:66",
				Addresses: []netip.Addr{netip.MustParseAddr("10.20.30.40")},
			},
		},
	}

	data := ToTopologyData(result, TopologyDataOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})

	require.Len(t, data.Actors, 2)
	macs := make(map[string]struct{}, 2)
	for _, actor := range data.Actors {
		require.Equal(t, "device", actor.ActorType)
		require.NotEmpty(t, actor.Match.MacAddresses)
		macs[actor.Match.MacAddresses[0]] = struct{}{}
	}
	require.Contains(t, macs, "00:11:22:33:44:55")
	require.Contains(t, macs, "00:11:22:33:44:66")
}

func TestToTopologyData_MergesPairedAdjacenciesIntoBidirectionalLink(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:        "switch-a",
				Hostname:  "switch-a",
				ChassisID: "aa:aa:aa:aa:aa:aa",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.1")},
			},
			{
				ID:        "switch-b",
				Hostname:  "switch-b",
				ChassisID: "bb:bb:bb:bb:bb:bb",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.2")},
			},
		},
		Interfaces: []Interface{
			{DeviceID: "switch-a", IfIndex: 1, IfName: "Gi0/1", IfDescr: "Gi0/1"},
			{DeviceID: "switch-b", IfIndex: 2, IfName: "Gi0/2", IfDescr: "Gi0/2"},
		},
		Adjacencies: []Adjacency{
			{
				Protocol:   "lldp",
				SourceID:   "switch-a",
				SourcePort: "Gi0/1",
				TargetID:   "switch-b",
				TargetPort: "Gi0/2",
				Labels: map[string]string{
					adjacencyLabelPairID:   "lldp:pair-a-b",
					adjacencyLabelPairPass: lldpMatchPassDefault,
				},
			},
			{
				Protocol:   "lldp",
				SourceID:   "switch-b",
				SourcePort: "Gi0/2",
				TargetID:   "switch-a",
				TargetPort: "Gi0/1",
				Labels: map[string]string{
					adjacencyLabelPairID:   "lldp:pair-a-b",
					adjacencyLabelPairPass: lldpMatchPassDefault,
				},
			},
		},
	}

	data := ToTopologyData(result, TopologyDataOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})

	require.Len(t, data.Links, 1)
	link := data.Links[0]
	require.Equal(t, "lldp", link.Protocol)
	require.Equal(t, "bidirectional", link.Direction)
	require.Equal(t, "Gi0/1", link.Src.Attributes["if_name"])
	require.Equal(t, "Gi0/1", link.Src.Attributes["port_id"])
	require.Equal(t, "Gi0/2", link.Dst.Attributes["if_name"])
	require.Equal(t, "Gi0/2", link.Dst.Attributes["port_id"])

	require.NotNil(t, link.Metrics)
	require.Equal(t, "lldp:pair-a-b", link.Metrics[adjacencyLabelPairID])
	require.Equal(t, lldpMatchPassDefault, link.Metrics[adjacencyLabelPairPass])
	require.Equal(t, true, link.Metrics["pair_consistent"])

	require.Equal(t, 1, data.Stats["links_total"])
	require.Equal(t, 1, data.Stats["links_lldp"])
	require.Equal(t, 1, data.Stats["links_bidirectional"])
	require.Equal(t, 0, data.Stats["links_unidirectional"])
}

func TestToTopologyData_MergesPairedAdjacenciesPreservesRawAddressHints(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:        "switch-a",
				Hostname:  "switch-a",
				ChassisID: "aa:aa:aa:aa:aa:aa",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.1")},
			},
			{
				ID:        "switch-b",
				Hostname:  "switch-b",
				ChassisID: "bb:bb:bb:bb:bb:bb",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.2")},
			},
		},
		Adjacencies: []Adjacency{
			{
				Protocol:   "cdp",
				SourceID:   "switch-a",
				SourcePort: "Gi0/1",
				TargetID:   "switch-b",
				TargetPort: "Gi0/2",
				Labels: map[string]string{
					adjacencyLabelPairID:   "cdp:pair-a-b",
					adjacencyLabelPairPass: cdpMatchPassDefault,
					"remote_address_raw":   "edge-sw3.mgmt.local",
				},
			},
			{
				Protocol:   "cdp",
				SourceID:   "switch-b",
				SourcePort: "Gi0/2",
				TargetID:   "switch-a",
				TargetPort: "Gi0/1",
				Labels: map[string]string{
					adjacencyLabelPairID:   "cdp:pair-a-b",
					adjacencyLabelPairPass: cdpMatchPassDefault,
					"remote_address_raw":   "10.0.0.1",
				},
			},
		},
	}

	data := ToTopologyData(result, TopologyDataOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})

	require.Len(t, data.Links, 1)
	link := data.Links[0]
	require.Equal(t, "cdp", link.Protocol)
	require.Equal(t, "bidirectional", link.Direction)
	require.Contains(t, link.Dst.Match.IPAddresses, "edge-sw3.mgmt.local")
	require.Contains(t, link.Metrics, "src_remote_address_raw")
	require.Contains(t, link.Metrics, "dst_remote_address_raw")
}

func TestToTopologyData_MergesReversePairsWithoutDirectionalPairLabels(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:        "router-a",
				Hostname:  "MikroTik-router",
				ChassisID: "aa:aa:aa:aa:aa:aa",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.1")},
			},
			{
				ID:        "switch-b",
				Hostname:  "XS1930",
				ChassisID: "bb:bb:bb:bb:bb:bb",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.2")},
			},
		},
		Interfaces: []Interface{
			{DeviceID: "router-a", IfIndex: 3, IfName: "ether3", IfDescr: "ether3"},
			{DeviceID: "switch-b", IfIndex: 8, IfName: "swp07", IfDescr: "swp07"},
		},
		Adjacencies: []Adjacency{
			{
				Protocol:   "lldp",
				SourceID:   "router-a",
				SourcePort: "ether3",
				TargetID:   "switch-b",
				TargetPort: "",
				Labels: map[string]string{
					adjacencyLabelPairID:   "lldp:pair-router-xs",
					adjacencyLabelPairPass: lldpMatchPassPortDesc,
				},
			},
			{
				Protocol:   "lldp",
				SourceID:   "switch-b",
				SourcePort: "8",
				TargetID:   "router-a",
				TargetPort: "ether3",
				Labels: map[string]string{
					adjacencyLabelPairID:   "lldp:pair-router-xs",
					adjacencyLabelPairPass: lldpMatchPassPortDesc,
				},
			},
		},
	}

	data := ToTopologyData(result, TopologyDataOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})

	require.Equal(t, 1, data.Stats["links_total"])
	require.Equal(t, 1, data.Stats["links_lldp"])
	require.Equal(t, 1, data.Stats["links_bidirectional"])
	require.Equal(t, 0, data.Stats["links_unidirectional"])
	require.Len(t, data.Links, 1)

	link := data.Links[0]
	require.Equal(t, "lldp", link.Protocol)
	require.Equal(t, "bidirectional", link.Direction)
	require.Equal(t, "MikroTik-router", topologyAttrString(link.Src.Attributes, "sys_name"))
	require.Equal(t, "XS1930", topologyAttrString(link.Dst.Attributes, "sys_name"))
	require.Equal(t, "ether3", topologyAttrString(link.Src.Attributes, "if_name"))
	require.Equal(t, "swp07", topologyAttrString(link.Dst.Attributes, "if_name"))
	require.Equal(t, "8", topologyAttrString(link.Dst.Attributes, "port_id"))
	require.Equal(t, "swp07", topologyAttrString(link.Dst.Attributes, "port_name"))
}

func TestToTopologyData_UnknownAdjacencyPortsRemainUnsetWithoutZeroFallback(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:        "router-a",
				Hostname:  "MikroTik-router",
				ChassisID: "aa:aa:aa:aa:aa:aa",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.1")},
			},
			{
				ID:        "switch-b",
				Hostname:  "XS1930",
				ChassisID: "bb:bb:bb:bb:bb:bb",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.2")},
			},
		},
		Interfaces: []Interface{
			{DeviceID: "router-a", IfIndex: 3, IfName: "ether3", IfDescr: "ether3"},
		},
		Adjacencies: []Adjacency{
			{
				Protocol:   "lldp",
				SourceID:   "router-a",
				SourcePort: "ether3",
				TargetID:   "switch-b",
				TargetPort: "",
			},
		},
	}

	data := ToTopologyData(result, TopologyDataOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})

	require.Len(t, data.Links, 1)
	link := data.Links[0]
	require.Equal(t, "lldp", link.Protocol)
	require.Equal(t, "unidirectional", link.Direction)
	_, hasPortName := link.Dst.Attributes["port_name"]
	require.False(t, hasPortName)
	require.Equal(t, "", strings.TrimSpace(topologyMetricString(link.Metrics, "dst_port_name")))
	require.Contains(t, topologyMetricString(link.Metrics, "display_name"), ":[unset]")
}

func TestToTopologyData_DropsAmbiguousEndpointSegmentLinks(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:        "switch-a",
				Hostname:  "switch-a",
				ChassisID: "aa:aa:aa:aa:aa:aa",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.1")},
			},
			{
				ID:        "switch-b",
				Hostname:  "switch-b",
				ChassisID: "bb:bb:bb:bb:bb:bb",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.2")},
			},
		},
		Interfaces: []Interface{
			{DeviceID: "switch-a", IfIndex: 1, IfName: "Gi0/1", IfDescr: "Gi0/1"},
			{DeviceID: "switch-b", IfIndex: 2, IfName: "Gi0/2", IfDescr: "Gi0/2"},
		},
		Attachments: []Attachment{
			{DeviceID: "switch-a", IfIndex: 1, EndpointID: "mac:70:49:a2:65:72:cd", Method: "fdb"},
			{DeviceID: "switch-b", IfIndex: 2, EndpointID: "mac:70:49:a2:65:72:cd", Method: "fdb"},
		},
	}

	data := ToTopologyData(result, TopologyDataOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})

	bridgeLinks := 0
	fdbLinks := 0
	for _, link := range data.Links {
		switch link.Protocol {
		case "bridge":
			bridgeLinks++
		case "fdb":
			fdbLinks++
		}
	}

	require.Equal(t, 0, bridgeLinks)
	require.Equal(t, 0, fdbLinks)
	require.Equal(t, 0, data.Stats["links_total"])
	require.Equal(t, 0, data.Stats["links_fdb"])
	require.Equal(t, 2, data.Stats["links_fdb_endpoint_candidates"])
	require.Equal(t, 0, data.Stats["links_fdb_endpoint_emitted"])
	require.Equal(t, 2, data.Stats["links_fdb_endpoint_suppressed"])
	require.Equal(t, 1, data.Stats["endpoints_ambiguous_segments"])
	require.Equal(t, 2, data.Stats["segments_suppressed"])
}

func TestToTopologyData_ProbableConnectivityConnectsAmbiguousEndpoint(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:        "switch-a",
				Hostname:  "switch-a",
				ChassisID: "aa:aa:aa:aa:aa:aa",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.1")},
			},
			{
				ID:        "switch-b",
				Hostname:  "switch-b",
				ChassisID: "bb:bb:bb:bb:bb:bb",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.2")},
			},
		},
		Interfaces: []Interface{
			{DeviceID: "switch-a", IfIndex: 1, IfName: "Gi0/1", IfDescr: "Gi0/1"},
			{DeviceID: "switch-b", IfIndex: 2, IfName: "Gi0/2", IfDescr: "Gi0/2"},
		},
		Attachments: []Attachment{
			{DeviceID: "switch-a", IfIndex: 1, EndpointID: "mac:70:49:a2:65:72:cd", Method: "fdb"},
			{DeviceID: "switch-b", IfIndex: 2, EndpointID: "mac:70:49:a2:65:72:cd", Method: "fdb"},
		},
	}

	strictData := ToTopologyData(result, TopologyDataOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})
	require.Len(t, findFDBLinksByEndpointMAC(strictData.Links, "70:49:a2:65:72:cd"), 0)

	data := ToTopologyData(result, TopologyDataOptions{
		Source:                    "snmp",
		Layer:                     "2",
		View:                      "summary",
		ProbabilisticConnectivity: true,
	})

	fdbLinks := findFDBLinksByEndpointMAC(data.Links, "70:49:a2:65:72:cd")
	require.Len(t, fdbLinks, 1)
	require.Equal(t, "probable", strings.ToLower(strings.TrimSpace(fdbLinks[0].State)))
	require.Equal(t, "probable", strings.ToLower(strings.TrimSpace(topologyMetricString(fdbLinks[0].Metrics, "inference"))))
	require.Equal(t, "probable_segment", topologyMetricString(fdbLinks[0].Metrics, "attachment_mode"))
	require.Equal(t, "low", topologyMetricString(fdbLinks[0].Metrics, "confidence"))

	require.Equal(t, 1, data.Stats["links_probable"])
	require.Equal(t, 1, data.Stats["links_fdb_endpoint_emitted"])
	require.Equal(t, 1, data.Stats["links_fdb_endpoint_suppressed"])
}

func TestToTopologyData_ProbableConnectivityDoesNotReclassifyStrictSinglePortEndpoint(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:        "switch-a",
				Hostname:  "switch-a",
				ChassisID: "aa:aa:aa:aa:aa:aa",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.1")},
			},
		},
		Interfaces: []Interface{
			{DeviceID: "switch-a", IfIndex: 1, IfName: "Gi0/1", IfDescr: "Gi0/1"},
		},
		Attachments: []Attachment{
			{DeviceID: "switch-a", IfIndex: 1, EndpointID: "mac:dd:dd:dd:dd:dd:dd", Method: "fdb"},
		},
	}

	strictData := ToTopologyData(result, TopologyDataOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})
	strictFDBLinks := findFDBLinksByEndpointMAC(strictData.Links, "dd:dd:dd:dd:dd:dd")
	require.Len(t, strictFDBLinks, 1)
	require.Equal(t, "", strings.TrimSpace(strictFDBLinks[0].State))
	require.Equal(t, "", strings.TrimSpace(topologyMetricString(strictFDBLinks[0].Metrics, "inference")))

	data := ToTopologyData(result, TopologyDataOptions{
		Source:                    "snmp",
		Layer:                     "2",
		View:                      "summary",
		ProbabilisticConnectivity: true,
	})

	fdbLinks := findFDBLinksByEndpointMAC(data.Links, "dd:dd:dd:dd:dd:dd")
	require.Len(t, fdbLinks, 1)
	require.Equal(t, "", strings.TrimSpace(fdbLinks[0].State))
	require.Equal(t, "", strings.TrimSpace(topologyMetricString(fdbLinks[0].Metrics, "inference")))
	require.Equal(t, "direct", topologyMetricString(fdbLinks[0].Metrics, "attachment_mode"))
	require.Equal(t, 0, data.Stats["links_probable"])
}

func TestToTopologyData_ProbableConnectivityConnectsUnlinkedLLDPEndpoint(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:        "switch-a",
				Hostname:  "switch-a",
				ChassisID: "aa:aa:aa:aa:aa:aa",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.1")},
			},
			{
				ID:        "switch-b",
				Hostname:  "switch-b",
				ChassisID: "bb:bb:bb:bb:bb:bb",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.2")},
			},
		},
		Interfaces: []Interface{
			{DeviceID: "switch-a", IfIndex: 1, IfName: "Gi0/1", IfDescr: "Gi0/1"},
			{DeviceID: "switch-b", IfIndex: 2, IfName: "Gi0/2", IfDescr: "Gi0/2"},
		},
		Attachments: []Attachment{
			{DeviceID: "switch-a", IfIndex: 1, EndpointID: "mac:70:49:a2:65:72:cf", Method: "lldp"},
			{DeviceID: "switch-b", IfIndex: 2, EndpointID: "mac:70:49:a2:65:72:cf", Method: "lldp"},
		},
	}

	strictData := ToTopologyData(result, TopologyDataOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})
	require.Len(t, findFDBLinksByEndpointMAC(strictData.Links, "70:49:a2:65:72:cf"), 0)

	data := ToTopologyData(result, TopologyDataOptions{
		Source:                    "snmp",
		Layer:                     "2",
		View:                      "summary",
		ProbabilisticConnectivity: true,
	})

	fdbLinks := findFDBLinksByEndpointMAC(data.Links, "70:49:a2:65:72:cf")
	require.Len(t, fdbLinks, 1)
	require.Equal(t, "probable", strings.ToLower(strings.TrimSpace(fdbLinks[0].State)))
	require.Equal(t, "probable", strings.ToLower(strings.TrimSpace(topologyMetricString(fdbLinks[0].Metrics, "inference"))))
	require.Equal(t, "probable_segment", topologyMetricString(fdbLinks[0].Metrics, "attachment_mode"))
}

func TestToTopologyData_ProbableConnectivityAvoidsExtraBridgePathForLLDPPeers(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:        "switch-a",
				Hostname:  "switch-a",
				ChassisID: "aa:aa:aa:aa:aa:aa",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.1")},
			},
			{
				ID:        "switch-b",
				Hostname:  "switch-b",
				ChassisID: "bb:bb:bb:bb:bb:bb",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.2")},
			},
		},
		Interfaces: []Interface{
			{DeviceID: "switch-a", IfIndex: 1, IfName: "Gi0/1", IfDescr: "Gi0/1"},
			{DeviceID: "switch-a", IfIndex: 2, IfName: "Gi0/2", IfDescr: "Gi0/2"},
			{DeviceID: "switch-b", IfIndex: 1, IfName: "Gi0/1", IfDescr: "Gi0/1"},
			{DeviceID: "switch-b", IfIndex: 2, IfName: "Gi0/2", IfDescr: "Gi0/2"},
		},
		Adjacencies: []Adjacency{
			{
				Protocol:   "lldp",
				SourceID:   "switch-a",
				SourcePort: "Gi0/1",
				TargetID:   "switch-b",
				TargetPort: "Gi0/1",
			},
			{
				Protocol:   "lldp",
				SourceID:   "switch-a",
				SourcePort: "Gi0/2",
				TargetID:   "switch-b",
				TargetPort: "Gi0/2",
			},
		},
		Attachments: []Attachment{
			{DeviceID: "switch-a", IfIndex: 1, EndpointID: "mac:70:49:a2:65:72:aa", Method: "lldp"},
			{DeviceID: "switch-b", IfIndex: 1, EndpointID: "mac:70:49:a2:65:72:aa", Method: "lldp"},
			{DeviceID: "switch-a", IfIndex: 2, EndpointID: "mac:70:49:a2:65:72:aa", Method: "lldp"},
			{DeviceID: "switch-b", IfIndex: 2, EndpointID: "mac:70:49:a2:65:72:aa", Method: "lldp"},
		},
	}

	strictData := ToTopologyData(result, TopologyDataOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})
	require.Len(t, findFDBLinksByEndpointMAC(strictData.Links, "70:49:a2:65:72:aa"), 0)

	data := ToTopologyData(result, TopologyDataOptions{
		Source:                    "snmp",
		Layer:                     "2",
		View:                      "summary",
		ProbabilisticConnectivity: true,
	})

	fdbLinks := findFDBLinksByEndpointMAC(data.Links, "70:49:a2:65:72:aa")
	require.Len(t, fdbLinks, 1)
	require.Equal(t, "probable", strings.ToLower(strings.TrimSpace(fdbLinks[0].State)))

	bridgeLinksBySegment := make(map[string]map[string]struct{})
	for _, link := range data.Links {
		if !strings.EqualFold(strings.TrimSpace(link.Protocol), "bridge") {
			continue
		}
		segmentActorID := strings.TrimSpace(link.DstActorID)
		if segmentActorID == "" {
			continue
		}
		devices := bridgeLinksBySegment[segmentActorID]
		if devices == nil {
			devices = make(map[string]struct{})
			bridgeLinksBySegment[segmentActorID] = devices
		}
		devices[strings.TrimSpace(link.SrcActorID)] = struct{}{}
	}
	for _, devices := range bridgeLinksBySegment {
		require.LessOrEqual(t, len(devices), 1)
	}
}

func TestToTopologyData_ProbableConnectivityConnectsZeroCandidateEndpointUsingReporterHints(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:        "switch-a",
				Hostname:  "switch-a",
				ChassisID: "aa:aa:aa:aa:aa:aa",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.1")},
			},
		},
		Interfaces: []Interface{
			{DeviceID: "switch-a", IfIndex: 1, IfName: "Gi0/1", IfDescr: "Gi0/1"},
		},
		Attachments: []Attachment{
			{DeviceID: "switch-a", IfIndex: 1, EndpointID: "mac:dd:dd:dd:dd:dd:dd", Method: "fdb"},
		},
		Enrichments: []Enrichment{
			{
				EndpointID: "ip:10.0.0.99",
				IPs:        []netip.Addr{netip.MustParseAddr("10.0.0.99")},
				Labels: map[string]string{
					"sources":    "arp",
					"device_ids": "switch-a",
					"if_indexes": "1",
					"if_names":   "Gi0/1",
				},
			},
		},
	}

	strictData := ToTopologyData(result, TopologyDataOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})
	require.Len(t, findFDBLinksByEndpointIP(strictData.Links, "10.0.0.99"), 0)

	data := ToTopologyData(result, TopologyDataOptions{
		Source:                    "snmp",
		Layer:                     "2",
		View:                      "summary",
		ProbabilisticConnectivity: true,
	})

	fdbLinks := findFDBLinksByEndpointIP(data.Links, "10.0.0.99")
	require.Len(t, fdbLinks, 1)
	require.Equal(t, "probable", strings.ToLower(strings.TrimSpace(fdbLinks[0].State)))
	require.Equal(t, "probable", strings.ToLower(strings.TrimSpace(topologyMetricString(fdbLinks[0].Metrics, "inference"))))
	require.Equal(t, "probable_segment", topologyMetricString(fdbLinks[0].Metrics, "attachment_mode"))
	require.Equal(t, "low", topologyMetricString(fdbLinks[0].Metrics, "confidence"))

	strictSignatures := topologyLinkSignatures(strictData.Links)
	probableSignatures := topologyLinkSignatures(data.Links)
	for signature := range strictSignatures {
		_, ok := probableSignatures[signature]
		require.Truef(t, ok, "strict link signature missing in probable output: %s", signature)
	}
}

func TestToTopologyData_ProbableConnectivityCreatesPortlessAttachmentForZeroCandidateEndpoint(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:        "switch-a",
				Hostname:  "switch-a",
				ChassisID: "aa:aa:aa:aa:aa:aa",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.1")},
			},
		},
		Interfaces: []Interface{
			{DeviceID: "switch-a", IfIndex: 1, IfName: "Gi0/1", IfDescr: "Gi0/1"},
		},
		Attachments: []Attachment{
			{DeviceID: "switch-a", IfIndex: 1, EndpointID: "mac:dd:dd:dd:dd:dd:dd", Method: "fdb"},
		},
		Enrichments: []Enrichment{
			{
				EndpointID: "ip:10.0.0.199",
				IPs:        []netip.Addr{netip.MustParseAddr("10.0.0.199")},
				Labels: map[string]string{
					"sources":    "arp",
					"device_ids": "switch-a",
					"if_indexes": "999",
					"if_names":   "Gi0/999",
				},
			},
		},
	}

	data := ToTopologyData(result, TopologyDataOptions{
		Source:                    "snmp",
		Layer:                     "2",
		View:                      "summary",
		ProbabilisticConnectivity: true,
	})

	fdbLinks := findFDBLinksByEndpointIP(data.Links, "10.0.0.199")
	require.Len(t, fdbLinks, 1)
	require.Equal(t, "probable", strings.ToLower(strings.TrimSpace(fdbLinks[0].State)))
	require.Equal(t, "probable_portless", topologyMetricString(fdbLinks[0].Metrics, "attachment_mode"))

	segmentActor := findActorByMatch(data.Actors, fdbLinks[0].Src.Match)
	require.NotNil(t, segmentActor)
	require.Contains(t, topologyAttrString(segmentActor.Attributes, "segment_id"), "bridge-domain:probable:")
	require.Equal(t, []string{"switch-a"}, segmentActor.Attributes["parent_devices"])

	bridgeCount := 0
	for _, link := range data.Links {
		if !strings.EqualFold(strings.TrimSpace(link.Protocol), "bridge") {
			continue
		}
		if canonicalTopologyMatchKey(link.Dst.Match) != canonicalTopologyMatchKey(fdbLinks[0].Src.Match) {
			continue
		}
		bridgeCount++
	}
	require.Equal(t, 1, bridgeCount)
}

func TestToTopologyData_InferenceStrategy_STPParentDoesNotSuppressFDBEndpointOwnership(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:        "switch-a",
				Hostname:  "switch-a",
				ChassisID: "aa:aa:aa:aa:aa:aa",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.1")},
			},
			{
				ID:        "switch-b",
				Hostname:  "switch-b",
				ChassisID: "bb:bb:bb:bb:bb:bb",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.2")},
			},
		},
		Interfaces: []Interface{
			{DeviceID: "switch-a", IfIndex: 1, IfName: "Gi0/1", IfDescr: "Gi0/1"},
			{DeviceID: "switch-b", IfIndex: 2, IfName: "Gi0/2", IfDescr: "Gi0/2"},
		},
		Adjacencies: []Adjacency{
			{
				Protocol:   "stp",
				SourceID:   "switch-a",
				SourcePort: "Gi0/1",
				TargetID:   "switch-b",
				TargetPort: "Gi0/2",
			},
		},
		Attachments: []Attachment{
			{DeviceID: "switch-a", IfIndex: 1, EndpointID: "mac:00:00:00:00:00:11", Method: "fdb"},
			{DeviceID: "switch-b", IfIndex: 2, EndpointID: "mac:00:00:00:00:00:22", Method: "fdb"},
		},
	}

	baseline := ToTopologyData(result, TopologyDataOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})
	require.Equal(t, topologyInferenceStrategyFDBMinimumKnowledge, baseline.Stats["inference_strategy"])
	require.Greater(t, baseline.Stats["links_fdb_endpoint_emitted"].(int), 0)

	stpData := ToTopologyData(result, TopologyDataOptions{
		Source:            "snmp",
		Layer:             "2",
		View:              "summary",
		InferenceStrategy: topologyInferenceStrategySTPParentTree,
	})
	require.Equal(t, topologyInferenceStrategySTPParentTree, stpData.Stats["inference_strategy"])
	require.Greater(t, stpData.Stats["links_fdb_endpoint_emitted"].(int), 0)
}

func TestToTopologyData_InferenceStrategy_CDPHybridPrefersCDPBridgeLinks(t *testing.T) {
	result := Result{
		Devices: []Device{
			{ID: "sw-a", Hostname: "sw-a", ChassisID: "00:00:00:00:00:aa"},
			{ID: "sw-b", Hostname: "sw-b", ChassisID: "00:00:00:00:00:bb"},
			{ID: "sw-c", Hostname: "sw-c", ChassisID: "00:00:00:00:00:cc"},
		},
		Interfaces: []Interface{
			{DeviceID: "sw-a", IfIndex: 1, IfName: "Gi0/1"},
			{DeviceID: "sw-a", IfIndex: 2, IfName: "Gi0/2"},
			{DeviceID: "sw-b", IfIndex: 1, IfName: "Gi0/1"},
			{DeviceID: "sw-c", IfIndex: 1, IfName: "Gi0/1"},
		},
		Adjacencies: []Adjacency{
			{
				Protocol:   "lldp",
				SourceID:   "sw-a",
				SourcePort: "Gi0/1",
				TargetID:   "sw-b",
				TargetPort: "Gi0/1",
			},
			{
				Protocol:   "cdp",
				SourceID:   "sw-a",
				SourcePort: "Gi0/2",
				TargetID:   "sw-c",
				TargetPort: "Gi0/1",
			},
		},
		Attachments: []Attachment{
			{DeviceID: "sw-a", IfIndex: 1, EndpointID: "mac:00:00:00:00:10:01", Method: "fdb"},
			{DeviceID: "sw-a", IfIndex: 2, EndpointID: "mac:00:00:00:00:10:02", Method: "fdb"},
		},
	}

	baseline := ToTopologyData(result, TopologyDataOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})
	require.Equal(t, topologyInferenceStrategyFDBMinimumKnowledge, baseline.Stats["inference_strategy"])
	require.Equal(t, 0, baseline.Stats["links_fdb_endpoint_emitted"])

	data := ToTopologyData(result, TopologyDataOptions{
		Source:            "snmp",
		Layer:             "2",
		View:              "summary",
		InferenceStrategy: topologyInferenceStrategyCDPFDBHybrid,
	})

	require.Equal(t, topologyInferenceStrategyCDPFDBHybrid, data.Stats["inference_strategy"])
	require.Equal(t, 1, data.Stats["links_cdp"])
	require.Equal(t, 0, data.Stats["links_fdb_endpoint_emitted"])
}

func TestPickProbableSegmentAnchorPortID_PrefersManagedPortWhenOwnerPointsToUnmanaged(t *testing.T) {
	unmanagedPort := bridgePortRef{
		deviceID: "ghost-switch",
		ifIndex:  900,
		ifName:   "ghost0",
	}
	managedPort := bridgePortRef{
		deviceID: "managed-switch",
		ifIndex:  7,
		ifName:   "swp06",
	}

	segment := newBridgeDomainSegment(unmanagedPort)
	segment.addPort(managedPort)

	endpointID := "mac:50:2c:c6:a6:fc:35"
	owner := fdbEndpointOwner{
		portKey:     bridgePortObservationKey(unmanagedPort),
		portVLANKey: bridgePortObservationVLANKey(unmanagedPort),
		port:        unmanagedPort,
		source:      "single_port_mac",
	}

	picked := pickProbableSegmentAnchorPortID(
		segment,
		map[string]struct{}{endpointID: {}},
		map[string]fdbEndpointOwner{endpointID: owner},
		map[string]struct{}{"managed-switch": {}},
	)

	require.NotEmpty(t, picked)
	require.Equal(t, bridgePortRefSortKey(managedPort), bridgePortRefSortKey(segment.ports[picked]))
}

func TestSelectProbableEndpointReporterHint_PrefersManagedHintsOverUnmanagedOwner(t *testing.T) {
	endpointLabels := map[string]string{
		"learned_device_ids": "ghost-switch,managed-switch",
		"learned_if_indexes": "7",
		"learned_if_names":   "swp06",
	}
	reporterHints := map[string][]bridgePortRef{
		"ghost-switch": {
			{deviceID: "ghost-switch", ifIndex: 900, ifName: "ghost0"},
		},
		"managed-switch": {
			{deviceID: "managed-switch", ifIndex: 7, ifName: "swp06"},
		},
	}
	owner := fdbEndpointOwner{
		port: bridgePortRef{
			deviceID: "ghost-switch",
			ifIndex:  900,
			ifName:   "ghost0",
		},
		source: "single_port_mac",
	}

	hint := selectProbableEndpointReporterHint(
		endpointLabels,
		reporterHints,
		owner,
		nil,
		map[string]struct{}{"managed-switch": {}},
	)

	require.Equal(t, "managed-switch", hint.deviceID)
	require.Equal(t, 7, hint.ifIndex)
	require.Equal(t, "swp06", hint.ifName)
}

func TestProbableCandidateSegmentsFromReporterHints_PrefersManagedReporterSegments(t *testing.T) {
	index := segmentReporterIndex{
		byDevice: map[string]map[string]struct{}{
			"ghost-switch":   {"segment:ghost": {}},
			"managed-switch": {"segment:managed": {}},
		},
		byDeviceIfIndex: map[string]map[string]struct{}{
			"ghost-switch\x007":   {"segment:ghost": {}},
			"managed-switch\x007": {"segment:managed": {}},
		},
		byDeviceIfName: map[string]map[string]struct{}{
			"ghost-switch\x00swp06":   {"segment:ghost": {}},
			"managed-switch\x00swp06": {"segment:managed": {}},
		},
	}

	segments := probableCandidateSegmentsFromReporterHints(
		map[string]string{
			"learned_device_ids": "ghost-switch,managed-switch",
			"learned_if_indexes": "7",
			"learned_if_names":   "swp06",
		},
		nil,
		index,
		nil,
		map[string]struct{}{"managed-switch": {}},
	)

	require.Equal(t, []string{"segment:managed"}, segments)
}

func TestEnsureManagedProbableReporterHint_UpgradesUnmanagedHint(t *testing.T) {
	hint := probableEndpointReporterHint{
		deviceID: "ghost-switch",
	}
	endpointLabels := map[string]string{
		"learned_device_ids": "macAddress:18:fd:74:7e:c5:80",
		"learned_if_indexes": "7",
		"learned_if_names":   "swp06",
	}
	aliasOwnerIDs := map[string]map[string]struct{}{
		"mac:18:fd:74:7e:c5:80": {
			"managed-switch": {},
		},
	}

	updated := ensureManagedProbableReporterHint(
		hint,
		endpointLabels,
		nil,
		aliasOwnerIDs,
		map[string]struct{}{"managed-switch": {}},
		[]string{"managed-switch"},
	)

	require.Equal(t, "managed-switch", updated.deviceID)
	require.Equal(t, 7, updated.ifIndex)
	require.Equal(t, "swp06", updated.ifName)
}

func TestEnsureManagedProbableReporterHint_FallsBackToFirstManagedDevice(t *testing.T) {
	updated := ensureManagedProbableReporterHint(
		probableEndpointReporterHint{deviceID: "ghost-switch"},
		nil,
		nil,
		nil,
		map[string]struct{}{"managed-a": {}, "managed-b": {}},
		[]string{"managed-a", "managed-b"},
	)

	require.Equal(t, "managed-a", updated.deviceID)
	require.Equal(t, 0, updated.ifIndex)
	require.Equal(t, "0", updated.ifName)
}

func TestToTopologyData_ProbableConnectivityRecoversUnmanagedOverlapSuppression(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:        "switch-a",
				Hostname:  "switch-a",
				ChassisID: "aa:aa:aa:aa:aa:aa",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.1")},
			},
		},
		Interfaces: []Interface{
			{DeviceID: "switch-a", IfIndex: 1, IfName: "Gi0/1", IfDescr: "Gi0/1"},
			{DeviceID: "switch-a", IfIndex: 2, IfName: "Gi0/2", IfDescr: "Gi0/2"},
		},
		Adjacencies: []Adjacency{
			{
				Protocol:   "lldp",
				SourceID:   "switch-a",
				SourcePort: "Gi0/1",
				TargetID:   "remote-peer",
				TargetPort: "cc:cc:cc:cc:cc:cc",
			},
		},
		Attachments: []Attachment{
			{DeviceID: "switch-a", IfIndex: 2, EndpointID: "mac:cc:cc:cc:cc:cc:cc", Method: "fdb"},
		},
	}

	strictData := ToTopologyData(result, TopologyDataOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})
	require.Len(t, findFDBLinksByEndpointMAC(strictData.Links, "cc:cc:cc:cc:cc:cc"), 0)

	data := ToTopologyData(result, TopologyDataOptions{
		Source:                    "snmp",
		Layer:                     "2",
		View:                      "summary",
		ProbabilisticConnectivity: true,
	})

	fdbLinks := findFDBLinksByEndpointMAC(data.Links, "cc:cc:cc:cc:cc:cc")
	require.Len(t, fdbLinks, 1)
	require.Equal(t, "probable", strings.ToLower(strings.TrimSpace(fdbLinks[0].State)))
	require.Equal(t, "probable", strings.ToLower(strings.TrimSpace(topologyMetricString(fdbLinks[0].Metrics, "inference"))))
	require.Equal(t, "probable_direct", topologyMetricString(fdbLinks[0].Metrics, "attachment_mode"))
	require.Equal(t, "low", topologyMetricString(fdbLinks[0].Metrics, "confidence"))
}

func TestToTopologyData_CollapseByIPPrunesSuppressedManagedOverlapEndpoint(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:        "switch-a",
				Hostname:  "switch-a",
				ChassisID: "aa:aa:aa:aa:aa:aa",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.1")},
			},
			{
				ID:        "nova",
				Hostname:  "nova",
				ChassisID: "9c:6b:00:7b:98:c6",
				Addresses: []netip.Addr{netip.MustParseAddr("172.22.0.1")},
				Labels:    map[string]string{"inferred": "true"},
			},
		},
		Interfaces: []Interface{
			{DeviceID: "switch-a", IfIndex: 1, IfName: "Gi0/1", IfDescr: "Gi0/1"},
			{DeviceID: "switch-a", IfIndex: 2, IfName: "Gi0/2", IfDescr: "Gi0/2"},
		},
		Adjacencies: []Adjacency{
			{
				Protocol:   "lldp",
				SourceID:   "switch-a",
				SourcePort: "Gi0/1",
				TargetID:   "nova",
				TargetPort: "9c:6b:00:7b:98:c7",
			},
		},
		Attachments: []Attachment{
			{DeviceID: "switch-a", IfIndex: 2, EndpointID: "mac:9c:6b:00:7b:98:c7", Method: "fdb"},
		},
		Enrichments: []Enrichment{
			{
				EndpointID: "mac:9c:6b:00:7b:98:c7",
				MAC:        "9c:6b:00:7b:98:c7",
				IPs:        []netip.Addr{netip.MustParseAddr("10.20.4.22")},
				Labels: map[string]string{
					"sources": "arp",
				},
			},
		},
	}

	withoutCollapse := ToTopologyData(result, TopologyDataOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})
	require.NotNil(t, findActorByMAC(withoutCollapse.Actors, "9c:6b:00:7b:98:c7"))

	withCollapse := ToTopologyData(result, TopologyDataOptions{
		Source:             "snmp",
		Layer:              "2",
		View:               "summary",
		CollapseActorsByIP: true,
	})
	require.NotNil(t, findActorByMAC(withCollapse.Actors, "9c:6b:00:7b:98:c6"))
	require.Nil(t, findActorByMAC(withCollapse.Actors, "9c:6b:00:7b:98:c7"))
	require.Equal(t, 1, withCollapse.Stats["actors_unlinked_suppressed"])
}

func TestToTopologyData_ReplacesKnownDeviceEndpointWithManagedDeviceEdge(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:        "router-a",
				Hostname:  "router-a",
				ChassisID: "aa:aa:aa:aa:aa:aa",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.1")},
			},
			{
				ID:        "switch-b",
				Hostname:  "switch-b",
				ChassisID: "bb:bb:bb:bb:bb:bb",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.2")},
			},
		},
		Interfaces: []Interface{
			{DeviceID: "router-a", IfIndex: 1, IfName: "ether1"},
		},
		Attachments: []Attachment{
			{DeviceID: "router-a", IfIndex: 1, EndpointID: "mac:bb:bb:bb:bb:bb:bb", Method: "fdb"},
		},
	}

	data := ToTopologyData(result, TopologyDataOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})

	fdbLinks := findFDBLinksByDstSysName(data.Links, "switch-b")
	require.Len(t, fdbLinks, 1)
	require.Equal(t, "managed_device_overlap", fdbLinks[0].Metrics["attachment_mode"])
	require.Equal(t, 1, data.Stats["links_fdb_endpoint_emitted"])
	require.Equal(t, 0, data.Stats["links_fdb_endpoint_suppressed"])

	for _, actor := range data.Actors {
		if actor.ActorType != "endpoint" {
			continue
		}
		for _, mac := range actor.Match.MacAddresses {
			require.NotEqual(t, "bb:bb:bb:bb:bb:bb", mac)
		}
	}
}

func TestToTopologyData_KnownDeviceOverlapUsesInterfaceMACAlias(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:        "router-a",
				Hostname:  "router-a",
				ChassisID: "aa:aa:aa:aa:aa:80",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.1")},
			},
			{
				ID:        "switch-b",
				Hostname:  "switch-b",
				ChassisID: "bb:bb:bb:bb:bb:bb",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.2")},
			},
		},
		Interfaces: []Interface{
			{DeviceID: "router-a", IfIndex: 1, IfName: "ether1", MAC: "aa:aa:aa:aa:aa:8c"},
			{DeviceID: "switch-b", IfIndex: 1, IfName: "ether1"},
		},
		Attachments: []Attachment{
			{DeviceID: "switch-b", IfIndex: 1, EndpointID: "mac:aa:aa:aa:aa:aa:8c", Method: "fdb"},
		},
	}

	data := ToTopologyData(result, TopologyDataOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})

	fdbLinks := findFDBLinksByDstSysName(data.Links, "router-a")
	require.Len(t, fdbLinks, 1)
	require.Equal(t, "managed_device_overlap", fdbLinks[0].Metrics["attachment_mode"])
	require.Equal(t, 1, data.Stats["links_fdb_endpoint_emitted"])
	require.Equal(t, 0, data.Stats["links_fdb_endpoint_suppressed"])
}

func TestToTopologyData_DeviceActorIncludesInterfaceMACAliases(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:        "switch-a",
				Hostname:  "switch-a",
				ChassisID: "aa:aa:aa:aa:aa:01",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.1")},
			},
		},
		Interfaces: []Interface{
			{DeviceID: "switch-a", IfIndex: 1, IfName: "Gi0/1", MAC: "aa:aa:aa:aa:aa:11"},
			{DeviceID: "switch-a", IfIndex: 2, IfName: "Gi0/2", MAC: "aa:aa:aa:aa:aa:12"},
			{DeviceID: "switch-a", IfIndex: 3, IfName: "Gi0/3", MAC: "AA-AA-AA-AA-AA-12"},
		},
	}

	data := ToTopologyData(result, TopologyDataOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})

	actor := findActorBySysName(data.Actors, "switch-a")
	require.NotNil(t, actor)
	require.ElementsMatch(
		t,
		[]string{"aa:aa:aa:aa:aa:01", "aa:aa:aa:aa:aa:11", "aa:aa:aa:aa:aa:12"},
		actor.Match.MacAddresses,
	)
}

func TestToTopologyData_KeepsUnlinkedEndpointsAndDevices(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:        "device-a",
				Hostname:  "device-a",
				ChassisID: "aa:aa:aa:aa:aa:aa",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.1")},
			},
		},
		Enrichments: []Enrichment{
			{
				EndpointID: "ip:10.0.0.42",
				IPs:        []netip.Addr{netip.MustParseAddr("10.0.0.42")},
			},
		},
	}

	data := ToTopologyData(result, TopologyDataOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})

	require.Len(t, data.Actors, 2)
	deviceCount := 0
	endpointCount := 0
	for _, actor := range data.Actors {
		switch actor.ActorType {
		case "device":
			deviceCount++
		case "endpoint":
			endpointCount++
		}
	}
	require.Equal(t, 1, deviceCount)
	require.Equal(t, 1, endpointCount)
	require.Equal(t, 0, data.Stats["links_total"])
	require.Equal(t, 0, data.Stats["actors_unlinked_suppressed"])
}

func TestToTopologyData_KeepsUnlinkedEndpointWhenIdentityOverlapsLinkedDevice(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:        "switch-a",
				Hostname:  "switch-a",
				ChassisID: "aa:aa:aa:aa:aa:aa",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.1")},
			},
			{
				ID:        "mega",
				Hostname:  "mega",
				ChassisID: "bb:bb:bb:bb:bb:bb",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.2")},
			},
		},
		Interfaces: []Interface{
			{DeviceID: "switch-a", IfIndex: 1, IfName: "Gi0/1"},
			{DeviceID: "mega", IfIndex: 1, IfName: "eth0"},
		},
		Adjacencies: []Adjacency{
			{
				Protocol:   "lldp",
				SourceID:   "switch-a",
				SourcePort: "Gi0/1",
				TargetID:   "mega",
				TargetPort: "eth0",
			},
		},
		Enrichments: []Enrichment{
			{
				EndpointID: "mac:cc:cc:cc:cc:cc:cc",
				IPs:        []netip.Addr{netip.MustParseAddr("10.0.0.2")},
			},
		},
	}

	data := ToTopologyData(result, TopologyDataOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})

	require.NotNil(t, findActorByMAC(data.Actors, "cc:cc:cc:cc:cc:cc"))
	require.Equal(t, 3, data.Stats["actors_total"])
	require.Equal(t, 1, data.Stats["links_total"])
	require.Equal(t, 0, data.Stats["actors_unlinked_suppressed"])
}

func TestToTopologyData_DisplayNamesPreferDNSThenIPThenMAC(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:        "switch-a",
				Hostname:  "switch-a",
				ChassisID: "aa:aa:aa:aa:aa:aa",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.1")},
			},
		},
		Interfaces: []Interface{
			{DeviceID: "switch-a", IfIndex: 3, IfName: "Gi0/3", IfDescr: "Gi0/3"},
		},
		Attachments: []Attachment{
			{DeviceID: "switch-a", IfIndex: 3, EndpointID: "ip:10.0.0.42", Method: "arp"},
			{DeviceID: "switch-a", IfIndex: 3, EndpointID: "mac:70:49:a2:65:72:cd", Method: "fdb"},
		},
	}

	data := ToTopologyData(result, TopologyDataOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
		ResolveDNSName: func(ip string) string {
			switch ip {
			case "10.0.0.1":
				return "switch-a.example.net."
			default:
				return ""
			}
		},
	})

	device := findActorBySysName(data.Actors, "switch-a")
	require.NotNil(t, device)
	require.Equal(t, "switch-a.example.net", device.Labels["display_name"])
	require.Equal(t, "dns", device.Labels["display_source"])
	require.Equal(t, "switch-a.example.net", device.Attributes["display_name"])

	ipEndpoint := findActorByIP(data.Actors, "10.0.0.42")
	require.NotNil(t, ipEndpoint)
	require.Equal(t, "10.0.0.42", ipEndpoint.Labels["display_name"])
	require.Equal(t, "ip", ipEndpoint.Labels["display_source"])

	macEndpoint := findActorByMAC(data.Actors, "70:49:a2:65:72:cd")
	require.NotNil(t, macEndpoint)
	require.Equal(t, "70:49:a2:65:72:cd", macEndpoint.Labels["display_name"])
	require.Equal(t, "mac", macEndpoint.Labels["display_source"])

	require.NotEmpty(t, data.Links)
	for _, link := range data.Links {
		require.NotNil(t, link.Src.Attributes)
		require.NotNil(t, link.Dst.Attributes)
		require.NotEmpty(t, link.Src.Attributes["display_name"])
		require.NotEmpty(t, link.Dst.Attributes["display_name"])
	}
}

func TestTopologyDisplayNameFromMatch_PrefersSysNameBeforeIP(t *testing.T) {
	display := topologyDisplayNameFromMatch(topology.Match{
		SysName:     "MikroTik-router",
		IPAddresses: []string{"10.20.4.1"},
	}, &topologyDisplayNameResolver{
		lookup: func(string) string { return "" },
		cache:  map[string]string{},
	})

	require.Equal(t, "MikroTik-router", display.name)
	require.Equal(t, "sys_name", display.source)
}

func TestTopologyDisplayNameFromMatch_PrefersHostnameBeforeIPWhenSysNameMissing(t *testing.T) {
	display := topologyDisplayNameFromMatch(topology.Match{
		Hostnames:   []string{"nova"},
		IPAddresses: []string{"10.20.4.22"},
	}, &topologyDisplayNameResolver{
		lookup: func(string) string { return "" },
		cache:  map[string]string{},
	})

	require.Equal(t, "nova", display.name)
	require.Equal(t, "hostname", display.source)
}

func TestToTopologyData_SegmentDisplayNameUsesParentPortPattern(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:        "switch-a",
				Hostname:  "switch-a",
				ChassisID: "aa:aa:aa:aa:aa:aa",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.1")},
			},
		},
		Interfaces: []Interface{
			{DeviceID: "switch-a", IfIndex: 3, IfName: "Gi0/3", IfDescr: "Gi0/3"},
		},
		Attachments: []Attachment{
			{DeviceID: "switch-a", IfIndex: 3, EndpointID: "mac:70:49:a2:65:72:cd", Method: "fdb"},
			{DeviceID: "switch-a", IfIndex: 3, EndpointID: "mac:70:49:a2:65:72:ce", Method: "fdb"},
		},
	}

	data := ToTopologyData(result, TopologyDataOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
		ResolveDNSName: func(ip string) string {
			if ip == "10.0.0.1" {
				return "switch-a.example.net."
			}
			return ""
		},
	})

	segment := findActorByType(data.Actors, "segment")
	require.NotNil(t, segment)
	require.Equal(t, "switch-a.example.net.gi0/3.segment", segment.Labels["display_name"])
	require.Equal(t, "segment", segment.Labels["display_source"])
	require.Equal(t, "switch-a.example.net.gi0/3.segment", segment.Attributes["display_name"])
}

func TestToTopologyData_FDBOwnerInferencePrefersNonLLDPSide(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:        "switch-a",
				Hostname:  "switch-a",
				ChassisID: "aa:aa:aa:aa:aa:aa",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.1")},
			},
			{
				ID:        "switch-b",
				Hostname:  "switch-b",
				ChassisID: "bb:bb:bb:bb:bb:bb",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.2")},
			},
		},
		Interfaces: []Interface{
			{DeviceID: "switch-a", IfIndex: 1, IfName: "Gi0/1", MAC: "aa:aa:aa:aa:aa:aa"},
			{DeviceID: "switch-b", IfIndex: 1, IfName: "Gi0/1", MAC: "bb:bb:bb:bb:bb:bb"},
			{DeviceID: "switch-b", IfIndex: 2, IfName: "Gi0/2", MAC: "bb:bb:bb:bb:bb:bc"},
		},
		Adjacencies: []Adjacency{
			{
				Protocol:   "lldp",
				SourceID:   "switch-a",
				SourcePort: "Gi0/1",
				TargetID:   "switch-b",
				TargetPort: "Gi0/1",
			},
		},
		Attachments: []Attachment{
			{DeviceID: "switch-a", IfIndex: 1, EndpointID: "mac:bb:bb:bb:bb:bb:bb", Method: "fdb"},
			{DeviceID: "switch-b", IfIndex: 1, EndpointID: "mac:aa:aa:aa:aa:aa:aa", Method: "fdb"},
			{DeviceID: "switch-a", IfIndex: 1, EndpointID: "mac:70:49:a2:65:72:cd", Method: "fdb"},
			{DeviceID: "switch-b", IfIndex: 2, EndpointID: "mac:70:49:a2:65:72:cd", Method: "fdb"},
			{DeviceID: "switch-b", IfIndex: 2, EndpointID: "mac:ee:ee:ee:ee:ee:ee", Method: "fdb"},
		},
	}

	data := ToTopologyData(result, TopologyDataOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})

	targetLinks := findFDBLinksByEndpointMAC(data.Links, "70:49:a2:65:72:cd")
	require.Len(t, targetLinks, 1)

	segmentActor := findActorByMatch(data.Actors, targetLinks[0].Src.Match)
	require.NotNil(t, segmentActor)
	require.Equal(t, []string{"switch-b"}, segmentActor.Attributes["parent_devices"])
	require.Equal(t, []string{"Gi0/2"}, segmentActor.Attributes["if_names"])
}

func TestToTopologyData_FDBOwnerInferenceUsesSingleMACPortRule(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:       "switch-a",
				Hostname: "switch-a",
			},
			{
				ID:       "switch-b",
				Hostname: "switch-b",
			},
		},
		Interfaces: []Interface{
			{DeviceID: "switch-a", IfIndex: 1, IfName: "Gi0/1"},
			{DeviceID: "switch-a", IfIndex: 2, IfName: "Gi0/2"},
			{DeviceID: "switch-b", IfIndex: 1, IfName: "Gi0/1"},
		},
		Attachments: []Attachment{
			{DeviceID: "switch-a", IfIndex: 1, EndpointID: "mac:dd:dd:dd:dd:dd:dd", Method: "fdb"},
			{DeviceID: "switch-a", IfIndex: 2, EndpointID: "mac:aa:aa:aa:aa:aa:aa", Method: "fdb"},
			{DeviceID: "switch-b", IfIndex: 1, EndpointID: "mac:dd:dd:dd:dd:dd:dd", Method: "fdb"},
			{DeviceID: "switch-b", IfIndex: 1, EndpointID: "mac:bb:bb:bb:bb:bb:bb", Method: "fdb"},
		},
	}

	data := ToTopologyData(result, TopologyDataOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})

	targetLinks := findFDBLinksByEndpointMAC(data.Links, "dd:dd:dd:dd:dd:dd")
	require.Len(t, targetLinks, 1)

	srcActor := findActorByMatch(data.Actors, targetLinks[0].Src.Match)
	require.NotNil(t, srcActor)
	require.Equal(t, "device", srcActor.ActorType)
	require.Equal(t, "switch-a", srcActor.Match.SysName)

	endpointActor := findActorByMAC(data.Actors, "dd:dd:dd:dd:dd:dd")
	require.NotNil(t, endpointActor)
	require.Equal(t, "endpoint", endpointActor.ActorType)
	require.Equal(t, "single_port_mac", endpointActor.Attributes["attachment_source"])
	require.Equal(t, "switch-a", endpointActor.Attributes["attached_device"])
	require.Equal(t, "Gi0/1", endpointActor.Attributes["attached_port"])
	require.Equal(t, "single_port_mac", endpointActor.Labels["attached_by"])
}

func TestToTopologyData_FDBOwnerInferenceSuppressesManagedAliasSwitchFacingPorts(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:        "switch-a",
				Hostname:  "switch-a",
				ChassisID: "aa:aa:aa:aa:aa:aa",
			},
			{
				ID:        "switch-b",
				Hostname:  "switch-b",
				ChassisID: "bb:bb:bb:bb:bb:bb",
			},
		},
		Interfaces: []Interface{
			{DeviceID: "switch-a", IfIndex: 1, IfName: "Gi0/1"},
			{DeviceID: "switch-a", IfIndex: 2, IfName: "Gi0/2"},
			{DeviceID: "switch-b", IfIndex: 1, IfName: "Gi0/1"},
		},
		Attachments: []Attachment{
			// Managed-device aliases learned on the same port mark it as switch-facing.
			{DeviceID: "switch-a", IfIndex: 1, EndpointID: "mac:bb:bb:bb:bb:bb:bb", Method: "fdb"},
			{DeviceID: "switch-b", IfIndex: 1, EndpointID: "mac:aa:aa:aa:aa:aa:aa", Method: "fdb"},

			// Candidate endpoint appears behind the managed-link port and must be suppressed.
			{DeviceID: "switch-a", IfIndex: 1, EndpointID: "mac:dd:dd:dd:dd:dd:dd", Method: "fdb"},
			{DeviceID: "switch-b", IfIndex: 1, EndpointID: "mac:dd:dd:dd:dd:dd:dd", Method: "fdb"},

			// Control endpoint on non-switch-facing port remains directly attachable.
			{DeviceID: "switch-a", IfIndex: 2, EndpointID: "mac:ee:ee:ee:ee:ee:ee", Method: "fdb"},
		},
	}

	data := ToTopologyData(result, TopologyDataOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})

	ddLinks := findFDBLinksByEndpointMAC(data.Links, "dd:dd:dd:dd:dd:dd")
	require.Len(t, ddLinks, 0)
	ddActor := findActorByMAC(data.Actors, "dd:dd:dd:dd:dd:dd")
	require.NotNil(t, ddActor)
	_, hasAttachmentSource := ddActor.Attributes["attachment_source"]
	require.False(t, hasAttachmentSource)
	_, hasAttachedDevice := ddActor.Attributes["attached_device"]
	require.False(t, hasAttachedDevice)

	eeLinks := findFDBLinksByEndpointMAC(data.Links, "ee:ee:ee:ee:ee:ee")
	require.Len(t, eeLinks, 1)
	eeSrc := findActorByMatch(data.Actors, eeLinks[0].Src.Match)
	require.NotNil(t, eeSrc)
	require.Equal(t, "device", eeSrc.ActorType)
	require.Equal(t, "switch-a", eeSrc.Match.SysName)
}

func TestToTopologyData_SuppressesFDBEndpointsOnLLDPPorts(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:        "switch-a",
				Hostname:  "switch-a",
				ChassisID: "aa:aa:aa:aa:aa:aa",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.1")},
			},
			{
				ID:        "host-b",
				Hostname:  "host-b",
				ChassisID: "bb:bb:bb:bb:bb:bb",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.2")},
			},
		},
		Interfaces: []Interface{
			{DeviceID: "switch-a", IfIndex: 1, IfName: "Gi0/1", MAC: "aa:aa:aa:aa:aa:aa"},
			{DeviceID: "host-b", IfIndex: 1, IfName: "eth0", MAC: "bb:bb:bb:bb:bb:bb"},
		},
		Adjacencies: []Adjacency{
			{
				Protocol:   "lldp",
				SourceID:   "switch-a",
				SourcePort: "Gi0/1",
				TargetID:   "host-b",
				TargetPort: "eth0",
			},
		},
		Attachments: []Attachment{
			{DeviceID: "switch-a", IfIndex: 1, EndpointID: "mac:bb:bb:bb:bb:bb:bb", Method: "fdb"},
			{DeviceID: "host-b", IfIndex: 1, EndpointID: "mac:aa:aa:aa:aa:aa:aa", Method: "fdb"},
		},
	}

	data := ToTopologyData(result, TopologyDataOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})

	bridgeLinks := 0
	fdbLinks := 0
	segmentActors := 0
	for _, actor := range data.Actors {
		if actor.ActorType == "segment" {
			segmentActors++
		}
	}
	for _, link := range data.Links {
		switch link.Protocol {
		case "bridge":
			bridgeLinks++
		case "fdb":
			fdbLinks++
		}
	}

	require.Equal(t, 0, segmentActors)
	require.Equal(t, 0, bridgeLinks)
	require.Equal(t, 0, fdbLinks)
	require.Equal(t, 1, data.Stats["links_total"])
	require.Equal(t, 0, data.Stats["links_fdb"])
	require.Equal(t, 0, data.Stats["links_fdb_endpoint_candidates"])
	require.Equal(t, 0, data.Stats["links_fdb_endpoint_emitted"])
	require.Equal(t, 0, data.Stats["links_fdb_endpoint_suppressed"])
}

func TestToTopologyData_KeepsChassisPlaceholderDevicesAsDevices(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:        "switch-a",
				Hostname:  "switch-a",
				ChassisID: "aa:aa:aa:aa:aa:aa",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.1")},
			},
			{
				ID:        "chassis-788cb595dfcc",
				Hostname:  "chassis-788cb595dfcc",
				ChassisID: "78:8c:b5:95:df:cc",
			},
		},
		Adjacencies: []Adjacency{
			{
				Protocol:   "lldp",
				SourceID:   "switch-a",
				SourcePort: "Gi0/1",
				TargetID:   "chassis-788cb595dfcc",
				TargetPort: "eth0",
			},
		},
	}

	data := ToTopologyData(result, TopologyDataOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})

	placeholder := findActorBySysName(data.Actors, "chassis-788cb595dfcc")
	require.NotNil(t, placeholder)
	require.Equal(t, "device", placeholder.ActorType)
}

func TestPruneSegmentArtifacts_SuppressesLLDPDuplicateSegmentPath(t *testing.T) {
	actors := []topology.Actor{
		{
			ActorType: "device",
			Match:     topology.Match{IPAddresses: []string{"10.0.0.1"}, SysName: "switch-a"},
		},
		{
			ActorType: "device",
			Match:     topology.Match{IPAddresses: []string{"10.0.0.2"}, SysName: "switch-b"},
		},
		{
			ActorType: "segment",
			Match:     topology.Match{Hostnames: []string{"segment:dup"}},
		},
	}

	links := []topology.Link{
		{
			Protocol: "lldp",
			Src:      topology.LinkEndpoint{Match: topology.Match{IPAddresses: []string{"10.0.0.1"}}},
			Dst:      topology.LinkEndpoint{Match: topology.Match{IPAddresses: []string{"10.0.0.2"}}},
		},
		{
			Protocol: "bridge",
			Src:      topology.LinkEndpoint{Match: topology.Match{IPAddresses: []string{"10.0.0.1"}}},
			Dst:      topology.LinkEndpoint{Match: topology.Match{Hostnames: []string{"segment:dup"}}},
		},
		{
			Protocol: "bridge",
			Src:      topology.LinkEndpoint{Match: topology.Match{Hostnames: []string{"segment:dup"}}},
			Dst:      topology.LinkEndpoint{Match: topology.Match{IPAddresses: []string{"10.0.0.2"}}},
		},
	}

	filteredActors, filteredLinks, suppressed := pruneSegmentArtifacts(actors, links)
	require.Equal(t, 1, suppressed)
	require.Len(t, filteredActors, 2)
	require.Len(t, filteredLinks, 1)
	require.Equal(t, "lldp", filteredLinks[0].Protocol)
}

func TestPruneSegmentArtifacts_SuppressesCDPDuplicateSegmentPath(t *testing.T) {
	actors := []topology.Actor{
		{
			ActorType: "device",
			Match:     topology.Match{IPAddresses: []string{"10.0.1.1"}, SysName: "switch-a"},
		},
		{
			ActorType: "device",
			Match:     topology.Match{IPAddresses: []string{"10.0.1.2"}, SysName: "switch-b"},
		},
		{
			ActorType: "segment",
			Match:     topology.Match{Hostnames: []string{"segment:dup-cdp"}},
		},
	}

	links := []topology.Link{
		{
			Protocol: "cdp",
			Src:      topology.LinkEndpoint{Match: topology.Match{IPAddresses: []string{"10.0.1.1"}}},
			Dst:      topology.LinkEndpoint{Match: topology.Match{IPAddresses: []string{"10.0.1.2"}}},
		},
		{
			Protocol: "bridge",
			Src:      topology.LinkEndpoint{Match: topology.Match{IPAddresses: []string{"10.0.1.1"}}},
			Dst:      topology.LinkEndpoint{Match: topology.Match{Hostnames: []string{"segment:dup-cdp"}}},
		},
		{
			Protocol: "bridge",
			Src:      topology.LinkEndpoint{Match: topology.Match{Hostnames: []string{"segment:dup-cdp"}}},
			Dst:      topology.LinkEndpoint{Match: topology.Match{IPAddresses: []string{"10.0.1.2"}}},
		},
	}

	filteredActors, filteredLinks, suppressed := pruneSegmentArtifacts(actors, links)
	require.Equal(t, 1, suppressed)
	require.Len(t, filteredActors, 2)
	require.Len(t, filteredLinks, 1)
	require.Equal(t, "cdp", filteredLinks[0].Protocol)
}

func TestPruneSegmentArtifacts_SuppressesSegmentsWithSingleNeighbor(t *testing.T) {
	actors := []topology.Actor{
		{
			ActorType: "device",
			Match:     topology.Match{IPAddresses: []string{"10.0.0.1"}, SysName: "router-a"},
		},
		{
			ActorType: "segment",
			Match:     topology.Match{Hostnames: []string{"segment:orphan"}},
		},
	}

	links := []topology.Link{
		{
			Protocol: "bridge",
			Src:      topology.LinkEndpoint{Match: topology.Match{IPAddresses: []string{"10.0.0.1"}}},
			Dst:      topology.LinkEndpoint{Match: topology.Match{Hostnames: []string{"segment:orphan"}}},
		},
	}

	filteredActors, filteredLinks, suppressed := pruneSegmentArtifacts(actors, links)
	require.Equal(t, 1, suppressed)
	require.Len(t, filteredActors, 1)
	require.Len(t, filteredLinks, 0)
}

func TestToTopologyData_DeterministicTransitRuleSuppressesFDBOnLLDPPortInExperimental(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:        "switch-a",
				Hostname:  "switch-a",
				ChassisID: "aa:aa:aa:aa:aa:aa",
			},
			{
				ID:        "switch-b",
				Hostname:  "switch-b",
				ChassisID: "bb:bb:bb:bb:bb:bb",
			},
		},
		Interfaces: []Interface{
			{DeviceID: "switch-a", IfIndex: 1, IfName: "Gi0/1", IfDescr: "Gi0/1"},
			{DeviceID: "switch-b", IfIndex: 2, IfName: "Gi0/2", IfDescr: "Gi0/2"},
		},
		Adjacencies: []Adjacency{
			{
				Protocol:   "lldp",
				SourceID:   "switch-a",
				SourcePort: "Gi0/1",
				TargetID:   "switch-b",
				TargetPort: "Gi0/2",
			},
		},
		Attachments: []Attachment{
			{DeviceID: "switch-a", IfIndex: 1, EndpointID: "mac:00:00:00:00:00:11", Method: "fdb"},
		},
	}

	data := ToTopologyData(result, TopologyDataOptions{
		Source:            "snmp",
		Layer:             "2",
		View:              "summary",
		InferenceStrategy: topologyInferenceStrategyFDBMinimumKnowledge,
	})

	require.Equal(t, topologyInferenceStrategyFDBMinimumKnowledge, data.Stats["inference_strategy"])
	require.Equal(t, 0, data.Stats["links_fdb_endpoint_emitted"])
	require.Nil(t, findActorByType(data.Actors, "segment"))
}

func TestToTopologyData_DeterministicTransitRuleMatchesNumericLLDPPortToIfIndex(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:        "switch-a",
				Hostname:  "switch-a",
				ChassisID: "aa:aa:aa:aa:aa:aa",
			},
			{
				ID:        "switch-b",
				Hostname:  "switch-b",
				ChassisID: "bb:bb:bb:bb:bb:bb",
			},
		},
		Interfaces: []Interface{
			{DeviceID: "switch-a", IfIndex: 2, IfName: "GigabitEthernet2", IfDescr: "GigabitEthernet2"},
			{DeviceID: "switch-b", IfIndex: 4, IfName: "ether4", IfDescr: "ether4"},
		},
		Adjacencies: []Adjacency{
			{
				Protocol:   "lldp",
				SourceID:   "switch-a",
				SourcePort: "2",
				TargetID:   "switch-b",
				TargetPort: "ether4",
			},
		},
		Attachments: []Attachment{
			{DeviceID: "switch-a", IfIndex: 2, EndpointID: "mac:00:00:00:00:00:11", Method: "fdb"},
		},
	}

	data := ToTopologyData(result, TopologyDataOptions{
		Source:            "snmp",
		Layer:             "2",
		View:              "summary",
		InferenceStrategy: topologyInferenceStrategyFDBMinimumKnowledge,
	})

	require.Equal(t, 0, data.Stats["links_fdb_endpoint_emitted"])
	require.Nil(t, findActorByType(data.Actors, "segment"))
	lldpLink := findLinkByProtocol(data.Links, "lldp")
	require.NotNil(t, lldpLink)
	require.Equal(t, 2, lldpLink.Src.Attributes["if_index"])
	require.Equal(t, "GigabitEthernet2", lldpLink.Src.Attributes["if_name"])
	require.Equal(t, "2", lldpLink.Src.Attributes["port_id"])
	require.Equal(t, 4, lldpLink.Dst.Attributes["if_index"])
	require.Equal(t, "ether4", lldpLink.Dst.Attributes["if_name"])
	require.Equal(t, "ether4", lldpLink.Dst.Attributes["port_id"])
}

func TestToTopologyData_SwitchFacingPortDoesNotSuppressEndpointOwnership(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:        "switch-a",
				Hostname:  "switch-a",
				ChassisID: "aa:aa:aa:aa:aa:aa",
			},
			{
				ID:        "switch-b",
				Hostname:  "switch-b",
				ChassisID: "bb:bb:bb:bb:bb:bb",
			},
		},
		Interfaces: []Interface{
			{DeviceID: "switch-a", IfIndex: 1, IfName: "Gi0/1", MAC: "aa:aa:aa:aa:aa:aa"},
			{DeviceID: "switch-b", IfIndex: 1, IfName: "Gi0/1", MAC: "bb:bb:bb:bb:bb:bb"},
		},
		Attachments: []Attachment{
			// Reciprocal managed-alias observations make this a switch-facing bridge pair.
			{DeviceID: "switch-a", IfIndex: 1, EndpointID: "mac:bb:bb:bb:bb:bb:bb", Method: "fdb"},
			{DeviceID: "switch-b", IfIndex: 1, EndpointID: "mac:aa:aa:aa:aa:aa:aa", Method: "fdb"},
			// Host endpoint learned on the same switch-facing port.
			{DeviceID: "switch-a", IfIndex: 1, EndpointID: "mac:00:00:00:00:00:11", Method: "fdb"},
		},
	}

	data := ToTopologyData(result, TopologyDataOptions{
		Source:            "snmp",
		Layer:             "2",
		View:              "summary",
		InferenceStrategy: topologyInferenceStrategyFDBPairwise,
	})

	actor := findActorBySysName(data.Actors, "switch-a")
	require.NotNil(t, actor)

	statuses, ok := actor.Attributes["if_statuses"].([]map[string]any)
	require.True(t, ok)
	port1 := findInterfaceStatusByIndex(statuses, 1)
	require.Equal(t, "switch_facing", port1["topology_role"])

	targetLinks := findFDBLinksByEndpointMAC(data.Links, "00:00:00:00:00:11")
	require.Len(t, targetLinks, 1)
	segmentActor := findActorByMatch(data.Actors, targetLinks[0].Src.Match)
	require.NotNil(t, segmentActor)
	parentDevices, ok := segmentActor.Attributes["parent_devices"].([]string)
	require.True(t, ok)
	require.Contains(t, parentDevices, "switch-a")
	ifNames, ok := segmentActor.Attributes["if_names"].([]string)
	require.True(t, ok)
	require.Contains(t, ifNames, "Gi0/1")
}

func TestSuppressInferredBridgeLinksOnDeterministicDiscovery(t *testing.T) {
	deterministic := make(map[string]struct{})
	addBridgePortObservationKeys(deterministic, bridgePortRef{
		deviceID: "switch-a",
		ifIndex:  1,
		ifName:   "Gi0/1",
	})
	discoveryPairs := map[string]struct{}{
		topologyUndirectedPairKey("switch-a", "switch-b"): {},
	}

	links := []bridgeBridgeLinkRecord{
		{
			designatedPort: bridgePortRef{deviceID: "switch-a", ifIndex: 1, ifName: "Gi0/1"},
			port:           bridgePortRef{deviceID: "switch-b", ifIndex: 1, ifName: "Gi0/1"},
			method:         "lldp",
		},
		{
			designatedPort: bridgePortRef{deviceID: "switch-a", ifIndex: 2, ifName: "Gi0/2"},
			port:           bridgePortRef{deviceID: "switch-b", ifIndex: 2, ifName: "Gi0/2"},
			method:         "fdb_pairwise",
		},
		{
			designatedPort: bridgePortRef{deviceID: "switch-a", ifIndex: 1, ifName: "Gi0/1"},
			port:           bridgePortRef{deviceID: "switch-c", ifIndex: 7, ifName: "Gi0/7"},
			method:         "stp",
		},
		{
			designatedPort: bridgePortRef{deviceID: "switch-a", ifIndex: 3, ifName: "Gi0/3"},
			port:           bridgePortRef{deviceID: "switch-c", ifIndex: 3, ifName: "Gi0/3"},
			method:         "fdb_pairwise",
		},
	}

	filtered := suppressInferredBridgeLinksOnDeterministicDiscovery(links, deterministic, discoveryPairs)
	require.Len(t, filtered, 2)
	require.Equal(t, "lldp", filtered[0].method)
	require.Equal(t, "fdb_pairwise", filtered[1].method)
	require.Equal(t, "switch-c", filtered[1].port.deviceID)
}

func TestToTopologyData_FDBOwnerInferenceUsesReporterMatrixRule(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:        "switch-a",
				Hostname:  "switch-a",
				ChassisID: "aa:aa:aa:aa:aa:aa",
			},
			{
				ID:        "switch-b",
				Hostname:  "switch-b",
				ChassisID: "bb:bb:bb:bb:bb:bb",
			},
			{
				ID:        "switch-c",
				Hostname:  "switch-c",
				ChassisID: "cc:cc:cc:cc:cc:cc",
			},
		},
		Interfaces: []Interface{
			{DeviceID: "switch-a", IfIndex: 1, IfName: "Gi0/1", MAC: "aa:aa:aa:aa:aa:aa"},
			{DeviceID: "switch-b", IfIndex: 1, IfName: "Gi0/1", MAC: "bb:bb:bb:bb:bb:bb"},
			{DeviceID: "switch-b", IfIndex: 2, IfName: "Gi0/2", MAC: "bb:bb:bb:bb:bb:bc"},
			{DeviceID: "switch-c", IfIndex: 1, IfName: "Gi0/1", MAC: "cc:cc:cc:cc:cc:cc"},
			{DeviceID: "switch-c", IfIndex: 2, IfName: "Gi0/2", MAC: "cc:cc:cc:cc:cc:cd"},
		},
		Attachments: []Attachment{
			{DeviceID: "switch-a", IfIndex: 1, EndpointID: "mac:bb:bb:bb:bb:bb:bb", Method: "fdb"},
			{DeviceID: "switch-a", IfIndex: 1, EndpointID: "mac:cc:cc:cc:cc:cc:cc", Method: "fdb"},
			{DeviceID: "switch-a", IfIndex: 1, EndpointID: "mac:dd:dd:dd:dd:dd:dd", Method: "fdb"},
			{DeviceID: "switch-b", IfIndex: 1, EndpointID: "mac:aa:aa:aa:aa:aa:aa", Method: "fdb"},
			{DeviceID: "switch-b", IfIndex: 2, EndpointID: "mac:cc:cc:cc:cc:cc:cc", Method: "fdb"},
			{DeviceID: "switch-b", IfIndex: 2, EndpointID: "mac:dd:dd:dd:dd:dd:dd", Method: "fdb"},
			{DeviceID: "switch-c", IfIndex: 1, EndpointID: "mac:aa:aa:aa:aa:aa:aa", Method: "fdb"},
			{DeviceID: "switch-c", IfIndex: 1, EndpointID: "mac:bb:bb:bb:bb:bb:bb", Method: "fdb"},
			{DeviceID: "switch-c", IfIndex: 2, EndpointID: "mac:dd:dd:dd:dd:dd:dd", Method: "fdb"},
			{DeviceID: "switch-c", IfIndex: 2, EndpointID: "mac:ee:ee:ee:ee:ee:ee", Method: "fdb"},
		},
	}

	data := ToTopologyData(result, TopologyDataOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})

	targetLinks := findFDBLinksByEndpointMAC(data.Links, "dd:dd:dd:dd:dd:dd")
	require.Len(t, targetLinks, 1)

	segmentActor := findActorByMatch(data.Actors, targetLinks[0].Src.Match)
	require.NotNil(t, segmentActor)
	require.Equal(t, []string{"switch-c"}, segmentActor.Attributes["parent_devices"])
	require.Equal(t, []string{"Gi0/2"}, segmentActor.Attributes["if_names"])
}

func findInterfaceStatusByIndex(statuses []map[string]any, ifIndex int) map[string]any {
	for _, status := range statuses {
		value, ok := status["if_index"].(int)
		if ok && value == ifIndex {
			return status
		}
	}
	return nil
}

func findNeighborByProtocol(neighbors []map[string]any, protocol string) map[string]any {
	for _, neighbor := range neighbors {
		value, ok := neighbor["protocol"].(string)
		if ok && strings.EqualFold(value, protocol) {
			return neighbor
		}
	}
	return nil
}

func findFDBLinksByEndpointMAC(links []topology.Link, mac string) []topology.Link {
	out := make([]topology.Link, 0)
	for _, link := range links {
		if link.Protocol != "fdb" {
			continue
		}
		for _, candidate := range link.Dst.Match.MacAddresses {
			if candidate == mac {
				out = append(out, link)
				break
			}
		}
	}
	return out
}

func findFDBLinksByEndpointIP(links []topology.Link, ip string) []topology.Link {
	out := make([]topology.Link, 0)
	for _, link := range links {
		if link.Protocol != "fdb" {
			continue
		}
		for _, candidate := range link.Dst.Match.IPAddresses {
			if candidate == ip {
				out = append(out, link)
				break
			}
		}
	}
	return out
}

func topologyLinkSignatures(links []topology.Link) map[string]struct{} {
	out := make(map[string]struct{}, len(links))
	for _, link := range links {
		srcKey := canonicalTopologyMatchKey(link.Src.Match)
		dstKey := canonicalTopologyMatchKey(link.Dst.Match)
		if srcKey == "" || dstKey == "" {
			continue
		}
		key := strings.Join([]string{
			strings.ToLower(strings.TrimSpace(link.Protocol)),
			strings.ToLower(strings.TrimSpace(link.Direction)),
			srcKey,
			dstKey,
			strings.ToLower(strings.TrimSpace(link.State)),
			strings.ToLower(topologyMetricString(link.Metrics, "attachment_mode")),
		}, keySep)
		out[key] = struct{}{}
	}
	return out
}

func findFDBLinksByDstSysName(links []topology.Link, sysName string) []topology.Link {
	out := make([]topology.Link, 0)
	for _, link := range links {
		if link.Protocol != "fdb" {
			continue
		}
		if link.Dst.Match.SysName != sysName {
			continue
		}
		out = append(out, link)
	}
	return out
}

func findActorByMatch(actors []topology.Actor, match topology.Match) *topology.Actor {
	target := canonicalTopologyMatchKey(match)
	if target == "" {
		return nil
	}
	for i := range actors {
		if canonicalTopologyMatchKey(actors[i].Match) == target {
			return &actors[i]
		}
	}
	return nil
}

func findActorBySysName(actors []topology.Actor, sysName string) *topology.Actor {
	for i := range actors {
		if actors[i].Match.SysName == sysName {
			return &actors[i]
		}
	}
	return nil
}

func findActorByMAC(actors []topology.Actor, mac string) *topology.Actor {
	for i := range actors {
		for _, candidate := range actors[i].Match.MacAddresses {
			if candidate == mac {
				return &actors[i]
			}
		}
	}
	return nil
}

func findActorByIP(actors []topology.Actor, ip string) *topology.Actor {
	for i := range actors {
		for _, candidate := range actors[i].Match.IPAddresses {
			if candidate == ip {
				return &actors[i]
			}
		}
	}
	return nil
}

func findActorByType(actors []topology.Actor, actorType string) *topology.Actor {
	for i := range actors {
		if actors[i].ActorType == actorType {
			return &actors[i]
		}
	}
	return nil
}

func findLinkByProtocol(links []topology.Link, protocol string) *topology.Link {
	for i := range links {
		if links[i].Protocol == protocol {
			return &links[i]
		}
	}
	return nil
}
