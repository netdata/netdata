// SPDX-License-Identifier: GPL-3.0-or-later

package topologymodel

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
	"github.com/stretchr/testify/require"
)

func TestInitializeActorHandlesPreservesGenerationHighWater(t *testing.T) {
	handles := graph.NewActorHandleAllocator()
	first := handles.Next()
	_ = handles.Next()
	third := handles.Next()
	data := Data{
		Actors: []Actor{{ActorHandle: first}, {ActorHandle: third}},
		Links:  []Link{{SrcActorHandle: first, DstActorHandle: third}},
	}

	require.NoError(t, data.InitializeActorHandles())
	next := data.NextActorHandle()
	require.NotEqual(t, first, next)
	require.NotEqual(t, third, next)
	data.Actors = append(data.Actors, Actor{ActorHandle: next})
	require.NoError(t, data.ValidateActorHandles())
}

func TestValidateActorHandlesRejectsBrokenIdentityGraph(t *testing.T) {
	firstGeneration := graph.NewActorHandleAllocator()
	first := firstGeneration.Next()
	second := firstGeneration.Next()
	otherGeneration := graph.NewActorHandleAllocator()
	other := otherGeneration.Next()

	tests := map[string]Data{
		"zero actor": {
			Actors: []Actor{{}},
		},
		"duplicate actor": {
			Actors: []Actor{{ActorHandle: first}, {ActorHandle: first}},
		},
		"mixed actor generations": {
			Actors: []Actor{{ActorHandle: first}, {ActorHandle: other}},
		},
		"dangling source": {
			Actors: []Actor{{ActorHandle: first}, {ActorHandle: second}},
			Links:  []Link{{SrcActorHandle: other, DstActorHandle: second}},
		},
		"dangling destination": {
			Actors: []Actor{{ActorHandle: first}, {ActorHandle: second}},
			Links:  []Link{{SrcActorHandle: first, DstActorHandle: other}},
		},
	}

	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			require.Error(t, data.InitializeActorHandles())
			require.Error(t, data.ValidateActorHandles())
		})
	}
}
