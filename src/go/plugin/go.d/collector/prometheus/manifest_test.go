// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/pkg/prometheus/selector"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

// updateGolden, when set, makes the manifest test (re)write the golden files and
// skip the comparison. The goldens are a frozen baseline of the V1 collector's
// observable contract; the V2 migration (PR5) checks parity by diffing its render
// against them. Regenerate ONLY when intentionally adding a fixture or accepting a
// contract change, then review the git diff — never to silently absorb V2 drift,
// which would defeat the baseline. Hand-editing the JSON is error-prone, so this is
// the supported way to maintain them:
//
//	go test ./plugin/go.d/collector/prometheus/ -run TestCollector_compatManifest -update-golden
var updateGolden = flag.Bool("update-golden", false, "regenerate the prometheus compat-manifest golden files")

// The compat manifest captures the V1 collector's observable CONTRACT, so the V2
// migration can be verified to preserve it.
//
// manifestChart top-level fields are the HARD contract a V2 migration must
// reproduce: the chart context, its labels (incl. label_prefix), and its dims by
// semantic name with algo + the real (de-scaled) value. `soft` holds the chart
// metadata: units and family are reproduced (the writer feeds the V1 chart helpers
// into the metrix instrument meta) and ASSERTED; chart type is autogen-derived and
// only logged — V1 leaves distribution charts type-empty while autogen sets "line",
// which is equivalent (an empty type renders as line).
//
// Chart title and priority are NOT in the manifest: the writer's feed of them is
// asserted directly in writer_test.go (mm.Description / mm.ChartPriority), and autogen
// carries them through unchanged (the instrument Description becomes the chart title;
// effectiveChartPriority is the identity for positive priorities). The units/family
// parity exercised here already proves that same instrument-meta → chart-meta path.
//
// The V1 chart-ID strings and the ×1000 / ×1e6 precision divisor are INTENTIONALLY
// excluded — both change by design in V2 (autogen chart-IDs; float dimensions).
//
// Values: V1 pre-scales to int64 (×1000 / ×1e6) then de-scales (mx ÷ Div), so a V1
// value can sit up to 1/Div (≤ 1/1000) below the true value while V2 writes the true
// float directly; the comparison tolerates that ≤1e-3 truncation (manifestValueTolerance).
// V2 does no scaling arithmetic, so it adds no sub-1e-3 error of its own — a real
// divergence would be gross, not within tolerance. A gap (a dimension with no value
// this cycle, e.g. a skipped NaN summary quantile) is NOT representable in this JSON
// shape; both renderers fail loudly on one. Faithful gap handling rides with the
// deferred NaN-summary decision; the current cases have none.
type manifestChart struct {
	Context string            `json:"context"`
	Labels  map[string]string `json:"labels,omitempty"`
	Dims    []manifestDim     `json:"dims"`
	Soft    manifestSoft      `json:"soft"`
}

type manifestDim struct {
	Name  string  `json:"name"`
	Algo  string  `json:"algo"`
	Value float64 `json:"value"`
}

type manifestSoft struct {
	Units  string `json:"units"`
	Family string `json:"family"`
	Type   string `json:"type"`
}

func renderManifest(t *testing.T, charts *collectorapi.Charts, mx map[string]int64) []manifestChart {
	t.Helper()
	out := make([]manifestChart, 0, len(*charts))

	for _, ch := range *charts {
		if ch.Obsolete {
			continue
		}

		mc := manifestChart{
			Context: ch.Ctx,
			Soft:    manifestSoft{Units: ch.Units, Family: ch.Fam, Type: string(ch.Type)},
		}
		if len(ch.Labels) > 0 {
			mc.Labels = make(map[string]string, len(ch.Labels))
			for _, l := range ch.Labels {
				mc.Labels[l.Key] = l.Value
			}
		}
		for _, d := range ch.Dims {
			div := d.Div
			if div == 0 {
				div = 1
			}
			algo := "absolute"
			if d.Algo != "" {
				algo = string(d.Algo)
			}
			raw, ok := mx[d.ID]
			require.Truef(t, ok, "V1 dim %q has no value this cycle (a gap); the manifest cannot represent gaps — faithful gap handling rides with the NaN-summary decision", d.ID)
			mc.Dims = append(mc.Dims, manifestDim{
				Name:  d.Name,
				Algo:  algo,
				Value: float64(raw) / float64(div),
			})
		}
		sort.Slice(mc.Dims, func(i, j int) bool { return mc.Dims[i].Name < mc.Dims[j].Name })

		out = append(out, mc)
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Context != out[j].Context {
			return out[i].Context < out[j].Context
		}
		return manifestLabelsKey(out[i].Labels) < manifestLabelsKey(out[j].Labels)
	})

	return out
}

