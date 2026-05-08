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

func TestCollector_Collect_HuaweiBGP_FromLibreNMSFixture(t *testing.T) {
	profile := matchedBGPProjectionByFile(t, "1.3.6.1.4.1.2011.2.224.279", "huawei-routers.yaml")
	pdus := loadSnmprecPDUs(t, "librenms/huawei_vrp_ne8000_bgp_peer_table.snmprec")

	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	expectSNMPGet(mockHandler, []string{
		"1.3.6.1.2.1.1.1.0",
		"1.3.6.1.2.1.1.2.0",
		"1.3.6.1.2.1.1.5.0",
		"1.3.6.1.2.1.1.6.0",
	}, []gosnmp.SnmpPDU{
		createStringPDU("1.3.6.1.2.1.1.1.0", "Huawei VRP"),
		createPDU("1.3.6.1.2.1.1.2.0", gosnmp.ObjectIdentifier, "1.3.6.1.4.1.2011.2.224.279"),
		createStringPDU("1.3.6.1.2.1.1.5.0", "huawei"),
		createStringPDU("1.3.6.1.2.1.1.6.0", "lab"),
	})
	expectSNMPGet(mockHandler, []string{
		"1.3.6.1.2.1.1.5.0",
		"1.3.6.1.4.1.2011.5.25.31.6.5.0",
	}, []gosnmp.SnmpPDU{
		createStringPDU("1.3.6.1.2.1.1.5.0", "huawei"),
		createStringPDU("1.3.6.1.4.1.2011.5.25.31.6.5.0", "NetEngine"),
	})
	expectSNMPGet(mockHandler, []string{
		"1.3.6.1.4.1.2011.5.25.177.1.4.1",
		"1.3.6.1.4.1.2011.5.25.177.1.4.2",
		"1.3.6.1.4.1.2011.5.25.177.1.4.3",
	}, []gosnmp.SnmpPDU{
		createGauge32PDU("1.3.6.1.4.1.2011.5.25.177.1.4.1", 12),
		createGauge32PDU("1.3.6.1.4.1.2011.5.25.177.1.4.2", 4),
		createGauge32PDU("1.3.6.1.4.1.2011.5.25.177.1.4.3", 8),
	})
	expectBGPTableWalksFromFixture(mockHandler, profile, pdus)

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
	require.Empty(t, results[0].TopologyMetrics)
	require.Empty(t, results[0].LicenseRows)
	require.NotEmpty(t, results[0].BGPRows)

	device := requireBGPRowMatching(t, results[0].BGPRows, func(row ddsnmp.BGPRow) bool {
		return row.Kind == ddprofiledefinition.BGPRowKindDevice
	})
	require.True(t, device.Device.Peers.Has)
	assert.EqualValues(t, 12, device.Device.Peers.Value)
	assert.EqualValues(t, 4, device.Device.InternalPeers.Value)
	assert.EqualValues(t, 8, device.Device.ExternalPeers.Value)

	ipv4Family := requireHuaweiPeerFamilyRow(t, results[0].BGPRows, "10.45.2.2", ddprofiledefinition.BGPAddressFamilyIPv4)
	assert.Equal(t, "Public", ipv4Family.Identity.RoutingInstance)
	assert.Equal(t, "26479", ipv4Family.Identity.RemoteAS)
	assert.Equal(t, ddprofiledefinition.BGPSubsequentAddressFamilyUnicast, ipv4Family.Identity.SubsequentAddressFamily)
	require.True(t, ipv4Family.Admin.Enabled.Has)
	assert.True(t, ipv4Family.Admin.Enabled.Value)
	require.True(t, ipv4Family.State.Has)
	assert.Equal(t, ddprofiledefinition.BGPPeerStateEstablished, ipv4Family.State.State)
	assert.EqualValues(t, 2, ipv4Family.Transitions.Established.Value)
	assert.EqualValues(t, 5, ipv4Family.LastError.Code.Value)
	assert.EqualValues(t, 0, ipv4Family.LastError.Subcode.Value)

	ipv4Routes := requireHuaweiPeerFamilyRowByTable(t, results[0].BGPRows, "hwBgpPeerRouteTable", "10.45.2.2", ddprofiledefinition.BGPAddressFamilyIPv4)
	assert.EqualValues(t, 13970, ipv4Routes.Routes.Total.Received.Value)
	assert.EqualValues(t, 1007, ipv4Routes.Routes.Total.Active.Value)
	assert.EqualValues(t, 8, ipv4Routes.Routes.Total.Advertised.Value)

	ipv4Peer := requireHuaweiPeerRow(t, results[0].BGPRows, "10.45.2.2")
	assert.Equal(t, "0", ipv4Peer.Identity.RoutingInstance)
	assert.Equal(t, "26479", ipv4Peer.Identity.RemoteAS)
	assert.Equal(t, "ipv4", ipv4Peer.Descriptors.PeerType)
	assert.EqualValues(t, 2, ipv4Peer.Transitions.Established.Value)
	assert.EqualValues(t, 1, ipv4Peer.Transitions.Down.Value)
	assert.EqualValues(t, 70063, ipv4Peer.Traffic.Updates.Received.Value)
	assert.EqualValues(t, 971, ipv4Peer.Traffic.Updates.Sent.Value)
	assert.EqualValues(t, 99928, ipv4Peer.Traffic.Messages.Received.Value)
	assert.EqualValues(t, 31448, ipv4Peer.Traffic.Messages.Sent.Value)

	ipv6Neighbor := "2001:12f8::223:253"
	ipv6Family := requireHuaweiPeerFamilyRow(t, results[0].BGPRows, ipv6Neighbor, ddprofiledefinition.BGPAddressFamilyIPv6)
	assert.Equal(t, "PublicV6", ipv6Family.Identity.RoutingInstance)
	assert.Equal(t, "26162", ipv6Family.Identity.RemoteAS)
	assert.Equal(t, ddprofiledefinition.BGPSubsequentAddressFamilyUnicast, ipv6Family.Identity.SubsequentAddressFamily)
	require.True(t, ipv6Family.State.Has)
	assert.Equal(t, ddprofiledefinition.BGPPeerStateEstablished, ipv6Family.State.State)
	assert.EqualValues(t, 7, ipv6Family.Transitions.Established.Value)

	ipv6Routes := requireHuaweiPeerFamilyRowByTable(t, results[0].BGPRows, "hwBgpPeerRouteTable", ipv6Neighbor, ddprofiledefinition.BGPAddressFamilyIPv6)
	assert.EqualValues(t, 0, ipv6Routes.Routes.Total.Received.Value)
	assert.EqualValues(t, 0, ipv6Routes.Routes.Total.Active.Value)
	assert.EqualValues(t, 0, ipv6Routes.Routes.Total.Advertised.Value)

	ipv6Peer := requireHuaweiPeerRow(t, results[0].BGPRows, ipv6Neighbor)
	assert.Equal(t, "0", ipv6Peer.Identity.RoutingInstance)
	assert.Equal(t, "26162", ipv6Peer.Identity.RemoteAS)
	assert.Equal(t, "ipv6", ipv6Peer.Descriptors.PeerType)
	assert.EqualValues(t, 3318661, ipv6Peer.Traffic.Updates.Received.Value)
	assert.EqualValues(t, 6, ipv6Peer.Traffic.Updates.Sent.Value)
	assert.EqualValues(t, 3337254, ipv6Peer.Traffic.Messages.Received.Value)
	assert.EqualValues(t, 19035, ipv6Peer.Traffic.Messages.Sent.Value)
}

func requireHuaweiPeerFamilyRow(t *testing.T, rows []ddsnmp.BGPRow, neighbor string, af ddprofiledefinition.BGPAddressFamily) ddsnmp.BGPRow {
	t.Helper()

	return requireHuaweiPeerFamilyRowByTable(t, rows, "hwBgpPeerTable", neighbor, af)
}

func requireHuaweiPeerFamilyRowByTable(t *testing.T, rows []ddsnmp.BGPRow, table, neighbor string, af ddprofiledefinition.BGPAddressFamily) ddsnmp.BGPRow {
	t.Helper()

	return requireBGPRowMatching(t, rows, func(row ddsnmp.BGPRow) bool {
		return row.Kind == ddprofiledefinition.BGPRowKindPeerFamily &&
			row.Table == table &&
			row.Identity.Neighbor == neighbor &&
			row.Identity.AddressFamily == af
	})
}

func requireHuaweiPeerRow(t *testing.T, rows []ddsnmp.BGPRow, neighbor string) ddsnmp.BGPRow {
	t.Helper()

	return requireBGPRowMatching(t, rows, func(row ddsnmp.BGPRow) bool {
		return row.Kind == ddprofiledefinition.BGPRowKindPeer && row.Identity.Neighbor == neighbor
	})
}
