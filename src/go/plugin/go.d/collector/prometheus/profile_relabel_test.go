// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	commonmodel "github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	prompkg "github.com/netdata/netdata/go/plugins/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/promprofiles"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/relabel"
)

func TestCollector_ProfileRelabelingAppliesAutomatically(t *testing.T) {
	catalog := loadTestCatalog(t, map[string]string{
		"app": testRelabelProfileYAML("app_*", "app_raw", "app_final"),
	})
	collr, srv := newProfileRelabelCollector(t, catalog, `
# TYPE app_raw gauge
app_raw{id="1"} 7
`, "app")
	defer srv.Close()
	require.NoError(t, collr.Init(context.Background()))

	require.NoError(t, collr.Check(context.Background()))
	require.Contains(t, collr.ChartTemplateYAML(), "app_final")
	collectProfileRelabelOnce(t, collr)

	got := collr.MetricStore().Read(metrix.ReadRaw(), metrix.ReadFlatten())
	assert.InDelta(t, 7, value(t, got, "app_final", metrix.Labels{"id": "1"}), 1e-9)
	noSeries(t, got, "app_raw", metrix.Labels{"id": "1"})
}

func TestCollector_ProfileRelabelingRunsAfterJobRelabeling(t *testing.T) {
	catalog := loadTestCatalog(t, map[string]string{
		"app": testRelabelProfileYAML("app_*", "app_raw", "app_final"),
	})
	collr, srv := newProfileRelabelCollector(t, catalog, `
# TYPE source_raw gauge
source_raw 7
`, "app")
	defer srv.Close()
	collr.Relabeling = []relabel.Block{{
		Match: "source_*",
		MetricRelabelConfigs: []relabel.Config{{
			SourceLabels: []string{"__name__"},
			Regex:        relabel.MustNewRegexp("source_raw"),
			TargetLabel:  "__name__",
			Replacement:  "app_raw",
			Action:       relabel.Replace,
		}},
	}}
	require.NoError(t, collr.Init(context.Background()))

	require.NoError(t, collr.Check(context.Background()))
	collectProfileRelabelOnce(t, collr)
	got := collr.MetricStore().Read(metrix.ReadRaw(), metrix.ReadFlatten())
	assert.InDelta(t, 7, value(t, got, "app_final", nil), 1e-9)
	noSeries(t, got, "app_raw", nil)
}

func TestCollector_ProfileRelabelingRemapsHelp(t *testing.T) {
	catalog := loadTestCatalog(t, map[string]string{
		"app": testRelabelProfileYAML("app_*", "app_raw", "app_final"),
	})
	profiles, err := catalog.Resolve([]string{"app"})
	require.NoError(t, err)
	normalizers, err := compileProfileNormalizers(profiles)
	require.NoError(t, err)

	batch := scrapeSamples(t, "# HELP app_raw Application value.\n# TYPE app_raw gauge\napp_raw 1\n")
	mfs, err := New().profileRelabelAndAssemble(batch, normalizers, true)
	require.NoError(t, err)
	renamed := mfs.Get("app_final")
	require.NotNil(t, renamed)
	assert.Equal(t, "Application value.", renamed.Help())
}

func TestCollector_ProfileRelabelingUsesFirstApplicableNormalizer(t *testing.T) {
	catalog := loadTestCatalog(t, map[string]string{
		"first":  testRelabelProfileYAML("app_*", "app_raw", "app_first"),
		"second": testRelabelProfileYAML("app_*", "app_raw", "app_second"),
	})

	tests := map[string]struct {
		profiles []string
		want     string
		shadowed string
	}{
		"configured first profile wins": {
			profiles: []string{"first", "second"},
			want:     "app_first",
			shadowed: "app_second",
		},
		"reversing configured order changes precedence": {
			profiles: []string{"second", "first"},
			want:     "app_second",
			shadowed: "app_first",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			collr, srv := newProfileRelabelCollector(t, catalog, "# TYPE app_raw gauge\napp_raw 1\n", tc.profiles...)
			defer srv.Close()
			require.NoError(t, collr.Init(context.Background()))
			require.NoError(t, collr.Check(context.Background()))

			collectProfileRelabelOnce(t, collr)
			got := collr.MetricStore().Read(metrix.ReadRaw(), metrix.ReadFlatten())
			assert.InDelta(t, 1, value(t, got, tc.want, nil), 1e-9)
			noSeries(t, got, tc.shadowed, nil)
			noSeries(t, got, "app_raw", nil)
		})
	}
}

