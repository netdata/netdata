// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"math"
	"sort"
	"sync"
)

type storeReader struct {
	snap       *readSnapshot
	raw        bool // true => ReadRaw semantics (no freshness filtering)
	flattened  bool
	seriesOnce sync.Once
	series     map[string]*committedSeries
	indexOnce  sync.Once
	index      map[string][]*committedSeries
}

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

func (r *storeReader) SeriesMeta(name string, labels Labels) (SeriesMeta, bool) {
	s, ok := r.lookup(name, labels)
	if !ok || !r.visible(s) {
		return SeriesMeta{}, false
	}
	return s.meta, true
}

func (r *storeReader) MetricMeta(name string) (MetricMeta, bool) {
	index := r.byNameIndex()
	series := index[name]
	if len(series) == 0 {
		return MetricMeta{}, false
	}
	for _, s := range series {
		if s.desc == nil {
			continue
		}
		if r.raw || r.visible(s) {
			return s.desc.meta, true
		}
	}
	for _, s := range series {
		if s.desc != nil {
			return s.desc.meta, true
		}
	}
	return MetricMeta{}, false
}

func (r *storeReader) CollectMeta() CollectMeta {
	return r.snap.collectMeta
}

func flattenSnapshot(src *readSnapshot) *readSnapshot {
	series := snapshotSeriesView(src)
	dst := &readSnapshot{
		collectMeta: src.collectMeta,
		series:      make(map[string]*committedSeries, len(series)),
		byName:      make(map[string][]*committedSeries),
	}

	for _, s := range series {
		if s.desc == nil {
			continue
		}
		switch s.desc.kind {
		case kindGauge, kindCounter:
			dst.series[s.key] = cloneCommittedSeries(s)
		case kindHistogram:
			appendFlattenedHistogramSeries(dst, s)
		case kindSummary:
			appendFlattenedSummarySeries(dst, s)
		case kindStateSet:
			appendFlattenedStateSetSeries(dst, s)
		}
	}

	dst.byName = buildByName(dst.series)
	return dst
}

func appendFlattenedHistogramSeries(dst *readSnapshot, src *committedSeries) {
	schema := src.desc.histogram
	if schema == nil {
		return
	}
	if len(src.histogramCumulative) != len(schema.bounds) {
		// Defensive guard against malformed snapshots.
		// Histogram() follows the same rule and returns unavailable.
		return
	}

	for i, ub := range schema.bounds {
		labelsMap := make(map[string]string, len(src.labels)+1)
		for _, lbl := range src.labels {
			labelsMap[lbl.Key] = lbl.Value
		}
		labelsMap[histogramBucketLabel] = formatHistogramBucketLabel(ub)

		labels, labelsKey, err := canonicalizeLabels(labelsMap)
		if err != nil {
			continue
		}

		name := src.name + "_bucket"
		key := makeSeriesKey(name, labelsKey)
		dst.series[key] = &committedSeries{
			id:        SeriesID(key),
			hash64:    seriesIDHash(SeriesID(key)),
			key:       key,
			name:      name,
			labels:    labels,
			labelsKey: labelsKey,
			desc: &instrumentDescriptor{
				name:      name,
				kind:      kindCounter,
				mode:      src.desc.mode,
				freshness: src.desc.freshness,
				window:    src.desc.window,
				meta:      src.desc.meta,
			},
			value: src.histogramCumulative[i],
			meta: flattenedSeriesMeta(
				src.meta,
				MetricKindCounter,
				MetricKindHistogram,
				FlattenRoleHistogramBucket,
			),
		}
	}

	infMap := make(map[string]string, len(src.labels)+1)
	for _, lbl := range src.labels {
		infMap[lbl.Key] = lbl.Value
	}
	infMap[histogramBucketLabel] = formatHistogramBucketLabel(math.Inf(1))
	infLabels, infLabelsKey, err := canonicalizeLabels(infMap)
	if err == nil {
		infName := src.name + "_bucket"
		infKey := makeSeriesKey(infName, infLabelsKey)
		dst.series[infKey] = &committedSeries{
			id:        SeriesID(infKey),
			hash64:    seriesIDHash(SeriesID(infKey)),
			key:       infKey,
			name:      infName,
			labels:    infLabels,
			labelsKey: infLabelsKey,
			desc: &instrumentDescriptor{
				name:      infName,
				kind:      kindCounter,
				mode:      src.desc.mode,
				freshness: src.desc.freshness,
				window:    src.desc.window,
				meta:      src.desc.meta,
			},
			value: src.histogramCount,
			meta: flattenedSeriesMeta(
				src.meta,
				MetricKindCounter,
				MetricKindHistogram,
				FlattenRoleHistogramBucket,
			),
		}
	}

	appendFlattenedHistogramScalar(
		dst,
		src.name+"_count",
		src.labels,
		src.histogramCount,
		flattenedSeriesMeta(src.meta, MetricKindCounter, MetricKindHistogram, FlattenRoleHistogramCount),
		src.desc,
	)
	appendFlattenedHistogramScalar(
		dst,
		src.name+"_sum",
		src.labels,
		src.histogramSum,
		flattenedSeriesMeta(src.meta, MetricKindCounter, MetricKindHistogram, FlattenRoleHistogramSum),
		src.desc,
	)
}

