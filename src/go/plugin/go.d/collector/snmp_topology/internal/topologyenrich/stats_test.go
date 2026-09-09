// SPDX-License-Identifier: GPL-3.0-or-later

package topologyenrich

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	topologyv1renderer "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyv1"
	"github.com/stretchr/testify/require"
)

func topologyStatsToV1ForTest(t *testing.T, stats topologymodel.Stats) map[string]any {
	t.Helper()

	payload, err := topologyv1renderer.Render(topologymodel.Data{Stats: stats})
	require.NoError(t, err)
	return payload.Stats
}
