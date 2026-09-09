// SPDX-License-Identifier: GPL-3.0-or-later

package promsemantics

import (
	"fmt"
	"slices"
)

type compiledDesignExclusion struct {
	id           string
	definition   DesignExclusion
	availability compiledEnvironmentCondition
	occurrences  []compiledExclusionOccurrence
}

type compiledExclusionOccurrence struct {
	occurrence   *compiledOccurrence
	availability compiledEnvironmentCondition
}

func (c *semanticCompiler) compileDesignExclusions() error {
	for _, id := range sortedMapKeys(c.input.Contract.Design.Exclusions) {
		definition := c.input.Contract.Design.Exclusions[id]
		availability, err := c.resolveCondition("exclusions."+id+".when", definition.When)
		if err != nil {
			return err
		}
		compiled, err := c.compileDesignExclusion(id, definition, availability)
		if err != nil {
			return err
		}
		if err := c.validateDesignExclusionMeaning(compiled); err != nil {
			return err
		}
		if err := c.validateDesignExclusionDisjoint(compiled); err != nil {
			return err
		}
		c.program.exclusions[id] = compiled
	}
	return nil
}

func (c *semanticCompiler) compileDesignExclusion(
	id string,
	definition DesignExclusion,
	availability compiledEnvironmentCondition,
) (*compiledDesignExclusion, error) {
	signal := c.program.signals[definition.Source.Signal]
	if signal == nil {
		return nil, fmt.Errorf("exclusion %q references unknown signal %q", id, definition.Source.Signal)
	}
	components := make(map[string]struct{}, len(definition.Source.Components))
	for _, component := range definition.Source.Components {
		if _, ok := signal.components[component]; !ok {
			return nil, fmt.Errorf("exclusion %q references unknown component %q on signal %q",
				id, component, definition.Source.Signal)
		}
		components[component] = struct{}{}
	}
	candidates := make([]*compiledOccurrence, 0, len(signal.occurrences))
	for _, occurrenceKey := range signal.occurrences {
		occurrence := c.program.occurrences[occurrenceKey]
		if _, ok := components[occurrence.component]; ok && occurrence.terminalExclusion == "" {
			registration := c.program.registrations[occurrence.registration]
			if registration.prometheus.Shape == "info" {
				return nil, fmt.Errorf(
					"exclusion %q references writer-ineligible info occurrence %q; writer eligibility is reconciled directly",
					id, occurrenceKey)
			}
			candidates = append(candidates, occurrence)
		}
	}
	catalog, err := mergeOccurrenceLabelCatalog(candidates)
	if err != nil {
		return nil, fmt.Errorf("exclusion %q: %w", id, err)
	}
	if err := validateLabelConditionAgainstCatalog(
		"exclusions."+id+".source.where",
		definition.Source.Where,
		catalog,
	); err != nil {
		return nil, err
	}
	compiled := &compiledDesignExclusion{
		id:           id,
		definition:   definition,
		availability: availability,
	}
	for _, occurrence := range candidates {
		if !labelConditionMayMatch(definition.Source.Where, occurrence.labels) {
			continue
		}
		active := occurrence.availability.and(availability, c.environment.axes)
		if len(active.clauses) != 0 {
			compiled.occurrences = append(compiled.occurrences, compiledExclusionOccurrence{
				occurrence:   occurrence,
				availability: active,
			})
		}
	}
	if len(compiled.occurrences) == 0 {
		return nil, fmt.Errorf("exclusion %q has no active source occurrence", id)
	}
	return compiled, nil
}

