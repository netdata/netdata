// SPDX-License-Identifier: GPL-3.0-or-later

package promsemantics

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
)

type SemanticCompileInput struct {
	Contract SemanticContract
	Supports map[string]*CompiledSemanticContract
}

type CompiledSemanticContract struct {
	profile             string
	header              compiledProfileHeader
	environment         compiledEnvironment
	registrations       map[string]*compiledRegistration
	signals             map[string]*compiledSignal
	occurrences         map[string]*compiledOccurrence
	normalizations      []*compiledNormalization
	views               map[string]*compiledView
	relationships       map[string]*compiledRelationship
	stateEncodings      map[string]*compiledStateEncoding
	exclusions          map[string]*compiledDesignExclusion
	limitations         map[string]*compiledCumulativeLimitation
	fallbacks           map[string]compiledFallbackClassification
	sourceIndex         compiledSourceIndex
	supports            map[string]*CompiledSemanticContract
	supportAvailability map[string]compiledEnvironmentCondition
}

type compiledProfileHeader struct {
	match     string
	app       string
	hasApp    bool
	namespace string
}

func (c *CompiledSemanticContract) Profile() string {
	if c == nil {
		return ""
	}
	return c.profile
}

type compiledSignal struct {
	id                       string
	availability             compiledEnvironmentCondition
	components               map[string]compiledComponent
	labels                   map[string]SourceLabel
	labelPresenceConstraints map[string]LabelPresenceConstraint
	registrations            []string
	occurrences              []string
	contributors             []compiledContributorVariant
}

type compiledContributorVariant struct {
	id           string
	definition   ContributorVariant
	availability compiledEnvironmentCondition
}

type compiledOccurrence struct {
	key               string
	signal            string
	component         string
	registration      string
	family            FamilySelector
	availability      compiledEnvironmentCondition
	sourceLabels      map[string]SourceLabel
	labels            map[string]SourceLabel
	dependencies      []FunctionalDependency
	labelRenames      map[string]string
	terminalExclusion string
}

type compiledNormalization struct {
	id               string
	definition       Normalization
	occurrences      []string
	familyAliases    map[string]string
	predecessors     map[string]struct{}
	coverageBranches []string
}

type compiledView struct {
	context         string
	definition      ViewDefinition
	entity          EntityDefinition
	labels          ViewLabels
	reduction       *ReductionDefinition
	inputs          map[string]*compiledViewInput
	destinationAxes map[string]EnvironmentAxis
	unit            string
	scale           rationalScale
	presentation    string
}

type compiledViewInput struct {
	id           string
	renderedRole string
	definition   ViewInput
	occurrences  []compiledViewOccurrence
}

type compiledViewOccurrence struct {
	sourceProfile           string
	program                 *CompiledSemanticContract
	occurrence              *compiledOccurrence
	component               compiledComponent
	algorithm               string
	destinationAvailability compiledEnvironmentCondition
}

type compiledRelationship struct {
	id           string
	definition   Relationship
	availability compiledEnvironmentCondition
}

type compiledCumulativeLimitation struct {
	target       string
	view         *compiledView
	input        *compiledViewInput
	variant      compiledContributorVariant
	definition   CumulativeLimitation
	availability compiledEnvironmentCondition
}

type compiledRegistration struct {
	key          string
	id           string
	group        string
	family       FamilySelector
	prometheus   PrometheusContract
	components   map[string]string
	availability compiledEnvironmentCondition
	rawBranches  map[string]compiledEnvironmentCondition
	owners       []compiledRegistrationOwner
}

type compiledRegistrationOwner struct {
	kind                string
	id                  string
	availability        compiledEnvironmentCondition
	componentByWireRole map[string]string
}

type semanticCompiler struct {
	input       SemanticCompileInput
	environment compiledEnvironment
	program     *CompiledSemanticContract
	policyUses  map[string]int
	axisUses    map[string]int
	registry    map[string]registryRegistrationReference
}

type registryRegistrationReference struct {
	group        string
	registration RegistryRegistration
}

