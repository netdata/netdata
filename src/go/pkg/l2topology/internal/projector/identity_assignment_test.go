// SPDX-License-Identifier: GPL-3.0-or-later

package projector

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
	"github.com/stretchr/testify/require"
)

func TestCanonicalTopologyKeyHelpers_NormalizeDeterministically(t *testing.T) {
	require.Equal(t,
		"00:11:22:33:44:55,0a000001,switch-a",
		canonicalTopologyHardwareKey([]string{" Switch-A ", "0A000001", "00:11:22:33:44:55", "switch-a"}),
	)
	require.Equal(t,
		"example.net,switch-a",
		canonicalTopologyStringListKey([]string{" Switch-A ", "", "example.net", "switch-a"}),
	)
}

func TestAssignTopologyActorIDsAndLinkEndpoints_IsDeterministic(t *testing.T) {
	actors := []projectedActor{
		{
			Actor: graph.Actor{
				ActorType: "device",
				Layer:     "l2",
				Source:    "snmp",
				Match: graph.Match{
					MacAddresses: []string{"00:11:22:33:44:55"},
					Hostnames:    []string{"switch-a"},
				},
			},
		},
		{
			Actor: graph.Actor{
				ActorType: "device",
				Layer:     "l2",
				Source:    "snmp",
				Match: graph.Match{
					MacAddresses: []string{"00-11-22-33-44-55"},
					Hostnames:    []string{"switch-a-duplicate"},
				},
			},
		},
		{
			Actor: graph.Actor{
				ActorType: "endpoint",
				Layer:     "l2",
				Source:    "derived",
				Match: graph.Match{
					IPAddresses: []string{"10.0.0.2"},
				},
			},
		},
		{
			Actor: graph.Actor{
				ActorType: "segment",
				Layer:     "l2",
				Source:    "derived",
				Match:     graph.Match{},
			},
		},
	}
	links := []graph.Link{
		{
			Protocol:  "lldp",
			Direction: "outbound",
			State:     "up",
			Src: graph.LinkEndpoint{
				Match:  graph.Match{MacAddresses: []string{"00:11:22:33:44:55"}},
				IfName: "eth0",
			},
			Dst: graph.LinkEndpoint{
				Match:  graph.Match{IPAddresses: []string{"10.0.0.2"}},
				IfName: "eth9",
			},
		},
		{
			Protocol:  "lldp",
			Direction: "outbound",
			State:     "up",
			Src: graph.LinkEndpoint{
				Match:  graph.Match{IPAddresses: []string{"10.0.0.2"}},
				IfName: "eth9",
			},
			Dst: graph.LinkEndpoint{
				Match:  graph.Match{MacAddresses: []string{"00:11:22:33:44:55"}},
				IfName: "eth0",
			},
		},
	}

	assignTopologyActorIDsAndLinkEndpoints(actors, links)

	require.Equal(t, "mac:00:11:22:33:44:55", actors[0].Actor.ActorID)
	require.Equal(t, "mac:00:11:22:33:44:55#2", actors[1].Actor.ActorID)
	require.Equal(t, "ip:10.0.0.2", actors[2].Actor.ActorID)
	require.Equal(t, "generated:segment", actors[3].Actor.ActorID)
	require.Equal(t, "mac:00:11:22:33:44:55", links[0].SrcActorID)
	require.Equal(t, "ip:10.0.0.2", links[0].DstActorID)
	require.Equal(t, "ip:10.0.0.2", links[1].SrcActorID)
	require.Equal(t, "mac:00:11:22:33:44:55", links[1].DstActorID)

	sortProjectedTopologyActors(actors)
	require.Equal(t, []string{
		"mac:00:11:22:33:44:55",
		"mac:00:11:22:33:44:55#2",
		"ip:10.0.0.2",
		"generated:segment",
	}, []string{actors[0].Actor.ActorID, actors[1].Actor.ActorID, actors[2].Actor.ActorID, actors[3].Actor.ActorID})

	sortTopologyLinks(links)
	require.Equal(t, "ip:10.0.0.2", links[0].SrcActorID)
	require.Equal(t, "mac:00:11:22:33:44:55", links[0].DstActorID)
	require.Equal(t, "mac:00:11:22:33:44:55", links[1].SrcActorID)
	require.Equal(t, "ip:10.0.0.2", links[1].DstActorID)
}

func TestEnrichTopologyPortDetailsWithLinkCounts_AddsCountsToMatchingPorts(t *testing.T) {
	actors := []projectedActor{
		{
			Actor: graph.Actor{ActorID: "device-a"},
			Detail: ProjectionActorDetail{
				Device: ProjectionDeviceActorDetail{
					Ports: []ProjectionPortDetail{
						{Name: "eth0"},
						{Name: "eth1"},
					},
				},
			},
		},
		{
			Actor: graph.Actor{ActorID: "device-b"},
			Detail: ProjectionActorDetail{
				Device: ProjectionDeviceActorDetail{
					Ports: []ProjectionPortDetail{
						{Name: "xe-0/0/0"},
					},
				},
			},
		},
	}
	links := []graph.Link{
		{
			SrcActorID: "device-a",
			DstActorID: "device-b",
			Src:        graph.LinkEndpoint{IfName: "eth0"},
			Dst:        graph.LinkEndpoint{IfName: "xe-0/0/0"},
		},
		{
			SrcActorID: "device-a",
			DstActorID: "device-b",
			Src:        graph.LinkEndpoint{IfName: "eth0"},
			Dst:        graph.LinkEndpoint{IfName: "xe-0/0/0"},
		},
	}

	enrichTopologyPortDetailsWithLinkCounts(actors, links)

	require.Equal(t, OptionalValue[int]{Value: 2, Has: true}, actors[0].Detail.Device.Ports[0].LinkCount)
	require.False(t, actors[0].Detail.Device.Ports[1].LinkCount.Has)
	require.Equal(t, OptionalValue[int]{Value: 2, Has: true}, actors[1].Detail.Device.Ports[0].LinkCount)
}
