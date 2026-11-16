// SPDX-License-Identifier: GPL-3.0-or-later

package chartemit

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpiredChartDefinitionRevival(t *testing.T) {
	for name, tc := range map[string]struct {
		option  metrix.InstrumentOption
		counter bool
	}{
		"title":                {option: metrix.WithDescription("New title")},
		"units and chart type": {option: metrix.WithUnit("bytes")},
		"family":               {option: metrix.WithChartFamily("New family")},
		"priority":             {option: metrix.WithChartPriority(200)},
		"float dimension":      {option: metrix.WithFloat(true)},
		"resolved algorithm":   {counter: true},
	} {
		for _, changeBeforeExpiry := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/change_before_expiry=%t", name, changeBeforeExpiry), func(t *testing.T) {
				f := newRevivalFixture(t, "", 2)
				observe := func(changed bool) func() {
					return func() {
						opts := []metrix.InstrumentOption{
							metrix.WithUnit("items"),
							metrix.WithDescription("Old title"),
							metrix.WithChartFamily("Old family"),
							metrix.WithChartPriority(100),
						}
						if changed && tc.option != nil {
							opts = append(opts, tc.option)
						}
						meter := f.store.Write().SnapshotMeter("").WithLabels(metrix.Label{
							Key:   "service",
							Value: "api",
						})
						if changed && tc.counter {
							meter.Counter("queue", opts...).ObserveTotal(20)
						} else {
							meter.Gauge("queue", opts...).Observe(20)
						}
					}
				}
				first := f.collect(t, observe(false))
				oldCharts, oldDims := revivalDefinitions(first.Plan())
				f.publish(t, first)
				if changeBeforeExpiry {
					// The descriptor expires, but the chart returns exactly at its expiry boundary.
					f.collect(t, nil).Abort()
					wire := f.publish(t, f.collect(t, observe(true)))
					assert.NotContains(t, wire, "CHART ")
					assert.NotContains(t, wire, "DIMENSION ")
				}
				for range 3 {
					f.collect(t, nil).Abort()
				}
				attempt := f.collect(t, observe(true))

				fresh, err := chartengine.New(chartengine.WithRuntimeStore(nil))
				require.NoError(t, err)
				expected, err := fresh.PreparePlanWithOptions(
					f.store.Read(metrix.ReadRaw(), metrix.ReadFlatten()),
					chartengine.PlanOptions{
						TemplateSet: f.set,
					},
				)
				require.NoError(t, err)
				wantCharts, wantDims := revivalDefinitions(expected.Plan())
				expected.Abort()
				require.False(
					t,
					assert.ObjectsAreEqual(oldCharts, wantCharts) && assert.ObjectsAreEqual(oldDims, wantDims),
					"fixture must change an emitted definition",
				)

				// Publication rejection must leave the old committed definition available for retry.
				for range 2 {
					charts, dims := revivalDefinitions(attempt.Plan())
					assert.Equal(t, wantCharts, charts)
					assert.Equal(t, wantDims, dims)
					assertNoRevivalRemovals(t, attempt.Plan())
					attempt.Abort()
					attempt = f.prepare(t)
				}
				wire := f.publish(t, attempt)
				assert.Contains(t, wire, "CLABEL 'service' 'api'")
				assert.NotContains(t, wire, "obsolete")
				assert.Less(t, strings.Index(wire, "CHART "), strings.Index(wire, "SET "))
				assert.Less(t, strings.Index(wire, "DIMENSION "), strings.Index(wire, "SET "))
				wire = f.publish(t, f.collect(t, observe(true)))
				assert.NotContains(t, wire, "CHART ")
				assert.NotContains(t, wire, "DIMENSION ")
			})
		}
	}
}

func TestChartRevivalExpiryBoundaries(t *testing.T) {
	for name, tc := range map[string]struct {
		ttl      uint64
		empty    int
		changed  bool
		recreate bool
	}{
		"TTL one continuous input":        {ttl: 1},
		"exact expiry boundary":           {ttl: 2, empty: 1, changed: true},
		"expired in intervening cycle":    {ttl: 2, empty: 2, changed: true, recreate: true},
		"disabled expiry":                 {empty: 4, changed: true},
		"unchanged after long output gap": {ttl: 2, empty: 8},
	} {
		t.Run(name, func(t *testing.T) {
			f := newRevivalFixture(t, "", tc.ttl)
			observe := func(unit string) func() {
				return func() { f.store.Write().SnapshotMeter("").Gauge("queue", metrix.WithUnit(unit)).Observe(20) }
			}
			f.publish(t, f.collect(t, observe("items")))
			for range tc.empty {
				f.collect(t, nil).Abort()
			}
			unit := "items"
			if tc.changed {
				unit = "bytes"
			}
			attempt := f.collect(t, observe(unit))
			charts, dims := revivalDefinitions(attempt.Plan())
			assert.Equal(t, tc.recreate, len(charts) > 0)
			assert.Equal(t, tc.recreate, len(dims) > 0)
			assertNoRevivalRemovals(t, attempt.Plan())
			f.publish(t, attempt)
		})
	}
}

