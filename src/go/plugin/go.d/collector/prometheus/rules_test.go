// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/prometheus/selector"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

func TestCollector_InitRulesValidation(t *testing.T) {
	tests := map[string]Config{
		"invalid relabel action": {
			HTTPConfig: webHTTPConfig("http://127.0.0.1:9090/metrics"),
			LabelRelabel: []RelabelRule{
				{Action: "bad", TargetLabel: "x"},
			},
		},
		"invalid context type": {
			HTTPConfig: webHTTPConfig("http://127.0.0.1:9090/metrics"),
			ContextRules: []ContextRule{
				{Match: ".*", Type: "bad"},
			},
		},
		"empty dimension template": {
			HTTPConfig: webHTTPConfig("http://127.0.0.1:9090/metrics"),
			DimensionRules: []DimensionRule{
				{Match: ".*", Dimension: ""},
			},
		},
		"reserved dimension label quantile": {
			HTTPConfig: webHTTPConfig("http://127.0.0.1:9090/metrics"),
			DimensionRules: []DimensionRule{
				{Match: ".*", Dimension: "${quantile}"},
			},
		},
		"reserved dimension label le": {
			HTTPConfig: webHTTPConfig("http://127.0.0.1:9090/metrics"),
			DimensionRules: []DimensionRule{
				{Match: ".*", Dimension: "${le}"},
			},
		},
	}

	for name, cfg := range tests {
		t.Run(name, func(t *testing.T) {
			c := New()
			c.Config = cfg
			assert.Error(t, c.Init(context.Background()))
		})
	}
}

func TestCollector_ApplyRelabel(t *testing.T) {
	c := New()
	c.labelRelabelRules = []compiledRelabelRule{
		{
			sourceLabels: []string{"project"},
			re:           mustCompile(`org/([^/]+)/(.+)`),
			targetLabel:  "gitlab_group",
			replacement:  "$1",
			action:       relabelReplace,
		},
		{
			sourceLabels: []string{"project"},
			re:           mustCompile(`org/([^/]+)/(.+)`),
			targetLabel:  "gitlab_project",
			replacement:  "$2",
			action:       relabelReplace,
		},
		{
			re:     mustCompile(`^project$`),
			action: relabelLabelDrop,
		},
	}

	in := labels.Labels{
		{Name: "project", Value: "org/cloud/my-service"},
		{Name: "status", Value: "success"},
	}
	out := c.applyRelabel(in)

	assert.Equal(t, "org/cloud/my-service", in.Get("project"), "original labels must not be mutated")
	assert.Equal(t, "cloud", out.Get("gitlab_group"))
	assert.Equal(t, "my-service", out.Get("gitlab_project"))
	assert.Equal(t, "", out.Get("project"))
	assert.Equal(t, "success", out.Get("status"))
}

func TestCollector_ApplyRelabel_ReplaceUsesSingleMatchExpansion(t *testing.T) {
	c := New()
	c.labelRelabelRules = []compiledRelabelRule{
		{
			sourceLabels: []string{"src"},
			re:           mustCompile(`foo`),
			targetLabel:  "dst",
			replacement:  "$0",
			action:       relabelReplace,
		},
	}

	in := labels.Labels{
		{Name: "src", Value: "foofoo"},
	}
	out := c.applyRelabel(in)

	// Prometheus replace semantics: expand replacement for the first match only.
	assert.Equal(t, "foo", out.Get("dst"))
}

func TestCollector_ApplyRelabel_LabelMapKeepOrder(t *testing.T) {
	c := New()
	c.labelRelabelRules = []compiledRelabelRule{
		{
			re:          mustCompile(`^x_(.+)$`),
			replacement: "$1",
			action:      relabelLabelMap,
		},
		{
			re:     mustCompile(`^(status|env)$`),
			action: relabelLabelKeep,
		},
	}

	in := labels.Labels{
		{Name: "x_status", Value: "success"},
		{Name: "x_env", Value: "prod"},
		{Name: "project", Value: "service-a"},
	}
	out := c.applyRelabel(in)

	assert.Equal(t, "success", out.Get("status"))
	assert.Equal(t, "prod", out.Get("env"))
	assert.Equal(t, "", out.Get("x_status"))
	assert.Equal(t, "", out.Get("x_env"))
	assert.Equal(t, "", out.Get("project"))
}

