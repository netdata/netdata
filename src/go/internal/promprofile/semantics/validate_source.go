// SPDX-License-Identifier: GPL-3.0-or-later

package promsemantics

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

var fullGitCommitPattern = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)

func (d SourceSemanticsDocument) validate() error {
	if err := validateIdentity("source semantics", d.Version, SourceSemanticsVersion, d.Profile); err != nil {
		return err
	}
	if err := validateIDMap("upstreams", d.Upstreams, true); err != nil {
		return err
	}
	if err := validateIDMap("evidence", d.Evidence, true); err != nil {
		return err
	}
	if d.Environment.Axes == nil || d.Environment.Policies == nil {
		return fmt.Errorf("environment.axes and environment.policies must be present")
	}
	if err := validateIDMap("environment.axes", d.Environment.Axes, false); err != nil {
		return err
	}
	if err := validateIDMap("environment.policies", d.Environment.Policies, false); err != nil {
		return err
	}
	if err := validateIDMap("signals", d.Signals, true); err != nil {
		return err
	}
	if err := validateIDMap("component_policies", d.ComponentPolicies, false); err != nil {
		return err
	}
	if err := validateIDMap("label_policies", d.LabelPolicies, false); err != nil {
		return err
	}
	if err := validateIDMap("relationships", d.Relationships, false); err != nil {
		return err
	}
	if err := validateIDMap("state_encodings", d.StateEncodings, false); err != nil {
		return err
	}
	if err := validateIDMap("source_exclusions", d.SourceExclusions, false); err != nil {
		return err
	}
	for _, id := range sortedMapKeys(d.Upstreams) {
		if err := validateSourceUpstream("upstreams."+id, d.Upstreams[id]); err != nil {
			return err
		}
	}
	for _, id := range sortedMapKeys(d.Evidence) {
		if err := validateSourceEvidence("evidence."+id, d.Evidence[id], d.Upstreams); err != nil {
			return err
		}
	}
	for _, id := range sortedMapKeys(d.Environment.Axes) {
		if err := validateEnvironmentAxis("environment.axes."+id, d.Environment.Axes[id], d.Evidence); err != nil {
			return err
		}
	}
	for _, id := range sortedMapKeys(d.Environment.Policies) {
		policy := d.Environment.Policies[id]
		if err := validateEnvironmentCondition(
			"environment.policies."+id+".when",
			policy.When,
			d.Environment.Axes,
		); err != nil {
			return err
		}
		if err := validateEvidenceKinds(
			"environment.policies."+id+".evidence",
			policy.Evidence,
			d.Evidence,
			"availability",
		); err != nil {
			return err
		}
	}
	for _, id := range sortedMapKeys(d.ComponentPolicies) {
		components := d.ComponentPolicies[id]
		if len(components) == 0 {
			return fmt.Errorf("component_policies.%s must not be empty", id)
		}
		for _, componentID := range sortedMapKeys(components) {
			if !validID(componentID) {
				return fmt.Errorf("component_policies.%s key %q is invalid", id, componentID)
			}
			if err := validateComponent(
				"component_policies."+id+"."+componentID,
				components[componentID],
				d.Evidence,
			); err != nil {
				return err
			}
		}
	}
	for _, id := range sortedMapKeys(d.LabelPolicies) {
		labels := d.LabelPolicies[id]
		for _, labelName := range sortedMapKeys(labels) {
			if err := validateLabelName("label_policies."+id+"."+labelName, labelName); err != nil {
				return err
			}
			if err := validateSourceLabel(
				"label_policies."+id+"."+labelName,
				labels[labelName],
				d.Evidence,
			); err != nil {
				return err
			}
			if err := validateSourceLabelEnvironment(
				"label_policies."+id+"."+labelName,
				labels[labelName],
				d,
			); err != nil {
				return err
			}
		}
	}
	for _, id := range sortedMapKeys(d.Signals) {
		if err := validateSignal("signals."+id, d.Signals[id], d); err != nil {
			return err
		}
	}
	for _, id := range sortedMapKeys(d.Relationships) {
		if err := validateRelationship("relationships."+id, d.Relationships[id], d); err != nil {
			return err
		}
	}
	for _, id := range sortedMapKeys(d.StateEncodings) {
		if err := validateStateEncoding("state_encodings."+id, d.StateEncodings[id], d); err != nil {
			return err
		}
	}
	for _, id := range sortedMapKeys(d.SourceExclusions) {
		if err := validateSourceExclusion("source_exclusions."+id, d.SourceExclusions[id], d); err != nil {
			return err
		}
	}
	return nil
}