func TestChartRevivalUsesLastEmittedMetadata(t *testing.T) {
	for name, tc := range map[string]struct {
		addDimension    bool
		removeDimension bool
		abortDefinition bool
	}{
		"return to published settings": {},
		"dimension addition":           {addDimension: true},
		"aborted dimension addition":   {addDimension: true, abortDefinition: true},
		"dimension retirement":         {removeDimension: true},
		"aborted dimension retirement": {removeDimension: true, abortDefinition: true},
	} {
		t.Run(name, func(t *testing.T) {
			f := newRevivalFixture(t, "", 4)
			observe := func(unit string, includeMax bool) func() {
				return func() {
					fields := []metrix.MeasureFieldSpec{{Name: "min"}}
					values := []metrix.SampleValue{10}
					if includeMax {
						fields = append(fields, metrix.MeasureFieldSpec{
							Name: "max",
						})
						values = append(values, 20)
					}
					f.store.Write().SnapshotMeter("").
						MeasureSetGauge("queue", metrix.WithDescription("Queue"), metrix.WithUnit(unit), metrix.WithMeasureSetFields(fields...)).
						ObservePoint(metrix.MeasureSetPoint{
							Values: values,
						})
				}
			}
			f.publish(t, f.collect(t, observe("items", tc.removeDimension)))
			f.collect(t, nil).Abort()
			attempt := f.collect(t, observe("bytes", tc.addDimension))
			if tc.removeDimension {
				wire := f.publish(t, attempt)
				assert.NotContains(t, wire, "CHART ")
				f.publish(t, f.collect(t, observe("bytes", false)))
				attempt = f.collect(t, observe("bytes", false))
			}
			returnUnit := "items"
			if tc.abortDefinition {
				attempt.Abort()
			} else {
				wire := f.publish(t, attempt)
				if tc.addDimension || tc.removeDimension {
					// These actions emit CHART without a CreateChartAction.
					assert.Contains(t, wire, "CHART 'collector.job.queue' '' 'Queue' 'bytes'")
					returnUnit = "bytes"
				} else {
					assert.NotContains(t, wire, "CHART ")
				}
			}
			for range 5 {
				f.collect(t, nil).Abort()
			}
			// Match the last committed wire metadata, even if a later observation differed.
			attempt = f.collect(t, observe(returnUnit, tc.addDimension && !tc.abortDefinition))
			charts, dims := revivalDefinitions(attempt.Plan())
			assert.Empty(t, charts, "the published chart definition is unchanged")
			assert.Empty(t, dims, "returning dimensions retain their published definitions")
			f.publish(t, attempt)
		})
	}
}

