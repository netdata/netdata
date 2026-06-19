// SPDX-License-Identifier: GPL-3.0-or-later

package l2topology

import (
	"github.com/stretchr/testify/require"
	"testing"
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
	actors := []Actor{
		{
			ActorType: "device",
			Layer:     "l2",
			Source:    "snmp",
			Match: Match{
				MacAddresses: []string{"00:11:22:33:44:55"},
				Hostnames:    []string{"switch-a"},
			},
		},
		{
			ActorType: "device",
			Layer:     "l2",
			Source:    "snmp",
			Match: Match{
				MacAddresses: []string{"00-11-22-33-44-55"},
				Hostnames:    []string{"switch-a-duplicate"},
			},
		},
		{
			ActorType: "endpoint",
			Layer:     "l2",
			Source:    "derived",
			Match: Match{
				IPAddresses: []string{"10.0.0.2"},
			},
		},
		{
			ActorType: "segment",
			Layer:     "l2",
			Source:    "derived",
			Match:     Match{},
		},
	}
	links := []Link{
		{
			Protocol:  "lldp",
			Direction: "outbound",
			State:     "up",
			Src: LinkEndpoint{
				Match: Match{MacAddresses: []string{"00:11:22:33:44:55"}},
				Attributes: map[string]any{
					"if_name": "eth0",
				},
			},
			Dst: LinkEndpoint{
				Match: Match{IPAddresses: []string{"10.0.0.2"}},
				Attributes: map[string]any{
					"if_name": "eth9",
				},
			},
		},
		{
			Protocol:  "lldp",
			Direction: "outbound",
			State:     "up",
			Src: LinkEndpoint{
				Match: Match{IPAddresses: []string{"10.0.0.2"}},
				Attributes: map[string]any{
					"if_name": "eth9",
				},
			},
			Dst: LinkEndpoint{
				Match: Match{MacAddresses: []string{"00:11:22:33:44:55"}},
				Attributes: map[string]any{
					"if_name": "eth0",
				},
			},
		},
	}

	assignTopologyActorIDsAndLinkEndpoints(actors, links)

	require.Equal(t, "mac:00:11:22:33:44:55", actors[0].ActorID)
	require.Equal(t, "mac:00:11:22:33:44:55#2", actors[1].ActorID)
	require.Equal(t, "ip:10.0.0.2", actors[2].ActorID)
	require.Equal(t, "generated:segment", actors[3].ActorID)
	require.Equal(t, "mac:00:11:22:33:44:55", links[0].SrcActorID)
	require.Equal(t, "ip:10.0.0.2", links[0].DstActorID)
	require.Equal(t, "ip:10.0.0.2", links[1].SrcActorID)
	require.Equal(t, "mac:00:11:22:33:44:55", links[1].DstActorID)

	sortTopologyActors(actors)
	require.Equal(t, []string{
		"mac:00:11:22:33:44:55",
		"mac:00:11:22:33:44:55#2",
		"ip:10.0.0.2",
		"generated:segment",
	}, []string{actors[0].ActorID, actors[1].ActorID, actors[2].ActorID, actors[3].ActorID})

	sortTopologyLinks(links)
	require.Equal(t, "ip:10.0.0.2", links[0].SrcActorID)
	require.Equal(t, "mac:00:11:22:33:44:55", links[0].DstActorID)
	require.Equal(t, "mac:00:11:22:33:44:55", links[1].SrcActorID)
	require.Equal(t, "ip:10.0.0.2", links[1].DstActorID)
}

func TestEnrichTopologyPortTablesWithLinkCounts_AddsCountsToMatchingPorts(t *testing.T) {
	actors := []Actor{
		{
			ActorID: "device-a",
			Tables: map[string][]map[string]any{
				"ports": {
					{"name": "eth0"},
					{"name": "eth1"},
				},
			},
		},
		{
			ActorID: "device-b",
			Tables: map[string][]map[string]any{
				"ports": {
					{"name": "xe-0/0/0"},
				},
			},
		},
	}
	links := []Link{
		{
			SrcActorID: "device-a",
			DstActorID: "device-b",
			Src:        LinkEndpoint{Attributes: map[string]any{"if_name": "eth0"}},
			Dst:        LinkEndpoint{Attributes: map[string]any{"if_name": "xe-0/0/0"}},
		},
		{
			SrcActorID: "device-a",
			DstActorID: "device-b",
			Src:        LinkEndpoint{Attributes: map[string]any{"if_name": "eth0"}},
			Dst:        LinkEndpoint{Attributes: map[string]any{"if_name": "xe-0/0/0"}},
		},
	}

	enrichTopologyPortTablesWithLinkCounts(actors, links)

	require.Equal(t, 2, actors[0].Tables["ports"][0]["link_count"])
	_, exists := actors[0].Tables["ports"][1]["link_count"]
	require.False(t, exists)
	require.Equal(t, 2, actors[1].Tables["ports"][0]["link_count"])
}
