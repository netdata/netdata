// SPDX-License-Identifier: GPL-3.0-or-later

package projector

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
	"github.com/stretchr/testify/require"
)

type topologyGraphForTest struct {
	graph.Graph
	ActorDetails map[string]ProjectionActorDetail
}

func toGraphForTest(result Result, opts GraphOptions) (topologyGraphForTest, ProjectionStats) {
	projection := ToGraph(result, opts)
	return topologyGraphForTest{
		Graph:        projection.Graph,
		ActorDetails: projection.ActorDetails,
	}, projection.Stats
}

func requireActorDetail(t *testing.T, data topologyGraphForTest, actor *graph.Actor) ProjectionActorDetail {
	t.Helper()
	require.NotNil(t, actor)
	detail, ok := data.ActorDetails[actor.ActorID]
	require.True(t, ok, "missing actor detail for %q", actor.ActorID)
	return detail
}
