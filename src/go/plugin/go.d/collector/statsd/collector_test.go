// SPDX-License-Identifier: GPL-3.0-or-later

package statsd

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"math"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/pkg/relabel"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartemit"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed testdata/config.json
var configJSON []byte

//go:embed testdata/config.yaml
var configYAML []byte

func TestConfigurationSerialize(t *testing.T) {
	c := New()
	collecttest.TestConfigurationSerialize(t, c, configJSON, configYAML)
	require.NoError(t, c.Init(context.Background()))
	assert.Zero(t, c.receiver.idle) // Explicit zero survives New, decode, retrieval and Init.
}

type coreFixture struct {
	c      *Collector
	cycle  metrix.CycleController
	source *collectorapi.ChartTemplateSource
	engine *chartengine.Engine
	time   time.Time
}

func newCoreFixture(t testing.TB, capacity int, idle time.Duration, opts ...metrix.CollectorStoreOption) *coreFixture {
	t.Helper()
	c := New()
	c.MaxSeries = capacity
	c.MetricIdleTimeout = confopt.Duration(idle)
	if len(opts) > 0 {
		c.store = metrix.NewCollectorStore(opts...)
	}
	f := &coreFixture{
		c:    c,
		time: time.Unix(1000, 0),
	}
	c.now = func() time.Time { return f.time }
	require.NoError(t, c.Init(context.Background()))
	require.NoError(t, c.Check(context.Background()))
	managed, ok := metrix.AsCycleManagedStore(c.MetricStore())
	require.True(t, ok)
	f.cycle = managed.CycleController()
	var err error
	f.source, err = collectorapi.NewChartTemplateSource(c)
	require.NoError(t, err)
	_, err = f.source.Capture()
	require.NoError(t, err)
	f.engine, err = chartengine.New(chartengine.WithRuntimeStore(nil))
	require.NoError(t, err)
	c.receiver.start() // Test harness supplies operational readiness; socket Run is the next stage.
	t.Cleanup(func() { c.Cleanup(context.Background()) })
	return f
}

func (f *coreFixture) ingest(t testing.TB, lines ...string) {
	t.Helper()
	for _, line := range lines {
		require.NoError(t, f.c.receiver.ingest(line, f.time), line)
	}
}

func (f *coreFixture) collect(t testing.TB, metricAbort, outputAbort bool) string {
	t.Helper()
	f.cycle.BeginCycle()
	require.NoError(t, f.c.Collect(context.Background()))
	return f.finish(t, metricAbort, outputAbort)
}

func (f *coreFixture) finish(t testing.TB, metricAbort, outputAbort bool) string {
	t.Helper()
	captured, err := f.source.Capture()
	require.NoError(t, err)
	if metricAbort {
		f.cycle.AbortCycle()
		return ""
	}
	require.NoError(t, f.cycle.CommitCycleSuccess())
	attempt, err := f.engine.PreparePlanWithOptions(
		f.c.store.Read(metrix.ReadRaw(), metrix.ReadFlatten()),
		chartengine.PlanOptions{
			TemplateSet: captured,
		},
	)
	require.NoError(t, err)
	var buf bytes.Buffer
	require.NoError(
		t,
		chartemit.ApplyPlan(
			netdataapi.New(&buf),
			attempt.Plan(),
			chartemit.EmitEnv{
				TypeID:      "statsd.test",
				UpdateEvery: 1,
				StoreFirst:  true,
			},
		),
	)
	if outputAbort {
		attempt.Abort()
		return ""
	}
	require.NoError(t, attempt.Commit())
	return buf.String()
}

func value(t testing.TB, c *Collector, name string, want float64, ls metrix.Labels) {
	t.Helper()
	got, ok := c.store.Read().Value(name, ls)
	require.True(t, ok, name)
	assert.Equal(t, want, got, name)
}