func TestExpiredDimensionRevivalWithLiveChart(t *testing.T) {
	for name, tc := range map[string]struct {
		options             string
		counter             bool
		wantOptions         string
		multiplier, divisor int
	}{
		"multiplier": {options: "multiplier: 3", multiplier: 3, divisor: 1, wantOptions: "type=int"},
		"divisor":    {options: "divisor: 4", multiplier: 1, divisor: 4, wantOptions: "type=int"},
		"hidden":     {options: "hidden: true", multiplier: 1, divisor: 1, wantOptions: "hidden type=int"},
		"float":      {options: "float: true", multiplier: 1, divisor: 1, wantOptions: "type=float"},
		"algorithm":  {counter: true, multiplier: 1, divisor: 1, wantOptions: "type=int"},
	} {
		for _, changeBeforeExpiry := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/change_before_expiry=%t", name, changeBeforeExpiry), func(t *testing.T) {
				template := fmt.Sprintf(`
version: v1
groups:
  - family: Queue
    metrics: [old, new, keeper]
    charts:
      - id: queue
        title: Queue
        context: queue
        units: items
        lifecycle:
          expire_after_cycles: 5
          dimensions:
            expire_after_cycles: 2
        dimensions:
          - selector: old
            name_from_label: slot
          - selector: new
            name_from_label: slot
            options: {%s}
          - selector: keeper
            name: keeper
`, tc.options)
				f := newRevivalFixture(t, template, 0)
				keeper := func() { f.store.Write().SnapshotMeter("").Gauge("keeper").Observe(1) }
				observe := func(changed bool) func() {
					return func() {
						keeper()
						meter := f.store.Write().
							SnapshotMeter("").
							WithLabels(metrix.Label{
								Key:   "slot",
								Value: "returning",
							}, metrix.Label{
								Key:   "service",
								Value: "api",
							})
						if !changed {
							meter.Gauge("old").Observe(10)
						} else if tc.counter {
							meter.Counter("new").ObserveTotal(20)
						} else {
							meter.Gauge("new").Observe(20)
						}
					}
				}
				f.publish(t, f.collect(t, observe(false)))
				f.publish(t, f.collect(t, keeper))
				if changeBeforeExpiry {
					wire := f.publish(t, f.collect(t, observe(true)))
					assert.NotContains(t, wire, "CHART ")
					assert.NotContains(t, wire, "DIMENSION ")
				}
				for range 2 {
					f.collect(t, keeper).Abort()
				}
				attempt := f.collect(t, observe(true))
				charts, dims := revivalDefinitions(attempt.Plan())
				assert.Empty(t, charts, "the parent is still active")
				require.Len(t, dims, 1)
				algorithm := chartengine.AlgorithmAbsolute
				if tc.counter {
					algorithm = chartengine.AlgorithmIncremental
				}
				assert.Equal(t, algorithm, dims[0].Algorithm)
				assert.Equal(t, "returning", dims[0].Name)
				assert.Equal(t, tc.multiplier, dims[0].Multiplier)
				assert.Equal(t, tc.divisor, dims[0].Divisor)
				assertNoRevivalRemovals(t, attempt.Plan())
				attempt.Abort()
				wire := f.publish(t, f.prepare(t))
				want := fmt.Sprintf(
					"DIMENSION 'returning' 'returning' '%s' '%d' '%d' '%s'",
					algorithm,
					tc.multiplier,
					tc.divisor,
					tc.wantOptions,
				)
				assert.Contains(t, wire, want)
				assert.NotContains(t, wire, "DIMENSION 'keeper'")
				assert.NotContains(t, wire, "obsolete")
				wire = f.publish(t, f.collect(t, observe(true)))
				assert.NotContains(t, wire, "DIMENSION ")
			})
		}
	}
}

func TestRevivedChartRetiresOnlyMissingDimensions(t *testing.T) {
	for name, tc := range map[string]struct {
		changedLabels bool
		maxDims       int
	}{
		"same labels":               {},
		"changed labels":            {changedLabels: true},
		"cap removes old dimension": {maxDims: 2},
		"cap and changed labels":    {maxDims: 2, changedLabels: true},
	} {
		t.Run(name, func(t *testing.T) {
			template := fmt.Sprintf(`
version: v1
groups:
  - family: Latency
    metrics: [latency_*]
    charts:
      - id: latency
        title: Latency
        context: latency
        units: seconds
        lifecycle:
          expire_after_cycles: 2
          dimensions:
            max_dims: %d
        dimensions:
          - selector: latency_*
            name_from_label: measure_field
`, tc.maxDims)
			f := newRevivalFixture(t, template, 0)
			observe := func(changed bool) func() {
				return func() {
					region := "east"
					if changed && tc.changedLabels {
						region = "west"
					}
					fields := []metrix.MeasureFieldSpec{{Name: "min", Float: !changed}}
					values := []metrix.SampleValue{10}
					if changed && tc.maxDims > 0 {
						fields = append(fields, metrix.MeasureFieldSpec{
							Name: "mean",
						})
						values = append(values, 15)
					}
					if !changed {
						fields = append(fields, metrix.MeasureFieldSpec{
							Name: "max",
						})
						values = append(values, 20)
					}
					f.store.Write().
						SnapshotMeter("").
						WithLabels(metrix.Label{
							Key:   "region",
							Value: region,
						}).
						MeasureSetGauge("latency", metrix.WithMeasureSetFields(fields...)).
						ObservePoint(metrix.MeasureSetPoint{
							Values: values,
						})
				}
			}
			f.publish(t, f.collect(t, observe(false)))
			for range 3 {
				f.collect(t, nil).Abort()
			}
			attempt := f.collect(t, observe(true))
			charts, dims := revivalDefinitions(attempt.Plan())
			require.Len(t, charts, 1)
			wantDims := 1
			if tc.maxDims > 0 {
				wantDims = 2
			}
			require.Len(t, dims, wantDims)
			assert.False(t, dims[0].Float, "recreation clears the previous float mode")
			var removed []string
			for _, action := range attempt.Plan().Actions {
				switch a := action.(type) {
				case chartengine.RemoveChartAction:
					t.Errorf("surviving chart removed: %#v", a)
				case chartengine.RemoveDimensionAction:
					removed = append(removed, a.Name)
				}
			}
			assert.Equal(t, []string{"max"}, removed)
			attempt.Abort()
			wire := f.publish(t, f.prepare(t))
			region := "east"
			if tc.changedLabels {
				region = "west"
			}
			assert.Contains(t, wire, "CLABEL 'region' '"+region+"'")
			assert.Contains(t, wire, "DIMENSION 'min' 'min' 'absolute' '1' '1' 'type=int'")
			assert.Contains(t, wire, "DIMENSION 'max' 'max' 'absolute' '1' '1' 'obsolete type=int'")
			assert.Equal(t, 1, strings.Count(wire, "obsolete"))
			assert.Less(t, strings.Index(wire, "DIMENSION 'min'"), strings.Index(wire, "SET 'min'"))
			wire = f.publish(t, f.collect(t, observe(true)))
			assert.NotContains(t, wire, "CHART ")
			assert.NotContains(t, wire, "DIMENSION ")
		})
	}
}

