// SPDX-License-Identifier: GPL-3.0-or-later

// Package promreplay defines the neutral, typed result boundary between
// the Prometheus validator and stock-proof orchestration.
package promreplay

// Snapshot is the deterministic replay-facing projection of a validator report.
type Snapshot struct {
	Errors                       map[string]int
	UnsupportedFindingSeverities map[string]int
	Semantics                    *SemanticSnapshot
}

// Result includes the proof-facing snapshot and human-readable validator
// details used when reconciliation fails.
type Result struct {
	Snapshot
	Details string
}

// SemanticSnapshot is the neutral validation-time boundary between production
// collector/planner diagnostics and semantic proof reconciliation. It is not
// part of the validator's stable JSON or text report.
type SemanticSnapshot struct {
	ContextRoot      string
	Job              SemanticJobPolicy
	SelectedProfiles []string
	Profiles         []SemanticProfile
	Sources          []SemanticSource
	PlanActions      []SemanticPlanAction
}

type SemanticJobPolicy struct {
	HasSelector     bool
	HasRelabeling   bool
	HasFallbackType bool
	HasApp          bool
	HasProfiles     bool
}

type SemanticProfile struct {
	Name                 string
	Match                string
	App                  string
	HasApp               bool
	ContextNamespace     string
	AutogenSelectorAllow []string
	AutogenSelectorDeny  []string
	FallbackRules        []SemanticFallbackRule
	Charts               []SemanticChartPolicy
}

type SemanticFallbackRule struct {
	RuntimePath  string
	AssertedType string
	Pattern      string
}

type SemanticChartPolicy struct {
	RuntimePath         string
	TemplateID          string
	ExplicitID          string
	Priority            int
	WildcardIdentity    bool
	DeclaredAlgorithm   string
	DeclaredAggregation string
	DeclaredType        string
	MaxInstances        int
	MaxDimensions       int
	ExpireAfterCycles   int
	DimensionExpiry     int
	Dimensions          []SemanticDimensionPolicy
}

type SemanticDimensionPolicy struct {
	Index              int
	ExplicitMultiplier int
	ExplicitDivisor    int
	ExplicitFloat      bool
}

type SemanticLabel struct {
	Name  string
	Value string
}

type SemanticSource struct {
	OccurrenceID    string
	MetricName      string
	Component       string
	PrometheusType  string
	Value           float64
	Labels          []SemanticLabel
	FinalMetricName string
	FinalLabels     []SemanticLabel
	// WriterStructuralLabel and WriterStructuralValue retain metrix's canonical flattened label.
	WriterStructuralLabel string
	WriterStructuralValue string
	WriterSeries          int
	AutogenSeries         int
	UnmatchedSeries       int
	AutogenSuppressions   []SemanticAutogenSuppression
	Terminal              *SemanticTerminal
	Routes                []SemanticRoute
	RelabelRules          []SemanticRelabelOccurrence
}

// SemanticAutogenSuppression identifies the selected profile rule that kept a
// writer series out of generic fallback after authored routing did not claim it.
type SemanticAutogenSuppression struct {
	Profile string
	Family  string
}

type SemanticTerminal struct {
	Disposition  string
	Profile      string
	RuntimePath  string
	WriterReason string
}

type SemanticRelabelOccurrence struct {
	Profile          string
	RuntimePath      string
	Action           string
	Matched          bool
	Dropped          bool
	InputMetricName  string
	OutputMetricName string
	InputLabels      []SemanticLabel
	OutputLabels     []SemanticLabel
}

type SemanticRoute struct {
	Profile           string
	TemplatePath      string
	MetricName        string
	ChartID           string
	Context           string
	DisplayedFamily   string
	IdentityLabels    []string
	ChartLabels       []string
	ChartLabelValues  []SemanticLabel
	DimensionIndex    int
	DimensionName     string
	DimensionKeyLabel string
	PromotionMode     string
	PromotedLabels    []string
	Algorithm         string
	SeriesKind        string
	Aggregation       string
	Units             string
	Multiplier        int64
	Divisor           int64
	Presentation      string
	ContributorCount  int
}

