// SPDX-License-Identifier: GPL-3.0-or-later

package promsemantics

import "testing"

func TestCompileComponentUnitDerivesLifecycleDefaults(t *testing.T) {
	tests := map[string]struct {
		component Component
		algorithm string
		rate      string
		unit      string
		scale     rationalScale
	}{
		"cumulative count": {
			component: testUnitComponent("cumulative", "count", "one", "none", "requests"),
			algorithm: "incremental",
			rate:      "per_second",
			unit:      "requests/s",
			scale:     newRationalScale(1, 1),
		},
		"current duration": {
			component: testUnitComponent("current", "duration", "second", "none", "request_time"),
			algorithm: "absolute",
			rate:      "none",
			unit:      "seconds",
			scale:     newRationalScale(1, 1),
		},
		"current duration in source hours": {
			component: testUnitComponent("current", "duration", "hour", "none", "budget_reset_time"),
			algorithm: "absolute",
			rate:      "none",
			unit:      "seconds",
			scale:     newRationalScale(3600, 1),
		},
		"current duration in source milliseconds": {
			component: testUnitComponent("current", "duration", "millisecond", "none", "latency"),
			algorithm: "absolute",
			rate:      "none",
			unit:      "seconds",
			scale:     newRationalScale(1, 1_000),
		},
		"current source percentage": {
			component: testUnitComponent("current", "ratio", "percent", "none", "utilization"),
			algorithm: "absolute",
			rate:      "none",
			unit:      "ratio",
			scale:     newRationalScale(1, 100),
		},
		"current source per mille": {
			component: testUnitComponent("current", "ratio", "per_mille", "none", "fragmentation"),
			algorithm: "absolute",
			rate:      "none",
			unit:      "ratio",
			scale:     newRationalScale(1, 1_000),
		},
		"current source megabits per second": {
			component: testUnitComponent("current", "data", "megabit", "per_second", "network_data"),
			algorithm: "absolute",
			rate:      "per_second",
			unit:      "bytes/s",
			scale:     newRationalScale(125_000, 1),
		},
		"cumulative source squared microseconds": {
			component: testUnitComponent("cumulative", "duration_squared", "microsecond_squared", "none", "latency"),
			algorithm: "incremental",
			rate:      "per_second",
			unit:      "seconds²/s",
			scale:     newRationalScale(1, 1_000_000_000_000),
		},
		"current rate": {
			component: testUnitComponent("current", "data", "byte", "per_second", "network_data"),
			algorithm: "absolute",
			rate:      "per_second",
			unit:      "bytes/s",
			scale:     newRationalScale(1, 1),
		},
		"current source rate per minute": {
			component: testUnitComponent("current", "count", "one", "per_minute", "requests"),
			algorithm: "absolute",
			rate:      "per_second",
			unit:      "requests/s",
			scale:     newRationalScale(1, 60),
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			compiled, err := compileComponentUnit(name, tc.component)
			if err != nil {
				t.Fatalf("compileComponentUnit() error = %v", err)
			}
			if compiled.algorithm != tc.algorithm || compiled.effectiveRate != tc.rate ||
				compiled.canonicalUnit != tc.unit || compiled.scale != tc.scale {
				t.Fatalf("compileComponentUnit() = %#v", compiled)
			}
		})
	}
}

