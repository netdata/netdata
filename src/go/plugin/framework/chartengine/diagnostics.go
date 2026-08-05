// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import "github.com/netdata/netdata/go/plugins/pkg/metrix"

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

// PlanRouteDiagnostic is a fact emitted from the production routing path.
// Empty chart or dimension fields mean that the decision happened before that
// value could be resolved. Slice fields are detached from engine state and are
// owned by the callback.
type PlanRouteDiagnostic struct {
	Decision                PlanRouteDecision
	Reason                  PlanRouteReason
	SeriesIdentity          metrix.SeriesIdentity
	MetricName              string
	ChartTemplateID         string
	DimensionIndex          int
	ChartID                 string
	DimensionName           string
	DimensionKeyLabel       string
	InstanceIdentity        PlanInstanceIdentity
	MissingInstanceLabels   []string
	ExistingChartTemplateID string
	AutogenRuleIndex        int
	AutogenRuleScope        string
	Autogen                 bool
}

func (ctx *planBuildContext) observeRouteDiagnostic(fact PlanRouteDiagnostic) {
	if ctx == nil || ctx.routeObserver == nil {
		return
	}
	ctx.routeObserver(fact)
}
