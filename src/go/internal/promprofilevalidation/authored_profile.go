// SPDX-License-Identifier: GPL-3.0-or-later

package promprofilevalidation

import (
	"fmt"
	"path/filepath"

	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/promprofiles"
)

type authoredChart struct {
	path     string
	priority int
}

func inspectAuthoredCharts(profile promprofiles.Profile, r *report) ([]authoredChart, error) {
	root, err := profile.Template()
	if err != nil {
		return nil, err
	}
	var charts []authoredChart
	seenPriorities := make(map[int]string)
	var previous *authoredChart
	var walk func(group charttpl.Group, path string)
	walk = func(group charttpl.Group, path string) {
		for i, chart := range group.Charts {
			chartPath := fmt.Sprintf("%s.charts[%d]", path, i)
			current := authoredChart{path: chartPath, priority: chart.Priority}
			charts = append(charts, current)
			switch {
			case chart.Priority <= 0:
				r.addError(
					"priority_missing",
					chartPath,
					fmt.Sprintf("chart %q has no explicit positive priority", chart.Title),
					"Missing and zero priorities collapse to 70000; the author must decide the operator-facing presentation order.",
				)
			default:
				if first, ok := seenPriorities[chart.Priority]; ok {
					r.addWarning(
						"priority_duplicate",
						chartPath,
						fmt.Sprintf("priority %d is already used by %s", chart.Priority, first),
						"Unique priorities express a deterministic total order; a tie can be deliberate, but otherwise dashboard placement falls back to unrelated chart-ID ordering.",
					)
				} else {
					seenPriorities[chart.Priority] = chartPath
				}
				if previous != nil && chart.Priority < previous.priority {
					r.addError(
						"priority_source_order",
						chartPath,
						fmt.Sprintf("priority %d does not follow %d from %s", chart.Priority, previous.priority, previous.path),
						"YAML family/chart order must mirror dashboard presentation order so the authored operator journey is reviewable. Reorder the source or correct the priorities; deliberate ties remain available when a total order is unnecessary.",
					)
				}
				previous = &current
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