func TestCollector_ProfileRelabelingCombinedEntriesPrecedeAutoProfiles(t *testing.T) {
	catalog := loadTestCatalog(t, map[string]string{
		"alpha": strings.Replace(testRelabelProfileYAML("app_*", "app_raw", "app_alpha"), "template:\n", "app: alpha\ntemplate:\n", 1),
		"zeta":  strings.Replace(testRelabelProfileYAML("app_*", "app_raw", "app_zeta"), "template:\n", "app: zeta\ntemplate:\n", 1),
	})
	collr, srv := newProfileRelabelCollector(t, catalog, "# TYPE app_raw gauge\napp_raw 1\n")
	defer srv.Close()
	collr.Profiles = ProfilesConfig{Mode: profilesModeCombined, ModeCombined: &ProfilesModeConfig{
		Entries: profileEntries("zeta"),
	}}
	require.NoError(t, collr.Init(context.Background()))
	require.NoError(t, collr.Check(context.Background()))

	collectProfileRelabelOnce(t, collr)
	got := collr.MetricStore().Read(metrix.ReadRaw(), metrix.ReadFlatten())
	assert.InDelta(t, 1, value(t, got, "app_zeta", nil), 1e-9)
	noSeries(t, got, "app_alpha", nil)
	noSeries(t, got, "app_raw", nil)

	spec, err := charttpl.DecodeYAML([]byte(collr.ChartTemplateYAML()))
	require.NoError(t, err)
	assert.Equal(t, "prometheus.alpha", spec.ContextNamespace)
	require.Len(t, spec.Groups, 3)
	assert.Equal(t, []string{"app_alpha"}, spec.Groups[1].Metrics)
	assert.Equal(t, []string{"app_zeta"}, spec.Groups[2].Metrics)
}

func TestCollector_ProfileRelabelingAutoNameOrderDoesNotChangeProfileSelectionOrder(t *testing.T) {
	catalog := loadTestCatalogFromOrderedDirs(t,
		map[string]string{
			"zeta": strings.Replace(testRelabelProfileYAML("app_*", "app_raw", "app_zeta"), "template:\n", "app: zeta\ntemplate:\n", 1),
		},
		map[string]string{
			"alpha": strings.Replace(testRelabelProfileYAML("app_*", "app_raw", "app_alpha"), "template:\n", "app: alpha\ntemplate:\n", 1),
		},
	)
	collr, srv := newProfileRelabelCollector(t, catalog, "# TYPE app_raw gauge\napp_raw 1\n")
	defer srv.Close()
	collr.Profiles = ProfilesConfig{Mode: profilesModeAuto}
	require.NoError(t, collr.Init(context.Background()))
	require.NoError(t, collr.Check(context.Background()))

	collectProfileRelabelOnce(t, collr)
	got := collr.MetricStore().Read(metrix.ReadRaw(), metrix.ReadFlatten())
	assert.InDelta(t, 1, value(t, got, "app_alpha", nil), 1e-9)
	noSeries(t, got, "app_zeta", nil)

	spec, err := charttpl.DecodeYAML([]byte(collr.ChartTemplateYAML()))
	require.NoError(t, err)
	assert.Equal(t, "prometheus.zeta", spec.ContextNamespace)
	require.Len(t, spec.Groups, 3)
	assert.Equal(t, []string{"app_zeta"}, spec.Groups[1].Metrics)
	assert.Equal(t, []string{"app_alpha"}, spec.Groups[2].Metrics)
}

func TestSelectProfileNormalizersSkipsProfilesWhoseBlocksDoNotMatch(t *testing.T) {
	catalog := loadTestCatalog(t, map[string]string{
		"first":  testRelabelProfileYAML("app_*", "app_other", "app_first"),
		"second": testRelabelProfileYAML("app_*", "app_raw", "app_second"),
	})
	profiles, err := catalog.Resolve([]string{"first", "second"})
	require.NoError(t, err)
	normalizers, err := compileProfileNormalizers(profiles)
	require.NoError(t, err)
	batch := scrapeSamples(t, "# TYPE app_raw gauge\napp_raw 1\n")

	selected := selectProfileNormalizers(batch, normalizers)
	require.Contains(t, selected, "app_raw")
	assert.Equal(t, 1, selected["app_raw"])
}

func TestSelectProfileNormalizersUsesFamilyPrecedenceAcrossPhysicalSamples(t *testing.T) {
	catalog := loadTestCatalog(t, map[string]string{
		"first":  testRelabelProfileYAML("app_*", "app_lat_sum", "app_first_sum"),
		"middle": testRelabelProfileYAML("app_*", "app_lat_count", "app_middle_count"),
		"last":   testRelabelProfileYAML("app_*", "app_lat_bucket", "app_last_bucket"),
	})
	profiles, err := catalog.Resolve([]string{"first", "middle", "last"})
	require.NoError(t, err)
	normalizers, err := compileProfileNormalizers(profiles)
	require.NoError(t, err)

	inputs := map[string]string{
		"lower precedence components first": `
# TYPE app_lat histogram
app_lat_bucket{le="0.5"} 4
app_lat_bucket{le="+Inf"} 6
app_lat_count 6
app_lat_sum 2.5
`,
		"highest precedence component first": `
# TYPE app_lat histogram
app_lat_sum 2.5
app_lat_count 6
app_lat_bucket{le="0.5"} 4
app_lat_bucket{le="+Inf"} 6
`,
	}

	for name, input := range inputs {
		t.Run(name, func(t *testing.T) {
			selected := selectProfileNormalizers(scrapeSamples(t, input), normalizers)
			require.Contains(t, selected, "app_lat")
			assert.Equal(t, 0, selected["app_lat"])
		})
	}
}