func TestMeasurementsAndGenericCharts(t *testing.T) {
	f := newCoreFixture(t, 20, time.Minute)
	f.ingest(t, "requests:2.5|c|@.5|#zone:a,nd_unit:requests", "level:10|g", "level:-2|g",
		"latency:10|ms", "latency:100|ms", "difference:-10|h|@.5", "difference:20|h",
		"members:a|s", "members:a|s", "members:b:c|s")
	wire := f.collect(t, false, false)
	value(t, f.c, "c.total.requests", 5, metrix.Labels{
		"zone": "a",
	})
	value(t, f.c, "g.value.level", 8, nil)
	value(t, f.c, "s.cardinality.members", 2, nil)
	value(t, f.c, "ms.count.latency", 2, nil)
	value(t, f.c, "ms.sum.latency", 110, nil)
	value(t, f.c, "h.count.difference", 3, nil)
	value(t, f.c, "h.sum.difference", 0, nil)
	for name, want := range map[string][]float64{"ms.values.latency": {10, 100, 55, 10, 100}, "h.values.difference": {-10, 20, 0, -10, 20}} {
		p, ok := f.c.store.Read().MeasureSet(name, nil)
		require.True(t, ok)
		require.Len(t, p.Values, 5)
		assert.Equal(t, want[:3], p.Values[:3])
		for i := 3; i < 5; i++ {
			assert.InDelta(t, want[i], p.Values[i], math.Abs(want[i])*.01)
		}
	}
	assert.Equal(t, 9, strings.Count(wire, "CHART "))
	assert.Contains(t, wire, "requests/s")
	assert.Contains(t, wire, "incremental")
	assert.Contains(t, wire, "CLABEL 'zone' 'a'")
	assert.NotContains(t, wire, "CLABEL 'nd_unit'")
	collecttest.AssertChartCoverage(t, f.c, collecttest.ChartCoverageExpectation{
		RequiredContexts: map[string][]string{
			"statsd.c.total.requests": {"requests"}, "statsd.ms.values.latency": {"min", "max", "mean", "p50", "p95"},
		},
	})
	setPointer := f.c.ChartTemplateSet()
	f.time = f.time.Add(time.Second)
	f.collect(t, false, false)
	assert.Same(t, setPointer, f.c.ChartTemplateSet())
	value(t, f.c, "c.total.requests", 5, metrix.Labels{
		"zone": "a",
	})
	value(t, f.c, "g.value.level", 8, nil)
	value(t, f.c, "s.cardinality.members", 0, nil)
	value(t, f.c, "ms.count.latency", 0, nil)
	value(t, f.c, "ms.sum.latency", 0, nil)
	p, ok := f.c.store.Read().MeasureSet("ms.values.latency", nil)
	require.True(t, ok)
	require.Len(t, p.Values, 5)
	for _, v := range p.Values {
		assert.True(t, math.IsNaN(v))
	}
	f.ingest(t, "latency:0|ms")
	f.collect(t, false, false)
	p, ok = f.c.store.Read().MeasureSet("ms.values.latency", nil)
	require.True(t, ok)
	assert.Equal(t, []float64{0, 0, 0, 0, 0}, p.Values)
}

func TestNumericFailuresAreAtomic(t *testing.T) {
	for name, tc := range map[string]struct {
		initial, bad, next, metric string
		want                       float64
	}{
		"counter": {"x:1e308|c", "x:1e308|c", "x:1|c", "c.total.x", 1e308},
		"gauge":   {"x:1e308|g", "x:+1e308|g", "x:-1e308|g", "g.value.x", 0},
		"sum":     {"x:1e308|h", "x:1e308|h", "x:-1e308|h", "h.sum.x", 0},
		"count":   {"x:0|h|@1e-308", "x:0|h|@1e-308", "x:0|h", "h.count.x", 1e308},
	} {
		t.Run(name, func(t *testing.T) {
			f := newCoreFixture(t, 1, 10*time.Second)
			f.ingest(t, tc.initial)
			f.time = f.time.Add(9 * time.Second)
			require.ErrorIs(t, f.c.receiver.ingest(tc.bad, f.time), rejectOverflow)
			for _, e := range f.c.receiver.entries {
				assert.Equal(t, time.Unix(1000, 0), e.lastInput)
			}
			f.ingest(t, tc.next)
			f.collect(t, false, false)
			value(t, f.c, tc.metric, tc.want, nil)
		})
	}
	t.Run("invalid first update cannot bind type or metadata", func(t *testing.T) {
		f := newCoreFixture(t, 1, time.Minute)
		require.ErrorIs(t, f.c.receiver.ingest("x:1e308|h|@.1|#nd_unit:bytes", f.time), rejectOverflow)
		require.Empty(t, f.c.receiver.entries)
		require.Empty(t, f.c.receiver.metadata)
		require.Empty(t, f.c.receiver.bindings)
		f.ingest(t, "x:5|g|#nd_unit:items")
		f.collect(t, false, false)
		value(t, f.c, "g.value.x", 5, nil)
	})
	t.Run("direct counter quotient", func(t *testing.T) {
		f := newCoreFixture(t, 1, time.Minute)
		f.ingest(t, "x:1e-320|c|@1e-320")
		f.collect(t, false, false)
		value(t, f.c, "c.total.x", 1, nil)
	})
}

