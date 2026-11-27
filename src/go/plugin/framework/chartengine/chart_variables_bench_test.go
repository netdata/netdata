// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"strconv"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

// benchVariablesTemplateYAML has one chart per instance fed by bench_rx; the
// VARIABLES placeholder adds chart variables fed by bench_speed and bench_limit.
const benchVariablesTemplateYAML = `
version: v1
groups:
  - family: "Bench"
    metrics: ["bench_rx", "bench_speed", "bench_limit"]
    charts:
      - title: "Bench traffic"
        context: "bench.traffic"
        units: "units"
        instances:
          by_labels: [id]
        dimensions:
          - selector: bench_rx
            name: rx
VARIABLES`

const benchVariablesBlock = `        variables:
          - name: speed
            selector: bench_speed
          - name: limit
            selector: bench_limit
`

// BenchmarkBuildPlanChartVariables measures steady planning for one chart per
// instance (charts = instances, one dimension each):
//   - none: the template declares no variables; the variable path is skipped by
//     the match-index check, so allocs/op match a build without the feature;
//   - mixed: the template declares two variables, fed for one instance in ten
//     (many charts, few variable values);
//   - every: both variables are fed for every instance.
//
// Envelope: route resolution stays O(candidates) per series; variable values
// add one map entry per routed variable (the map is allocated once per build,
// sized from the previous build) and one value-block slot per emitted variable,
// so allocs/op stay O(1) per build plus map growth, not O(series).
// ns/op is a development-machine trend indicator, not a gate.
func BenchmarkBuildPlanChartVariables(b *testing.B) {
	const instances = 1000
	tests := map[string]struct {
		variables bool
		fedEvery  int // feed variables for every n-th instance; 0 feeds none
	}{
		"none":  {},
		"mixed": {variables: true, fedEvery: 10},
		"every": {variables: true, fedEvery: 1},
	}
	for name, tc := range tests {
		b.Run(name, func(b *testing.B) {
			block := ""
			if tc.variables {
				block = benchVariablesBlock
			}
			engine, err := New()
			if err != nil {
				b.Fatalf("new engine: %v", err)
			}
			if err := engine.LoadYAML([]byte(replaceVariablesPlaceholder(block)), 1); err != nil {
				b.Fatalf("load template: %v", err)
			}
			reader := benchmarkVariablesReader(b, instances, tc.fedEvery)

			reader.seq = 1
			if _, err := buildPlan(engine, reader); err != nil {
				b.Fatalf("warm plan: %v", err)
			}
			var plan Plan
			b.ReportAllocs()
			b.ResetTimer()
			for i := range b.N {
				reader.seq = uint64(i) + 2
				plan, err = buildPlan(engine, reader)
				if err != nil {
					b.Fatalf("build plan: %v", err)
				}
			}
			b.StopTimer()

			wantVariables := 0
			if tc.fedEvery > 0 {
				wantVariables = 2 * ((instances + tc.fedEvery - 1) / tc.fedEvery)
			}
			updates, variables := 0, 0
			for _, action := range plan.Actions {
				update, ok := action.(UpdateChartAction)
				if !ok {
					b.Fatalf("unexpected warmed action %T", action)
				}
				updates++
				variables += len(update.Variables)
			}
			if updates != instances || variables != wantVariables {
				b.Fatalf("updates=%d variables=%d; want %d/%d", updates, variables, instances, wantVariables)
			}
		})
	}
}

func replaceVariablesPlaceholder(block string) string {
	const placeholder = "VARIABLES"
	out := benchVariablesTemplateYAML[:len(benchVariablesTemplateYAML)-len(placeholder)]
	return out + block
}

func benchmarkVariablesReader(b *testing.B, instances, fedEvery int) *benchmarkSequenceReader {
	b.Helper()

	store := metrix.NewCollectorStore()
	managed, ok := metrix.AsCycleManagedStore(store)
	if !ok {
		b.Fatalf("collector store is not cycle-managed")
	}
	cc := managed.CycleController()

	meter := store.Write().SnapshotMeter("")
	rx := meter.Gauge("bench_rx")
	speed := meter.Gauge("bench_speed")
	limit := meter.Gauge("bench_limit")

	cc.BeginCycle()
	for i := range instances {
		labels := meter.LabelSet(metrix.Label{
			Key:   "id",
			Value: strconv.Itoa(i),
		})
		rx.Observe(metrix.SampleValue(i), labels)
		if fedEvery > 0 && i%fedEvery == 0 {
			speed.Observe(1e9, labels)
			limit.Observe(1, labels)
		}
	}
	if err := cc.CommitCycleSuccess(); err != nil {
		b.Fatalf("commit benchmark cycle: %v", err)
	}
	reader := store.Read(metrix.ReadRaw())
	return &benchmarkSequenceReader{
		Reader: reader,
		raw:    reader.(metrix.SeriesIdentityRawIterator),
		seq:    1,
	}
}
