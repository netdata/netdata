// SPDX-License-Identifier: GPL-3.0-or-later

package profilemetrics

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/catalog"
	"github.com/stretchr/testify/require"
)

func TestStaticTestCatalogSnapshotsMutableInputs(t *testing.T) {
	enabled := true
	exists := true
	absent := false
	whereValues := []any{"original"}
	stateRange := []any{1, 2}
	dimensions := &charttpl.DimensionLifecycle{MaxDims: 5, ExpireAfterCycles: 6}
	inlineVarbind := map[any]any{
		"name": "inlineVarbind",
		"oid":  "1.3.6.1.4.1.99999.3",
		"type": "INTEGER",
		"enum": map[any]any{"1": "original"},
	}
	labels := map[string]string{"site": "original"}
	enum := map[string]string{"1": "original"}

	trap := &catalog.TrapDef{
		OID:         "1.3.6.1.4.1.99999.1",
		Name:        "TEST-MIB::event",
		VarbindRefs: []any{inlineVarbind},
		Labels:      labels,
		SharedVarbinds: map[string]*catalog.VarbindDef{
			"1.3.6.1.4.1.99999.2": {OID: "1.3.6.1.4.1.99999.2", Type: "INTEGER", Enum: enum},
		},
	}
	rule := catalog.MetricRule{
		Name:     "test.rule",
		Type:     catalog.MetricTypeState,
		Enabled:  &enabled,
		OnTrap:   trap.OID,
		Identity: catalog.MetricIdentity{Device: catalog.MetricIdentitySource},
		Where: catalog.MetricPredicates{
			{Field: "category", In: whereValues},
			{Field: "severity", Exists: &exists},
		},
		State: catalog.MetricState{
			SetWhen:   &catalog.MetricPredicate{Field: "severity", Range: stateRange},
			ClearWhen: &catalog.MetricPredicate{Field: "category", Absent: &absent},
		},
		Output:  catalog.MetricOutput{Metric: "snmp_trap_test_state", Dimension: "state", Chart: "test_chart"},
		Missing: catalog.MetricMissingDrop,
		Scale:   catalog.MetricScale{Multiplier: 1, Divisor: 1},
	}
	chart := catalog.MetricChart{
		ID:        "test_chart",
		Title:     "Test chart",
		Context:   "snmp.trap.test.chart",
		Units:     "state",
		Algorithm: "absolute",
		Type:      "line",
		Lifecycle: &charttpl.Lifecycle{Dimensions: dimensions},
	}

	idx := newStaticTestCatalog([]*catalog.TrapDef{trap}, []catalog.MetricRule{rule}, []catalog.MetricChart{chart})

	enabled = false
	exists = false
	absent = true
	whereValues[0] = "changed"
	stateRange[0] = 99
	dimensions.MaxDims = 99
	inlineVarbind["name"] = "changed"
	inlineVarbind["enum"].(map[any]any)["1"] = "changed"
	labels["site"] = "changed"
	enum["1"] = "changed"

	defs, err := idx.Definitions([]string{"test.rule"})
	require.NoError(t, err)
	gotRule := defs.RulesByName["test.rule"]
	require.True(t, *gotRule.Enabled)
	require.Equal(t, "original", gotRule.Where[0].In[0])
	require.True(t, *gotRule.Where[1].Exists)
	require.Equal(t, 1, gotRule.State.SetWhen.Range[0])
	require.False(t, *gotRule.State.ClearWhen.Absent)
	require.Equal(t, 5, defs.ChartsByID["test_chart"].Lifecycle.Dimensions.MaxDims)

	gotTrap, err := idx.ResolveTrap(trap.OID)
	require.NoError(t, err)
	gotInline := gotTrap.VarbindRefs[0].(map[any]any)
	require.Equal(t, "inlineVarbind", gotInline["name"])
	require.Equal(t, "original", gotInline["enum"].(map[any]any)["1"])
	require.Equal(t, "original", gotTrap.Labels["site"])
	require.Equal(t, "original", gotTrap.SharedVarbinds["1.3.6.1.4.1.99999.2"].Enum["1"])

	replacementDimensions := &charttpl.DimensionLifecycle{MaxDims: 7, ExpireAfterCycles: 8}
	next := idx.withChartLifecycle("test_chart", charttpl.Lifecycle{Dimensions: replacementDimensions})
	replacementDimensions.MaxDims = 100

	nextDefs, err := next.Definitions([]string{"test.rule"})
	require.NoError(t, err)
	require.Equal(t, 7, nextDefs.ChartsByID["test_chart"].Lifecycle.Dimensions.MaxDims)
	require.Equal(t, 5, defs.ChartsByID["test_chart"].Lifecycle.Dimensions.MaxDims)
}
