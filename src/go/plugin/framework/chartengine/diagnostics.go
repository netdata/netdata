// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"slices"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine/internal/program"
)

// PlanRouteDecision classifies one production planner routing decision.
type PlanRouteDecision string

const (
	PlanRouteSeriesFilteredBySequence  PlanRouteDecision = "series_filtered_by_sequence"
	PlanRouteSeriesFilteredBySelector  PlanRouteDecision = "series_filtered_by_selector"
	PlanRouteCandidateSelectorRejected PlanRouteDecision = "candidate_selector_rejected"
	PlanRouteChartIdentityRejected     PlanRouteDecision = "chart_identity_rejected"
	PlanRouteDimensionRejected         PlanRouteDecision = "dimension_rejected"
	PlanRouteResolved                  PlanRouteDecision = "route_resolved"
	PlanRouteAccepted                  PlanRouteDecision = "route_accepted"
	PlanRouteAutogenDisplaced          PlanRouteDecision = "autogen_displaced"
	PlanRouteCollisionRejected         PlanRouteDecision = "chart_id_collision_rejected"
	PlanRouteLifecycleRejected         PlanRouteDecision = "lifecycle_rejected"
	PlanRouteUnmatched                 PlanRouteDecision = "series_unmatched"
)

// PlanInstanceIdentity is an opaque collision-resistant identity for the raw
// instance-label key/value set before chart-ID sanitization.
type PlanInstanceIdentity [32]byte

// PlanRouteReason provides the production reason behind decisions that have
// multiple policy branches.
type PlanRouteReason string

const (
	PlanRouteReasonAutogenDisabled          PlanRouteReason = "autogen_disabled"
	PlanRouteReasonAutogenSourceUnsupported PlanRouteReason = "autogen_source_unsupported"
	PlanRouteReasonAutogenRuleRejected      PlanRouteReason = "autogen_rule_rejected"
	PlanRouteReasonAutogenBuildRejected     PlanRouteReason = "autogen_build_rejected"
	PlanRouteReasonChartInstanceCap         PlanRouteReason = "chart_instance_cap"
	PlanRouteReasonDimensionCap             PlanRouteReason = "dimension_cap"
)

// PlanLabelPromotionMode is the effective non-identity label policy for an
// accepted authored route.
type PlanLabelPromotionMode string

const (
	PlanLabelPromotionAutomatic    PlanLabelPromotionMode = "automatic"
	PlanLabelPromotionAllowlist    PlanLabelPromotionMode = "allowlist"
	PlanLabelPromotionIdentityOnly PlanLabelPromotionMode = "identity_only"
)

// PlanRouteDiagnostic is a fact emitted from the production routing path.
// Empty chart or dimension fields mean that the decision happened before that
// value could be resolved. Slice fields are detached from engine state and are
// owned by the callback.
type PlanRouteDiagnostic struct {
	Decision                PlanRouteDecision
	Reason                  PlanRouteReason
	SeriesIdentity          metrix.SeriesIdentity
	MetricName              string
	MetricFamilyName        string
	ChartTemplateID         string
	DimensionIndex          int
	ChartID                 string
	DimensionName           string
	DimensionKeyLabel       string
	InstanceIdentity        PlanInstanceIdentity
	InstanceLabels          []string
	MissingInstanceLabels   []string
	Context                 string
	Family                  string
	Units                   string
	Algorithm               string
	Aggregation             string
	Presentation            string
	SeriesKind              string
	Multiplier              int
	Divisor                 int
	LabelPromotionMode      PlanLabelPromotionMode
	PromotedLabels          []string
	ExistingChartTemplateID string
	AutogenRuleIndex        int
	AutogenRuleScope        string
	Autogen                 bool
}

func diagnosticLabelPromotion(policy *chartLabelPolicy) (PlanLabelPromotionMode, []string) {
	if policy == nil || policy.mode == program.PromotionModeAutoIntersection {
		return PlanLabelPromotionAutomatic, nil
	}
	labels := make([]string, 0, len(policy.promoteKeys))
	for label := range policy.promoteKeys {
		labels = append(labels, label)
	}
	slices.Sort(labels)
	if len(labels) == 0 {
		return PlanLabelPromotionIdentityOnly, labels
	}
	return PlanLabelPromotionAllowlist, labels
}

func diagnosticAggregation(aggregation program.Aggregation) string {
	switch aggregation {
	case program.AggregationSum:
		return "sum"
	case program.AggregationMin:
		return "min"
	case program.AggregationMax:
		return "max"
	case program.AggregationAvg:
		return "avg"
	default:
		return ""
	}
}

func diagnosticMetricKind(kind metrix.MetricKind) string {
	switch kind {
	case metrix.MetricKindGauge:
		return "gauge"
	case metrix.MetricKindCounter:
		return "counter"
	case metrix.MetricKindHistogram:
		return "histogram"
	case metrix.MetricKindSummary:
		return "summary"
	case metrix.MetricKindStateSet:
		return "stateset"
	case metrix.MetricKindMeasureSet:
		return "measureset"
	default:
		return "unknown"
	}
}

func (ctx *planBuildContext) observeRouteDiagnostic(fact PlanRouteDiagnostic) {
	if ctx == nil || ctx.routeObserver == nil {
		return
	}
	ctx.routeObserver(fact)
}

func (ctx *planBuildContext) observeAutogenDisplacement(
	route routeBinding,
	identity metrix.SeriesIdentity,
	metricName string,
	displacedTemplateID string,
) {
	if ctx == nil || ctx.routeObserver == nil {
		return
	}
	ctx.observeRouteDiagnostic(PlanRouteDiagnostic{
		Decision:                PlanRouteAutogenDisplaced,
		SeriesIdentity:          identity,
		MetricName:              metricName,
		ChartTemplateID:         route.ChartTemplateID,
		DimensionIndex:          route.DimensionIndex,
		ChartID:                 route.ChartID,
		DimensionName:           route.DimensionName,
		DimensionKeyLabel:       route.DimensionKeyLabel,
		ExistingChartTemplateID: displacedTemplateID,
		Autogen:                 route.Autogen,
	})
}
