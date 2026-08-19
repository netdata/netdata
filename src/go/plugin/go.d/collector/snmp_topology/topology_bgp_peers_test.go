// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"errors"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"

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
		wantPeer  topologymodel.BGPPeer
	}{
		"normalizes-non-unspecified-ip-addresses": {
			neighbor:  "C0000202",
			localAddr: "192.0.2.1",
			wantPeer: topologymodel.BGPPeer{
				NeighborIP: "192.0.2.2",
				LocalIP:    "192.0.2.1",
			},
		},
		"drops-unspecified-ip-addresses": {
			neighbor:  "0.0.0.0",
			localAddr: "::",
			wantPeer:  topologymodel.BGPPeer{},
		},
		"preserves-raw-non-ip-diagnostics": {
			neighbor:  "peer-token",
			localAddr: "local-token",
			wantPeer: topologymodel.BGPPeer{
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
