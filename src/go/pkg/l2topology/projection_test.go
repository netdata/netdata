// SPDX-License-Identifier: GPL-3.0-or-later

package l2topology

import (
	"encoding/json"
	"fmt"
	"slices"
	"testing"
	"time"

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

func TestProjectionPreservesObservedCDPIfIndexAcrossNumericNameCollision(t *testing.T) {
	result, err := BuildL2ResultFromObservations([]L2Observation{
		{
			DeviceID:   "switch-a",
			Hostname:   "switch-a",
			Interfaces: []ObservedInterface{{IfIndex: 99, IfName: "7"}},
			CDPRemotes: []CDPRemoteObservation{{
				LocalIfIndex: 7,
				DeviceID:     "switch-b",
				SysName:      "switch-b",
				DevicePort:   "Ethernet2",
			}},
		},
		{DeviceID: "switch-b", Hostname: "switch-b"},
	}, DiscoverOptions{EnableCDP: true})
	require.NoError(t, err)

	projection := ToGraph(result, GraphOptions{Source: "snmp", Layer: "2"})
	require.Len(t, projection.Graph.Links, 1)
	link := projection.Graph.Links[0]
	require.Equal(t, "cdp", link.Protocol)
	require.Equal(t, 7, link.Src.IfIndex)
	require.Empty(t, link.Src.IfName)
	require.Equal(t, "7", link.Src.PortID)
}

func TestProjectionDoesNotInferIfIndexFromNumericLLDPRawPortID(t *testing.T) {
	result, err := BuildL2ResultFromObservations([]L2Observation{
		{
			DeviceID:   "switch-a",
			Hostname:   "switch-a",
			ChassisID:  "02:00:00:00:00:01",
			Interfaces: []ObservedInterface{{IfIndex: 99, IfName: "7"}},
			LLDPRemotes: []LLDPRemoteObservation{{
				LocalPortNum:       "7",
				LocalPortID:        "7",
				LocalPortIDSubtype: "local",
				ChassisID:          "02:00:00:00:00:02",
				SysName:            "switch-b",
				PortID:             "Ethernet2",
				PortIDSubtype:      "interfaceName",
			}},
		},
		{DeviceID: "switch-b", Hostname: "switch-b", ChassisID: "02:00:00:00:00:02"},
	}, DiscoverOptions{EnableLLDP: true})
	require.NoError(t, err)

	projection := ToGraph(result, GraphOptions{Source: "snmp", Layer: "2"})
	require.Len(t, projection.Graph.Links, 1)
	link := projection.Graph.Links[0]
	require.Equal(t, "lldp", link.Protocol)
	require.Zero(t, link.Src.IfIndex)
	require.Empty(t, link.Src.IfName)
	require.Equal(t, "7", link.Src.PortID)
	require.Equal(t, "Ethernet2", link.Dst.IfName)
	require.Equal(t, "Ethernet2", link.Dst.PortID)
}

func TestProjectionDoesNotInferIfIndexFromNumericCDPRemotePort(t *testing.T) {
	result, err := BuildL2ResultFromObservations([]L2Observation{
		{
			DeviceID: "switch-a",
			Hostname: "switch-a",
			CDPRemotes: []CDPRemoteObservation{{
				LocalIfIndex: 1,
				DeviceID:     "switch-b",
				SysName:      "switch-b",
				DevicePort:   "7",
			}},
		},
		{
			DeviceID:   "switch-b",
			Hostname:   "switch-b",
			Interfaces: []ObservedInterface{{IfIndex: 99, IfName: "7"}},
		},
	}, DiscoverOptions{EnableCDP: true})
	require.NoError(t, err)

	projection := ToGraph(result, GraphOptions{Source: "snmp", Layer: "2"})
	require.Len(t, projection.Graph.Links, 1)
	link := projection.Graph.Links[0]
	require.Equal(t, "cdp", link.Protocol)
	require.Zero(t, link.Dst.IfIndex)
	require.Empty(t, link.Dst.IfName)
	require.Equal(t, "7", link.Dst.PortID)
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

func TestProjectionCorrelatesQBridgeFDBWithVLANScopedSTPSegment(t *testing.T) {
	result, err := BuildL2ResultFromObservations([]L2Observation{
		{
			DeviceID:          "switch-a",
			Hostname:          "switch-a",
			ChassisID:         "02:00:00:00:00:01",
			BaseBridgeAddress: "02:00:00:00:00:01",
			Interfaces:        []ObservedInterface{{IfIndex: 1, IfName: "Ethernet1"}},
			BridgePorts:       []BridgePortObservation{{BasePort: "1", IfIndex: 1}},
			FDBEntries: []FDBObservation{
				{
					MAC:         "70:49:a2:65:72:cd",
					BridgePort:  "1",
					Status:      "learned",
					FDBDomainID: "fdb:500",
					VLANID:      "100",
				},
				{
					MAC:         "70:49:a2:65:72:ce",
					BridgePort:  "1",
					Status:      "learned",
					FDBDomainID: "fdb:500",
					VLANID:      "100",
				},
			},
			STPPorts: []STPPortObservation{{
				Port:             "1",
				VLANID:           "100",
				State:            "forwarding",
				DesignatedBridge: "02:00:00:00:00:02",
				DesignatedPort:   "8002",
			}},
		},
		{
			DeviceID:          "switch-b",
			Hostname:          "switch-b",
			ChassisID:         "02:00:00:00:00:02",
			BaseBridgeAddress: "02:00:00:00:00:02",
			Interfaces:        []ObservedInterface{{IfIndex: 2, IfName: "Ethernet2"}},
			BridgePorts:       []BridgePortObservation{{BasePort: "2", IfIndex: 2}},
			FDBEntries: []FDBObservation{
				{MAC: "70:49:a2:65:73:cd", BridgePort: "2", Status: "learned", FDBDomainID: "fdb:900", VLANID: "100"},
				{MAC: "70:49:a2:65:73:ce", BridgePort: "2", Status: "learned", FDBDomainID: "fdb:900", VLANID: "100"},
			},
		},
	}, DiscoverOptions{EnableBridge: true, EnableSTP: true})
	require.NoError(t, err)

	projection := ToGraph(result, GraphOptions{
		SchemaVersion:     "1.0.0",
		Source:            "snmp",
		Layer:             "2",
		AgentID:           "agent-1",
		LocalDeviceID:     "switch-a",
		InferenceStrategy: "stp_parent_tree",
	})

	var segments []ProjectionSegmentActorDetail
	for _, detail := range projection.ActorDetails {
		if detail.Segment.SegmentID != "" {
			segments = append(segments, detail.Segment)
		}
	}
	require.Len(t, segments, 1)
	require.ElementsMatch(t, []string{"switch-a", "switch-b"}, segments[0].ParentDevices)
	require.True(t, segments[0].EndpointsTotal.Has)
	require.Equal(t, 4, segments[0].EndpointsTotal.Value)

	stpLinkIndex := -1
	for i := range projection.Graph.Links {
		if projection.Graph.Links[i].LinkType == "stp" {
			stpLinkIndex = i
			break
		}
	}
	require.NotEqual(t, -1, stpLinkIndex)
	stpLink := projection.Graph.Links[stpLinkIndex]
	require.Equal(t, 2, stpLink.Dst.IfIndex)
	require.Equal(t, "Ethernet2", stpLink.Dst.IfName)
	require.Equal(t, "2", stpLink.Dst.BridgePort)
	require.Equal(t, "8002", stpLink.Dst.PortID)
}

func TestProjectionCoalescesOverlappingFDBSourcesBeforeAssignment(t *testing.T) {
	tests := map[string]struct {
		entries       []FDBObservation
		managedPeers  []L2Observation
		wantEndpoints int
	}{
		"domainless bridge and raw q-bridge": {
			entries: []FDBObservation{
				{MAC: "02:00:00:00:01:01", BridgePort: "1", Status: "learned"},
				{MAC: "02:00:00:00:01:01", BridgePort: "1", Status: "learned", FDBDomainID: "fdb:500", VLANID: "100"},
			},
			managedPeers: []L2Observation{
				{DeviceID: "switch-b", Hostname: "switch-b", ChassisID: "02:00:00:00:01:01"},
			},
			wantEndpoints: 1,
		},
		"raw q-bridge and vlan-context fdb": {
			entries: []FDBObservation{
				{MAC: "02:00:00:00:01:01", BridgePort: "1", Status: "learned", FDBDomainID: "fdb:500", VLANID: "100"},
				{MAC: "02:00:00:00:01:01", BridgePort: "1", Status: "learned", FDBDomainID: "vlan:100", VLANID: "100"},
			},
			managedPeers: []L2Observation{
				{DeviceID: "switch-b", Hostname: "switch-b", ChassisID: "02:00:00:00:01:01"},
			},
			wantEndpoints: 1,
		},
		"unique vlan alias spans endpoint rows": {
			entries: []FDBObservation{
				{MAC: "02:00:00:00:01:01", BridgePort: "1", Status: "learned", FDBDomainID: "fdb:500", VLANID: "100"},
				{MAC: "02:00:00:00:01:02", BridgePort: "1", Status: "learned", FDBDomainID: "vlan:100", VLANID: "100"},
			},
			managedPeers: []L2Observation{
				{DeviceID: "switch-b", Hostname: "switch-b", ChassisID: "02:00:00:00:01:01"},
				{DeviceID: "switch-c", Hostname: "switch-c", ChassisID: "02:00:00:00:01:02"},
			},
			wantEndpoints: 2,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			observations := []L2Observation{{
				DeviceID:    "switch-a",
				Hostname:    "switch-a",
				ChassisID:   "02:00:00:00:00:01",
				Interfaces:  []ObservedInterface{{IfIndex: 1, IfName: "Ethernet1"}},
				BridgePorts: []BridgePortObservation{{BasePort: "1", IfIndex: 1}},
				FDBEntries:  tt.entries,
			}}
			observations = append(observations, tt.managedPeers...)
			result, err := BuildL2ResultFromObservations(observations, DiscoverOptions{EnableBridge: true})
			require.NoError(t, err)

			projection := ToGraph(result, GraphOptions{Source: "snmp", Layer: "2"})
			var segments []ProjectionSegmentActorDetail
			fdbLinks := 0
			for _, detail := range projection.ActorDetails {
				if detail.Segment.SegmentID != "" {
					segments = append(segments, detail.Segment)
				}
			}
			for _, link := range projection.Graph.Links {
				if link.Protocol == "fdb" {
					fdbLinks++
				}
			}

			require.Len(t, segments, 1)
			require.Equal(t, []string{"switch-a"}, segments[0].ParentDevices)
			require.True(t, segments[0].EndpointsTotal.Has)
			require.Equal(t, tt.wantEndpoints, segments[0].EndpointsTotal.Value)
			require.Equal(t, tt.wantEndpoints, fdbLinks)
		})
	}
}

func TestProjectionKeepsUnmappedFDBBasePortOpaque(t *testing.T) {
	result, err := BuildL2ResultFromObservations([]L2Observation{
		{
			DeviceID:  "switch-a",
			Hostname:  "switch-a",
			ChassisID: "02:00:00:00:00:01",
			Interfaces: []ObservedInterface{
				{IfIndex: 99, IfName: "7"},
				{IfIndex: 98, IfName: "8"},
			},
			FDBEntries: []FDBObservation{
				{MAC: "02:00:00:00:01:01", BridgePort: "7", Status: "learned"},
				{MAC: "02:00:00:00:01:02", BridgePort: "7", Status: "learned"},
				{MAC: "02:00:00:00:01:03", BridgePort: "8", Status: "learned"},
			},
		},
		{DeviceID: "switch-b", Hostname: "switch-b", ChassisID: "02:00:00:00:01:01"},
	}, DiscoverOptions{EnableBridge: true})
	require.NoError(t, err)

	projection := ToGraph(result, GraphOptions{Source: "snmp", Layer: "2"})
	bridgeLinks := 0
	directFDBLinks := 0
	for _, link := range projection.Graph.Links {
		if link.Src.SysName != "switch-a" {
			continue
		}
		switch link.Protocol {
		case "bridge":
			bridgeLinks++
			require.Zero(t, link.Src.IfIndex)
			require.Empty(t, link.Src.IfName)
			require.Equal(t, "7", link.Src.BridgePort)
		case "fdb":
			directFDBLinks++
			require.Zero(t, link.Src.IfIndex)
			require.Empty(t, link.Src.IfName)
			require.Equal(t, "8", link.Src.BridgePort)
		}
	}
	require.Equal(t, 1, bridgeLinks)
	require.Equal(t, 1, directFDBLinks)
}

func TestProjectionPairwiseCoalescesOverlappingFDBSources(t *testing.T) {
	result, err := BuildL2ResultFromObservations([]L2Observation{
		{
			DeviceID:    "switch-a",
			Hostname:    "switch-a",
			ChassisID:   "02:00:00:00:00:01",
			Interfaces:  []ObservedInterface{{IfIndex: 1, IfName: "Ethernet1"}},
			BridgePorts: []BridgePortObservation{{BasePort: "1", IfIndex: 1}},
			FDBEntries: []FDBObservation{
				{MAC: "02:00:00:00:00:02", BridgePort: "1", Status: "learned"},
				{MAC: "02:00:00:00:00:02", BridgePort: "1", Status: "learned", FDBDomainID: "fdb:500", VLANID: "100"},
				{MAC: "02:00:00:00:01:01", BridgePort: "1", Status: "learned"},
			},
		},
		{
			DeviceID:    "switch-b",
			Hostname:    "switch-b",
			ChassisID:   "02:00:00:00:00:02",
			Interfaces:  []ObservedInterface{{IfIndex: 2, IfName: "Ethernet2"}},
			BridgePorts: []BridgePortObservation{{BasePort: "2", IfIndex: 2}},
			FDBEntries: []FDBObservation{
				{MAC: "02:00:00:00:00:01", BridgePort: "2", Status: "learned"},
				{MAC: "02:00:00:00:00:01", BridgePort: "2", Status: "learned", FDBDomainID: "fdb:900", VLANID: "100"},
				{MAC: "02:00:00:00:01:02", BridgePort: "2", Status: "learned"},
			},
		},
	}, DiscoverOptions{EnableBridge: true})
	require.NoError(t, err)

	for _, strategy := range []string{
		"fdb_pairwise_minimum_knowledge",
		"stp_fdb_correlated",
		"cdp_fdb_hybrid",
	} {
		t.Run(strategy, func(t *testing.T) {
			projection := ToGraph(result, GraphOptions{
				Source:            "snmp",
				Layer:             "2",
				InferenceStrategy: strategy,
			})
			multiParentSegments := 0
			for _, detail := range projection.ActorDetails {
				if len(detail.Segment.ParentDevices) == 2 {
					multiParentSegments++
				}
			}
			require.Equal(t, 1, multiParentSegments)
		})
	}
}

func TestProjectionCorrelatesBridgeMIBWithCanonicalSTPPort(t *testing.T) {
	result, err := BuildL2ResultFromObservations([]L2Observation{
		{
			DeviceID:          "switch-a",
			Hostname:          "switch-a",
			ChassisID:         "02:00:00:00:00:01",
			BaseBridgeAddress: "02:00:00:00:00:01",
			Interfaces:        []ObservedInterface{{IfIndex: 7, IfName: "Ethernet1"}},
			BridgePorts:       []BridgePortObservation{{BasePort: "1", IfIndex: 7}},
			FDBEntries: []FDBObservation{
				{MAC: "70:49:a2:65:72:cd", BridgePort: "1", Status: "learned"},
				{MAC: "70:49:a2:65:72:ce", BridgePort: "1", Status: "learned"},
			},
			STPPorts: []STPPortObservation{{
				Port:             "1",
				State:            "forwarding",
				DesignatedBridge: "02:00:00:00:00:02",
				DesignatedPort:   "8002",
			}},
		},
		{
			DeviceID:          "switch-b",
			Hostname:          "switch-b",
			ChassisID:         "02:00:00:00:00:02",
			BaseBridgeAddress: "02:00:00:00:00:02",
			Interfaces:        []ObservedInterface{{IfIndex: 9, IfName: "Ethernet2"}},
			BridgePorts:       []BridgePortObservation{{BasePort: "2", IfIndex: 9}},
		},
	}, DiscoverOptions{EnableBridge: true, EnableSTP: true})
	require.NoError(t, err)

	projection := ToGraph(result, GraphOptions{
		Source:            "snmp",
		Layer:             "2",
		InferenceStrategy: "stp_parent_tree",
	})

	var segments []ProjectionSegmentActorDetail
	for _, detail := range projection.ActorDetails {
		if detail.Segment.SegmentID != "" {
			segments = append(segments, detail.Segment)
		}
	}
	require.Len(t, segments, 1)
	require.ElementsMatch(t, []string{"switch-a", "switch-b"}, segments[0].ParentDevices)
}

func TestProjectionDeduplicatesDirectSTPAcrossVLANScopes(t *testing.T) {
	result, err := BuildL2ResultFromObservations([]L2Observation{
		{
			DeviceID:          "switch-a",
			Hostname:          "switch-a",
			ChassisID:         "02:00:00:00:00:01",
			BaseBridgeAddress: "02:00:00:00:00:01",
			Interfaces:        []ObservedInterface{{IfIndex: 1, IfName: "Ethernet1"}},
			BridgePorts:       []BridgePortObservation{{BasePort: "1", IfIndex: 1}},
			STPPorts: []STPPortObservation{
				{Port: "1", VLANID: "100", DesignatedBridge: "02:00:00:00:00:02", DesignatedPort: "8002"},
				{Port: "1", VLANID: "200", DesignatedBridge: "02:00:00:00:00:02", DesignatedPort: "8002"},
			},
		},
		{
			DeviceID:          "switch-b",
			Hostname:          "switch-b",
			ChassisID:         "02:00:00:00:00:02",
			BaseBridgeAddress: "02:00:00:00:00:02",
			Interfaces:        []ObservedInterface{{IfIndex: 2, IfName: "Ethernet2"}},
			BridgePorts:       []BridgePortObservation{{BasePort: "2", IfIndex: 2}},
			STPPorts: []STPPortObservation{
				{Port: "2", VLANID: "100", DesignatedBridge: "02:00:00:00:00:01", DesignatedPort: "8001"},
			},
		},
	}, DiscoverOptions{EnableSTP: true})
	require.NoError(t, err)

	projection := ToGraph(result, GraphOptions{Source: "snmp", Layer: "2"})
	stpLinks := 0
	stpLinkIndex := -1
	for i := range projection.Graph.Links {
		if projection.Graph.Links[i].LinkType == "stp" {
			stpLinks++
			stpLinkIndex = i
		}
	}
	require.Equal(t, 1, stpLinks)
	require.NotEqual(t, -1, stpLinkIndex)
	stpLink := projection.Graph.Links[stpLinkIndex]
	require.Equal(t, 1, stpLink.Src.IfIndex)
	require.Equal(t, "1", stpLink.Src.BridgePort)
	require.Equal(t, 2, stpLink.Dst.IfIndex)
	require.Equal(t, "2", stpLink.Dst.BridgePort)
}

func TestProjectionKeepsUnresolvedSTPPortsInBridgePortNamespace(t *testing.T) {
	result, err := BuildL2ResultFromObservations([]L2Observation{
		{
			DeviceID:          "switch-a",
			Hostname:          "switch-a",
			ChassisID:         "02:00:00:00:00:01",
			BaseBridgeAddress: "02:00:00:00:00:01",
			STPPorts: []STPPortObservation{{
				Port:             "1",
				DesignatedBridge: "02:00:00:00:00:02",
				DesignatedPort:   "8002",
			}},
		},
		{
			DeviceID:          "switch-b",
			Hostname:          "switch-b",
			ChassisID:         "02:00:00:00:00:02",
			BaseBridgeAddress: "02:00:00:00:00:02",
		},
	}, DiscoverOptions{EnableSTP: true})
	require.NoError(t, err)

	projection := ToGraph(result, GraphOptions{Source: "snmp", Layer: "2"})
	var stpLinks []int
	for i := range projection.Graph.Links {
		if projection.Graph.Links[i].LinkType == "stp" {
			stpLinks = append(stpLinks, i)
		}
	}
	require.Len(t, stpLinks, 1)
	link := projection.Graph.Links[stpLinks[0]]
	require.Zero(t, link.Src.IfIndex)
	require.Empty(t, link.Src.IfName)
	require.Equal(t, "1", link.Src.PortID)
	require.Equal(t, "1", link.Src.BridgePort)
	require.Equal(t, "1", link.Src.PortName)
	require.Zero(t, link.Dst.IfIndex)
	require.Empty(t, link.Dst.IfName)
	require.Equal(t, "8002", link.Dst.PortID)
	require.Equal(t, "2", link.Dst.BridgePort)
	require.Equal(t, "2", link.Dst.PortName)
}

func TestProjectionDisplaysDecodedUnresolvedSTPTargetPort(t *testing.T) {
	result, err := BuildL2ResultFromObservations([]L2Observation{
		{
			DeviceID:          "switch-a",
			Hostname:          "switch-a",
			ChassisID:         "02:00:00:00:00:01",
			BaseBridgeAddress: "02:00:00:00:00:01",
			STPPorts: []STPPortObservation{{
				Port:             "port-a",
				DesignatedBridge: "02:00:00:00:00:02",
				DesignatedPort:   "8002",
			}},
		},
		{
			DeviceID:          "switch-b",
			Hostname:          "switch-b",
			ChassisID:         "02:00:00:00:00:02",
			BaseBridgeAddress: "02:00:00:00:00:02",
		},
	}, DiscoverOptions{EnableSTP: true})
	require.NoError(t, err)

	projection := ToGraph(result, GraphOptions{Source: "snmp", Layer: "2"})
	for _, link := range projection.Graph.Links {
		if link.LinkType != "stp" {
			continue
		}
		require.Equal(t, "8002", link.Dst.PortID)
		require.Equal(t, "2", link.Dst.BridgePort)
		require.Equal(t, "2", link.Dst.PortName)
		return
	}
	t.Fatal("missing STP link")
}

func TestProjectionPreservesMappedSTPIfIndexWithoutInterfaceName(t *testing.T) {
	result, err := BuildL2ResultFromObservations([]L2Observation{
		{
			DeviceID:          "switch-a",
			Hostname:          "switch-a",
			ChassisID:         "02:00:00:00:00:01",
			BaseBridgeAddress: "02:00:00:00:00:01",
			BridgePorts:       []BridgePortObservation{{BasePort: "1", IfIndex: 7}},
			STPPorts: []STPPortObservation{{
				Port:             "1",
				DesignatedBridge: "02:00:00:00:00:02",
				DesignatedPort:   "8002",
			}},
		},
		{
			DeviceID:          "switch-b",
			Hostname:          "switch-b",
			ChassisID:         "02:00:00:00:00:02",
			BaseBridgeAddress: "02:00:00:00:00:02",
		},
	}, DiscoverOptions{EnableSTP: true})
	require.NoError(t, err)

	projection := ToGraph(result, GraphOptions{Source: "snmp", Layer: "2"})
	for _, link := range projection.Graph.Links {
		if link.LinkType != "stp" {
			continue
		}
		require.Equal(t, 7, link.Src.IfIndex)
		require.Empty(t, link.Src.IfName)
		require.Equal(t, "1", link.Src.PortID)
		require.Equal(t, "1", link.Src.BridgePort)
		return
	}
	t.Fatal("missing STP link")
}

func TestProjectionDoesNotResolveEncodedSTPPortAsInterfaceName(t *testing.T) {
	result, err := BuildL2ResultFromObservations([]L2Observation{
		{
			DeviceID:          "switch-a",
			Hostname:          "switch-a",
			ChassisID:         "02:00:00:00:00:01",
			BaseBridgeAddress: "02:00:00:00:00:01",
			STPPorts: []STPPortObservation{{
				Port:             "1",
				DesignatedBridge: "02:00:00:00:00:02",
				DesignatedPort:   "8002",
			}},
		},
		{
			DeviceID:          "switch-b",
			Hostname:          "switch-b",
			ChassisID:         "02:00:00:00:00:02",
			BaseBridgeAddress: "02:00:00:00:00:02",
			Interfaces:        []ObservedInterface{{IfIndex: 99, IfName: "8002"}},
		},
	}, DiscoverOptions{EnableSTP: true})
	require.NoError(t, err)

	projection := ToGraph(result, GraphOptions{Source: "snmp", Layer: "2"})
	for _, link := range projection.Graph.Links {
		if link.LinkType != "stp" {
			continue
		}
		require.Zero(t, link.Dst.IfIndex)
		require.Empty(t, link.Dst.IfName)
		require.Equal(t, "8002", link.Dst.PortID)
		require.Equal(t, "2", link.Dst.BridgePort)
		require.Equal(t, "2", link.Dst.PortName)
		return
	}
	t.Fatal("missing STP link")
}

func TestProjectionDoesNotReresolveObservedSTPIfIndexAsInterfaceName(t *testing.T) {
	result, err := BuildL2ResultFromObservations([]L2Observation{
		{
			DeviceID:          "switch-a",
			Hostname:          "switch-a",
			ChassisID:         "02:00:00:00:00:01",
			BaseBridgeAddress: "02:00:00:00:00:01",
			STPPorts: []STPPortObservation{{
				Port:             "1",
				DesignatedBridge: "02:00:00:00:00:02",
				DesignatedPort:   "8002",
			}},
		},
		{
			DeviceID:          "switch-b",
			Hostname:          "switch-b",
			ChassisID:         "02:00:00:00:00:02",
			BaseBridgeAddress: "02:00:00:00:00:02",
			Interfaces: []ObservedInterface{
				{IfIndex: 7, IfName: "Ethernet7"},
				{IfIndex: 99, IfName: "7"},
			},
			BridgePorts: []BridgePortObservation{{BasePort: "2", IfIndex: 7}},
			FDBEntries: []FDBObservation{{
				MAC:        "70:49:a2:65:72:cd",
				BridgePort: "2",
				Status:     "learned",
			}},
		},
	}, DiscoverOptions{EnableBridge: true, EnableSTP: true})
	require.NoError(t, err)

	projection := ToGraph(result, GraphOptions{Source: "snmp", Layer: "2"})
	for _, link := range projection.Graph.Links {
		if link.LinkType != "stp" {
			continue
		}
		require.Equal(t, 7, link.Dst.IfIndex)
		require.Equal(t, "Ethernet7", link.Dst.IfName)
		require.Equal(t, "8002", link.Dst.PortID)
		require.Equal(t, "2", link.Dst.BridgePort)
		return
	}
	t.Fatal("missing STP link")
}

func TestProjectionDoesNotReresolveObservedSTPSourceIfIndexAsInterfaceName(t *testing.T) {
	result, err := BuildL2ResultFromObservations([]L2Observation{
		{
			DeviceID:          "switch-a",
			Hostname:          "switch-a",
			ChassisID:         "02:00:00:00:00:01",
			BaseBridgeAddress: "02:00:00:00:00:01",
			Interfaces: []ObservedInterface{
				{IfIndex: 7, IfName: "Ethernet7"},
				{IfIndex: 99, IfName: "7"},
			},
			BridgePorts: []BridgePortObservation{{BasePort: "1", IfIndex: 7}},
			STPPorts: []STPPortObservation{{
				Port:             "1",
				DesignatedBridge: "02:00:00:00:00:02",
				DesignatedPort:   "8002",
			}},
		},
		{
			DeviceID:          "switch-b",
			Hostname:          "switch-b",
			ChassisID:         "02:00:00:00:00:02",
			BaseBridgeAddress: "02:00:00:00:00:02",
		},
	}, DiscoverOptions{EnableSTP: true})
	require.NoError(t, err)

	projection := ToGraph(result, GraphOptions{Source: "snmp", Layer: "2"})
	for _, link := range projection.Graph.Links {
		if link.LinkType != "stp" {
			continue
		}
		require.Equal(t, 7, link.Src.IfIndex)
		require.Equal(t, "Ethernet7", link.Src.IfName)
		require.Equal(t, "1", link.Src.PortID)
		require.Equal(t, "1", link.Src.BridgePort)
		return
	}
	t.Fatal("missing STP link")
}

func TestProjectionDeduplicatesReciprocalSTPWithOnlyBridgePortIdentity(t *testing.T) {
	result, err := BuildL2ResultFromObservations([]L2Observation{
		{
			DeviceID:          "switch-a",
			Hostname:          "switch-a",
			ChassisID:         "02:00:00:00:00:01",
			BaseBridgeAddress: "02:00:00:00:00:01",
			STPPorts: []STPPortObservation{{
				Port:             "1",
				DesignatedBridge: "02:00:00:00:00:02",
				DesignatedPort:   "8002",
			}},
		},
		{
			DeviceID:          "switch-b",
			Hostname:          "switch-b",
			ChassisID:         "02:00:00:00:00:02",
			BaseBridgeAddress: "02:00:00:00:00:02",
			STPPorts: []STPPortObservation{{
				Port:             "2",
				DesignatedBridge: "02:00:00:00:00:01",
				DesignatedPort:   "8001",
			}},
		},
	}, DiscoverOptions{EnableSTP: true})
	require.NoError(t, err)

	projection := ToGraph(result, GraphOptions{Source: "snmp", Layer: "2"})
	stpLinks := 0
	for _, link := range projection.Graph.Links {
		if link.LinkType == "stp" {
			stpLinks++
		}
	}
	require.Equal(t, 1, stpLinks)
}

func TestProjectionKeepsDistinctSTPLinksWithoutPortIdentity(t *testing.T) {
	result := Result{
		Devices: []Device{
			{ID: "switch-a", Hostname: "switch-a", ChassisID: "02:00:00:00:00:01"},
			{ID: "switch-b", Hostname: "switch-b", ChassisID: "02:00:00:00:00:02"},
			{ID: "switch-c", Hostname: "switch-c", ChassisID: "02:00:00:00:00:03"},
			{ID: "switch-d", Hostname: "switch-d", ChassisID: "02:00:00:00:00:04"},
		},
		Adjacencies: []Adjacency{
			{Protocol: "stp", SourceID: "switch-a", TargetID: "switch-b"},
			{Protocol: "stp", SourceID: "switch-c", TargetID: "switch-d"},
		},
	}

	projection := ToGraph(result, GraphOptions{Source: "snmp", Layer: "2"})
	stpLinks := 0
	for _, link := range projection.Graph.Links {
		if link.LinkType == "stp" {
			stpLinks++
		}
	}
	require.Equal(t, 2, stpLinks)
}

func TestProjectionDoesNotCorrelateAmbiguousVLANAliasWithSTP(t *testing.T) {
	result, err := BuildL2ResultFromObservations([]L2Observation{
		{
			DeviceID:          "switch-a",
			Hostname:          "switch-a",
			ChassisID:         "02:00:00:00:00:01",
			BaseBridgeAddress: "02:00:00:00:00:01",
			Interfaces:        []ObservedInterface{{IfIndex: 1, IfName: "Ethernet1"}},
			BridgePorts:       []BridgePortObservation{{BasePort: "1", IfIndex: 1}},
			FDBEntries: []FDBObservation{
				{MAC: "70:49:a2:65:72:01", BridgePort: "1", Status: "learned", FDBDomainID: "fdb:500", VLANID: "100"},
				{MAC: "70:49:a2:65:72:02", BridgePort: "1", Status: "learned", FDBDomainID: "fdb:500", VLANID: "100"},
				{MAC: "70:49:a2:65:73:01", BridgePort: "1", Status: "learned", FDBDomainID: "fdb:600", VLANID: "100"},
				{MAC: "70:49:a2:65:73:02", BridgePort: "1", Status: "learned", FDBDomainID: "fdb:600", VLANID: "100"},
			},
			STPPorts: []STPPortObservation{{Port: "1", VLANID: "100", DesignatedBridge: "02:00:00:00:00:02", DesignatedPort: "8002"}},
		},
		{DeviceID: "switch-b", Hostname: "switch-b", ChassisID: "02:00:00:00:00:02", BaseBridgeAddress: "02:00:00:00:00:02"},
	}, DiscoverOptions{EnableBridge: true, EnableSTP: true})
	require.NoError(t, err)

	projection := ToGraph(result, GraphOptions{Source: "snmp", Layer: "2", InferenceStrategy: "stp_parent_tree"})
	segments := make([]ProjectionSegmentActorDetail, 0, 2)
	for _, detail := range projection.ActorDetails {
		if detail.Segment.SegmentID != "" {
			segments = append(segments, detail.Segment)
		}
	}
	require.Len(t, segments, 2)
	for _, segment := range segments {
		require.Equal(t, []string{"switch-a"}, segment.ParentDevices)
	}
}

func TestProjectionDoesNotTreatDomainlessSTPAsQBridgeWildcard(t *testing.T) {
	result, err := BuildL2ResultFromObservations([]L2Observation{
		{
			DeviceID:          "switch-a",
			Hostname:          "switch-a",
			ChassisID:         "02:00:00:00:00:01",
			BaseBridgeAddress: "02:00:00:00:00:01",
			Interfaces:        []ObservedInterface{{IfIndex: 1, IfName: "Ethernet1"}},
			BridgePorts:       []BridgePortObservation{{BasePort: "1", IfIndex: 1}},
			FDBEntries: []FDBObservation{
				{MAC: "70:49:a2:65:72:01", BridgePort: "1", Status: "learned", FDBDomainID: "fdb:500", VLANID: "100"},
				{MAC: "70:49:a2:65:72:02", BridgePort: "1", Status: "learned", FDBDomainID: "fdb:500", VLANID: "100"},
			},
			STPPorts: []STPPortObservation{{Port: "1", DesignatedBridge: "02:00:00:00:00:02", DesignatedPort: "8002"}},
		},
		{DeviceID: "switch-b", Hostname: "switch-b", ChassisID: "02:00:00:00:00:02", BaseBridgeAddress: "02:00:00:00:00:02"},
	}, DiscoverOptions{EnableBridge: true, EnableSTP: true})
	require.NoError(t, err)

	projection := ToGraph(result, GraphOptions{Source: "snmp", Layer: "2", InferenceStrategy: "stp_parent_tree"})
	var segments []ProjectionSegmentActorDetail
	for _, detail := range projection.ActorDetails {
		if detail.Segment.SegmentID != "" {
			segments = append(segments, detail.Segment)
		}
	}
	require.Len(t, segments, 1)
	require.Equal(t, []string{"switch-a"}, segments[0].ParentDevices)
}

func TestProjectionPairwiseMultiDomainIsDeterministic(t *testing.T) {
	collectedAt := time.Unix(1_700_000_000, 0).UTC()
	base := []L2Observation{
		{
			DeviceID: "switch-a", Hostname: "switch-a", ChassisID: "aa:aa:aa:aa:aa:aa",
			Interfaces:  []ObservedInterface{{IfIndex: 1, IfName: "Ethernet1"}},
			BridgePorts: []BridgePortObservation{{BasePort: "1", IfIndex: 1}},
			FDBEntries: []FDBObservation{
				{MAC: "bb:bb:bb:bb:bb:bb", BridgePort: "1", Status: "learned", FDBDomainID: "fdb:10", VLANID: "100"},
				{MAC: "bb:bb:bb:bb:bb:bb", BridgePort: "1", Status: "learned", FDBDomainID: "fdb:20", VLANID: "200"},
			},
		},
		{
			DeviceID: "switch-b", Hostname: "switch-b", ChassisID: "bb:bb:bb:bb:bb:bb",
			Interfaces:  []ObservedInterface{{IfIndex: 2, IfName: "Ethernet2"}},
			BridgePorts: []BridgePortObservation{{BasePort: "2", IfIndex: 2}},
			FDBEntries: []FDBObservation{
				{MAC: "aa:aa:aa:aa:aa:aa", BridgePort: "2", Status: "learned", FDBDomainID: "fdb:30", VLANID: "100"},
				{MAC: "aa:aa:aa:aa:aa:aa", BridgePort: "2", Status: "learned", FDBDomainID: "fdb:40", VLANID: "200"},
			},
		},
	}

	var expected []byte
	for iteration := range 100 {
		observations := append([]L2Observation(nil), base...)
		for i := range observations {
			observations[i].FDBEntries = append([]FDBObservation(nil), base[i].FDBEntries...)
		}
		if iteration%2 == 1 {
			observations[0], observations[1] = observations[1], observations[0]
		}
		if iteration%4 >= 2 {
			for i := range observations {
				slices.Reverse(observations[i].FDBEntries)
			}
		}
		result, err := BuildL2ResultFromObservations(observations, DiscoverOptions{EnableBridge: true, CollectedAt: collectedAt})
		require.NoError(t, err)
		projection := ToGraph(result, GraphOptions{
			Source:            "snmp",
			Layer:             "2",
			CollectedAt:       collectedAt,
			InferenceStrategy: "fdb_pairwise_minimum_knowledge",
		})
		payload, err := json.Marshal(projection)
		require.NoError(t, err)
		if iteration == 0 {
			expected = payload
			continue
		}
		require.Equal(t, expected, payload)
	}
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
