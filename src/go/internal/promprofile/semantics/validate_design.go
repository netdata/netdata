// SPDX-License-Identifier: GPL-3.0-or-later

package promsemantics

import (
	"fmt"
	"slices"
	"strings"
)

func (d ProfileDesignDocument) validate() error {
	if err := validateIdentity("profile design", d.Version, ProfileDesignVersion, d.Profile); err != nil {
		return err
	}
	if err := requireText("match", d.Match); err != nil {
		return err
	}
	if err := validateRelativeContext("namespace", d.Namespace); err != nil {
		return err
	}
	if d.App != nil {
		if err := requireText("app", *d.App); err != nil {
			return err
		}
	}
	if d.Composition.Supports == nil {
		return fmt.Errorf("composition.supports must be present")
	}
	if err := validateIDMap("entities", d.Entities, true); err != nil {
		return err
	}
	if err := validateMap("views", d.Views, true); err != nil {
		return err
	}
	if err := validateIDMap("label_policies", d.LabelPolicies, false); err != nil {
		return err
	}
	if err := validateIDMap("reduction_policies", d.ReductionPolicies, false); err != nil {
		return err
	}
	if err := validateIDMap("normalizations", d.Normalizations, false); err != nil {
		return err
	}
	if err := validateIDMap("exclusions", d.Exclusions, false); err != nil {
		return err
	}
	for id, support := range d.Composition.Supports {
		if !validID(id) {
			return fmt.Errorf("composition.supports key %q is invalid", id)
		}
		if support.When.IsZero() {
			return fmt.Errorf("composition.supports.%s.when must be present", id)
		}
	}
	for _, id := range sortedMapKeys(d.Entities) {
		if err := validateEntity("entities."+id, d.Entities[id]); err != nil {
			return err
		}
	}
	for _, id := range sortedMapKeys(d.LabelPolicies) {
		if err := validateViewLabels("label_policies."+id, d.LabelPolicies[id]); err != nil {
			return err
		}
	}
	for _, id := range sortedMapKeys(d.ReductionPolicies) {
		if err := validateReduction("reduction_policies."+id, d.ReductionPolicies[id]); err != nil {
			return err
		}
	}
	for _, context := range sortedMapKeys(d.Views) {
		if err := validateRelativeContext("views key", context); err != nil {
			return err
		}
		if err := validateView("views."+context, d.Views[context], d); err != nil {
			return err
		}
	}
	for _, id := range sortedMapKeys(d.Normalizations) {
		if err := validateNormalization("normalizations."+id, d.Normalizations[id]); err != nil {
			return err
		}
	}
	for _, id := range sortedMapKeys(d.Exclusions) {
		if err := validateDesignExclusion("exclusions."+id, d.Exclusions[id]); err != nil {
			return err
		}
	}
	for key, limitation := range d.Limitations {
		if strings.Count(key, "#") != 1 {
			return fmt.Errorf("limitations key %q must be <view>#<input>", key)
		}
		if err := validateCumulativeLimitation("limitations."+key, limitation); err != nil {
			return err
		}
	}
	return nil
}

func validateEntity(field string, entity EntityDefinition) error {
	if err := requireText(field+".grain", entity.Grain); err != nil {
		return err
	}
	if entity.Identity.Required == nil || entity.Identity.Optional == nil {
		return fmt.Errorf("%s.identity required and optional lists must be present", field)
	}
	if err := validateLabelSet(field+".identity.required", entity.Identity.Required, true); err != nil {
		return err
	}
	if err := validateLabelSet(field+".identity.optional", entity.Identity.Optional, true); err != nil {
		return err
	}
	claimed := make(map[string]string, len(entity.Identity.Required)+len(entity.Identity.Optional))
	for _, label := range entity.Identity.Required {
		claimed[label] = "required"
	}
	for _, label := range entity.Identity.Optional {
		if _, ok := claimed[label]; ok {
			return fmt.Errorf("%s.identity label %q is both required and optional", field, label)
		}
		claimed[label] = "optional"
	}
	if entity.Identity.Alternatives != nil {
		if len(entity.Identity.Alternatives) < 2 {
			return fmt.Errorf("%s.identity.alternatives must contain at least two alternatives", field)
		}
		for index, alternative := range entity.Identity.Alternatives {
			alternativeField := fmt.Sprintf("%s.identity.alternatives[%d]", field, index)
			if err := requireList(alternativeField, alternative); err != nil {
				return err
			}
			if err := validateLabelSet(alternativeField, alternative, false); err != nil {
				return err
			}
			for _, label := range alternative {
				if owner, ok := claimed[label]; ok {
					if owner == "required" || owner == "optional" {
						return fmt.Errorf("%s.identity label %q is both %s and alternative", field, label, owner)
					}
					return fmt.Errorf("%s.identity label %q occurs in multiple alternatives", field, label)
				}
				claimed[label] = "alternative"
			}
		}
	}
	if entity.HighCardinalityAcceptance != nil {
		return requireText(field+".high_cardinality_acceptance.operator_value",
			entity.HighCardinalityAcceptance.OperatorValue)
	}
	return nil
}

