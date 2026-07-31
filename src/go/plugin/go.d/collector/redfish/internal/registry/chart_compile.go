// SPDX-License-Identifier: GPL-3.0-or-later

package registry

import (
	"fmt"
	"slices"
	"strings"
)

func compileFieldCharts(contract Contract, kinds map[Kind]KindSpec) ([]ChartSpec, error) {
	type groupKey struct {
		kind    Kind
		context string
	}
	type group struct {
		first      FieldSpec
		context    string
		dimensions []DimensionSpec
		roles      map[string]struct{}
	}
	var order []groupKey
	groups := make(map[groupKey]*group)
	for _, field := range contract.Fields {
		if field.Exposure != ExposureOperationalScalar {
			continue
		}
		context := scalarBaseRowContext(field.Context, field.Role)
		key := groupKey{kind: field.Kind, context: context}
		current := groups[key]
		if current == nil {
			current = &group{
				first: field, context: context,
				roles: make(map[string]struct{}),
			}
			groups[key] = current
			order = append(order, key)
		} else if current.first.Units != field.Units ||
			current.first.ComponentClass != field.ComponentClass {
			return nil, fmt.Errorf(
				"resource scalar context %q combines incompatible fields %q and %q",
				context,
				current.first.ID,
				field.ID,
			)
		}
		if _, exists := current.roles[field.Role]; exists {
			return nil, fmt.Errorf("resource scalar context %q has duplicate role %q", context, field.Role)
		}
		current.roles[field.Role] = struct{}{}
		current.dimensions = append(current.dimensions, DimensionSpec{
			ID: field.Role, Name: field.Role, Metric: field.Metric,
			Selector: field.Metric, Algorithm: "absolute", Float: field.Float,
		})
	}

	result := make([]ChartSpec, 0, len(order))
	for _, key := range order {
		current := groups[key]
		kind, err := compiledKind(kinds, "field "+current.first.ID, current.first.Kind)
		if err != nil {
			return nil, err
		}
		titleField := current.first
		titleField.Context = current.context
		titleField.Role = ""
		title, err := fieldTitle(titleField)
		if err != nil {
			return nil, err
		}
		result = append(result, ChartSpec{
			Module:         "redfish",
			ID:             chartID(current.context),
			Context:        current.context,
			BaseRowContext: current.context,
			ScalarRoleRank: current.first.Order + 1,
			Title:          title,
			Units:          current.first.Units,
			Type:           ChartLine,
			Class:          ClassResourceScalar,
			TopFamily:      kind.TopFamily,
			LeafFamily:     kind.LeafFamily,
			InstanceLabels: []string{"endpoint_key", "resource_key"},
			PromotedLabels: promotedLabels(current.first.ComponentClass),
			Dimensions:     current.dimensions,
			ExpireAfter:    5,
		})
	}
	return result, nil
}

func compileReadingCharts(contract Contract) []ChartSpec {
	var result []ChartSpec
	seenCharts := make(map[string]struct{})
	seenAlarms := make(map[string]struct{})
	for _, reading := range contract.Readings {
		if reading.Exposure != ExposureOperationalReading {
			continue
		}
		if _, ok := seenCharts[reading.Context]; !ok {
			seenCharts[reading.Context] = struct{}{}
			class := ClassReadingScalar
			dimension := DimensionSpec{
				ID: "value", Name: "value", Metric: reading.Metric,
				Selector: reading.Metric, Algorithm: "absolute", Float: true,
			}
			promoted := promotedLabels("reading")
			if reading.CommonContext {
				dimension.ID = reading.Role
				dimension.Name = reading.Role
				if !reading.Primary {
					class = ClassReadingAuxiliary
				}
				promoted = append(promoted, "_collect_module")
			}
			result = append(result, ChartSpec{
				Module:         "redfish",
				ID:             chartID(reading.Context),
				Context:        reading.Context,
				BaseRowContext: reading.Context,
				ScalarRoleRank: reading.Order + 1,
				Title:          reading.Title,
				Units:          reading.Units,
				Type:           ChartLine,
				Class:          class,
				TopFamily:      reading.TopFamily,
				LeafFamily:     reading.LeafFamily,
				InstanceLabels: []string{"endpoint_key", "reading_key"},
				PromotedLabels: uniqueStrings(promoted),
				Dimensions:     []DimensionSpec{dimension},
				ExpireAfter:    5,
			})
		}
		if reading.AlarmContext == "" {
			continue
		}
		if _, ok := seenAlarms[reading.AlarmContext]; ok {
			continue
		}
		seenAlarms[reading.AlarmContext] = struct{}{}
		promoted := promotedLabels("reading")
		top, leaf := "Overview", "reading_alarm"
		title := "Redfish Reading Alarm State"
		if reading.CommonContext {
			promoted = append(promoted, "_collect_module")
			top, leaf = reading.TopFamily, reading.LeafFamily
			title = readingFamilyTitles[reading.Family] + " Alarm State"
		}
		result = append(result, ChartSpec{
			Module:         "redfish",
			ID:             chartID(reading.AlarmContext),
			Context:        reading.AlarmContext,
			BaseRowContext: reading.AlarmContext,
			Title:          title,
			Units:          "state",
			Type:           ChartStacked,
			Class:          ClassReadingAlarm,
			TopFamily:      top,
			LeafFamily:     leaf,
			InstanceLabels: []string{"endpoint_key", "reading_key"},
			PromotedLabels: uniqueStrings(promoted),
			Dimensions:     stateDimensions(reading.AlarmMetric, AlarmStates),
			ExpireAfter:    5,
		})
	}
	return result
}

