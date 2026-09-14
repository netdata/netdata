// SPDX-License-Identifier: GPL-3.0-or-later

package promsemantics

type SourceSemanticsDocument struct {
	Version           string                            `yaml:"version"`
	Profile           string                            `yaml:"profile"`
	Upstreams         map[string]SourceUpstream         `yaml:"upstreams"`
	Evidence          map[string]SourceEvidence         `yaml:"evidence"`
	Environment       EnvironmentSchema                 `yaml:"environment"`
	ComponentPolicies map[string]map[string]Component   `yaml:"component_policies,omitempty"`
	LabelPolicies     map[string]map[string]SourceLabel `yaml:"label_policies,omitempty"`
	Signals           map[string]SignalDefinition       `yaml:"signals"`
	Relationships     map[string]Relationship           `yaml:"relationships,omitempty"`
	StateEncodings    map[string]StateEncoding          `yaml:"state_encodings,omitempty"`
	SourceExclusions  map[string]SourceExclusion        `yaml:"source_exclusions,omitempty"`
}

type SourceUpstream struct {
	Repository string `yaml:"repository"`
	Commit     string `yaml:"commit"`
}

type SourceEvidence struct {
	Kind      string   `yaml:"kind"`
	Upstream  string   `yaml:"upstream"`
	Locations []string `yaml:"locations"`
	Claim     string   `yaml:"claim"`
}

type EnvironmentSchema struct {
	Axes     map[string]EnvironmentAxis   `yaml:"axes"`
	Policies map[string]EnvironmentPolicy `yaml:"policies"`
}

type EnvironmentAxis struct {
	Kind     string   `yaml:"kind"`
	Values   []string `yaml:"values,omitempty"`
	Min      *int     `yaml:"min,omitempty"`
	Max      *int     `yaml:"max,omitempty"`
	Meaning  string   `yaml:"meaning"`
	Evidence []string `yaml:"evidence"`
}

type EnvironmentPolicy struct {
	When     EnvironmentCondition `yaml:"when"`
	Evidence []string             `yaml:"evidence"`
}

type SignalDefinition struct {
	Availability             ConditionUse                       `yaml:"availability,omitempty"`
	Source                   SignalSource                       `yaml:"source"`
	Population               SignalPopulation                   `yaml:"population"`
	Components               map[string]Component               `yaml:"components,omitempty"`
	ComponentPolicy          string                             `yaml:"component_policy,omitempty"`
	Labels                   map[string]SourceLabel             `yaml:"labels,omitempty"`
	LabelPolicy              string                             `yaml:"label_policy,omitempty"`
	LabelPresenceConstraints map[string]LabelPresenceConstraint `yaml:"label_presence_constraints,omitempty"`
	FunctionalDependencies   map[string]FunctionalDependency    `yaml:"functional_dependencies"`
	Contributors             *ContributorDefinition             `yaml:"contributors,omitempty"`
}

type LabelPresenceConstraint struct {
	Kind         string     `yaml:"kind"`
	Alternatives [][]string `yaml:"alternatives"`
	Evidence     []string   `yaml:"evidence"`
}

type SignalSource struct {
	Inline    *InlineSource    `yaml:"inline,omitempty"`
	Generated *GeneratedSource `yaml:"generated,omitempty"`
}

type InlineSource struct {
	Registrations map[string]InlineRegistration `yaml:"registrations"`
}

type InlineRegistration struct {
	Family     FamilySelector     `yaml:"family"`
	Prometheus PrometheusContract `yaml:"prometheus"`
	When       ConditionUse       `yaml:"when,omitempty"`
	Evidence   []string           `yaml:"evidence"`
}

type GeneratedSource struct {
	RegistryGroups []string             `yaml:"registry_groups"`
	Scope          GeneratedSourceScope `yaml:"scope"`
}

type GeneratedSourceScope struct {
	Registrations []string             `yaml:"registrations,omitempty"`
	Families      GeneratedFamilyScope `yaml:"families,omitempty"`
}

type GeneratedFamilyScope struct {
	Exact    []string `yaml:"exact,omitempty"`
	Grammars []string `yaml:"grammars,omitempty"`
}

