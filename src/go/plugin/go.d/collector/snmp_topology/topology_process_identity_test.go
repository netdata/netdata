// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyenrich"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	topologyv1renderer "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyv1"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyv1test"
)

func TestTopologyProcessIdentitySurvivesInventoryAndRendering(t *testing.T) {
	var rows []ddsnmp.BGPRow
	for process := 0; process < 3; process++ {
		rows = append(
			rows,
			ddsnmp.BGPRow{
				Kind: ddprofiledefinition.BGPRowKindPeer,
				Identity: ddsnmp.BGPIdentity{
					ProcessID:       fmt.Sprint(process),
					RoutingInstance: "7",
					Neighbor:        "192.0.2.2",
				},
			},
		)
	}
	cache := newTopologyBuilder()
	cache.ingestTopologyBGPPeers([]*ddsnmp.ProfileMetrics{{BGPRows: rows}})
	peers := cache.snapshotBGPPeers("router-a")
	require.Len(t, peers, 3, "fallback inventory keys retain the process axis without remote AS")
	data := topologymodel.Data{
		AgentID: "agent-test",
		View:    "summary",
		Actors:  []topologymodel.Actor{{ActorID: "router-a", ActorType: "router", Source: "snmp"}},
	}
	assignSNMPTopologyTestHandles(t, &data)
	stats := topologyenrich.ApplyBGPAdjacency(&data, topologymodel.ObservationAggregate{
		BGPPeers: peers,
	})
	assert.Equal(t, 3, stats.AttachedPeerRows)
	assert.Empty(t, data.Links, "statistics rows do not claim an established adjacency")
	require.Len(t, data.Actors[0].Detail.BGP, 3)
	payload, err := topologyv1renderer.Render(data)
	require.NoError(t, err)
	require.NoError(t, topologyv1test.ValidateData(payload))
	table := payload.Tables.Actor["actor_bgp_peers"].Table
	assert.Equal(t, 3, table.Rows)
	assert.Equal(t, []string{"0", "1", "2"}, topologyV1StringColumnValues(t, payload, table, "process_id"))
	assert.Equal(t, []string{"7", "7", "7"}, topologyV1StringColumnValues(t, payload, table, "routing_instance"))
	assert.Equal(t, []any{nil, nil, nil}, topologyV1ColumnValues(t, table, "remote_as"))
}
