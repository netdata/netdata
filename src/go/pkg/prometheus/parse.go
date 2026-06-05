// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/textparse"

	"github.com/netdata/netdata/go/plugins/pkg/prometheus/selector"
)

const (
	quantileLabel = "quantile"
	bucketLabel   = "le"
)

const (
	countSuffix  = "_count"
	sumSuffix    = "_sum"
	bucketSuffix = "_bucket"
)

// promTextParser orchestrates a single parse pass. The driver parses the
// exposition once into a flat sample stream; the assembler folds that stream
// into typed MetricFamilies. Scrape() (families), ScrapeSeries() (Series), and
// the exported sample stream are all produced from this one model.
type promTextParser struct {
	sr selector.Selector

	driver parseDriver
	asm    assembler
	series Series
}

func (p *promTextParser) parseToMetricFamilies(text []byte) (MetricFamilies, error) {
	p.driver.sr = p.sr
	p.asm.reset()

	// ownLabels=false: the assembler copies labels into its own buffers, so the
	// driver may lend the scratch label set (the no-allocation fast path).
	if err := p.driver.parseSamples(text, false, p.asm.applyHelp, p.asm.applySample); err != nil {
		return nil, err
	}

	return p.asm.families(), nil
}

func (p *promTextParser) parseToSeries(text []byte) (Series, error) {
	p.driver.sr = p.sr
	p.series.Reset()

	// Series keeps the raw label set straight from textparse (sorted, with __name__
	// in its sorted position) — identical to the legacy parser. It does NOT go
	// through the Sample model (which separates __name__), so there is no deferral,
	// no reordering, and the sorted-label invariant is preserved.
	err := p.driver.iterate(text, nil, nil, func(series labels.Labels, value float64) error {
		p.series.Add(SeriesSample{Labels: copyLabels(series), Value: value})
		return nil
	})
	if err != nil {
		return nil, err
	}

	p.series.Sort()

	return p.series, nil
}

func (p *promTextParser) parseToStream(text []byte, onHelp func(name, help string), onSample func(Sample) error) error {
	if onSample == nil {
		return nil
	}
	p.driver.sr = p.sr

	// ownLabels=true: each Sample must own its labels because the consumer may
	// retain or mutate them (e.g. relabeling) past the next sample.
	return p.driver.parseSamples(text, true, onHelp, onSample)
}

// parseDriver runs the single exposition parse pass (iterate). On top of it,
// parseSamples emits a flat, classified sample stream, deferring a _sum/_count
// whose family type is not yet known and back-resolving it once the type appears
// (a later # TYPE, _bucket, or quantile series) or at EOF. familyTypes/pending
// hold that deferral state.
type parseDriver struct {
	sr selector.Selector

	familyTypes map[string]model.MetricType
	pending     []pendingSample
	currSeries  labels.Labels
}

type pendingSample struct {
	baseName string
	sample   Sample
	role     pendingRole
}

type pendingRole uint8

const (
	pendingNone pendingRole = iota
	pendingSum
	pendingCount
)

