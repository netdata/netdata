// SPDX-License-Identifier: GPL-3.0-or-later

package scriptsd

import (
	"context"
	"math"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/output"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/units"
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
	samples := router.route("default", "gate_job", []output.PerfDatum{
		{
			Label: "latency", Unit: "ms", Value: 120,
			Warn: &output.ThresholdRange{Inclusive: true, Low: &warnLow, High: &warnHigh},
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

	assertString(t, byUnit["perf_time_latency_value"], "seconds")
	assertString(t, byUnit["perf_bytes_throughput_value"], "bytes")
	assertString(t, byUnit["perf_bits_wire_rate_value"], "bits")
	assertString(t, byUnit["perf_percent_free_pct_value"], "%")
	assertString(t, byUnit["perf_counter_requests_value"], "c")
	assertString(t, byUnit["perf_generic_custom_value"], "generic")

	_ = router.route("default", "gate_job", []output.PerfDatum{
		{Label: "latency", Unit: "%", Value: 1}, // unit drift: time -> percent
	})
	counters := router.dropCounters()
	if counters.Collision == 0 {
		t.Fatalf("expected collision drop counter to increment")
	}
	if counters.UnitDrift == 0 {
		t.Fatalf("expected unit drift drop counter to increment")
	}
}

func TestV2Gate_G3_ChartLifecycleChurn(t *testing.T) {
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
		sm := store.Write().SnapshotMeter("scriptsd")
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

	plan1 := emit(true)
	if countActions[chartengine.CreateChartAction](plan1.Actions) == 0 {
		t.Fatalf("expected create chart action on first cycle")
	}

	plan2 := emit(false)
	if removeActionsCount(plan2.Actions) != 0 {
		t.Fatalf("expected no remove actions on cycle 2")
	}
	plan3 := emit(false)
	if removeActionsCount(plan3.Actions) != 0 {
		t.Fatalf("expected no remove actions on cycle 3")
	}
	plan4 := emit(false)
	if removeActionsCount(plan4.Actions) == 0 {
		t.Fatalf("expected remove action after expiry window")
	}
}

func TestV2Gate_G5_ScalingPrecisionEquivalence(t *testing.T) {
	type tc struct {
		name string
		unit string
		raw  float64
	}
	cases := []tc{
		{name: "time", unit: "ms", raw: 5.2},
		{name: "bytes", unit: "KB", raw: 1024},
		{name: "bits", unit: "kb", raw: 8},
		{name: "percent", unit: "%", raw: 99.5},
		{name: "counter", unit: "c", raw: 42},
		{name: "generic", unit: "widgets", raw: 3.14},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			router := newPerfdataRouter(64)
			scale := units.NewScale(tc.unit)
			displayV1 := float64(scale.Apply(tc.raw)) / float64(scale.Divisor)

			samples := router.route("default", "gate_job", []output.PerfDatum{
				{Label: "sample", Unit: tc.unit, Value: tc.raw},
			})
			got := sampleMap(samples)
			var candidate float64
			found := false
			for name, v := range got {
				if len(name) >= 6 && name[len(name)-6:] == "_value" {
					candidate = v
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

func TestV2Gate_SmokeCollect(t *testing.T) {
	coll := NewWithRegistry(newFakeRegistry())
	coll.Config.JobConfig.Plugin = "/bin/true"
	coll.Config.JobConfig.Name = "smoke"
	if err := coll.Check(context.Background()); err != nil {
		t.Fatalf("check failed: %v", err)
	}
}
