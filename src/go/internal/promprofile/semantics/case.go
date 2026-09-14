// SPDX-License-Identifier: GPL-3.0-or-later

package promsemantics

import (
	"context"
	"fmt"
	"slices"
	"strings"
)

// CompiledSemanticCase is one fully assigned, realizable activation of a
// compiled candidate and its transitive supports.
type CompiledSemanticCase struct {
	root        *CompiledSemanticContract
	programs    map[string]*CompiledSemanticContract
	assignments map[string]map[string]AxisValue
}

// EvaluateCaseEnvironment validates a complete owner-keyed environment and
// derives the exact active support closure.
func (c *CompiledSemanticContract) EvaluateCaseEnvironment(
	ctx context.Context,
	environment map[string]map[string]AxisValue,
) (*CompiledSemanticCase, error) {
	if err := checkSemanticContext(ctx, "before proof environment evaluation"); err != nil {
		return nil, err
	}
	if c == nil {
		return nil, fmt.Errorf("proof environment: compiled semantic contract is nil")
	}
	if environment == nil {
		return nil, fmt.Errorf("proof environment: owner mapping is nil")
	}
	compiled := &CompiledSemanticCase{
		root:        c,
		programs:    make(map[string]*CompiledSemanticContract),
		assignments: make(map[string]map[string]AxisValue),
	}
	if err := compiled.activate(c, environment); err != nil {
		return nil, fmt.Errorf("proof environment: %w", err)
	}
	for _, profile := range sortedMapKeys(environment) {
		if _, ok := compiled.programs[profile]; !ok {
			return nil, fmt.Errorf("inactive or undeclared profile %q has an environment assignment", profile)
		}
	}
	return compiled, nil
}

func (c *CompiledSemanticCase) activate(
	program *CompiledSemanticContract,
	environment map[string]map[string]AxisValue,
) error {
	if existing, ok := c.programs[program.profile]; ok {
		if existing != program {
			return fmt.Errorf("profile %q resolves to more than one compiled contract", program.profile)
		}
		return nil
	}
	assignment, ok := environment[program.profile]
	if !ok {
		return fmt.Errorf("active profile %q has no environment assignment", program.profile)
	}
	if err := validateCompleteAssignment(program, assignment); err != nil {
		return err
	}
	c.programs[program.profile] = program
	c.assignments[program.profile] = cloneAxisAssignment(assignment)

	for _, support := range sortedMapKeys(program.supports) {
		if !program.supportAvailability[support].evaluate(program.environment.axes, assignment) {
			continue
		}
		if err := c.activate(program.supports[support], environment); err != nil {
			return err
		}
	}
	return nil
}

func validateCompleteAssignment(program *CompiledSemanticContract, assignment map[string]AxisValue) error {
	if assignment == nil {
		return fmt.Errorf("active profile %q environment must be a mapping", program.profile)
	}
	for _, axis := range sortedMapKeys(program.environment.axes) {
		value, ok := assignment[axis]
		if !ok {
			return fmt.Errorf("active profile %q environment is missing axis %q", program.profile, axis)
		}
		if err := validateAxisValue("profile "+program.profile+" environment."+axis, value, program.environment.axes[axis]); err != nil {
			return err
		}
	}
	for _, axis := range sortedMapKeys(assignment) {
		if _, ok := program.environment.axes[axis]; !ok {
			return fmt.Errorf("active profile %q environment has unknown axis %q", program.profile, axis)
		}
	}
	return nil
}

// ActiveProfiles returns the deterministic active profile closure.
func (c *CompiledSemanticCase) ActiveProfiles() []string {
	if c == nil {
		return nil
	}
	return sortedMapKeys(c.programs)
}

// ValidateObservationTarget requires a semantically active candidate view/input
// and, when present, its exact active limitation.
func (c *CompiledSemanticCase) ValidateObservationTarget(target, limitation string) error {
	if c == nil || c.root == nil {
		return fmt.Errorf("compiled semantic case is nil")
	}
	viewID, inputID, ok := strings.Cut(target, "#")
	if !ok || viewID == "" || inputID == "" || strings.Contains(inputID, "#") {
		return fmt.Errorf("observation target %q must be <view>#<input>", target)
	}
	view, ok := c.root.views[viewID]
	if !ok {
		return fmt.Errorf("observation target %q references unknown candidate view %q", target, viewID)
	}
	input, ok := view.inputs[inputID]
	if !ok {
		return fmt.Errorf("observation target %q references unknown input %q", target, inputID)
	}
	if !c.inputActive(view, input) {
		return fmt.Errorf("observation target %q is inactive in the case environment", target)
	}
	if limitation == "" {
		return nil
	}
	if limitation != target {
		return fmt.Errorf("observation target %q cannot use limitation %q", target, limitation)
	}
	compiled, ok := c.root.limitations[limitation]
	if !ok {
		return fmt.Errorf("observation target %q references unknown limitation %q", target, limitation)
	}
	if !compiled.availability.evaluate(c.root.environment.axes, c.assignments[c.root.profile]) {
		return fmt.Errorf("observation target %q limitation %q is inactive in the case environment", target, limitation)
	}
	return nil
}

// ValidateLimitationProofSequence binds a limitation observation to its one
// authored persistent proof case without exposing compiled limitation bodies.
func (c *CompiledSemanticCase) ValidateLimitationProofSequence(target, sequence string) error {
	if c == nil || c.root == nil {
		return fmt.Errorf("compiled semantic case is nil")
	}
	limitation := c.root.limitations[target]
	if limitation == nil {
		return fmt.Errorf("observation target %q has no declared limitation", target)
	}
	if limitation.definition.ProofSequence != sequence {
		return fmt.Errorf("observation target %q limitation requires proof sequence %q, got %q",
			target, limitation.definition.ProofSequence, sequence)
	}
	return nil
}

// ValidateLimitationProofCases requires every declared sequence to resolve to
// one persistent proof case before fixture replay begins.
func (c *CompiledSemanticContract) ValidateLimitationProofCases(persistent map[string]bool) error {
	if c == nil {
		return fmt.Errorf("compiled semantic contract is nil")
	}
	for _, target := range sortedMapKeys(c.limitations) {
		sequence := c.limitations[target].definition.ProofSequence
		isPersistent, ok := persistent[sequence]
		if !ok {
			return fmt.Errorf("limitation %q references missing proof sequence %q", target, sequence)
		}
		if !isPersistent {
			return fmt.Errorf("limitation %q proof sequence %q is not persistent", target, sequence)
		}
	}
	return nil
}

func (c *CompiledSemanticCase) inputActive(view *compiledView, input *compiledViewInput) bool {
	for _, occurrence := range input.occurrences {
		sourceAssignment, ok := c.assignments[occurrence.sourceProfile]
		if !ok {
			continue
		}
		if !occurrence.occurrence.availability.evaluate(occurrence.program.environment.axes, sourceAssignment) {
			continue
		}
		destinationAssignment := c.assignments[c.root.profile]
		if occurrence.destinationAvailability.evaluate(view.destinationAxes, destinationAssignment) {
			return true
		}
	}
	return false
}

func cloneAxisAssignment(values map[string]AxisValue) map[string]AxisValue {
	cloned := make(map[string]AxisValue, len(values))
	for key, value := range values {
		copyValue := AxisValue{Strings: slices.Clone(value.Strings)}
		if value.String != nil {
			item := *value.String
			copyValue.String = &item
		}
		if value.Integer != nil {
			item := *value.Integer
			copyValue.Integer = &item
		}
		cloned[key] = copyValue
	}
	return cloned
}