func TestDimensionRuleRender_NoCascadingSubstitution(t *testing.T) {
	rule := compiledDimensionRule{
		template: "${a}-${b}",
		vars:     []string{"a", "b"},
	}
	lbls := labels.Labels{
		{Name: "a", Value: "${b}"},
		{Name: "b", Value: "x"},
	}

	repl, consumed, ok := rule.render(lbls)
	require.True(t, ok)
	assert.Equal(t, "${b}-x", repl)
	assert.True(t, consumed["a"])
	assert.True(t, consumed["b"])
}

func TestDimensionMetricID_NoSanitizationCollision(t *testing.T) {
	chartID := "test_status-project=a"

	idWithSpace := dimensionMetricID(chartID, "a b")
	idWithUnderscore := dimensionMetricID(chartID, "a_b")

	assert.NotEqual(t, idWithSpace, idWithUnderscore)
}

func TestCollector_DimensionRulesGauge(t *testing.T) {
	var (
		mu   sync.RWMutex
		data = `
# HELP test_status Test status
# TYPE test_status gauge
test_status{project="org/cloud/my-service",status="success"} 1
test_status{project="org/cloud/my-service",status="failed"} 2
`
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.RLock()
		defer mu.RUnlock()
		_, _ = w.Write([]byte(data))
	}))
	defer srv.Close()

	c := New()
	c.URL = srv.URL
	c.LabelRelabel = []RelabelRule{
		{SourceLabels: []string{"project"}, Regex: `org/([^/]+)/(.+)`, TargetLabel: "gitlab_group", Replacement: "$1"},
		{SourceLabels: []string{"project"}, Regex: `org/([^/]+)/(.+)`, TargetLabel: "gitlab_project", Replacement: "$2"},
		{Action: "labeldrop", Regex: "^project$"},
	}
	c.ContextRules = []ContextRule{
		{Match: "^test_status$", Context: "project_pipeline_status", Title: "Pipeline Status", Units: "pipelines", Type: "stacked"},
	}
	c.DimensionRules = []DimensionRule{
		{Match: "^test_status$", Dimension: "${status}"},
	}

	require.NoError(t, c.Init(context.Background()))

	mx := c.Collect(context.Background())
	require.NotNil(t, mx)

	chartID := "test_status-gitlab_group=cloud-gitlab_project=my-service"
	successDim := chartID + "-dim=success"
	failedDim := chartID + "-dim=failed"

	assert.Equal(t, int64(1000), mx[successDim])
	assert.Equal(t, int64(2000), mx[failedDim])

	ch := c.Charts().Get(chartID)
	require.NotNil(t, ch)
	assert.Equal(t, "prometheus.project_pipeline_status", ch.Ctx)
	assert.Equal(t, "Pipeline Status", ch.Title)
	assert.Equal(t, "pipelines", ch.Units)
	assert.Equal(t, "stacked", ch.Type.String())
	assert.True(t, ch.HasDim(successDim))
	assert.True(t, ch.HasDim(failedDim))

	mu.Lock()
	data = `
# HELP test_status Test status
# TYPE test_status gauge
test_status{project="org/cloud/my-service",status="success"} 3
`
	mu.Unlock()

	mx = c.Collect(context.Background())
	require.NotNil(t, mx)
	assert.Equal(t, int64(3000), mx[successDim])

	ch = c.Charts().Get(chartID)
	require.NotNil(t, ch)
	require.True(t, ch.HasDim(failedDim))
	assert.True(t, ch.GetDim(failedDim).Obsolete)
}

func TestCollector_DimensionRulesGaugeMissingLabelSkipsSeries(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`
# HELP test_status Test status
# TYPE test_status gauge
test_status{project="a",status="success"} 1
test_status{project="a"} 2
`))
	}))
	defer srv.Close()

	c := New()
	c.URL = srv.URL
	c.DimensionRules = []DimensionRule{
		{Match: "^test_status$", Dimension: "${status}"},
	}

	require.NoError(t, c.Init(context.Background()))
	mx := c.Collect(context.Background())
	require.NotNil(t, mx)

	chartID := "test_status-project=a"
	successDim := chartID + "-dim=success"
	valueDim := chartID + "-dim=value"

	assert.Equal(t, int64(1000), mx[successDim])
	_, exists := mx[valueDim]
	assert.False(t, exists, "series with missing template labels must be skipped")
}

