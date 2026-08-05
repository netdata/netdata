// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"fmt"
	"maps"
	"regexp/syntax"
	"slices"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	metrixselector "github.com/netdata/netdata/go/plugins/pkg/metrix/selector"
	prompkg "github.com/netdata/netdata/go/plugins/pkg/prometheus"
	promselector "github.com/netdata/netdata/go/plugins/pkg/prometheus/selector"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	promcollector "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/promprofiles"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/relabel"
	commonmodel "github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
)

// Varied stems prevent a narrow job allowlist from passing merely because it
// happens to admit the validator's primary probe name.
var futureMetricStems = [...]string{
	"netdata_future_metric",
	"upstream_added_metric",
	"exporter_new_signal",
}

const futureMetricCandidateAlphabet = "_abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789:"

// addForwardCompatibilityChecks keeps openness structural and uses synthetic
// names beyond the bounded source fixture. The fixture is consulted only to
// prove exact exclusions and every explicitly bounded wildcard name grammar.
func addForwardCompatibilityChecks(
	ctx context.Context,
	profile promprofiles.Profile,
	policy jobPolicy,
	rawFamilies []rawFamilyReport,
	rawSamples prompkg.SampleBatch,
	relabelAudits relabelPolicyAudits,
	r *report,
) error {
	analyzer, err := relabel.NewAnalyzer(ctx, relabel.AnalysisBudget{
		MaxValues: maxBoundedMetricNameGrammarBranches,
	})
	if err != nil {
		return err
	}
	matcherAnalyzer, err := matcher.NewAnalyzer(ctx, matcher.AnalysisBudget{})
	if err != nil {
		return err
	}
	flowAnalyzer, err := promcollector.NewRelabelNameFlowAnalyzer(
		ctx,
		analyzer,
		matcherAnalyzer,
		promcollector.RelabelNameFlowBudget{},
	)
	if err != nil {
		return err
	}
	expr := profile.AutogenSelector()
	authoredMetricNames := profileAuthoredMetricNames(profile)
	rawFamilyNames := make(map[string]struct{}, len(rawFamilies))
	for _, family := range rawFamilies {
		rawFamilyNames[family.Name] = struct{}{}
	}
	rawSampleNames := make(map[string]struct{}, len(rawSamples.Samples))
	for _, sample := range rawSamples.Samples {
		rawSampleNames[sample.Name] = struct{}{}
	}
	if collapse := relabelAudits.identityCollapse; collapse.finalIdentities > 0 {
		r.addError(
			"observed_relabel_identity_collapse",
			"relabeling",
			fmt.Sprintf(
				"recommended relabeling maps %d observed writer-admissible source identities onto %d final identities across metric families %q",
				collapse.sourceIdentities, collapse.finalIdentities, collapse.finalFamilies,
			),
			"Relabeling may normalize names or labels only when each observed source identity remains distinct. Many-to-one relabeling makes the writer retain one value for multiple source series instead of aggregating them.",
		)
	}
	if drops := relabelAudits.invalidNameDrops; drops.rawSeries > 0 {
		r.addError(
			"invalid_relabel_metric_name_discard",
			"relabeling",
			fmt.Sprintf(
				"recommended relabeling implicitly drops %d observed logical identities across %d source families (%d raw exposition series) by producing an empty or invalid metric name in blocks %v",
				len(drops.logicalIdentities), len(drops.families), drops.rawSeries, slices.Sorted(maps.Keys(drops.blocks)),
			),
			"A metric-name rewrite must produce a valid name for every source-fixture sample it reaches. Express an intentional known exclusion with an explicit, bounded, source-evidenced drop rule instead of relying on relabel validation to discard it.",
		)
	}
	if expr != nil {
		if len(expr.Allow) > 0 {
			r.addError(
				"closed_profile_fallback",
				"autogen.selector.allow",
				"profile fallback uses an allowlist",
				"An allowlist suppresses every unmatched current or future family outside the list. Contributed profiles must leave unknown matching families eligible for generic fallback.",
			)
		}
		for i, item := range expr.Deny {
			name, ok := exactMetricFamilySelector(item)
			if !ok {
				r.addError(
					"open_ended_profile_fallback_deny",
					fmt.Sprintf("autogen.selector.deny[%d]", i),
					fmt.Sprintf("fallback deny %q does not name one exact metric family", item),
					"Profile fallback denies must identify a bounded, source-proven family. A wildcard or label-only rule can hide future exporter families.",
				)
			} else if _, ok := rawFamilyNames[name]; !ok {
				r.addError(
					"unproven_profile_fallback_deny",
					fmt.Sprintf("autogen.selector.deny[%d]", i),
					fmt.Sprintf("fallback deny names metric family %q, which is absent from the source-complete fixture", name),
					"An exact name is not proof that a metric is known. Add sanitized source-derived family evidence or remove the fallback exclusion.",
				)
			}
		}
	}
	for i, item := range policy.Selector.Deny {
		name, ok := exactMetricFamilySelector(item)
		if !ok {
			r.addError(
				"open_ended_job_selector_deny",
				fmt.Sprintf("selector.deny[%d]", i),
				fmt.Sprintf("job selector deny %q does not name one exact exposition metric", item),
				"Recommended job selectors may deny exact known exposition metric names. Use a bounded, source-proven relabel rule when a dynamic alias grammar cannot be enumerated.",
			)
		} else if _, ok := rawSampleNames[name]; !ok {
			r.addError(
				"unproven_job_selector_deny",
				fmt.Sprintf("selector.deny[%d]", i),
				fmt.Sprintf("job selector deny names exposition metric %q, which is absent from the source-complete fixture", name),
				"An exact name is not proof that a metric is known. Add sanitized source-derived exposition evidence or remove the job exclusion.",
			)
		}
	}
	if missing := uncoveredWildcardProfileTerms(profile.Match, policy.Selector.Allow); len(missing) > 0 {
		r.addError(
			"closed_job_selector_allow",
			"selector.allow",
			fmt.Sprintf("job selector allow does not structurally cover profile namespace terms %q", missing),
			"When a recommended job has an allow list, copy profile.match, copy each positive wildcard term unchanged into an unconstrained allow expression, or use '*'. Synthetic probes alone cannot prove an allowlist is open to every future name.",
		)
	}
	var priorNameMutations []promcollector.RelabelNameMutation
	for blockIndex, block := range policy.Relabeling {
		mayReceiveMutation, err := flowAnalyzer.MutationsMayReach(priorNameMutations, block.Match, false)
		if err != nil {
			return err
		}
		mayReceiveApplicationMutation, err := flowAnalyzer.MutationsMayReach(priorNameMutations, block.Match, true)
		if err != nil {
			return err
		}
		profileOverlap, err := simplePatternScopesMayOverlap(matcherAnalyzer, profile.Match, block.Match)
		if err != nil {
			return err
		}
		originalNameAtEntry := !mayReceiveMutation
		nameDerivedOnlyFromOriginal := !mayReceiveApplicationMutation
		blockReachable := profileOverlap || !originalNameAtEntry
		exactScopeNames, exactScope := exactRelabelBlockMetricScope(block.Match)
		boundedExactScope := exactScope && originalNameAtEntry
		if boundedExactScope {
			for _, name := range exactScopeNames {
				if _, ok := rawSampleNames[name]; !ok {
					r.addError(
						"unproven_exact_relabel_scope",
						fmt.Sprintf("relabeling[%d].match", blockIndex),
						fmt.Sprintf("exact relabel scope names exposition metric %q, which is absent from the source-complete fixture", name),
						"An exact block is bounded only after every positive metric name is proven by sanitized source-derived exposition evidence.",
					)
				}
			}
		}
		for ruleIndex, rule := range block.MetricRelabelConfigs {
			action := relabel.EffectiveAction(rule)
			if action == relabel.Keep || action == relabel.KeepEqual {
				r.addError(
					"closed_relabel_filter",
					fmt.Sprintf("relabeling[%d].metric_relabel_configs[%d].action", blockIndex, ruleIndex),
					fmt.Sprintf("relabel action %q drops every sample that does not satisfy its condition", action),
					"Contributed profile jobs must express known exclusions with bounded drop or dropequal rules instead of inverse keep filters.",
				)
				continue
			}
			if !blockReachable {
				continue
			}
			mayWriteMetricName, err := analyzer.RuleMayWriteLabel(rule, labels.MetricName)
			if err != nil {
				return err
			}
			mutationReadsOnlyName := relabel.RuleNameDerivedOnly(rule)
			mutationOutputIsNameDerived := mutationReadsOnlyName && nameDerivedOnlyFromOriginal
			if mayWriteMetricName && !boundedExactScope && !mutationOutputIsNameDerived {
				r.addError(
					"unbounded_metric_name_rewrite",
					fmt.Sprintf("relabeling[%d].metric_relabel_configs[%d].target_label", blockIndex, ruleIndex),
					fmt.Sprintf("relabel action %q may rewrite __name__ from application labels without an original exact metric scope %q", action, block.Match),
					"A wildcard relabel block may rewrite the metric name only from the existing __name__. Label-derived name mutation can rename or invalidate unknown future families; use an exact known metric block instead.",
				)
			}
			if mayWriteMetricName && !boundedExactScope && mutationOutputIsNameDerived {
				path := fmt.Sprintf("relabeling[%d].metric_relabel_configs[%d]", blockIndex, ruleIndex)
				audit := relabelAudits.nameRewrites[relabelDiscardRuleKey{block: blockIndex, rule: ruleIndex}]
				grammar, ok, err := analyzeBoundedMetricNameRewriteGrammar(analyzer, rule, action)
				if err != nil {
					return err
				}
				outputs, outputsFinite, err := analyzer.ReplacementOutputs(rule.Regex, rule.Replacement)
				if err != nil {
					return err
				}
				identityLabel := ""
				identityPreserved := true
				if ok && grammar.hasDynamicIdentity() && outputsFinite {
					identityLabel, identityPreserved, err = flowAnalyzer.PreservedCaptureLabel(
						policy.Relabeling, blockIndex, ruleIndex, rule, grammar.dynamicCaptureIDs, outputs,
					)
					if err != nil {
						return err
					}
					if identityPreserved && audit != nil {
						if _, exists := audit.blockInputLabelNames[identityLabel]; exists {
							identityPreserved = false
						}
					}
				}
				if !ok {
					r.addError(
						"open_ended_relabel_name_rewrite",
						path+".regex",
						fmt.Sprintf("relabel action %q under wildcard scope %q does not define a bounded metric-name rewrite input grammar", action, block.Match),
						"A wildcard name rewrite must be replace from __name__ to __name__ and enumerate exact input names, one non-empty internal entity key between finite exporter prefixes and finite terminal metric suffixes, or a canonical_name_<dynamic> input rewritten exactly to canonical_name. Open-ended conditional rewrites can reroute or invalidate future families.",
					)
				} else if grammar.hasInternalDynamicKey() && !outputsFinite {
					r.addError(
						"open_ended_relabel_name_rewrite",
						path+".replacement",
						fmt.Sprintf("relabel replacement %q depends on the input grammar's dynamic entity capture", rule.Replacement),
						"An internal dynamic identity may vary beyond the fixture. Metric-name output must use only constants and finite capture branches so every future identity maps to a fixed authored canonical metric.",
					)
				} else if grammar.hasDynamicIdentity() && !identityPreserved {
					r.addError(
						"unpreserved_relabel_name_identity",
						path,
						fmt.Sprintf("relabel rewrite under wildcard scope %q removes a dynamic metric-name identity without first copying it to a stable label", block.Match),
						"Before canonicalizing a dynamic metric name, an earlier replace rule in the same block must copy unchanged the capture enclosing the entire dynamic entity region from __name__ into a static non-__name__ label. A nested capture covering only one alternative is incomplete; distinct exporter entities could otherwise collapse into one writer series.",
					)
				} else {
					var observed map[string]struct{}
					if audit != nil {
						observed = audit.metricNames
					}
					if missing := grammar.missingEvidence(observed); len(missing) > 0 {
						r.addError(
							"unproven_relabel_name_rewrite",
							path,
							fmt.Sprintf("bounded metric-name rewrite grammar has no supplied source-fixture evidence for %v", missing),
							"Every finite exact name, dynamic-alias prefix/suffix branch, or canonical dynamic-tail prefix must reach the rewrite in the source-complete fixture. Add sanitized source-derived evidence or remove the rewrite.",
						)
					} else if invalidOutputs := grammar.nonCanonicalRewriteOutputs(outputs, authoredMetricNames); len(invalidOutputs) > 0 {
						r.addError(
							"open_ended_relabel_name_rewrite",
							path+".replacement",
							fmt.Sprintf("bounded metric-name rewrite can produce non-authored canonical outputs %q", invalidOutputs),
							"A dynamic alias rewrite must map every finite branch to a valid metric name selected by the profile. Otherwise a future identity can leave the profile namespace or fall through under unintended semantics.",
						)
					} else if want := grammar.finiteBranchCount(); grammar.hasDynamicIdentity() && len(outputs) != want {
						r.addError(
							"unpreserved_relabel_name_identity",
							path+".replacement",
							fmt.Sprintf("bounded metric-name rewrite has %d finite input branches but %d distinct canonical outputs", want, len(outputs)),
							"The canonical output name must preserve every finite prefix/suffix distinction not carried by the extracted dynamic identity label. Otherwise different raw families can become one writer series.",
						)
					}
				}
			}
			if (action == relabel.Drop || action == relabel.DropEqual) &&
				!boundedExactScope && !nameDerivedOnlyFromOriginal &&
				relabelDiscardIsMetricNameBound(rule, action) {
				r.addError(
					"tainted_relabel_name_discard",
					fmt.Sprintf("relabeling[%d].metric_relabel_configs[%d].action", blockIndex, ruleIndex),
					fmt.Sprintf("relabel action %q reads a metric name that an earlier rule may derive from application labels", action),
					"Sample discard may use __name__ only while it remains derived exclusively from the original metric name. Move discard before label-derived renaming or scope it in a separate recommended job.",
				)
			}
			if (action == relabel.Drop || action == relabel.DropEqual) &&
				!boundedExactScope && relabelDiscardIsMetricNameBound(rule, action) {
				path := fmt.Sprintf("relabeling[%d].metric_relabel_configs[%d]", blockIndex, ruleIndex)
				grammar, ok, err := analyzeBoundedMetricNameDiscardGrammar(analyzer, rule, action)
				if err != nil {
					return err
				}
				if !ok {
					r.addError(
						"open_ended_relabel_name_discard",
						path+".regex",
						fmt.Sprintf("relabel action %q under wildcard scope %q does not define a bounded metric-name alias grammar", action, block.Match),
						"A wildcard name drop must enumerate exact metric names or match one non-empty internal entity key between a finite exporter prefix and finite terminal metric suffixes. Open-ended terminal regexes and wildcard dropequal can suppress future families.",
					)
				} else {
					audit := relabelAudits.discards[relabelDiscardRuleKey{block: blockIndex, rule: ruleIndex}]
					var observed map[string]struct{}
					if audit != nil {
						observed = audit.metricNames
					}
					if missing := grammar.missingEvidence(observed); len(missing) > 0 {
						r.addError(
							"unproven_relabel_name_discard",
							path,
							fmt.Sprintf("bounded metric-name discard grammar has no supplied source-fixture evidence for %v", missing),
							"Every finite exact name or dynamic-alias prefix/suffix branch must drop at least one sample in the source-complete fixture. Add sanitized source-derived evidence or remove the exclusion.",
						)
					}
				}
			}
			if (action == relabel.Drop || action == relabel.DropEqual) && boundedExactScope {
				audit := relabelAudits.discards[relabelDiscardRuleKey{block: blockIndex, rule: ruleIndex}]
				if audit == nil || audit.rawSeries == 0 {
					r.addError(
						"unproven_exact_relabel_discard",
						fmt.Sprintf("relabeling[%d].metric_relabel_configs[%d]", blockIndex, ruleIndex),
						fmt.Sprintf("relabel action %q under exact scope %q drops no source-complete fixture sample", action, block.Match),
						"An exact block proves only the possible metric names. Every sample-discard rule must also exercise its label/name condition against sanitized source-derived evidence.",
					)
				}
			}
			if (action == relabel.Drop || action == relabel.DropEqual) &&
				!boundedExactScope && !relabelDiscardIsMetricNameBound(rule, action) {
				r.addError(
					"unbounded_relabel_discard",
					fmt.Sprintf("relabeling[%d].metric_relabel_configs[%d].action", blockIndex, ruleIndex),
					fmt.Sprintf("relabel action %q can discard samples by labels without an original exact metric scope %q", action, block.Match),
					"Label-dependent discard is bounded only when the relabel block names exact known metrics. Under wildcard blocks, only a structurally bounded, source-evidenced drop derived exclusively from __name__ is accepted.",
				)
			}
			if mayWriteMetricName {
				effect, err := flowAnalyzer.Mutation(rule, mutationOutputIsNameDerived)
				if err != nil {
					return err
				}
				priorNameMutations = append(priorNameMutations, effect)
				if !mutationOutputIsNameDerived {
					nameDerivedOnlyFromOriginal = false
				}
			}
		}
	}

	canaries, wildcard := syntheticFutureMetrics(profile.Match)
	if len(canaries) == 0 {
		if wildcard {
			r.addError(
				"future_metric_canary_unavailable",
				"match",
				fmt.Sprintf("cannot synthesize a valid future Prometheus family from profile match %q", profile.Match),
				"A contributed wildcard profile must have a deterministic valid future-family probe so closed profile and job policy cannot pass on bounded current evidence.",
			)
		}
		return nil
	}
	r.Profile.FutureMetricCanary = canaries[0]
	checkedCanaries := make(map[string]struct{}, len(canaries))
	appliedBlocksByCanary := make(map[string]map[int]struct{}, len(canaries))
	processedPrimaryNames := make(map[string]string, len(canaries))
	for _, canary := range canaries {
		processed, dropped, appliedBlocks := checkFutureMetricPolicy(canary, expr, policy, authoredMetricNames, true, r)
		if !dropped {
			if first, ok := processedPrimaryNames[processed.Name]; ok && first != canary {
				r.addError(
					"future_metric_identity_collapse",
					"relabeling",
					fmt.Sprintf("synthetic future families %q and %q are both relabeled to %q", first, canary, processed.Name),
					"Distinct unknown families must keep distinct generic identities after recommended relabeling. Many-to-one renaming can merge unrelated values or metric types.",
				)
			} else {
				processedPrimaryNames[processed.Name] = canary
			}
		}
		checkedCanaries[canary] = struct{}{}
		appliedBlocksByCanary[canary] = appliedBlocks
	}

	profileScope, err := matcher.NewSimplePatternsMatcher(profile.Match)
	if err != nil {
		return err
	}
	for blockIndex, block := range policy.Relabeling {
		termCanaries, _ := syntheticFutureMetricTerms(block.Match)
		for _, term := range termCanaries {
			covered := false
			for _, blockCanary := range term.canaries {
				if !profileScope.MatchString(blockCanary) {
					continue
				}
				if _, ok := checkedCanaries[blockCanary]; !ok {
					_, _, appliedBlocks := checkFutureMetricPolicy(blockCanary, expr, policy, authoredMetricNames, false, r)
					checkedCanaries[blockCanary] = struct{}{}
					appliedBlocksByCanary[blockCanary] = appliedBlocks
				}
				if _, ok := appliedBlocksByCanary[blockCanary][blockIndex]; ok {
					covered = true
				}
			}
			affectsRouting, err := analyzer.RulesMayAffectFutureRouting(block.MetricRelabelConfigs)
			if err != nil {
				return err
			}
			termOverlap, err := simplePatternScopesMayOverlap(matcherAnalyzer, profile.Match, term.scopeExpr())
			if err != nil {
				return err
			}
			if !covered && affectsRouting && termOverlap {
				r.addError(
					"future_relabel_canary_unavailable",
					fmt.Sprintf("relabeling[%d].match", blockIndex),
					fmt.Sprintf("cannot synthesize a future-family probe for wildcard term %q inside profile scope %q and relabel scope %q", term.pattern, profile.Match, block.Match),
					"Every positive wildcard term in a relabel block that can discard or rename metrics must expose a deterministic future-family probe. One harmless term cannot prove that another term preserves later exporter families.",
				)
			}
		}
	}
	return nil
}

