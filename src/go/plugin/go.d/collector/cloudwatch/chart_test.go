// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/cwprofiles"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/cwquery"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func chartTestCollector(t *testing.T, baseNames ...string) *Collector {
	t.Helper()
	catalog, err := cwprofiles.LoadFromDefaultDirs()
	require.NoError(t, err)
	want := make(map[string]struct{}, len(baseNames))
	for _, n := range baseNames {
		want[n] = struct{}{}
	}
	var profs []cwprofiles.ResolvedProfile
	for _, p := range catalog.AllProfiles() {
		if _, ok := want[p.Name]; ok {
			profs = append(profs, p)
		}
	}
	require.Len(t, profs, len(baseNames))

	c := New()
	c.plan = &collectionPlan{Profiles: profs}
	return c
}

func TestBuildChartSpec_InjectsMetricsWithoutStaticRateDivisors(t *testing.T) {
	c := chartTestCollector(t, "ec2")

	spec := buildChartSpec(c.plan.Profiles)
	require.Len(t, spec.Groups, 2)

	group := spec.Groups[0]
	assert.NotEmpty(t, group.Metrics, "template.metrics (visible series) is injected")

	dims := 0
	for _, chart := range group.Charts {
		for _, d := range chart.Dimensions {
			dims++
			assert.Nilf(t, d.Options, "dimension %q gets no collector-injected options", d.Selector)
		}
	}
	assert.Positive(t, dims)
}

func TestBuildChartSpec_DoesNotMutateCatalog(t *testing.T) {
	c := chartTestCollector(t, "ec2")

	// Building the spec deep-copies each profile template before injecting options.
	buildChartSpec(c.plan.Profiles)

	// The deep copy must leave the resolved profile's template untouched.
	for _, chart := range c.plan.Profiles[0].Config.Template.Charts {
		for _, d := range chart.Dimensions {
			assert.Nilf(t, d.Options, "catalog profile dimension %q must not be mutated", d.Selector)
		}
	}
}

func TestActivityChartGroupContract(t *testing.T) {
	group := activityChartGroup()
	assert.Equal(t, "Collector Activity", group.Family)
	require.NotNil(t, group.ChartDefaults)
	require.NotNil(t, group.ChartDefaults.Instances)
	assert.Equal(t, []string{"account_id", "region"}, group.ChartDefaults.Instances.ByLabels)
	assert.ElementsMatch(t, []string{
		activityAPICallsMetric,
		activityMetricRequestsMetric,
		activityQueriesMetric,
	}, group.Metrics)

	want := map[string]struct {
		context       string
		units         string
		selector      string
		name          string
		nameFromLabel string
	}{
		"aws_cloudwatch_collector_api_calls": {
			context: "collector_api_calls", units: "calls/s",
			selector: activityAPICallsMetric, nameFromLabel: "operation",
		},
		"aws_cloudwatch_collector_metric_requests": {
			context: "collector_metric_requests", units: "requests/s",
			selector: activityMetricRequestsMetric, name: "requests",
		},
		"aws_cloudwatch_collector_queries": {
			context: "collector_queries", units: "queries/s",
			selector: activityQueriesMetric, nameFromLabel: "profile",
		},
	}
	require.Len(t, group.Charts, len(want))
	for _, chart := range group.Charts {
		expected, ok := want[chart.ID]
		require.True(t, ok, "unexpected chart %q", chart.ID)
		assert.Equal(t, expected.context, chart.Context)
		assert.Equal(t, expected.units, chart.Units)
		assert.Equal(t, "incremental", chart.Algorithm)
		require.Len(t, chart.Dimensions, 1)
		assert.Equal(t, expected.selector, chart.Dimensions[0].Selector)
		assert.Equal(t, expected.name, chart.Dimensions[0].Name)
		assert.Equal(t, expected.nameFromLabel, chart.Dimensions[0].NameFromLabel)
	}
}

func TestEnsurePlan_BuildsValidChartTemplate(t *testing.T) {
	cat, err := cwprofiles.LoadFromDefaultDirs()
	require.NoError(t, err)

	enabled := 0
	for _, p := range cat.AllProfiles() {
		if !p.Config.Disabled {
			enabled++
		}
	}

	var names []string
	for _, profile := range cat.AllProfiles() {
		names = append(names, profile.Name)
	}
	tests := map[string]struct {
		explicitAll bool
		wantCount   int
	}{
		"default-enabled profiles": {wantCount: enabled},
		"explicit all profiles":    {explicitAll: true, wantCount: len(cat.AllProfiles())},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			c := New()
			c.Config = validConfig()
			if tc.explicitAll {
				falseValue := false
				c.Config.Rules[0].Profiles = &ProfileSelectorConfig{Defaults: &falseValue, Include: names}
			}
			c.applyDefaults()
			c.newCatalog = cwprofiles.LoadFromDefaultDirs

			require.NoError(t, c.ensurePlan())
			assert.Len(t, c.plan.Profiles, tc.wantCount)
			require.NotEmpty(t, c.chartTemplateYAML)
			collecttest.AssertChartTemplateSchema(t, c.chartTemplateYAML)
		})
	}
}

func TestProfileSeries_ContainsEveryExportedStatistic(t *testing.T) {
	prof := cwprofiles.Profile{
		Query: cwquery.Config{Period: longDuration(5 * time.Minute)},
		Metrics: []cwprofiles.Metric{
			{ID: "req", Statistics: []string{"sum", "average"}, Rate: true},
			{ID: "evt", Statistics: []string{"sample_count"}, Rate: true},
		},
	}

	series := profileSeries("svc", prof)

	require.Contains(t, series, "svc.req_sum")
	require.Contains(t, series, "svc.evt_sample_count")
	require.Contains(t, series, "svc.req_average")
}
