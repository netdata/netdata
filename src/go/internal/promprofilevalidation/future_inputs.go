// SPDX-License-Identifier: GPL-3.0-or-later

package promprofilevalidation

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	prompkg "github.com/netdata/netdata/go/plugins/pkg/prometheus"
	promcollector "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/promprofiles"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/relabel"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	commonmodel "github.com/prometheus/common/model"
	promlabels "github.com/prometheus/prometheus/model/labels"
)

const (
	maxFutureInputs           = 256
	futureWitnessesPerScope   = 3
	futureInputValueBase      = 1_000_000_000
	futureInputSearchAttempts = 128
)

type futureInput struct {
	Name   string            `yaml:"name"`
	Type   string            `yaml:"type,omitempty"`
	Labels map[string]string `yaml:"labels,omitempty"`
}

func (input futureInput) effectiveType() commonmodel.MetricType {
	switch input.Type {
	case "", "gauge":
		return commonmodel.MetricTypeGauge
	case "counter":
		return commonmodel.MetricTypeCounter
	case "untyped":
		return commonmodel.MetricTypeUnknown
	default:
		return commonmodel.MetricType(input.Type)
	}
}

func validateFutureInputs(inputs []futureInput) error {
	if len(inputs) > maxFutureInputs {
		return fmt.Errorf("future_inputs has %d entries; maximum is %d", len(inputs), maxFutureInputs)
	}
	seen := make(map[prompkg.RawSampleIdentity]struct{}, len(inputs))
	typesByName := make(map[string]commonmodel.MetricType, len(inputs))
	for index, input := range inputs {
		if !commonmodel.UTF8Validation.IsValidMetricName(input.Name) {
			return fmt.Errorf("future_inputs[%d].name %q is not a valid UTF-8 Prometheus metric name", index, input.Name)
		}
		switch input.effectiveType() {
		case commonmodel.MetricTypeGauge, commonmodel.MetricTypeCounter, commonmodel.MetricTypeUnknown:
		default:
			return fmt.Errorf(
				"future_inputs[%d].type %q is not supported; use gauge, counter, or untyped",
				index, input.Type,
			)
		}
		if previous, ok := typesByName[input.Name]; ok && previous != input.effectiveType() {
			return fmt.Errorf("future_inputs[%d] gives metric family %q more than one type", index, input.Name)
		}
		typesByName[input.Name] = input.effectiveType()
		labelSet := promlabels.FromMap(input.Labels)
		for _, label := range labelSet {
			if label.Name == promlabels.MetricName || !commonmodel.UTF8Validation.IsValidLabelName(label.Name) {
				return fmt.Errorf("future_inputs[%d].labels contains invalid label name %q", index, label.Name)
			}
		}
		key := prompkg.IdentifyRawSample(input.Name, labelSet)
		if _, ok := seen[key]; ok {
			return fmt.Errorf("future_inputs[%d] duplicates an earlier raw metric identity", index)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func encodeFutureInputs(inputs []futureInput) ([]byte, error) {
	ordered := slices.Clone(inputs)
	slices.SortFunc(ordered, func(a, b futureInput) int {
		if cmp := strings.Compare(a.Name, b.Name); cmp != 0 {
			return cmp
		}
		return strings.Compare(promlabels.FromMap(a.Labels).String(), promlabels.FromMap(b.Labels).String())
	})

	type encodedFamily struct {
		typeValue dto.MetricType
		metrics   []*dto.Metric
	}
	byName := make(map[string]encodedFamily)
	for index, input := range ordered {
		value := float64(futureInputValueBase + index)
		metric := dto.Metric{}
		typeValue := dto.MetricType_GAUGE
		switch input.effectiveType() {
		case commonmodel.MetricTypeCounter:
			typeValue = dto.MetricType_COUNTER
			metric.Counter = &dto.Counter{Value: &value}
		case commonmodel.MetricTypeUnknown:
			typeValue = dto.MetricType_UNTYPED
			metric.Untyped = &dto.Untyped{Value: &value}
		default:
			metric.Gauge = &dto.Gauge{Value: &value}
		}
		for _, label := range promlabels.FromMap(input.Labels) {
			labelName, labelValue := label.Name, label.Value
			metric.Label = append(metric.Label, &dto.LabelPair{Name: &labelName, Value: &labelValue})
		}
		family := byName[input.Name]
		family.typeValue = typeValue
		family.metrics = append(family.metrics, &metric)
		byName[input.Name] = family
	}

	var output bytes.Buffer
	for _, name := range sortedStringKeys(byName) {
		familyName := name
		encoded := byName[name]
		family := dto.MetricFamily{Name: &familyName, Type: &encoded.typeValue, Metric: encoded.metrics}
		if _, err := expfmt.MetricFamilyToText(&output, &family); err != nil {
			return nil, fmt.Errorf("encode future input family %q: %w", name, err)
		}
	}
	return output.Bytes(), nil
}

func sortedStringKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

type futureScopeRequirement struct {
	path       string
	pattern    string
	scopeExpr  string
	blockIndex int
}

type futureRuleRequirement struct {
	blockIndex            int
	ruleIndex             int
	requireHit            bool
	allowsAuthoredRouting bool
}

type futureRequirements struct {
	profileScopes    []futureScopeRequirement
	blockScopes      []futureScopeRequirement
	rules            []futureRuleRequirement
	requiresExplicit bool
	matcher          *matcher.Analyzer
}

type futureNameWrite struct {
	rule        relabel.Config
	futureReach bool
}

// buildFutureRequirements identifies contributor-policy coverage obligations.
// Runtime acceptance is proved later from diagnostics emitted by the separate
// future collector run; this function only identifies which scopes and routing
// rules require a witness.
func buildFutureRequirements(
	ctx context.Context,
	profile promprofiles.Profile,
	policy jobPolicy,
	current prompkg.SampleBatch,
) (futureRequirements, error) {
	matcherAnalyzer, err := matcher.NewAnalyzer(ctx, matcher.AnalysisBudget{})
	if err != nil {
		return futureRequirements{}, err
	}
	relabelAnalyzer, err := relabel.NewAnalyzer(ctx, relabel.AnalysisBudget{
		MaxValues: maxBoundedMetricNameGrammarBranches,
	})
	if err != nil {
		return futureRequirements{}, err
	}
	flowAnalyzer, err := promcollector.NewRelabelNameFlowAnalyzer(
		ctx, relabelAnalyzer, matcherAnalyzer, promcollector.RelabelNameFlowBudget{},
	)
	if err != nil {
		return futureRequirements{}, err
	}

	requirements := futureRequirements{
		profileScopes: positiveWildcardScopes("match", profile.Match, -1),
		matcher:       matcherAnalyzer,
	}
	authoredMetricNames := profileAuthoredMetricNames(profile)
	currentNames := make(map[string]struct{}, len(current.Samples))
	for _, sample := range current.Samples {
		currentNames[sample.Name] = struct{}{}
	}
	var priorMutations []promcollector.RelabelNameMutation
	for blockIndex, block := range policy.Relabeling {
		profileOverlap, err := simplePatternScopesMayOverlap(matcherAnalyzer, profile.Match, block.Match)
		if err != nil {
			return futureRequirements{}, err
		}
		mutationReachable, err := flowAnalyzer.MutationsMayReach(priorMutations, block.Match, false)
		if err != nil {
			return futureRequirements{}, err
		}

		blockRelevant := profileOverlap || mutationReachable
		nameWrites := make(map[int]futureNameWrite)
		for ruleIndex, rule := range block.MetricRelabelConfigs {
			writesName, err := relabelAnalyzer.RuleMayWriteLabel(rule, promlabels.MetricName)
			if err != nil {
				return futureRequirements{}, err
			}
			if !writesName {
				continue
			}
			futureReach, err := nameWriteCanReachFuture(relabelAnalyzer, rule, currentNames)
			if err != nil {
				return futureRequirements{}, err
			}

			outputOverlapsProfile := true
			if relabel.EffectiveAction(rule) == relabel.Replace {
				outputPattern, possible, err := relabelAnalyzer.ReplacementGlob(rule.Regex, rule.Replacement)
				if err != nil {
					return futureRequirements{}, err
				}
				outputOverlapsProfile = possible
				if possible {
					outputOverlapsProfile, err = simplePatternScopesMayOverlap(
						matcherAnalyzer, profile.Match, outputPattern,
					)
					if err != nil {
						return futureRequirements{}, err
					}
				}
			}
			nameWrites[ruleIndex] = futureNameWrite{rule: rule, futureReach: futureReach}
			blockRelevant = blockRelevant || (futureReach && outputOverlapsProfile)
		}

		blockScopes := positiveWildcardScopes(
			fmt.Sprintf("relabeling[%d].match", blockIndex), block.Match, blockIndex,
		)
		futureCapable := len(blockScopes) > 0 || mutationReachable
		affectsRouting, err := relabelAnalyzer.RulesMayAffectFutureRouting(block.MetricRelabelConfigs)
		if err != nil {
			return futureRequirements{}, err
		}
		if blockRelevant && futureCapable && affectsRouting {
			requirements.blockScopes = append(requirements.blockScopes, blockScopes...)
			for ruleIndex, rule := range block.MetricRelabelConfigs {
				action := relabel.EffectiveAction(rule)
				writesName, err := relabelAnalyzer.RuleMayWriteLabel(rule, promlabels.MetricName)
				if err != nil {
					return futureRequirements{}, err
				}
				switch action {
				case relabel.Drop, relabel.DropEqual, relabel.Keep, relabel.KeepEqual:
					requirements.rules = append(requirements.rules, futureRuleRequirement{
						blockIndex: blockIndex, ruleIndex: ruleIndex,
					})
				default:
					if writesName && nameWrites[ruleIndex].futureReach {
						allowsAuthoredRouting := false
						if action == relabel.Replace {
							grammar, bounded, err := analyzeBoundedMetricNameRewriteGrammar(
								relabelAnalyzer, rule, action,
							)
							if err != nil {
								return futureRequirements{}, err
							}
							outputs, finite, err := relabelAnalyzer.ReplacementOutputs(rule.Regex, rule.Replacement)
							if err != nil {
								return futureRequirements{}, err
							}
							allowsAuthoredRouting = bounded && finite &&
								len(grammar.nonCanonicalRewriteOutputs(outputs, authoredMetricNames)) == 0
						}
						requirements.rules = append(requirements.rules, futureRuleRequirement{
							blockIndex:            blockIndex,
							ruleIndex:             ruleIndex,
							requireHit:            true,
							allowsAuthoredRouting: allowsAuthoredRouting,
						})
						requirements.requiresExplicit = true
					}
				}
			}
		}

		for _, ruleIndex := range slices.Sorted(maps.Keys(nameWrites)) {
			write := nameWrites[ruleIndex]
			if !write.futureReach {
				continue
			}
			effect, err := flowAnalyzer.Mutation(write.rule, relabel.RuleNameDerivedOnly(write.rule))
			if err != nil {
				return futureRequirements{}, err
			}
			priorMutations = append(priorMutations, effect)
		}
	}
	return requirements, nil
}

func nameWriteCanReachFuture(
	analyzer *relabel.Analyzer,
	rule relabel.Config,
	currentNames map[string]struct{},
) (bool, error) {
	if !relabel.RuleNameDerivedOnly(rule) || len(rule.SourceLabels) != 1 ||
		rule.SourceLabels[0] != promlabels.MetricName {
		return true, nil
	}
	inputs, finite, err := analyzer.EnumerateFiniteRegexp(rule.Regex.String())
	if err != nil || !finite {
		return true, err
	}
	for _, input := range inputs {
		if _, exists := currentNames[input]; !exists {
			return true, nil
		}
	}
	return false, nil
}

func positiveWildcardScopes(path, expr string, blockIndex int) []futureScopeRequirement {
	var requirements []futureScopeRequirement
	var earlier []string
	for term := range strings.FieldsSeq(expr) {
		pattern := strings.TrimPrefix(term, "!")
		negative := strings.HasPrefix(term, "!")
		if !negative && hasUnescapedGlobMeta(pattern) {
			parts := make([]string, 0, len(earlier)+1)
			for _, previous := range earlier {
				parts = append(parts, "!"+previous)
			}
			parts = append(parts, pattern)
			requirements = append(requirements, futureScopeRequirement{
				path: path, pattern: pattern, scopeExpr: strings.Join(parts, " "), blockIndex: blockIndex,
			})
		}
		earlier = append(earlier, pattern)
	}
	return requirements
}

func prepareFutureInputs(
	requirements futureRequirements,
	declared []futureInput,
	current prompkg.SampleBatch,
	authoredMetricNames map[string]struct{},
	r *report,
) ([]futureInput, bool) {
	valid := true
	currentNames := make(map[string]struct{}, len(current.Samples))
	for _, sample := range current.Samples {
		currentNames[sample.Name] = struct{}{}
	}
	for index, input := range declared {
		if _, exists := currentNames[input.Name]; exists {
			valid = false
			r.addError(
				"future_input_not_future",
				fmt.Sprintf("future_inputs[%d].name", index),
				fmt.Sprintf("declared future metric %q already exists in the current source fixture", input.Name),
				"A future probe must add a raw exporter family that is absent from the source-complete current-evidence dump.",
			)
		}
	}
	if requirements.requiresExplicit {
		if len(declared) == 0 {
			valid = false
			r.addError(
				"future_inputs_required",
				"future_inputs",
				"recommended relabeling can change a future metric namespace but declares no raw future inputs",
				"Namespace-changing relabeling cannot be inverted soundly. Declare raw future inputs, including labels needed to exercise every reachable rename/drop-capable branch.",
			)
		}
		return slices.Clone(declared), valid
	}

	inputs := slices.Clone(declared)
	excluded := make(map[string]struct{}, len(currentNames)+len(authoredMetricNames)+len(inputs))
	for name := range currentNames {
		excluded[name] = struct{}{}
	}
	for name := range authoredMetricNames {
		excluded[name] = struct{}{}
	}
	for _, input := range inputs {
		excluded[input.Name] = struct{}{}
	}
	derive := func(scope futureScopeRequirement, unavailableCode string) bool {
		found := 0
		for range futureWitnessesPerScope {
			witness, ok, err := futureScopeWitness(requirements.matcher, scope.scopeExpr, excluded)
			if err != nil {
				valid = false
				r.addError(
					"future_metric_analysis",
					scope.path,
					err.Error(),
					"Future witness generation must complete within the shared matcher analysis budget.",
				)
				return false
			}
			if !ok {
				break
			}
			inputs = append(inputs, futureInput{Name: witness})
			excluded[witness] = struct{}{}
			found++
		}
		if found == 0 {
			valid = false
			r.addError(
				unavailableCode,
				scope.path,
				fmt.Sprintf("cannot derive a new valid raw metric name for wildcard term %q", scope.pattern),
				"Every positive wildcard profile namespace needs at least one raw future witness outside current and authored metric names.",
			)
		}
		return found > 0
	}
	for _, scope := range requirements.profileScopes {
		if !derive(scope, "future_metric_canary_unavailable") {
			return inputs, false
		}
	}
	for _, scope := range requirements.blockScopes {
		if !derive(scope, "future_relabel_canary_unavailable") {
			return inputs, false
		}
	}
	return inputs, valid
}

func futureScopeWitness(
	analyzer *matcher.Analyzer,
	scopeExpr string,
	excluded map[string]struct{},
) (string, bool, error) {
	localExclusions := make(map[string]struct{}, len(excluded))
	for name := range excluded {
		localExclusions[name] = struct{}{}
	}
	for range futureInputSearchAttempts {
		parts := make([]string, 0, len(localExclusions)+1)
		for _, name := range sortedStringKeys(localExclusions) {
			parts = append(parts, "!"+matcher.QuoteGlobLiteral(name))
		}
		parts = append(parts, scopeExpr)
		witness, intersects, err := analyzer.SimplePatternIntersectionWitness(
			scopeExpr, strings.Join(parts, " "), true,
		)
		if err != nil || !intersects {
			return "", intersects, err
		}
		if commonmodel.UTF8Validation.IsValidMetricName(witness) {
			return witness, true, nil
		}
		localExclusions[witness] = struct{}{}
	}
	return "", false, nil
}

type writerSnapshot map[metrix.SeriesID]float64

func snapshotWriter(reader metrix.Reader) writerSnapshot {
	snapshot := make(writerSnapshot)
	reader.ForEachSeriesIdentity(func(identity metrix.SeriesIdentity, _ metrix.SeriesMeta, _ string, _ metrix.LabelView, value metrix.SampleValue) {
		snapshot[identity.ID] = value
	})
	return snapshot
}

func destinationSeriesIDs(reader metrix.Reader) map[prompkg.SampleSeriesIdentity]metrix.SeriesID {
	ids := make(map[prompkg.SampleSeriesIdentity]metrix.SeriesID)
	reader.ForEachSeriesIdentity(func(identity metrix.SeriesIdentity, _ metrix.SeriesMeta, name string, labelView metrix.LabelView, _ metrix.SampleValue) {
		var lbs promlabels.Labels
		labelView.Range(func(key, value string) bool {
			lbs = append(lbs, promlabels.Label{Name: key, Value: value})
			return true
		})
		slices.SortFunc(lbs, func(a, b promlabels.Label) int { return strings.Compare(a.Name, b.Name) })
		ids[prompkg.IdentifySeries(name, lbs, "")] = identity.ID
	})
	return ids
}

type futureProbeState struct {
	index       int
	input       futureInput
	rawIdentity prompkg.RawSampleIdentity
	finalNames  []string
	seriesIDs   []metrix.SeriesID
	open        bool
}

func probeAllowsAuthoredRouting(
	requirements futureRequirements,
	pipeline *pipelineDiagnosticSummary,
	rawIdentity prompkg.RawSampleIdentity,
) bool {
	for _, requirement := range requirements.rules {
		if !requirement.allowsAuthoredRouting {
			continue
		}
		fact, evaluated := pipeline.rulesEvaluated[rawIdentity][pipelineRuleKey{
			block: requirement.blockIndex, rule: requirement.ruleIndex,
		}]
		if evaluated && fact.RelabelRuleMatched {
			return true
		}
	}
	return false
}

func inspectFutureOpenness(
	inputs []futureInput,
	requirements futureRequirements,
	pipeline *pipelineDiagnosticSummary,
	routes *planRouteSummary,
	current writerSnapshot,
	futureReader metrix.Reader,
	r *report,
) {
	futureSnapshot := snapshotWriter(futureReader)
	for id, value := range current {
		futureValue, ok := futureSnapshot[id]
		if !ok || futureValue != value {
			r.addError(
				"future_run_changed_current_evidence",
				"future_inputs",
				fmt.Sprintf("future run changed or removed current writer series %q", id),
				"The future-openness sequence is isolated from current coverage, but adding probes must not mutate, evict, or overwrite current evidence identities.",
			)
		}
	}

	seriesIDs := destinationSeriesIDs(futureReader)
	probes := make([]futureProbeState, 0, len(inputs))
	sourcesByDestination := make(map[prompkg.SampleSeriesIdentity][]int)
	for index, input := range inputs {
		rawIdentity := prompkg.IdentifyRawSample(input.Name, promlabels.FromMap(input.Labels))
		probe := futureProbeState{index: index, input: input, rawIdentity: rawIdentity}
		path := fmt.Sprintf("future_inputs[%d]", index)

		_, rawAccepted := pipeline.rawAccepted[rawIdentity]
		if _, rejected := pipeline.selectorRejected[rawIdentity]; rejected {
			r.addError(
				"future_metric_blocked_by_job_selector", path,
				fmt.Sprintf("raw future metric %q is rejected by the job selector", input.Name),
				"Every future probe must enter the real relabel, assembly, writer, profile, and fallback pipeline.",
			)
		} else if !rawAccepted {
			r.addError(
				"future_metric_missing_from_pipeline", path,
				fmt.Sprintf("raw future metric %q produced no selector decision", input.Name),
				"The staged exposition and production parser must preserve every declared or derived raw probe.",
			)
		}
		drop, dropped := pipeline.relabelDropped[rawIdentity]
		if dropped {
			r.addError(
				"future_metric_blocked_by_job_relabel", path,
				fmt.Sprintf("raw future metric %q is dropped by relabeling block %d rule %d (%s)", input.Name, drop.BlockIndex, drop.RuleIndex, drop.RelabelDrop.Reason),
				"Recommended relabeling must leave unknown future exporter metrics open unless the exclusion is exact, bounded, and current-source-proven.",
			)
		}

		destinations := pipeline.destinationsByRaw[rawIdentity]
		if rawAccepted && !dropped && len(destinations) == 0 {
			r.addError(
				"future_metric_missing_after_relabel", path,
				fmt.Sprintf("raw future metric %q produced no relabel output identity", input.Name),
				"A probe accepted by the raw selector must either produce a production relabel destination or an explicit relabel-drop fact.",
			)
		}
		for destination := range destinations {
			if _, accepted := pipeline.writerAccepted[destination]; !accepted {
				reason := pipeline.writerSeriesRejects[destination]
				if reason == "" {
					reason = pipeline.writerFamilyRejects[destination.series.Family]
				}
				r.addError(
					"future_metric_rejected_by_writer", path,
					fmt.Sprintf("future metric %q reaches writer destination %q but is rejected (%s)", input.Name, destination.series.Family, reason),
					"A future witness proves openness only when the production writer commits its final identity and value.",
				)
				continue
			}
			probe.finalNames = append(probe.finalNames, destination.series.Family)
			sourcesByDestination[destination.series] = append(sourcesByDestination[destination.series], index)
			seriesID, ok := seriesIDs[destination.series]
			if !ok {
				r.addError(
					"future_metric_missing_from_writer", path,
					fmt.Sprintf("accepted future destination %q is absent from the committed reader", destination.series.Family),
					"Pipeline diagnostics and committed metrix evidence must describe the same production writer result.",
				)
				continue
			}
			probe.seriesIDs = append(probe.seriesIDs, seriesID)
			if _, existed := current[seriesID]; existed {
				r.addError(
					"future_metric_identity_collapse", path,
					fmt.Sprintf("future metric %q collapses onto current writer series %q", input.Name, seriesID),
					"A future raw family must retain a distinct final writer identity instead of overwriting current evidence.",
				)
			}
		}
		slices.Sort(probe.finalNames)
		probe.finalNames = slices.Compact(probe.finalNames)
		slices.Sort(probe.seriesIDs)
		probe.seriesIDs = slices.Compact(probe.seriesIDs)

		if len(probe.seriesIDs) > 0 {
			probe.open = true
			allowsAuthoredRouting := probeAllowsAuthoredRouting(requirements, pipeline, rawIdentity)
			for _, id := range probe.seriesIDs {
				route := routes.series[id]
				switch {
				case route == nil:
					probe.open = false
					r.addError(
						"future_metric_missing_from_planner", path,
						fmt.Sprintf("future writer series %q produced no chartengine route fact", id),
						"A future witness must traverse the production chart planner as well as the collector.",
					)
				case route.unmatched:
					probe.open = false
					r.addError(
						"future_metric_blocked_by_profile", path,
						fmt.Sprintf("future writer series %q is unmatched by chart routing (%s)", id, route.unmatchedReason),
						"Unknown future families in the profile namespace must remain eligible for generic fallback charts.",
					)
				case !route.autogen && !allowsAuthoredRouting:
					probe.open = false
					r.addError(
						"future_metric_routed_to_authored_metric", path,
						fmt.Sprintf("future writer series %q routes to authored charts instead of generic fallback", id),
						"A name rewrite must not map unknown future families onto an authored metric contract.",
					)
				}
			}
		}
		probes = append(probes, probe)
	}

	for destination, sources := range sourcesByDestination {
		if len(sources) < 2 {
			continue
		}
		slices.Sort(sources)
		r.addError(
			"future_metric_identity_collapse",
			"future_inputs",
			fmt.Sprintf("future inputs %v collapse onto writer destination %q", sources, destination.Family),
			"Distinct raw future identities must remain distinct after relabeling and writer normalization.",
		)
	}

	for _, scope := range requirements.profileScopes {
		matched := false
		scopeMatcher, err := matcher.NewSimplePatternsMatcher(scope.scopeExpr)
		if err != nil {
			r.addError("future_metric_analysis", scope.path, err.Error(), "Profile wildcard coverage must use the production matcher grammar.")
			continue
		}
		for _, probe := range probes {
			if !probe.open {
				continue
			}
			if slices.ContainsFunc(probe.finalNames, scopeMatcher.MatchString) {
				matched = true
				break
			}
		}
		if !matched {
			r.addError(
				"future_profile_term_uncovered", scope.path,
				fmt.Sprintf("no open future probe reaches positive wildcard profile term %q", scope.pattern),
				"Each positive wildcard profile namespace requires a raw probe that survives selector, relabeling, writer, and generic chart routing.",
			)
		}
	}

	for _, scope := range requirements.blockScopes {
		matched := false
		scopeMatcher, err := matcher.NewSimplePatternsMatcher(scope.scopeExpr)
		if err != nil {
			r.addError("future_metric_analysis", scope.path, err.Error(), "Relabel wildcard coverage must use the production matcher grammar.")
			continue
		}
		for _, probe := range probes {
			if !probe.open {
				continue
			}
			entry, entered := pipeline.blockEntries[probe.rawIdentity][scope.blockIndex]
			if entered && scopeMatcher.MatchString(entry) {
				matched = true
				break
			}
		}
		if !matched {
			r.addError(
				"future_relabel_scope_uncovered", scope.path,
				fmt.Sprintf("no open future probe enters relabel wildcard term %q", scope.pattern),
				"Every reachable wildcard relabel scope that can rename or drop samples needs a surviving raw future witness.",
			)
		}
	}

	for _, requirement := range requirements.rules {
		covered := false
		for _, probe := range probes {
			if !probe.open {
				continue
			}
			fact, evaluated := pipeline.rulesEvaluated[probe.rawIdentity][pipelineRuleKey{
				block: requirement.blockIndex, rule: requirement.ruleIndex,
			}]
			if evaluated && (!requirement.requireHit || fact.RelabelRuleMatched) {
				covered = true
				break
			}
		}
		if !covered {
			path := fmt.Sprintf("relabeling[%d].metric_relabel_configs[%d]", requirement.blockIndex, requirement.ruleIndex)
			r.addError(
				"future_relabel_branch_uncovered", path,
				"no open future probe exercises this reachable rename/drop-capable relabel rule",
				"Declare enough raw future inputs and labels to cover every reachable routing-affecting relabel branch; name-writing rules must take their matching branch.",
			)
		}
	}

}

func appendFutureInputs(exposition []byte, inputs []futureInput) ([]byte, error) {
	encoded, err := encodeFutureInputs(inputs)
	if err != nil {
		return nil, err
	}
	trimmed := bytes.TrimSpace(exposition)
	if bytes.HasSuffix(trimmed, []byte("# EOF")) {
		trimmed = bytes.TrimSpace(bytes.TrimSuffix(trimmed, []byte("# EOF")))
	}
	if len(trimmed) == 0 {
		return nil, errors.New("current exposition is empty")
	}
	combined := make([]byte, 0, len(trimmed)+1+len(encoded))
	combined = append(combined, trimmed...)
	combined = append(combined, '\n')
	combined = append(combined, encoded...)
	return combined, nil
}
