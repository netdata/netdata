// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"strings"
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

func TestCollector_Collect_JuniperBGP_FromLibreNMSFixtures(t *testing.T) {
	tests := map[string]struct {
		sysObjectID string
		profileFile string
		fixture     string
		validate    func(t *testing.T, rows []ddsnmp.BGPRow)
	}{
		"Juniper MX emits typed peer and peer-family rows": {
			sysObjectID: "1.3.6.1.4.1.2636.1.1.1.2.57",
			profileFile: "juniper-mx.yaml",
			fixture:     "librenms/juniper_mx_bgp_peer_table.snmprec",
			validate: func(t *testing.T, rows []ddsnmp.BGPRow) {
				peer := requireBGPRowMatching(t, rows, func(row ddsnmp.BGPRow) bool {
					return row.Kind == ddprofiledefinition.BGPRowKindPeer && row.Identity.Neighbor == "192.168.99.25"
				})
				assert.Equal(t, "_juniper-bgp4-v2.yaml", peer.OriginProfileID)
				assert.Equal(t, "jnxBgpM2PeerTable", peer.Table)
				assert.Equal(t, "0", peer.Identity.RoutingInstance)
				assert.Equal(t, "64513", peer.Identity.RemoteAS)
				assert.Equal(t, "192.168.99.24", peer.Descriptors.LocalAddress)
				assert.Equal(t, "64513", peer.Descriptors.LocalAS)
				assert.Equal(t, "192.168.99.25", peer.Descriptors.PeerIdentifier)
				require.True(t, peer.Admin.Enabled.Has)
				assert.True(t, peer.Admin.Enabled.Value)
				require.True(t, peer.State.Has)
				assert.Equal(t, ddprofiledefinition.BGPPeerStateEstablished, peer.State.State)
				assert.EqualValues(t, 361781, peer.Connection.EstablishedUptime.Value)
				assert.EqualValues(t, 49798, peer.Connection.LastReceivedUpdateAge.Value)
				assert.EqualValues(t, 56, peer.Traffic.Updates.Received.Value)
				assert.EqualValues(t, 1, peer.Traffic.Updates.Sent.Value)
				assert.EqualValues(t, 13199, peer.Traffic.Messages.Received.Value)
				assert.EqualValues(t, 13351, peer.Traffic.Messages.Sent.Value)

				unicast := requireBGPRowMatching(t, rows, func(row ddsnmp.BGPRow) bool {
					return row.Kind == ddprofiledefinition.BGPRowKindPeerFamily &&
						row.Identity.Neighbor == "192.168.99.25" &&
						row.Identity.AddressFamily == ddprofiledefinition.BGPAddressFamilyIPv4 &&
						row.Identity.SubsequentAddressFamily == ddprofiledefinition.BGPSubsequentAddressFamilyUnicast
				})
				assert.Equal(t, "jnxBgpM2PrefixCountersTable", unicast.Table)
				assert.Equal(t, "0", unicast.Identity.RoutingInstance)
				assert.Equal(t, "64513", unicast.Identity.RemoteAS)
				assert.EqualValues(t, 20, unicast.Routes.Current.Received.Value)
				assert.EqualValues(t, 20, unicast.Routes.Current.Accepted.Value)
				assert.EqualValues(t, 0, unicast.Routes.Current.Advertised.Value)
				assert.EqualValues(t, 6, unicast.Routes.Current.Active.Value)

				vpls := requireBGPRowMatching(t, rows, func(row ddsnmp.BGPRow) bool {
					return row.Kind == ddprofiledefinition.BGPRowKindPeerFamily &&
						row.Identity.Neighbor == "192.168.99.25" &&
						row.Identity.AddressFamily == ddprofiledefinition.BGPAddressFamilyL2VPN &&
						row.Identity.SubsequentAddressFamily == ddprofiledefinition.BGPSubsequentAddressFamilyVPLS
				})
				assert.EqualValues(t, 0, vpls.Routes.Current.Received.Value)
			},
		},
		"Juniper vMX emits typed peer and peer-family rows": {
			sysObjectID: "1.3.6.1.4.1.2636.1.1.1.2.108",
			profileFile: "juniper-mx.yaml",
			fixture:     "librenms/juniper_vmx_bgp_peer_table.snmprec",
			validate: func(t *testing.T, rows []ddsnmp.BGPRow) {
				firstPeer := requireBGPRowMatching(t, rows, func(row ddsnmp.BGPRow) bool {
					return row.Kind == ddprofiledefinition.BGPRowKindPeer && row.Identity.Neighbor == "10.1.1.1"
				})
				assert.Equal(t, "9", firstPeer.Identity.RoutingInstance)
				assert.Equal(t, "62371", firstPeer.Identity.RemoteAS)
				assert.Equal(t, "10.1.1.2", firstPeer.Descriptors.LocalAddress)
				assert.Equal(t, "172.16.0.10", firstPeer.Descriptors.PeerIdentifier)
				require.True(t, firstPeer.State.Has)
				assert.Equal(t, ddprofiledefinition.BGPPeerStateEstablished, firstPeer.State.State)
				assert.EqualValues(t, 2, firstPeer.Traffic.Updates.Received.Value)
				assert.EqualValues(t, 2, firstPeer.Traffic.Updates.Sent.Value)
				assert.EqualValues(t, 468, firstPeer.Traffic.Messages.Received.Value)
				assert.EqualValues(t, 12859, firstPeer.Connection.EstablishedUptime.Value)

				secondPeer := requireBGPRowMatching(t, rows, func(row ddsnmp.BGPRow) bool {
					return row.Kind == ddprofiledefinition.BGPRowKindPeer && row.Identity.Neighbor == "10.1.1.5"
				})
				assert.Equal(t, "10.1.1.6", secondPeer.Descriptors.LocalAddress)
				assert.Equal(t, "172.16.0.11", secondPeer.Descriptors.PeerIdentifier)
				assert.EqualValues(t, 477, secondPeer.Traffic.Messages.Sent.Value)
				assert.EqualValues(t, 12757, secondPeer.Connection.LastReceivedUpdateAge.Value)

				firstFamily := requireBGPRowMatching(t, rows, func(row ddsnmp.BGPRow) bool {
					return row.Kind == ddprofiledefinition.BGPRowKindPeerFamily &&
						row.Identity.Neighbor == "10.1.1.1" &&
						row.Identity.AddressFamily == ddprofiledefinition.BGPAddressFamilyIPv4 &&
						row.Identity.SubsequentAddressFamily == ddprofiledefinition.BGPSubsequentAddressFamilyUnicast
				})
				assert.EqualValues(t, 1, firstFamily.Routes.Current.Accepted.Value)
				assert.EqualValues(t, 2, firstFamily.Routes.Current.Advertised.Value)
				assert.EqualValues(t, 1, firstFamily.Routes.Current.Active.Value)

				secondFamily := requireBGPRowMatching(t, rows, func(row ddsnmp.BGPRow) bool {
					return row.Kind == ddprofiledefinition.BGPRowKindPeerFamily &&
						row.Identity.Neighbor == "10.1.1.5" &&
						row.Identity.AddressFamily == ddprofiledefinition.BGPAddressFamilyIPv4 &&
						row.Identity.SubsequentAddressFamily == ddprofiledefinition.BGPSubsequentAddressFamilyUnicast
				})
				assert.EqualValues(t, 0, secondFamily.Routes.Current.Rejected.Value)
				assert.EqualValues(t, 1, secondFamily.Routes.Current.Active.Value)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			profile := matchedBGPProjectionByFile(t, tc.sysObjectID, tc.profileFile)
			pdus := loadSnmprecPDUs(t, tc.fixture)

			ctrl, mockHandler := setupMockHandler(t)
			defer ctrl.Finish()

			expectJuniperMetadataGets(mockHandler)
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

func matchedBGPProjectionByFile(t *testing.T, sysObjectID, profileFile string) *ddsnmp.Profile {
	t.Helper()

	profiles := ddsnmp.DefaultCatalog().Resolve(ddsnmp.ResolveRequest{
		SysObjectID: sysObjectID,
	}).Project(ddsnmp.ConsumerBGP).Profiles()

	for _, prof := range profiles {
		if strings.HasSuffix(prof.SourceFile, profileFile) {
			return prof
		}
	}

	require.FailNowf(t, "missing profile", "expected %s BGP projection for %s", profileFile, sysObjectID)
	return nil
}

func expectJuniperMetadataGets(mockHandler *snmpmock.MockHandler) {
	mockHandler.EXPECT().Get(gomock.Any()).Return(&gosnmp.SnmpPacket{Variables: []gosnmp.SnmpPDU{
		createStringPDU("1.0.8802.1.1.2.1.3.1.0", "juniper"),
		createStringPDU("1.0.8802.1.1.2.1.3.2.0", "juniper"),
		createStringPDU("1.0.8802.1.1.2.1.3.3.0", "juniper"),
		createStringPDU("1.0.8802.1.1.2.1.3.4.0", "juniper"),
		createStringPDU("1.0.8802.1.1.2.1.3.5.0", "juniper"),
		createStringPDU("1.0.8802.1.1.2.1.3.6.0", "juniper"),
		createStringPDU("1.3.6.1.2.1.1.1.0", "Juniper Networks"),
		createPDU("1.3.6.1.2.1.1.2.0", gosnmp.ObjectIdentifier, "1.3.6.1.4.1.2636.1.1.1.2.57"),
		createStringPDU("1.3.6.1.2.1.1.5.0", "juniper"),
		createStringPDU("1.3.6.1.2.1.1.6.0", "lab"),
	}}, nil).AnyTimes()
}

func expectBGPTableWalksFromFixture(mockHandler *snmpmock.MockHandler, profile *ddsnmp.Profile, pdus []gosnmp.SnmpPDU) {
	mockHandler.EXPECT().Version().Return(gosnmp.Version2c).AnyTimes()

	tableNameToOID := bgpTableNameToOID(profile.Definition.BGP)
	walked := make(map[string]bool)
	for _, cfg := range profile.Definition.BGP {
		if cfg.Table.OID == "" {
			continue
		}
		tableOID := trimOID(cfg.Table.OID)
		if !walked[tableOID] {
			mockHandler.EXPECT().BulkWalkAll(tableOID).Return(snmprecPDUsWithPrefix(pdus, tableOID+"."), nil)
			walked[tableOID] = true
		}
		metricsCfg := bgpConfigAsMetricsConfig(cfg)
		for _, depOID := range bgpTableDependencies(cfg, metricsCfg, tableNameToOID) {
			depOID = trimOID(depOID)
			if walked[depOID] {
				continue
			}
			mockHandler.EXPECT().BulkWalkAll(depOID).Return(snmprecPDUsWithPrefix(pdus, depOID+"."), nil)
			walked[depOID] = true
		}
	}
}

func requireBGPRowMatching(t *testing.T, rows []ddsnmp.BGPRow, match func(ddsnmp.BGPRow) bool) ddsnmp.BGPRow {
	t.Helper()

	for _, row := range rows {
		if match(row) {
			return row
		}
	}

	require.FailNowf(t, "missing BGP row", "available rows: %v", rows)
	return ddsnmp.BGPRow{}
}
