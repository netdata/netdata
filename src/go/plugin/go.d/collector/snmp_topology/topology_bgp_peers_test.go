// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"errors"
	"testing"

	topologyv1 "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
	"github.com/stretchr/testify/require"
)

func TestTopologyBGPPeerFromRowNormalizesState(t *testing.T) {
	tests := map[string]struct {
		state ddsnmp.BGPState
		want  string
	}{
		"typed-numeric": {
			state: ddsnmp.BGPState{Has: true, State: ddprofiledefinition.BGPPeerState("6")},
			want:  "established",
		},
		"typed-case": {
			state: ddsnmp.BGPState{Has: true, State: ddprofiledefinition.BGPPeerState("Established")},
			want:  "established",
		},
		"typed-open-sent-variant": {
			state: ddsnmp.BGPState{Has: true, State: ddprofiledefinition.BGPPeerState("open_sent")},
			want:  "opensent",
		},
		"raw-numeric": {
			state: ddsnmp.BGPState{Raw: "6"},
			want:  "established",
		},
		"unknown": {
			state: ddsnmp.BGPState{Has: true, State: ddprofiledefinition.BGPPeerState("vendor-specific")},
			want:  "vendor-specific",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			row := bgpRowForTest("peer-1", "192.0.2.2", "65002")
			row.State = tc.state

			peer, ok := topologyBGPPeerFromRow(row)

			require.True(t, ok)
			require.Equal(t, tc.want, peer.State)
		})
	}
}

func TestTopologyBGPPeerFromRowKeepsOnlyDiagnosticRawAddresses(t *testing.T) {
	tests := map[string]struct {
		neighbor  string
		localAddr string
		wantPeer  topologyBGPPeer
	}{
		"normalizes-non-unspecified-ip-addresses": {
			neighbor:  "C0000202",
			localAddr: "192.0.2.1",
			wantPeer: topologyBGPPeer{
				NeighborIP: "192.0.2.2",
				LocalIP:    "192.0.2.1",
			},
		},
		"drops-unspecified-ip-addresses": {
			neighbor:  "0.0.0.0",
			localAddr: "::",
			wantPeer:  topologyBGPPeer{},
		},
		"preserves-raw-non-ip-diagnostics": {
			neighbor:  "peer-token",
			localAddr: "local-token",
			wantPeer: topologyBGPPeer{
				NeighborIP: "peer-token",
				LocalIP:    "local-token",
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			row := bgpRowForTest("peer-1", tc.neighbor, "65002")
			row.Descriptors.LocalAddress = tc.localAddr

			peer, ok := topologyBGPPeerFromRow(row)

			require.True(t, ok)
			require.Equal(t, tc.wantPeer.NeighborIP, peer.NeighborIP)
			require.Equal(t, tc.wantPeer.LocalIP, peer.LocalIP)
		})
	}
}

func TestSortTopologyBGPPeerRowsUsesRawNeighborFallback(t *testing.T) {
	rows := []topologyBGPPeerDetailRow{
		{
			RoutingInstance: "default",
			RemoteAS:        "65002",
			NeighborIP:      "raw-b",
			State:           "established",
		},
		{
			RoutingInstance: "default",
			RemoteAS:        "65002",
			NeighborIP:      "raw-a",
			State:           "established",
		},
	}

	sortTopologyBGPPeerDetailRows(rows)

	require.Equal(t, "raw-a", rows[0].NeighborIP)
	require.Equal(t, "raw-b", rows[1].NeighborIP)
}

func TestBuildSNMPTopologyV1BGPPeersTableHandlesRawAndUnspecifiedAddresses(t *testing.T) {
	tests := map[string]struct {
		values          map[string]any
		wantNeighborIPs []string
		wantLocalIPs    []string
		wantRawValues   map[string][]any
	}{
		"preserves-raw-non-ip-diagnostics": {
			values: map[string]any{
				"neighbor_ip": "peer-token",
				"local_ip":    "local-token",
			},
			wantNeighborIPs: []string{"peer-token"},
			wantLocalIPs:    []string{"local-token"},
		},
		"drops-unspecified-ip-addresses": {
			values: map[string]any{
				"neighbor_ip": "0.0.0.0",
				"local_ip":    "::",
			},
			wantRawValues: map[string][]any{
				"neighbor_ip": {nil},
				"local_ip":    {nil},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			stringsDict := topologyv1.NewStringDictionary()

			table := buildSNMPTopologyV1BGPPeersTable([]topologyV1DynamicRow{
				{
					actorRef: 0,
					values:   tc.values,
				},
			}, nil, stringsDict)
			data := topologyv1.Data{
				Dictionaries: topologyv1.Dictionaries{
					"strings": stringsDict.Values(),
				},
			}

			if tc.wantNeighborIPs != nil {
				require.Equal(t, tc.wantNeighborIPs, topologyV1StringColumnValues(t, data, table, "neighbor_ip"))
			}
			if tc.wantLocalIPs != nil {
				require.Equal(t, tc.wantLocalIPs, topologyV1StringColumnValues(t, data, table, "local_ip"))
			}
			for column, values := range tc.wantRawValues {
				require.Equal(t, values, topologyV1ColumnValues(t, table, column))
			}
		})
	}
}

func TestTopologyCacheIngestTopologyBGPPeersSkipsErrorsAndInvalidRows(t *testing.T) {
	cache := newTopologyCache()

	cache.ingestTopologyBGPPeers([]*ddsnmp.ProfileMetrics{
		nil,
		{
			BGPCollectError: errors.New("collect failed"),
			BGPRows: []ddsnmp.BGPRow{
				bgpRowForTest("collect-error", "192.0.2.2", "65002"),
			},
		},
		{
			BGPRows: []ddsnmp.BGPRow{
				bgpRowForTest("valid", "192.0.2.3", "65003"),
				bgpRowForTest("missing-neighbor", "", "65004"),
				bgpRowForTest("missing-remote-as", "192.0.2.5", ""),
				{
					Kind: ddprofiledefinition.BGPRowKindPeerFamily,
					Identity: ddsnmp.BGPIdentity{
						Neighbor: "192.0.2.6",
						RemoteAS: "65006",
					},
				},
			},
		},
	})

	require.Len(t, cache.bgpPeersByKey, 1)
	require.Contains(t, cache.bgpPeersByKey, "valid")
	require.Equal(t, "192.0.2.3", cache.bgpPeersByKey["valid"].NeighborIP)
}

func bgpRowForTest(structuralID, neighbor, remoteAS string) ddsnmp.BGPRow {
	return ddsnmp.BGPRow{
		Kind:         ddprofiledefinition.BGPRowKindPeer,
		StructuralID: structuralID,
		Identity: ddsnmp.BGPIdentity{
			RoutingInstance: "default",
			Neighbor:        neighbor,
			RemoteAS:        remoteAS,
		},
		Descriptors: ddsnmp.BGPDescriptors{
			LocalAddress: "192.0.2.1",
			LocalAS:      "65001",
		},
		State: ddsnmp.BGPState{
			Has:   true,
			State: ddprofiledefinition.BGPPeerStateEstablished,
		},
	}
}
