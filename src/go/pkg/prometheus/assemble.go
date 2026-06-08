// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"strconv"
	"strings"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
)

// assembler folds the driver's classified sample stream into typed
// MetricFamilies. Grouping of summary quantiles / histogram buckets with their
// _sum/_count is keyed by (family name, hash of the base labels). Buffers are
// reused across cycles via reset().
type assembler struct {
	metrics    MetricFamilies
	summaries  map[assemblyKey]*Summary
	histograms map[assemblyKey]*Histogram
	scratch    labels.Labels

	// currName/currFamily cache the most recent family to skip the metrics map
	// lookup for the common case of consecutive samples in the same family.
	currName   string
	currFamily *MetricFamily
}

type assemblyKey struct {
	name string
	hash uint64
}

func (a *assembler) reset() {
	a.currName = ""
	a.currFamily = nil

	if a.metrics == nil {
		a.metrics = make(MetricFamilies)
	}
	for _, mf := range a.metrics {
		mf.help = ""
		mf.typ = ""
		mf.metrics = mf.metrics[:0]
	}

	if a.summaries == nil {
		a.summaries = make(map[assemblyKey]*Summary)
	}
	for k := range a.summaries {
		delete(a.summaries, k)
	}

	if a.histograms == nil {
		a.histograms = make(map[assemblyKey]*Histogram)
	}
	for k := range a.histograms {
		delete(a.histograms, k)
	}
}

func (a *assembler) applyHelp(name, help string) {
	mf := a.ensureFamily(name)
	mf.help = help
}

func (a *assembler) applySample(sample Sample) error {
	switch sample.Kind {
	case SampleKindSummaryQuantile:
		a.addSummaryQuantile(sample)
	case SampleKindSummarySum:
		a.addSummarySum(sample)
	case SampleKindSummaryCount:
		a.addSummaryCount(sample)
	case SampleKindHistogramBucket:
		a.addHistogramBucket(sample)
	case SampleKindHistogramSum:
		a.addHistogramSum(sample)
	case SampleKindHistogramCount:
		a.addHistogramCount(sample)
	default:
		switch sample.FamilyType {
		case model.MetricTypeSummary:
			mf := a.summaryFamily(sample.Name)
			a.summaryFor(mf, assemblyKey{name: sample.Name, hash: sample.Labels.Hash()}, sample.Labels)
		case model.MetricTypeHistogram:
			mf := a.histogramFamily(sample.Name)
			a.histogramFor(mf, assemblyKey{name: sample.Name, hash: sample.Labels.Hash()}, sample.Labels)
		default:
			a.addScalar(sample)
		}
	}
	return nil
}

func (a *assembler) families() MetricFamilies {
	for name, mf := range a.metrics {
		if len(mf.metrics) == 0 {
			delete(a.metrics, name)
		}
	}
	return a.metrics
}

func (a *assembler) addScalar(sample Sample) {
	mf := a.ensureFamily(sample.Name)

	typ := sample.FamilyType
	if typ == "" {
		typ = model.MetricTypeUnknown
	}
	if mf.typ == "" || mf.typ == model.MetricTypeUnknown {
		mf.typ = typ
	}

	m := a.appendMetric(mf, sample.Labels)

	switch typ {
	case model.MetricTypeGauge:
		if m.gauge == nil {
			m.gauge = &Gauge{}
		}
		m.gauge.value = sample.Value
	case model.MetricTypeCounter:
		if m.counter == nil {
			m.counter = &Counter{}
		}
		m.counter.value = sample.Value
	default:
		if m.untyped == nil {
			m.untyped = &Untyped{}
		}
		m.untyped.value = sample.Value
	}
}

func (a *assembler) addSummaryQuantile(sample Sample) {
	mf := a.summaryFamily(sample.Name)
	base, qv, ok := a.stripLabel(sample.Labels, quantileLabel)
	key := assemblyKey{name: sample.Name}
	if ok {
		key.hash = labels.Labels(base).Hash()
	} else {
		base = sample.Labels
		key.hash = sample.Labels.Hash()
	}

	s := a.summaryFor(mf, key, base)
	if !ok {
		return
	}
	quantile, _ := strconv.ParseFloat(qv, 64)
	s.quantiles = append(s.quantiles, Quantile{quantile: quantile, value: sample.Value})
}

func (a *assembler) addSummarySum(sample Sample) {
	name := strings.TrimSuffix(sample.Name, sumSuffix)
	mf := a.summaryFamily(name)
	s := a.summaryFor(mf, assemblyKey{name: name, hash: sample.Labels.Hash()}, sample.Labels)
	s.sum = sample.Value
}