func validateView(field string, view ViewDefinition, design ProfileDesignDocument) error {
	for name, value := range map[string]string{
		"family":   view.Family,
		"question": view.Question,
		"entity":   view.Entity,
	} {
		if err := requireText(field+"."+name, value); err != nil {
			return err
		}
	}
	if _, ok := design.Entities[view.Entity]; !ok {
		return fmt.Errorf("%s.entity references unknown entity %q", field, view.Entity)
	}
	if err := validateIDMap(field+".inputs", view.Inputs, true); err != nil {
		return err
	}
	for _, id := range sortedMapKeys(view.Inputs) {
		input := view.Inputs[id]
		if err := requireText(field+".inputs."+id+".signal", input.Signal); err != nil {
			return err
		}
		if err := requireList(field+".inputs."+id+".components", input.Components); err != nil {
			return err
		}
		for index, component := range input.Components {
			if !validID(component) {
				return fmt.Errorf(
					"%s.inputs.%s.components[%d] %q is not a valid component ID",
					field,
					id,
					index,
					component,
				)
			}
		}
		if err := validateLabelCondition(field+".inputs."+id+".where", input.Where); err != nil {
			return err
		}
		if input.RenderAs != "" && !validID(input.RenderAs) {
			return fmt.Errorf("%s.inputs.%s.render_as %q is not a valid semantic role", field, id, input.RenderAs)
		}
		if input.Direction != nil {
			if input.Direction.Negative == nil || !*input.Direction.Negative {
				return fmt.Errorf("%s.inputs.%s.direction.negative must be true", field, id)
			}
			if err := requireText(field+".inputs."+id+".direction.reason", input.Direction.Reason); err != nil {
				return err
			}
			if err := requireList(field+".inputs."+id+".direction.evidence", input.Direction.Evidence); err != nil {
				return err
			}
		}
		if input.Algorithm != nil {
			if err := requireEnum(field+".inputs."+id+".algorithm.value",
				input.Algorithm.Value, "absolute", "incremental"); err != nil {
				return err
			}
			if err := requireText(field+".inputs."+id+".algorithm.reason", input.Algorithm.Reason); err != nil {
				return err
			}
			if err := requireList(field+".inputs."+id+".algorithm.evidence", input.Algorithm.Evidence); err != nil {
				return err
			}
		}
	}
	if (view.Labels == nil) == (view.LabelPolicy == "") {
		return fmt.Errorf("%s must declare exactly labels or label_policy", field)
	}
	if view.Labels != nil {
		if err := validateViewLabels(field+".labels", *view.Labels); err != nil {
			return err
		}
	} else if _, ok := design.LabelPolicies[view.LabelPolicy]; !ok {
		return fmt.Errorf("%s.label_policy references unknown policy %q", field, view.LabelPolicy)
	}
	if view.Reduction != nil && view.ReductionPolicy != "" {
		return fmt.Errorf("%s cannot declare both reduction and reduction_policy", field)
	}
	if view.Reduction != nil {
		return validateReduction(field+".reduction", *view.Reduction)
	}
	if view.ReductionPolicy != "" {
		if _, ok := design.ReductionPolicies[view.ReductionPolicy]; !ok {
			return fmt.Errorf("%s.reduction_policy references unknown policy %q", field, view.ReductionPolicy)
		}
	}
	if view.Display != nil {
		if !validID(view.Display.Convention) {
			return fmt.Errorf("%s.display.convention %q is not a valid ID", field, view.Display.Convention)
		}
		if err := requireText(field+".display.reason", view.Display.Reason); err != nil {
			return err
		}
		if err := requireList(field+".display.evidence", view.Display.Evidence); err != nil {
			return err
		}
	}
	if view.Presentation != nil {
		if err := requireEnum(field+".presentation.type", view.Presentation.Type, "area", "stacked"); err != nil {
			return err
		}
		if err := requireText(field+".presentation.reason", view.Presentation.Reason); err != nil {
			return err
		}
		if view.Presentation.Type == "stacked" {
			if !validID(view.Presentation.Relationship) {
				return fmt.Errorf("%s.presentation.relationship must be a valid ID for stacked", field)
			}
		} else if view.Presentation.Relationship != "" {
			return fmt.Errorf("%s.presentation.relationship is allowed only for stacked", field)
		}
	}
	return nil
}