func manifestLabelsKey(m map[string]string) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	for _, k := range keys {
		v := m[k]
		// Length-prefixed so distinct label sets cannot collide, e.g. {"a":"b;c=d"}
		// vs {"a":"b","c":"d"}.
		fmt.Fprintf(&sb, "%d:%s=%d:%s;", len(k), k, len(v), v)
	}
	return sb.String()
}

type compatManifestCase struct {
	prepare func() *Collector
	input   string
}

// compatManifestCases is the shared fixture for the V1 and V2 compat-manifest tests:
// the same scraped input and collector config are rendered through both the V1 chart
// builder and the V2 writer + chartengine path, and checked against the same golden.
func compatManifestCases() map[string]compatManifestCase {
	return map[string]compatManifestCase{
		"gauge": {
			prepare: New,
			input: `
# HELP test_gauge_metric A gauge.
# TYPE test_gauge_metric gauge
test_gauge_metric{label1="value1"} 11
test_gauge_metric{label1="value2"} 12.5
`,
		},
		"counter": {
			prepare: New,
			input: `
# TYPE test_counter_metric_total counter
test_counter_metric_total{label1="value1"} 11
`,
		},
		"summary": {
			prepare: New,
			input: `
# TYPE test_summary_duration_seconds summary
test_summary_duration_seconds{label1="value1",quantile="0.5"} 0.25
test_summary_duration_seconds{label1="value1",quantile="0.99"} 0.5
test_summary_duration_seconds_sum{label1="value1"} 12.5
test_summary_duration_seconds_count{label1="value1"} 42
`,
		},
		"histogram": {
			prepare: New,
			input: `
# TYPE test_histogram_duration_seconds histogram
test_histogram_duration_seconds_bucket{label1="value1",le="0.1"} 4
test_histogram_duration_seconds_bucket{label1="value1",le="+Inf"} 6
test_histogram_duration_seconds_sum{label1="value1"} 2.5
test_histogram_duration_seconds_count{label1="value1"} 6
`,
		},
		"untyped_total": {
			prepare: New,
			input: `
test_untyped_metric_total{label1="value1"} 11
`,
		},
		"app": {
			prepare: func() *Collector { c := New(); c.Application = "custom_app"; return c },
			input: `
# TYPE test_gauge_metric gauge
test_gauge_metric{label1="value1"} 11
`,
		},
		"app_job_name": {
			// Application empty -> the app segment falls back to the job Name (charts.go:238-241).
			prepare: func() *Collector { c := New(); c.Name = "job_app"; return c },
			input: `
# TYPE test_gauge_metric gauge
test_gauge_metric{label1="value1"} 11
`,
		},
		"label_prefix": {
			prepare: func() *Collector { c := New(); c.LabelPrefix = "px"; return c },
			input: `
# TYPE test_gauge_metric gauge
test_gauge_metric{label1="value1"} 11
`,
		},
		"snmp_units": {
			// Special unit mappings (charts.go getChartUnits): uppercase snmp-exporter
			// names octets->bytes, pkts->packets, mtu->octets, speed->bits; underscore
			// suffix hertz->Hz.
			prepare: New,
			input: `
# TYPE ifOutOctets gauge
ifOutOctets{ifDescr="eth0"} 12345
# TYPE ifOutUcastPkts gauge
ifOutUcastPkts{ifDescr="eth0"} 678
# TYPE ifMtu gauge
ifMtu{ifDescr="eth0"} 1500
# TYPE ifHighSpeed gauge
ifHighSpeed{ifDescr="eth0"} 1000
# TYPE test_clock_hertz gauge
test_clock_hertz{cpu="0"} 2400
`,
		},
		"selector": {
			prepare: func() *Collector {
				c := New()
				c.Selector = selector.Expr{Allow: []string{"test_gauge_metric_keep"}}
				return c
			},
			input: `
# TYPE test_gauge_metric_keep gauge
test_gauge_metric_keep{label1="value1"} 11
# TYPE test_gauge_metric_drop gauge
test_gauge_metric_drop{label1="value1"} 22
`,
		},
		"info_skipped": {
			prepare: New,
			input: `
# TYPE test_metric gauge
test_metric{label1="value1"} 11
# TYPE test_metric_info gauge
test_metric_info{version="1.2.3"} 1
`,
		},
		"fallback_gauge": {
			prepare: func() *Collector {
				c := New()
				c.FallbackType.Gauge = []string{"test_untyped_metric"}
				return c
			},
			input: `
test_untyped_metric{label1="value1"} 11
`,
		},
		"fallback_counter": {
			// Untyped metric forced to counter by regex — a distinct path from the
			// _total auto-counter (independent `if` at collect.go:176); algo incremental.
			prepare: func() *Collector {
				c := New()
				c.FallbackType.Counter = []string{"test_untyped_metric"}
				return c
			},
			input: `
test_untyped_metric{label1="value1"} 11
`,
		},
	}
}