func TestCollector_ProfileRelabelingDoesNotChainProfiles(t *testing.T) {
	catalog := loadTestCatalog(t, map[string]string{
		"first":  testRelabelProfileYAML("app_*", "app_raw", "app_mid"),
		"second": testRelabelProfileYAML("app_*", "app_mid", "app_final"),
	})
	collr, srv := newProfileRelabelCollector(t, catalog, "# TYPE app_raw gauge\napp_raw 1\n", "first", "second")
	defer srv.Close()
	require.NoError(t, collr.Init(context.Background()))
	require.NoError(t, collr.Check(context.Background()))

	collectProfileRelabelOnce(t, collr)
	got := collr.MetricStore().Read(metrix.ReadRaw(), metrix.ReadFlatten())
	assert.InDelta(t, 1, value(t, got, "app_mid", nil), 1e-9)
	noSeries(t, got, "app_final", nil)
}

func TestCollector_ProfileRelabelingAllowsOutputOutsideProfileMatch(t *testing.T) {
	catalog := loadTestCatalog(t, map[string]string{
		"app": testRelabelProfileYAML("app_*", "app_raw", "other_final"),
	})
	collr, srv := newProfileRelabelCollector(t, catalog, "# TYPE app_raw gauge\napp_raw 1\n", "app")
	defer srv.Close()
	require.NoError(t, collr.Init(context.Background()))
	require.NoError(t, collr.Check(context.Background()))

	collectProfileRelabelOnce(t, collr)
	got := collr.MetricStore().Read(metrix.ReadRaw(), metrix.ReadFlatten())
	assert.InDelta(t, 1, value(t, got, "other_final", nil), 1e-9)
	noSeries(t, got, "app_raw", nil)
}

func TestCollector_ProfileRelabelingPreservesPreProfileFallbackType(t *testing.T) {
	catalog := loadTestCatalog(t, map[string]string{
		"app": testRelabelProfileYAML("app_*", "app_value_total", "app_value"),
	})
	collr, srv := newProfileRelabelCollector(t, catalog, "app_value_total 7\n", "app")
	defer srv.Close()
	require.NoError(t, collr.Init(context.Background()))

	require.NoError(t, collr.Check(context.Background()))
	collectProfileRelabelOnce(t, collr)
	require.Contains(t, collr.writer.handles, "app_value")
	assert.Equal(t, commonmodel.MetricTypeCounter, collr.writer.handles["app_value"].typ)
}

func TestCollector_ProfileRelabelingPreservesConfiguredGaugePrecedence(t *testing.T) {
	catalog := loadTestCatalog(t, map[string]string{
		"app": testRelabelProfileYAML("app_*", "app_value_total", "app_value"),
	})
	collr, srv := newProfileRelabelCollector(t, catalog, "app_value_total 7\n", "app")
	defer srv.Close()
	collr.FallbackType.Gauge = []string{"app_value_total"}
	require.NoError(t, collr.Init(context.Background()))

	require.NoError(t, collr.Check(context.Background()))
	collectProfileRelabelOnce(t, collr)
	require.Contains(t, collr.writer.handles, "app_value")
	assert.Equal(t, commonmodel.MetricTypeGauge, collr.writer.handles["app_value"].typ)
}

func TestCollector_ProfileRelabelingCannotCreateFallbackType(t *testing.T) {
	catalog := loadTestCatalog(t, map[string]string{
		"app": testRelabelProfileYAML("app_*", "app_value", "app_value_total"),
	})
	collr, srv := newProfileRelabelCollector(t, catalog, "app_value 7\n", "app")
	defer srv.Close()
	require.NoError(t, collr.Init(context.Background()))

	err := collr.Check(context.Background())
	require.ErrorContains(t, err, "exposes no usable metrics")
	assert.Nil(t, collr.runtime)
}

