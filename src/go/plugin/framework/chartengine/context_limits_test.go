// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"bytes"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const contextLimitYAML = `
version: v1
groups:
  - family: Work
    metrics: [work]
    charts:
      - id: detail
        title: Detail
        context: work.detail
        units: tasks
        lifecycle: {expire_after_cycles: 0}
        dimensions:
          - selector: work
            name_from_label: client
      - id: model
        title: Model
        context: work.model
        units: tasks
        instances: {by_labels: [model]}
        dimensions:
          - selector: work
            name: tasks
      - id: team
        title: Team
        context: work.team
        units: tasks
        instances: {by_labels: [team]}
        dimensions:
          - selector: work
            name: tasks
`

func newContextLimitEngine(t testing.TB, limit int, template string, opts ...Option) *Engine {
	t.Helper()
	opts = append(opts, WithRuntimeStore(nil), WithRuntimePlannerMode(), WithEnginePolicy(EnginePolicy{
		MaxOutputSeriesPerContext: &limit,
	}))
	e, err := New(opts...)
	require.NoError(t, err)
	require.NoError(t, e.LoadYAML([]byte(template), 1))
	return e
}

func contextLimitCycle(t testing.TB, store metrix.CollectorStore, n, offset int, value float64) metrix.Reader {
	t.Helper()
	managed, ok := metrix.AsCycleManagedStore(store)
	require.True(t, ok)
	cc := managed.CycleController()
	metric := store.Write().SnapshotMeter("").Vec("client", "model", "team").Gauge("work", metrix.WithFloat(true))
	cc.BeginCycle()
	for i := range n {
		metric.WithLabelValues(fmt.Sprint(i+offset), fmt.Sprint(i%2), fmt.Sprint(i%3)).Observe(value)
	}
	require.NoError(t, cc.CommitCycleSuccess())
	return store.Read(metrix.ReadFlatten())
}

func contextLimitUpdates(plan Plan) map[string]UpdateChartAction {
	out := make(map[string]UpdateChartAction)
	for _, a := range plan.Actions {
		if update, ok := a.(UpdateChartAction); ok {
			out[update.ChartID] = update
		}
	}
	return out
}

func TestContextLimitKeepsCompleteRollupsAndRecovers(t *testing.T) {
	var logs bytes.Buffer
	e := newContextLimitEngine(t, 3, contextLimitYAML, WithLogger(logger.NewWithWriter(&logs)))
	store := metrix.NewCollectorStore()
	for _, n := range []int{600, 3, 600, 3} {
		logs.Reset()
		attempt, err := e.PreparePlan(contextLimitCycle(t, store, n, 0, 3))
		require.NoError(t, err)
		assert.Empty(t, logs.String(), "output transitions must wait for commit")
		updates := contextLimitUpdates(attempt.Plan())
		_, detail := updates["detail"]
		assert.Equal(t, n == 3, detail)
		var modelTotal, teamTotal float64
		for id, update := range updates {
			for _, v := range update.Values {
				if strings.HasPrefix(id, "model") {
					modelTotal += v.Float64
				}
				if strings.HasPrefix(id, "team") {
					teamTotal += v.Float64
				}
			}
		}
		assert.Equal(t, float64(n*3), modelTotal)
		assert.Equal(t, float64(n*3), teamTotal)
		require.NoError(t, attempt.Commit())
		if n == 600 {
			assert.Contains(t, logs.String(), "newly omitted=1")
		} else {
			assert.Contains(t, logs.String(), "no longer omitted=1")
		}
		assert.Equal(t, 1, strings.Count(logs.String(), "output limit per context"))
	}
	assert.Nil(t, e.state.cfg.routeObserver, "ordinary reporting must retain route caching")
}

func TestContextLimitSharedContextBeforeLifecycleCaps(t *testing.T) {
	template := strings.ReplaceAll(contextLimitYAML, "context: work.team", "context: work.model")
	// Five outputs in a shared context must fail even if each chart has one
	// dimension and a template lifecycle cap would subsequently prune instances.
	template = strings.ReplaceAll(template, "instances: {by_labels: [model]}", "instances: {by_labels: [model]}\n        lifecycle: {max_instances: 1}")
	for _, limit := range []int{0, 4, 5} {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			var facts []PlanRouteDiagnostic
			e := newContextLimitEngine(t, limit, template, WithPlanRouteDiagnosticObserver(func(f PlanRouteDiagnostic) {
				if f.Decision == PlanRouteContextRejected {
					facts = append(facts, f)
				}
			}))
			store := metrix.NewCollectorStore()
			plan, err := buildPlan(e, contextLimitCycle(t, store, 3, 0, 1))
			require.NoError(t, err)
			updates := contextLimitUpdates(plan)
			assert.Contains(t, updates, "detail")
			if limit == 4 {
				assert.Len(t, updates, 1)
				require.Len(t, facts, 5)
				for _, fact := range facts {
					assert.Equal(t, 5, fact.ContextSeriesCount)
					assert.Equal(t, 4, fact.ContextSeriesLimit)
					assert.Equal(t, "work.model", fact.Context)
				}
			} else {
				assert.Greater(t, len(updates), 1)
				assert.Empty(t, facts)
			}
		})
	}
}

func TestContextLimitAbortChurnAndQuietHistory(t *testing.T) {
	e := newContextLimitEngine(t, 3, contextLimitYAML)
	store := metrix.NewCollectorStore(metrix.WithExpireAfterSuccessCycles(2))
	_, err := buildPlan(e, contextLimitCycle(t, store, 3, 0, 1))
	require.NoError(t, err)
	chart := e.state.materialized.charts["detail"]
	require.NotNil(t, chart)
	initialSeen := chart.lastSeenSuccessSeq
	initialPresentation := chart.presentation
	initialEntries := chart.scratchEntries
	for cycle := range 80 {
		reader := contextLimitCycle(t, store, 64, 1000+cycle*64, 2)
		attempt, err := e.PreparePlan(reader)
		require.NoError(t, err)
		assert.Nil(t, chart.scratchEntries)
		if cycle == 0 {
			attempt.Abort()
			assert.Equal(t, initialEntries, chart.scratchEntries)
			assert.Len(t, chart.scratchEntries, 3)
			assert.Empty(t, e.state.outputContextCounts["never present"])
			assert.Equal(t, 3, e.state.outputContextCounts["work.detail"])
			attempt, err = e.PreparePlan(reader)
			require.NoError(t, err)
		}
		require.NoError(t, attempt.Commit())
		assert.Len(t, chart.dimensions, 3)
		assert.Nil(t, chart.scratchEntries)
		assert.Equal(t, initialSeen, chart.lastSeenSuccessSeq)
		assert.Same(t, initialPresentation, chart.presentation)
	}
	// Quiet historical dimensions are not newly observed output. A different
	// three-dimensional population fits, despite three retained old dimensions.
	plan, err := buildPlan(e, contextLimitCycle(t, store, 3, 99999, 0))
	require.NoError(t, err)
	require.Contains(t, contextLimitUpdates(plan), "detail")
	assert.Equal(t, 3, e.state.outputContextCounts["work.detail"])
}

func TestContextLimitReportingCommitAbsenceFailureAndReset(t *testing.T) {
	var logs bytes.Buffer
	e := newContextLimitEngine(t, 3, contextLimitYAML, WithLogger(logger.NewWithWriter(&logs)))
	store := metrix.NewCollectorStore()
	reader := contextLimitCycle(t, store, 4, 0, 1)
	logs.Reset()
	attempt, err := e.PreparePlan(reader)
	require.NoError(t, err)
	attempt.Abort()
	assert.Empty(t, logs.String())
	_, err = buildPlan(e, reader)
	require.NoError(t, err)
	assert.Contains(t, logs.String(), "newly omitted=1")
	logs.Reset()
	_, err = buildPlan(e, contextLimitCycle(t, store, 5, 0, 1))
	require.NoError(t, err)
	assert.Empty(t, logs.String(), "changing count alone is silent")
	managed, _ := metrix.AsCycleManagedStore(store)
	managed.CycleController().BeginCycle()
	managed.CycleController().AbortCycle()
	_, err = buildPlan(e, store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	assert.Empty(t, logs.String())
	assert.Equal(t, 5, e.state.outputContextCounts["work.detail"])
	// Recovery is reported only after the admitted context is committed.
	reader = contextLimitCycle(t, store, 3, 0, 1)
	attempt, err = e.PreparePlan(reader)
	require.NoError(t, err)
	attempt.Abort()
	assert.Empty(t, logs.String())
	plan, err := buildPlan(e, reader)
	require.NoError(t, err)
	assert.Contains(t, logs.String(), "no longer omitted=1")
	assert.Contains(t, contextLimitUpdates(plan), "detail")
	_, err = buildPlan(e, contextLimitCycle(t, store, 4, 0, 1))
	require.NoError(t, err)
	logs.Reset()
	_, err = buildPlan(e, contextLimitCycle(t, store, 0, 0, 0))
	require.NoError(t, err)
	assert.Empty(t, logs.String(), "disappearance is not recovery")
	assert.Empty(t, e.state.outputContextCounts)
	_, err = buildPlan(e, contextLimitCycle(t, store, 4, 0, 1))
	require.NoError(t, err)
	logs.Reset()
	attempt, err = e.PreparePlanWithOptions(contextLimitCycle(t, store, 3, 0, 1), PlanOptions{
		ResetMaterialized: true,
	})
	require.NoError(t, err)
	require.NoError(t, attempt.Commit())
	assert.Empty(t, logs.String(), "new host does not recover a previous host")
}

func TestContextLimitPolicyPropagation(t *testing.T) {
	negative := -1
	_, err := New(WithEnginePolicy(EnginePolicy{
		MaxOutputSeriesPerContext: &negative,
	}))
	require.Error(t, err)
	spec, err := charttpl.DecodeYAML([]byte(contextLimitYAML))
	require.NoError(t, err)
	limit := 3
	native, err := NewTemplateSet(TemplateSetSpec{
		Entries: []TemplateEntry{{ID: "work", Groups: spec.Groups}},
		Policy: EnginePolicy{
			MaxOutputSeriesPerContext: &limit,
		},
	})
	require.NoError(t, err)
	legacy, err := NewTemplateSetYAML([]byte(contextLimitYAML))
	require.NoError(t, err)
	for _, input := range []*TemplateSet{native, legacy} {
		option := WithEnginePolicy(EnginePolicy{
			MaxOutputSeriesPerContext: &limit,
		})
		limit = 99 // the option owns its policy
		set, err := PrepareTemplateSet(input, option)
		require.NoError(t, err)
		assert.Equal(t, 3, *set.GlobalPolicy().MaxOutputSeriesPerContext)
		policy := set.GlobalPolicy()
		*policy.MaxOutputSeriesPerContext = 50
		assert.Equal(t, 3, *set.GlobalPolicy().MaxOutputSeriesPerContext)
		e, err := New(WithRuntimeStore(nil))
		require.NoError(t, err)
		store := metrix.NewCollectorStore()
		reader := contextLimitCycle(t, store, 4, 0, 1)
		attempt, err := e.PreparePlanWithOptions(reader, PlanOptions{
			TemplateSet: set,
		})
		require.NoError(t, err)
		assert.NotContains(t, contextLimitUpdates(attempt.Plan()), "detail")
		require.NoError(t, attempt.Commit())
		zero := 0
		disabled, err := PrepareTemplateSet(set, WithEnginePolicy(EnginePolicy{
			MaxOutputSeriesPerContext: &zero,
		}))
		require.NoError(t, err)
		assert.False(t, disabled.SameGlobalPolicy(set))
		_, err = e.PreparePlanWithOptions(reader, PlanOptions{
			TemplateSet: disabled,
		})
		require.ErrorContains(t, err, "fixed global policy")
		limit = 3
	}
}

func TestContextLimitDistributionsHiddenAndEmptyRecovery(t *testing.T) {
	const template = `
