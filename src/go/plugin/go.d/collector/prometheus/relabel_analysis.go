// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/relabel"
	commonmodel "github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
)

const defaultRelabelNameFlowMaxScopeIntersections = 1_024

// RelabelNameFlowBudget bounds block-to-block name provenance analysis.
type RelabelNameFlowBudget struct {
	MaxScopeIntersections int
}

// RelabelNameFlowStats reports deterministic flow-analysis work.
type RelabelNameFlowStats struct {
	ScopeIntersections int
}

// RelabelNameMutation is a conservative glob over one rule's possible metric-
// name outputs plus whether application labels contributed to that output.
type RelabelNameMutation struct {
	outputPattern      string
	applicationDerived bool
	reachable          bool
}

// RelabelNameFlowAnalyzer follows metric-name and extracted-identity label
// provenance across collector relabel blocks. It is not goroutine-safe.
type RelabelNameFlowAnalyzer struct {
	ctx     context.Context
	relabel *relabel.Analyzer
	matcher *matcher.Analyzer
	budget  RelabelNameFlowBudget
	stats   RelabelNameFlowStats
}

// NewRelabelNameFlowAnalyzer creates a bounded flow analyzer. relabelAnalyzer
// is shared so regexp/replacement work consumes one aggregate relabel budget.
func NewRelabelNameFlowAnalyzer(
	ctx context.Context,
	relabelAnalyzer *relabel.Analyzer,
	matcherAnalyzer *matcher.Analyzer,
	budget RelabelNameFlowBudget,
) (*RelabelNameFlowAnalyzer, error) {
	if ctx == nil || relabelAnalyzer == nil || matcherAnalyzer == nil {
		return nil, errors.New("nil relabel name-flow dependency")
	}
	if budget.MaxScopeIntersections < 0 {
		return nil, errors.New("invalid negative relabel name-flow budget")
	}
	if budget.MaxScopeIntersections == 0 {
		budget.MaxScopeIntersections = defaultRelabelNameFlowMaxScopeIntersections
	}
	return &RelabelNameFlowAnalyzer{
		ctx: ctx, relabel: relabelAnalyzer, matcher: matcherAnalyzer, budget: budget,
	}, nil
}

// Stats returns work consumed so far.
func (a *RelabelNameFlowAnalyzer) Stats() RelabelNameFlowStats { return a.stats }

// Mutation describes a metric-name-writing rule. nameDerived says the rule's
// input is still derived exclusively from the original metric name.
func (a *RelabelNameFlowAnalyzer) Mutation(
	rule relabel.Config,
	nameDerived bool,
) (RelabelNameMutation, error) {
	rule = rule.WithDefaults()
	effect := RelabelNameMutation{applicationDerived: !nameDerived, reachable: true, outputPattern: "*"}
	if relabel.EffectiveAction(rule) != relabel.Replace {
		return effect, nil
	}
	pattern, possible, err := a.relabel.ReplacementGlob(rule.Regex, rule.Replacement)
	if err != nil {
		return RelabelNameMutation{}, err
	}
	if !possible {
		effect.reachable = false
		return effect, nil
	}
	effect.outputPattern = pattern
	return effect, nil
}

// MutationsMayReach reports whether a prior mutation can enter matchExpr.
func (a *RelabelNameFlowAnalyzer) MutationsMayReach(
	effects []RelabelNameMutation,
	matchExpr string,
	applicationDerivedOnly bool,
) (bool, error) {
	for _, effect := range effects {
		if !effect.reachable || (applicationDerivedOnly && !effect.applicationDerived) {
			continue
		}
		if err := a.addScopeIntersection(); err != nil {
			return false, err
		}
		_, intersects, err := a.matcher.SimplePatternIntersectionWitness(
			effect.outputPattern, matchExpr, true,
		)
		if err != nil {
			return false, err
		}
		if intersects {
			return true, nil
		}
	}
	return false, nil
}

