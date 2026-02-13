// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"sort"
	"time"
)

type runtimeStoreView struct {
	core    *storeCore
	backend *runtimeStoreBackend
}

type runtimeStoreBackend struct {
	core            *storeCore
	summarySketches map[string]*summaryQuantileSketch
	retention       runtimeRetentionPolicy
	now             func() time.Time
}

type runtimeWriteView struct {
	backend *runtimeStoreBackend
}

type runtimeRetentionPolicy struct {
	ttl       time.Duration
	maxSeries int
}

const (
	defaultRuntimeRetentionTTL       = 30 * time.Minute
	defaultRuntimeRetentionMaxSeries = 0 // disabled
)

// NewRuntimeStore creates a dedicated runtime/internal metrics store with
// stateful-only, immediate-commit write semantics.
func NewRuntimeStore() RuntimeStore {
	core := &storeCore{
		instruments: make(map[string]*instrumentDescriptor),
	}
	core.snapshot.Store(&readSnapshot{
		collectMeta: CollectMeta{LastAttemptStatus: CollectStatusUnknown},
		series:      make(map[string]*committedSeries),
	})

	backend := &runtimeStoreBackend{core: core}
	backend.summarySketches = make(map[string]*summaryQuantileSketch)
	backend.retention = runtimeRetentionPolicy{
		ttl:       defaultRuntimeRetentionTTL,
		maxSeries: defaultRuntimeRetentionMaxSeries,
	}
	backend.now = time.Now
	return &runtimeStoreView{
		core:    core,
		backend: backend,
	}
}

func (s *runtimeStoreView) Read() Reader {
	return &storeReader{snap: s.core.snapshot.Load(), raw: false}
}

func (s *runtimeStoreView) ReadRaw() Reader {
	return &storeReader{snap: s.core.snapshot.Load(), raw: true}
}

func (s *runtimeStoreView) Write() RuntimeWriter {
	return &runtimeWriteView{backend: s.backend}
}

func (w *runtimeWriteView) StatefulMeter(prefix string) StatefulMeter {
	return &statefulMeter{backend: w.backend, prefix: prefix}
}

func (r *runtimeStoreBackend) compileLabelSet(labels ...Label) LabelSet {
	return compileLabelSetForOwner(r, labels...)
}

func (r *runtimeStoreBackend) registerInstrument(name string, kind metricKind, mode metricMode, opts ...InstrumentOption) (*instrumentDescriptor, error) {
	if mode != modeStateful {
		return nil, errRuntimeSnapshotWrite
	}

	cfg := instrumentConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt.apply(&cfg)
		}
	}
	if cfg.freshnessSet && cfg.freshness != FreshnessCommitted {
		return nil, errRuntimeFreshness
	}
	if cfg.windowSet && cfg.window == WindowCycle {
		return nil, errRuntimeWindowCycle
	}

	desc, err := r.core.registerInstrument(name, kind, modeStateful, opts...)
	if err != nil {
		return nil, err
	}
	if desc.freshness != FreshnessCommitted {
		return nil, errRuntimeFreshness
	}
	return desc, nil
}

func (r *runtimeStoreBackend) recordGaugeSet(desc *instrumentDescriptor, value SampleValue, sets []LabelSet) {
	labels, labelsKey, err := labelsFromSet(sets, r)
	if err != nil {
		panic(err)
	}
	key := makeSeriesKey(desc.name, labelsKey)
	r.commitRuntimeWrite(func(old, next *readSnapshot, seq uint64, nowUnixNano int64) {
		series := runtimeEnsureSeriesMutable(old, next, key, desc.name, labels, labelsKey, desc)
		series.value = value
		series.meta.LastSeenSuccessSeq = seq
		series.runtimeLastSeenUnixNano = nowUnixNano
	})
}

func (r *runtimeStoreBackend) recordGaugeAdd(desc *instrumentDescriptor, delta SampleValue, sets []LabelSet) {
	labels, labelsKey, err := labelsFromSet(sets, r)
	if err != nil {
		panic(err)
	}
	key := makeSeriesKey(desc.name, labelsKey)
	r.commitRuntimeWrite(func(old, next *readSnapshot, seq uint64, nowUnixNano int64) {
		series := runtimeEnsureSeriesMutable(old, next, key, desc.name, labels, labelsKey, desc)
		series.value += delta
		series.meta.LastSeenSuccessSeq = seq
		series.runtimeLastSeenUnixNano = nowUnixNano
	})
}

