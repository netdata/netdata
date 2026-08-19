// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	metrixselector "github.com/netdata/netdata/go/plugins/pkg/metrix/selector"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/promprofiles"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildChartTemplate(t *testing.T) {
	tests := map[string]struct {
		app           string
		wantNamespace string
	}{
		"no app uses the prometheus namespace": {
			app:           "",
			wantNamespace: "prometheus",
		},
		"app is folded into the namespace with the separating dot": {
			app:           "myapp",
			wantNamespace: "prometheus.myapp",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			out, err := buildChartTemplate(tc.app)
			require.NoError(t, err)

			// Parse back through charttpl's own canonical decoder (yaml.v2 UnmarshalStrict, the path
			// chartengine uses) so the round-trip is validated against the real template contract.
			spec, err := charttpl.DecodeYAML([]byte(out))
			require.NoError(t, err)

			assert.Equal(t, charttpl.VersionV1, spec.Version)
			assert.Equal(t, tc.wantNamespace, spec.ContextNamespace, "context_namespace drives the V1-parity chart context")
			require.NotNil(t, spec.Engine)
			require.NotNil(t, spec.Engine.Autogen)
			assert.True(t, spec.Engine.Autogen.Enabled, "autogen must be enabled (no static charts)")
			assert.Equal(t, uint64(chartExpireAfterCycles), spec.Engine.Autogen.ExpireAfterSuccessCycles,
				"autogen chart expiry must mirror V1's 10-cycle stale removal")
			require.Len(t, spec.Groups, 1, "a stub group satisfies the non-empty groups requirement")
			assert.Equal(t, "prometheus", spec.Groups[0].Family)

			// chartengine must accept the generated template (compiles + publishes a revision).
			eng, err := chartengine.New()
			require.NoError(t, err)
			require.NoError(t, eng.LoadYAML([]byte(out), 1))
		})
	}
}

func TestBuildMergedChartTemplateAutogenRulesPreserveProfileScopes(t *testing.T) {
	catalog := loadTestCatalog(t, map[string]string{
		"first": testProfileYAMLWithAutogenSelector(
			"test_* !test_internal_*",
			[]string{"test_*"},
			[]string{`test_metric{environment="dev"}`},
		),
		"second": testProfileYAMLWithAutogenSelector(
			"μέτρο*",
			nil,
			[]string{"μέτρο_debug_*"},
		),
		"plain": testProfileYAML("plain_*"),
	})

	profiles, err := catalog.Resolve([]string{"second", "plain", "first"})
	require.NoError(t, err)
	out, err := buildMergedChartTemplate("", profiles)
	require.NoError(t, err)
	spec, err := charttpl.DecodeYAML([]byte(out))
	require.NoError(t, err)
	require.NotNil(t, spec.Engine)
	require.NotNil(t, spec.Engine.Autogen)
	assert.Equal(t, []charttpl.EngineAutogenRule{
		{
			Scope: "μέτρο*",
			Selector: metrixselector.Expr{
				Deny: []string{"μέτρο_debug_*"},
			},
		},
		{
			Scope: "test_* !test_internal_*",
			Selector: metrixselector.Expr{
				Allow: []string{"test_*"},
				Deny:  []string{`test_metric{environment="dev"}`},
			},
		},
	}, spec.Engine.Autogen.Rules)
}

func TestBuildMergedChartTemplateHistogramProfileSuppressesFallbackComponents(t *testing.T) {
	catalog := loadTestCatalog(t, map[string]string{
		"latency": `
match: "http_request_duration_seconds"
autogen:
  selector:
    deny:
      - "http_request_duration_seconds"
template:
  family: HTTP
  metrics:
    - http_request_duration_seconds_bucket
  charts:
    - title: Request duration
      context: request_duration
      type: heatmap
      units: observations/s
      algorithm: incremental
      dimensions:
        - selector: http_request_duration_seconds_bucket
`,
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`
# TYPE http_request_duration_seconds histogram
http_request_duration_seconds_bucket{le="0.1"} 1
http_request_duration_seconds_bucket{le="0.5"} 3
http_request_duration_seconds_bucket{le="1"} 4
http_request_duration_seconds_bucket{le="+Inf"} 4
http_request_duration_seconds_sum 1.7
http_request_duration_seconds_count 4
`))
	}))
	defer srv.Close()

	collector := New()
	collector.URL = srv.URL
	collector.Profiles = ProfilesConfig{
		Mode: profilesModeExact,
		ModeExact: &ProfilesModeConfig{
			Entries: []ProfileEntryConfig{{Name: "latency"}},
		},
	}
	collector.loadProfileCatalog = func() (promprofiles.Catalog, error) {
		return catalog, nil
	}
	require.NoError(t, collector.Init(context.Background()))
	require.NoError(t, collector.Check(context.Background()))

	cc, ok := metrix.AsCycleManagedStore(collector.MetricStore())
	require.True(t, ok)
	cc.CycleController().BeginCycle()
	require.NoError(t, collector.Collect(context.Background()))
	require.NoError(t, cc.CycleController().CommitCycleSuccess())

	rawHistogram, ok := collector.MetricStore().Read(metrix.ReadRaw()).Histogram(
		"http_request_duration_seconds",
		nil,
	)
	require.True(t, ok)
	assert.InDelta(t, 4, rawHistogram.Count, 1e-9)
	assert.InDelta(t, 1.7, rawHistogram.Sum, 1e-9)
	require.Len(t, rawHistogram.Buckets, 3)
	assert.Equal(t, metrix.BucketPoint{UpperBound: 0.1, CumulativeCount: 1}, rawHistogram.Buckets[0])
	assert.Equal(t, metrix.BucketPoint{UpperBound: 0.5, CumulativeCount: 3}, rawHistogram.Buckets[1])
	assert.Equal(t, metrix.BucketPoint{UpperBound: 1, CumulativeCount: 4}, rawHistogram.Buckets[2])

	engine, err := chartengine.New()
	require.NoError(t, err)
	require.NoError(t, engine.LoadYAML([]byte(collector.ChartTemplateYAML()), 1))

	attempt, err := engine.PreparePlan(collector.MetricStore().Read(metrix.ReadRaw(), metrix.ReadFlatten()))
	require.NoError(t, err)
	defer attempt.Abort()
	plan := attempt.Plan()

	var created []chartengine.CreateChartAction
	for _, action := range plan.Actions {
		if create, ok := action.(chartengine.CreateChartAction); ok {
			created = append(created, create)
		}
	}
	require.Len(t, created, 1)
	assert.Equal(t, "prometheus.request_duration", created[0].Meta.Context)
	assert.Equal(t, "heatmap", string(created[0].Meta.Type))
	assert.NotContains(t, created[0].ChartID, "_sum")
	assert.NotContains(t, created[0].ChartID, "_count")
}

