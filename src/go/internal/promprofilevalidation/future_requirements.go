// SPDX-License-Identifier: GPL-3.0-or-later

package promprofilevalidation

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	prompkg "github.com/netdata/netdata/go/plugins/pkg/prometheus"
	promcollector "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/promprofiles"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/relabel"
	commonmodel "github.com/prometheus/common/model"
	promlabels "github.com/prometheus/prometheus/model/labels"
)

const (
	futureWitnessesPerScope   = 3
	futureInputSearchAttempts = 128
)

type futureScopeRequirement struct {
	path      string
	pattern   string
	scopeExpr string
	location  pipelineRelabelLocation
}

type futureRuleRequirement struct {
	location              pipelineRelabelLocation
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
		profileScopes: positiveWildcardScopes("match", profile.Match, pipelineRelabelLocation{block: -1}),
		matcher:       matcherAnalyzer,
	}
	authoredMetricNames := profileAuthoredMetricNames(profile)
	currentNames := make(map[string]struct{}, len(current.Samples))
	for _, sample := range current.Samples {
		currentNames[sample.Name] = struct{}{}
	}
	stages, err := validationRelabelStages(policy, profile)
	if err != nil {
		return futureRequirements{}, err
	}
	var priorMutations []promcollector.RelabelNameMutation
	for _, stage := range stages {
		for blockIndex, block := range stage.blocks {
			location := pipelineRelabelLocation{stage: stage.stage, profile: stage.profile, block: blockIndex}
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
				rule = rule.WithDefaults()
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
				relabelBlockPath(stage.stage, blockIndex)+".match", block.Match, location,
			)
			futureCapable := len(blockScopes) > 0 || mutationReachable
			affectsRouting, err := relabelAnalyzer.RulesMayAffectFutureRouting(block.MetricRelabelConfigs)
			if err != nil {
				return futureRequirements{}, err
			}
			if blockRelevant && futureCapable && affectsRouting {
				requirements.blockScopes = append(requirements.blockScopes, blockScopes...)
				for ruleIndex, rule := range block.MetricRelabelConfigs {
					rule = rule.WithDefaults()
					action := relabel.EffectiveAction(rule)
					writesName, err := relabelAnalyzer.RuleMayWriteLabel(rule, promlabels.MetricName)
					if err != nil {
						return futureRequirements{}, err
					}
					switch action {
					case relabel.Drop, relabel.DropEqual, relabel.Keep, relabel.KeepEqual:
						requirements.rules = append(requirements.rules, futureRuleRequirement{
							location: location, ruleIndex: ruleIndex,
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
								location:              location,
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
	}
	return requirements, nil
}

func nameWriteCanReachFuture(
	analyzer *relabel.Analyzer,
	rule relabel.Config,
	currentNames map[string]struct{},
) (bool, error) {
	rule = rule.WithDefaults()
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

func positiveWildcardScopes(path, expr string, location pipelineRelabelLocation) []futureScopeRequirement {
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
				path: path, pattern: pattern, scopeExpr: strings.Join(parts, " "), location: location,
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
	r *Report,
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
		if !derive(scope, "future_metric_witness_unavailable") {
			return inputs, false
		}
	}
	for _, scope := range requirements.blockScopes {
		if !derive(scope, "future_relabel_witness_unavailable") {
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
