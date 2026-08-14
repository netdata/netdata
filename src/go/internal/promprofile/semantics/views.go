// SPDX-License-Identifier: GPL-3.0-or-later

package promsemantics

import (
	"fmt"
	"slices"
	"strings"
)

func (c *semanticCompiler) compileViews() error {
	entityUses := make(map[string]int, len(c.input.Contract.Design.Entities))
	entityRisk := make(map[string]bool, len(c.input.Contract.Design.Entities))
	entityLabelsSeen := make(map[string]map[string]struct{}, len(c.input.Contract.Design.Entities))
	for _, contextID := range sortedMapKeys(c.input.Contract.Design.Views) {
		definition := c.input.Contract.Design.Views[contextID]
		entity := c.input.Contract.Design.Entities[definition.Entity]
		view := &compiledView{
			context:         c.input.Contract.Design.Namespace + "." + contextID,
			definition:      definition,
			entity:          entity,
			labels:          effectiveViewLabels(definition, c.input.Contract.Design),
			reduction:       effectiveViewReduction(definition, c.input.Contract.Design),
			inputs:          make(map[string]*compiledViewInput, len(definition.Inputs)),
			destinationAxes: c.environment.axes,
		}
		for _, inputID := range sortedMapKeys(definition.Inputs) {
			input, err := c.compileViewInput(contextID, inputID, definition.Inputs[inputID])
			if err != nil {
				return err
			}
			view.inputs[inputID] = input
		}
		labels, err := compileViewLabelUniverse(contextID, view.inputs)
		if err != nil {
			return err
		}
		if err := c.validateViewEntity(contextID, view, labels, entityRisk, entityLabelsSeen); err != nil {
			return err
		}
		if err := c.validateViewLabelClosure(contextID, view, labels); err != nil {
			return err
		}
		if err := c.validateViewStateEncodings(contextID, view); err != nil {
			return err
		}
		if err := validateViewReduction(contextID, view); err != nil {
			return err
		}
		if err := compileViewAxis(contextID, view); err != nil {
			return err
		}
		if err := c.validateViewPresentation(contextID, view); err != nil {
			return err
		}
		entityUses[definition.Entity]++
		c.program.views[contextID] = view
	}
	for _, entityID := range sortedMapKeys(c.input.Contract.Design.Entities) {
		entity := c.input.Contract.Design.Entities[entityID]
		if entityUses[entityID] == 0 {
			return fmt.Errorf("entity %q is unused", entityID)
		}
		for _, label := range entityIdentityLabels(entity.Identity) {
			if _, ok := entityLabelsSeen[entityID][label]; !ok {
				return fmt.Errorf("entity %q identity label %q occurs in no referenced source", entityID, label)
			}
		}
		if entityRisk[entityID] && entity.HighCardinalityAcceptance == nil {
			return fmt.Errorf("entity %q requires high_cardinality_acceptance", entityID)
		}
		if !entityRisk[entityID] && entity.HighCardinalityAcceptance != nil {
			return fmt.Errorf("entity %q high_cardinality_acceptance is unnecessary", entityID)
		}
	}
	return nil
}

