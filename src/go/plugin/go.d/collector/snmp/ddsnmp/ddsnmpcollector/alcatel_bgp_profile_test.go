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

func TestCollector_Collect_AlcatelBGP_TypedRows(t *testing.T) {
	sysObjectID := "1.3.6.1.4.1.6486.801.1.1.2.1.1"
	profile := matchedBGPProjectionByFile(t, sysObjectID, "alcatel-lucent-ent.yaml")

	ipv4Idx := "192.0.2.10"
	ipv6Idx := "32.1.13.184.0.0.0.0.0.0.0.0.0.0.0.1"
	ipv6Neighbor := []byte{0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}
	ipv6Local := []byte{0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2}
	pdus := []gosnmp.SnmpPDU{
		createPDU(oidWithIndex("1.3.6.1.2.1.15.3.1.1", ipv4Idx), gosnmp.IPAddress, "198.51.100.1"),
		createIntegerPDU(oidWithIndex("1.3.6.1.2.1.15.3.1.2", ipv4Idx), 6),
		createIntegerPDU(oidWithIndex("1.3.6.1.2.1.15.3.1.3", ipv4Idx), 2),
		createIntegerPDU(oidWithIndex("1.3.6.1.2.1.15.3.1.4", ipv4Idx), 4),
		createPDU(oidWithIndex("1.3.6.1.2.1.15.3.1.5", ipv4Idx), gosnmp.IPAddress, "192.0.2.1"),
		createPDU(oidWithIndex("1.3.6.1.2.1.15.3.1.7", ipv4Idx), gosnmp.IPAddress, "192.0.2.10"),
		createGauge32PDU(oidWithIndex("1.3.6.1.2.1.15.3.1.9", ipv4Idx), 65001),
		createCounter32PDU(oidWithIndex("1.3.6.1.2.1.15.3.1.10", ipv4Idx), 12),
		createCounter32PDU(oidWithIndex("1.3.6.1.2.1.15.3.1.11", ipv4Idx), 13),
		createCounter32PDU(oidWithIndex("1.3.6.1.2.1.15.3.1.12", ipv4Idx), 100),
		createCounter32PDU(oidWithIndex("1.3.6.1.2.1.15.3.1.13", ipv4Idx), 110),
		createPDU(oidWithIndex("1.3.6.1.2.1.15.3.1.14", ipv4Idx), gosnmp.OctetString, []byte{0x02, 0x03}),
		createCounter32PDU(oidWithIndex("1.3.6.1.2.1.15.3.1.15", ipv4Idx), 5),
		createGauge32PDU(oidWithIndex("1.3.6.1.2.1.15.3.1.16", ipv4Idx), 3600),
		createIntegerPDU(oidWithIndex("1.3.6.1.2.1.15.3.1.17", ipv4Idx), 30),
		createIntegerPDU(oidWithIndex("1.3.6.1.2.1.15.3.1.18", ipv4Idx), 90),
		createIntegerPDU(oidWithIndex("1.3.6.1.2.1.15.3.1.19", ipv4Idx), 30),
		createIntegerPDU(oidWithIndex("1.3.6.1.2.1.15.3.1.20", ipv4Idx), 180),
		createIntegerPDU(oidWithIndex("1.3.6.1.2.1.15.3.1.21", ipv4Idx), 60),
		createIntegerPDU(oidWithIndex("1.3.6.1.2.1.15.3.1.22", ipv4Idx), 15),
		createIntegerPDU(oidWithIndex("1.3.6.1.2.1.15.3.1.23", ipv4Idx), 30),
		createGauge32PDU(oidWithIndex("1.3.6.1.2.1.15.3.1.24", ipv4Idx), 7),

		createPDU(oidWithIndex("1.3.6.1.4.1.6486.801.1.2.1.10.5.1.1.2.1.1", ipv4Idx), gosnmp.IPAddress, "192.0.2.10"),
		createGauge32PDU(oidWithIndex("1.3.6.1.4.1.6486.801.1.2.1.10.5.1.1.2.1.2", ipv4Idx), 65001),
		createStringPDU(oidWithIndex("1.3.6.1.4.1.6486.801.1.2.1.10.5.1.1.2.1.4", ipv4Idx), "alcatel-ipv4"),
		createCounter32PDU(oidWithIndex("1.3.6.1.4.1.6486.801.1.2.1.10.5.1.1.2.1.13", ipv4Idx), 3),
		createCounter32PDU(oidWithIndex("1.3.6.1.4.1.6486.801.1.2.1.10.5.1.1.2.1.14", ipv4Idx), 4),
		createPDU(oidWithIndex("1.3.6.1.4.1.6486.801.1.2.1.10.5.1.1.2.1.17", ipv4Idx), gosnmp.IPAddress, "192.0.2.1"),
		createIntegerPDU(oidWithIndex("1.3.6.1.4.1.6486.801.1.2.1.10.5.1.1.2.1.18", ipv4Idx), 7),
		createCounter32PDU(oidWithIndex("1.3.6.1.4.1.6486.801.1.2.1.10.5.1.1.2.1.21", ipv4Idx), 1),
		createCounter32PDU(oidWithIndex("1.3.6.1.4.1.6486.801.1.2.1.10.5.1.1.2.1.22", ipv4Idx), 2),
		createIntegerPDU(oidWithIndex("1.3.6.1.4.1.6486.801.1.2.1.10.5.1.1.2.1.23", ipv4Idx), 25),
		createIntegerPDU(oidWithIndex("1.3.6.1.4.1.6486.801.1.2.1.10.5.1.1.2.1.24", ipv4Idx), 22),
		createGauge32PDU(oidWithIndex("1.3.6.1.4.1.6486.801.1.2.1.10.5.1.1.2.1.25", ipv4Idx), 42),
		createCounter32PDU(oidWithIndex("1.3.6.1.4.1.6486.801.1.2.1.10.5.1.1.2.1.26", ipv4Idx), 2),
		createIntegerPDU(oidWithIndex("1.3.6.1.4.1.6486.801.1.2.1.10.5.1.1.2.1.27", ipv4Idx), 2),
		createCounter32PDU(oidWithIndex("1.3.6.1.4.1.6486.801.1.2.1.10.5.1.1.2.1.47", ipv4Idx), 6),

		createPDU(oidWithIndex("1.3.6.1.4.1.6486.801.1.2.1.10.5.1.1.14.1.1", ipv6Idx), gosnmp.OctetString, ipv6Neighbor),
		createGauge32PDU(oidWithIndex("1.3.6.1.4.1.6486.801.1.2.1.10.5.1.1.14.1.2", ipv6Idx), 65002),
		createStringPDU(oidWithIndex("1.3.6.1.4.1.6486.801.1.2.1.10.5.1.1.14.1.4", ipv6Idx), "alcatel-ipv6"),
		createCounter32PDU(oidWithIndex("1.3.6.1.4.1.6486.801.1.2.1.10.5.1.1.14.1.13", ipv6Idx), 5),
		createCounter32PDU(oidWithIndex("1.3.6.1.4.1.6486.801.1.2.1.10.5.1.1.14.1.14", ipv6Idx), 6),
		createPDU(oidWithIndex("1.3.6.1.4.1.6486.801.1.2.1.10.5.1.1.14.1.17", ipv6Idx), gosnmp.OctetString, ipv6Local),
		createIntegerPDU(oidWithIndex("1.3.6.1.4.1.6486.801.1.2.1.10.5.1.1.14.1.18", ipv6Idx), 9),
		createCounter32PDU(oidWithIndex("1.3.6.1.4.1.6486.801.1.2.1.10.5.1.1.14.1.21", ipv6Idx), 7),
		createCounter32PDU(oidWithIndex("1.3.6.1.4.1.6486.801.1.2.1.10.5.1.1.14.1.22", ipv6Idx), 8),
		createIntegerPDU(oidWithIndex("1.3.6.1.4.1.6486.801.1.2.1.10.5.1.1.14.1.23", ipv6Idx), 32),
		createIntegerPDU(oidWithIndex("1.3.6.1.4.1.6486.801.1.2.1.10.5.1.1.14.1.24", ipv6Idx), 23),
		createGauge32PDU(oidWithIndex("1.3.6.1.4.1.6486.801.1.2.1.10.5.1.1.14.1.25", ipv6Idx), 84),
		createCounter32PDU(oidWithIndex("1.3.6.1.4.1.6486.801.1.2.1.10.5.1.1.14.1.26", ipv6Idx), 1),
		createIntegerPDU(oidWithIndex("1.3.6.1.4.1.6486.801.1.2.1.10.5.1.1.14.1.27", ipv6Idx), 1),
		createCounter32PDU(oidWithIndex("1.3.6.1.4.1.6486.801.1.2.1.10.5.1.1.14.1.47", ipv6Idx), 8),
		createCounter32PDU(oidWithIndex("1.3.6.1.4.1.6486.801.1.2.1.10.5.1.1.14.1.49", ipv6Idx), 200),
		createCounter32PDU(oidWithIndex("1.3.6.1.4.1.6486.801.1.2.1.10.5.1.1.14.1.50", ipv6Idx), 210),
		createCounter32PDU(oidWithIndex("1.3.6.1.4.1.6486.801.1.2.1.10.5.1.1.14.1.51", ipv6Idx), 22),
		createCounter32PDU(oidWithIndex("1.3.6.1.4.1.6486.801.1.2.1.10.5.1.1.14.1.52", ipv6Idx), 23),
		createIntegerPDU(oidWithIndex("1.3.6.1.4.1.6486.801.1.2.1.10.5.1.1.14.1.61", ipv6Idx), 90),
		createIntegerPDU(oidWithIndex("1.3.6.1.4.1.6486.801.1.2.1.10.5.1.1.14.1.62", ipv6Idx), 30),
		createIntegerPDU(oidWithIndex("1.3.6.1.4.1.6486.801.1.2.1.10.5.1.1.14.1.63", ipv6Idx), 30),
		createIntegerPDU(oidWithIndex("1.3.6.1.4.1.6486.801.1.2.1.10.5.1.1.14.1.64", ipv6Idx), 180),
		createIntegerPDU(oidWithIndex("1.3.6.1.4.1.6486.801.1.2.1.10.5.1.1.14.1.65", ipv6Idx), 60),
		createIntegerPDU(oidWithIndex("1.3.6.1.4.1.6486.801.1.2.1.10.5.1.1.14.1.68", ipv6Idx), 2),
		createIntegerPDU(oidWithIndex("1.3.6.1.4.1.6486.801.1.2.1.10.5.1.1.14.1.69", ipv6Idx), 6),
		createIntegerPDU(oidWithIndex("1.3.6.1.4.1.6486.801.1.2.1.10.5.1.1.14.1.73", ipv6Idx), 30),
	}

	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	expectSystemMetadataGets(mockHandler, sysObjectID, "alcatel")
	expectBGPTableWalksFromFixture(mockHandler, profile, pdus)

	collector := New(Config{
		SnmpClient:  mockHandler,
		Profiles:    []*ddsnmp.Profile{profile},
		Log:         logger.New(),
		SysObjectID: sysObjectID,
	})

	results, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Empty(t, results[0].Metrics)
	require.Empty(t, results[0].TopologyMetrics)
	require.Empty(t, results[0].LicenseRows)
	require.Len(t, results[0].BGPRows, 4)

	tests := map[string]struct {
		match    func(ddsnmp.BGPRow) bool
		validate func(t *testing.T, row ddsnmp.BGPRow)
	}{
		"IPv4 peer row": {
			match: func(row ddsnmp.BGPRow) bool {
				return row.Kind == ddprofiledefinition.BGPRowKindPeer && row.Identity.Neighbor == "192.0.2.10"
			},
			validate: func(t *testing.T, row ddsnmp.BGPRow) {
				assert.Equal(t, "default", row.Identity.RoutingInstance)
				assert.Equal(t, "65001", row.Identity.RemoteAS)
				assert.Equal(t, "192.0.2.1", row.Descriptors.LocalAddress)
				assert.Equal(t, "alcatel-ipv4", row.Descriptors.Description)
				assert.Equal(t, "external", row.Descriptors.PeerType)
				require.True(t, row.Admin.Enabled.Has)
				assert.True(t, row.Admin.Enabled.Value)
				require.True(t, row.State.Has)
				assert.Equal(t, ddprofiledefinition.BGPPeerStateEstablished, row.State.State)
				assert.EqualValues(t, 12, row.Traffic.Updates.Received.Value)
				assert.EqualValues(t, 13, row.Traffic.Updates.Sent.Value)
				assert.EqualValues(t, 3, row.Traffic.RouteRefreshes.Received.Value)
				assert.EqualValues(t, 4, row.Traffic.RouteRefreshes.Sent.Value)
				assert.EqualValues(t, 2, row.LastError.Code.Value)
				assert.EqualValues(t, 3, row.LastError.Subcode.Value)
				assert.Equal(t, "peer_notify", row.Reasons.LastDown.Value)
				assert.Equal(t, "hold_timeout", row.LastNotify.Received.Reason.Value)
				assert.Equal(t, "cease_admin_shutdown", row.LastNotify.Sent.Reason.Value)
			},
		},
		"IPv4 peer-family row": {
			match: func(row ddsnmp.BGPRow) bool {
				return row.Kind == ddprofiledefinition.BGPRowKindPeerFamily && row.Identity.Neighbor == "192.0.2.10"
			},
			validate: func(t *testing.T, row ddsnmp.BGPRow) {
				assert.Equal(t, ddprofiledefinition.BGPAddressFamilyIPv4, row.Identity.AddressFamily)
				assert.Equal(t, ddprofiledefinition.BGPSubsequentAddressFamilyUnicast, row.Identity.SubsequentAddressFamily)
				assert.EqualValues(t, 42, row.Routes.Current.Received.Value)
			},
		},
		"IPv6 peer row": {
			match: func(row ddsnmp.BGPRow) bool {
				return row.Kind == ddprofiledefinition.BGPRowKindPeer && row.Identity.Neighbor == "2001:db8::1"
			},
			validate: func(t *testing.T, row ddsnmp.BGPRow) {
				assert.Equal(t, "65002", row.Identity.RemoteAS)
				assert.Equal(t, "2001:db8::2", row.Descriptors.LocalAddress)
				assert.Equal(t, "internal", row.Descriptors.PeerType)
				require.True(t, row.Admin.Enabled.Has)
				assert.True(t, row.Admin.Enabled.Value)
				require.True(t, row.State.Has)
				assert.Equal(t, ddprofiledefinition.BGPPeerStateEstablished, row.State.State)
				assert.EqualValues(t, 22, row.Traffic.Updates.Received.Value)
				assert.EqualValues(t, 23, row.Traffic.Updates.Sent.Value)
				assert.EqualValues(t, 5, row.Traffic.RouteRefreshes.Received.Value)
				assert.EqualValues(t, 6, row.Traffic.RouteRefreshes.Sent.Value)
				assert.Equal(t, "none", row.Reasons.LastDown.Value)
				assert.Equal(t, "fsm_error", row.LastNotify.Received.Reason.Value)
				assert.Equal(t, "none", row.LastNotify.Sent.Reason.Value)
			},
		},
		"IPv6 peer-family row": {
			match: func(row ddsnmp.BGPRow) bool {
				return row.Kind == ddprofiledefinition.BGPRowKindPeerFamily && row.Identity.Neighbor == "2001:db8::1"
			},
			validate: func(t *testing.T, row ddsnmp.BGPRow) {
				assert.Equal(t, ddprofiledefinition.BGPAddressFamilyIPv6, row.Identity.AddressFamily)
				assert.Equal(t, ddprofiledefinition.BGPSubsequentAddressFamilyUnicast, row.Identity.SubsequentAddressFamily)
				assert.EqualValues(t, 84, row.Routes.Current.Received.Value)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			row := requireBGPRowMatching(t, results[0].BGPRows, tc.match)
			tc.validate(t, row)
		})
	}
}