func compileAggregateCharts(contract Contract, _ map[Kind]KindSpec) []ChartSpec {
	var result []ChartSpec
	for _, summary := range contract.SummaryClasses {
		top, leaf := aggregateClassPlacement(summary.ID)
		dimensions := []DimensionSpec{
			aggregateDimension(summary.ID, "minimum", true),
			aggregateDimension(summary.ID, "average", true),
			aggregateDimension(summary.ID, "maximum", true),
		}
		if summary.Additive {
			dimensions = append(dimensions, aggregateDimension(summary.ID, "total", true))
		}
		context := "redfish.aggregate." + summary.ID
		result = append(result, ChartSpec{
			Module: "redfish", ID: chartID(context), Context: context,
			Title: summary.Title + " Summary", Units: summary.Units,
			Type: ChartLine, Class: ClassNumericParent,
			TopFamily: top, LeafFamily: leaf,
			InstanceLabels: []string{"endpoint_key", "aggregate_key"},
			PromotedLabels: promotedLabels("aggregate"),
			Dimensions:     dimensions, ExpireAfter: 5,
		})
	}

	result = append(result,
		aggregateFixedClassChart(
			"population",
			"Redfish Aggregate Population",
			"components",
			[]string{"total", "readable", "unreadable", "unknown", "histogram_eligible", "histogram_ineligible"},
			ChartStacked,
			ClassNumericParent,
		),
		aggregateFixedClassChart(
			"completeness",
			"Redfish Aggregate Completeness",
			"state",
			[]string{"complete", "incomplete", "histogram_available", "histogram_unavailable"},
			ChartStacked,
			ClassNumericParent,
		),
	)
	for _, histogram := range contract.Histograms {
		histogramID := histogram.ID
		dimensions := make([]string, 0, len(histogram.Buckets))
		for _, bucket := range histogram.Buckets {
			dimensions = append(dimensions, bucket.ID)
		}
		result = append(result, aggregateFixedClassChart(
			histogramID+"_distribution",
			histogramTitle(histogramID),
			"components",
			dimensions,
			ChartHeatmap,
			ClassHistogramParent,
		))
	}
	return result
}

func histogramTitle(id string) string {
	switch id {
	case "temperature":
		return "Temperature Distribution"
	case "percentage":
		return "Percentage Distribution"
	case "range_percentage":
		return "Range-Normalized Distribution"
	default:
		return strings.ReplaceAll(id, "_", " ") + " Distribution"
	}
}

func aggregateDimension(class, statistic string, float bool) DimensionSpec {
	metric := "aggregate_" + class + "_" + statistic
	return DimensionSpec{
		ID: statistic, Name: statistic, Metric: metric, Selector: metric,
		Algorithm: "absolute", Float: float,
	}
}

