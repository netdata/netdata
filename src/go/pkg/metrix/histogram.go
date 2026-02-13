// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"fmt"
	"math"
	"sort"
	"strconv"
)

const histogramBucketLabel = "le"

// snapshotHistogramInstrument writes sampled full histogram points.
type snapshotHistogramInstrument struct {
	backend meterBackend
	desc    *instrumentDescriptor
	base    []LabelSet
}

// statefulHistogramInstrument writes observed samples into maintained histogram state.
type statefulHistogramInstrument struct {
	backend meterBackend
	desc    *instrumentDescriptor
	base    []LabelSet
}

// stagedHistogram holds one in-cycle histogram sample for a single series identity.
type stagedHistogram struct {
	key        string
	name       string
	labels     []Label
	labelsKey  string
	desc       *instrumentDescriptor
	bounds     []float64
	count      SampleValue
	sum        SampleValue
	cumulative []SampleValue
}

// Histogram declares or reuses a snapshot histogram under this meter.
func (m *snapshotMeter) Histogram(name string, opts ...InstrumentOption) SnapshotHistogram {
	desc, err := m.backend.registerInstrument(metricName(m.prefix, name), kindHistogram, modeSnapshot, opts...)
	if err != nil {
		panic(err)
	}
	return &snapshotHistogramInstrument{
		backend: m.backend,
		desc:    desc,
		base:    appendLabelSets(m.sets, nil),
	}
}

// Histogram declares or reuses a stateful histogram under this meter.
func (m *statefulMeter) Histogram(name string, opts ...InstrumentOption) StatefulHistogram {
	desc, err := m.backend.registerInstrument(metricName(m.prefix, name), kindHistogram, modeStateful, opts...)
	if err != nil {
		panic(err)
	}
	return &statefulHistogramInstrument{
		backend: m.backend,
		desc:    desc,
		base:    appendLabelSets(m.sets, nil),
	}
}

// ObservePoint writes one full histogram point for this collect cycle.
func (h *snapshotHistogramInstrument) ObservePoint(p HistogramPoint, labels ...LabelSet) {
	h.backend.recordHistogramObservePoint(h.desc, p, appendLabelSets(h.base, labels))
}

// Observe adds one sample to a stateful histogram for this collect cycle.
func (h *statefulHistogramInstrument) Observe(v SampleValue, labels ...LabelSet) {
	h.backend.recordHistogramObserve(h.desc, v, appendLabelSets(h.base, labels))
}

// recordHistogramObservePoint writes one full histogram point into the active frame.
func (c *storeCore) recordHistogramObservePoint(desc *instrumentDescriptor, point HistogramPoint, sets []LabelSet) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.active == nil {
		panic(errCycleInactive)
	}

	labels, labelsKey, err := labelsFromSet(sets, c)
	if err != nil {
		panic(err)
	}
	if labelsContainKey(labels, histogramBucketLabel) {
		panic(errHistogramLabelKey)
	}

	schema := desc.histogram
	if schema == nil {
		// For snapshot histograms without explicit bounds, validate against
		// previously captured family schema (if available).
		schema = c.snapshotHistogramSchema[desc.name]
	}
	bounds, count, sum, cumulative := normalizeHistogramPoint(point, schema)

	key := makeSeriesKey(desc.name, labelsKey)
	entry, ok := c.active.histograms[key]
	if !ok {
		entry = &stagedHistogram{
			key:       key,
			name:      desc.name,
			labels:    labels,
			labelsKey: labelsKey,
			desc:      desc,
		}
		c.active.histograms[key] = entry
	}
	if len(entry.bounds) > 0 && !equalHistogramBounds(entry.bounds, bounds) {
		panic("metrix: histogram point schema mismatch within cycle")
	}
	entry.bounds = append(entry.bounds[:0], bounds...)
	entry.count = count
	entry.sum = sum
	entry.cumulative = append(entry.cumulative[:0], cumulative...)
}

