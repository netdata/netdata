// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"fmt"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	metrixselector "github.com/netdata/netdata/go/plugins/pkg/metrix/selector"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testTemplateSet(t *testing.T, entries ...TemplateEntry) *TemplateSet {
	t.Helper()
	set, err := NewTemplateSet(TemplateSetSpec{
		Entries: entries,
	})
	require.NoError(t, err)
	return set
}

func templateCycle(t *testing.T, store metrix.CollectorStore, values map[string]float64) {
	t.Helper()
	cc := mustCycleController(t, store)
	cc.BeginCycle()
	for metric, value := range values {
		store.Write().SnapshotMeter("").Gauge(metric).Observe(value)
	}
	require.NoError(t, cc.CommitCycleSuccess())
}

func templateAttempt(t *testing.T, e *Engine, store metrix.CollectorStore, set *TemplateSet, values map[string]float64) PlanAttempt {
	t.Helper()
	templateCycle(t, store, values)
	attempt, err := e.PreparePlanWithOptions(store.Read(metrix.ReadFlatten()), PlanOptions{
		TemplateSet: set,
	})
	require.NoError(t, err)
	return attempt
}

func TestTemplateTransitionPreservesUnchangedAndReorderedEntries(t *testing.T) {
	e, err := New(WithRuntimeStore(nil))
	require.NoError(t, err)
	store := metrix.NewCollectorStore()
	a, b := nativeEntry("z", "shared", "requests"), nativeEntry("a", "shared", "requests")
	a.Groups[0].Charts[0].Lifecycle = &charttpl.Lifecycle{
		ExpireAfterCycles: 3,
	}
	first := testTemplateSet(t, a, b)
	attempt := templateAttempt(t, e, store, first, map[string]float64{"requests": 1})
	created := findCreateChartAction(attempt.Plan())
	require.NotNil(t, created)
	assert.Equal(t, first.program.Charts()[0].TemplateID, created.ChartTemplateID, "entry names must not determine precedence")
	require.NoError(t, attempt.Commit())

	reordered := testTemplateSet(t, b, a)
	attempt = templateAttempt(t, e, store, reordered, nil)
	assert.Empty(t, attempt.Plan().Actions, "quiet unchanged incumbents retain state")
	require.NoError(t, attempt.Commit())
	attempt = templateAttempt(t, e, store, reordered, map[string]float64{"requests": 2})
	assert.Nil(t, findCreateChartAction(attempt.Plan()), "reorder must not replace a live owner")
	require.NoError(t, attempt.Commit())
	// Expiry remains relative to the last observation, not snapshot adoption.
	for i := 0; i < 2; i++ {
		attempt = templateAttempt(t, e, store, reordered, nil)
		assert.Nil(t, findRemoveChartAction(attempt.Plan()))
		require.NoError(t, attempt.Commit())
	}
	attempt = templateAttempt(t, e, store, reordered, nil)
	require.NotNil(t, findRemoveChartAction(attempt.Plan()))
	require.NoError(t, attempt.Commit())
	attempt = templateAttempt(t, e, store, reordered, map[string]float64{"requests": 3})
	created = findCreateChartAction(attempt.Plan())
	require.NotNil(t, created)
	assert.Equal(t, reordered.program.Charts()[0].TemplateID, created.ChartTemplateID, "new unowned collision follows new order")
	require.NoError(t, attempt.Commit())
}

func TestTemplateTransitionReplacementRetiresOnlyOldIdentities(t *testing.T) {
	e, err := New(WithRuntimeStore(nil))
	require.NoError(t, err)
	store := metrix.NewCollectorStore()
	entry := nativeEntry("profile", "shared", "requests")
	entry.Groups[0].Charts[0].Dimensions = append(entry.Groups[0].Charts[0].Dimensions, charttpl.Dimension{
		Selector: "requests",
		Name:     "old",
	})
	quiet := nativeEntry("quiet", "quiet", "quiet_metric").Groups[0].Charts[0]
	entry.Groups[0].Metrics = append(entry.Groups[0].Metrics, "quiet_metric")
	entry.Groups[0].Charts = append(entry.Groups[0].Charts, quiet)
	initial := testTemplateSet(t, entry)
	attempt := templateAttempt(t, e, store, initial, map[string]float64{"requests": 1, "quiet_metric": 1})
	require.NoError(t, attempt.Commit())
	entry.Groups[0].Charts[0].Title = "Replacement"
	entry.Groups[0].Charts[0].Dimensions = entry.Groups[0].Charts[0].Dimensions[:1]
	next := testTemplateSet(t, entry)
	attempt = templateAttempt(t, e, store, next, map[string]float64{"requests": 2})
	created := findCreateChartActionByID(attempt.Plan(), "shared")
	require.NotNil(t, created)
	assert.Equal(t, "Replacement", created.Meta.Title)
	var removedCharts, removedDims []string
	for _, action := range attempt.Plan().Actions {
		switch a := action.(type) {
		case RemoveChartAction:
			removedCharts = append(removedCharts, a.ChartID)
		case RemoveDimensionAction:
			removedDims = append(removedDims, a.Name)
			assert.Equal(t, "Replacement", a.ChartMeta.Title, "retirement headers use final metadata")
		}
	}
	assert.Equal(t, []string{"quiet"}, removedCharts)
	assert.Equal(t, []string{"old"}, removedDims)
	require.NoError(t, attempt.Commit())
}

