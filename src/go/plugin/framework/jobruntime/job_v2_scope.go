// SPDX-License-Identifier: GPL-3.0-or-later

package jobruntime

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
)

const defaultHostScopeKey = ""

func (j *JobV2) ensureScopeState(scope metrix.HostScope) (*jobV2ScopeState, error) {
	if j == nil {
		return nil, fmt.Errorf("nil job")
	}
	scopeKey := scope.ScopeKey
	if scope.IsDefault() {
		scope = metrix.HostScope{}
		scopeKey = defaultHostScopeKey
	}
	if j.scopeStates == nil {
		j.scopeStates = make(map[string]*jobV2ScopeState)
	}
	if state := j.scopeStates[scopeKey]; state != nil {
		state.scope = scope
		return state, nil
	}

	engine, err := j.newScopeEngine()
	if err != nil {
		return nil, err
	}
	state := &jobV2ScopeState{
		scopeKey: scopeKey,
		scope:    scope,
		engine:   engine,
	}
	j.scopeStates[scopeKey] = state
	return state, nil
}

func (j *JobV2) newScopeEngine() (*chartengine.Engine, error) {
	opts := append([]chartengine.Option{}, j.engineOptions...)
	opts = append(opts, chartengine.WithRuntimeStore(nil))
	if j.runtimeAggregator != nil {
		opts = append(opts, chartengine.WithRuntimeSampleObserver(j.runtimeAggregator.Observe))
	}
	return chartengine.New(opts...)
}

// jobV2ScopeWork is one host scope a cycle plans.
type jobV2ScopeWork struct {
	scope metrix.HostScope
	live  bool
}

// scopeWork lists this cycle's host scopes ordered by scope key, so the default
// scope comes first: every fresh-visible scope, plus previously emitted scopes
// retained until their engine emits lifecycle removals. The latter includes the
// default scope when unscoped series disappear.
func (j *JobV2) scopeWork() []jobV2ScopeWork {
	reader := j.store.Read(metrix.ReadFlatten())
	live := reader.(metrix.FreshVisibleHostScopesReader).FreshVisibleHostScopes()
	work := make([]jobV2ScopeWork, 0, max(len(live), len(j.scopeStates)))
	for _, scope := range live {
		work = append(work, jobV2ScopeWork{
			scope: scope,
			live:  true,
		})
	}
	slices.SortFunc(work, compareScopeWork)
	liveWork := work
	for key, state := range j.scopeStates {
		if _, found := slices.BinarySearchFunc(liveWork, key, compareScopeWorkKey); !found {
			work = append(work, jobV2ScopeWork{
				scope: state.scope,
			})
		}
	}
	if len(work) > len(liveWork) {
		slices.SortFunc(work, compareScopeWork)
	}
	return work
}

func compareScopeWork(a, b jobV2ScopeWork) int {
	return strings.Compare(a.scope.ScopeKey, b.scope.ScopeKey)
}

func compareScopeWorkKey(w jobV2ScopeWork, key string) int {
	return strings.Compare(w.scope.ScopeKey, key)
}

// sortedScopeStateKeys orders scope keys like scopeWork: the default scope's empty
// key sorts first.
func sortedScopeStateKeys(scopes map[string]*jobV2ScopeState) []string {
	return slices.Sorted(maps.Keys(scopes))
}

func metrixHostScopeInfo(scope metrix.HostScope) netdataapi.HostInfo {
	return netdataapi.HostInfo{
		GUID:     scope.GUID,
		Hostname: scope.Hostname,
		Labels:   scope.Labels,
	}
}
