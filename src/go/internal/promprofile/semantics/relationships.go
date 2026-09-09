// SPDX-License-Identifier: GPL-3.0-or-later

package promsemantics

import (
	"fmt"
	"slices"
	"strings"
)

func (c *semanticCompiler) compileContributorVariants() error {
	for _, signalID := range sortedMapKeys(c.input.Contract.Source.Signals) {
		definition := c.input.Contract.Source.Signals[signalID]
		if definition.Contributors == nil {
			continue
		}
		signal := c.program.signals[signalID]
		covering := make([]compiledEnvironmentCondition, 0, len(definition.Contributors.Variants))
		for _, variantID := range sortedMapKeys(definition.Contributors.Variants) {
			variant := definition.Contributors.Variants[variantID]
			condition, err := c.resolveCondition(
				"signals."+signalID+".contributors.variants."+variantID+".when",
				variant.When,
			)
			if err != nil {
				return err
			}
			availability := signal.availability.and(condition, c.environment.axes)
			if len(availability.clauses) == 0 {
				return fmt.Errorf("signal %q contributor variant %q is unreachable", signalID, variantID)
			}
			if err := c.validateContributorIdentity(signal, variantID, variant, availability); err != nil {
				return err
			}
			if variant.Cardinality.Axis != "" {
				c.axisUses[variant.Cardinality.Axis]++
			}
			signal.contributors = append(signal.contributors, compiledContributorVariant{
				id:           variantID,
				definition:   variant,
				availability: availability,
			})
			covering = append(covering, availability)
		}
		for left := 0; left < len(signal.contributors); left++ {
			for right := left + 1; right < len(signal.contributors); right++ {
				if signal.contributors[left].availability.overlaps(
					signal.contributors[right].availability,
					c.environment.axes,
				) {
					return fmt.Errorf("signal %q contributor variants %q and %q overlap",
						signalID, signal.contributors[left].id, signal.contributors[right].id)
				}
			}
		}
		if !signal.availability.coveredBy(c.environment.axes, covering...) {
			return fmt.Errorf("signal %q contributor variants do not cover every active environment", signalID)
		}
	}
	return nil
}

func (c *semanticCompiler) validateContributorIdentity(
	signal *compiledSignal,
	variantID string,
	variant ContributorVariant,
	availability compiledEnvironmentCondition,
) error {
	for _, occurrenceKey := range signal.occurrences {
		occurrence := c.program.occurrences[occurrenceKey]
		if !occurrence.availability.overlaps(availability, c.environment.axes) {
			continue
		}
		for _, identity := range variant.Identity {
			normalized := normalizedOccurrenceIdentityLabel(occurrence, identity)
			if _, ok := occurrence.labels[normalized]; !ok {
				return fmt.Errorf(
					"signal %q contributor variant %q identity %q is absent from post-normalization occurrence %q",
					occurrence.signal, variantID, identity, occurrenceKey,
				)
			}
		}
	}
	return nil
}

func (c *semanticCompiler) compileRelationships() error {
	seen := make(map[string]string)
	for _, id := range sortedMapKeys(c.input.Contract.Source.Relationships) {
		definition := c.input.Contract.Source.Relationships[id]
		availability, err := c.resolveCondition("relationships."+id+".when", definition.When)
		if err != nil {
			return err
		}
		if err := c.validateCompiledRelationship(id, definition, availability); err != nil {
			return err
		}
		canonical := canonicalRelationship(definition)
		if previous := seen[canonical]; previous != "" {
			return fmt.Errorf("relationships %q and %q duplicate or reverse the same semantic body", previous, id)
		}
		seen[canonical] = id
		c.program.relationships[id] = &compiledRelationship{
			id:           id,
			definition:   definition,
			availability: availability,
		}
	}
	return nil
}

