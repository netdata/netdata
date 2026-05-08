// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/gosnmp/gosnmp"
	snmpmock "github.com/gosnmp/gosnmp/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

func TestCollector_Collect_TiMOSBGP_FromLibreNMSFixtures(t *testing.T) {
	tests := map[string]struct {
		sysObjectID string
		fixture     string
		validate    func(t *testing.T, rows []ddsnmp.BGPRow)
	}{
		"7750 emits typed peer and peer-family rows": {
			sysObjectID: "1.3.6.1.4.1.6527.1.3.17",
			fixture:     "librenms/timos_7750_bgp_peer_table.snmprec",
			validate: func(t *testing.T, rows []ddsnmp.BGPRow) {
				peer := requireBGPRowMatching(t, rows, func(row ddsnmp.BGPRow) bool {
					return row.Kind == ddprofiledefinition.BGPRowKindPeer && row.Identity.Neighbor == "192.168.119.2"
				})
				assert.Equal(t, "_nokia-timetra-bgp.yaml", peer.OriginProfileID)
				assert.Equal(t, "tBgpPeerNgTable", peer.Table)
				assert.Equal(t, "Base", peer.Identity.RoutingInstance)
				assert.Equal(t, "12345", peer.Identity.RemoteAS)
				assert.Equal(t, "62.40.119.6", peer.Descriptors.LocalAddress)
				assert.Equal(t, "12345", peer.Descriptors.LocalAS)
				assert.Equal(t, "router.name", peer.Descriptors.Description)
				require.True(t, peer.Admin.Enabled.Has)
				assert.True(t, peer.Admin.Enabled.Value)
				require.True(t, peer.State.Has)
				assert.Equal(t, ddprofiledefinition.BGPPeerStateEstablished, peer.State.State)
				assert.EqualValues(t, 4656225, peer.Connection.EstablishedUptime.Value)

				unicast := requireTiMOSFamilyRow(t, rows, "192.168.119.2", ddprofiledefinition.BGPAddressFamilyIPv4, ddprofiledefinition.BGPSubsequentAddressFamilyUnicast)
				assert.Equal(t, "ipv4 unicast", unicast.Tags["address_family_name"])
				assert.EqualValues(t, 13, unicast.Routes.Total.Received.Value)
				assert.EqualValues(t, 660, unicast.Routes.Total.Advertised.Value)
				assert.EqualValues(t, 6, unicast.Routes.Current.Active.Value)
				assert.Equal(t, "router.name", unicast.Descriptors.Description)

				vpnv4 := requireTiMOSFamilyRow(t, rows, "192.168.119.2", ddprofiledefinition.BGPAddressFamilyIPv4, ddprofiledefinition.BGPSubsequentAddressFamilyVPN)
				assert.EqualValues(t, 43, vpnv4.Routes.Total.Received.Value)
				assert.EqualValues(t, 25, vpnv4.Routes.Current.Active.Value)

				vpnv6 := requireTiMOSFamilyRow(t, rows, "192.168.119.2", ddprofiledefinition.BGPAddressFamilyIPv6, ddprofiledefinition.BGPSubsequentAddressFamilyVPN)
				assert.EqualValues(t, 18, vpnv6.Routes.Total.Received.Value)

				vpls := requireTiMOSFamilyRow(t, rows, "192.168.119.2", ddprofiledefinition.BGPAddressFamilyL2VPN, ddprofiledefinition.BGPSubsequentAddressFamilyVPLS)
				assert.EqualValues(t, 2, vpls.Routes.Total.Received.Value)

				evpn := requireTiMOSFamilyRow(t, rows, "192.168.119.2", ddprofiledefinition.BGPAddressFamilyL2VPN, ddprofiledefinition.BGPSubsequentAddressFamilyEVPN)
				assert.EqualValues(t, 0, evpn.Routes.Total.Rejected.Value)
			},
		},
		"IXR-S emits typed peer and peer-family rows": {
			sysObjectID: "1.3.6.1.4.1.6527.1.3.23",
			fixture:     "librenms/timos_ixr_s_bgp_peer_table.snmprec",
			validate: func(t *testing.T, rows []ddsnmp.BGPRow) {
				peer := requireBGPRowMatching(t, rows, func(row ddsnmp.BGPRow) bool {
					return row.Kind == ddprofiledefinition.BGPRowKindPeer && row.Identity.Neighbor == "172.30.0.17"
				})
				assert.Equal(t, "Base", peer.Identity.RoutingInstance)
				assert.Equal(t, "393227", peer.Identity.RemoteAS)
				require.True(t, peer.Admin.Enabled.Has)
				assert.True(t, peer.Admin.Enabled.Value)
				require.True(t, peer.State.Has)
				assert.Equal(t, ddprofiledefinition.BGPPeerStateEstablished, peer.State.State)
				assert.EqualValues(t, 1800, peer.Connection.EstablishedUptime.Value)

				unicast := requireTiMOSFamilyRow(t, rows, "172.30.0.17", ddprofiledefinition.BGPAddressFamilyIPv4, ddprofiledefinition.BGPSubsequentAddressFamilyUnicast)
				assert.Equal(t, "ipv4 unicast", unicast.Tags["address_family_name"])
				assert.EqualValues(t, 0, unicast.Routes.Total.Received.Value)

				vpnv4 := requireTiMOSFamilyRow(t, rows, "172.30.0.17", ddprofiledefinition.BGPAddressFamilyIPv4, ddprofiledefinition.BGPSubsequentAddressFamilyVPN)
				assert.EqualValues(t, 184, vpnv4.Routes.Total.Received.Value)
				assert.EqualValues(t, 1, vpnv4.Routes.Total.Advertised.Value)
				assert.EqualValues(t, 174, vpnv4.Routes.Current.Active.Value)

				secondPeerVPNv4 := requireTiMOSFamilyRow(t, rows, "172.30.0.18", ddprofiledefinition.BGPAddressFamilyIPv4, ddprofiledefinition.BGPSubsequentAddressFamilyVPN)
				assert.EqualValues(t, 9, secondPeerVPNv4.Routes.Current.Active.Value)

				evpn := requireTiMOSFamilyRow(t, rows, "172.30.0.17", ddprofiledefinition.BGPAddressFamilyL2VPN, ddprofiledefinition.BGPSubsequentAddressFamilyEVPN)
				assert.EqualValues(t, 0, evpn.Routes.Total.Rejected.Value)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			profile := matchedBGPProjectionByFile(t, tc.sysObjectID, "nokia-service-router-os.yaml")
			pdus := loadSnmprecPDUs(t, tc.fixture)

			ctrl, mockHandler := setupMockHandler(t)
			defer ctrl.Finish()

			expectTiMOSMetadataGets(mockHandler, tc.sysObjectID)
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

			tc.validate(t, results[0].BGPRows)
		})
	}
}

func expectTiMOSMetadataGets(mockHandler *snmpmock.MockHandler, sysObjectID string) {
	mockHandler.EXPECT().Get(gomock.Any()).Return(&gosnmp.SnmpPacket{Variables: []gosnmp.SnmpPDU{
		createStringPDU("1.3.6.1.2.1.1.1.0", "Nokia SR OS"),
		createPDU("1.3.6.1.2.1.1.2.0", gosnmp.ObjectIdentifier, sysObjectID),
		createStringPDU("1.3.6.1.2.1.1.5.0", "timos"),
	}}, nil).AnyTimes()
}

func requireTiMOSFamilyRow(t *testing.T, rows []ddsnmp.BGPRow, neighbor string, af ddprofiledefinition.BGPAddressFamily, safi ddprofiledefinition.BGPSubsequentAddressFamily) ddsnmp.BGPRow {
	t.Helper()

	return requireBGPRowMatching(t, rows, func(row ddsnmp.BGPRow) bool {
		return row.Kind == ddprofiledefinition.BGPRowKindPeerFamily &&
			row.Identity.Neighbor == neighbor &&
			row.Identity.AddressFamily == af &&
			row.Identity.SubsequentAddressFamily == safi
	})
}
