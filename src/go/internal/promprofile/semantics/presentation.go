// SPDX-License-Identifier: GPL-3.0-or-later

package promsemantics

import (
	"fmt"
	"slices"
	"strings"
)

func (c *semanticCompiler) validateViewPresentation(contextID string, view *compiledView) error {
	intent := view.definition.Presentation
	if intent == nil {
		return nil
	}
	if viewHasOnlyHistogramBuckets(view) {
		return fmt.Errorf("view %q cannot override the derived histogram heatmap presentation", contextID)
	}
	if intent.Type == "area" {
		return nil
	}
	relationship := c.program.relationships[intent.Relationship]
	if relationship == nil {
		return fmt.Errorf("view %q stacked presentation references unknown relationship %q",
			contextID, intent.Relationship)
	}
	definition := relationship.definition
	if definition.Kind != "partition" || definition.Disjoint == nil || !*definition.Disjoint ||
		definition.Exhaustive == nil || !*definition.Exhaustive {
		return fmt.Errorf("view %q stacked presentation relationship %q must be a disjoint exhaustive partition",
			contextID, intent.Relationship)
	}
	for _, input := range view.inputs {
		for _, source := range input.occurrences {
			if source.program != c.program || !source.occurrence.availability.coveredBy(
				c.environment.axes,
				relationship.availability,
			) {
				return fmt.Errorf("view %q stacked presentation relationship %q does not cover every input occurrence",
					contextID, intent.Relationship)
			}
		}
	}
	if stackedInputPartitionMatches(view, definition) || stackedLabelPartitionMatches(view, definition) {
		return nil
	}
	return fmt.Errorf("view %q stacked presentation relationship %q does not exactly partition its inputs or closed label dimensions",
		contextID, intent.Relationship)
}

func viewHasOnlyHistogramBuckets(view *compiledView) bool {
	total := 0
	buckets := 0
	for _, input := range view.inputs {
		for _, source := range input.occurrences {
			total++
			if source.component.source.WireRole == "histogram_bucket" {
				buckets++
			}
		}
	}
	return total > 0 && total == buckets
}

func stackedInputPartitionMatches(view *compiledView, relationship Relationship) bool {
	want, ok := atomicViewInputReferences(view, true)
	if !ok {
		return false
	}
	got := atomicSourceReferences(relationship.Parts)
	return equalStringSets(want, got)
}

func stackedLabelPartitionMatches(view *compiledView, relationship Relationship) bool {
	dimensionLabels := make([]string, 0, len(view.labels.Dimensions))
	for label, rendering := range view.labels.Dimensions {
		if rendering.Render != "label_value" {
			return false
		}
		dimensionLabels = append(dimensionLabels, label)
	}
	if len(dimensionLabels) == 0 {
		return false
	}
	slices.Sort(dimensionLabels)

	base, ok := atomicViewInputReferences(view, false)
	if !ok || !equalStringSets(base, atomicSourceReference(*relationship.Whole)) {
		return false
	}
	catalog, ok := viewLabelPartitionCatalog(view)
	if !ok {
		return false
	}
	domainProduct := 1
	for _, label := range dimensionLabels {
		schema, exists := catalog[label]
		if !exists || schema.Presence.Kind != "required" || schema.Domain.Kind != "closed" ||
			len(schema.Domain.Values) == 0 {
			return false
		}
		if domainProduct > int(^uint(0)>>1)/len(schema.Domain.Values) {
			return false
		}
		domainProduct *= len(schema.Domain.Values)
	}

	seen := make(map[string]struct{})
	for _, part := range relationship.Parts {
		values, ok := exactDimensionValues(part.Where, dimensionLabels, catalog)
		if !ok {
			return false
		}
		for atom := range atomicSourceReferenceWithoutCondition(part) {
			if _, exists := base[atom]; !exists {
				return false
			}
			key := atom + "#" + strings.Join(values, "\x00")
			if _, duplicate := seen[key]; duplicate {
				return false
			}
			seen[key] = struct{}{}
		}
	}
	return len(seen) == len(base)*domainProduct
}

func atomicViewInputReferences(view *compiledView, includeWhere bool) (map[string]struct{}, bool) {
	result := make(map[string]struct{})
	for _, input := range view.inputs {
		if strings.Contains(input.definition.Signal, "/") || (!includeWhere && input.definition.Where != nil) {
			return nil, false
		}
		reference := SourceReference{
			Signal:     input.definition.Signal,
			Components: input.definition.Components,
		}
		if includeWhere {
			reference.Where = input.definition.Where
		}
		for atom := range atomicSourceReference(reference) {
			if _, duplicate := result[atom]; duplicate {
				return nil, false
			}
			result[atom] = struct{}{}
		}
	}
	return result, true
}

func atomicSourceReferences(references []SourceReference) map[string]struct{} {
	result := make(map[string]struct{})
	for _, reference := range references {
		for atom := range atomicSourceReference(reference) {
			result[atom] = struct{}{}
		}
	}
	return result
}

func atomicSourceReference(reference SourceReference) map[string]struct{} {
	result := make(map[string]struct{}, len(reference.Components))
	condition := canonicalLabelCondition(reference.Where)
	for _, component := range reference.Components {
		result[reference.Signal+"#"+component+"#"+condition] = struct{}{}
	}
	return result
}

func atomicSourceReferenceWithoutCondition(reference SourceReference) map[string]struct{} {
	reference.Where = nil
	return atomicSourceReference(reference)
}

func equalStringSets(left, right map[string]struct{}) bool {
	if len(left) != len(right) {
		return false
	}
	for value := range left {
		if _, ok := right[value]; !ok {
			return false
		}
	}
	return true
}

func viewLabelPartitionCatalog(view *compiledView) (map[string]SourceLabel, bool) {
	occurrences := make([]*compiledOccurrence, 0)
	for _, input := range view.inputs {
		for _, source := range input.occurrences {
			occurrences = append(occurrences, source.occurrence)
		}
	}
	catalog, err := mergeOccurrenceLabelCatalog(occurrences)
	return catalog, err == nil
}

func exactDimensionValues(
	condition *LabelCondition,
	dimensionLabels []string,
	catalog map[string]SourceLabel,
) ([]string, bool) {
	if condition == nil || len(condition.Any) != 1 {
		return nil, false
	}
	clause := condition.Any[0]
	if len(clause.All) != len(dimensionLabels) {
		return nil, false
	}
	values := make(map[string]string, len(dimensionLabels))
	for _, predicate := range clause.All {
		value := ""
		switch predicate.Op {
		case "eq":
			value = *predicate.Value
		case "in":
			if len(predicate.Values) != 1 {
				return nil, false
			}
			value = predicate.Values[0]
		default:
			return nil, false
		}
		if _, expected := catalog[predicate.Label]; !expected || !slices.Contains(dimensionLabels, predicate.Label) ||
			values[predicate.Label] != "" || !slices.Contains(catalog[predicate.Label].Domain.Values, value) {
			return nil, false
		}
		values[predicate.Label] = value
	}
	ordered := make([]string, 0, len(dimensionLabels))
	for _, label := range dimensionLabels {
		value, ok := values[label]
		if !ok {
			return nil, false
		}
		ordered = append(ordered, value)
	}
	return ordered, true
}