func (c *semanticCompiler) validateDesignExclusionMeaning(exclusion *compiledDesignExclusion) error {
	definition := exclusion.definition
	switch definition.Reason {
	case "equivalent_duplicate":
		covering := c.program.views[definition.CoveringView]
		if covering == nil {
			return fmt.Errorf("exclusion %q references unknown covering view %q",
				exclusion.id, definition.CoveringView)
		}
		if !c.equivalentRelationshipCoversExclusion(exclusion, covering) {
			return fmt.Errorf("exclusion %q has no source-equivalent relationship to covering view %q",
				exclusion.id, definition.CoveringView)
		}
	case "source_superseded":
		if !validID(definition.Replacement) {
			return fmt.Errorf("exclusion %q replacement %q is not a signal ID", exclusion.id, definition.Replacement)
		}
		if definition.Replacement == definition.Source.Signal || c.program.signals[definition.Replacement] == nil {
			return fmt.Errorf("exclusion %q references invalid replacement signal %q",
				exclusion.id, definition.Replacement)
		}
		replacement := c.program.signals[definition.Replacement]
		covering := make([]compiledEnvironmentCondition, 0, len(replacement.occurrences))
		for _, occurrenceKey := range replacement.occurrences {
			covering = append(covering, c.program.occurrences[occurrenceKey].availability)
		}
		for _, source := range exclusion.occurrences {
			if !source.availability.coveredBy(c.environment.axes, covering...) {
				return fmt.Errorf("exclusion %q replacement signal %q is unavailable in part of the excluded source environment",
					exclusion.id, definition.Replacement)
			}
		}
	case "not_chartable":
		for _, source := range exclusion.occurrences {
			component := c.program.signals[source.occurrence.signal].components[source.occurrence.component].source
			if component.Lifecycle.Kind != "current" || component.Unit.Quantity != "timestamp" ||
				component.Unit.Base != "unix_second" || component.Unit.Rate != "none" {
				return fmt.Errorf("exclusion %q age_from_unix_epoch source %s/%s is not a current Unix timestamp",
					exclusion.id, source.occurrence.signal, source.occurrence.component)
			}
		}
	case "metadata_only":
		for _, source := range exclusion.occurrences {
			registration := c.program.registrations[source.occurrence.registration]
			if registration.prometheus.Shape != "scalar" {
				return fmt.Errorf("exclusion %q source %s/%s is not a scalar",
					exclusion.id, source.occurrence.signal, source.occurrence.component)
			}
			component := c.program.signals[source.occurrence.signal].components[source.occurrence.component].source
			if component.Lifecycle.Kind != "constant" || component.Unit.Quantity != "count" ||
				component.Unit.Base != "one" || component.Unit.Rate != "none" {
				return fmt.Errorf("exclusion %q source %s/%s is not constant unit-one metadata",
					exclusion.id, source.occurrence.signal, source.occurrence.component)
			}
			if len(source.occurrence.sourceLabels) == 0 {
				return fmt.Errorf("exclusion %q source %s/%s has no metadata labels",
					exclusion.id, source.occurrence.signal, source.occurrence.component)
			}
		}
	}
	return nil
}

func (c *semanticCompiler) equivalentRelationshipCoversExclusion(
	exclusion *compiledDesignExclusion,
	covering *compiledView,
) bool {
	excluded := atomicSourceReference(exclusion.definition.Source)

relationshipLoop:
	for _, relationship := range c.program.relationships {
		definition := relationship.definition
		var counterparts []SourceReference
		switch definition.Kind {
		case "equivalent":
			left := atomicSourceReference(*definition.Left)
			right := atomicSourceReference(*definition.Right)
			if equalStringSets(excluded, left) {
				counterparts = []SourceReference{*definition.Right}
			} else if equalStringSets(excluded, right) {
				counterparts = []SourceReference{*definition.Left}
			}
		case "sum_projection":
			coarse := atomicSourceReference(*definition.Coarse)
			if equalStringSets(excluded, coarse) {
				counterparts = []SourceReference{*definition.Fine}
			}
		case "partition":
			if definition.Disjoint != nil && *definition.Disjoint &&
				definition.Exhaustive != nil && *definition.Exhaustive &&
				equalStringSets(excluded, atomicSourceReference(*definition.Whole)) {
				counterparts = definition.Parts
			}
		}
		if len(counterparts) == 0 {
			continue
		}
		for _, source := range exclusion.occurrences {
			if !source.availability.coveredBy(c.environment.axes, relationship.availability) {
				continue relationshipLoop
			}
		}
		for _, counterpart := range counterparts {
			if !viewCoversSourceReference(covering, counterpart) ||
				!c.coveringViewAvailabilityCoversExclusion(exclusion, covering, counterpart) {
				continue relationshipLoop
			}
		}
		return true
	}
	return false
}