func TestCollector_ProfileRelabelingDoesNotShareFallbackEligibilityAcrossMergedFamily(t *testing.T) {
	tests := map[string]struct {
		boundName    string
		boundType    string
		unboundFirst bool
		configure    func(*Collector)
		wantType     commonmodel.MetricType
	}{
		"configured gauge": {
			boundName: "app_bound",
			configure: func(c *Collector) {
				c.FallbackType.Gauge = []string{"app_bound"}
			},
			wantType: commonmodel.MetricTypeGauge,
		},
		"configured counter": {
			boundName:    "app_bound",
			unboundFirst: true,
			configure: func(c *Collector) {
				c.FallbackType.Counter = []string{"app_bound"}
			},
			wantType: commonmodel.MetricTypeCounter,
		},
		"implicit total counter": {
			boundName: "app_bound_total",
			wantType:  commonmodel.MetricTypeCounter,
		},
		"declared gauge": {
			boundName: "app_bound",
			boundType: "gauge",
			wantType:  commonmodel.MetricTypeGauge,
		},
		"declared counter": {
			boundName: "app_bound",
			boundType: "counter",
			wantType:  commonmodel.MetricTypeCounter,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			catalog := loadTestCatalog(t, map[string]string{
				"app": testRelabelProfilePatternYAML("app_*", "app_*", "app_.+", "app_final", "app_final"),
			})
			bound := fmt.Sprintf("%s{eligibility=\"bound\"} 7\n", tc.boundName)
			unbound := "app_unbound{eligibility=\"unbound\"} 9\n"
			if tc.unboundFirst {
				bound, unbound = unbound, bound
			}
			input := bound + unbound
			if tc.boundType != "" {
				input = fmt.Sprintf("# TYPE %s %s\n", tc.boundName, tc.boundType) + input
			}
			collr, srv := newProfileRelabelCollector(t, catalog, input, "app")
			defer srv.Close()
			if tc.configure != nil {
				tc.configure(collr)
			}
			require.NoError(t, collr.Init(context.Background()))

			require.NoError(t, collr.Check(context.Background()))
			mfs, err := collr.scrapeProfilePipeline(context.Background(), collr.runtime, true)
			require.NoError(t, err)
			assert.Equal(t, 1, collr.writer.countBoundWritable(mfs), "Check must count only the pre-profile bound sample")

			collectProfileRelabelOnce(t, collr)
			got := collr.MetricStore().Read(metrix.ReadRaw(), metrix.ReadFlatten())
			assert.InDelta(t, 7, value(t, got, "app_final", metrix.Labels{"eligibility": "bound"}), 1e-9)
			noSeries(t, got, "app_final", metrix.Labels{"eligibility": "unbound"})
			require.Contains(t, collr.writer.handles, "app_final")
			assert.Equal(t, tc.wantType, collr.writer.handles["app_final"].typ)
		})
	}
}

func TestCollector_ProfileRelabelingDispatchesDisjointFamilies(t *testing.T) {
	catalog := loadTestCatalog(t, map[string]string{
		"first":  testRelabelProfileYAML("app_*", "app_first_raw", "app_first_final"),
		"second": testRelabelProfileYAML("app_*", "app_second_raw", "app_second_final"),
	})
	collr, srv := newProfileRelabelCollector(t, catalog, `
# TYPE app_first_raw gauge
app_first_raw 1
# TYPE app_second_raw gauge
app_second_raw 2
`, "first", "second")
	defer srv.Close()
	require.NoError(t, collr.Init(context.Background()))

	require.NoError(t, collr.Check(context.Background()))
	collectProfileRelabelOnce(t, collr)
	got := collr.MetricStore().Read(metrix.ReadRaw(), metrix.ReadFlatten())
	assert.InDelta(t, 1, value(t, got, "app_first_final", nil), 1e-9)
	assert.InDelta(t, 2, value(t, got, "app_second_final", nil), 1e-9)
}

func TestCollector_ProfileRelabelingUsesFirstNormalizerForNewRuntimeFamily(t *testing.T) {
	catalog := loadTestCatalog(t, map[string]string{
		"first":  testRelabelProfileYAML("app_*", "app_dynamic", "app_first"),
		"second": testRelabelProfileYAML("app_*", "app_dynamic", "app_second"),
	})
	input := "# TYPE app_safe gauge\napp_safe 1\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(input))
	}))
	defer srv.Close()

	collr := New()
	collr.URL = srv.URL
	collr.Profiles = ProfilesConfig{Mode: profilesModeExact, ModeExact: &ProfilesModeConfig{
		Entries: profileEntries("first", "second"),
	}}
	collr.loadProfileCatalog = func() (promprofiles.Catalog, error) { return catalog, nil }
	require.NoError(t, collr.Init(context.Background()))
	require.NoError(t, collr.Check(context.Background()))

	input = `
# TYPE app_safe gauge
app_safe 2
# TYPE app_dynamic gauge
app_dynamic 3
`
	collectProfileRelabelOnce(t, collr)
	got := collr.MetricStore().Read(metrix.ReadRaw(), metrix.ReadFlatten())
	assert.InDelta(t, 2, value(t, got, "app_safe", nil), 1e-9)
	noSeries(t, got, "app_dynamic", nil)
	assert.InDelta(t, 3, value(t, got, "app_first", nil), 1e-9)
	noSeries(t, got, "app_second", nil)
}