func (r *runtimeStoreBackend) recordCounterObserveTotal(_ *instrumentDescriptor, _ SampleValue, _ []LabelSet) {
	panic(errRuntimeSnapshotWrite)
}

func (r *runtimeStoreBackend) recordCounterAdd(desc *instrumentDescriptor, delta SampleValue, sets []LabelSet) {
	if delta < 0 {
		panic(errCounterNegativeDelta)
	}

	labels, labelsKey, err := labelsFromSet(sets, r)
	if err != nil {
		panic(err)
	}
	key := makeSeriesKey(desc.name, labelsKey)
	r.commitRuntimeWrite(func(old, next *readSnapshot, seq uint64, nowUnixNano int64) {
		series := runtimeEnsureSeriesMutable(old, next, key, desc.name, labels, labelsKey, desc)

		hadCurrent := series.desc != nil && series.desc.kind == kindCounter && series.counterCurrentAttemptSeq > 0
		if hadCurrent {
			series.counterPrevious = series.counterCurrent
			series.counterPreviousAttemptSeq = series.counterCurrentAttemptSeq
			series.counterHasPrev = true
		} else {
			series.counterPrevious = 0
			series.counterPreviousAttemptSeq = 0
			series.counterHasPrev = false
		}

		series.counterCurrent += delta
		series.counterCurrentAttemptSeq = seq
		series.value = series.counterCurrent
		series.meta.LastSeenSuccessSeq = seq
		series.runtimeLastSeenUnixNano = nowUnixNano
	})
}

func (r *runtimeStoreBackend) recordHistogramObservePoint(_ *instrumentDescriptor, _ HistogramPoint, _ []LabelSet) {
	panic(errRuntimeSnapshotWrite)
}

func (r *runtimeStoreBackend) recordHistogramObserve(desc *instrumentDescriptor, value SampleValue, sets []LabelSet) {
	schema := desc.histogram
	if schema == nil || len(schema.bounds) == 0 {
		panic(errHistogramBounds)
	}

	labels, labelsKey, err := labelsFromSet(sets, r)
	if err != nil {
		panic(err)
	}
	if labelsContainKey(labels, histogramBucketLabel) {
		panic(errHistogramLabelKey)
	}

	key := makeSeriesKey(desc.name, labelsKey)
	r.commitRuntimeWrite(func(old, next *readSnapshot, seq uint64, nowUnixNano int64) {
		series := runtimeEnsureSeriesMutable(old, next, key, desc.name, labels, labelsKey, desc)

		if series.desc.histogram == nil || !equalHistogramBounds(series.desc.histogram.bounds, schema.bounds) {
			panic("metrix: histogram schema drift detected")
		}
		if len(series.histogramCumulative) == 0 {
			series.histogramCumulative = make([]SampleValue, len(schema.bounds))
		}

		idx := findHistogramBucket(schema.bounds, value)
		if idx < len(series.histogramCumulative) {
			for i := idx; i < len(series.histogramCumulative); i++ {
				series.histogramCumulative[i]++
			}
		}
		series.histogramCount++
		series.histogramSum += value
		series.meta.LastSeenSuccessSeq = seq
		series.runtimeLastSeenUnixNano = nowUnixNano
	})
}

func (r *runtimeStoreBackend) recordSummaryObservePoint(_ *instrumentDescriptor, _ SummaryPoint, _ []LabelSet) {
	panic(errRuntimeSnapshotWrite)
}

func (r *runtimeStoreBackend) recordSummaryObserve(desc *instrumentDescriptor, value SampleValue, sets []LabelSet) {
	labels, labelsKey, err := labelsFromSet(sets, r)
	if err != nil {
		panic(err)
	}
	if labelsContainKey(labels, summaryQuantileLabel) {
		panic(errSummaryLabelKey)
	}

	key := makeSeriesKey(desc.name, labelsKey)
	r.commitRuntimeWrite(func(old, next *readSnapshot, seq uint64, nowUnixNano int64) {
		series := runtimeEnsureSeriesMutable(old, next, key, desc.name, labels, labelsKey, desc)

		series.summaryCount++
		series.summarySum += value

		qs := desc.summaryQuantiles()
		if len(qs) > 0 {
			sketch := r.summarySketches[key]
			if sketch == nil {
				sketch = newSummaryQuantileSketch(desc.summaryReservoirSize(), summarySketchSeed(key))
				r.summarySketches[key] = sketch
			}
			sketch.observe(value)
			series.summaryQuantiles = sketch.quantiles(qs)
		} else {
			delete(r.summarySketches, key)
			series.summaryQuantiles = nil
		}
		series.meta.LastSeenSuccessSeq = seq
		series.runtimeLastSeenUnixNano = nowUnixNano
	})
}