func TestTemplateTransitionAbortAndSkipCandidate(t *testing.T) {
	e, err := New(WithRuntimeStore(nil))
	require.NoError(t, err)
	store := metrix.NewCollectorStore()
	initial := testTemplateSet(t, nativeEntry("profile", "old", "requests"))
	attempt := templateAttempt(t, e, store, initial, map[string]float64{"requests": 1})
	require.NoError(t, attempt.Commit())
	intermediate := testTemplateSet(t, nativeEntry("profile", "intermediate", "requests"))
	attempt = templateAttempt(t, e, store, intermediate, map[string]float64{"requests": 2})
	require.NotNil(t, findCreateChartActionByID(attempt.Plan(), "intermediate"))
	attempt.Abort()
	// Committed state and cache must still produce the old presentation.
	retry, err := e.PreparePlan(store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	assert.Nil(t, findCreateChartAction(retry.Plan()))
	update := findUpdateAction(retry.Plan())
	require.NotNil(t, update)
	assert.Equal(t, "old", update.ChartID)
	require.NoError(t, retry.Commit())
	latest := testTemplateSet(t, nativeEntry("profile", "latest", "requests"))
	attempt = templateAttempt(t, e, store, latest, map[string]float64{"requests": 3})
	require.NotNil(t, findCreateChartActionByID(attempt.Plan(), "latest"))
	remove := findRemoveChartAction(attempt.Plan())
	require.NotNil(t, remove)
	assert.Equal(t, "old", remove.ChartID)
	require.NoError(t, attempt.Commit())
}

func TestTemplateTransitionReleasesEveryReplacedOwnerBeforeRouting(t *testing.T) {
	e, err := New(WithRuntimeStore(nil))
	require.NoError(t, err)
	store := metrix.NewCollectorStore()
	entry := nativeEntry("profile", "first", "requests")
	sibling := entry.Groups[0].Charts[0]
	sibling.ID = "second"
	sibling.Context = "second"
	entry.Groups[0].Charts = append(entry.Groups[0].Charts, sibling)
	initial := testTemplateSet(t, entry)
	attempt := templateAttempt(t, e, store, initial, map[string]float64{"requests": 1})
	require.NoError(t, attempt.Commit())
	entry.Groups[0].Charts[0].ID = "second"
	entry.Groups[0].Charts[1].ID = "first"
	next := testTemplateSet(t, entry)
	attempt = templateAttempt(t, e, store, next, map[string]float64{"requests": 2})
	assert.NotNil(t, findCreateChartActionByID(attempt.Plan(), "first"))
	assert.NotNil(t, findCreateChartActionByID(attempt.Plan(), "second"))
	assert.Nil(t, findRemoveChartAction(attempt.Plan()), "both public identities survive the ownership swap")
	assert.Nil(t, findRemoveDimensionAction(attempt.Plan()))
	require.NoError(t, attempt.Commit())
}

func TestTemplateTransitionAuthoredDisplacesAutogen(t *testing.T) {
	e, err := New(WithRuntimeStore(nil))
	require.NoError(t, err)
	store := metrix.NewCollectorStore()
	policy := EnginePolicy{
		Autogen: &AutogenPolicy{
			Enabled: true,
		},
	}
	initial, err := NewTemplateSet(TemplateSetSpec{
		Policy: policy,
	})
	require.NoError(t, err)
	attempt := templateAttempt(t, e, store, initial, map[string]float64{"requests": 1})
	autogenerated := findCreateChartAction(attempt.Plan())
	require.NotNil(t, autogenerated)
	require.NoError(t, attempt.Commit())
	authored := nativeEntry("profile", autogenerated.ChartID, "requests")
	authored.Groups[0].Charts[0].Title = "Authored"
	next, err := NewTemplateSet(TemplateSetSpec{
		Policy:  policy,
		Entries: []TemplateEntry{authored},
	})
	require.NoError(t, err)
	attempt = templateAttempt(t, e, store, next, map[string]float64{"requests": 2})
	assert.Nil(t, findRemoveChartAction(attempt.Plan()))
	removed := findRemoveDimensionAction(attempt.Plan())
	require.NotNil(t, removed)
	assert.Equal(t, "requests", removed.Name)
	assert.Equal(t, "Authored", removed.ChartMeta.Title)
	require.NoError(t, attempt.Commit())
	// Removing authored content allows fallback to take the same ID again.
	attempt = templateAttempt(t, e, store, initial, map[string]float64{"requests": 3})
	assert.Nil(t, findRemoveChartAction(attempt.Plan()))
	removed = findRemoveDimensionAction(attempt.Plan())
	require.NotNil(t, removed)
	assert.Equal(t, "value", removed.Name)
	require.NoError(t, attempt.Commit())
}

func TestTemplateTransitionHostResetIsStaged(t *testing.T) {
	e, err := New(WithRuntimeStore(nil))
	require.NoError(t, err)
	store := metrix.NewCollectorStore()
	set := testTemplateSet(t, nativeEntry("profile", "requests", "requests"))
	attempt := templateAttempt(t, e, store, set, map[string]float64{"requests": 1})
	require.NoError(t, attempt.Commit())
	attempt, err = e.PreparePlanWithOptions(store.Read(metrix.ReadFlatten()), PlanOptions{
		ResetMaterialized: true,
	})
	require.NoError(t, err)
	require.NotNil(t, findCreateChartAction(attempt.Plan()))
	assert.Nil(t, findRemoveChartAction(attempt.Plan()))
	attempt.Abort()
	retry, err := e.PreparePlan(store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	assert.Nil(t, findCreateChartAction(retry.Plan()))
	require.NoError(t, retry.Commit())
	// An empty new host still commits the reset without old-host retirements.
	templateCycle(t, store, nil)
	attempt, err = e.PreparePlanWithOptions(store.Read(metrix.ReadFlatten()), PlanOptions{
		ResetMaterialized: true,
	})
	require.NoError(t, err)
	assert.Empty(t, attempt.Plan().Actions)
	require.NoError(t, attempt.Commit())
	attempt = templateAttempt(t, e, store, set, map[string]float64{"requests": 2})
	require.NotNil(t, findCreateChartAction(attempt.Plan()))
	require.NoError(t, attempt.Commit())
}

func TestTemplateTransitionEmptySetAndStaleAttempt(t *testing.T) {
	e, err := New(WithRuntimeStore(nil))
	require.NoError(t, err)
	store := metrix.NewCollectorStore()
	set := testTemplateSet(t, nativeEntry("profile", "requests", "requests"))
	attempt := templateAttempt(t, e, store, set, map[string]float64{"requests": 1})
	require.NoError(t, attempt.Commit())
	attempt = templateAttempt(t, e, store, testTemplateSet(t), nil)
	require.NotNil(t, findRemoveChartAction(attempt.Plan()))
	e.ResetMaterialized()
	assert.ErrorIs(t, attempt.Commit(), ErrStalePlanAttempt)
	// Reset invalidates only materialized state, not the previous program.
	attempt = templateAttempt(t, e, store, nil, map[string]float64{"requests": 2})
	require.NotNil(t, findCreateChartAction(attempt.Plan()))
	require.NoError(t, attempt.Commit())
	attempt = templateAttempt(t, e, store, testTemplateSet(t), nil)
	require.NoError(t, attempt.Commit())
	attempt = templateAttempt(t, e, store, nil, map[string]float64{"requests": 3})
	assert.Empty(t, attempt.Plan().Actions)
	require.NoError(t, attempt.Commit())
}

func findRemoveChartAction(plan Plan) *RemoveChartAction {
	for _, action := range plan.Actions {
		if remove, ok := action.(RemoveChartAction); ok {
			return &remove
		}
	}
	return nil
}

func TestTemplateSetDiagnosticProvenance(t *testing.T) {
	var facts []PlanRouteDiagnostic
	e, err := New(WithRuntimeStore(nil), WithPlanRouteDiagnosticObserver(func(fact PlanRouteDiagnostic) { facts = append(facts, fact) }))
	require.NoError(t, err)
	store := metrix.NewCollectorStore()
	entry := nativeEntry("profile", "requests", "requests")
	entry.AutogenRules = []charttpl.EngineAutogenRule{{Scope: "private_*", Selector: metrixselector.Expr{
		Deny: []string{"*"},
	}}}
	set, err := NewTemplateSet(TemplateSetSpec{
		Entries: []TemplateEntry{entry},
		Policy: EnginePolicy{
			Autogen: &AutogenPolicy{
				Enabled: true,
			},
		},
	})
	require.NoError(t, err)
	attempt := templateAttempt(t, e, store, set, map[string]float64{"requests": 1, "private_value": 1})
	require.NoError(t, attempt.Commit())
	accepted, restricted := false, false
	for _, fact := range facts {
		if fact.Decision == PlanRouteAccepted {
			accepted = true
			assert.Equal(t, "profile", fact.TemplateEntryID)
			assert.Equal(t, "g0.c0", fact.LocalChartTemplateID)
		}
		if fact.Reason == PlanRouteReasonAutogenRuleRejected {
			restricted = true
			assert.Equal(t, "profile", fact.AutogenRuleEntryID)
			assert.Equal(t, "private_*", fact.AutogenRuleScope)
			assert.Zero(t, fact.AutogenRuleLocalIndex)
		}
	}
	assert.True(t, accepted)
	assert.True(t, restricted)
}

func TestTemplateSetVirtualDocumentPrecedence(t *testing.T) {
	entries := []TemplateEntry{{ID: "base", Groups: []charttpl.Group{{Family: "Base"}}}}
	for i := 1; i <= 10; i++ {
		metric := "inactive"
		if i >= 9 {
			metric = "requests"
		}
		entry := nativeEntry(fmt.Sprintf("profile%d", i), "shared", metric)
		entry.Groups[0].Charts[0].Title = entry.ID
		entries = append(entries, entry)
	}
	e, err := New(WithRuntimeStore(nil))
	require.NoError(t, err)
	store := metrix.NewCollectorStore()
	attempt := templateAttempt(t, e, store, testTemplateSet(t, entries...), map[string]float64{"requests": 1})
	chart := findCreateChartAction(attempt.Plan())
	require.NotNil(t, chart)
	assert.Equal(t, "profile10", chart.Meta.Title, "g10 precedes g9 lexically, as in the virtual document")
	require.NoError(t, attempt.Commit())
}

func TestTemplateTransferDoesNotInheritOutgoingDimensionCapIncumbents(t *testing.T) {
	e, err := New(WithRuntimeStore(nil))
	require.NoError(t, err)
	store := metrix.NewCollectorStore()
	policy := EnginePolicy{
		Autogen: &AutogenPolicy{
			Enabled: true,
		},
	}
	empty, err := NewTemplateSet(TemplateSetSpec{
		Policy: policy,
	})
	require.NoError(t, err)
	attempt := templateAttempt(t, e, store, empty, map[string]float64{"requests": 1})
	chart := findCreateChartAction(attempt.Plan())
	require.NotNil(t, chart)
	require.NoError(t, attempt.Commit())
	entry := nativeEntry("profile", chart.ChartID, "requests")
	entry.Groups[0].Charts[0].Dimensions = []charttpl.Dimension{{Selector: "requests", Name: "a"}, {Selector: "requests", Name: "requests"}}
	entry.Groups[0].Charts[0].Lifecycle = &charttpl.Lifecycle{
		Dimensions: &charttpl.DimensionLifecycle{
			MaxDims: 1,
		},
	}
	next, err := NewTemplateSet(TemplateSetSpec{
		Entries: []TemplateEntry{entry},
		Policy:  policy,
	})
	require.NoError(t, err)
	attempt = templateAttempt(t, e, store, next, map[string]float64{"requests": 2})
	update := findUpdateAction(attempt.Plan())
	require.NotNil(t, update)
	require.Len(t, update.Values, 1)
	assert.Equal(t, "a", update.Values[0].Name, "outgoing owner cannot affect the new owner's dimension cap")
	require.NoError(t, attempt.Commit())
}
