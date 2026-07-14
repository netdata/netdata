// SPDX-License-Identifier: GPL-3.0-or-later

package cwprofiles

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
)

// TestStockProfiles_MetricChartCoverage guards every stock metric, including
// opt-in metrics. Every declared series must have a chart dimension, and every
// chart dimension must resolve to a declared series.
func TestStockProfiles_MetricChartCoverage(t *testing.T) {
	catalog, err := LoadFromDefaultDirs()
	require.NoError(t, err)

	disabled := 0
	// Two-way coverage between declared metric series and chart selectors:
	//   (1) every chart selector resolves to a declared metric series (no dangling
	//       selector); Normalize leaves an unresolved shorthand un-qualified, so it
	//       will not be a key in the visible (fully-qualified) series set; and
	//   (2) every declared metric series is targeted by at least one chart dimension.
	//       An un-charted metric would still bill GetMetricData yet render nothing.
	for _, rp := range catalog.AllProfiles() {
		for _, metric := range rp.Config.Metrics {
			if metric.Disabled {
				disabled++
			}
		}
		visible := visibleSeriesForProfile(rp.Name, rp.Config.Metrics)
		selected := make(map[string]struct{})
		for _, sel := range chartSelectors(rp.Config.Template) {
			series, _, ok := splitSelectorSeries(sel)
			if !assert.Truef(t, ok, "%s: unparseable chart selector %q", rp.Name, sel) {
				continue
			}
			selected[series] = struct{}{}
			_, found := visible[series]
			assert.Truef(t, found, "%s: chart selector %q does not resolve to a declared metric series", rp.Name, sel)
		}
		for series := range visible {
			_, charted := selected[series]
			assert.Truef(t, charted, "%s: metric series %q has no chart dimension (would bill GetMetricData but render nothing)", rp.Name, series)
		}
	}
	assert.Positive(t, disabled, "stock catalog must contain opt-in metrics")
}

// chartSelectors returns every chart-dimension selector in a group, recursively.
func chartSelectors(group charttpl.Group) []string {
	var out []string
	for _, c := range group.Charts {
		for _, d := range c.Dimensions {
			out = append(out, d.Selector)
		}
	}
	for _, g := range group.Groups {
		out = append(out, chartSelectors(g)...)
	}
	return out
}