func TestCollector_DimensionRulesCounter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`
# HELP test_requests_total Test requests
# TYPE test_requests_total counter
test_requests_total{project="a",status="success"} 10
test_requests_total{project="a",status="failed"} 2
`))
	}))
	defer srv.Close()

	c := New()
	c.URL = srv.URL
	c.DimensionRules = []DimensionRule{
		{Match: "^test_requests_total$", Dimension: "${status}"},
	}

	require.NoError(t, c.Init(context.Background()))
	mx := c.Collect(context.Background())
	require.NotNil(t, mx)

	chartID := "test_requests_total-project=a"
	successDim := chartID + "-dim=success"
	failedDim := chartID + "-dim=failed"

	assert.Equal(t, int64(10000), mx[successDim])
	assert.Equal(t, int64(2000), mx[failedDim])

	ch := c.Charts().Get(chartID)
	require.NotNil(t, ch)
	require.True(t, ch.HasDim(successDim))
	require.True(t, ch.HasDim(failedDim))
	assert.Equal(t, collectorapi.Incremental, ch.GetDim(successDim).Algo)
	assert.Equal(t, collectorapi.Incremental, ch.GetDim(failedDim).Algo)
}

func TestCollector_DimensionRulesSummary(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`
# HELP test_latency_seconds Test latency
# TYPE test_latency_seconds summary
test_latency_seconds{project="a",status="success",quantile="0.5"} 1
test_latency_seconds{project="a",status="success",quantile="0.9"} 2
test_latency_seconds_sum{project="a",status="success"} 3
test_latency_seconds_count{project="a",status="success"} 4
test_latency_seconds{project="a",status="failed",quantile="0.5"} 5
test_latency_seconds{project="a",status="failed",quantile="0.9"} 6
test_latency_seconds_sum{project="a",status="failed"} 7
test_latency_seconds_count{project="a",status="failed"} 8
`))
	}))
	defer srv.Close()

	c := New()
	c.URL = srv.URL
	c.DimensionRules = []DimensionRule{
		{Match: "^test_latency_seconds$", Dimension: "${status}"},
	}

	require.NoError(t, c.Init(context.Background()))
	mx := c.Collect(context.Background())
	require.NotNil(t, mx)

	chartID := "test_latency_seconds-project=a"
	q50Success := chartID + "_quantile=0.5-dim=success"
	q90Failed := chartID + "_quantile=0.9-dim=failed"
	sumSuccess := chartID + "_sum-dim=success"
	countFailed := chartID + "_count-dim=failed"

	assert.Equal(t, int64(1000000), mx[q50Success])
	assert.Equal(t, int64(6000000), mx[q90Failed])
	assert.Equal(t, int64(3000), mx[sumSuccess])
	assert.Equal(t, int64(8), mx[countFailed])

	require.NotNil(t, c.Charts().Get(chartID))
	require.NotNil(t, c.Charts().Get(chartID+"_sum"))
	require.NotNil(t, c.Charts().Get(chartID+"_count"))
}

func TestCollector_DimensionRulesHistogram(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`
# HELP test_duration_seconds Test duration
# TYPE test_duration_seconds histogram
test_duration_seconds_bucket{project="a",status="success",le="0.5"} 1
test_duration_seconds_bucket{project="a",status="success",le="+Inf"} 2
test_duration_seconds_sum{project="a",status="success"} 3
test_duration_seconds_count{project="a",status="success"} 4
test_duration_seconds_bucket{project="a",status="failed",le="0.5"} 5
test_duration_seconds_bucket{project="a",status="failed",le="+Inf"} 6
test_duration_seconds_sum{project="a",status="failed"} 7
test_duration_seconds_count{project="a",status="failed"} 8
`))
	}))
	defer srv.Close()

	c := New()
	c.URL = srv.URL
	c.DimensionRules = []DimensionRule{
		{Match: "^test_duration_seconds$", Dimension: "${status}"},
	}

	require.NoError(t, c.Init(context.Background()))
	mx := c.Collect(context.Background())
	require.NotNil(t, mx)

	chartID := "test_duration_seconds-project=a"
	bucketSuccess := chartID + "_bucket=0.5-dim=success"
	bucketFailed := chartID + "_bucket=+Inf-dim=failed"
	sumFailed := chartID + "_sum-dim=failed"
	countSuccess := chartID + "_count-dim=success"

	assert.Equal(t, int64(1), mx[bucketSuccess])
	assert.Equal(t, int64(6), mx[bucketFailed])
	assert.Equal(t, int64(7000), mx[sumFailed])
	assert.Equal(t, int64(4), mx[countSuccess])

	require.NotNil(t, c.Charts().Get(chartID))
	require.NotNil(t, c.Charts().Get(chartID+"_sum"))
	require.NotNil(t, c.Charts().Get(chartID+"_count"))
}

