// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"strings"
)

func flattenSnapshot(src *readSnapshot) *readSnapshot {
	series := snapshotSeriesView(src)

	// Size every structured family first so all children share exactly-sized blocks,
	// and the series map holds the scalars and every child without growing.
	var b flattenBlock
	var need flattenSize
	scalars := 0
	for _, s := range series {
		switch {
		case s.desc == nil:
		case isScalarKind(s.desc.kind):
			scalars++
		default:
			need.add(b.size(s))
		}
	}
	dst := &readSnapshot{
		collectMeta: src.collectMeta,
		series:      make(map[string]*committedSeries, scalars+need.children),
	}
	b.init(dst, need)
	for _, s := range series {
		if s.desc == nil {
			continue
		}
		switch s.desc.kind {
		case kindGauge, kindCounter:
			dst.series[s.key] = s
		case kindHistogram:
			b.appendHistogram(s)
		case kindSummary:
			b.appendSummary(s)
		case kindStateSet:
			b.appendStateSet(s)
		case kindMeasureSet:
			b.appendMeasureSet(s)
		}
	}

	dst.index = buildSnapshotSeriesIndex(dst.series, dst.collectMeta)
	return dst
}

// flattenBlock builds the flattened children of one snapshot's structured series.
// The snapshot's children share one series block, one descriptor block and one
// label backing, sized exactly before use so pointers into them stay valid; they
// all live exactly as long as the flattened snapshot. Each structured series gets
// one text allocation holding its children's derived names, canonical labels keys
// and series keys. Consumers keep series IDs and label values across snapshots
// (chartengine route caches and dimension names), so text is not shared across
// series: a retained ID pins only its own series' text. Bucket and quantile label
// values come from the descriptor schema.
type flattenBlock struct {
	dst    *readSnapshot
	src    *committedSeries
	series []committedSeries
	descs  []instrumentDescriptor
	labels []Label
	text   strings.Builder
}

// flattenSize counts what a structured family's children occupy: shared block
// entries and the family's own text.
type flattenSize struct {
	children int
	descs    int
	labels   int
	text     int
}

func (n *flattenSize) add(o flattenSize) {
	n.children += o.children
	n.descs += o.descs
	n.labels += o.labels
}

func (b *flattenBlock) init(dst *readSnapshot, need flattenSize) {
	b.dst = dst
	if need.children == 0 {
		return
	}
	b.series = make([]committedSeries, 0, need.children)
	b.descs = make([]instrumentDescriptor, 0, need.descs)
	if need.labels > 0 {
		b.labels = make([]Label, 0, need.labels)
	}
}

// begin starts a structured family's text buffer.
func (b *flattenBlock) begin(src *committedSeries, need flattenSize) {
	b.src = src
	b.text = strings.Builder{}
	b.text.Grow(need.text)
}

// size sizes the children the matching append method publishes for src.
func (b *flattenBlock) size(src *committedSeries) flattenSize {
	b.src = src
	switch src.desc.kind {
	case kindHistogram:
		return b.sizeHistogram()
	case kindSummary:
		return b.sizeSummary()
	case kindStateSet:
		return b.sizeStateSet()
	case kindMeasureSet:
		return b.sizeMeasureSet()
	}
	return flattenSize{}
}

// mergedSets sizes the label backing of children that each add one label to src.
func (b *flattenBlock) mergedSets(children int) int {
	return children * (len(b.src.labels) + 1)
}

// Text sizing: each child writes, in order, its derived name (if any), its canonical
// labels key (if merged) and its series key. Sizes are exact except that replacing
// an existing source label shortens the key.

// labelsKeyLen is the canonical labels key length after adding one label.
func (b *flattenBlock) labelsKeyLen(key string, valueLen int) int {
	return len(b.src.labelsKey) + len(key) + valueLen + 2
}

