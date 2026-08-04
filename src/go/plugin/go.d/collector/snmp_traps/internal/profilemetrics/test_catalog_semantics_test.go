// SPDX-License-Identifier: GPL-3.0-or-later

package profilemetrics

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/catalog"
	"github.com/stretchr/testify/require"
)

func TestProfileMetricTestCatalogSemanticsMatchProduction(t *testing.T) {
	trap := func(oid, name string) *testTrapDef {
		return &testTrapDef{
			OID:      oid,
			Name:     name,
			Category: "state_change",
			Severity: "notice",
		}
	}

	t.Run("duplicate OID", func(t *testing.T) {
		idx := newTestCatalog(t)
		require.NoError(t, idx.addTraps([]*testTrapDef{trap("1.3.6.1.4.1.99999.1", "TEST-MIB::first")}))
		require.Error(t, idx.addTraps([]*testTrapDef{trap("1.3.6.1.4.1.99999.1", "TEST-MIB::second")}))
	})

	t.Run("duplicate name", func(t *testing.T) {
		idx := newTestCatalog(t)
		require.NoError(t, idx.addTraps([]*testTrapDef{trap("1.3.6.1.4.1.99999.1", "TEST-MIB::same")}))
		require.Error(t, idx.addTraps([]*testTrapDef{trap("1.3.6.1.4.1.99999.2", "TEST-MIB::same")}))
	})

	t.Run("alternate OID", func(t *testing.T) {
		idx := newTestCatalog(t)
		require.NoError(t, idx.addTraps([]*testTrapDef{trap("1.3.6.1.4.1.99999.0.1", "TEST-MIB::first")}))
		require.Error(t, idx.addTraps([]*testTrapDef{trap("1.3.6.1.4.1.99999.1", "TEST-MIB::second")}))
	})

	t.Run("failed batch is atomic", func(t *testing.T) {
		idx := newTestCatalog(t)
		candidate := trap("1.3.6.1.4.1.99999.1", "TEST-MIB::candidate")
		require.Error(t, idx.addTraps([]*testTrapDef{candidate, nil}))
		_, err := idx.ResolveTrap(candidate.OID)
		require.Error(t, err)
	})

	t.Run("references are trimmed", func(t *testing.T) {
		idx := newTestCatalog(t)
		candidate := trap("1.3.6.1.4.1.99999.1", "TEST-MIB::candidate")
		require.NoError(t, idx.addTraps([]*testTrapDef{candidate}))
		for _, ref := range []string{" 1.3.6.1.4.1.99999.1 ", " TEST-MIB::candidate "} {
			resolved, err := idx.ResolveTrap(ref)
			require.NoError(t, err)
			require.Equal(t, candidate.OID, resolved.OID)
		}
	})

	t.Run("duplicate metric definitions", func(t *testing.T) {
		idx := newTestCatalog(t)
		require.NoError(t, idx.addTraps([]*testTrapDef{trap("1.3.6.1.4.1.99999.1", "TEST-MIB::candidate")}))
		chart := profileMetricChartForTest("test_events", "Test events", "snmp.trap.test.events", "events/s", "incremental")
		rule := profileMetricRule{
			Name:   "test.events",
			Type:   profileMetricTypeCounter,
			OnTrap: "TEST-MIB::candidate",
			Output: profileMetricOutputForTest("snmp_trap_test_events", "events", chart.ID),
		}
		require.NoError(t, idx.addDefinitions([]profileMetricRule{rule}, []profileMetricChart{chart}))
		require.Error(t, idx.addDefinitions([]profileMetricRule{rule}, nil))
		require.Error(t, idx.addDefinitions(nil, []profileMetricChart{chart}))
	})

	t.Run("metric definitions are normalized", func(t *testing.T) {
		idx := newTestCatalog(t)
		require.NoError(t, idx.addTraps([]*testTrapDef{trap("1.3.6.1.4.1.99999.1", "TEST-MIB::candidate")}))
		chart := profileMetricChartForTest("test_events", "Test events", "snmp.trap.test.events", "events/s", "")
		rule := profileMetricRule{
			Name:   "test.events",
			Type:   " COUNTER ",
			OnTrap: " TEST-MIB::candidate ",
			Output: profileMetricOutputForTest("snmp_trap_test_events", "events", chart.ID),
		}
		require.NoError(t, idx.addDefinitions([]profileMetricRule{rule}, []profileMetricChart{chart}))
		defs, err := idx.Definitions([]string{rule.Name})
		require.NoError(t, err)
		gotRule := requireRule(t, defs, rule.Name)
		require.Equal(t, profileMetricTypeCounter, gotRule.Type)
		require.Equal(t, catalog.MetricIdentitySource, gotRule.Identity.Device)
		require.Equal(t, profileMetricMissingDrop, gotRule.Missing)
		require.Equal(t, profileMetricScale{Multiplier: 1, Divisor: 1}, gotRule.Scale)
		gotChart := defs.ChartsByID[chart.ID]
		require.NotNil(t, gotChart)
		require.Equal(t, "incremental", gotChart.Algorithm)
		require.Equal(t, "line", gotChart.Type)
		require.NotNil(t, gotChart.Lifecycle)
	})
}

func requireRule(t *testing.T, defs catalog.MetricDefinitions, name string) *catalog.MetricRule {
	t.Helper()
	rule := defs.RulesByName[name]
	require.NotNil(t, rule)
	return rule
}
