// SPDX-License-Identifier: GPL-3.0-or-later

package promproof

import (
	"context"
	"fmt"

	"github.com/netdata/netdata/go/plugins/internal/promprofile/semantics"
)

// CompiledBundle joins a strict proof descriptor with its compiled semantic
// contract before any fixture is replayed.
type CompiledBundle struct {
	Bundle  Bundle
	Program *promsemantics.CompiledSemanticContract
	Cases   map[string]CompiledCase
}

type CompiledCase struct {
	Definition ProofCase
	Semantics  *promsemantics.CompiledSemanticCase
}

func Compile(
	ctx context.Context,
	bundle Bundle,
	program *promsemantics.CompiledSemanticContract,
) (*CompiledBundle, error) {
	if ctx == nil {
		return nil, fmt.Errorf("compile proof: context is nil")
	}
	if err := bundle.validate(); err != nil {
		return nil, fmt.Errorf("compile proof: %w", err)
	}
	if program == nil {
		return nil, fmt.Errorf("compile proof: semantic contract is nil")
	}
	if program.Profile() != bundle.Descriptor.Profile {
		return nil, fmt.Errorf("compile proof: descriptor profile %q differs from semantic profile %q",
			bundle.Descriptor.Profile, program.Profile())
	}
	compiled := &CompiledBundle{
		Bundle:  bundle,
		Program: program,
		Cases:   make(map[string]CompiledCase, len(bundle.Descriptor.Cases)),
	}
	persistent := make(map[string]bool, len(bundle.Descriptor.Cases))
	for name, definition := range bundle.Descriptor.Cases {
		persistent[name] = len(definition.Steps) != 0
	}
	if err := program.ValidateLimitationProofCases(persistent); err != nil {
		return nil, fmt.Errorf("compile proof: %w", err)
	}
	for _, name := range sortedMapKeys(bundle.Descriptor.Cases) {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("compile proof case %q: %w", name, err)
		}
		definition := bundle.Descriptor.Cases[name]
		semanticCase, err := program.EvaluateCaseEnvironment(ctx, definition.Environment)
		if err != nil {
			return nil, fmt.Errorf("compile proof case %q: %w", name, err)
		}
		if err := compileCaseObservations(name, semanticCase, definition); err != nil {
			return nil, fmt.Errorf("compile proof case %q: %w", name, err)
		}
		compiled.Cases[name] = CompiledCase{Definition: definition, Semantics: semanticCase}
	}
	return compiled, nil
}

func compileCaseObservations(
	caseName string,
	semanticCase *promsemantics.CompiledSemanticCase,
	proofCase ProofCase,
) error {
	seen := make(map[string]struct{})
	for stepIndex, step := range proofCase.Steps {
		for _, target := range sortedMapKeys(step.Observations) {
			observation := step.Observations[target]
			if err := semanticCase.ValidateObservationTarget(target, observation.Limitation); err != nil {
				return fmt.Errorf("steps[%d].observations.%s: %w", stepIndex, target, err)
			}
			if observation.Limitation != "" {
				if err := semanticCase.ValidateLimitationProofSequence(target, caseName); err != nil {
					return fmt.Errorf("steps[%d].observations.%s: %w", stepIndex, target, err)
				}
			}
			_, established := seen[target]
			if !established {
				if observation.Predicates.Membership != "establish" ||
					observation.Predicates.Aggregate != "matches_reducer" ||
					observation.Predicates.Identity != "establish" {
					return fmt.Errorf("steps[%d].observations.%s first observation must establish membership, aggregate, and identity",
						stepIndex, target)
				}
				seen[target] = struct{}{}
				continue
			}
			if observation.Predicates.Membership == "establish" ||
				observation.Predicates.Aggregate == "matches_reducer" ||
				observation.Predicates.Identity == "establish" {
				return fmt.Errorf("steps[%d].observations.%s may establish predicates only on its first observation",
					stepIndex, target)
			}
		}
	}
	return nil
}
