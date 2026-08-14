// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"maps"
	"slices"
	"sync"
)

type storeReader struct {
	snap         *readSnapshot
	raw          bool // true => ReadRaw semantics (no freshness filtering)
	flattened    bool
	hostScopeKey string
	seriesOnce   sync.Once
	series       map[string]*committedSeries
	indexOnce    sync.Once
	index        *snapshotSeriesIndex
}

type collectorReader struct {
	storeReader
}

var emptySnapshotScopeIndex snapshotScopeIndex

type familyView struct {
	name   string
	reader *storeReader
}

// FlattenedRead reports whether this reader was created with ReadFlatten().
func (r *storeReader) FlattenedRead() bool {
	return r.flattened
}

func (r *storeReader) Value(name string, labels Labels) (SampleValue, bool) {
	s, ok := r.lookup(name, labels)
	if !ok || !r.visible(s) {
		return 0, false
	}
	if s.desc == nil || !isScalarKind(s.desc.kind) {
		return 0, false
	}
	return s.value, true
}

func (r *storeReader) Delta(name string, labels Labels) (SampleValue, bool) {
	s, ok := r.lookup(name, labels)
	if !ok || !r.visible(s) {
		return 0, false
	}
	if s.desc == nil || s.desc.kind != kindCounter {
		return 0, false
	}
	if !s.counterHasPrev || s.counterCurrentSeq != s.counterPreviousSeq+1 {
		return 0, false
	}
	if s.counterCurrent < s.counterPrevious {
		if s.counterNoResetFallback {
			return 0, false
		}
		return s.counterCurrent, true
	}
	return s.counterCurrent - s.counterPrevious, true
}

func (r *storeReader) Histogram(name string, labels Labels) (HistogramPoint, bool) {
	if r.flattened {
		return HistogramPoint{}, false
	}

	s, ok := r.lookup(name, labels)
	if !ok || !r.visible(s) {
		return HistogramPoint{}, false
	}
	if s.desc == nil || s.desc.kind != kindHistogram || s.desc.histogram == nil {
		return HistogramPoint{}, false
	}
	if len(s.histogramCumulative) != len(s.desc.histogram.bounds) {
		return HistogramPoint{}, false
	}

	point := HistogramPoint{
		Count:   s.histogramCount,
		Sum:     s.histogramSum,
		Buckets: make([]BucketPoint, len(s.desc.histogram.bounds)),
	}
	for i, ub := range s.desc.histogram.bounds {
		point.Buckets[i] = BucketPoint{
			UpperBound:      ub,
			CumulativeCount: s.histogramCumulative[i],
		}
	}
	return point, true
}

func (r *storeReader) Summary(name string, labels Labels) (SummaryPoint, bool) {
	if r.flattened {
		return SummaryPoint{}, false
	}

	s, ok := r.lookup(name, labels)
	if !ok || !r.visible(s) {
		return SummaryPoint{}, false
	}
	if s.desc == nil || s.desc.kind != kindSummary {
		return SummaryPoint{}, false
	}

	point := SummaryPoint{
		Count: s.summaryCount,
		Sum:   s.summarySum,
	}

	if schema := s.desc.summary; schema != nil && len(schema.quantiles) > 0 {
		if len(s.summaryQuantiles) != len(schema.quantiles) {
			return SummaryPoint{}, false
		}
		point.Quantiles = make([]QuantilePoint, len(schema.quantiles))
		for i, q := range schema.quantiles {
			point.Quantiles[i] = QuantilePoint{
				Quantile: q,
				Value:    s.summaryQuantiles[i],
			}
		}
	}

	return point, true
}

func (r *storeReader) StateSet(name string, labels Labels) (StateSetPoint, bool) {
	if r.flattened {
		return StateSetPoint{}, false
	}

	s, ok := r.lookup(name, labels)
	if !ok || !r.visible(s) {
		return StateSetPoint{}, false
	}
	if s.desc == nil || s.desc.kind != kindStateSet {
		return StateSetPoint{}, false
	}
	return StateSetPoint{States: cloneStateMap(s.stateSetValues)}, true
}

func (r *storeReader) MeasureSet(name string, labels Labels) (MeasureSetPoint, bool) {
	if r.flattened {
		return MeasureSetPoint{}, false
	}

	s, ok := r.lookup(name, labels)
	if !ok || !r.visible(s) {
		return MeasureSetPoint{}, false
	}
	if s.desc == nil || s.desc.kind != kindMeasureSet || s.desc.measureSet == nil {
		return MeasureSetPoint{}, false
	}
	if len(s.measureSetValues) != len(s.desc.measureSet.fields) {
		return MeasureSetPoint{}, false
	}
	return MeasureSetPoint{Values: append([]SampleValue(nil), s.measureSetValues...)}, true
}

func (r *storeReader) SeriesMeta(name string, labels Labels) (SeriesMeta, bool) {
	s, ok := r.lookup(name, labels)
	if !ok || !r.visible(s) {
		return SeriesMeta{}, false
	}
	return s.meta, true
}

func (r *storeReader) MetricMeta(name string) (MetricMeta, bool) {
	scope := r.scopeIndex()
	if series := scope.byName[name]; len(series) > 0 {
		return series[0].desc.meta, true
	}
	desc := scope.structuredMetaByName[name]
	if desc == nil {
		return MetricMeta{}, false
	}
	return desc.meta, true
}

func (r *storeReader) CollectMeta() CollectMeta {
	return r.snap.collectMeta
}

