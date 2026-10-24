// SPDX-License-Identifier: GPL-3.0-or-later

package topologyv1

import (
	"testing"

	topologyapi "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/stretchr/testify/require"
)

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
			stringsDict := topologyapi.NewStringDictionary()

			table := buildSNMPTopologyV1BGPPeersTable([]topologyV1DynamicRow{
				{
					actorRef: 0,
					values:   tc.values,
				},
			}, nil, stringsDict)
			data := topologyapi.Data{
				Dictionaries: topologyapi.Dictionaries{
					"strings": stringsDict.Values(),
				},
			}

			if tc.wantNeighborIPs != nil {
				require.Equal(t, tc.wantNeighborIPs, topologyV1StringColumnValuesForTest(t, data, table, "neighbor_ip"))
			}
			if tc.wantLocalIPs != nil {
				require.Equal(t, tc.wantLocalIPs, topologyV1StringColumnValuesForTest(t, data, table, "local_ip"))
			}
			for column, values := range tc.wantRawValues {
				require.Equal(t, values, topologyV1TestColumnValues(t, table, column))
			}
		})
	}
}

func TestBuildSNMPTopologyV1BGPPeersTablePreservesOptionalUptimePresence(t *testing.T) {
	var zero int64
	stringsDict := topologyapi.NewStringDictionary()

	table := buildSNMPTopologyV1BGPPeersTable([]topologyV1DynamicRow{
		{
			actorRef: 0,
			values: snmpTopologyV1BGPPeerValues(topologymodel.BGPPeerDetailRow{
				NeighborIP:      "192.0.2.2",
				RoutingInstance: "default",
			}),
		},
		{
			actorRef: 0,
			values: snmpTopologyV1BGPPeerValues(topologymodel.BGPPeerDetailRow{
				NeighborIP:            "192.0.2.3",
				RoutingInstance:       "default",
				EstablishedUptime:     &zero,
				LastReceivedUpdateAge: &zero,
			}),
		},
	}, nil, stringsDict)

	require.Equal(t, []any{nil, uint64(0)}, topologyV1TestColumnValues(t, table, "established_uptime"))
	require.Equal(t, []any{nil, uint64(0)}, topologyV1TestColumnValues(t, table, "last_received_update_age"))
}

func topologyV1StringColumnValuesForTest(t *testing.T, data topologyapi.Data, table topologyapi.Table, columnID string) []string {
	t.Helper()

	for columnIndex, column := range table.Columns {
		if column.ID != columnID {
			continue
		}
		require.Equal(t, "string_ref", column.Type)
		require.NotEmpty(t, column.Dictionary)
		dict := data.Dictionaries[column.Dictionary]
		require.NotNil(t, dict)

		values := topologyV1TestColumnValues(t, table, table.Columns[columnIndex].ID)
		out := make([]string, 0, len(values))
		for _, value := range values {
			ref, ok := value.(int)
			require.Truef(t, ok, "expected integer dictionary reference for %q, got %T", columnID, value)
			require.GreaterOrEqual(t, ref, 0)
			require.Less(t, ref, len(dict))
			text, ok := dict[ref].(string)
			require.Truef(t, ok, "expected string dictionary value for %q, got %T", columnID, dict[ref])
			out = append(out, text)
		}
		return out
	}

	require.Failf(t, "missing column", "column %q not found", columnID)
	return nil
}
