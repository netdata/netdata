// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"fmt"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/chartengine/internal/program"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/charttpl"
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
	return &Engine{
		state: engineState{
			cfg:          cfg,
			routeCache:   newRouteCache(),
			materialized: newMaterializedState(),
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
		return err
	}

	e.mu.Lock()
	e.state.program = compiled
	e.state.matchIndex = buildMatchIndex(compiled.Charts())
	// Template revision change resets routing/materialization internals.
	e.state.routeCache = newRouteCache()
	e.state.materialized = newMaterializedState()
	e.mu.Unlock()
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

// LoadYAMLFile reads chart-template YAML from file, compiles and publishes it.
func (e *Engine) LoadYAMLFile(path string, revision uint64) error {
	spec, err := charttpl.DecodeYAMLFile(path)
	if err != nil {
		return err
	}
	return e.Load(spec, revision)
}

// Program returns the latest compiled immutable program snapshot.
func (e *Engine) Program() *program.Program {
	if e == nil {
		return nil
	}
	e.mu.RLock()
	p := e.state.program
	e.mu.RUnlock()
	return p
}

// Ready reports whether a compiled program is currently loaded.
func (e *Engine) Ready() bool {
	return e.Program() != nil
}