// keyLen is the makeSeriesKey length for the source host scope.
func (b *flattenBlock) keyLen(nameLen, labelsKeyLen int) int {
	n := nameLen
	if b.src.hostScopeKey != "" {
		n += len(b.src.hostScopeKey) + 1
	}
	if labelsKeyLen > 0 {
		n += labelsKeyLen + 1
	}
	return n
}

// mergedChildLen sizes a child that adds one label and uses a name of the given length.
func (b *flattenBlock) mergedChildLen(nameLen int, key, value string) int {
	labelsKey := b.labelsKeyLen(key, len(value))
	return labelsKey + b.keyLen(nameLen, labelsKey)
}

// scalarChildLen sizes a count/sum child: its name and its key over the source labels.
func (b *flattenBlock) scalarChildLen(nameLen int) int {
	return nameLen + b.keyLen(nameLen, len(b.src.labelsKey))
}

func (b *flattenBlock) take(start int) string {
	return b.text.String()[start:]
}

func (b *flattenBlock) joinName(sep, suffix string) string {
	start := b.text.Len()
	b.text.WriteString(b.src.name)
	b.text.WriteString(sep)
	b.text.WriteString(suffix)
	return b.take(start)
}

// seriesKey writes makeSeriesKey(src.hostScopeKey, name, labelsKey).
func (b *flattenBlock) seriesKey(name, labelsKey string) string {
	start := b.text.Len()
	if b.src.hostScopeKey != "" {
		b.text.WriteString(b.src.hostScopeKey)
		b.text.WriteByte('\xff')
	}
	b.text.WriteString(name)
	if labelsKey != "" {
		b.text.WriteByte('\xfe')
		b.text.WriteString(labelsKey)
	}
	return b.take(start)
}

func (b *flattenBlock) mergeLabel(key, value string) ([]Label, string, bool) {
	start, textStart := len(b.labels), b.text.Len()
	labels, ok := appendCanonicalLabel(b.labels, &b.text, b.src.labels, Label{
		Key:   key,
		Value: value,
	})
	if !ok {
		return nil, "", false
	}
	b.labels = labels
	return labels[start:len(labels):len(labels)], b.take(textStart), true
}

func (b *flattenBlock) desc(kind metricKind, name string, meta MetricMeta) *instrumentDescriptor {
	b.descs = append(b.descs, instrumentDescriptor{
		name:      name,
		kind:      kind,
		mode:      b.src.desc.mode,
		freshness: b.src.desc.freshness,
		window:    b.src.desc.window,
		meta:      meta,
	})
	return &b.descs[len(b.descs)-1]
}

// child publishes one flattened series sharing the source host scope.
func (b *flattenBlock) child(
	name string,
	labels []Label,
	labelsKey, key string,
	desc *instrumentDescriptor,
) *committedSeries {
	b.series = append(b.series, committedSeries{
		id:           SeriesID(key),
		hash64:       seriesIDHash(SeriesID(key)),
		key:          key,
		name:         name,
		hostScopeKey: b.src.hostScopeKey,
		hostScope:    b.src.hostScope,
		labels:       labels,
		labelsKey:    labelsKey,
		desc:         desc,
	})
	series := &b.series[len(b.series)-1]
	b.dst.series[key] = series
	return series
}

func histogramFlattenable(src *committedSeries) bool {
	schema := src.desc.histogram
	// A cumulative length mismatch is a malformed snapshot; Histogram() also reports unavailable.
	return schema != nil && len(src.histogramCumulative) == len(schema.bounds)
}

func (b *flattenBlock) sizeHistogram() flattenSize {
	src := b.src
	if !histogramFlattenable(src) {
		return flattenSize{}
	}
	labels := src.desc.histogram.bucketLabels()
	bucketNameLen := len(src.name) + len("_bucket")
	text := bucketNameLen
	for _, value := range labels {
		text += b.mergedChildLen(bucketNameLen, HistogramBucketLabel, value)
	}
	text += b.scalarChildLen(len(src.name)+len("_count")) + b.scalarChildLen(len(src.name)+len("_sum"))
	return flattenSize{
		children: len(labels) + 2,
		descs:    3,
		labels:   b.mergedSets(len(labels)),
		text:     text,
	}
}

