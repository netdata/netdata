// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

func TestCollector_Collect_CiscoBgpPeer2Prefixes_FromLibreNMSFixtures(t *testing.T) {
	tests := map[string]struct {
		sysObjectID string
		profileFile string
		fixture     string
		validate    func(t *testing.T, rows []ddsnmp.BGPRow)
	}{
		"ASR9001 emits VPNv4 prefix rows": {
			sysObjectID: "1.3.6.1.4.1.9.1.1639",
			profileFile: "cisco-asr.yaml",
			fixture:     "librenms/cisco_iosxr_asr9001_bgp_peer2_prefix_table.snmprec",
			validate: func(t *testing.T, rows []ddsnmp.BGPRow) {
				row := requireCiscoPeerFamilyRow(t, rows, "10.246.0.3", "ipv4", "vpn")
				assert.Equal(t, "10.246.0.1", row.Descriptors.LocalAddress)
				assert.Equal(t, "65056", row.Identity.RemoteAS)
				assert.EqualValues(t, 5, row.Routes.Current.Accepted.Value)
				assert.EqualValues(t, 2097152, row.RouteLimits.Limit.Value)
				assert.EqualValues(t, 75, row.RouteLimits.Threshold.Value)
				assert.EqualValues(t, 25, row.Routes.Current.Advertised.Value)
				assert.EqualValues(t, 7, row.Routes.Current.Withdrawn.Value)

				idleRow := requireCiscoPeerFamilyRow(t, rows, "10.246.0.4", "ipv4", "vpn")
				assert.Equal(t, "0.0.0.0", idleRow.Descriptors.LocalAddress)
				assert.Equal(t, "65056", idleRow.Identity.RemoteAS)
				assert.EqualValues(t, 0, idleRow.Routes.Current.Accepted.Value)
			},
		},
		"NCS540 emits IPv4 unicast prefix rows": {
			sysObjectID: "1.3.6.1.4.1.9.1.2984",
			profileFile: "cisco-ncs.yaml",
			fixture:     "librenms/cisco_iosxr_ncs540_bgp_peer2_prefix_table.snmprec",
			validate: func(t *testing.T, rows []ddsnmp.BGPRow) {
				row := requireCiscoPeerFamilyRow(t, rows, "10.255.9.1", "ipv4", "unicast")
				assert.Equal(t, "10.255.9.31", row.Descriptors.LocalAddress)
				assert.Equal(t, "65011", row.Identity.RemoteAS)
				assert.EqualValues(t, 30, row.Routes.Current.Accepted.Value)
				assert.EqualValues(t, 4294967295, row.RouteLimits.Limit.Value)
				assert.EqualValues(t, 75, row.RouteLimits.Threshold.Value)
				assert.EqualValues(t, 75, row.RouteLimits.ClearThreshold.Value)
				assert.EqualValues(t, 8, row.Routes.Current.Advertised.Value)
				assert.EqualValues(t, 0, row.Routes.Current.Withdrawn.Value)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			rows := collectCiscoPeer2PrefixRowsFromFixture(t, tc.sysObjectID, tc.profileFile, tc.fixture)
			tc.validate(t, rows)
		})
	}
}

func collectCiscoPeer2PrefixRowsFromFixture(t *testing.T, sysObjectID, profileFile, fixturePath string) []ddsnmp.BGPRow {
	t.Helper()

	profile := matchedProfileByFile(t, sysObjectID, profileFile)
	filterProfileForTypedBGPByID(t, profile, "cisco-bgp-peer-family")

	pdus := loadSnmprecPDUs(t, fixturePath)

	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.3.6.1.4.1.9.9.187.1.2.8", snmprecPDUsWithPrefix(pdus, "1.3.6.1.4.1.9.9.187.1.2.8."))
	expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.3.6.1.4.1.9.9.187.1.2.5.1", snmprecPDUsWithPrefix(pdus, "1.3.6.1.4.1.9.9.187.1.2.5.1."))

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
	require.NotEmpty(t, results[0].BGPRows)

	return results[0].BGPRows
}

func requireCiscoPeerFamilyRow(t *testing.T, rows []ddsnmp.BGPRow, neighbor, afi, safi string) ddsnmp.BGPRow {
	t.Helper()

	for _, row := range rows {
		if row.Identity.Neighbor == neighbor &&
			string(row.Identity.AddressFamily) == afi &&
			string(row.Identity.SubsequentAddressFamily) == safi {
			return row
		}
	}

	require.FailNowf(t, "missing BGP row", "missing Cisco BGP peer-family row for %s %s/%s", neighbor, afi, safi)
	return ddsnmp.BGPRow{}
}