func (c *semanticCompiler) validateCompiledRelationship(
	id string,
	definition Relationship,
	availability compiledEnvironmentCondition,
) error {
	references := relationshipReferences(definition)
	for index, reference := range references {
		signal := c.program.signals[reference.Signal]
		catalog := signal.labels
		if err := validateLabelConditionAgainstCatalog(
			fmt.Sprintf("relationships.%s.references[%d].where", id, index),
			reference.Where,
			catalog,
		); err != nil {
			return err
		}
		if len(signal.availability.and(availability, c.environment.axes).clauses) == 0 {
			return fmt.Errorf("relationship %q can never apply to signal %q", id, reference.Signal)
		}
	}
	if err := relationshipComponentsCompatible(id, definition, c.program.signals); err != nil {
		return err
	}
	switch definition.Kind {
	case "equivalent":
		left := c.input.Contract.Source.Signals[definition.Left.Signal]
		right := c.input.Contract.Source.Signals[definition.Right.Signal]
		if left.Population.ID != right.Population.ID {
			return fmt.Errorf("relationship %q equivalent signals have different populations %q and %q",
				id, left.Population.ID, right.Population.ID)
		}
		if err := c.validateRelationshipGroupLabels(
			id,
			definition.Kind,
			"left",
			"right",
			c.program.signals[definition.Left.Signal],
			c.program.signals[definition.Right.Signal],
			definition.GroupBy,
		); err != nil {
			return err
		}
	case "sum_projection":
		if err := c.validateSumProjectionRelationship(id, definition); err != nil {
			return err
		}
	case "partition":
		seen := make(map[string]struct{}, len(definition.Parts))
		for _, part := range definition.Parts {
			key := canonicalSourceReference(part)
			if _, ok := seen[key]; ok {
				return fmt.Errorf("relationship %q has duplicate partition part", id)
			}
			seen[key] = struct{}{}
		}
	case "overlap":
		seen := make(map[string]struct{}, len(definition.Members))
		for _, member := range definition.Members {
			key := canonicalSourceReference(member)
			if _, ok := seen[key]; ok {
				return fmt.Errorf("relationship %q has duplicate overlap member", id)
			}
			seen[key] = struct{}{}
		}
	}
	return nil
}

func relationshipReferences(definition Relationship) []SourceReference {
	result := make([]SourceReference, 0, 6)
	for _, reference := range []*SourceReference{
		definition.Whole,
		definition.Left,
		definition.Right,
		definition.Subset,
		definition.Superset,
		definition.Coarse,
		definition.Fine,
	} {
		if reference != nil {
			result = append(result, *reference)
		}
	}
	result = append(result, definition.Parts...)
	result = append(result, definition.Members...)
	return result
}

func (c *semanticCompiler) validateSumProjectionRelationship(id string, definition Relationship) error {
	coarse := c.program.signals[definition.Coarse.Signal]
	fine := c.program.signals[definition.Fine.Signal]
	coarsePopulation := c.input.Contract.Source.Signals[definition.Coarse.Signal].Population.ID
	finePopulation := c.input.Contract.Source.Signals[definition.Fine.Signal].Population.ID
	if coarsePopulation != finePopulation {
		return fmt.Errorf(
			"relationship %q sum_projection signals have different populations %q and %q",
			id, coarsePopulation, finePopulation,
		)
	}
	coarseComponent := coarse.components[definition.Coarse.Components[0]]
	if kind := coarseComponent.source.Lifecycle.Kind; kind != "cumulative" && kind != "current" {
		return fmt.Errorf("relationship %q sum_projection coarse component must be current or cumulative", id)
	}
	if err := c.validateRelationshipGroupLabels(
		id, definition.Kind, "coarse", "fine", coarse, fine, definition.GroupBy,
	); err != nil {
		return err
	}
	groupLabels := make(map[string]struct{}, len(definition.GroupBy))
	for _, label := range definition.GroupBy {
		groupLabels[label] = struct{}{}
	}
	for label := range coarse.labels {
		if _, ok := groupLabels[label]; !ok {
			return fmt.Errorf("relationship %q sum_projection coarse signal has non-group label %q", id, label)
		}
	}
	return nil
}

func (c *semanticCompiler) validateRelationshipGroupLabels(
	id string,
	kind string,
	leftRole string,
	rightRole string,
	left *compiledSignal,
	right *compiledSignal,
	groupBy []string,
) error {
	for _, label := range groupBy {
		leftLabel, leftOK := left.labels[label]
		rightLabel, rightOK := right.labels[label]
		if !leftOK || !rightOK {
			return fmt.Errorf("relationship %q %s group label %q must exist on %s and %s signals",
				id, kind, label, leftRole, rightRole)
		}
		if err := c.validateRelationshipGroupLabelPresence(
			id, kind, label, leftRole, rightRole, leftLabel, rightLabel,
		); err != nil {
			return err
		}
		if !slices.Equal(leftLabel.Domain.Values, rightLabel.Domain.Values) {
			return fmt.Errorf("relationship %q %s group label %q has incompatible domains", id, kind, label)
		}
		if _, err := mergeLabelSchemas(label, leftLabel, rightLabel); err != nil {
			return fmt.Errorf("relationship %q %s: %w", id, kind, err)
		}
	}
	return nil
}

