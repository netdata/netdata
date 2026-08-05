// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"slices"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/promprofiles"
)

type planSeriesDiagnostic struct {
	autogen          bool
	unmatched        bool
	unmatchedReason  chartengine.PlanRouteReason
	autogenRuleIndex int
	autogenRuleScope string
}

type planRouteSummary struct {
	series map[metrix.SeriesID]*planSeriesDiagnostic
}

func newPlanRouteSummary() *planRouteSummary {
	return &planRouteSummary{series: make(map[metrix.SeriesID]*planSeriesDiagnostic)}
}

func (s *planRouteSummary) observe(fact chartengine.PlanRouteDiagnostic) {
	if s == nil || fact.SeriesIdentity.ID == "" {
		return
	}
	series := s.series[fact.SeriesIdentity.ID]
	if series == nil {
		series = &planSeriesDiagnostic{autogenRuleIndex: -1}
		s.series[fact.SeriesIdentity.ID] = series
	}

	switch fact.Decision {
	case chartengine.PlanRouteAccepted:
		series.autogen = series.autogen || fact.Autogen
	case chartengine.PlanRouteUnmatched:
		series.unmatched = true
		series.unmatchedReason = fact.Reason
		series.autogenRuleIndex = fact.AutogenRuleIndex
		series.autogenRuleScope = fact.AutogenRuleScope
	}
}

func (s *planRouteSummary) counts() (scanned, autogen, unmatched int) {
	if s == nil {
		return 0, 0, 0
	}
	for _, series := range s.series {
		if series == nil {
			continue
		}
		scanned++
		if series.autogen {
			autogen++
		}
		if series.unmatched {
			unmatched++
		}
	}
	return scanned, autogen, unmatched
}

func (s *planRouteSummary) allUnmatchedExplainedByProfile(profile promprofiles.Profile, spec *charttpl.Spec) bool {
	if s == nil || spec == nil || spec.Engine == nil || spec.Engine.Autogen == nil {
		return false
	}
	selector := profile.AutogenSelector()
	if selector == nil || len(selector.Deny) == 0 {
		return false
	}

	unmatched := 0
	for _, series := range s.series {
		if series == nil || !series.unmatched {
			continue
		}
		unmatched++
		if series.unmatchedReason != chartengine.PlanRouteReasonAutogenRuleRejected ||
			series.autogenRuleIndex < 0 ||
			series.autogenRuleIndex >= len(spec.Engine.Autogen.Rules) {
			return false
		}
		rule := spec.Engine.Autogen.Rules[series.autogenRuleIndex]
		if rule.Scope != profile.Match ||
			series.autogenRuleScope != profile.Match ||
			!slices.Equal(rule.Selector.Allow, selector.Allow) ||
			!slices.Equal(rule.Selector.Deny, selector.Deny) {
			return false
		}
	}
	return unmatched > 0
}
