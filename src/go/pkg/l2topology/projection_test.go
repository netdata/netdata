// SPDX-License-Identifier: GPL-3.0-or-later

package l2topology

import (
	"fmt"
	"slices"
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

func TestProjectionKeepsLinkEndpointIPHintsConstantSized(t *testing.T) {
	const (
		aliasCount = 256
		linkCount  = 64
	)

	aliases := make([]string, 0, aliasCount)
	for i := range aliasCount {
		aliases = append(aliases, fmt.Sprintf("10.1.%d.%d", i/254, i%254+1))
	}
	remotes := make([]LLDPRemoteObservation, 0, linkCount)
	for i := range linkCount {
		remotes = append(remotes, LLDPRemoteObservation{
			LocalPortNum: fmt.Sprintf("%d", i+1),
			LocalPortID:  fmt.Sprintf("Ethernet%d", i+1),
			ChassisID:    fmt.Sprintf("02:00:00:00:01:%02x", i+1),
			SysName:      fmt.Sprintf("switch-%d", i+1),
			PortID:       "Ethernet1",
			ManagementIP: fmt.Sprintf("172.16.0.%d", i+1),
		})
	}

	result, err := BuildL2ResultFromObservations([]L2Observation{{
		DeviceID:          "router-a",
		Hostname:          "router-a",
		ManagementIP:      "10.0.0.1",
		ManagementAliases: aliases,
		ChassisID:         "02:00:00:00:00:01",
		LLDPRemotes:       remotes,
	}}, DiscoverOptions{EnableLLDP: true})
	require.NoError(t, err)

	projection := ToGraph(result, GraphOptions{
		SchemaVersion: "1.0.0",
		Source:        "snmp",
		Layer:         "2",
		AgentID:       "agent-1",
		LocalDeviceID: "router-a",
	})

	var routerActorID string
	for _, actor := range projection.Graph.Actors {
		if actor.Match.SysName != "router-a" {
			continue
		}
		routerActorID = actor.ActorID
		require.Len(t, actor.Match.IPAddresses, aliasCount+1)
	}
	require.NotEmpty(t, routerActorID)
	require.Len(t, projection.Graph.Links, linkCount)

	totalEndpointIPHints := 0
	for _, link := range projection.Graph.Links {
		totalEndpointIPHints += len(link.Src.Match.IPAddresses) + len(link.Dst.Match.IPAddresses)
		if link.SrcActorID == routerActorID {
			require.Equal(t, []string{"10.0.0.1"}, link.Src.Match.IPAddresses)
		}
		if link.DstActorID == routerActorID {
			require.Equal(t, []string{"10.0.0.1"}, link.Dst.Match.IPAddresses)
		}
	}
	require.LessOrEqual(t, totalEndpointIPHints, 2*linkCount)
}

func TestProjectionKeepsFDBLinkEndpointIPHintsConstantSized(t *testing.T) {
	const aliasCount = 256

	arpEntries := make([]ARPNDObservation, 0, aliasCount)
	for i := range aliasCount {
		arpEntries = append(arpEntries, ARPNDObservation{
			Protocol: "arp",
			IfIndex:  1,
			IfName:   "Ethernet1",
			IP:       fmt.Sprintf("10.2.%d.%d", i/254, i%254+1),
			MAC:      "70:49:a2:65:72:cd",
		})
	}

	result, err := BuildL2ResultFromObservations([]L2Observation{{
		DeviceID:     "switch-a",
		Hostname:     "switch-a",
		ManagementIP: "192.0.2.1",
		ChassisID:    "02:00:00:00:00:01",
		Interfaces: []ObservedInterface{{
			IfIndex: 1,
			IfName:  "Ethernet1",
		}},
		BridgePorts: []BridgePortObservation{{
			BasePort: "1",
			IfIndex:  1,
		}},
		FDBEntries: []FDBObservation{{
			MAC:        "70:49:a2:65:72:cd",
			BridgePort: "1",
			Status:     "learned",
		}},
		ARPNDEntries: arpEntries,
	}}, DiscoverOptions{EnableBridge: true, EnableARP: true})
	require.NoError(t, err)

	projection := ToGraph(result, GraphOptions{
		SchemaVersion: "1.0.0",
		Source:        "snmp",
		Layer:         "2",
		AgentID:       "agent-1",
		LocalDeviceID: "switch-a",
	})

	var endpointActorID string
	for _, actor := range projection.Graph.Actors {
		if !slices.Contains(actor.Match.MacAddresses, "70:49:a2:65:72:cd") {
			continue
		}
		endpointActorID = actor.ActorID
		require.Len(t, actor.Match.IPAddresses, aliasCount)
	}
	require.NotEmpty(t, endpointActorID)
	require.Len(t, projection.Graph.Links, 1)

	link := projection.Graph.Links[0]
	var endpointHints []string
	if link.SrcActorID == endpointActorID {
		endpointHints = link.Src.Match.IPAddresses
	} else {
		require.Equal(t, endpointActorID, link.DstActorID)
		endpointHints = link.Dst.Match.IPAddresses
	}
	require.Equal(t, []string{"10.2.0.1"}, endpointHints)
}

func TestProjectionKeepsFDBSegmentEndpointIPHintsConstantSized(t *testing.T) {
	const aliasCount = 256
	const endpointMAC = "70:49:a2:65:72:cd"

	arpEntries := make([]ARPNDObservation, 0, aliasCount)
	for i := range aliasCount {
		arpEntries = append(arpEntries, ARPNDObservation{
			Protocol: "arp",
			IfIndex:  1,
			IfName:   "Ethernet1",
			IP:       fmt.Sprintf("10.3.%d.%d", i/254, i%254+1),
			MAC:      endpointMAC,
		})
	}

	observations := []L2Observation{
		{
			DeviceID:     "switch-a",
			Hostname:     "switch-a",
			ManagementIP: "192.0.2.1",
			ChassisID:    "02:00:00:00:00:01",
			Interfaces:   []ObservedInterface{{IfIndex: 1, IfName: "Ethernet1"}},
			BridgePorts:  []BridgePortObservation{{BasePort: "1", IfIndex: 1}},
			FDBEntries:   []FDBObservation{{MAC: endpointMAC, BridgePort: "1", Status: "learned"}},
			ARPNDEntries: arpEntries,
		},
		{
			DeviceID:     "switch-b",
			Hostname:     "switch-b",
			ManagementIP: "192.0.2.2",
			ChassisID:    "02:00:00:00:00:02",
			Interfaces:   []ObservedInterface{{IfIndex: 1, IfName: "Ethernet1"}},
			BridgePorts:  []BridgePortObservation{{BasePort: "1", IfIndex: 1}},
			FDBEntries:   []FDBObservation{{MAC: endpointMAC, BridgePort: "1", Status: "learned"}},
		},
	}
	result, err := BuildL2ResultFromObservations(observations, DiscoverOptions{EnableBridge: true, EnableARP: true})
	require.NoError(t, err)

	projection := ToGraph(result, GraphOptions{
		SchemaVersion:             "1.0.0",
		Source:                    "snmp",
		Layer:                     "2",
		AgentID:                   "agent-1",
		LocalDeviceID:             "switch-a",
		ProbabilisticConnectivity: true,
	})

	var endpointActorID string
	for _, actor := range projection.Graph.Actors {
		if !slices.Contains(actor.Match.MacAddresses, endpointMAC) {
			continue
		}
		endpointActorID = actor.ActorID
		require.Len(t, actor.Match.IPAddresses, aliasCount)
	}
	require.NotEmpty(t, endpointActorID)

	linked := 0
	for _, link := range projection.Graph.Links {
		if link.Protocol != "fdb" {
			continue
		}
		if link.SrcActorID == endpointActorID {
			linked++
			require.Equal(t, []string{"10.3.0.1"}, link.Src.Match.IPAddresses)
		}
		if link.DstActorID == endpointActorID {
			linked++
			require.Equal(t, []string{"10.3.0.1"}, link.Dst.Match.IPAddresses)
		}
	}
	require.Equal(t, 1, linked)
}

func TestProjectionResolvesConstantEndpointHintsPastSharedPrimary(t *testing.T) {
	observations := []L2Observation{
		{
			DeviceID:          "switch-a",
			Hostname:          "switch-a",
			ManagementIP:      "10.0.0.1",
			ManagementAliases: []string{"10.0.0.2"},
			ChassisID:         "02:00:00:00:00:01",
			LLDPRemotes: []LLDPRemoteObservation{{
				LocalPortNum: "1",
				LocalPortID:  "Ethernet1",
				ChassisID:    "02:00:00:00:01:01",
				SysName:      "remote-a",
				PortID:       "Ethernet1",
				ManagementIP: "172.16.0.1",
			}},
		},
		{
			DeviceID:          "switch-b",
			Hostname:          "switch-b",
			ManagementIP:      "10.0.0.1",
			ManagementAliases: []string{"10.0.0.3"},
			ChassisID:         "02:00:00:00:00:02",
			LLDPRemotes: []LLDPRemoteObservation{{
				LocalPortNum: "1",
				LocalPortID:  "Ethernet1",
				ChassisID:    "02:00:00:00:01:02",
				SysName:      "remote-b",
				PortID:       "Ethernet1",
				ManagementIP: "172.16.0.2",
			}},
		},
	}

	result, err := BuildL2ResultFromObservations(observations, DiscoverOptions{EnableLLDP: true})
	require.NoError(t, err)
	projection := ToGraph(result, GraphOptions{
		SchemaVersion: "1.0.0",
		Source:        "snmp",
		Layer:         "2",
		AgentID:       "agent-1",
		LocalDeviceID: "switch-a",
	})

	actorIDBySysName := make(map[string]string)
	for _, actor := range projection.Graph.Actors {
		actorIDBySysName[actor.Match.SysName] = actor.ActorID
	}
	require.NotEmpty(t, actorIDBySysName["switch-a"], "actors: %#v", projection.Graph.Actors)
	require.NotEmpty(t, actorIDBySysName["switch-b"], "actors: %#v", projection.Graph.Actors)
	require.NotEqual(t, actorIDBySysName["switch-a"], actorIDBySysName["switch-b"])

	for _, link := range projection.Graph.Links {
		sysName := link.Src.Match.SysName
		if sysName != "switch-a" && sysName != "switch-b" {
			continue
		}
		require.Equal(t, actorIDBySysName[sysName], link.SrcActorID)
	}
}
