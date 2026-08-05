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
	"github.com/prometheus/prometheus/model/labels"
)

// addForwardCompatibilityChecks owns structural contributor policy. Runtime
// openness is proved separately by raw future inputs traversing an isolated
// production collector and chart-planner sequence.
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
