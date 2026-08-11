// SPDX-License-Identifier: GPL-3.0-or-later

package promsemantics

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/netdata/netdata/go/plugins/internal/promprofile/replay"
)

// ProductionCoverage retains only declaration-bounded coverage state. It
// never retains replay snapshots, source occurrences, or open-domain values.
type ProductionCoverage struct {
	root                  *CompiledSemanticContract
	required              map[string]struct{}
	seen                  map[string]struct{}
	conditions            []productionCoverageCondition
	productionChartOwners map[string]string
	productionDimOwners   map[string]string
	productionAutogenDeny []string
	autogenDenyObserved   bool
}

type productionCoverageCondition struct {
	name         string
	condition    compiledEnvironmentCondition
	requireFalse bool
}

// NewProductionCoverage builds the immutable coverage obligations for one
// candidate proof. Support profiles prove their local declarations in their own
// proof; support-owned occurrences consumed by candidate views remain candidate
// edge obligations.
func NewProductionCoverage(root *CompiledSemanticContract) (*ProductionCoverage, error) {
	if root == nil {
		return nil, fmt.Errorf("production coverage: compiled semantic contract is nil")
	}
	coverage := &ProductionCoverage{
		root:                  root,
		required:              make(map[string]struct{}),
		seen:                  make(map[string]struct{}),
		productionChartOwners: make(map[string]string),
		productionDimOwners:   make(map[string]string),
	}
	coverage.compileObligations()
	return coverage, nil
}

func (c *ProductionCoverage) compileObligations() {
	root := c.root
	for _, support := range sortedMapKeys(root.supportAvailability) {
		c.addCondition("support "+support, root.supportAvailability[support])
	}
	for _, key := range sortedMapKeys(root.registrations) {
		registration := root.registrations[key]
		c.require("registration " + key)
		c.addCondition("registration "+key, registration.availability)
		for _, branch := range sortedMapKeys(registration.rawBranches) {
			prefix := "registration " + key + " raw branch " + branch
			c.require(prefix)
			c.addCondition(prefix, registration.rawBranches[branch])
		}
		for _, owner := range registration.owners {
			c.addCondition("registration owner "+key+" "+owner.kind+" "+owner.id, owner.availability)
		}
	}
	for _, signalID := range sortedMapKeys(root.signals) {
		signal := root.signals[signalID]
		c.addCondition("signal "+signalID, signal.availability)
		for _, constraintID := range sortedMapKeys(signal.labelPresenceConstraints) {
			constraint := signal.labelPresenceConstraints[constraintID]
			for index := range constraint.Alternatives {
				c.require(labelPresenceConstraintCoverageKey(signalID, constraintID, index))
			}
		}
		for _, variant := range signal.contributors {
			c.addCondition("contributor "+signalID+" "+variant.id, variant.availability)
		}
	}
	for _, occurrenceKey := range sortedMapKeys(root.occurrences) {
		occurrence := root.occurrences[occurrenceKey]
		c.require("source occurrence " + occurrenceKey)
		c.addCondition("source occurrence "+occurrenceKey, occurrence.availability)
		for _, label := range sortedMapKeys(occurrence.sourceLabels) {
			schema := occurrence.sourceLabels[label]
			prefix := labelCoveragePrefix(occurrence, label)
			switch schema.Presence.Kind {
			case "required":
				c.require(prefix + " present")
			case "present":
				c.require(prefix + " present")
			case "optional":
				c.require(prefix + " present")
				c.require(prefix + " absent")
			default:
				condition, err := root.environment.resolve(schema.Presence.When)
				if err == nil {
					if len(occurrence.availability.and(condition, root.environment.axes).clauses) != 0 {
						c.require(prefix + " present")
					}
					if !occurrence.availability.coveredBy(root.environment.axes, condition) {
						c.require(prefix + " absent")
					}
					c.addCondition(prefix+" conditional presence", condition)
				}
			}
			if schema.Domain.Kind == "closed" {
				for _, value := range schema.Domain.Values {
					c.require(prefix + " value " + value)
				}
			}
		}
	}
	for _, node := range root.normalizations {
		for _, branch := range node.coverageBranches {
			c.require("normalization " + node.id + " branch " + branch)
		}
	}
	for _, viewID := range sortedMapKeys(root.views) {
		view := root.views[viewID]
		for _, inputID := range sortedMapKeys(view.inputs) {
			input := view.inputs[inputID]
			for _, source := range input.occurrences {
				prefix := viewInputCoveragePrefix(viewID, inputID, source)
				c.require(prefix)
				c.require(renderingCoverageKey(prefix, view, source))
				if input.definition.Where != nil {
					c.require(prefix + " where match")
					c.require(prefix + " where nonmatch")
				}
			}
		}
		if view.reduction != nil {
			c.require("reducer collision " + viewID)
		}
	}
	for _, id := range sortedMapKeys(root.relationships) {
		c.require("relationship " + id)
		c.addCondition("relationship "+id, root.relationships[id].availability)
	}
	for _, id := range sortedMapKeys(root.stateEncodings) {
		c.require("state encoding " + id)
		c.addCondition("state encoding "+id, root.stateEncodings[id].availability)
	}
	for _, id := range sortedMapKeys(root.exclusions) {
		exclusion := root.exclusions[id]
		c.require("exclusion " + id + " outcome " + exclusion.definition.Outcome)
		c.addCondition("exclusion "+id, exclusion.availability)
	}
	for _, target := range sortedMapKeys(root.limitations) {
		limitation := root.limitations[target]
		c.require("limitation " + target + " sequence " + limitation.definition.ProofSequence)
		c.addCondition("limitation "+target, limitation.availability)
	}
}

