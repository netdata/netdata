// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"maps"
	"slices"

	metrixselector "github.com/netdata/netdata/go/plugins/pkg/metrix/selector"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/cwprofiles"
)

// seriesPresentation describes how a profile series renders in charts.
type seriesPresentation struct {
	rate   bool
	period int
}

// buildChartTemplate assembles the dynamic chart-template YAML from the selected
// profiles. It is a pure transform of the profiles (no collector state), so it
// is a free function the collector calls with its resolved profiles.
func buildChartTemplate(profiles []cwprofiles.ResolvedProfile) (string, error) {
	return buildChartSpec(profiles).MarshalTemplate()
}

// buildChartSpec builds the chart-template spec, injecting template.metrics (the
// visible series) and options.divisor=period on rate dimensions (per-second
// presentation). Float is a metric-level hint (metrix.WithFloat) inherited by the
// chart dimension, not injected here. Each profile's template is deep-copied
// (Group.Clone) before injection so the shared profile catalog is never mutated.
// The assembled spec is validated when buildChartTemplate marshals it.
func buildChartSpec(profiles []cwprofiles.ResolvedProfile) charttpl.Spec {
	groups := make([]charttpl.Group, 0, len(profiles))
	for _, rp := range profiles {
		group := rp.Config.Template.Clone()
		series := profileSeries(rp.Name, rp.Config)
		group.Metrics = slices.Sorted(maps.Keys(series))
		injectDimensionOptions(&group, series)
		groups = append(groups, group)
	}

	return charttpl.Spec{
		Version:          charttpl.VersionV1,
		ContextNamespace: cwprofiles.ContextNamespace,
		Groups:           groups,
	}
}

// profileSeries maps each exported series name to its presentation.
func profileSeries(profileName string, prof cwprofiles.Profile) map[string]seriesPresentation {
	out := make(map[string]seriesPresentation)
	for _, m := range prof.Metrics {
		period := prof.EffectivePeriod(m)
		for _, stat := range m.Statistics {
			token := cwprofiles.NormalizeStatistic(stat)
			if token == "" {
				continue
			}
			// Rate applies to per-period totals (sum, sample_count): both divide by
			// the period to yield a per-second value. Other statistics (average,
			// maximum, percentiles) are per-observation aggregates and never get the
			// divisor, even when the metric sets rate: true.
			isRate := m.Rate && cwprofiles.IsPerPeriodTotal(token)
			out[cwprofiles.ExportedSeriesName(profileName, m.ID, token)] = seriesPresentation{rate: isRate, period: period}
		}
	}
	return out
}

// injectDimensionOptions sets options.divisor=period on rate dimensions so a
// per-period total renders as a per-second rate. Float is not injected here: it
// is a metric-level hint (metrix.WithFloat on the gauge) that chartengine
// inherits onto the dimension, so every series already renders at full precision.
func injectDimensionOptions(group *charttpl.Group, series map[string]seriesPresentation) {
	for i := range group.Charts {
		for j := range group.Charts[i].Dimensions {
			d := &group.Charts[i].Dimensions[j]
			if pres, ok := series[selectorSeriesName(d.Selector)]; ok && pres.rate {
				// Set only the rate divisor, preserving any author-defined dimension
				// options (multiplier, hidden, float). Group.Clone deep-copies Options,
				// so mutating it in place never touches the shared profile catalog.
				if d.Options == nil {
					d.Options = &charttpl.DimensionOptions{}
				}
				d.Options.Divisor = pres.period
			}
		}
	}
	for i := range group.Groups {
		injectDimensionOptions(&group.Groups[i], series)
	}
}

// selectorSeriesName returns the single series a dimension selector targets,
// parsed with the same selector library chartengine compiles selectors with, so
// the extracted name always agrees with the engine's matching. It returns "" when
// the selector is invalid or targets anything other than exactly one metric, so
// no divisor is injected in those cases.
func selectorSeriesName(selector string) string {
	c, err := metrixselector.ParseCompiled(selector)
	if err != nil {
		return ""
	}
	if names := c.Meta().MetricNames; len(names) == 1 {
		return names[0]
	}
	return ""
}
