// SPDX-License-Identifier: GPL-3.0-or-later

package topologyv1

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/stretchr/testify/require"
)

func TestRenderRejectsInvalidActorHandles(t *testing.T) {
	handles := graph.NewActorHandleAllocator()
	first := handles.Next()
	second := handles.Next()
	otherGeneration := graph.NewActorHandleAllocator()
	dangling := otherGeneration.Next()

	tests := map[string]topologymodel.Data{
		"zero actor": {
			Actors: []topologymodel.Actor{{ActorID: "device-a"}},
		},
		"duplicate actor": {
			Actors: []topologymodel.Actor{
				{ActorHandle: first, ActorID: "device-a"},
				{ActorHandle: first, ActorID: "device-b"},
			},
		},
		"dangling link": {
			Actors: []topologymodel.Actor{
				{ActorHandle: first, ActorID: "device-a"},
				{ActorHandle: second, ActorID: "device-b"},
			},
			Links: []topologymodel.Link{{SrcActorHandle: first, DstActorHandle: dangling}},
		},
	}

	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := Render(data)
			require.Error(t, err)
		})
	}
}
