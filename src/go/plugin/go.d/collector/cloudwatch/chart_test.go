// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"testing"

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
	c.plan = &collectionPlan{Profiles: profs}
	return c
}

func TestBuildChartSpec_InjectsMetricsWithoutStaticRateDivisors(t *testing.T) {
	c := chartTestCollector(t, "ec2")

	spec := buildChartSpec(c.plan.Profiles)
	require.Len(t, spec.Groups, 1)

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

func TestEnsurePlan_BuildsValidChartTemplate(t *testing.T) {
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

	require.NoError(t, c.ensurePlan())
	assert.Len(t, c.plan.Profiles, enabled)

	require.NotEmpty(t, c.chartTemplateYAML)
	collecttest.AssertChartTemplateSchema(t, c.chartTemplateYAML)
}

func TestEnsurePlan_ExplicitAllBuildsValidChartTemplate(t *testing.T) {
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

	require.NoError(t, c.ensurePlan())
	assert.Len(t, c.plan.Profiles, len(cat.AllProfiles()))

	require.NotEmpty(t, c.chartTemplateYAML)
	collecttest.AssertChartTemplateSchema(t, c.chartTemplateYAML)
}

func TestProfileSeries_ContainsEveryExportedStatistic(t *testing.T) {
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
}