type SignalPopulation struct {
	ID       string   `yaml:"id"`
	Meaning  string   `yaml:"meaning"`
	Evidence []string `yaml:"evidence"`
}

type Component struct {
	WireRole  string             `yaml:"wire_role"`
	Lifecycle ComponentLifecycle `yaml:"lifecycle"`
	Unit      ComponentUnit      `yaml:"unit"`
}

type ComponentLifecycle struct {
	Kind     string   `yaml:"kind"`
	Evidence []string `yaml:"evidence"`
}

type ComponentUnit struct {
	Quantity string   `yaml:"quantity"`
	Base     string   `yaml:"base"`
	Rate     string   `yaml:"rate"`
	Object   string   `yaml:"object"`
	Aspect   string   `yaml:"aspect"`
	Evidence []string `yaml:"evidence"`
}

type SourceLabel struct {
	Meaning             string              `yaml:"meaning"`
	Presence            LabelPresence       `yaml:"presence"`
	Domain              LabelDomain         `yaml:"domain"`
	EndpointCardinality EndpointCardinality `yaml:"endpoint_cardinality"`
	Stability           string              `yaml:"stability"`
	Evidence            []string            `yaml:"evidence"`
}

type FunctionalDependency struct {
	Determinants []string     `yaml:"determinants"`
	Dependents   []string     `yaml:"dependents"`
	When         ConditionUse `yaml:"when,omitempty"`
	Evidence     []string     `yaml:"evidence"`
}

type ContributorDefinition struct {
	Variants map[string]ContributorVariant `yaml:"variants"`
}

type ContributorVariant struct {
	When        ConditionUse          `yaml:"when,omitempty"`
	Identity    []string              `yaml:"identity"`
	Cardinality EndpointCardinality   `yaml:"cardinality"`
	Concurrency string                `yaml:"concurrency"`
	ValueModel  map[string]string     `yaml:"value_model"`
	Membership  ContributorMembership `yaml:"membership"`
	Reset       ContributorReset      `yaml:"reset"`
	Join        ContributorJoin       `yaml:"join"`
	Evidence    ContributorEvidence   `yaml:"evidence"`
}

type ContributorMembership struct {
	Stability string `yaml:"stability"`
}

type ContributorReset struct {
	Scope string `yaml:"scope"`
}

type ContributorJoin struct {
	NewContributorBaseline string `yaml:"new_contributor_baseline"`
}

type ContributorEvidence struct {
	Population   []string `yaml:"population"`
	Lifecycle    []string `yaml:"lifecycle"`
	Relationship []string `yaml:"relationship"`
}

type Relationship struct {
	Kind       string            `yaml:"kind"`
	Whole      *SourceReference  `yaml:"whole,omitempty"`
	Parts      []SourceReference `yaml:"parts,omitempty"`
	Disjoint   *bool             `yaml:"disjoint,omitempty"`
	Exhaustive *bool             `yaml:"exhaustive,omitempty"`
	Left       *SourceReference  `yaml:"left,omitempty"`
	Right      *SourceReference  `yaml:"right,omitempty"`
	Subset     *SourceReference  `yaml:"subset,omitempty"`
	Superset   *SourceReference  `yaml:"superset,omitempty"`
	Members    []SourceReference `yaml:"members,omitempty"`
	Coarse     *SourceReference  `yaml:"coarse,omitempty"`
	Fine       *SourceReference  `yaml:"fine,omitempty"`
	GroupBy    []string          `yaml:"group_by,omitempty"`
	When       ConditionUse      `yaml:"when,omitempty"`
	Evidence   []string          `yaml:"evidence"`
}

type StateEncoding struct {
	Signal    string       `yaml:"signal"`
	Component string       `yaml:"component"`
	Label     string       `yaml:"label"`
	States    []string     `yaml:"states"`
	Encoding  string       `yaml:"encoding"`
	When      ConditionUse `yaml:"when,omitempty"`
	Evidence  []string     `yaml:"evidence"`
}

type SourceExclusion struct {
	Registrations []string     `yaml:"registrations"`
	Reason        string       `yaml:"reason"`
	When          ConditionUse `yaml:"when,omitempty"`
	Evidence      []string     `yaml:"evidence"`
}
