// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"strconv"
	"strings"
	"sync"
)

// snapshotGaugeVec caches snapshot gauge series handles by vec label values.
type snapshotGaugeVec struct {
	backend meterBackend
	desc    *instrumentDescriptor
	base    []LabelSet
	keys    []string

	mu    sync.RWMutex
	cache map[string]*snapshotGaugeInstrument
}

// statefulGaugeVec caches stateful gauge series handles by vec label values.
type statefulGaugeVec struct {
	backend meterBackend
	desc    *instrumentDescriptor
	base    []LabelSet
	keys    []string

	mu    sync.RWMutex
	cache map[string]*statefulGaugeInstrument
}

// snapshotCounterVec caches snapshot counter series handles by vec label values.
type snapshotCounterVec struct {
	backend meterBackend
	desc    *instrumentDescriptor
	base    []LabelSet
	keys    []string

	mu    sync.RWMutex
	cache map[string]*snapshotCounterInstrument
}

// statefulCounterVec caches stateful counter series handles by vec label values.
type statefulCounterVec struct {
	backend meterBackend
	desc    *instrumentDescriptor
	base    []LabelSet
	keys    []string

	mu    sync.RWMutex
	cache map[string]*statefulCounterInstrument
}

// snapshotHistogramVec caches snapshot histogram series handles by vec label values.
type snapshotHistogramVec struct {
	backend meterBackend
	desc    *instrumentDescriptor
	base    []LabelSet
	keys    []string

	mu    sync.RWMutex
	cache map[string]*snapshotHistogramInstrument
}

// statefulHistogramVec caches stateful histogram series handles by vec label values.
type statefulHistogramVec struct {
	backend meterBackend
	desc    *instrumentDescriptor
	base    []LabelSet
	keys    []string

	mu    sync.RWMutex
	cache map[string]*statefulHistogramInstrument
}

// snapshotSummaryVec caches snapshot summary series handles by vec label values.
type snapshotSummaryVec struct {
	backend meterBackend
	desc    *instrumentDescriptor
	base    []LabelSet
	keys    []string

	mu    sync.RWMutex
	cache map[string]*snapshotSummaryInstrument
}

// statefulSummaryVec caches stateful summary series handles by vec label values.
type statefulSummaryVec struct {
	backend meterBackend
	desc    *instrumentDescriptor
	base    []LabelSet
	keys    []string

	mu    sync.RWMutex
	cache map[string]*statefulSummaryInstrument
}

// snapshotStateSetVec caches snapshot stateset series handles by vec label values.
type snapshotStateSetVec struct {
	backend meterBackend
	desc    *instrumentDescriptor
	base    []LabelSet
	keys    []string

	mu    sync.RWMutex
	cache map[string]*snapshotStateSetInstrument
}

// statefulStateSetVec caches stateful stateset series handles by vec label values.
type statefulStateSetVec struct {
	backend meterBackend
	desc    *instrumentDescriptor
	base    []LabelSet
	keys    []string

	mu    sync.RWMutex
	cache map[string]*statefulStateSetInstrument
}

// GaugeVec declares or reuses a snapshot gauge and exposes a label-values lookup API.
func (m *snapshotMeter) GaugeVec(name string, labelKeys []string, opts ...InstrumentOption) SnapshotGaugeVec {
	desc, err := m.backend.registerInstrument(metricName(m.prefix, name), kindGauge, modeSnapshot, opts...)
	if err != nil {
		panic(err)
	}
	keys, err := normalizeVecLabelKeys(labelKeys)
	if err != nil {
		panic(err)
	}
	return &snapshotGaugeVec{
		backend: m.backend,
		desc:    desc,
		base:    appendLabelSets(m.sets, nil),
		keys:    keys,
		cache:   make(map[string]*snapshotGaugeInstrument),
	}
}

// GaugeVec declares or reuses a stateful gauge and exposes a label-values lookup API.
func (m *statefulMeter) GaugeVec(name string, labelKeys []string, opts ...InstrumentOption) StatefulGaugeVec {
	desc, err := m.backend.registerInstrument(metricName(m.prefix, name), kindGauge, modeStateful, opts...)
	if err != nil {
		panic(err)
	}
	keys, err := normalizeVecLabelKeys(labelKeys)
	if err != nil {
		panic(err)
	}
	return &statefulGaugeVec{
		backend: m.backend,
		desc:    desc,
		base:    appendLabelSets(m.sets, nil),
		keys:    keys,
		cache:   make(map[string]*statefulGaugeInstrument),
	}
}