func aggregateFixedClassChart(
	class, title, units string,
	dimensions []string,
	chartType ChartType,
	chartClass ChartClass,
) ChartSpec {
	context := "redfish.aggregate." + class
	result := ChartSpec{
		Module: "redfish", ID: chartID(context), Context: context,
		Title: title, Units: units, Type: chartType, Class: chartClass,
		TopFamily: "Overview", LeafFamily: "aggregate",
		InstanceLabels: []string{"endpoint_key", "aggregate_key"},
		PromotedLabels: promotedLabels("aggregate"),
		ExpireAfter:    5,
	}
	for _, dimension := range dimensions {
		metric := "aggregate_" + class + "_" + dimension
		result.Dimensions = append(result.Dimensions, DimensionSpec{
			ID: dimension, Name: dimension, Metric: metric, Selector: metric, Algorithm: "absolute",
		})
	}
	return result
}

func aggregateClassPlacement(class string) (string, string) {
	switch class {
	case "temperature", "pressure", "length", "rotational_speed", "air_flow", "liquid_flow",
		"absolute_humidity", "linear_velocity", "linear_acceleration", "rotational_position",
		"rotational_velocity", "rotational_acceleration":
		return "Thermal", "aggregate"
	case "power", "power_budget", "charge", "voltage", "current", "frequency",
		"apparent_power", "reactive_power", "phase_angle", "stored_energy":
		return "Power", "aggregate"
	default:
		return "Overview", "aggregate"
	}
}

func compileCategoricalAggregateCharts(contract Contract, _ map[Kind]KindSpec) ([]ChartSpec, error) {
	type category struct {
		id, title string
		states    []string
	}
	categories := []category{
		{"health", "Health Counts", HealthStates},
		{"health_rollup", "Health Rollup Counts", HealthStates},
		{"resource_state", "Resource State Counts", ResourceStates},
		{"power_state", "Power State Counts", PowerStates},
		{"failure_predicted", "Failure Prediction Counts", FailureStates},
		{"acquisition_state", "Acquisition State Counts", AcquisitionStates},
		{"conditions", "Condition Counts", []string{"ok", "warning", "critical", "unknown"}},
		{"reading_alarm", "Reading Alarm Counts", AlarmStates},
	}
	seen := make(map[string][]string)
	for _, item := range categories {
		seen[item.id] = slices.Clone(item.states)
	}
	for _, state := range contract.States {
		if len(state.AggregateKinds) == 0 {
			continue
		}
		id := aggregateCategoricalClass(state.Metric)
		if existing, ok := seen[id]; ok {
			if !slices.Equal(existing, state.States) {
				return nil, fmt.Errorf("categorical aggregate class %q has incompatible states", id)
			}
			continue
		}
		seen[id] = slices.Clone(state.States)
		categories = append(categories, category{id, state.Title + " Counts", slices.Clone(state.States)})
	}
	for _, flags := range contract.Flags {
		if len(flags.AggregateKinds) == 0 {
			continue
		}
		states := make([]string, 0, len(flags.Members))
		for _, member := range flags.Members {
			states = append(states, member.Role)
		}
		id := aggregateCategoricalClass(flags.Metric)
		if existing, ok := seen[id]; ok {
			if !slices.Equal(existing, states) {
				return nil, fmt.Errorf("categorical aggregate class %q has incompatible states", id)
			}
			continue
		}
		seen[id] = slices.Clone(states)
		categories = append(categories, category{id, flags.Title + " Counts", states})
	}

	result := make([]ChartSpec, 0, len(categories))
	for _, item := range categories {
		context := "redfish.aggregate." + item.id
		metric := "aggregate_" + item.id
		result = append(result, ChartSpec{
			Module: "redfish", ID: chartID(context), Context: context,
			Title: item.title, Units: categoricalAggregateUnits(item.id), Type: ChartStacked,
			Class:     ClassCategoricalParent,
			TopFamily: "Overview", LeafFamily: "aggregate",
			InstanceLabels: []string{"endpoint_key", "aggregate_key"},
			PromotedLabels: promotedLabels("aggregate"),
			Dimensions:     fixedDimensions(metric, item.states),
			ExpireAfter:    5,
		})
	}
	return result, nil
}

func categoricalAggregateUnits(class string) string {
	if class == "conditions" {
		return "conditions"
	}
	return "components"
}

func aggregateCategoricalClass(metric string) string {
	return strings.TrimPrefix(metric, "aggregate_")
}

func fixedDimensions(metric string, states []string) []DimensionSpec {
	result := make([]DimensionSpec, 0, len(states))
	for _, state := range states {
		selector := metric + "_" + state
		result = append(result, DimensionSpec{
			ID: state, Name: state, Metric: selector, Selector: selector, Algorithm: "absolute",
		})
	}
	return result
}
