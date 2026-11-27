// SPDX-License-Identifier: GPL-3.0-or-later

package chartemit

import (
	"maps"
	"slices"

	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
)

// EmittedCharts holds the charts ApplyPlan has defined on the Agent and not yet
// marked obsolete, keyed by source chart ID. Each value is the chart metadata
// used to obsolete that chart. A nil set is empty and ready for Record.
type EmittedCharts map[string]chartengine.ChartMeta

// Record updates the set after ApplyPlan emitted plan, in ApplyPlan's phase
// order: created charts take their metadata, charts that only gained
// dimensions join with their first dimension's chart metadata, and removed
// charts leave last, so a chart one plan both defines and removes ends removed.
func (c *EmittedCharts) Record(plan Plan) {
	var hasDimensions, hasRemovals bool
	for _, action := range plan.Actions {
		switch a := action.(type) {
		case CreateChartAction:
			c.set(a.ChartID, a.Meta)
		case CreateDimensionAction:
			hasDimensions = true
		case RemoveChartAction:
			hasRemovals = true
		}
	}
	if hasDimensions {
		for _, action := range plan.Actions {
			if a, ok := action.(CreateDimensionAction); ok {
				if _, recorded := (*c)[a.ChartID]; !recorded {
					c.set(a.ChartID, a.ChartMeta)
				}
			}
		}
	}
	if hasRemovals {
		for _, action := range plan.Actions {
			if a, ok := action.(RemoveChartAction); ok {
				delete(*c, a.ChartID)
			}
		}
	}
}

func (c *EmittedCharts) set(chartID string, meta chartengine.ChartMeta) {
	if *c == nil {
		*c = make(EmittedCharts)
	}
	(*c)[chartID] = meta
}

// ObsoletePlan returns a plan that marks every chart in the set obsolete, in
// chart ID order.
func (c EmittedCharts) ObsoletePlan() Plan {
	if len(c) == 0 {
		return Plan{}
	}
	actions := make([]EngineAction, 0, len(c))
	for _, chartID := range slices.Sorted(maps.Keys(c)) {
		actions = append(actions, RemoveChartAction{
			ChartID: chartID,
			Meta:    c[chartID],
		})
	}
	return Plan{
		Actions: actions,
	}
}
