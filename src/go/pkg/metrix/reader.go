// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"sort"
	"sync"
)

type storeReader struct {
	snap        *readSnapshot
	raw         bool // true => ReadRaw semantics (no freshness filtering)
	flattened   bool
	flattenOnce sync.Once
	flatten     Reader
}

type familyView struct {
	name   string
	reader *storeReader
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
	if !s.counterHasPrev || s.counterCurrentAttemptSeq != s.counterPreviousAttemptSeq+1 {
		return 0, false
	}
	if s.counterCurrent < s.counterPrevious {
		return s.counterCurrent, true
	}
	return s.counterCurrent - s.counterPrevious, true
}

func (r *storeReader) Histogram(name string, labels Labels) (HistogramPoint, bool) {
	_ = name
	_ = labels
	return HistogramPoint{}, false
}

func (r *storeReader) Summary(name string, labels Labels) (SummaryPoint, bool) {
	_ = name
	_ = labels
	return SummaryPoint{}, false
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

func (r *storeReader) CollectMeta() CollectMeta {
	return r.snap.collectMeta
}

func (r *storeReader) Flatten() Reader {
	if r.flattened {
		return r
	}
	r.flattenOnce.Do(func() {
		r.flatten = &storeReader{
			snap:      flattenSnapshot(r.snap),
			raw:       r.raw,
			flattened: true,
		}
	})
	if r.flatten == nil {
		return r
	}
	return r.flatten
}

func flattenSnapshot(src *readSnapshot) *readSnapshot {
	dst := &readSnapshot{
		collectMeta: src.collectMeta,
		series:      make(map[string]*committedSeries, len(src.series)),
		byName:      make(map[string][]*committedSeries),
	}

	for _, s := range src.series {
		if s.desc == nil {
			continue
		}
		switch s.desc.kind {
		case kindGauge, kindCounter:
			dst.series[s.key] = cloneCommittedSeries(s)
		case kindStateSet:
			appendFlattenedStateSetSeries(dst, s)
		}
	}

	dst.byName = buildByName(dst.series)
	return dst
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
			key:       key,
			name:      src.name,
			labels:    labels,
			labelsKey: labelsKey,
			desc: &instrumentDescriptor{
				name:      src.name,
				kind:      kindGauge,
				mode:      src.desc.mode,
				freshness: src.desc.freshness,
			},
			value: value,
			meta:  src.meta,
		}
	}
}

func (r *storeReader) Family(name string) (FamilyView, bool) {
	if len(r.snap.byName[name]) == 0 {
		return nil, false
	}
	return familyView{name: name, reader: r}, true
}

func (r *storeReader) ForEachByName(name string, fn func(labels LabelView, v SampleValue)) {
	for _, s := range r.snap.byName[name] {
		if r.visible(s) {
			fn(labelView{items: s.labels}, s.value)
		}
	}
}

func (r *storeReader) ForEachSeries(fn func(name string, labels LabelView, v SampleValue)) {
	names := make([]string, 0, len(r.snap.byName))
	for name := range r.snap.byName {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		for _, s := range r.snap.byName[name] {
			if r.visible(s) {
				fn(name, labelView{items: s.labels}, s.value)
			}
		}
	}
}

func (r *storeReader) ForEachMatch(name string, match func(labels LabelView) bool, fn func(labels LabelView, v SampleValue)) {
	for _, s := range r.snap.byName[name] {
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
	s, ok := r.snap.series[key]
	return s, ok
}

// visible applies freshness policy for Read(); ReadRaw() bypasses it.
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