// uncoveredWildcardProfileTerms returns wildcard exporter namespaces that a
// non-empty job allowlist does not cover structurally. Contributor jobs can
// always express a safe scope by copying profile.match or its positive terms;
// this avoids pretending that a finite set of public canaries proves inclusion.
func uncoveredWildcardProfileTerms(profileMatch string, allows []string) []string {
	if len(allows) == 0 {
		return nil
	}

	var required []string
	for term := range strings.FieldsSeq(profileMatch) {
		if strings.HasPrefix(term, "!") || !hasUnescapedGlobMeta(term) {
			continue
		}
		required = append(required, term)
	}
	if len(required) == 0 {
		return nil
	}

	covered := make(map[string]struct{}, len(required))
	profileTerms := strings.Join(strings.Fields(profileMatch), " ")
	for _, item := range allows {
		pattern, ok := unconstrainedMetricPattern(item)
		if !ok {
			continue
		}
		terms := strings.Fields(pattern)
		if strings.Join(terms, " ") == profileTerms {
			for _, term := range required {
				covered[term] = struct{}{}
			}
			continue
		}
		allPositive := true
		for _, term := range terms {
			if strings.HasPrefix(term, "!") {
				allPositive = false
				break
			}
		}
		if !allPositive {
			continue
		}
		for _, allowTerm := range terms {
			for _, requiredTerm := range required {
				if allowTerm == "*" || allowTerm == requiredTerm {
					covered[requiredTerm] = struct{}{}
				}
			}
		}
	}

	missing := make([]string, 0, len(required))
	for _, term := range required {
		if _, ok := covered[term]; !ok {
			missing = append(missing, term)
		}
	}
	return missing
}

