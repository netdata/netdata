// SPDX-License-Identifier: GPL-3.0-or-later

package promsemantics

type SourceRegistryDocument struct {
	Version        string                   `yaml:"version"`
	Profile        string                   `yaml:"profile"`
	Generated      *bool                    `yaml:"generated"`
	FamilyGrammars map[string]FamilyGrammar `yaml:"family_grammars"`
	Groups         map[string]RegistryGroup `yaml:"groups"`
}

type FamilyGrammar struct {
	Interpretation string                 `yaml:"interpretation,omitempty"`
	Forms          map[string]GrammarForm `yaml:"forms"`
}

type GrammarForm struct {
	Exact     string           `yaml:"exact,omitempty"`
	Canonical *GrammarAffix    `yaml:"canonical,omitempty"`
	Embedded  *GrammarEmbedded `yaml:"embedded,omitempty"`
}

type GrammarAffix struct {
	Prefix string `yaml:"prefix"`
	Suffix string `yaml:"suffix"`
}

type GrammarEmbedded struct {
	Prefix           string       `yaml:"prefix"`
	ExcludedPrefixes []string     `yaml:"excluded_prefixes,omitempty"`
	Suffix           string       `yaml:"suffix"`
	Separator        string       `yaml:"separator"`
	IdentitySlot     IdentitySlot `yaml:"identity_slot"`
}

type IdentitySlot struct {
	Name     string `yaml:"name"`
	Nonempty *bool  `yaml:"nonempty"`
}

type RegistryGroup struct {
	Registrations map[string]RegistryRegistration `yaml:"registrations"`
}

type RegistryRegistration struct {
	Family          FamilySelector               `yaml:"family"`
	RawBranches     map[string]RegistryRawBranch `yaml:"raw_branches,omitempty"`
	Prometheus      PrometheusContract           `yaml:"prometheus"`
	Components      map[string]RegistryComponent `yaml:"components"`
	When            ConditionUse                 `yaml:"when,omitempty"`
	SourceLocations []GeneratedSourceLocation    `yaml:"source_locations"`
}

type RegistryRawBranch struct {
	When ConditionUse `yaml:"when,omitempty"`
}

type RegistryComponent struct {
	WireRole string `yaml:"wire_role"`
}

type GeneratedSourceLocation struct {
	Upstream string           `yaml:"upstream"`
	Path     string           `yaml:"path"`
	Line     *int             `yaml:"line,omitempty"`
	Range    *SourceLineRange `yaml:"range,omitempty"`
}

type SourceLineRange struct {
	Start int `yaml:"start"`
	End   int `yaml:"end"`
}

type SourceRegistryGeneratorDocument struct {
	Version   string                       `yaml:"version"`
	Profile   string                       `yaml:"profile"`
	Runner    string                       `yaml:"runner"`
	Upstreams map[string]GeneratorUpstream `yaml:"upstreams"`
}

type GeneratorUpstream struct {
	Repository string   `yaml:"repository"`
	Commit     string   `yaml:"commit"`
	Paths      []string `yaml:"paths"`
}

type SourceRegistryPair struct {
	Registry  SourceRegistryDocument
	Generator SourceRegistryGeneratorDocument
}
