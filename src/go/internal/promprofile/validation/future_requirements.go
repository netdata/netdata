// SPDX-License-Identifier: GPL-3.0-or-later

package promvalidation

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	prompkg "github.com/netdata/netdata/go/plugins/pkg/prometheus"
	promcollector "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus"
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
	owner     string
}

type futureRuleRequirement struct {
	location              pipelineRelabelLocation
	ruleIndex             int
	path                  string
	requireHit            bool
	allowsAuthoredRouting bool
	owner                 string
}

type futureRequirements struct {
	profileScopes    []futureScopeRequirement
	blockScopes      []futureScopeRequirement
	rules            []futureRuleRequirement
	boundedDropRules map[pipelineRuleKey]struct{}
	requiresExplicit bool
	matcher          *matcher.Analyzer
}

type ownedFutureRequirements struct {
	context       *profileValidationContext
	requirements  futureRequirements
	authoredNames map[string]struct{}
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
	subject contributorPolicySubject,
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
	flowAnalyzer, err := newRelabelNameFlowAnalyzer(
		ctx, relabelAnalyzer, matcherAnalyzer, relabelNameFlowBudget{},
	)
	if err != nil {
		return futureRequirements{}, err
	}

	requirements := futureRequirements{
		boundedDropRules: make(map[pipelineRuleKey]struct{}),
		matcher:          matcherAnalyzer,
	}
	if subject.context != nil {
		requirements.profileScopes = positiveWildcardScopes(
			subject.context.path("match"), subject.context.profile.Match, pipelineRelabelLocation{block: -1},
		)
		setFutureScopeOwner(requirements.profileScopes, subject.context.profile.Name)
	}
	authoredMetricNames := subject.authoredNames
	currentNames := make(map[string]struct{}, len(current.Samples))
	for _, sample := range current.Samples {
		currentNames[sample.Name] = struct{}{}
	}
	stages := subject.stages
	var priorMutations []relabelNameMutation
	for _, stage := range stages {
		emitStage := (stage.stage == promcollector.PipelineRelabelStageJob && subject.emitJobStage) ||
			(stage.stage == promcollector.PipelineRelabelStageProfile && subject.emitProfile)
		for blockIndex, block := range stage.blocks {
			location := pipelineRelabelLocation{stage: stage.stage, profile: stage.profile, block: blockIndex}
			_, exactBlockScope := exactRelabelBlockMetricScope(block.Match)
			profileOverlap, err := subject.namespaceOverlaps(matcherAnalyzer, block.Match)
			if err != nil {
				return futureRequirements{}, err
			}
			mutationReachable, err := flowAnalyzer.mutationsMayReach(priorMutations, block.Match, false)
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
						outputOverlapsProfile, err = subject.namespaceOverlaps(matcherAnalyzer, outputPattern)
						if err != nil {
							return futureRequirements{}, err
						}
					}
				}
				nameWrites[ruleIndex] = futureNameWrite{rule: rule, futureReach: futureReach}
				blockRelevant = blockRelevant || (futureReach && outputOverlapsProfile)
			}

			boundedDropRules := make(map[int]struct{})
			futureRoutingRules := make([]relabel.Config, 0, len(block.MetricRelabelConfigs))
			for ruleIndex, rule := range block.MetricRelabelConfigs {
				rule = rule.WithDefaults()
				action := relabel.EffectiveAction(rule)
				if action == relabel.Drop {
					bounded := exactBlockScope
					if !bounded && relabelDiscardIsMetricNameBound(rule, action) {
						_, bounded, err = analyzeBoundedMetricNameDiscardGrammar(relabelAnalyzer, rule, action)
						if err != nil {
							return futureRequirements{}, err
						}
					}
					if bounded && emitStage {
						boundedDropRules[ruleIndex] = struct{}{}
						requirements.boundedDropRules[pipelineRuleKey{location: location, rule: ruleIndex}] = struct{}{}
						continue
					}
				}
				futureRoutingRules = append(futureRoutingRules, rule)
			}

			blockPath := relabelBlockLocationPath(
				location, subject.context != nil && subject.context.composed,
			)
			blockScopes := positiveWildcardScopes(blockPath+".match", block.Match, location)
			if subject.context != nil {
				setFutureScopeOwner(blockScopes, subject.context.profile.Name)
			}
			futureCapable := len(blockScopes) > 0 || mutationReachable
			affectsRouting, err := relabelAnalyzer.RulesMayAffectFutureRouting(futureRoutingRules)
			if err != nil {
				return futureRequirements{}, err
			}
			if emitStage && blockRelevant && futureCapable && affectsRouting {
				requirements.blockScopes = append(requirements.blockScopes, blockScopes...)
				for ruleIndex, rule := range block.MetricRelabelConfigs {
					if _, bounded := boundedDropRules[ruleIndex]; bounded {
						// Current source evidence and the bounded grammar prove this
						// deliberate exclusion. It does not need an open future witness.
						continue
					}
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
							path:  fmt.Sprintf("%s.metric_relabel_configs[%d]", blockPath, ruleIndex),
							owner: futureRequirementOwner(subject),
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
								path:                  fmt.Sprintf("%s.metric_relabel_configs[%d]", blockPath, ruleIndex),
								requireHit:            true,
								allowsAuthoredRouting: allowsAuthoredRouting,
								owner:                 futureRequirementOwner(subject),
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
				effect, err := flowAnalyzer.mutation(write.rule, relabel.RuleNameDerivedOnly(write.rule))
				if err != nil {
					return futureRequirements{}, err
				}
				priorMutations = append(priorMutations, effect)
			}
		}
	}
	return requirements, nil
}

