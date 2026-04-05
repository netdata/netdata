// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import "strings"

type topologyFocusGraph struct {
	actorByID        map[string]topologyActor
	segmentSet       map[string]struct{}
	nonSegmentSet    map[string]struct{}
	nonSegmentAdj    map[string]map[string]struct{}
	nodeSegments     map[string]map[string]struct{}
	segmentNeighbors map[string]map[string]struct{}
}

func buildTopologyFocusGraph(data *topologyData) topologyFocusGraph {
	graph := topologyFocusGraph{
		actorByID:        make(map[string]topologyActor, len(data.Actors)),
		segmentSet:       make(map[string]struct{}),
		nonSegmentSet:    make(map[string]struct{}),
		nonSegmentAdj:    make(map[string]map[string]struct{}),
		nodeSegments:     make(map[string]map[string]struct{}),
		segmentNeighbors: make(map[string]map[string]struct{}),
	}

	for _, actor := range data.Actors {
		id := strings.TrimSpace(actor.ActorID)
		if id == "" {
			continue
		}
		graph.actorByID[id] = actor
		if strings.EqualFold(strings.TrimSpace(actor.ActorType), "segment") {
			graph.segmentSet[id] = struct{}{}
		} else {
			graph.nonSegmentSet[id] = struct{}{}
		}
	}

	for actorID := range graph.nonSegmentSet {
		graph.nonSegmentAdj[actorID] = make(map[string]struct{})
		graph.nodeSegments[actorID] = make(map[string]struct{})
	}
	for segmentID := range graph.segmentSet {
		graph.segmentNeighbors[segmentID] = make(map[string]struct{})
	}

	for _, link := range data.Links {
		src := strings.TrimSpace(link.SrcActorID)
		dst := strings.TrimSpace(link.DstActorID)
		if src == "" || dst == "" || src == dst {
			continue
		}
		_, srcSegment := graph.segmentSet[src]
		_, dstSegment := graph.segmentSet[dst]
		_, srcNonSegment := graph.nonSegmentSet[src]
		_, dstNonSegment := graph.nonSegmentSet[dst]

		switch {
		case srcNonSegment && dstNonSegment:
			graph.nonSegmentAdj[src][dst] = struct{}{}
			graph.nonSegmentAdj[dst][src] = struct{}{}
		case srcSegment && dstNonSegment:
			graph.segmentNeighbors[src][dst] = struct{}{}
			graph.nodeSegments[dst][src] = struct{}{}
		case dstSegment && srcNonSegment:
			graph.segmentNeighbors[dst][src] = struct{}{}
			graph.nodeSegments[src][dst] = struct{}{}
		}
	}

	return graph
}

func traverseTopologyFocusDepth(graph topologyFocusGraph, roots map[string]struct{}, depth int) map[string]int {
	distance := make(map[string]int, len(graph.nonSegmentSet))
	queue := make([]string, 0, len(roots))
	for root := range roots {
		distance[root] = 0
		queue = append(queue, root)
	}
	segmentExpandedDepth := make(map[string]int)

	for head := 0; head < len(queue); head++ {
		current := queue[head]
		currentDepth := distance[current]
		if depth != topologyDepthAllInternal && currentDepth >= depth {
			continue
		}

		for neighbor := range graph.nonSegmentAdj[current] {
			if _, seen := distance[neighbor]; seen {
				continue
			}
			distance[neighbor] = currentDepth + 1
			queue = append(queue, neighbor)
		}

		for segmentID := range graph.nodeSegments[current] {
			if expandedAt, ok := segmentExpandedDepth[segmentID]; ok && expandedAt <= currentDepth {
				continue
			}
			segmentExpandedDepth[segmentID] = currentDepth
			for neighbor := range graph.segmentNeighbors[segmentID] {
				if _, seen := distance[neighbor]; seen {
					continue
				}
				distance[neighbor] = currentDepth + 1
				queue = append(queue, neighbor)
			}
		}
	}

	return distance
}

func collectTopologyFocusDepthSets(
	graph topologyFocusGraph,
	distance map[string]int,
	depth int,
) (map[string]struct{}, map[string]struct{}) {
	includedNonSegment := make(map[string]struct{}, len(distance))
	for actorID, currentDepth := range distance {
		if depth == topologyDepthAllInternal || currentDepth <= depth {
			includedNonSegment[actorID] = struct{}{}
		}
	}

	includedActorsByDepth := make(map[string]struct{}, len(includedNonSegment)+len(graph.segmentSet))
	for actorID := range includedNonSegment {
		includedActorsByDepth[actorID] = struct{}{}
	}
	if depth == topologyDepthAllInternal || depth > 0 {
		for segmentID, neighbors := range graph.segmentNeighbors {
			for actorID := range neighbors {
				if _, ok := includedNonSegment[actorID]; ok {
					includedActorsByDepth[segmentID] = struct{}{}
					break
				}
			}
		}
	}

	return includedNonSegment, includedActorsByDepth
}
