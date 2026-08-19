// SPDX-License-Identifier: GPL-3.0-or-later

package topologyv1

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
	topologyapi "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnyStringSlice_DropsNilAndNonScalarItems(t *testing.T) {
	tests := map[string]struct {
		in   any
		want []string
	}{
		"nil": {
			in: nil,
		},
		"mixed-values": {
			in: []any{
				nil,
				" up ",
				42,
				false,
				map[string]any{"not": "scalar"},
			},
			want: []string{"up", "42", "false"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, anyStringSlice(tc.in))
		})
	}
}

func TestBuildSNMPTopologyV1Actors_UsesStableFallbackActorID(t *testing.T) {
	actors := []topologymodel.Actor{
		{ActorType: "device", Match: topologymodel.Match{IPAddresses: []string{"10.0.0.2"}}},
		{ActorType: "device", Match: topologymodel.Match{IPAddresses: []string{"10.0.0.1"}}},
	}
	reorderedActors := []topologymodel.Actor{actors[1], actors[0]}
	handles := graph.NewActorHandleAllocator()
	actors[0].ActorHandle = handles.Next()
	actors[1].ActorHandle = handles.Next()
	reorderedActors = []topologymodel.Actor{actors[1], actors[0]}

	stringsDict := topologyapi.NewStringDictionary("")
	buildSNMPTopologyV1Actors(actors, stringsDict)
	reorderedStringsDict := topologyapi.NewStringDictionary("")
	buildSNMPTopologyV1Actors(reorderedActors, reorderedStringsDict)

	require.Contains(t, stringsDict.Values(), "generated:device:ip:x_10.0.0.1")
	require.Contains(t, stringsDict.Values(), "generated:device:ip:x_10.0.0.2")
	require.Contains(t, reorderedStringsDict.Values(), "generated:device:ip:x_10.0.0.1")
	require.Contains(t, reorderedStringsDict.Values(), "generated:device:ip:x_10.0.0.2")
}

func TestBuildSNMPTopologyV1Actors_FallbackDoesNotCollideWithExplicitActorID(t *testing.T) {
	handles := graph.NewActorHandleAllocator()
	stringsDict := topologyapi.NewStringDictionary("")
	buildSNMPTopologyV1Actors([]topologymodel.Actor{
		{
			ActorHandle: handles.Next(),
			ActorType:   "device",
			Match:       topologymodel.Match{IPAddresses: []string{"10.0.0.1"}},
		},
		{
			ActorHandle: handles.Next(),
			ActorID:     "generated:device:ip:x_10.0.0.1",
			ActorType:   "device",
		},
	}, stringsDict)

	require.Contains(t, stringsDict.Values(), "generated:device:ip:x_10.0.0.1_2")
	require.Contains(t, stringsDict.Values(), "generated:device:ip:x_10.0.0.1")
}

func TestBuildSNMPTopologyV1PortNeighborSummaries_RemotePortName(t *testing.T) {
	tests := map[string]struct {
		links              []topologymodel.Link
		wantRemotePortName string
	}{
		"normalizes-before-ambiguity-check": {
			links: []topologymodel.Link{
				topologyV1PortNeighborSummaryLinkForTest(" Gi0/2 "),
				topologyV1PortNeighborSummaryLinkForTest("gi0/2"),
			},
			wantRemotePortName: "Gi0/2",
		},
		"fills-missing-remote-port-name": {
			links: []topologymodel.Link{
				topologyV1PortNeighborSummaryLinkForTest(""),
				topologyV1PortNeighborSummaryLinkForTest("Gi0/2"),
			},
			wantRemotePortName: "Gi0/2",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			handles := graph.NewActorHandleAllocator()
			deviceA := handles.Next()
			deviceB := handles.Next()
			for i := range tc.links {
				tc.links[i].SrcActorHandle = deviceA
				tc.links[i].DstActorHandle = deviceB
			}
			summaries := buildSNMPTopologyV1PortNeighborSummaries(tc.links, topologyV1ActorIndex{deviceA: 0, deviceB: 1})

			summary, ok := summaries[snmpTopologyV1PortNeighborKey{actorRef: 0, ifIndex: 1}]
			require.True(t, ok)
			assert.False(t, summary.ambiguous)
			assert.Equal(t, tc.wantRemotePortName, summary.remotePortName)
		})
	}
}

func topologyV1PortNeighborSummaryLinkForTest(remotePortName string) topologymodel.Link {
	link := topologymodel.Link{
		SrcActorHandle: topologyV1TestActorHandle("device-a"),
		DstActorHandle: topologyV1TestActorHandle("device-b"),
		Src:            topologymodel.LinkEndpoint{IfIndex: 1, PortName: "Gi0/1"},
	}
	if remotePortName != "" {
		link.Dst = topologymodel.LinkEndpoint{PortName: remotePortName}
	}
	return link
}

func topologyV1TestColumnValues(t *testing.T, table topologyapi.Table, columnID string) []any {
	t.Helper()

	for i, column := range table.Columns {
		if column.ID != columnID {
			continue
		}
		values, ok := table.Values[i].(topologyapi.ValuesEncoding)
		require.True(t, ok)
		return values.Values
	}
	require.Failf(t, "missing column", "column %q not found", columnID)
	return nil
}
