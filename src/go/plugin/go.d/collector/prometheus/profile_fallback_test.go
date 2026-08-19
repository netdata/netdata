// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	commonmodel "github.com/prometheus/common/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/promprofiles"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/relabel"
)

func TestCollector_ProfileFallbackOnlyUsesBoundPipeline(t *testing.T) {
	profile := testProfileYAMLWithFallbackType("app_*", []string{"app_value"}, nil)
	profile = strings.ReplaceAll(profile, "test_up", "app_value")
	catalog := loadTestCatalog(t, map[string]string{
		"app": profile,
	})
	collr, srv := newProfileRelabelCollector(t, catalog, strings.Join([]string{
		"app_value{id=\"one\"} 7",
		"other_requests_total 11",
		"",
	}, "\n"), "app")
	defer srv.Close()
	require.NoError(t, collr.Init(context.Background()))

	require.NoError(t, collr.Check(context.Background()))
	require.Empty(t, collr.runtime.normalizers, "the test must exercise a fallback-only profile")
	require.NotEmpty(t, collr.runtime.fallbacks)

	collectProfileRelabelOnce(t, collr)
	got := collr.MetricStore().Read(metrix.ReadRaw(), metrix.ReadFlatten())
	assert.InDelta(t, 7, value(t, got, "app_value", metrix.Labels{"id": "one"}), 1e-9)
	assert.InDelta(t, 11, value(t, got, "other_requests_total", nil), 1e-9)
	require.Contains(t, collr.writer.handles, "app_value")
	assert.Equal(t, commonmodel.MetricTypeGauge, collr.writer.handles["app_value"].typ)
	require.Contains(t, collr.writer.handles, "other_requests_total")
	assert.Equal(t, commonmodel.MetricTypeCounter, collr.writer.handles["other_requests_total"].typ)

	engine, err := chartengine.New()
	require.NoError(t, err)
	require.NoError(t, engine.LoadYAML([]byte(collr.ChartTemplateYAML()), 1))
	attempt, err := engine.PreparePlan(got)
	require.NoError(t, err)
	defer attempt.Abort()

	var curatedValue *chartengine.UpdateDimensionValue
	for _, action := range attempt.Plan().Actions {
		update, ok := action.(chartengine.UpdateChartAction)
		if !ok {
			continue
		}
		for i := range update.Values {
			if update.Values[i].Name == "up" {
				curatedValue = &update.Values[i]
			}
		}
	}
	require.NotNil(t, curatedValue, "the profile-classified family must reach its curated chart dimension")
	assert.InDelta(t, 7, curatedValue.Float64, 1e-9)
}

func TestCollector_ProfileFallbackUsesPreProfileName(t *testing.T) {
	profile := testProfileYAMLWithFallbackType("app_*", []string{"app_value"}, nil)
	profile = strings.Replace(profile, "template:\n", strings.Join([]string{
		"relabeling:",
		"  - match: app_value",
		"    metric_relabel_configs:",
		"      - source_labels: [__name__]",
		"        regex: app_value",
		"        target_label: __name__",
		"        replacement: app_final",
		"        action: replace",
		"template:",
	}, "\n")+"\n", 1)
	catalog := loadTestCatalog(t, map[string]string{"app": profile})
	collr, srv := newProfileRelabelCollector(t, catalog, "app_value 9\n", "app")
	defer srv.Close()
	require.NoError(t, collr.Init(context.Background()))

	require.NoError(t, collr.Check(context.Background()))
	collectProfileRelabelOnce(t, collr)
	got := collr.MetricStore().Read(metrix.ReadRaw(), metrix.ReadFlatten())
	assert.InDelta(t, 9, value(t, got, "app_final", nil), 1e-9)
	noSeries(t, got, "app_value", nil)
	require.Contains(t, collr.writer.handles, "app_final")
	assert.Equal(t, commonmodel.MetricTypeGauge, collr.writer.handles["app_final"].typ)
}

func TestCollector_ProfileFallbackUsesPostJobName(t *testing.T) {
	catalog := loadTestCatalog(t, map[string]string{
		"app": testProfileYAMLWithFallbackType("app_*", []string{"app_value"}, nil),
	})
	collr, srv := newProfileRelabelCollector(t, catalog, "source_value 9\n", "app")
	defer srv.Close()
	collr.Relabeling = []relabel.Block{{
		Match: "source_value",
		MetricRelabelConfigs: []relabel.Config{{
			SourceLabels: []string{"__name__"},
			Regex:        relabel.MustNewRegexp("source_value"),
			TargetLabel:  "__name__",
			Replacement:  "app_value",
			Action:       relabel.Replace,
		}},
	}}
	require.NoError(t, collr.Init(context.Background()))

	require.NoError(t, collr.Check(context.Background()))
	collectProfileRelabelOnce(t, collr)
	got := collr.MetricStore().Read(metrix.ReadRaw(), metrix.ReadFlatten())
	assert.InDelta(t, 9, value(t, got, "app_value", nil), 1e-9)
	require.Contains(t, collr.writer.handles, "app_value")
	assert.Equal(t, commonmodel.MetricTypeGauge, collr.writer.handles["app_value"].typ)
}

