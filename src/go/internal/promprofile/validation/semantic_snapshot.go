// SPDX-License-Identifier: GPL-3.0-or-later

package promvalidation

import (
	"encoding/hex"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	promreplay "github.com/netdata/netdata/go/plugins/internal/promprofile/replay"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	prompkg "github.com/netdata/netdata/go/plugins/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartemit"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	promcollector "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/promprofiles"
	"github.com/prometheus/prometheus/model/labels"
)

type semanticChartOwner struct {
	profile string
	path    string
}

type semanticFlatSeries struct {
	id              metrix.SeriesID
	structuralValue string
}

type semanticStructuralSeriesKey struct {
	identity  prompkg.RawSampleIdentity
	component string
}

type semanticFlatSeriesIndex struct {
	physical   map[prompkg.RawSampleIdentity][]semanticFlatSeries
	structural map[semanticStructuralSeriesKey][]semanticFlatSeries
}

func buildSemanticSnapshot(
	policy jobPolicy,
	profiles []promprofiles.Profile,
	batch prompkg.SampleBatch,
	reader metrix.Reader,
	spec *charttpl.Spec,
	refs []chartRef,
	plan chartengine.Plan,
	emission chartemit.PlanInspection,
	pipeline *pipelineDiagnosticSummary,
	routes *planRouteSummary,
	retainedChartLabels map[string][]promreplay.SemanticLabel,
) (*promreplay.SemanticSnapshot, map[string][]promreplay.SemanticLabel, error) {
	if pipeline == nil || routes == nil || spec == nil {
		return nil, nil, fmt.Errorf("semantic snapshot requires pipeline, route, and template diagnostics")
	}
	profiles, err := semanticSelectedProfiles(profiles, pipeline.selectedProfileOrder)
	if err != nil {
		return nil, nil, err
	}

	owners, err := semanticChartOwners(refs)
	if err != nil {
		return nil, nil, err
	}
	autogenRuleOwners, ok := resolveProfileAutogenRuleOwners(profiles, spec)
	if !ok {
		return nil, nil, fmt.Errorf("semantic snapshot cannot resolve merged autogen rules to selected profiles")
	}
	chartLabels := semanticChartLabels(plan, retainedChartLabels)
	contributors := semanticContributorCounts(routes)
	flatSeries := newSemanticFlatSeriesIndex(reader)

	snapshot := &promreplay.SemanticSnapshot{
		ContextRoot: spec.ContextNamespace,
		Job: promreplay.SemanticJobPolicy{
			HasSelector:     len(policy.Selector.Allow) != 0 || len(policy.Selector.Deny) != 0,
			HasRelabeling:   len(policy.Relabeling) != 0,
			HasFallbackType: len(policy.FallbackType.Gauge) != 0 || len(policy.FallbackType.Counter) != 0,
			HasApp:          hasJobPolicyKey(policy, "app"),
			HasProfiles:     hasJobPolicyKey(policy, "profiles"),
		},
		SelectedProfiles: slices.Sorted(maps.Keys(pipeline.selectedProfiles)),
	}
	planActions, err := semanticPlanActions(plan, emission)
	if err != nil {
		return nil, nil, err
	}
	snapshot.PlanActions = planActions
	for _, profile := range profiles {
		fact, err := semanticProfileFact(profile, refs, owners)
		if err != nil {
			return nil, nil, err
		}
		snapshot.Profiles = append(snapshot.Profiles, fact)
	}

	for _, sample := range batch.Samples {
		rawIdentity := prompkg.IdentifyRawSample(sample.Name, sample.Labels)
		initialLabels := semanticPromLabels(sample.Labels)
		source := promreplay.SemanticSource{
			OccurrenceID:    hex.EncodeToString(rawIdentity[:]),
			MetricName:      sample.Name,
			Component:       prompkg.SampleComponentToken(sample.Kind),
			PrometheusType:  prompkg.MetricTypeToken(sample.FamilyType),
			Value:           sample.Value,
			Labels:          initialLabels,
			FinalMetricName: sample.Name,
			FinalLabels:     slices.Clone(initialLabels),
		}
		writerSeries := make(map[metrix.SeriesID]struct{})
		if _, rejected := pipeline.selectorRejected[rawIdentity]; rejected {
			source.Terminal = &promreplay.SemanticTerminal{
				Disposition:  "job_excluded",
				WriterReason: string(promcollector.PipelineReasonSelectorDenied),
			}
			snapshot.Sources = append(snapshot.Sources, source)
			continue
		}

		reachable := pipeline.reachableFromRaw(rawIdentity)
		source.RelabelRules = semanticRelabelOccurrences(pipeline, rawIdentity, profiles)
		physical, finalOutput, hasFinalOutput := pipeline.finalRawForOrigin(rawIdentity)
		if hasFinalOutput {
			source.FinalMetricName = finalOutput.MetricName
			source.FinalLabels = semanticPipelineLabels(finalOutput.OutputLabels)
		}

		finals := pipeline.finalDestinationsForRaw(rawIdentity)
		for occurrence := range finals {
			if fact, rejected := pipeline.writerRejectFacts[occurrence]; rejected {
				source.Terminal = &promreplay.SemanticTerminal{
					Disposition:  "writer_ineligible",
					WriterReason: string(fact.Reason),
				}
				continue
			}
			if reason, rejected := pipeline.writerFamilyRejects[occurrence.series.Family]; rejected {
				source.Terminal = &promreplay.SemanticTerminal{
					Disposition:  "writer_ineligible",
					WriterReason: string(reason),
				}
				continue
			}
			if _, accepted := pipeline.writerAccepted[occurrence]; !accepted {
				continue
			}
			for _, flat := range flatSeries.lookup(physical, source) {
				if _, seen := writerSeries[flat.id]; seen {
					continue
				}
				if structuralLabel := semanticComponentStructuralLabel(source.Component); structuralLabel != "" {
					if source.WriterStructuralValue != "" && source.WriterStructuralValue != flat.structuralValue {
						return nil, nil, fmt.Errorf(
							"semantic source %q maps to conflicting writer structural values %q and %q",
							source.OccurrenceID, source.WriterStructuralValue, flat.structuralValue,
						)
					}
					source.WriterStructuralLabel = structuralLabel
					source.WriterStructuralValue = flat.structuralValue
				}
				writerSeries[flat.id] = struct{}{}
				source.WriterSeries++
				if series := routes.series[flat.id]; series != nil {
					if series.autogen {
						source.AutogenSeries++
					}
					if series.unmatched {
						if series.unmatchedReason == chartengine.PlanRouteReasonAutogenRuleRejected &&
							series.autogenRuleIndex >= 0 && series.autogenRuleIndex < len(autogenRuleOwners) {
							family := prompkg.SampleFamilyName(
								prompkg.Sample{
									Name: source.FinalMetricName,
									Kind: sample.Kind,
								},
							)
							source.AutogenSuppressions = append(
								source.AutogenSuppressions,
								promreplay.SemanticAutogenSuppression{
									Profile: autogenRuleOwners[series.autogenRuleIndex].profile,
									Family:  family,
								},
							)
						} else {
							source.UnmatchedSeries++
						}
					}
				}
				for _, route := range routes.acceptedBySeries[flat.id] {
					if route.Autogen {
						continue
					}
					owner, ok := owners[route.ChartTemplateID]
					if !ok {
						return nil, nil, fmt.Errorf("authored template %q has no profile owner", route.ChartTemplateID)
					}
					source.Routes = append(source.Routes, promreplay.SemanticRoute{
						Profile:           owner.profile,
						TemplatePath:      owner.path,
						MetricName:        route.MetricName,
						ChartID:           route.ChartID,
						Context:           route.Context,
						DisplayedFamily:   route.Family,
						IdentityLabels:    slices.Clone(route.InstanceLabels),
						ChartLabels:       semanticLabelNames(chartLabels[route.ChartID]),
						ChartLabelValues:  slices.Clone(chartLabels[route.ChartID]),
						DimensionIndex:    route.DimensionIndex,
						DimensionName:     route.DimensionName,
						DimensionKeyLabel: route.DimensionKeyLabel,
						PromotionMode:     string(route.LabelPromotionMode),
						PromotedLabels:    slices.Clone(route.PromotedLabels),
						Algorithm:         route.Algorithm,
						SeriesKind:        route.SeriesKind,
						Aggregation:       route.Aggregation,
						Units:             route.Units,
						Multiplier:        int64(route.Multiplier),
						Divisor:           int64(route.Divisor),
						Presentation:      route.Presentation,
						ContributorCount:  contributors[semanticContributorKey(route)],
					})
				}
			}
		}
		if source.Terminal == nil && len(source.Routes) == 0 {
			for occurrence := range reachable {
				if _, rejected := pipeline.typedRejects[occurrence.series]; rejected {
					source.Terminal = &promreplay.SemanticTerminal{
						Disposition:  "writer_ineligible",
						WriterReason: string(promcollector.PipelineReasonInvalidFamilySchema),
					}
					break
				}
			}
		}

		if drop, ok := pipeline.relabelDropForRaw(rawIdentity); ok {
			disposition := "job_excluded"
			if drop.RelabelStage == promcollector.PipelineRelabelStageProfile {
				disposition = "profile_excluded"
			}
			source.Terminal = &promreplay.SemanticTerminal{
				Disposition: disposition,
				Profile:     drop.ProfileName,
				RuntimePath: semanticRelabelRulePath(drop.RelabelStage, drop.BlockIndex, drop.RuleIndex),
			}
		}

		slices.SortFunc(source.Routes, compareSemanticRoutes)
		slices.SortFunc(source.AutogenSuppressions, func(a, b promreplay.SemanticAutogenSuppression) int {
			if order := strings.Compare(a.Profile, b.Profile); order != 0 {
				return order
			}
			return strings.Compare(a.Family, b.Family)
		})
		snapshot.Sources = append(snapshot.Sources, source)
	}

	slices.SortFunc(snapshot.Profiles, func(a, b promreplay.SemanticProfile) int {
		return strings.Compare(a.Name, b.Name)
	})
	slices.SortFunc(snapshot.Sources, func(a, b promreplay.SemanticSource) int {
		return strings.Compare(a.OccurrenceID, b.OccurrenceID)
	})
	return snapshot, chartLabels, nil
}

