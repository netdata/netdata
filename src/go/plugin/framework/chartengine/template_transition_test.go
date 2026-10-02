// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"fmt"
	"slices"
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

// collisionEntries returns one entry per ID; the listed colliding IDs render the
// same "shared" chart from the "requests" series and are titled by their ID.
func collisionEntries(ids []string, colliding ...string) []TemplateEntry {
	entries := make([]TemplateEntry, 0, len(ids))
	for _, id := range ids {
		entry := nativeEntry(id, id, "inactive")
		if slices.Contains(colliding, id) {
			entry = nativeEntry(id, "shared", "requests")
		}
		entry.Groups[0].Charts[0].Title = id
		entries = append(entries, entry)
	}
	return entries
}

func numberedIDs(prefix string, n int) []string {
	ids := make([]string, n)
	for i := range ids {
		ids[i] = fmt.Sprintf("%s%d", prefix, i)
	}
	return ids
}

// collisionWinner plans one fresh cycle with no incumbent and returns the title
// of the chart created for the contested chart ID.
func collisionWinner(t *testing.T, set *TemplateSet) string {
	t.Helper()
	e, err := New(WithRuntimeStore(nil))
	require.NoError(t, err)
	store := metrix.NewCollectorStore()
	attempt := templateAttempt(t, e, store, set, map[string]float64{"requests": 1})
	chart := findCreateChartAction(attempt.Plan())
	require.NotNil(t, chart)
	require.NoError(t, attempt.Commit())
	return chart.Meta.Title
}

func TestTemplateSetCollisionPrecedenceFollowsCompileOrder(t *testing.T) {
	sharedChart := func(title string) charttpl.Chart {
		return charttpl.Chart{
			ID:         "shared",
			Title:      title,
			Context:    "shared",
			Units:      "requests",
			Dimensions: []charttpl.Dimension{{Selector: "requests", Name: "value"}},
		}
	}
	nestedEntry := TemplateEntry{
		ID: "nested",
		Groups: []charttpl.Group{{
			Family:  "Parent",
			Metrics: []string{"requests"},
			Charts:  []charttpl.Chart{sharedChart("parent")},
			Groups:  []charttpl.Group{{Family: "Child", Charts: []charttpl.Chart{sharedChart("child")}}},
		}},
	}
	manyCharts := TemplateEntry{
		ID:     "many",
		Groups: []charttpl.Group{{Family: "Many", Metrics: []string{"requests", "inactive"}}},
	}
	for i := range 11 {
		chart := sharedChart(fmt.Sprintf("chart%d", i))
		if i != 2 && i != 10 {
			chart.ID, chart.Context = chart.Title, chart.Title
			chart.Dimensions[0].Selector = "inactive"
		}
		manyCharts.Groups[0].Charts = append(manyCharts.Groups[0].Charts, chart)
	}
	withoutLead := append([]string{"p0", "a"}, append(numberedIDs("p", 9)[2:], "b")...)

	tests := map[string]struct {
		entries []TemplateEntry
		want    string
	}{
		"earlier entry beats a double-digit position": {
			entries: collisionEntries(numberedIDs("profile", 11), "profile2", "profile10"),
			want:    "profile2",
		},
		"unrelated entry order is stable": {
			entries: collisionEntries(withoutLead, "a", "b"),
			want:    "a",
		},
		"inserting an unrelated entry keeps the winner": {
			entries: collisionEntries(append([]string{"lead"}, withoutLead...), "a", "b"),
			want:    "a",
		},
		"group charts precede nested groups": {
			entries: []TemplateEntry{nestedEntry},
			want:    "parent",
		},
		"authored chart order within a group": {
			entries: []TemplateEntry{manyCharts},
			want:    "chart2",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, collisionWinner(t, testTemplateSet(t, tc.entries...)))
		})
	}
}

// Compile order ranks the routes of one series. Different series contend in scan
// order, so the series with the first metric name claims the chart whatever the
// entry order.
func TestTemplateSetCrossSeriesCollisionFollowsScanOrder(t *testing.T) {
	first, second := nativeEntry("first", "shared", "zzz"), nativeEntry("second", "shared", "aaa")
	first.Groups[0].Charts[0].Title = first.ID
	second.Groups[0].Charts[0].Title = second.ID
	e, err := New(WithRuntimeStore(nil))
	require.NoError(t, err)
	store := metrix.NewCollectorStore()
	attempt := templateAttempt(t, e, store, testTemplateSet(t, first, second), map[string]float64{"zzz": 1, "aaa": 1})
	chart := findCreateChartAction(attempt.Plan())
	require.NotNil(t, chart)
	require.NoError(t, attempt.Commit())

	assert.Equal(t, "second", chart.Meta.Title)
}

// YAML documents are shipped collector contracts: their unowned collisions keep
// comparing positional template IDs as strings.
func TestTemplateSetDocumentCollisionPrecedenceIsLexical(t *testing.T) {
	spec := charttpl.Spec{
		Version: charttpl.VersionV1,
	}
	for _, entry := range collisionEntries(numberedIDs("group", 11), "group2", "group10") {
		spec.Groups = append(spec.Groups, entry.Groups[0])
	}
	data, err := spec.MarshalTemplate()
	require.NoError(t, err)
	set, err := NewTemplateSetYAML([]byte(data))
	require.NoError(t, err)

	assert.Equal(t, "group10", collisionWinner(t, set), "g10.c0 sorts before g2.c0")
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
