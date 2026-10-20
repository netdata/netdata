// SPDX-License-Identifier: GPL-3.0-or-later

package promsemantics

import (
	"context"
	"errors"
	"fmt"

	"github.com/netdata/netdata/go/plugins/internal/promprofile/replay"
)

type semanticChartPolicyExpectation struct {
	profile   string
	view      string
	path      string
	algorithm string
	aggregate string
	chartType string
	dims      map[int]semanticDimensionPolicyExpectation
}

type semanticDimensionPolicyExpectation struct {
	multiplier int
	divisor    int
}

// ReconcileProductionChartPolicies checks the decoded stock profile YAML for
// redundant or missing authored mechanics after semantic routes are joined.
func (c *CompiledSemanticCase) ReconcileProductionChartPolicies(
	ctx context.Context,
	snapshot *promreplay.SemanticSnapshot,
	reconciled *ReconciledSemanticCase,
) error {
	if err := checkSemanticContext(ctx, "before production chart-policy reconciliation"); err != nil {
		return err
	}
	if c == nil || c.root == nil {
		return fmt.Errorf("production chart-policy reconciliation: compiled semantic case is nil")
	}
	if snapshot == nil || reconciled == nil {
		return fmt.Errorf("production chart-policy reconciliation: snapshot and reconciled case must be present")
	}

	policies, err := indexSemanticChartPolicies(snapshot)
	if err != nil {
		return err
	}
	expected := make(map[string]*semanticChartPolicyExpectation)
	var errs []error
	for _, edge := range reconciled.Edges {
		if err := checkSemanticContext(ctx, "during production chart-policy reconciliation"); err != nil {
			return err
		}
		destination := c.programs[edge.DestinationProfile]
		if destination == nil {
			errs = append(errs, fmt.Errorf("semantic edge references inactive destination profile %q", edge.DestinationProfile))
			continue
		}
		view := destination.views[edge.View]
		if view == nil || view.inputs[edge.Input] == nil {
			errs = append(errs, fmt.Errorf("semantic edge references unknown view/input %s/%s#%s",
				edge.DestinationProfile, edge.View, edge.Input))
			continue
		}
		route, err := semanticRouteForEdge(snapshot, edge)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		key := semanticChartPolicyKey(edge.DestinationProfile, edge.TemplatePath)
		want := expected[key]
		if want == nil {
			want = &semanticChartPolicyExpectation{
				profile: edge.DestinationProfile,
				view:    edge.View,
				path:    edge.TemplatePath,
				dims:    make(map[int]semanticDimensionPolicyExpectation),
			}
			if view.reduction != nil {
				want.aggregate = view.reduction.Reducer
			}
			if view.definition.Presentation != nil {
				want.chartType = view.definition.Presentation.Type
			}
			expected[key] = want
		} else if want.view != edge.View {
			errs = append(errs, fmt.Errorf("profile %q chart %s implements semantic views %q and %q",
				edge.DestinationProfile, edge.TemplatePath, want.view, edge.View))
			continue
		}
		requiredAlgorithm, err := requiredExplicitAlgorithm(view.inputs[edge.Input])
		if err != nil {
			errs = append(errs, fmt.Errorf("profile %q view %q input %q: %w",
				edge.DestinationProfile, edge.View, edge.Input, err))
			continue
		}
		if requiredAlgorithm != "" {
			if want.algorithm != "" && want.algorithm != requiredAlgorithm {
				errs = append(errs, fmt.Errorf("profile %q chart %s requires algorithm overrides %q and %q",
					edge.DestinationProfile, edge.TemplatePath, want.algorithm, requiredAlgorithm))
				continue
			}
			want.algorithm = requiredAlgorithm
		}
		dim := semanticDimensionPolicyExpectation{
			multiplier: necessaryExplicitScale(route.Multiplier),
			divisor:    necessaryExplicitScale(route.Divisor),
		}
		if previous, ok := want.dims[edge.DimensionIndex]; ok && previous != dim {
			errs = append(errs, fmt.Errorf(
				"profile %q chart %s dimension %d requires incompatible scale declarations %d/%d and %d/%d",
				edge.DestinationProfile, edge.TemplatePath, edge.DimensionIndex,
				previous.multiplier, previous.divisor, dim.multiplier, dim.divisor))
			continue
		}
		want.dims[edge.DimensionIndex] = dim
	}

	for _, key := range sortedMapKeys(expected) {
		want := expected[key]
		policy, ok := policies[key]
		if !ok {
			errs = append(errs, fmt.Errorf("profile %q semantic view %q references missing chart policy %s",
				want.profile, want.view, want.path))
			continue
		}
		if policy.DeclaredAggregation != want.aggregate {
			errs = append(errs, fmt.Errorf("profile %q chart %s aggregation declaration got %q, want %q",
				want.profile, want.path, policy.DeclaredAggregation, want.aggregate))
		}
		if policy.DeclaredAlgorithm != want.algorithm {
			errs = append(errs, fmt.Errorf("profile %q chart %s algorithm declaration got %q, want %q",
				want.profile, want.path, policy.DeclaredAlgorithm, want.algorithm))
		}
		if policy.DeclaredType != want.chartType {
			errs = append(errs, fmt.Errorf("profile %q chart %s type declaration got %q, want %q",
				want.profile, want.path, policy.DeclaredType, want.chartType))
		}
		byIndex := make(map[int]promreplay.SemanticDimensionPolicy, len(policy.Dimensions))
		for _, dimension := range policy.Dimensions {
			if _, duplicate := byIndex[dimension.Index]; duplicate {
				errs = append(errs, fmt.Errorf("profile %q chart %s duplicates dimension policy index %d",
					want.profile, want.path, dimension.Index))
				continue
			}
			byIndex[dimension.Index] = dimension
		}
		for index, dimWant := range want.dims {
			dimGot, ok := byIndex[index]
			if !ok {
				errs = append(errs, fmt.Errorf("profile %q chart %s has no dimension policy index %d",
					want.profile, want.path, index))
				continue
			}
			if dimGot.ExplicitMultiplier != dimWant.multiplier || dimGot.ExplicitDivisor != dimWant.divisor {
				errs = append(errs, fmt.Errorf(
					"profile %q chart %s dimension %d scale declaration got %d/%d, want %d/%d",
					want.profile, want.path, index,
					dimGot.ExplicitMultiplier, dimGot.ExplicitDivisor, dimWant.multiplier, dimWant.divisor))
			}
		}
	}
	return errors.Join(errs...)
}