func semanticSelectedProfiles(
	available []promprofiles.Profile,
	selectedOrder []string,
) ([]promprofiles.Profile, error) {
	byName := make(map[string]promprofiles.Profile, len(available))
	for _, profile := range available {
		if _, duplicate := byName[profile.Name]; duplicate {
			return nil, fmt.Errorf("semantic snapshot has duplicate available profile %q", profile.Name)
		}
		byName[profile.Name] = profile
	}
	selected := make([]promprofiles.Profile, 0, len(selectedOrder))
	seen := make(map[string]struct{}, len(selectedOrder))
	for _, name := range selectedOrder {
		if _, duplicate := seen[name]; duplicate {
			return nil, fmt.Errorf("semantic snapshot has duplicate selected profile %q", name)
		}
		profile, ok := byName[name]
		if !ok {
			return nil, fmt.Errorf("semantic snapshot selected profile %q is unavailable", name)
		}
		seen[name] = struct{}{}
		selected = append(selected, profile)
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("semantic snapshot has no selected profiles")
	}
	return selected, nil
}

func hasJobPolicyKey(policy jobPolicy, key string) bool {
	_, ok := policy.declaredKeys[key]
	return ok
}

func semanticProfileFact(
	profile promprofiles.Profile,
	refs []chartRef,
	owners map[string]semanticChartOwner,
) (promreplay.SemanticProfile, error) {
	fact := promreplay.SemanticProfile{
		Name:   profile.Name,
		Match:  profile.Match,
		App:    profile.App,
		HasApp: profile.HasApp(),
	}
	if selector := profile.AutogenSelector(); selector != nil {
		fact.AutogenSelectorAllow = slices.Clone(selector.Allow)
		fact.AutogenSelectorDeny = slices.Clone(selector.Deny)
	}
	fallback, err := profile.FallbackType()
	if err != nil {
		return fact, err
	}
	for i, pattern := range fallback.Gauge {
		fact.FallbackRules = append(fact.FallbackRules, promreplay.SemanticFallbackRule{
			RuntimePath:  fmt.Sprintf("fallback_type.gauge[%d]", i),
			AssertedType: "gauge",
			Pattern:      pattern,
		})
	}
	for i, pattern := range fallback.Counter {
		fact.FallbackRules = append(fact.FallbackRules, promreplay.SemanticFallbackRule{
			RuntimePath:  fmt.Sprintf("fallback_type.counter[%d]", i),
			AssertedType: "counter",
			Pattern:      pattern,
		})
	}
	template, err := profile.Template()
	if err != nil {
		return fact, err
	}
	fact.ContextNamespace = template.ContextNamespace
	fact.Charts, err = semanticChartPolicies(profile.Name, template, refs, owners)
	if err != nil {
		return fact, err
	}
	return fact, nil
}

