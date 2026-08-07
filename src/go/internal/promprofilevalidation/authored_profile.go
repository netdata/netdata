// SPDX-License-Identifier: GPL-3.0-or-later

package promprofilevalidation

import (
	"fmt"
	"path/filepath"

	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/promprofiles"
)

type authoredChart struct {
	path string
}

func inspectAuthoredCharts(profile promprofiles.Profile, r *Report) ([]authoredChart, error) {
	root, err := profile.Template()
	if err != nil {
		return nil, err
	}
	var charts []authoredChart
	var walk func(group charttpl.Group, path string)
	walk = func(group charttpl.Group, path string) {
		for i, chart := range group.Charts {
			chartPath := fmt.Sprintf("%s.charts[%d]", path, i)
			charts = append(charts, authoredChart{path: chartPath})
			if chart.Priority != 0 {
				r.addError(
					"priority_forbidden",
					chartPath,
					fmt.Sprintf("chart %q declares priority %d", chart.Title, chart.Priority),
					"Prometheus profiles give every chart the same runtime priority; omit priority and let the UI own presentation sorting.",
				)
			}
		}
		for i, child := range group.Groups {
			childPath := fmt.Sprintf("%s.groups[%d](%s)", path, i, filepath.Base(child.Family))
			walk(child, childPath)
		}
	}
	walk(root, "template")
	return charts, nil
}
