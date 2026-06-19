// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/multipath"
)

func Test_JuniperBGPProfilesUseOnlyJuniperPeerTables(t *testing.T) {
	dir, err := filepath.Abs("../../../config/go.d/snmp.profiles/default")
	require.NoError(t, err)

	profiles, err := loadProfilesFromDir(dir, multipath.New(dir))
	require.NoError(t, err)
	require.NotEmpty(t, profiles)

	tests := map[string]struct {
		sysObjectID string
		profileFile string
	}{
		"generic Juniper": {sysObjectID: "1.3.6.1.4.1.2636.1.1.1.2.9999", profileFile: "juniper.yaml"},
		"Juniper MX":      {sysObjectID: "1.3.6.1.4.1.2636.1.1.1.2.21", profileFile: "juniper-mx.yaml"},
		"Juniper EX":      {sysObjectID: "1.3.6.1.4.1.2636.1.1.1.2.30", profileFile: "juniper-ex.yaml"},
		"Juniper QFX":     {sysObjectID: "1.3.6.1.4.1.2636.1.1.1.2.82", profileFile: "juniper-qfx.yaml"},
		"Juniper SRX":     {sysObjectID: "1.3.6.1.4.1.2636.1.1.1.2.26", profileFile: "juniper-srx.yaml"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			matched := FindProfiles(tc.sysObjectID, "", nil)
			index := slices.IndexFunc(matched, func(p *Profile) bool {
				return strings.HasSuffix(p.SourceFile, tc.profileFile)
			})
			require.NotEqual(t, -1, index, "expected %s profile to match", tc.profileFile)

			profile := matched[index]
			assertNoMetricTable(t, profile, "bgpPeerTable")
			assertNoMetricTable(t, profile, "jnxBgpM2PeerTable")
			assertNoMetricTable(t, profile, "jnxBgpM2PeerCountersTable")
			assertNoVirtualMetric(t, profile, "bgpPeerAvailability")
			assertNoVirtualMetric(t, profile, "bgpPeerUpdates")
			assertBGPTableForRowID(t, profile, "juniper-bgp-peer", "jnxBgpM2PeerTable")
			assertBGPTableForRowID(t, profile, "juniper-bgp-peer-family", "jnxBgpM2PrefixCountersTable")
			assertBGPSixStateMapping(t, requireBGPRowByID(t, profile, "juniper-bgp-peer").State)
		})
	}
}

func assertNoMetricTable(t *testing.T, profile *Profile, tableName string) {
	t.Helper()

	for _, metric := range profile.Definition.Metrics {
		assert.NotEqual(t, tableName, metric.Table.Name, "did not expect %s in %s", tableName, profile.SourceFile)
	}
}

func assertNoVirtualMetric(t *testing.T, profile *Profile, name string) {
	t.Helper()

	for _, metric := range profile.Definition.VirtualMetrics {
		assert.NotEqual(t, name, metric.Name, "did not expect virtual metric %s in %s", name, profile.SourceFile)
	}
}

func assertBGPTableForRowID(t *testing.T, profile *Profile, rowID, wantTable string) {
	t.Helper()

	var tables []string
	for _, row := range profile.Definition.BGP {
		if row.ID == rowID {
			tables = append(tables, row.Table.Name)
		}
	}

	require.Equal(t, []string{wantTable}, tables, "unexpected BGP tables for %s in %s", rowID, profile.SourceFile)
}
