// SPDX-License-Identifier: GPL-3.0-or-later

package topologyshape

import (
	"errors"
	"math/rand"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
	"github.com/stretchr/testify/require"
)

func TestTopologyShortestPathUnionMatchesPairPerTargetReference(t *testing.T) {
	tests := map[string]struct {
		nodes int
		edges [][2]int
		roots []int
	}{
		"diamond": {
			nodes: 4, edges: [][2]int{{0, 1}, {0, 2}, {1, 3}, {2, 3}}, roots: []int{0, 3},
		},
		"cycle": {
			nodes: 4, edges: [][2]int{{0, 1}, {1, 2}, {2, 3}, {3, 0}}, roots: []int{0, 2},
		},
		"disconnected": {
			nodes: 5, edges: [][2]int{{0, 1}, {1, 2}, {3, 4}}, roots: []int{0, 2, 4},
		},
		"parallel links": {
			nodes: 3, edges: [][2]int{{0, 1}, {0, 1}, {1, 2}, {1, 2}}, roots: []int{0, 2},
		},
		"all-roots chain": {
			nodes: 12,
			edges: [][2]int{{0, 1}, {1, 2}, {2, 3}, {3, 4}, {4, 5}, {5, 6}, {6, 7}, {7, 8}, {8, 9}, {9, 10}, {10, 11}},
			roots: []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			random := rand.New(rand.NewSource(42))
			for iteration := range 100 {
				allocator := graph.NewActorHandleAllocator()
				handles := make([]topologymodel.ActorHandle, tc.nodes)
				data := topologymodel.Data{Actors: make([]topologymodel.Actor, tc.nodes)}
				for i := range data.Actors {
					handles[i] = allocator.Next()
					data.Actors[i] = topologymodel.Actor{ActorHandle: handles[i]}
				}
				edges := append([][2]int(nil), tc.edges...)
				random.Shuffle(len(edges), func(i, j int) { edges[i], edges[j] = edges[j], edges[i] })
				for _, edge := range edges {
					data.Links = append(data.Links, topologymodel.Link{
						SrcActorHandle: handles[edge[0]], DstActorHandle: handles[edge[1]],
					})
				}
				roots := make(map[topologymodel.ActorHandle]struct{}, len(tc.roots))
				for _, index := range tc.roots {
					roots[handles[index]] = struct{}{}
				}

				wantActors, wantPairs := pairPerTargetShortestPathUnionReference(&data, roots)
				gotActors, gotPairs, err := topologyShortestPathUnion(&data, roots, nil)
				require.NoError(t, err, "iteration %d", iteration)
				require.Equal(t, wantActors, gotActors, "actor set iteration %d", iteration)
				require.Equal(t, wantPairs, gotPairs, "pair set iteration %d", iteration)
			}
		})
	}
}

func pairPerTargetShortestPathUnionReference(
	data *topologymodel.Data,
	roots map[topologymodel.ActorHandle]struct{},
) (map[topologymodel.ActorHandle]struct{}, map[topologyActorPair]struct{}) {
	includedActors := make(map[topologymodel.ActorHandle]struct{})
	includedPairs := make(map[topologyActorPair]struct{})
	if data == nil || len(roots) < 2 {
		return includedActors, includedPairs
	}
	adjacency := make(map[topologymodel.ActorHandle]map[topologymodel.ActorHandle]struct{})
	for _, link := range data.Links {
		src, dst := link.SrcActorHandle, link.DstActorHandle
		if src.IsZero() || dst.IsZero() || src == dst {
			continue
		}
		if adjacency[src] == nil {
			adjacency[src] = make(map[topologymodel.ActorHandle]struct{})
		}
		if adjacency[dst] == nil {
			adjacency[dst] = make(map[topologymodel.ActorHandle]struct{})
		}
		adjacency[src][dst] = struct{}{}
		adjacency[dst][src] = struct{}{}
	}
	rootIDs := make([]topologymodel.ActorHandle, 0, len(roots))
	for root := range roots {
		rootIDs = append(rootIDs, root)
	}
	for i, source := range rootIDs {
		parents, distance := topologyShortestParents(adjacency, source)
		for _, target := range rootIDs[i+1:] {
			if _, reachable := distance[target]; !reachable {
				continue
			}
			visited := make(map[topologymodel.ActorHandle]struct{})
			stack := []topologymodel.ActorHandle{target}
			for len(stack) > 0 {
				node := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				if _, seen := visited[node]; seen {
					continue
				}
				visited[node] = struct{}{}
				includedActors[node] = struct{}{}
				if node == source {
					continue
				}
				for _, parent := range parents[node] {
					includedActors[parent] = struct{}{}
					includedPairs[topologyActorPair{src: node, dst: parent}] = struct{}{}
					includedPairs[topologyActorPair{src: parent, dst: node}] = struct{}{}
					stack = append(stack, parent)
				}
			}
		}
	}
	return includedActors, includedPairs
}

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
	want := uint64(len(data.Links)+len(roots)) + rootCount*uint64(nodeCount+2*len(data.Links))
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

func TestApplyDepthFocusFilterChargesAllDevicesStatsBeforeMutation(t *testing.T) {
	allocator := graph.NewActorHandleAllocator()
	left := allocator.Next()
	right := allocator.Next()
	data := topologymodel.Data{
		Actors: []topologymodel.Actor{{ActorHandle: left}, {ActorHandle: right}},
		Links:  []topologymodel.Link{{SrcActorHandle: left, DstActorHandle: right}},
	}
	want := data
	want.Actors = append([]topologymodel.Actor(nil), data.Actors...)
	want.Links = append([]topologymodel.Link(nil), data.Links...)
	limitErr := errors.New("all-devices work exhausted")
	err := ApplyDepthFocusFilter(&data, topologyoptions.QueryOptions{
		ManagedDeviceFocus: topologyoptions.ManagedFocusAllDevices,
		WorkLimiter:        func(uint64) error { return limitErr },
	})
	require.ErrorIs(t, err, limitErr)
	require.Equal(t, want, data)
}
