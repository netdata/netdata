// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"strconv"
	"strings"
	"sync"
)

// vecCache stores per-label-values instrument handles with a read-fast path.
//
// Contract: cache entries are intentionally unbounded for the lifetime of the
// vec handle. Store-level retention evicts committed series state, but it does
// not prune vecCache handles. Collectors must keep vec label cardinality bounded.
type vecCache[T any] struct {
	backend meterBackend
	base    []LabelSet
	keys    []string

	mu    sync.RWMutex
	cache map[string]T

	makeHandle func(scope HostScope, base []LabelSet, vecSet LabelSet) T
}

func newVecCache[T any](
	backend meterBackend,
	base []LabelSet,
	keys []string,
	makeHandle func(scope HostScope, base []LabelSet, vecSet LabelSet) T,
) *vecCache[T] {
	return &vecCache[T]{
		backend:    backend,
		base:       base,
		keys:       keys,
		cache:      make(map[string]T),
		makeHandle: makeHandle,
	}
}

func (v *vecCache[T]) get(scope HostScope, labelValues ...string) (T, error) {
	var zero T

	cacheKey, err := vecSeriesCacheKey(v.keys, labelValues)
	if err != nil {
		return zero, err
	}
	scope = mustNormalizeHostScope(scope)
	cacheKey = scopedVecCacheKey(scope.ScopeKey, cacheKey)

	v.mu.RLock()
	inst, ok := v.cache[cacheKey]
	v.mu.RUnlock()
	if ok {
		return inst, nil
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	inst, ok = v.cache[cacheKey]
	if ok {
		return inst, nil
	}

	vecSet, err := vecSeriesCompileLabelSet(v.backend, v.keys, labelValues)
	if err != nil {
		return zero, err
	}

	inst = v.makeHandle(scope, v.base, vecSet)
	v.cache[cacheKey] = inst
	return inst, nil
}

func appendVecSet(base []LabelSet, vecSet LabelSet) []LabelSet {
	return appendLabelSets(base, []LabelSet{vecSet})
}

func mustRegisterInstrument(backend meterBackend, name string, kind metricKind, mode metricMode, opts ...InstrumentOption) *instrumentDescriptor {
	desc, err := backend.registerInstrument(name, kind, mode, opts...)
	if err != nil {
		panic(err)
	}
	return desc
}

func mustNormalizeVecLabelKeys(labelKeys []string) []string {
	keys, err := normalizeVecLabelKeys(labelKeys)
	if err != nil {
		panic(err)
	}
	return keys
}

// normalizeVecLabelKeys validates and copies vec label keys in declared order.
func normalizeVecLabelKeys(labelKeys []string) ([]string, error) {
	keys := append([]string(nil), labelKeys...)
	seen := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		if k == "" {
			return nil, errInvalidLabelKey
		}
		if _, ok := seen[k]; ok {
			return nil, errDuplicateLabelKey
		}
		seen[k] = struct{}{}
	}
	return keys, nil
}

// vecSeriesCacheKey validates vec arity and returns a collision-safe cache key.
func vecSeriesCacheKey(keys []string, values []string) (string, error) {
	if len(values) != len(keys) {
		return "", errVecLabelValueCount
	}
	if len(keys) == 0 {
		return "", nil
	}
	return packVecLabelValues(values), nil
}

// vecSeriesCompileLabelSet compiles a vec label-values tuple into a store-owned LabelSet.
// Callers should validate arity first via vecSeriesCacheKey.
func vecSeriesCompileLabelSet(backend meterBackend, keys []string, values []string) (LabelSet, error) {
	if len(values) != len(keys) {
		return LabelSet{}, errVecLabelValueCount
	}
	if len(keys) == 0 {
		return backend.compileLabelSet(), nil
	}

	labels := make([]Label, len(keys))
	for i, key := range keys {
		labels[i] = Label{Key: key, Value: values[i]}
	}
	return backend.compileLabelSet(labels...), nil
}

// packVecLabelValues builds a collision-safe cache key for an ordered values tuple.
func packVecLabelValues(values []string) string {
	if len(values) == 0 {
		return ""
	}

	var b strings.Builder
	for _, v := range values {
		b.WriteString(strconv.Itoa(len(v)))
		b.WriteByte(':')
		b.WriteString(v)
		b.WriteByte('\xff')
	}
	return b.String()
}

func scopedVecCacheKey(scopeKey, labelsKey string) string {
	if scopeKey == "" {
		return labelsKey
	}
	if labelsKey == "" {
		return "\xfe" + scopeKey
	}
	return "\xfe" + scopeKey + "\xff" + labelsKey
}
