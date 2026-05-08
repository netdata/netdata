// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

func TestCollector_Collect_HuaweiTypedBGPRows(t *testing.T) {
	tests := map[string]struct {
		rowID    string
		pdus     []gosnmp.SnmpPDU
		validate func(t *testing.T, rows []ddsnmp.BGPRow)
	}{
		"peer family maps state admin and routing instance": {
			rowID: "huawei-bgp-peer-family",
			pdus: []gosnmp.SnmpPDU{
				createStringPDU("1.3.6.1.4.1.2011.5.25.177.1.1.1.1.6.0.1.1.1.4.192.0.2.21", "blue"),
				createGauge32PDU("1.3.6.1.4.1.2011.5.25.177.1.1.2.1.2.0.1.1.1.4.192.0.2.21", 65001),
				createPDU("1.3.6.1.4.1.2011.5.25.177.1.1.2.1.4.0.1.1.1.4.192.0.2.21", gosnmp.OctetString, []byte{192, 0, 2, 21}),
				createIntegerPDU("1.3.6.1.4.1.2011.5.25.177.1.1.2.1.5.0.1.1.1.4.192.0.2.21", 3),
				createIntegerPDU("1.3.6.1.4.1.2011.5.25.177.1.1.2.1.11.0.1.1.1.4.192.0.2.21", 1),
				createCounter32PDU("1.3.6.1.4.1.2011.5.25.177.1.1.2.1.6.0.1.1.1.4.192.0.2.21", 4),
			},
			validate: func(t *testing.T, rows []ddsnmp.BGPRow) {
				require.Len(t, rows, 1)
				row := rows[0]
				assert.Equal(t, ddprofiledefinition.BGPRowKindPeerFamily, row.Kind)
				assert.Equal(t, "blue", row.Identity.RoutingInstance)
				assert.Equal(t, "192.0.2.21", row.Identity.Neighbor)
				assert.Equal(t, "65001", row.Identity.RemoteAS)
				assert.Equal(t, ddprofiledefinition.BGPAddressFamilyIPv4, row.Identity.AddressFamily)
				assert.Equal(t, ddprofiledefinition.BGPSubsequentAddressFamilyUnicast, row.Identity.SubsequentAddressFamily)
				require.True(t, row.Admin.Enabled.Has)
				assert.False(t, row.Admin.Enabled.Value)
				require.True(t, row.State.Has)
				assert.Equal(t, ddprofiledefinition.BGPPeerStateActive, row.State.State)
				assert.EqualValues(t, 4, row.Transitions.Established.Value)
			},
		},
		"peer statistics resolves non-default VRF index and remote AS by peer address": {
			rowID: "huawei-bgp-peer-statistics",
			pdus: []gosnmp.SnmpPDU{
				createPDU("1.3.6.1.4.1.2011.5.25.177.1.1.2.1.4.42.1.1.1.4.192.0.2.22", gosnmp.OctetString, []byte{192, 0, 2, 22}),
				createGauge32PDU("1.3.6.1.4.1.2011.5.25.177.1.1.2.1.2.42.1.1.1.4.192.0.2.22", 65002),
				createCounter32PDU("1.3.6.1.4.1.2011.5.25.177.1.1.7.1.4.0.42.4.192.0.2.22", 5),
				createCounter32PDU("1.3.6.1.4.1.2011.5.25.177.1.1.7.1.5.0.42.4.192.0.2.22", 2),
				createCounter32PDU("1.3.6.1.4.1.2011.5.25.177.1.1.7.1.6.0.42.4.192.0.2.22", 101),
				createCounter32PDU("1.3.6.1.4.1.2011.5.25.177.1.1.7.1.7.0.42.4.192.0.2.22", 202),
			},
			validate: func(t *testing.T, rows []ddsnmp.BGPRow) {
				require.Len(t, rows, 1)
				row := rows[0]
				assert.Equal(t, ddprofiledefinition.BGPRowKindPeer, row.Kind)
				assert.Equal(t, "42", row.Identity.RoutingInstance)
				assert.Equal(t, "192.0.2.22", row.Identity.Neighbor)
				assert.Equal(t, "65002", row.Identity.RemoteAS)
				assert.Equal(t, "ipv4", row.Descriptors.PeerType)
				assert.EqualValues(t, 5, row.Transitions.Established.Value)
				assert.EqualValues(t, 2, row.Transitions.Down.Value)
				assert.EqualValues(t, 101, row.Traffic.Updates.Received.Value)
				assert.EqualValues(t, 202, row.Traffic.Updates.Sent.Value)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			profile := matchedBGPProjectionByFile(t, "1.3.6.1.4.1.2011.2.224.279", "huawei-routers.yaml")
			filterProfileForTypedBGPByID(t, profile, tc.rowID)

			ctrl, mockHandler := setupMockHandler(t)
			defer ctrl.Finish()

			expectBGPTableWalksFromFixture(mockHandler, profile, tc.pdus)

			collector := New(Config{
				SnmpClient:  mockHandler,
				Profiles:    []*ddsnmp.Profile{profile},
				Log:         logger.New(),
				SysObjectID: "1.3.6.1.4.1.2011.2.224.279",
			})

			results, err := collector.Collect()
			require.NoError(t, err)
			require.Len(t, results, 1)
			require.Empty(t, results[0].Metrics)
			require.Len(t, results[0].BGPRows, 1)

			tc.validate(t, results[0].BGPRows)
		})
	}
}
