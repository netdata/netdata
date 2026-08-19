// SPDX-License-Identifier: GPL-3.0-or-later

package promsemantics

type ProfileDesignDocument struct {
	Version           string                          `yaml:"version"`
	Profile           string                          `yaml:"profile"`
	Match             string                          `yaml:"match"`
	App               *string                         `yaml:"app,omitempty"`
	Namespace         string                          `yaml:"namespace"`
	Composition       DesignComposition               `yaml:"composition"`
	Entities          map[string]EntityDefinition     `yaml:"entities"`
	LabelPolicies     map[string]ViewLabels           `yaml:"label_policies,omitempty"`
	ReductionPolicies map[string]ReductionDefinition  `yaml:"reduction_policies,omitempty"`
	Normalizations    map[string]Normalization        `yaml:"normalizations,omitempty"`
	Exclusions        map[string]DesignExclusion      `yaml:"exclusions,omitempty"`
	Limitations       map[string]CumulativeLimitation `yaml:"limitations,omitempty"`
	Views             map[string]ViewDefinition       `yaml:"views"`
}

type DesignComposition struct {
	Supports map[string]SupportDependency `yaml:"supports"`
}

type SupportDependency struct {
	When ConditionUse `yaml:"when,omitempty"`
}

type EntityDefinition struct {
	Grain                     string                     `yaml:"grain"`
	Identity                  EntityIdentity             `yaml:"identity"`
	HighCardinalityAcceptance *HighCardinalityAcceptance `yaml:"high_cardinality_acceptance,omitempty"`
}

type EntityIdentity struct {
	Required     []string   `yaml:"required"`
	Alternatives [][]string `yaml:"alternatives,omitempty"`
	Optional     []string   `yaml:"optional"`
}

type HighCardinalityAcceptance struct {
	OperatorValue string `yaml:"operator_value"`
}

type ViewDefinition struct {
	Family          string               `yaml:"family"`
	Question        string               `yaml:"question"`
	Entity          string               `yaml:"entity"`
	Inputs          map[string]ViewInput `yaml:"inputs"`
	Labels          *ViewLabels          `yaml:"labels,omitempty"`
	LabelPolicy     string               `yaml:"label_policy,omitempty"`
	Reduction       *ReductionDefinition `yaml:"reduction,omitempty"`
	ReductionPolicy string               `yaml:"reduction_policy,omitempty"`
	Display         *DisplayDefinition   `yaml:"display,omitempty"`
	Presentation    *PresentationIntent  `yaml:"presentation,omitempty"`
}

type ViewInput struct {
	Signal     string             `yaml:"signal"`
	Components []string           `yaml:"components"`
	Where      *LabelCondition    `yaml:"where,omitempty"`
	RenderAs   string             `yaml:"render_as,omitempty"`
	Direction  *DirectionIntent   `yaml:"direction,omitempty"`
	Algorithm  *AlgorithmOverride `yaml:"algorithm,omitempty"`
}

type DirectionIntent struct {
	Negative *bool    `yaml:"negative"`
	Reason   string   `yaml:"reason"`
	Evidence []string `yaml:"evidence"`
}

type AlgorithmOverride struct {
	Value    string   `yaml:"value"`
	Reason   string   `yaml:"reason"`
	Evidence []string `yaml:"evidence"`
}

type ViewLabels struct {
	Dimensions map[string]DimensionRendering `yaml:"dimensions"`
	Promote    []string                      `yaml:"promote"`
	Omit       map[string]string             `yaml:"omit"`
}

type DimensionRendering struct {
	Render string `yaml:"render"`
}

type ReductionDefinition struct {
	Reducer        string `yaml:"reducer"`
	LostComparison string `yaml:"lost_comparison"`
}

type DisplayDefinition struct {
	Convention string   `yaml:"convention"`
	Reason     string   `yaml:"reason"`
	Evidence   []string `yaml:"evidence"`
}

type PresentationIntent struct {
	Type         string `yaml:"type"`
	Relationship string `yaml:"relationship,omitempty"`
	Reason       string `yaml:"reason"`
}

