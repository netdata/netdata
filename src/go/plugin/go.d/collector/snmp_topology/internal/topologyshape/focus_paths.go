// SPDX-License-Identifier: GPL-3.0-or-later

package topologyshape

import (
	"sort"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
)

func topologyShortestPathUnion(
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
		src := link.SrcActorHandle
		dst := link.DstActorHandle
		if src.IsZero() || dst.IsZero() || src == dst {
			continue
		}
		if _, ok := adjacency[src]; !ok {
			adjacency[src] = make(map[topologymodel.ActorHandle]struct{})
		}
		if _, ok := adjacency[dst]; !ok {
			adjacency[dst] = make(map[topologymodel.ActorHandle]struct{})
		}
		adjacency[src][dst] = struct{}{}
		adjacency[dst][src] = struct{}{}
	}

	actorOrder := topologyActorLexicalOrder(data.Actors)
	rootIDs := make([]topologymodel.ActorHandle, 0, len(roots))
	for actorHandle := range roots {
		rootIDs = append(rootIDs, actorHandle)
	}
	sort.Slice(rootIDs, func(i, j int) bool { return actorOrder[rootIDs[i]] < actorOrder[rootIDs[j]] })

	for i := 0; i < len(rootIDs); i++ {
		source := rootIDs[i]
		if _, ok := adjacency[source]; !ok {
			continue
		}

		parents, distance := topologyShortestParents(adjacency, actorOrder, source)
		for j := i + 1; j < len(rootIDs); j++ {
			target := rootIDs[j]
			if _, ok := distance[target]; !ok {
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

func topologyShortestParents(
	adjacency map[topologymodel.ActorHandle]map[topologymodel.ActorHandle]struct{},
	actorOrder map[topologymodel.ActorHandle]int,
	source topologymodel.ActorHandle,
) (map[topologymodel.ActorHandle][]topologymodel.ActorHandle, map[topologymodel.ActorHandle]int) {
	parents := make(map[topologymodel.ActorHandle][]topologymodel.ActorHandle)
	distance := map[topologymodel.ActorHandle]int{source: 0}
	queue := []topologymodel.ActorHandle{source}

	for head := 0; head < len(queue); head++ {
		current := queue[head]
		neighbors := make([]topologymodel.ActorHandle, 0, len(adjacency[current]))
		for neighbor := range adjacency[current] {
			neighbors = append(neighbors, neighbor)
		}
		sort.Slice(neighbors, func(i, j int) bool { return actorOrder[neighbors[i]] < actorOrder[neighbors[j]] })
		for _, neighbor := range neighbors {
			nextDepth := distance[current] + 1
			currentDepth, seen := distance[neighbor]
			if !seen {
				distance[neighbor] = nextDepth
				parents[neighbor] = []topologymodel.ActorHandle{current}
				queue = append(queue, neighbor)
				continue
			}
			if nextDepth == currentDepth {
				parents[neighbor] = append(parents[neighbor], current)
			}
		}
	}

	for node := range parents {
		sort.Slice(parents[node], func(i, j int) bool { return actorOrder[parents[node][i]] < actorOrder[parents[node][j]] })
	}

	return parents, distance
}

type topologyActorPair struct {
	src topologymodel.ActorHandle
	dst topologymodel.ActorHandle
}