func (c *ProductionCoverage) addCondition(name string, condition compiledEnvironmentCondition) {
	entry := productionCoverageCondition{
		name:         name,
		condition:    condition,
		requireFalse: !unconditionalEnvironmentCondition().coveredBy(c.root.environment.axes, condition),
	}
	c.conditions = append(c.conditions, entry)
	c.require("activation " + name + " true")
	if entry.requireFalse {
		c.require("activation " + name + " false")
	}
}

func (c *ProductionCoverage) require(key string) {
	c.required[key] = struct{}{}
}

func (c *ProductionCoverage) mark(key string) {
	if _, ok := c.required[key]; ok {
		c.seen[key] = struct{}{}
	}
}

// ObserveCase consumes one already reconciled coverage snapshot.
func (c *ProductionCoverage) ObserveCase(
	ctx context.Context,
	semanticCase *CompiledSemanticCase,
	snapshot *promreplay.SemanticSnapshot,
	reconciled *ReconciledSemanticCase,
) error {
	if err := checkSemanticContext(ctx, "before production coverage observation"); err != nil {
		return err
	}
	if c == nil || c.root == nil || semanticCase == nil || snapshot == nil || reconciled == nil {
		return fmt.Errorf("production coverage: coverage, semantic case, snapshot, and reconciliation must be present")
	}
	if semanticCase.root != c.root {
		return fmt.Errorf("production coverage: semantic case belongs to profile %q, want %q",
			semanticCase.root.profile, c.root.profile)
	}
	assignment := semanticCase.assignments[c.root.profile]
	for _, condition := range c.conditions {
		active := condition.condition.evaluate(c.root.environment.axes, assignment)
		c.mark(fmt.Sprintf("activation %s %t", condition.name, active))
	}
	for _, registration := range c.root.registrations {
		for _, owner := range registration.owners {
			if owner.kind == "source_exclusion" && owner.availability.evaluate(c.root.environment.axes, assignment) {
				c.mark("registration " + registration.key)
				for _, branch := range sortedMapKeys(registration.rawBranches) {
					if registration.rawBranches[branch].evaluate(c.root.environment.axes, assignment) {
						c.mark("registration " + registration.key + " raw branch " + branch)
					}
				}
			}
		}
	}
	for _, binding := range reconciled.Sources {
		if binding.Profile != c.root.profile || binding.occurrence == nil || binding.SourceIndex < 0 ||
			binding.SourceIndex >= len(snapshot.Sources) {
			continue
		}
		c.mark("registration " + binding.Registration)
		if binding.entry.rawBranch == "canonical" || binding.entry.rawBranch == "embedded" {
			c.mark("registration " + binding.Registration + " raw branch " + binding.entry.rawBranch)
		}
		c.mark("source occurrence " + binding.occurrence.key)
		labels := semanticLabelMap(snapshot.Sources[binding.SourceIndex].Labels)
		signal := c.root.signals[binding.occurrence.signal]
		for _, constraintID := range sortedMapKeys(signal.labelPresenceConstraints) {
			constraint := signal.labelPresenceConstraints[constraintID]
			selected, err := selectedLabelAlternative(constraint.Alternatives, labels)
			if err != nil {
				return fmt.Errorf("coverage source %d label presence constraint %q: %w",
					binding.SourceIndex, constraintID, err)
			}
			c.mark(labelPresenceConstraintCoverageKey(binding.occurrence.signal, constraintID, selected))
		}
		for _, label := range sortedMapKeys(binding.occurrence.sourceLabels) {
			schema := binding.occurrence.sourceLabels[label]
			prefix := labelCoveragePrefix(binding.occurrence, label)
			value, present := labels[label]
			if present {
				c.mark(prefix + " present")
				if schema.Domain.Kind == "closed" {
					c.mark(prefix + " value " + value)
				}
			} else {
				c.mark(prefix + " absent")
			}
		}
	}
	for _, profile := range snapshot.Profiles {
		if profile.Name != c.root.profile {
			continue
		}
		denies := slices.Clone(profile.AutogenSelectorDeny)
		slices.Sort(denies)
		if !c.autogenDenyObserved {
			c.productionAutogenDeny = denies
			c.autogenDenyObserved = true
		} else if !slices.Equal(c.productionAutogenDeny, denies) {
			return fmt.Errorf("production profile %q autogen denies changed across proof cases: got %v, want %v",
				c.root.profile, denies, c.productionAutogenDeny)
		}
		for _, family := range denies {
			c.require("autogen deny " + family)
		}
		for _, chart := range profile.Charts {
			c.require("production chart " + chart.RuntimePath)
			for _, dimension := range chart.Dimensions {
				c.require(fmt.Sprintf("production dimension %s index %d", chart.RuntimePath, dimension.Index))
			}
		}
	}
	for _, fact := range reconciled.Normalizations {
		if fact.Profile == c.root.profile {
			c.mark("normalization " + fact.Normalization + " branch " + fact.Branch)
		}
	}
	c.observeViewCoverage(semanticCase, snapshot, reconciled)
	for _, fact := range reconciled.Exclusions {
		if fact.Profile == c.root.profile {
			c.mark("exclusion " + fact.Exclusion + " outcome " + fact.Outcome)
			if fact.Outcome == "retain_writable_unrendered" && fact.AutogenFamily != "" {
				c.mark("autogen deny " + fact.AutogenFamily)
			}
		}
	}
	for _, claim := range reconciled.Claims {
		if claim.Profile != c.root.profile {
			continue
		}
		switch claim.Kind {
		case "relationship":
			c.mark("relationship " + claim.ID)
		case "state_encoding":
			c.mark("state encoding " + claim.ID)
		}
	}
	return nil
}

