// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/topology"
	"github.com/stretchr/testify/require"
)

func TestBackfillPairGroupMissingEndpointPortsCopiesPeerInterfaceAttributes(t *testing.T) {
	entries := []*builtAdjacencyLink{
		{
			adj: Adjacency{
				SourceID: "device-a",
				TargetID: "device-b",
			},
			link: topology.Link{
				Src: topology.LinkEndpoint{Attributes: map[string]any{}},
				Dst: topology.LinkEndpoint{Attributes: map[string]any{}},
			},
		},
		{
			adj: Adjacency{
				SourceID: "device-b",
				TargetID: "device-a",
			},
			link: topology.Link{
				Src: topology.LinkEndpoint{Attributes: map[string]any{
					"if_index": 2,
					"if_name":  "Gi0/2",
					"port_id":  "Gi0/2",
				}},
				Dst: topology.LinkEndpoint{Attributes: map[string]any{
					"if_index": 1,
					"if_name":  "Gi0/1",
					"port_id":  "Gi0/1",
				}},
			},
		},
	}

	backfillPairGroupMissingEndpointPorts(entries)

	require.Equal(t, 1, topologyAttrInt(entries[0].link.Src.Attributes, "if_index"))
	require.Equal(t, "Gi0/1", topologyAttrString(entries[0].link.Src.Attributes, "if_name"))
	require.Equal(t, "Gi0/1", topologyAttrString(entries[0].link.Src.Attributes, "port_id"))
	require.Equal(t, 2, topologyAttrInt(entries[0].link.Dst.Attributes, "if_index"))
	require.Equal(t, "Gi0/2", topologyAttrString(entries[0].link.Dst.Attributes, "if_name"))
	require.Equal(t, "Gi0/2", topologyAttrString(entries[0].link.Dst.Attributes, "port_id"))
}

func TestBackfillPairGroupMissingEndpointPortsSkipsAmbiguousReverseCandidates(t *testing.T) {
	entries := []*builtAdjacencyLink{
		{
			adj: Adjacency{
				SourceID: "device-a",
				TargetID: "device-b",
			},
			link: topology.Link{
				Src: topology.LinkEndpoint{Attributes: map[string]any{}},
				Dst: topology.LinkEndpoint{Attributes: map[string]any{}},
			},
		},
		{
			adj: Adjacency{
				SourceID: "device-b",
				TargetID: "device-a",
			},
			link: topology.Link{
				Src: topology.LinkEndpoint{Attributes: map[string]any{
					"if_name": "Gi0/2",
				}},
				Dst: topology.LinkEndpoint{Attributes: map[string]any{
					"if_name": "Gi0/1",
				}},
			},
		},
		{
			adj: Adjacency{
				SourceID: "device-b",
				TargetID: "device-a",
			},
			link: topology.Link{
				Src: topology.LinkEndpoint{Attributes: map[string]any{
					"if_name": "Gi0/22",
				}},
				Dst: topology.LinkEndpoint{Attributes: map[string]any{
					"if_name": "Gi0/11",
				}},
			},
		},
	}

	backfillPairGroupMissingEndpointPorts(entries)

	require.Equal(t, "", topologyAttrString(entries[0].link.Src.Attributes, "if_name"))
	require.Equal(t, "", topologyAttrString(entries[0].link.Dst.Attributes, "if_name"))
}

func TestBackfillEndpointPortFromPeerPreservesExistingCanonicalPort(t *testing.T) {
	endpoint := topology.LinkEndpoint{
		Attributes: map[string]any{
			"if_name": "Gi0/10",
		},
	}
	peer := topology.LinkEndpoint{
		Attributes: map[string]any{
			"if_index": 7,
			"if_name":  "Gi0/7",
			"port_id":  "Gi0/7",
		},
	}

	backfilled := backfillEndpointPortFromPeer(endpoint, peer)

	require.Equal(t, "Gi0/10", topologyAttrString(backfilled.Attributes, "if_name"))
	require.Zero(t, topologyAttrInt(backfilled.Attributes, "if_index"))
	require.Equal(t, "", topologyAttrString(backfilled.Attributes, "port_id"))
}

func TestSegmentProjectionBuilderPruneSegmentsWithoutLinksRemovesEmptySegments(t *testing.T) {
	builder := &segmentProjectionBuilder{
		segmentIDs: []string{"segment-a", "segment-b"},
		out: projectedSegments{
			actors: []topology.Actor{
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
			links: []topology.Link{
				{
					Protocol:  "fdb",
					Direction: "bidirectional",
					Metrics: map[string]any{
						"bridge_domain": "segment-a",
					},
				},
				{
					Protocol:  "fdb",
					Direction: "bidirectional",
					Metrics: map[string]any{
						"bridge_domain": "segment-b",
					},
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
	require.Equal(t, "segment-a", topologyMetricString(builder.out.links[0].Metrics, "bridge_domain"))
	require.Equal(t, 1, builder.out.linksFdb)
	require.Equal(t, 1, builder.out.bidirectionalCount)
	require.Equal(t, 1, builder.out.endpointLinksEmitted)
}