func CompileSemanticContract(
	ctx context.Context,
	input SemanticCompileInput,
) (*CompiledSemanticContract, error) {
	if err := checkSemanticContext(ctx, "before static compilation"); err != nil {
		return nil, err
	}
	if err := input.Contract.validateIdentity(); err != nil {
		return nil, fmt.Errorf("compile semantic contract: %w", err)
	}
	environment := compileEnvironment(input.Contract.Source)
	header := compiledProfileHeader{
		match:     input.Contract.Design.Match,
		namespace: input.Contract.Design.Namespace,
	}
	if input.Contract.Design.App != nil {
		header.app = *input.Contract.Design.App
		header.hasApp = true
	}
	compiler := semanticCompiler{
		input:       input,
		environment: environment,
		policyUses:  make(map[string]int),
		axisUses:    make(map[string]int),
		registry:    make(map[string]registryRegistrationReference),
		program: &CompiledSemanticContract{
			profile:             input.Contract.Design.Profile,
			header:              header,
			environment:         environment,
			registrations:       make(map[string]*compiledRegistration),
			signals:             make(map[string]*compiledSignal),
			occurrences:         make(map[string]*compiledOccurrence),
			views:               make(map[string]*compiledView),
			relationships:       make(map[string]*compiledRelationship),
			stateEncodings:      make(map[string]*compiledStateEncoding),
			exclusions:          make(map[string]*compiledDesignExclusion),
			limitations:         make(map[string]*compiledCumulativeLimitation),
			fallbacks:           make(map[string]compiledFallbackClassification),
			supports:            make(map[string]*CompiledSemanticContract),
			supportAvailability: make(map[string]compiledEnvironmentCondition),
		},
	}
	if err := compiler.compileSupports(); err != nil {
		return nil, fmt.Errorf("compile semantic contract: %w", err)
	}
	if err := checkSemanticContext(ctx, "before source registration compilation"); err != nil {
		return nil, err
	}
	if err := compiler.compileRegistry(); err != nil {
		return nil, fmt.Errorf("compile semantic contract: %w", err)
	}
	if err := compiler.compileSignals(); err != nil {
		return nil, fmt.Errorf("compile semantic contract: %w", err)
	}
	if err := compiler.compileNormalizations(); err != nil {
		return nil, fmt.Errorf("compile semantic contract: %w", err)
	}
	if err := compiler.compileContributorVariants(); err != nil {
		return nil, fmt.Errorf("compile semantic contract: %w", err)
	}
	if err := compiler.compileRelationships(); err != nil {
		return nil, fmt.Errorf("compile semantic contract: %w", err)
	}
	if err := compiler.compileStateEncodings(); err != nil {
		return nil, fmt.Errorf("compile semantic contract: %w", err)
	}
	if err := compiler.compileViews(); err != nil {
		return nil, fmt.Errorf("compile semantic contract: %w", err)
	}
	if err := compiler.compileDesignExclusions(); err != nil {
		return nil, fmt.Errorf("compile semantic contract: %w", err)
	}
	if err := compiler.compileLimitations(); err != nil {
		return nil, fmt.Errorf("compile semantic contract: %w", err)
	}
	if err := compiler.compileSourceExclusions(); err != nil {
		return nil, fmt.Errorf("compile semantic contract: %w", err)
	}
	if err := compiler.compileRemainingConditionUses(); err != nil {
		return nil, fmt.Errorf("compile semantic contract: %w", err)
	}
	if err := compiler.validateReusablePolicyUsage(); err != nil {
		return nil, fmt.Errorf("compile semantic contract: %w", err)
	}
	if err := compiler.validateEvidenceClosure(); err != nil {
		return nil, fmt.Errorf("compile semantic contract: %w", err)
	}
	if err := compiler.validateGeneratedOwnership(); err != nil {
		return nil, fmt.Errorf("compile semantic contract: %w", err)
	}
	if err := compiler.compileFallbackClassifications(); err != nil {
		return nil, fmt.Errorf("compile semantic contract: %w", err)
	}
	if err := compiler.validateRegistrationLanguages(); err != nil {
		return nil, fmt.Errorf("compile semantic contract: %w", err)
	}
	if err := compiler.compileSourceIndex(); err != nil {
		return nil, fmt.Errorf("compile semantic contract: %w", err)
	}
	if err := compiler.validateEnvironmentUsage(); err != nil {
		return nil, fmt.Errorf("compile semantic contract: %w", err)
	}
	return compiler.program, nil
}

