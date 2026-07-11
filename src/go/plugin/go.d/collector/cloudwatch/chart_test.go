// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/cwprofiles"
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
	c.runtime = &collectorRuntime{Profiles: profs}
	return c
}

func TestBuildChartSpec_InjectsDimensionOptions(t *testing.T) {
	c := chartTestCollector(t, "ec2")

	spec := buildChartSpec(c.runtime.Profiles)
	require.Len(t, spec.Groups, 1)

	group := spec.Groups[0]
	assert.NotEmpty(t, group.Metrics, "template.metrics (visible series) is injected")

	series := profileSeries("ec2", c.runtime.Profiles[0].Config)
	dims := 0
	for _, chart := range group.Charts {
		for _, d := range chart.Dimensions {
			dims++
			// Float is a metric hint (metrix.WithFloat), not a template option.
			// Only rate dimensions get an injected divisor; gauges get no options.
			if pres := series[selectorSeriesName(d.Selector)]; pres.rate {
				require.NotNilf(t, d.Options, "rate dimension %q has options", d.Selector)
				assert.Equalf(t, pres.period, d.Options.Divisor, "rate dimension gets divisor=period: %q", d.Selector)
			} else {
				assert.Nilf(t, d.Options, "gauge dimension %q gets no injected options", d.Selector)
			}
		}
	}
	assert.Positive(t, dims)
}

func TestBuildChartSpec_DoesNotMutateCatalog(t *testing.T) {
	c := chartTestCollector(t, "ec2")

	// Building the spec deep-copies each profile template before injecting options.
	buildChartSpec(c.runtime.Profiles)

	// The deep copy must leave the resolved profile's template untouched.
	for _, chart := range c.runtime.Profiles[0].Config.Template.Charts {
		for _, d := range chart.Dimensions {
			assert.Nilf(t, d.Options, "catalog profile dimension %q must not be mutated", d.Selector)
		}
	}
}

func TestInjectDimensionOptions_NestedGroups(t *testing.T) {
	group := charttpl.Group{
		Charts: []charttpl.Chart{{Dimensions: []charttpl.Dimension{{Selector: "ec2.network_in_sum"}}}},
		Groups: []charttpl.Group{{
			Charts: []charttpl.Chart{{Dimensions: []charttpl.Dimension{
				{Selector: "ec2.network_out_sum"},         // rate -> divisor (proves recursion descends)
				{Selector: "ec2.cpu_utilization_average"}, // gauge -> no injected options
			}}},
		}},
	}
	series := map[string]seriesPresentation{
		"ec2.network_in_sum":          {rate: true, period: 300},
		"ec2.network_out_sum":         {rate: true, period: 60},
		"ec2.cpu_utilization_average": {rate: false, period: 300},
	}

	injectDimensionOptions(&group, series)

	top := group.Charts[0].Dimensions[0].Options
	require.NotNil(t, top, "top-level rate dim gets options")
	assert.Equal(t, 300, top.Divisor, "top-level rate dim gets divisor=period")

	nestedRate := group.Groups[0].Charts[0].Dimensions[0].Options
	require.NotNil(t, nestedRate, "recursion reaches nested-group dimensions")
	assert.Equal(t, 60, nestedRate.Divisor, "nested rate dim gets its divisor=period")

	nestedGauge := group.Groups[0].Charts[0].Dimensions[1].Options
	assert.Nil(t, nestedGauge, "nested gauge dim gets no injected options (float is a metric hint)")
}

func TestInjectDimensionOptions_PreservesAuthoredOptions(t *testing.T) {
	// A rate dimension may carry author-defined options; injecting the divisor must
	// set only Divisor and preserve the rest, not replace the whole options struct.
	group := charttpl.Group{
		Charts: []charttpl.Chart{{Dimensions: []charttpl.Dimension{
			{Selector: "ec2.network_in_sum", Options: &charttpl.DimensionOptions{Multiplier: 8, Hidden: true}},
		}}},
	}
	series := map[string]seriesPresentation{"ec2.network_in_sum": {rate: true, period: 300}}

	injectDimensionOptions(&group, series)

	opts := group.Charts[0].Dimensions[0].Options
	require.NotNil(t, opts)
	assert.Equal(t, 300, opts.Divisor, "rate divisor is injected")
	assert.Equal(t, 8, opts.Multiplier, "authored multiplier is preserved")
	assert.True(t, opts.Hidden, "authored hidden flag is preserved")
}

func TestEnsureRuntime_BuildsValidChartTemplate(t *testing.T) {
	c := New()
	c.Config = validConfig()
	c.applyDefaults()
	c.newCatalog = cwprofiles.LoadFromDefaultDirs

	cat, err := cwprofiles.LoadFromDefaultDirs()
	require.NoError(t, err)

	enabled := 0
	for _, p := range cat.AllProfiles() {
		if !p.Config.Disabled {
			enabled++
		}
	}

	require.NoError(t, c.ensureRuntime())
	assert.Len(t, c.runtime.Profiles, enabled)

	require.NotEmpty(t, c.chartTemplateYAML)
	collecttest.AssertChartTemplateSchema(t, c.chartTemplateYAML)
}

func TestEnsureRuntime_ExplicitAllBuildsValidChartTemplate(t *testing.T) {
	c := New()
	c.newCatalog = cwprofiles.LoadFromDefaultDirs

	cat, err := cwprofiles.LoadFromDefaultDirs()
	require.NoError(t, err)
	falseValue := false
	var names []string
	for _, profile := range cat.AllProfiles() {
		names = append(names, profile.Name)
	}
	c.Config = validConfig()
	c.Config.Rules[0].Profiles = &ProfileSelectorConfig{Defaults: &falseValue, Include: names}
	c.applyDefaults()

	require.NoError(t, c.ensureRuntime())
	assert.Len(t, c.runtime.Profiles, len(cat.AllProfiles()))

	require.NotEmpty(t, c.chartTemplateYAML)
	collecttest.AssertChartTemplateSchema(t, c.chartTemplateYAML)
}

func TestProfileSeries_RateAppliesToPerPeriodTotals(t *testing.T) {
	// rate applies to the per-period totals (sum, sample_count) but never to a
	// per-observation aggregate (average/maximum/percentile).
	prof := cwprofiles.Profile{
		Period: 300,
		Metrics: []cwprofiles.Metric{
			{ID: "req", Statistics: []string{"sum", "average"}, Rate: true},
			{ID: "evt", Statistics: []string{"sample_count"}, Rate: true},
		},
	}

	series := profileSeries("svc", prof)

	require.Contains(t, series, "svc.req_sum")
	require.Contains(t, series, "svc.evt_sample_count")
	require.Contains(t, series, "svc.req_average")
	assert.True(t, series["svc.req_sum"].rate, "sum series of a rate metric is a per-second rate")
	assert.True(t, series["svc.evt_sample_count"].rate, "sample_count series of a rate metric is a per-second rate")
	assert.False(t, series["svc.req_average"].rate, "a per-observation stat is never a rate (no period divisor)")
}
