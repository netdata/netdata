// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"fmt"
	"maps"
	"math/rand/v2"
	"slices"
	"testing"
	"unsafe"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine/internal/program"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const journalCoreYAML = `
version: v1
groups:
  - family: Core
    metrics: [work, depth, lat_bucket, state]
    charts:
      - id: work
        title: Work
        context: work
        units: ops
        label_promotion: [zone]
        instances:
          by_labels: [host]
        lifecycle:
          max_instances: 3
          expire_after_cycles: 2
          dimensions:
            max_dims: 3
            expire_after_cycles: 2
        dimensions:
          - selector: work
            name_from_label: dev
      - id: depth
        title: Depth
        context: depth
        units: items
        aggregation: max
        instances:
          by_labels: [host]
          optional_by_labels: [pid]
        dimensions:
          - selector: depth
            name: depth
      - id: lat
        title: Latency
        context: lat
        units: obs
        dimensions:
          - selector: lat_bucket
      - id: state
        title: State
        context: state
        units: state
        instances:
          by_labels: [host]
        dimensions:
          - selector: state
`

const journalExtraYAMLA = `
version: v1
groups:
  - family: Extra
    metrics: [extra]
    charts:
      - id: extra_a
        title: Extra A
        context: extra_a
        units: v
        instances:
          by_labels: [host]
        dimensions:
          - selector: extra
            name: a
`

const journalExtraYAMLB = `
version: v1
groups:
  - family: Extra
    metrics: [extra]
    charts:
      - id: extra_b
        title: Extra B
        context: extra_b
        units: v
        dimensions:
          - selector: extra
            name_from_label: host
