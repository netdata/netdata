// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	metrixselector "github.com/netdata/netdata/go/plugins/pkg/metrix/selector"
	"github.com/netdata/netdata/go/plugins/plugin/framework/runtimecomp"
)

type engineConfig struct {
	autogen          AutogenPolicy
	autogenTypeID    string
	selector         metrixselector.Selector
	autogenOverride  policyOverride[AutogenPolicy]
	selectorOverride policyOverride[metrixselector.Selector]
	runtimeStore     metrix.RuntimeStore
	runtimeStoreSet  bool
	log              *logger.Logger
	seriesSelection  seriesSelectionMode
	runtimePlanner   bool
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

func normalizeAutogenPolicy(policy AutogenPolicy) (AutogenPolicy, error) {
	maxLen := policy.MaxTypeIDLen
	if maxLen <= 0 {
		maxLen = defaultMaxTypeIDLen
	}
	if maxLen < 4 {
		return AutogenPolicy{}, fmt.Errorf("autogen max type.id len must be >= 4, got %d", maxLen)
	}
	policy.MaxTypeIDLen = maxLen
	return policy, nil
}

func compileEngineSelector(expr metrixselector.Expr) (metrixselector.Selector, error) {
	if expr.Empty() {
		return nil, nil
	}
	return expr.Parse()
}

// WithEnginePolicy configures chartengine matching/materialization policy.
func WithEnginePolicy(policy EnginePolicy) Option {
	return func(cfg *engineConfig) error {
		if policy.Autogen != nil {
			autogen, err := normalizeAutogenPolicy(*policy.Autogen)
			if err != nil {
				return err
			}
			cfg.autogenOverride = policyOverride[AutogenPolicy]{set: true, value: autogen}
			cfg.autogen = autogen
		}
		if policy.Selector != nil {
			selector, err := compileEngineSelector(*policy.Selector)
			if err != nil {
				return fmt.Errorf("invalid engine selector: %w", err)
			}
			cfg.selectorOverride = policyOverride[metrixselector.Selector]{set: true, value: selector}
			cfg.selector = selector
		}
		return nil
	}
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
