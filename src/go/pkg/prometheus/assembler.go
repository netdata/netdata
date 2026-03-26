// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"strconv"
	"strings"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"

	"github.com/netdata/netdata/go/plugins/pkg/prometheus/promscrapemodel"
)

type Assembler struct {
	metrics    MetricFamilies
	summaries  map[assemblyKey]*Summary
	histograms map[assemblyKey]*Histogram
	scratch    labels.Labels
	sealed     bool
}

type assemblyKey struct {
	name string
	hash uint64
}

func NewAssembler() *Assembler {
	return &Assembler{sealed: true}
}

func (a *Assembler) ApplySample(sample promscrapemodel.Sample) error {
	if a.sealed {
		a.beginCycle()
	}

	switch sample.Kind {
	case promscrapemodel.SampleKindSummaryQuantile:
		return a.addSummarySample(sample)
	case promscrapemodel.SampleKindSummarySum:
		return a.addSummarySum(sample)
	case promscrapemodel.SampleKindSummaryCount:
		return a.addSummaryCount(sample)
	case promscrapemodel.SampleKindHistogramBucket:
		return a.addHistogramBucket(sample)
	case promscrapemodel.SampleKindHistogramSum:
		return a.addHistogramSum(sample)
	case promscrapemodel.SampleKindHistogramCount:
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
		for name, mf := range a.metrics {
			if len(mf.metrics) == 0 {
				delete(a.metrics, name)
			}
		}
		a.sealed = true
	}

	return a.metrics
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
		a.summaries = make(map[assemblyKey]*Summary)
	}
	for key := range a.summaries {
		delete(a.summaries, key)
	}

	if a.histograms == nil {
		a.histograms = make(map[assemblyKey]*Histogram)
	}
	for key := range a.histograms {
		delete(a.histograms, key)
	}
}

func (a *Assembler) addScalarSample(sample promscrapemodel.Sample) {
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

func (a *Assembler) addSummarySample(sample promscrapemodel.Sample) error {
	baseLabels, quantile, ok := a.stripFloatLabel(sample.Labels, quantileLabel)
	if !ok {
		return nil
	}

	mf := a.ensureMetricFamily(sample.Name)
	mf.typ = model.MetricTypeSummary

	key := assemblyKey{name: sample.Name, hash: labels.Labels(baseLabels).Hash()}
	summary := a.summaryFor(mf, key, baseLabels)
	summary.quantiles = append(summary.quantiles, Quantile{quantile: quantile, value: sample.Value})
	return nil
}

func (a *Assembler) addSummaryBase(sample promscrapemodel.Sample) {
	mf := a.ensureMetricFamily(sample.Name)
	mf.typ = model.MetricTypeSummary

	key := assemblyKey{name: sample.Name, hash: sample.Labels.Hash()}
	_ = a.summaryFor(mf, key, sample.Labels)
}

func (a *Assembler) addSummarySum(sample promscrapemodel.Sample) error {
	familyName := strings.TrimSuffix(sample.Name, sumSuffix)
	mf := a.ensureMetricFamily(familyName)
	mf.typ = model.MetricTypeSummary

	key := assemblyKey{name: familyName, hash: sample.Labels.Hash()}
	summary := a.summaryFor(mf, key, sample.Labels)
	summary.sum = sample.Value
	return nil
}

func (a *Assembler) addSummaryCount(sample promscrapemodel.Sample) error {
	familyName := strings.TrimSuffix(sample.Name, countSuffix)
	mf := a.ensureMetricFamily(familyName)
	mf.typ = model.MetricTypeSummary

	key := assemblyKey{name: familyName, hash: sample.Labels.Hash()}
	summary := a.summaryFor(mf, key, sample.Labels)
	summary.count = sample.Value
	return nil
}

func (a *Assembler) addHistogramBucket(sample promscrapemodel.Sample) error {
	baseLabels, bound, ok := a.stripFloatLabel(sample.Labels, bucketLabel)
	if !ok {
		return nil
	}

	familyName := strings.TrimSuffix(sample.Name, bucketSuffix)
	mf := a.ensureMetricFamily(familyName)
	mf.typ = model.MetricTypeHistogram

	key := assemblyKey{name: familyName, hash: labels.Labels(baseLabels).Hash()}
	histogram := a.histogramFor(mf, key, baseLabels)
	histogram.buckets = append(histogram.buckets, Bucket{upperBound: bound, cumulativeCount: sample.Value})
	return nil
}

func (a *Assembler) addHistogramBase(sample promscrapemodel.Sample) {
	mf := a.ensureMetricFamily(sample.Name)
	mf.typ = model.MetricTypeHistogram

	key := assemblyKey{name: sample.Name, hash: sample.Labels.Hash()}
	_ = a.histogramFor(mf, key, sample.Labels)
}

func (a *Assembler) addHistogramSum(sample promscrapemodel.Sample) error {
	familyName := strings.TrimSuffix(sample.Name, sumSuffix)
	mf := a.ensureMetricFamily(familyName)
	mf.typ = model.MetricTypeHistogram

	key := assemblyKey{name: familyName, hash: sample.Labels.Hash()}
	histogram := a.histogramFor(mf, key, sample.Labels)
	histogram.sum = sample.Value
	return nil
}

func (a *Assembler) addHistogramCount(sample promscrapemodel.Sample) error {
	familyName := strings.TrimSuffix(sample.Name, countSuffix)
	mf := a.ensureMetricFamily(familyName)
	mf.typ = model.MetricTypeHistogram

	key := assemblyKey{name: familyName, hash: sample.Labels.Hash()}
	histogram := a.histogramFor(mf, key, sample.Labels)
	histogram.count = sample.Value
	return nil
}

func (a *Assembler) summaryFor(mf *MetricFamily, key assemblyKey, lbs labels.Labels) *Summary {
	if summary, ok := a.summaries[key]; ok {
		return summary
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

	a.summaries[key] = metric.summary
	return metric.summary
}

func (a *Assembler) histogramFor(mf *MetricFamily, key assemblyKey, lbs labels.Labels) *Histogram {
	if histogram, ok := a.histograms[key]; ok {
		return histogram
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

	a.histograms[key] = metric.histogram
	return metric.histogram
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

func (a *Assembler) stripFloatLabel(lbs labels.Labels, name string) (labels.Labels, float64, bool) {
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
		return nil, 0, false
	}

	v, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return a.scratch, 0, true
	}

	return a.scratch, v, true
}
