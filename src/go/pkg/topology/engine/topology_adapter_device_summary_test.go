// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSortedTopologyPortNeighbors_NormalizesAndOrders(t *testing.T) {
	neighbors := map[string]topologyPortNeighborStatus{
		"b": {
			Protocol:           " CDP ",
			RemoteDevice:       " switch-b ",
			RemotePort:         " Gi0/2 ",
			RemoteIP:           " 10.0.0.2 ",
			RemoteChassisID:    "00:11:22:33:44:55",
			RemoteCapabilities: []string{"router", "bridge", "router"},
		},
		"a": {
			Protocol:           " lldp ",
			RemoteDevice:       " switch-a ",
			RemotePort:         " Gi0/1 ",
			RemoteIP:           " 10.0.0.1 ",
			RemoteChassisID:    " aa:bb:cc:dd:ee:ff ",
			RemoteCapabilities: []string{"bridge", "router", "bridge"},
		},
		"empty": {},
	}

	sorted := sortedTopologyPortNeighbors(neighbors)
	require.Len(t, sorted, 2)

	require.Equal(t, "cdp", sorted[0].Protocol)
	require.Equal(t, "switch-b", sorted[0].RemoteDevice)
	require.Equal(t, "Gi0/2", sorted[0].RemotePort)
	require.Equal(t, "10.0.0.2", sorted[0].RemoteIP)
	require.Equal(t, "00:11:22:33:44:55", sorted[0].RemoteChassisID)
	require.Equal(t, []string{"bridge", "router"}, sorted[0].RemoteCapabilities)

	require.Equal(t, "lldp", sorted[1].Protocol)
	require.Equal(t, "switch-a", sorted[1].RemoteDevice)
	require.Equal(t, "Gi0/1", sorted[1].RemotePort)
	require.Equal(t, "10.0.0.1", sorted[1].RemoteIP)
	require.Equal(t, "aa:bb:cc:dd:ee:ff", sorted[1].RemoteChassisID)
	require.Equal(t, []string{"bridge", "router"}, sorted[1].RemoteCapabilities)
}

func TestBuildTopologyDevicePortStatusAttributes_RendersOptionalFields(t *testing.T) {
	status := topologyDevicePortStatus{
		IfIndex:        7,
		IfName:         "Gi0/7",
		IfDescr:        "Uplink",
		IfAlias:        "core",
		MAC:            "00:11:22:33:44:55",
		SpeedBps:       1000000000,
		LastChange:     12345,
		Duplex:         "full",
		InterfaceType:  "ethernetCsmacd",
		AdminStatus:    "up",
		OperStatus:     "up",
		LinkMode:       "trunk",
		ModeConfidence: "high",
		ModeSources:    []string{"fdb", "stp"},
		VLANIDs:        []string{"100", "200"},
		VLANs: []map[string]any{
			{"vlan_id": "100", "tagged": true},
			{"vlan_id": "200", "tagged": true, "vlan_name": "servers"},
		},
		TopologyRole:   "switch_facing",
		RoleConfidence: "high",
		RoleSources:    []string{"peer_link", "bridge_link"},
		FDBMACCount:    3,
		STPState:       "forwarding",
		Neighbors: []topologyPortNeighborStatus{
			{
				Protocol:           "lldp",
				RemoteDevice:       "switch-b",
				RemotePort:         "Gi0/1",
				RemoteIP:           "10.0.0.2",
				RemoteChassisID:    "aa:bb:cc:dd:ee:ff",
				RemoteCapabilities: []string{"bridge", "router"},
			},
		},
	}

	attrs := buildTopologyDevicePortStatusAttributes(status)
	require.Equal(t, 7, attrs["if_index"])
	require.Equal(t, "Gi0/7", attrs["if_name"])
	require.Equal(t, "Uplink", attrs["if_descr"])
	require.Equal(t, "core", attrs["if_alias"])
	require.Equal(t, "00:11:22:33:44:55", attrs["mac"])
	require.Equal(t, int64(1000000000), attrs["speed"])
	require.Equal(t, int64(12345), attrs["last_change"])
	require.Equal(t, "full", attrs["duplex"])
	require.Equal(t, "trunk", attrs["link_mode"])
	require.Equal(t, "high", attrs["link_mode_confidence"])
	require.Equal(t, []string{"fdb", "stp"}, attrs["link_mode_sources"])
	require.Equal(t, []string{"100", "200"}, attrs["vlan_ids"])
	require.Equal(t, "switch_facing", attrs["topology_role"])
	require.Equal(t, "high", attrs["topology_role_confidence"])
	require.Equal(t, []string{"peer_link", "bridge_link"}, attrs["topology_role_sources"])
	require.Equal(t, 3, attrs["fdb_mac_count"])
	require.Equal(t, "forwarding", attrs["stp_state"])
	require.Equal(t, "up", attrs["admin_status"])
	require.Equal(t, "up", attrs["oper_status"])
	require.Equal(t, "ethernetCsmacd", attrs["if_type"])

	neighbors, ok := attrs["neighbors"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, neighbors, 1)
	require.Equal(t, "lldp", neighbors[0]["protocol"])
	require.Equal(t, "switch-b", neighbors[0]["remote_device"])
	require.Equal(t, "Gi0/1", neighbors[0]["remote_port"])
	require.Equal(t, "10.0.0.2", neighbors[0]["remote_ip"])
	require.Equal(t, "aa:bb:cc:dd:ee:ff", neighbors[0]["remote_chassis_id"])
	require.Equal(t, []string{"bridge", "router"}, neighbors[0]["remote_capabilities"])

	vlans, ok := attrs["vlans"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, vlans, 2)
	require.Equal(t, "100", vlans[0]["vlan_id"])
	require.Equal(t, true, vlans[0]["tagged"])
	require.Equal(t, "200", vlans[1]["vlan_id"])
	require.Equal(t, "servers", vlans[1]["vlan_name"])
}