func (b *flattenBlock) appendHistogram(src *committedSeries) {
	if !histogramFlattenable(src) {
		return
	}
	b.begin(src, b.size(src))
	schema := src.desc.histogram
	values := schema.bucketLabels()
	bucketName := b.joinName("", "_bucket")

	hasPrev := flattenedCounterDeltaSupported(src) && src.histogramHasPrev &&
		len(src.histogramPreviousCumulative) == len(schema.bounds)
	bucketDesc := b.desc(kindCounter, bucketName, src.desc.meta)
	bucketMeta := flattenedSeriesMeta(src.meta, MetricKindCounter, MetricKindHistogram, FlattenRoleHistogramBucket)
	prevCumulative := SampleValue(0)
	for i := range schema.bounds {
		cumulative := src.histogramCumulative[i]
		bucketValue := cumulative - prevCumulative
		prevCumulative = cumulative

		labels, labelsKey, ok := b.mergeLabel(HistogramBucketLabel, values[i])
		if !ok {
			continue
		}
		series := b.child(bucketName, labels, labelsKey, b.seriesKey(bucketName, labelsKey), bucketDesc)
		series.value = bucketValue
		series.meta = bucketMeta
		previous := SampleValue(0)
		if hasPrev {
			previous = src.histogramPreviousCumulative[i] - previousHistogramBucketFloor(src.histogramPreviousCumulative, i)
		}
		setFlattenedCounterState(
			series,
			bucketValue,
			previous,
			hasPrev,
			src.histogramCurrentSeq,
			src.histogramPreviousSeq,
		)
	}

	if labels, labelsKey, ok := b.mergeLabel(HistogramBucketLabel, values[len(schema.bounds)]); ok {
		series := b.child(bucketName, labels, labelsKey, b.seriesKey(bucketName, labelsKey), bucketDesc)
		series.value = src.histogramCount - prevCumulative
		series.meta = bucketMeta
		previous := SampleValue(0)
		if hasPrev {
			previous = src.histogramPreviousCount
			if len(src.histogramPreviousCumulative) > 0 {
				previous -= src.histogramPreviousCumulative[len(src.histogramPreviousCumulative)-1]
			}
		}
		setFlattenedCounterState(
			series,
			src.histogramCount-prevCumulative,
			previous,
			hasPrev,
			src.histogramCurrentSeq,
			src.histogramPreviousSeq,
		)
	}

	scalarPrev := flattenedCounterDeltaSupported(src) && src.histogramHasPrev
	b.scalar("_count", src.histogramCount, src.histogramPreviousCount, scalarPrev,
		flattenedSeriesMeta(src.meta, MetricKindCounter, MetricKindHistogram, FlattenRoleHistogramCount))
	b.scalar("_sum", src.histogramSum, src.histogramPreviousSum, scalarPrev,
		flattenedSeriesMeta(src.meta, MetricKindCounter, MetricKindHistogram, FlattenRoleHistogramSum))
}

// scalar publishes a histogram or summary count/sum child over the source labels.
func (b *flattenBlock) scalar(suffix string, value, previous SampleValue, hasPrev bool, meta SeriesMeta) {
	name := b.joinName("", suffix)
	src := b.src
	series := b.child(
		name,
		src.labels,
		src.labelsKey,
		b.seriesKey(name, src.labelsKey),
		b.desc(kindCounter, name, src.desc.meta),
	)
	series.value = value
	series.meta = meta
	series.counterNoResetFallback = flattenedCounterNoResetFallback(meta)
	setFlattenedCounterState(
		series,
		value,
		previous,
		hasPrev,
		flattenedCounterCurrentSeq(src, meta),
		flattenedCounterPreviousSeq(src, meta),
	)
}

