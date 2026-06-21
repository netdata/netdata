// SPDX-License-Identifier: GPL-3.0-or-later

package l2topology

import "github.com/netdata/netdata/go/plugins/pkg/topology/graph"

func toGraphForTest(result Result, opts GraphOptions) (graph.Graph, ProjectionStats) {
	projection := ToGraph(result, opts)
	return projection.Graph, projection.Stats
}
