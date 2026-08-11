// SPDX-License-Identifier: GPL-3.0-or-later

package promsemantics

import (
	"fmt"
	"slices"
)

func (c *semanticCompiler) validateReusablePolicyUsage() error {
	sourceComponentUses := make(map[string]int)
	sourceLabelUses := make(map[string]int)
	for _, signal := range c.input.Contract.Source.Signals {
		if signal.ComponentPolicy != "" {
			sourceComponentUses[signal.ComponentPolicy]++
		}
		if signal.LabelPolicy != "" {
			sourceLabelUses[signal.LabelPolicy]++
		}
	}
	designLabelUses := make(map[string]int)
	designReductionUses := make(map[string]int)
	for _, view := range c.input.Contract.Design.Views {
		if view.LabelPolicy != "" {
			designLabelUses[view.LabelPolicy]++
		}
		if view.ReductionPolicy != "" {
			designReductionUses[view.ReductionPolicy]++
		}
	}
	for _, policy := range sortedMapKeys(c.input.Contract.Source.ComponentPolicies) {
		if sourceComponentUses[policy] < 2 {
			return fmt.Errorf("source component policy %q has %d uses; reusable policies require at least two",
				policy, sourceComponentUses[policy])
		}
	}
	for _, policy := range sortedMapKeys(c.input.Contract.Source.LabelPolicies) {
		if sourceLabelUses[policy] < 2 {
			return fmt.Errorf("source label policy %q has %d uses; reusable policies require at least two",
				policy, sourceLabelUses[policy])
		}
	}
	for _, policy := range sortedMapKeys(c.input.Contract.Design.LabelPolicies) {
		if designLabelUses[policy] < 2 {
			return fmt.Errorf("design label policy %q has %d uses; reusable policies require at least two",
				policy, designLabelUses[policy])
		}
	}
	for _, policy := range sortedMapKeys(c.input.Contract.Design.ReductionPolicies) {
		if designReductionUses[policy] < 2 {
			return fmt.Errorf("design reduction policy %q has %d uses; reusable policies require at least two",
				policy, designReductionUses[policy])
		}
	}
	return nil
}

