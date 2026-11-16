// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"fmt"
	"maps"
	"math/rand/v2"
	"reflect"
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

	j := new(newPlanJournal(9, journalSizing{}))
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

	untouched := new(newPlanJournal(9, journalSizing{}))
	committed := untouched.committedChart(chart)
	assert.Equal(t, chart.meta, committed.meta)
	assert.Equal(t, chart.dimensions, committed.dimensions)

	j := new(newPlanJournal(10, journalSizing{}))
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

func TestPlanJournalCommittedChartSeesRecordsAfterEarlierLookups(t *testing.T) {
	state := newMaterializedState()
	a, _ := state.ensureChart(nil, "a", "tpl.a", program.ChartMeta{
		Title: "A",
	}, program.LifecyclePolicy{})
	b, _ := state.ensureChart(nil, "b", "tpl.b", program.ChartMeta{
		Title: "B",
	}, program.LifecyclePolicy{})
	b.ensureDimension(nil, "b1", dimensionState{
		order:  1,
		static: true,
	})

	j := new(newPlanJournal(11, journalSizing{}))
	state.ensureChart(j, "a", "tpl.a", program.ChartMeta{
		Title: "A2",
	}, program.LifecyclePolicy{})
	assert.Equal(t, "A", j.committedChart(a).meta.Title)

	// Records appended after the first lookup are found by later ones.
	state.ensureChart(j, "b", "tpl.b", program.ChartMeta{
		Title: "B2",
	}, program.LifecyclePolicy{})
	b.removeDimension(j, "b1")
	committed := j.committedChart(b)
	assert.Equal(t, "B", committed.meta.Title)
	assert.Equal(t, []string{"b1"}, slices.Sorted(maps.Keys(committed.dimensions)))
	assert.Empty(t, b.dimensions)
	assert.Equal(t, "A", j.committedChart(a).meta.Title)
}

const cardinalitySpikeTemplateYAML = `
version: v1
groups:
  - family: Spike
    metrics:
      - svc.m
    charts:
      - id: per_id
        title: Per id
        context: per_id
        units: x
        instances:
          by_labels: [id]
        lifecycle:
          expire_after_cycles: 1
        dimensions:
          - selector: svc.m
            name: value
      - id: fanout
        title: Fanout
        context: fanout
        units: x
        lifecycle:
          dimensions:
            expire_after_cycles: 1
        dimensions:
          - selector: svc.m
            name_from_label: id
`

// TestMaterializedMapsFollowCardinalityDown checks that once a cardinality spike expires, the
// committed chart, dimension and scratch maps are rebuilt at their live size. Staging edits the
// committed maps in place and Go maps keep their capacity after deletions, so without rebuilds
// an engine would hold its peak capacity for its lifetime.
func TestMaterializedMapsFollowCardinalityDown(t *testing.T) {
	const peak, low = 1000, 10
	engine, err := New(WithRuntimeStore(nil))
	require.NoError(t, err)
	require.NoError(t, engine.LoadYAML([]byte(cardinalitySpikeTemplateYAML), 1))
	store := metrix.NewCollectorStore()
	cc := mustCycleController(t, store)
	vec := store.Write().SnapshotMeter("svc").Vec("id").Gauge("m")
	cycle := func(n int) {
		cc.BeginCycle()
		for i := range n {
			vec.WithLabelValues(fmt.Sprint(i)).Observe(1)
		}
		require.NoError(t, cc.CommitCycleSuccess())
		_, err := prepareAndCommitPlan(engine, store.Read(metrix.ReadRaw(), metrix.ReadFlatten()))
		require.NoError(t, err)
	}
	fanoutChart := func() *materializedChartState {
		for _, chart := range engine.state.materialized.charts {
			if len(chart.scratchEntries) > 0 && chart.lifecycle.Dimensions.ExpireAfterCycles > 0 {
				return chart
			}
		}
		t.Fatal("fanout chart not materialized")
		return nil
	}
	mapID := func(m any) unsafe.Pointer { return reflect.ValueOf(m).UnsafePointer() }

	cycle(peak)
	state := &engine.state.materialized
	fanout := fanoutChart()
	require.Len(t, state.charts, peak+1)
	require.Len(t, fanout.dimensions, peak)
	peakCharts, peakDims, peakScratch := mapID(state.charts), mapID(fanout.dimensions), mapID(fanout.scratchEntries)

	for range 5 {
		cycle(low)
	}
	require.Len(t, state.charts, low+1)
	require.Same(t, fanout, fanoutChart())
	require.Len(t, fanout.dimensions, low)
	assert.NotEqual(t, peakCharts, mapID(state.charts), "chart map kept its peak capacity")
	assert.NotEqual(t, peakDims, mapID(fanout.dimensions), "dimension map kept its peak capacity")
	assert.NotEqual(t, peakScratch, mapID(fanout.scratchEntries), "scratch map kept its peak capacity")
	assert.Less(t, state.chartDeletes, compactMinDeletes)
	assert.Less(t, int(fanout.dimDeletes), compactMinDeletes)
	assert.Less(t, int(fanout.scratchDeletes), compactMinDeletes)
}