func futureRequirementOwner(subject contributorPolicySubject) string {
	if subject.context == nil {
		return ""
	}
	return subject.context.profile.Name
}

func setFutureScopeOwner(requirements []futureScopeRequirement, owner string) {
	for index := range requirements {
		requirements[index].owner = owner
	}
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
	owned []ownedFutureRequirements,
	declared []futureInput,
	current prompkg.SampleBatch,
	r *Report,
) ([]futureInput, bool) {
	valid := true
	currentNames := currentPhysicalAndFamilyNames(current)
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
	inputs := slices.Clone(declared)
	excluded := make(map[string]struct{}, len(currentNames)+len(inputs))
	for name := range currentNames {
		excluded[name] = struct{}{}
	}
	for _, item := range owned {
		for name := range item.authoredNames {
			excluded[name] = struct{}{}
		}
	}
	for _, input := range inputs {
		excluded[input.Name] = struct{}{}
	}
	for _, item := range owned {
		if !item.requirements.requiresExplicit {
			continue
		}
		if len(declared) == 0 {
			valid = false
			path := "future_inputs"
			if item.context != nil && item.context.composed {
				path += ", " + item.context.path("relabeling")
			}
			r.addError(
				"future_inputs_required",
				path,
				"recommended relabeling can change a future metric namespace but declares no raw future inputs",
				"Namespace-changing relabeling cannot be inverted soundly. Declare raw future inputs, including labels needed to exercise every reachable rename/drop-capable branch.",
			)
		}
	}
	derive := func(analyzer *matcher.Analyzer, scope futureScopeRequirement, unavailableCode string) bool {
		found := 0
		for range futureWitnessesPerScope {
			witness, ok, err := futureScopeWitness(analyzer, scope.scopeExpr, excluded)
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
	for _, item := range owned {
		requirements := item.requirements
		if requirements.requiresExplicit {
			continue
		}
		for _, scope := range requirements.profileScopes {
			if !derive(requirements.matcher, scope, "future_metric_witness_unavailable") {
				return inputs, false
			}
		}
		for _, scope := range requirements.blockScopes {
			if !derive(requirements.matcher, scope, "future_relabel_witness_unavailable") {
				return inputs, false
			}
		}
	}
	return inputs, valid
}

func currentPhysicalAndFamilyNames(current prompkg.SampleBatch) map[string]struct{} {
	names := make(map[string]struct{}, len(current.Samples)*2)
	for _, sample := range current.Samples {
		names[sample.Name] = struct{}{}
		names[prompkg.SampleFamilyName(sample)] = struct{}{}
	}
	return names
}

func combineFutureRequirements(owned []ownedFutureRequirements) futureRequirements {
	combined := futureRequirements{boundedDropRules: make(map[pipelineRuleKey]struct{})}
	for _, item := range owned {
		requirements := item.requirements
		combined.profileScopes = append(combined.profileScopes, requirements.profileScopes...)
		combined.blockScopes = append(combined.blockScopes, requirements.blockScopes...)
		combined.rules = append(combined.rules, requirements.rules...)
		for key := range requirements.boundedDropRules {
			combined.boundedDropRules[key] = struct{}{}
		}
	}
	return combined
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
