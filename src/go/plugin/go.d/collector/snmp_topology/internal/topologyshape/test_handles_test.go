// SPDX-License-Identifier: GPL-3.0-or-later

package topologyshape

import (
	"strings"
	"sync"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
)

var topologyShapeTestHandles = struct {
	sync.Mutex
	allocator graph.ActorHandleAllocator
	byActorID map[string]topologymodel.ActorHandle
}{
	allocator: graph.NewActorHandleAllocator(),
	byActorID: make(map[string]topologymodel.ActorHandle),
}

func topologyShapeTestActorHandle(actorID string) topologymodel.ActorHandle {
	actorID = strings.TrimSpace(actorID)
	topologyShapeTestHandles.Lock()
	defer topologyShapeTestHandles.Unlock()
	if handle := topologyShapeTestHandles.byActorID[actorID]; !handle.IsZero() {
		return handle
	}
	handle := topologyShapeTestHandles.allocator.Next()
	topologyShapeTestHandles.byActorID[actorID] = handle
	return handle
}

func assignTopologyShapeTestHandles(t testing.TB, data *topologymodel.Data) map[string]topologymodel.ActorHandle {
	t.Helper()
	byActorID := make(map[string]topologymodel.ActorHandle, len(data.Actors))
	for i := range data.Actors {
		data.Actors[i].ActorHandle = topologyShapeTestActorHandle(data.Actors[i].ActorID)
		byActorID[strings.TrimSpace(data.Actors[i].ActorID)] = data.Actors[i].ActorHandle
	}
	return byActorID
}