func unconstrainedMetricPattern(selector string) (string, bool) {
	selector = strings.TrimSpace(selector)
	if selector == "" || strings.Contains(selector, "{") {
		return "", false
	}
	return selector, true
}

func exactRelabelBlockMetricScope(matchExpr string) ([]string, bool) {
	hasPositive := false
	var names []string
	for term := range strings.FieldsSeq(matchExpr) {
		if strings.HasPrefix(term, "!") {
			continue
		}
		hasPositive = true
		if hasUnescapedGlobMeta(term) || !commonmodel.UTF8Validation.IsValidMetricName(term) {
			return nil, false
		}
		names = append(names, term)
	}
	// A negative-only simple-pattern expression matches nothing and is bounded.
	return names, hasPositive || len(strings.Fields(matchExpr)) > 0
}

func relabelDiscardIsMetricNameBound(rule relabel.Config, action relabel.Action) bool {
	if len(rule.SourceLabels) != 1 || rule.SourceLabels[0] != labels.MetricName {
		return false
	}
	return action != relabel.DropEqual || rule.TargetLabel == labels.MetricName
}

const maxBoundedMetricNameGrammarBranches = 256

type boundedMetricNameGrammar struct {
	exactNames          []string
	prefixes            []string
	suffixes            []string
	dynamicTailPrefixes []string
	dynamicCaptureIDs   []int
}

