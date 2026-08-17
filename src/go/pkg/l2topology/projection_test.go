// SPDX-License-Identifier: GPL-3.0-or-later

package l2topology

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOptionalValueDistinguishesAbsentAndPresentZero(t *testing.T) {
	tests := map[string]struct {
		value OptionalValue[int]
		has   bool
		want  int
	}{
		"absent": {
			value: OptionalValue[int]{},
			has:   false,
			want:  0,
		},
		"present zero": {
			value: OptionalValue[int]{Value: 0, Has: true},
			has:   true,
			want:  0,
		},
		"present value": {
			value: OptionalValue[int]{Value: 42, Has: true},
			has:   true,
			want:  42,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tt.has, tt.value.Has)
			require.Equal(t, tt.want, tt.value.Value)
		})
	}
}

func TestProjectionPreservesSelectedManagementIPAfterARPAliasReconciliation(t *testing.T) {
	result, err := BuildL2ResultFromObservations([]L2Observation{
		{
			DeviceID:     "switch-a",
			Hostname:     "switch-a",
			ManagementIP: "192.0.2.1",
			ChassisID:    "00:11:22:33:44:55",
			LLDPRemotes: []LLDPRemoteObservation{
				{
					LocalPortNum: "1",
					LocalPortID:  "Gi0/1",
					ChassisID:    "aa:bb:cc:dd:ee:ff",
					SysName:      "switch-b",
					PortID:       "Gi0/2",
					ManagementIP: "192.0.2.20",
				},
			},
			ARPNDEntries: []ARPNDObservation{
				{
					Protocol: "arp",
					IfIndex:  1,
					IfName:   "Gi0/1",
					IP:       "10.0.0.1",
					MAC:      "aa:bb:cc:dd:ee:ff",
					State:    "reachable",
					AddrType: "ipv4",
				},
			},
		},
	}, DiscoverOptions{EnableLLDP: true, EnableARP: true})
	require.NoError(t, err)
	require.Len(t, result.Devices, 2)

	projection := ToGraph(result, GraphOptions{
		SchemaVersion: "1.0.0",
		Source:        "snmp",
		Layer:         "2",
		AgentID:       "agent-1",
		LocalDeviceID: "switch-a",
	})

	var remoteActorID string
	for _, actor := range projection.Graph.Actors {
		if actor.Match.SysName == "switch-b" {
			remoteActorID = actor.ActorID
			break
		}
	}
	require.NotEmpty(t, remoteActorID)
	require.Equal(t, "192.0.2.20", projection.ActorDetails[remoteActorID].Device.ManagementIP)

	require.Len(t, projection.Graph.Links, 1)
	require.Equal(t, "192.0.2.20", projection.Graph.Links[0].Dst.ManagementIP)
}

func TestProjectionDoesNotCollapseSelectedIPOwnerWithConflictingARPAlias(t *testing.T) {
	result, err := BuildL2ResultFromObservations([]L2Observation{
		{
			DeviceID:     "switch-a",
			Hostname:     "switch-a",
			ManagementIP: "192.0.2.1",
			ChassisID:    "00:11:22:33:44:55",
			ARPNDEntries: []ARPNDObservation{
				{
					Protocol: "arp",
					IfIndex:  1,
					IfName:   "Gi0/1",
					IP:       "192.0.2.2",
					MAC:      "00:11:22:33:44:55",
					State:    "reachable",
					AddrType: "ipv4",
				},
			},
		},
		{
			DeviceID:     "switch-b",
			Hostname:     "switch-b",
			ManagementIP: "192.0.2.2",
			ChassisID:    "aa:bb:cc:dd:ee:ff",
		},
	}, DiscoverOptions{EnableARP: true})
	require.NoError(t, err)

	projection := ToGraph(result, GraphOptions{
		SchemaVersion:      "1.0.0",
		Source:             "snmp",
		Layer:              "2",
		AgentID:            "agent-1",
		LocalDeviceID:      "switch-a",
		CollapseActorsByIP: true,
	})

	require.Len(t, projection.Graph.Actors, 2)
}

func TestProjectionDoesNotReintroduceRejectedCDPAddressAsEndpointIdentity(t *testing.T) {
	result, err := BuildL2ResultFromObservations([]L2Observation{
		{
			DeviceID: "target-a",
			Hostname: "target-a",
		},
		{
			DeviceID:     "target-b",
			Hostname:     "target-b",
			ManagementIP: "192.0.2.2",
		},
		{
			DeviceID:     "observer",
			Hostname:     "observer",
			ManagementIP: "192.0.2.3",
			Interfaces: []ObservedInterface{{
				IfIndex: 1,
				IfName:  "Gi0/1",
			}},
			CDPRemotes: []CDPRemoteObservation{{
				LocalIfIndex: 1,
				LocalIfName:  "Gi0/1",
				DeviceID:     "target-a",
				SysName:      "target-a",
				DevicePort:   "Gi0/2",
				Address:      "192.0.2.2",
			}},
		},
	}, DiscoverOptions{EnableCDP: true})
	require.NoError(t, err)

	projection := ToGraph(result, GraphOptions{
		SchemaVersion: "1.0.0",
		Source:        "snmp",
		Layer:         "2",
		AgentID:       "agent-1",
		LocalDeviceID: "observer",
	})

	var targetAID, targetBID, observerID string
	for _, actor := range projection.Graph.Actors {
		switch actor.Match.SysName {
		case "target-a":
			targetAID = actor.ActorID
		case "target-b":
			targetBID = actor.ActorID
		case "observer":
			observerID = actor.ActorID
		}
	}
	require.NotEmpty(t, targetAID)
	require.NotEmpty(t, targetBID)
	require.NotEqual(t, targetAID, targetBID)
	require.Len(t, projection.Graph.Links, 1)
	require.Equal(t, targetAID, projection.Graph.Links[0].DstActorID)
	require.Empty(t, projection.Graph.Links[0].Dst.Match.IPAddresses)
	require.Empty(t, projection.Graph.Links[0].Dst.ManagementIP)

	observerDetail := projection.ActorDetails[observerID].Device
	require.Len(t, observerDetail.Ports, 1)
	require.Len(t, observerDetail.Ports[0].Neighbors, 1)
	require.Empty(t, observerDetail.Ports[0].Neighbors[0].RemoteIP)
}
