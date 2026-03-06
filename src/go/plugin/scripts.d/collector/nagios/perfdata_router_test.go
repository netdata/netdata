// SPDX-License-Identifier: GPL-3.0-or-later

package nagios

import (
	"math"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/collector/nagios/internal/output"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testPluginPath = "/opt/nagios-scripts/check_memory.pl"

func TestPerfdataRouterRoutesAndCanonicalizesUnits(t *testing.T) {
	router := newPerfdataRouter(64)

	warnLow := 100.0
	warnHigh := 500.0
	got := router.route(testPluginPath, []output.PerfDatum{
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

	values := valueSampleMap(got.values)
	units := valueSampleUnits(got.values)
	thresholds := thresholdStateMap(got.thresholdStates)

	assertNear(t, values["check_memory.time_latency_value"], 0.12)
	assertNear(t, values["check_memory.bytes_throughput_value"], 30_000)
	assertNear(t, values["check_memory.bits_traffic_value"], 1_500_000)
	assertNear(t, values["check_memory.percent_free_pct_value"], 40)
	assertNear(t, values["check_memory.counter_checks_value"], 3)
	assertNear(t, values["check_memory.generic_custom_value"], 7.25)

	assertString(t, units["check_memory.time_latency_value"], "seconds")
	assertString(t, units["check_memory.bytes_throughput_value"], "bytes")
	assertString(t, units["check_memory.bits_traffic_value"], "bits")
	assertString(t, units["check_memory.percent_free_pct_value"], "%")
	assertString(t, units["check_memory.counter_checks_value"], "c")
	assertString(t, units["check_memory.generic_custom_value"], "generic")

	assertString(t, thresholds["check_memory.time_latency_threshold_state"], perfThresholdStateWarning)
	assertString(t, thresholds["check_memory.bytes_throughput_threshold_state"], perfThresholdStateNone)
	assertString(t, thresholds["check_memory.bits_traffic_threshold_state"], perfThresholdStateNone)
	assertString(t, thresholds["check_memory.percent_free_pct_threshold_state"], perfThresholdStateNone)
	assertString(t, thresholds["check_memory.generic_custom_threshold_state"], perfThresholdStateNone)
	_, hasCounterThreshold := thresholds["check_memory.counter_checks_threshold_state"]
	assert.False(t, hasCounterThreshold)
}

func TestPerfdataRouterPolicies(t *testing.T) {
	tests := map[string]struct {
		budget int
		prime  []output.PerfDatum
		input  []output.PerfDatum
		assert func(*testing.T, perfRouteResult)
	}{
		"collision keeps first lexical metric key": {
			budget: 64,
			input: []output.PerfDatum{
				{Label: "used-kb", Unit: "KB", Value: 2},
				{Label: "used kb", Unit: "KB", Value: 1},
			},
			assert: func(t *testing.T, got perfRouteResult) {
				t.Helper()
				samples := valueSampleMap(got.values)
				assertNear(t, samples["check_memory.bytes_used_kb_value"], 1_000)
			},
		},
		"budget drops metrics beyond cap": {
			budget: 2,
			input: []output.PerfDatum{
				{Label: "a", Unit: "c", Value: 1},
				{Label: "b", Unit: "c", Value: 2},
				{Label: "c", Unit: "c", Value: 3},
			},
			assert: func(t *testing.T, got perfRouteResult) {
				t.Helper()
				samples := valueSampleMap(got.values)
				_, okA := samples["check_memory.counter_a_value"]
				_, okB := samples["check_memory.counter_b_value"]
				_, okC := samples["check_memory.counter_c_value"]
				assert.True(t, okA)
				assert.True(t, okB)
				assert.False(t, okC)
			},
		},
		"class changes create a new metric identity": {
			budget: 64,
			prime: []output.PerfDatum{
				{Label: "latency", Unit: "ms", Value: 10},
			},
			input: []output.PerfDatum{
				{Label: "latency", Unit: "KB", Value: 10},
			},
			assert: func(t *testing.T, got perfRouteResult) {
				t.Helper()
				samples := valueSampleMap(got.values)
				assertNear(t, samples["check_memory.bytes_latency_value"], 10_000)
			},
		},
		"invalid samples are ignored": {
			budget: 64,
			input: []output.PerfDatum{
				{Label: "", Unit: "ms", Value: 1},
				{Label: "bad", Unit: "ms", Value: math.NaN()},
			},
			assert: func(t *testing.T, got perfRouteResult) {
				t.Helper()
				assert.Empty(t, got.values)
				assert.Empty(t, got.thresholdStates)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			router := newPerfdataRouter(tc.budget)
			if len(tc.prime) > 0 {
				primed := router.route(testPluginPath, tc.prime)
				require.NotEmpty(t, primed.values)
			}

			got := router.route(testPluginPath, tc.input)
			tc.assert(t, got)
		})
	}
}

func TestSanitizeMetricKey(t *testing.T) {
	tests := map[string]struct {
		input        string
		expect       string
		expectPrefix string
	}{
		"plain text preserves words": {
			input:  "Disk usage",
			expect: "disk_usage",
		},
		"trailing punctuation keeps boundary underscore": {
			input:  "Disk usage /",
			expect: "disk_usage_",
		},
		"non alnum falls back to synthetic id": {
			input:        "///",
			expectPrefix: "id_",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := sanitizeMetricKey(tc.input)
			if tc.expect != "" {
				assert.Equal(t, tc.expect, got)
			}
			if tc.expectPrefix != "" {
				assert.True(t, strings.HasPrefix(got, tc.expectPrefix))
			}
		})
	}
}

func valueSampleMap(sets []perfValueMeasureSet) map[string]float64 {
	out := make(map[string]float64, len(sets))
	for _, set := range sets {
		out[set.name+"_"+perfFieldValue] = float64(set.value)
	}
	return out
}

func valueSampleUnits(sets []perfValueMeasureSet) map[string]string {
	out := make(map[string]string, len(sets))
	for _, set := range sets {
		out[set.name+"_"+perfFieldValue] = set.unit
	}
	return out
}

func thresholdStateMap(sets []perfThresholdStateSet) map[string]string {
	out := make(map[string]string, len(sets))
	for _, set := range sets {
		out[set.name] = set.state
	}
	return out
}

func assertNear(t *testing.T, got, want float64) {
	t.Helper()
	assert.InDelta(t, want, got, 1e-9)
}

func assertString(t *testing.T, got, want string) {
	t.Helper()
	assert.Equal(t, want, got)
}
