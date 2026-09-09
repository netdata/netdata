// SPDX-License-Identifier: GPL-3.0-or-later

package promsemantics

import (
	"fmt"
	"slices"
	"strings"
)

func (c *semanticCompiler) compileLimitations() error {
	for _, target := range sortedMapKeys(c.input.Contract.Design.Limitations) {
		definition := c.input.Contract.Design.Limitations[target]
		viewID, inputID, ok := parseViewInputTarget(target)
		if !ok {
			return fmt.Errorf("limitation target %q must be <view>#<input>", target)
		}
		view := c.program.views[viewID]
		if view == nil {
			return fmt.Errorf("limitation target %q references unknown view %q", target, viewID)
		}
		input := view.inputs[inputID]
		if input == nil {
			return fmt.Errorf("limitation target %q references unknown input %q", target, inputID)
		}
		program, signalID, err := c.resolveViewSignal(input.definition.Signal)
		if err != nil {
			return fmt.Errorf("limitation target %q: %w", target, err)
		}
		if program != c.program {
			return fmt.Errorf("limitation target %q cannot bind a support-owned signal", target)
		}
		signal := program.signals[signalID]
		variant, ok := contributorVariantByID(signal, definition.ContributorVariant)
		if !ok {
			return fmt.Errorf("limitation target %q references unknown contributor variant %q",
				target, definition.ContributorVariant)
		}
		availability, err := c.resolveCondition("limitations."+target+".when", definition.When)
		if err != nil {
			return err
		}
		if err := validateLimitationTarget(target, view, input, variant, availability); err != nil {
			return err
		}
		c.program.limitations[target] = &compiledCumulativeLimitation{
			target:       target,
			view:         view,
			input:        input,
			variant:      variant,
			definition:   definition,
			availability: availability,
		}
	}
	return c.validateCumulativeReductionLimitations()
}

func parseViewInputTarget(target string) (string, string, bool) {
	viewID, inputID, ok := strings.Cut(target, "#")
	return viewID, inputID, ok && viewID != "" && inputID != "" && !strings.Contains(inputID, "#")
}

func contributorVariantByID(signal *compiledSignal, id string) (compiledContributorVariant, bool) {
	for _, variant := range signal.contributors {
		if variant.id == id {
			return variant, true
		}
	}
	return compiledContributorVariant{}, false
}

func validateLimitationTarget(
	target string,
	view *compiledView,
	input *compiledViewInput,
	variant compiledContributorVariant,
	availability compiledEnvironmentCondition,
) error {
	if view.reduction == nil || view.reduction.Reducer != "sum" {
		return fmt.Errorf("limitation target %q requires sum reduction", target)
	}
	if variant.definition.Membership.Stability != "dynamic" {
		return fmt.Errorf("limitation target %q contributor variant %q is not dynamic", target, variant.id)
	}
	if variant.definition.Concurrency != "may_coexist" {
		return fmt.Errorf("limitation target %q contributor variant %q cannot coexist", target, variant.id)
	}
	if variant.definition.Reset.Scope != "per_contributor" {
		return fmt.Errorf("limitation target %q contributor variant %q does not reset independently", target, variant.id)
	}
	matched := false
	for _, source := range input.occurrences {
		if source.component.source.Lifecycle.Kind != "cumulative" {
			return fmt.Errorf("limitation target %q includes noncumulative component %s/%s",
				target, source.occurrence.signal, source.occurrence.component)
		}
		if variant.definition.ValueModel[source.occurrence.component] != "additive" {
			return fmt.Errorf("limitation target %q component %s/%s is not additive for contributor variant %q",
				target, source.occurrence.signal, source.occurrence.component, variant.id)
		}
		dynamic := source.occurrence.availability.and(variant.availability, source.program.environment.axes)
		limited := source.occurrence.availability.and(availability, source.program.environment.axes)
		if len(dynamic.clauses) == 0 && len(limited.clauses) == 0 {
			continue
		}
		matched = true
		if !dynamic.coveredBy(source.program.environment.axes, availability) ||
			!limited.coveredBy(source.program.environment.axes, variant.availability) {
			return fmt.Errorf("limitation target %q availability does not exactly select contributor variant %q",
				target, variant.id)
		}
	}
	if !matched {
		return fmt.Errorf("limitation target %q is inactive", target)
	}
	return nil
}

func (c *semanticCompiler) validateCumulativeReductionLimitations() error {
	required := make(map[string]string)
	for _, viewID := range sortedMapKeys(c.program.views) {
		view := c.program.views[viewID]
		if view.reduction == nil || view.reduction.Reducer != "sum" {
			continue
		}
		for _, cause := range viewFanInCauses(view) {
			if cause.label == "" || cause.source.component.source.Lifecycle.Kind != "cumulative" {
				continue
			}
			signal := cause.source.program.signals[cause.source.occurrence.signal]
			for _, variant := range signal.contributors {
				if !variant.availability.overlaps(
					cause.source.occurrence.availability,
					cause.source.program.environment.axes,
				) || !slices.Contains(variant.definition.Identity, cause.label) {
					continue
				}
				target := viewID + "#" + cause.input.id
				switch variant.definition.Membership.Stability {
				case "dynamic":
					if previous := required[target]; previous != "" && previous != variant.id {
						return fmt.Errorf("cumulative reduction target %q requires limitations for contributor variants %q and %q",
							target, previous, variant.id)
					}
					required[target] = variant.id
				case "stable", "restart_stable":
					if variant.definition.Reset.Scope != "shared" {
						return fmt.Errorf("cumulative reduction target %q permits independent resets for stable contributor variant %q",
							target, variant.id)
					}
				}
			}
		}
	}
	for _, target := range sortedMapKeys(required) {
		variantID := required[target]
		limitation := c.program.limitations[target]
		if limitation == nil || limitation.variant.id != variantID {
			return fmt.Errorf("cumulative reduction target %q requires an exact limitation for contributor variant %q",
				target, variantID)
		}
	}
	for _, target := range sortedMapKeys(c.program.limitations) {
		limitation := c.program.limitations[target]
		if required[target] != limitation.variant.id {
			return fmt.Errorf("limitation target %q is unnecessary", target)
		}
	}
	return nil
}