version: v1
groups:
  - family: Latency
    metrics: [lat_bucket, lat_count, lat_sum, request]
    charts:
      - id: buckets
        title: Buckets
        context: latency
        units: observations
        dimensions:
          - selector: lat_bucket
      - id: count
        title: Count
        context: latency
        units: observations
        dimensions:
          - selector: lat_count
            name: count
            options: {hidden: true}
      - id: sum
        title: Sum
        context: latency_sum
        units: seconds
        dimensions:
          - selector: lat_sum
            name: sum
      - id: quantiles
        title: Quantiles
        context: quantiles
        units: seconds
        instances: {by_labels: [client]}
        dimensions:
          - selector: request{quantile=~".+"}
`
	var logs bytes.Buffer
	e := newContextLimitEngine(t, 3, template, WithLogger(logger.NewWithWriter(&logs)))
	store := metrix.NewCollectorStore()
	cc := mustCycleController(t, store)
	meter := store.Write().SnapshotMeter("")
	hist := meter.Histogram("lat", metrix.WithHistogramBounds(1, 2))
	summary := meter.Vec("client").Summary("request", metrix.WithSummaryQuantiles(.5, .9))
	for _, clients := range []int{2, 1} {
		cc.BeginCycle()
		hist.ObservePoint(metrix.HistogramPoint{
			Count:   3,
			Sum:     5,
			Buckets: []metrix.BucketPoint{{UpperBound: 1, CumulativeCount: 1}, {UpperBound: 2, CumulativeCount: 2}},
		})
		for i := range clients {
			summary.WithLabelValues(fmt.Sprint(i)).ObservePoint(metrix.SummaryPoint{
				Count:     0,
				Sum:       0,
				Quantiles: []metrix.QuantilePoint{{Quantile: .5, Value: math.NaN()}, {Quantile: .9, Value: math.NaN()}},
			})
		}
		require.NoError(t, cc.CommitCycleSuccess())
		logs.Reset()
		plan, err := buildPlan(e, store.Read(metrix.ReadFlatten()))
		require.NoError(t, err)
		updates := contextLimitUpdates(plan)
		assert.NotContains(t, updates, "buckets", "three buckets including +Inf plus one hidden count exceed limit")
		assert.NotContains(t, updates, "count")
		assert.Contains(t, updates, "sum", "different histogram context remains complete")
		assert.Equal(t, 4, e.state.outputContextCounts["latency"])
		assert.Equal(t, clients*2, e.state.outputContextCounts["quantiles"])
		if clients == 1 {
			assert.Contains(t, logs.String(), "no longer omitted=1")
			assert.NotContains(t, logs.String(), "healthy")
			for id, update := range updates {
				if strings.HasPrefix(id, "quantiles") {
					for _, value := range update.Values {
						assert.True(t, value.IsEmpty)
					}
				}
			}
		} else {
			assert.Len(t, updates, 1)
		}
	}
}

func TestContextLimitBoundedLogsAndScopeIsolation(t *testing.T) {
	var template strings.Builder
	template.WriteString("version: v1\ngroups:\n  - family: Many\n    metrics: [work]\n    charts:\n")
	for i := range 80 {
		fmt.Fprintf(&template, "      - id: c%d\n        title: Work\n        context: context_%03d\n        units: tasks\n        dimensions:\n          - selector: work\n            name_from_label: client\n", i, i)
	}
	var logs bytes.Buffer
	shared := logger.NewWithWriter(&logs)
	first := newContextLimitEngine(t, 3, template.String(), WithLogger(shared))
	second := newContextLimitEngine(t, 3, template.String(), WithLogger(shared))
	store := metrix.NewCollectorStore()
	logs.Reset()
	_, err := buildPlan(first, contextLimitCycle(t, store, 4, 0, 1))
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(logs.String(), "output limit per context"))
	assert.Contains(t, logs.String(), "newly omitted=80")
	assert.NotContains(t, logs.String(), "context_003")
	assert.Less(t, logs.Len(), 1000)
	logs.Reset()
	_, err = buildPlan(second, contextLimitCycle(t, store, 3, 0, 1))
	require.NoError(t, err)
	assert.Empty(t, logs.String(), "scope with no prior rejection must not report recovery")
	_, err = buildPlan(first, store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	assert.Contains(t, logs.String(), "no longer omitted=80")
	assert.Equal(t, 1, strings.Count(logs.String(), "output limit per context"))
}

func TestContextLimitCountsDuplicateRoutesAfterOverflow(t *testing.T) {
	template := strings.Replace(contextLimitYAML, "            name_from_label: client", "            name_from_label: client\n          - selector: work\n            name_from_label: client", 1)
	e := newContextLimitEngine(t, 3, template)
	store := metrix.NewCollectorStore()
	for _, n := range []int{100, 3, 100, 3} {
		plan, err := buildPlan(e, contextLimitCycle(t, store, n, 0, 1))
		require.NoError(t, err)
		assert.Equal(t, n, e.state.outputContextCounts["work.detail"])
		if n == 3 {
			require.Contains(t, contextLimitUpdates(plan), "detail")
			for _, value := range contextLimitUpdates(plan)["detail"].Values {
				assert.Equal(t, float64(2), value.Float64)
			}
		} else {
			assert.NotContains(t, contextLimitUpdates(plan), "detail")
		}
	}
}

func TestContextLimitRejectedOwnerReplacementDiscardsBorrowedScratch(t *testing.T) {
	const template = `