func (r *runtimeStoreBackend) recordStateSetObserve(desc *instrumentDescriptor, point StateSetPoint, sets []LabelSet) {
	schema := desc.stateSet
	if schema == nil {
		panic(errStateSetSchema)
	}

	labels, labelsKey, err := labelsFromSet(sets, r)
	if err != nil {
		panic(err)
	}
	if labelsContainKey(labels, desc.name) {
		panic(errStateSetLabelKey)
	}
	states := normalizeStateSetPoint(point, schema)

	key := makeSeriesKey(desc.name, labelsKey)
	r.commitRuntimeWrite(func(old, next *readSnapshot, seq uint64, nowUnixNano int64) {
		series := runtimeEnsureSeriesMutable(old, next, key, desc.name, labels, labelsKey, desc)
		series.stateSetValues = cloneStateMap(states)
		series.meta.LastSeenSuccessSeq = seq
		series.runtimeLastSeenUnixNano = nowUnixNano
	})
}

func (r *runtimeStoreBackend) commitRuntimeWrite(apply func(old, next *readSnapshot, seq uint64, nowUnixNano int64)) {
	r.core.mu.Lock()
	defer r.core.mu.Unlock()

	oldSnap := r.core.snapshot.Load()
	next := &readSnapshot{
		collectMeta: oldSnap.collectMeta,
		series:      make(map[string]*committedSeries, len(oldSnap.series)),
		// byName index is built lazily by readers for runtime snapshots.
		byName: nil,
	}
	for k, s := range oldSnap.series {
		next.series[k] = s
	}

	nowUnixNano := r.now().UnixNano()
	r.core.sequence++
	seq := r.core.sequence
	apply(oldSnap, next, seq, nowUnixNano)
	evicted := applyRuntimeRetention(next.series, r.retention, nowUnixNano)
	for _, key := range evicted {
		delete(r.summarySketches, key)
	}

	next.collectMeta.LastAttemptSeq = seq
	next.collectMeta.LastAttemptStatus = CollectStatusSuccess
	next.collectMeta.LastSuccessSeq = seq
	r.core.snapshot.Store(next)
}

func runtimeEnsureSeriesMutable(old, next *readSnapshot, key, name string, labels []Label, labelsKey string, desc *instrumentDescriptor) *committedSeries {
	series := next.series[key]
	if series != nil {
		if oldSeries, ok := old.series[key]; ok && oldSeries == series {
			series = cloneCommittedSeries(series)
			next.series[key] = series
		}
		return series
	}
	series = &committedSeries{
		id:        SeriesID(key),
		key:       key,
		name:      name,
		labels:    append([]Label(nil), labels...),
		labelsKey: labelsKey,
		desc:      desc,
	}
	next.series[key] = series
	return series
}

func applyRuntimeRetention(series map[string]*committedSeries, policy runtimeRetentionPolicy, nowUnixNano int64) []string {
	evicted := make([]string, 0)

	if policy.ttl > 0 {
		cutoff := nowUnixNano - int64(policy.ttl)
		for key, s := range series {
			if s.runtimeLastSeenUnixNano <= cutoff {
				delete(series, key)
				evicted = append(evicted, key)
			}
		}
	}

	if policy.maxSeries > 0 && len(series) > policy.maxSeries {
		type candidate struct {
			key      string
			lastSeen int64
		}
		candidates := make([]candidate, 0, len(series))
		for key, s := range series {
			candidates = append(candidates, candidate{
				key:      key,
				lastSeen: s.runtimeLastSeenUnixNano,
			})
		}
		sort.Slice(candidates, func(i, j int) bool {
			if candidates[i].lastSeen != candidates[j].lastSeen {
				return candidates[i].lastSeen < candidates[j].lastSeen
			}
			return candidates[i].key < candidates[j].key
		})

		evictCount := len(series) - policy.maxSeries
		for i := 0; i < evictCount; i++ {
			key := candidates[i].key
			delete(series, key)
			evicted = append(evicted, key)
		}
	}

	return evicted
}

var _ RuntimeStore = (*runtimeStoreView)(nil)
var _ RuntimeWriter = (*runtimeWriteView)(nil)
var _ meterBackend = (*runtimeStoreBackend)(nil)