func (c *semanticCompiler) compileViewInput(
	contextID string,
	inputID string,
	definition ViewInput,
) (*compiledViewInput, error) {
	program, signalID, err := c.resolveViewSignal(definition.Signal)
	if err != nil {
		return nil, fmt.Errorf("view %q input %q: %w", contextID, inputID, err)
	}
	signal := program.signals[signalID]
	componentSet := make(map[string]struct{}, len(definition.Components))
	for _, component := range definition.Components {
		if _, ok := signal.components[component]; !ok {
			return nil, fmt.Errorf("view %q input %q references unknown component %q on signal %q",
				contextID, inputID, component, definition.Signal)
		}
		componentSet[component] = struct{}{}
	}
	candidates := make([]*compiledOccurrence, 0, len(signal.occurrences))
	for _, occurrenceKey := range signal.occurrences {
		occurrence := program.occurrences[occurrenceKey]
		if _, ok := componentSet[occurrence.component]; ok && occurrence.terminalExclusion == "" {
			candidates = append(candidates, occurrence)
		}
	}
	catalog, err := mergeOccurrenceLabelCatalog(candidates)
	if err != nil {
		return nil, fmt.Errorf("view %q input %q: %w", contextID, inputID, err)
	}
	if err := validateLabelConditionAgainstCatalog(
		"views."+contextID+".inputs."+inputID+".where",
		definition.Where,
		catalog,
	); err != nil {
		return nil, err
	}

	compiled := &compiledViewInput{
		id:           inputID,
		renderedRole: inputID,
		definition:   definition,
	}
	destinationAvailability := unconditionalEnvironmentCondition()
	if supportID, _, ok := strings.Cut(definition.Signal, "/"); ok {
		destinationAvailability = c.program.supportAvailability[supportID]
	}
	if definition.RenderAs != "" {
		if definition.RenderAs == inputID {
			return nil, fmt.Errorf("view %q input %q render_as is redundant", contextID, inputID)
		}
		compiled.renderedRole = definition.RenderAs
	}
	defaultAlgorithms := make(map[string]struct{})
	for _, occurrence := range candidates {
		if !labelConditionMayMatch(definition.Where, occurrence.labels) {
			continue
		}
		component := signal.components[occurrence.component]
		algorithm := component.algorithm
		defaultAlgorithms[algorithm] = struct{}{}
		if definition.Algorithm != nil {
			algorithm = definition.Algorithm.Value
		}
		compiled.occurrences = append(compiled.occurrences, compiledViewOccurrence{
			sourceProfile:           program.profile,
			program:                 program,
			occurrence:              occurrence,
			component:               component,
			algorithm:               algorithm,
			destinationAvailability: destinationAvailability,
		})
	}
	if len(compiled.occurrences) == 0 {
		return nil, fmt.Errorf("view %q input %q can match no source occurrence", contextID, inputID)
	}
	if definition.Algorithm != nil && len(defaultAlgorithms) == 1 {
		if _, same := defaultAlgorithms[definition.Algorithm.Value]; same {
			return nil, fmt.Errorf("view %q input %q algorithm override is redundant", contextID, inputID)
		}
	}
	return compiled, nil
}

func (c *semanticCompiler) resolveViewSignal(reference string) (*CompiledSemanticContract, string, error) {
	parts := strings.Split(reference, "/")
	switch len(parts) {
	case 1:
		if !validID(parts[0]) {
			return nil, "", fmt.Errorf("signal reference %q is invalid", reference)
		}
		if _, ok := c.program.signals[parts[0]]; !ok {
			return nil, "", fmt.Errorf("references unknown signal %q", reference)
		}
		return c.program, parts[0], nil
	case 2:
		if !validID(parts[0]) || !validID(parts[1]) {
			return nil, "", fmt.Errorf("support signal reference %q is invalid", reference)
		}
		program, ok := c.program.supports[parts[0]]
		if !ok {
			return nil, "", fmt.Errorf("references undeclared support %q", parts[0])
		}
		if _, ok := program.signals[parts[1]]; !ok {
			return nil, "", fmt.Errorf("references unknown support signal %q", reference)
		}
		return program, parts[1], nil
	default:
		return nil, "", fmt.Errorf("signal reference %q must be <signal> or <support>/<signal>", reference)
	}
}

func mergeOccurrenceLabelCatalog(occurrences []*compiledOccurrence) (map[string]SourceLabel, error) {
	result := make(map[string]SourceLabel)
	counts := make(map[string]int)
	for _, occurrence := range occurrences {
		for label, schema := range occurrence.labels {
			if previous, ok := result[label]; ok {
				merged, err := mergeLabelSchemas(label, previous, schema)
				if err != nil {
					return nil, err
				}
				result[label] = merged
			} else {
				result[label] = schema
			}
			counts[label]++
		}
	}
	for label, schema := range result {
		if counts[label] != len(occurrences) {
			schema.Presence = LabelPresence{Kind: "optional"}
			result[label] = schema
		}
	}
	return result, nil
}

func mergeLabelSchemas(label string, left, right SourceLabel) (SourceLabel, error) {
	if left.Meaning != right.Meaning || left.Domain.Kind != right.Domain.Kind ||
		!sameEndpointCardinality(left.EndpointCardinality, right.EndpointCardinality) ||
		left.Stability != right.Stability {
		return SourceLabel{}, fmt.Errorf("label %q has incompatible schemas across source occurrences", label)
	}
	if !slices.Equal(left.Domain.Values, right.Domain.Values) {
		if left.Domain.Kind != "closed" || left.EndpointCardinality.Kind != "closed_domain" {
			return SourceLabel{}, fmt.Errorf("label %q has incompatible schemas across source occurrences", label)
		}
		left.Domain.Values = append(slices.Clone(left.Domain.Values), right.Domain.Values...)
		slices.Sort(left.Domain.Values)
		left.Domain.Values = slices.Compact(left.Domain.Values)
	}
	if left.Presence.Kind == "required" && right.Presence.Kind == "required" {
		left.Presence = LabelPresence{Kind: "required"}
	} else if left.Presence.keyIsAlwaysPresent() && right.Presence.keyIsAlwaysPresent() {
		left.Presence = LabelPresence{Kind: "present"}
	} else {
		left.Presence = LabelPresence{Kind: "optional"}
	}
	return left, nil
}

