// SPDX-License-Identifier: GPL-3.0-or-later

package profilemetrics

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/catalog"
	"github.com/stretchr/testify/require"
)

func TestProfileMetricTestProfileBuilderUsesProductionCatalog(t *testing.T) {
	trap := func(oid, name string) *testTrapDef {
		return &testTrapDef{
			OID:      oid,
			Name:     name,
			Category: "state_change",
			Severity: "notice",
		}
	}

	t.Run("stock resolution before mutation", func(t *testing.T) {
		idx := newTestProfileBuilder(t).Build()
		resolved, err := idx.ResolveTrap("IF-MIB::linkDown")
		require.NoError(t, err)
		require.Equal(t, testLinkDownTrapOID, resolved.OID)
	})

	t.Run("duplicate OID", func(t *testing.T) {
		idx := newTestProfileBuilder(t)
		require.NoError(t, idx.addTraps([]*testTrapDef{trap("1.3.6.1.4.1.99999.1", "TEST-MIB::first")}))
		require.Error(t, idx.addTraps([]*testTrapDef{trap("1.3.6.1.4.1.99999.1", "TEST-MIB::second")}))
	})

	t.Run("duplicate name", func(t *testing.T) {
		idx := newTestProfileBuilder(t)
		require.NoError(t, idx.addTraps([]*testTrapDef{trap("1.3.6.1.4.1.99999.1", "TEST-MIB::same")}))
		require.Error(t, idx.addTraps([]*testTrapDef{trap("1.3.6.1.4.1.99999.2", "TEST-MIB::same")}))
	})

	t.Run("alternate OID", func(t *testing.T) {
		idx := newTestProfileBuilder(t)
		require.NoError(t, idx.addTraps([]*testTrapDef{trap("1.3.6.1.4.1.99999.0.1", "TEST-MIB::first")}))
		require.Error(t, idx.addTraps([]*testTrapDef{trap("1.3.6.1.4.1.99999.1", "TEST-MIB::second")}))
	})

	t.Run("failed batch is atomic", func(t *testing.T) {
		idx := newTestProfileBuilder(t)
		existing := trap("1.3.6.1.4.1.99999.1", "TEST-MIB::existing")
		require.NoError(t, idx.addTraps([]*testTrapDef{existing}))
		candidate := trap("1.3.6.1.4.1.99999.2", "TEST-MIB::candidate")
		require.Error(t, idx.addTraps([]*testTrapDef{candidate, nil}))
		built := idx.Build()
		resolved, err := built.ResolveTrap(existing.OID)
		require.NoError(t, err)
		require.Equal(t, existing.OID, resolved.OID)
		_, err = built.ResolveTrap(candidate.OID)
		require.Error(t, err)
	})

	t.Run("invalid file varbind reaches production validation", func(t *testing.T) {
		idx := newTestProfileBuilder(t)
		candidate := trap("1.3.6.1.4.1.99999.1", "TEST-MIB::candidate")
		candidate.Varbinds = []any{"brokenVarbind"}
		candidate.FileVarbinds = []testFileVarbind{{Name: "brokenVarbind"}}

		var err error
		require.NotPanics(t, func() {
			err = idx.addTraps([]*testTrapDef{candidate})
		})
		require.Error(t, err)
	})

	t.Run("build returns an immutable snapshot", func(t *testing.T) {
		profile := newTestProfileBuilder(t)
		first := trap("1.3.6.1.4.1.99999.1", "TEST-MIB::first")
		require.NoError(t, profile.addTraps([]*testTrapDef{first}))
		firstEpoch := profile.Build()

		second := trap("1.3.6.1.4.1.99999.2", "TEST-MIB::second")
		require.NoError(t, profile.addTraps([]*testTrapDef{second}))
		secondEpoch := profile.Build()

		_, err := firstEpoch.ResolveTrap(second.OID)
		require.Error(t, err)
		resolved, err := secondEpoch.ResolveTrap(second.OID)
		require.NoError(t, err)
		require.Equal(t, second.OID, resolved.OID)
	})

	t.Run("references are trimmed", func(t *testing.T) {
		idx := newTestProfileBuilder(t)
		candidate := trap("1.3.6.1.4.1.99999.1", "TEST-MIB::candidate")
		require.NoError(t, idx.addTraps([]*testTrapDef{candidate}))
		built := idx.Build()
		for _, ref := range []string{" 1.3.6.1.4.1.99999.1 ", " TEST-MIB::candidate "} {
			resolved, err := built.ResolveTrap(ref)
			require.NoError(t, err)
			require.Equal(t, candidate.OID, resolved.OID)
		}
	})

	t.Run("duplicate metric definitions", func(t *testing.T) {
		idx := newTestProfileBuilder(t)
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
		idx := newTestProfileBuilder(t)
		require.NoError(t, idx.addTraps([]*testTrapDef{trap("1.3.6.1.4.1.99999.1", "TEST-MIB::candidate")}))
		chart := profileMetricChartForTest("test_events", "Test events", "snmp.trap.test.events", "events/s", "")
		rule := profileMetricRule{
			Name:   "test.events",
			Type:   " COUNTER ",
			OnTrap: " TEST-MIB::candidate ",
			Output: profileMetricOutputForTest("snmp_trap_test_events", "events", chart.ID),
		}
		require.NoError(t, idx.addDefinitions([]profileMetricRule{rule}, []profileMetricChart{chart}))
		defs, err := idx.Build().Definitions([]string{rule.Name})
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