func TestBuildMergedChartTemplateCrossNamespaceRenameLeavesProfileAutogenScope(t *testing.T) {
	profile := testRelabelProfilePatternYAML("app_*", "app_raw", "app_raw", "other_final", "app_never")
	profile = strings.Replace(profile, "template:\n", `autogen:
  selector:
    deny: ["*"]
template:
`, 1)
	catalog := loadTestCatalog(t, map[string]string{"app": profile})
	collector, srv := newProfileRelabelCollector(t, catalog, "# TYPE app_raw gauge\napp_raw 1\n", "app")
	defer srv.Close()
	require.NoError(t, collector.Init(context.Background()))
	require.NoError(t, collector.Check(context.Background()))
	collectProfileRelabelOnce(t, collector)

	reader := collector.MetricStore().Read(metrix.ReadRaw(), metrix.ReadFlatten())
	assert.InDelta(t, 1, value(t, reader, "other_final", nil), 1e-9)

	engine, err := chartengine.New()
	require.NoError(t, err)
	require.NoError(t, engine.LoadYAML([]byte(collector.ChartTemplateYAML()), 1))
	attempt, err := engine.PreparePlan(reader)
	require.NoError(t, err)
	defer attempt.Abort()

	var created []chartengine.CreateChartAction
	for _, action := range attempt.Plan().Actions {
		if create, ok := action.(chartengine.CreateChartAction); ok {
			created = append(created, create)
		}
	}
	require.Len(t, created, 1)
	assert.Equal(t, "prometheus.other_final", created[0].Meta.Context)
}

func TestBuildMergedChartTemplateAggregatesProjectedPrometheusSeries(t *testing.T) {
	catalog := loadTestCatalog(t, map[string]string{
		"litellm": `
match: "litellm_api_key_last_used_timestamp_seconds"
template:
  family: LiteLLM
  metrics:
    - litellm_api_key_last_used_timestamp_seconds
  charts:
    - id: api_key_last_used
      title: API key last used
      context: api_key_last_used
      units: seconds
      aggregation: max
      instances:
        by_labels: [team]
      dimensions:
        - selector: litellm_api_key_last_used_timestamp_seconds
          name: latest
`,
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`
# TYPE litellm_api_key_last_used_timestamp_seconds gauge
litellm_api_key_last_used_timestamp_seconds{team="team-a",api_key="key-1"} 100
litellm_api_key_last_used_timestamp_seconds{team="team-a",api_key="key-2"} 200
`))
	}))
	defer srv.Close()

	collector := New()
	collector.URL = srv.URL
	collector.Profiles = ProfilesConfig{
		Mode: profilesModeExact,
		ModeExact: &ProfilesModeConfig{
			Entries: []ProfileEntryConfig{{Name: "litellm"}},
		},
	}
	collector.loadProfileCatalog = func() (promprofiles.Catalog, error) {
		return catalog, nil
	}
	require.NoError(t, collector.Init(context.Background()))
	require.NoError(t, collector.Check(context.Background()))

	cc, ok := metrix.AsCycleManagedStore(collector.MetricStore())
	require.True(t, ok)
	cc.CycleController().BeginCycle()
	require.NoError(t, collector.Collect(context.Background()))
	require.NoError(t, cc.CycleController().CommitCycleSuccess())

	reader := collector.MetricStore().Read(metrix.ReadRaw(), metrix.ReadFlatten())
	_, ok = reader.Value("litellm_api_key_last_used_timestamp_seconds", metrix.Labels{
		"team": "team-a", "api_key": "key-1",
	})
	assert.True(t, ok, "the first full-label source series must remain in metrix")
	_, ok = reader.Value("litellm_api_key_last_used_timestamp_seconds", metrix.Labels{
		"team": "team-a", "api_key": "key-2",
	})
	assert.True(t, ok, "the second full-label source series must remain in metrix")

	engine, err := chartengine.New()
	require.NoError(t, err)
	require.NoError(t, engine.LoadYAML([]byte(collector.ChartTemplateYAML()), 1))

	attempt, err := engine.PreparePlan(reader)
	require.NoError(t, err)
	defer attempt.Abort()
	plan := attempt.Plan()

	var updates []chartengine.UpdateChartAction
	for _, action := range plan.Actions {
		if update, ok := action.(chartengine.UpdateChartAction); ok {
			updates = append(updates, update)
		}
	}
	require.Len(t, updates, 1)
	require.Len(t, updates[0].Values, 1)
	assert.Equal(t, "latest", updates[0].Values[0].Name)
	assert.Equal(t, float64(200), updates[0].Values[0].Float64)
}
