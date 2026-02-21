// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildL3ResultFromObservations_OSPFPairs(t *testing.T) {
	observations := []L3Observation{
		{
			DeviceID:     "node:mumbai",
			Hostname:     "mumbai",
			ManagementIP: "10.20.4.1",
			OSPFElement:  &OSPFElementObservation{RouterID: "192.168.5.21", AdminState: 1, VersionNumber: 2},
			Interfaces: []ObservedInterface{
				{IfIndex: 978, IfName: "ge-0/1/1.0", IfDescr: "ge-0/1/1.0"},
			},
			OSPFIfTable: []OSPFInterfaceObservation{
				{IP: "192.168.5.21", IfIndex: 978, IfName: "ge-0/1/1.0", Netmask: "255.255.255.252", AreaID: "0.0.0.0"},
			},
			OSPFNbrTable: []OSPFNeighborObservation{
				{RemoteIP: "192.168.5.22", RemoteRouterID: "192.168.5.22"},
			},
		},
		{
			DeviceID:     "node:mysore",
			Hostname:     "mysore",
			ManagementIP: "10.20.4.2",
			OSPFElement:  &OSPFElementObservation{RouterID: "192.168.5.22", AdminState: 1, VersionNumber: 2},
			Interfaces: []ObservedInterface{
				{IfIndex: 508, IfName: "ge-0/0/1.0", IfDescr: "ge-0/0/1.0"},
			},
			OSPFIfTable: []OSPFInterfaceObservation{
				{IP: "192.168.5.22", IfIndex: 508, IfName: "ge-0/0/1.0", Netmask: "255.255.255.252", AreaID: "0.0.0.0"},
			},
			OSPFNbrTable: []OSPFNeighborObservation{
				{RemoteIP: "192.168.5.21", RemoteRouterID: "192.168.5.21"},
			},
		},
	}

	result, err := BuildL3ResultFromObservations(observations)
	require.NoError(t, err)
	require.Len(t, result.Devices, 2)
	require.Len(t, result.Adjacencies, 2)
	require.Equal(t, "ospf", result.Adjacencies[0].Protocol)
	require.Equal(t, "ospf", result.Adjacencies[1].Protocol)
	require.Equal(t, "node:mumbai", result.Adjacencies[0].SourceID)
	require.Equal(t, "node:mysore", result.Adjacencies[0].TargetID)
	require.Equal(t, "source", result.Adjacencies[0].Labels[adjacencyLabelPairSide])
	require.Equal(t, "target", result.Adjacencies[1].Labels[adjacencyLabelPairSide])
	require.Equal(t, result.Adjacencies[0].Labels[adjacencyLabelPairID], result.Adjacencies[1].Labels[adjacencyLabelPairID])
}

func TestBuildL3ResultFromObservations_ISISPairs(t *testing.T) {
	observations := []L3Observation{
		{
			DeviceID: "node:delhi",
			ISISElement: &ISISElementObservation{
				SysID:      "49.0001.1921.6800.1001.00",
				AdminState: 1,
			},
			Interfaces: []ObservedInterface{
				{IfIndex: 510, IfName: "ge-0/0/2.0"},
			},
			ISISCircTable: []ISISCircuitObservation{
				{CircIndex: 7, IfIndex: 510, AdminState: 1},
			},
			ISISAdjTable: []ISISAdjacencyObservation{
				{
					CircIndex:       7,
					AdjIndex:        42,
					NeighborSysID:   "49.0001.1921.6800.1002.00",
					NeighborSNPA:    "00:11:22:33:44:55",
					NeighborSysType: 3,
				},
			},
		},
		{
			DeviceID: "node:bangalore",
			ISISElement: &ISISElementObservation{
				SysID:      "49.0001.1921.6800.1002.00",
				AdminState: 1,
			},
			Interfaces: []ObservedInterface{
				{IfIndex: 2397, IfName: "ge-0/0/1.0"},
			},
			ISISCircTable: []ISISCircuitObservation{
				{CircIndex: 9, IfIndex: 2397, AdminState: 1},
			},
			ISISAdjTable: []ISISAdjacencyObservation{
				{
					CircIndex:       9,
					AdjIndex:        42,
					NeighborSysID:   "49.0001.1921.6800.1001.00",
					NeighborSNPA:    "00:aa:bb:cc:dd:ee",
					NeighborSysType: 3,
				},
			},
		},
	}

	result, err := BuildL3ResultFromObservations(observations)
	require.NoError(t, err)
	require.Len(t, result.Devices, 2)
	require.Len(t, result.Adjacencies, 2)
	require.Equal(t, "isis", result.Adjacencies[0].Protocol)
	require.Equal(t, "isis", result.Adjacencies[1].Protocol)
	require.Equal(t, result.Adjacencies[0].Labels[adjacencyLabelPairID], result.Adjacencies[1].Labels[adjacencyLabelPairID])
	require.Equal(t, "isis:adjindex-sysid", result.Adjacencies[0].Labels[adjacencyLabelPairPass])
}

func TestBuildL3ResultFromObservations_EmptyInput(t *testing.T) {
	_, err := BuildL3ResultFromObservations(nil)
	require.Error(t, err)
}
