// SPDX-License-Identifier: GPL-3.0-or-later

package promsemantics

import (
	"fmt"
	"slices"
)

type fanInCause struct {
	input  *compiledViewInput
	source compiledViewOccurrence
	label  string
}

func validateViewReduction(contextID string, view *compiledView) error {
	causes := viewFanInCauses(view)
	if len(causes) == 0 {
		if view.reduction != nil {
			return fmt.Errorf("view %q reduction is unnecessary", contextID)
		}
		return nil
	}
	if view.reduction == nil {
		return fmt.Errorf("view %q requires reduction because its projection can combine distinct observations", contextID)
	}
	for _, cause := range causes {
		if !reducerAuthorized(cause, view.reduction.Reducer) {
			return fmt.Errorf("view %q reducer %q is not source-authorized for %s/%s label %q",
				contextID,
				view.reduction.Reducer,
				cause.source.occurrence.signal,
				cause.source.occurrence.component,
				cause.label,
			)
		}
	}
	return nil
}

func viewFanInCauses(view *compiledView) []fanInCause {
	causes := make(map[string]fanInCause)
	add := func(input *compiledViewInput, source compiledViewOccurrence, label string) {
		key := input.id + "#" + source.sourceProfile + "#" + source.occurrence.key + "#" + label
		causes[key] = fanInCause{input: input, source: source, label: label}
	}
	for _, inputID := range sortedMapKeys(view.inputs) {
		input := view.inputs[inputID]
		for _, source := range input.occurrences {
			for label := range view.labels.Omit {
				if labelCanVaryWithinProjection(source, input, label, view.entity) {
					add(input, source, label)
				}
			}
			for label, rendering := range view.labels.Dimensions {
				if rendering.Render == "input_role" && labelCanVaryWithinProjection(source, input, label, view.entity) {
					add(input, source, label)
				}
			}
		}
		components := make(map[string]compiledViewOccurrence)
		for _, source := range input.occurrences {
			components[source.occurrence.component] = source
		}
		if len(components) > 1 {
			for _, component := range sortedMapKeys(components) {
				add(input, components[component], "")
			}
		}
	}
	inputIDs := sortedMapKeys(view.inputs)
	for left := range inputIDs {
		leftInput := view.inputs[inputIDs[left]]
		for right := left + 1; right < len(inputIDs); right++ {
			rightInput := view.inputs[inputIDs[right]]
			if !inputRoleDomainsMayOverlap(leftInput, rightInput, view.labels) {
				continue
			}
			for _, leftSource := range leftInput.occurrences {
				for _, rightSource := range rightInput.occurrences {
					if !viewSourcesMayCoexist(leftSource, rightSource, view) {
						continue
					}
					add(leftInput, leftSource, "")
					add(rightInput, rightSource, "")
				}
			}
		}
	}
	result := make([]fanInCause, 0, len(causes))
	for _, key := range sortedMapKeys(causes) {
		result = append(result, causes[key])
	}
	return result
}

func labelCanVaryWithinProjection(
	source compiledViewOccurrence,
	input *compiledViewInput,
	label string,
	entity EntityDefinition,
) bool {
	schema, ok := source.occurrence.labels[label]
	if !ok || labelDeterminedByEntity(source, label, entity) || conditionFixesOneValue(input.definition.Where, label) {
		return false
	}
	if schema.EndpointCardinality.Kind == "singleton" {
		return false
	}
	return schema.Domain.Kind != "closed" || len(schema.Domain.Values) > 1
}

func conditionFixesOneValue(condition *LabelCondition, label string) bool {
	_, ok := conditionFixedValue(condition, label)
	return ok
}

func conditionFixedValue(condition *LabelCondition, label string) (string, bool) {
	if condition == nil {
		return "", false
	}
	fixed := ""
	for _, clause := range condition.Any {
		clauseValue := ""
		for _, predicate := range clause.All {
			if predicate.Label != label {
				continue
			}
			switch predicate.Op {
			case "eq":
				clauseValue = *predicate.Value
			case "in":
				if len(predicate.Values) == 1 {
					clauseValue = predicate.Values[0]
				}
			}
		}
		if clauseValue == "" {
			return "", false
		}
		if fixed == "" {
			fixed = clauseValue
		} else if fixed != clauseValue {
			return "", false
		}
	}
	return fixed, fixed != ""
}