func TestCollector_ProfileRelabelingTypedFamilyCorruptionFailsCheck(t *testing.T) {
	catalog := loadTestCatalog(t, map[string]string{
		"app": testRelabelProfileYAML("app_*", "app_lat_sum", "app_other_sum"),
	})
	collr, srv := newProfileRelabelCollector(t, catalog, `
# TYPE app_lat histogram
app_lat_bucket{le="+Inf"} 6
app_lat_sum 2.5
app_lat_count 6
`, "app")
	defer srv.Close()
	require.NoError(t, collr.Init(context.Background()))

	err := collr.Check(context.Background())
	require.ErrorContains(t, err, "profile relabeling corrupts typed metric families")
	assert.Nil(t, collr.runtime)
}

func TestCollector_ProfileRelabelingTypedFamilyRuntimeDriftIsContained(t *testing.T) {
	catalog := loadTestCatalog(t, map[string]string{
		"app": testRelabelProfileYAML("app_*", "app_lat_sum", "app_other_sum"),
	})
	input := "# TYPE app_safe gauge\napp_safe 1\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(input))
	}))
	defer srv.Close()

	collr := New()
	collr.URL = srv.URL
	collr.Profiles = ProfilesConfig{Mode: profilesModeExact, ModeExact: &ProfilesModeConfig{
		Entries: profileEntries("app"),
	}}
	collr.loadProfileCatalog = func() (promprofiles.Catalog, error) { return catalog, nil }
	require.NoError(t, collr.Init(context.Background()))
	require.NoError(t, collr.Check(context.Background()))

	input = `
# TYPE app_safe gauge
app_safe 2
# TYPE app_lat histogram
app_lat_bucket{le="+Inf"} 6
app_lat_sum 2.5
app_lat_count 6
`
	collectProfileRelabelOnce(t, collr)
	got := collr.MetricStore().Read(metrix.ReadRaw(), metrix.ReadFlatten())
	assert.InDelta(t, 2, value(t, got, "app_safe", nil), 1e-9)
	noSeries(t, got, "app_lat_bucket", metrix.Labels{"le": "+Inf"})
	noSeries(t, got, "app_other_sum", nil)
}

func TestCollector_ProfileRelabelingCleanTypedFamilyRename(t *testing.T) {
	catalog := loadTestCatalog(t, map[string]string{
		"app": testRelabelProfilePatternYAML("app_*", "app_lat*", "app_lat(.*)", "app_renamed${1}", "app_renamed"),
	})
	collr, srv := newProfileRelabelCollector(t, catalog, `
# TYPE app_lat histogram
app_lat_bucket{le="+Inf"} 6
app_lat_sum 2.5
app_lat_count 6
`, "app")
	defer srv.Close()
	require.NoError(t, collr.Init(context.Background()))

	require.NoError(t, collr.Check(context.Background()))
	collectProfileRelabelOnce(t, collr)
	got := collr.MetricStore().Read(metrix.ReadRaw(), metrix.ReadFlatten())
	assert.InDelta(t, 6, value(t, got, "app_renamed_bucket", metrix.Labels{"le": "+Inf"}), 1e-9)
	assert.InDelta(t, 2.5, value(t, got, "app_renamed_sum", nil), 1e-9)
	assert.InDelta(t, 6, value(t, got, "app_renamed_count", nil), 1e-9)
}

func TestCollector_CombinedRelabelingDoesNotResurrectJobCorruption(t *testing.T) {
	catalog := loadTestCatalog(t, map[string]string{
		"app": testRelabelProfileYAML("app_*", "app_safe", "app_safe_final"),
	})
	input := "# TYPE app_safe gauge\napp_safe 1\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(input))
	}))
	defer srv.Close()

	collr := New()
	collr.URL = srv.URL
	collr.Profiles = ProfilesConfig{Mode: profilesModeExact, ModeExact: &ProfilesModeConfig{
		Entries: profileEntries("app"),
	}}
	collr.Relabeling = []relabel.Block{{
		Match: "*",
		MetricRelabelConfigs: []relabel.Config{{
			SourceLabels: []string{"__name__"},
			Regex:        relabel.MustNewRegexp("app_lat_sum"),
			TargetLabel:  "__name__",
			Replacement:  "app_other_sum",
			Action:       relabel.Replace,
		}},
	}}
	collr.loadProfileCatalog = func() (promprofiles.Catalog, error) { return catalog, nil }
	require.NoError(t, collr.Init(context.Background()))
	require.NoError(t, collr.Check(context.Background()))

	input = `
# TYPE app_safe gauge
app_safe 2
# TYPE app_lat histogram
app_lat_bucket{le="+Inf"} 6
app_lat_sum 2.5
app_lat_count 6
`
	collectProfileRelabelOnce(t, collr)
	got := collr.MetricStore().Read(metrix.ReadRaw(), metrix.ReadFlatten())
	assert.InDelta(t, 2, value(t, got, "app_safe_final", nil), 1e-9)
	noSeries(t, got, "app_lat_bucket", metrix.Labels{"le": "+Inf"})
	noSeries(t, got, "app_other_sum", nil)
}

