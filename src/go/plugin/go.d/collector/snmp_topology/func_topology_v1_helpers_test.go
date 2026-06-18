// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"testing"

	topologyv1 "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildSNMPTopologyV1DynamicTable_TrimsColumnKeysBeforeLookup(t *testing.T) {
	dict := topologyv1.NewStringDictionary("")

	table, err := buildSNMPTopologyV1DynamicTable([]topologyV1DynamicRow{
		{
			actorRef: 0,
			values: map[string]any{
				" speed ": uint64(1000),
			},
		},
	}, dict)

	require.NoError(t, err)
	assert.Equal(t, []any{uint64(1000)}, topologyV1TestColumnValues(t, table, "speed"))
}

func TestAnyStringSlice_DropsNilAndNonScalarItems(t *testing.T) {
	require.Nil(t, anyStringSlice(nil))
	assert.Equal(t, []string{"up", "42", "false"}, anyStringSlice([]any{
		nil,
		" up ",
		42,
		false,
		map[string]any{"not": "scalar"},
	}))
}

func TestBuildSNMPTopologyV1Actors_UsesStableFallbackActorID(t *testing.T) {
	actors := []topologyActor{
		{ActorType: "device", Match: topologyMatch{IPAddresses: []string{"10.0.0.2"}}},
		{ActorType: "device", Match: topologyMatch{IPAddresses: []string{"10.0.0.1"}}},
	}
	reorderedActors := []topologyActor{actors[1], actors[0]}

	_, actorIndex := buildSNMPTopologyV1Actors(actors, topologyv1.NewStringDictionary(""))
	_, reorderedActorIndex := buildSNMPTopologyV1Actors(reorderedActors, topologyv1.NewStringDictionary(""))

	require.Contains(t, actorIndex, "generated:device:ip:x_10.0.0.1")
	require.Contains(t, actorIndex, "generated:device:ip:x_10.0.0.2")
	require.Contains(t, reorderedActorIndex, "generated:device:ip:x_10.0.0.1")
	require.Contains(t, reorderedActorIndex, "generated:device:ip:x_10.0.0.2")
}

func TestBuildSNMPTopologyV1PortNeighborSummaries_NormalizesRemotePortName(t *testing.T) {
	summaries := buildSNMPTopologyV1PortNeighborSummaries([]topologyLink{
		{
			SrcActorID: "device-a",
			DstActorID: "device-b",
			Src: topologyLinkEndpoint{Attributes: map[string]any{
				"if_index":  uint64(1),
				"port_name": "Gi0/1",
			}},
			Dst: topologyLinkEndpoint{Attributes: map[string]any{
				"port_name": " Gi0/2 ",
			}},
		},
		{
			SrcActorID: "device-a",
			DstActorID: "device-b",
			Src: topologyLinkEndpoint{Attributes: map[string]any{
				"if_index":  uint64(1),
				"port_name": "Gi0/1",
			}},
			Dst: topologyLinkEndpoint{Attributes: map[string]any{
				"port_name": "gi0/2",
			}},
		},
	}, map[string]int{"device-a": 0, "device-b": 1})

	summary, ok := summaries[snmpTopologyV1PortNeighborKey{actorRef: 0, ifIndex: 1}]
	require.True(t, ok)
	assert.False(t, summary.ambiguous)
}

func TestBuildSNMPTopologyV1PortNeighborSummaries_FillsMissingRemotePortName(t *testing.T) {
	summaries := buildSNMPTopologyV1PortNeighborSummaries([]topologyLink{
		{
			SrcActorID: "device-a",
			DstActorID: "device-b",
			Src: topologyLinkEndpoint{Attributes: map[string]any{
				"if_index":  uint64(1),
				"port_name": "Gi0/1",
			}},
		},
		{
			SrcActorID: "device-a",
			DstActorID: "device-b",
			Src: topologyLinkEndpoint{Attributes: map[string]any{
				"if_index":  uint64(1),
				"port_name": "Gi0/1",
			}},
			Dst: topologyLinkEndpoint{Attributes: map[string]any{
				"port_name": "Gi0/2",
			}},
		},
	}, map[string]int{"device-a": 0, "device-b": 1})

	summary, ok := summaries[snmpTopologyV1PortNeighborKey{actorRef: 0, ifIndex: 1}]
	require.True(t, ok)
	assert.False(t, summary.ambiguous)
	assert.Equal(t, "Gi0/2", summary.remotePortName)
}

func topologyV1TestColumnValues(t *testing.T, table topologyv1.Table, columnID string) []any {
	t.Helper()

	for i, column := range table.Columns {
		if column.ID != columnID {
			continue
		}
		values, ok := table.Values[i].(topologyv1.ValuesEncoding)
		require.True(t, ok)
		return values.Values
	}
	require.Failf(t, "missing column", "column %q not found", columnID)
	return nil
}
