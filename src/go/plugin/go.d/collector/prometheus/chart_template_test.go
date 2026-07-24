// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
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

func TestBuildMergedChartTemplateAutogenExcludeUnion(t *testing.T) {
	catalog := loadTestCatalog(t, map[string]string{
		"first":  testProfileYAMLWithAutogenExclude("test_*", " z*", "a*", "z*"),
		"second": testProfileYAMLWithAutogenExclude("test_*", "Business Unit", "μέτρο*", "a*"),
		"all":    testProfileYAMLWithAutogenExclude("test_*", "specific*", "*"),
	})

	firstOrder, err := catalog.Resolve([]string{"first", "second"})
	require.NoError(t, err)
	secondOrder, err := catalog.Resolve([]string{"second", "first"})
	require.NoError(t, err)

	for _, profiles := range [][]promprofiles.Profile{firstOrder, secondOrder} {
		out, err := buildMergedChartTemplate("", profiles)
		require.NoError(t, err)
		spec, err := charttpl.DecodeYAML([]byte(out))
		require.NoError(t, err)
		require.NotNil(t, spec.Engine)
		require.NotNil(t, spec.Engine.Autogen)
		assert.Equal(t, []string{"Business Unit", "a*", "z*", "μέτρο*"}, spec.Engine.Autogen.Exclude)
	}

	all, err := catalog.Resolve([]string{"first", "all"})
	require.NoError(t, err)
	out, err := buildMergedChartTemplate("", all)
	require.NoError(t, err)
	spec, err := charttpl.DecodeYAML([]byte(out))
	require.NoError(t, err)
	assert.Equal(t, []string{"*"}, spec.Engine.Autogen.Exclude)
}

func TestBuildMergedChartTemplateHistogramProfileExcludesFallbackComponents(t *testing.T) {
	catalog := loadTestCatalog(t, map[string]string{
		"latency": `
match: "http_request_duration_seconds"
autogen:
  exclude:
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
