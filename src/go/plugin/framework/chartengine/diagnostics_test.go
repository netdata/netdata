// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

func TestPlanRouteDiagnosticsDoNotChangeChartIdentityRejection(t *testing.T) {
	template := []byte(`
version: v1
groups:
  - family: Test
    metrics: [app_state]
    charts:
      - title: State
        context: app.state
        units: state
        instances:
          by_labels: [instance]
        dimensions:
          - selector: app_state
`)

	store := metrix.NewCollectorStore()
	cycle := mustCycleController(t, store)
	cycle.BeginCycle()
	store.Write().SnapshotMeter("").Gauge("app_state").Observe(1)
	require.NoError(t, cycle.CommitCycleSuccess())
	reader := store.Read(metrix.ReadFlatten())

	withoutDiagnostics, err := New()
	require.NoError(t, err)
	require.NoError(t, withoutDiagnostics.LoadYAML(template, 1))
	want, err := buildPlan(withoutDiagnostics, reader)
	require.NoError(t, err)

	var facts []PlanRouteDiagnostic
	withDiagnostics, err := New(WithPlanRouteDiagnosticObserver(func(fact PlanRouteDiagnostic) {
		facts = append(facts, fact)
	}))
	require.NoError(t, err)
	require.NoError(t, withDiagnostics.LoadYAML(template, 1))
	got, err := buildPlan(withDiagnostics, reader)
	require.NoError(t, err)

	assert.Equal(t, want, got)
	require.Equal(t, 1, countPlanRouteDecisions(facts, PlanRouteChartIdentityRejected))
	for _, fact := range facts {
		if fact.Decision == PlanRouteChartIdentityRejected {
			assert.Equal(t, []string{"instance"}, fact.MissingInstanceLabels)
		}
	}
}

func TestPlanRouteDiagnosticsReportAutogenCollisionInBothOrders(t *testing.T) {
	tests := map[string]struct {
		autogenMetric  string
		authoredMetric string
		wantDecision   PlanRouteDecision
	}{
		"accepted autogen owner is displaced": {
			autogenMetric:  "aaa_total",
			authoredMetric: "zzz_total",
			wantDecision:   PlanRouteAutogenDisplaced,
		},
		"incoming autogen route is rejected": {
			autogenMetric:  "zzz_total",
			authoredMetric: "aaa_total",
			wantDecision:   PlanRouteCollisionRejected,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var facts []PlanRouteDiagnostic
			engine, err := New(
				WithEnginePolicy(EnginePolicy{Autogen: &AutogenPolicy{Enabled: true}}),
				WithPlanRouteDiagnosticObserver(func(fact PlanRouteDiagnostic) {
					facts = append(facts, fact)
				}),
			)
			require.NoError(t, err)
			template := fmt.Sprintf(`
version: v1
groups:
  - family: Test
    metrics: [svc.%s]
    charts:
      - id: svc.%s-method=GET
        title: Authored
        context: svc.authored
        units: requests/s
        dimensions:
          - selector: svc.%s{method="GET"}
            name: total
`, tc.authoredMetric, tc.autogenMetric, tc.authoredMetric)
			require.NoError(t, engine.LoadYAML([]byte(template), 1))

			store := metrix.NewCollectorStore()
			cycle := mustCycleController(t, store)
			meter := store.Write().SnapshotMeter("svc")
			labels := meter.LabelSet(metrix.Label{Key: "method", Value: "GET"})
			cycle.BeginCycle()
			meter.Counter(tc.autogenMetric).ObserveTotal(1, labels)
			meter.Counter(tc.authoredMetric).ObserveTotal(2, labels)
			require.NoError(t, cycle.CommitCycleSuccess())

			_, err = buildPlan(engine, store.Read(metrix.ReadFlatten()))
			require.NoError(t, err)
			require.Equal(t, 1, countPlanRouteDecisions(facts, tc.wantDecision))
			for _, fact := range facts {
				if fact.Decision != tc.wantDecision {
					continue
				}
				if tc.wantDecision == PlanRouteAutogenDisplaced {
					assert.False(t, fact.Autogen)
					assert.False(t, strings.HasPrefix(fact.ChartTemplateID, autogenTemplatePrefix))
					assert.True(t, strings.HasPrefix(fact.ExistingChartTemplateID, autogenTemplatePrefix))
				} else {
					assert.True(t, fact.Autogen)
					assert.True(t, strings.HasPrefix(fact.ChartTemplateID, autogenTemplatePrefix))
					assert.False(t, strings.HasPrefix(fact.ExistingChartTemplateID, autogenTemplatePrefix))
				}
			}
		})
	}
}

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

	var accepted PlanRouteDiagnostic
	for _, fact := range facts {
		if fact.Decision == PlanRouteAccepted {
			accepted = fact
			break
		}
	}
	assert.Equal(t, []string{"instance"}, accepted.InstanceLabels)
	assert.Equal(t, "app.value", accepted.Context)
	assert.Equal(t, "Test", accepted.Family)
	assert.Equal(t, "value", accepted.Units)
	assert.Equal(t, "absolute", accepted.Algorithm)
	assert.Equal(t, "sum", accepted.Aggregation)
	assert.Equal(t, "line", accepted.Presentation)
	assert.Equal(t, "gauge", accepted.SeriesKind)
	assert.Equal(t, 1, accepted.Multiplier)
	assert.Equal(t, 1, accepted.Divisor)
	assert.Equal(t, PlanLabelPromotionAutomatic, accepted.LabelPromotionMode)
	assert.Empty(t, accepted.PromotedLabels)

	assert.Zero(t, engineRuntimeMetricValue(t, engine, routeCacheHitsMetricName))
	assert.Zero(t, engineRuntimeMetricValue(t, engine, routeCacheMissesMetricName))
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
	assert.Equal(t, []string{"instance"}, resolved[0].InstanceLabels)
	require.Len(t, rejected, 1)
	assert.Equal(t, []string{"instance"}, rejected[0].MissingInstanceLabels)
}