// CounterVec declares or reuses a snapshot counter and exposes a label-values lookup API.
func (m *snapshotMeter) CounterVec(name string, labelKeys []string, opts ...InstrumentOption) SnapshotCounterVec {
	desc, err := m.backend.registerInstrument(metricName(m.prefix, name), kindCounter, modeSnapshot, opts...)
	if err != nil {
		panic(err)
	}
	keys, err := normalizeVecLabelKeys(labelKeys)
	if err != nil {
		panic(err)
	}
	return &snapshotCounterVec{
		backend: m.backend,
		desc:    desc,
		base:    appendLabelSets(m.sets, nil),
		keys:    keys,
		cache:   make(map[string]*snapshotCounterInstrument),
	}
}

// CounterVec declares or reuses a stateful counter and exposes a label-values lookup API.
func (m *statefulMeter) CounterVec(name string, labelKeys []string, opts ...InstrumentOption) StatefulCounterVec {
	desc, err := m.backend.registerInstrument(metricName(m.prefix, name), kindCounter, modeStateful, opts...)
	if err != nil {
		panic(err)
	}
	keys, err := normalizeVecLabelKeys(labelKeys)
	if err != nil {
		panic(err)
	}
	return &statefulCounterVec{
		backend: m.backend,
		desc:    desc,
		base:    appendLabelSets(m.sets, nil),
		keys:    keys,
		cache:   make(map[string]*statefulCounterInstrument),
	}
}

// HistogramVec declares or reuses a snapshot histogram and exposes a label-values lookup API.
func (m *snapshotMeter) HistogramVec(name string, labelKeys []string, opts ...InstrumentOption) SnapshotHistogramVec {
	desc, err := m.backend.registerInstrument(metricName(m.prefix, name), kindHistogram, modeSnapshot, opts...)
	if err != nil {
		panic(err)
	}
	keys, err := normalizeVecLabelKeys(labelKeys)
	if err != nil {
		panic(err)
	}
	return &snapshotHistogramVec{
		backend: m.backend,
		desc:    desc,
		base:    appendLabelSets(m.sets, nil),
		keys:    keys,
		cache:   make(map[string]*snapshotHistogramInstrument),
	}
}

// HistogramVec declares or reuses a stateful histogram and exposes a label-values lookup API.
func (m *statefulMeter) HistogramVec(name string, labelKeys []string, opts ...InstrumentOption) StatefulHistogramVec {
	desc, err := m.backend.registerInstrument(metricName(m.prefix, name), kindHistogram, modeStateful, opts...)
	if err != nil {
		panic(err)
	}
	keys, err := normalizeVecLabelKeys(labelKeys)
	if err != nil {
		panic(err)
	}
	return &statefulHistogramVec{
		backend: m.backend,
		desc:    desc,
		base:    appendLabelSets(m.sets, nil),
		keys:    keys,
		cache:   make(map[string]*statefulHistogramInstrument),
	}
}

// SummaryVec declares or reuses a snapshot summary and exposes a label-values lookup API.
func (m *snapshotMeter) SummaryVec(name string, labelKeys []string, opts ...InstrumentOption) SnapshotSummaryVec {
	desc, err := m.backend.registerInstrument(metricName(m.prefix, name), kindSummary, modeSnapshot, opts...)
	if err != nil {
		panic(err)
	}
	keys, err := normalizeVecLabelKeys(labelKeys)
	if err != nil {
		panic(err)
	}
	return &snapshotSummaryVec{
		backend: m.backend,
		desc:    desc,
		base:    appendLabelSets(m.sets, nil),
		keys:    keys,
		cache:   make(map[string]*snapshotSummaryInstrument),
	}
}

// SummaryVec declares or reuses a stateful summary and exposes a label-values lookup API.
func (m *statefulMeter) SummaryVec(name string, labelKeys []string, opts ...InstrumentOption) StatefulSummaryVec {
	desc, err := m.backend.registerInstrument(metricName(m.prefix, name), kindSummary, modeStateful, opts...)
	if err != nil {
		panic(err)
	}
	keys, err := normalizeVecLabelKeys(labelKeys)
	if err != nil {
		panic(err)
	}
	return &statefulSummaryVec{
		backend: m.backend,
		desc:    desc,
		base:    appendLabelSets(m.sets, nil),
		keys:    keys,
		cache:   make(map[string]*statefulSummaryInstrument),
	}
}