// iterate runs the shared single-pass exposition loop. For every series it calls
// onSeries with the raw label set (textparse order — sorted, __name__ in its
// sorted position) and value, after applying the selector; onHelp/onType deliver
// per-family metadata. This is the one parse loop: ScrapeSeries consumes it
// directly (raw labels, byte-identical to the legacy parser), while the flat
// sample stream is layered on top by parseSamples.
func (d *parseDriver) iterate(
	text []byte,
	onHelp func(name, help string),
	onType func(name string, typ model.MetricType) error,
	onSeries func(series labels.Labels, value float64) error,
) error {
	parser := textparse.NewPromParser(text, labels.NewSymbolTable())
	for {
		entry, err := parser.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			if entry == textparse.EntryInvalid && strings.HasPrefix(err.Error(), "invalid metric type") {
				continue
			}
			return fmt.Errorf("failed to parse prometheus metrics: %v", err)
		}

		switch entry {
		case textparse.EntryHelp:
			if onHelp != nil {
				name, help := parser.Help()
				onHelp(string(name), sanitizeHelp(string(help)))
			}
		case textparse.EntryType:
			if onType != nil {
				name, typ := parser.Type()
				if err := onType(string(name), typ); err != nil {
					return err
				}
			}
		case textparse.EntrySeries:
			d.currSeries = d.currSeries[:0]
			parser.Metric(&d.currSeries)

			if d.sr != nil && !d.sr.Matches(d.currSeries) {
				continue
			}

			_, _, value := parser.Series()

			if onSeries != nil {
				if err := onSeries(d.currSeries, value); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// parseSamples layers the flat, classified sample stream on top of iterate. It
// turns each series into a Sample (Kind + FamilyType) and defers a _sum/_count
// whose family type is not yet known, back-resolving it once the type appears (a
// later # TYPE, _bucket, or quantile series) or flushing it at EOF. Deferral can
// emit a _sum/_count after a later, unrelated series — see ScrapeStream's doc.
func (d *parseDriver) parseSamples(text []byte, ownLabels bool, onHelp func(name, help string), onSample func(Sample) error) error {
	d.reset()

	err := d.iterate(text, onHelp,
		func(name string, typ model.MetricType) error {
			d.familyTypes[name] = typ
			var err error
			d.pending, err = emitResolvedPending(d.pending, name, typ, onSample)
			return err
		},
		func(series labels.Labels, value float64) error {
			sample, baseName, role, ok := d.makeSample(series, value, ownLabels)
			if !ok {
				return nil
			}

			// A quantile/bucket series reveals the family type; back-resolve any
			// _sum/_count buffered before it.
			switch sample.Kind {
			case SampleKindSummaryQuantile:
				var err error
				d.pending, err = emitResolvedPending(d.pending, sample.Name, model.MetricTypeSummary, onSample)
				if err != nil {
					return err
				}
			case SampleKindHistogramBucket:
				var err error
				d.pending, err = emitResolvedPending(d.pending, strings.TrimSuffix(sample.Name, bucketSuffix), model.MetricTypeHistogram, onSample)
				if err != nil {
					return err
				}
			}

			if role != pendingNone {
				if !ownLabels {
					sample.Labels = copyLabels(sample.Labels)
				}
				d.pending = append(d.pending, pendingSample{
					baseName: baseName,
					sample:   sample,
					role:     role,
				})
				return nil
			}

			return onSample(sample)
		},
	)
	if err != nil {
		return err
	}

	// Flush still-unresolved _sum/_count as plain scalars (matches the legacy
	// behavior for a _sum/_count whose family type never appears).
	for _, ps := range d.pending {
		if err := onSample(ps.sample); err != nil {
			return err
		}
	}
	d.pending = d.pending[:0]

	return nil
}

func (d *parseDriver) makeSample(series labels.Labels, value float64, ownLabels bool) (Sample, string, pendingRole, bool) {
	name, ok := metricNameValue(series)
	if !ok {
		return Sample{}, "", pendingNone, false
	}

	var lbs labels.Labels
	if ownLabels {
		lbs = copyLabelsWithoutName(series)
	} else {
		lbs, _, _ = removeLabel(series, labels.MetricName)
	}

	sample := Sample{
		Name:       name,
		Labels:     lbs,
		Value:      value,
		Kind:       SampleKindScalar,
		FamilyType: d.familyTypes[name],
	}
	if sample.FamilyType == "" {
		sample.FamilyType = model.MetricTypeUnknown
	}

	if sample.Labels.Has(quantileLabel) {
		if sample.FamilyType != model.MetricTypeUnknown && sample.FamilyType != model.MetricTypeSummary {
			return sample, "", pendingNone, true
		}
		sample.Kind = SampleKindSummaryQuantile
		sample.FamilyType = model.MetricTypeSummary
		d.familyTypes[name] = model.MetricTypeSummary
		return sample, "", pendingNone, true
	}

	// A histogram bucket requires an "le" label. A _bucket-named series without le
	// is malformed: it is NOT treated as a bucket but falls through to a plain
	// metric, preserving its value. (The legacy parser folded such a series into
	// the histogram family and dropped its value; valid buckets always carry le,
	// so real input is unaffected.)
	if strings.HasSuffix(name, bucketSuffix) && sample.Labels.Has(bucketLabel) {
		if sample.FamilyType != model.MetricTypeUnknown && sample.FamilyType != model.MetricTypeHistogram {
			return sample, "", pendingNone, true
		}
		baseName := strings.TrimSuffix(name, bucketSuffix)
		sample.Kind = SampleKindHistogramBucket
		sample.FamilyType = model.MetricTypeHistogram
		d.familyTypes[baseName] = model.MetricTypeHistogram
		return sample, "", pendingNone, true
	}

	if strings.HasSuffix(name, sumSuffix) {
		if sample.FamilyType != model.MetricTypeUnknown &&
			sample.FamilyType != model.MetricTypeSummary &&
			sample.FamilyType != model.MetricTypeHistogram {
			return sample, "", pendingNone, true
		}

		baseName := strings.TrimSuffix(name, sumSuffix)
		switch d.familyTypes[baseName] {
		case model.MetricTypeSummary:
			sample.Kind = SampleKindSummarySum
			sample.FamilyType = model.MetricTypeSummary
			return sample, "", pendingNone, true
		case model.MetricTypeHistogram:
			sample.Kind = SampleKindHistogramSum
			sample.FamilyType = model.MetricTypeHistogram
			return sample, "", pendingNone, true
		default:
			return sample, baseName, pendingSum, true
		}
	}

	if strings.HasSuffix(name, countSuffix) {
		if sample.FamilyType != model.MetricTypeUnknown &&
			sample.FamilyType != model.MetricTypeSummary &&
			sample.FamilyType != model.MetricTypeHistogram {
			return sample, "", pendingNone, true
		}

		baseName := strings.TrimSuffix(name, countSuffix)
		switch d.familyTypes[baseName] {
		case model.MetricTypeSummary:
			sample.Kind = SampleKindSummaryCount
			sample.FamilyType = model.MetricTypeSummary
			return sample, "", pendingNone, true
		case model.MetricTypeHistogram:
			sample.Kind = SampleKindHistogramCount
			sample.FamilyType = model.MetricTypeHistogram
			return sample, "", pendingNone, true
		default:
			return sample, baseName, pendingCount, true
		}
	}

	return sample, "", pendingNone, true
}

func (d *parseDriver) reset() {
	d.currSeries = d.currSeries[:0]

	if d.familyTypes == nil {
		d.familyTypes = make(map[string]model.MetricType)
	}
	for k := range d.familyTypes {
		delete(d.familyTypes, k)
	}

	d.pending = d.pending[:0]
}

// emitResolvedPending flushes buffered _sum/_count samples for baseName now that
// its family type is known, emitting them in buffered (exposition) order.
func emitResolvedPending(pending []pendingSample, baseName string, typ model.MetricType, onSample func(Sample) error) ([]pendingSample, error) {
	if len(pending) == 0 {
		return pending, nil
	}

	out := pending[:0]
	for _, ps := range pending {
		if ps.baseName != baseName {
			out = append(out, ps)
			continue
		}

		sample := ps.sample
		sample.FamilyType = typ
		switch typ {
		case model.MetricTypeSummary:
			if ps.role == pendingSum {
				sample.Kind = SampleKindSummarySum
			} else {
				sample.Kind = SampleKindSummaryCount
			}
		case model.MetricTypeHistogram:
			if ps.role == pendingSum {
				sample.Kind = SampleKindHistogramSum
			} else {
				sample.Kind = SampleKindHistogramCount
			}
		default:
			sample.Kind = SampleKindScalar
			sample.FamilyType = model.MetricTypeUnknown
		}

		if err := onSample(sample); err != nil {
			return nil, err
		}
	}

	return out, nil
}

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

func copyLabels(lbs []labels.Label) []labels.Label {
	return append([]labels.Label(nil), lbs...)
}

// copyLabelsWithoutName returns a fresh copy of lbs with __name__ removed. In the
// common case __name__ sorts first (it precedes lowercase label names), so the
// remainder is contiguous and copied directly; otherwise a rare label that sorts
// before __name__ (e.g. "UUID") is skipped element by element.
func copyLabelsWithoutName(lbs labels.Labels) labels.Labels {
	if len(lbs) > 0 && lbs[0].Name == labels.MetricName {
		return copyLabels(lbs[1:])
	}
	out := make([]labels.Label, 0, len(lbs))
	for _, lb := range lbs {
		if lb.Name == labels.MetricName {
			continue
		}
		out = append(out, lb)
	}
	return out
}

func removeLabel(lbs labels.Labels, name string) (labels.Labels, string, bool) {
	for i, v := range lbs {
		if v.Name == name {
			return append(lbs[:i], lbs[i+1:]...), v.Value, true
		}
	}
	return lbs, "", false
}

func metricNameValue(lbs labels.Labels) (string, bool) {
	for _, v := range lbs {
		if v.Name == labels.MetricName {
			return v.Value, true
		}
	}
	return "", false
}

func sanitizeHelp(help string) string {
	if strings.IndexByte(help, '\n') == -1 {
		return help
	}
	// HELP is used as a chart title; collapse multiline help to one line.
	return strings.Join(strings.Fields(help), " ")
}
