// SPDX-License-Identifier: GPL-3.0-or-later

package topologydiag

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
	snmpdiag "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/diagnostics"
)

func TestProcessIdentitySurvivesAcquisitionArchiveReplayAndInspection(t *testing.T) {
	input := AcquisitionProfileValues{
		BGPRows: []AcquisitionBGPRowValue{
			{
				RouteOrdinal:    1,
				ProcessID:       "0",
				RoutingInstance: "7",
				Neighbor:        "192.0.2.1",
				Kind:            ddprofiledefinition.BGPRowKindPeer,
			},
		},
	}
	archived := newArchiveProfileValuesV1(input)
	encoded, err := json.Marshal(archived)
	require.NoError(t, err)
	var wire snmpdiag.ProfileValues
	require.NoError(t, json.Unmarshal(encoded, &wire))
	restored, err := restoreArchiveProfileValues(wire, map[uint32]struct{}{1: {}})
	require.NoError(t, err)
	require.Len(t, restored.BGPRows, 1)
	row := restored.BGPRows[0]
	assert.Equal(t, "0", row.ProcessID)
	assert.Empty(t, row.RemoteAS)
	semantic := semanticBGPRowFromAcquisition(row)
	assert.Equal(t, "0", semantic.Identity.ProcessID)
	assert.True(t, semantic.Identity.HasPeerIdentity())
	assert.Equal(t, "0", newDiagnosticBGPFact(&row).ProcessID)
}

func TestSemanticIdentifiersSurviveAcquisitionArchiveReplayAndInspection(t *testing.T) {
	for _, value := range []string{"edge", " edge ", "\tedge\r\n", "\r\nedge\t", "edge'a", "edge\x00a", "edge\\", "edge\r\na"} {
		t.Run(value, func(t *testing.T) {
			input := AcquisitionProfileValues{
				BGPRows: []AcquisitionBGPRowValue{{
					RouteOrdinal: 1, Kind: ddprofiledefinition.BGPRowKindPeer,
					ProcessID: value, RoutingInstance: value, Neighbor: "192.0.2.1",
				}},
			}
			encoded, err := json.Marshal(newArchiveProfileValuesV1(input))
			require.NoError(t, err)
			var wire snmpdiag.ProfileValues
			require.NoError(t, json.Unmarshal(encoded, &wire))
			restored, err := restoreArchiveProfileValues(wire, map[uint32]struct{}{1: {}})
			require.NoError(t, err)
			require.Len(t, restored.BGPRows, 1)
			semantic := semanticBGPRowFromAcquisition(restored.BGPRows[0])
			assert.Equal(t, value, semantic.Identity.ProcessID)
			assert.Equal(t, value, semantic.Identity.RoutingInstance)
			inspected := newDiagnosticBGPFact(&restored.BGPRows[0])
			assert.Equal(t, value, inspected.ProcessID)
			assert.Equal(t, value, inspected.RoutingInstance)
		})
	}
}
