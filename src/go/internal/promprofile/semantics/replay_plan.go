// SPDX-License-Identifier: GPL-3.0-or-later

package promsemantics

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/netdata/netdata/go/plugins/internal/promprofile/replay"
)

type semanticChartPlanExpectation struct {
	templateID   string
	context      string
	family       string
	units        string
	presentation string
	labels       []promreplay.SemanticLabel
	dimensions   map[string]semanticDimensionPlanExpectation
}

type semanticDimensionPlanExpectation struct {
	algorithm  string
	multiplier int64
	divisor    int64
}

type semanticWireChartOwner struct {
	chartID string
	context string
}

type semanticWireDimensionOwner struct {
	chartID string
	name    string
}

// ReconcileProductionPlan validates structured chartemit identities and any
// create/update definitions emitted for the currently routed authored charts.
// Action presence across cycles is reconciled by the later persistent runner.
func (c *CompiledSemanticCase) ReconcileProductionPlan(
	ctx context.Context,
	snapshot *promreplay.SemanticSnapshot,
	reconciled *ReconciledSemanticCase,
) error {
	if err := checkSemanticContext(ctx, "before production plan reconciliation"); err != nil {
		return err
	}
	if c == nil || c.root == nil {
		return fmt.Errorf("production plan reconciliation: compiled semantic case is nil")
	}
	if snapshot == nil || reconciled == nil {
		return fmt.Errorf("production plan reconciliation: snapshot and reconciled case must be present")
	}
	policies, err := indexSemanticChartPolicies(snapshot)
	if err != nil {
		return err
	}
	expected, err := expectedSemanticPlan(snapshot, reconciled, policies)
	if err != nil {
		return err
	}
	return reconcileSemanticPlanActions(ctx, snapshot.PlanActions, expected)
}

func expectedSemanticPlan(
	snapshot *promreplay.SemanticSnapshot,
	reconciled *ReconciledSemanticCase,
	policies map[string]promreplay.SemanticChartPolicy,
) (map[string]*semanticChartPlanExpectation, error) {
	out := make(map[string]*semanticChartPlanExpectation)
	var errs []error
	for _, edge := range reconciled.Edges {
		route, err := semanticRouteForEdge(snapshot, edge)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		policy, ok := policies[semanticChartPolicyKey(edge.DestinationProfile, edge.TemplatePath)]
		if !ok {
			errs = append(errs, fmt.Errorf("semantic edge references missing chart policy %s/%s",
				edge.DestinationProfile, edge.TemplatePath))
			continue
		}
		want := &semanticChartPlanExpectation{
			templateID:   policy.TemplateID,
			context:      route.Context,
			family:       route.DisplayedFamily,
			units:        route.Units,
			presentation: route.Presentation,
			labels:       canonicalSemanticLabelValues(route.ChartLabelValues),
			dimensions:   make(map[string]semanticDimensionPlanExpectation),
		}
		if previous := out[route.ChartID]; previous != nil {
			if mismatch := compareSemanticChartPlanExpectation(previous, want); mismatch != "" {
				errs = append(errs, fmt.Errorf("chart %q has incompatible semantic route definitions: %s",
					route.ChartID, mismatch))
				continue
			}
			want = previous
		} else {
			out[route.ChartID] = want
		}
		dimension := semanticDimensionPlanExpectation{
			algorithm: route.Algorithm, multiplier: route.Multiplier, divisor: route.Divisor,
		}
		if previous, ok := want.dimensions[route.DimensionName]; ok && previous != dimension {
			errs = append(errs, fmt.Errorf("chart %q dimension %q has incompatible semantic route definitions",
				route.ChartID, route.DimensionName))
			continue
		}
		want.dimensions[route.DimensionName] = dimension
	}
	return out, errors.Join(errs...)
}