func TestCollector_compatManifest(t *testing.T) {
	for name, tc := range compatManifestCases() {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(
				func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(tc.input)) }))
			defer srv.Close()

			collr := tc.prepare()
			collr.URL = srv.URL
			require.NoError(t, collr.Init(context.Background()))

			mx := collr.Collect(context.Background())
			require.NotNil(t, mx)

			got := renderManifest(t, collr.Charts(), mx)
			data, err := json.MarshalIndent(got, "", "  ")
			require.NoError(t, err)
			data = append(data, '\n')

			path := filepath.Join("testdata", "golden", goldenName(name)+".json")
			if *updateGolden {
				require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
				require.NoError(t, os.WriteFile(path, data, 0o644))
				return
			}

			want, err := os.ReadFile(path)
			require.NoErrorf(t, err, "missing golden %q — run: go test -run TestCollector_compatManifest -update-golden ./...", path)
			assert.Equal(t, string(want), string(data))
		})
	}
}

// Config defaults the V2 migration (PR5) must preserve: update_every is the
// registered Creator default (collectorapi.Defaults); max_time_series[_per_metric]
// are New() defaults. A V2 re-registration can silently drop them.
func TestCollector_compatConfigDefaults(t *testing.T) {
	creator, ok := collectorapi.DefaultRegistry.Lookup("prometheus")
	require.True(t, ok, "prometheus collector must be registered")
	assert.Equal(t, 10, creator.Defaults.UpdateEvery, "update_every default")

	c := New()
	assert.Equal(t, 2000, c.MaxTS, "max_time_series default")
	assert.Equal(t, 200, c.MaxTSPerMetric, "max_time_series_per_metric default")
}

func goldenName(name string) string {
	return strings.NewReplacer(" ", "_", "(", "", ")", "", ">", "", "-", "_", ".", "_", "/", "_", "<", "").Replace(name)
}

// manifestValueTolerance bounds V1's pre-scale truncation: V1 stores int64(value×Div)
// then de-scales, losing up to 1/Div (≤ 1/1000) of precision, while V2 writes the true
// float. V2 does no scaling arithmetic, so it adds no sub-1e-3 error — a real divergence
// would exceed this.
const manifestValueTolerance = 1e-3

// algoString maps a chartengine algorithm to the manifest's algo string.
func algoString(a chartengine.Algorithm) string {
	if a == chartengine.AlgorithmIncremental {
		return "incremental"
	}
	return "absolute"
}

// dimValue resolves a chartengine dimension value to the float the manifest records.
// Gaps are rejected by the caller (renderManifestV2), so only real values reach here.
func dimValue(dv chartengine.UpdateDimensionValue) float64 {
	if dv.IsFloat {
		return dv.Float64
	}
	return float64(dv.Int64)
}