func (c *semanticCompiler) validateEvidenceClosure() error {
	uses := make(map[string]int, len(c.input.Contract.Source.Evidence))
	upstreamUses := make(map[string]int, len(c.input.Contract.Source.Upstreams))
	use := func(field string, references []string, allowedKinds ...string) error {
		if err := requireList(field, references); err != nil {
			return err
		}
		for _, reference := range references {
			record, ok := c.input.Contract.Source.Evidence[reference]
			if !ok {
				return fmt.Errorf("%s references unknown source evidence %q", field, reference)
			}
			if !slices.Contains(allowedKinds, record.Kind) {
				return fmt.Errorf("%s evidence %q has incompatible kind %q", field, reference, record.Kind)
			}
			uses[reference]++
			upstreamUses[record.Upstream]++
		}
		return nil
	}
	useAll := func(field string, references []string, requiredKinds ...string) error {
		if err := use(field, references, requiredKinds...); err != nil {
			return err
		}
		found := make(map[string]struct{}, len(requiredKinds))
		for _, reference := range references {
			found[c.input.Contract.Source.Evidence[reference].Kind] = struct{}{}
		}
		for _, kind := range requiredKinds {
			if _, ok := found[kind]; !ok {
				return fmt.Errorf("%s must reference %q evidence", field, kind)
			}
		}
		return nil
	}

	source := c.input.Contract.Source
	for axisID, axis := range source.Environment.Axes {
		if err := use("environment.axes."+axisID+".evidence", axis.Evidence, "availability"); err != nil {
			return err
		}
	}
	for policyID, policy := range source.Environment.Policies {
		if c.policyUses[policyID] == 0 {
			continue
		}
		if err := use("environment.policies."+policyID+".evidence", policy.Evidence, "availability"); err != nil {
			return err
		}
	}
	for signalID, signal := range source.Signals {
		field := "signals." + signalID
		if signal.Source.Inline != nil {
			for registrationID, registration := range signal.Source.Inline.Registrations {
				if err := use(field+".source.inline.registrations."+registrationID+".evidence",
					registration.Evidence, "registration"); err != nil {
					return err
				}
			}
		}
		if err := use(field+".population.evidence", signal.Population.Evidence, "population"); err != nil {
			return err
		}
		for componentID, component := range effectiveSignalComponents(signal, source) {
			componentField := field + ".components." + componentID
			if err := use(componentField+".lifecycle.evidence", component.Lifecycle.Evidence, "lifecycle"); err != nil {
				return err
			}
			if err := use(componentField+".unit.evidence", component.Unit.Evidence, "unit"); err != nil {
				return err
			}
		}
		for labelID, label := range effectiveSignalLabels(signal, source) {
			if err := use(field+".labels."+labelID+".evidence", label.Evidence, "label"); err != nil {
				return err
			}
		}
		for constraintID, constraint := range signal.LabelPresenceConstraints {
			if err := use(field+".label_presence_constraints."+constraintID+".evidence",
				constraint.Evidence, "relationship"); err != nil {
				return err
			}
		}
		for dependencyID, dependency := range signal.FunctionalDependencies {
			if err := use(field+".functional_dependencies."+dependencyID+".evidence",
				dependency.Evidence, "relationship"); err != nil {
				return err
			}
		}
		if signal.Contributors != nil {
			for variantID, variant := range signal.Contributors.Variants {
				variantField := field + ".contributors.variants." + variantID + ".evidence"
				if err := use(variantField+".population", variant.Evidence.Population, "population"); err != nil {
					return err
				}
				if err := use(variantField+".lifecycle", variant.Evidence.Lifecycle, "lifecycle"); err != nil {
					return err
				}
				if err := use(variantField+".relationship", variant.Evidence.Relationship, "relationship"); err != nil {
					return err
				}
			}
		}
	}
	for relationshipID, relationship := range source.Relationships {
		if err := use("relationships."+relationshipID+".evidence", relationship.Evidence, "relationship"); err != nil {
			return err
		}
	}
	for encodingID, encoding := range source.StateEncodings {
		if err := use("state_encodings."+encodingID+".evidence", encoding.Evidence, "state_encoding"); err != nil {
			return err
		}
	}
	for exclusionID, exclusion := range source.SourceExclusions {
		requiredKind := "registration"
		if exclusion.Reason == "out_of_endpoint" {
			requiredKind = "availability"
		}
		if err := use("source_exclusions."+exclusionID+".evidence", exclusion.Evidence, requiredKind); err != nil {
			return err
		}
	}

	design := c.input.Contract.Design
	for contextID, view := range design.Views {
		for inputID, input := range view.Inputs {
			field := "views." + contextID + ".inputs." + inputID
			if input.Direction != nil {
				if err := use(field+".direction.evidence", input.Direction.Evidence, "display_convention"); err != nil {
					return err
				}
			}
			if input.Algorithm != nil {
				if err := use(field+".algorithm.evidence", input.Algorithm.Evidence, "lifecycle"); err != nil {
					return err
				}
			}
		}
		if view.Display != nil {
			if err := use("views."+contextID+".display.evidence", view.Display.Evidence, "display_convention"); err != nil {
				return err
			}
		}
	}
	for normalizationID, normalization := range design.Normalizations {
		field := "normalizations." + normalizationID
		if normalization.Output != nil {
			if err := use(field+".output.evidence", normalization.Output.Evidence, "label"); err != nil {
				return err
			}
		}
		switch normalization.Kind {
		case "category":
			if err := use(field+".evidence", normalization.Evidence, "normalization"); err != nil {
				return err
			}
		case "finite_alias", "namespace_alias":
			if err := use(field+".evidence", normalization.Evidence, "relationship"); err != nil {
				return err
			}
		case "embedded_identity_repair":
			if err := use(field+".evidence", normalization.Evidence, "identity"); err != nil {
				return err
			}
			if err := use(field+".duplicate_exclusion.evidence",
				normalization.DuplicateExclusion.Evidence, "relationship"); err != nil {
				return err
			}
		case "embedded_identity_extract":
			if err := use(field+".evidence", normalization.Evidence, "identity"); err != nil {
				return err
			}
		case "generated_component_exclusion":
			if err := useAll(field+".evidence", normalization.Evidence, "registration", "lifecycle", "unit"); err != nil {
				return err
			}
		}
	}
	for exclusionID, exclusion := range design.Exclusions {
		field := "exclusions." + exclusionID + ".evidence"
		var allowed []string
		switch exclusion.Reason {
		case "equivalent_duplicate":
			allowed = []string{"relationship"}
		case "source_superseded":
			allowed = []string{"deprecation"}
		case "not_chartable":
			allowed = []string{"lifecycle", "unit"}
		case "metadata_only":
			allowed = []string{"lifecycle", "unit", "label"}
		case "collection_hazard":
			allowed = []string{"collection_hazard"}
		case "scope_delegation":
			allowed = []string{"delegation"}
		}
		if exclusion.Reason == "not_chartable" {
			if err := useAll(field, exclusion.Evidence, "lifecycle", "unit"); err != nil {
				return err
			}
			continue
		}
		if exclusion.Reason == "metadata_only" {
			if err := useAll(field, exclusion.Evidence, "lifecycle", "unit", "label"); err != nil {
				return err
			}
			continue
		}
		if err := use(field, exclusion.Evidence, allowed...); err != nil {
			return err
		}
	}
	for target, limitation := range design.Limitations {
		if err := use("limitations."+target+".evidence", limitation.Evidence, "lifecycle", "relationship"); err != nil {
			return err
		}
	}

	for _, evidenceID := range sortedMapKeys(source.Evidence) {
		if uses[evidenceID] == 0 {
			return fmt.Errorf("source evidence %q is unused", evidenceID)
		}
	}
	for _, upstreamID := range sortedMapKeys(source.Upstreams) {
		if upstreamUses[upstreamID] == 0 {
			return fmt.Errorf("source upstream %q is unused", upstreamID)
		}
	}
	return nil
}