type revivalFixture struct {
	store  metrix.CollectorStore
	engine *chartengine.Engine
	set    *chartengine.TemplateSet
}

func newRevivalFixture(t *testing.T, template string, ttl uint64) *revivalFixture {
	t.Helper()
	f := &revivalFixture{
		store: metrix.NewCollectorStore(metrix.WithExpireAfterSuccessCycles(1), metrix.WithDescriptorGraceCycles(0)),
	}
	var err error
	f.engine, err = chartengine.New(chartengine.WithRuntimeStore(nil))
	require.NoError(t, err)
	if template == "" {
		f.set, err = chartengine.NewTemplateSet(
			chartengine.TemplateSetSpec{
				Policy: chartengine.EnginePolicy{
					Autogen: &chartengine.AutogenPolicy{
						Enabled:                  true,
						ExpireAfterSuccessCycles: ttl,
					},
				},
			},
		)
	} else {
		f.set, err = chartengine.NewTemplateSetYAML([]byte(template))
	}
	require.NoError(t, err)
	return f
}

func (f *revivalFixture) collect(t *testing.T, observe func()) chartengine.PlanAttempt {
	t.Helper()
	managed, ok := metrix.AsCycleManagedStore(f.store)
	require.True(t, ok)
	cycle := managed.CycleController()
	cycle.BeginCycle()
	if observe != nil {
		observe()
	}
	require.NoError(t, cycle.CommitCycleSuccess())
	return f.prepare(t)
}

func (f *revivalFixture) prepare(t *testing.T) chartengine.PlanAttempt {
	t.Helper()
	attempt, err := f.engine.PreparePlanWithOptions(
		f.store.Read(metrix.ReadRaw(), metrix.ReadFlatten()),
		chartengine.PlanOptions{
			TemplateSet: f.set,
		},
	)
	require.NoError(t, err)
	return attempt
}

func (f *revivalFixture) publish(t *testing.T, attempt chartengine.PlanAttempt) string {
	t.Helper()
	var wire bytes.Buffer
	require.NoError(
		t,
		ApplyPlan(
			netdataapi.New(&wire),
			attempt.Plan(),
			EmitEnv{
				TypeID:      "collector.job",
				UpdateEvery: 1,
				Plugin:      "go.d.plugin",
				Module:      "test",
				JobName:     "job",
			},
		),
	)
	require.NoError(t, attempt.Commit())
	return wire.String()
}

func revivalDefinitions(plan chartengine.Plan) ([]chartengine.CreateChartAction, []chartengine.CreateDimensionAction) {
	var charts []chartengine.CreateChartAction
	var dims []chartengine.CreateDimensionAction
	for _, action := range plan.Actions {
		switch a := action.(type) {
		case chartengine.CreateChartAction:
			charts = append(charts, a)
		case chartengine.CreateDimensionAction:
			dims = append(dims, a)
		}
	}
	return charts, dims
}

func assertNoRevivalRemovals(t *testing.T, plan chartengine.Plan) {
	t.Helper()
	for _, action := range plan.Actions {
		switch action.(type) {
		case chartengine.RemoveChartAction, chartengine.RemoveDimensionAction:
			t.Errorf("unexpected retirement: %#v", action)
		}
	}
}