func validateSourceUpstream(field string, upstream SourceUpstream) error {
	if err := requireText(field+".repository", upstream.Repository); err != nil {
		return err
	}
	parts := strings.Split(upstream.Repository, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return fmt.Errorf("%s.repository %q must be owner/repository", field, upstream.Repository)
	}
	if !fullGitCommitPattern.MatchString(upstream.Commit) {
		return fmt.Errorf("%s.commit must be a full 40-character Git commit", field)
	}
	return nil
}

func validateSourceEvidence(
	field string,
	evidence SourceEvidence,
	upstreams map[string]SourceUpstream,
) error {
	if err := requireEnum(field+".kind", evidence.Kind,
		"availability", "registration", "lifecycle", "unit", "population", "label", "relationship",
		"state_encoding", "normalization", "identity", "deprecation", "collection_hazard", "delegation",
		"display_convention"); err != nil {
		return err
	}
	if _, ok := upstreams[evidence.Upstream]; !ok {
		return fmt.Errorf("%s.upstream references unknown upstream %q", field, evidence.Upstream)
	}
	if len(evidence.Locations) == 0 {
		return fmt.Errorf("%s.locations must not be empty", field)
	}
	if err := validateStringSet(field+".locations", evidence.Locations, false); err != nil {
		return err
	}
	for index, location := range evidence.Locations {
		if err := validateEvidenceLocation(fmt.Sprintf("%s.locations[%d]", field, index), location); err != nil {
			return err
		}
	}
	return requireText(field+".claim", evidence.Claim)
}

func validateEnvironmentAxis(
	field string,
	axis EnvironmentAxis,
	evidence map[string]SourceEvidence,
) error {
	if err := requireEnum(field+".kind", axis.Kind, "enum", "ordered_enum", "integer", "enum_set"); err != nil {
		return err
	}
	switch axis.Kind {
	case "enum", "ordered_enum", "enum_set":
		if err := requireList(field+".values", axis.Values); err != nil {
			return err
		}
		if axis.Min != nil || axis.Max != nil {
			return fmt.Errorf("%s min/max are valid only for integer axes", field)
		}
	case "integer":
		if len(axis.Values) != 0 || axis.Min == nil || axis.Max == nil || *axis.Min > *axis.Max {
			return fmt.Errorf("%s integer axis requires a finite min/max and no values", field)
		}
	}
	if err := requireText(field+".meaning", axis.Meaning); err != nil {
		return err
	}
	return validateEvidenceKinds(field+".evidence", axis.Evidence, evidence, "availability")
}

