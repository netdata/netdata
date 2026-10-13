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

func TestCollector_Collect_AristaAndDellBGP_FromLibreNMSFixtures(t *testing.T) {
	tests := map[string]struct {
		sysObjectID     string
		profileFile     string
		originProfileID string
		fixture         string
		validate        func(t *testing.T, rows []ddsnmp.BGPRow, originProfileID string)
	}{
		"Arista EOS emits typed peer and peer-family rows": {
			sysObjectID:     "1.3.6.1.4.1.30065.1.3011.7050.1958.128",
			profileFile:     "arista-switch.yaml",
			originProfileID: "_arista-bgp4-v2.yaml",
			fixture:         "librenms/arista_eos_bgp_peer_table.snmprec",
			validate: func(t *testing.T, rows []ddsnmp.BGPRow, originProfileID string) {
				peer := requireBGPRowMatching(t, rows, func(row ddsnmp.BGPRow) bool {
					return row.Kind == ddprofiledefinition.BGPRowKindPeer && row.Identity.Neighbor == "192.168.0.2"
				})
				assert.Equal(t, originProfileID, peer.OriginProfileID)
				assert.Equal(t, "aristaBgp4V2PeerTable", peer.Table)
				assert.Equal(t, "1", peer.Identity.RoutingInstance)
				assert.Equal(t, "65000", peer.Identity.RemoteAS)
				assert.Equal(t, "192.168.0.1", peer.Descriptors.LocalAddress)
				assert.Equal(t, "ipv4", peer.Descriptors.PeerType)
				assert.Equal(t, "peer.example.com EBGP", peer.Descriptors.Description)
				require.True(t, peer.Admin.Enabled.Has)
				assert.True(t, peer.Admin.Enabled.Value)
				require.True(t, peer.State.Has)
				assert.Equal(t, ddprofiledefinition.BGPPeerStateEstablished, peer.State.State)
				assert.EqualValues(t, 13090321, peer.Connection.EstablishedUptime.Value)
				assert.EqualValues(t, 13362513, peer.Traffic.Updates.Received.Value)
				assert.EqualValues(t, 13579799, peer.Traffic.Updates.Sent.Value)
				assert.EqualValues(t, 13579799, peer.Traffic.Messages.Received.Value)
				assert.EqualValues(t, 17644316, peer.Traffic.Messages.Sent.Value)
				assert.EqualValues(t, 0, peer.LastError.Code.Value)
				assert.EqualValues(t, 0, peer.LastError.Subcode.Value)

				family := requireBGPRowMatching(t, rows, func(row ddsnmp.BGPRow) bool {
					return row.Kind == ddprofiledefinition.BGPRowKindPeerFamily &&
						row.Identity.Neighbor == "192.168.0.2" &&
						row.Identity.AddressFamily == ddprofiledefinition.BGPAddressFamilyIPv4 &&
						row.Identity.SubsequentAddressFamily == ddprofiledefinition.BGPSubsequentAddressFamilyUnicast
				})
				assert.Equal(t, "aristaBgp4V2PrefixGaugesTable", family.Table)
				assert.Equal(t, "1", family.Identity.RoutingInstance)
				assert.Equal(t, "65000", family.Identity.RemoteAS)
				assert.Equal(t, "192.168.0.1", family.Descriptors.LocalAddress)
				assert.Equal(t, "peer.example.com EBGP", family.Descriptors.Description)
				assert.EqualValues(t, 540336, family.Routes.Current.Received.Value)
				assert.EqualValues(t, 540339, family.Routes.Current.Accepted.Value)
			},
		},
		"Dell OS10 emits typed peer and peer-family rows": {
			sysObjectID:     "1.3.6.1.4.1.674.11000.5000.100.2.1.7",
			profileFile:     "dell-os10.yaml",
			originProfileID: "_dell-os10-bgp4-v2.yaml",
			fixture:         "librenms/dell_os10_bgp_peer_table.snmprec",
			validate: func(t *testing.T, rows []ddsnmp.BGPRow, originProfileID string) {
				peer := requireBGPRowMatching(t, rows, func(row ddsnmp.BGPRow) bool {
					return row.Kind == ddprofiledefinition.BGPRowKindPeer && row.Identity.Neighbor == "169.254.247.1"
				})
				assert.Equal(t, originProfileID, peer.OriginProfileID)
				assert.Equal(t, "dell.os10bgp4V2PeerTable", peer.Table)
				assert.Equal(t, "1", peer.Identity.RoutingInstance)
				assert.Equal(t, "64513", peer.Identity.RemoteAS)
				assert.Equal(t, "169.254.247.2", peer.Descriptors.LocalAddress)
				assert.Equal(t, "64514", peer.Descriptors.LocalAS)
				assert.Equal(t, "192.168.255.10", peer.Descriptors.LocalIdentifier)
				assert.Equal(t, "54.240.205.233", peer.Descriptors.PeerIdentifier)
				assert.Equal(t, "DIRECT CONNECT", peer.Descriptors.Description)
				require.True(t, peer.Admin.Enabled.Has)
				assert.False(t, peer.Admin.Enabled.Value)
				require.True(t, peer.State.Has)
				assert.Equal(t, ddprofiledefinition.BGPPeerStateEstablished, peer.State.State)
				assert.EqualValues(t, 28063000, peer.Connection.EstablishedUptime.Value)
				assert.EqualValues(t, 27997600, peer.Connection.LastReceivedUpdateAge.Value)
				assert.EqualValues(t, 17, peer.Traffic.Updates.Received.Value)
				assert.EqualValues(t, 4830, peer.Traffic.Updates.Sent.Value)
				assert.EqualValues(t, 430177, peer.Traffic.Messages.Received.Value)
				assert.EqualValues(t, 489554, peer.Traffic.Messages.Sent.Value)
				assert.EqualValues(t, 7, peer.Transitions.Established.Value)
				assert.EqualValues(t, 0, peer.LastError.Code.Value)
				assert.EqualValues(t, 0, peer.LastError.Subcode.Value)

				family := requireBGPRowMatching(t, rows, func(row ddsnmp.BGPRow) bool {
					return row.Kind == ddprofiledefinition.BGPRowKindPeerFamily &&
						row.Identity.Neighbor == "169.254.247.1" &&
						row.Identity.AddressFamily == ddprofiledefinition.BGPAddressFamilyIPv4 &&
						row.Identity.SubsequentAddressFamily == ddprofiledefinition.BGPSubsequentAddressFamilyUnicast
				})
				assert.Equal(t, "dell.os10bgp4V2PrefixGaugesTable", family.Table)
				assert.Equal(t, "1", family.Identity.RoutingInstance)
				assert.Equal(t, "64513", family.Identity.RemoteAS)
				assert.Equal(t, "169.254.247.2", family.Descriptors.LocalAddress)
				assert.Equal(t, "DIRECT CONNECT", family.Descriptors.Description)
				assert.EqualValues(t, 27, family.Routes.Current.Received.Value)
				assert.EqualValues(t, 27, family.Routes.Current.Accepted.Value)
				assert.EqualValues(t, 69, family.Routes.Current.Advertised.Value)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			profile := matchedBGPProjectionByFile(t, tc.sysObjectID, tc.profileFile)
			pdus := loadSnmprecPDUs(t, tc.fixture)

			ctrl, mockHandler := setupMockHandler(t)
			defer ctrl.Finish()

			expectSystemMetadataGets(mockHandler, tc.sysObjectID, "bgp-device")
			expectBGPTableWalksFromFixture(mockHandler, profile, pdus)

			collector := New(Config{
				SnmpClient:  mockHandler,
				Profiles:    []*ddsnmp.Profile{profile},
				Log:         logger.New(),
				SysObjectID: tc.sysObjectID,
			})

			results, err := collector.Collect()
			require.NoError(t, err)
			require.Len(t, results, 1)
			require.Empty(t, results[0].Metrics)
			require.Empty(t, results[0].TopologyMetrics)
			require.Empty(t, results[0].LicenseRows)
			require.NotEmpty(t, results[0].BGPRows)

			tc.validate(t, results[0].BGPRows, tc.originProfileID)
		})
	}
}

