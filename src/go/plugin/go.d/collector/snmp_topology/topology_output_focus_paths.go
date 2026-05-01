// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"sort"
	"strings"
)

func topologyShortestPathUnion(
	data *topologyData,
	roots map[string]struct{},
) (map[string]struct{}, map[string]struct{}) {
	includedActors := make(map[string]struct{})
	includedPairs := make(map[string]struct{})
	if data == nil || len(roots) < 2 {
		return includedActors, includedPairs
	}

	adjacency := make(map[string]map[string]struct{})
	for _, link := range data.Links {
		src := strings.TrimSpace(link.SrcActorID)
		dst := strings.TrimSpace(link.DstActorID)
		if src == "" || dst == "" || src == dst {
			continue
		}
		if _, ok := adjacency[src]; !ok {
			adjacency[src] = make(map[string]struct{})
		}
		if _, ok := adjacency[dst]; !ok {
			adjacency[dst] = make(map[string]struct{})
		}
		adjacency[src][dst] = struct{}{}
		adjacency[dst][src] = struct{}{}
	}

	rootIDs := make([]string, 0, len(roots))
	for actorID := range roots {
		rootIDs = append(rootIDs, actorID)
	}
	sort.Strings(rootIDs)

	for i := 0; i < len(rootIDs); i++ {
		source := rootIDs[i]
		if _, ok := adjacency[source]; !ok {
			continue
		}

		parents, distance := topologyShortestParents(adjacency, source)
		for j := i + 1; j < len(rootIDs); j++ {
			target := rootIDs[j]
			if _, ok := distance[target]; !ok {
				continue
			}

			visited := make(map[string]struct{})
			stack := []string{target}
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
					includedPairs[topologyActorPairKey(node, parent)] = struct{}{}
					stack = append(stack, parent)
				}
			}
		}
	}

	return includedActors, includedPairs
}

func topologyShortestParents(
	adjacency map[string]map[string]struct{},
	source string,
) (map[string][]string, map[string]int) {
	parents := make(map[string][]string)
	distance := map[string]int{source: 0}
	queue := []string{source}

	for head := 0; head < len(queue); head++ {
		current := queue[head]
		neighbors := make([]string, 0, len(adjacency[current]))
		for neighbor := range adjacency[current] {
			neighbors = append(neighbors, neighbor)
		}
		sort.Strings(neighbors)
		for _, neighbor := range neighbors {
			nextDepth := distance[current] + 1
			currentDepth, seen := distance[neighbor]
			if !seen {
				distance[neighbor] = nextDepth
				parents[neighbor] = []string{current}
				queue = append(queue, neighbor)
				continue
			}
			if nextDepth == currentDepth {
				parents[neighbor] = append(parents[neighbor], current)
			}
		}
	}

	for node := range parents {
		sort.Strings(parents[node])
	}

	return parents, distance
}

func topologyActorPairKey(left, right string) string {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == "" || right == "" {
		return ""
	}
	if left > right {
		left, right = right, left
	}
	return left + "|" + right
}