version: v1
engine:
  autogen: {enabled: true}
groups:
  - family: Work
    metrics: [zzz]
    charts:
      - id: aaa
        title: Authored
        context: authored
        units: work
        dimensions:
          - selector: zzz
            name_from_label: client
`
	e := newContextLimitEngine(t, 3, template)
	store := metrix.NewCollectorStore()
	cc := mustCycleController(t, store)
	meter := store.Write().SnapshotMeter("")
	auto := meter.Gauge("aaa")
	authored := meter.Vec("client").Gauge("zzz")
	cycle := func(n int) metrix.Reader {
		cc.BeginCycle()
		auto.Observe(float64(n + 1))
		for i := range n {
			authored.WithLabelValues(fmt.Sprint(i)).Observe(2)
		}
		require.NoError(t, cc.CommitCycleSuccess())
		return store.Read(metrix.ReadFlatten())
	}
	_, err := buildPlan(e, cycle(0))
	require.NoError(t, err)
	previous := snapshotMaterialized(e.state.materialized)
	owner := e.state.materialized.charts["aaa"]
	require.NotNil(t, owner)
	reader := cycle(4)
	attempt, err := e.PreparePlan(reader)
	require.NoError(t, err)
	assert.Empty(t, contextLimitUpdates(attempt.Plan()))
	attempt.Abort()
	assert.Equal(t, previous, snapshotMaterialized(e.state.materialized))
	attempt, err = e.PreparePlan(reader)
	require.NoError(t, err)
	require.NoError(t, attempt.Commit())
	assert.Nil(t, owner.scratchEntries)
	assert.Equal(t, map[string]int{"authored": 4}, e.state.outputContextCounts)
	plan, err := buildPlan(e, cycle(3))
	require.NoError(t, err)
	assert.Contains(t, contextLimitUpdates(plan), "aaa")
	assert.Equal(t, "authored", e.state.materialized.charts["aaa"].meta.Context)
}

func TestContextLimitRecoveryRebuildsLabelsAndMembership(t *testing.T) {
	for _, diagnostics := range []bool{false, true} {
		t.Run(fmt.Sprint(diagnostics), func(t *testing.T) {
			var opts []Option
			if diagnostics {
				opts = append(opts, WithPlanRouteDiagnosticObserver(func(PlanRouteDiagnostic) {}))
			}
			e := newContextLimitEngine(t, 3, contextLimitYAML, opts...)
			store := metrix.NewCollectorStore()
			cc := mustCycleController(t, store)
			gauge := store.Write().SnapshotMeter("").Vec("client", "zone").Gauge("work")
			cycle := func(n int, zone string) metrix.Reader {
				cc.BeginCycle()
				for i := range n {
					gauge.WithLabelValues(fmt.Sprint(i), zone).Observe(1)
				}
				require.NoError(t, cc.CommitCycleSuccess())
				return store.Read(metrix.ReadFlatten())
			}
			_, err := buildPlan(e, cycle(2, "old"))
			require.NoError(t, err)
			_, err = buildPlan(e, cycle(6, "omitted"))
			require.NoError(t, err)
			_, err = buildPlan(e, cycle(5, "still omitted"))
			require.NoError(t, err)
			reader := newCountingMutableLabelReader(t, cycle(2, "new"))
			attempt, err := e.PreparePlan(reader)
			require.NoError(t, err)
			assert.Equal(t, 2, reader.iterations, "recovery replays labels after admission")
			aborted := attempt.Plan()
			attempt.Abort()
			assert.Equal(t, "old", e.state.materialized.charts["detail"].presentation.labelValues["zone"])
			plan, err := buildPlan(e, reader)
			require.NoError(t, err)
			assert.Equal(t, aborted, plan)
			assert.Equal(t, "new", e.state.materialized.charts["detail"].presentation.labelValues["zone"])
			assert.Len(t, e.state.materialized.charts["detail"].presentation.labelMembership, 2)
			plan, err = buildPlan(e, cycle(2, "new"))
			require.NoError(t, err)
			for _, action := range plan.Actions {
				_, changed := action.(UpdateChartLabelsAction)
				assert.False(t, changed, "rebuilt membership must be reusable on the next cycle")
			}
		})
	}
}

func TestContextLimitRejectedDetailAllocationEnvelope(t *testing.T) {
	e := newContextLimitEngine(t, 3, contextLimitYAML)
	store := metrix.NewCollectorStore()
	reader := contextLimitCycle(t, store, 10000, 0, 1)
	_, err := buildPlan(e, reader)
	require.NoError(t, err)
	allocs := testing.AllocsPerRun(10, func() {
		_, err = buildPlan(e, reader)
		if err != nil {
			t.Fatal(err)
		}
	})
	// Map growth needs a small number of allocations. Rejected dimensions must
	// not allocate one value/label accumulator each on every collection.
	assert.LessOrEqual(t, allocs, float64(200))
}
