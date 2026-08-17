// SPDX-License-Identifier: GPL-3.0-or-later

package topologyenrich

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/stretchr/testify/require"
)

func TestApplyLayer3MatchesStandaloneEnrichmentSequence(t *testing.T) {
	newData := func() topologymodel.Data {
		return topologymodel.Data{Actors: []topologymodel.Actor{
			topologyL3ManagedActorForTest("router-a", nil, "198.51.100.1"),
			topologyL3ManagedActorForTest("router-b", nil, "198.51.100.2"),
		}}
	}
	aggregate := topologymodel.ObservationAggregate{
		ProducerScopeID: "producer-a",
		L3Interfaces: []topologymodel.L3Interface{
			{DeviceID: "router-a", IP: "198.51.100.1", Netmask: "255.255.255.252", IfIndex: "1"},
			{DeviceID: "router-b", IP: "198.51.100.2", Netmask: "255.255.255.252", IfIndex: "1"},
		},
		OSPFNeighbors: []topologymodel.OSPFNeighbor{{
			DeviceID:         "router-a",
			NeighborRouterID: "192.0.2.2",
			NeighborIP:       "198.51.100.2",
			LocalIP:          "198.51.100.1",
			State:            "full",
		}},
		BGPPeers: []topologymodel.BGPPeer{{
			DeviceID:        "router-a",
			NeighborIP:      "198.51.100.2",
			LocalIP:         "198.51.100.1",
			LocalIdentifier: "192.0.2.1",
			PeerIdentifier:  "192.0.2.2",
			State:           "established",
		}},
	}

	want := newData()
	ApplyL3Subnet(&want, aggregate)
	ApplyOSPFAdjacency(&want, aggregate)
	ApplyBGPAdjacency(&want, aggregate)

	got := newData()
	ApplyLayer3(&got, aggregate)

	require.Equal(t, want, got)
}

func TestLayer3ResolverProviderStaysLazyWithoutUsableSubnetCandidates(t *testing.T) {
	data := topologymodel.Data{Actors: []topologymodel.Actor{
		topologyL3ManagedActorForTest("router-a", nil, "198.51.100.1"),
	}}
	aggregate := topologymodel.ObservationAggregate{L3Interfaces: []topologymodel.L3Interface{{
		DeviceID: "router-a",
		IP:       "198.51.100.1",
		Netmask:  "255.255.255.252",
	}}}
	provider := newTopologyL3ActorResolverProvider(&data, aggregate.Snapshots)

	applyL3SubnetWithResolver(&data, aggregate, provider)

	require.False(t, provider.initialized)
}

func TestLayer3ResolverProviderReusesOneActorGeneration(t *testing.T) {
	data := topologymodel.Data{Actors: []topologymodel.Actor{
		topologyL3ManagedActorForTest("router-a", nil, "198.51.100.1"),
	}}
	provider := newTopologyL3ActorResolverProvider(&data, nil)

	first := provider.resolve()
	data.Actors = append(data.Actors, topologyL3ManagedActorForTest("router-b", nil, "198.51.100.1"))
	second := provider.resolve()

	firstRef, firstOK := first.resolveIPAddress("198.51.100.1")
	secondRef, secondOK := second.resolveIPAddress("198.51.100.1")
	require.True(t, firstOK)
	require.True(t, secondOK)
	require.Equal(t, "router-a", firstRef.actorID)
	require.Equal(t, firstRef, secondRef)
}