func (c *semanticCompiler) compileSupports() error {
	for _, id := range sortedMapKeys(c.input.Contract.Design.Composition.Supports) {
		dependency := c.input.Contract.Design.Composition.Supports[id]
		support, ok := c.input.Supports[id]
		if !ok || support == nil {
			return fmt.Errorf("composition support %q was not supplied", id)
		}
		if support.profile != id {
			return fmt.Errorf("composition support %q supplied profile %q", id, support.profile)
		}
		if id == c.program.profile || support.dependsOn(c.program.profile, make(map[string]struct{})) {
			return fmt.Errorf("composition support %q creates a self-reference", id)
		}
		availability, err := c.resolveCondition("composition.supports."+id+".when", dependency.When)
		if err != nil {
			return err
		}
		if len(availability.clauses) == 0 {
			return fmt.Errorf("composition support %q is unreachable", id)
		}
		c.program.supports[id] = support
		c.program.supportAvailability[id] = availability
	}
	for _, id := range sortedMapKeys(c.input.Supports) {
		if _, ok := c.input.Contract.Design.Composition.Supports[id]; !ok {
			return fmt.Errorf("undeclared composition support %q was supplied", id)
		}
	}
	return nil
}

func (c *CompiledSemanticContract) dependsOn(profile string, seen map[string]struct{}) bool {
	if c == nil {
		return false
	}
	if c.profile == profile {
		return true
	}
	if _, ok := seen[c.profile]; ok {
		return false
	}
	seen[c.profile] = struct{}{}
	for _, support := range c.supports {
		if support.dependsOn(profile, seen) {
			return true
		}
	}
	return false
}

func (c *semanticCompiler) compileRegistry() error {
	if c.input.Contract.Registry == nil {
		return nil
	}
	for groupID, group := range c.input.Contract.Registry.Registry.Groups {
		for registrationID, registration := range group.Registrations {
			c.registry[registrationID] = registryRegistrationReference{
				group:        groupID,
				registration: registration,
			}
			availability, err := c.resolveCondition(
				"source_registry.groups."+groupID+".registrations."+registrationID+".when",
				registration.When,
			)
			if err != nil {
				return err
			}
			rawBranches := make(map[string]compiledEnvironmentCondition, len(registration.RawBranches))
			for _, branch := range sortedMapKeys(registration.RawBranches) {
				condition, err := c.resolveCondition(
					"source_registry.groups."+groupID+".registrations."+registrationID+".raw_branches."+branch+".when",
					registration.RawBranches[branch].When,
				)
				if err != nil {
					return err
				}
				active := availability.and(condition, c.environment.axes)
				if len(active.clauses) == 0 {
					return fmt.Errorf("generated registration %q raw branch %q is unreachable", registrationID, branch)
				}
				rawBranches[branch] = active
			}
			if len(rawBranches) != 0 {
				covering := make([]compiledEnvironmentCondition, 0, len(rawBranches))
				for _, branch := range sortedMapKeys(rawBranches) {
					covering = append(covering, rawBranches[branch])
				}
				if !availability.coveredBy(c.environment.axes, covering...) {
					return fmt.Errorf("generated registration %q raw branches do not cover its availability", registrationID)
				}
			}
			key := generatedRegistrationKey(registrationID)
			c.program.registrations[key] = &compiledRegistration{
				key:          key,
				id:           registrationID,
				group:        groupID,
				family:       registration.Family,
				prometheus:   registration.Prometheus,
				components:   registryComponentRoles(registration.Components),
				availability: availability,
				rawBranches:  rawBranches,
			}
		}
	}
	return nil
}

