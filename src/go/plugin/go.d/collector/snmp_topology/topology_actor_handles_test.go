// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"strings"
	"sync"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/stretchr/testify/require"
)

var snmpTopologyTestHandles = struct {
	sync.Mutex
	allocator graph.ActorHandleAllocator
	byActorID map[string]topologymodel.ActorHandle
}{
	allocator: graph.NewActorHandleAllocator(),
	byActorID: make(map[string]topologymodel.ActorHandle),
}

func snmpTopologyTestActorHandle(actorID string) topologymodel.ActorHandle {
	actorID = strings.TrimSpace(actorID)
	snmpTopologyTestHandles.Lock()
	defer snmpTopologyTestHandles.Unlock()
	if handle := snmpTopologyTestHandles.byActorID[actorID]; !handle.IsZero() {
		return handle
	}
	handle := snmpTopologyTestHandles.allocator.Next()
	snmpTopologyTestHandles.byActorID[actorID] = handle
	return handle
}

func assignSNMPTopologyTestHandles(t testing.TB, data *topologymodel.Data) map[string]topologymodel.ActorHandle {
	t.Helper()
	byActorID := make(map[string]topologymodel.ActorHandle, len(data.Actors))
	allAssigned := len(data.Actors) > 0
	for _, actor := range data.Actors {
		allAssigned = allAssigned && !actor.ActorHandle.IsZero()
	}
	if !allAssigned {
		for i := range data.Actors {
			data.Actors[i].ActorHandle = snmpTopologyTestActorHandle(data.Actors[i].ActorID)
		}
	}
	for _, actor := range data.Actors {
		byActorID[strings.TrimSpace(actor.ActorID)] = actor.ActorHandle
	}
	require.NoError(t, data.InitializeActorHandles())
	return byActorID
}
