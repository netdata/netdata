// SPDX-License-Identifier: GPL-3.0-or-later

package projector

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/l2topology/internal/model"
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

func TestBuildTopologyPortNeighborStatus_PreservesTextualPortBeforeIfIndex(t *testing.T) {
	tests := map[string]struct {
		adj  model.Adjacency
		want string
	}{
		"observed-interface-name": {
			adj: model.Adjacency{
				TargetPort: "remote-port-id",
				TargetPortEvidence: model.AdjacencyPortEvidence{
					IfIndex: 7,
					IfName:  "Ethernet7",
				},
			},
			want: "Ethernet7",
		},
		"raw-protocol-port": {
			adj: model.Adjacency{
				TargetPort: "Ethernet2",
				TargetPortEvidence: model.AdjacencyPortEvidence{
					IfIndex: 2,
				},
			},
			want: "Ethernet2",
		},
		"bridge-base-port": {
			adj: model.Adjacency{
				TargetPortEvidence: model.AdjacencyPortEvidence{
					IfIndex:    2,
					BridgePort: "24",
				},
			},
			want: "24",
		},
		"ifindex-fallback": {
			adj: model.Adjacency{
				TargetPortEvidence: model.AdjacencyPortEvidence{
					IfIndex: 2,
				},
			},
			want: "2",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := buildTopologyPortNeighborStatus("cdp", tc.adj, nil)
			require.Equal(t, tc.want, got.RemotePort)
		})
	}
}

func TestBuildTopologyDevicePortDetail_RendersOptionalFields(t *testing.T) {
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
		VLANs: []model.ProjectionPortVLAN{
			{VLANID: "100", Tagged: true},
			{VLANID: "200", Tagged: true, VLANName: "servers"},
		},
		TopologyRole:   "switch_facing",
		RoleConfidence: "high",
		RoleSources:    []string{"peer_link", "bridge_link"},
		FDBMACCount:    3,
		STPState:       "forwarding",
		Neighbors: []topologyPortNeighborStatus{
			{},
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

	detail := buildTopologyDevicePortDetail(status)
	require.Equal(t, model.OptionalValue[int]{Value: 7, Has: true}, detail.IfIndex)
	require.Empty(t, detail.PortID)
	require.Equal(t, "Gi0/7", detail.Name)
	require.Equal(t, "Gi0/7", detail.IfName)
	require.Equal(t, "Uplink", detail.IfDescr)
	require.Equal(t, "core", detail.IfAlias)
	require.Equal(t, "00:11:22:33:44:55", detail.MAC)
	require.Equal(t, model.OptionalValue[int64]{Value: 1000000000, Has: true}, detail.Speed)
	require.Equal(t, "12345", detail.LastChange)
	require.Equal(t, "full", detail.Duplex)
	require.Equal(t, "trunk", detail.LinkMode)
	require.Equal(t, "high", detail.LinkModeConfidence)
	require.Equal(t, []string{"fdb", "stp"}, detail.LinkModeSources)
	require.Equal(t, []string{"100", "200"}, detail.VLANIDs)
	require.Equal(t, "switch_facing", detail.TopologyRole)
	require.Equal(t, "high", detail.TopologyRoleConfidence)
	require.Equal(t, []string{"peer_link", "bridge_link"}, detail.TopologyRoleSources)
	require.Equal(t, model.OptionalValue[int]{Value: 3, Has: true}, detail.FDBMACCount)
	require.Equal(t, "forwarding", detail.STPState)
	require.Equal(t, "up", detail.AdminStatus)
	require.Equal(t, "up", detail.OperStatus)
	require.Equal(t, "ethernetCsmacd", detail.PortType)
	require.Equal(t, model.OptionalValue[int]{Value: 1, Has: true}, detail.NeighborCount)

	require.Len(t, detail.Neighbors, 1)
	require.Equal(t, "lldp", detail.Neighbors[0].Protocol)
	require.Equal(t, "switch-b", detail.Neighbors[0].RemoteDevice)
	require.Equal(t, "Gi0/1", detail.Neighbors[0].RemotePort)
	require.Equal(t, "10.0.0.2", detail.Neighbors[0].RemoteIP)
	require.Equal(t, "aa:bb:cc:dd:ee:ff", detail.Neighbors[0].RemoteChassisID)
	require.Equal(t, []string{"bridge", "router"}, detail.Neighbors[0].RemoteCapabilities)

	require.Len(t, detail.VLANs, 2)
	require.Equal(t, "100", detail.VLANs[0].VLANID)
	require.True(t, detail.VLANs[0].Tagged)
	require.Equal(t, "200", detail.VLANs[1].VLANID)
	require.Equal(t, "servers", detail.VLANs[1].VLANName)
}