func TestCollector_ProfileRelabelingFinalGatesUseNormalizedOutput(t *testing.T) {
	catalog := loadTestCatalog(t, map[string]string{
		"app": testDropProfileYAML("app_*", "app_drop", "app_drop", "app_keep"),
	})
	collr, srv := newProfileRelabelCollector(t, catalog, `
# TYPE app_keep gauge
app_keep 1
# TYPE app_drop gauge
app_drop 2
`, "app")
	defer srv.Close()
	collr.MaxTS = 1
	require.NoError(t, collr.Init(context.Background()))

	require.NoError(t, collr.Check(context.Background()), "the final one-series output must satisfy MaxTS")
	collectProfileRelabelOnce(t, collr)
	got := collr.MetricStore().Read(metrix.ReadRaw(), metrix.ReadFlatten())
	assert.InDelta(t, 1, value(t, got, "app_keep", nil), 1e-9)
	noSeries(t, got, "app_drop", nil)
}

func TestCollector_ProfileRelabelingEmptyFinalOutputFailsCheck(t *testing.T) {
	catalog := loadTestCatalog(t, map[string]string{
		"app": testDropProfileYAML("app_*", "app_drop", "app_drop", "app_drop"),
	})
	collr, srv := newProfileRelabelCollector(t, catalog, "# TYPE app_drop gauge\napp_drop 2\n", "app")
	defer srv.Close()
	require.NoError(t, collr.Init(context.Background()))

	err := collr.Check(context.Background())
	require.ErrorContains(t, err, "returned 0 metric families")
	assert.Nil(t, collr.runtime)
}

func TestCollector_ProfilesModeNoneSkipsCatalogAndProfileRelabeling(t *testing.T) {
	var catalogLoads atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("# TYPE app_raw gauge\napp_raw 1\n"))
	}))
	defer srv.Close()

	collr := New()
	collr.URL = srv.URL
	collr.Profiles = ProfilesConfig{Mode: profilesModeNone}
	collr.loadProfileCatalog = func() (promprofiles.Catalog, error) {
		catalogLoads.Add(1)
		return promprofiles.Catalog{}, fmt.Errorf("must not load")
	}
	require.NoError(t, collr.Init(context.Background()))
	require.NoError(t, collr.Check(context.Background()))
	collectProfileRelabelOnce(t, collr)

	assert.Zero(t, catalogLoads.Load())
	got := collr.MetricStore().Read(metrix.ReadRaw(), metrix.ReadFlatten())
	assert.InDelta(t, 1, value(t, got, "app_raw", nil), 1e-9)
}

func TestCollector_UnselectedProfileRelabelingDoesNotApply(t *testing.T) {
	catalog := loadTestCatalog(t, map[string]string{
		"other": testRelabelProfileYAML("other_*", "app_raw", "other_final"),
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("# TYPE app_raw gauge\napp_raw 1\n"))
	}))
	defer srv.Close()

	collr := New()
	collr.URL = srv.URL
	collr.loadProfileCatalog = func() (promprofiles.Catalog, error) { return catalog, nil }
	require.NoError(t, collr.Init(context.Background()))
	require.NoError(t, collr.Check(context.Background()))
	collectProfileRelabelOnce(t, collr)

	got := collr.MetricStore().Read(metrix.ReadRaw(), metrix.ReadFlatten())
	assert.InDelta(t, 1, value(t, got, "app_raw", nil), 1e-9)
	noSeries(t, got, "other_final", nil)
}

