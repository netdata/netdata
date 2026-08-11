// SPDX-License-Identifier: GPL-3.0-or-later

package promsemantics

import (
	"fmt"
	"slices"
)

type sourceFamilyComponentKey struct {
	family    string
	component string
}

type compiledSourceIndex struct {
	exact            map[sourceFamilyComponentKey][]compiledSourceEntry
	embeddedByPrefix map[string][]compiledSourceEntry
	prefixLengths    []int
}

type compiledSourceEntry struct {
	registration   *compiledRegistration
	owner          compiledRegistrationOwner
	occurrence     *compiledOccurrence
	wireRole       string
	grammar        string
	formID         string
	interpretation string
	canonical      *GrammarAffix
	embedded       *GrammarEmbedded
	rawBranch      string
	availability   compiledEnvironmentCondition
}

func (c *semanticCompiler) compileSourceIndex() error {
	index := compiledSourceIndex{
		exact:            make(map[sourceFamilyComponentKey][]compiledSourceEntry),
		embeddedByPrefix: make(map[string][]compiledSourceEntry),
	}
	prefixes := make(map[int]struct{})
	for _, registrationKey := range sortedMapKeys(c.program.registrations) {
		registration := c.program.registrations[registrationKey]
		language, err := c.registrationLanguage(registration)
		if err != nil {
			return err
		}
		for _, wireRole := range sortedWireRoles(registration.components) {
			for _, owner := range registration.owners {
				base := compiledSourceEntry{
					registration: registration,
					owner:        owner,
					wireRole:     wireRole,
					grammar:      registration.family.Grammar,
					formID:       registration.family.Form,
				}
				if base.grammar != "" {
					grammar := c.input.Contract.Registry.Registry.FamilyGrammars[base.grammar]
					base.interpretation = grammar.Interpretation
					if base.interpretation == "" {
						base.interpretation = "injective"
					}
					form := grammar.Forms[base.formID]
					if form.Canonical != nil {
						canonical := *form.Canonical
						base.canonical = &canonical
					}
					if form.Embedded != nil {
						embedded := *form.Embedded
						base.embedded = &embedded
					}
				}
				if owner.kind == "signal" {
					component, ok := owner.componentByWireRole[wireRole]
					if !ok {
						return fmt.Errorf("registration %q signal owner %q has no component for wire role %q",
							registration.key, owner.id, wireRole)
					}
					base.occurrence = c.program.occurrences[owner.id+"/"+registration.key+"/"+component]
					if base.occurrence == nil {
						return fmt.Errorf("registration %q signal owner %q has no compiled occurrence for component %q",
							registration.key, owner.id, component)
					}
				}
				for _, family := range language.exact {
					entry := base
					entry.rawBranch = "exact"
					entry.availability = owner.availability
					if language.exactAvailability != nil {
						entry.rawBranch = "canonical"
						entry.availability = owner.availability.and(language.exactAvailability[family], c.environment.axes)
					}
					if len(entry.availability.clauses) == 0 {
						continue
					}
					key := sourceFamilyComponentKey{family: family, component: wireRole}
					index.exact[key] = append(index.exact[key], entry)
				}
				if language.embedded != nil {
					entry := base
					entry.rawBranch = "embedded"
					entry.availability = owner.availability.and(language.embedded.availability, c.environment.axes)
					if len(entry.availability.clauses) == 0 {
						continue
					}
					index.embeddedByPrefix[entry.embedded.Prefix] = append(index.embeddedByPrefix[entry.embedded.Prefix], entry)
					prefixes[len(entry.embedded.Prefix)] = struct{}{}
				}
			}
		}
	}
	index.prefixLengths = make([]int, 0, len(prefixes))
	for length := range prefixes {
		index.prefixLengths = append(index.prefixLengths, length)
	}
	slices.Sort(index.prefixLengths)
	slices.Reverse(index.prefixLengths)
	c.program.sourceIndex = index
	return nil
}
