// SPDX-License-Identifier: GPL-3.0-or-later

package chartemit

import (
	"cmp"
	"slices"
	"sort"

	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
)

// PlanInspection contains the chart and dimension definitions ApplyPlan would
// emit after production normalization and type-ID budget validation. Value
// updates and host-selection commands are intentionally omitted.
type PlanInspection struct {
	Charts     []ChartInspection
	Dimensions []DimensionInspection
}

type ChartInspection struct {
	SourceChartID string
	SourceContext string
	WireTypeID    string
	WireChartID   string
	WireContext   string
	Obsolete      bool
}

type DimensionInspection struct {
	SourceChartID string
	SourceName    string
	WireTypeID    string
	WireChartID   string
	WireName      string
	Obsolete      bool
}

// InspectPlan returns structured definition facts from the same normalization,
// sanitization, ordering, and type-ID budget used by ApplyPlan.
func InspectPlan(plan Plan, env EmitEnv) (PlanInspection, error) {
	if err := validateEmitEnv(env); err != nil {
		return PlanInspection{}, err
	}
	actions := normalizeActions(plan.Actions)
	if err := validateTypeIDBudget(env.TypeID, actions); err != nil {
		return PlanInspection{}, err
	}

	var out PlanInspection
	inspectCreatePhase(&out, env, actions)
	inspectLabelUpdatePhase(&out, env, actions.updateLabels)
	inspectRemovePhase(&out, env, actions)
	return out, nil
}

func inspectCreatePhase(out *PlanInspection, env EmitEnv, actions normalizedActions) {
	createdChartIDs := make([]string, 0, len(actions.createCharts))
	for chartID := range actions.createCharts {
		createdChartIDs = append(createdChartIDs, chartID)
	}
	sort.Strings(createdChartIDs)
	for _, chartID := range createdChartIDs {
		createChart := actions.createCharts[chartID]
		out.addChart(env, createChart.ChartID, createChart.Meta, false)
		for _, dim := range actions.createDimsByID[chartID] {
			out.addDimension(env, dim.ChartID, dimensionEmission{
				Name:       dim.Name,
				Hidden:     dim.Hidden,
				Float:      dim.Float,
				Algorithm:  string(dim.Algorithm),
				Multiplier: dim.Multiplier,
				Divisor:    dim.Divisor,
			})
		}
		delete(actions.createDimsByID, chartID)
	}

	remainingChartIDs := make([]string, 0, len(actions.createDimsByID))
	for chartID := range actions.createDimsByID {
		remainingChartIDs = append(remainingChartIDs, chartID)
	}
	sort.Strings(remainingChartIDs)
	for _, chartID := range remainingChartIDs {
		dims := actions.createDimsByID[chartID]
		if len(dims) == 0 {
			continue
		}
		out.addChart(env, chartID, dims[0].ChartMeta, false)
		for _, dim := range dims {
			out.addDimension(env, chartID, dimensionEmission{
				Name:       dim.Name,
				Hidden:     dim.Hidden,
				Float:      dim.Float,
				Algorithm:  string(dim.Algorithm),
				Multiplier: dim.Multiplier,
				Divisor:    dim.Divisor,
			})
		}
	}
}

func inspectLabelUpdatePhase(out *PlanInspection, env EmitEnv, updates []UpdateChartLabelsAction) {
	if len(updates) > 1 {
		slices.SortFunc(updates, func(a, b UpdateChartLabelsAction) int {
			return cmp.Compare(a.ChartID, b.ChartID)
		})
	}
	for _, update := range updates {
		out.addChart(env, update.ChartID, update.Meta, false)
	}
}

func inspectRemovePhase(out *PlanInspection, env EmitEnv, actions normalizedActions) {
	for _, removeDim := range actions.removeDimensions {
		out.addChart(env, removeDim.ChartID, removeDim.ChartMeta, false)
		out.addDimension(env, removeDim.ChartID, dimensionEmission{
			Name:       removeDim.Name,
			Hidden:     removeDim.Hidden,
			Float:      removeDim.Float,
			Algorithm:  string(removeDim.Algorithm),
			Multiplier: removeDim.Multiplier,
			Divisor:    removeDim.Divisor,
			Obsolete:   true,
		})
	}
	for _, removeChart := range actions.removeCharts {
		out.addChart(env, removeChart.ChartID, removeChart.Meta, true)
	}
}

func (p *PlanInspection) addChart(env EmitEnv, chartID string, meta chartengine.ChartMeta, obsolete bool) {
	opts := prepareChart(env, chartID, meta, obsolete)
	p.Charts = append(p.Charts, ChartInspection{
		SourceChartID: chartID,
		SourceContext: meta.Context,
		WireTypeID:    opts.TypeID,
		WireChartID:   opts.ID,
		WireContext:   opts.Context,
		Obsolete:      obsolete,
	})
}

func (p *PlanInspection) addDimension(env EmitEnv, chartID string, dim dimensionEmission) {
	opts, ok := prepareDimension(dim)
	if !ok {
		return
	}
	p.Dimensions = append(p.Dimensions, DimensionInspection{
		SourceChartID: chartID,
		SourceName:    dim.Name,
		WireTypeID:    sanitizeWireID(env.TypeID),
		WireChartID:   sanitizeWireID(chartID),
		WireName:      opts.ID,
		Obsolete:      dim.Obsolete,
	})
}