func TestCollector_ContextRulesFirstMatchWins(t *testing.T) {
	c := New()
	rules, err := compileContextRules([]ContextRule{
		{
			Match:   "^test_.*$",
			Context: "general",
			Title:   "General Title",
			Units:   "general_units",
			Type:    "area",
		},
		{
			Match:   "^test_counter_total$",
			Context: "specific",
			Title:   "Specific Title",
			Units:   "specific_units",
			Type:    "stacked",
		},
	})
	require.NoError(t, err)

	c.contextRules = rules
	c.Application = "gitlab_ci"

	title, ctx, units, cType := c.chartMeta("test_counter_total", "help", "events/s", collectorapi.Line)
	assert.Equal(t, "General Title", title)
	assert.Equal(t, "prometheus.gitlab_ci.general", ctx)
	assert.Equal(t, "general_units", units)
	assert.Equal(t, collectorapi.Area, cType)
}

func TestCollector_SelectorGroupsValidation(t *testing.T) {
	tests := map[string]Config{
		"duplicate group name": {
			HTTPConfig: webHTTPConfig("http://127.0.0.1:9090/metrics"),
			SelectorGroups: []SelectorGroup{
				{Name: "dup"},
				{Name: "dup"},
			},
		},
		"sanitized group name collision": {
			HTTPConfig: webHTTPConfig("http://127.0.0.1:9090/metrics"),
			SelectorGroups: []SelectorGroup{
				{Name: "prod team"},
				{Name: "prod_team"},
			},
		},
		"invalid group selector": {
			HTTPConfig: webHTTPConfig("http://127.0.0.1:9090/metrics"),
			SelectorGroups: []SelectorGroup{
				{
					Name: "bad",
					Selector: selector.Expr{
						Allow: []string{`name{label=#"value"}`},
					},
				},
			},
		},
	}

	for name, cfg := range tests {
		t.Run(name, func(t *testing.T) {
			c := New()
			c.Config = cfg
			assert.Error(t, c.Init(context.Background()))
		})
	}
}

func TestCollector_SelectorGroupsSingleScrapeMultiViews(t *testing.T) {
	var requests atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte(`
# HELP test_pipeline_status Pipeline status
# TYPE test_pipeline_status gauge
test_pipeline_status{project="org/cloud/my-service",status="success"} 1
test_pipeline_status{project="org/cloud/my-service",status="failed"} 2
`))
	}))
	defer srv.Close()

	c := New()
	c.URL = srv.URL
	c.Application = "gitlab_ci"
	c.SelectorGroups = []SelectorGroup{
		{
			Name: "projects",
			Selector: selector.Expr{
				Allow: []string{"test_pipeline_status"},
			},
			LabelRelabel: []RelabelRule{
				{SourceLabels: []string{"project"}, Regex: `org/([^/]+)/(.+)`, TargetLabel: "gitlab_group", Replacement: "$1"},
				{SourceLabels: []string{"project"}, Regex: `org/([^/]+)/(.+)`, TargetLabel: "gitlab_project", Replacement: "$2"},
				{Action: "labeldrop", Regex: "^project$"},
			},
			ContextRules: []ContextRule{
				{Match: "^test_pipeline_status$", Context: "project_pipeline_status", Type: "stacked"},
			},
			DimensionRules: []DimensionRule{
				{Match: "^test_pipeline_status$", Dimension: "${status}"},
			},
		},
		{
			Name: "overview",
			Selector: selector.Expr{
				Allow: []string{"test_pipeline_status"},
			},
			LabelRelabel: []RelabelRule{
				{Action: "labeldrop", Regex: "^project$"},
			},
			ContextRules: []ContextRule{
				{Match: "^test_pipeline_status$", Context: "pipeline_status_overview", Type: "stacked"},
			},
			DimensionRules: []DimensionRule{
				{Match: "^test_pipeline_status$", Dimension: "${status}"},
			},
		},
	}

	require.NoError(t, c.Init(context.Background()))
	mx := c.Collect(context.Background())
	require.NotNil(t, mx)
	assert.Equal(t, int32(1), requests.Load(), "single collect should scrape only once")

	assert.NotNil(t, c.Charts().Get("sg_projects_test_pipeline_status-gitlab_group=cloud-gitlab_project=my-service"))
	assert.NotNil(t, c.Charts().Get("sg_overview_test_pipeline_status"))
}

