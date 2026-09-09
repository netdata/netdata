// SPDX-License-Identifier: GPL-3.0-or-later

package promsemantics

import (
	"fmt"
	"slices"
	"strings"
)

func (c *semanticCompiler) compileNormalizations() error {
	if err := c.compileOccurrences(); err != nil {
		return err
	}
	if len(c.input.Contract.Design.Normalizations) == 0 {
		return nil
	}

	nodes := make(map[string]*compiledNormalization, len(c.input.Contract.Design.Normalizations))
	eventualLabels := make(map[string]map[string]struct{}, len(c.program.occurrences))
	for key, occurrence := range c.program.occurrences {
		eventualLabels[key] = make(map[string]struct{}, len(occurrence.labels))
		for label := range occurrence.labels {
			eventualLabels[key][label] = struct{}{}
		}
	}

	for _, id := range sortedMapKeys(c.input.Contract.Design.Normalizations) {
		definition := c.input.Contract.Design.Normalizations[id]
		node := &compiledNormalization{id: id, definition: definition}
		var err error
		switch definition.Kind {
		case "category":
			node.occurrences, err = c.sourceReferenceOccurrences(id, *definition.AppliesTo)
		case "finite_alias":
			node.occurrences, err = c.sourceReferenceOccurrences(id, *definition.AppliesTo)
			node.familyAliases = cloneStringMap(definition.SourceFamily)
		case "namespace_alias":
			node.occurrences, node.familyAliases, err = c.namespaceAliasOccurrences(id, definition)
		case "embedded_identity_repair", "embedded_identity_extract":
			node.occurrences, err = c.grammarOccurrences(id, definition)
		case "generated_component_exclusion":
			node.occurrences, err = c.generatedComponentOccurrences(id, definition)
		case "label_rename":
			// Its profile-wide scope is closed below from the complete possible-label catalog.
		default:
			panic("validated normalization kind has no compiler")
		}
		if err != nil {
			return err
		}
		if definition.Kind != "label_rename" && len(node.occurrences) == 0 {
			return fmt.Errorf("normalization %q has no applicable source occurrence", id)
		}
		nodes[id] = node
		if target := normalizationTargetLabel(definition); target != "" && definition.Kind != "label_rename" {
			for _, occurrence := range node.occurrences {
				eventualLabels[occurrence][target] = struct{}{}
			}
		}
	}

	if err := c.resolveRenameScopes(nodes, eventualLabels); err != nil {
		return err
	}
	order, err := c.normalizationOrder(nodes)
	if err != nil {
		return err
	}
	for _, id := range order {
		node := nodes[id]
		if err := c.applyNormalizationSchema(node); err != nil {
			return err
		}
		c.program.normalizations = append(c.program.normalizations, node)
	}
	return nil
}

func (c *semanticCompiler) compileOccurrences() error {
	for _, signalID := range sortedMapKeys(c.program.signals) {
		signal := c.program.signals[signalID]
		for _, registrationKey := range signal.registrations {
			registration := c.program.registrations[registrationKey]
			availability, ok := signalOwnerAvailability(registration, signalID)
			if !ok {
				return fmt.Errorf("signal %q has registration %q without its compiled owner", signalID, registrationKey)
			}
			for _, componentID := range sortedMapKeys(signal.components) {
				component := signal.components[componentID]
				if !registrationHasWireRole(registration, component.source.WireRole) {
					return fmt.Errorf("signal %q component %q wire role %q is absent from registration %q",
						signalID, componentID, component.source.WireRole, registrationKey)
				}
				key := signalID + "/" + registrationKey + "/" + componentID
				c.program.occurrences[key] = &compiledOccurrence{
					key:          key,
					signal:       signalID,
					component:    componentID,
					registration: registrationKey,
					family:       registration.family,
					availability: availability,
					sourceLabels: cloneSourceLabelMap(signal.labels),
					labels:       runtimeSourceLabelMap(signal.labels),
					dependencies: cloneFunctionalDependencies(
						c.input.Contract.Source.Signals[signalID].FunctionalDependencies,
					),
				}
				signal.occurrences = append(signal.occurrences, key)
			}
		}
		slices.Sort(signal.occurrences)
	}
	return nil
}

func runtimeSourceLabelMap(source map[string]SourceLabel) map[string]SourceLabel {
	result := make(map[string]SourceLabel, len(source))
	for name, schema := range source {
		hadBlank := false
		if schema.Domain.Kind == "closed" {
			values := make([]string, 0, len(schema.Domain.Values))
			for _, value := range schema.Domain.Values {
				if strings.TrimSpace(value) == "" {
					hadBlank = true
					continue
				}
				values = append(values, value)
			}
			schema.Domain.Values = values
		}
		switch schema.Presence.Kind {
		case "present":
			if schema.Domain.Kind == "open" || hadBlank {
				schema.Presence = LabelPresence{Kind: "optional"}
			} else {
				schema.Presence = LabelPresence{Kind: "required"}
			}
		}
		if schema.Domain.Kind == "closed" && len(schema.Domain.Values) == 0 {
			continue
		}
		result[name] = schema
	}
	return result
}