func (c *ProductionCoverage) observeViewCoverage(
	semanticCase *CompiledSemanticCase,
	snapshot *promreplay.SemanticSnapshot,
	reconciled *ReconciledSemanticCase,
) {
	for _, viewID := range sortedMapKeys(c.root.views) {
		view := c.root.views[viewID]
		for _, inputID := range sortedMapKeys(view.inputs) {
			input := view.inputs[inputID]
			if input.definition.Where == nil {
				continue
			}
			for _, source := range input.occurrences {
				if !semanticCase.viewOccurrenceActive(c.root, view, source) {
					continue
				}
				prefix := viewInputCoveragePrefix(viewID, inputID, source)
				for _, binding := range reconciled.Sources {
					if binding.program != source.program || binding.occurrence != source.occurrence ||
						binding.SourceIndex < 0 || binding.SourceIndex >= len(snapshot.Sources) {
						continue
					}
					labels := semanticPipelineLabelMap(snapshot.Sources[binding.SourceIndex].FinalLabels)
					if replayLabelConditionMatches(input.definition.Where, labels) {
						c.mark(prefix + " where match")
					} else {
						c.mark(prefix + " where nonmatch")
					}
				}
			}
		}
	}
	for _, edge := range reconciled.Edges {
		if edge.DestinationProfile != c.root.profile || edge.SourceIndex < 0 || edge.SourceIndex >= len(reconciled.Sources) {
			continue
		}
		binding := reconciled.Sources[edge.SourceIndex]
		view := c.root.views[edge.View]
		input := view.inputs[edge.Input]
		if binding.occurrence == nil || input == nil {
			continue
		}
		for _, source := range input.occurrences {
			if source.program != binding.program || source.occurrence != binding.occurrence {
				continue
			}
			prefix := viewInputCoveragePrefix(edge.View, edge.Input, source)
			c.mark(prefix)
			c.mark(renderingCoverageKey(prefix, view, source))
		}
		chartKey := edge.TemplatePath
		if previous := c.productionChartOwners[chartKey]; previous == "" {
			c.productionChartOwners[chartKey] = edge.View
		} else if previous != edge.View {
			// Reconciliation already prevents this within one snapshot. Retain
			// the cross-case invariant without retaining the snapshots.
			c.require("production chart owner conflict " + chartKey + " " + previous + " " + edge.View)
		}
		c.mark("production chart " + chartKey)
		dimensionKey := fmt.Sprintf("%s index %d", chartKey, edge.DimensionIndex)
		owner := edge.View + "#" + edge.Input
		if previous := c.productionDimOwners[dimensionKey]; previous == "" {
			c.productionDimOwners[dimensionKey] = owner
		} else if previous != owner {
			c.require("production dimension owner conflict " + dimensionKey + " " + previous + " " + owner)
		}
		c.mark("production dimension " + dimensionKey)
	}
	c.observeReducerCoverage(snapshot, reconciled)
}

