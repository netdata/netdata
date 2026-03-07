// SPDX-License-Identifier: GPL-3.0-or-later

package nagios

import (
	"context"
	"math"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/collector/nagios/internal/output"
)

func TestV2Gate_G1_TemplateCompileProof(t *testing.T) {
	templateYAML := New().ChartTemplateYAML()
	collecttest.AssertChartTemplateSchema(t, templateYAML)

	specYAML, err := charttpl.DecodeYAML([]byte(templateYAML))
	if err != nil {
		t.Fatalf("decode template: %v", err)
	}
	if err := specYAML.Validate(); err != nil {
		t.Fatalf("validate template: %v", err)
	}
	if _, err := chartengine.Compile(specYAML, 1); err != nil {
		t.Fatalf("compile template: %v", err)
	}
}

func TestV2Gate_G2_PerfdataRouting(t *testing.T) {
	router := newPerfdataRouter(64)
	warnLow := 100.0
	warnHigh := 500.0
	critLow := 200.0
	critHigh := 900.0
	samples := router.route("default", "gate_job", []output.PerfDatum{
		{
			Label: "latency", Unit: "ms", Value: 120,
			Warn: &output.ThresholdRange{Inclusive: true, Low: &warnLow, High: &warnHigh},
			Crit: &output.ThresholdRange{Inclusive: true, Low: &critLow, High: &critHigh},
		},
		{Label: "throughput", Unit: "KB", Value: 30},
		{Label: "wire_rate", Unit: "kb", Value: 80},
		{Label: "free_pct", Unit: "%", Value: 40},
		{Label: "requests", Unit: "c", Value: 42},
		{Label: "custom", Unit: "widgets", Value: 3.14},
		{Label: "dup-one", Unit: "widgets", Value: 11}, // collides with dup_one
		{Label: "dup_one", Unit: "widgets", Value: 22},
	})

	byName := sampleMap(samples)
	byUnit := sampleUnits(samples)
	assertNear(t, byName["perf_time_latency_value"], 0.12)
	assertNear(t, byName["perf_bytes_throughput_value"], 30000)
	assertNear(t, byName["perf_bits_wire_rate_value"], 80000)
	assertNear(t, byName["perf_percent_free_pct_value"], 40)
	assertNear(t, byName["perf_counter_requests_value"], 42)
	assertNear(t, byName["perf_generic_custom_value"], 3.14)
	assertNear(t, byName["perf_time_latency_warn_low"], 0.1)
	assertNear(t, byName["perf_time_latency_warn_high"], 0.5)
	assertNear(t, byName["perf_time_latency_crit_low"], 0.2)
	assertNear(t, byName["perf_time_latency_crit_high"], 0.9)
	assertNear(t, byName["perf_generic_dup_one_value"], 11)

	assertString(t, byUnit["perf_time_latency_value"], "seconds")
	assertString(t, byUnit["perf_bytes_throughput_value"], "bytes")
	assertString(t, byUnit["perf_bits_wire_rate_value"], "bits")
	assertString(t, byUnit["perf_percent_free_pct_value"], "%")
	assertString(t, byUnit["perf_counter_requests_value"], "c")
	assertString(t, byUnit["perf_generic_custom_value"], "generic")

	store := metrix.NewCollectorStore()
	cc := gateCycleController(t, store)
	cc.BeginCycle()
	sm := store.Write().SnapshotMeter("nagios")
	labels := sm.LabelSet(
		metrix.Label{Key: "nagios_scheduler", Value: "default"},
		metrix.Label{Key: "nagios_job", Value: "gate_job"},
	)
	for _, sample := range samples {
		sm.Gauge(sample.name, metrix.WithUnit(sample.unit), metrix.WithFloat(sample.float)).Observe(sample.value, labels)
	}
	cc.CommitCycleSuccess()

	reader := store.Read(metrix.ReadRaw())
	assertMetricMeta(t, reader, "nagios.perf_time_latency_value", "seconds", true)
	assertMetricMeta(t, reader, "nagios.perf_bytes_throughput_value", "bytes", true)
	assertMetricMeta(t, reader, "nagios.perf_bits_wire_rate_value", "bits", true)
	assertMetricMeta(t, reader, "nagios.perf_percent_free_pct_value", "%", true)
	assertMetricMeta(t, reader, "nagios.perf_counter_requests_value", "c", true)
	assertMetricMeta(t, reader, "nagios.perf_generic_custom_value", "generic", true)
	assertMetricMeta(t, reader, "nagios.perf_time_latency_warn_low", "seconds", true)
	assertMetricMeta(t, reader, "nagios.perf_time_latency_warn_high", "seconds", true)
	assertMetricMeta(t, reader, "nagios.perf_time_latency_warn_defined", "state", false)
	assertMetricMeta(t, reader, "nagios.perf_time_latency_crit_low", "seconds", true)
	assertMetricMeta(t, reader, "nagios.perf_time_latency_crit_high", "seconds", true)
	assertMetricMeta(t, reader, "nagios.perf_time_latency_crit_defined", "state", false)

	_ = router.route("default", "gate_job", []output.PerfDatum{
		{Label: "latency", Unit: "%", Value: 1}, // unit drift: time -> percent
	})
	counters := router.dropCounters()
	if counters.Collision != 1 {
		t.Fatalf("expected collision drop counter to be 1, got %d", counters.Collision)
	}
	if counters.UnitDrift != 1 {
		t.Fatalf("expected unit drift drop counter to be 1, got %d", counters.UnitDrift)
	}
	if counters.Invalid != 0 {
		t.Fatalf("expected invalid drop counter to be 0, got %d", counters.Invalid)
	}
	if counters.Budget != 0 {
		t.Fatalf("expected budget drop counter to be 0, got %d", counters.Budget)
	}
}