func TestRegisteredDisplayUsesExactScale(t *testing.T) {
	tests := map[string]struct {
		component  Component
		convention string
		unit       string
		scale      rationalScale
	}{
		"network megabits per second": {
			component:  testUnitComponent("current", "data", "byte", "per_second", "network_data"),
			convention: "network_megabits_per_second",
			unit:       "Mbps",
			scale:      newRationalScale(8, 1_000_000),
		},
		"cpu cores": {
			component:  testUnitComponent("cumulative", "duration", "second", "none", "process_cpu_time"),
			convention: "cpu_cores",
			unit:       "cores",
			scale:      newRationalScale(1, 1),
		},
		"file descriptors": {
			component:  testUnitComponent("current", "count", "one", "none", "file_descriptors"),
			convention: "file_descriptors",
			unit:       "fds",
			scale:      newRationalScale(1, 1),
		},
		"source hours displayed as hours": {
			component:  testUnitComponent("current", "duration", "hour", "none", "budget_reset_time"),
			convention: "duration_hours",
			unit:       "hours",
			scale:      newRationalScale(1, 1),
		},
		"source requests per minute displayed per minute": {
			component:  testUnitComponent("current", "count", "one", "per_minute", "requests"),
			convention: "requests_per_minute",
			unit:       "requests/min",
			scale:      newRationalScale(1, 1),
		},
		"source tokens per minute displayed per minute": {
			component:  testUnitComponent("current", "count", "one", "per_minute", "tokens"),
			convention: "tokens_per_minute",
			unit:       "tokens/min",
			scale:      newRationalScale(1, 1),
		},
		"source milliseconds displayed as milliseconds": {
			component:  testUnitComponent("current", "duration", "millisecond", "none", "latency"),
			convention: "duration_milliseconds",
			unit:       "milliseconds",
			scale:      newRationalScale(1, 1),
		},
		"source percentage displayed as percentage": {
			component:  testUnitComponent("current", "ratio", "percent", "none", "utilization"),
			convention: "percentage",
			unit:       "percentage",
			scale:      newRationalScale(1, 1),
		},
		"source per mille displayed as per mille": {
			component:  testUnitComponent("current", "ratio", "per_mille", "none", "fragmentation"),
			convention: "per_mille",
			unit:       "per mille",
			scale:      newRationalScale(1, 1),
		},
		"source megabits per second displayed as megabits per second": {
			component:  testUnitComponent("current", "data", "megabit", "per_second", "network_data"),
			convention: "network_megabits_per_second",
			unit:       "Mbps",
			scale:      newRationalScale(1, 1),
		},
		"source squared microseconds displayed as squared microseconds per second": {
			component:  testUnitComponent("cumulative", "duration_squared", "microsecond_squared", "none", "latency"),
			convention: "duration_squared_microseconds_per_second",
			unit:       "microseconds²/s",
			scale:      newRationalScale(1, 1),
		},
		"request failures": {
			component:  testUnitComponent("cumulative", "count", "one", "none", "requests"),
			convention: "request_failures",
			unit:       "failures/s",
			scale:      newRationalScale(1, 1),
		},
		"placement groups": {
			component:  testUnitComponent("current", "count", "one", "none", "placement_groups"),
			convention: "placement_groups",
			unit:       "PGs",
			scale:      newRationalScale(1, 1),
		},
		"virtual machines": {
			component:  testUnitComponent("current", "count", "one", "none", "vms"),
			convention: "virtual_machines",
			unit:       "VMs",
			scale:      newRationalScale(1, 1),
		},
		"OSDs": {
			component:  testUnitComponent("current", "count", "one", "none", "osds"),
			convention: "osds",
			unit:       "OSDs",
			scale:      newRationalScale(1, 1),
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			component, err := compileComponentUnit(name, tc.component)
			if err != nil {
				t.Fatal(err)
			}
			unit, scale, err := resolveRegisteredDisplay(name, component, tc.convention)
			if err != nil {
				t.Fatalf("resolveRegisteredDisplay() error = %v", err)
			}
			if unit != tc.unit || scale != tc.scale {
				t.Fatalf("display = %q %#v, want %q %#v", unit, scale, tc.unit, tc.scale)
			}
		})
	}
}

func TestCompileComponentUnitRejectsUnregisteredBase(t *testing.T) {
	component := testUnitComponent("current", "duration", "fortnight", "none", "request_time")
	if _, err := compileComponentUnit("duration", component); err == nil {
		t.Fatal("compileComponentUnit() error = nil")
	}
}

func testUnitComponent(lifecycle, quantity, base, rate, object string) Component {
	return Component{
		WireRole: "scalar",
		Lifecycle: ComponentLifecycle{
			Kind: lifecycle,
		},
		Unit: ComponentUnit{
			Quantity: quantity,
			Base:     base,
			Rate:     rate,
			Object:   object,
			Aspect:   "value",
		},
	}
}