func requiredExplicitAlgorithm(input *compiledViewInput) (string, error) {
	required := ""
	for _, occurrence := range input.occurrences {
		registration := occurrence.program.registrations[occurrence.occurrence.registration]
		kind := expectedSeriesKindForComponent(registration.prometheus, occurrence.component.source)
		if occurrence.algorithm == defaultAlgorithmForSeriesKind(kind) {
			continue
		}
		if required != "" && required != occurrence.algorithm {
			return "", fmt.Errorf("source variants require incompatible explicit algorithms %q and %q",
				required, occurrence.algorithm)
		}
		required = occurrence.algorithm
	}
	return required, nil
}

func indexSemanticChartPolicies(
	snapshot *promreplay.SemanticSnapshot,
) (map[string]promreplay.SemanticChartPolicy, error) {
	out := make(map[string]promreplay.SemanticChartPolicy)
	var errs []error
	for _, profile := range snapshot.Profiles {
		for _, policy := range profile.Charts {
			key := semanticChartPolicyKey(profile.Name, policy.RuntimePath)
			if _, duplicate := out[key]; duplicate {
				errs = append(errs, fmt.Errorf("production profile %q duplicates chart policy path %q",
					profile.Name, policy.RuntimePath))
				continue
			}
			out[key] = policy
			if policy.RuntimePath == "" || policy.TemplateID == "" {
				errs = append(errs, fmt.Errorf("production profile %q has chart policy with empty runtime path/template ID",
					profile.Name))
			}
			if policy.ExplicitID != "" {
				errs = append(errs, fmt.Errorf("production profile %q chart %s declares redundant explicit ID %q",
					profile.Name, policy.RuntimePath, policy.ExplicitID))
			}
			if policy.WildcardIdentity {
				errs = append(errs, fmt.Errorf("production profile %q chart %s uses wildcard all-label identity",
					profile.Name, policy.RuntimePath))
			}
			if policy.MaxInstances != 0 || policy.MaxDimensions != 0 || policy.ExpireAfterCycles != 0 ||
				policy.DimensionExpiry != 0 {
				errs = append(errs, fmt.Errorf(
					"production profile %q chart %s declares unsupported lifecycle caps/expiry %d/%d/%d/%d",
					profile.Name, policy.RuntimePath, policy.MaxInstances, policy.MaxDimensions,
					policy.ExpireAfterCycles, policy.DimensionExpiry))
			}
			for _, dimension := range policy.Dimensions {
				if dimension.ExplicitFloat {
					errs = append(errs, fmt.Errorf(
						"production profile %q chart %s dimension %d declares redundant float storage",
						profile.Name, policy.RuntimePath, dimension.Index))
				}
			}
		}
	}
	return out, errors.Join(errs...)
}

func semanticRouteForEdge(
	snapshot *promreplay.SemanticSnapshot,
	edge ReconciledSemanticEdge,
) (promreplay.SemanticRoute, error) {
	if edge.SourceIndex < 0 || edge.SourceIndex >= len(snapshot.Sources) {
		return promreplay.SemanticRoute{}, fmt.Errorf("semantic edge has invalid source index %d", edge.SourceIndex)
	}
	for _, route := range snapshot.Sources[edge.SourceIndex].Routes {
		if route.Profile == edge.DestinationProfile && route.TemplatePath == edge.TemplatePath &&
			route.ChartID == edge.ChartID && route.DimensionIndex == edge.DimensionIndex &&
			route.DimensionName == edge.DimensionName {
			return route, nil
		}
	}
	return promreplay.SemanticRoute{}, fmt.Errorf(
		"semantic edge %s/%s#%s has no retained production route at %s dimension %d/%q",
		edge.DestinationProfile, edge.View, edge.Input, edge.TemplatePath, edge.DimensionIndex, edge.DimensionName)
}

func semanticChartPolicyKey(profile, path string) string {
	return profile + "\x00" + path
}

func necessaryExplicitScale(value int64) int {
	if value == 1 {
		return 0
	}
	return int(value)
}
