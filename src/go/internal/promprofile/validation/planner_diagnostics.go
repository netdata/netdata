// SPDX-License-Identifier: GPL-3.0-or-later

package promvalidation

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
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

func (s *planRouteSummary) autogenProfilePaths(profiles []promprofiles.Profile) []string {
	if s == nil || len(profiles) < 2 {
		return nil
	}
	type profileMatcher struct {
		name  string
		match matcher.Matcher
	}
	compiled := make([]profileMatcher, 0, len(profiles))
	for _, profile := range profiles {
		m, err := matcher.NewSimplePatternsMatcher(profile.Match)
		if err != nil {
			continue
		}
		compiled = append(compiled, profileMatcher{name: profile.Name, match: m})
	}
	paths := make(map[string]struct{})
	for _, facts := range s.acceptedBySeries {
		for _, fact := range facts {
			if !fact.Autogen {
				continue
			}
			familyName := fact.MetricFamilyName
			if familyName == "" {
				familyName = fact.MetricName
			}
			for _, profile := range compiled {
				if profile.match.MatchString(familyName) {
					paths[fmt.Sprintf("profiles[%s].template", profile.name)] = struct{}{}
				}
			}
		}
	}
	return slices.Sorted(maps.Keys(paths))
}

type profileAutogenRuleOwner struct {
	profile string
	scope   string
	allow   []string
	deny    []string
}

type planRouteSummary struct {
	series                    map[metrix.SeriesID]*planSeriesDiagnostic
	templates                 map[string]*planTemplateDiagnostic
	resolvedTemplatesBySeries map[metrix.SeriesID]map[string]struct{}
	ownersByChart             map[string]map[string]struct{}
	acceptedBySeries          map[metrix.SeriesID][]chartengine.PlanRouteDiagnostic
}

func newPlanRouteSummary() *planRouteSummary {
	return &planRouteSummary{
		series:                    make(map[metrix.SeriesID]*planSeriesDiagnostic),
		templates:                 make(map[string]*planTemplateDiagnostic),
		resolvedTemplatesBySeries: make(map[metrix.SeriesID]map[string]struct{}),
		ownersByChart:             make(map[string]map[string]struct{}),
		acceptedBySeries:          make(map[metrix.SeriesID][]chartengine.PlanRouteDiagnostic),
	}
}

func (s *planRouteSummary) observe(fact chartengine.PlanRouteDiagnostic) {
	if s == nil {
		return
	}
	if fact.Decision == chartengine.PlanRouteLifecycleRejected {
		template := s.template(fact.ChartTemplateID)
		switch fact.Reason {
		case chartengine.PlanRouteReasonChartInstanceCap:
			template.droppedCharts[fact.ChartID] = struct{}{}
		case chartengine.PlanRouteReasonDimensionCap:
			template.droppedDimensions[planDimensionOutput{chartID: fact.ChartID, name: fact.DimensionName}] = struct{}{}
		}
		return
	}
	if fact.SeriesIdentity.ID == "" {
		return
	}
	series := s.series[fact.SeriesIdentity.ID]
	if series == nil {
		series = &planSeriesDiagnostic{autogenRuleIndex: -1}
		s.series[fact.SeriesIdentity.ID] = series
	}

	switch fact.Decision {
	case chartengine.PlanRouteResolved:
		template := s.template(fact.ChartTemplateID)
		template.chartIDs[fact.ChartID] = struct{}{}
		template.instanceIdentities[fact.InstanceIdentity] = struct{}{}
		template.dimensionIndexes[fact.DimensionIndex] = struct{}{}
		if fact.DimensionKeyLabel != "" {
			template.dimensionKeyLabels[fact.DimensionKeyLabel] = struct{}{}
		}
		resolvedTemplates := s.resolvedTemplatesBySeries[fact.SeriesIdentity.ID]
		if resolvedTemplates == nil {
			resolvedTemplates = make(map[string]struct{})
			s.resolvedTemplatesBySeries[fact.SeriesIdentity.ID] = resolvedTemplates
		}
		resolvedTemplates[fact.ChartTemplateID] = struct{}{}
		output := planDimensionOutput{chartID: fact.ChartID, name: fact.DimensionName}
		template.dimensionOutputs[output] = struct{}{}
		template.dimensionIdentities[planDimensionIdentity{
			instance: fact.InstanceIdentity,
			index:    fact.DimensionIndex,
			name:     fact.DimensionName,
		}] = struct{}{}
		owners := s.ownersByChart[fact.ChartID]
		if owners == nil {
			owners = make(map[string]struct{})
			s.ownersByChart[fact.ChartID] = owners
		}
		owners[fact.ChartTemplateID] = struct{}{}
	case chartengine.PlanRouteChartIdentityRejected:
		if len(fact.MissingInstanceLabels) == 0 {
			break
		}
		template := s.template(fact.ChartTemplateID)
		key := planMissingInstanceKey{
			dimensionIndex: fact.DimensionIndex,
			labels:         strings.Join(fact.MissingInstanceLabels, "\x00"),
		}
		series := template.missingInstances[key]
		if series == nil {
			series = make(map[metrix.SeriesID]struct{})
			template.missingInstances[key] = series
		}
		series[fact.SeriesIdentity.ID] = struct{}{}
	case chartengine.PlanRouteAccepted:
		series.autogen = series.autogen || fact.Autogen
		s.acceptedBySeries[fact.SeriesIdentity.ID] = append(s.acceptedBySeries[fact.SeriesIdentity.ID], fact)
	case chartengine.PlanRouteUnmatched:
		series.unmatched = true
		series.unmatchedReason = fact.Reason
		series.autogenRuleIndex = fact.AutogenRuleIndex
		series.autogenRuleScope = fact.AutogenRuleScope
	}
}