func sameEndpointCardinality(left, right EndpointCardinality) bool {
	if left.Kind != right.Kind || left.Axis != right.Axis || (left.Max == nil) != (right.Max == nil) {
		return false
	}
	return left.Max == nil || *left.Max == *right.Max
}

func labelConditionMayMatch(condition *LabelCondition, labels map[string]SourceLabel) bool {
	if condition == nil {
		return true
	}
	for _, clause := range condition.Any {
		matches := true
		for _, predicate := range clause.All {
			if !labelPredicateMayMatch(predicate, labels) {
				matches = false
				break
			}
		}
		if matches {
			return true
		}
	}
	return false
}

func labelPredicateMayMatch(predicate LabelPredicate, labels map[string]SourceLabel) bool {
	label, exists := labels[predicate.Label]
	if !exists {
		return predicate.Op == "absent"
	}
	mayBeAbsent := label.Presence.keyMayBeAbsent()
	switch predicate.Op {
	case "absent":
		return mayBeAbsent
	case "present":
		return true
	case "nonblank":
		return labelMayHaveNonblankValue(label)
	case "eq":
		return labelValueMayMatch(label, *predicate.Value)
	case "in":
		return slices.ContainsFunc(predicate.Values, func(value string) bool { return labelValueMayMatch(label, value) })
	default:
		panic("validated label predicate has no matcher")
	}
}

func labelValueMayMatch(label SourceLabel, value string) bool {
	if label.Presence.Kind == "required" && strings.TrimSpace(value) == "" {
		return false
	}
	return label.Domain.Kind == "open" || slices.Contains(label.Domain.Values, value)
}

func labelMayHaveNonblankValue(label SourceLabel) bool {
	if label.Domain.Kind == "open" {
		return true
	}
	return slices.ContainsFunc(label.Domain.Values, func(value string) bool {
		return strings.TrimSpace(value) != ""
	})
}

func compileViewLabelUniverse(
	contextID string,
	inputs map[string]*compiledViewInput,
) (map[string]SourceLabel, error) {
	result := make(map[string]SourceLabel)
	for _, inputID := range sortedMapKeys(inputs) {
		for _, source := range inputs[inputID].occurrences {
			for label, schema := range source.occurrence.labels {
				if previous, ok := result[label]; ok {
					merged, err := mergeLabelSchemas(label, previous, schema)
					if err != nil {
						return nil, fmt.Errorf("view %q: %w", contextID, err)
					}
					result[label] = merged
				} else {
					result[label] = schema
				}
			}
		}
	}
	return result, nil
}

func (c *semanticCompiler) validateViewEntity(
	contextID string,
	view *compiledView,
	labels map[string]SourceLabel,
	entityRisk map[string]bool,
	entityLabelsSeen map[string]map[string]struct{},
) error {
	entityID := view.definition.Entity
	if entityLabelsSeen[entityID] == nil {
		entityLabelsSeen[entityID] = make(map[string]struct{})
	}
	required := make(map[string]struct{}, len(view.entity.Identity.Required))
	for _, label := range view.entity.Identity.Required {
		required[label] = struct{}{}
	}
	identity := entityIdentityLabels(view.entity.Identity)
	for _, label := range identity {
		if schema, ok := labels[label]; ok {
			entityLabelsSeen[entityID][label] = struct{}{}
			if highCardinalityClass(schema.EndpointCardinality.Kind) {
				entityRisk[entityID] = true
			}
		}
	}
	for _, input := range view.inputs {
		for _, source := range input.occurrences {
			for label := range required {
				schema, ok := source.occurrence.labels[label]
				if !ok || !labelGuaranteedOnOccurrence(source.program, source.occurrence, schema) &&
					!conditionGuaranteesLabel(input.definition.Where, label) {
					return fmt.Errorf("view %q entity %q required identity label %q is not guaranteed on occurrence %q",
						contextID, entityID, label, source.occurrence.key)
				}
			}
			if err := validateOccurrenceIdentityAlternatives(contextID, entityID, input, source, view.entity); err != nil {
				return err
			}
		}
	}
	return nil
}