func validateViewLabels(field string, labels ViewLabels) error {
	if labels.Dimensions == nil || labels.Promote == nil || labels.Omit == nil {
		return fmt.Errorf("%s dimensions, promote, and omit must be present", field)
	}
	seen := make(map[string]string)
	for label, rendering := range labels.Dimensions {
		if err := validateLabelName(field+".dimensions."+label, label); err != nil {
			return err
		}
		if err := requireEnum(field+".dimensions."+label+".render", rendering.Render, "label_value", "input_role"); err != nil {
			return err
		}
		seen[label] = "dimensions"
	}
	if err := validateLabelSet(field+".promote", labels.Promote, true); err != nil {
		return err
	}
	for _, label := range labels.Promote {
		if previous := seen[label]; previous != "" {
			return fmt.Errorf("%s label %q is in both %s and promote", field, label, previous)
		}
		seen[label] = "promote"
	}
	for label, reason := range labels.Omit {
		if err := validateLabelName(field+".omit."+label, label); err != nil {
			return err
		}
		if err := requireText(field+".omit."+label, reason); err != nil {
			return err
		}
		if previous := seen[label]; previous != "" {
			return fmt.Errorf("%s label %q is in both %s and omit", field, label, previous)
		}
		seen[label] = "omit"
	}
	return nil
}

func validateReduction(field string, reduction ReductionDefinition) error {
	if err := requireEnum(field+".reducer", reduction.Reducer, "sum", "min", "max", "avg"); err != nil {
		return err
	}
	return requireText(field+".lost_comparison", reduction.LostComparison)
}

