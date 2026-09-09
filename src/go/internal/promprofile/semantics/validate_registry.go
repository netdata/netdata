// SPDX-License-Identifier: GPL-3.0-or-later

package promsemantics

import (
	"fmt"
	"slices"
	"strings"
)

func (d SourceRegistryDocument) validate() error {
	if err := validateIdentity("source registry", d.Version, SourceRegistryVersion, d.Profile); err != nil {
		return err
	}
	if d.Generated == nil || !*d.Generated {
		return fmt.Errorf("generated must be true")
	}
	if err := validateIDMap("family_grammars", d.FamilyGrammars, false); err != nil {
		return err
	}
	if err := validateIDMap("groups", d.Groups, true); err != nil {
		return err
	}
	for _, id := range sortedMapKeys(d.FamilyGrammars) {
		if err := validateFamilyGrammar("family_grammars."+id, d.FamilyGrammars[id]); err != nil {
			return err
		}
	}

	registrationOwner := make(map[string]string)
	usedForms := make(map[string]map[string]struct{})
	for _, groupID := range sortedMapKeys(d.Groups) {
		group := d.Groups[groupID]
		if err := validateIDMap("groups."+groupID+".registrations", group.Registrations, true); err != nil {
			return err
		}
		for _, registrationID := range sortedMapKeys(group.Registrations) {
			if previous := registrationOwner[registrationID]; previous != "" {
				return fmt.Errorf("registration %q occurs in groups %q and %q", registrationID, previous, groupID)
			}
			registrationOwner[registrationID] = groupID
			registration := group.Registrations[registrationID]
			field := "groups." + groupID + ".registrations." + registrationID
			if err := validateRegistryRegistration(field, registration, d.FamilyGrammars); err != nil {
				return err
			}
			if registration.Family.Grammar != "" {
				if usedForms[registration.Family.Grammar] == nil {
					usedForms[registration.Family.Grammar] = make(map[string]struct{})
				}
				usedForms[registration.Family.Grammar][registration.Family.Form] = struct{}{}
			}
		}
	}
	for grammarID, grammar := range d.FamilyGrammars {
		for formID := range grammar.Forms {
			if _, ok := usedForms[grammarID][formID]; !ok {
				return fmt.Errorf("family grammar %q form %q is unused", grammarID, formID)
			}
		}
	}
	return nil
}

func validateFamilyGrammar(field string, grammar FamilyGrammar) error {
	if grammar.Interpretation == "" {
		grammar.Interpretation = "injective"
	}
	if err := requireEnum(field+".interpretation", grammar.Interpretation, "injective", "longest_known_suffix"); err != nil {
		return err
	}
	if err := validateIDMap(field+".forms", grammar.Forms, true); err != nil {
		return err
	}
	canonicalFamilies := make(map[string]string)
	embedded := make([]struct {
		id   string
		form GrammarEmbedded
	}, 0, len(grammar.Forms))
	for _, id := range sortedMapKeys(grammar.Forms) {
		form := grammar.Forms[id]
		formField := field + ".forms." + id
		if form.Exact != "" {
			if form.Canonical != nil || form.Embedded != nil {
				return fmt.Errorf("%s must declare exactly exact or canonical+embedded", formField)
			}
			if err := validateMetricName(formField+".exact", form.Exact); err != nil {
				return err
			}
			if previous := canonicalFamilies[form.Exact]; previous != "" {
				return fmt.Errorf("%s exact family duplicates form %q", formField, previous)
			}
			canonicalFamilies[form.Exact] = id
			continue
		}
		if form.Canonical == nil || form.Embedded == nil {
			return fmt.Errorf("%s must declare exactly exact or canonical+embedded", formField)
		}
		if err := validateGrammarAffix(formField+".canonical", *form.Canonical); err != nil {
			return err
		}
		if err := validateGrammarEmbedded(formField+".embedded", *form.Embedded); err != nil {
			return err
		}
		canonical := form.Canonical.Prefix + form.Canonical.Suffix
		if err := validateMetricName(formField+".canonical family", canonical); err != nil {
			return err
		}
		if previous := canonicalFamilies[canonical]; previous != "" {
			return fmt.Errorf("%s canonical family duplicates form %q", formField, previous)
		}
		canonicalFamilies[canonical] = id
		embedded = append(embedded, struct {
			id   string
			form GrammarEmbedded
		}{id: id, form: *form.Embedded})
	}
	if grammar.Interpretation == "injective" {
		for left := 0; left < len(embedded); left++ {
			for right := left + 1; right < len(embedded); right++ {
				if embeddedFormsOverlap(embedded[left].form, embedded[right].form) {
					return fmt.Errorf("%s forms %q and %q have a noninjective embedded language",
						field, embedded[left].id, embedded[right].id)
				}
			}
		}
	} else {
		ambiguousPair := false
		for left := 0; left < len(embedded); left++ {
			for right := left + 1; right < len(embedded); right++ {
				if embeddedFormsOverlap(embedded[left].form, embedded[right].form) {
					ambiguousPair = true
				}
			}
		}
		if !ambiguousPair {
			return fmt.Errorf("%s longest_known_suffix is unnecessary without overlapping embedded forms", field)
		}
	}
	return nil
}

