// SPDX-License-Identifier: GPL-3.0-or-later

package topologyshape

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/stretchr/testify/require"
)

func TestFilterDanglingLinksUsesActorHandles(t *testing.T) {
	data := &topologymodel.Data{
		Actors: []topologymodel.Actor{
			{ActorID: "device-a"},
			{ActorID: "device-b"},
		},
		Links: []topologymodel.Link{
			{SrcActorHandle: topologyShapeTestActorHandle(" device-a "), DstActorHandle: topologyShapeTestActorHandle("\tdevice-b\n")},
			{SrcActorHandle: topologyShapeTestActorHandle("device-a"), DstActorHandle: topologyShapeTestActorHandle("missing")},
		},
	}
	assignTopologyShapeTestHandles(t, data)

	filterDanglingLinks(data)

	require.Len(t, data.Links, 1)
	require.Equal(t, topologyShapeTestActorHandle("device-a"), data.Links[0].SrcActorHandle)
	require.Equal(t, topologyShapeTestActorHandle("device-b"), data.Links[0].DstActorHandle)
}

func TestPruneSparseSegmentsRemovesMultiRoundFixpoint(t *testing.T) {
	data := &topologymodel.Data{
		Actors: []topologymodel.Actor{
			{ActorID: "device-a", ActorType: "device"},
			{ActorID: "segment-a", ActorType: "segment", SegmentKind: topologymodel.SegmentKindBroadcastDomain},
			{ActorID: "segment-b", ActorType: "segment", SegmentKind: topologymodel.SegmentKindBroadcastDomain},
		},
		Links: []topologymodel.Link{
			{SrcActorHandle: topologyShapeTestActorHandle("device-a"), DstActorHandle: topologyShapeTestActorHandle("segment-a")},
			{SrcActorHandle: topologyShapeTestActorHandle("segment-a"), DstActorHandle: topologyShapeTestActorHandle("segment-b")},
		},
	}
	assignTopologyShapeTestHandles(t, data)
	expectedActor := data.Actors[0]

	removed := pruneSparseSegments(data, 1)

	require.Equal(t, 2, removed)
	require.Equal(t, []topologymodel.Actor{expectedActor}, data.Actors)
	require.Empty(t, data.Links)
}

func TestPruneSparseSegmentsUsesLinkEndpointHandles(t *testing.T) {
	data := &topologymodel.Data{
		Actors: []topologymodel.Actor{
			{ActorID: "device-a", ActorType: "device"},
			{ActorID: "device-b", ActorType: "device"},
			{ActorID: "segment-a", ActorType: "segment", SegmentKind: topologymodel.SegmentKindBroadcastDomain},
		},
		Links: []topologymodel.Link{
			{SrcActorHandle: topologyShapeTestActorHandle("device-a"), DstActorHandle: topologyShapeTestActorHandle(" segment-a ")},
			{SrcActorHandle: topologyShapeTestActorHandle("\tsegment-a\n"), DstActorHandle: topologyShapeTestActorHandle("device-b")},
		},
	}
	assignTopologyShapeTestHandles(t, data)

	removed := pruneSparseSegments(data, 1)

	require.Zero(t, removed)
	require.Len(t, data.Actors, 3)
	require.Len(t, data.Links, 2)
}

func TestPruneSparseSegmentsKeepsVisibleL3SubnetSegment(t *testing.T) {
	data := &topologymodel.Data{
		Actors: []topologymodel.Actor{
			{ActorID: "router-a", ActorType: "router"},
			{ActorID: "subnet-a", ActorType: topologymodel.L3SubnetSegmentActorType, SegmentKind: topologymodel.SegmentKindL3Subnet},
		},
		Links: []topologymodel.Link{
			{SrcActorHandle: topologyShapeTestActorHandle("router-a"), DstActorHandle: topologyShapeTestActorHandle("subnet-a"), Protocol: topologymodel.L3SubnetMembershipLinkType, LinkType: topologymodel.L3SubnetMembershipLinkType},
		},
	}
	assignTopologyShapeTestHandles(t, data)

	removed := pruneSparseSegments(data, 1)

	require.Zero(t, removed)
	require.Len(t, data.Actors, 2)
	require.Len(t, data.Links, 1)
}