func validateNormalization(field string, normalization Normalization) error {
	if err := requireEnum(field+".kind", normalization.Kind,
		"category", "label_rename", "finite_alias", "namespace_alias", "embedded_identity_repair", "embedded_identity_extract",
		"generated_component_exclusion"); err != nil {
		return err
	}
	allowed := map[string][]string{
		"category": {
			"applies_to", "source_label", "target_label", "exact", "ranges", "missing", "malformed", "unknown",
			"output", "evidence",
		},
		"label_rename": {
			"source_label", "target_label", "retain_source",
		},
		"finite_alias": {
			"applies_to", "source_family", "evidence",
		},
		"namespace_alias": {
			"registry_group", "source_prefix", "target_prefix", "evidence",
		},
		"embedded_identity_repair": {
			"registry_grammar", "source_identity_label", "canonical", "embedded", "identity",
			"duplicate_exclusion", "output", "evidence",
		},
		"embedded_identity_extract": {
			"registry_grammar", "target_label", "retain", "output", "evidence",
		},
		"generated_component_exclusion": {
			"source", "outcome", "evidence",
		},
	}[normalization.Kind]
	for _, present := range presentNormalizationFields(normalization) {
		if !slices.Contains(allowed, present) {
			return fmt.Errorf("%s.%s is not allowed for kind %s", field, present, normalization.Kind)
		}
	}

	switch normalization.Kind {
	case "category":
		if err := validateSourceReference(field+".applies_to", normalization.AppliesTo, true); err != nil {
			return err
		}
		if err := validateLabelName(field+".source_label", normalization.SourceLabel); err != nil {
			return err
		}
		if err := validateLabelName(field+".target_label", normalization.TargetLabel); err != nil {
			return err
		}
		if normalization.SourceLabel == normalization.TargetLabel {
			return fmt.Errorf("%s source_label and target_label must differ", field)
		}
		if len(normalization.Exact) == 0 && len(normalization.Ranges) == 0 {
			return fmt.Errorf("%s requires exact and/or ranges", field)
		}
		for source, target := range normalization.Exact {
			if err := requireText(field+".exact."+source, target); err != nil {
				return err
			}
		}
		var previousMax *uint64
		for index, value := range normalization.Ranges {
			if value.Min == nil || value.Max == nil {
				return fmt.Errorf("%s.ranges[%d] requires min and max", field, index)
			}
			if *value.Min > *value.Max {
				return fmt.Errorf("%s.ranges[%d] min exceeds max", field, index)
			}
			if previousMax != nil && *value.Min <= *previousMax {
				return fmt.Errorf("%s.ranges[%d] is unsorted or overlaps the previous range", field, index)
			}
			if err := requireText(fmt.Sprintf("%s.ranges[%d].value", field, index), value.Value); err != nil {
				return err
			}
			previousMax = value.Max
		}
		for name, action := range map[string]*CategoryAction{
			"missing":   normalization.Missing,
			"malformed": normalization.Malformed,
			"unknown":   normalization.Unknown,
		} {
			if err := validateCategoryAction(field+"."+name, action); err != nil {
				return err
			}
		}
		if normalization.Output == nil {
			return fmt.Errorf("%s.output must be present", field)
		}
		if err := validateNormalizedLabelOutput(field+".output", *normalization.Output); err != nil {
			return err
		}
		if normalization.Output.EndpointCardinality != nil {
			return fmt.Errorf("%s.output.endpoint_cardinality is derived for kind category", field)
		}
		if normalization.Output.Stability != "" {
			return fmt.Errorf("%s.output.stability is derived for kind category", field)
		}
		return requireList(field+".evidence", normalization.Evidence)
	case "label_rename":
		if err := validateLabelName(field+".source_label", normalization.SourceLabel); err != nil {
			return err
		}
		if err := validateLabelName(field+".target_label", normalization.TargetLabel); err != nil {
			return err
		}
		if normalization.SourceLabel == normalization.TargetLabel {
			return fmt.Errorf("%s source_label and target_label must differ", field)
		}
		if normalization.RetainSource == nil {
			return fmt.Errorf("%s.retain_source must be present", field)
		}
		return nil
	case "finite_alias":
		if err := validateSourceReference(field+".applies_to", normalization.AppliesTo, true); err != nil {
			return err
		}
		if len(normalization.SourceFamily) == 0 {
			return fmt.Errorf("%s.source_family must not be empty", field)
		}
		for source, target := range normalization.SourceFamily {
			if err := validateMetricName(field+".source_family."+source, source); err != nil {
				return err
			}
			if err := validateMetricName(field+".source_family."+source, target); err != nil {
				return err
			}
		}
		return requireList(field+".evidence", normalization.Evidence)
	case "namespace_alias":
		if !validID(normalization.RegistryGroup) {
			return fmt.Errorf("%s.registry_group %q is not a valid ID", field, normalization.RegistryGroup)
		}
		if err := requireText(field+".source_prefix", normalization.SourcePrefix); err != nil {
			return err
		}
		if err := requireText(field+".target_prefix", normalization.TargetPrefix); err != nil {
			return err
		}
		if normalization.SourcePrefix == normalization.TargetPrefix {
			return fmt.Errorf("%s source_prefix and target_prefix must differ", field)
		}
		if err := validateMetricName(field+".source_prefix representative", normalization.SourcePrefix+"x"); err != nil {
			return err
		}
		if err := validateMetricName(field+".target_prefix representative", normalization.TargetPrefix+"x"); err != nil {
			return err
		}
		return requireList(field+".evidence", normalization.Evidence)
	case "embedded_identity_repair":
		if !validID(normalization.RegistryGrammar) {
			return fmt.Errorf("%s.registry_grammar %q is not a valid ID", field, normalization.RegistryGrammar)
		}
		if err := validateLabelName(field+".source_identity_label", normalization.SourceIdentityLabel); err != nil {
			return err
		}
		if normalization.Canonical == nil || normalization.Embedded == nil || normalization.Identity == nil ||
			normalization.DuplicateExclusion == nil {
			return fmt.Errorf("%s requires canonical, embedded, identity, and duplicate_exclusion", field)
		}
		if err := requireText(field+".canonical.family_prefix", normalization.Canonical.FamilyPrefix); err != nil {
			return err
		}
		if err := validateLabelName(field+".canonical.identity_label", normalization.Canonical.IdentityLabel); err != nil {
			return err
		}
		if err := requireText(field+".embedded.family_prefix", normalization.Embedded.FamilyPrefix); err != nil {
			return err
		}
		if !validID(normalization.Embedded.Capture) {
			return fmt.Errorf("%s.embedded.capture %q is not a valid ID", field, normalization.Embedded.Capture)
		}
		if err := requireList(field+".identity.operands", normalization.Identity.Operands); err != nil {
			return err
		}
		if err := requireText(field+".identity.separator", normalization.Identity.Separator); err != nil {
			return err
		}
		if err := requireEnum(field+".identity.blank",
			normalization.Identity.Blank, "omit_operand_and_separator"); err != nil {
			return err
		}
		if err := requireEnum(field+".identity.sanitizer",
			normalization.Identity.Sanitizer, "prometheus_label_value"); err != nil {
			return err
		}
		if normalization.DuplicateExclusion.WhenIdentityLabel != "absent" ||
			normalization.DuplicateExclusion.Outcome != "drop_before_writer" {
			return fmt.Errorf("%s.duplicate_exclusion must be absent/drop_before_writer", field)
		}
		if err := requireList(field+".duplicate_exclusion.evidence",
			normalization.DuplicateExclusion.Evidence); err != nil {
			return err
		}
		if normalization.Output == nil {
			return fmt.Errorf("%s.output must be present", field)
		}
		if err := validateNormalizedLabelOutput(field+".output", *normalization.Output); err != nil {
			return err
		}
		if normalization.Output.EndpointCardinality == nil {
			return fmt.Errorf("%s.output.endpoint_cardinality must be present", field)
		}
		if normalization.Output.Stability == "" {
			return fmt.Errorf("%s.output.stability must be present", field)
		}
		return requireList(field+".evidence", normalization.Evidence)
	case "embedded_identity_extract":
		if !validID(normalization.RegistryGrammar) {
			return fmt.Errorf("%s.registry_grammar %q is not a valid ID", field, normalization.RegistryGrammar)
		}
		if err := validateLabelName(field+".target_label", normalization.TargetLabel); err != nil {
			return err
		}
		if normalization.Retain == nil || normalization.Retain.Family != "canonical_branch" ||
			normalization.Retain.CapturedIdentity == nil || !*normalization.Retain.CapturedIdentity {
			return fmt.Errorf("%s.retain must preserve canonical_branch and captured identity", field)
		}
		if normalization.Output == nil {
			return fmt.Errorf("%s.output must be present", field)
		}
		if err := validateNormalizedLabelOutput(field+".output", *normalization.Output); err != nil {
			return err
		}
		if normalization.Output.EndpointCardinality == nil {
			return fmt.Errorf("%s.output.endpoint_cardinality must be present", field)
		}
		if normalization.Output.Stability == "" {
			return fmt.Errorf("%s.output.stability must be present", field)
		}
		return requireList(field+".evidence", normalization.Evidence)
	case "generated_component_exclusion":
		if normalization.Source == nil {
			return fmt.Errorf("%s.source must be present", field)
		}
		if err := requireText(field+".source.namespace_prefix", normalization.Source.NamespacePrefix); err != nil {
			return err
		}
		if err := requireText(field+".source.terminal_suffix", normalization.Source.TerminalSuffix); err != nil {
			return err
		}
		if !validID(normalization.Source.Component) {
			return fmt.Errorf("%s.source.component %q is not a valid ID", field, normalization.Source.Component)
		}
		if normalization.Outcome != "drop_before_writer" {
			return fmt.Errorf("%s.outcome must be drop_before_writer", field)
		}
		return requireList(field+".evidence", normalization.Evidence)
	}
	panic("validated normalization kind has no validator")
}

