// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"slices"
	"sort"
	"strings"
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
	// Keep metadata separate so structured families never enter scalar iteration.
	structuredMetaByName map[string]*instrumentDescriptor
	names                []string
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
		// scalars collects scalar series once; sorting them by scope, name and labels
		// key yields every scope's name list and per-name series slices without
		// per-name allocation.
		scalars = make([]*committedSeries, 0, len(series))
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
		if item.desc == nil {
			continue
		}
		scope := index.ensureScope(item.hostScopeKey)
		if !isScalarKind(item.desc.kind) {
			if scope.structuredMetaByName == nil {
				scope.structuredMetaByName = make(map[string]*instrumentDescriptor)
			}
			scope.structuredMetaByName[item.name] = item.desc
			continue
		}
		scalars = append(scalars, item)
	}
	indexScalarSeries(index, scalars)

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

	return index
}

// indexScalarSeries sorts scalars by scope, name and labels key and carves each
// scope's names and per-name series from the sorted slice. Keys are unique per
// series, so the order is total.
func indexScalarSeries(index *snapshotSeriesIndex, scalars []*committedSeries) {
	slices.SortFunc(scalars, func(a, b *committedSeries) int {
		if c := strings.Compare(a.hostScopeKey, b.hostScopeKey); c != 0 {
			return c
		}
		if c := strings.Compare(a.name, b.name); c != 0 {
			return c
		}
		return strings.Compare(a.labelsKey, b.labelsKey)
	})
	for start := 0; start < len(scalars); {
		scopeKey := scalars[start].hostScopeKey
		end := start
		names := 0
		for end < len(scalars) && scalars[end].hostScopeKey == scopeKey {
			if end == start || scalars[end].name != scalars[end-1].name {
				names++
			}
			end++
		}
		scope := index.ensureScope(scopeKey)
		scope.byName = make(map[string][]*committedSeries, names)
		scope.names = make([]string, 0, names)
		for i := start; i < end; {
			j := i + 1
			for j < end && scalars[j].name == scalars[i].name {
				j++
			}
			scope.byName[scalars[i].name] = scalars[i:j:j]
			scope.names = append(scope.names, scalars[i].name)
			i = j
		}
		start = end
	}
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