func TestV2Gate_G3_ChartLifecycleChurn(t *testing.T) {
	newHarness := func(t *testing.T) (*chartengine.Engine, metrix.CollectorStore, func(includeB bool) chartengine.Plan) {
		t.Helper()
		engine, err := chartengine.New()
		if err != nil {
			t.Fatalf("new chartengine: %v", err)
		}
		if err := engine.LoadYAML([]byte(New().ChartTemplateYAML()), 1); err != nil {
			t.Fatalf("load template: %v", err)
		}

		store := metrix.NewCollectorStore()
		emit := func(includeB bool) chartengine.Plan {
			cc := gateCycleController(t, store)
			cc.BeginCycle()
			sm := store.Write().SnapshotMeter("nagios")
			ls := sm.LabelSet(
				metrix.Label{Key: "nagios_scheduler", Value: "default"},
				metrix.Label{Key: "nagios_job", Value: "gate_job"},
			)
			sm.Gauge("perf_bytes_a_value", metrix.WithUnit("bytes"), metrix.WithFloat(true)).Observe(1, ls)
			if includeB {
				sm.Gauge("perf_bytes_b_value", metrix.WithUnit("bytes"), metrix.WithFloat(true)).Observe(2, ls)
			}
			cc.CommitCycleSuccess()

			plan, err := engine.BuildPlan(store.Read(metrix.ReadFlatten()))
			if err != nil {
				t.Fatalf("build plan: %v", err)
			}
			return plan
		}
		return engine, store, emit
	}

	t.Run("abort-cycle does not remove", func(t *testing.T) {
		engine, store, emit := newHarness(t)

		plan1 := emit(true)
		if countActions[chartengine.CreateChartAction](plan1.Actions) == 0 {
			t.Fatalf("expected create chart action on first cycle")
		}

		cc := gateCycleController(t, store)
		cc.BeginCycle()
		sm := store.Write().SnapshotMeter("nagios")
		ls := sm.LabelSet(
			metrix.Label{Key: "nagios_scheduler", Value: "default"},
			metrix.Label{Key: "nagios_job", Value: "gate_job"},
		)
		sm.Gauge("perf_bytes_a_value", metrix.WithUnit("bytes"), metrix.WithFloat(true)).Observe(1, ls)
		cc.AbortCycle()
		if status := store.Read(metrix.ReadRaw()).CollectMeta().LastAttemptStatus; status != metrix.CollectStatusFailed {
			t.Fatalf("expected failed collect status after abort, got %q", status)
		}

		planAbort, err := engine.BuildPlan(store.Read(metrix.ReadFlatten()))
		if err != nil {
			t.Fatalf("build plan after abort: %v", err)
		}
		if removeActionsCount(planAbort.Actions) != 0 {
			t.Fatalf("expected no remove actions on aborted cycle")
		}
	})

	t.Run("failed-attempt gap contributes to expiry aging", func(t *testing.T) {
		_, store, emit := newHarness(t)

		plan1 := emit(true)
		if countActions[chartengine.CreateChartAction](plan1.Actions) == 0 {
			t.Fatalf("expected create chart action on first cycle")
		}
		plan2 := emit(false)
		if removeActionsCount(plan2.Actions) != 0 {
			t.Fatalf("expected no remove actions on cycle 2")
		}
		assertPlanHasUpdateForTarget(t, plan2, "nagios.perf_bytes_a_value")
		assertPlanHasNoRemoveForTarget(t, plan2, "nagios.perf_bytes_b_value")

		cc := gateCycleController(t, store)
		cc.BeginCycle()
		sm := store.Write().SnapshotMeter("nagios")
		ls := sm.LabelSet(
			metrix.Label{Key: "nagios_scheduler", Value: "default"},
			metrix.Label{Key: "nagios_job", Value: "gate_job"},
		)
		sm.Gauge("perf_bytes_a_value", metrix.WithUnit("bytes"), metrix.WithFloat(true)).Observe(1, ls)
		cc.AbortCycle()
		if status := store.Read(metrix.ReadRaw()).CollectMeta().LastAttemptStatus; status != metrix.CollectStatusFailed {
			t.Fatalf("expected failed collect status after abort, got %q", status)
		}

		plan3 := emit(false)
		if removeActionsCount(plan3.Actions) == 0 {
			t.Fatalf("expected remove actions on cycle 3 after failed-attempt gap")
		}
		assertPlanHasUpdateAndRemoveForTargets(t, plan3,
			"nagios.perf_bytes_a_value",
			"nagios.perf_bytes_b_value",
		)
		plan4 := emit(false)
		if removeActionsCount(plan4.Actions) != 0 {
			t.Fatalf("expected no additional remove actions on cycle 4")
		}
	})
}