func (c *semanticCompiler) compileSignals() error {
	for _, signalID := range sortedMapKeys(c.input.Contract.Source.Signals) {
		signal := c.input.Contract.Source.Signals[signalID]
		availability, err := c.resolveCondition("signals."+signalID+".availability", signal.Availability)
		if err != nil {
			return err
		}
		compiled := &compiledSignal{
			id:                       signalID,
			availability:             availability,
			labels:                   cloneSourceLabelMap(effectiveSignalLabels(signal, c.input.Contract.Source)),
			labelPresenceConstraints: cloneLabelPresenceConstraints(signal.LabelPresenceConstraints),
		}
		compiled.components, err = compileSignalComponents(
			"signals."+signalID+".components",
			effectiveSignalComponents(signal, c.input.Contract.Source),
		)
		if err != nil {
			return err
		}
		c.program.signals[signalID] = compiled

		if signal.Source.Inline != nil {
			if err := c.compileInlineRegistrations(signalID, signal, compiled); err != nil {
				return err
			}
		} else if err := c.compileGeneratedSignal(signalID, signal, compiled); err != nil {
			return err
		}
	}
	return nil
}

func (c *semanticCompiler) compileInlineRegistrations(
	signalID string,
	signal SignalDefinition,
	compiled *compiledSignal,
) error {
	components := effectiveSignalComponents(signal, c.input.Contract.Source)
	componentRoles := semanticComponentRoles(components)
	for _, registrationID := range sortedMapKeys(signal.Source.Inline.Registrations) {
		registration := signal.Source.Inline.Registrations[registrationID]
		registrationAvailability, err := c.resolveCondition(
			"signals."+signalID+".source.inline.registrations."+registrationID+".when",
			registration.When,
		)
		if err != nil {
			return err
		}
		availability := compiled.availability.and(registrationAvailability, c.environment.axes)
		if len(availability.clauses) == 0 {
			return fmt.Errorf("signal %q inline registration %q is unreachable", signalID, registrationID)
		}
		key := inlineRegistrationKey(signalID, registrationID)
		owner := compiledRegistrationOwner{
			kind:                "signal",
			id:                  signalID,
			availability:        availability,
			componentByWireRole: semanticComponentByWireRole(components),
		}
		c.program.registrations[key] = &compiledRegistration{
			key:          key,
			id:           registrationID,
			family:       registration.Family,
			prometheus:   registration.Prometheus,
			components:   cloneStringMap(componentRoles),
			availability: availability,
			owners:       []compiledRegistrationOwner{owner},
		}
		compiled.registrations = append(compiled.registrations, key)
	}
	return nil
}

func (c *semanticCompiler) compileGeneratedSignal(
	signalID string,
	signal SignalDefinition,
	compiled *compiledSignal,
) error {
	if c.input.Contract.Registry == nil {
		return fmt.Errorf("signal %q uses generated source without a registry", signalID)
	}
	selected, err := c.selectGeneratedRegistrations(signalID, *signal.Source.Generated)
	if err != nil {
		return err
	}
	componentRoles := semanticComponentRoles(effectiveSignalComponents(signal, c.input.Contract.Source))
	componentByWireRole := semanticComponentByWireRole(effectiveSignalComponents(signal, c.input.Contract.Source))
	for _, registrationID := range selected {
		registration := c.program.registrations[generatedRegistrationKey(registrationID)]
		if !sameWireRoleSet(registration.components, componentRoles) {
			return fmt.Errorf("signal %q components %v do not match generated registration %q components %v",
				signalID, sortedWireRoles(componentRoles), registrationID, sortedWireRoles(registration.components))
		}
		ownerAvailability := registration.availability.and(compiled.availability, c.environment.axes)
		if len(ownerAvailability.clauses) == 0 {
			return fmt.Errorf("signal %q can never own generated registration %q", signalID, registrationID)
		}
		registration.owners = append(registration.owners, compiledRegistrationOwner{
			kind:                "signal",
			id:                  signalID,
			availability:        ownerAvailability,
			componentByWireRole: cloneStringMap(componentByWireRole),
		})
		compiled.registrations = append(compiled.registrations, registration.key)
	}
	return nil
}