func (g boundedMetricNameGrammar) hasInternalDynamicKey() bool {
	return len(g.prefixes) > 0
}

func (g boundedMetricNameGrammar) hasDynamicIdentity() bool {
	return g.hasInternalDynamicKey() || len(g.dynamicTailPrefixes) > 0
}

func analyzeBoundedMetricNameDiscardGrammar(
	analyzer *relabel.Analyzer,
	rule relabel.Config,
	action relabel.Action,
) (boundedMetricNameGrammar, bool, error) {
	if action != relabel.Drop {
		return boundedMetricNameGrammar{}, false, nil
	}
	return analyzeBoundedMetricNameGrammar(analyzer, rule.Regex.String(), false)
}

func analyzeBoundedMetricNameRewriteGrammar(
	analyzer *relabel.Analyzer,
	rule relabel.Config,
	action relabel.Action,
) (boundedMetricNameGrammar, bool, error) {
	if action != relabel.Replace || len(rule.SourceLabels) != 1 || rule.SourceLabels[0] != labels.MetricName ||
		rule.TargetLabel != labels.MetricName {
		return boundedMetricNameGrammar{}, false, nil
	}
	grammar, ok, err := analyzeBoundedMetricNameGrammar(analyzer, rule.Regex.String(), true)
	if err != nil || !ok {
		return boundedMetricNameGrammar{}, false, err
	}
	if len(grammar.dynamicTailPrefixes) == 0 {
		return grammar, true, nil
	}
	if len(grammar.dynamicTailPrefixes) != 1 {
		return boundedMetricNameGrammar{}, false, nil
	}
	prefix := grammar.dynamicTailPrefixes[0]
	if !strings.HasSuffix(prefix, "_") || rule.Replacement != strings.TrimSuffix(prefix, "_") {
		return boundedMetricNameGrammar{}, false, nil
	}
	return grammar, true, nil
}