func highCardinalityClass(kind string) bool {
	return kind == "operational_population" || kind == "unbounded" || kind == "unknown"
}

func labelGuaranteedOnOccurrence(
	program *CompiledSemanticContract,
	occurrence *compiledOccurrence,
	label SourceLabel,
) bool {
	if label.Presence.Kind == "required" {
		return true
	}
	if label.Presence.Kind == "optional" {
		return false
	}
	presence, err := program.environment.resolve(label.Presence.When)
	return err == nil && occurrence.availability.coveredBy(program.environment.axes, presence)
}

func (c *semanticCompiler) validateViewLabelClosure(
	contextID string,
	view *compiledView,
	labels map[string]SourceLabel,
) error {
	for _, label := range entityIdentityLabels(view.entity.Identity) {
		delete(labels, label)
	}
	classified := make(map[string]string, len(view.labels.Dimensions)+len(view.labels.Promote)+len(view.labels.Omit))
	for label := range view.labels.Dimensions {
		classified[label] = "dimensions"
	}
	for _, label := range view.labels.Promote {
		classified[label] = "promote"
	}
	for label := range view.labels.Omit {
		classified[label] = "omit"
	}
	for _, label := range sortedMapKeys(labels) {
		if classified[label] == "" {
			return fmt.Errorf("view %q has unclassified label %q", contextID, label)
		}
	}
	for _, label := range sortedMapKeys(classified) {
		if _, ok := labels[label]; !ok {
			return fmt.Errorf("view %q classifies inapplicable label %q as %s", contextID, label, classified[label])
		}
	}
	for _, label := range sortedMapKeys(view.labels.Dimensions) {
		if err := validateDimensionLabel(contextID, label, view.labels.Dimensions[label], view.inputs); err != nil {
			return err
		}
	}
	for _, label := range view.labels.Promote {
		if err := validatePromotedLabel(contextID, label, view); err != nil {
			return err
		}
	}
	return nil
}

func validateDimensionLabel(
	contextID string,
	label string,
	rendering DimensionRendering,
	inputs map[string]*compiledViewInput,
) error {
	seen := false
	for _, inputID := range sortedMapKeys(inputs) {
		input := inputs[inputID]
		inputHasLabel := false
		for _, source := range input.occurrences {
			schema, ok := source.occurrence.labels[label]
			if !ok {
				continue
			}
			seen = true
			inputHasLabel = true
			if rendering.Render == "label_value" {
				bounded := schema.Domain.Kind == "closed" || schema.EndpointCardinality.Kind == "bounded_configuration"
				if !bounded {
					return fmt.Errorf("view %q dimension label %q has no closed or bounded domain", contextID, label)
				}
				if !labelGuaranteedOnOccurrence(source.program, source.occurrence, schema) &&
					!conditionGuaranteesLabel(input.definition.Where, label) {
					return fmt.Errorf("view %q optional dimension label %q has no explicit membership guarantee", contextID, label)
				}
			}
		}
		if rendering.Render == "input_role" && inputHasLabel && !conditionPartitionsLabel(input.definition.Where, label) {
			return fmt.Errorf("view %q input-role label %q is not partitioned by input %q", contextID, label, inputID)
		}
	}
	if !seen {
		return fmt.Errorf("view %q dimension label %q is inapplicable", contextID, label)
	}
	return nil
}

func conditionGuaranteesLabel(condition *LabelCondition, label string) bool {
	if condition == nil {
		return false
	}
	for _, clause := range condition.Any {
		guaranteed := slices.ContainsFunc(clause.All, func(predicate LabelPredicate) bool {
			return predicate.Label == label && predicate.Op != "absent"
		})
		if !guaranteed {
			return false
		}
	}
	return true
}

func conditionPartitionsLabel(condition *LabelCondition, label string) bool {
	if condition == nil {
		return false
	}
	for _, clause := range condition.Any {
		partitioned := slices.ContainsFunc(clause.All, func(predicate LabelPredicate) bool {
			return predicate.Label == label && (predicate.Op == "eq" || predicate.Op == "in")
		})
		if !partitioned {
			return false
		}
	}
	return true
}

