// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/executable"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/promprofiles"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
)

func TestCollector_validateConfigProfiles(t *testing.T) {
	tests := map[string]struct {
		mode     string
		profiles []string
		wantMode string
		wantIDs  []string
		wantErr  bool
	}{
		"default mode is auto": {
			wantMode: profileSelectionModeAuto,
		},
		"auto rejects explicit profiles": {
			mode:     profileSelectionModeAuto,
			profiles: []string{"demo"},
			wantErr:  true,
		},
		"exact requires explicit profiles": {
			mode:    profileSelectionModeExact,
			wantErr: true,
		},
		"combined requires explicit profiles": {
			mode:    profileSelectionModeCombined,
			wantErr: true,
		},
		"exact accepts explicit profiles": {
			mode:     profileSelectionModeExact,
			profiles: []string{"demo"},
			wantMode: profileSelectionModeExact,
			wantIDs:  []string{"demo"},
		},
		"combined accepts explicit profiles": {
			mode:     profileSelectionModeCombined,
			profiles: []string{"demo"},
			wantMode: profileSelectionModeCombined,
			wantIDs:  []string{"demo"},
		},
		"duplicate explicit profiles fail": {
			mode:     profileSelectionModeExact,
			profiles: []string{"demo", "DEMO"},
			wantErr:  true,
		},
		"empty explicit profile fails": {
			mode:     profileSelectionModeExact,
			profiles: []string{" "},
			wantErr:  true,
		},
		"unsupported mode fails": {
			mode:    "bad",
			wantErr: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			collr.URL = "http://127.0.0.1:9090/metrics"
			collr.ProfileSelectionMode = test.mode
			collr.Profiles = append([]string(nil), test.profiles...)

			err := collr.validateConfig()
			if test.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, test.mode, collr.ProfileSelectionMode)
			assert.Equal(t, test.profiles, collr.Profiles)
			assert.Equal(t, test.wantMode, collr.cfgState.profileSelectionMode)
			assert.Equal(t, test.wantIDs, collr.cfgState.profiles)
		})
	}
}

func TestCollector_CheckKeepsProbeLimitsInInternalState(t *testing.T) {
	collr := New()
	collr.URL = newMetricsServer(t, `
# TYPE demo_metric gauge
demo_metric 1
`)
	collr.ExpectedPrefix = "demo_"
	collr.MaxTS = 5

	require.NoError(t, collr.Init(context.Background()))
	require.NoError(t, collr.Check(context.Background()))

	assert.Equal(t, "demo_", collr.ExpectedPrefix)
	assert.Equal(t, 5, collr.MaxTS)
	assert.Empty(t, collr.probeState.expectedPrefix)
	assert.Zero(t, collr.probeState.maxTS)
}

func TestCollector_InitDoesNotLoadProfileCatalog(t *testing.T) {
	collr := New()
	collr.URL = "http://127.0.0.1:9090/metrics"
	called := false
	collr.loadProfileCatalog = func() (promprofiles.Catalog, error) {
		called = true
		return promprofiles.Catalog{}, errors.New("boom")
	}

	require.NoError(t, collr.Init(context.Background()))
	assert.False(t, called)
}

func TestCollector_CheckLoadProfileCatalogError(t *testing.T) {
	collr := New()
	collr.URL = newMetricsServer(t, `
# TYPE demo_metric gauge
demo_metric 1
`)
	called := false
	collr.loadProfileCatalog = func() (promprofiles.Catalog, error) {
		called = true
		return promprofiles.Catalog{}, errors.New("boom")
	}

	require.NoError(t, collr.Init(context.Background()))

	err := collr.Check(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "load profiles catalog")
	assert.True(t, called)
}

func TestCollector_InitDefaultCatalogMissingStockDirDoesNotFailGenericJob(t *testing.T) {
	oldName := executable.Name
	oldDir := executable.Directory
	executable.Name = "go.d"
	executable.Directory = t.TempDir()
	t.Cleanup(func() {
		executable.Name = oldName
		executable.Directory = oldDir
	})

	collr := New()
	collr.URL = "http://127.0.0.1:9090/metrics"
	collr.loadProfileCatalog = nil

	require.NoError(t, collr.Init(context.Background()))
}

