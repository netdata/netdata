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

	makeHandle func(base []LabelSet, vecSet LabelSet) T
}

func newVecCache[T any](
	backend meterBackend,
	base []LabelSet,
	keys []string,
	makeHandle func(base []LabelSet, vecSet LabelSet) T,
) *vecCache[T] {
	return &vecCache[T]{
		backend:    backend,
		base:       base,
		keys:       keys,
		cache:      make(map[string]T),
		makeHandle: makeHandle,
	}
}

func (v *vecCache[T]) get(labelValues ...string) (T, error) {
	var zero T

	cacheKey, err := vecSeriesCacheKey(v.keys, labelValues)
	if err != nil {
		return zero, err
	}

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

	inst = v.makeHandle(v.base, vecSet)
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

// snapshotGaugeVec caches snapshot gauge series handles by vec label values.
type snapshotGaugeVec struct {
	cache *vecCache[*snapshotGaugeInstrument]
}

// statefulGaugeVec caches stateful gauge series handles by vec label values.
type statefulGaugeVec struct {
	cache *vecCache[*statefulGaugeInstrument]
}

// snapshotCounterVec caches snapshot counter series handles by vec label values.
type snapshotCounterVec struct {
	cache *vecCache[*snapshotCounterInstrument]
}

// statefulCounterVec caches stateful counter series handles by vec label values.
type statefulCounterVec struct {
	cache *vecCache[*statefulCounterInstrument]
}

// snapshotHistogramVec caches snapshot histogram series handles by vec label values.
type snapshotHistogramVec struct {
	cache *vecCache[*snapshotHistogramInstrument]
}

// statefulHistogramVec caches stateful histogram series handles by vec label values.
type statefulHistogramVec struct {
	cache *vecCache[*statefulHistogramInstrument]
}

// snapshotSummaryVec caches snapshot summary series handles by vec label values.
type snapshotSummaryVec struct {
	cache *vecCache[*snapshotSummaryInstrument]
}

// statefulSummaryVec caches stateful summary series handles by vec label values.
type statefulSummaryVec struct {
	cache *vecCache[*statefulSummaryInstrument]
}

// snapshotStateSetVec caches snapshot stateset series handles by vec label values.
type snapshotStateSetVec struct {
	cache *vecCache[*snapshotStateSetInstrument]
}

// statefulStateSetVec caches stateful stateset series handles by vec label values.
type statefulStateSetVec struct {
	cache *vecCache[*statefulStateSetInstrument]
}

// GaugeVec declares or reuses a snapshot gauge and exposes a label-values lookup API.
func (m *snapshotMeter) GaugeVec(name string, labelKeys []string, opts ...InstrumentOption) SnapshotGaugeVec {
	desc := mustRegisterInstrument(m.backend, metricName(m.prefix, name), kindGauge, modeSnapshot, opts...)
	keys := mustNormalizeVecLabelKeys(labelKeys)
	base := appendLabelSets(m.sets, nil)
	return &snapshotGaugeVec{
		cache: newVecCache(m.backend, base, keys, func(base []LabelSet, vecSet LabelSet) *snapshotGaugeInstrument {
			return &snapshotGaugeInstrument{
				backend: m.backend,
				desc:    desc,
				base:    appendVecSet(base, vecSet),
			}
		}),
	}
}

// GaugeVec declares or reuses a stateful gauge and exposes a label-values lookup API.
func (m *statefulMeter) GaugeVec(name string, labelKeys []string, opts ...InstrumentOption) StatefulGaugeVec {
	desc := mustRegisterInstrument(m.backend, metricName(m.prefix, name), kindGauge, modeStateful, opts...)
	keys := mustNormalizeVecLabelKeys(labelKeys)
	base := appendLabelSets(m.sets, nil)
	return &statefulGaugeVec{
		cache: newVecCache(m.backend, base, keys, func(base []LabelSet, vecSet LabelSet) *statefulGaugeInstrument {
			return &statefulGaugeInstrument{
				backend: m.backend,
				desc:    desc,
				base:    appendVecSet(base, vecSet),
			}
		}),
	}
}

// CounterVec declares or reuses a snapshot counter and exposes a label-values lookup API.
func (m *snapshotMeter) CounterVec(name string, labelKeys []string, opts ...InstrumentOption) SnapshotCounterVec {
	desc := mustRegisterInstrument(m.backend, metricName(m.prefix, name), kindCounter, modeSnapshot, opts...)
	keys := mustNormalizeVecLabelKeys(labelKeys)
	base := appendLabelSets(m.sets, nil)
	return &snapshotCounterVec{
		cache: newVecCache(m.backend, base, keys, func(base []LabelSet, vecSet LabelSet) *snapshotCounterInstrument {
			return &snapshotCounterInstrument{
				backend: m.backend,
				desc:    desc,
				base:    appendVecSet(base, vecSet),
			}
		}),
	}
}

// CounterVec declares or reuses a stateful counter and exposes a label-values lookup API.
func (m *statefulMeter) CounterVec(name string, labelKeys []string, opts ...InstrumentOption) StatefulCounterVec {
	desc := mustRegisterInstrument(m.backend, metricName(m.prefix, name), kindCounter, modeStateful, opts...)
	keys := mustNormalizeVecLabelKeys(labelKeys)
	base := appendLabelSets(m.sets, nil)
	return &statefulCounterVec{
		cache: newVecCache(m.backend, base, keys, func(base []LabelSet, vecSet LabelSet) *statefulCounterInstrument {
			return &statefulCounterInstrument{
				backend: m.backend,
				desc:    desc,
				base:    appendVecSet(base, vecSet),
			}
		}),
	}
}

// HistogramVec declares or reuses a snapshot histogram and exposes a label-values lookup API.
func (m *snapshotMeter) HistogramVec(name string, labelKeys []string, opts ...InstrumentOption) SnapshotHistogramVec {
	desc := mustRegisterInstrument(m.backend, metricName(m.prefix, name), kindHistogram, modeSnapshot, opts...)
	keys := mustNormalizeVecLabelKeys(labelKeys)
	base := appendLabelSets(m.sets, nil)
	return &snapshotHistogramVec{
		cache: newVecCache(m.backend, base, keys, func(base []LabelSet, vecSet LabelSet) *snapshotHistogramInstrument {
			return &snapshotHistogramInstrument{
				backend: m.backend,
				desc:    desc,
				base:    appendVecSet(base, vecSet),
			}
		}),
	}
}

// HistogramVec declares or reuses a stateful histogram and exposes a label-values lookup API.
func (m *statefulMeter) HistogramVec(name string, labelKeys []string, opts ...InstrumentOption) StatefulHistogramVec {
	desc := mustRegisterInstrument(m.backend, metricName(m.prefix, name), kindHistogram, modeStateful, opts...)
	keys := mustNormalizeVecLabelKeys(labelKeys)
	base := appendLabelSets(m.sets, nil)
	return &statefulHistogramVec{
		cache: newVecCache(m.backend, base, keys, func(base []LabelSet, vecSet LabelSet) *statefulHistogramInstrument {
			return &statefulHistogramInstrument{
				backend: m.backend,
				desc:    desc,
				base:    appendVecSet(base, vecSet),
			}
		}),
	}
}

// SummaryVec declares or reuses a snapshot summary and exposes a label-values lookup API.
func (m *snapshotMeter) SummaryVec(name string, labelKeys []string, opts ...InstrumentOption) SnapshotSummaryVec {
	desc := mustRegisterInstrument(m.backend, metricName(m.prefix, name), kindSummary, modeSnapshot, opts...)
	keys := mustNormalizeVecLabelKeys(labelKeys)
	base := appendLabelSets(m.sets, nil)
	return &snapshotSummaryVec{
		cache: newVecCache(m.backend, base, keys, func(base []LabelSet, vecSet LabelSet) *snapshotSummaryInstrument {
			return &snapshotSummaryInstrument{
				backend: m.backend,
				desc:    desc,
				base:    appendVecSet(base, vecSet),
			}
		}),
	}
}

// SummaryVec declares or reuses a stateful summary and exposes a label-values lookup API.
func (m *statefulMeter) SummaryVec(name string, labelKeys []string, opts ...InstrumentOption) StatefulSummaryVec {
	desc := mustRegisterInstrument(m.backend, metricName(m.prefix, name), kindSummary, modeStateful, opts...)
	keys := mustNormalizeVecLabelKeys(labelKeys)
	base := appendLabelSets(m.sets, nil)
	return &statefulSummaryVec{
		cache: newVecCache(m.backend, base, keys, func(base []LabelSet, vecSet LabelSet) *statefulSummaryInstrument {
			return &statefulSummaryInstrument{
				backend: m.backend,
				desc:    desc,
				base:    appendVecSet(base, vecSet),
			}
		}),
	}
}

// StateSetVec declares or reuses a snapshot stateset and exposes a label-values lookup API.
func (m *snapshotMeter) StateSetVec(name string, labelKeys []string, opts ...InstrumentOption) SnapshotStateSetVec {
	desc := mustRegisterInstrument(m.backend, metricName(m.prefix, name), kindStateSet, modeSnapshot, opts...)
	keys := mustNormalizeVecLabelKeys(labelKeys)
	base := appendLabelSets(m.sets, nil)
	return &snapshotStateSetVec{
		cache: newVecCache(m.backend, base, keys, func(base []LabelSet, vecSet LabelSet) *snapshotStateSetInstrument {
			return &snapshotStateSetInstrument{
				backend: m.backend,
				desc:    desc,
				base:    appendVecSet(base, vecSet),
			}
		}),
	}
}

// StateSetVec declares or reuses a stateful stateset and exposes a label-values lookup API.
func (m *statefulMeter) StateSetVec(name string, labelKeys []string, opts ...InstrumentOption) StatefulStateSetVec {
	desc := mustRegisterInstrument(m.backend, metricName(m.prefix, name), kindStateSet, modeStateful, opts...)
	keys := mustNormalizeVecLabelKeys(labelKeys)
	base := appendLabelSets(m.sets, nil)
	return &statefulStateSetVec{
		cache: newVecCache(m.backend, base, keys, func(base []LabelSet, vecSet LabelSet) *statefulStateSetInstrument {
			return &statefulStateSetInstrument{
				backend: m.backend,
				desc:    desc,
				base:    appendVecSet(base, vecSet),
			}
		}),
	}
}

// GetWithLabelValues returns a snapshot gauge handle for the provided vec label values.
func (v *snapshotGaugeVec) GetWithLabelValues(labelValues ...string) (SnapshotGauge, error) {
	inst, err := v.cache.get(labelValues...)
	if err != nil {
		return nil, err
	}
	return inst, nil
}

// WithLabelValues returns a snapshot gauge handle and panics on invalid label values.
func (v *snapshotGaugeVec) WithLabelValues(labelValues ...string) SnapshotGauge {
	inst, err := v.GetWithLabelValues(labelValues...)
	if err != nil {
		panic(err)
	}
	return inst
}

// GetWithLabelValues returns a stateful gauge handle for the provided vec label values.
func (v *statefulGaugeVec) GetWithLabelValues(labelValues ...string) (StatefulGauge, error) {
	inst, err := v.cache.get(labelValues...)
	if err != nil {
		return nil, err
	}
	return inst, nil
}

// WithLabelValues returns a stateful gauge handle and panics on invalid label values.
func (v *statefulGaugeVec) WithLabelValues(labelValues ...string) StatefulGauge {
	inst, err := v.GetWithLabelValues(labelValues...)
	if err != nil {
		panic(err)
	}
	return inst
}

// GetWithLabelValues returns a snapshot counter handle for the provided vec label values.
func (v *snapshotCounterVec) GetWithLabelValues(labelValues ...string) (SnapshotCounter, error) {
	inst, err := v.cache.get(labelValues...)
	if err != nil {
		return nil, err
	}
	return inst, nil
}

// WithLabelValues returns a snapshot counter handle and panics on invalid label values.
func (v *snapshotCounterVec) WithLabelValues(labelValues ...string) SnapshotCounter {
	inst, err := v.GetWithLabelValues(labelValues...)
	if err != nil {
		panic(err)
	}
	return inst
}

// GetWithLabelValues returns a stateful counter handle for the provided vec label values.
func (v *statefulCounterVec) GetWithLabelValues(labelValues ...string) (StatefulCounter, error) {
	inst, err := v.cache.get(labelValues...)
	if err != nil {
		return nil, err
	}
	return inst, nil
}

// WithLabelValues returns a stateful counter handle and panics on invalid label values.
func (v *statefulCounterVec) WithLabelValues(labelValues ...string) StatefulCounter {
	inst, err := v.GetWithLabelValues(labelValues...)
	if err != nil {
		panic(err)
	}
	return inst
}

// GetWithLabelValues returns a snapshot histogram handle for the provided vec label values.
func (v *snapshotHistogramVec) GetWithLabelValues(labelValues ...string) (SnapshotHistogram, error) {
	inst, err := v.cache.get(labelValues...)
	if err != nil {
		return nil, err
	}
	return inst, nil
}

// WithLabelValues returns a snapshot histogram handle and panics on invalid label values.
func (v *snapshotHistogramVec) WithLabelValues(labelValues ...string) SnapshotHistogram {
	inst, err := v.GetWithLabelValues(labelValues...)
	if err != nil {
		panic(err)
	}
	return inst
}

// GetWithLabelValues returns a stateful histogram handle for the provided vec label values.
func (v *statefulHistogramVec) GetWithLabelValues(labelValues ...string) (StatefulHistogram, error) {
	inst, err := v.cache.get(labelValues...)
	if err != nil {
		return nil, err
	}
	return inst, nil
}

// WithLabelValues returns a stateful histogram handle and panics on invalid label values.
func (v *statefulHistogramVec) WithLabelValues(labelValues ...string) StatefulHistogram {
	inst, err := v.GetWithLabelValues(labelValues...)
	if err != nil {
		panic(err)
	}
	return inst
}

// GetWithLabelValues returns a snapshot summary handle for the provided vec label values.
func (v *snapshotSummaryVec) GetWithLabelValues(labelValues ...string) (SnapshotSummary, error) {
	inst, err := v.cache.get(labelValues...)
	if err != nil {
		return nil, err
	}
	return inst, nil
}

// WithLabelValues returns a snapshot summary handle and panics on invalid label values.
func (v *snapshotSummaryVec) WithLabelValues(labelValues ...string) SnapshotSummary {
	inst, err := v.GetWithLabelValues(labelValues...)
	if err != nil {
		panic(err)
	}
	return inst
}

// GetWithLabelValues returns a stateful summary handle for the provided vec label values.
func (v *statefulSummaryVec) GetWithLabelValues(labelValues ...string) (StatefulSummary, error) {
	inst, err := v.cache.get(labelValues...)
	if err != nil {
		return nil, err
	}
	return inst, nil
}

// WithLabelValues returns a stateful summary handle and panics on invalid label values.
func (v *statefulSummaryVec) WithLabelValues(labelValues ...string) StatefulSummary {
	inst, err := v.GetWithLabelValues(labelValues...)
	if err != nil {
		panic(err)
	}
	return inst
}

// GetWithLabelValues returns a snapshot stateset handle for the provided vec label values.
func (v *snapshotStateSetVec) GetWithLabelValues(labelValues ...string) (StateSetInstrument, error) {
	inst, err := v.cache.get(labelValues...)
	if err != nil {
		return nil, err
	}
	return inst, nil
}

// WithLabelValues returns a snapshot stateset handle and panics on invalid label values.
func (v *snapshotStateSetVec) WithLabelValues(labelValues ...string) StateSetInstrument {
	inst, err := v.GetWithLabelValues(labelValues...)
	if err != nil {
		panic(err)
	}
	return inst
}

// GetWithLabelValues returns a stateful stateset handle for the provided vec label values.
func (v *statefulStateSetVec) GetWithLabelValues(labelValues ...string) (StateSetInstrument, error) {
	inst, err := v.cache.get(labelValues...)
	if err != nil {
		return nil, err
	}
	return inst, nil
}

// WithLabelValues returns a stateful stateset handle and panics on invalid label values.
func (v *statefulStateSetVec) WithLabelValues(labelValues ...string) StateSetInstrument {
	inst, err := v.GetWithLabelValues(labelValues...)
	if err != nil {
		panic(err)
	}
	return inst
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
