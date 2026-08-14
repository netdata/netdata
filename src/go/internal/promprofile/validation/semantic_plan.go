// SPDX-License-Identifier: GPL-3.0-or-later

package promvalidation

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/internal/promprofile/replay"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartemit"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
)

type semanticChartInspectionKey struct {
	chartID  string
	context  string
	obsolete bool
}

type semanticDimensionInspectionKey struct {
	chartID  string
	name     string
	obsolete bool
}

func semanticPlanActions(
	plan chartengine.Plan,
	inspection chartemit.PlanInspection,
) ([]promreplay.SemanticPlanAction, error) {
	charts := make(map[semanticChartInspectionKey]chartemit.ChartInspection)
	for _, item := range inspection.Charts {
		key := semanticChartInspectionKey{
			chartID: item.SourceChartID, context: item.SourceContext, obsolete: item.Obsolete,
		}
		charts[key] = item
	}
	dimensions := make(map[semanticDimensionInspectionKey]chartemit.DimensionInspection)
	for _, item := range inspection.Dimensions {
		key := semanticDimensionInspectionKey{
			chartID: item.SourceChartID, name: item.SourceName, obsolete: item.Obsolete,
		}
		dimensions[key] = item
	}

	out := make([]promreplay.SemanticPlanAction, 0, len(plan.Actions))
	for _, action := range plan.Actions {
		switch item := action.(type) {
		case chartengine.CreateChartAction:
			wire, ok := charts[semanticChartInspectionKey{chartID: item.ChartID, context: item.Meta.Context}]
			if !ok {
				return nil, fmt.Errorf("semantic plan: created chart %q has no public inspection", item.ChartID)
			}
			out = append(out, semanticChartPlanAction("create_chart", item.ChartID, item.Meta, wire, item.Labels))
			out[len(out)-1].ChartTemplateID = item.ChartTemplateID
		case chartengine.CreateDimensionAction:
			wire, ok := dimensions[semanticDimensionInspectionKey{chartID: item.ChartID, name: item.Name}]
			if !ok {
				return nil, fmt.Errorf("semantic plan: created dimension %q/%q has no public inspection", item.ChartID, item.Name)
			}
			out = append(out, semanticDimensionPlanAction(
				"create_dimension", item.ChartID, item.Name, item.ChartMeta,
				item.Hidden, item.Float, string(item.Algorithm), item.Multiplier, item.Divisor, wire,
			))
		case chartengine.UpdateChartLabelsAction:
			wire, ok := charts[semanticChartInspectionKey{chartID: item.ChartID, context: item.Meta.Context}]
			if !ok {
				return nil, fmt.Errorf("semantic plan: updated chart %q has no public inspection", item.ChartID)
			}
			out = append(out, semanticChartPlanAction("update_chart_labels", item.ChartID, item.Meta, wire, item.Labels))
		case chartengine.UpdateChartAction:
			for _, value := range item.Values {
				out = append(out, promreplay.SemanticPlanAction{
					Kind: "update_dimension", ChartID: item.ChartID, DimensionName: value.Name,
					IsEmpty: value.IsEmpty, Float: value.IsFloat, Int64: value.Int64, Float64: value.Float64,
				})
			}
		case chartengine.RemoveDimensionAction:
			wire, ok := dimensions[semanticDimensionInspectionKey{chartID: item.ChartID, name: item.Name, obsolete: true}]
			if !ok {
				return nil, fmt.Errorf("semantic plan: removed dimension %q/%q has no public inspection", item.ChartID, item.Name)
			}
			out = append(out, semanticDimensionPlanAction(
				"remove_dimension", item.ChartID, item.Name, item.ChartMeta,
				item.Hidden, item.Float, string(item.Algorithm), item.Multiplier, item.Divisor, wire,
			))
		case chartengine.RemoveChartAction:
			wire, ok := charts[semanticChartInspectionKey{
				chartID: item.ChartID, context: item.Meta.Context, obsolete: true,
			}]
			if !ok {
				return nil, fmt.Errorf("semantic plan: removed chart %q has no public inspection", item.ChartID)
			}
			out = append(out, semanticChartPlanAction("remove_chart", item.ChartID, item.Meta, wire, nil))
		default:
			return nil, fmt.Errorf("semantic plan: unsupported chartengine action %T", action)
		}
	}
	return out, nil
}

func semanticChartPlanAction(
	kind, chartID string,
	meta chartengine.ChartMeta,
	wire chartemit.ChartInspection,
	labels map[string]string,
) promreplay.SemanticPlanAction {
	return promreplay.SemanticPlanAction{
		Kind:            kind,
		ChartID:         chartID,
		Context:         meta.Context,
		DisplayedFamily: meta.Family,
		Units:           meta.Units,
		Presentation:    string(meta.Type),
		Labels:          semanticStringMapLabels(labels),
		WireTypeID:      wire.WireTypeID,
		WireChartID:     wire.WireChartID,
		WireContext:     wire.WireContext,
	}
}

func semanticDimensionPlanAction(
	kind, chartID, name string,
	meta chartengine.ChartMeta,
	hidden, float bool,
	algorithm string,
	multiplier, divisor int,
	wire chartemit.DimensionInspection,
) promreplay.SemanticPlanAction {
	return promreplay.SemanticPlanAction{
		Kind:            kind,
		ChartID:         chartID,
		Context:         meta.Context,
		DisplayedFamily: meta.Family,
		Units:           meta.Units,
		Presentation:    string(meta.Type),
		DimensionName:   name,
		Algorithm:       algorithm,
		Hidden:          hidden,
		Float:           float,
		Multiplier:      int64(multiplier),
		Divisor:         int64(divisor),
		WireTypeID:      wire.WireTypeID,
		WireChartID:     wire.WireChartID,
		WireDimensionID: wire.WireName,
	}
}