func TestCollector_ProfileFallbackCannotBeCreatedByProfileRelabeling(t *testing.T) {
	profile := testProfileYAMLWithFallbackType("app_*", []string{"app_final"}, nil)
	profile = strings.Replace(profile, "template:\n", strings.Join([]string{
		"relabeling:",
		"  - match: app_value",
		"    metric_relabel_configs:",
		"      - source_labels: [__name__]",
		"        regex: app_value",
		"        target_label: __name__",
		"        replacement: app_final",
		"        action: replace",
		"template:",
	}, "\n")+"\n", 1)
	catalog := loadTestCatalog(t, map[string]string{"app": profile})
	collr, srv := newProfileRelabelCollector(t, catalog, "# TYPE app_safe gauge\napp_safe 1\napp_value 9\n", "app")
	defer srv.Close()
	require.NoError(t, collr.Init(context.Background()))

	require.NoError(t, collr.Check(context.Background()))
	collectProfileRelabelOnce(t, collr)
	got := collr.MetricStore().Read(metrix.ReadRaw(), metrix.ReadFlatten())
	assert.InDelta(t, 1, value(t, got, "app_safe", nil), 1e-9)
	noSeries(t, got, "app_final", nil)
}

func TestCollector_ProfileFallbackUsesNormalizationOrder(t *testing.T) {
	catalog := loadTestCatalog(t, map[string]string{
		"alpha": testProfileYAMLWithFallbackType("app_*", []string{"app_value"}, nil),
		"zeta":  testProfileYAMLWithFallbackType("app_*", nil, []string{"app_value"}),
	})
	tests := map[string]struct {
		profiles ProfilesConfig
		want     commonmodel.MetricType
	}{
		"auto uses profile-name order": {
			profiles: ProfilesConfig{Mode: profilesModeAuto},
			want:     commonmodel.MetricTypeGauge,
		},
		"exact preserves configured order": {
			profiles: ProfilesConfig{Mode: profilesModeExact, ModeExact: &ProfilesModeConfig{
				Entries: profileEntries("zeta", "alpha"),
			}},
			want: commonmodel.MetricTypeCounter,
		},
		"combined entries precede auto profiles": {
			profiles: ProfilesConfig{Mode: profilesModeCombined, ModeCombined: &ProfilesModeConfig{
				Entries: profileEntries("zeta"),
			}},
			want: commonmodel.MetricTypeCounter,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			collr, srv := newProfileFallbackCollector(t, catalog, "app_value 1\n", tc.profiles)
			defer srv.Close()
			require.NoError(t, collr.Init(context.Background()))
			require.NoError(t, collr.Check(context.Background()))
			collectProfileRelabelOnce(t, collr)

			require.Contains(t, collr.writer.handles, "app_value")
			assert.Equal(t, tc.want, collr.writer.handles["app_value"].typ)
		})
	}
}

func TestCollector_ProfileFallbackRequiresSelection(t *testing.T) {
	catalog := loadTestCatalog(t, map[string]string{
		"active":   testProfileYAML("app_*"),
		"inactive": testProfileYAMLWithFallbackType("app_*", []string{"app_value"}, nil),
	})
	collr, srv := newProfileRelabelCollector(t, catalog, "app_value 1\n", "active")
	defer srv.Close()
	require.NoError(t, collr.Init(context.Background()))

	err := collr.Check(context.Background())
	require.ErrorContains(t, err, "exposes no usable metrics")
	assert.Nil(t, collr.runtime)
}

func TestCollector_ProfileFallbackIsRootScoped(t *testing.T) {
	catalog := loadTestCatalog(t, map[string]string{
		"app": testProfileYAMLWithFallbackType("app_*", []string{"*"}, nil),
	})
	collr, srv := newProfileRelabelCollector(t, catalog, "# TYPE app_safe gauge\napp_safe 1\nother_value 2\n", "app")
	defer srv.Close()
	require.NoError(t, collr.Init(context.Background()))
	require.NoError(t, collr.Check(context.Background()))

	collectProfileRelabelOnce(t, collr)
	got := collr.MetricStore().Read(metrix.ReadRaw(), metrix.ReadFlatten())
	assert.InDelta(t, 1, value(t, got, "app_safe", nil), 1e-9)
	noSeries(t, got, "other_value", nil)
}

