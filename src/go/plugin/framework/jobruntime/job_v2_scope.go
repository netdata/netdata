// SPDX-License-Identifier: GPL-3.0-or-later

package jobruntime

import (
	"fmt"
	"maps"
	"sort"

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
	engine, err := chartengine.New(opts...)
	if err != nil {
		return nil, err
	}
	if err := engine.LoadYAML(j.chartTemplateYAML, j.chartTemplateRevision); err != nil {
		return nil, err
	}
	return engine, nil
}

func (j *JobV2) liveScopeSet() map[string]metrix.HostScope {
	scopes := make(map[string]metrix.HostScope)
	reader := j.store.Read(metrix.ReadRaw(), metrix.ReadFlatten())
	for _, scope := range reader.HostScopes() {
		if j.scopeHasVisibleSeries(scope.ScopeKey) {
			scopes[scope.ScopeKey] = scope
		}
	}
	return scopes
}

func (j *JobV2) scopeWorkSet(liveScopes map[string]metrix.HostScope) map[string]metrix.HostScope {
	scopes := make(map[string]metrix.HostScope, len(liveScopes)+len(j.scopeStates))
	maps.Copy(scopes, liveScopes)
	// Retain previously emitted scopes until their engine emits lifecycle
	// removals. This includes default scope when unscoped series disappear.
	for key, state := range j.scopeStates {
		if _, ok := scopes[key]; ok {
			continue
		}
		scopes[key] = state.scope
	}
	return scopes
}

func (j *JobV2) scopeHasVisibleSeries(scopeKey string) bool {
	reader := j.store.Read(metrix.ReadFlatten(), metrix.ReadHostScope(scopeKey))
	found := false
	reader.ForEachSeries(func(string, metrix.LabelView, metrix.SampleValue) {
		found = true
	})
	return found
}

func sortedScopeKeys(scopes map[string]metrix.HostScope) []string {
	keys := make([]string, 0, len(scopes))
	for key := range scopes {
		keys = append(keys, key)
	}
	sortHostScopeKeys(keys)
	return keys
}

func sortedScopeStateKeys(scopes map[string]*jobV2ScopeState) []string {
	keys := make([]string, 0, len(scopes))
	for key := range scopes {
		keys = append(keys, key)
	}
	sortHostScopeKeys(keys)
	return keys
}

func sortHostScopeKeys(keys []string) {
	sort.Slice(keys, func(i, j int) bool {
		if keys[i] == defaultHostScopeKey {
			return true
		}
		if keys[j] == defaultHostScopeKey {
			return false
		}
		return keys[i] < keys[j]
	})
}

func metrixHostScopeInfo(scope metrix.HostScope) netdataapi.HostInfo {
	return netdataapi.HostInfo{
		GUID:     scope.GUID,
		Hostname: scope.Hostname,
		Labels:   scope.Labels,
	}
}