func matcherFunc(m matcher.Matcher) func(string) bool {
	if m == nil {
		return nil
	}
	return m.MatchString
}

func manifestLabels(m map[string]string) map[string]string {
	if len(m) == 0 {
		return nil
	}
	return maps.Clone(m)
}

// renderManifestV2 renders the V2 path into the same manifestChart shape as the V1
// renderManifest, by inspecting the chartengine plan for a store that already holds
// exactly one freshly-committed cycle of the metric-family writer's output plus the
// per-job autogen template. The create actions (context, labels, dim name+algo, soft
// fields) are emitted only on the first cycle, so a single cycle MUST be committed
// before calling this.
func renderManifestV2(t *testing.T, store metrix.CollectorStore, app string) []manifestChart {
	t.Helper()

	tmpl, err := buildChartTemplate(app)
	require.NoError(t, err)

	eng, err := chartengine.New()
	require.NoError(t, err)
	require.NoError(t, eng.LoadYAML([]byte(tmpl), 1))

	attempt, err := eng.PreparePlan(store.Read(metrix.ReadRaw(), metrix.ReadFlatten()))
	require.NoError(t, err)
	defer attempt.Abort()
	plan := attempt.Plan()
	require.NoError(t, attempt.Commit())

	type chartAcc struct {
		mc      manifestChart
		dimAlgo map[string]string
		dimVal  map[string]float64
	}
	charts := make(map[string]*chartAcc)

	for _, a := range plan.Actions {
		switch v := a.(type) {
		case chartengine.CreateChartAction:
			charts[v.ChartID] = &chartAcc{
				mc: manifestChart{
					Context: v.Meta.Context,
					Labels:  manifestLabels(v.Labels),
					Soft:    manifestSoft{Units: v.Meta.Units, Family: v.Meta.Family, Type: string(v.Meta.Type)},
				},
				dimAlgo: make(map[string]string),
				dimVal:  make(map[string]float64),
			}
		case chartengine.CreateDimensionAction:
			c := charts[v.ChartID]
			require.NotNilf(t, c, "dimension %q references unknown chart %q", v.Name, v.ChartID)
			c.dimAlgo[v.Name] = algoString(v.Algorithm)
		case chartengine.UpdateChartAction:
			c := charts[v.ChartID]
			require.NotNilf(t, c, "values reference unknown chart %q", v.ChartID)
			for _, dv := range v.Values {
				require.Falsef(t, dv.IsEmpty, "V2 dim %q is a gap; the manifest cannot represent gaps — faithful gap handling rides with the NaN-summary decision", dv.Name)
				c.dimVal[dv.Name] = dimValue(dv)
			}
		}
	}

	out := make([]manifestChart, 0, len(charts))
	for _, c := range charts {
		for name, algo := range c.dimAlgo {
			c.mc.Dims = append(c.mc.Dims, manifestDim{Name: name, Algo: algo, Value: c.dimVal[name]})
		}
		sort.Slice(c.mc.Dims, func(i, j int) bool { return c.mc.Dims[i].Name < c.mc.Dims[j].Name })
		out = append(out, c.mc)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Context != out[j].Context {
			return out[i].Context < out[j].Context
		}
		return manifestLabelsKey(out[i].Labels) < manifestLabelsKey(out[j].Labels)
	})
	return out
}

