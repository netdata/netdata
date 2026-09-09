// SPDX-License-Identifier: GPL-3.0-or-later

package topologyv1

import (
	"strings"
	"sync"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/stretchr/testify/require"
)

var topologyV1TestHandles = struct {
	sync.Mutex
	allocator graph.ActorHandleAllocator
	byActorID map[string]topologymodel.ActorHandle
}{
	allocator: graph.NewActorHandleAllocator(),
	byActorID: make(map[string]topologymodel.ActorHandle),
}

func topologyV1TestActorHandle(actorID string) topologymodel.ActorHandle {
	actorID = strings.TrimSpace(actorID)
	topologyV1TestHandles.Lock()
	defer topologyV1TestHandles.Unlock()
	if handle := topologyV1TestHandles.byActorID[actorID]; !handle.IsZero() {
		return handle
	}
	handle := topologyV1TestHandles.allocator.Next()
	topologyV1TestHandles.byActorID[actorID] = handle
	return handle
}

func assignTopologyV1TestHandles(t testing.TB, data *topologymodel.Data) map[string]topologymodel.ActorHandle {
	t.Helper()
	byActorID := make(map[string]topologymodel.ActorHandle, len(data.Actors))
	for i := range data.Actors {
		data.Actors[i].ActorHandle = topologyV1TestActorHandle(data.Actors[i].ActorID)
		byActorID[strings.TrimSpace(data.Actors[i].ActorID)] = data.Actors[i].ActorHandle
	}
	require.NoError(t, data.InitializeActorHandles())
	return byActorID
}