func TestCollector_SelectorGroupsOverlapIsAllowed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`
# HELP test_pipeline_status Pipeline status
# TYPE test_pipeline_status gauge
test_pipeline_status{project="org/cloud/my-service",status="success"} 1
`))
	}))
	defer srv.Close()

	c := New()
	c.URL = srv.URL
	c.SelectorGroups = []SelectorGroup{
		{
			Name: "projects",
			Selector: selector.Expr{
				Allow: []string{"test_pipeline_status"},
			},
		},
		{
			Name: "overview",
			Selector: selector.Expr{
				Allow: []string{"test_pipeline_status"},
			},
		},
	}

	require.NoError(t, c.Init(context.Background()))
	require.NotNil(t, c.Collect(context.Background()))
	require.NotNil(t, c.Collect(context.Background()))
}

func TestCollector_UntypedWithFallbackTypeGaugeDimensionRules(t *testing.T) {
	// Untyped metrics treated as gauge via fallback_type should support
	// label_relabel, context_rules, and dimension_rules — same as native gauge metrics.
	// label_relabel drops "project" label; the chart ID must not contain "project=" as proof.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`
# HELP test_status Test status (untyped)
test_status{project="a",status="success"} 1
test_status{project="a",status="failed"} 2
`))
	}))
	defer srv.Close()

	c := New()
	c.URL = srv.URL
	c.FallbackType.Gauge = []string{"test_status"}
	c.LabelRelabel = []RelabelRule{
		{Action: "labeldrop", Regex: "^project$"},
	}
	c.ContextRules = []ContextRule{
		{Match: "^test_status$", Context: "pipeline_status", Title: "Pipeline Status", Units: "pipelines", Type: "stacked"},
	}
	c.DimensionRules = []DimensionRule{
		{Match: "^test_status$", Dimension: "${status}"},
	}

	require.NoError(t, c.Init(context.Background()))
	mx := c.Collect(context.Background())
	require.NotNil(t, mx)

	// "project" was dropped by label_relabel, so the chart has no instance labels.
	chartID := "test_status"
	successDim := chartID + "-dim=success"
	failedDim := chartID + "-dim=failed"

	assert.Equal(t, int64(1000), mx[successDim])
	assert.Equal(t, int64(2000), mx[failedDim])

	ch := c.Charts().Get(chartID)
	require.NotNil(t, ch)
	assert.Equal(t, "prometheus.pipeline_status", ch.Ctx)
	assert.Equal(t, "Pipeline Status", ch.Title)
	assert.Equal(t, "pipelines", ch.Units)
	assert.Equal(t, "stacked", ch.Type.String())
	assert.True(t, ch.HasDim(successDim))
	assert.True(t, ch.HasDim(failedDim))
	// Gauge dims use the default algo (empty string = absolute in Netdata).
	assert.NotEqual(t, collectorapi.Incremental, ch.GetDim(successDim).Algo)
	// Verify label_relabel actually dropped "project" from chart labels.
	for _, lbl := range ch.Labels {
		assert.NotEqual(t, "project", lbl.Key, "label_relabel should have dropped project label")
	}
}

func TestCollector_UntypedWithFallbackTypeCounterDimensionRules(t *testing.T) {
	// Untyped metrics treated as counter via fallback_type should fully support dimension_rules.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`
# HELP test_requests_total Test requests (untyped)
test_requests_total{project="a",status="success"} 10
test_requests_total{project="a",status="failed"} 2
`))
	}))
	defer srv.Close()

	c := New()
	c.URL = srv.URL
	c.FallbackType.Counter = []string{"test_requests_total"}
	c.DimensionRules = []DimensionRule{
		{Match: "^test_requests_total$", Dimension: "${status}"},
	}

	require.NoError(t, c.Init(context.Background()))
	mx := c.Collect(context.Background())
	require.NotNil(t, mx)

	chartID := "test_requests_total-project=a"
	successDim := chartID + "-dim=success"
	failedDim := chartID + "-dim=failed"

	assert.Equal(t, int64(10000), mx[successDim])
	assert.Equal(t, int64(2000), mx[failedDim])

	ch := c.Charts().Get(chartID)
	require.NotNil(t, ch)
	require.True(t, ch.HasDim(successDim))
	require.True(t, ch.HasDim(failedDim))
	// Incremental (counter) algo expected
	assert.Equal(t, collectorapi.Incremental, ch.GetDim(successDim).Algo)
}

func TestCollector_UntypedWithoutFallbackTypeIgnored(t *testing.T) {
	// Untyped metrics without a matching fallback_type should produce no charts,
	// even when dimension_rules are configured — same behavior as before.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`
# HELP test_other_metric Some untyped metric
test_other_metric{label="x"} 42
`))
	}))
	defer srv.Close()

	c := New()
	c.URL = srv.URL
	// fallback_type only matches a different metric name, so test_other_metric is ignored
	c.FallbackType.Gauge = []string{"something_else"}
	c.DimensionRules = []DimensionRule{
		{Match: ".*", Dimension: "${label}"},
	}

	require.NoError(t, c.Init(context.Background()))
	c.Collect(context.Background())
	// No charts should be created for unmatched untyped metrics.
	// Checking chart count (not mx) distinguishes "no matching metrics" from scrape failure.
	assert.Empty(t, *c.Charts())
}

func TestCollector_HistogramStructuralDims(t *testing.T) {
	// Verifies that bucket/sum/count dims are pre-declared in the chart (withDims=true path).
	// Without this, values in mx are silently dropped by the plugin wire protocol.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`
# HELP test_latency_seconds Latency
# TYPE test_latency_seconds histogram
test_latency_seconds_bucket{le="0.1"} 1
test_latency_seconds_bucket{le="0.5"} 2
test_latency_seconds_bucket{le="+Inf"} 3
test_latency_seconds_sum 1.5
test_latency_seconds_count 3
`))
	}))
	defer srv.Close()

	c := New()
	c.URL = srv.URL
	require.NoError(t, c.Init(context.Background()))
	require.NotNil(t, c.Collect(context.Background()))

	id := "test_latency_seconds"
	ch := c.Charts().Get(id)
	require.NotNil(t, ch)
	assert.True(t, ch.HasDim(id+"_bucket=0.1"), "bucket dim must be pre-declared")
	assert.True(t, ch.HasDim(id+"_bucket=0.5"), "bucket dim must be pre-declared")
	assert.True(t, ch.HasDim(id+"_bucket=+Inf"), "bucket dim must be pre-declared")

	chSum := c.Charts().Get(id + "_sum")
	require.NotNil(t, chSum)
	assert.True(t, chSum.HasDim(id+"_sum"), "sum dim must be pre-declared")

	chCount := c.Charts().Get(id + "_count")
	require.NotNil(t, chCount)
	assert.True(t, chCount.HasDim(id+"_count"), "count dim must be pre-declared")
}

func TestCollector_SummaryStructuralDims(t *testing.T) {
	// Verifies that quantile/sum/count dims are pre-declared in the chart (withDims=true path).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`
# HELP test_latency_seconds Latency
# TYPE test_latency_seconds summary
test_latency_seconds{quantile="0.5"} 0.1
test_latency_seconds{quantile="0.9"} 0.2
test_latency_seconds_sum 5.0
test_latency_seconds_count 10
`))
	}))
	defer srv.Close()

	c := New()
	c.URL = srv.URL
	require.NoError(t, c.Init(context.Background()))
	require.NotNil(t, c.Collect(context.Background()))

	id := "test_latency_seconds"
	ch := c.Charts().Get(id)
	require.NotNil(t, ch)
	assert.True(t, ch.HasDim(id+"_quantile=0.5"), "quantile dim must be pre-declared")
	assert.True(t, ch.HasDim(id+"_quantile=0.9"), "quantile dim must be pre-declared")

	chSum := c.Charts().Get(id + "_sum")
	require.NotNil(t, chSum)
	assert.True(t, chSum.HasDim(id+"_sum"), "sum dim must be pre-declared")

	chCount := c.Charts().Get(id + "_count")
	require.NotNil(t, chCount)
	assert.True(t, chCount.HasDim(id+"_count"), "count dim must be pre-declared")
}

func TestCollector_DimensionRulesSummaryNoGhostDims(t *testing.T) {
	// When a dimension_rule matches a summary metric, no structural (phantom) dims
	// (e.g. _quantile=0.5) should appear on the chart — only rule-based dims.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`
# HELP test_latency_seconds Test latency
# TYPE test_latency_seconds summary
test_latency_seconds{project="a",quantile="0.5"} 0.1
test_latency_seconds{project="a",quantile="0.9"} 0.2
test_latency_seconds_sum{project="a"} 5.0
test_latency_seconds_count{project="a"} 10
`))
	}))
	defer srv.Close()

	c := New()
	c.URL = srv.URL
	c.DimensionRules = []DimensionRule{
		{Match: "^test_latency_seconds$", Dimension: "${project}"},
	}

	require.NoError(t, c.Init(context.Background()))
	mx := c.Collect(context.Background())
	require.NotNil(t, mx)

	chartID := "test_latency_seconds"
	ch := c.Charts().Get(chartID)
	require.NotNil(t, ch)

	// Only rule-based dims (with -dim= suffix) must exist; no bare _quantile= dims.
	assert.NotEmpty(t, ch.Dims, "chart must have at least one rule-based dim")
	for _, dim := range ch.Dims {
		assert.Contains(t, dim.ID, "-dim=", "unexpected bare dimension ID: %s", dim.ID)
		assert.False(t, dim.Obsolete, "no dim should be obsolete after first collect: %s", dim.ID)
	}
}

func TestCollector_DimensionRulesHistogramNoGhostDims(t *testing.T) {
	// When a dimension_rule matches a histogram metric, no structural (phantom) dims
	// (e.g. _bucket=0.1) should appear on the chart — only rule-based dims.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`
# HELP test_duration_seconds Test duration
# TYPE test_duration_seconds histogram
test_duration_seconds_bucket{project="a",le="0.1"} 3
test_duration_seconds_bucket{project="a",le="0.5"} 7
test_duration_seconds_bucket{project="a",le="+Inf"} 10
test_duration_seconds_sum{project="a"} 3.5
test_duration_seconds_count{project="a"} 10
`))
	}))
	defer srv.Close()

	c := New()
	c.URL = srv.URL
	c.DimensionRules = []DimensionRule{
		{Match: "^test_duration_seconds$", Dimension: "${project}"},
	}

	require.NoError(t, c.Init(context.Background()))
	mx := c.Collect(context.Background())
	require.NotNil(t, mx)

	chartID := "test_duration_seconds"
	ch := c.Charts().Get(chartID)
	require.NotNil(t, ch)

	// Only rule-based dims (with -dim= suffix) must exist; no bare _bucket= dims.
	assert.NotEmpty(t, ch.Dims, "chart must have at least one rule-based dim")
	for _, dim := range ch.Dims {
		assert.Contains(t, dim.ID, "-dim=", "unexpected bare dimension ID: %s", dim.ID)
		assert.False(t, dim.Obsolete, "no dim should be obsolete after first collect: %s", dim.ID)
	}
}

func mustCompile(expr string) *regexp.Regexp {
	re, err := regexp.Compile(expr)
	if err != nil {
		panic(err)
	}
	return re
}

func webHTTPConfig(url string) web.HTTPConfig {
	return web.HTTPConfig{
		RequestConfig: web.RequestConfig{
			URL: url,
		},
	}
}