func validateSignal(field string, signal SignalDefinition, document SourceSemanticsDocument) error {
	if err := validateConditionUse(
		field+".availability",
		signal.Availability,
		document.Environment.Axes,
		document.Environment.Policies,
		false,
	); err != nil {
		return err
	}
	if (signal.Source.Inline == nil) == (signal.Source.Generated == nil) {
		return fmt.Errorf("%s.source must declare exactly inline or generated", field)
	}
	if signal.Source.Inline != nil {
		if err := validateIDMap(field+".source.inline.registrations", signal.Source.Inline.Registrations, true); err != nil {
			return err
		}
		for _, id := range sortedMapKeys(signal.Source.Inline.Registrations) {
			registration := signal.Source.Inline.Registrations[id]
			registrationField := field + ".source.inline.registrations." + id
			if err := validateFamilySelector(registrationField+".family", registration.Family, true); err != nil {
				return err
			}
			if err := validatePrometheusContract(registrationField+".prometheus", registration.Prometheus); err != nil {
				return err
			}
			if err := validateEvidenceKinds(
				registrationField+".evidence",
				registration.Evidence,
				document.Evidence,
				"registration",
			); err != nil {
				return err
			}
			if err := validateConditionUse(
				registrationField+".when",
				registration.When,
				document.Environment.Axes,
				document.Environment.Policies,
				false,
			); err != nil {
				return err
			}
		}
	}
	if signal.Source.Generated != nil {
		generated := signal.Source.Generated
		if err := requireList(field+".source.generated.registry_groups", generated.RegistryGroups); err != nil {
			return err
		}
		for index, id := range generated.RegistryGroups {
			if !validID(id) {
				return fmt.Errorf("%s.source.generated.registry_groups[%d] %q is invalid", field, index, id)
			}
		}
		if generated.Scope.Registrations != nil {
			if err := requireList(field+".source.generated.scope.registrations", generated.Scope.Registrations); err != nil {
				return err
			}
		}
		if generated.Scope.Families.Exact != nil {
			if err := requireList(field+".source.generated.scope.families.exact", generated.Scope.Families.Exact); err != nil {
				return err
			}
		}
		if generated.Scope.Families.Grammars != nil {
			if err := requireList(field+".source.generated.scope.families.grammars", generated.Scope.Families.Grammars); err != nil {
				return err
			}
		}
		if len(generated.Scope.Registrations) == 0 &&
			len(generated.Scope.Families.Exact) == 0 &&
			len(generated.Scope.Families.Grammars) == 0 {
			return fmt.Errorf("%s.source.generated.scope must not be empty", field)
		}
		for index, id := range generated.Scope.Registrations {
			if !validID(id) {
				return fmt.Errorf("%s.source.generated.scope.registrations[%d] %q is invalid", field, index, id)
			}
		}
		for index, family := range generated.Scope.Families.Exact {
			if err := validateMetricName(
				fmt.Sprintf("%s.source.generated.scope.families.exact[%d]", field, index),
				family,
			); err != nil {
				return err
			}
		}
		for index, grammar := range generated.Scope.Families.Grammars {
			if !validID(grammar) {
				return fmt.Errorf("%s.source.generated.scope.families.grammars[%d] %q is invalid", field, index, grammar)
			}
		}
	}
	if err := requireText(field+".population.id", signal.Population.ID); err != nil {
		return err
	}
	if !validID(signal.Population.ID) {
		return fmt.Errorf("%s.population.id %q is not a valid ID", field, signal.Population.ID)
	}
	if err := requireText(field+".population.meaning", signal.Population.Meaning); err != nil {
		return err
	}
	if err := validateEvidenceKinds(
		field+".population.evidence",
		signal.Population.Evidence,
		document.Evidence,
		"population",
	); err != nil {
		return err
	}
	if (signal.Components == nil) == (signal.ComponentPolicy == "") {
		return fmt.Errorf("%s must declare exactly components or component_policy", field)
	}
	if (signal.Labels == nil) == (signal.LabelPolicy == "") {
		return fmt.Errorf("%s must declare exactly labels or label_policy", field)
	}
	if signal.FunctionalDependencies == nil {
		return fmt.Errorf("%s.functional_dependencies must be present", field)
	}
	if err := validateIDMap(field+".functional_dependencies", signal.FunctionalDependencies, false); err != nil {
		return err
	}
	if signal.ComponentPolicy != "" {
		if _, ok := document.ComponentPolicies[signal.ComponentPolicy]; !ok {
			return fmt.Errorf("%s.component_policy references unknown policy %q", field, signal.ComponentPolicy)
		}
	}
	if signal.LabelPolicy != "" {
		if _, ok := document.LabelPolicies[signal.LabelPolicy]; !ok {
			return fmt.Errorf("%s.label_policy references unknown policy %q", field, signal.LabelPolicy)
		}
	}
	if signal.Components != nil {
		if len(signal.Components) == 0 {
			return fmt.Errorf("%s.components must not be empty", field)
		}
		for _, id := range sortedMapKeys(signal.Components) {
			if !validID(id) {
				return fmt.Errorf("%s.components key %q is invalid", field, id)
			}
			if err := validateComponent(field+".components."+id, signal.Components[id], document.Evidence); err != nil {
				return err
			}
		}
	}
	effectiveComponents := signal.Components
	if signal.ComponentPolicy != "" {
		effectiveComponents = document.ComponentPolicies[signal.ComponentPolicy]
	}
	if err := validateComponentSet(field, signal, effectiveComponents); err != nil {
		return err
	}
	for _, name := range sortedMapKeys(signal.Labels) {
		if err := validateLabelName(field+".labels."+name, name); err != nil {
			return err
		}
		if err := validateSourceLabel(field+".labels."+name, signal.Labels[name], document.Evidence); err != nil {
			return err
		}
		if err := validateSourceLabelEnvironment(
			field+".labels."+name,
			signal.Labels[name],
			document,
		); err != nil {
			return err
		}
	}
	labels := signal.Labels
	if signal.LabelPolicy != "" {
		labels = document.LabelPolicies[signal.LabelPolicy]
	}
	if signal.LabelPresenceConstraints != nil {
		if err := validateIDMap(field+".label_presence_constraints", signal.LabelPresenceConstraints, true); err != nil {
			return err
		}
		claimed := make(map[string]string)
		for _, id := range sortedMapKeys(signal.LabelPresenceConstraints) {
			constraint := signal.LabelPresenceConstraints[id]
			constraintField := field + ".label_presence_constraints." + id
			if err := requireEnum(constraintField+".kind", constraint.Kind, "exactly_one"); err != nil {
				return err
			}
			if len(constraint.Alternatives) < 2 {
				return fmt.Errorf("%s.alternatives must contain at least two alternatives", constraintField)
			}
			for index, alternative := range constraint.Alternatives {
				alternativeField := fmt.Sprintf("%s.alternatives[%d]", constraintField, index)
				if err := requireList(alternativeField, alternative); err != nil {
					return err
				}
				if err := validateLabelSet(alternativeField, alternative, false); err != nil {
					return err
				}
				for _, label := range alternative {
					schema, ok := labels[label]
					if !ok {
						return fmt.Errorf("%s references unknown label %q", alternativeField, label)
					}
					if schema.Presence.Kind != "optional" {
						return fmt.Errorf("%s label %q must have optional presence", constraintField, label)
					}
					if owner, ok := claimed[label]; ok {
						return fmt.Errorf("%s label %q is already owned by constraint %q", constraintField, label, owner)
					}
					claimed[label] = id
				}
			}
			if err := validateEvidenceKinds(
				constraintField+".evidence",
				constraint.Evidence,
				document.Evidence,
				"relationship",
			); err != nil {
				return err
			}
		}
	}
	for _, id := range sortedMapKeys(signal.FunctionalDependencies) {
		dependency := signal.FunctionalDependencies[id]
		dependencyField := field + ".functional_dependencies." + id
		if err := requireList(dependencyField+".determinants", dependency.Determinants); err != nil {
			return err
		}
		if err := requireList(dependencyField+".dependents", dependency.Dependents); err != nil {
			return err
		}
		for _, label := range append(append([]string(nil), dependency.Determinants...), dependency.Dependents...) {
			if _, ok := labels[label]; !ok {
				return fmt.Errorf("%s references unknown label %q", dependencyField, label)
			}
		}
		if slices.ContainsFunc(dependency.Dependents, func(label string) bool {
			return slices.Contains(dependency.Determinants, label)
		}) {
			return fmt.Errorf("%s determinants and dependents must be disjoint", dependencyField)
		}
		if err := validateConditionUse(
			dependencyField+".when",
			dependency.When,
			document.Environment.Axes,
			document.Environment.Policies,
			false,
		); err != nil {
			return err
		}
		if err := validateEvidenceKinds(
			dependencyField+".evidence",
			dependency.Evidence,
			document.Evidence,
			"relationship",
		); err != nil {
			return err
		}
	}
	if signal.Contributors != nil {
		if err := validateContributors(field+".contributors", *signal.Contributors, signal, document); err != nil {
			return err
		}
	}
	return nil
}