func reconcileSemanticPlanActions(
	ctx context.Context,
	actions []promreplay.SemanticPlanAction,
	expected map[string]*semanticChartPlanExpectation,
) error {
	chartWireOwners := make(map[string]semanticWireChartOwner)
	chartInternalWire := make(map[string]string)
	contextToWire := make(map[string]string)
	wireToContext := make(map[string]string)
	dimensionWireOwners := make(map[string]semanticWireDimensionOwner)
	dimensionInternalWire := make(map[string]string)
	createdCharts := make(map[string]int)
	labelUpdates := make(map[string]int)
	createdDimensions := make(map[string]int)
	var errs []error
	for index, action := range actions {
		if err := checkSemanticContext(ctx, "during production plan reconciliation"); err != nil {
			return err
		}
		field := fmt.Sprintf("plan action %d (%s)", index, action.Kind)
		switch action.Kind {
		case "create_chart", "update_chart_labels", "remove_chart":
			if err := reconcileSemanticWireChart(
				field, action, chartWireOwners, chartInternalWire, contextToWire, wireToContext,
			); err != nil {
				errs = append(errs, err)
				continue
			}
		case "create_dimension", "remove_dimension":
			if err := reconcileSemanticWireDimension(
				field, action, chartWireOwners, chartInternalWire, dimensionWireOwners, dimensionInternalWire,
			); err != nil {
				errs = append(errs, err)
				continue
			}
		case "update_dimension":
			continue
		default:
			errs = append(errs, fmt.Errorf("%s has unsupported kind", field))
			continue
		}

		want := expected[action.ChartID]
		if want == nil {
			continue
		}
		switch action.Kind {
		case "create_chart":
			createdCharts[action.ChartID]++
			if mismatch := compareSemanticChartPlanAction(action, want); mismatch != "" {
				errs = append(errs, fmt.Errorf("%s: %s", field, mismatch))
			}
		case "update_chart_labels":
			labelUpdates[action.ChartID]++
			if mismatch := compareSemanticChartPlanAction(action, want); mismatch != "" {
				errs = append(errs, fmt.Errorf("%s: %s", field, mismatch))
			}
		case "create_dimension":
			key := action.ChartID + "\x00" + action.DimensionName
			createdDimensions[key]++
			dimension, ok := want.dimensions[action.DimensionName]
			if !ok {
				errs = append(errs, fmt.Errorf("%s creates unexpected authored dimension %q/%q",
					field, action.ChartID, action.DimensionName))
				continue
			}
			if mismatch := compareSemanticDimensionPlanAction(action, want, dimension); mismatch != "" {
				errs = append(errs, fmt.Errorf("%s: %s", field, mismatch))
			}
		}
	}
	for chartID, count := range createdCharts {
		if count > 1 {
			errs = append(errs, fmt.Errorf("authored chart %q has %d create actions", chartID, count))
		}
		if labelUpdates[chartID] != 0 {
			errs = append(errs, fmt.Errorf("authored chart %q has both create and label-update actions", chartID))
		}
	}
	for chartID, count := range labelUpdates {
		if count > 1 {
			errs = append(errs, fmt.Errorf("authored chart %q has %d label-update actions", chartID, count))
		}
	}
	for key, count := range createdDimensions {
		if count > 1 {
			chartID, name, _ := strings.Cut(key, "\x00")
			errs = append(errs, fmt.Errorf("authored chart %q dimension %q has %d create actions", chartID, name, count))
		}
	}
	return errors.Join(errs...)
}

func reconcileSemanticWireChart(
	field string,
	action promreplay.SemanticPlanAction,
	wireOwners map[string]semanticWireChartOwner,
	internalWire map[string]string,
	contextToWire map[string]string,
	wireToContext map[string]string,
) error {
	if action.ChartID == "" || action.Context == "" || action.WireTypeID == "" ||
		action.WireChartID == "" || action.WireContext == "" {
		return fmt.Errorf("%s has empty internal or public chart identity", field)
	}
	wireKey := action.WireTypeID + "\x00" + action.WireChartID
	owner := semanticWireChartOwner{chartID: action.ChartID, context: action.Context}
	if previous, ok := wireOwners[wireKey]; ok && previous != owner {
		return fmt.Errorf("%s public chart identity collides between %#v and %#v", field, previous, owner)
	}
	wireOwners[wireKey] = owner
	if previous, ok := internalWire[action.ChartID]; ok && previous != wireKey {
		return fmt.Errorf("%s internal chart %q maps to public identities %q and %q",
			field, action.ChartID, previous, wireKey)
	}
	internalWire[action.ChartID] = wireKey
	if previous, ok := contextToWire[action.Context]; ok && previous != action.WireContext {
		return fmt.Errorf("%s internal context %q maps to public contexts %q and %q",
			field, action.Context, previous, action.WireContext)
	}
	contextToWire[action.Context] = action.WireContext
	if previous, ok := wireToContext[action.WireContext]; ok && previous != action.Context {
		return fmt.Errorf("%s public context %q collides between internal contexts %q and %q",
			field, action.WireContext, previous, action.Context)
	}
	wireToContext[action.WireContext] = action.Context
	return nil
}

