// SPDX-License-Identifier: GPL-3.0-or-later

package topologyshape

import (
	"errors"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/stretchr/testify/require"
)

func TestTopologyShortestPathUnionChargesOneTraversalEnvelopePerRoot(t *testing.T) {
	const nodeCount = 16
	const rootCount = 8
	allocator := graph.NewActorHandleAllocator()
	data := topologymodel.Data{Actors: make([]topologymodel.Actor, nodeCount)}
	roots := make(map[topologymodel.ActorHandle]struct{}, rootCount)
	for i := range data.Actors {
		handle := allocator.Next()
		data.Actors[i] = topologymodel.Actor{ActorHandle: handle}
		if i < rootCount {
			roots[handle] = struct{}{}
		}
		if i > 0 {
			data.Links = append(data.Links, topologymodel.Link{
				SrcActorHandle: data.Actors[i-1].ActorHandle,
				DstActorHandle: handle,
			})
		}
	}

	var charged uint64
	actors, pairs, err := topologyShortestPathUnion(&data, roots, func(units uint64) error {
		charged += units
		return nil
	})
	require.NoError(t, err)
	require.Len(t, actors, rootCount)
	require.Len(t, pairs, 2*(rootCount-1))
	want := uint64(len(data.Links)) + rootCount*uint64(nodeCount+2*len(data.Links))
	require.Equal(t, want, charged)
}

func TestTopologyShortestPathUnionRejectsBeforeRootTraversals(t *testing.T) {
	allocator := graph.NewActorHandleAllocator()
	left := allocator.Next()
	right := allocator.Next()
	data := topologymodel.Data{
		Actors: []topologymodel.Actor{{ActorHandle: left}, {ActorHandle: right}},
		Links:  []topologymodel.Link{{SrcActorHandle: left, DstActorHandle: right}},
	}
	limitErr := errors.New("focus work exhausted")
	calls := 0
	_, _, err := topologyShortestPathUnion(
		&data,
		map[topologymodel.ActorHandle]struct{}{left: {}, right: {}},
		func(uint64) error {
			calls++
			if calls == 2 {
				return limitErr
			}
			return nil
		},
	)
	require.ErrorIs(t, err, limitErr)
	require.Equal(t, 2, calls)
}
