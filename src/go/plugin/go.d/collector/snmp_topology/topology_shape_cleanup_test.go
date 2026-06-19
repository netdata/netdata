// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFilterDanglingLinks_TrimActorIDsBeforeLookup(t *testing.T) {
	data := &topologyData{
		Actors: []topologyActor{
			{ActorID: "device-a"},
			{ActorID: "device-b"},
		},
		Links: []topologyLink{
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
	data := &topologyData{
		Actors: []topologyActor{
			{ActorID: "device-a", ActorType: "device"},
			{ActorID: "segment-a", ActorType: "segment"},
			{ActorID: "segment-b", ActorType: "segment"},
		},
		Links: []topologyLink{
			{SrcActorID: "device-a", DstActorID: "segment-a"},
			{SrcActorID: "segment-a", DstActorID: "segment-b"},
		},
	}

	removed := pruneSparseSegments(data, 1)

	require.Equal(t, 2, removed)
	require.Equal(t, []topologyActor{{ActorID: "device-a", ActorType: "device"}}, data.Actors)
	require.Empty(t, data.Links)
}