func TestPercentileGapsKeepBasicStatistics(t *testing.T) {
	f := newCoreFixture(t, 2, time.Minute)
	f.ingest(t, "wide:1|h", "wide:1e20|h", "tiny:1|ms|@1e-40")
	f.collect(t, false, false)
	for name := range map[string]bool{"h.values.wide": true, "ms.values.tiny": true} {
		p, ok := f.c.store.Read().MeasureSet(name, nil)
		require.True(t, ok)
		for _, v := range p.Values[:3] {
			assert.True(t, finite(v))
		}
		for _, v := range p.Values[3:] {
			assert.True(t, math.IsNaN(v))
		}
	}
	value(t, f.c, "h.count.wide", 2, nil)
	value(t, f.c, "ms.count.tiny", 1e40, nil)
	f.ingest(t, "wide:10|h", "wide:100|h")
	f.collect(t, false, false)
	p, ok := f.c.store.Read().MeasureSet("h.values.wide", nil)
	require.True(t, ok)
	assert.InDelta(t, 10, p.Values[3], .1)
}

func TestIdleCapacityAndTypeTransitions(t *testing.T) {
	t.Run("pending counter keeps slot until handoff", func(t *testing.T) {
		f := newCoreFixture(t, 1, 10*time.Second)
		f.ingest(t, "x:5|c")
		f.time = f.time.Add(10 * time.Second)
		require.ErrorIs(t, f.c.receiver.ingest("y:1|c", f.time), rejectCapacity)
		f.ingest(t, "x:2|c")
		f.time = f.time.Add(10 * time.Second)
		f.collect(t, false, false)
		value(t, f.c, "c.total.x", 7, nil)
		assert.Empty(t, f.c.receiver.entries)
		assert.Empty(t, f.c.receiver.bindings)
		f.ingest(t, "x:10|g")
		f.collect(t, false, false)
		value(t, f.c, "g.value.x", 10, nil)
	})
	t.Run("expired gauge baseline requires absolute", func(t *testing.T) {
		f := newCoreFixture(t, 1, 10*time.Second)
		require.ErrorIs(t, f.c.receiver.ingest("x:+2|g", f.time), rejectBaseline)
		f.ingest(t, "x:10|g")
		f.time = f.time.Add(10 * time.Second)
		require.ErrorIs(t, f.c.receiver.ingest("x:+2|g", f.time), rejectBaseline)
		f.ingest(t, "x:20|g", "x:+2|g")
		f.collect(t, false, false)
		value(t, f.c, "g.value.x", 22, nil)
		f.time = f.time.Add(10 * time.Second)
		f.collect(t, false, false)
		_, ok := f.c.store.Read().Value("g.value.x", nil)
		assert.False(t, ok)
	})
	t.Run("name binding crosses label instances", func(t *testing.T) {
		f := newCoreFixture(t, 3, time.Minute)
		f.ingest(t, "x:1|c|#a:one")
		require.ErrorIs(t, f.c.receiver.ingest("x:1|g|#a:two", f.time), rejectType)
		f.ingest(t, "x:1|c|#a:two")
		assert.Len(t, f.c.receiver.entries, 2)
	})
	t.Run("disabled idle retains slot", func(t *testing.T) {
		f := newCoreFixture(t, 1, 0)
		f.ingest(t, "x:10|g")
		f.collect(t, false, false)
		f.time = f.time.Add(24 * time.Hour)
		require.ErrorIs(t, f.c.receiver.ingest("y:1|g", f.time), rejectCapacity)
		f.collect(t, false, false)
		value(t, f.c, "g.value.x", 10, nil)
	})
}