func validateComponentSet(field string, signal SignalDefinition, components map[string]Component) error {
	if signal.Source.Inline == nil {
		return nil
	}
	var shape string
	for _, registration := range signal.Source.Inline.Registrations {
		if shape == "" {
			shape = registration.Prometheus.Shape
		}
		if shape != registration.Prometheus.Shape {
			return fmt.Errorf("%s inline registrations must use one Prometheus shape", field)
		}
	}
	roles := make([]string, 0, len(components))
	for _, component := range components {
		roles = append(roles, component.WireRole)
	}
	slices.Sort(roles)
	for _, valid := range validComponentSets(shape) {
		if slices.Equal(roles, valid) {
			return nil
		}
	}
	return fmt.Errorf("%s component wire roles %v do not match shape %q", field, roles, shape)
}

func validateComponent(
	field string,
	component Component,
	evidence map[string]SourceEvidence,
) error {
	if err := requireEnum(field+".wire_role", component.WireRole,
		"scalar", "histogram_bucket", "histogram_count", "histogram_sum",
		"summary_quantile", "summary_count", "summary_sum"); err != nil {
		return err
	}
	if err := requireEnum(field+".lifecycle.kind", component.Lifecycle.Kind, "current", "cumulative", "constant"); err != nil {
		return err
	}
	if err := validateEvidenceKinds(
		field+".lifecycle.evidence",
		component.Lifecycle.Evidence,
		evidence,
		"lifecycle",
	); err != nil {
		return err
	}
	if err := requireEnum(field+".unit.quantity",
		component.Unit.Quantity,
		"count", "data", "duration", "duration_squared", "timestamp", "ratio", "currency", "temperature", "frequency", "state"); err != nil {
		return err
	}
	for name, value := range map[string]string{
		"base":   component.Unit.Base,
		"object": component.Unit.Object,
		"aspect": component.Unit.Aspect,
	} {
		if !validID(value) {
			return fmt.Errorf("%s.unit.%s %q is not a valid lower-snake identifier", field, name, value)
		}
	}
	if _, ok := sourceRateRegistry[component.Unit.Rate]; !ok {
		return fmt.Errorf("%s.unit.rate %q is not registered", field, component.Unit.Rate)
	}
	if (component.Lifecycle.Kind == "constant" || component.Lifecycle.Kind == "cumulative") &&
		component.Unit.Rate != "none" {
		return fmt.Errorf("%s %s lifecycle requires unit rate none", field, component.Lifecycle.Kind)
	}
	if component.Unit.Quantity == "timestamp" && component.Lifecycle.Kind == "cumulative" {
		return fmt.Errorf("%s timestamp quantity cannot have cumulative lifecycle", field)
	}
	return validateEvidenceKinds(field+".unit.evidence", component.Unit.Evidence, evidence, "unit")
}