func TestCollector_Collect_DellOS10BGP_IPv6Rows(t *testing.T) {
	sysObjectID := "1.3.6.1.4.1.674.11000.5000.100.2.1.7"
	profile := matchedBGPProjectionByFile(t, sysObjectID, "dell-os10.yaml")

	peerIndex := "1.2.16.32.1.13.184.0.0.0.0.0.0.0.0.0.0.0.1"
	familyIndex := peerIndex + ".2.1"
	peerOID := func(column string) string { return column + "." + peerIndex }
	familyOID := func(column string) string { return column + "." + familyIndex }
	pdus := []gosnmp.SnmpPDU{
		createPDU(peerOID("1.3.6.1.4.1.674.11000.5000.200.1.1.2.1.3"), gosnmp.OctetString, []byte{0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2}),
		createGauge32PDU(peerOID("1.3.6.1.4.1.674.11000.5000.200.1.1.2.1.7"), 64514),
		createStringPDU(peerOID("1.3.6.1.4.1.674.11000.5000.200.1.1.2.1.8"), "192.0.2.10"),
		createGauge32PDU(peerOID("1.3.6.1.4.1.674.11000.5000.200.1.1.2.1.10"), 64513),
		createStringPDU(peerOID("1.3.6.1.4.1.674.11000.5000.200.1.1.2.1.11"), "192.0.2.20"),
		createIntegerPDU(peerOID("1.3.6.1.4.1.674.11000.5000.200.1.1.2.1.12"), 2),
		createIntegerPDU(peerOID("1.3.6.1.4.1.674.11000.5000.200.1.1.2.1.13"), 6),
		createStringPDU(peerOID("1.3.6.1.4.1.674.11000.5000.200.1.1.2.1.14"), "IPv6 DIRECT CONNECT"),
		createGauge32PDU(peerOID("1.3.6.1.4.1.674.11000.5000.200.1.1.7.1.3"), 4096),
		createGauge32PDU(familyOID("1.3.6.1.4.1.674.11000.5000.200.1.1.8.1.3"), 127),
		createGauge32PDU(familyOID("1.3.6.1.4.1.674.11000.5000.200.1.1.8.1.4"), 126),
		createGauge32PDU(familyOID("1.3.6.1.4.1.674.11000.5000.200.1.1.8.1.5"), 269),
	}

	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	expectSystemMetadataGets(mockHandler, sysObjectID, "dell-ipv6")
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

	rows := results[0].BGPRows
	peer := requireBGPRowMatching(t, rows, func(row ddsnmp.BGPRow) bool {
		return row.Kind == ddprofiledefinition.BGPRowKindPeer &&
			row.Identity.Neighbor == "2001:db8::1"
	})
	assert.Equal(t, "_dell-os10-bgp4-v2.yaml", peer.OriginProfileID)
	assert.Equal(t, "1", peer.Identity.RoutingInstance)
	assert.Equal(t, "64513", peer.Identity.RemoteAS)
	assert.Equal(t, "2001:db8::2", peer.Descriptors.LocalAddress)
	assert.Equal(t, "64514", peer.Descriptors.LocalAS)
	assert.Equal(t, "ipv6", peer.Descriptors.PeerType)
	assert.Equal(t, "IPv6 DIRECT CONNECT", peer.Descriptors.Description)
	require.True(t, peer.State.Has)
	assert.Equal(t, ddprofiledefinition.BGPPeerStateEstablished, peer.State.State)
	require.True(t, peer.Traffic.Messages.Received.Has)
	assert.EqualValues(t, 4096, peer.Traffic.Messages.Received.Value)

	family := requireBGPRowMatching(t, rows, func(row ddsnmp.BGPRow) bool {
		return row.Kind == ddprofiledefinition.BGPRowKindPeerFamily &&
			row.Identity.Neighbor == "2001:db8::1" &&
			row.Identity.AddressFamily == ddprofiledefinition.BGPAddressFamilyIPv6 &&
			row.Identity.SubsequentAddressFamily == ddprofiledefinition.BGPSubsequentAddressFamilyUnicast
	})
	assert.Equal(t, "64513", family.Identity.RemoteAS)
	assert.Equal(t, "2001:db8::2", family.Descriptors.LocalAddress)
	assert.Equal(t, "ipv6", family.Descriptors.PeerType)
	assert.Equal(t, "IPv6 DIRECT CONNECT", family.Descriptors.Description)
	assert.EqualValues(t, 127, family.Routes.Current.Received.Value)
	assert.EqualValues(t, 126, family.Routes.Current.Accepted.Value)
	assert.EqualValues(t, 269, family.Routes.Current.Advertised.Value)
}