type planDimensionOutput struct {
	chartID string
	name    string
}

type planDimensionIdentity struct {
	instance chartengine.PlanInstanceIdentity
	index    int
	name     string
}

type planMissingInstanceKey struct {
	dimensionIndex int
	labels         string
}

type planTemplateDiagnostic struct {
	chartIDs            map[string]struct{}
	instanceIdentities  map[chartengine.PlanInstanceIdentity]struct{}
	dimensionIndexes    map[int]struct{}
	dimensionKeyLabels  map[string]struct{}
	dimensionOutputs    map[planDimensionOutput]struct{}
	dimensionIdentities map[planDimensionIdentity]struct{}
	droppedCharts       map[string]struct{}
	droppedDimensions   map[planDimensionOutput]struct{}
	missingInstances    map[planMissingInstanceKey]map[metrix.SeriesID]struct{}
}

func (s *planRouteSummary) template(templateID string) *planTemplateDiagnostic {
	template := s.templates[templateID]
	if template != nil {
		return template
	}
	template = &planTemplateDiagnostic{
		chartIDs:            make(map[string]struct{}),
		instanceIdentities:  make(map[chartengine.PlanInstanceIdentity]struct{}),
		dimensionIndexes:    make(map[int]struct{}),
		dimensionKeyLabels:  make(map[string]struct{}),
		dimensionOutputs:    make(map[planDimensionOutput]struct{}),
		dimensionIdentities: make(map[planDimensionIdentity]struct{}),
		droppedCharts:       make(map[string]struct{}),
		droppedDimensions:   make(map[planDimensionOutput]struct{}),
		missingInstances:    make(map[planMissingInstanceKey]map[metrix.SeriesID]struct{}),
	}
	s.templates[templateID] = template
	return template
}

type planRouteInspection struct {
	unavailableInstances []unavailableInstanceIdentity
	deadCharts           []deadChartReport
	deadDimensions       []deadDimensionReport
	dimensionLosses      []dimensionMaterializationLossReport
	collisions           []collisionReport
	instanceLosses       []instanceMaterializationLossReport
	errs                 []chartRouteDiagnosticError
}