`

// TestPlanAbortRestoresCommittedState drives randomized collection cycles through
// the real store and planner (caps, expiry, label changes, optional identity,
// inferred dimensions, autogen, template-set transitions and host resets) and
// aborts attempts at random. After every Abort the materialized state must equal
// its state before Prepare, and preparing again must reproduce the aborted plan.
func TestPlanAbortRestoresCommittedState(t *testing.T) {
	for seed := range uint64(40) {
		t.Run(fmt.Sprint(seed), func(t *testing.T) {
			runPlanAbortScenario(t, seed, 60)
		})
	}
}

func runPlanAbortScenario(t *testing.T, seed uint64, cycles int) {
	rng := rand.New(rand.NewPCG(seed, seed+1))
	sets := journalTemplateSets(t)
	engine, err := New(WithRuntimeStore(nil))
	require.NoError(t, err)

	store := metrix.NewCollectorStore(metrix.WithExpireAfterSuccessCycles(3))
	cc := mustCycleController(t, store)
	meter := store.Write().SnapshotMeter("")
	work := meter.Vec("host", "dev", "zone").Gauge("work")
	depth := meter.Vec("host", "pid").Gauge("depth")
	lat := meter.Histogram("lat", metrix.WithHistogramBounds(1, 5))
	state := meter.Vec("host").
		StateSet("state", metrix.WithStateSetStates("ok", "bad"), metrix.WithStateSetMode(metrix.ModeEnum))
	extra := meter.Vec("host").Gauge("extra")
	active := 0
	var latCount float64

	for cycle := range cycles {
		cc.BeginCycle()
		for _, host := range []string{"h0", "h1", "h2", "h3", "h4"} {
			if rng.IntN(10) < 3 {
				continue
			}
			for _, dev := range []string{"d0", "d1", "d2", "d3"} {
				if rng.IntN(2) == 0 {
					zone := []string{"za", "zb"}[rng.IntN(2)]
					work.WithLabelValues(host, dev, zone).Observe(metrix.SampleValue(rng.IntN(100)))
				}
			}
			pid := ""
			if rng.IntN(2) == 0 {
				pid = fmt.Sprint(rng.IntN(3))
			}
			depth.WithLabelValues(host, pid).Observe(metrix.SampleValue(rng.IntN(10)))
			state.WithLabelValues(host).Enable([]string{"ok", "bad"}[rng.IntN(2)])
			extra.WithLabelValues(host).Observe(1)
		}
		latCount += float64(rng.IntN(4))
		lat.ObservePoint(metrix.HistogramPoint{
			Count: metrix.SampleValue(latCount),
			Buckets: []metrix.BucketPoint{
				{UpperBound: 1, CumulativeCount: metrix.SampleValue(latCount / 2)},
				{UpperBound: 5, CumulativeCount: metrix.SampleValue(latCount)},
			},
		})
		for range rng.IntN(3) {
			meter.Gauge(fmt.Sprintf("auto_%d", rng.IntN(6))).Observe(1)
		}
		require.NoError(t, cc.CommitCycleSuccess())
		if rng.IntN(10) == 0 {
			active = 1 - active
		}
		opts := PlanOptions{TemplateSet: sets[active], ResetMaterialized: rng.IntN(30) == 0}
		reader := store.Read(metrix.ReadRaw(), metrix.ReadFlatten())

		before := snapshotMaterialized(engine.state.materialized)
		attempt, err := engine.PreparePlanWithOptions(reader, opts)
		require.NoError(t, err)
		if rng.IntN(3) == 0 {
			aborted := attempt.Plan()
			attempt.Abort()
			require.Equalf(
				t,
				before,
				snapshotMaterialized(engine.state.materialized),
				"cycle %d: abort left staged state",
				cycle,
			)
			attempt, err = engine.PreparePlanWithOptions(reader, opts)
			require.NoError(t, err)
			assert.Equalf(t, aborted, attempt.Plan(), "cycle %d: replan after abort differs", cycle)
		}
		require.NoError(t, attempt.Commit())
	}
}

func journalTemplateSets(t *testing.T) [2]*TemplateSet {
	t.Helper()
	decode := func(doc string) []charttpl.Group {
		spec, err := charttpl.DecodeYAML([]byte(doc))
		require.NoError(t, err)
		return spec.Groups
	}
	var sets [2]*TemplateSet
	for i, extra := range []string{journalExtraYAMLA, journalExtraYAMLB} {
		set, err := NewTemplateSet(TemplateSetSpec{
			Entries: []TemplateEntry{
				{ID: "core", ContextNamespace: "jr", Groups: decode(journalCoreYAML)},
				{ID: fmt.Sprintf("extra%d", i), ContextNamespace: "jr", Groups: decode(extra)},
			},
			FallbackContextNamespace: "jr",
			Policy: EnginePolicy{
				Autogen: &AutogenPolicy{Enabled: true, ExpireAfterSuccessCycles: 2},
			},
		})
		require.NoError(t, err)
		sets[i] = set
	}
	return sets
}

// materializedSnapshot is a deep copy of materialized state for comparing state
// across an aborted attempt. Object identities are kept as unsafe.Pointer values,
// which reflect.DeepEqual compares by address.
type materializedSnapshot map[string]chartSnapshot

type chartSnapshot struct {
	ptr          unsafe.Pointer
	header       materializedChartState
	presentation *materializedChartPresentation
	dims         map[string]dimSnapshot
	entries      map[string]entrySnapshot
}

type dimSnapshot struct {
	ptr   unsafe.Pointer
	value materializedDimensionState
}

type entrySnapshot struct {
	ptr   unsafe.Pointer
	value dimBuildEntry
}

func snapshotMaterialized(state materializedState) materializedSnapshot {
	out := make(materializedSnapshot, len(state.charts))
	for id, chart := range state.charts {
		header := *chart
		header.dimensions = nil
		header.scratchEntries = nil
		header.presentation = nil
		snap := chartSnapshot{
			ptr:          unsafe.Pointer(chart),
			header:       header,
			presentation: chart.presentation,
			dims:         make(map[string]dimSnapshot, len(chart.dimensions)),
		}
		for name, dim := range chart.dimensions {
			snap.dims[name] = dimSnapshot{ptr: unsafe.Pointer(dim), value: *dim}
		}
		if chart.scratchEntries != nil {
			snap.entries = make(map[string]entrySnapshot, len(chart.scratchEntries))
			for name, entry := range chart.scratchEntries {
				snap.entries[name] = entrySnapshot{ptr: unsafe.Pointer(entry), value: *entry}
			}
		}
		out[id] = snap
	}
	return out
}

func TestPlanJournalRollbackRestoresEveryRecordKind(t *testing.T) {
	state := newMaterializedState()
	chart, _ := state.ensureChart(nil, "a", "tpl.a", program.ChartMeta{Title: "A"}, program.LifecyclePolicy{})
	d1, _ := chart.ensureDimension(nil, "d1", dimensionState{order: 1, static: true})
	d2, _ := chart.ensureDimension(nil, "d2", dimensionState{order: 2, static: true})
	d1.lastSeenSuccessSeq, d2.lastSeenSuccessSeq, chart.lastSeenSuccessSeq = 4, 4, 4
	chart.orderedDimensionNames(nil)
	chart.replaceLabels(nil, map[string]string{"k": "v"}, nil)
	chart.scratchEntries = map[string]*dimBuildEntry{"d1": {seenSeq: 4, value: 7}}
	other, _ := state.ensureChart(nil, "b", "tpl.b", program.ChartMeta{Title: "B"}, program.LifecyclePolicy{})
	other.lastSeenSuccessSeq = 3
	before := snapshotMaterialized(state)

	j := newPlanJournal(9, journalSizing{})
	j.setChartSeen(chart, 5)
	j.setDimSeen(d1, 5)
	chart.ensureDimension(j, "d2", dimensionState{order: 7})
	entry := chart.scratchEntries["d1"]
	j.touchEntry(entry)
	entry.seenSeq, entry.value = 5, 1
	j.putEntry(chart.scratchEntries, "d3", &dimBuildEntry{seenSeq: 5})
	chart.ensureDimension(j, "d3", dimensionState{order: 3, static: true})
	chart.removeDimension(j, "d1")
	chart.orderedDimensionNames(j)
	chart.replaceLabels(j, map[string]string{"k": "w"}, nil)
	state.ensureChart(j, "a", "tpl.a", program.ChartMeta{Title: "A2"}, program.LifecyclePolicy{})
	state.recordEmittedChartDefinitions(
		j,
		[]EngineAction{CreateChartAction{ChartID: "a", Meta: program.ChartMeta{Title: "A2"}}},
	)
	chart.pruneScratchEntries(j, 5)
	j.deleteChart(state.charts, "b")
	created, _ := state.ensureChart(j, "c", "tpl.c", program.ChartMeta{Title: "C"}, program.LifecyclePolicy{})
	j.setChartSeen(created, 5)
	require.NotEqual(t, before, snapshotMaterialized(state))

	j.rollback()
	assert.Equal(t, before, snapshotMaterialized(state))
}

func TestPlanJournalCommittedChartUndoesThisBuild(t *testing.T) {
	state := newMaterializedState()
	chart, _ := state.ensureChart(nil, "a", "tpl.a", program.ChartMeta{Title: "A"}, program.LifecyclePolicy{})
	d1, _ := chart.ensureDimension(nil, "d1", dimensionState{order: 1, static: true})
	chart.ensureDimension(nil, "d2", dimensionState{order: 2, static: true})

	untouched := newPlanJournal(9, journalSizing{})
	committed := untouched.committedChart(chart)
	assert.Equal(t, chart.meta, committed.meta)
	assert.Equal(t, chart.dimensions, committed.dimensions)

	j := newPlanJournal(10, journalSizing{})
	chart.removeDimension(j, "d1")
	chart.ensureDimension(j, "d3", dimensionState{order: 3, static: true})
	state.ensureChart(j, "a", "tpl.a", program.ChartMeta{Title: "A2"}, program.LifecyclePolicy{})

	committed = j.committedChart(chart)
	assert.Equal(t, "A", committed.meta.Title)
	assert.Equal(t, []string{"d1", "d2"}, slices.Sorted(maps.Keys(committed.dimensions)))
	assert.Same(t, d1, committed.dimensions["d1"])
	assert.Equal(t, "A2", chart.meta.Title)
	assert.Equal(t, []string{"d2", "d3"}, slices.Sorted(maps.Keys(chart.dimensions)))
}