func signalOwnerAvailability(
	registration *compiledRegistration,
	signal string,
) (compiledEnvironmentCondition, bool) {
	for _, owner := range registration.owners {
		if owner.kind == "signal" && owner.id == signal {
			return owner.availability, true
		}
	}
	return compiledEnvironmentCondition{}, false
}

func registrationHasWireRole(registration *compiledRegistration, role string) bool {
	for _, candidate := range registration.components {
		if candidate == role {
			return true
		}
	}
	return false
}

func (c *semanticCompiler) sourceReferenceOccurrences(
	normalizationID string,
	reference SourceReference,
) ([]string, error) {
	signal, ok := c.program.signals[reference.Signal]
	if !ok {
		return nil, fmt.Errorf("normalization %q references unknown signal %q", normalizationID, reference.Signal)
	}
	components := make(map[string]struct{}, len(reference.Components))
	for _, component := range reference.Components {
		if _, ok := signal.components[component]; !ok {
			return nil, fmt.Errorf("normalization %q references unknown component %q on signal %q",
				normalizationID, component, reference.Signal)
		}
		components[component] = struct{}{}
	}
	result := make([]string, 0, len(signal.occurrences))
	for _, key := range signal.occurrences {
		if _, ok := components[c.program.occurrences[key].component]; ok {
			result = append(result, key)
		}
	}
	return result, nil
}

func (c *semanticCompiler) namespaceAliasOccurrences(
	normalizationID string,
	definition Normalization,
) ([]string, map[string]string, error) {
	if c.input.Contract.Registry == nil {
		return nil, nil, fmt.Errorf("normalization %q requires a generated source registry", normalizationID)
	}
	group, ok := c.input.Contract.Registry.Registry.Groups[definition.RegistryGroup]
	if !ok {
		return nil, nil, fmt.Errorf("normalization %q references unknown registry group %q",
			normalizationID, definition.RegistryGroup)
	}

	aliases := make(map[string]string, len(group.Registrations))
	usedRegistrations := make(map[string]struct{}, len(group.Registrations))
	var occurrences []string
	for _, occurrenceKey := range sortedMapKeys(c.program.occurrences) {
		occurrence := c.program.occurrences[occurrenceKey]
		registration := c.program.registrations[occurrence.registration]
		if registration.group != definition.RegistryGroup {
			continue
		}
		language, err := c.registrationLanguage(registration)
		if err != nil {
			return nil, nil, err
		}
		if language.embedded != nil || len(language.exact) != 1 {
			return nil, nil, fmt.Errorf("normalization %q namespace alias requires exact source registration %q",
				normalizationID, registration.id)
		}
		source := language.exact[0]
		if !strings.HasPrefix(source, definition.SourcePrefix) || len(source) == len(definition.SourcePrefix) {
			return nil, nil, fmt.Errorf("normalization %q source registration %q family %q is outside prefix %q",
				normalizationID, registration.id, source, definition.SourcePrefix)
		}
		target := definition.TargetPrefix + strings.TrimPrefix(source, definition.SourcePrefix)
		if err := validateMetricName("normalization "+normalizationID+" derived target family", target); err != nil {
			return nil, nil, err
		}
		if err := c.validateNamespaceAliasTarget(normalizationID, occurrence, registration, target); err != nil {
			return nil, nil, err
		}
		if previous, exists := aliases[source]; exists && previous != target {
			return nil, nil, fmt.Errorf("normalization %q source family %q maps to both %q and %q",
				normalizationID, source, previous, target)
		}
		aliases[source] = target
		usedRegistrations[registration.id] = struct{}{}
		occurrences = append(occurrences, occurrenceKey)
	}
	for registrationID := range group.Registrations {
		if _, ok := usedRegistrations[registrationID]; !ok {
			return nil, nil, fmt.Errorf("normalization %q registry group %q registration %q has no signal-owned occurrence",
				normalizationID, definition.RegistryGroup, registrationID)
		}
	}
	return occurrences, aliases, nil
}

