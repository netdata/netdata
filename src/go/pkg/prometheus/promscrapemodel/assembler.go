// SPDX-License-Identifier: GPL-3.0-or-later

package promscrapemodel

import (
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
)

type Assembler struct {
	metrics    MetricFamilies
	summaries  map[assemblyKey]*summaryState
	histograms map[assemblyKey]*histogramState
	scratch    labels.Labels
	sealed     bool
}

type assemblyKey struct {
	name string
	hash uint64
}

type summaryState struct {
	family   *MetricFamily
	index    int
	summary  *Summary
	hasSum   bool
	hasCount bool
	invalid  bool
}

type histogramState struct {
	family    *MetricFamily
	index     int
	histogram *Histogram
	hasSum    bool
	hasCount  bool
	invalid   bool
}

func NewAssembler() *Assembler {
	return &Assembler{sealed: true}
}

// BeginCycle resets the assembler for a new scrape/assembly cycle.
func (a *Assembler) BeginCycle() {
	a.beginCycle()
}

func (a *Assembler) ApplySample(sample Sample) error {
	if a.sealed {
		a.beginCycle()
	}

	switch sample.Kind {
	case SampleKindSummaryQuantile:
		return a.addSummarySample(sample)
	case SampleKindSummarySum:
		return a.addSummarySum(sample)
	case SampleKindSummaryCount:
		return a.addSummaryCount(sample)
	case SampleKindHistogramBucket:
		return a.addHistogramBucket(sample)
	case SampleKindHistogramSum:
		return a.addHistogramSum(sample)
	case SampleKindHistogramCount:
		return a.addHistogramCount(sample)
	default:
		switch sample.FamilyType {
		case model.MetricTypeSummary:
			a.addSummaryBase(sample)
			return nil
		case model.MetricTypeHistogram:
			a.addHistogramBase(sample)
			return nil
		}

		a.addScalarSample(sample)
		return nil
	}
}

func (a *Assembler) MetricFamilies() MetricFamilies {
	if a.metrics == nil {
		a.metrics = make(MetricFamilies)
	}

	if !a.sealed {
		a.pruneInvalidTypedFamilies()
		for name, mf := range a.metrics {
			if len(mf.metrics) == 0 {
				delete(a.metrics, name)
			}
		}
		a.sealed = true
	}

	return a.metrics
}

func (a *Assembler) ApplyHelp(name, help string) {
	a.applyHelp(name, help)
}

func (a *Assembler) applyHelp(name, help string) {
	if a.sealed {
		a.beginCycle()
	}

	mf := a.ensureMetricFamily(name)
	mf.help = help
}

func (a *Assembler) beginCycle() {
	a.reset()
	a.sealed = false
}

func (a *Assembler) reset() {
	if a.metrics == nil {
		a.metrics = make(MetricFamilies)
	}
	for _, mf := range a.metrics {
		mf.help = ""
		mf.typ = model.MetricTypeUnknown
		mf.metrics = mf.metrics[:0]
	}

	if a.summaries == nil {
		a.summaries = make(map[assemblyKey]*summaryState)
	}
	for key := range a.summaries {
		delete(a.summaries, key)
	}

	if a.histograms == nil {
		a.histograms = make(map[assemblyKey]*histogramState)
	}
	for key := range a.histograms {
		delete(a.histograms, key)
	}
}

func (a *Assembler) addScalarSample(sample Sample) {
	familyName := sample.Name
	typ := sample.FamilyType
	if typ == "" {
		typ = model.MetricTypeUnknown
	}

	mf := a.ensureMetricFamily(familyName)
	if mf.typ == model.MetricTypeUnknown && typ != model.MetricTypeUnknown {
		mf.typ = typ
	} else if mf.typ == "" {
		mf.typ = typ
	}

	idx := len(mf.metrics)
	if idx == cap(mf.metrics) {
		mf.metrics = append(mf.metrics, Metric{})
	} else {
		mf.metrics = mf.metrics[:idx+1]
	}

	metric := &mf.metrics[idx]
	copyMetricLabels(metric, sample.Labels)
	metric.gauge = nil
	metric.counter = nil
	metric.summary = nil
	metric.histogram = nil
	metric.untyped = nil

	switch typ {
	case model.MetricTypeGauge:
		if metric.gauge == nil {
			metric.gauge = &Gauge{}
		}
		metric.gauge.value = sample.Value
	case model.MetricTypeCounter:
		if metric.counter == nil {
			metric.counter = &Counter{}
		}
		metric.counter.value = sample.Value
	default:
		if metric.untyped == nil {
			metric.untyped = &Untyped{}
		}
		metric.untyped.value = sample.Value
	}
}

func (a *Assembler) addSummarySample(sample Sample) error {
	mf := a.ensureMetricFamily(sample.Name)
	mf.typ = model.MetricTypeSummary
	baseLabels, value, ok := a.stripLabel(sample.Labels, quantileLabel)
	key := assemblyKey{name: sample.Name}
	if ok {
		key.hash = labels.Labels(baseLabels).Hash()
	} else {
		baseLabels = sample.Labels
		key.hash = sample.Labels.Hash()
	}

	state := a.summaryStateFor(mf, key, baseLabels)
	if !ok {
		state.invalid = true
		return nil
	}

	quantile, err := strconv.ParseFloat(value, 64)
	if err != nil {
		state.invalid = true
		return nil
	}
	if state.invalid {
		return nil
	}

	state.summary.quantiles = append(state.summary.quantiles, Quantile{quantile: quantile, value: sample.Value})
	return nil
}