func validateSourceLabel(
	field string,
	label SourceLabel,
	evidence map[string]SourceEvidence,
) error {
	if err := requireText(field+".meaning", label.Meaning); err != nil {
		return err
	}
	if label.Presence.Kind != "" {
		if err := requireEnum(field+".presence", label.Presence.Kind, "required", "present", "optional"); err != nil {
			return err
		}
	} else if label.Presence.When.IsZero() {
		return fmt.Errorf("%s.presence must be required, present, optional, or conditional", field)
	}
	if err := requireEnum(field+".domain.kind", label.Domain.Kind, "closed", "open"); err != nil {
		return err
	}
	if label.Domain.Kind == "closed" {
		if len(label.Domain.Values) == 0 {
			return fmt.Errorf("%s.domain.values must not be empty", field)
		}
		if err := validateStringSet(field+".domain.values", label.Domain.Values, true); err != nil {
			return err
		}
	} else if len(label.Domain.Values) != 0 {
		return fmt.Errorf("%s.domain.values is allowed only for a closed domain", field)
	}
	if err := validateCardinality(
		field+".endpoint_cardinality",
		label.EndpointCardinality,
	); err != nil {
		return err
	}
	if label.EndpointCardinality.Kind == "closed_domain" && label.Domain.Kind != "closed" {
		return fmt.Errorf("%s.endpoint_cardinality closed_domain cardinality requires a closed domain", field)
	}
	if err := requireEnum(field+".stability", label.Stability, "stable", "restart_stable", "dynamic"); err != nil {
		return err
	}
	return validateEvidenceKinds(field+".evidence", label.Evidence, evidence, "label")
}

func validateSourceLabelEnvironment(
	field string,
	label SourceLabel,
	document SourceSemanticsDocument,
) error {
	if err := validateConditionUse(
		field+".presence.when",
		label.Presence.When,
		document.Environment.Axes,
		document.Environment.Policies,
		false,
	); err != nil {
		return err
	}
	if label.EndpointCardinality.Axis == "" {
		return nil
	}
	axis, ok := document.Environment.Axes[label.EndpointCardinality.Axis]
	if !ok {
		return fmt.Errorf("%s.endpoint_cardinality.axis references unknown axis %q",
			field, label.EndpointCardinality.Axis)
	}
	if axis.Kind != "integer" && axis.Kind != "enum" && axis.Kind != "ordered_enum" && axis.Kind != "enum_set" {
		return fmt.Errorf("%s.endpoint_cardinality.axis is not finite", field)
	}
	return nil
}