func analyzeBoundedMetricNameGrammar(
	analyzer *relabel.Analyzer,
	expr string,
	allowDynamicTail bool,
) (boundedMetricNameGrammar, bool, error) {
	parsed, err := syntax.Parse(expr, syntax.Perl)
	if err != nil {
		return boundedMetricNameGrammar{}, false, nil
	}
	parsed = parsed.Simplify()
	if names, finite, err := analyzer.EnumerateFiniteSyntax(parsed); err != nil {
		return boundedMetricNameGrammar{}, false, err
	} else if finite {
		if len(names) == 0 {
			return boundedMetricNameGrammar{}, false, nil
		}
		for _, name := range names {
			if !commonmodel.UTF8Validation.IsValidMetricName(name) {
				return boundedMetricNameGrammar{}, false, nil
			}
		}
		return boundedMetricNameGrammar{exactNames: names}, true, nil
	}

	parts := flattenRegexpConcat(parsed, nil)
	dynamicIndex := -1
	for i, part := range parts {
		if _, finite, err := analyzer.EnumerateFiniteSyntax(part.expr); err != nil {
			return boundedMetricNameGrammar{}, false, err
		} else if finite {
			continue
		}
		if dynamicIndex >= 0 {
			// Stock alias normalization has one dynamic entity key. Multiple
			// open-ended regions cannot be reviewed as a bounded metric grammar.
			return boundedMetricNameGrammar{}, false, nil
		}
		dynamicIndex = i
	}
	if dynamicIndex <= 0 || regexpCanMatchEmpty(parts[dynamicIndex].expr) {
		return boundedMetricNameGrammar{}, false, nil
	}

	prefixes, ok, err := enumerateFiniteRegexpSequence(analyzer, parts[:dynamicIndex])
	if err != nil {
		return boundedMetricNameGrammar{}, false, err
	}
	if !ok || len(prefixes) == 0 {
		return boundedMetricNameGrammar{}, false, nil
	}
	for _, prefix := range prefixes {
		if prefix == "" || !commonmodel.UTF8Validation.IsValidMetricName(prefix) {
			return boundedMetricNameGrammar{}, false, nil
		}
	}
	if dynamicIndex == len(parts)-1 {
		if !allowDynamicTail {
			return boundedMetricNameGrammar{}, false, nil
		}
		return boundedMetricNameGrammar{
			dynamicTailPrefixes: prefixes,
			dynamicCaptureIDs:   slices.Clone(parts[dynamicIndex].enclosingCaptureIDs),
		}, true, nil
	}

	suffixes, ok, err := enumerateFiniteRegexpSequence(analyzer, parts[dynamicIndex+1:])
	if err != nil {
		return boundedMetricNameGrammar{}, false, err
	}
	if !ok || len(suffixes) == 0 || len(prefixes) > maxBoundedMetricNameGrammarBranches/len(suffixes) {
		return boundedMetricNameGrammar{}, false, nil
	}
	for _, suffix := range suffixes {
		if !strings.HasPrefix(suffix, "_") ||
			!commonmodel.UTF8Validation.IsValidMetricName(strings.TrimPrefix(suffix, "_")) {
			return boundedMetricNameGrammar{}, false, nil
		}
	}
	return boundedMetricNameGrammar{
		prefixes:          prefixes,
		suffixes:          suffixes,
		dynamicCaptureIDs: slices.Clone(parts[dynamicIndex].enclosingCaptureIDs),
	}, true, nil
}