// PreservedCaptureLabel finds an earlier rule that copies one of
// dynamicCaptureIDs into a stable label and proves that later reachable rules
// preserve that label.
func (a *RelabelNameFlowAnalyzer) PreservedCaptureLabel(
	blocks []relabel.Block,
	blockIndex int,
	rewriteIndex int,
	rewrite relabel.Config,
	dynamicCaptureIDs []int,
	possibleOutputs []string,
) (string, bool, error) {
	rewrite = rewrite.WithDefaults()
	dynamicCaptures := make(map[int]struct{}, len(dynamicCaptureIDs))
	for _, capture := range dynamicCaptureIDs {
		dynamicCaptures[capture] = struct{}{}
	}
	if len(dynamicCaptures) == 0 {
		return "", false, nil
	}

	rules := blocks[blockIndex].MetricRelabelConfigs
	for ruleIndex := rewriteIndex - 1; ruleIndex >= 0; ruleIndex-- {
		candidate := rules[ruleIndex].WithDefaults()
		if relabel.EffectiveAction(candidate) != relabel.Replace ||
			len(candidate.SourceLabels) != 1 || candidate.SourceLabels[0] != labels.MetricName ||
			candidate.Regex.String() != rewrite.Regex.String() ||
			candidate.TargetLabel == labels.MetricName ||
			!commonmodel.UTF8Validation.IsValidLabelName(candidate.TargetLabel) {
			continue
		}
		capture, exact, err := a.relabel.ExactCaptureReference(candidate.Regex, candidate.Replacement)
		if err != nil {
			return "", false, err
		}
		if !exact {
			continue
		}
		if _, ok := dynamicCaptures[capture]; !ok {
			continue
		}
		preservesBefore, err := a.relabel.RulesPreserveLabel(
			rules[ruleIndex+1:rewriteIndex], candidate.TargetLabel, nil,
		)
		if err != nil {
			return "", false, err
		}
		preservesNameBefore, err := a.relabel.RulesPreserveLabel(
			rules[ruleIndex+1:rewriteIndex], labels.MetricName, nil,
		)
		if err != nil {
			return "", false, err
		}
		preservesAfter, err := a.relabel.RulesPreserveLabel(
			rules[rewriteIndex+1:], candidate.TargetLabel, possibleOutputs,
		)
		if err != nil {
			return "", false, err
		}
		nameUnknown, err := a.rulesMayMutateMetricName(rules[rewriteIndex+1:])
		if err != nil {
			return "", false, err
		}
		preservesLater, err := a.laterBlocksPreserveLabel(
			blocks[blockIndex+1:], candidate.TargetLabel, possibleOutputs, nameUnknown,
		)
		if err != nil {
			return "", false, err
		}
		if preservesBefore && preservesNameBefore && preservesAfter && preservesLater {
			return candidate.TargetLabel, true, nil
		}
	}
	return "", false, nil
}

func (a *RelabelNameFlowAnalyzer) laterBlocksPreserveLabel(
	blocks []relabel.Block,
	labelName string,
	possibleNames []string,
	nameUnknown bool,
) (bool, error) {
	for _, block := range blocks {
		reachable := nameUnknown
		if !reachable {
			match, err := matcher.NewSimplePatternsMatcher(block.Match)
			if err != nil {
				return false, err
			}
			reachable = slices.ContainsFunc(possibleNames, match.MatchString)
		}
		if !reachable {
			continue
		}
		var names []string
		if !nameUnknown {
			names = possibleNames
		}
		preserves, err := a.relabel.RulesPreserveLabel(block.MetricRelabelConfigs, labelName, names)
		if err != nil || !preserves {
			return false, err
		}
		mutatesName, err := a.rulesMayMutateMetricName(block.MetricRelabelConfigs)
		if err != nil {
			return false, err
		}
		if mutatesName {
			nameUnknown = true
		}
	}
	return true, nil
}

func (a *RelabelNameFlowAnalyzer) rulesMayMutateMetricName(rules []relabel.Config) (bool, error) {
	for _, rule := range rules {
		writes, err := a.relabel.RuleMayWriteLabel(rule, labels.MetricName)
		if err != nil {
			return false, err
		}
		if writes {
			return true, nil
		}
	}
	return false, nil
}

func (a *RelabelNameFlowAnalyzer) addScopeIntersection() error {
	if err := a.ctx.Err(); err != nil {
		return err
	}
	if a.stats.ScopeIntersections >= a.budget.MaxScopeIntersections {
		return fmt.Errorf(
			"relabel name-flow scope-intersection budget exceeded: reached %d",
			a.budget.MaxScopeIntersections,
		)
	}
	a.stats.ScopeIntersections++
	return nil
}
