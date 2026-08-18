// SPDX-License-Identifier: GPL-3.0-or-later

package topologyenrich

import (
	"strings"
	"sync"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/stretchr/testify/require"
)

var topologyEnrichTestHandles = struct {
	sync.Mutex
	allocator graph.ActorHandleAllocator
	byActorID map[string]topologymodel.ActorHandle
}{
	allocator: graph.NewActorHandleAllocator(),
	byActorID: make(map[string]topologymodel.ActorHandle),
}

func topologyEnrichTestActorHandle(actorID string) topologymodel.ActorHandle {
	actorID = strings.TrimSpace(actorID)
	topologyEnrichTestHandles.Lock()
	defer topologyEnrichTestHandles.Unlock()
	if handle := topologyEnrichTestHandles.byActorID[actorID]; !handle.IsZero() {
		return handle
	}
	handle := topologyEnrichTestHandles.allocator.Next()
	topologyEnrichTestHandles.byActorID[actorID] = handle
	return handle
}

func assignTopologyEnrichTestHandles(t testing.TB, data *topologymodel.Data) map[string]topologymodel.ActorHandle {
	t.Helper()
	byActorID := make(map[string]topologymodel.ActorHandle, len(data.Actors))
	for i := range data.Actors {
		data.Actors[i].ActorHandle = topologyEnrichTestActorHandle(data.Actors[i].ActorID)
		byActorID[strings.TrimSpace(data.Actors[i].ActorID)] = data.Actors[i].ActorHandle
	}
	require.NoError(t, data.InitializeActorHandles())
	return byActorID
}