type regexpConcatPart struct {
	expr                *syntax.Regexp
	enclosingCaptureIDs []int
}

func flattenRegexpConcat(re *syntax.Regexp, enclosingCaptureIDs []int) []regexpConcatPart {
	if re.Op == syntax.OpCapture && len(re.Sub) == 1 {
		return flattenRegexpConcat(re.Sub[0], append(slices.Clone(enclosingCaptureIDs), re.Cap))
	}
	if re.Op != syntax.OpConcat {
		return []regexpConcatPart{{expr: re, enclosingCaptureIDs: slices.Clone(enclosingCaptureIDs)}}
	}
	var out []regexpConcatPart
	for _, sub := range re.Sub {
		out = append(out, flattenRegexpConcat(sub, enclosingCaptureIDs)...)
	}
	return out
}

func enumerateFiniteRegexpSequence(
	analyzer *relabel.Analyzer,
	parts []regexpConcatPart,
) ([]string, bool, error) {
	if len(parts) == 0 {
		return []string{""}, true, nil
	}
	subs := make([]*syntax.Regexp, 0, len(parts))
	for _, part := range parts {
		subs = append(subs, part.expr)
	}
	return analyzer.EnumerateFiniteSyntax(&syntax.Regexp{Op: syntax.OpConcat, Sub: subs})
}

func regexpCanMatchEmpty(re *syntax.Regexp) bool {
	switch re.Op {
	case syntax.OpNoMatch, syntax.OpLiteral, syntax.OpCharClass, syntax.OpAnyCharNotNL, syntax.OpAnyChar:
		return false
	case syntax.OpEmptyMatch, syntax.OpBeginLine, syntax.OpEndLine, syntax.OpBeginText,
		syntax.OpEndText, syntax.OpWordBoundary, syntax.OpNoWordBoundary, syntax.OpQuest, syntax.OpStar:
		return true
	case syntax.OpCapture, syntax.OpPlus:
		return len(re.Sub) == 0 || regexpCanMatchEmpty(re.Sub[0])
	case syntax.OpConcat:
		for _, sub := range re.Sub {
			if !regexpCanMatchEmpty(sub) {
				return false
			}
		}
		return true
	case syntax.OpAlternate:
		return slices.ContainsFunc(re.Sub, regexpCanMatchEmpty)
	case syntax.OpRepeat:
		return re.Min == 0 || len(re.Sub) == 0 || regexpCanMatchEmpty(re.Sub[0])
	default:
		return false
	}
}

func (g boundedMetricNameGrammar) missingEvidence(observed map[string]struct{}) []string {
	if len(g.exactNames) > 0 {
		var missing []string
		for _, name := range g.exactNames {
			if _, ok := observed[name]; !ok {
				missing = append(missing, name)
			}
		}
		return missing
	}
	if len(g.dynamicTailPrefixes) > 0 {
		var missing []string
		for _, prefix := range g.dynamicTailPrefixes {
			found := false
			for name := range observed {
				if strings.HasPrefix(name, prefix) && len(name) > len(prefix) {
					found = true
					break
				}
			}
			if !found {
				missing = append(missing, prefix+"<dynamic>")
			}
		}
		return missing
	}

	var missing []string
	for _, prefix := range g.prefixes {
		for _, suffix := range g.suffixes {
			found := false
			for name := range observed {
				if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, suffix) &&
					len(name) > len(prefix)+len(suffix) {
					found = true
					break
				}
			}
			if !found {
				missing = append(missing, prefix+"<dynamic>"+suffix)
			}
		}
	}
	return missing
}