// StateSetVec declares or reuses a snapshot stateset and exposes a label-values lookup API.
func (m *snapshotMeter) StateSetVec(name string, labelKeys []string, opts ...InstrumentOption) SnapshotStateSetVec {
	desc, err := m.backend.registerInstrument(metricName(m.prefix, name), kindStateSet, modeSnapshot, opts...)
	if err != nil {
		panic(err)
	}
	keys, err := normalizeVecLabelKeys(labelKeys)
	if err != nil {
		panic(err)
	}
	return &snapshotStateSetVec{
		backend: m.backend,
		desc:    desc,
		base:    appendLabelSets(m.sets, nil),
		keys:    keys,
		cache:   make(map[string]*snapshotStateSetInstrument),
	}
}

// StateSetVec declares or reuses a stateful stateset and exposes a label-values lookup API.
func (m *statefulMeter) StateSetVec(name string, labelKeys []string, opts ...InstrumentOption) StatefulStateSetVec {
	desc, err := m.backend.registerInstrument(metricName(m.prefix, name), kindStateSet, modeStateful, opts...)
	if err != nil {
		panic(err)
	}
	keys, err := normalizeVecLabelKeys(labelKeys)
	if err != nil {
		panic(err)
	}
	return &statefulStateSetVec{
		backend: m.backend,
		desc:    desc,
		base:    appendLabelSets(m.sets, nil),
		keys:    keys,
		cache:   make(map[string]*statefulStateSetInstrument),
	}
}

