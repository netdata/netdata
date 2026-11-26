// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

const updateSpanYAML = `
version: v1
groups:
  - family: Span
    metrics: [a, b, c]
    charts:
      - id: a
        title: A
        context: a
        units: v
        dimensions:
          - selector: a
            name: a
      - id: b
        title: B
        context: b
        units: v
        dimensions:
          - selector: b
            name: b
      - id: c
        title: C
        context: c
        units: v
        lifecycle:
          expire_after_cycles: 1
        dimensions:
          - selector: c
            name: c
`

// spanStep is one successful collection cycle planned at at (zero: no time).
type spanStep struct {
	at     time.Time
	write  []string // metrics written this cycle
	reset  bool     // ResetMaterialized
	abort  bool     // abort the attempt instead of committing it
	want   map[string]time.Duration
	remove []string // charts the plan removes
}

func TestUpdateChartActionSinceLast(t *testing.T) {
	sec := func(s int) time.Time { return time.Unix(int64(1_000_000+s), 0) }
	spans := func(kv ...any) map[string]time.Duration {
		m := make(map[string]time.Duration)
		for i := 0; i < len(kv); i += 2 {
			m[kv[i].(string)] = time.Duration(kv[i+1].(int)) * time.Second
		}
		return m
	}
	tests := map[string][]spanStep{
		"consecutive, skipped, aborted, untimed, backward and reset plans": {
			{at: sec(10), write: []string{"a", "b"}, want: spans("a", 0, "b", 0)},
			{at: sec(20), write: []string{"a", "b"}, want: spans("a", 10, "b", 10)},
			// b's series are not observed: no update, its stamp stays at 20.
			{at: sec(30), write: []string{"a"}, want: spans("a", 10)},
			{at: sec(40), write: []string{"a", "b"}, want: spans("a", 10, "b", 20)},
			// An aborted attempt delivers nothing: the next spans cover it.
			{at: sec(50), write: []string{"a", "b"}, abort: true, want: spans("a", 10, "b", 10)},
			{at: sec(60), write: []string{"a", "b"}, want: spans("a", 20, "b", 20)},
			// An untimed plan sends no span and the next timed one sends none either.
			{write: []string{"a", "b"}, want: spans("a", 0, "b", 0)},
			{at: sec(80), write: []string{"a", "b"}, want: spans("a", 0, "b", 0)},
			// A clock step backwards sends no span and restamps.
			{at: sec(75), write: []string{"a", "b"}, want: spans("a", 0, "b", 0)},
			{at: sec(85), write: []string{"a", "b"}, want: spans("a", 10, "b", 10)},
			// A host reset starts new charts.
			{at: sec(95), write: []string{"a", "b"}, reset: true, want: spans("a", 0, "b", 0)},
			{at: sec(105), write: []string{"a", "b"}, want: spans("a", 10, "b", 10)},
		},
		"revived chart starts without a span": {
			{at: sec(10), write: []string{"a", "c"}, want: spans("a", 0, "c", 0)},
			{at: sec(20), write: []string{"a", "c"}, want: spans("a", 10, "c", 10)},
			{at: sec(30), write: []string{"a"}, want: spans("a", 10), remove: []string{"c"}},
			{at: sec(40), write: []string{"a", "c"}, want: spans("a", 10, "c", 0)},
		},
	}
	for name, steps := range tests {
		t.Run(name, func(t *testing.T) {
			engine, err := New(WithRuntimeStore(nil))
			require.NoError(t, err)
			require.NoError(t, engine.LoadYAML([]byte(updateSpanYAML), 1))
			store := metrix.NewCollectorStore()
			cc := mustCycleController(t, store)
			meter := store.Write().SnapshotMeter("")
			for i, step := range steps {
				cc.BeginCycle()
				for _, name := range step.write {
					meter.Gauge(name).Observe(1)
				}
				require.NoError(t, cc.CommitCycleSuccess())
				attempt, err := engine.PreparePlanWithOptions(
					store.Read(metrix.ReadRaw(), metrix.ReadFlatten()),
					PlanOptions{ResetMaterialized: step.reset, CollectedAt: step.at},
				)
				require.NoError(t, err)
				got, removed := planSpans(attempt.Plan())
				assert.Equalf(t, step.want, got, "step %d spans", i)
				assert.Equalf(t, step.remove, removed, "step %d removals", i)
				if step.abort {
					attempt.Abort()
					continue
				}
				require.NoError(t, attempt.Commit())
			}
		})
	}
}

// A runtime planner emits updates on every plan while the source sequence
// repeats; each one must still advance its chart's stamp.
func TestUpdateChartActionSinceLastRuntimePlanner(t *testing.T) {
	engine, err := New(WithRuntimeStore(nil), WithRuntimePlannerMode())
	require.NoError(t, err)
	require.NoError(t, engine.LoadYAML([]byte(updateSpanYAML), 1))
	store := metrix.NewCollectorStore()
	cc := mustCycleController(t, store)
	cc.BeginCycle()
	store.Write().SnapshotMeter("").Gauge("a").Observe(1)
	require.NoError(t, cc.CommitCycleSuccess())
	reader := store.Read(metrix.ReadRaw(), metrix.ReadFlatten())

	var got []time.Duration
	for _, at := range []int64{10, 20, 30, 45} {
		attempt, err := engine.PreparePlanWithOptions(reader, PlanOptions{CollectedAt: time.Unix(at, 0)})
		require.NoError(t, err)
		spans, _ := planSpans(attempt.Plan())
		got = append(got, spans["a"])
		require.NoError(t, attempt.Commit())
	}
	assert.Equal(t, []time.Duration{0, 10 * time.Second, 10 * time.Second, 15 * time.Second}, got)
}

func planSpans(plan Plan) (map[string]time.Duration, []string) {
	spans := make(map[string]time.Duration)
	var removed []string
	for _, action := range plan.Actions {
		switch v := action.(type) {
		case UpdateChartAction:
			spans[v.ChartID] = v.SinceLast
		case RemoveChartAction:
			removed = append(removed, v.ChartID)
		}
	}
	return spans, removed
}