func validateCardinality(field string, cardinality EndpointCardinality) error {
	if err := requireEnum(field+".kind", cardinality.Kind,
		"singleton", "closed_domain", "bounded_configuration", "operational_population", "unbounded", "unknown"); err != nil {
		return err
	}
	switch cardinality.Kind {
	case "bounded_configuration":
		if cardinality.Max != nil && *cardinality.Max <= 0 {
			return fmt.Errorf("%s.max must be positive", field)
		}
		if cardinality.Max != nil && cardinality.Axis != "" {
			return fmt.Errorf("%s must declare at most one of max or axis", field)
		}
	default:
		if cardinality.Max != nil || cardinality.Axis != "" {
			return fmt.Errorf("%s kind %q rejects max/axis", field, cardinality.Kind)
		}
	}
	return nil
}

func validateEvidenceKinds(
	field string,
	references []string,
	evidence map[string]SourceEvidence,
	requiredKinds ...string,
) error {
	if err := requireList(field, references); err != nil {
		return err
	}
	found := make(map[string]struct{}, len(requiredKinds))
	for _, reference := range references {
		record, ok := evidence[reference]
		if !ok {
			return fmt.Errorf("%s references unknown evidence %q", field, reference)
		}
		if !slices.Contains(requiredKinds, record.Kind) {
			return fmt.Errorf("%s evidence %q has incompatible kind %q", field, reference, record.Kind)
		}
		found[record.Kind] = struct{}{}
	}
	for _, kind := range requiredKinds {
		if _, ok := found[kind]; !ok {
			return fmt.Errorf("%s must reference %q evidence", field, kind)
		}
	}
	return nil
}