func TestCollector_JobFallbackOverridesProfileFallback(t *testing.T) {
	catalog := loadTestCatalog(t, map[string]string{
		"app": testProfileYAMLWithFallbackType("app_*", []string{"app_value"}, nil),
	})
	collr, srv := newProfileRelabelCollector(t, catalog, "app_value 1\n", "app")
	defer srv.Close()
	collr.FallbackType.Counter = []string{"app_value"}
	require.NoError(t, collr.Init(context.Background()))
	require.NoError(t, collr.Check(context.Background()))

	collectProfileRelabelOnce(t, collr)
	require.Contains(t, collr.writer.handles, "app_value")
	assert.Equal(t, commonmodel.MetricTypeCounter, collr.writer.handles["app_value"].typ)
}

func TestCollector_ProfileFallbackAppliesToFamiliesAppearingAfterCheck(t *testing.T) {
	catalog := loadTestCatalog(t, map[string]string{
		"app": testProfileYAMLWithFallbackType("app_*", []string{"app_dynamic"}, nil),
	})
	var exposition atomic.Value
	exposition.Store("# TYPE app_safe gauge\napp_safe 1\n")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(exposition.Load().(string)))
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

	exposition.Store("app_dynamic 2\n")
	collectProfileRelabelOnce(t, collr)
	got := collr.MetricStore().Read(metrix.ReadRaw(), metrix.ReadFlatten())
	assert.InDelta(t, 2, value(t, got, "app_dynamic", nil), 1e-9)
}

func TestCollector_InvalidProfileFallbackDoesNotReplacePublishedRuntime(t *testing.T) {
	goodCatalog := loadTestCatalog(t, map[string]string{
		"app": testProfileYAMLWithFallbackType("app_*", []string{"app_value"}, nil),
	})
	badCatalog := loadTestCatalog(t, map[string]string{
		"app": strings.Replace(testProfileYAML("app_*"), "template:\n", "fallback_type: {}\ntemplate:\n", 1),
	})
	collr, srv := newProfileRelabelCollector(t, goodCatalog, "app_value 1\n", "app")
	defer srv.Close()
	require.NoError(t, collr.Init(context.Background()))
	require.NoError(t, collr.Check(context.Background()))
	published := collr.runtime

	collr.loadProfileCatalog = func() (promprofiles.Catalog, error) { return badCatalog, nil }
	err := collr.Check(context.Background())
	require.ErrorContains(t, err, "fallback_type")
	assert.Same(t, published, collr.runtime)

	collectProfileRelabelOnce(t, collr)
	got := collr.MetricStore().Read(metrix.ReadRaw(), metrix.ReadFlatten())
	assert.InDelta(t, 1, value(t, got, "app_value", nil), 1e-9)
}

func TestCollector_ProfileFallbackDoesNotShareEligibilityAcrossRelabelMerge(t *testing.T) {
	profile := testProfileYAMLWithFallbackType("app_*", []string{"app_bound"}, nil)
	profile = strings.Replace(profile, "template:\n", strings.Join([]string{
		"relabeling:",
		"  - match: app_*",
		"    metric_relabel_configs:",
		"      - source_labels: [__name__]",
		"        regex: app_.*",
		"        target_label: __name__",
		"        replacement: app_final",
		"        action: replace",
		"template:",
	}, "\n")+"\n", 1)
	catalog := loadTestCatalog(t, map[string]string{"app": profile})
	collr, srv := newProfileRelabelCollector(t, catalog, "app_bound{id=\"bound\"} 1\napp_unbound{id=\"unbound\"} 2\n", "app")
	defer srv.Close()
	require.NoError(t, collr.Init(context.Background()))
	require.NoError(t, collr.Check(context.Background()))

	collectProfileRelabelOnce(t, collr)
	got := collr.MetricStore().Read(metrix.ReadRaw(), metrix.ReadFlatten())
	assert.InDelta(t, 1, value(t, got, "app_final", metrix.Labels{"id": "bound"}), 1e-9)
	noSeries(t, got, "app_final", metrix.Labels{"id": "unbound"})
}

func newProfileFallbackCollector(
	t *testing.T,
	catalog promprofiles.Catalog,
	input string,
	profiles ProfilesConfig,
) (*Collector, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(input))
	}))
	collr := New()
	collr.URL = srv.URL
	collr.Profiles = profiles
	collr.loadProfileCatalog = func() (promprofiles.Catalog, error) { return catalog, nil }
	return collr, srv
}

func testProfileYAMLWithFallbackType(match string, gauge, counter []string) string {
	var fallback strings.Builder
	fallback.WriteString("fallback_type:\n")
	if gauge != nil {
		fallback.WriteString("  gauge:\n")
		for _, pattern := range gauge {
			fmt.Fprintf(&fallback, "    - %q\n", pattern)
		}
	}
	if counter != nil {
		fallback.WriteString("  counter:\n")
		for _, pattern := range counter {
			fmt.Fprintf(&fallback, "    - %q\n", pattern)
		}
	}
	return strings.Replace(testProfileYAML(match), "template:\n", fallback.String()+"template:\n", 1)
}