func TestDetachedBatchOwnership(t *testing.T) {
	f := newCoreFixture(t, 2, time.Second)
	f.ingest(t, "x:5|c", "latency:10|ms")
	f.time = f.time.Add(time.Second)
	f.cycle.BeginCycle()
	batch, err := f.c.receiver.cut(f.time)
	require.NoError(t, err)
	// New admission while the old batch is being published uses fresh receiver state.
	f.ingest(t, "x:20|g", "latency:100|ms")
	for _, m := range batch {
		f.c.writeMeasurement(m)
	}
	f.c.receiver.release(batch)
	f.finish(t, false, false)
	value(t, f.c, "c.total.x", 5, nil)
	value(t, f.c, "ms.sum.latency", 10, nil)
	f.collect(t, false, false)
	value(t, f.c, "g.value.x", 20, nil)
	value(t, f.c, "ms.sum.latency", 100, nil)
}

func TestAbortedIntervalsAreNotReplayed(t *testing.T) {
	for name, metricAbort := range map[string]bool{"metric abort": true, "publication abort": false} {
		t.Run(name, func(t *testing.T) {
			f := newCoreFixture(t, 3, time.Minute)
			f.ingest(t, "latency:10|ms", "members:a|s", "requests:2|c")
			f.collect(t, metricAbort, !metricAbort)
			f.collect(t, false, false)
			value(t, f.c, "ms.count.latency", 0, nil)
			value(t, f.c, "s.cardinality.members", 0, nil)
			value(t, f.c, "c.total.requests", 2, nil)
		})
	}
}

func TestMetadataRetentionAndRevival(t *testing.T) {
	for name, opts := range map[string][]metrix.CollectorStoreOption{
		"default retention": nil,
		"chart horizon":     {metrix.WithExpireAfterSuccessCycles(2), metrix.WithDescriptorGraceCycles(2)},
	} {
		t.Run(name, func(t *testing.T) {
			f := newCoreFixture(t, 1, time.Second, opts...)
			f.ingest(t, "x:1|g|#nd_unit:items,nd_title:Queue,nd_family:work")
			first := f.collect(t, false, false)
			assert.Contains(t, first, "items")
			f.time = f.time.Add(time.Second)
			for i := uint64(0); i < f.c.receiver.horizon; i++ {
				require.ErrorIs(t, f.c.receiver.ingest("x:2|g|#nd_unit:bytes", f.time), rejectMetadata)
				f.collect(t, false, true) // Chart publication stays at its first definition.
			}
			f.collect(t, false, true) // Reconciles successful retirement before fresh admission.
			f.ingest(t, "x:2|g|#nd_unit:bytes,nd_title:Payload,nd_family:traffic")
			wire := f.collect(t, false, false)
			assert.Contains(t, wire, "bytes")
			assert.Contains(t, wire, "Payload")
			assert.Contains(t, wire, "CHART ")
			value(t, f.c, "g.value.x", 2, nil)
		})
	}
	t.Run("in flight metric cycle retains declaration", func(t *testing.T) {
		f := newCoreFixture(t, 1, time.Second)
		f.ingest(t, "x:1|g|#nd_unit:items")
		f.time = f.time.Add(time.Second)
		f.cycle.BeginCycle()
		require.NoError(t, f.c.Collect(context.Background()))
		require.ErrorIs(t, f.c.receiver.ingest("x:2|g|#nd_unit:bytes", f.time), rejectMetadata)
		f.cycle.AbortCycle()
		f.collect(t, false, false)
		f.ingest(t, "x:2|g|#nd_unit:bytes")
		f.collect(t, false, false)
		value(t, f.c, "g.value.x", 2, nil)
	})
	t.Run("failed cohorts stay bounded", func(t *testing.T) {
		f := newCoreFixture(t, 8, time.Second)
		for n := 0; n < 100; n++ {
			for i := 0; i < 8; i++ {
				f.ingest(t, fmt.Sprintf("cohort%d.%d:1|g", n, i))
			}
			f.time = f.time.Add(time.Second)
			f.collect(t, true, false)
			assert.LessOrEqual(t, len(f.c.receiver.metadata), 16)
			assert.Empty(t, f.c.receiver.entries)
		}
	})
}