func summaryQuantileCount(src *committedSeries) int {
	if src.desc.summary == nil {
		return 0
	}
	return len(src.desc.summary.quantiles)
}

func (b *flattenBlock) sizeSummary() flattenSize {
	src := b.src
	if !summaryQuantilesMatchSchema(src) {
		return flattenSize{}
	}
	quantiles := summaryQuantileCount(src)
	text := b.scalarChildLen(len(src.name)+len("_count")) + b.scalarChildLen(len(src.name)+len("_sum"))
	if quantiles > 0 {
		for _, value := range src.desc.summary.quantileLabels() {
			text += b.mergedChildLen(len(src.name), SummaryQuantileLabel, value)
		}
	}
	descs := 2
	if quantiles > 0 {
		descs++
	}
	return flattenSize{
		children: 2 + quantiles,
		descs:    descs,
		labels:   b.mergedSets(quantiles),
		text:     text,
	}
}

func (b *flattenBlock) appendSummary(src *committedSeries) {
	if !summaryQuantilesMatchSchema(src) {
		return
	}
	b.begin(src, b.size(src))

	scalarPrev := flattenedCounterDeltaSupported(src) && src.summaryHasPrev
	b.scalar("_count", src.summaryCount, src.summaryPreviousCount, scalarPrev,
		flattenedSeriesMeta(src.meta, MetricKindCounter, MetricKindSummary, FlattenRoleSummaryCount))
	b.scalar("_sum", src.summarySum, src.summaryPreviousSum, scalarPrev,
		flattenedSeriesMeta(src.meta, MetricKindCounter, MetricKindSummary, FlattenRoleSummarySum))

	if summaryQuantileCount(src) == 0 {
		return
	}
	quantileDesc := b.desc(kindGauge, src.name, src.desc.meta)
	quantileMeta := flattenedSeriesMeta(src.meta, MetricKindGauge, MetricKindSummary, FlattenRoleSummaryQuantile)
	for i, value := range src.desc.summary.quantileLabels() {
		labels, labelsKey, ok := b.mergeLabel(SummaryQuantileLabel, value)
		if !ok {
			continue
		}
		series := b.child(src.name, labels, labelsKey, b.seriesKey(src.name, labelsKey), quantileDesc)
		series.value = src.summaryQuantiles[i]
		series.meta = quantileMeta
	}
}

func summaryQuantilesMatchSchema(src *committedSeries) bool {
	schema := src.desc.summary
	if schema == nil {
		return len(src.summaryQuantiles) == 0
	}
	return len(src.summaryQuantiles) == len(schema.quantiles)
}

func previousHistogramBucketFloor(values []SampleValue, idx int) SampleValue {
	if idx == 0 {
		return 0
	}
	return values[idx-1]
}

func flattenedCounterDeltaSupported(src *committedSeries) bool {
	// Cycle-window histogram and summary samples are independent per cycle.
	return src.desc != nil && src.desc.window == WindowCumulative
}

func setFlattenedCounterState(series *committedSeries, current, previous SampleValue, hasPrev bool, currentSeq, previousSeq uint64) {
	series.counterCurrent = current
	series.counterCurrentSeq = currentSeq
	if !hasPrev {
		return
	}
	series.counterPrevious = previous
	series.counterPreviousSeq = previousSeq
	series.counterHasPrev = true
}

func flattenedCounterCurrentSeq(src *committedSeries, meta SeriesMeta) uint64 {
	switch meta.SourceKind {
	case MetricKindHistogram:
		return src.histogramCurrentSeq
	case MetricKindSummary:
		return src.summaryCurrentSeq
	default:
		return 0
	}
}

func flattenedCounterPreviousSeq(src *committedSeries, meta SeriesMeta) uint64 {
	switch meta.SourceKind {
	case MetricKindHistogram:
		return src.histogramPreviousSeq
	case MetricKindSummary:
		return src.summaryPreviousSeq
	default:
		return 0
	}
}

