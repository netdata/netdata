// SPDX-License-Identifier: GPL-3.0-or-later

package promvalidation

import (
	"fmt"
	"path/filepath"

	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
)

func inspectAuthoredCharts(root charttpl.Group, rootPath string, r *Report) int {
	var charts int
	var walk func(group charttpl.Group, path string)
	walk = func(group charttpl.Group, path string) {
		for i, chart := range group.Charts {
			charts++
			if chart.Priority != 0 {
				chartPath := fmt.Sprintf("%s.charts[%d]", path, i)
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
	walk(root, rootPath)
	return charts
}
