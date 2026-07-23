// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"sort"
	"sync"
	"sync/atomic"
)

type snapshotSeriesIndex struct {
	hostScopes             []HostScope
	freshVisibleHostScopes []HostScope
	defaultScope           snapshotScopeIndex
	hasDefaultScope        bool
	byScope                map[string]*snapshotScopeIndex
	// singleHostScope backs the common unscoped catalog without a slice allocation.
	singleHostScope [1]HostScope
}

type snapshotScopeIndex struct {
	byName map[string][]*committedSeries
	names  []string
}

type indexedHostScope struct {
	scope HostScope
	fresh bool
}

type retryableLazyPointer[T any] struct {
	value atomic.Pointer[T]
	mu    sync.Mutex
}

func (lazy *retryableLazyPointer[T]) get(build func() *T) *T {
	if value := lazy.value.Load(); value != nil {
		return value
	}

	lazy.mu.Lock()
	defer lazy.mu.Unlock()
	if value := lazy.value.Load(); value != nil {
		return value
	}
	value := build()
	lazy.value.Store(value)
	return value
}

func attachCollectorSnapshot(snapshot *readSnapshot) *readSnapshot {
	snapshot.collector = &collectorSnapshotState{}
	return snapshot
}

func (snapshot *readSnapshot) collectorFlattenedSnapshot() *readSnapshot {
	return snapshot.collector.flattened.get(func() *readSnapshot {
		return attachCollectorSnapshot(flattenSnapshot(snapshot))
	})
}

func (snapshot *readSnapshot) seriesIndex() *snapshotSeriesIndex {
	if snapshot.index != nil {
		return snapshot.index
	}
	return snapshot.collector.index.get(func() *snapshotSeriesIndex {
		return buildSnapshotSeriesIndex(snapshot.series, snapshot.collectMeta)
	})
}

func buildSnapshotSeriesIndex(series map[string]*committedSeries, meta CollectMeta) *snapshotSeriesIndex {
	index := &snapshotSeriesIndex{}
	var (
		defaultHostScope indexedHostScope
		hasDefaultHost   bool
		hostScopes       map[string]indexedHostScope
	)

	for _, item := range series {
		fresh := freshSeriesVisible(item, meta)
		if item.hostScopeKey == "" {
			defaultHostScope.scope = item.hostScope
			defaultHostScope.fresh = defaultHostScope.fresh || fresh
			hasDefaultHost = true
		} else {
			if hostScopes == nil {
				hostScopes = make(map[string]indexedHostScope)
			}
			hostScope := hostScopes[item.hostScopeKey]
			hostScope.scope = item.hostScope
			hostScope.fresh = hostScope.fresh || fresh
			hostScopes[item.hostScopeKey] = hostScope
		}
		if item.desc == nil || !isScalarKind(item.desc.kind) {
			continue
		}
		scope := index.ensureScope(item.hostScopeKey)
		if scope.byName == nil {
			scope.byName = make(map[string][]*committedSeries)
		}
		scope.byName[item.name] = append(scope.byName[item.name], item)
	}

	totalScopes := len(hostScopes)
	freshScopes := 0
	if hasDefaultHost {
		totalScopes++
		if defaultHostScope.fresh {
			freshScopes++
		}
	}
	for _, hostScope := range hostScopes {
		if hostScope.fresh {
			freshScopes++
		}
	}

	if totalScopes == 1 && hasDefaultHost {
		index.singleHostScope[0] = cloneHostScope(defaultHostScope.scope)
		index.hostScopes = index.singleHostScope[:]
	} else if totalScopes > 0 {
		index.hostScopes = make([]HostScope, 0, totalScopes)
		if hasDefaultHost {
			index.hostScopes = append(index.hostScopes, cloneHostScope(defaultHostScope.scope))
		}
		keys := make([]string, 0, len(hostScopes))
		for key := range hostScopes {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			index.hostScopes = append(index.hostScopes, cloneHostScope(hostScopes[key].scope))
		}
	}

	if freshScopes == totalScopes {
		index.freshVisibleHostScopes = index.hostScopes
	} else if freshScopes > 0 {
		index.freshVisibleHostScopes = make([]HostScope, 0, freshScopes)
		if hasDefaultHost && defaultHostScope.fresh {
			index.freshVisibleHostScopes = append(index.freshVisibleHostScopes, index.hostScopes[0])
		}
		for _, scope := range index.hostScopes {
			if scope.ScopeKey == "" {
				continue
			}
			if hostScopes[scope.ScopeKey].fresh {
				index.freshVisibleHostScopes = append(index.freshVisibleHostScopes, scope)
			}
		}
	}

	if index.hasDefaultScope {
		buildSnapshotScopeIndex(&index.defaultScope)
	}
	for _, scope := range index.byScope {
		buildSnapshotScopeIndex(scope)
	}
	return index
}

func (index *snapshotSeriesIndex) ensureScope(scopeKey string) *snapshotScopeIndex {
	if scopeKey == "" {
		index.hasDefaultScope = true
		return &index.defaultScope
	}
	if index.byScope == nil {
		index.byScope = make(map[string]*snapshotScopeIndex)
	}
	scope := index.byScope[scopeKey]
	if scope == nil {
		scope = &snapshotScopeIndex{}
		index.byScope[scopeKey] = scope
	}
	return scope
}

func (index *snapshotSeriesIndex) scope(scopeKey string) *snapshotScopeIndex {
	if scopeKey == "" {
		if index.hasDefaultScope {
			return &index.defaultScope
		}
		return nil
	}
	return index.byScope[scopeKey]
}

func buildSnapshotScopeIndex(scope *snapshotScopeIndex) {
	scope.names = make([]string, 0, len(scope.byName))
	for name, items := range scope.byName {
		scope.names = append(scope.names, name)
		sort.Slice(items, func(i, j int) bool {
			return items[i].labelsKey < items[j].labelsKey
		})
	}
	sort.Strings(scope.names)
}

func freshSeriesVisible(series *committedSeries, meta CollectMeta) bool {
	if series.desc == nil {
		return false
	}
	if series.desc.freshness == FreshnessCommitted {
		return true
	}
	return meta.LastAttemptStatus == CollectStatusSuccess &&
		series.meta.LastSeenSuccessSeq == meta.LastSuccessSeq
}

func cloneHostScopes(scopes []HostScope) []HostScope {
	if len(scopes) == 0 {
		return nil
	}
	cloned := make([]HostScope, len(scopes))
	for i, scope := range scopes {
		cloned[i] = cloneHostScope(scope)
	}
	return cloned
}