func appendFlattenedHistogramScalar(dst *readSnapshot, name string, labels []Label, value SampleValue, meta SeriesMeta, desc *instrumentDescriptor) {
	labelsMap := make(map[string]string, len(labels))
	for _, lbl := range labels {
		labelsMap[lbl.Key] = lbl.Value
	}
	items, labelsKey, err := canonicalizeLabels(labelsMap)
	if err != nil {
		return
	}
	key := makeSeriesKey(name, labelsKey)
	dst.series[key] = &committedSeries{
		id:        SeriesID(key),
		hash64:    seriesIDHash(SeriesID(key)),
		key:       key,
		name:      name,
		labels:    items,
		labelsKey: labelsKey,
		desc: &instrumentDescriptor{
			name:      name,
			kind:      kindCounter,
			mode:      desc.mode,
			freshness: desc.freshness,
			window:    desc.window,
			meta:      desc.meta,
		},
		value: value,
		meta:  meta,
	}
}

func appendFlattenedSummarySeries(dst *readSnapshot, src *committedSeries) {
	appendFlattenedHistogramScalar(
		dst,
		src.name+"_count",
		src.labels,
		src.summaryCount,
		flattenedSeriesMeta(src.meta, MetricKindCounter, MetricKindSummary, FlattenRoleSummaryCount),
		src.desc,
	)
	appendFlattenedHistogramScalar(
		dst,
		src.name+"_sum",
		src.labels,
		src.summarySum,
		flattenedSeriesMeta(src.meta, MetricKindCounter, MetricKindSummary, FlattenRoleSummarySum),
		src.desc,
	)

	schema := src.desc.summary
	if schema == nil {
		return
	}
	if len(src.summaryQuantiles) != len(schema.quantiles) {
		return
	}

	for i, q := range schema.quantiles {
		labelsMap := make(map[string]string, len(src.labels)+1)
		for _, lbl := range src.labels {
			labelsMap[lbl.Key] = lbl.Value
		}
		labelsMap[summaryQuantileLabel] = formatSummaryQuantileLabel(q)

		labels, labelsKey, err := canonicalizeLabels(labelsMap)
		if err != nil {
			continue
		}
		key := makeSeriesKey(src.name, labelsKey)
		dst.series[key] = &committedSeries{
			id:        SeriesID(key),
			hash64:    seriesIDHash(SeriesID(key)),
			key:       key,
			name:      src.name,
			labels:    labels,
			labelsKey: labelsKey,
			desc: &instrumentDescriptor{
				name:      src.name,
				kind:      kindGauge,
				mode:      src.desc.mode,
				freshness: src.desc.freshness,
				window:    src.desc.window,
				meta:      src.desc.meta,
			},
			value: src.summaryQuantiles[i],
			meta: flattenedSeriesMeta(
				src.meta,
				MetricKindGauge,
				MetricKindSummary,
				FlattenRoleSummaryQuantile,
			),
		}
	}
}

func appendFlattenedStateSetSeries(dst *readSnapshot, src *committedSeries) {
	schema := src.desc.stateSet
	if schema == nil {
		return
	}

	for _, state := range schema.states {
		value := SampleValue(0)
		if src.stateSetValues[state] {
			value = 1
		}

		labelsMap := make(map[string]string, len(src.labels)+1)
		for _, lbl := range src.labels {
			labelsMap[lbl.Key] = lbl.Value
		}
		labelsMap[src.name] = state

		labels, labelsKey, err := canonicalizeLabels(labelsMap)
		if err != nil {
			continue
		}

		key := makeSeriesKey(src.name, labelsKey)
		dst.series[key] = &committedSeries{
			id:        SeriesID(key),
			hash64:    seriesIDHash(SeriesID(key)),
			key:       key,
			name:      src.name,
			labels:    labels,
			labelsKey: labelsKey,
			desc: &instrumentDescriptor{
				name:      src.name,
				kind:      kindGauge,
				mode:      src.desc.mode,
				freshness: src.desc.freshness,
				window:    src.desc.window,
				meta:      src.desc.meta,
			},
			value: value,
			meta: flattenedSeriesMeta(
				src.meta,
				MetricKindGauge,
				MetricKindStateSet,
				FlattenRoleStateSetState,
			),
		}
	}
}

func (r *storeReader) Family(name string) (FamilyView, bool) {
	index := r.byNameIndex()
	if len(index[name]) == 0 {
		return nil, false
	}
	return familyView{name: name, reader: r}, true
}

func (r *storeReader) ForEachByName(name string, fn func(labels LabelView, v SampleValue)) {
	index := r.byNameIndex()
	for _, s := range index[name] {
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
	index := r.byNameIndex()
	names := make([]string, 0, len(index))
	for name := range index {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		for _, s := range index[name] {
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
	index := r.byNameIndex()
	for _, s := range index[name] {
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
	key := makeSeriesKey(name, labelsKey)
	return lookupSnapshotSeries(r.snap, key)
}

func (r *storeReader) byNameIndex() map[string][]*committedSeries {
	if r.snap.runtimeBase == nil && r.snap.byName != nil {
		return r.snap.byName
	}
	r.indexOnce.Do(func() {
		r.index = buildByName(r.seriesView())
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
		for key, s := range chain[i].series {
			series[key] = s
		}
	}
	return series
}

// visible applies freshness policy for Read(); Read(ReadRaw()) bypasses it.
func (r *storeReader) visible(s *committedSeries) bool {
	if r.raw {
		return true
	}
	if s.desc == nil {
		return false
	}
	if s.desc.freshness == FreshnessCommitted {
		return true
	}
	meta := r.snap.collectMeta
	return meta.LastAttemptStatus == CollectStatusSuccess && s.meta.LastSeenSuccessSeq == meta.LastSuccessSeq
}