func (r *storeReader) HostScopes() []HostScope {
	// Scope discovery intentionally ignores this reader's active scope filter.
	return cloneHostScopes(r.snapshotIndex().hostScopes)
}

func (r *collectorReader) FreshVisibleHostScopes() []HostScope {
	return cloneHostScopes(r.snapshotIndex().freshVisibleHostScopes)
}

func (r *storeReader) Family(name string) (FamilyView, bool) {
	if slices.ContainsFunc(r.scopeIndex().byName[name], r.visible) {
		return familyView{name: name, reader: r}, true
	}
	return nil, false
}

func (r *storeReader) ForEachByName(name string, fn func(labels LabelView, v SampleValue)) {
	for _, s := range r.scopeIndex().byName[name] {
		if r.visible(s) {
			fn(labelView{items: s.labels}, s.value)
		}
	}
}

func (r *storeReader) ForEachSeries(fn func(name string, labels LabelView, v SampleValue)) {
	r.ForEachSeriesIdentity(func(_ SeriesIdentity, _ SeriesMeta, name string, labels LabelView, v SampleValue) {
		fn(name, labels, v)
	})
}

func (r *storeReader) ForEachSeriesIdentity(fn func(identity SeriesIdentity, meta SeriesMeta, name string, labels LabelView, v SampleValue)) {
	r.ForEachSeriesIdentityRaw(func(identity SeriesIdentity, meta SeriesMeta, name string, labels []Label, v SampleValue) {
		fn(identity, meta, name, labelView{items: labels}, v)
	})
}

func (r *storeReader) ForEachSeriesIdentityRaw(fn func(identity SeriesIdentity, meta SeriesMeta, name string, labels []Label, v SampleValue)) {
	scope := r.scopeIndex()
	for _, name := range scope.names {
		for _, s := range scope.byName[name] {
			if r.visible(s) {
				hash := s.hash64
				if hash == 0 {
					hash = seriesIDHash(s.id)
				}
				fn(SeriesIdentity{
					ID:     s.id,
					Hash64: hash,
				}, s.meta, name, s.labels, s.value)
			}
		}
	}
}

func (r *storeReader) ForEachMatch(name string, match func(labels LabelView) bool, fn func(labels LabelView, v SampleValue)) {
	for _, s := range r.scopeIndex().byName[name] {
		if !r.visible(s) {
			continue
		}
		lv := labelView{items: s.labels}
		if match(lv) {
			fn(lv, s.value)
		}
	}
}

func (f familyView) Name() string {
	return f.name
}

func (f familyView) ForEach(fn func(labels LabelView, v SampleValue)) {
	f.reader.ForEachByName(f.name, fn)
}

func (r *storeReader) lookup(name string, labels Labels) (*committedSeries, bool) {
	items, labelsKey, err := canonicalizeLabels(labels)
	if err != nil {
		_ = items
		return nil, false
	}
	key := makeSeriesKey(r.hostScopeKey, name, labelsKey)
	return lookupSnapshotSeries(r.snap, key)
}

func (r *storeReader) scopeIndex() *snapshotScopeIndex {
	if scope := r.snapshotIndex().scope(r.hostScopeKey); scope != nil {
		return scope
	}
	return &emptySnapshotScopeIndex
}

func (r *storeReader) snapshotIndex() *snapshotSeriesIndex {
	if r.snap.index != nil || r.snap.collector != nil {
		return r.snap.seriesIndex()
	}
	r.indexOnce.Do(func() {
		r.index = buildSnapshotSeriesIndex(r.seriesView(), r.snap.collectMeta)
	})
	return r.index
}

func (r *storeReader) seriesView() map[string]*committedSeries {
	if r.snap.runtimeBase == nil {
		return r.snap.series
	}
	r.seriesOnce.Do(func() {
		r.series = materializeRuntimeSeries(r.snap)
	})
	return r.series
}

func lookupSnapshotSeries(snap *readSnapshot, key string) (*committedSeries, bool) {
	for curr := snap; curr != nil; curr = curr.runtimeBase {
		if s, ok := curr.series[key]; ok {
			return s, true
		}
	}
	return nil, false
}

func snapshotSeriesView(snap *readSnapshot) map[string]*committedSeries {
	if snap.runtimeBase == nil {
		return snap.series
	}
	return materializeRuntimeSeries(snap)
}

func materializeRuntimeSeries(snap *readSnapshot) map[string]*committedSeries {
	chain := make([]*readSnapshot, 0, snap.runtimeDepth+1)
	for curr := snap; curr != nil; curr = curr.runtimeBase {
		chain = append(chain, curr)
	}
	if len(chain) == 0 {
		return nil
	}
	// Chain is leaf->root; root map gives the best starting capacity hint.
	series := make(map[string]*committedSeries, len(chain[len(chain)-1].series))
	for i := len(chain) - 1; i >= 0; i-- {
		maps.Copy(series, chain[i].series)
	}
	return series
}

// visible applies freshness policy for Read(); Read(ReadRaw()) bypasses it.
func (r *storeReader) visible(s *committedSeries) bool {
	if s.hostScopeKey != r.hostScopeKey {
		return false
	}
	if r.raw {
		return true
	}
	return freshSeriesVisible(s, r.snap.collectMeta)
}

var _ Reader = (*storeReader)(nil)
var _ Reader = (*collectorReader)(nil)
var _ SeriesIdentityRawIterator = (*collectorReader)(nil)
var _ FreshVisibleHostScopesReader = (*collectorReader)(nil)