func (g boundedMetricNameGrammar) nonCanonicalRewriteOutputs(
	possibleOutputs []string,
	authoredMetricNames map[string]struct{},
) []string {
	if !g.hasInternalDynamicKey() && len(g.dynamicTailPrefixes) == 0 {
		return nil
	}

	outputs := make(map[string]struct{})
	for _, output := range possibleOutputs {
		if !commonmodel.UTF8Validation.IsValidMetricName(output) {
			outputs[output] = struct{}{}
			continue
		}
		if _, ok := authoredMetricNames[output]; !ok {
			outputs[output] = struct{}{}
		}
	}
	return slices.Sorted(maps.Keys(outputs))
}

func (g boundedMetricNameGrammar) finiteBranchCount() int {
	if len(g.dynamicTailPrefixes) > 0 {
		return len(g.dynamicTailPrefixes)
	}
	return len(g.prefixes) * len(g.suffixes)
}

func simplePatternScopesMayOverlap(analyzer *matcher.Analyzer, leftExpr, rightExpr string) (bool, error) {
	_, intersects, err := analyzer.SimplePatternIntersectionWitness(leftExpr, rightExpr, true)
	return intersects, err
}

func profileAuthoredMetricNames(profile promprofiles.Profile) map[string]struct{} {
	root, err := profile.Template()
	if err != nil {
		return nil
	}
	names := make(map[string]struct{})
	var walk func(charttpl.Group)
	walk = func(group charttpl.Group) {
		for _, chart := range group.Charts {
			for _, dimension := range chart.Dimensions {
				compiled, err := metrixselector.ParseCompiled(dimension.Selector)
				if err != nil {
					continue
				}
				for _, name := range compiled.Meta().MetricNames {
					names[name] = struct{}{}
				}
			}
		}
		for _, child := range group.Groups {
			walk(child)
		}
	}
	walk(root)
	return names
}

type prometheusLabelView struct {
	labels labels.Labels
}

func (v prometheusLabelView) Len() int { return len(v.labels) }

func (v prometheusLabelView) Get(key string) (string, bool) {
	for _, label := range v.labels {
		if label.Name == key {
			return label.Value, true
		}
	}
	return "", false
}

func (v prometheusLabelView) Range(fn func(key, value string) bool) {
	for _, label := range v.labels {
		if !fn(label.Name, label.Value) {
			return
		}
	}
}

func (v prometheusLabelView) CloneMap() map[string]string {
	values := make(map[string]string, len(v.labels))
	v.Range(func(key, value string) bool {
		values[key] = value
		return true
	})
	return values
}

var _ metrix.LabelView = prometheusLabelView{}

func checkFutureMetricPolicy(
	canary string,
	expr *metrixselector.Expr,
	policy jobPolicy,
	authoredMetricNames map[string]struct{},
	checkGenericRouting bool,
	r *report,
) (prompkg.Sample, bool, map[int]struct{}) {
	if !policy.Selector.Empty() {
		selector, err := (promselector.Expr{Allow: policy.Selector.Allow, Deny: policy.Selector.Deny}).Parse()
		if err == nil && selector != nil && !selector.Matches(labels.FromStrings(labels.MetricName, canary)) {
			r.addError(
				"future_metric_blocked_by_job_selector",
				"selector",
				fmt.Sprintf("job selector rejects synthetic future family %q", canary),
				"The recommended job policy must remain open over the profile's future exporter namespace; use only bounded exclusions for known families.",
			)
		}
	}

	processed, dropped, appliedBlocks := checkFutureMetricRelabeling(canary, policy, r)
	if dropped || !checkGenericRouting {
		return processed, dropped, appliedBlocks
	}
	if _, ok := authoredMetricNames[processed.Name]; ok {
		r.addError(
			"future_metric_routed_to_authored_metric",
			"relabeling",
			fmt.Sprintf("synthetic future family %q is relabeled to authored metric %q", canary, processed.Name),
			"Unknown future families must remain generic. Relabeling them onto an authored metric silently changes their chart meaning instead of preserving fallback visibility.",
		)
		return processed, false, appliedBlocks
	}
	if expr != nil {
		selector, err := (metrixselector.Expr{Allow: expr.Allow, Deny: expr.Deny}).Parse()
		if err == nil && selector != nil && !selector.Matches(processed.Name, prometheusLabelView{labels: processed.Labels}) {
			r.addError(
				"future_metric_blocked_by_profile",
				"autogen.selector",
				fmt.Sprintf("synthetic future family %q becomes %q and is not eligible for generic fallback", canary, processed.Name),
				"A profile may suppress exact known families, but it must preserve unknown families matching its exporter scope after recommended job relabeling.",
			)
		}
	}
	return processed, false, appliedBlocks
}