// TestCollector_compatManifestV2 proves the V2 path (metric-family writer + per-job
// autogen template rendered by chartengine) reproduces the V1 collector's observable
// contract captured in the goldens: identical chart contexts, labels, and dimensions
// (name, algorithm, value), plus units and family. Only chart type is logged rather
// than asserted — V1 leaves distribution charts type-empty while autogen sets "line"
// (equivalent) — matching manifestChart's hard/soft split.
func TestCollector_compatManifestV2(t *testing.T) {
	for name, tc := range compatManifestCases() {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(
				func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(tc.input)) }))
			defer srv.Close()

			collr := tc.prepare()
			collr.URL = srv.URL
			require.NoError(t, collr.Init(context.Background()))

			mfs, err := collr.prom.Scrape()
			require.NoError(t, err)

			policy := metricFamilyWriterPolicy{
				labelPrefix:           collr.LabelPrefix,
				maxTSPerMetric:        collr.MaxTSPerMetric,
				isFallbackTypeGauge:   matcherFunc(collr.fallbackType.gauge),
				isFallbackTypeCounter: matcherFunc(collr.fallbackType.counter),
			}
			store := metrix.NewCollectorStore()
			w := newMetricFamilyWriter(store, policy, logger.New())

			cc := cycle(t, store)
			cc.BeginCycle()
			w.writeMetricFamilies(mfs)
			require.NoError(t, cc.CommitCycleSuccess())

			got := renderManifestV2(t, store, collr.application())

			data, err := os.ReadFile(filepath.Join("testdata", "golden", goldenName(name)+".json"))
			require.NoError(t, err)
			var want []manifestChart
			require.NoError(t, json.Unmarshal(data, &want))

			assertManifestParity(t, want, got)
		})
	}
}

// assertManifestParity checks the V2 render against the V1 golden. The chart set,
// labels, dimensions (name, algorithm, value), units, and family are asserted; only
// chart type is logged (autogen-derived, equivalent to V1's empty type).
func assertManifestParity(t *testing.T, want, got []manifestChart) {
	t.Helper()

	key := func(mc manifestChart) string { return mc.Context + "\x00" + manifestLabelsKey(mc.Labels) }

	wantByKey := make(map[string]manifestChart, len(want))
	for _, mc := range want {
		k := key(mc)
		_, dup := wantByKey[k]
		require.Falsef(t, dup, "duplicate golden chart key (context=%q labels=%v)", mc.Context, mc.Labels)
		wantByKey[k] = mc
	}
	gotByKey := make(map[string]manifestChart, len(got))
	for _, mc := range got {
		k := key(mc)
		_, dup := gotByKey[k]
		require.Falsef(t, dup, "duplicate V2 chart key (context=%q labels=%v)", mc.Context, mc.Labels)
		gotByKey[k] = mc
	}

	for k, w := range wantByKey {
		g, ok := gotByKey[k]
		if !assert.Truef(t, ok, "V2 is missing chart context=%q labels=%v", w.Context, w.Labels) {
			continue
		}
		assertDimsParity(t, w, g)
		// Units and family are reproduced by feeding the V1 chart helpers into the
		// metrix instrument meta, so they are asserted. Chart type is the one residual
		// difference: V1 leaves distribution charts (histogram/summary) type-empty while
		// autogen sets "line" — semantically identical (an empty type renders as line),
		// so it is only logged.
		assert.Equalf(t, w.Soft.Units, g.Soft.Units, "units for context=%q", w.Context)
		assert.Equalf(t, w.Soft.Family, g.Soft.Family, "family for context=%q", w.Context)
		if w.Soft.Type != g.Soft.Type {
			t.Logf("chart type differs (cosmetic) context=%q: V1=%q V2=%q", w.Context, w.Soft.Type, g.Soft.Type)
		}
	}
	for k, g := range gotByKey {
		if _, ok := wantByKey[k]; !ok {
			assert.Failf(t, "V2 produced an unexpected chart", "context=%q labels=%v", g.Context, g.Labels)
		}
	}
}

func assertDimsParity(t *testing.T, w, g manifestChart) {
	t.Helper()

	assert.Equalf(t, dimNames(w.Dims), dimNames(g.Dims), "dim names for context=%q", w.Context)

	gotDims := make(map[string]manifestDim, len(g.Dims))
	for _, d := range g.Dims {
		gotDims[d.Name] = d
	}
	for _, wd := range w.Dims {
		gd, ok := gotDims[wd.Name]
		if !ok {
			continue // already reported by the dim-names assertion
		}
		assert.Equalf(t, wd.Algo, gd.Algo, "algo for dim %q in context=%q", wd.Name, w.Context)
		assert.InDeltaf(t, wd.Value, gd.Value, manifestValueTolerance, "value for dim %q in context=%q", wd.Name, w.Context)
	}
}

func dimNames(dims []manifestDim) []string {
	names := make([]string, 0, len(dims))
	for _, d := range dims {
		names = append(names, d.Name)
	}
	sort.Strings(names)
	return names
}