func (c *ProductionCoverage) observeReducerCoverage(
	snapshot *promreplay.SemanticSnapshot,
	reconciled *ReconciledSemanticCase,
) {
	type reducerGroup struct {
		view   string
		values map[uint64]struct{}
		seen   map[int]struct{}
	}
	groups := make(map[string]*reducerGroup)
	for _, edge := range reconciled.Edges {
		if edge.DestinationProfile != c.root.profile || c.root.views[edge.View].reduction == nil ||
			edge.SourceIndex < 0 || edge.SourceIndex >= len(snapshot.Sources) {
			continue
		}
		key := edge.View + "\x00" + edge.ChartID + "\x00" + edge.DimensionName
		group := groups[key]
		if group == nil {
			group = &reducerGroup{view: edge.View, values: make(map[uint64]struct{}), seen: make(map[int]struct{})}
			groups[key] = group
		}
		value := snapshot.Sources[edge.SourceIndex].Value
		if !math.IsNaN(value) {
			group.values[math.Float64bits(value)] = struct{}{}
		}
		group.seen[edge.SourceIndex] = struct{}{}
	}
	for _, group := range groups {
		if len(group.seen) >= 2 && len(group.values) >= 2 {
			c.mark("reducer collision " + group.view)
		}
	}
}

// ObserveLimitation marks only the declared persistent sequence and the
// disappearance/reset-gap transition that the one supported limitation owns.
func (c *ProductionCoverage) ObserveLimitation(
	caseName, target, membership, aggregate string,
) {
	limitation := c.root.limitations[target]
	if limitation == nil || limitation.definition.ProofSequence != caseName {
		return
	}
	if (membership == "removed" || membership == "replaced") &&
		(aggregate == "decreased" || aggregate == "became_gap") {
		c.mark("limitation " + target + " sequence " + caseName)
	}
}

// Verify reports the complete deterministic set of unexercised semantic
// obligations after all coverage-enabled cases have been discarded.
func (c *ProductionCoverage) Verify(ctx context.Context) error {
	if err := checkSemanticContext(ctx, "before production coverage verification"); err != nil {
		return err
	}
	if c == nil || c.root == nil {
		return fmt.Errorf("production coverage: coverage is nil")
	}
	missing := make([]string, 0)
	for key := range c.required {
		if _, ok := c.seen[key]; !ok {
			missing = append(missing, key)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	slices.Sort(missing)
	return fmt.Errorf("production semantic coverage is incomplete for profile %q: %s",
		c.root.profile, strings.Join(missing, "; "))
}

func labelCoveragePrefix(occurrence *compiledOccurrence, label string) string {
	return "label " + occurrence.signal + "/" + occurrence.component + "/" + label
}

func labelPresenceConstraintCoverageKey(signal, constraint string, alternative int) string {
	return fmt.Sprintf("label presence constraint %s/%s alternative %d", signal, constraint, alternative)
}

func viewInputCoveragePrefix(viewID, inputID string, source compiledViewOccurrence) string {
	return "view " + viewID + " input " + inputID + " source " +
		source.sourceProfile + "/" + source.occurrence.key
}

func renderingCoverageKey(prefix string, view *compiledView, source compiledViewOccurrence) string {
	if structural := structuralLabelForComponent(source.component.source.WireRole); structural != "" {
		return prefix + " rendering structural:" + structural
	}
	for _, label := range sortedMapKeys(view.labels.Dimensions) {
		if _, ok := source.occurrence.labels[label]; ok {
			return prefix + " rendering " + view.labels.Dimensions[label].Render + ":" + label
		}
	}
	return prefix + " rendering input_role:static"
}