func TestCollector_ChartTemplateYAML_RuntimeOverride(t *testing.T) {
	collr := New()
	runtime, err := buildCollectorRuntimeFromProfiles([]promprofiles.Profile{testPromProfile("demo")})
	require.NoError(t, err)

	collr.runtime = runtime

	templateYAML := collr.ChartTemplateYAML()
	collecttest.AssertChartTemplateSchema(t, templateYAML)
	assert.Contains(t, templateYAML, "demo_metric_total")
	assert.Contains(t, templateYAML, "enabled: true")
}

func TestBuildCollectorRuntimeFromProfiles_DerivesMissingChartIDFromEffectiveContext(t *testing.T) {
	profiles := []promprofiles.Profile{
		{
			Name:  "demo_a",
			Match: "demo_a_*",
			Template: charttpl.Group{
				Family:           "prometheus_curated",
				ContextNamespace: "alpha",
				Metrics:          []string{"demo_a_metric"},
				Charts: []charttpl.Chart{
					{
						Title:   "Demo A",
						Context: "requests",
						Units:   "requests/s",
						Dimensions: []charttpl.Dimension{
							{Selector: "demo_a_metric", Name: "value"},
						},
					},
				},
			},
		},
		{
			Name:  "demo_b",
			Match: "demo_b_*",
			Template: charttpl.Group{
				Family:           "prometheus_curated",
				ContextNamespace: "beta",
				Metrics:          []string{"demo_b_metric"},
				Charts: []charttpl.Chart{
					{
						Title:   "Demo B",
						Context: "requests",
						Units:   "requests/s",
						Dimensions: []charttpl.Dimension{
							{Selector: "demo_b_metric", Name: "value"},
						},
					},
				},
			},
		},
	}

	runtime, err := buildCollectorRuntimeFromProfiles(profiles)
	require.NoError(t, err)
	require.NotNil(t, runtime)
	assert.Contains(t, runtime.chartTemplateYAML, "context_namespace: prometheus")
	assert.Contains(t, runtime.chartTemplateYAML, "context_namespace: alpha")
	assert.Contains(t, runtime.chartTemplateYAML, "context_namespace: beta")
}

func TestCollector_CheckFailsWhenExactProfileMatchesNothing(t *testing.T) {
	collr := New()
	collr.URL = newMetricsServer(t, `# TYPE other_metric gauge
other_metric 1
`)
	collr.ProfileSelectionMode = profileSelectionModeExact
	collr.Profiles = []string{"demo"}
	collr.loadProfileCatalog = func() (promprofiles.Catalog, error) {
		return loadTestCatalog(t, map[string]string{
			"demo.yaml": testProfileYAML("Demo", "demo_*", "", "demo_metric_total", "demo_chart", "Demo Metric"),
		})
	}

	require.NoError(t, collr.Init(context.Background()))
	err := collr.Check(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), `selected profile "Demo" matches nothing during probe`)
}

func TestCollector_CheckAppliesProfileRelabelAndRemapsHelp(t *testing.T) {
	collr := New()
	collr.URL = newMetricsServer(t, `
# HELP demo_metric Original Help
# TYPE demo_metric gauge
demo_metric 1
`)
	collr.ProfileSelectionMode = profileSelectionModeExact
	collr.Profiles = []string{"demo"}
	collr.loadProfileCatalog = func() (promprofiles.Catalog, error) {
		return loadTestCatalog(t, map[string]string{
			"demo.yaml": testProfileYAML("Demo", "demo_metric", `
metrics_relabeling:
  - selector: demo_metric
    rules:
      - action: replace
        target_label: __name__
        replacement: renamed_metric
`, "renamed_metric", "demo_chart", "Demo Metric"),
		})
	}

	require.NoError(t, collr.Init(context.Background()))
	require.NoError(t, collr.Check(context.Background()))

	mfs, err := collr.scrapeMetricFamilies()
	require.NoError(t, err)

	assert.Nil(t, mfs.GetGauge("demo_metric"))
	mf := mfs.GetGauge("renamed_metric")
	require.NotNil(t, mf)
	assert.Equal(t, "Original Help", mf.Help())
}

