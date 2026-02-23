// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"fmt"
	"sync"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine/internal/program"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
)

// Engine is the chartengine runtime scaffold.
//
// Current scope:
//   - owns compiled immutable program snapshots,
//   - supports load/reload from decoded specs or YAML,
//   - keeps previous compiled snapshot on failed reload.
//   - owns planner runtime state (match index, route cache, materialized lifecycle).
type Engine struct {
	mu    sync.RWMutex
	state engineState
}

// New creates an engine scaffold with optional configuration.
func New(opts ...Option) (*Engine, error) {
	cfg, err := applyOptions(opts...)
	if err != nil {
		return nil, err
	}

	runtimeStore := cfg.runtimeStore
	if !cfg.runtimeStoreSet {
		runtimeStore = metrix.NewRuntimeStore()
	}

	return &Engine{
		state: engineState{
			cfg:          cfg,
			routeCache:   newRouteCache(),
			materialized: newMaterializedState(),
			runtimeStore: runtimeStore,
			runtimeStats: newRuntimeMetrics(runtimeStore),
			log:          cfg.log,
		},
	}, nil
}

// Load compiles and publishes a new immutable program revision.
func (e *Engine) Load(spec *charttpl.Spec, revision uint64) error {
	if e == nil {
		return fmt.Errorf("chartengine: nil engine")
	}

	compiled, err := Compile(spec, revision)
	if err != nil {
		e.logWarningf("chartengine load failed revision=%d: %v", revision, err)
		return err
	}

	e.mu.RLock()
	cfg := e.state.cfg
	e.mu.RUnlock()

	autogenPolicy, selectorPolicy, err := resolveEffectivePolicy(cfg, spec.Engine)
	if err != nil {
		e.logWarningf("chartengine load failed revision=%d: %v", revision, err)
		return err
	}

	e.mu.Lock()
	e.state.cfg.autogen = autogenPolicy
	e.state.cfg.selector = selectorPolicy
	e.state.program = compiled
	e.state.matchIndex = buildMatchIndex(compiled.Charts())
	// Template revision change resets routing/materialization internals.
	e.state.routeCache = newRouteCache()
	e.state.materialized = newMaterializedState()
	e.mu.Unlock()
	e.logInfof("chartengine program loaded revision=%d charts=%d metrics=%d", revision, len(compiled.Charts()), len(compiled.MetricNames()))
	return nil
}

// LoadYAML decodes chart-template YAML, compiles it, and publishes the program.
func (e *Engine) LoadYAML(data []byte, revision uint64) error {
	spec, err := charttpl.DecodeYAML(data)
	if err != nil {
		return err
	}
	return e.Load(spec, revision)
}

// loadYAMLFile reads chart-template YAML from file, compiles and publishes it.
func (e *Engine) loadYAMLFile(path string, revision uint64) error {
	spec, err := charttpl.DecodeYAMLFile(path)
	if err != nil {
		return err
	}
	return e.Load(spec, revision)
}

// program returns the latest compiled immutable program snapshot.
func (e *Engine) program() *program.Program {
	if e == nil {
		return nil
	}
	e.mu.RLock()
	p := e.state.program
	e.mu.RUnlock()
	return p
}

// ready reports whether a compiled program is currently loaded.
func (e *Engine) ready() bool {
	return e.program() != nil
}
