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
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

func Test_JuniperBGPProfilesUseOnlyJuniperPeerTables(t *testing.T) {
	dir, err := filepath.Abs("../../../config/go.d/snmp.profiles/default")
	require.NoError(t, err)

	profiles, err := loadProfilesFromDir(dir, multipath.New(dir))
	require.NoError(t, err)
	require.NotEmpty(t, profiles)

	tests := []struct {
		name        string
		sysObjectID string
		profileFile string
	}{
		{name: "generic Juniper", sysObjectID: "1.3.6.1.4.1.2636.1.1.1.2.9999", profileFile: "juniper.yaml"},
		{name: "Juniper MX", sysObjectID: "1.3.6.1.4.1.2636.1.1.1.2.21", profileFile: "juniper-mx.yaml"},
		{name: "Juniper EX", sysObjectID: "1.3.6.1.4.1.2636.1.1.1.2.30", profileFile: "juniper-ex.yaml"},
		{name: "Juniper QFX", sysObjectID: "1.3.6.1.4.1.2636.1.1.1.2.82", profileFile: "juniper-qfx.yaml"},
		{name: "Juniper SRX", sysObjectID: "1.3.6.1.4.1.2636.1.1.1.2.26", profileFile: "juniper-srx.yaml"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			matched := FindProfiles(tc.sysObjectID, "", nil)
			index := slices.IndexFunc(matched, func(p *Profile) bool {
				return strings.HasSuffix(p.SourceFile, tc.profileFile)
			})
			require.NotEqual(t, -1, index, "expected %s profile to match", tc.profileFile)

			profile := matched[index]
			assertNoMetricTable(t, profile, "bgpPeerTable")
			assertMetricTableForSymbol(t, profile, "bgpPeerState", "jnxBgpM2PeerTable")
			assertMetricTableForSymbol(t, profile, "bgpPeerLastErrorCode", "jnxBgpM2PeerErrorsTable")
			assertMetricTableForSymbol(t, profile, "bgpPeerInUpdates", "jnxBgpM2PeerCountersTable")
			assertMetricTableForSymbol(t, profile, "bgpPeerFsmEstablishedTransitions", "jnxBgpM2PeerCountersTable")
		})
	}
}

func assertNoMetricTable(t *testing.T, profile *Profile, tableName string) {
	t.Helper()

	for _, metric := range profile.Definition.Metrics {
		assert.NotEqual(t, tableName, metric.Table.Name, "did not expect %s in %s", tableName, profile.SourceFile)
	}
}

func assertMetricTableForSymbol(t *testing.T, profile *Profile, symbolName, wantTable string) {
	t.Helper()

	var tables []string
	for _, metric := range profile.Definition.Metrics {
		if !metricDefinesSymbol(metric, symbolName) {
			continue
		}
		tables = append(tables, metric.Table.Name)
	}

	require.Equal(t, []string{wantTable}, tables, "unexpected tables for %s in %s", symbolName, profile.SourceFile)
}

func metricDefinesSymbol(metric ddprofiledefinition.MetricsConfig, symbolName string) bool {
	for _, symbol := range metric.Symbols {
		if symbol.Name == symbolName {
			return true
		}
	}
	return false
}