func (c *semanticCompiler) coveringViewAvailabilityCoversExclusion(
	exclusion *compiledDesignExclusion,
	view *compiledView,
	reference SourceReference,
) bool {
	for _, component := range reference.Components {
		inputs, ok := coveringInputsForSourceReference(view, reference, component)
		if !ok {
			return false
		}
		covering := make([]compiledEnvironmentCondition, 0)
		for _, input := range inputs {
			for _, source := range input.occurrences {
				if source.program == c.program && source.occurrence.signal == reference.Signal &&
					source.occurrence.component == component {
					covering = append(covering, source.occurrence.availability)
				}
			}
		}
		for _, source := range exclusion.occurrences {
			if !source.availability.coveredBy(c.environment.axes, covering...) {
				return false
			}
		}
	}
	return true
}

func viewCoversSourceReference(view *compiledView, reference SourceReference) bool {
	for _, component := range reference.Components {
		if _, ok := coveringInputsForSourceReference(view, reference, component); !ok {
			return false
		}
	}
	return true
}

func coveringInputsForSourceReference(
	view *compiledView,
	reference SourceReference,
	component string,
) ([]*compiledViewInput, bool) {
	inputs := make([]*compiledViewInput, 0)
	exact := make([]*compiledViewInput, 0)
	unconditional := make([]*compiledViewInput, 0)
	for _, input := range view.inputs {
		if input.definition.Signal != reference.Signal || !slices.Contains(input.definition.Components, component) {
			continue
		}
		inputs = append(inputs, input)
		if input.definition.Where == nil {
			unconditional = append(unconditional, input)
		}
		if canonicalLabelCondition(input.definition.Where) == canonicalLabelCondition(reference.Where) {
			exact = append(exact, input)
		}
	}
	if len(exact) != 0 {
		return exact, true
	}
	if len(unconditional) != 0 {
		return unconditional, true
	}
	if reference.Where != nil || len(inputs) == 0 {
		return nil, false
	}

	label := ""
	values := make(map[string]struct{})
	for _, input := range inputs {
		condition := input.definition.Where
		if condition == nil || len(condition.Any) != 1 || len(condition.Any[0].All) != 1 {
			return nil, false
		}
		predicate := condition.Any[0].All[0]
		if predicate.Op != "eq" && predicate.Op != "in" {
			return nil, false
		}
		if label == "" {
			label = predicate.Label
		} else if label != predicate.Label {
			return nil, false
		}
		predicateValues := predicate.Values
		if predicate.Op == "eq" {
			predicateValues = []string{*predicate.Value}
		}
		for _, value := range predicateValues {
			if _, duplicate := values[value]; duplicate {
				return nil, false
			}
			values[value] = struct{}{}
		}
	}

	var schema SourceLabel
	found := false
	for _, input := range inputs {
		for _, source := range input.occurrences {
			candidate, ok := source.occurrence.labels[label]
			if !ok {
				return nil, false
			}
			if !found {
				schema = candidate
				found = true
			} else {
				merged, err := mergeLabelSchemas(label, schema, candidate)
				if err != nil {
					return nil, false
				}
				schema = merged
			}
		}
	}
	if !found || schema.Presence.Kind != "required" || schema.Domain.Kind != "closed" ||
		len(values) != len(schema.Domain.Values) {
		return nil, false
	}
	for _, value := range schema.Domain.Values {
		if _, ok := values[value]; !ok {
			return nil, false
		}
	}
	return inputs, true
}