func presentNormalizationFields(normalization Normalization) []string {
	fields := make([]string, 0, 20)
	add := func(name string, present bool) {
		if present {
			fields = append(fields, name)
		}
	}
	add("applies_to", normalization.AppliesTo != nil)
	add("source_label", normalization.SourceLabel != "")
	add("target_label", normalization.TargetLabel != "")
	add("retain_source", normalization.RetainSource != nil)
	add("exact", normalization.Exact != nil)
	add("ranges", normalization.Ranges != nil)
	add("missing", normalization.Missing != nil)
	add("malformed", normalization.Malformed != nil)
	add("unknown", normalization.Unknown != nil)
	add("source_family", normalization.SourceFamily != nil)
	add("registry_group", normalization.RegistryGroup != "")
	add("source_prefix", normalization.SourcePrefix != "")
	add("target_prefix", normalization.TargetPrefix != "")
	add("registry_grammar", normalization.RegistryGrammar != "")
	add("source_identity_label", normalization.SourceIdentityLabel != "")
	add("canonical", normalization.Canonical != nil)
	add("embedded", normalization.Embedded != nil)
	add("identity", normalization.Identity != nil)
	add("duplicate_exclusion", normalization.DuplicateExclusion != nil)
	add("retain", normalization.Retain != nil)
	add("source", normalization.Source != nil)
	add("outcome", normalization.Outcome != "")
	add("output", normalization.Output != nil)
	add("evidence", normalization.Evidence != nil)
	return fields
}

