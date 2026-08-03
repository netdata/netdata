// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"fmt"
	"maps"
	"regexp"
	"regexp/syntax"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	metrixselector "github.com/netdata/netdata/go/plugins/pkg/metrix/selector"
	prompkg "github.com/netdata/netdata/go/plugins/pkg/prometheus"
	promselector "github.com/netdata/netdata/go/plugins/pkg/prometheus/selector"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	promcollector "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/promprofiles"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/relabel"
	"github.com/prometheus/prometheus/model/labels"
)

var prometheusMetricNamePattern = regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]*$`)
var prometheusLabelNamePattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

const metricNameCandidateAlphabet = "_abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789:"
const metricNameInitialAlphabet = "_abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ:"

// Varied stems prevent a narrow job allowlist from passing merely because it
// happens to admit the validator's primary probe name.
var futureMetricStems = [...]string{
	"netdata_future_metric",
	"upstream_added_metric",
	"exporter_new_signal",
}

// addForwardCompatibilityChecks keeps openness structural and uses synthetic
// names beyond the bounded source fixture. The fixture is consulted only to
// prove exact exclusions and every explicitly bounded wildcard name grammar.
func addForwardCompatibilityChecks(
	profile promprofiles.Profile,
	policy jobPolicy,
	rawFamilies []rawFamilyReport,
	rawSamples prompkg.SampleBatch,
	relabelAudits relabelPolicyAudits,
	r *report,
) {
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
	var priorNameMutations []nameMutationEffect
	for blockIndex, block := range policy.Relabeling {
		originalNameAtEntry := !nameMutationEffectsMayReach(priorNameMutations, block.Match, false)
		nameDerivedOnlyFromOriginal := !nameMutationEffectsMayReach(priorNameMutations, block.Match, true)
		blockReachable := simplePatternScopesMayOverlap(profile.Match, block.Match) || !originalNameAtEntry
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
			action := normalizedRelabelAction(rule.Action)
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
			mayWriteMetricName := relabelRuleMayWriteMetricName(rule, action)
			mutationReadsOnlyName := relabelNameMutationIsNameDerived(rule, action)
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
				grammar, ok := analyzeBoundedMetricNameRewriteGrammar(rule, action)
				outputs, outputsFinite := finiteRegexpReplacementOutputs(
					rule.Regex.String(), rule.Replacement, maxBoundedMetricNameGrammarBranches,
				)
				identityLabel := ""
				identityPreserved := true
				if ok && grammar.hasDynamicIdentity() && outputsFinite {
					identityLabel, identityPreserved = relabelRewritePreservesDynamicIdentity(
						policy.Relabeling, blockIndex, ruleIndex, rule, grammar, outputs,
					)
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
				grammar, ok := analyzeBoundedMetricNameDiscardGrammar(rule, action)
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
				priorNameMutations = append(priorNameMutations, relabelNameMutationEffect(rule, action, mutationOutputIsNameDerived))
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
		return
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
		return
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
			if !covered && relabelRulesMayAffectFutureRouting(block.MetricRelabelConfigs) &&
				simplePatternScopesMayOverlap(profile.Match, term.scopeExpr()) {
				r.addError(
					"future_relabel_canary_unavailable",
					fmt.Sprintf("relabeling[%d].match", blockIndex),
					fmt.Sprintf("cannot synthesize a future-family probe for wildcard term %q inside profile scope %q and relabel scope %q", term.pattern, profile.Match, block.Match),
					"Every positive wildcard term in a relabel block that can discard or rename metrics must expose a deterministic future-family probe. One harmless term cannot prove that another term preserves later exporter families.",
				)
			}
		}
	}
}

type nameMutationEffect struct {
	outputPattern      string
	applicationDerived bool
	reachable          bool
}

func relabelNameMutationEffect(rule relabel.Config, action relabel.Action, nameDerived bool) nameMutationEffect {
	effect := nameMutationEffect{applicationDerived: !nameDerived, reachable: true, outputPattern: "*"}
	if action != relabel.Replace {
		return effect
	}
	metadata, ok := parseRegexpCaptureMetadata(rule.Regex.String())
	if !ok {
		return effect
	}
	tokens, ok := parseRegexpReplacementTemplate(
		rule.Replacement, metadata.named, metadata.ambiguousNames, metadata.captures,
	)
	if !ok {
		return effect
	}
	var output strings.Builder
	lastWildcard := false
	for _, token := range tokens {
		if token.isCapture {
			if !lastWildcard {
				output.WriteByte('*')
			}
			lastWildcard = true
		} else {
			output.WriteString(token.literal)
			lastWildcard = false
		}
	}
	if output.Len() == 0 {
		effect.reachable = false
		return effect
	}
	effect.outputPattern = output.String()
	return effect
}

func nameMutationEffectsMayReach(effects []nameMutationEffect, matchExpr string, applicationDerivedOnly bool) bool {
	for _, effect := range effects {
		if !effect.reachable || (applicationDerivedOnly && !effect.applicationDerived) {
			continue
		}
		if simplePatternScopesMayOverlapAnyMetricName(effect.outputPattern, matchExpr) {
			return true
		}
	}
	return false
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
	for _, term := range strings.Fields(profileMatch) {
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
	for _, term := range strings.Fields(matchExpr) {
		if strings.HasPrefix(term, "!") {
			continue
		}
		hasPositive = true
		if hasUnescapedGlobMeta(term) || !prometheusMetricNamePattern.MatchString(term) {
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

func analyzeBoundedMetricNameDiscardGrammar(rule relabel.Config, action relabel.Action) (boundedMetricNameGrammar, bool) {
	if action != relabel.Drop {
		return boundedMetricNameGrammar{}, false
	}
	return analyzeBoundedMetricNameGrammar(rule.Regex.String(), false)
}

func analyzeBoundedMetricNameRewriteGrammar(rule relabel.Config, action relabel.Action) (boundedMetricNameGrammar, bool) {
	if action != relabel.Replace || len(rule.SourceLabels) != 1 || rule.SourceLabels[0] != labels.MetricName ||
		rule.TargetLabel != labels.MetricName {
		return boundedMetricNameGrammar{}, false
	}
	grammar, ok := analyzeBoundedMetricNameGrammar(rule.Regex.String(), true)
	if !ok {
		return boundedMetricNameGrammar{}, false
	}
	if len(grammar.dynamicTailPrefixes) == 0 {
		return grammar, true
	}
	if len(grammar.dynamicTailPrefixes) != 1 {
		return boundedMetricNameGrammar{}, false
	}
	prefix := grammar.dynamicTailPrefixes[0]
	if !strings.HasSuffix(prefix, "_") || rule.Replacement != strings.TrimSuffix(prefix, "_") {
		return boundedMetricNameGrammar{}, false
	}
	return grammar, true
}

func analyzeBoundedMetricNameGrammar(expr string, allowDynamicTail bool) (boundedMetricNameGrammar, bool) {
	parsed, err := syntax.Parse(expr, syntax.Perl)
	if err != nil {
		return boundedMetricNameGrammar{}, false
	}
	parsed = parsed.Simplify()
	if names, finite := enumerateFiniteRegexp(parsed, maxBoundedMetricNameGrammarBranches); finite {
		if len(names) == 0 {
			return boundedMetricNameGrammar{}, false
		}
		for _, name := range names {
			if !prometheusMetricNamePattern.MatchString(name) {
				return boundedMetricNameGrammar{}, false
			}
		}
		return boundedMetricNameGrammar{exactNames: names}, true
	}

	parts := flattenRegexpConcat(parsed, nil)
	dynamicIndex := -1
	for i, part := range parts {
		if _, finite := enumerateFiniteRegexp(part.expr, maxBoundedMetricNameGrammarBranches); finite {
			continue
		}
		if dynamicIndex >= 0 {
			// Stock alias normalization has one dynamic entity key. Multiple
			// open-ended regions cannot be reviewed as a bounded metric grammar.
			return boundedMetricNameGrammar{}, false
		}
		dynamicIndex = i
	}
	if dynamicIndex <= 0 || regexpCanMatchEmpty(parts[dynamicIndex].expr) {
		return boundedMetricNameGrammar{}, false
	}

	prefixes, ok := enumerateFiniteRegexpSequence(parts[:dynamicIndex], maxBoundedMetricNameGrammarBranches)
	if !ok || len(prefixes) == 0 {
		return boundedMetricNameGrammar{}, false
	}
	for _, prefix := range prefixes {
		if prefix == "" || !prometheusMetricNamePattern.MatchString(prefix) {
			return boundedMetricNameGrammar{}, false
		}
	}
	if dynamicIndex == len(parts)-1 {
		if !allowDynamicTail {
			return boundedMetricNameGrammar{}, false
		}
		return boundedMetricNameGrammar{
			dynamicTailPrefixes: prefixes,
			dynamicCaptureIDs:   slices.Clone(parts[dynamicIndex].enclosingCaptureIDs),
		}, true
	}

	suffixes, ok := enumerateFiniteRegexpSequence(parts[dynamicIndex+1:], maxBoundedMetricNameGrammarBranches)
	if !ok || len(suffixes) == 0 || len(prefixes) > maxBoundedMetricNameGrammarBranches/len(suffixes) {
		return boundedMetricNameGrammar{}, false
	}
	for _, suffix := range suffixes {
		if !strings.HasPrefix(suffix, "_") || !prometheusMetricNamePattern.MatchString(strings.TrimPrefix(suffix, "_")) {
			return boundedMetricNameGrammar{}, false
		}
	}
	return boundedMetricNameGrammar{
		prefixes:          prefixes,
		suffixes:          suffixes,
		dynamicCaptureIDs: slices.Clone(parts[dynamicIndex].enclosingCaptureIDs),
	}, true
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

func enumerateFiniteRegexpSequence(parts []regexpConcatPart, limit int) ([]string, bool) {
	values := []string{""}
	for _, part := range parts {
		item, finite := enumerateFiniteRegexp(part.expr, limit)
		if !finite {
			return nil, false
		}
		values, finite = concatFiniteRegexpValues(values, item, limit)
		if !finite {
			return nil, false
		}
	}
	return values, true
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
		for _, sub := range re.Sub {
			if regexpCanMatchEmpty(sub) {
				return true
			}
		}
		return false
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
		if !prometheusMetricNamePattern.MatchString(output) {
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

type regexpReplacementToken struct {
	literal   string
	capture   int
	isCapture bool
}

type regexpCaptureMetadata struct {
	parsed         *syntax.Regexp
	captures       map[int]*syntax.Regexp
	named          map[string]int
	ambiguousNames map[string]struct{}
	nestedInOpen   map[int]struct{}
}

func parseRegexpCaptureMetadata(expr string) (regexpCaptureMetadata, bool) {
	parsed, err := syntax.Parse(expr, syntax.Perl)
	if err != nil {
		return regexpCaptureMetadata{}, false
	}

	metadata := regexpCaptureMetadata{
		parsed:         parsed,
		captures:       map[int]*syntax.Regexp{0: parsed},
		named:          make(map[string]int),
		ambiguousNames: make(map[string]struct{}),
		nestedInOpen:   make(map[int]struct{}),
	}
	var walk func(*syntax.Regexp, bool)
	walk = func(re *syntax.Regexp, openAncestor bool) {
		openHere := false
		if re.Op == syntax.OpCapture {
			metadata.captures[re.Cap] = re.Sub[0]
			if openAncestor {
				metadata.nestedInOpen[re.Cap] = struct{}{}
			}
			if _, finite := enumerateFiniteRegexp(re.Sub[0], maxBoundedMetricNameGrammarBranches); !finite {
				openHere = true
			}
			if re.Name != "" {
				if _, ok := metadata.named[re.Name]; ok {
					metadata.ambiguousNames[re.Name] = struct{}{}
				} else {
					metadata.named[re.Name] = re.Cap
				}
			}
		}
		for _, sub := range re.Sub {
			walk(sub, openAncestor || openHere)
		}
	}
	walk(parsed, false)
	return metadata, true
}

func finiteRegexpReplacementOutputs(expr, replacement string, limit int) ([]string, bool) {
	metadata, ok := parseRegexpCaptureMetadata(expr)
	if !ok || limit <= 0 {
		return nil, false
	}
	requiredCaptures := regexpRequiredCaptures(metadata.parsed)

	tokens, ok := parseRegexpReplacementTemplate(
		replacement, metadata.named, metadata.ambiguousNames, metadata.captures,
	)
	if !ok {
		return nil, false
	}
	values := make(map[int][]string)
	for _, token := range tokens {
		if !token.isCapture {
			continue
		}
		if _, ok := values[token.capture]; ok {
			continue
		}
		if _, nested := metadata.nestedInOpen[token.capture]; nested {
			return nil, false
		}
		items, finite := enumerateFiniteRegexp(metadata.captures[token.capture], limit)
		if !finite {
			return nil, false
		}
		if _, required := requiredCaptures[token.capture]; !required && !slices.Contains(items, "") {
			items = append(items, "")
		}
		values[token.capture] = items
	}

	captureIDs := slices.Sorted(maps.Keys(values))
	assignments := make(map[int]string, len(captureIDs))
	outputs := make(map[string]struct{})
	assignmentCount := 0
	var enumerate func(int) bool
	enumerate = func(index int) bool {
		if index < len(captureIDs) {
			capture := captureIDs[index]
			for _, value := range values[capture] {
				assignments[capture] = value
				if !enumerate(index + 1) {
					return false
				}
			}
			return true
		}

		assignmentCount++
		if assignmentCount > limit {
			return false
		}
		var output strings.Builder
		for _, token := range tokens {
			if token.isCapture {
				output.WriteString(assignments[token.capture])
			} else {
				output.WriteString(token.literal)
			}
		}
		outputs[output.String()] = struct{}{}
		return len(outputs) <= limit
	}
	if !enumerate(0) {
		return nil, false
	}
	return slices.Sorted(maps.Keys(outputs)), true
}

func relabelRewritePreservesDynamicIdentity(
	blocks []promcollector.RelabelBlock,
	blockIndex, rewriteIndex int,
	rewrite relabel.Config,
	grammar boundedMetricNameGrammar,
	possibleOutputs []string,
) (string, bool) {
	metadata, ok := parseRegexpCaptureMetadata(rewrite.Regex.String())
	if !ok {
		return "", false
	}
	dynamicCaptures := make(map[int]struct{}, len(grammar.dynamicCaptureIDs))
	for _, capture := range grammar.dynamicCaptureIDs {
		dynamicCaptures[capture] = struct{}{}
	}
	if len(dynamicCaptures) == 0 {
		return "", false
	}

	rules := blocks[blockIndex].MetricRelabelConfigs
	for ruleIndex := rewriteIndex - 1; ruleIndex >= 0; ruleIndex-- {
		candidate := rules[ruleIndex]
		if normalizedRelabelAction(candidate.Action) != relabel.Replace ||
			len(candidate.SourceLabels) != 1 || candidate.SourceLabels[0] != labels.MetricName ||
			candidate.Regex.String() != rewrite.Regex.String() ||
			candidate.TargetLabel == labels.MetricName ||
			!prometheusLabelNamePattern.MatchString(candidate.TargetLabel) {
			continue
		}
		tokens, ok := parseRegexpReplacementTemplate(
			candidate.Replacement, metadata.named, metadata.ambiguousNames, metadata.captures,
		)
		if !ok || len(tokens) != 1 || !tokens[0].isCapture {
			continue
		}
		if _, ok := dynamicCaptures[tokens[0].capture]; !ok {
			continue
		}
		if !relabelRulesPreserveLabel(rules[ruleIndex+1:rewriteIndex], candidate.TargetLabel, nil) ||
			!relabelRulesPreserveLabel(rules[rewriteIndex+1:], candidate.TargetLabel, possibleOutputs) ||
			!laterRelabelBlocksPreserveLabel(blocks[blockIndex+1:], candidate.TargetLabel, possibleOutputs,
				relabelRulesMayMutateMetricName(rules[rewriteIndex+1:])) {
			continue
		}
		return candidate.TargetLabel, true
	}
	return "", false
}

func laterRelabelBlocksPreserveLabel(
	blocks []promcollector.RelabelBlock,
	labelName string,
	possibleNames []string,
	nameUnknown bool,
) bool {
	for _, block := range blocks {
		reachable := nameUnknown
		if !reachable {
			match, err := matcher.NewSimplePatternsMatcher(block.Match)
			if err != nil {
				return false
			}
			for _, name := range possibleNames {
				if match.MatchString(name) {
					reachable = true
					break
				}
			}
		}
		if !reachable {
			continue
		}
		var names []string
		if !nameUnknown {
			names = possibleNames
		}
		if !relabelRulesPreserveLabel(block.MetricRelabelConfigs, labelName, names) {
			return false
		}
		if relabelRulesMayMutateMetricName(block.MetricRelabelConfigs) {
			nameUnknown = true
		}
	}
	return true
}

func relabelRulesMayMutateMetricName(rules []relabel.Config) bool {
	for _, rule := range rules {
		if relabelRuleMayWriteMetricName(rule, normalizedRelabelAction(rule.Action)) {
			return true
		}
	}
	return false
}

func relabelRulesPreserveLabel(rules []relabel.Config, labelName string, possibleNames []string) bool {
	for _, rule := range rules {
		action := normalizedRelabelAction(rule.Action)
		mayApply := relabelRuleMayApplyToMetricNames(rule, action, possibleNames)
		switch action {
		case relabel.Replace, relabel.HashMod, relabel.Lowercase, relabel.Uppercase:
			if mayApply && (rule.TargetLabel == labelName || strings.Contains(rule.TargetLabel, "$")) {
				return false
			}
		case relabel.LabelDrop:
			if rule.Regex.MatchString(labelName) {
				return false
			}
		case relabel.LabelKeep:
			if !rule.Regex.MatchString(labelName) {
				return false
			}
		case relabel.LabelMap:
			if relabelTemplateMayExpandToLabelName(rule, action, rule.Replacement, labelName) {
				return false
			}
		}
		if relabelRuleMayWriteMetricName(rule, action) {
			possibleNames = finiteMetricNamesAfterRelabelRule(rule, action, possibleNames)
		}
	}
	return true
}

func relabelRuleMayApplyToMetricNames(rule relabel.Config, action relabel.Action, possibleNames []string) bool {
	if possibleNames == nil || action != relabel.Replace ||
		len(rule.SourceLabels) != 1 || rule.SourceLabels[0] != labels.MetricName {
		return true
	}
	for _, name := range possibleNames {
		if rule.Regex.MatchString(name) {
			return true
		}
	}
	return false
}

func finiteMetricNamesAfterRelabelRule(
	rule relabel.Config,
	action relabel.Action,
	possibleNames []string,
) []string {
	if possibleNames == nil || action != relabel.Replace ||
		len(rule.SourceLabels) != 1 || rule.SourceLabels[0] != labels.MetricName ||
		rule.TargetLabel != labels.MetricName {
		return nil
	}
	outputs := make(map[string]struct{}, len(possibleNames))
	for _, name := range possibleNames {
		output := name
		if rule.Regex.MatchString(name) {
			output = rule.Regex.ReplaceAllString(name, rule.Replacement)
		}
		if prometheusMetricNamePattern.MatchString(output) {
			outputs[output] = struct{}{}
		}
	}
	return slices.Sorted(maps.Keys(outputs))
}

func parseRegexpReplacementTemplate(
	template string,
	named map[string]int,
	ambiguousNames map[string]struct{},
	captures map[int]*syntax.Regexp,
) ([]regexpReplacementToken, bool) {
	var tokens []regexpReplacementToken
	appendLiteral := func(value string) {
		if value == "" {
			return
		}
		if len(tokens) > 0 && !tokens[len(tokens)-1].isCapture {
			tokens[len(tokens)-1].literal += value
			return
		}
		tokens = append(tokens, regexpReplacementToken{literal: value})
	}
	for len(template) > 0 {
		before, after, ok := strings.Cut(template, "$")
		appendLiteral(before)
		if !ok {
			break
		}
		template = after
		if strings.HasPrefix(template, "$") {
			appendLiteral("$")
			template = template[1:]
			continue
		}
		name, num, rest, ok := extractRegexpReplacementReference(template)
		if !ok {
			appendLiteral("$")
			continue
		}
		template = rest
		capture := num
		if capture < 0 {
			if _, ambiguous := ambiguousNames[name]; ambiguous {
				return nil, false
			}
			var exists bool
			capture, exists = named[name]
			if !exists {
				continue
			}
		}
		if _, exists := captures[capture]; exists {
			tokens = append(tokens, regexpReplacementToken{capture: capture, isCapture: true})
		}
	}
	return tokens, true
}

func regexpRequiredCaptures(re *syntax.Regexp) map[int]struct{} {
	switch re.Op {
	case syntax.OpCapture:
		required := regexpRequiredCaptures(re.Sub[0])
		required[re.Cap] = struct{}{}
		return required
	case syntax.OpConcat:
		required := make(map[int]struct{})
		for _, sub := range re.Sub {
			for capture := range regexpRequiredCaptures(sub) {
				required[capture] = struct{}{}
			}
		}
		return required
	case syntax.OpAlternate:
		if len(re.Sub) == 0 {
			return map[int]struct{}{}
		}
		required := regexpRequiredCaptures(re.Sub[0])
		for _, sub := range re.Sub[1:] {
			branch := regexpRequiredCaptures(sub)
			for capture := range required {
				if _, ok := branch[capture]; !ok {
					delete(required, capture)
				}
			}
		}
		return required
	case syntax.OpPlus:
		return regexpRequiredCaptures(re.Sub[0])
	case syntax.OpRepeat:
		if re.Min > 0 {
			return regexpRequiredCaptures(re.Sub[0])
		}
	}
	return map[int]struct{}{}
}

// extractRegexpReplacementReference mirrors regexp's $name/${name} parsing.
func extractRegexpReplacementReference(str string) (name string, num int, rest string, ok bool) {
	if str == "" {
		return "", 0, "", false
	}
	brace := false
	if str[0] == '{' {
		brace = true
		str = str[1:]
	}
	i := 0
	for i < len(str) {
		r, size := utf8.DecodeRuneInString(str[i:])
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			break
		}
		i += size
	}
	if i == 0 {
		return "", 0, "", false
	}
	name = str[:i]
	if brace {
		if i >= len(str) || str[i] != '}' {
			return "", 0, "", false
		}
		i++
	}

	num = 0
	for i := range len(name) {
		if name[i] < '0' || name[i] > '9' || num >= 1e8 {
			num = -1
			break
		}
		num = num*10 + int(name[i]) - '0'
	}
	if name[0] == '0' && len(name) > 1 {
		num = -1
	}
	return name, num, str[i:], true
}

func relabelRuleMayWriteMetricName(rule relabel.Config, action relabel.Action) bool {
	switch action {
	case relabel.Replace:
		return relabelTemplateMayExpandToLabelName(rule, action, rule.TargetLabel, labels.MetricName)
	case relabel.HashMod, relabel.Lowercase, relabel.Uppercase:
		return rule.TargetLabel == labels.MetricName
	case relabel.LabelMap:
		return relabelTemplateMayExpandToLabelName(rule, action, rule.Replacement, labels.MetricName)
	default:
		return false
	}
}

func relabelTemplateMayExpandToLabelName(
	rule relabel.Config,
	action relabel.Action,
	template, labelName string,
) bool {
	if !strings.Contains(template, "$") {
		if template != labelName {
			return false
		}
	}
	if action == relabel.LabelMap && labelMapPreservesInputName(rule.Regex.String(), template) {
		// Mapping every input label back to itself cannot overwrite the protected
		// label with another label's value. rangeLabels also excludes __name__.
		return false
	}
	metadata, ok := parseRegexpCaptureMetadata(rule.Regex.String())
	if !ok {
		return true
	}
	tokens, ok := parseRegexpReplacementTemplate(
		template, metadata.named, metadata.ambiguousNames, metadata.captures,
	)
	if !ok {
		return true
	}
	if len(tokens) > 0 && !tokens[0].isCapture && !strings.HasPrefix(labelName, tokens[0].literal) {
		return false
	}
	if len(tokens) > 0 && !tokens[len(tokens)-1].isCapture &&
		!strings.HasSuffix(labelName, tokens[len(tokens)-1].literal) {
		return false
	}

	candidates, finite := finiteRegexpLanguage(rule.Regex.String(), 256)
	if !finite {
		return true
	}
	for _, candidate := range candidates {
		if action == relabel.LabelMap && candidate == labels.MetricName {
			continue
		}
		if rule.Regex.MatchString(candidate) &&
			rule.Regex.ReplaceAllString(candidate, template) == labelName &&
			(action != relabel.LabelMap || candidate != labelName) {
			return true
		}
	}
	return false
}

func labelMapPreservesInputName(regex, replacement string) bool {
	switch replacement {
	case "$0", "${0}":
		return true
	case "$1", "${1}":
		return regex == "(.*)" || regex == "(.+)"
	default:
		return false
	}
}

func relabelNameMutationIsNameDerived(rule relabel.Config, action relabel.Action) bool {
	if action == relabel.LabelMap {
		// Runtime labelmap ignores source_labels and derives new label names
		// from the labels it maps, so it cannot prove name-only provenance.
		return false
	}
	if len(rule.SourceLabels) == 0 {
		return false
	}
	for _, source := range rule.SourceLabels {
		if source != labels.MetricName {
			return false
		}
	}
	return true
}

func relabelRulesMayAffectFutureRouting(rules []relabel.Config) bool {
	for _, rule := range rules {
		action := normalizedRelabelAction(rule.Action)
		switch action {
		case relabel.Drop, relabel.DropEqual, relabel.Keep, relabel.KeepEqual:
			return true
		default:
			if relabelRuleMayWriteMetricName(rule, action) {
				return true
			}
		}
	}
	return false
}

func finiteRegexpLanguage(expr string, limit int) ([]string, bool) {
	parsed, err := syntax.Parse(expr, syntax.Perl)
	if err != nil {
		return nil, false
	}
	return enumerateFiniteRegexp(parsed.Simplify(), limit)
}

func enumerateFiniteRegexp(re *syntax.Regexp, limit int) ([]string, bool) {
	if limit <= 0 {
		return nil, false
	}
	switch re.Op {
	case syntax.OpNoMatch:
		return nil, true
	case syntax.OpEmptyMatch, syntax.OpBeginLine, syntax.OpEndLine, syntax.OpBeginText,
		syntax.OpEndText, syntax.OpWordBoundary, syntax.OpNoWordBoundary:
		return []string{""}, true
	case syntax.OpLiteral:
		if re.Flags&syntax.FoldCase != 0 {
			return nil, false
		}
		return []string{string(re.Rune)}, true
	case syntax.OpCharClass:
		var values []string
		for i := 0; i+1 < len(re.Rune); i += 2 {
			lo, hi := re.Rune[i], re.Rune[i+1]
			if hi-lo+1 > rune(limit-len(values)) {
				return nil, false
			}
			for value := lo; value <= hi; value++ {
				values = append(values, string(value))
			}
		}
		return values, true
	case syntax.OpCapture:
		return enumerateFiniteRegexp(re.Sub[0], limit)
	case syntax.OpConcat:
		values := []string{""}
		for _, sub := range re.Sub {
			part, finite := enumerateFiniteRegexp(sub, limit)
			if !finite {
				return nil, false
			}
			values, finite = concatFiniteRegexpValues(values, part, limit)
			if !finite {
				return nil, false
			}
		}
		return values, true
	case syntax.OpAlternate:
		var values []string
		seen := make(map[string]struct{})
		for _, sub := range re.Sub {
			part, finite := enumerateFiniteRegexp(sub, limit)
			if !finite {
				return nil, false
			}
			for _, value := range part {
				if _, ok := seen[value]; ok {
					continue
				}
				if len(values) == limit {
					return nil, false
				}
				seen[value] = struct{}{}
				values = append(values, value)
			}
		}
		return values, true
	case syntax.OpQuest:
		part, finite := enumerateFiniteRegexp(re.Sub[0], limit)
		if !finite {
			return nil, false
		}
		for _, value := range part {
			if value == "" {
				return part, true
			}
		}
		if len(part) == limit {
			return nil, false
		}
		return append([]string{""}, part...), true
	case syntax.OpRepeat:
		if re.Max < 0 || re.Max > limit {
			return nil, false
		}
		part, finite := enumerateFiniteRegexp(re.Sub[0], limit)
		if !finite {
			return nil, false
		}
		all := make([]string, 0, limit)
		current := []string{""}
		for count := 0; count <= re.Max; count++ {
			if count >= re.Min {
				if len(all)+len(current) > limit {
					return nil, false
				}
				all = append(all, current...)
			}
			if count == re.Max {
				break
			}
			current, finite = concatFiniteRegexpValues(current, part, limit)
			if !finite {
				return nil, false
			}
		}
		return all, true
	default:
		// AnyChar, AnyCharNotNL, Star, and Plus describe an unbounded language.
		return nil, false
	}
}

func concatFiniteRegexpValues(left, right []string, limit int) ([]string, bool) {
	if len(left) == 0 || len(right) == 0 {
		return nil, true
	}
	if len(left) > limit/len(right) {
		return nil, false
	}
	values := make([]string, 0, len(left)*len(right))
	for _, prefix := range left {
		for _, suffix := range right {
			values = append(values, prefix+suffix)
		}
	}
	return values, true
}

func simplePatternScopesMayOverlap(leftExpr, rightExpr string) bool {
	return simplePatternScopesMayOverlapWith(leftExpr, rightExpr, true)
}

func simplePatternScopesMayOverlapAnyMetricName(leftExpr, rightExpr string) bool {
	return simplePatternScopesMayOverlapWith(leftExpr, rightExpr, false)
}

func simplePatternScopesMayOverlapWith(leftExpr, rightExpr string, legacyMetricName bool) bool {
	left := orderedSimplePatternPositiveBranches(leftExpr)
	right := orderedSimplePatternPositiveBranches(rightExpr)
	if len(left) == 0 || len(right) == 0 {
		return false
	}
	for _, leftBranch := range left {
		for _, rightBranch := range right {
			negatives := append(slices.Clone(leftBranch.earlierNegatives), rightBranch.earlierNegatives...)
			intersects, ok := simpleGlobPatternsIntersectExcluding(
				leftBranch.pattern,
				rightBranch.pattern,
				negatives,
				legacyMetricName,
			)
			if !ok || intersects {
				return true
			}
		}
	}
	return false
}

type simplePatternPositiveBranch struct {
	pattern          string
	earlierNegatives []string
}

func orderedSimplePatternPositiveBranches(expr string) []simplePatternPositiveBranch {
	var branches []simplePatternPositiveBranch
	var negatives []string
	for _, term := range strings.Fields(expr) {
		if strings.HasPrefix(term, "!") {
			negatives = append(negatives, strings.TrimPrefix(term, "!"))
			continue
		}
		branches = append(branches, simplePatternPositiveBranch{
			pattern:          term,
			earlierNegatives: append([]string(nil), negatives...),
		})
	}
	return branches
}

type simpleGlobToken struct {
	kind    byte
	literal rune
	ranges  []simpleGlobRange
	negated bool
}

type simpleGlobRange struct {
	lo rune
	hi rune
}

const (
	simpleGlobLiteral byte = iota
	simpleGlobAny
	simpleGlobStar
	simpleGlobClass
)

func simpleGlobPatternsIntersectOnMetricName(leftPattern, rightPattern string) (bool, bool) {
	return simpleGlobPatternsIntersect(leftPattern, rightPattern, true)
}

func simpleGlobPatternsIntersect(leftPattern, rightPattern string, legacyMetricName bool) (bool, bool) {
	return simpleGlobPatternsIntersectExcluding(leftPattern, rightPattern, nil, legacyMetricName)
}

func simpleGlobPatternsIntersectExcluding(
	leftPattern string,
	rightPattern string,
	negativePatterns []string,
	legacyMetricName bool,
) (bool, bool) {
	left, ok := parseSimpleGlob(leftPattern)
	if !ok {
		return false, false
	}
	right, ok := parseSimpleGlob(rightPattern)
	if !ok {
		return false, false
	}
	negative := make([][]simpleGlobToken, 0, len(negativePatterns))
	patterns := [][]simpleGlobToken{left, right}
	for _, pattern := range negativePatterns {
		parsed, ok := parseSimpleGlob(pattern)
		if !ok {
			return false, false
		}
		negative = append(negative, parsed)
		patterns = append(patterns, parsed)
	}
	generalAlphabet := simpleGlobIntersectionAlphabet(patterns...)

	type state struct {
		left     []int
		right    []int
		negative [][]int
		started  bool
	}
	start := state{
		left:     simpleGlobEpsilonClosure(left, 0),
		right:    simpleGlobEpsilonClosure(right, 0),
		negative: make([][]int, len(negative)),
	}
	for i := range negative {
		start.negative[i] = simpleGlobEpsilonClosure(negative[i], 0)
	}
	queue := []state{start}
	seen := make(map[string]struct{})
	seen[simpleGlobProductStateKey(start.left, start.right, start.negative, false)] = struct{}{}
	add := func(candidate state) {
		key := simpleGlobProductStateKey(candidate.left, candidate.right, candidate.negative, candidate.started)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		queue = append(queue, candidate)
	}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if current.started && simpleGlobPositionSetAccepts(current.left, len(left)) &&
			simpleGlobPositionSetAccepts(current.right, len(right)) {
			excluded := false
			for i := range negative {
				if simpleGlobPositionSetAccepts(current.negative[i], len(negative[i])) {
					excluded = true
					break
				}
			}
			if !excluded {
				return true, true
			}
		}
		alphabet := generalAlphabet
		if legacyMetricName && !current.started {
			alphabet = metricNameInitialAlphabet
		} else if legacyMetricName {
			alphabet = metricNameCandidateAlphabet
		}
		for _, candidate := range alphabet {
			leftNext := simpleGlobNextPositionSet(left, current.left, candidate)
			if len(leftNext) == 0 {
				continue
			}
			rightNext := simpleGlobNextPositionSet(right, current.right, candidate)
			if len(rightNext) == 0 {
				continue
			}
			negativeNext := make([][]int, len(negative))
			for i := range negative {
				negativeNext[i] = simpleGlobNextPositionSet(negative[i], current.negative[i], candidate)
			}
			add(state{left: leftNext, right: rightNext, negative: negativeNext, started: true})
		}
	}
	return false, true
}

func simpleGlobNextPositionSet(pattern []simpleGlobToken, positions []int, candidate rune) []int {
	seen := make(map[int]struct{})
	for _, position := range positions {
		for _, next := range simpleGlobNextPositions(pattern, position, candidate) {
			seen[next] = struct{}{}
		}
	}
	return slices.Sorted(maps.Keys(seen))
}

func simpleGlobPositionSetAccepts(positions []int, end int) bool {
	_, ok := slices.BinarySearch(positions, end)
	return ok
}

func simpleGlobProductStateKey(left, right []int, negative [][]int, started bool) string {
	var b strings.Builder
	if started {
		b.WriteByte('1')
	} else {
		b.WriteByte('0')
	}
	write := func(positions []int) {
		b.WriteByte('|')
		for _, position := range positions {
			fmt.Fprintf(&b, "%d,", position)
		}
	}
	write(left)
	write(right)
	for _, positions := range negative {
		write(positions)
	}
	return b.String()
}

func simpleGlobIntersectionAlphabet(patterns ...[]simpleGlobToken) string {
	candidates := map[rune]struct{}{
		'a': {}, '_': {}, '0': {}, '-': {}, '.': {}, ':': {}, 'é': {},
	}
	add := func(value rune) {
		if value >= 0 && value <= 0x10ffff {
			candidates[value] = struct{}{}
		}
	}
	for _, pattern := range patterns {
		for _, token := range pattern {
			switch token.kind {
			case simpleGlobLiteral:
				add(token.literal)
			case simpleGlobClass:
				for _, item := range token.ranges {
					add(item.lo)
					add(item.hi)
					add(item.lo - 1)
					add(item.hi + 1)
				}
			}
		}
	}
	var alphabet strings.Builder
	for candidate := range candidates {
		alphabet.WriteRune(candidate)
	}
	return alphabet.String()
}

func simpleGlobEpsilonClosure(tokens []simpleGlobToken, position int) []int {
	positions := []int{position}
	for position < len(tokens) && tokens[position].kind == simpleGlobStar {
		position++
		positions = append(positions, position)
	}
	return positions
}

func simpleGlobNextPositions(tokens []simpleGlobToken, position int, candidate rune) []int {
	if position >= len(tokens) {
		return nil
	}
	token := tokens[position]
	if token.kind == simpleGlobStar {
		return simpleGlobEpsilonClosure(tokens, position)
	}
	if !simpleGlobTokenMatches(token, candidate) {
		return nil
	}
	return simpleGlobEpsilonClosure(tokens, position+1)
}

func simpleGlobTokenMatches(token simpleGlobToken, candidate rune) bool {
	switch token.kind {
	case simpleGlobLiteral:
		return token.literal == candidate
	case simpleGlobAny:
		return true
	case simpleGlobClass:
		matched := false
		for _, item := range token.ranges {
			if item.lo <= candidate && candidate <= item.hi {
				matched = true
				break
			}
		}
		return matched != token.negated
	default:
		return false
	}
}

func parseSimpleGlob(pattern string) ([]simpleGlobToken, bool) {
	runes := []rune(pattern)
	var tokens []simpleGlobToken
	for index := 0; index < len(runes); index++ {
		switch runes[index] {
		case '\\':
			index++
			if index >= len(runes) {
				return nil, false
			}
			tokens = append(tokens, simpleGlobToken{kind: simpleGlobLiteral, literal: runes[index]})
		case '*':
			if len(tokens) == 0 || tokens[len(tokens)-1].kind != simpleGlobStar {
				tokens = append(tokens, simpleGlobToken{kind: simpleGlobStar})
			}
		case '?':
			tokens = append(tokens, simpleGlobToken{kind: simpleGlobAny})
		case '[':
			token, end, ok := parseSimpleGlobClass(runes, index)
			if !ok {
				return nil, false
			}
			tokens = append(tokens, token)
			index = end
		default:
			tokens = append(tokens, simpleGlobToken{kind: simpleGlobLiteral, literal: runes[index]})
		}
	}
	return tokens, true
}

func parseSimpleGlobClass(pattern []rune, start int) (simpleGlobToken, int, bool) {
	token := simpleGlobToken{kind: simpleGlobClass}
	index := start + 1
	if index < len(pattern) && pattern[index] == '^' {
		token.negated = true
		index++
	}
	for index < len(pattern) && pattern[index] != ']' {
		lo, next, ok := parseSimpleGlobClassRune(pattern, index)
		if !ok {
			return simpleGlobToken{}, 0, false
		}
		index = next
		hi := lo
		if index < len(pattern) && pattern[index] == '-' {
			hi, index, ok = parseSimpleGlobClassRune(pattern, index+1)
			if !ok || hi < lo {
				return simpleGlobToken{}, 0, false
			}
		}
		token.ranges = append(token.ranges, simpleGlobRange{lo: lo, hi: hi})
	}
	if index >= len(pattern) || pattern[index] != ']' || len(token.ranges) == 0 {
		return simpleGlobToken{}, 0, false
	}
	return token, index, true
}

func parseSimpleGlobClassRune(pattern []rune, index int) (rune, int, bool) {
	if index >= len(pattern) || pattern[index] == '-' || pattern[index] == ']' {
		return 0, 0, false
	}
	if pattern[index] == '\\' {
		index++
		if index >= len(pattern) {
			return 0, 0, false
		}
	}
	return pattern[index], index + 1, true
}

func simplePatternLiteralPrefix(pattern string) (string, bool) {
	var prefix strings.Builder
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '\\':
			if i+1 >= len(pattern) {
				return prefix.String(), false
			}
			i++
			prefix.WriteByte(pattern[i])
		case '*', '?', '[':
			return prefix.String(), false
		default:
			prefix.WriteByte(pattern[i])
		}
	}
	return prefix.String(), true
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
	return name, prometheusMetricNamePattern.MatchString(name)
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
	for _, term := range strings.Fields(matchExpr) {
		if strings.HasPrefix(term, "!") {
			earlierNegatives = append(earlierNegatives, strings.TrimPrefix(term, "!"))
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
		for attempt := 0; attempt < len(metricNameCandidateAlphabet)*4; attempt++ {
			candidate, ok := syntheticMetricFromGlob(term, attempt)
			if ok && prometheusMetricNamePattern.MatchString(candidate) && scope.MatchString(candidate) {
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
			b.WriteByte(metricNameCandidateAlphabet[(attempt+wildcardIndex)%len(metricNameCandidateAlphabet)])
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
	for i := 0; i < len(metricNameCandidateAlphabet); i++ {
		candidate := metricNameCandidateAlphabet[i]
		if classMatcher.MatchString(string(candidate)) {
			candidates = append(candidates, candidate)
		}
	}
	if len(candidates) == 0 {
		return 0, false
	}
	return candidates[offset%len(candidates)], true
}
