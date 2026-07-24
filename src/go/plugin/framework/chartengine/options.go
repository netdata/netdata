// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"fmt"
	"slices"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	metrixselector "github.com/netdata/netdata/go/plugins/pkg/metrix/selector"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/framework/runtimecomp"
)

type engineConfig struct {
	autogen       AutogenPolicy
	autogenRules  []charttpl.ValidatedAutogenRule
	autogenTypeID string
	// autogenContextNamespace prefixes autogen chart contexts (the spec's root
	// context_namespace), so autogen and template charts share one namespace.
	autogenContextNamespace string
	selector                metrixselector.Selector
	autogenOverride         policyOverride[AutogenPolicy]
	autogenRulesOverride    policyOverride[[]charttpl.ValidatedAutogenRule]
	selectorOverride        policyOverride[metrixselector.Selector]
	runtimeStore            metrix.RuntimeStore
	runtimeStoreSet         bool
	runtimeObserver         func(PlanRuntimeSample)
	log                     *logger.Logger
	seriesSelection         seriesSelectionMode
	runtimePlanner          bool
}

type policyOverride[T any] struct {
	set   bool
	value T
}

// Option mutates engine configuration at construction time.
type Option func(*engineConfig) error

const (
	defaultMaxTypeIDLen = 1200
)

type seriesSelectionMode uint8

const (
	seriesSelectionLastSuccessOnly seriesSelectionMode = iota
	seriesSelectionAllVisible
)

// AutogenPolicy controls unmatched-series fallback chart generation.
// It aliases runtime component policy to keep one policy contract.
type AutogenPolicy = runtimecomp.AutogenPolicy

// AutogenRule conditionally constrains unmatched-series fallback.
type AutogenRule = runtimecomp.AutogenRule

// EnginePolicy controls chartengine matching/materialization behavior.
type EnginePolicy struct {
	// Selector filters input series globally before template/autogen routing.
	// Nil means "no override", and an explicitly empty expr means "override to allow all".
	Selector *metrixselector.Expr

	// Autogen controls unmatched-series fallback behavior.
	Autogen *AutogenPolicy
}

func defaultAutogenPolicy() AutogenPolicy {
	return AutogenPolicy{
		Enabled:      false,
		MaxTypeIDLen: defaultMaxTypeIDLen,
	}
}

func normalizeAutogenPolicy(policy AutogenPolicy) (AutogenPolicy, []charttpl.ValidatedAutogenRule, error) {
	templateRules := make([]charttpl.EngineAutogenRule, len(policy.Rules))
	for i, rule := range policy.Rules {
		templateRules[i] = charttpl.EngineAutogenRule{
			Scope: rule.Scope,
			Selector: metrixselector.Expr{
				Allow: slices.Clone(rule.Selector.Allow),
				Deny:  slices.Clone(rule.Selector.Deny),
			},
		}
	}
	rules, err := charttpl.CompileAutogenRules(templateRules)
	if err != nil {
		return AutogenPolicy{}, nil, fmt.Errorf("autogen rules: %w", err)
	}
	return normalizeAutogenPolicyWithRules(policy, rules)
}

func normalizeAutogenPolicyWithRules(
	policy AutogenPolicy,
	rules []charttpl.ValidatedAutogenRule,
) (AutogenPolicy, []charttpl.ValidatedAutogenRule, error) {
	maxLen := policy.MaxTypeIDLen
	if maxLen <= 0 {
		maxLen = defaultMaxTypeIDLen
	}
	if maxLen < 4 {
		return AutogenPolicy{}, nil,
			fmt.Errorf("autogen max type.id len must be >= 4, got %d", maxLen)
	}
	policy.Rules = cloneAutogenRules(policy.Rules)
	policy.MaxTypeIDLen = maxLen
	return policy, slices.Clone(rules), nil
}

func compileEngineSelector(expr metrixselector.Expr) (metrixselector.Selector, error) {
	if expr.Empty() {
		return nil, nil
	}
	return expr.Parse()
}