func validateGrammarAffix(field string, affix GrammarAffix) error {
	return requireText(field+".prefix", affix.Prefix)
}

func validateGrammarEmbedded(field string, embedded GrammarEmbedded) error {
	if err := requireText(field+".prefix", embedded.Prefix); err != nil {
		return err
	}
	if err := validateStringSet(field+".excluded_prefixes", embedded.ExcludedPrefixes, true); err != nil {
		return err
	}
	for index, prefix := range embedded.ExcludedPrefixes {
		if len(prefix) <= len(embedded.Prefix) || !strings.HasPrefix(prefix, embedded.Prefix) {
			return fmt.Errorf("%s.excluded_prefixes[%d] %q must be a proper nested prefix of %q",
				field, index, prefix, embedded.Prefix)
		}
		if err := validateMetricName(
			fmt.Sprintf("%s.excluded_prefixes[%d] representative family", field, index),
			prefix+"x",
		); err != nil {
			return err
		}
	}
	if (embedded.Suffix == "") != (embedded.Separator == "") {
		return fmt.Errorf("%s suffix and separator must be both empty or both nonempty", field)
	}
	if !validID(embedded.IdentitySlot.Name) {
		return fmt.Errorf("%s.identity_slot.name %q is not a valid ID", field, embedded.IdentitySlot.Name)
	}
	if embedded.IdentitySlot.Nonempty == nil || !*embedded.IdentitySlot.Nonempty {
		return fmt.Errorf("%s.identity_slot.nonempty must be true", field)
	}
	return validateMetricName(field+".representative family",
		embedded.Prefix+"x"+embedded.Separator+embedded.Suffix)
}

func embeddedFormsOverlap(left, right GrammarEmbedded) bool {
	prefixRelated := strings.HasPrefix(left.Prefix, right.Prefix) || strings.HasPrefix(right.Prefix, left.Prefix)
	if !prefixRelated {
		return false
	}
	leftTail := left.Separator + left.Suffix
	rightTail := right.Separator + right.Suffix
	if !strings.HasSuffix(leftTail, rightTail) && !strings.HasSuffix(rightTail, leftTail) {
		return false
	}
	if len(left.Prefix) < len(right.Prefix) && embeddedPrefixExcluded(left, right.Prefix) {
		return false
	}
	if len(right.Prefix) < len(left.Prefix) && embeddedPrefixExcluded(right, left.Prefix) {
		return false
	}
	return true
}

func embeddedPrefixExcluded(embedded GrammarEmbedded, prefix string) bool {
	return slices.ContainsFunc(embedded.ExcludedPrefixes, func(excluded string) bool {
		return strings.HasPrefix(prefix, excluded)
	})
}