// SemanticPlanAction is the detached, ordered projection of one chartengine
// action. Kind determines which fields apply. Public wire identities come from
// chartemit's structured production inspection.
type SemanticPlanAction struct {
	Kind            string
	ChartTemplateID string
	ChartID         string
	Context         string
	DisplayedFamily string
	Units           string
	Presentation    string
	Labels          []SemanticLabel
	DimensionName   string
	Algorithm       string
	Hidden          bool
	Float           bool
	Multiplier      int64
	Divisor         int64
	IsEmpty         bool
	Int64           int64
	Float64         float64
	WireTypeID      string
	WireChartID     string
	WireContext     string
	WireDimensionID string
}

// CloneSemanticSnapshot returns a fully detached copy of a semantic replay
// snapshot. The validation report retains its private construction state.
func CloneSemanticSnapshot(in *SemanticSnapshot) *SemanticSnapshot {
	if in == nil {
		return nil
	}
	out := *in
	out.SelectedProfiles = append([]string(nil), in.SelectedProfiles...)
	out.Profiles = make([]SemanticProfile, len(in.Profiles))
	for i := range in.Profiles {
		out.Profiles[i] = in.Profiles[i]
		out.Profiles[i].AutogenSelectorAllow = append([]string(nil), in.Profiles[i].AutogenSelectorAllow...)
		out.Profiles[i].AutogenSelectorDeny = append([]string(nil), in.Profiles[i].AutogenSelectorDeny...)
		out.Profiles[i].FallbackRules = append([]SemanticFallbackRule(nil), in.Profiles[i].FallbackRules...)
		out.Profiles[i].Charts = make([]SemanticChartPolicy, len(in.Profiles[i].Charts))
		for j := range in.Profiles[i].Charts {
			out.Profiles[i].Charts[j] = in.Profiles[i].Charts[j]
			out.Profiles[i].Charts[j].Dimensions = append(
				[]SemanticDimensionPolicy(nil), in.Profiles[i].Charts[j].Dimensions...,
			)
		}
	}
	out.Sources = make([]SemanticSource, len(in.Sources))
	for i := range in.Sources {
		out.Sources[i] = in.Sources[i]
		out.Sources[i].Labels = append([]SemanticLabel(nil), in.Sources[i].Labels...)
		out.Sources[i].FinalLabels = append([]SemanticLabel(nil), in.Sources[i].FinalLabels...)
		out.Sources[i].AutogenSuppressions = append(
			[]SemanticAutogenSuppression(nil), in.Sources[i].AutogenSuppressions...,
		)
		if in.Sources[i].Terminal != nil {
			terminal := *in.Sources[i].Terminal
			out.Sources[i].Terminal = &terminal
		}
		out.Sources[i].RelabelRules = make([]SemanticRelabelOccurrence, len(in.Sources[i].RelabelRules))
		for j := range in.Sources[i].RelabelRules {
			out.Sources[i].RelabelRules[j] = in.Sources[i].RelabelRules[j]
			out.Sources[i].RelabelRules[j].InputLabels = append(
				[]SemanticLabel(nil), in.Sources[i].RelabelRules[j].InputLabels...,
			)
			out.Sources[i].RelabelRules[j].OutputLabels = append(
				[]SemanticLabel(nil), in.Sources[i].RelabelRules[j].OutputLabels...,
			)
		}
		out.Sources[i].Routes = make([]SemanticRoute, len(in.Sources[i].Routes))
		for j := range in.Sources[i].Routes {
			out.Sources[i].Routes[j] = in.Sources[i].Routes[j]
			out.Sources[i].Routes[j].IdentityLabels = append(
				[]string(nil), in.Sources[i].Routes[j].IdentityLabels...,
			)
			out.Sources[i].Routes[j].ChartLabels = append(
				[]string(nil), in.Sources[i].Routes[j].ChartLabels...,
			)
			out.Sources[i].Routes[j].ChartLabelValues = append(
				[]SemanticLabel(nil), in.Sources[i].Routes[j].ChartLabelValues...,
			)
			out.Sources[i].Routes[j].PromotedLabels = append(
				[]string(nil), in.Sources[i].Routes[j].PromotedLabels...,
			)
		}
	}
	out.PlanActions = make([]SemanticPlanAction, len(in.PlanActions))
	for i := range in.PlanActions {
		out.PlanActions[i] = in.PlanActions[i]
		out.PlanActions[i].Labels = append([]SemanticLabel(nil), in.PlanActions[i].Labels...)
	}
	return &out
}