func validatePromotedLabel(contextID, label string, view *compiledView) error {
	for _, input := range view.inputs {
		for _, source := range input.occurrences {
			schema, ok := source.occurrence.labels[label]
			if !ok {
				return fmt.Errorf("view %q promoted label %q is absent from occurrence %q",
					contextID, label, source.occurrence.key)
			}
			if schema.Stability == "dynamic" {
				return fmt.Errorf("view %q promoted label %q is dynamic", contextID, label)
			}
			if !labelDeterminedByEntity(source, label, view.entity) {
				return fmt.Errorf("view %q promoted label %q is not functionally determined by entity identity",
					contextID, label)
			}
		}
	}
	return nil
}

func labelDeterminedByEntity(source compiledViewOccurrence, label string, entity EntityDefinition) bool {
	if len(entity.Identity.Alternatives) == 0 {
		return labelDeterminedByIdentity(
			source,
			label,
			append(slices.Clone(entity.Identity.Required), entity.Identity.Optional...),
		)
	}
	for _, alternative := range entity.Identity.Alternatives {
		identity := slices.Clone(entity.Identity.Required)
		identity = append(identity, alternative...)
		identity = append(identity, entity.Identity.Optional...)
		if !labelDeterminedByIdentity(source, label, identity) {
			return false
		}
	}
	return true
}

func validateOccurrenceIdentityAlternatives(
	contextID string,
	entityID string,
	input *compiledViewInput,
	source compiledViewOccurrence,
	entity EntityDefinition,
) error {
	if len(entity.Identity.Alternatives) == 0 ||
		identityAlternativesStaticallyGuaranteed(input, source, entity.Identity.Alternatives) ||
		identityAlternativesConstrained(source, entity.Identity.Alternatives) {
		return nil
	}
	return fmt.Errorf(
		"view %q entity %q identity alternatives are neither statically guaranteed nor constrained on occurrence %q",
		contextID, entityID, source.occurrence.key,
	)
}

func identityAlternativesStaticallyGuaranteed(
	input *compiledViewInput,
	source compiledViewOccurrence,
	alternatives [][]string,
) bool {
	selected := -1
	for index, alternative := range alternatives {
		guaranteed := true
		for _, label := range alternative {
			schema, ok := source.occurrence.labels[label]
			if !ok || !identityLabelGuaranteedNonblankOnOccurrence(source.program, source.occurrence, schema) &&
				!conditionGuaranteesNonblankLabel(input.definition.Where, label) {
				guaranteed = false
				break
			}
		}
		if guaranteed {
			if selected != -1 {
				return false
			}
			selected = index
		}
	}
	if selected == -1 {
		return false
	}
	for index, alternative := range alternatives {
		if index == selected {
			continue
		}
		for _, label := range alternative {
			if _, ok := source.occurrence.labels[label]; ok &&
				!conditionGuaranteesAbsentLabel(input.definition.Where, label) {
				return false
			}
		}
	}
	return true
}

func identityLabelGuaranteedNonblankOnOccurrence(
	program *CompiledSemanticContract,
	occurrence *compiledOccurrence,
	label SourceLabel,
) bool {
	switch label.Presence.Kind {
	case "required":
		return true
	case "present", "optional":
		return false
	default:
		presence, err := program.environment.resolve(label.Presence.When)
		return err == nil && occurrence.availability.coveredBy(program.environment.axes, presence)
	}
}

func identityAlternativesConstrained(source compiledViewOccurrence, alternatives [][]string) bool {
	want := canonicalLabelAlternatives(alternatives)
	signal := source.program.signals[source.occurrence.signal]
	for _, constraint := range signal.labelPresenceConstraints {
		normalized := make([][]string, len(constraint.Alternatives))
		for index, alternative := range constraint.Alternatives {
			normalized[index] = make([]string, len(alternative))
			for labelIndex, label := range alternative {
				normalized[index][labelIndex] = normalizedOccurrenceIdentityLabel(source.occurrence, label)
			}
		}
		if slices.Equal(want, canonicalLabelAlternatives(normalized)) {
			return true
		}
	}
	return false
}

func conditionGuaranteesNonblankLabel(condition *LabelCondition, label string) bool {
	if condition == nil {
		return false
	}
	for _, clause := range condition.Any {
		guaranteed := slices.ContainsFunc(clause.All, func(predicate LabelPredicate) bool {
			if predicate.Label != label {
				return false
			}
			switch predicate.Op {
			case "nonblank":
				return true
			case "eq":
				return predicate.Value != nil && strings.TrimSpace(*predicate.Value) != ""
			case "in":
				return len(predicate.Values) != 0 && !slices.ContainsFunc(predicate.Values, func(value string) bool {
					return strings.TrimSpace(value) == ""
				})
			default:
				return false
			}
		})
		if !guaranteed {
			return false
		}
	}
	return true
}