func validateRegistryRegistration(
	field string,
	registration RegistryRegistration,
	grammars map[string]FamilyGrammar,
) error {
	if err := validateFamilySelector(field+".family", registration.Family, false); err != nil {
		return err
	}
	grammarFormHasBranches := false
	if registration.Family.Exact == "" {
		grammar, ok := grammars[registration.Family.Grammar]
		if !ok {
			return fmt.Errorf("%s.family references unknown grammar %q", field, registration.Family.Grammar)
		}
		form, ok := grammar.Forms[registration.Family.Form]
		if !ok {
			return fmt.Errorf("%s.family references unknown form %q", field, registration.Family.Form)
		}
		grammarFormHasBranches = form.Canonical != nil && form.Embedded != nil
	}
	if grammarFormHasBranches {
		if err := validateIDMap(field+".raw_branches", registration.RawBranches, true); err != nil {
			return err
		}
		for _, branch := range sortedMapKeys(registration.RawBranches) {
			if err := requireEnum(field+".raw_branches branch", branch, "canonical", "embedded"); err != nil {
				return err
			}
			if registration.RawBranches[branch].When.Policy != "" {
				return fmt.Errorf("%s.raw_branches.%s.when must be an inline mechanical condition", field, branch)
			}
		}
	} else if len(registration.RawBranches) != 0 {
		return fmt.Errorf("%s.raw_branches is allowed only for a canonical+embedded grammar form", field)
	}
	if err := validatePrometheusContract(field+".prometheus", registration.Prometheus); err != nil {
		return err
	}
	if registration.When.Policy != "" {
		return fmt.Errorf("%s.when must be an inline mechanical condition", field)
	}
	if err := validateIDMap(field+".components", registration.Components, true); err != nil {
		return err
	}
	roles := make([]string, 0, len(registration.Components))
	for _, id := range sortedMapKeys(registration.Components) {
		role := registration.Components[id].WireRole
		if err := requireEnum(field+".components."+id+".wire_role",
			role, "scalar", "histogram_bucket", "histogram_count", "histogram_sum",
			"summary_quantile", "summary_count", "summary_sum"); err != nil {
			return err
		}
		roles = append(roles, role)
	}
	slices.Sort(roles)
	if !slices.ContainsFunc(validComponentSets(registration.Prometheus.Shape), func(valid []string) bool {
		return slices.Equal(valid, roles)
	}) {
		return fmt.Errorf("%s component wire roles %v do not match shape %q",
			field, roles, registration.Prometheus.Shape)
	}
	if len(registration.SourceLocations) == 0 {
		return fmt.Errorf("%s.source_locations must not be empty", field)
	}
	for index, location := range registration.SourceLocations {
		if err := validateGeneratedSourceLocation(
			fmt.Sprintf("%s.source_locations[%d]", field, index),
			location,
		); err != nil {
			return err
		}
	}
	return nil
}

func validateGeneratedSourceLocation(field string, location GeneratedSourceLocation) error {
	if !validID(location.Upstream) {
		return fmt.Errorf("%s.upstream %q is not a valid ID", field, location.Upstream)
	}
	if err := validateRepositoryRelativePath(field+".path", location.Path); err != nil {
		return err
	}
	if (location.Line == nil) == (location.Range == nil) {
		return fmt.Errorf("%s must declare exactly line or range", field)
	}
	if location.Line != nil && *location.Line <= 0 {
		return fmt.Errorf("%s.line must be positive", field)
	}
	if location.Range != nil &&
		(location.Range.Start <= 0 || location.Range.End < location.Range.Start) {
		return fmt.Errorf("%s.range must be a positive ordered range", field)
	}
	return nil
}

func (d SourceRegistryGeneratorDocument) validate() error {
	if err := validateIdentity(
		"source registry generator",
		d.Version,
		SourceRegistryGeneratorVersion,
		d.Profile,
	); err != nil {
		return err
	}
	if d.Runner != "netdata-prometheus-source-registry-v1" {
		return fmt.Errorf("runner %q is unsupported", d.Runner)
	}
	if err := validateIDMap("upstreams", d.Upstreams, true); err != nil {
		return err
	}
	for _, id := range sortedMapKeys(d.Upstreams) {
		upstream := d.Upstreams[id]
		if err := validateSourceUpstream("upstreams."+id, SourceUpstream{
			Repository: upstream.Repository,
			Commit:     upstream.Commit,
		}); err != nil {
			return err
		}
		if err := requireList("upstreams."+id+".paths", upstream.Paths); err != nil {
			return err
		}
		for index, sourcePath := range upstream.Paths {
			if err := validateRepositoryRelativePath(
				fmt.Sprintf("upstreams.%s.paths[%d]", id, index),
				sourcePath,
			); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p SourceRegistryPair) validate() error {
	if p.Registry.Profile != p.Generator.Profile {
		return fmt.Errorf("profile mismatch: registry %q, generator %q", p.Registry.Profile, p.Generator.Profile)
	}
	for groupID, group := range p.Registry.Groups {
		for registrationID, registration := range group.Registrations {
			for index, location := range registration.SourceLocations {
				upstream, ok := p.Generator.Upstreams[location.Upstream]
				if !ok {
					return fmt.Errorf("groups.%s.registrations.%s.source_locations[%d] references unknown upstream %q",
						groupID, registrationID, index, location.Upstream)
				}
				if !slices.Contains(upstream.Paths, location.Path) {
					return fmt.Errorf(
						"groups.%s.registrations.%s.source_locations[%d] path %q is outside upstream %q declared path closure",
						groupID,
						registrationID,
						index,
						location.Path,
						location.Upstream,
					)
				}
			}
		}
	}
	return nil
}