// WithEnginePolicy configures chartengine matching/materialization policy.
func WithEnginePolicy(policy EnginePolicy) Option {
	policy = cloneEnginePolicy(policy)
	var (
		autogen          AutogenPolicy
		autogenRules     []charttpl.ValidatedAutogenRule
		autogenErr       error
		compiledSelector metrixselector.Selector
		selectorErr      error
	)
	if policy.Autogen != nil {
		autogen, autogenRules, autogenErr = normalizeAutogenPolicy(*policy.Autogen)
	}
	if policy.Selector != nil {
		compiledSelector, selectorErr = compileEngineSelector(*policy.Selector)
	}
	return func(cfg *engineConfig) error {
		if autogenErr != nil {
			return autogenErr
		}
		if selectorErr != nil {
			return fmt.Errorf("invalid engine selector: %w", selectorErr)
		}
		if policy.Autogen != nil {
			cfg.autogenOverride = policyOverride[AutogenPolicy]{set: true, value: cloneAutogenPolicy(autogen)}
			cfg.autogenRulesOverride = policyOverride[[]charttpl.ValidatedAutogenRule]{
				set:   true,
				value: slices.Clone(autogenRules),
			}
			cfg.autogen = cloneAutogenPolicy(autogen)
			cfg.autogenRules = slices.Clone(autogenRules)
		}
		if policy.Selector != nil {
			cfg.selectorOverride = policyOverride[metrixselector.Selector]{set: true, value: compiledSelector}
			cfg.selector = compiledSelector
		}
		return nil
	}
}

func cloneEnginePolicy(policy EnginePolicy) EnginePolicy {
	out := policy
	if policy.Autogen != nil {
		autogen := cloneAutogenPolicy(*policy.Autogen)
		out.Autogen = &autogen
	}
	if policy.Selector != nil {
		selector := *policy.Selector
		selector.Allow = slices.Clone(policy.Selector.Allow)
		selector.Deny = slices.Clone(policy.Selector.Deny)
		out.Selector = &selector
	}
	return out
}

func cloneAutogenPolicy(policy AutogenPolicy) AutogenPolicy {
	out := policy
	out.Rules = cloneAutogenRules(policy.Rules)
	return out
}

func cloneAutogenRules(rules []runtimecomp.AutogenRule) []runtimecomp.AutogenRule {
	out := make([]runtimecomp.AutogenRule, len(rules))
	for i, rule := range rules {
		out[i] = runtimecomp.AutogenRule{
			Scope: rule.Scope,
			Selector: metrixselector.Expr{
				Allow: slices.Clone(rule.Selector.Allow),
				Deny:  slices.Clone(rule.Selector.Deny),
			},
		}
	}
	return out
}

// WithRuntimeStore configures internal chartengine runtime metrics store.
// Passing nil disables chartengine self-metrics.
func WithRuntimeStore(store metrix.RuntimeStore) Option {
	return func(cfg *engineConfig) error {
		cfg.runtimeStore = store
		cfg.runtimeStoreSet = true
		return nil
	}
}

// WithRuntimeSampleObserver configures a callback for per-build runtime samples.
//
// The callback fires for successful builds, build errors, and collect-status
// skips. It does not fire for pre-build contract errors such as an outstanding
// plan attempt.
//
// The callback is in addition to WithRuntimeStore. Pass WithRuntimeStore(nil)
// when samples are aggregated elsewhere and the engine should not write its own
// runtime metrics.
func WithRuntimeSampleObserver(fn func(PlanRuntimeSample)) Option {
	return func(cfg *engineConfig) error {
		cfg.runtimeObserver = fn
		return nil
	}
}

// WithLogger configures chartengine logger.
func WithLogger(l *logger.Logger) Option {
	return func(cfg *engineConfig) error {
		cfg.log = l
		return nil
	}
}

// WithSeriesSelectionAllVisible configures planner scan to use all visible
// series from the reader (instead of only latest-success-seq series).
// This is intended for runtime/internal stores.
func WithSeriesSelectionAllVisible() Option {
	return func(cfg *engineConfig) error {
		cfg.seriesSelection = seriesSelectionAllVisible
		return nil
	}
}

// WithRuntimePlannerMode enables runtime/internal planner semantics for
// build-cycle dedupe bookkeeping while keeping lifecycle/cache tied to
// source LastSuccessSeq.
func WithRuntimePlannerMode() Option {
	return func(cfg *engineConfig) error {
		cfg.runtimePlanner = true
		return nil
	}
}

// WithEmitTypeIDBudgetPrefix configures chartengine autogen type-id budget
// checks to use the effective emission type-id prefix (for example job fullName).
func WithEmitTypeIDBudgetPrefix(typeID string) Option {
	return func(cfg *engineConfig) error {
		cfg.autogenTypeID = typeID
		return nil
	}
}

func applyOptions(opts ...Option) (engineConfig, error) {
	cfg := engineConfig{
		autogen:         defaultAutogenPolicy(),
		seriesSelection: seriesSelectionLastSuccessOnly,
	}
	for i, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt(&cfg); err != nil {
			return engineConfig{}, fmt.Errorf("chartengine: option[%d]: %w", i, err)
		}
	}
	return cfg, nil
}
