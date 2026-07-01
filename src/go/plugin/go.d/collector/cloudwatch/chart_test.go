// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/cwprofiles"
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
	c.Config.Regions = []string{"us-east-1"}
	c.applyDefaults()
	c.profiles = profs
	return c
}

func TestBuildChartSpec_InjectsDimensionOptions(t *testing.T) {
	c := chartTestCollector(t, "ec2")

	spec := buildChartSpec(c.profiles)
	require.Len(t, spec.Groups, 1)

	group := spec.Groups[0]
	assert.NotEmpty(t, group.Metrics, "template.metrics (visible series) is injected")

	series := profileSeries("ec2", c.profiles[0].Config)
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
	buildChartSpec(c.profiles)

	// The deep copy must leave the resolved profile's template untouched.
	for _, chart := range c.profiles[0].Config.Template.Charts {
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

func TestEnsureProfiles_BuildsValidChartTemplate(t *testing.T) {
	c := New()
	c.Config.Regions = []string{"us-east-1"}
	c.Namespaces = NamespacesConfig{Mode: namespacesModeAuto}
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

	require.NoError(t, c.ensureProfiles())
	assert.Len(t, c.profiles, enabled)

	require.NotEmpty(t, c.chartTemplateYAML)
	collecttest.AssertChartTemplateSchema(t, c.chartTemplateYAML)
}

func TestEnsureProfiles_CombinedBuildsValidChartTemplate(t *testing.T) {
	c := New()
	c.Config.Regions = []string{"us-east-1"}
	c.Namespaces = NamespacesConfig{Mode: namespacesModeCombined}
	c.applyDefaults()
	c.newCatalog = cwprofiles.LoadFromDefaultDirs

	cat, err := cwprofiles.LoadFromDefaultDirs()
	require.NoError(t, err)

	require.NoError(t, c.ensureProfiles())
	assert.Len(t, c.profiles, len(cat.AllProfiles())) // combined = every profile, incl. deep-grain

	require.NotEmpty(t, c.chartTemplateYAML)
	collecttest.AssertChartTemplateSchema(t, c.chartTemplateYAML)
}