func TestV2Gate_G5_ScalingPrecisionEquivalence(t *testing.T) {
	type tc struct {
		name         string
		unit         string
		raw          float64
		expectedUnit string
	}
	cases := []tc{
		{name: "time", unit: "ms", raw: 5.2, expectedUnit: "seconds"},
		{name: "bytes", unit: "KB", raw: 1024, expectedUnit: "bytes"},
		{name: "bits", unit: "kb", raw: 8, expectedUnit: "bits"},
		{name: "percent", unit: "%", raw: 99.5, expectedUnit: "%"},
		{name: "counter", unit: "c", raw: 42, expectedUnit: "c"},
		{name: "generic", unit: "widgets", raw: 3.14, expectedUnit: "generic"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			router := newPerfdataRouter(64)
			displayV1 := legacyDisplayValue(tc.unit, tc.raw)

			samples := router.route("default", "gate_job", []output.PerfDatum{
				{Label: "sample", Unit: tc.unit, Value: tc.raw},
			})
			var (
				candidate    float64
				candidateKey string
				candidateMet perfMetricSample
			)
			found := false
			for _, sample := range samples {
				if len(sample.name) >= 6 && sample.name[len(sample.name)-6:] == "_value" {
					candidate = sample.value
					candidateKey = sample.name
					candidateMet = sample
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("missing routed value sample")
			}

			if displayV1 == 0 {
				if math.Abs(candidate-displayV1) > 1e-9 {
					t.Fatalf("abs error too high: got=%f want=%f", candidate, displayV1)
				}
				return
			}
			rel := math.Abs(candidate-displayV1) / math.Abs(displayV1)
			if rel > 0.001 {
				t.Fatalf("relative error too high: got=%f want=%f rel=%f", candidate, displayV1, rel)
			}
			if candidateMet.unit != tc.expectedUnit {
				t.Fatalf("unexpected routed unit: got=%q want=%q", candidateMet.unit, tc.expectedUnit)
			}
			if !candidateMet.float {
				t.Fatalf("expected routed sample to set float=true")
			}

			store := metrix.NewCollectorStore()
			cc := gateCycleController(t, store)
			cc.BeginCycle()
			sm := store.Write().SnapshotMeter("nagios")
			sm.Gauge(candidateKey, metrix.WithUnit(candidateMet.unit), metrix.WithFloat(candidateMet.float)).
				Observe(candidate, sm.LabelSet())
			cc.CommitCycleSuccess()
			assertMetricMeta(t, store.Read(metrix.ReadRaw()), "nagios."+candidateKey, tc.expectedUnit, true)
		})
	}
}

