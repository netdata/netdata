// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"testing"

	topologyv1 "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
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

func TestBuildSNMPTopologyV1Actors_FallbackDoesNotCollideWithExplicitActorID(t *testing.T) {
	_, actorIndex := buildSNMPTopologyV1Actors([]topologyActor{
		{
			ActorType: "device",
			Match:     topologyMatch{IPAddresses: []string{"10.0.0.1"}},
		},
		{
			ActorID:   "generated:device:ip:x_10.0.0.1",
			ActorType: "device",
		},
	}, topologyv1.NewStringDictionary(""))

	assert.Equal(t, map[string]int{
		"generated:device:ip:x_10.0.0.1_2": 0,
		"generated:device:ip:x_10.0.0.1":   1,
	}, actorIndex)
}

func TestBuildSNMPTopologyV1PortNeighborSummaries_RemotePortName(t *testing.T) {
	tests := map[string]struct {
		links              []topologyLink
		wantRemotePortName string
	}{
		"normalizes-before-ambiguity-check": {
			links: []topologyLink{
				topologyV1PortNeighborSummaryLinkForTest(" Gi0/2 "),
				topologyV1PortNeighborSummaryLinkForTest("gi0/2"),
			},
			wantRemotePortName: "Gi0/2",
		},
		"fills-missing-remote-port-name": {
			links: []topologyLink{
				topologyV1PortNeighborSummaryLinkForTest(""),
				topologyV1PortNeighborSummaryLinkForTest("Gi0/2"),
			},
			wantRemotePortName: "Gi0/2",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			summaries := buildSNMPTopologyV1PortNeighborSummaries(tc.links, map[string]int{"device-a": 0, "device-b": 1})

			summary, ok := summaries[snmpTopologyV1PortNeighborKey{actorRef: 0, ifIndex: 1}]
			require.True(t, ok)
			assert.False(t, summary.ambiguous)
			assert.Equal(t, tc.wantRemotePortName, summary.remotePortName)
		})
	}
}

func topologyV1PortNeighborSummaryLinkForTest(remotePortName string) topologyLink {
	link := topologyLink{
		SrcActorID: "device-a",
		DstActorID: "device-b",
		Src:        topologyLinkEndpoint{IfIndex: 1, PortName: "Gi0/1"},
	}
	if remotePortName != "" {
		link.Dst = topologyLinkEndpoint{PortName: remotePortName}
	}
	return link
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