func checkFutureMetricRelabeling(canary string, policy jobPolicy, r *report) (prompkg.Sample, bool, map[int]struct{}) {
	sample := prompkg.Sample{Name: canary, Labels: labels.EmptyLabels()}
	appliedBlocks := make(map[int]struct{})
	for blockIndex, block := range policy.Relabeling {
		match, err := matcher.NewSimplePatternsMatcher(block.Match)
		if err != nil || !match.MatchString(sample.Name) {
			continue
		}
		appliedBlocks[blockIndex] = struct{}{}
		processor, err := relabel.New(block.MetricRelabelConfigs)
		if err != nil {
			continue
		}
		processed, drop := processor.Apply(sample)
		if drop.Dropped() {
			r.addError(
				"future_metric_blocked_by_job_relabel",
				fmt.Sprintf("relabeling[%d].metric_relabel_configs[%d]", blockIndex, drop.RuleIndex),
				fmt.Sprintf("job relabeling drops synthetic future family %q", canary),
				"A contributed job may drop exact known series or bounded source-proven aliases, but it must not discard an unknown future family in the profile namespace.",
			)
			return sample, true, appliedBlocks
		}
		sample = processed
	}
	return sample, false, appliedBlocks
}

func exactMetricFamilySelector(expr string) (string, bool) {
	name := strings.TrimSpace(expr)
	if strings.Contains(name, "{") {
		return "", false
	}
	return name, !hasUnescapedGlobMeta(name) && commonmodel.UTF8Validation.IsValidMetricName(name)
}

type futureMetricTermCanaries struct {
	pattern          string
	earlierNegatives []string
	canaries         []string
}

func (t futureMetricTermCanaries) scopeExpr() string {
	terms := make([]string, 0, len(t.earlierNegatives)+1)
	for _, negative := range t.earlierNegatives {
		terms = append(terms, "!"+negative)
	}
	terms = append(terms, t.pattern)
	return strings.Join(terms, " ")
}

func syntheticFutureMetricTerms(matchExpr string) ([]futureMetricTermCanaries, bool) {
	scope, err := matcher.NewSimplePatternsMatcher(matchExpr)
	if err != nil {
		return nil, false
	}
	hasWildcard := false
	var terms []futureMetricTermCanaries
	var earlierNegatives []string
	for term := range strings.FieldsSeq(matchExpr) {
		if after, ok := strings.CutPrefix(term, "!"); ok {
			earlierNegatives = append(earlierNegatives, after)
			continue
		}
		if !hasUnescapedGlobMeta(term) {
			continue
		}
		hasWildcard = true
		entry := futureMetricTermCanaries{
			pattern:          term,
			earlierNegatives: append([]string(nil), earlierNegatives...),
		}
		seen := make(map[string]struct{})
		for attempt := range len(futureMetricCandidateAlphabet) * 4 {
			candidate, ok := syntheticMetricFromGlob(term, attempt)
			if ok && commonmodel.UTF8Validation.IsValidMetricName(candidate) && scope.MatchString(candidate) {
				if _, ok := seen[candidate]; !ok {
					entry.canaries = append(entry.canaries, candidate)
					seen[candidate] = struct{}{}
				}
				if len(entry.canaries) == len(futureMetricStems) {
					break
				}
			}
		}
		terms = append(terms, entry)
	}
	return terms, hasWildcard
}

func syntheticFutureMetrics(matchExpr string) ([]string, bool) {
	terms, wildcard := syntheticFutureMetricTerms(matchExpr)
	var canaries []string
	seen := make(map[string]struct{})
	for _, term := range terms {
		for _, canary := range term.canaries {
			if _, ok := seen[canary]; ok {
				continue
			}
			seen[canary] = struct{}{}
			canaries = append(canaries, canary)
		}
	}
	return canaries, wildcard
}

func syntheticMetricFromGlob(pattern string, attempt int) (string, bool) {
	var b strings.Builder
	wildcardIndex := 0
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '\\':
			if i+1 >= len(pattern) {
				return "", false
			}
			i++
			b.WriteByte(pattern[i])
		case '*':
			stem := futureMetricStems[(attempt+wildcardIndex)%len(futureMetricStems)]
			fmt.Fprintf(&b, "%s_%d", stem, attempt)
			wildcardIndex++
		case '?':
			b.WriteByte(futureMetricCandidateAlphabet[(attempt+wildcardIndex)%len(futureMetricCandidateAlphabet)])
			wildcardIndex++
		case '[':
			end := globClassEnd(pattern, i)
			if end < 0 {
				return "", false
			}
			value, ok := syntheticGlobClassValue(pattern[i:end+1], attempt+wildcardIndex)
			if !ok {
				return "", false
			}
			b.WriteByte(value)
			wildcardIndex++
			i = end
		default:
			b.WriteByte(pattern[i])
		}
	}
	return b.String(), true
}

func hasUnescapedGlobMeta(pattern string) bool {
	for i := 0; i < len(pattern); i++ {
		if pattern[i] == '\\' {
			i++
			continue
		}
		if pattern[i] == '*' || pattern[i] == '?' || pattern[i] == '[' {
			return true
		}
	}
	return false
}

func globClassEnd(pattern string, start int) int {
	for i := start + 1; i < len(pattern); i++ {
		if pattern[i] == '\\' {
			i++
			continue
		}
		if pattern[i] == ']' {
			return i
		}
	}
	return -1
}

func syntheticGlobClassValue(class string, offset int) (byte, bool) {
	classMatcher, err := matcher.NewGlobMatcher(class)
	if err != nil {
		return 0, false
	}
	var candidates []byte
	for i := range len(futureMetricCandidateAlphabet) {
		candidate := futureMetricCandidateAlphabet[i]
		if classMatcher.MatchString(string(candidate)) {
			candidates = append(candidates, candidate)
		}
	}
	if len(candidates) == 0 {
		return 0, false
	}
	return candidates[offset%len(candidates)], true
}