func flattenedCounterNoResetFallback(meta SeriesMeta) bool {
	return meta.FlattenRole == FlattenRoleHistogramSum || meta.FlattenRole == FlattenRoleSummarySum
}

func (b *flattenBlock) sizeStateSet() flattenSize {
	src := b.src
	schema := src.desc.stateSet
	if schema == nil {
		return flattenSize{}
	}
	text := 0
	for _, state := range schema.states {
		text += b.mergedChildLen(len(src.name), src.name, state)
	}
	return flattenSize{
		children: len(schema.states),
		descs:    1,
		labels:   b.mergedSets(len(schema.states)),
		text:     text,
	}
}

func (b *flattenBlock) appendStateSet(src *committedSeries) {
	schema := src.desc.stateSet
	if schema == nil {
		return
	}
	b.begin(src, b.size(src))
	stateDesc := b.desc(kindGauge, src.name, src.desc.meta)
	stateMeta := flattenedSeriesMeta(src.meta, MetricKindGauge, MetricKindStateSet, FlattenRoleStateSetState)
	for _, state := range schema.states {
		labels, labelsKey, ok := b.mergeLabel(src.name, state)
		if !ok {
			continue
		}
		series := b.child(src.name, labels, labelsKey, b.seriesKey(src.name, labelsKey), stateDesc)
		if src.stateSetValues[state] {
			series.value = 1
		}
		series.meta = stateMeta
	}
}

func measureSetFlattenable(src *committedSeries) bool {
	schema := src.desc.measureSet
	return schema != nil && len(src.measureSetValues) == len(schema.fields)
}

func (b *flattenBlock) sizeMeasureSet() flattenSize {
	src := b.src
	if !measureSetFlattenable(src) {
		return flattenSize{}
	}
	fields := src.desc.measureSet.fields
	text := 0
	for _, field := range fields {
		name := len(src.name) + 1 + len(field.Name)
		labelsKey := b.labelsKeyLen(MeasureSetFieldLabel, len(field.Name))
		text += name + labelsKey + b.keyLen(name, labelsKey)
	}
	return flattenSize{
		children: len(fields),
		descs:    len(fields),
		labels:   b.mergedSets(len(fields)),
		text:     text,
	}
}

func (b *flattenBlock) appendMeasureSet(src *committedSeries) {
	if !measureSetFlattenable(src) {
		return
	}
	b.begin(src, b.size(src))
	schema := src.desc.measureSet
	kind := MetricKindGauge
	descKind := kindGauge
	if schema.semantics == MeasureSetSemanticsCounter {
		kind = MetricKindCounter
		descKind = kindCounter
	}

	fieldMeta := flattenedSeriesMeta(src.meta, kind, MetricKindMeasureSet, FlattenRoleMeasureSetField)
	hasPrev := src.measureSetHasPrev && len(src.measureSetPreviousValues) == len(schema.fields)
	for i, field := range schema.fields {
		labels, labelsKey, ok := b.mergeLabel(MeasureSetFieldLabel, field.Name)
		if !ok {
			continue
		}
		name := b.joinName("_", field.Name)
		meta := src.desc.meta
		meta.Float = field.Float
		series := b.child(name, labels, labelsKey, b.seriesKey(name, labelsKey), b.desc(descKind, name, meta))
		series.value = src.measureSetValues[i]
		series.meta = fieldMeta
		if schema.semantics == MeasureSetSemanticsCounter {
			series.counterCurrent = src.measureSetValues[i]
			series.counterCurrentSeq = src.measureSetCurrentSeq
			if hasPrev {
				series.counterHasPrev = true
				series.counterPrevious = src.measureSetPreviousValues[i]
				series.counterPreviousSeq = src.measureSetPreviousSeq
			}
		}
	}
}