func semanticChartOwners(refs []chartRef) (map[string]semanticChartOwner, error) {
	owners := make(map[string]semanticChartOwner)
	for _, ref := range refs {
		if len(ref.groupPath) == 0 || ref.groupPath[0] == 0 {
			continue
		}
		if ref.profile == "" || ref.sourcePath == "" {
			return nil, fmt.Errorf("chart %q has no resolved profile owner", ref.path)
		}
		templateID, ok := chartengine.ChartTemplateIDAt(ref.groupPath, ref.chartIndex)
		if !ok {
			return nil, fmt.Errorf("resolve chart template identity for %q", ref.path)
		}
		owners[templateID] = semanticChartOwner{
			profile: ref.profile,
			path:    ref.sourcePath,
		}
	}
	return owners, nil
}

func semanticChartPolicies(
	profileName string,
	root charttpl.Group,
	refs []chartRef,
	owners map[string]semanticChartOwner,
) ([]promreplay.SemanticChartPolicy, error) {
	idsByPath := make(map[string]string)
	for id, owner := range owners {
		if owner.profile == profileName {
			idsByPath[owner.path] = id
		}
	}
	resolvedByPath := make(map[string]charttpl.Chart)
	for _, ref := range refs {
		if ref.profile == profileName {
			resolvedByPath[ref.sourcePath] = ref.chart
		}
	}
	var out []promreplay.SemanticChartPolicy
	var walk func(charttpl.Group, []int) error
	walk = func(group charttpl.Group, groupPath []int) error {
		for index, chart := range group.Charts {
			path := profileChartPath(groupPath, index)
			// Merged refs come from the authoritative decoder, so group defaults are already materialized.
			resolved, ok := resolvedByPath[path]
			if !ok {
				return fmt.Errorf("profile %q chart %q has no resolved merged template", profileName, path)
			}
			policy := promreplay.SemanticChartPolicy{
				RuntimePath:         path,
				TemplateID:          idsByPath[path],
				ExplicitID:          chart.ID,
				Priority:            resolved.Priority,
				DeclaredAlgorithm:   chart.Algorithm,
				DeclaredAggregation: string(chart.Aggregation),
				DeclaredType:        chart.Type,
			}
			if resolved.Instances != nil {
				policy.WildcardIdentity = slices.Contains(resolved.Instances.ByLabels, "*")
			}
			if chart.Lifecycle != nil {
				policy.MaxInstances = chart.Lifecycle.MaxInstances
				policy.ExpireAfterCycles = chart.Lifecycle.ExpireAfterCycles
				if chart.Lifecycle.Dimensions != nil {
					policy.MaxDimensions = chart.Lifecycle.Dimensions.MaxDims
					policy.DimensionExpiry = chart.Lifecycle.Dimensions.ExpireAfterCycles
				}
			}
			for dimensionIndex, dimension := range chart.Dimensions {
				dim := promreplay.SemanticDimensionPolicy{
					Index: dimensionIndex,
				}
				if dimension.Options != nil {
					dim.ExplicitMultiplier = dimension.Options.Multiplier
					dim.ExplicitDivisor = dimension.Options.Divisor
					dim.ExplicitFloat = dimension.Options.Float
				}
				policy.Dimensions = append(policy.Dimensions, dim)
			}
			out = append(out, policy)
		}
		for index, child := range group.Groups {
			if err := walk(child, append(slices.Clone(groupPath), index)); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(root, nil); err != nil {
		return nil, err
	}
	return out, nil
}

func semanticRelabelRulePath(stage promcollector.PipelineRelabelStage, block, rule int) string {
	prefix := "relabeling"
	if stage == promcollector.PipelineRelabelStageJob {
		prefix = "job.relabeling"
	}
	return fmt.Sprintf("%s[%d].metric_relabel_configs[%d]", prefix, block, rule)
}

func semanticChartLabels(
	plan chartengine.Plan,
	retained map[string][]promreplay.SemanticLabel,
) map[string][]promreplay.SemanticLabel {
	out := make(map[string][]promreplay.SemanticLabel, len(retained))
	for chartID, values := range retained {
		out[chartID] = slices.Clone(values)
	}
	for _, action := range plan.Actions {
		var chartID string
		var labelValues map[string]string
		switch value := action.(type) {
		case chartengine.CreateChartAction:
			chartID, labelValues = value.ChartID, value.Labels
		case chartengine.UpdateChartLabelsAction:
			chartID, labelValues = value.ChartID, value.Labels
		case chartengine.RemoveChartAction:
			delete(out, value.ChartID)
			continue
		default:
			continue
		}
		out[chartID] = semanticStringMapLabels(labelValues)
	}
	return out
}

func semanticLabelNames(labels []promreplay.SemanticLabel) []string {
	out := make([]string, 0, len(labels))
	for _, label := range labels {
		out = append(out, label.Name)
	}
	return out
}

func semanticStringMapLabels(labels map[string]string) []promreplay.SemanticLabel {
	out := make([]promreplay.SemanticLabel, 0, len(labels))
	for name, value := range labels {
		out = append(out, promreplay.SemanticLabel{
			Name:  name,
			Value: value,
		})
	}
	slices.SortFunc(out, compareSemanticLabels)
	return out
}

func semanticContributorKey(route chartengine.PlanRouteDiagnostic) string {
	return route.ChartID + "\x00" + route.DimensionName
}

func semanticContributorCounts(routes *planRouteSummary) map[string]int {
	out := make(map[string]int)
	for _, facts := range routes.acceptedBySeries {
		for _, fact := range facts {
			if !fact.Autogen {
				out[semanticContributorKey(fact)]++
			}
		}
	}
	return out
}

func newSemanticFlatSeriesIndex(reader metrix.Reader) semanticFlatSeriesIndex {
	out := semanticFlatSeriesIndex{
		physical:   make(map[prompkg.RawSampleIdentity][]semanticFlatSeries),
		structural: make(map[semanticStructuralSeriesKey][]semanticFlatSeries),
	}
	reader.ForEachSeriesIdentity(
		func(identity metrix.SeriesIdentity, meta metrix.SeriesMeta, name string, view metrix.LabelView, _ metrix.SampleValue) {
			lbs := make(labels.Labels, 0, view.Len())
			view.Range(func(key, value string) bool {
				lbs = append(lbs, labels.Label{
					Name:  key,
					Value: value,
				})
				return true
			})
			slices.SortFunc(lbs, func(a, b labels.Label) int {
				if order := strings.Compare(a.Name, b.Name); order != 0 {
					return order
				}
				return strings.Compare(a.Value, b.Value)
			})
			physical := prompkg.IdentifyRawSample(name, lbs)
			flat := semanticFlatSeries{
				id: identity.ID,
			}
			component := semanticFlattenComponent(meta.FlattenRole)
			if structuralLabel := semanticComponentStructuralLabel(component); structuralLabel != "" {
				flat.structuralValue = lbs.Get(structuralLabel)
				key := semanticStructuralSeriesKey{
					identity:  semanticRawIdentityWithoutLabel(name, lbs, structuralLabel),
					component: component,
				}
				out.structural[key] = append(out.structural[key], flat)
			}
			out.physical[physical] = append(out.physical[physical], flat)
		},
	)
	return out
}

func (index semanticFlatSeriesIndex) lookup(
	physical prompkg.RawSampleIdentity,
	source promreplay.SemanticSource,
) []semanticFlatSeries {
	if exact := index.physical[physical]; len(exact) != 0 {
		return exact
	}
	structuralLabel := semanticComponentStructuralLabel(source.Component)
	if structuralLabel == "" {
		return nil
	}
	lbs := semanticReplayLabels(source.FinalLabels)
	wantValue := lbs.Get(structuralLabel)
	key := semanticStructuralSeriesKey{
		identity:  semanticRawIdentityWithoutLabel(source.FinalMetricName, lbs, structuralLabel),
		component: source.Component,
	}
	for _, candidate := range index.structural[key] {
		if semanticNumericLabelEqual(candidate.structuralValue, wantValue) {
			return []semanticFlatSeries{candidate}
		}
	}
	return nil
}

func semanticFlattenComponent(role metrix.FlattenRole) string {
	switch role {
	case metrix.FlattenRoleHistogramBucket:
		return "histogram_bucket"
	case metrix.FlattenRoleSummaryQuantile:
		return "summary_quantile"
	default:
		return ""
	}
}

func semanticComponentStructuralLabel(component string) string {
	switch component {
	case "histogram_bucket":
		return metrix.HistogramBucketLabel
	case "summary_quantile":
		return metrix.SummaryQuantileLabel
	default:
		return ""
	}
}

func semanticRawIdentityWithoutLabel(name string, lbs labels.Labels, excluded string) prompkg.RawSampleIdentity {
	filtered := make(labels.Labels, 0, len(lbs))
	for _, label := range lbs {
		if label.Name != excluded {
			filtered = append(filtered, label)
		}
	}
	return prompkg.IdentifyRawSample(name, filtered)
}

func semanticReplayLabels(in []promreplay.SemanticLabel) labels.Labels {
	out := make(labels.Labels, 0, len(in))
	for _, label := range in {
		out = append(out, labels.Label{
			Name:  label.Name,
			Value: label.Value,
		})
	}
	return out
}

func semanticNumericLabelEqual(left, right string) bool {
	if left == right {
		return true
	}
	leftValue, leftErr := strconv.ParseFloat(left, 64)
	rightValue, rightErr := strconv.ParseFloat(right, 64)
	return leftErr == nil && rightErr == nil && leftValue == rightValue
}

func semanticPromLabels(in labels.Labels) []promreplay.SemanticLabel {
	out := make([]promreplay.SemanticLabel, 0, len(in))
	for _, label := range in {
		if label.Name != labels.MetricName {
			out = append(out, promreplay.SemanticLabel{
				Name:  label.Name,
				Value: label.Value,
			})
		}
	}
	slices.SortFunc(out, compareSemanticLabels)
	return out
}

func semanticPipelineLabels(in []promcollector.PipelineLabel) []promreplay.SemanticLabel {
	out := make([]promreplay.SemanticLabel, 0, len(in))
	for _, label := range in {
		out = append(out, promreplay.SemanticLabel{
			Name:  label.Name,
			Value: label.Value,
		})
	}
	slices.SortFunc(out, compareSemanticLabels)
	return out
}

func compareSemanticLabels(a, b promreplay.SemanticLabel) int {
	if order := strings.Compare(a.Name, b.Name); order != 0 {
		return order
	}
	return strings.Compare(a.Value, b.Value)
}

func compareSemanticRoutes(a, b promreplay.SemanticRoute) int {
	if order := strings.Compare(a.Profile, b.Profile); order != 0 {
		return order
	}
	if order := strings.Compare(a.TemplatePath, b.TemplatePath); order != 0 {
		return order
	}
	if order := strings.Compare(a.ChartID, b.ChartID); order != 0 {
		return order
	}
	if a.DimensionIndex < b.DimensionIndex {
		return -1
	}
	if a.DimensionIndex > b.DimensionIndex {
		return 1
	}
	return strings.Compare(a.DimensionName, b.DimensionName)
}
