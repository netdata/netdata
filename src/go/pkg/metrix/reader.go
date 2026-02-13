// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import "sort"

type storeReader struct {
	snap *readSnapshot
	raw  bool // true => ReadRaw semantics (no freshness filtering)
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
	_ = name
	_ = labels
	return StateSetPoint{}, false
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
	return r
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