func validateContributors(
	field string,
	contributors ContributorDefinition,
	signal SignalDefinition,
	document SourceSemanticsDocument,
) error {
	if err := validateIDMap(field+".variants", contributors.Variants, true); err != nil {
		return err
	}
	components := signal.Components
	if signal.ComponentPolicy != "" {
		components = document.ComponentPolicies[signal.ComponentPolicy]
	}
	for _, id := range sortedMapKeys(contributors.Variants) {
		variant := contributors.Variants[id]
		variantField := field + ".variants." + id
		if err := validateConditionUse(
			variantField+".when",
			variant.When,
			document.Environment.Axes,
			document.Environment.Policies,
			false,
		); err != nil {
			return err
		}
		if variant.Identity == nil {
			return fmt.Errorf("%s.identity must be present", variantField)
		}
		if err := validateLabelSet(variantField+".identity", variant.Identity, true); err != nil {
			return err
		}
		if err := validateCardinality(variantField+".cardinality", variant.Cardinality); err != nil {
			return err
		}
		if err := requireEnum(variantField+".concurrency",
			variant.Concurrency, "mutually_exclusive", "may_coexist"); err != nil {
			return err
		}
		if variant.ValueModel == nil || len(variant.ValueModel) != len(components) {
			return fmt.Errorf("%s.value_model must classify every component", variantField)
		}
		for component, model := range variant.ValueModel {
			if _, ok := components[component]; !ok {
				return fmt.Errorf("%s.value_model references unknown component %q", variantField, component)
			}
			if err := requireEnum(variantField+".value_model."+component,
				model, "additive", "comparable_point", "ordered_state", "not_reducible"); err != nil {
				return err
			}
		}
		if err := requireEnum(variantField+".membership.stability",
			variant.Membership.Stability, "stable", "restart_stable", "dynamic"); err != nil {
			return err
		}
		if err := requireEnum(variantField+".reset.scope", variant.Reset.Scope, "shared", "per_contributor"); err != nil {
			return err
		}
		if err := requireEnum(variantField+".join.new_contributor_baseline",
			variant.Join.NewContributorBaseline, "zero", "current_total", "unknown"); err != nil {
			return err
		}
		for evidenceField, values := range map[string]struct {
			refs []string
			kind string
		}{
			"population":   {variant.Evidence.Population, "population"},
			"lifecycle":    {variant.Evidence.Lifecycle, "lifecycle"},
			"relationship": {variant.Evidence.Relationship, "relationship"},
		} {
			if err := validateEvidenceKinds(
				variantField+".evidence."+evidenceField,
				values.refs,
				document.Evidence,
				values.kind,
			); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateRelationship(field string, relationship Relationship, document SourceSemanticsDocument) error {
	if err := requireEnum(
		field+".kind", relationship.Kind,
		"equivalent", "sum_projection", "partition", "subset", "overlap",
	); err != nil {
		return err
	}
	if err := validateConditionUse(
		field+".when",
		relationship.When,
		document.Environment.Axes,
		document.Environment.Policies,
		false,
	); err != nil {
		return err
	}
	if err := validateEvidenceKinds(
		field+".evidence",
		relationship.Evidence,
		document.Evidence,
		"relationship",
	); err != nil {
		return err
	}
	allowed := map[string][]string{
		"equivalent":     {"left", "right", "group_by", "when", "evidence"},
		"sum_projection": {"coarse", "fine", "group_by", "when", "evidence"},
		"partition":      {"whole", "parts", "disjoint", "exhaustive", "when", "evidence"},
		"subset":         {"subset", "superset", "when", "evidence"},
		"overlap":        {"members", "when", "evidence"},
	}[relationship.Kind]
	for _, present := range presentRelationshipFields(relationship) {
		if !slices.Contains(allowed, present) {
			return fmt.Errorf("%s.%s is not allowed for kind %s", field, present, relationship.Kind)
		}
	}
	switch relationship.Kind {
	case "equivalent":
		if err := validateSourceReference(field+".left", relationship.Left, true); err != nil {
			return err
		}
		if err := validateSourceReference(field+".right", relationship.Right, true); err != nil {
			return err
		}
		if relationship.GroupBy != nil {
			if len(relationship.GroupBy) == 0 {
				return fmt.Errorf("%s.group_by must not be empty when present", field)
			}
			if err := validateLabelSet(field+".group_by", relationship.GroupBy, false); err != nil {
				return err
			}
		}
	case "sum_projection":
		if err := validateSourceReference(field+".coarse", relationship.Coarse, true); err != nil {
			return err
		}
		if err := validateSourceReference(field+".fine", relationship.Fine, true); err != nil {
			return err
		}
		if len(relationship.Coarse.Components) != 1 || len(relationship.Fine.Components) != 1 {
			return fmt.Errorf("%s sum_projection requires one coarse and one fine component", field)
		}
		if len(relationship.GroupBy) == 0 {
			return fmt.Errorf("%s.group_by must not be empty", field)
		}
		if err := validateLabelSet(field+".group_by", relationship.GroupBy, false); err != nil {
			return err
		}
	case "partition":
		if err := validateSourceReference(field+".whole", relationship.Whole, true); err != nil {
			return err
		}
		if len(relationship.Parts) < 2 || relationship.Disjoint == nil || relationship.Exhaustive == nil {
			return fmt.Errorf("%s partition requires at least two parts plus disjoint/exhaustive", field)
		}
		for index := range relationship.Parts {
			if err := validateSourceReference(
				fmt.Sprintf("%s.parts[%d]", field, index),
				&relationship.Parts[index],
				true,
			); err != nil {
				return err
			}
		}
	case "subset":
		if err := validateSourceReference(field+".subset", relationship.Subset, true); err != nil {
			return err
		}
		if err := validateSourceReference(field+".superset", relationship.Superset, true); err != nil {
			return err
		}
	case "overlap":
		if len(relationship.Members) < 2 {
			return fmt.Errorf("%s.members must contain at least two references", field)
		}
		for index := range relationship.Members {
			if err := validateSourceReference(
				fmt.Sprintf("%s.members[%d]", field, index),
				&relationship.Members[index],
				true,
			); err != nil {
				return err
			}
		}
	}
	return validateRelationshipReferences(field, relationship, document)
}

func presentRelationshipFields(relationship Relationship) []string {
	fields := make([]string, 0, 12)
	add := func(name string, present bool) {
		if present {
			fields = append(fields, name)
		}
	}
	add("whole", relationship.Whole != nil)
	add("parts", relationship.Parts != nil)
	add("disjoint", relationship.Disjoint != nil)
	add("exhaustive", relationship.Exhaustive != nil)
	add("left", relationship.Left != nil)
	add("right", relationship.Right != nil)
	add("subset", relationship.Subset != nil)
	add("superset", relationship.Superset != nil)
	add("members", relationship.Members != nil)
	add("coarse", relationship.Coarse != nil)
	add("fine", relationship.Fine != nil)
	add("group_by", relationship.GroupBy != nil)
	add("when", !relationship.When.IsZero())
	add("evidence", relationship.Evidence != nil)
	return fields
}

func validateRelationshipReferences(
	field string,
	relationship Relationship,
	document SourceSemanticsDocument,
) error {
	references := []*SourceReference{
		relationship.Whole,
		relationship.Left,
		relationship.Right,
		relationship.Subset,
		relationship.Superset,
		relationship.Coarse,
		relationship.Fine,
	}
	for index := range relationship.Parts {
		references = append(references, &relationship.Parts[index])
	}
	for index := range relationship.Members {
		references = append(references, &relationship.Members[index])
	}
	for _, reference := range references {
		if reference == nil {
			continue
		}
		signal, ok := document.Signals[reference.Signal]
		if !ok {
			return fmt.Errorf("%s references unknown signal %q", field, reference.Signal)
		}
		components := signal.Components
		if signal.ComponentPolicy != "" {
			components = document.ComponentPolicies[signal.ComponentPolicy]
		}
		for _, component := range reference.Components {
			if _, ok := components[component]; !ok {
				return fmt.Errorf("%s references unknown component %q on signal %q", field, component, reference.Signal)
			}
		}
	}
	return nil
}

func validateStateEncoding(field string, encoding StateEncoding, document SourceSemanticsDocument) error {
	signal, ok := document.Signals[encoding.Signal]
	if !ok {
		return fmt.Errorf("%s.signal references unknown signal %q", field, encoding.Signal)
	}
	components := signal.Components
	if signal.ComponentPolicy != "" {
		components = document.ComponentPolicies[signal.ComponentPolicy]
	}
	if _, ok := components[encoding.Component]; !ok {
		return fmt.Errorf("%s.component references unknown component %q", field, encoding.Component)
	}
	component := components[encoding.Component]
	if component.Lifecycle.Kind != "current" || component.Unit.Quantity != "state" {
		return fmt.Errorf("%s.component must reference a current state component", field)
	}
	labels := signal.Labels
	if signal.LabelPolicy != "" {
		labels = document.LabelPolicies[signal.LabelPolicy]
	}
	label, ok := labels[encoding.Label]
	if !ok {
		return fmt.Errorf("%s.label references unknown label %q", field, encoding.Label)
	}
	if label.Presence.Kind != "required" {
		return fmt.Errorf("%s.label must reference a required label", field)
	}
	if err := requireList(field+".states", encoding.States); err != nil {
		return err
	}
	if label.Domain.Kind != "closed" || !slices.Equal(label.Domain.Values, encoding.States) {
		return fmt.Errorf("%s.states must equal the source label's closed domain", field)
	}
	if err := requireEnum(field+".encoding", encoding.Encoding, "one_hot_exactly_one", "bitset_zero_or_one"); err != nil {
		return err
	}
	if err := validateConditionUse(
		field+".when",
		encoding.When,
		document.Environment.Axes,
		document.Environment.Policies,
		false,
	); err != nil {
		return err
	}
	return validateEvidenceKinds(field+".evidence", encoding.Evidence, document.Evidence, "state_encoding")
}

func validateSourceExclusion(
	field string,
	exclusion SourceExclusion,
	document SourceSemanticsDocument,
) error {
	if err := requireList(field+".registrations", exclusion.Registrations); err != nil {
		return err
	}
	for index, registration := range exclusion.Registrations {
		if !validID(registration) {
			return fmt.Errorf("%s.registrations[%d] %q is not a valid ID", field, index, registration)
		}
	}
	if err := requireEnum(field+".reason", exclusion.Reason, "non_emitting", "out_of_endpoint"); err != nil {
		return err
	}
	if err := validateConditionUse(
		field+".when",
		exclusion.When,
		document.Environment.Axes,
		document.Environment.Policies,
		false,
	); err != nil {
		return err
	}
	requiredKind := "registration"
	if exclusion.Reason == "out_of_endpoint" {
		requiredKind = "availability"
	}
	return validateEvidenceKinds(field+".evidence", exclusion.Evidence, document.Evidence, requiredKind)
}
