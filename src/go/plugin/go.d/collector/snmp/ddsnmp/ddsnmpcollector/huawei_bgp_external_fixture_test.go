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

	families := map[string]struct {
		neighbor         string
		af               ddprofiledefinition.BGPAddressFamily
		routingInstance  string
		remoteAS         string
		establishedTrans int64
		lastErrorCode    int64
		lastErrorSubcode int64
		wantAdmin        bool
		wantAdminEnabled bool
		wantLastError    bool
	}{
		"IPv4 family": {
			neighbor:         "10.45.2.2",
			af:               ddprofiledefinition.BGPAddressFamilyIPv4,
			routingInstance:  "Public",
			remoteAS:         "26479",
			establishedTrans: 2,
			lastErrorCode:    5,
			lastErrorSubcode: 0,
			wantAdmin:        true,
			wantAdminEnabled: true,
			wantLastError:    true,
		},
		"IPv6 family": {
			neighbor:         "2001:12f8::223:253",
			af:               ddprofiledefinition.BGPAddressFamilyIPv6,
			routingInstance:  "PublicV6",
			remoteAS:         "26162",
			establishedTrans: 7,
		},
	}

	for name, tc := range families {
		t.Run(name, func(t *testing.T) {
			row := requireHuaweiPeerFamilyRow(t, results[0].BGPRows, tc.neighbor, tc.af)
			assert.Equal(t, tc.routingInstance, row.Identity.RoutingInstance)
			assert.Equal(t, tc.remoteAS, row.Identity.RemoteAS)
			assert.Equal(t, ddprofiledefinition.BGPSubsequentAddressFamilyUnicast, row.Identity.SubsequentAddressFamily)
			if tc.wantAdmin {
				require.True(t, row.Admin.Enabled.Has)
				assert.Equal(t, tc.wantAdminEnabled, row.Admin.Enabled.Value)
			}
			require.True(t, row.State.Has)
			assert.Equal(t, ddprofiledefinition.BGPPeerStateEstablished, row.State.State)
			assert.EqualValues(t, tc.establishedTrans, row.Transitions.Established.Value)
			if tc.wantLastError {
				assert.EqualValues(t, tc.lastErrorCode, row.LastError.Code.Value)
				assert.EqualValues(t, tc.lastErrorSubcode, row.LastError.Subcode.Value)
			}
		})
	}

	routeRows := map[string]struct {
		neighbor   string
		af         ddprofiledefinition.BGPAddressFamily
		received   int64
		active     int64
		advertised int64
	}{
		"IPv4 routes": {
			neighbor:   "10.45.2.2",
			af:         ddprofiledefinition.BGPAddressFamilyIPv4,
			received:   13970,
			active:     1007,
			advertised: 8,
		},
		"IPv6 routes": {
			neighbor: "2001:12f8::223:253",
			af:       ddprofiledefinition.BGPAddressFamilyIPv6,
		},
	}

	for name, tc := range routeRows {
		t.Run(name, func(t *testing.T) {
			row := requireHuaweiPeerFamilyRowByTable(t, results[0].BGPRows, "hwBgpPeerRouteTable", tc.neighbor, tc.af)
			assert.EqualValues(t, tc.received, row.Routes.Total.Received.Value)
			assert.EqualValues(t, tc.active, row.Routes.Total.Active.Value)
			assert.EqualValues(t, tc.advertised, row.Routes.Total.Advertised.Value)
		})
	}

	peers := map[string]struct {
		neighbor         string
		remoteAS         string
		peerType         string
		establishedTrans int64
		downTrans        int64
		wantTransitions  bool
		updatesReceived  int64
		updatesSent      int64
		messagesReceived int64
		messagesSent     int64
	}{
		"IPv4 peer": {
			neighbor:         "10.45.2.2",
			remoteAS:         "26479",
			peerType:         "ipv4",
			establishedTrans: 2,
			downTrans:        1,
			wantTransitions:  true,
			updatesReceived:  70063,
			updatesSent:      971,
			messagesReceived: 99928,
			messagesSent:     31448,
		},
		"IPv6 peer": {
			neighbor:         "2001:12f8::223:253",
			remoteAS:         "26162",
			peerType:         "ipv6",
			updatesReceived:  3318661,
			updatesSent:      6,
			messagesReceived: 3337254,
			messagesSent:     19035,
		},
	}

	for name, tc := range peers {
		t.Run(name, func(t *testing.T) {
			row := requireHuaweiPeerRow(t, results[0].BGPRows, tc.neighbor)
			assert.Equal(t, "0", row.Identity.RoutingInstance)
			assert.Equal(t, tc.remoteAS, row.Identity.RemoteAS)
			assert.Equal(t, tc.peerType, row.Descriptors.PeerType)
			if tc.wantTransitions {
				assert.EqualValues(t, tc.establishedTrans, row.Transitions.Established.Value)
				assert.EqualValues(t, tc.downTrans, row.Transitions.Down.Value)
			}
			assert.EqualValues(t, tc.updatesReceived, row.Traffic.Updates.Received.Value)
			assert.EqualValues(t, tc.updatesSent, row.Traffic.Updates.Sent.Value)
			assert.EqualValues(t, tc.messagesReceived, row.Traffic.Messages.Received.Value)
			assert.EqualValues(t, tc.messagesSent, row.Traffic.Messages.Sent.Value)
		})
	}
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