func conditionGuaranteesAbsentLabel(condition *LabelCondition, label string) bool {
	if condition == nil {
		return false
	}
	for _, clause := range condition.Any {
		if !slices.ContainsFunc(clause.All, func(predicate LabelPredicate) bool {
			return predicate.Label == label && predicate.Op == "absent"
		}) {
			return false
		}
	}
	return true
}

func labelDeterminedByIdentity(source compiledViewOccurrence, label string, identity []string) bool {
	schema := source.occurrence.labels[label]
	if schema.EndpointCardinality.Kind == "singleton" {
		return true
	}
	closure := make(map[string]struct{}, len(identity))
	for _, name := range identity {
		name = normalizedOccurrenceIdentityLabel(source.occurrence, name)
		if _, ok := source.occurrence.labels[name]; ok {
			closure[name] = struct{}{}
		}
	}
	changed := true
	for changed {
		changed = false
		for _, dependency := range source.occurrence.dependencies {
			if !dependencyGuaranteed(source.program, source.occurrence, dependency) ||
				!allLabelsPresent(closure, dependency.Determinants) {
				continue
			}
			for _, dependent := range dependency.Dependents {
				if _, ok := closure[dependent]; !ok {
					closure[dependent] = struct{}{}
					changed = true
				}
			}
		}
	}
	_, ok := closure[label]
	return ok
}

func normalizedOccurrenceIdentityLabel(occurrence *compiledOccurrence, label string) string {
	for next := occurrence.labelRenames[label]; next != ""; next = occurrence.labelRenames[next] {
		label = next
	}
	return label
}

func dependencyGuaranteed(
	program *CompiledSemanticContract,
	occurrence *compiledOccurrence,
	dependency FunctionalDependency,
) bool {
	if dependency.When.IsZero() {
		return true
	}
	condition, err := program.environment.resolve(dependency.When)
	return err == nil && occurrence.availability.coveredBy(program.environment.axes, condition)
}

func allLabelsPresent(values map[string]struct{}, labels []string) bool {
	for _, label := range labels {
		if _, ok := values[label]; !ok {
			return false
		}
	}
	return true
}

func compileViewAxis(contextID string, view *compiledView) error {
	var (
		quantity      string
		object        string
		effectiveRate string
		unit          string
		scale         rationalScale
		initialized   bool
		buckets       int
		total         int
	)
	for _, inputID := range sortedMapKeys(view.inputs) {
		input := view.inputs[inputID]
		for _, source := range input.occurrences {
			component := source.component
			resolvedUnit := component.canonicalUnit
			resolvedScale := component.scale
			if view.definition.Display != nil {
				var err error
				resolvedUnit, resolvedScale, err = resolveRegisteredDisplay(
					"views."+contextID+".display",
					component,
					view.definition.Display.Convention,
				)
				if err != nil {
					return err
				}
			}
			if !initialized {
				quantity = component.source.Unit.Quantity
				object = component.source.Unit.Object
				effectiveRate = component.effectiveRate
				unit = resolvedUnit
				scale = resolvedScale
				initialized = true
			} else if quantity != component.source.Unit.Quantity || object != component.source.Unit.Object ||
				effectiveRate != component.effectiveRate || unit != resolvedUnit || scale != resolvedScale {
				return fmt.Errorf("view %q mixes incompatible component algebra", contextID)
			}
			if component.source.WireRole == "histogram_bucket" {
				buckets++
			}
			total++
		}
	}
	view.unit = unit
	view.scale = scale
	if buckets == total {
		view.presentation = "heatmap"
	} else if buckets == 0 {
		view.presentation = "line"
	} else {
		return fmt.Errorf("view %q mixes histogram buckets with non-bucket components", contextID)
	}
	if view.definition.Presentation != nil {
		view.presentation = view.definition.Presentation.Type
	}
	return nil
}

func effectiveViewLabels(view ViewDefinition, design ProfileDesignDocument) ViewLabels {
	if view.LabelPolicy != "" {
		return design.LabelPolicies[view.LabelPolicy]
	}
	return *view.Labels
}

func effectiveViewReduction(view ViewDefinition, design ProfileDesignDocument) *ReductionDefinition {
	if view.ReductionPolicy != "" {
		value := design.ReductionPolicies[view.ReductionPolicy]
		return &value
	}
	return view.Reduction
}