func (a *assembler) addSummaryCount(sample Sample) {
	name := strings.TrimSuffix(sample.Name, countSuffix)
	mf := a.summaryFamily(name)
	s := a.summaryFor(mf, assemblyKey{name: name, hash: sample.Labels.Hash()}, sample.Labels)
	s.count = sample.Value
}

func (a *assembler) addHistogramBucket(sample Sample) {
	name := strings.TrimSuffix(sample.Name, bucketSuffix)
	mf := a.histogramFamily(name)
	base, lev, ok := a.stripLabel(sample.Labels, bucketLabel)
	key := assemblyKey{name: name}
	if ok {
		key.hash = labels.Labels(base).Hash()
	} else {
		base = sample.Labels
		key.hash = sample.Labels.Hash()
	}

	h := a.histogramFor(mf, key, base)
	if !ok {
		return
	}
	bound, _ := strconv.ParseFloat(lev, 64)
	h.buckets = append(h.buckets, Bucket{upperBound: bound, cumulativeCount: sample.Value})
}

func (a *assembler) addHistogramSum(sample Sample) {
	name := strings.TrimSuffix(sample.Name, sumSuffix)
	mf := a.histogramFamily(name)
	h := a.histogramFor(mf, assemblyKey{name: name, hash: sample.Labels.Hash()}, sample.Labels)
	h.sum = sample.Value
}

func (a *assembler) addHistogramCount(sample Sample) {
	name := strings.TrimSuffix(sample.Name, countSuffix)
	mf := a.histogramFamily(name)
	h := a.histogramFor(mf, assemblyKey{name: name, hash: sample.Labels.Hash()}, sample.Labels)
	h.count = sample.Value
}

func (a *assembler) summaryFamily(name string) *MetricFamily {
	mf := a.ensureFamily(name)
	mf.typ = model.MetricTypeSummary
	return mf
}

func (a *assembler) histogramFamily(name string) *MetricFamily {
	mf := a.ensureFamily(name)
	mf.typ = model.MetricTypeHistogram
	return mf
}

func (a *assembler) summaryFor(mf *MetricFamily, key assemblyKey, lbs labels.Labels) *Summary {
	if s, ok := a.summaries[key]; ok {
		return s
	}

	m := a.appendMetric(mf, lbs)
	if m.summary == nil {
		m.summary = &Summary{}
	} else {
		m.summary.sum = 0
		m.summary.count = 0
		m.summary.quantiles = m.summary.quantiles[:0]
	}

	a.summaries[key] = m.summary
	return m.summary
}

func (a *assembler) histogramFor(mf *MetricFamily, key assemblyKey, lbs labels.Labels) *Histogram {
	if h, ok := a.histograms[key]; ok {
		return h
	}

	m := a.appendMetric(mf, lbs)
	if m.histogram == nil {
		m.histogram = &Histogram{}
	} else {
		m.histogram.sum = 0
		m.histogram.count = 0
		m.histogram.buckets = m.histogram.buckets[:0]
	}

	a.histograms[key] = m.histogram
	return m.histogram
}

// appendMetric grows mf.metrics by one, reusing the backing array across cycles
// and storing a copy of lbs. Instrument pointers on a reused slot are left in
// place: a family keeps a stable type across scrapes, so the caller reuses the
// existing instrument and allocates only when its pointer is nil. This preserves
// the legacy allocation profile (no per-scrape instrument churn).
func (a *assembler) appendMetric(mf *MetricFamily, lbs labels.Labels) *Metric {
	idx := len(mf.metrics)
	if idx == cap(mf.metrics) {
		mf.metrics = append(mf.metrics, Metric{})
	} else {
		mf.metrics = mf.metrics[:idx+1]
	}

	m := &mf.metrics[idx]
	m.labels = m.labels[:0]
	m.labels = append(m.labels, lbs...)
	return m
}

func (a *assembler) ensureFamily(name string) *MetricFamily {
	if a.currFamily != nil && a.currName == name {
		return a.currFamily
	}
	mf, ok := a.metrics[name]
	if !ok {
		mf = &MetricFamily{name: name, typ: model.MetricTypeUnknown}
		a.metrics[name] = mf
	}
	a.currName = name
	a.currFamily = mf
	return mf
}

// stripLabel returns the label set without name and the removed value, using a
// reusable scratch buffer. The result is valid only until the next stripLabel.
func (a *assembler) stripLabel(lbs labels.Labels, name string) (labels.Labels, string, bool) {
	a.scratch = a.scratch[:0]
	var (
		value string
		found bool
	)
	for _, lb := range lbs {
		if lb.Name == name {
			value = lb.Value
			found = true
			continue
		}
		a.scratch = append(a.scratch, lb)
	}
	if !found {
		return nil, "", false
	}
	return a.scratch, value, true
}
