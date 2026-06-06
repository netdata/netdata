// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"context"
	"encoding/json"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/prometheus/selector"
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
// migration (PR5) can be verified to preserve it.
//
// manifestChart top-level fields are the HARD contract a V2 migration must
// reproduce: the chart context, its labels (incl. label_prefix), and its dims by
// semantic name with algo + the real (de-scaled) value. `soft` holds best-effort
// fields autogen may derive differently (units/family/type). The V1 chart-ID
// strings and the ×1000 / ×1e6 precision divisor are INTENTIONALLY excluded — both
// change by design in V2 (autogen chart-IDs; float dimensions). Values are
// de-scaled (mx ÷ Div) to the real number V2's float dims emit directly.
//
// Note: V1 scales values into int64 (×1000 / ×1e6), so sub-precision values are
// truncated (0.00147889 → 1 → 0.001). The manifest records V1's truncated value;
// the PR5 verification must diff values float-tolerantly (V2 is more precise).
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

func renderManifest(charts *collectorapi.Charts, mx map[string]int64) []manifestChart {
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
			mc.Dims = append(mc.Dims, manifestDim{
				Name:  d.Name,
				Algo:  algo,
				Value: float64(mx[d.ID]) / float64(div),
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
		sb.WriteString(k + "=" + m[k] + ";")
	}
	return sb.String()
}

func TestCollector_compatManifest(t *testing.T) {
	tests := map[string]struct {
		prepare func() *Collector
		input   string
	}{
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

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(
				func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(tc.input)) }))
			defer srv.Close()

			collr := tc.prepare()
			collr.URL = srv.URL
			require.NoError(t, collr.Init(context.Background()))

			mx := collr.Collect(context.Background())
			require.NotNil(t, mx)

			got := renderManifest(collr.Charts(), mx)
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