// GetWithLabelValues returns a snapshot gauge handle for the provided vec label values.
func (v *snapshotGaugeVec) GetWithLabelValues(labelValues ...string) (SnapshotGauge, error) {
	cacheKey, vecSet, err := vecSeriesLabelSet(v.backend, v.keys, labelValues)
	if err != nil {
		return nil, err
	}

	v.mu.RLock()
	inst := v.cache[cacheKey]
	v.mu.RUnlock()
	if inst != nil {
		return inst, nil
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	if inst = v.cache[cacheKey]; inst != nil {
		return inst, nil
	}
	inst = &snapshotGaugeInstrument{
		backend: v.backend,
		desc:    v.desc,
		base:    appendLabelSets(v.base, []LabelSet{vecSet}),
	}
	v.cache[cacheKey] = inst
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
	cacheKey, vecSet, err := vecSeriesLabelSet(v.backend, v.keys, labelValues)
	if err != nil {
		return nil, err
	}

	v.mu.RLock()
	inst := v.cache[cacheKey]
	v.mu.RUnlock()
	if inst != nil {
		return inst, nil
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	if inst = v.cache[cacheKey]; inst != nil {
		return inst, nil
	}
	inst = &statefulGaugeInstrument{
		backend: v.backend,
		desc:    v.desc,
		base:    appendLabelSets(v.base, []LabelSet{vecSet}),
	}
	v.cache[cacheKey] = inst
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
	cacheKey, vecSet, err := vecSeriesLabelSet(v.backend, v.keys, labelValues)
	if err != nil {
		return nil, err
	}

	v.mu.RLock()
	inst := v.cache[cacheKey]
	v.mu.RUnlock()
	if inst != nil {
		return inst, nil
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	if inst = v.cache[cacheKey]; inst != nil {
		return inst, nil
	}
	inst = &snapshotCounterInstrument{
		backend: v.backend,
		desc:    v.desc,
		base:    appendLabelSets(v.base, []LabelSet{vecSet}),
	}
	v.cache[cacheKey] = inst
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
	cacheKey, vecSet, err := vecSeriesLabelSet(v.backend, v.keys, labelValues)
	if err != nil {
		return nil, err
	}

	v.mu.RLock()
	inst := v.cache[cacheKey]
	v.mu.RUnlock()
	if inst != nil {
		return inst, nil
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	if inst = v.cache[cacheKey]; inst != nil {
		return inst, nil
	}
	inst = &statefulCounterInstrument{
		backend: v.backend,
		desc:    v.desc,
		base:    appendLabelSets(v.base, []LabelSet{vecSet}),
	}
	v.cache[cacheKey] = inst
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
	cacheKey, vecSet, err := vecSeriesLabelSet(v.backend, v.keys, labelValues)
	if err != nil {
		return nil, err
	}

	v.mu.RLock()
	inst := v.cache[cacheKey]
	v.mu.RUnlock()
	if inst != nil {
		return inst, nil
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	if inst = v.cache[cacheKey]; inst != nil {
		return inst, nil
	}
	inst = &snapshotHistogramInstrument{
		backend: v.backend,
		desc:    v.desc,
		base:    appendLabelSets(v.base, []LabelSet{vecSet}),
	}
	v.cache[cacheKey] = inst
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
	cacheKey, vecSet, err := vecSeriesLabelSet(v.backend, v.keys, labelValues)
	if err != nil {
		return nil, err
	}

	v.mu.RLock()
	inst := v.cache[cacheKey]
	v.mu.RUnlock()
	if inst != nil {
		return inst, nil
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	if inst = v.cache[cacheKey]; inst != nil {
		return inst, nil
	}
	inst = &statefulHistogramInstrument{
		backend: v.backend,
		desc:    v.desc,
		base:    appendLabelSets(v.base, []LabelSet{vecSet}),
	}
	v.cache[cacheKey] = inst
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
	cacheKey, vecSet, err := vecSeriesLabelSet(v.backend, v.keys, labelValues)
	if err != nil {
		return nil, err
	}

	v.mu.RLock()
	inst := v.cache[cacheKey]
	v.mu.RUnlock()
	if inst != nil {
		return inst, nil
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	if inst = v.cache[cacheKey]; inst != nil {
		return inst, nil
	}
	inst = &snapshotSummaryInstrument{
		backend: v.backend,
		desc:    v.desc,
		base:    appendLabelSets(v.base, []LabelSet{vecSet}),
	}
	v.cache[cacheKey] = inst
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
	cacheKey, vecSet, err := vecSeriesLabelSet(v.backend, v.keys, labelValues)
	if err != nil {
		return nil, err
	}

	v.mu.RLock()
	inst := v.cache[cacheKey]
	v.mu.RUnlock()
	if inst != nil {
		return inst, nil
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	if inst = v.cache[cacheKey]; inst != nil {
		return inst, nil
	}
	inst = &statefulSummaryInstrument{
		backend: v.backend,
		desc:    v.desc,
		base:    appendLabelSets(v.base, []LabelSet{vecSet}),
	}
	v.cache[cacheKey] = inst
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
	cacheKey, vecSet, err := vecSeriesLabelSet(v.backend, v.keys, labelValues)
	if err != nil {
		return nil, err
	}

	v.mu.RLock()
	inst := v.cache[cacheKey]
	v.mu.RUnlock()
	if inst != nil {
		return inst, nil
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	if inst = v.cache[cacheKey]; inst != nil {
		return inst, nil
	}
	inst = &snapshotStateSetInstrument{
		backend: v.backend,
		desc:    v.desc,
		base:    appendLabelSets(v.base, []LabelSet{vecSet}),
	}
	v.cache[cacheKey] = inst
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
	cacheKey, vecSet, err := vecSeriesLabelSet(v.backend, v.keys, labelValues)
	if err != nil {
		return nil, err
	}

	v.mu.RLock()
	inst := v.cache[cacheKey]
	v.mu.RUnlock()
	if inst != nil {
		return inst, nil
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	if inst = v.cache[cacheKey]; inst != nil {
		return inst, nil
	}
	inst = &statefulStateSetInstrument{
		backend: v.backend,
		desc:    v.desc,
		base:    appendLabelSets(v.base, []LabelSet{vecSet}),
	}
	v.cache[cacheKey] = inst
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

// vecSeriesLabelSet compiles a vec label-values tuple into a store-owned LabelSet.
func vecSeriesLabelSet(backend meterBackend, keys []string, values []string) (string, LabelSet, error) {
	if len(values) != len(keys) {
		return "", LabelSet{}, errVecLabelValueCount
	}
	if len(keys) == 0 {
		return "", backend.compileLabelSet(), nil
	}

	labels := make([]Label, len(keys))
	for i, key := range keys {
		labels[i] = Label{Key: key, Value: values[i]}
	}
	return packVecLabelValues(values), backend.compileLabelSet(labels...), nil
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