// TestPlanBuildPanicLeavesCommittedState checks that a plan interrupted by a panic after its
// build staged changes restores committed state, so an engine a caller keeps using after
// recovering plans exactly like one that never panicked.
func TestPlanBuildPanicLeavesCommittedState(t *testing.T) {
	tests := map[string]struct {
		// option returns an engine option that panics once armed.
		option func(armed *bool) Option
	}{
		"route observer during the series scan": {
			option: func(armed *bool) Option {
				accepted := 0
				return WithPlanRouteDiagnosticObserver(func(d PlanRouteDiagnostic) {
					if *armed && d.Decision == PlanRouteAccepted {
						if accepted++; accepted == 3 {
							panic("route observer")
						}
					}
				})
			},
		},
		"sample observer after a successful build": {
			option: func(armed *bool) Option {
				return WithRuntimeSampleObserver(func(PlanRuntimeSample) {
					if *armed {
						panic("sample observer")
					}
				})
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			armed := false
			panicking, err := New(WithRuntimeStore(nil), test.option(&armed))
			require.NoError(t, err)
			twin, err := New(WithRuntimeStore(nil), test.option(new(bool)))
			require.NoError(t, err)
			for _, e := range []*Engine{panicking, twin} {
				require.NoError(t, e.LoadYAML([]byte(cardinalitySpikeTemplateYAML), 1))
			}
			store := metrix.NewCollectorStore()
			cc := mustCycleController(t, store)
			vec := store.Write().SnapshotMeter("svc").Vec("id").Gauge("m")
			collect := func(ids ...int) metrix.Reader {
				cc.BeginCycle()
				for _, id := range ids {
					vec.WithLabelValues(fmt.Sprint(id)).Observe(metrix.SampleValue(id))
				}
				require.NoError(t, cc.CommitCycleSuccess())
				return store.Read(metrix.ReadRaw(), metrix.ReadFlatten())
			}

			reader := collect(0, 1, 2)
			for _, e := range []*Engine{panicking, twin} {
				_, err := prepareAndCommitPlan(e, reader)
				require.NoError(t, err)
			}

			reader = collect(1, 2, 3, 4)
			before := snapshotMaterialized(panicking.state.materialized)
			armed = true
			require.Panics(t, func() { _, _ = panicking.PreparePlan(reader) })
			armed = false
			require.Equal(t, before, snapshotMaterialized(panicking.state.materialized), "panic left staged state")

			want, err := prepareAndCommitPlan(twin, reader)
			require.NoError(t, err)
			got, err := prepareAndCommitPlan(panicking, reader)
			require.NoError(t, err)
			assert.Equal(t, want, got)
		})
	}
}

func TestChartStateAdoptedScratchEntries(t *testing.T) {
	build := func(inserted, deleted int, owner *materializedChartState) *chartState {
		cs := &chartState{
			entries:      make(map[string]*dimBuildEntry),
			entriesOwner: owner,
		}
		for i := range inserted {
			cs.entries[fmt.Sprint(i)] = &dimBuildEntry{}
		}
		for i := range deleted {
			delete(cs.entries, fmt.Sprint(i))
			if owner == nil {
				cs.unownedDeletes++
			}
		}
		return cs
	}
	mapID := func(m any) unsafe.Pointer { return reflect.ValueOf(m).UnsafePointer() }
	tests := map[string]struct {
		cs      *chartState
		rebuilt bool
	}{
		"new map trimmed below its deletions is rebuilt": {cs: build(1000, 990, nil), rebuilt: true},
		"new map with few deletions is kept":             {cs: build(100, 10, nil)},
		"new map mostly kept is kept":                    {cs: build(1000, 400, nil)},
		"a chart's own map is left to commit compaction": {cs: build(1000, 990, &materializedChartState{})},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			before := mapID(test.cs.entries)
			want := maps.Clone(test.cs.entries)
			got := test.cs.adoptedScratchEntries()
			assert.Equal(t, want, got)
			assert.Equal(t, test.rebuilt, mapID(got) != before)
		})
	}
}
