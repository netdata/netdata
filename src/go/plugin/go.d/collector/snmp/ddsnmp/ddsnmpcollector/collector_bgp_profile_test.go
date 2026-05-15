// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

func TestCollector_Collect_StdBGP4Profile(t *testing.T) {
	tests := map[string]struct {
		fixture string
	}{
		"generic BGP4-MIB peer table": {
			fixture: "librenms/pfsense_frr_bgp_peer_table.snmprec",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			profile := mustLoadTypedBGPProfile(t, "_std-bgp4-mib", func(row ddprofiledefinition.BGPConfig) bool {
				return row.ID == "bgp4-peer"
			})

			pdus := loadSnmprecPDUs(t, tc.fixture)
			pdus = append(pdus,
				createPDU("1.3.6.1.2.1.15.3.1.7.169.254.1.1", gosnmp.IPAddress, "169.254.1.1"),
				createPDU("1.3.6.1.2.1.15.3.1.7.169.254.1.9", gosnmp.IPAddress, "169.254.1.9"),
			)

			ctrl, mockHandler := setupMockHandler(t)
			defer ctrl.Finish()

			expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.3.6.1.2.1.15.3", pdus)

			collector := New(Config{
				SnmpClient: mockHandler,
				Profiles:   []*ddsnmp.Profile{profile},
				Log:        logger.New(),
			})

			results, err := collector.Collect()
			require.NoError(t, err)
			require.Len(t, results, 1)

			pm := results[0]
			require.Empty(t, pm.HiddenMetrics)
			require.Empty(t, pm.Metrics)
			require.Empty(t, pm.TopologyMetrics)
			require.Empty(t, pm.LicenseRows)
			require.Len(t, pm.BGPRows, 2)
			assert.EqualValues(t, 2, pm.Stats.Metrics.BGP)

			rows := bgpRowsByNeighbor(pm.BGPRows)
			row := rows["169.254.1.1"]
			require.NotZero(t, row)
			assert.Equal(t, "_std-bgp4-mib.yaml", row.OriginProfileID)
			assert.Equal(t, ddprofiledefinition.BGPRowKindPeer, row.Kind)
			assert.Equal(t, "1.3.6.1.2.1.15.3", row.TableOID)
			assert.Equal(t, "bgpPeerTable", row.Table)
			assert.Equal(t, "169.254.1.1", row.RowKey)
			assert.Equal(t, "default", row.Identity.RoutingInstance)
			assert.Equal(t, "169.254.1.1", row.Identity.Neighbor)
			assert.Equal(t, "4200000000", row.Identity.RemoteAS)
			assert.Equal(t, "169.254.1.2", row.Descriptors.LocalAddress)
			assert.True(t, row.Admin.Enabled.Has)
			assert.True(t, row.Admin.Enabled.Value)
			require.True(t, row.State.Has)
			assert.Equal(t, ddprofiledefinition.BGPPeerStateEstablished, row.State.State)
			assert.EqualValues(t, 96951, row.Connection.EstablishedUptime.Value)
			assert.EqualValues(t, 96950, row.Connection.LastReceivedUpdateAge.Value)
			assert.EqualValues(t, 8330, row.Traffic.Messages.Received.Value)
			assert.EqualValues(t, 8323, row.Traffic.Messages.Sent.Value)
			assert.EqualValues(t, 6, row.Traffic.Updates.Received.Value)
			assert.EqualValues(t, 14, row.Traffic.Updates.Sent.Value)
			assert.EqualValues(t, 4, row.LastError.Code.Value)
			assert.EqualValues(t, 0, row.LastError.Subcode.Value)
		})
	}
}

func mustLoadTypedBGPProfile(t *testing.T, profileName string, keep func(row ddprofiledefinition.BGPConfig) bool) *ddsnmp.Profile {
	t.Helper()

	profile, err := ddsnmp.LoadProfileByName(profileName)
	require.NoError(t, err)

	filterProfileForTypedBGP(t, profile, keep)
	assert.Equal(t, profileName+".yaml", filepath.Base(profile.SourceFile))

	return profile
}

func filterProfileForTypedBGPByID(t *testing.T, profile *ddsnmp.Profile, id string) {
	t.Helper()

	filterProfileForTypedBGP(t, profile, func(row ddprofiledefinition.BGPConfig) bool {
		return row.ID == id
	})
}

func filterProfileForTypedBGP(t *testing.T, profile *ddsnmp.Profile, keep func(row ddprofiledefinition.BGPConfig) bool) {
	t.Helper()

	profile.Definition.Metadata = nil
	profile.Definition.SysobjectIDMetadata = nil
	profile.Definition.MetricTags = nil
	profile.Definition.StaticTags = nil
	profile.Definition.VirtualMetrics = nil
	profile.Definition.Topology = nil
	profile.Definition.Metrics = nil
	profile.Definition.Licensing = nil
	profile.Definition.BGP = slices.DeleteFunc(profile.Definition.BGP, func(row ddprofiledefinition.BGPConfig) bool {
		return !keep(row)
	})

	require.NotEmpty(t, profile.Definition.BGP)
}

func bgpRowsByNeighbor(rows []ddsnmp.BGPRow) map[string]ddsnmp.BGPRow {
	out := make(map[string]ddsnmp.BGPRow, len(rows))
	for _, row := range rows {
		out[row.Identity.Neighbor] = row
	}
	return out
}