func reconcileSemanticWireDimension(
	field string,
	action promreplay.SemanticPlanAction,
	chartWireOwners map[string]semanticWireChartOwner,
	chartInternalWire map[string]string,
	wireOwners map[string]semanticWireDimensionOwner,
	internalWire map[string]string,
) error {
	if action.ChartID == "" || action.DimensionName == "" || action.WireTypeID == "" ||
		action.WireChartID == "" || action.WireDimensionID == "" {
		return fmt.Errorf("%s has empty internal or public dimension identity", field)
	}
	chartWireKey := action.WireTypeID + "\x00" + action.WireChartID
	if previous, ok := chartInternalWire[action.ChartID]; ok && previous != chartWireKey {
		return fmt.Errorf("%s dimension parent maps chart %q to public identity %q, want %q",
			field, action.ChartID, chartWireKey, previous)
	}
	chartInternalWire[action.ChartID] = chartWireKey
	if previous, ok := chartWireOwners[chartWireKey]; ok && previous.chartID != action.ChartID {
		return fmt.Errorf("%s dimension parent public chart identity belongs to %q", field, previous.chartID)
	}
	dimensionWireKey := chartWireKey + "\x00" + action.WireDimensionID
	owner := semanticWireDimensionOwner{chartID: action.ChartID, name: action.DimensionName}
	if previous, ok := wireOwners[dimensionWireKey]; ok && previous != owner {
		return fmt.Errorf("%s public dimension identity collides between %#v and %#v", field, previous, owner)
	}
	wireOwners[dimensionWireKey] = owner
	internalKey := action.ChartID + "\x00" + action.DimensionName
	if previous, ok := internalWire[internalKey]; ok && previous != dimensionWireKey {
		return fmt.Errorf("%s internal dimension %q maps to public identities %q and %q",
			field, internalKey, previous, dimensionWireKey)
	}
	internalWire[internalKey] = dimensionWireKey
	return nil
}

func compareSemanticChartPlanExpectation(
	left, right *semanticChartPlanExpectation,
) string {
	switch {
	case left.templateID != right.templateID:
		return fmt.Sprintf("template IDs %q and %q", left.templateID, right.templateID)
	case left.context != right.context:
		return fmt.Sprintf("contexts %q and %q", left.context, right.context)
	case left.family != right.family:
		return fmt.Sprintf("families %q and %q", left.family, right.family)
	case left.units != right.units:
		return fmt.Sprintf("units %q and %q", left.units, right.units)
	case left.presentation != right.presentation:
		return fmt.Sprintf("presentations %q and %q", left.presentation, right.presentation)
	case !slices.EqualFunc(left.labels, right.labels, equalSemanticLabelValue):
		return fmt.Sprintf("chart labels %v and %v", left.labels, right.labels)
	default:
		return ""
	}
}

func compareSemanticChartPlanAction(
	action promreplay.SemanticPlanAction,
	want *semanticChartPlanExpectation,
) string {
	labels := canonicalSemanticLabelValues(action.Labels)
	switch {
	case action.Kind == "create_chart" && action.ChartTemplateID != want.templateID:
		return fmt.Sprintf("chart template ID got %q, want %q", action.ChartTemplateID, want.templateID)
	case action.Context != want.context:
		return fmt.Sprintf("context got %q, want %q", action.Context, want.context)
	case action.DisplayedFamily != want.family:
		return fmt.Sprintf("family got %q, want %q", action.DisplayedFamily, want.family)
	case action.Units != want.units:
		return fmt.Sprintf("units got %q, want %q", action.Units, want.units)
	case action.Presentation != want.presentation:
		return fmt.Sprintf("presentation got %q, want %q", action.Presentation, want.presentation)
	case !slices.EqualFunc(labels, want.labels, equalSemanticLabelValue):
		return fmt.Sprintf("chart labels got %v, want %v", labels, want.labels)
	default:
		return ""
	}
}

func compareSemanticDimensionPlanAction(
	action promreplay.SemanticPlanAction,
	chart *semanticChartPlanExpectation,
	want semanticDimensionPlanExpectation,
) string {
	switch {
	case action.Context != chart.context:
		return fmt.Sprintf("context got %q, want %q", action.Context, chart.context)
	case action.DisplayedFamily != chart.family:
		return fmt.Sprintf("family got %q, want %q", action.DisplayedFamily, chart.family)
	case action.Units != chart.units:
		return fmt.Sprintf("units got %q, want %q", action.Units, chart.units)
	case action.Presentation != chart.presentation:
		return fmt.Sprintf("presentation got %q, want %q", action.Presentation, chart.presentation)
	case action.Hidden:
		return "authored dimension is unexpectedly hidden"
	case !action.Float:
		return "Prometheus-authored dimension did not inherit writer float storage"
	case action.Algorithm != want.algorithm:
		return fmt.Sprintf("algorithm got %q, want %q", action.Algorithm, want.algorithm)
	case action.Multiplier != want.multiplier || action.Divisor != want.divisor:
		return fmt.Sprintf("scale got %d/%d, want %d/%d",
			action.Multiplier, action.Divisor, want.multiplier, want.divisor)
	default:
		return ""
	}
}

func canonicalSemanticLabelValues(values []promreplay.SemanticLabel) []promreplay.SemanticLabel {
	out := slices.Clone(values)
	slices.SortFunc(out, compareSemanticLabelValues)
	return out
}