func (s *planRouteSummary) inspectAuthoredCharts(refs []chartRef) planRouteInspection {
	var out planRouteInspection
	pathsByTemplate := make(map[string]string, len(refs))

	for _, ref := range refs {
		templateID, ok := chartengine.ChartTemplateIDAt(ref.groupPath, ref.chartIndex)
		if !ok {
			out.errs = append(out.errs, chartRouteDiagnosticError{path: ref.path, err: fmt.Errorf("resolve compiler chart template identity")})
			continue
		}
		pathsByTemplate[templateID] = ref.path
		template := s.templates[templateID]
		if template == nil || len(template.chartIDs) == 0 {
			out.deadCharts = append(out.deadCharts, deadChartReport{
				Path:     ref.path,
				Title:    ref.chart.Title,
				Context:  ref.chart.Context,
				Priority: ref.chart.Priority,
			})
		} else {
			renderedCharts := 0
			for chartID := range template.chartIDs {
				if _, dropped := template.droppedCharts[chartID]; !dropped {
					renderedCharts++
				}
			}
			if len(template.instanceIdentities) > renderedCharts {
				cause := "rendered_id_collapse"
				if ref.chart.Lifecycle != nil &&
					ref.chart.Lifecycle.MaxInstances > 0 &&
					len(template.instanceIdentities) > ref.chart.Lifecycle.MaxInstances {
					cause = "lifecycle_limit_or_rendered_id_collapse"
				}
				out.instanceLosses = append(out.instanceLosses, instanceMaterializationLossReport{
					Path:               ref.path,
					ObservedIdentities: len(template.instanceIdentities),
					RenderedIDs:        renderedCharts,
					Cause:              cause,
				})
			}

			plannedDimensions := 0
			for dimension := range template.dimensionOutputs {
				if _, chartDropped := template.droppedCharts[dimension.chartID]; chartDropped {
					continue
				}
				if _, dimensionDropped := template.droppedDimensions[dimension]; dimensionDropped {
					continue
				}
				plannedDimensions++
			}
			if len(template.dimensionIdentities) > plannedDimensions {
				cause := "planner_dimension_omission"
				if ref.chart.Lifecycle != nil &&
					ref.chart.Lifecycle.Dimensions != nil &&
					ref.chart.Lifecycle.Dimensions.MaxDims > 0 {
					cause = "lifecycle_dimension_limit_or_planner_collapse"
				}
				out.dimensionLosses = append(out.dimensionLosses, dimensionMaterializationLossReport{
					Path:               ref.path,
					ObservedDimensions: len(template.dimensionIdentities),
					PlannedDimensions:  plannedDimensions,
					Cause:              cause,
				})
			}
		}

		for dimensionIndex, dimension := range ref.chart.Dimensions {
			if template != nil {
				if _, live := template.dimensionIndexes[dimensionIndex]; live {
					continue
				}
			}
			name := dimension.Name
			if name == "" {
				name = dimension.NameFromLabel
			}
			out.deadDimensions = append(out.deadDimensions, deadDimensionReport{
				Path:     fmt.Sprintf("%s.dimensions[%d]", ref.path, dimensionIndex),
				Selector: dimension.Selector,
				Name:     name,
			})
		}

		if template != nil {
			keys := make([]planMissingInstanceKey, 0, len(template.missingInstances))
			for key := range template.missingInstances {
				keys = append(keys, key)
			}
			slices.SortFunc(keys, func(a, b planMissingInstanceKey) int {
				if a.dimensionIndex != b.dimensionIndex {
					return a.dimensionIndex - b.dimensionIndex
				}
				return strings.Compare(a.labels, b.labels)
			})
			for _, key := range keys {
				if key.dimensionIndex < 0 || key.dimensionIndex >= len(ref.chart.Dimensions) {
					continue
				}
				out.unavailableInstances = append(out.unavailableInstances, unavailableInstanceIdentity{
					path:          fmt.Sprintf("%s.dimensions[%d]", ref.path, key.dimensionIndex),
					chartTitle:    ref.chart.Title,
					selector:      ref.chart.Dimensions[key.dimensionIndex].Selector,
					missingLabels: strings.Split(key.labels, "\x00"),
					series:        len(template.missingInstances[key]),
				})
			}
		}
	}

	for chartID, owners := range s.ownersByChart {
		if len(owners) < 2 {
			continue
		}
		var paths []string
		for templateID := range owners {
			if path := pathsByTemplate[templateID]; path != "" {
				paths = append(paths, path)
			}
		}
		if len(paths) < 2 {
			continue
		}
		slices.Sort(paths)
		out.collisions = append(out.collisions, collisionReport{
			RenderedIDFingerprint: fingerprintID(chartID),
			Charts:                paths,
		})
	}
	return out
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

func (s *planRouteSummary) unmatchedProfilePolicyPaths(
	profiles []promprofiles.Profile,
	spec *charttpl.Spec,
) ([]string, bool) {
	if s == nil {
		return nil, false
	}
	owners, ok := resolveProfileAutogenRuleOwners(profiles, spec)
	if !ok || len(owners) == 0 {
		return nil, false
	}

	unmatched := 0
	paths := make(map[string]struct{})
	for _, series := range s.series {
		if series == nil || !series.unmatched {
			continue
		}
		unmatched++
		if series.unmatchedReason != chartengine.PlanRouteReasonAutogenRuleRejected ||
			series.autogenRuleIndex < 0 ||
			series.autogenRuleIndex >= len(owners) {
			return nil, false
		}
		want := owners[series.autogenRuleIndex]
		if series.autogenRuleScope != want.scope {
			return nil, false
		}
		path := "autogen.selector"
		if len(profiles) > 1 {
			path = fmt.Sprintf("profiles[%s].autogen.selector", want.profile)
		}
		paths[path] = struct{}{}
	}
	return slices.Sorted(maps.Keys(paths)), unmatched > 0
}

func resolveProfileAutogenRuleOwners(
	profiles []promprofiles.Profile,
	spec *charttpl.Spec,
) ([]profileAutogenRuleOwner, bool) {
	if spec == nil || spec.Engine == nil || spec.Engine.Autogen == nil {
		return nil, false
	}
	owners := make([]profileAutogenRuleOwner, 0, len(profiles))
	for _, profile := range profiles {
		if selector := profile.AutogenSelector(); selector != nil {
			owners = append(owners, profileAutogenRuleOwner{
				profile: profile.Name,
				scope:   profile.Match,
				allow:   selector.Allow,
				deny:    selector.Deny,
			})
		}
	}
	if len(spec.Engine.Autogen.Rules) != len(owners) {
		return nil, false
	}
	resolved := make([]profileAutogenRuleOwner, 0, len(owners))
	used := make([]bool, len(owners))
	for _, rule := range spec.Engine.Autogen.Rules {
		match := -1
		for ownerIndex, owner := range owners {
			if used[ownerIndex] || rule.Scope != owner.scope ||
				!slices.Equal(rule.Selector.Allow, owner.allow) ||
				!slices.Equal(rule.Selector.Deny, owner.deny) {
				continue
			}
			if match == -1 {
				// Equivalent policies are still distinct generated rules. The
				// collector appends them in supplied profile order, so consume
				// equal owners in that same stable order.
				match = ownerIndex
			}
		}
		if match == -1 {
			return nil, false
		}
		used[match] = true
		resolved = append(resolved, owners[match])
	}
	return resolved, true
}