func countActions[T any](actions []chartengine.EngineAction) int {
	n := 0
	for _, action := range actions {
		if _, ok := action.(T); ok {
			n++
		}
	}
	return n
}

func removeActionsCount(actions []chartengine.EngineAction) int {
	return countActions[chartengine.RemoveChartAction](actions) + countActions[chartengine.RemoveDimensionAction](actions)
}

func gateCycleController(t *testing.T, store metrix.CollectorStore) metrix.CycleController {
	t.Helper()
	managed, ok := metrix.AsCycleManagedStore(store)
	if !ok {
		t.Fatalf("store does not expose cycle control")
	}
	return managed.CycleController()
}

func assertMetricMeta(t *testing.T, reader metrix.Reader, metricName, unit string, isFloat bool) {
	t.Helper()
	meta, ok := reader.MetricMeta(metricName)
	if !ok {
		t.Fatalf("missing metric metadata for %q", metricName)
	}
	if meta.Unit != unit {
		t.Fatalf("unexpected unit for %q: got=%q want=%q", metricName, meta.Unit, unit)
	}
	if meta.Float != isFloat {
		t.Fatalf("unexpected float metadata for %q: got=%t want=%t", metricName, meta.Float, isFloat)
	}
}

func assertPlanHasUpdateAndRemoveForTargets(t *testing.T, plan chartengine.Plan, updateMetricPrefix, removeMetricPrefix string) {
	t.Helper()

	hasUpdate := false
	hasRemove := false

	for _, action := range plan.Actions {
		switch a := action.(type) {
		case chartengine.UpdateChartAction:
			if strings.HasPrefix(a.ChartID, updateMetricPrefix) {
				hasUpdate = true
			}
		case chartengine.RemoveDimensionAction:
			if strings.HasPrefix(a.ChartID, removeMetricPrefix) {
				hasRemove = true
			}
		case chartengine.RemoveChartAction:
			if strings.HasPrefix(a.ChartID, removeMetricPrefix) {
				hasRemove = true
			}
		}
	}

	if !hasUpdate {
		t.Fatalf("expected update action for %q", updateMetricPrefix)
	}
	if !hasRemove {
		t.Fatalf("expected remove action for %q", removeMetricPrefix)
	}
}

func assertPlanHasUpdateForTarget(t *testing.T, plan chartengine.Plan, updateMetricPrefix string) {
	t.Helper()
	for _, action := range plan.Actions {
		update, ok := action.(chartengine.UpdateChartAction)
		if !ok {
			continue
		}
		if strings.HasPrefix(update.ChartID, updateMetricPrefix) {
			return
		}
	}
	t.Fatalf("expected update action for %q", updateMetricPrefix)
}

func assertPlanHasNoRemoveForTarget(t *testing.T, plan chartengine.Plan, removeMetricPrefix string) {
	t.Helper()
	for _, action := range plan.Actions {
		switch a := action.(type) {
		case chartengine.RemoveDimensionAction:
			if strings.HasPrefix(a.ChartID, removeMetricPrefix) {
				t.Fatalf("unexpected remove dimension action for %q", removeMetricPrefix)
			}
		case chartengine.RemoveChartAction:
			if strings.HasPrefix(a.ChartID, removeMetricPrefix) {
				t.Fatalf("unexpected remove chart action for %q", removeMetricPrefix)
			}
		}
	}
}

func TestV2Gate_SmokeCollect(t *testing.T) {
	coll := NewWithRegistry(newFakeRegistry())
	coll.Config.JobConfig.Plugin = "/bin/true"
	coll.Config.JobConfig.Name = "smoke"
	if err := coll.Check(context.Background()); err != nil {
		t.Fatalf("check failed: %v", err)
	}
}