func TestProfileAliasPreparation(t *testing.T) {
	// The shared replace API operates on names/labels only. Profile selection is
	// deliberately outside this stage; the payload retains its gauge operation.
	processor, err := relabel.New(
		[]relabel.Config{
			{Action: relabel.Replace, TargetLabel: "__name__", Replacement: "shared"},
			{Action: relabel.Replace, TargetLabel: "measure_field", Replacement: ""},
		},
	)
	require.NoError(t, err)
	f := newCoreFixture(t, 1, time.Minute)
	for _, line := range []string{"a:10|g|#measure_field:sender", "b:20|g", "a:+1|g"} {
		r, err := parseRecord(line)
		require.NoError(t, err)
		ls := make([]string, 0, len(r.labels)*2)
		for _, l := range r.labels {
			ls = append(ls, l.Key, l.Value)
		}
		transformed, drop := processor.Apply(relabel.Record{
			Name:   r.name,
			Labels: labels.FromStrings(ls...),
		})
		require.False(t, drop.Dropped())
		r.name = transformed.Name
		r.labels = nil
		transformed.Labels.Range(
			func(l labels.Label) {
				r.labels = append(r.labels, metrix.Label{
					Key:   l.Name,
					Value: l.Value,
				})
			},
		)
		p, err := prepareRecord(r)
		require.NoError(t, err)
		f.c.receiver.mu.Lock()
		err = f.c.receiver.admit(p, f.time)
		f.c.receiver.mu.Unlock()
		require.NoError(t, err)
	}
	f.collect(t, false, false)
	value(t, f.c, "g.value.shared", 21, nil)
	assert.Len(t, f.c.receiver.entries, 1)
}

func TestFrameworkOutputLimitDoesNotRejectInput(t *testing.T) {
	f := newCoreFixture(t, 2, time.Minute)
	name := strings.Repeat("x", 1500)
	f.ingest(t, name+":1|g", "short:2|g")
	wire := f.collect(t, false, false)
	value(t, f.c, "g.value."+name, 1, nil)
	assert.Equal(t, 1, strings.Count(wire, "CHART "))
	assert.Contains(t, wire, "short")
	assert.NotContains(t, wire, name)
}

func TestRecordBoundariesAndMetadata(t *testing.T) {
	f := newCoreFixture(t, 4, time.Minute)
	f.ingest(t, "requests:1|c|#route:日本 a.b,nd_unit:requests,nd_title:Accepted requests,nd_family:traffic")
	for _, line := range []string{"requests:2x|c", "requests:2|g", "requests:2|c|#a:b,a:c", "requests:2|c|#nd_unit:bytes"} {
		require.Error(t, f.c.receiver.ingest(line, f.time), line)
	}
	f.ingest(t, "requests:2|c|#route:日本 a.b", "requests:4|c|#route:other,nd_unit:requests")
	wire := f.collect(t, false, false)
	value(t, f.c, "c.total.requests", 3, metrix.Labels{
		"route": "日本 a.b",
	})
	value(t, f.c, "c.total.requests", 4, metrix.Labels{
		"route": "other",
	})
	assert.Contains(t, wire, "CLABEL 'route' '日本 a.b'")
	meta, ok := f.c.store.Read().MetricMeta("c.total.requests")
	require.True(t, ok)
	assert.Equal(
		t,
		metrix.MetricMeta{
			Unit:        "requests",
			Description: "Accepted requests",
			ChartFamily: "traffic",
			Float:       true,
		},
		meta,
	)
}

func TestRetainedStringsOwnStorage(t *testing.T) {
	f := newCoreFixture(t, 1, time.Minute)
	line := "members:" + strings.Repeat("member", 5000) + "|s|#pool:main,nd_unit:users,nd_title:Distinct users,nd_family:traffic"
	f.ingest(t, line)
	start := uintptr(unsafe.Pointer(unsafe.StringData(line)))
	end := start + uintptr(len(line))
	check := func(name, text string) {
		t.Helper()
		if text == "" {
			return
		}
		address := uintptr(unsafe.Pointer(unsafe.StringData(text)))
		assert.False(t, start <= address && address < end, "%s retains the complete input record", name)
	}
	for id, e := range f.c.receiver.entries {
		check("identity name", id.name)
		check("identity labels", id.labels)
		for _, label := range e.labels {
			check("label key", label.Key)
			check("label value", label.Value)
		}
	}
	for key, meta := range f.c.receiver.metadata {
		check("declaration name", key.name)
		check("declaration type", string(key.kind))
		check("unit", meta.unit)
		check("title", meta.title)
		check("family", meta.family)
		check("encoded name", meta.encodedName)
	}
	for name, binding := range f.c.receiver.bindings {
		check("binding name", name)
		check("binding type", string(binding.kind))
	}
}