func TestCollector_ProfileRelabelingCheckRetryIsTransactional(t *testing.T) {
	catalog := loadTestCatalog(t, map[string]string{
		"app": testRelabelProfileYAML("app_*", "app_lat_sum", "app_other_sum"),
	})
	input := `
# TYPE app_lat histogram
app_lat_bucket{le="+Inf"} 6
app_lat_sum 2.5
app_lat_count 6
`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(input))
	}))
	defer srv.Close()

	collr := New()
	collr.URL = srv.URL
	collr.Profiles = ProfilesConfig{Mode: profilesModeExact, ModeExact: &ProfilesModeConfig{
		Entries: profileEntries("app"),
	}}
	collr.loadProfileCatalog = func() (promprofiles.Catalog, error) { return catalog, nil }
	require.NoError(t, collr.Init(context.Background()))

	require.Error(t, collr.Check(context.Background()))
	assert.Nil(t, collr.runtime)
	assert.Empty(t, collr.ChartTemplateYAML())

	input = "# TYPE app_safe gauge\napp_safe 2\n"
	require.NoError(t, collr.Check(context.Background()))
	assert.NotNil(t, collr.runtime)
	assert.NotEmpty(t, collr.ChartTemplateYAML())
}

func TestCollector_ProfileRelabelingCannotSatisfyExpectedPrefix(t *testing.T) {
	catalog := loadTestCatalog(t, map[string]string{
		"app": testRelabelProfileYAML("app_*", "app_raw", "app_final"),
	})
	collr, srv := newProfileRelabelCollector(t, catalog, "# TYPE app_raw gauge\napp_raw 1\n", "app")
	defer srv.Close()
	collr.ExpectedPrefix = "app_final"
	require.NoError(t, collr.Init(context.Background()))

	err := collr.Check(context.Background())
	require.ErrorContains(t, err, "no expected prefix")
	assert.Nil(t, collr.runtime)
}

func TestCollector_CollectRequiresPublishedRuntime(t *testing.T) {
	var requests atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte("# TYPE app_raw gauge\napp_raw 1\n"))
	}))
	defer srv.Close()

	var catalogLoads atomic.Int64
	collr := New()
	collr.URL = srv.URL
	collr.Profiles = ProfilesConfig{Mode: profilesModeNone}
	collr.loadProfileCatalog = func() (promprofiles.Catalog, error) {
		catalogLoads.Add(1)
		return promprofiles.Catalog{}, nil
	}
	require.NoError(t, collr.Init(context.Background()))

	err := collr.Collect(context.Background())
	require.ErrorContains(t, err, "successful Check")
	assert.Zero(t, requests.Load())
	assert.Zero(t, catalogLoads.Load())

	require.NoError(t, collr.Check(context.Background()))
	collr.Cleanup(context.Background())
	requests.Store(0)
	err = collr.Collect(context.Background())
	require.ErrorContains(t, err, "successful Check")
	assert.Zero(t, requests.Load())
}

func TestCollector_ProfileRelabelingScrapeDispatch(t *testing.T) {
	plainCatalog := loadTestCatalog(t, map[string]string{"app": testProfileYAML("app_*")})
	relabelCatalog := loadTestCatalog(t, map[string]string{
		"app": testRelabelProfileYAML("app_*", "app_raw", "app_final"),
	})
	fallbackCatalog := loadTestCatalog(t, map[string]string{
		"app": testProfileYAMLWithFallbackType("app_*", []string{"app_raw"}, nil),
	})

	tests := map[string]struct {
		prepare         func(*Collector)
		hasCatalog      bool
		catalog         promprofiles.Catalog
		wantFamilyCalls int
		wantSampleCalls int
	}{
		"no profiles and no job rules stays direct": {
			prepare:         func(c *Collector) { c.Profiles = ProfilesConfig{Mode: profilesModeNone} },
			wantFamilyCalls: 2,
		},
		"job rules use samples": {
			prepare: func(c *Collector) {
				c.Profiles = ProfilesConfig{Mode: profilesModeNone}
				c.Relabeling = []relabel.Block{{Match: "*", MetricRelabelConfigs: []relabel.Config{{
					SourceLabels: []string{"__name__"},
					Regex:        relabel.MustNewRegexp("does_not_match"),
					Action:       relabel.Drop,
				}}}}
			},
			wantSampleCalls: 2,
		},
		"profile selection buffers Check but recurring no-rules Collect stays direct": {
			prepare: func(c *Collector) {
				c.Profiles = ProfilesConfig{Mode: profilesModeExact, ModeExact: &ProfilesModeConfig{Entries: profileEntries("app")}}
			},
			hasCatalog:      true,
			catalog:         plainCatalog,
			wantFamilyCalls: 1,
			wantSampleCalls: 1,
		},
		"selected profile rules use samples": {
			prepare: func(c *Collector) {
				c.Profiles = ProfilesConfig{Mode: profilesModeExact, ModeExact: &ProfilesModeConfig{Entries: profileEntries("app")}}
			},
			hasCatalog:      true,
			catalog:         relabelCatalog,
			wantSampleCalls: 2,
		},
		"selected profile fallback uses samples": {
			prepare: func(c *Collector) {
				c.Profiles = ProfilesConfig{Mode: profilesModeExact, ModeExact: &ProfilesModeConfig{Entries: profileEntries("app")}}
			},
			hasCatalog:      true,
			catalog:         fallbackCatalog,
			wantSampleCalls: 2,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			batch := scrapeSamples(t, "# TYPE app_raw gauge\napp_raw 1\n")
			mfs, err := prompkg.Assemble(batch)
			require.NoError(t, err)
			fake := &countingPrometheus{batch: batch, mfs: mfs}

			collr := New()
			collr.URL = "http://127.0.0.1:1"
			tc.prepare(collr)
			if tc.hasCatalog {
				collr.loadProfileCatalog = func() (promprofiles.Catalog, error) { return tc.catalog, nil }
			}
			require.NoError(t, collr.Init(context.Background()))
			collr.prom = fake
			require.NoError(t, collr.Check(context.Background()))
			collectProfileRelabelOnce(t, collr)

			assert.Equal(t, tc.wantFamilyCalls, fake.familyCalls)
			assert.Equal(t, tc.wantSampleCalls, fake.sampleCalls)
		})
	}
}