func (c *semanticCompiler) validateDesignExclusionDisjoint(exclusion *compiledDesignExclusion) error {
	for _, previousID := range sortedMapKeys(c.program.exclusions) {
		previous := c.program.exclusions[previousID]
		for _, left := range previous.occurrences {
			for _, right := range exclusion.occurrences {
				if left.occurrence.key != right.occurrence.key ||
					!left.availability.overlaps(right.availability, c.environment.axes) ||
					!labelConditionsMaySelectSameSeries(
						previous.definition.Source.Where,
						exclusion.definition.Source.Where,
						left.occurrence.labels,
					) {
					continue
				}
				return fmt.Errorf("exclusions %q and %q overlap on occurrence %q",
					previous.id, exclusion.id, left.occurrence.key)
			}
		}
	}
	for _, viewID := range sortedMapKeys(c.program.views) {
		view := c.program.views[viewID]
		for _, inputID := range sortedMapKeys(view.inputs) {
			input := view.inputs[inputID]
			for _, rendered := range input.occurrences {
				for _, excluded := range exclusion.occurrences {
					if rendered.program != c.program || rendered.occurrence.key != excluded.occurrence.key ||
						!rendered.occurrence.availability.overlaps(excluded.availability, c.environment.axes) ||
						!labelConditionsMaySelectSameSeries(
							input.definition.Where,
							exclusion.definition.Source.Where,
							excluded.occurrence.labels,
						) {
						continue
					}
					return fmt.Errorf("exclusion %q overlaps view %q input %q on occurrence %q",
						exclusion.id, viewID, inputID, excluded.occurrence.key)
				}
			}
		}
	}
	return nil
}

func labelConditionsMaySelectSameSeries(
	left *LabelCondition,
	right *LabelCondition,
	labels map[string]SourceLabel,
) bool {
	leftClauses := labelConditionClauses(left)
	rightClauses := labelConditionClauses(right)
	for _, leftClause := range leftClauses {
		for _, rightClause := range rightClauses {
			combined := append(slices.Clone(leftClause.All), rightClause.All...)
			if labelClauseMayMatch(combined, labels) {
				return true
			}
		}
	}
	return false
}

func labelConditionClauses(condition *LabelCondition) []LabelClause {
	if condition == nil {
		return []LabelClause{{}}
	}
	return condition.Any
}

func labelClauseMayMatch(predicates []LabelPredicate, labels map[string]SourceLabel) bool {
	byLabel := make(map[string][]LabelPredicate)
	for _, predicate := range predicates {
		byLabel[predicate.Label] = append(byLabel[predicate.Label], predicate)
	}
	for labelName, constraints := range byLabel {
		label, exists := labels[labelName]
		if !exists {
			if slices.ContainsFunc(constraints, func(predicate LabelPredicate) bool { return predicate.Op != "absent" }) {
				return false
			}
			continue
		}
		absentAllowed := label.Presence.keyMayBeAbsent()
		presentAllowed := true
		var allowed map[string]struct{}
		if label.Domain.Kind == "closed" {
			allowed = make(map[string]struct{}, len(label.Domain.Values))
			for _, value := range label.Domain.Values {
				allowed[value] = struct{}{}
			}
		}
		for _, predicate := range constraints {
			switch predicate.Op {
			case "absent":
				presentAllowed = false
			case "present", "nonblank":
				absentAllowed = false
			case "eq":
				absentAllowed = false
				allowed = intersectLabelValues(allowed, []string{*predicate.Value})
			case "in":
				absentAllowed = false
				allowed = intersectLabelValues(allowed, predicate.Values)
			}
		}
		if allowed != nil && len(allowed) == 0 {
			presentAllowed = false
		}
		if !absentAllowed && !presentAllowed {
			return false
		}
	}
	return true
}

func intersectLabelValues(current map[string]struct{}, values []string) map[string]struct{} {
	next := make(map[string]struct{}, len(values))
	for _, value := range values {
		if current == nil {
			next[value] = struct{}{}
		} else if _, ok := current[value]; ok {
			next[value] = struct{}{}
		}
	}
	return next
}