func validateNormalizedLabelOutput(field string, output NormalizedLabelOutput) error {
	if err := requireText(field+".meaning", output.Meaning); err != nil {
		return err
	}
	if output.EndpointCardinality != nil {
		if err := validateCardinality(field+".endpoint_cardinality", *output.EndpointCardinality); err != nil {
			return err
		}
	}
	if output.Stability != "" {
		if err := requireEnum(field+".stability", output.Stability, "stable", "restart_stable", "dynamic"); err != nil {
			return err
		}
	}
	return requireList(field+".evidence", output.Evidence)
}

func validateCategoryAction(field string, action *CategoryAction) error {
	if action == nil {
		return fmt.Errorf("%s must be present", field)
	}
	if (action.Set == nil) == (action.LeaveAbsent == nil) {
		return fmt.Errorf("%s must declare exactly set or leave_absent", field)
	}
	if action.Set != nil {
		return requireText(field+".set", *action.Set)
	}
	if !*action.LeaveAbsent {
		return fmt.Errorf("%s.leave_absent must be true", field)
	}
	return nil
}

func validateSourceReference(field string, reference *SourceReference, required bool) error {
	if reference == nil {
		if required {
			return fmt.Errorf("%s must be present", field)
		}
		return nil
	}
	if err := requireText(field+".signal", reference.Signal); err != nil {
		return err
	}
	if err := requireList(field+".components", reference.Components); err != nil {
		return err
	}
	for index, component := range reference.Components {
		if !validID(component) {
			return fmt.Errorf("%s.components[%d] %q is not a valid ID", field, index, component)
		}
	}
	return validateLabelCondition(field+".where", reference.Where)
}

func validateDesignExclusion(field string, exclusion DesignExclusion) error {
	if err := validateSourceReference(field+".source", &exclusion.Source, true); err != nil {
		return err
	}
	if err := requireEnum(field+".reason", exclusion.Reason,
		"equivalent_duplicate", "source_superseded", "not_chartable", "metadata_only", "collection_hazard"); err != nil {
		return err
	}
	if err := requireEnum(field+".outcome",
		exclusion.Outcome, "drop_before_writer", "retain_writable_unrendered"); err != nil {
		return err
	}
	if err := requireList(field+".evidence", exclusion.Evidence); err != nil {
		return err
	}
	allowed := map[string][]string{
		"equivalent_duplicate": {"covering_view"},
		"source_superseded":    {"replacement"},
		"not_chartable":        {"lost_question", "required_operation"},
		"metadata_only":        {},
		"collection_hazard":    {},
	}[exclusion.Reason]
	for name, present := range map[string]bool{
		"covering_view":      exclusion.CoveringView != "",
		"replacement":        exclusion.Replacement != "",
		"lost_question":      exclusion.LostQuestion != "",
		"required_operation": exclusion.RequiredOperation != "",
	} {
		if present && !slices.Contains(allowed, name) {
			return fmt.Errorf("%s.%s is not allowed for reason %s", field, name, exclusion.Reason)
		}
	}
	switch exclusion.Reason {
	case "equivalent_duplicate":
		return requireText(field+".covering_view", exclusion.CoveringView)
	case "source_superseded":
		return requireText(field+".replacement", exclusion.Replacement)
	case "not_chartable":
		if err := requireText(field+".lost_question", exclusion.LostQuestion); err != nil {
			return err
		}
		if exclusion.RequiredOperation != "age_from_unix_epoch" {
			return fmt.Errorf("%s.required_operation must be age_from_unix_epoch", field)
		}
	case "metadata_only":
		if exclusion.Outcome != "retain_writable_unrendered" {
			return fmt.Errorf("%s.outcome must be retain_writable_unrendered", field)
		}
	}
	return nil
}

func validateCumulativeLimitation(field string, limitation CumulativeLimitation) error {
	if !validID(limitation.ContributorVariant) {
		return fmt.Errorf("%s.contributor_variant %q is not a valid ID", field, limitation.ContributorVariant)
	}
	if err := requireList(field+".evidence", limitation.Evidence); err != nil {
		return err
	}
	if !validID(limitation.ProofSequence) {
		return fmt.Errorf("%s.proof_sequence %q is not a valid ID", field, limitation.ProofSequence)
	}
	if limitation.Effect != "aggregate_drop_may_create_one_reset_interpreted_rate_gap" {
		return fmt.Errorf("%s.effect is unsupported", field)
	}
	return nil
}