// recordHistogramObserve adds one sample to a stateful histogram in the active frame.
func (c *storeCore) recordHistogramObserve(desc *instrumentDescriptor, value SampleValue, sets []LabelSet) {
	mustFiniteSample(value)

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.active == nil {
		panic(errCycleInactive)
	}

	schema := desc.histogram
	if schema == nil || len(schema.bounds) == 0 {
		panic(errHistogramBounds)
	}

	labels, labelsKey, err := labelsFromSet(sets, c)
	if err != nil {
		panic(err)
	}
	if labelsContainKey(labels, histogramBucketLabel) {
		panic(errHistogramLabelKey)
	}

	key := makeSeriesKey(desc.name, labelsKey)
	entry, ok := c.active.histograms[key]
	if !ok {
		entry = &stagedHistogram{
			key:        key,
			name:       desc.name,
			labels:     labels,
			labelsKey:  labelsKey,
			desc:       desc,
			bounds:     append([]float64(nil), schema.bounds...),
			cumulative: make([]SampleValue, len(schema.bounds)),
		}
		if desc.window == WindowCumulative {
			if existing := c.snapshot.Load().series[key]; existing != nil && existing.desc != nil && existing.desc.kind == kindHistogram {
				entry.count = existing.histogramCount
				entry.sum = existing.histogramSum
				entry.cumulative = append(entry.cumulative[:0], existing.histogramCumulative...)
				if len(entry.cumulative) < len(schema.bounds) {
					entry.cumulative = append(entry.cumulative, make([]SampleValue, len(schema.bounds)-len(entry.cumulative))...)
				}
			}
		}
		c.active.histograms[key] = entry
	}

	idx := findHistogramBucket(schema.bounds, value)
	if idx < len(entry.cumulative) {
		for i := idx; i < len(entry.cumulative); i++ {
			entry.cumulative[i]++
		}
	}
	entry.count++
	entry.sum += value
}

func normalizeHistogramPoint(point HistogramPoint, schema *histogramSchema) ([]float64, SampleValue, SampleValue, []SampleValue) {
	mustFiniteSample(point.Count)
	mustFiniteSample(point.Sum)

	if point.Count < 0 {
		panic(fmt.Errorf("%w: negative count", errHistogramPoint))
	}

	bounds := make([]float64, 0, len(point.Buckets))
	cumulative := make([]SampleValue, 0, len(point.Buckets))
	prevBound := math.Inf(-1)
	prevCount := SampleValue(0)
	for i, b := range point.Buckets {
		ub := b.UpperBound
		if math.IsNaN(ub) || math.IsInf(ub, -1) {
			panic(fmt.Errorf("%w: invalid upper bound", errHistogramPoint))
		}
		if math.IsInf(ub, +1) {
			if i != len(point.Buckets)-1 {
				panic(fmt.Errorf("%w: +Inf bucket must be last", errHistogramPoint))
			}
			// +Inf bucket is implicit.
			continue
		}
		if ub <= prevBound {
			panic(fmt.Errorf("%w: bounds must be strictly increasing", errHistogramPoint))
		}
		if b.CumulativeCount < prevCount {
			panic(fmt.Errorf("%w: cumulative bucket counts must be monotonic", errHistogramPoint))
		}
		if b.CumulativeCount < 0 {
			panic(fmt.Errorf("%w: cumulative bucket counts must be non-negative", errHistogramPoint))
		}
		mustFiniteSample(b.CumulativeCount)
		bounds = append(bounds, ub)
		cumulative = append(cumulative, b.CumulativeCount)
		prevBound = ub
		prevCount = b.CumulativeCount
	}
	if len(cumulative) > 0 && cumulative[len(cumulative)-1] > point.Count {
		panic(fmt.Errorf("%w: last cumulative bucket exceeds count", errHistogramPoint))
	}

	if schema != nil {
		if !equalHistogramBounds(schema.bounds, bounds) {
			panic(fmt.Errorf("%w: bucket bounds mismatch", errHistogramPoint))
		}
		bounds = append([]float64(nil), schema.bounds...)
	}

	return bounds, point.Count, point.Sum, cumulative
}

// findHistogramBucket returns the index of the bucket for value, or len(bounds) for +Inf.
func findHistogramBucket(bounds []float64, value float64) int {
	n := len(bounds)
	if n == 0 {
		return 0
	}
	if value <= bounds[0] {
		return 0
	}
	if value > bounds[n-1] {
		return n
	}
	if n < 35 {
		for i, b := range bounds {
			if value <= b {
				return i
			}
		}
		return n
	}
	return sort.SearchFloat64s(bounds, value)
}

func formatHistogramBucketLabel(v float64) string {
	if math.IsInf(v, +1) {
		return "+Inf"
	}
	return strconv.FormatFloat(v, 'g', -1, 64)
}