func inputRoleDomainsMayOverlap(left, right *compiledViewInput, labels ViewLabels) bool {
	leftValues, leftOpen := inputRoleDomain(left, labels)
	rightValues, rightOpen := inputRoleDomain(right, labels)
	if leftOpen || rightOpen {
		return true
	}
	for value := range leftValues {
		if _, ok := rightValues[value]; ok {
			return true
		}
	}
	return false
}

func inputRoleDomain(input *compiledViewInput, labels ViewLabels) (map[string]struct{}, bool) {
	values := make(map[string]struct{})
	dynamic := false
	for label, rendering := range labels.Dimensions {
		if rendering.Render != "label_value" {
			continue
		}
		for _, source := range input.occurrences {
			schema, ok := source.occurrence.labels[label]
			if !ok {
				continue
			}
			dynamic = true
			if value, fixed := conditionFixedValue(input.definition.Where, label); fixed {
				values[value] = struct{}{}
				continue
			}
			if schema.Domain.Kind != "closed" {
				return nil, true
			}
			for _, value := range schema.Domain.Values {
				values[value] = struct{}{}
			}
		}
	}
	if !dynamic {
		values[input.renderedRole] = struct{}{}
	}
	return values, false
}

func viewSourcesMayCoexist(left, right compiledViewOccurrence, view *compiledView) bool {
	if !left.destinationAvailability.overlaps(right.destinationAvailability, view.destinationAxes) {
		return false
	}
	if left.program != right.program {
		return true
	}
	return left.occurrence.availability.overlaps(
		right.occurrence.availability,
		left.program.environment.axes,
	)
}

func reducerAuthorized(cause fanInCause, reducer string) bool {
	if contributorReducerAuthorized(cause, reducer) {
		return true
	}
	return reducer == "sum" && partitionReducerAuthorized(cause)
}

func contributorReducerAuthorized(cause fanInCause, reducer string) bool {
	signal := cause.source.program.signals[cause.source.occurrence.signal]
	if len(signal.contributors) == 0 || cause.label == "" {
		return false
	}
	authorized := false
	for _, variant := range signal.contributors {
		if !variant.availability.overlaps(
			cause.source.occurrence.availability,
			cause.source.program.environment.axes,
		) || variant.definition.Concurrency != "may_coexist" {
			continue
		}
		if !labelDeterminedByIdentity(cause.source, cause.label, variant.definition.Identity) {
			return false
		}
		model := variant.definition.ValueModel[cause.source.occurrence.component]
		if !reducerMatchesValueModel(reducer, model, cause.source.component.source.Lifecycle.Kind) {
			return false
		}
		authorized = true
	}
	return authorized
}

func reducerMatchesValueModel(reducer, model, lifecycle string) bool {
	switch reducer {
	case "sum":
		return model == "additive"
	case "avg":
		return lifecycle == "current" && model == "comparable_point"
	case "min", "max":
		return lifecycle == "current" && (model == "comparable_point" || model == "ordered_state")
	default:
		return false
	}
}

func partitionReducerAuthorized(cause fanInCause) bool {
	for _, relationship := range cause.source.program.relationships {
		definition := relationship.definition
		if definition.Kind != "partition" || definition.Disjoint == nil || !*definition.Disjoint ||
			definition.Exhaustive == nil || !*definition.Exhaustive ||
			!cause.source.occurrence.availability.coveredBy(
				cause.source.program.environment.axes,
				relationship.availability,
			) {
			continue
		}
		for _, part := range definition.Parts {
			if !sourceReferenceContains(part, cause.source.occurrence.signal, cause.source.occurrence.component) {
				continue
			}
			if cause.label == "" {
				if canonicalLabelCondition(part.Where) == canonicalLabelCondition(cause.input.definition.Where) {
					return true
				}
				continue
			}
			if conditionPartitionsLabel(part.Where, cause.label) {
				return true
			}
		}
	}
	return false
}

func sourceReferenceContains(reference SourceReference, signal, component string) bool {
	return reference.Signal == signal && slices.Contains(reference.Components, component)
}