func TestCollector_CheckFailsWhenProfileRelabelInvalidatesHistogramFamily(t *testing.T) {
	collr := New()
	collr.URL = newMetricsServer(t, `
# TYPE demo_duration_seconds histogram
demo_duration_seconds_bucket{le="0.1"} 1
demo_duration_seconds_bucket{le="+Inf"} 2
demo_duration_seconds_sum 0.2
demo_duration_seconds_count 2
`)
	collr.ProfileSelectionMode = profileSelectionModeExact
	collr.Profiles = []string{"demo"}
	collr.loadProfileCatalog = func() (promprofiles.Catalog, error) {
		return loadTestCatalog(t, map[string]string{
			"demo.yaml": testProfileYAML("Demo", "demo_duration_seconds*", `
metrics_relabeling:
  - selector: demo_duration_seconds_bucket
    rules:
      - action: drop
`, "demo_duration_seconds", "demo_chart", "Demo Metric"),
		})
	}

	require.NoError(t, collr.Init(context.Background()))
	err := collr.Check(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), `curated relabel leaves logical typed family "demo_duration_seconds" within profile "Demo" without a valid assembled family`)
}

func TestCollector_CheckFailsOnSelectedProfileOverlap(t *testing.T) {
	collr := New()
	collr.URL = newMetricsServer(t, `# TYPE demo_metric gauge
demo_metric 1
`)
	collr.ProfileSelectionMode = profileSelectionModeExact
	collr.Profiles = []string{"demo_a", "demo_b"}
	collr.loadProfileCatalog = func() (promprofiles.Catalog, error) {
		return loadTestCatalog(t, map[string]string{
			"demo-a.yaml": testProfileYAML("Demo_A", "demo_*", "", "demo_metric", "demo_a_chart", "Demo A"),
			"demo-b.yaml": testProfileYAML("Demo_B", "demo_*", "", "demo_metric", "demo_b_chart", "Demo B"),
		})
	}

	require.NoError(t, collr.Init(context.Background()))
	err := collr.Check(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), `matches multiple selected profiles`)
}

func testPromProfile(id string) promprofiles.Profile {
	return promprofiles.Profile{
		Name:  id,
		Match: "demo_*",
		Template: charttpl.Group{
			Family:  "prometheus_curated",
			Metrics: []string{"demo_metric_total"},
			Charts: []charttpl.Chart{
				{
					ID:      id + "_chart",
					Title:   "Demo Metric",
					Context: "prometheus.demo.metric",
					Units:   "events",
					Dimensions: []charttpl.Dimension{
						{Selector: "demo_metric_total", Name: "value"},
					},
				},
			},
		},
	}
}

func newMetricsServer(t *testing.T, body string) string {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func loadTestCatalog(t *testing.T, files map[string]string) (promprofiles.Catalog, error) {
	t.Helper()

	root := t.TempDir()
	stock := filepath.Join(root, "stock")
	require.NoError(t, os.MkdirAll(stock, 0o755))
	for name, body := range files {
		require.NoError(t, os.WriteFile(filepath.Join(stock, name), []byte(body), 0o644))
	}

	return promprofiles.LoadFromDirs([]promprofiles.DirSpec{{Path: stock, IsStock: true}})
}

func testProfileYAML(name, match, extra, metric, chartID, title string) string {
	return `
name: ` + name + `
match: ` + match + `
` + extra + `template:
  family: prometheus_curated
  metrics: [` + metric + `]
  charts:
    - id: ` + chartID + `
      title: ` + title + `
      context: prometheus.demo.metric
      units: events
      dimensions:
        - selector: ` + metric + `
          name: value
`
}
