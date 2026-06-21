// SPDX-License-Identifier: GPL-3.0-or-later

package l2topology

import (
	"math"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
	"github.com/stretchr/testify/require"
)

func TestBackfillPairGroupMissingEndpointPortsCopiesPeerInterfaceAttributes(t *testing.T) {
	entries := []*builtAdjacencyLink{
		{
			adj: Adjacency{
				SourceID: "device-a",
				TargetID: "device-b",
			},
			link: graph.Link{
				Src: graph.LinkEndpoint{},
				Dst: graph.LinkEndpoint{},
			},
		},
		{
			adj: Adjacency{
				SourceID: "device-b",
				TargetID: "device-a",
			},
			link: graph.Link{
				Src: graph.LinkEndpoint{
					IfIndex: 2,
					IfName:  "Gi0/2",
					PortID:  "Gi0/2",
				},
				Dst: graph.LinkEndpoint{
					IfIndex: 1,
					IfName:  "Gi0/1",
					PortID:  "Gi0/1",
				},
			},
		},
	}

	backfillPairGroupMissingEndpointPorts(entries)

	require.Equal(t, 1, entries[0].link.Src.IfIndex)
	require.Equal(t, "Gi0/1", entries[0].link.Src.IfName)
	require.Equal(t, "Gi0/1", entries[0].link.Src.PortID)
	require.Equal(t, 2, entries[0].link.Dst.IfIndex)
	require.Equal(t, "Gi0/2", entries[0].link.Dst.IfName)
	require.Equal(t, "Gi0/2", entries[0].link.Dst.PortID)
}

func TestBackfillPairGroupMissingEndpointPortsSkipsAmbiguousReverseCandidates(t *testing.T) {
	entries := []*builtAdjacencyLink{
		{
			adj: Adjacency{
				SourceID: "device-a",
				TargetID: "device-b",
			},
			link: graph.Link{
				Src: graph.LinkEndpoint{},
				Dst: graph.LinkEndpoint{},
			},
		},
		{
			adj: Adjacency{
				SourceID: "device-b",
				TargetID: "device-a",
			},
			link: graph.Link{
				Src: graph.LinkEndpoint{IfName: "Gi0/2"},
				Dst: graph.LinkEndpoint{IfName: "Gi0/1"},
			},
		},
		{
			adj: Adjacency{
				SourceID: "device-b",
				TargetID: "device-a",
			},
			link: graph.Link{
				Src: graph.LinkEndpoint{IfName: "Gi0/22"},
				Dst: graph.LinkEndpoint{IfName: "Gi0/11"},
			},
		},
	}

	backfillPairGroupMissingEndpointPorts(entries)

	require.Empty(t, entries[0].link.Src.IfName)
	require.Empty(t, entries[0].link.Dst.IfName)
}

func TestBackfillEndpointPortFromPeerPreservesExistingCanonicalPort(t *testing.T) {
	endpoint := graph.LinkEndpoint{
		IfName: "Gi0/10",
	}
	peer := graph.LinkEndpoint{
		IfIndex: 7,
		IfName:  "Gi0/7",
		PortID:  "Gi0/7",
	}

	backfilled := backfillEndpointPortFromPeer(endpoint, peer)

	require.Equal(t, "Gi0/10", backfilled.IfName)
	require.Zero(t, backfilled.IfIndex)
	require.Empty(t, backfilled.PortID)
}

func TestTopologyIntegerAttributesParseSNMPEnumLabels(t *testing.T) {
	attrs := map[string]any{
		"if_index": "up(1)",
		"speed":    "fast(1000000000)",
	}

	require.Equal(t, 1, topologyAttrInt(attrs, "if_index"))
	require.Equal(t, int64(1000000000), topologyAttrInt64(attrs, "speed"))
	require.Equal(t, OptionalValue[int]{Has: true, Value: 1}, topologyOptionalAttrInt(attrs, "if_index"))
	require.Equal(t, OptionalValue[int64]{Has: true, Value: 1000000000}, topologyOptionalAttrInt64(attrs, "speed"))
}

func TestTopologyAttrIntMapClampsParsedValues(t *testing.T) {
	counts := topologyAttrIntMap(map[string]any{
		"counts": map[string]any{
			"huge": int64(math.MaxInt64),
			"ok":   "active(7)",
			"neg":  int64(-1),
		},
	}, "counts")

	require.Equal(t, map[string]int{
		"huge": maxProjectionInt,
		"ok":   7,
		"neg":  0,
	}, counts)
}

func TestSegmentProjectionBuilderPruneSegmentsWithoutLinksRemovesEmptySegments(t *testing.T) {
	builder := &segmentProjectionBuilder{
		segmentIDs: []string{"segment-a", "segment-b"},
		out: projectedSegments{
			actors: []graph.Actor{
				{
					ActorID:   "segment-a",
					ActorType: "segment",
					Attributes: map[string]any{
						"segment_id": "segment-a",
					},
				},
				{
					ActorID:   "segment-b",
					ActorType: "segment",
					Attributes: map[string]any{
						"segment_id": "segment-b",
					},
				},
			},
			links: []graph.Link{
				{
					Protocol:  "fdb",
					Direction: "bidirectional",
					L2:        &graph.LinkL2{BridgeDomain: "segment-a"},
				},
				{
					Protocol:  "fdb",
					Direction: "bidirectional",
					L2:        &graph.LinkL2{BridgeDomain: "segment-b"},
				},
			},
		},
	}

	builder.pruneSegmentsWithoutLinks(map[string]struct{}{
		"segment-a": {},
	})

	require.Len(t, builder.out.actors, 1)
	require.Equal(t, "segment-a", builder.out.actors[0].ActorID)
	require.Len(t, builder.out.links, 1)
	require.Equal(t, "segment-a", topologyLinkBridgeDomain(builder.out.links[0]))
	require.Equal(t, 1, builder.out.linksFdb)
	require.Equal(t, 1, builder.out.bidirectionalCount)
	require.Equal(t, 1, builder.out.endpointLinksEmitted)
}
