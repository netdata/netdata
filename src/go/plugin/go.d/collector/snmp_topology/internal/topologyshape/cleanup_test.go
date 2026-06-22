// SPDX-License-Identifier: GPL-3.0-or-later

package topologyshape

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/stretchr/testify/require"
)

func TestFilterDanglingLinks_TrimActorIDsBeforeLookup(t *testing.T) {
	data := &topologymodel.Data{
		Actors: []topologymodel.Actor{
			{ActorID: "device-a"},
			{ActorID: "device-b"},
		},
		Links: []topologymodel.Link{
			{SrcActorID: " device-a ", DstActorID: "\tdevice-b\n"},
			{SrcActorID: "device-a", DstActorID: "missing"},
		},
	}

	filterDanglingLinks(data)

	require.Len(t, data.Links, 1)
	require.Equal(t, " device-a ", data.Links[0].SrcActorID)
	require.Equal(t, "\tdevice-b\n", data.Links[0].DstActorID)
}

func TestPruneSparseSegmentsRemovesMultiRoundFixpoint(t *testing.T) {
	data := &topologymodel.Data{
		Actors: []topologymodel.Actor{
			{ActorID: "device-a", ActorType: "device"},
			{ActorID: "segment-a", ActorType: "segment", SegmentKind: topologymodel.SegmentKindBroadcastDomain},
			{ActorID: "segment-b", ActorType: "segment", SegmentKind: topologymodel.SegmentKindBroadcastDomain},
		},
		Links: []topologymodel.Link{
			{SrcActorID: "device-a", DstActorID: "segment-a"},
			{SrcActorID: "segment-a", DstActorID: "segment-b"},
		},
	}

	removed := pruneSparseSegments(data, 1)

	require.Equal(t, 2, removed)
	require.Equal(t, []topologymodel.Actor{{ActorID: "device-a", ActorType: "device"}}, data.Actors)
	require.Empty(t, data.Links)
}

func TestPruneSparseSegmentsKeepsVisibleL3SubnetSegment(t *testing.T) {
	data := &topologymodel.Data{
		Actors: []topologymodel.Actor{
			{ActorID: "router-a", ActorType: "router"},
			{ActorID: "subnet-a", ActorType: topologymodel.L3SubnetSegmentActorType, SegmentKind: topologymodel.SegmentKindL3Subnet},
		},
		Links: []topologymodel.Link{
			{SrcActorID: "router-a", DstActorID: "subnet-a", Protocol: topologymodel.L3SubnetMembershipLinkType, LinkType: topologymodel.L3SubnetMembershipLinkType},
		},
	}

	removed := pruneSparseSegments(data, 1)

	require.Zero(t, removed)
	require.Len(t, data.Actors, 2)
	require.Len(t, data.Links, 1)
}