func (c *semanticCompiler) compileRemainingConditionUses() error {
	source := c.input.Contract.Source
	for signalID, signal := range source.Signals {
		labels := effectiveSignalLabels(signal, source)
		for labelID, label := range labels {
			if _, err := c.resolveCondition(
				"signals."+signalID+".labels."+labelID+".presence.when",
				label.Presence.When,
			); err != nil {
				return err
			}
			if label.EndpointCardinality.Axis != "" {
				c.axisUses[label.EndpointCardinality.Axis]++
			}
		}
		for dependencyID, dependency := range signal.FunctionalDependencies {
			if _, err := c.resolveCondition(
				"signals."+signalID+".functional_dependencies."+dependencyID+".when",
				dependency.When,
			); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *semanticCompiler) selectGeneratedRegistrations(
	signalID string,
	generated GeneratedSource,
) ([]string, error) {
	candidates := make(map[string]struct{})
	for _, groupID := range generated.RegistryGroups {
		group, ok := c.input.Contract.Registry.Registry.Groups[groupID]
		if !ok {
			return nil, fmt.Errorf("signal %q references unknown registry group %q", signalID, groupID)
		}
		for registrationID := range group.Registrations {
			candidates[registrationID] = struct{}{}
		}
	}
	selected := make(map[string]struct{})
	for _, registrationID := range generated.Scope.Registrations {
		if _, ok := candidates[registrationID]; !ok {
			return nil, fmt.Errorf("signal %q scope registration %q is outside its registry groups", signalID, registrationID)
		}
		selected[registrationID] = struct{}{}
	}
	for _, exact := range generated.Scope.Families.Exact {
		matched := false
		for registrationID := range candidates {
			if c.registry[registrationID].registration.Family.Exact == exact {
				selected[registrationID] = struct{}{}
				matched = true
			}
		}
		if !matched {
			return nil, fmt.Errorf("signal %q exact family scope %q matches no registration", signalID, exact)
		}
	}
	for _, grammar := range generated.Scope.Families.Grammars {
		matched := false
		for registrationID := range candidates {
			if c.registry[registrationID].registration.Family.Grammar == grammar {
				selected[registrationID] = struct{}{}
				matched = true
			}
		}
		if !matched {
			return nil, fmt.Errorf("signal %q grammar scope %q matches no registration", signalID, grammar)
		}
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("signal %q generated scope selects no registrations", signalID)
	}
	return sortedMapKeys(selected), nil
}

func (c *semanticCompiler) compileSourceExclusions() error {
	if len(c.input.Contract.Source.SourceExclusions) != 0 && c.input.Contract.Registry == nil {
		return fmt.Errorf("source exclusions require a generated source registry")
	}
	for _, exclusionID := range sortedMapKeys(c.input.Contract.Source.SourceExclusions) {
		exclusion := c.input.Contract.Source.SourceExclusions[exclusionID]
		exclusionAvailability, err := c.resolveCondition(
			"source_exclusions."+exclusionID+".when",
			exclusion.When,
		)
		if err != nil {
			return err
		}
		for _, registrationID := range exclusion.Registrations {
			registration, ok := c.program.registrations[generatedRegistrationKey(registrationID)]
			if !ok {
				return fmt.Errorf("source exclusion %q references unknown generated registration %q",
					exclusionID, registrationID)
			}
			availability := registration.availability.and(exclusionAvailability, c.environment.axes)
			if len(availability.clauses) == 0 {
				return fmt.Errorf("source exclusion %q can never own registration %q", exclusionID, registrationID)
			}
			registration.owners = append(registration.owners, compiledRegistrationOwner{
				kind:         "source_exclusion",
				id:           exclusionID,
				availability: availability,
			})
		}
	}
	return nil
}

func (c *semanticCompiler) validateGeneratedOwnership() error {
	for _, key := range sortedMapKeys(c.program.registrations) {
		registration := c.program.registrations[key]
		if registration.group == "" {
			continue
		}
		for left := 0; left < len(registration.owners); left++ {
			for right := left + 1; right < len(registration.owners); right++ {
				if registration.owners[left].availability.overlaps(
					registration.owners[right].availability,
					c.environment.axes,
				) {
					return fmt.Errorf("generated registration %q has overlapping owners %s %q and %s %q",
						registration.id,
						registration.owners[left].kind,
						registration.owners[left].id,
						registration.owners[right].kind,
						registration.owners[right].id,
					)
				}
			}
		}
		covering := make([]compiledEnvironmentCondition, 0, len(registration.owners))
		for _, owner := range registration.owners {
			covering = append(covering, owner.availability)
		}
		if !registration.availability.coveredBy(c.environment.axes, covering...) {
			return fmt.Errorf("generated registration %q is not owned in every active environment", registration.id)
		}
	}
	return nil
}

func (c *semanticCompiler) resolveCondition(
	field string,
	use ConditionUse,
) (compiledEnvironmentCondition, error) {
	condition, err := c.environment.resolve(use)
	if err != nil {
		return compiledEnvironmentCondition{}, fmt.Errorf("%s: %w", field, err)
	}
	if use.Policy != "" {
		c.policyUses[use.Policy]++
	}
	for _, clause := range condition.clauses {
		for _, predicate := range clause {
			c.axisUses[predicate.Axis]++
		}
	}
	return condition, nil
}

func (c *semanticCompiler) validateEnvironmentUsage() error {
	for policy := range c.environment.policies {
		if c.policyUses[policy] == 0 {
			return fmt.Errorf("environment policy %q is unused", policy)
		}
	}
	for axis := range c.environment.axes {
		if c.axisUses[axis] == 0 {
			return fmt.Errorf("environment axis %q is unused", axis)
		}
	}
	return nil
}

func (c *semanticCompiler) validateRegistrationLanguages() error {
	type exactRegistration struct {
		registration *compiledRegistration
		availability compiledEnvironmentCondition
	}
	exact := make(map[string][]exactRegistration)
	embedded := make([]compiledEmbeddedRegistration, 0)
	for _, registration := range c.program.registrations {
		variants, err := c.registrationLanguage(registration)
		if err != nil {
			return err
		}
		for _, family := range variants.exact {
			availability := registration.availability
			if variants.exactAvailability != nil {
				availability = variants.exactAvailability[family]
			}
			exact[family] = append(exact[family], exactRegistration{
				registration: registration,
				availability: availability,
			})
		}
		if variants.embedded != nil {
			embedded = append(embedded, *variants.embedded)
		}
	}
	for family, registrations := range exact {
		for left := range registrations {
			for right := left + 1; right < len(registrations); right++ {
				if registrations[left].availability.overlaps(registrations[right].availability, c.environment.axes) {
					return fmt.Errorf("source family %q has overlapping registrations %q and %q",
						family, registrations[left].registration.key, registrations[right].registration.key)
				}
			}
		}
	}
	slices.SortFunc(embedded, func(left, right compiledEmbeddedRegistration) int {
		if order := strings.Compare(left.form.Prefix, right.form.Prefix); order != 0 {
			return order
		}
		return strings.Compare(left.registration.key, right.registration.key)
	})
	for left := 0; left < len(embedded); left++ {
		for right := left + 1; right < len(embedded); right++ {
			if !strings.HasPrefix(embedded[right].form.Prefix, embedded[left].form.Prefix) {
				break
			}
			if !embeddedFormsOverlap(embedded[left].form, embedded[right].form) ||
				!embedded[left].availability.overlaps(
					embedded[right].availability,
					c.environment.axes,
				) {
				continue
			}
			if embedded[left].grammar == embedded[right].grammar &&
				embedded[left].formID != embedded[right].formID &&
				c.input.Contract.Registry.Registry.FamilyGrammars[embedded[left].grammar].Interpretation == "longest_known_suffix" {
				continue
			}
			return fmt.Errorf("embedded family languages for registrations %q and %q overlap",
				embedded[left].registration.key, embedded[right].registration.key)
		}
	}
	return nil
}

type compiledRegistrationLanguage struct {
	exact             []string
	exactAvailability map[string]compiledEnvironmentCondition
	embedded          *compiledEmbeddedRegistration
}

type compiledEmbeddedRegistration struct {
	registration *compiledRegistration
	grammar      string
	formID       string
	form         GrammarEmbedded
	availability compiledEnvironmentCondition
}

func (c *semanticCompiler) registrationLanguage(
	registration *compiledRegistration,
) (compiledRegistrationLanguage, error) {
	if registration.family.Exact != "" {
		return compiledRegistrationLanguage{exact: []string{registration.family.Exact}}, nil
	}
	if c.input.Contract.Registry == nil {
		return compiledRegistrationLanguage{}, fmt.Errorf("registration %q references a grammar without a registry", registration.key)
	}
	grammar := c.input.Contract.Registry.Registry.FamilyGrammars[registration.family.Grammar]
	form := grammar.Forms[registration.family.Form]
	if form.Exact != "" {
		return compiledRegistrationLanguage{exact: []string{form.Exact}}, nil
	}
	language := compiledRegistrationLanguage{exactAvailability: make(map[string]compiledEnvironmentCondition)}
	if availability, ok := registration.rawBranches["canonical"]; ok {
		family := form.Canonical.Prefix + form.Canonical.Suffix
		language.exact = []string{family}
		language.exactAvailability[family] = availability
	}
	if availability, ok := registration.rawBranches["embedded"]; ok {
		language.embedded = &compiledEmbeddedRegistration{
			registration: registration,
			grammar:      registration.family.Grammar,
			formID:       registration.family.Form,
			form:         *form.Embedded,
			availability: availability,
		}
	}
	return language, nil
}

func generatedRegistrationKey(registrationID string) string {
	return "generated/" + registrationID
}

func inlineRegistrationKey(signalID, registrationID string) string {
	return "inline/" + signalID + "/" + registrationID
}

func effectiveSignalComponents(
	signal SignalDefinition,
	document SourceSemanticsDocument,
) map[string]Component {
	if signal.ComponentPolicy != "" {
		return document.ComponentPolicies[signal.ComponentPolicy]
	}
	return signal.Components
}

func effectiveSignalLabels(
	signal SignalDefinition,
	document SourceSemanticsDocument,
) map[string]SourceLabel {
	if signal.LabelPolicy != "" {
		return document.LabelPolicies[signal.LabelPolicy]
	}
	return signal.Labels
}

func registryComponentRoles(components map[string]RegistryComponent) map[string]string {
	result := make(map[string]string, len(components))
	for id, component := range components {
		result[id] = component.WireRole
	}
	return result
}

func semanticComponentRoles(components map[string]Component) map[string]string {
	result := make(map[string]string, len(components))
	for id, component := range components {
		result[id] = component.WireRole
	}
	return result
}

func semanticComponentByWireRole(components map[string]Component) map[string]string {
	result := make(map[string]string, len(components))
	for id, component := range components {
		result[component.WireRole] = id
	}
	return result
}

func sameWireRoleSet(left, right map[string]string) bool {
	return slices.Equal(sortedWireRoles(left), sortedWireRoles(right))
}

func sortedWireRoles(components map[string]string) []string {
	roles := make([]string, 0, len(components))
	for _, role := range components {
		roles = append(roles, role)
	}
	slices.Sort(roles)
	return roles
}

func cloneStringMap(values map[string]string) map[string]string {
	result := make(map[string]string, len(values))
	maps.Copy(result, values)
	return result
}

func cloneSourceLabelMap(values map[string]SourceLabel) map[string]SourceLabel {
	result := make(map[string]SourceLabel, len(values))
	maps.Copy(result, values)
	return result
}