type Normalization struct {
	Kind                string                      `yaml:"kind"`
	AppliesTo           *SourceReference            `yaml:"applies_to,omitempty"`
	SourceLabel         string                      `yaml:"source_label,omitempty"`
	TargetLabel         string                      `yaml:"target_label,omitempty"`
	RetainSource        *bool                       `yaml:"retain_source,omitempty"`
	Exact               map[string]string           `yaml:"exact,omitempty"`
	Ranges              []CategoryRange             `yaml:"ranges,omitempty"`
	Missing             *CategoryAction             `yaml:"missing,omitempty"`
	Malformed           *CategoryAction             `yaml:"malformed,omitempty"`
	Unknown             *CategoryAction             `yaml:"unknown,omitempty"`
	SourceFamily        map[string]string           `yaml:"source_family,omitempty"`
	RegistryGroup       string                      `yaml:"registry_group,omitempty"`
	SourcePrefix        string                      `yaml:"source_prefix,omitempty"`
	TargetPrefix        string                      `yaml:"target_prefix,omitempty"`
	RegistryGrammar     string                      `yaml:"registry_grammar,omitempty"`
	SourceIdentityLabel string                      `yaml:"source_identity_label,omitempty"`
	Canonical           *IdentityRepairCanonical    `yaml:"canonical,omitempty"`
	Embedded            *IdentityRepairEmbedded     `yaml:"embedded,omitempty"`
	Identity            *IdentityJoin               `yaml:"identity,omitempty"`
	DuplicateExclusion  *IdentityDuplicateExclusion `yaml:"duplicate_exclusion,omitempty"`
	Retain              *EmbeddedIdentityRetain     `yaml:"retain,omitempty"`
	Source              *GeneratedComponentScope    `yaml:"source,omitempty"`
	Outcome             string                      `yaml:"outcome,omitempty"`
	Output              *NormalizedLabelOutput      `yaml:"output,omitempty"`
	Evidence            []string                    `yaml:"evidence,omitempty"`
}

type NormalizedLabelOutput struct {
	Meaning             string               `yaml:"meaning"`
	EndpointCardinality *EndpointCardinality `yaml:"endpoint_cardinality,omitempty"`
	Stability           string               `yaml:"stability,omitempty"`
	Evidence            []string             `yaml:"evidence"`
}

type CategoryRange struct {
	Min   *uint64 `yaml:"min"`
	Max   *uint64 `yaml:"max"`
	Value string  `yaml:"value"`
}

type CategoryAction struct {
	Set         *string `yaml:"set,omitempty"`
	LeaveAbsent *bool   `yaml:"leave_absent,omitempty"`
}

type IdentityRepairCanonical struct {
	FamilyPrefix  string `yaml:"family_prefix"`
	IdentityLabel string `yaml:"identity_label"`
}

type IdentityRepairEmbedded struct {
	FamilyPrefix string `yaml:"family_prefix"`
	Capture      string `yaml:"capture"`
}

type IdentityJoin struct {
	Operands  []string `yaml:"operands"`
	Separator string   `yaml:"separator"`
	Blank     string   `yaml:"blank"`
	Sanitizer string   `yaml:"sanitizer"`
}

type IdentityDuplicateExclusion struct {
	WhenIdentityLabel string   `yaml:"when_identity_label"`
	Outcome           string   `yaml:"outcome"`
	Evidence          []string `yaml:"evidence"`
}

type EmbeddedIdentityRetain struct {
	Family           string `yaml:"family"`
	CapturedIdentity *bool  `yaml:"captured_identity"`
}

type GeneratedComponentScope struct {
	NamespacePrefix string `yaml:"namespace_prefix"`
	TerminalSuffix  string `yaml:"terminal_suffix"`
	Component       string `yaml:"component"`
}

type DesignExclusion struct {
	Source            SourceReference `yaml:"source"`
	When              ConditionUse    `yaml:"when,omitempty"`
	Reason            string          `yaml:"reason"`
	CoveringView      string          `yaml:"covering_view,omitempty"`
	Replacement       string          `yaml:"replacement,omitempty"`
	LostQuestion      string          `yaml:"lost_question,omitempty"`
	RequiredOperation string          `yaml:"required_operation,omitempty"`
	Evidence          []string        `yaml:"evidence"`
	Outcome           string          `yaml:"outcome"`
}

type CumulativeLimitation struct {
	ContributorVariant string       `yaml:"contributor_variant"`
	When               ConditionUse `yaml:"when,omitempty"`
	Evidence           []string     `yaml:"evidence"`
	ProofSequence      string       `yaml:"proof_sequence"`
	Effect             string       `yaml:"effect"`
}