func (a *Assembler) addSummaryBase(sample Sample) {
	mf := a.ensureMetricFamily(sample.Name)
	mf.typ = model.MetricTypeSummary

	key := assemblyKey{name: sample.Name, hash: sample.Labels.Hash()}
	_ = a.summaryStateFor(mf, key, sample.Labels)
}

func (a *Assembler) addSummarySum(sample Sample) error {
	familyName := strings.TrimSuffix(sample.Name, sumSuffix)
	mf := a.ensureMetricFamily(familyName)
	mf.typ = model.MetricTypeSummary

	key := assemblyKey{name: familyName, hash: sample.Labels.Hash()}
	state := a.summaryStateFor(mf, key, sample.Labels)
	if state.invalid {
		return nil
	}
	if state.hasSum {
		state.invalid = true
		return nil
	}
	state.summary.sum = sample.Value
	state.hasSum = true
	return nil
}

func (a *Assembler) addSummaryCount(sample Sample) error {
	familyName := strings.TrimSuffix(sample.Name, countSuffix)
	mf := a.ensureMetricFamily(familyName)
	mf.typ = model.MetricTypeSummary

	key := assemblyKey{name: familyName, hash: sample.Labels.Hash()}
	state := a.summaryStateFor(mf, key, sample.Labels)
	if state.invalid {
		return nil
	}
	if state.hasCount {
		state.invalid = true
		return nil
	}
	state.summary.count = sample.Value
	state.hasCount = true
	return nil
}

func (a *Assembler) addHistogramBucket(sample Sample) error {
	familyName := strings.TrimSuffix(sample.Name, bucketSuffix)
	mf := a.ensureMetricFamily(familyName)
	mf.typ = model.MetricTypeHistogram
	baseLabels, value, ok := a.stripLabel(sample.Labels, bucketLabel)
	key := assemblyKey{name: familyName}
	if ok {
		key.hash = labels.Labels(baseLabels).Hash()
	} else {
		baseLabels = sample.Labels
		key.hash = sample.Labels.Hash()
	}

	state := a.histogramStateFor(mf, key, baseLabels)
	if !ok {
		state.invalid = true
		return nil
	}

	bound, err := strconv.ParseFloat(value, 64)
	if err != nil {
		state.invalid = true
		return nil
	}
	if state.invalid {
		return nil
	}
	state.histogram.buckets = append(state.histogram.buckets, Bucket{upperBound: bound, cumulativeCount: sample.Value})
	return nil
}

func (a *Assembler) addHistogramBase(sample Sample) {
	mf := a.ensureMetricFamily(sample.Name)
	mf.typ = model.MetricTypeHistogram

	key := assemblyKey{name: sample.Name, hash: sample.Labels.Hash()}
	_ = a.histogramStateFor(mf, key, sample.Labels)
}

func (a *Assembler) addHistogramSum(sample Sample) error {
	familyName := strings.TrimSuffix(sample.Name, sumSuffix)
	mf := a.ensureMetricFamily(familyName)
	mf.typ = model.MetricTypeHistogram

	key := assemblyKey{name: familyName, hash: sample.Labels.Hash()}
	state := a.histogramStateFor(mf, key, sample.Labels)
	if state.invalid {
		return nil
	}
	if state.hasSum {
		state.invalid = true
		return nil
	}
	state.histogram.sum = sample.Value
	state.hasSum = true
	return nil
}

func (a *Assembler) addHistogramCount(sample Sample) error {
	familyName := strings.TrimSuffix(sample.Name, countSuffix)
	mf := a.ensureMetricFamily(familyName)
	mf.typ = model.MetricTypeHistogram

	key := assemblyKey{name: familyName, hash: sample.Labels.Hash()}
	state := a.histogramStateFor(mf, key, sample.Labels)
	if state.invalid {
		return nil
	}
	if state.hasCount {
		state.invalid = true
		return nil
	}
	state.histogram.count = sample.Value
	state.hasCount = true
	return nil
}

func (a *Assembler) summaryStateFor(mf *MetricFamily, key assemblyKey, lbs labels.Labels) *summaryState {
	if state, ok := a.summaries[key]; ok {
		return state
	}

	idx := len(mf.metrics)
	if idx == cap(mf.metrics) {
		mf.metrics = append(mf.metrics, Metric{})
	} else {
		mf.metrics = mf.metrics[:idx+1]
	}

	metric := &mf.metrics[idx]
	copyMetricLabels(metric, lbs)
	metric.gauge = nil
	metric.counter = nil
	metric.histogram = nil
	metric.untyped = nil
	if metric.summary == nil {
		metric.summary = &Summary{}
	} else {
		metric.summary.sum = 0
		metric.summary.count = 0
		metric.summary.quantiles = metric.summary.quantiles[:0]
	}

	state := &summaryState{
		family:  mf,
		index:   idx,
		summary: metric.summary,
	}
	a.summaries[key] = state
	return state
}