func (c *semanticCompiler) validateRelationshipGroupLabelPresence(
	id string,
	kind string,
	label string,
	leftRole string,
	rightRole string,
	left SourceLabel,
	right SourceLabel,
) error {
	if left.Presence.Kind != right.Presence.Kind {
		return fmt.Errorf(
			"relationship %q %s group label %q has incompatible presence on %s and %s signals",
			id, kind, label, leftRole, rightRole,
		)
	}
	if left.Presence.Kind != "" {
		return nil
	}
	leftCondition, err := c.environment.resolve(left.Presence.When)
	if err != nil {
		return fmt.Errorf("relationship %q %s %s group label %q: %w", id, kind, leftRole, label, err)
	}
	rightCondition, err := c.environment.resolve(right.Presence.When)
	if err != nil {
		return fmt.Errorf("relationship %q %s %s group label %q: %w", id, kind, rightRole, label, err)
	}
	if !leftCondition.coveredBy(c.environment.axes, rightCondition) ||
		!rightCondition.coveredBy(c.environment.axes, leftCondition) {
		return fmt.Errorf(
			"relationship %q %s group label %q has incompatible conditional presence on %s and %s signals",
			id, kind, label, leftRole, rightRole,
		)
	}
	return nil
}

func relationshipComponentsCompatible(
	id string,
	definition Relationship,
	signals map[string]*compiledSignal,
) error {
	references := relationshipReferences(definition)
	var expected *compiledComponent
	for _, reference := range references {
		for _, componentID := range reference.Components {
			component := signals[reference.Signal].components[componentID]
			if expected == nil {
				copy := component
				expected = &copy
				continue
			}
			if !sameRelationshipAlgebra(*expected, component) {
				return fmt.Errorf("relationship %q mixes incompatible component algebra", id)
			}
		}
	}
	return nil
}

func sameRelationshipAlgebra(left, right compiledComponent) bool {
	return left.source.Lifecycle.Kind == right.source.Lifecycle.Kind &&
		left.source.Unit.Quantity == right.source.Unit.Quantity &&
		left.source.Unit.Base == right.source.Unit.Base &&
		left.source.Unit.Rate == right.source.Unit.Rate &&
		left.source.Unit.Object == right.source.Unit.Object &&
		left.source.Unit.Aspect == right.source.Unit.Aspect
}

func canonicalRelationship(definition Relationship) string {
	canonical := func(reference *SourceReference) string {
		if reference == nil {
			return ""
		}
		return canonicalSourceReference(*reference)
	}
	switch definition.Kind {
	case "equivalent", "subset":
		members := []string{canonical(definition.Left), canonical(definition.Right)}
		if definition.Kind == "subset" {
			members = []string{canonical(definition.Subset), canonical(definition.Superset)}
		}
		slices.Sort(members)
		result := definition.Kind + ":" + strings.Join(members, "|")
		if definition.Kind == "equivalent" && len(definition.GroupBy) != 0 {
			groupBy := slices.Clone(definition.GroupBy)
			slices.Sort(groupBy)
			result += ":" + strings.Join(groupBy, ",")
		}
		return result
	case "sum_projection":
		groupBy := slices.Clone(definition.GroupBy)
		slices.Sort(groupBy)
		return definition.Kind + ":" + canonical(definition.Coarse) + ":" +
			canonical(definition.Fine) + ":" + strings.Join(groupBy, ",")
	case "overlap":
		members := make([]string, 0, len(definition.Members))
		for _, member := range definition.Members {
			members = append(members, canonicalSourceReference(member))
		}
		slices.Sort(members)
		return definition.Kind + ":" + strings.Join(members, "|")
	case "partition":
		parts := make([]string, 0, len(definition.Parts))
		for _, part := range definition.Parts {
			parts = append(parts, canonicalSourceReference(part))
		}
		slices.Sort(parts)
		return definition.Kind + ":" + canonical(definition.Whole) + ":" + strings.Join(parts, "|")
	default:
		panic("validated relationship kind has no canonical form")
	}
}

func canonicalSourceReference(reference SourceReference) string {
	components := slices.Clone(reference.Components)
	slices.Sort(components)
	return reference.Signal + "#" + strings.Join(components, ",") + "#" + canonicalLabelCondition(reference.Where)
}

func canonicalLabelCondition(condition *LabelCondition) string {
	if condition == nil {
		return ""
	}
	clauses := make([]string, 0, len(condition.Any))
	for _, clause := range condition.Any {
		predicates := make([]string, 0, len(clause.All))
		for _, predicate := range clause.All {
			values := slices.Clone(predicate.Values)
			slices.Sort(values)
			value := ""
			if predicate.Value != nil {
				value = *predicate.Value
			}
			predicates = append(predicates,
				predicate.Label+":"+predicate.Op+":"+value+":"+strings.Join(values, ","))
		}
		slices.Sort(predicates)
		clauses = append(clauses, strings.Join(predicates, "&"))
	}
	slices.Sort(clauses)
	return strings.Join(clauses, "|")
}
