// SPDX-License-Identifier: GPL-3.0-or-later

package topologyshape

import (
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
)

type topologyFocusGraph struct {
	actorByHandle    map[topologymodel.ActorHandle]topologymodel.Actor
	segmentSet       map[topologymodel.ActorHandle]struct{}
	segmentKind      map[topologymodel.ActorHandle]string
	nonSegmentSet    map[topologymodel.ActorHandle]struct{}
	nonSegmentAdj    map[topologymodel.ActorHandle]map[topologymodel.ActorHandle]struct{}
	nodeSegments     map[topologymodel.ActorHandle]map[topologymodel.ActorHandle]struct{}
	segmentNeighbors map[topologymodel.ActorHandle]map[topologymodel.ActorHandle]struct{}
}

func buildTopologyFocusGraph(data *topologymodel.Data) topologyFocusGraph {
	graph := topologyFocusGraph{
		actorByHandle:    make(map[topologymodel.ActorHandle]topologymodel.Actor, len(data.Actors)),
		segmentSet:       make(map[topologymodel.ActorHandle]struct{}),
		segmentKind:      make(map[topologymodel.ActorHandle]string),
		nonSegmentSet:    make(map[topologymodel.ActorHandle]struct{}),
		nonSegmentAdj:    make(map[topologymodel.ActorHandle]map[topologymodel.ActorHandle]struct{}),
		nodeSegments:     make(map[topologymodel.ActorHandle]map[topologymodel.ActorHandle]struct{}),
		segmentNeighbors: make(map[topologymodel.ActorHandle]map[topologymodel.ActorHandle]struct{}),
	}

	for _, actor := range data.Actors {
		handle := actor.ActorHandle
		if handle.IsZero() {
			continue
		}
		graph.actorByHandle[handle] = actor
		if topologymodel.ActorIsSegment(actor) {
			graph.segmentSet[handle] = struct{}{}
			graph.segmentKind[handle] = topologymodel.ActorSegmentKind(actor)
		} else {
			graph.nonSegmentSet[handle] = struct{}{}
		}
	}

	for actorHandle := range graph.nonSegmentSet {
		graph.nonSegmentAdj[actorHandle] = make(map[topologymodel.ActorHandle]struct{})
		graph.nodeSegments[actorHandle] = make(map[topologymodel.ActorHandle]struct{})
	}
	for segmentHandle := range graph.segmentSet {
		graph.segmentNeighbors[segmentHandle] = make(map[topologymodel.ActorHandle]struct{})
	}

	for _, link := range data.Links {
		src := link.SrcActorHandle
		dst := link.DstActorHandle
		if src.IsZero() || dst.IsZero() || src == dst {
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

func traverseTopologyFocusDepth(graph topologyFocusGraph, roots map[topologymodel.ActorHandle]struct{}, depth int) map[topologymodel.ActorHandle]int {
	distance := make(map[topologymodel.ActorHandle]int, len(graph.nonSegmentSet))
	queue := make([]topologymodel.ActorHandle, 0, len(roots))
	for root := range roots {
		distance[root] = 0
		queue = append(queue, root)
	}
	segmentExpandedDepth := make(map[topologymodel.ActorHandle]int)

	for head := 0; head < len(queue); head++ {
		current := queue[head]
		currentDepth := distance[current]
		if depth != topologyoptions.DepthAllInternal && currentDepth >= depth {
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
			if graph.segmentKind[segmentID] == topologymodel.SegmentKindL3Subnet {
				continue
			}
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
	distance map[topologymodel.ActorHandle]int,
	depth int,
) (map[topologymodel.ActorHandle]struct{}, map[topologymodel.ActorHandle]struct{}) {
	includedNonSegment := make(map[topologymodel.ActorHandle]struct{}, len(distance))
	for actorHandle, currentDepth := range distance {
		if depth == topologyoptions.DepthAllInternal || currentDepth <= depth {
			includedNonSegment[actorHandle] = struct{}{}
		}
	}

	includedActorsByDepth := make(map[topologymodel.ActorHandle]struct{}, len(includedNonSegment)+len(graph.segmentSet))
	for actorHandle := range includedNonSegment {
		includedActorsByDepth[actorHandle] = struct{}{}
	}
	if depth == topologyoptions.DepthAllInternal || depth > 0 {
		for segmentHandle, neighbors := range graph.segmentNeighbors {
			for actorHandle := range neighbors {
				if _, ok := includedNonSegment[actorHandle]; ok {
					includedActorsByDepth[segmentHandle] = struct{}{}
					break
				}
			}
		}
	}

	return includedNonSegment, includedActorsByDepth
}

func topologyActorLexicalOrder(actors []topologymodel.Actor) map[topologymodel.ActorHandle]int {
	type entry struct {
		handle  topologymodel.ActorHandle
		actorID string
	}
	entries := make([]entry, 0, len(actors))
	for _, actor := range actors {
		if actor.ActorHandle.IsZero() {
			continue
		}
		entries = append(entries, entry{handle: actor.ActorHandle, actorID: strings.TrimSpace(actor.ActorID)})
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].actorID < entries[j].actorID })
	order := make(map[topologymodel.ActorHandle]int, len(entries))
	for i, entry := range entries {
		order[entry.handle] = i
	}
	return order
}