type countingPrometheus struct {
	batch       prompkg.SampleBatch
	mfs         prompkg.MetricFamilies
	familyCalls int
	sampleCalls int
}

func (p *countingPrometheus) ScrapeSeries() (prompkg.Series, error) { return nil, nil }

func (p *countingPrometheus) Scrape() (prompkg.MetricFamilies, error) {
	return p.ScrapeContext(context.Background())
}

func (p *countingPrometheus) ScrapeContext(context.Context) (prompkg.MetricFamilies, error) {
	p.familyCalls++
	return p.mfs, nil
}

func (p *countingPrometheus) ScrapeSamples(context.Context) (prompkg.SampleBatch, error) {
	p.sampleCalls++
	return prompkg.SampleBatch{
		Help:    slices.Clone(p.batch.Help),
		Samples: slices.Clone(p.batch.Samples),
	}, nil
}

func (p *countingPrometheus) HTTPClient() *http.Client { return nil }

func newProfileRelabelCollector(t *testing.T, catalog promprofiles.Catalog, input string, profiles ...string) (*Collector, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(input))
	}))
	collr := New()
	collr.URL = srv.URL
	collr.Profiles = ProfilesConfig{
		Mode:      profilesModeExact,
		ModeExact: &ProfilesModeConfig{Entries: profileEntries(profiles...)},
	}
	collr.loadProfileCatalog = func() (promprofiles.Catalog, error) { return catalog, nil }
	return collr, srv
}

func collectProfileRelabelOnce(t *testing.T, collr *Collector) {
	t.Helper()
	cc := cycle(t, collr.MetricStore())
	cc.BeginCycle()
	require.NoError(t, collr.Collect(context.Background()))
	require.NoError(t, cc.CommitCycleSuccess())
}

func profileEntries(names ...string) []ProfileEntryConfig {
	out := make([]ProfileEntryConfig, len(names))
	for i, name := range names {
		out[i] = ProfileEntryConfig{Name: name}
	}
	return out
}

func testRelabelProfileYAML(rootMatch, source, target string) string {
	return testRelabelProfilePatternYAML(rootMatch, source, source, target, target)
}

func testRelabelProfilePatternYAML(rootMatch, blockMatch, regex, replacement, chartMetric string) string {
	return strings.Join([]string{
		fmt.Sprintf("match: %q", rootMatch),
		"relabeling:",
		fmt.Sprintf("  - match: %q", blockMatch),
		"    metric_relabel_configs:",
		"      - source_labels: [__name__]",
		fmt.Sprintf("        regex: %q", regex),
		"        target_label: __name__",
		fmt.Sprintf("        replacement: %q", replacement),
		"        action: replace",
		"template:",
		"  family: Test",
		"  context_namespace: test",
		"  metrics:",
		"    - " + chartMetric,
		"  charts:",
		"    - title: Relabeled",
		"      context: " + chartMetric,
		"      units: value",
		"      dimensions:",
		"        - selector: " + chartMetric,
		"          name: value",
	}, "\n") + "\n"
}

func testDropProfileYAML(rootMatch, blockMatch, regex, chartMetric string) string {
	return strings.Join([]string{
		fmt.Sprintf("match: %q", rootMatch),
		"relabeling:",
		fmt.Sprintf("  - match: %q", blockMatch),
		"    metric_relabel_configs:",
		"      - source_labels: [__name__]",
		fmt.Sprintf("        regex: %q", regex),
		"        action: drop",
		"template:",
		"  family: Test",
		"  context_namespace: test",
		"  metrics:",
		"    - " + chartMetric,
		"  charts:",
		"    - title: Relabeled",
		"      context: " + chartMetric,
		"      units: value",
		"      dimensions:",
		"        - selector: " + chartMetric,
		"          name: value",
	}, "\n") + "\n"
}