func TestPlanRouteDiagnosticsResolveOptionalInstanceIdentity(t *testing.T) {
	var facts []PlanRouteDiagnostic
	engine, err := New(WithPlanRouteDiagnosticObserver(func(fact PlanRouteDiagnostic) {
		facts = append(facts, fact)
	}))
	require.NoError(t, err)
	require.NoError(t, engine.LoadYAML([]byte(`
version: v1
groups:
  - family: Test
    metrics: [worker_cpu]
    charts:
      - id: worker_cpu
        title: Worker CPU
        context: worker.cpu
        units: percentage
        instances:
          optional_by_labels: [pid]
        dimensions:
          - selector: worker_cpu
            name: cpu
`), 1))

	store := metrix.NewCollectorStore()
	cycle := mustCycleController(t, store)
	meter := store.Write().SnapshotMeter("")
	cycle.BeginCycle()
	meter.Gauge("worker_cpu").Observe(1)
	meter.Gauge("worker_cpu").Observe(2, meter.LabelSet(metrix.Label{Key: "pid", Value: "1234"}))
	require.NoError(t, cycle.CommitCycleSuccess())

	_, err = buildPlan(engine, store.Read(metrix.ReadRaw()))
	require.NoError(t, err)
	var resolved []PlanRouteDiagnostic
	for _, fact := range facts {
		if fact.Decision == PlanRouteResolved {
			resolved = append(resolved, fact)
		}
	}
	require.Len(t, resolved, 2)
	var base, perPID *PlanRouteDiagnostic
	for i := range resolved {
		fact := &resolved[i]
		if len(fact.InstanceLabels) == 0 {
			base = fact
		} else if assert.Equal(t, []string{"pid"}, fact.InstanceLabels) {
			perPID = fact
		}
	}
	require.NotNil(t, base)
	require.NotNil(t, perPID)
	assert.Equal(t, globalPlanInstanceIdentity, base.InstanceIdentity)
	assert.NotEqual(t, globalPlanInstanceIdentity, perPID.InstanceIdentity)
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
