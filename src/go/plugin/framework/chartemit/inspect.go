// SPDX-License-Identifier: GPL-3.0-or-later

package chartemit

import (
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

type inspectionDefinitionVisitor struct {
	out *PlanInspection
	env EmitEnv
}

func (v inspectionDefinitionVisitor) visitChart(
	chartID string,
	meta chartengine.ChartMeta,
	_ map[string]string,
	_, obsolete bool,
) {
	v.out.addChart(v.env, chartID, meta, obsolete)
}

func (v inspectionDefinitionVisitor) visitDimension(chartID string, dim dimensionEmission) {
	v.out.addDimension(v.env, chartID, dim)
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
	visitor := inspectionDefinitionVisitor{out: &out, env: env}
	visitCreatePhase(visitor, actions)
	visitLabelUpdatePhase(visitor, actions.updateLabels)
	visitRemovePhase(visitor, actions)
	return out, nil
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