func (c *semanticCompiler) validateNamespaceAliasTarget(
	normalizationID string,
	sourceOccurrence *compiledOccurrence,
	sourceRegistration *compiledRegistration,
	targetFamily string,
) error {
	signal := c.program.signals[sourceOccurrence.signal]
	found := false
	for _, registrationKey := range signal.registrations {
		registration := c.program.registrations[registrationKey]
		language, err := c.registrationLanguage(registration)
		if err != nil {
			return err
		}
		if language.embedded != nil || !slices.Contains(language.exact, targetFamily) {
			continue
		}
		if registration.group == sourceRegistration.group {
			return fmt.Errorf("normalization %q target family %q remains in source registry group %q",
				normalizationID, targetFamily, sourceRegistration.group)
		}
		if registration.prometheus != sourceRegistration.prometheus {
			return fmt.Errorf("normalization %q target family %q on signal %q changes Prometheus contract",
				normalizationID, targetFamily, sourceOccurrence.signal)
		}
		found = true
	}
	if !found {
		return fmt.Errorf("normalization %q target family %q has no registration on signal %q",
			normalizationID, targetFamily, sourceOccurrence.signal)
	}
	return nil
}

func (c *semanticCompiler) grammarOccurrences(
	normalizationID string,
	definition Normalization,
) ([]string, error) {
	if c.input.Contract.Registry == nil {
		return nil, fmt.Errorf("normalization %q requires a generated source registry", normalizationID)
	}
	grammar, ok := c.input.Contract.Registry.Registry.FamilyGrammars[definition.RegistryGrammar]
	if !ok {
		return nil, fmt.Errorf("normalization %q references unknown registry grammar %q",
			normalizationID, definition.RegistryGrammar)
	}
	if err := validateNormalizationGrammar(normalizationID, definition, grammar); err != nil {
		return nil, err
	}
	result := make([]string, 0)
	for _, key := range sortedMapKeys(c.program.occurrences) {
		if c.program.occurrences[key].family.Grammar == definition.RegistryGrammar {
			result = append(result, key)
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("normalization %q registry grammar %q has no signal-owned occurrences",
			normalizationID, definition.RegistryGrammar)
	}
	return result, nil
}

func validateNormalizationGrammar(
	normalizationID string,
	definition Normalization,
	grammar FamilyGrammar,
) error {
	identitySlot := ""
	for formID, form := range grammar.Forms {
		if form.Canonical == nil || form.Embedded == nil {
			return fmt.Errorf("normalization %q grammar form %q has no embedded identity", normalizationID, formID)
		}
		if definition.Kind == "embedded_identity_repair" {
			if form.Canonical.Prefix != definition.Canonical.FamilyPrefix ||
				form.Embedded.Prefix != definition.Embedded.FamilyPrefix ||
				form.Embedded.IdentitySlot.Name != definition.Embedded.Capture {
				return fmt.Errorf("normalization %q disagrees with registry grammar form %q", normalizationID, formID)
			}
		} else {
			if identitySlot == "" {
				identitySlot = form.Embedded.IdentitySlot.Name
			} else if form.Embedded.IdentitySlot.Name != identitySlot {
				return fmt.Errorf("normalization %q grammar forms do not share one identity slot", normalizationID)
			}
		}
	}
	if definition.Kind == "embedded_identity_repair" {
		if definition.Canonical.IdentityLabel != definition.SourceIdentityLabel {
			return fmt.Errorf("normalization %q canonical identity label must equal source_identity_label", normalizationID)
		}
		want := []string{definition.Embedded.Capture, definition.SourceIdentityLabel}
		if !slices.Equal(definition.Identity.Operands, want) {
			return fmt.Errorf("normalization %q identity operands must be %v", normalizationID, want)
		}
	}
	return nil
}

func (c *semanticCompiler) generatedComponentOccurrences(
	normalizationID string,
	definition Normalization,
) ([]string, error) {
	result := make([]string, 0)
	for _, key := range sortedMapKeys(c.program.occurrences) {
		occurrence := c.program.occurrences[key]
		registration := c.program.registrations[occurrence.registration]
		if registration.group == "" ||
			c.program.signals[occurrence.signal].components[occurrence.component].source.WireRole != definition.Source.Component {
			continue
		}
		language, err := c.registrationLanguage(registration)
		if err != nil {
			return nil, err
		}
		matched := slices.ContainsFunc(language.exact, func(family string) bool {
			return strings.HasPrefix(family, definition.Source.NamespacePrefix) &&
				strings.HasSuffix(family, definition.Source.TerminalSuffix)
		})
		if !matched && language.embedded != nil {
			form := language.embedded.form
			matched = strings.HasPrefix(form.Prefix, definition.Source.NamespacePrefix) &&
				strings.HasSuffix(form.Separator+form.Suffix, definition.Source.TerminalSuffix)
		}
		if matched {
			result = append(result, key)
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("normalization %q generated component class matches no occurrence", normalizationID)
	}
	return result, nil
}

func (c *semanticCompiler) resolveRenameScopes(
	nodes map[string]*compiledNormalization,
	eventualLabels map[string]map[string]struct{},
) error {
	consumers := make(map[string][]*compiledNormalization)
	seenScopes := make(map[string]map[string]struct{})
	terminalGenerated := make(map[string]struct{})
	for _, id := range sortedMapKeys(nodes) {
		node := nodes[id]
		if node.definition.Kind == "generated_component_exclusion" {
			for _, occurrence := range node.occurrences {
				terminalGenerated[occurrence] = struct{}{}
			}
		}
		if node.definition.Kind != "label_rename" {
			continue
		}
		consumers[node.definition.SourceLabel] = append(consumers[node.definition.SourceLabel], node)
		seenScopes[id] = make(map[string]struct{})
	}
	type labelEvent struct {
		occurrence string
		label      string
	}
	queue := make([]labelEvent, 0)
	for _, occurrence := range sortedMapKeys(eventualLabels) {
		if _, terminal := terminalGenerated[occurrence]; terminal {
			continue
		}
		for _, label := range sortedMapKeys(eventualLabels[occurrence]) {
			queue = append(queue, labelEvent{occurrence: occurrence, label: label})
		}
	}
	for head := 0; head < len(queue); head++ {
		event := queue[head]
		for _, node := range consumers[event.label] {
			if _, ok := seenScopes[node.id][event.occurrence]; ok {
				continue
			}
			seenScopes[node.id][event.occurrence] = struct{}{}
			node.occurrences = append(node.occurrences, event.occurrence)
			target := node.definition.TargetLabel
			if _, ok := eventualLabels[event.occurrence][target]; !ok {
				eventualLabels[event.occurrence][target] = struct{}{}
				queue = append(queue, labelEvent{occurrence: event.occurrence, label: target})
			}
		}
	}
	for _, id := range sortedMapKeys(nodes) {
		node := nodes[id]
		if node.definition.Kind != "label_rename" {
			continue
		}
		slices.Sort(node.occurrences)
		if len(node.occurrences) == 0 {
			return fmt.Errorf("normalization %q source label %q occurs nowhere", id, nodes[id].definition.SourceLabel)
		}
	}
	return nil
}

func (c *semanticCompiler) normalizationOrder(
	nodes map[string]*compiledNormalization,
) ([]string, error) {
	producers := make(map[string]map[string]string, len(c.program.occurrences))
	deleters := make(map[string]map[string]string, len(c.program.occurrences))
	nameWriters := make(map[string]string, len(c.program.occurrences))
	terminalWriters := make(map[string]string, len(c.program.occurrences))
	for _, id := range sortedMapKeys(nodes) {
		node := nodes[id]
		definition := node.definition
		for _, occurrenceKey := range node.occurrences {
			occurrence := c.program.occurrences[occurrenceKey]
			if target := normalizationTargetLabel(definition); target != "" {
				if producers[occurrenceKey] == nil {
					producers[occurrenceKey] = make(map[string]string)
				}
				if previous := producers[occurrenceKey][target]; previous != "" {
					return nil, fmt.Errorf("normalizations %q and %q are multiple writers of label %q on occurrence %q",
						previous, id, target, occurrenceKey)
				}
				_, targetIsExistingRepairIdentity := occurrence.labels[target]
				targetIsExistingRepairIdentity = targetIsExistingRepairIdentity &&
					definition.Kind == "embedded_identity_repair" && target == definition.Canonical.IdentityLabel
				if _, exists := occurrence.labels[target]; exists && !targetIsExistingRepairIdentity {
					return nil, fmt.Errorf("normalization %q target label %q already exists on occurrence %q",
						id, target, occurrenceKey)
				}
				producers[occurrenceKey][target] = id
			}
			if definition.Kind == "label_rename" && !*definition.RetainSource {
				if deleters[occurrenceKey] == nil {
					deleters[occurrenceKey] = make(map[string]string)
				}
				if previous := deleters[occurrenceKey][definition.SourceLabel]; previous != "" {
					return nil, fmt.Errorf("normalizations %q and %q both delete label %q on occurrence %q",
						previous, id, definition.SourceLabel, occurrenceKey)
				}
				deleters[occurrenceKey][definition.SourceLabel] = id
			}
			if normalizationWritesMetricName(definition.Kind) {
				if previous := nameWriters[occurrenceKey]; previous != "" {
					return nil, fmt.Errorf("normalizations %q and %q are multiple metric-name writers on occurrence %q",
						previous, id, occurrenceKey)
				}
				nameWriters[occurrenceKey] = id
			}
			if normalizationMayTerminate(definition.Kind) {
				if previous := terminalWriters[occurrenceKey]; previous != "" {
					return nil, fmt.Errorf("normalizations %q and %q are multiple terminal writers on occurrence %q",
						previous, id, occurrenceKey)
				}
				terminalWriters[occurrenceKey] = id
			}
		}
	}

	edges := make(map[string]map[string]struct{}, len(nodes))
	addEdge := func(before, after string) {
		if before == after {
			return
		}
		if edges[before] == nil {
			edges[before] = make(map[string]struct{})
		}
		edges[before][after] = struct{}{}
	}
	for _, id := range sortedMapKeys(nodes) {
		node := nodes[id]
		for _, occurrenceKey := range node.occurrences {
			occurrence := c.program.occurrences[occurrenceKey]
			for _, label := range normalizationReadLabels(node.definition) {
				producer := producers[occurrenceKey][label]
				_, sourceExists := occurrence.labels[label]
				if node.definition.Kind == "category" && label == semanticMetricNameField {
					sourceExists = true
				}
				if producer == id && !sourceExists {
					return nil, fmt.Errorf("normalization %q reads label %q that it creates on occurrence %q",
						id, label, occurrenceKey)
				}
				if producer == "" && !sourceExists {
					return nil, fmt.Errorf("normalization %q reads unknown label %q on occurrence %q",
						id, label, occurrenceKey)
				}
				if producer != "" && producer != id {
					addEdge(producer, id)
				}
				if deleter := deleters[occurrenceKey][label]; deleter != "" && deleter != id {
					addEdge(id, deleter)
				}
			}
			if node.definition.Kind == "generated_component_exclusion" {
				if writer := nameWriters[occurrenceKey]; writer != "" && writer != id {
					addEdge(id, writer)
				}
			}
		}
	}
	for predecessor, successors := range edges {
		for successor := range successors {
			if nodes[successor].predecessors == nil {
				nodes[successor].predecessors = make(map[string]struct{})
			}
			nodes[successor].predecessors[predecessor] = struct{}{}
		}
	}
	return topologicalNormalizationOrder(nodes, edges)
}

func topologicalNormalizationOrder(
	nodes map[string]*compiledNormalization,
	edges map[string]map[string]struct{},
) ([]string, error) {
	state := make(map[string]uint8, len(nodes))
	postorder := make([]string, 0, len(nodes))
	stack := make([]string, 0, len(nodes))
	var visit func(string) error
	visit = func(id string) error {
		switch state[id] {
		case 1:
			return fmt.Errorf("normalization dependency cycle includes %s -> %s", strings.Join(stack, " -> "), id)
		case 2:
			return nil
		}
		state[id] = 1
		stack = append(stack, id)
		for _, next := range sortedMapKeys(edges[id]) {
			if err := visit(next); err != nil {
				return err
			}
		}
		stack = stack[:len(stack)-1]
		state[id] = 2
		postorder = append(postorder, id)
		return nil
	}
	for _, id := range sortedMapKeys(nodes) {
		if err := visit(id); err != nil {
			return nil, err
		}
	}
	slices.Reverse(postorder)
	return postorder, nil
}

func (c *semanticCompiler) applyNormalizationSchema(node *compiledNormalization) error {
	definition := node.definition
	branches := make(map[string]struct{})
	if definition.Kind == "finite_alias" {
		if err := c.validateFiniteAlias(node); err != nil {
			return err
		}
		for family := range definition.SourceFamily {
			branches["family:"+family] = struct{}{}
		}
	}
	if definition.Kind == "namespace_alias" {
		for family := range node.familyAliases {
			branches["family:"+family] = struct{}{}
		}
	}
	for _, occurrenceKey := range node.occurrences {
		occurrence := c.program.occurrences[occurrenceKey]
		if definition.AppliesTo != nil {
			if err := validateLabelConditionAgainstCatalog(
				"normalizations."+node.id+".applies_to.where",
				definition.AppliesTo.Where,
				occurrence.labels,
			); err != nil {
				return err
			}
		}
		switch definition.Kind {
		case "category":
			var source SourceLabel
			if definition.SourceLabel == semanticMetricNameField {
				var err error
				source, err = c.metricNameCategorySourceSchema(node, occurrence)
				if err != nil {
					return err
				}
			} else {
				var ok bool
				source, ok = occurrence.labels[definition.SourceLabel]
				if !ok {
					return fmt.Errorf("normalization %q source label %q is unavailable after dependency resolution",
						node.id, definition.SourceLabel)
				}
			}
			if _, exists := occurrence.labels[definition.TargetLabel]; exists {
				return fmt.Errorf("normalization %q target label %q exists before application", node.id, definition.TargetLabel)
			}
			for _, branch := range categoryCoverageBranches(definition, source) {
				branches[branch] = struct{}{}
			}
			output := categoryOutputSchema(definition, source)
			if definition.AppliesTo.Where != nil {
				output.Presence = LabelPresence{Kind: "optional"}
			}
			if err := validateSourceLabel(
				"normalizations."+node.id+".compiled_output",
				output,
				c.input.Contract.Source.Evidence,
			); err != nil {
				return err
			}
			occurrence.labels[definition.TargetLabel] = output
			if definition.AppliesTo.Where == nil && definition.SourceLabel != semanticMetricNameField {
				occurrence.dependencies = append(occurrence.dependencies, FunctionalDependency{
					Determinants: []string{definition.SourceLabel},
					Dependents:   []string{definition.TargetLabel},
				})
			}
		case "label_rename":
			source, ok := occurrence.labels[definition.SourceLabel]
			if !ok {
				return fmt.Errorf("normalization %q source label %q is unavailable after dependency resolution",
					node.id, definition.SourceLabel)
			}
			if _, exists := occurrence.labels[definition.TargetLabel]; exists {
				return fmt.Errorf("normalization %q target label %q exists before application", node.id, definition.TargetLabel)
			}
			branches["present"] = struct{}{}
			if source.Presence.keyMayBeAbsent() {
				branches["absent"] = struct{}{}
			}
			occurrence.labels[definition.TargetLabel] = source
			if !*definition.RetainSource {
				delete(occurrence.labels, definition.SourceLabel)
				renameDependencyLabel(occurrence.dependencies, definition.SourceLabel, definition.TargetLabel)
				if occurrence.labelRenames == nil {
					occurrence.labelRenames = make(map[string]string)
				}
				occurrence.labelRenames[definition.SourceLabel] = definition.TargetLabel
			} else {
				occurrence.dependencies = append(occurrence.dependencies,
					FunctionalDependency{
						Determinants: []string{definition.SourceLabel},
						Dependents:   []string{definition.TargetLabel},
					},
					FunctionalDependency{
						Determinants: []string{definition.TargetLabel},
						Dependents:   []string{definition.SourceLabel},
					},
				)
			}
		case "embedded_identity_repair":
			source, ok := occurrence.labels[definition.SourceIdentityLabel]
			if !ok {
				return fmt.Errorf("normalization %q reads unknown source identity label %q",
					node.id, definition.SourceIdentityLabel)
			}
			if c.occurrenceHasRawGrammarBranch(occurrence, "canonical") {
				branches["canonical"] = struct{}{}
			}
			if c.occurrenceHasRawGrammarBranch(occurrence, "embedded") {
				branches["embedded:identity_present"] = struct{}{}
				if source.Presence.keyMayBeAbsent() {
					branches["embedded:identity_absent"] = struct{}{}
				}
			}
			output, err := c.normalizedOutputLabel(node)
			if err != nil {
				return err
			}
			occurrence.labels[definition.Canonical.IdentityLabel] = output
		case "embedded_identity_extract":
			if c.occurrenceHasRawGrammarBranch(occurrence, "canonical") {
				branches["canonical"] = struct{}{}
			}
			if c.occurrenceHasRawGrammarBranch(occurrence, "embedded") {
				branches["embedded"] = struct{}{}
			}
			if _, exists := occurrence.labels[definition.TargetLabel]; exists {
				return fmt.Errorf("normalization %q target label %q exists before application", node.id, definition.TargetLabel)
			}
			output, err := c.normalizedOutputLabel(node)
			if err != nil {
				return err
			}
			occurrence.labels[definition.TargetLabel] = output
		case "finite_alias", "namespace_alias":
			// The complete compiled alias language is validated once above.
		case "generated_component_exclusion":
			branches["generated_member"] = struct{}{}
			occurrence.terminalExclusion = node.id
		default:
			panic("validated normalization kind has no schema application")
		}
	}
	node.coverageBranches = sortedMapKeys(branches)
	return nil
}

func (c *semanticCompiler) normalizedOutputLabel(node *compiledNormalization) (SourceLabel, error) {
	output := SourceLabel{
		Meaning:             node.definition.Output.Meaning,
		Presence:            LabelPresence{Kind: "optional"},
		Domain:              LabelDomain{Kind: "open"},
		EndpointCardinality: *node.definition.Output.EndpointCardinality,
		Stability:           node.definition.Output.Stability,
		Evidence:            slices.Clone(node.definition.Output.Evidence),
	}
	field := "normalizations." + node.id + ".compiled_output"
	if err := validateSourceLabel(field, output, c.input.Contract.Source.Evidence); err != nil {
		return SourceLabel{}, err
	}
	if err := validateSourceLabelEnvironment(field, output, c.input.Contract.Source); err != nil {
		return SourceLabel{}, err
	}
	if output.EndpointCardinality.Axis != "" {
		c.axisUses[output.EndpointCardinality.Axis]++
	}
	return output, nil
}

func (c *semanticCompiler) occurrenceHasRawGrammarBranch(
	occurrence *compiledOccurrence,
	branch string,
) bool {
	availability, ok := c.program.registrations[occurrence.registration].rawBranches[branch]
	return ok && occurrence.availability.overlaps(availability, c.environment.axes)
}

func (c *semanticCompiler) metricNameCategorySourceSchema(
	node *compiledNormalization,
	occurrence *compiledOccurrence,
) (SourceLabel, error) {
	registration := c.program.registrations[occurrence.registration]
	language, err := c.registrationLanguage(registration)
	if err != nil {
		return SourceLabel{}, err
	}
	if language.embedded != nil {
		return SourceLabel{}, fmt.Errorf(
			"normalization %q cannot derive a closed category from unbounded metric-name registration %q",
			node.id, occurrence.registration,
		)
	}
	values := make([]string, 0, len(language.exact))
	wireRole := c.program.signals[occurrence.signal].components[occurrence.component].source.WireRole
	for _, family := range language.exact {
		values = append(values, replayMetricForFamily(family, wireRole))
	}
	slices.Sort(values)
	values = slices.Compact(values)
	return SourceLabel{
		Meaning:             "Prometheus sample metric name.",
		Presence:            LabelPresence{Kind: "required"},
		Domain:              LabelDomain{Kind: "closed", Values: values},
		EndpointCardinality: EndpointCardinality{Kind: "closed_domain"},
		Stability:           "stable",
		Evidence:            slices.Clone(node.definition.Output.Evidence),
	}, nil
}

func categoryCoverageBranches(definition Normalization, source SourceLabel) []string {
	branches := make(map[string]struct{})
	if source.Presence.keyMayBeAbsent() {
		branches["missing"] = struct{}{}
	}
	if source.Domain.Kind == "closed" {
		for _, value := range source.Domain.Values {
			_, branch := replayCategoryValue(definition, value, true)
			branches[branch] = struct{}{}
		}
		return sortedMapKeys(branches)
	}
	for value := range definition.Exact {
		branches["exact:"+value] = struct{}{}
	}
	for _, valueRange := range definition.Ranges {
		branches[fmt.Sprintf("range:%d-%d", *valueRange.Min, *valueRange.Max)] = struct{}{}
	}
	branches["malformed"] = struct{}{}
	branches["unknown"] = struct{}{}
	return sortedMapKeys(branches)
}

func (c *semanticCompiler) validateFiniteAlias(node *compiledNormalization) error {
	used := make(map[string]struct{})
	for _, occurrenceKey := range node.occurrences {
		occurrence := c.program.occurrences[occurrenceKey]
		registration := c.program.registrations[occurrence.registration]
		language, err := c.registrationLanguage(registration)
		if err != nil {
			return err
		}
		if language.embedded != nil {
			return fmt.Errorf("normalization %q finite alias cannot cover embedded family registration %q",
				node.id, occurrence.registration)
		}
		for _, family := range language.exact {
			if _, ok := node.definition.SourceFamily[family]; !ok {
				return fmt.Errorf("normalization %q source_family does not cover active family %q", node.id, family)
			}
			used[family] = struct{}{}
		}
	}
	for family := range node.definition.SourceFamily {
		if _, ok := used[family]; !ok {
			return fmt.Errorf("normalization %q source_family key %q is unused", node.id, family)
		}
	}
	return nil
}

func categoryOutputSchema(definition Normalization, source SourceLabel) SourceLabel {
	values := make(map[string]struct{})
	for _, value := range definition.Exact {
		values[value] = struct{}{}
	}
	for _, value := range definition.Ranges {
		values[value.Value] = struct{}{}
	}
	for _, action := range []*CategoryAction{definition.Missing, definition.Malformed, definition.Unknown} {
		if action.Set != nil {
			values[*action.Set] = struct{}{}
		}
	}
	presence := categoryOutputPresence(definition, source)
	return SourceLabel{
		Meaning:             definition.Output.Meaning,
		Presence:            presence,
		Domain:              LabelDomain{Kind: "closed", Values: sortedMapKeys(values)},
		EndpointCardinality: EndpointCardinality{Kind: "closed_domain"},
		Stability:           source.Stability,
		Evidence:            slices.Clone(definition.Output.Evidence),
	}
}

func categoryOutputPresence(definition Normalization, source SourceLabel) LabelPresence {
	presentAlwaysSets := definition.Malformed.Set != nil && definition.Unknown.Set != nil
	if source.Domain.Kind == "closed" {
		presentAlwaysSets = true
		for _, value := range source.Domain.Values {
			if categoryValue(definition, value) == nil {
				presentAlwaysSets = false
				break
			}
		}
	}
	missingSets := definition.Missing.Set != nil
	if source.Presence.keyIsAlwaysPresent() && presentAlwaysSets || missingSets && presentAlwaysSets {
		return LabelPresence{Kind: "required"}
	}
	if presentAlwaysSets && !missingSets {
		return source.Presence
	}
	return LabelPresence{Kind: "optional"}
}

func categoryValue(definition Normalization, value string) *string {
	if target, ok := definition.Exact[value]; ok {
		return &target
	}
	parsed, ok := parseCanonicalUint64(value)
	if !ok {
		return definition.Malformed.Set
	}
	for _, valueRange := range definition.Ranges {
		if parsed >= *valueRange.Min && parsed <= *valueRange.Max {
			target := valueRange.Value
			return &target
		}
	}
	return definition.Unknown.Set
}

func parseCanonicalUint64(value string) (uint64, bool) {
	if value == "0" {
		return 0, true
	}
	if value == "" || value[0] < '1' || value[0] > '9' {
		return 0, false
	}
	var result uint64
	for index := range value {
		if value[index] < '0' || value[index] > '9' {
			return 0, false
		}
		digit := uint64(value[index] - '0')
		if result > (^uint64(0)-digit)/10 {
			return 0, false
		}
		result = result*10 + digit
	}
	return result, true
}

func validateLabelConditionAgainstCatalog(
	field string,
	condition *LabelCondition,
	labels map[string]SourceLabel,
) error {
	if condition == nil {
		return nil
	}
	for clauseIndex, clause := range condition.Any {
		for predicateIndex, predicate := range clause.All {
			predicateField := fmt.Sprintf("%s.any[%d].all[%d]", field, clauseIndex, predicateIndex)
			label, ok := labels[predicate.Label]
			if !ok {
				return fmt.Errorf("%s reads unknown label %q", predicateField, predicate.Label)
			}
			if label.Domain.Kind != "closed" {
				continue
			}
			allowed := make(map[string]struct{}, len(label.Domain.Values))
			for _, value := range label.Domain.Values {
				allowed[value] = struct{}{}
			}
			if predicate.Op == "eq" {
				if _, ok := allowed[*predicate.Value]; !ok {
					return fmt.Errorf("%s value %q is outside label %q closed domain",
						predicateField, *predicate.Value, predicate.Label)
				}
			}
			if predicate.Op == "in" {
				for _, value := range predicate.Values {
					if _, ok := allowed[value]; !ok {
						return fmt.Errorf("%s value %q is outside label %q closed domain",
							predicateField, value, predicate.Label)
					}
				}
			}
		}
	}
	return nil
}

func normalizationTargetLabel(definition Normalization) string {
	switch definition.Kind {
	case "category", "label_rename", "embedded_identity_extract":
		return definition.TargetLabel
	case "embedded_identity_repair":
		return definition.Canonical.IdentityLabel
	default:
		return ""
	}
}

func normalizationReadLabels(definition Normalization) []string {
	labels := make([]string, 0, 4)
	switch definition.Kind {
	case "category", "label_rename":
		labels = append(labels, definition.SourceLabel)
	case "embedded_identity_repair":
		labels = append(labels, definition.SourceIdentityLabel)
	}
	if definition.AppliesTo != nil && definition.AppliesTo.Where != nil {
		for _, clause := range definition.AppliesTo.Where.Any {
			for _, predicate := range clause.All {
				labels = append(labels, predicate.Label)
			}
		}
	}
	slices.Sort(labels)
	return slices.Compact(labels)
}

func normalizationWritesMetricName(kind string) bool {
	return kind == "finite_alias" || kind == "namespace_alias" ||
		kind == "embedded_identity_repair" || kind == "embedded_identity_extract"
}

func normalizationMayTerminate(kind string) bool {
	return kind == "embedded_identity_repair" || kind == "generated_component_exclusion"
}

func cloneFunctionalDependencies(values map[string]FunctionalDependency) []FunctionalDependency {
	result := make([]FunctionalDependency, 0, len(values))
	for _, id := range sortedMapKeys(values) {
		value := values[id]
		value.Determinants = slices.Clone(value.Determinants)
		value.Dependents = slices.Clone(value.Dependents)
		value.Evidence = slices.Clone(value.Evidence)
		result = append(result, value)
	}
	return result
}

func renameDependencyLabel(dependencies []FunctionalDependency, source, target string) {
	for index := range dependencies {
		for labelIndex := range dependencies[index].Determinants {
			if dependencies[index].Determinants[labelIndex] == source {
				dependencies[index].Determinants[labelIndex] = target
			}
		}
		for labelIndex := range dependencies[index].Dependents {
			if dependencies[index].Dependents[labelIndex] == source {
				dependencies[index].Dependents[labelIndex] = target
			}
		}
	}
}