func (a *Assembler) histogramStateFor(mf *MetricFamily, key assemblyKey, lbs labels.Labels) *histogramState {
	if state, ok := a.histograms[key]; ok {
		return state
	}

	idx := len(mf.metrics)
	if idx == cap(mf.metrics) {
		mf.metrics = append(mf.metrics, Metric{})
	} else {
		mf.metrics = mf.metrics[:idx+1]
	}

	metric := &mf.metrics[idx]
	copyMetricLabels(metric, lbs)
	metric.gauge = nil
	metric.counter = nil
	metric.summary = nil
	metric.untyped = nil
	if metric.histogram == nil {
		metric.histogram = &Histogram{}
	} else {
		metric.histogram.sum = 0
		metric.histogram.count = 0
		metric.histogram.buckets = metric.histogram.buckets[:0]
	}

	state := &histogramState{
		family:    mf,
		index:     idx,
		histogram: metric.histogram,
	}
	a.histograms[key] = state
	return state
}

func (a *Assembler) ensureMetricFamily(name string) *MetricFamily {
	if a.metrics == nil {
		a.metrics = make(MetricFamilies)
	}

	if mf, ok := a.metrics[name]; ok {
		return mf
	}

	mf := &MetricFamily{name: name, typ: model.MetricTypeUnknown}
	a.metrics[name] = mf
	return mf
}

func copyMetricLabels(metric *Metric, lbs labels.Labels) {
	metric.labels = metric.labels[:0]
	metric.labels = append(metric.labels, lbs...)
}

func (a *Assembler) stripLabel(lbs labels.Labels, name string) (labels.Labels, string, bool) {
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

func (a *Assembler) pruneInvalidTypedFamilies() {
	drops := make(map[*MetricFamily]map[int]struct{})

	for _, state := range a.summaries {
		if validSummaryState(state) {
			continue
		}
		markMetricForDrop(drops, state.family, state.index)
	}
	for _, state := range a.histograms {
		if validHistogramState(state) {
			continue
		}
		markMetricForDrop(drops, state.family, state.index)
	}

	for mf, indexes := range drops {
		dst := mf.metrics[:0]
		for i := range mf.metrics {
			if _, drop := indexes[i]; drop {
				continue
			}
			dst = append(dst, mf.metrics[i])
		}
		mf.metrics = dst
	}
}

func markMetricForDrop(drops map[*MetricFamily]map[int]struct{}, mf *MetricFamily, idx int) {
	if mf == nil {
		return
	}
	if drops[mf] == nil {
		drops[mf] = make(map[int]struct{})
	}
	drops[mf][idx] = struct{}{}
}

func validSummaryState(state *summaryState) bool {
	if state == nil || state.invalid || state.summary == nil || !state.hasSum || !state.hasCount {
		return false
	}
	if len(state.summary.quantiles) == 0 {
		return false
	}
	if !isFiniteValue(state.summary.count) || !isFiniteValue(state.summary.sum) || state.summary.count < 0 {
		return false
	}

	qs := make([]float64, 0, len(state.summary.quantiles))
	for _, q := range state.summary.quantiles {
		// Prometheus summaries can legitimately expose NaN quantile values when
		// there are no observations yet. Keep the family and let consumers decide
		// whether to skip or normalize those values.
		if !isFiniteValue(q.quantile) || q.quantile < 0 || q.quantile > 1 {
			return false
		}
		qs = append(qs, q.quantile)
	}

	sort.Float64s(qs)
	for i := 1; i < len(qs); i++ {
		if qs[i] <= qs[i-1] {
			return false
		}
	}

	return true
}

func validHistogramState(state *histogramState) bool {
	if state == nil || state.invalid || state.histogram == nil || !state.hasSum || !state.hasCount {
		return false
	}
	if len(state.histogram.buckets) == 0 {
		return false
	}
	if !isFiniteValue(state.histogram.count) || !isFiniteValue(state.histogram.sum) || state.histogram.count < 0 {
		return false
	}

	buckets := append([]Bucket(nil), state.histogram.buckets...)
	for _, b := range buckets {
		if math.IsNaN(b.upperBound) || math.IsInf(b.upperBound, -1) || !isFiniteValue(b.cumulativeCount) || b.cumulativeCount < 0 {
			return false
		}
	}

	sort.Slice(buckets, func(i, j int) bool { return buckets[i].upperBound < buckets[j].upperBound })
	for i := 1; i < len(buckets); i++ {
		if buckets[i].upperBound <= buckets[i-1].upperBound {
			return false
		}
		if buckets[i].cumulativeCount < buckets[i-1].cumulativeCount {
			return false
		}
	}

	for i := len(buckets) - 1; i >= 0; i-- {
		if math.IsInf(buckets[i].upperBound, +1) {
			continue
		}
		if buckets[i].cumulativeCount > state.histogram.count {
			return false
		}
		break
	}

	return true
}

func isFiniteValue(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}
