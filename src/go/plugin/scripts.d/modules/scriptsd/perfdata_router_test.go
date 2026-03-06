// SPDX-License-Identifier: GPL-3.0-or-later

package scriptsd

import (
	"math"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/output"
)

func TestPerfdataRouterRoutesAndCanonicalizesUnits(t *testing.T) {
	router := newPerfdataRouter(64)

	warnLow := 100.0
	warnHigh := 500.0
	got := router.route("default", "job1", []output.PerfDatum{
		{
			Label: "latency",
			Unit:  "ms",
			Value: 120,
			Warn: &output.ThresholdRange{
				Inclusive: true,
				Low:       &warnLow,
				High:      &warnHigh,
			},
		},
		{Label: "throughput", Unit: "KB", Value: 30},
		{Label: "traffic", Unit: "Mb", Value: 1.5},
		{Label: "free_pct", Unit: "%", Value: 40},
		{Label: "checks", Unit: "c", Value: 3},
		{Label: "custom", Unit: "widgets", Value: 7.25},
	})

	samples := sampleMap(got)
	assertNear(t, samples["perf_time_latency_value"], 0.12)
	assertNear(t, samples["perf_bytes_throughput_value"], 30_000)
	assertNear(t, samples["perf_bits_traffic_value"], 1_500_000)
	assertNear(t, samples["perf_percent_free_pct_value"], 40)
	assertNear(t, samples["perf_counter_checks_value"], 3)
	assertNear(t, samples["perf_generic_custom_value"], 7.25)
	assertNear(t, samples["perf_time_latency_warn_defined"], 1)
	assertNear(t, samples["perf_time_latency_warn_inclusive"], 1)
	assertNear(t, samples["perf_time_latency_warn_low"], 0.1)
	assertNear(t, samples["perf_time_latency_warn_high"], 0.5)
}

func TestPerfdataRouterCollisionPolicy(t *testing.T) {
	router := newPerfdataRouter(64)

	got := router.route("default", "job1", []output.PerfDatum{
		{Label: "used-kb", Unit: "KB", Value: 2},
		{Label: "used kb", Unit: "KB", Value: 1},
	})

	samples := sampleMap(got)
	// Lexical order keeps "used kb" before "used-kb".
	assertNear(t, samples["perf_bytes_used_kb_value"], 1_000)

	counters := router.dropCounters()
	if counters.Collision != 1 {
		t.Fatalf("expected 1 collision drop, got %d", counters.Collision)
	}
}

func TestPerfdataRouterBudgetPolicy(t *testing.T) {
	router := newPerfdataRouter(2)

	got := router.route("default", "job1", []output.PerfDatum{
		{Label: "a", Unit: "c", Value: 1},
		{Label: "b", Unit: "c", Value: 2},
		{Label: "c", Unit: "c", Value: 3},
	})

	samples := sampleMap(got)
	if _, ok := samples["perf_counter_a_value"]; !ok {
		t.Fatalf("expected metric a to be present")
	}
	if _, ok := samples["perf_counter_b_value"]; !ok {
		t.Fatalf("expected metric b to be present")
	}
	if _, ok := samples["perf_counter_c_value"]; ok {
		t.Fatalf("expected metric c to be dropped by budget")
	}

	counters := router.dropCounters()
	if counters.Budget != 1 {
		t.Fatalf("expected 1 budget drop, got %d", counters.Budget)
	}
}

func TestPerfdataRouterUnitDriftPolicy(t *testing.T) {
	router := newPerfdataRouter(64)

	first := router.route("default", "job1", []output.PerfDatum{
		{Label: "latency", Unit: "ms", Value: 10},
	})
	if len(first) == 0 {
		t.Fatalf("expected first route to emit samples")
	}

	second := router.route("default", "job1", []output.PerfDatum{
		{Label: "latency", Unit: "KB", Value: 10},
	})
	samples := sampleMap(second)
	if _, ok := samples["perf_bytes_latency_value"]; ok {
		t.Fatalf("expected unit-drift sample to be dropped")
	}

	counters := router.dropCounters()
	if counters.UnitDrift != 1 {
		t.Fatalf("expected 1 unit-drift drop, got %d", counters.UnitDrift)
	}
}

func TestPerfdataRouterInvalidSamples(t *testing.T) {
	router := newPerfdataRouter(64)

	_ = router.route("default", "job1", []output.PerfDatum{
		{Label: "", Unit: "ms", Value: 1},
		{Label: "bad", Unit: "ms", Value: math.NaN()},
	})

	counters := router.dropCounters()
	if counters.Invalid != 2 {
		t.Fatalf("expected 2 invalid drops, got %d", counters.Invalid)
	}
}

func sampleMap(samples []perfMetricSample) map[string]float64 {
	out := make(map[string]float64, len(samples))
	for _, sample := range samples {
		out[sample.name] = sample.value
	}
	return out
}

func assertNear(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("value mismatch: got=%f want=%f", got, want)
	}
}
