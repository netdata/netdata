// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

func TestPlanRouteDiagnosticsRemainCompleteAcrossRepeatedPlans(t *testing.T) {
	var facts []PlanRouteDiagnostic
	engine, err := New(WithPlanRouteDiagnosticObserver(func(fact PlanRouteDiagnostic) {
		facts = append(facts, fact)
	}))
	require.NoError(t, err)
	require.NoError(t, engine.LoadYAML([]byte(`
version: v1
groups:
  - family: Test
    metrics: [app_value]
    charts:
      - title: Value
        context: app.value
        units: value
        instances:
          by_labels: [instance]
        dimensions:
          - selector: app_value{state="ready"}
            name: value
`), 1))

	store := metrix.NewCollectorStore()
	cycle := mustCycleController(t, store)
	meter := store.Write().SnapshotMeter("")
	cycle.BeginCycle()
	meter.Gauge("app_value").Observe(1, meter.LabelSet(
		metrix.Label{Key: "instance", Value: "node-a"},
		metrix.Label{Key: "state", Value: "ready"},
	))
	meter.Gauge("app_value").Observe(2, meter.LabelSet(
		metrix.Label{Key: "instance", Value: "node-b"},
		metrix.Label{Key: "state", Value: "starting"},
	))
	meter.Gauge("outside_value").Observe(3)
	require.NoError(t, cycle.CommitCycleSuccess())
	reader := store.Read(metrix.ReadRaw())

	for range 2 {
		facts = facts[:0]
		_, err := buildPlan(engine, reader)
		require.NoError(t, err)

		assert.Equal(t, 1, countPlanRouteDecisions(facts, PlanRouteAccepted))
		assert.Equal(t, 1, countPlanRouteDecisions(facts, PlanRouteCandidateSelectorRejected))
		assert.Equal(t, 2, countPlanRouteDecisions(facts, PlanRouteUnmatched))
	}

	stats := engine.stats()
	assert.Zero(t, stats.RouteCacheHits)
	assert.Zero(t, stats.RouteCacheMisses)
}

func TestPlanRouteDiagnosticsReportLifecycleCapRejection(t *testing.T) {
	var facts []PlanRouteDiagnostic
	engine, err := New(WithPlanRouteDiagnosticObserver(func(fact PlanRouteDiagnostic) {
		facts = append(facts, fact)
	}))
	require.NoError(t, err)
	require.NoError(t, engine.LoadYAML([]byte(`
version: v1
groups:
  - family: Test
    metrics: [svc_mode]
    charts:
      - id: mode
        title: Mode
        context: svc.mode
        units: state
        lifecycle:
          dimensions:
            max_dims: 1
        dimensions:
          - selector: svc_mode
            name_from_label: mode
`), 1))

	store := metrix.NewCollectorStore()
	cycle := mustCycleController(t, store)
	meter := store.Write().SnapshotMeter("")
	cycle.BeginCycle()
	meter.Gauge("svc_mode").Observe(1, meter.LabelSet(metrix.Label{Key: "mode", Value: "a"}))
	meter.Gauge("svc_mode").Observe(1, meter.LabelSet(metrix.Label{Key: "mode", Value: "z"}))
	require.NoError(t, cycle.CommitCycleSuccess())

	_, err = buildPlan(engine, store.Read(metrix.ReadRaw()))
	require.NoError(t, err)
	require.Equal(t, 1, countPlanRouteDecisions(facts, PlanRouteLifecycleRejected))
	for _, fact := range facts {
		if fact.Decision != PlanRouteLifecycleRejected {
			continue
		}
		assert.Equal(t, PlanRouteReasonDimensionCap, fact.Reason)
		assert.Equal(t, "z", fact.DimensionName)
	}
}

func TestPlanRouteDiagnosticsPreserveRawInstanceIdentity(t *testing.T) {
	var facts []PlanRouteDiagnostic
	engine, err := New(WithPlanRouteDiagnosticObserver(func(fact PlanRouteDiagnostic) {
		facts = append(facts, fact)
	}))
	require.NoError(t, err)
	require.NoError(t, engine.LoadYAML([]byte(`
version: v1
groups:
  - family: Test
    metrics: [app_value]
    charts:
      - id: value
        title: Value
        context: app.value
        units: value
        instances:
          by_labels: [instance]
        dimensions:
          - selector: app_value
            name: value
`), 1))

	store := metrix.NewCollectorStore()
	cycle := mustCycleController(t, store)
	meter := store.Write().SnapshotMeter("")
	cycle.BeginCycle()
	meter.Gauge("app_value").Observe(1, meter.LabelSet(metrix.Label{Key: "instance", Value: "a b"}))
	meter.Gauge("app_value").Observe(2, meter.LabelSet(metrix.Label{Key: "instance", Value: "a_b"}))
	meter.Gauge("app_value").Observe(3)
	require.NoError(t, cycle.CommitCycleSuccess())

	_, err = buildPlan(engine, store.Read(metrix.ReadRaw()))
	require.NoError(t, err)
	var resolved []PlanRouteDiagnostic
	var rejected []PlanRouteDiagnostic
	for _, fact := range facts {
		switch fact.Decision {
		case PlanRouteResolved:
			resolved = append(resolved, fact)
		case PlanRouteChartIdentityRejected:
			rejected = append(rejected, fact)
		}
	}
	require.Len(t, resolved, 2)
	assert.Equal(t, resolved[0].ChartID, resolved[1].ChartID)
	assert.NotEqual(t, resolved[0].InstanceIdentity, resolved[1].InstanceIdentity)
	require.Len(t, rejected, 1)
	assert.Equal(t, []string{"instance"}, rejected[0].MissingInstanceLabels)
}

func TestPlanRouteDiagnosticsExposeResolvedDimensionKeyLabel(t *testing.T) {
	var facts []PlanRouteDiagnostic
	engine, err := New(WithPlanRouteDiagnosticObserver(func(fact PlanRouteDiagnostic) {
		facts = append(facts, fact)
	}))
	require.NoError(t, err)
	require.NoError(t, engine.LoadYAML([]byte(`
version: v1
groups:
  - family: Service
    metrics: [system.status]
    charts:
      - title: System status
        context: system_status
        units: state
        dimensions:
          - selector: system.status
`), 1))

	store := metrix.NewCollectorStore()
	cycle := mustCycleController(t, store)
	stateSet := store.Write().SnapshotMeter("system").StateSet(
		"status",
		metrix.WithStateSetStates("ok", "failed"),
		metrix.WithStateSetMode(metrix.ModeEnum),
	)
	cycle.BeginCycle()
	stateSet.Enable("ok")
	require.NoError(t, cycle.CommitCycleSuccess())

	_, err = buildPlan(engine, store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)
	resolved := 0
	for _, fact := range facts {
		if fact.Decision != PlanRouteResolved {
			continue
		}
		resolved++
		assert.Equal(t, "system.status", fact.DimensionKeyLabel)
	}
	assert.Equal(t, 2, resolved)
}

func countPlanRouteDecisions(facts []PlanRouteDiagnostic, decision PlanRouteDecision) int {
	count := 0
	for _, fact := range facts {
		if fact.Decision == decision {
			count++
		}
	}
	return count
}