func TestRetirementUnderChurn(t *testing.T) {
	f := newCoreFixture(t, 4, time.Second)
	for generation := 0; generation < 70; generation++ {
		for i := 0; i < 4; i++ {
			f.ingest(t, fmt.Sprintf("name%d.%d:1|h|#worker:g%d", generation, i, generation))
		}
		f.time = f.time.Add(time.Second)
		f.collect(t, false, false)
		assert.Empty(t, f.c.receiver.entries)
		assert.Empty(t, f.c.receiver.bindings)
		assert.LessOrEqual(t, len(f.c.receiver.metadata), 4*int(f.c.receiver.horizon+3))
	}
	for range int(f.c.receiver.horizon) + 2 {
		f.collect(t, false, false)
	}
	assert.Empty(t, f.c.receiver.metadata)
	retained := 0
	f.c.store.Read(metrix.ReadRaw(), metrix.ReadFlatten()).
		ForEachSeries(func(_ string, _ metrix.LabelView, _ metrix.SampleValue) { retained++ })
	assert.Zero(t, retained)
}

func TestSetEstimate(t *testing.T) {
	f := newCoreFixture(t, 1, time.Minute)
	var squaredError float64
	for trial := 0; trial < 30; trial++ {
		for i := 0; i < 10000; i++ {
			f.ingest(t, fmt.Sprintf("members:t%d-member%d|s", trial, i))
		}
		f.collect(t, false, false)
		got, ok := f.c.store.Read().Value("s.cardinality.members", nil)
		require.True(t, ok)
		errorFraction := (got - 10000) / 10000
		squaredError += errorFraction * errorFraction
		f.collect(t, false, false)
		value(t, f.c, "s.cardinality.members", 0, nil)
	}
	// A deterministic regression check of typical p12 accuracy, not a universal bound.
	assert.Less(t, math.Sqrt(squaredError/30), .03)
}

func TestCollectorLifecycle(t *testing.T) {
	t.Run("no publication before readiness or after stop", func(t *testing.T) {
		c := New()
		require.NoError(t, c.Init(context.Background()))
		require.NoError(t, c.Check(context.Background()))
		require.ErrorIs(t, c.Collect(context.Background()), rejectUnavailable)
		c.receiver.start()
		require.NoError(t, c.receiver.ingest("x:1|g", time.Now()))
		c.Cleanup(context.Background())
		c.Cleanup(context.Background())
		require.ErrorIs(t, c.Collect(context.Background()), rejectUnavailable)
		require.ErrorIs(t, c.receiver.ingest("x:2|g", time.Now()), rejectUnavailable)
	})
	t.Run("cancel before cut preserves input", func(t *testing.T) {
		f := newCoreFixture(t, 1, time.Minute)
		f.ingest(t, "x:10|ms")
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		f.cycle.BeginCycle()
		require.ErrorIs(t, f.c.Collect(ctx), context.Canceled)
		f.cycle.AbortCycle()
		f.collect(t, false, false)
		value(t, f.c, "ms.sum.x", 10, nil)
	})
	for name, config := range map[string]Config{"zero cap": {MaxSeries: 0}, "negative cap": {MaxSeries: -1}, "negative idle": {MaxSeries: 1, MetricIdleTimeout: -1}} {
		t.Run(
			name,
			func(t *testing.T) { c := New(); c.Config = config; require.Error(t, c.Init(context.Background())) },
		)
	}

}

func TestConcurrentIngestionAndCollection(t *testing.T) {
	f := newCoreFixture(t, 4, 0)
	now := f.time
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			line := fmt.Sprintf("requests:1|c|#worker:%d", i)
			for n := 0; n < 1000; n++ {
				if err := f.c.receiver.ingest(line, now); err != nil {
					t.Error(err)
					return
				}
			}
		}(i)
	}
	for i := 0; i < 20; i++ {
		f.collect(t, false, false)
	}
	wg.Wait()
	f.collect(t, false, false)
	for i := 0; i < 4; i++ {
		value(t, f.c, "c.total.requests", 1000, metrix.Labels{
			"worker": fmt.Sprint(i),
		})
	}
}
