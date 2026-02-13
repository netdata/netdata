// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"fmt"
	"hash/fnv"
	"math"
	"sort"
	"strconv"
)

const summaryQuantileLabel = "quantile"
const defaultSummaryReservoirSize = 1024
const initialSummaryReservoirCapacity = 64

// snapshotSummaryInstrument writes sampled full summary points.
type snapshotSummaryInstrument struct {
	backend meterBackend
	desc    *instrumentDescriptor
	base    []LabelSet
}

// statefulSummaryInstrument writes observed samples into maintained summary state.
type statefulSummaryInstrument struct {
	backend meterBackend
	desc    *instrumentDescriptor
	base    []LabelSet
}

// stagedSummary holds one in-cycle summary sample for a single series identity.
type stagedSummary struct {
	key            string
	name           string
	labels         []Label
	labelsKey      string
	desc           *instrumentDescriptor
	count          SampleValue
	sum            SampleValue
	quantileValues []SampleValue
	sketch         *summaryQuantileSketch
}

// Summary declares or reuses a snapshot summary under this meter.
func (m *snapshotMeter) Summary(name string, opts ...InstrumentOption) SnapshotSummary {
	desc, err := m.backend.registerInstrument(metricName(m.prefix, name), kindSummary, modeSnapshot, opts...)
	if err != nil {
		panic(err)
	}
	return &snapshotSummaryInstrument{
		backend: m.backend,
		desc:    desc,
		base:    appendLabelSets(m.sets, nil),
	}
}

// Summary declares or reuses a stateful summary under this meter.
func (m *statefulMeter) Summary(name string, opts ...InstrumentOption) StatefulSummary {
	desc, err := m.backend.registerInstrument(metricName(m.prefix, name), kindSummary, modeStateful, opts...)
	if err != nil {
		panic(err)
	}
	return &statefulSummaryInstrument{
		backend: m.backend,
		desc:    desc,
		base:    appendLabelSets(m.sets, nil),
	}
}

// ObservePoint writes one full summary point for this collect cycle.
func (s *snapshotSummaryInstrument) ObservePoint(p SummaryPoint, labels ...LabelSet) {
	s.backend.recordSummaryObservePoint(s.desc, p, appendLabelSets(s.base, labels))
}

// Observe adds one sample to a stateful summary for this collect cycle.
func (s *statefulSummaryInstrument) Observe(v SampleValue, labels ...LabelSet) {
	s.backend.recordSummaryObserve(s.desc, v, appendLabelSets(s.base, labels))
}

// recordSummaryObservePoint writes one full summary point into the active frame.
func (c *storeCore) recordSummaryObservePoint(desc *instrumentDescriptor, point SummaryPoint, sets []LabelSet) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.active == nil {
		panic(errCycleInactive)
	}

	labels, labelsKey, err := labelsFromSet(sets, c)
	if err != nil {
		panic(err)
	}
	if labelsContainKey(labels, summaryQuantileLabel) {
		panic(errSummaryLabelKey)
	}

	count, sum, quantiles := normalizeSummaryPoint(point, desc.summary)

	key := makeSeriesKey(desc.name, labelsKey)
	entry, ok := c.active.summaries[key]
	if !ok {
		entry = &stagedSummary{
			key:       key,
			name:      desc.name,
			labels:    labels,
			labelsKey: labelsKey,
			desc:      desc,
		}
		c.active.summaries[key] = entry
	}

	entry.count = count
	entry.sum = sum
	entry.quantileValues = append(entry.quantileValues[:0], quantiles...)
	entry.sketch = nil
}

// recordSummaryObserve adds one sample to a stateful summary in the active frame.
func (c *storeCore) recordSummaryObserve(desc *instrumentDescriptor, value SampleValue, sets []LabelSet) {
	mustFiniteSample(value)

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.active == nil {
		panic(errCycleInactive)
	}

	labels, labelsKey, err := labelsFromSet(sets, c)
	if err != nil {
		panic(err)
	}
	if labelsContainKey(labels, summaryQuantileLabel) {
		panic(errSummaryLabelKey)
	}

	key := makeSeriesKey(desc.name, labelsKey)
	entry, ok := c.active.summaries[key]
	if !ok {
		entry = &stagedSummary{
			key:       key,
			name:      desc.name,
			labels:    labels,
			labelsKey: labelsKey,
			desc:      desc,
		}
		if desc.window == WindowCumulative {
			if existing := c.snapshot.Load().series[key]; existing != nil && existing.desc != nil && existing.desc.kind == kindSummary {
				entry.count = existing.summaryCount
				entry.sum = existing.summarySum
				if len(desc.summaryQuantiles()) > 0 && existing.summarySketch != nil {
					entry.sketch = existing.summarySketch.clone()
				}
			}
		}
		if len(desc.summaryQuantiles()) > 0 && entry.sketch == nil {
			entry.sketch = newSummaryQuantileSketch(desc.summaryReservoirSize(), summarySketchSeed(key))
		}
		c.active.summaries[key] = entry
	}

	entry.count++
	entry.sum += value
	if len(desc.summaryQuantiles()) > 0 {
		entry.sketch.observe(value)
	}
}

func normalizeSummaryPoint(point SummaryPoint, schema *summarySchema) (SampleValue, SampleValue, []SampleValue) {
	mustFiniteSample(point.Count)
	mustFiniteSample(point.Sum)

	if point.Count < 0 {
		panic(fmt.Errorf("%w: negative count", errSummaryPoint))
	}

	if schema == nil || len(schema.quantiles) == 0 {
		if len(point.Quantiles) > 0 {
			panic(fmt.Errorf("%w: quantiles are not configured for this instrument", errSummaryPoint))
		}
		return point.Count, point.Sum, nil
	}

	if len(point.Quantiles) != len(schema.quantiles) {
		panic(fmt.Errorf("%w: quantile count mismatch", errSummaryPoint))
	}

	values := make([]SampleValue, len(schema.quantiles))
	seen := make(map[float64]struct{}, len(schema.quantiles))
	for _, q := range point.Quantiles {
		if math.IsNaN(q.Quantile) || q.Quantile < 0 || q.Quantile > 1 {
			panic(fmt.Errorf("%w: invalid quantile %v", errSummaryPoint, q.Quantile))
		}
		if _, ok := seen[q.Quantile]; ok {
			panic(fmt.Errorf("%w: duplicate quantile %v", errSummaryPoint, q.Quantile))
		}
		seen[q.Quantile] = struct{}{}

		idx := summaryQuantileIndex(schema.quantiles, q.Quantile)
		if idx == -1 {
			panic(fmt.Errorf("%w: quantile %v is not declared", errSummaryPoint, q.Quantile))
		}
		mustFiniteSample(q.Value)
		values[idx] = q.Value
	}

	if len(seen) != len(schema.quantiles) {
		panic(fmt.Errorf("%w: missing quantiles in point", errSummaryPoint))
	}

	return point.Count, point.Sum, values
}

func summaryQuantileIndex(schema []float64, q float64) int {
	i := sort.SearchFloat64s(schema, q)
	if i < len(schema) && schema[i] == q {
		return i
	}
	return -1
}

func nanSummaryQuantiles(quantiles []float64) []SampleValue {
	if len(quantiles) == 0 {
		return nil
	}
	out := make([]SampleValue, len(quantiles))
	for i := range out {
		out[i] = math.NaN()
	}
	return out
}

func formatSummaryQuantileLabel(v float64) string {
	return strconv.FormatFloat(v, 'g', -1, 64)
}

func (d *instrumentDescriptor) summaryQuantiles() []float64 {
	if d == nil || d.summary == nil {
		return nil
	}
	return d.summary.quantiles
}

func (d *instrumentDescriptor) summaryReservoirSize() int {
	if d == nil || d.summary == nil || d.summary.reservoirSize <= 0 {
		return defaultSummaryReservoirSize
	}
	return d.summary.reservoirSize
}

// summaryQuantileSketch keeps bounded-memory approximate quantiles.
// It uses reservoir sampling, which is deterministic per series key seed.
type summaryQuantileSketch struct {
	capacity int
	count    uint64
	rng      uint64
	values   []SampleValue
	scratch  []SampleValue
}

func newSummaryQuantileSketch(capacity int, seed uint64) *summaryQuantileSketch {
	if capacity <= 0 {
		capacity = defaultSummaryReservoirSize
	}
	if seed == 0 {
		seed = 1
	}
	initCap := capacity
	if initCap > initialSummaryReservoirCapacity {
		initCap = initialSummaryReservoirCapacity
	}
	return &summaryQuantileSketch{
		capacity: capacity,
		rng:      seed,
		values:   make([]SampleValue, 0, initCap),
	}
}

func (s *summaryQuantileSketch) clone() *summaryQuantileSketch {
	if s == nil {
		return nil
	}
	cp := *s
	cp.values = append([]SampleValue(nil), s.values...)
	cp.scratch = nil
	return &cp
}

func (s *summaryQuantileSketch) observe(v SampleValue) {
	// Not safe for concurrent use. Callers must hold the owning store mutex.
	s.count++
	if len(s.values) < s.capacity {
		s.values = append(s.values, v)
		return
	}

	j := s.next() % s.count
	if j < uint64(s.capacity) {
		s.values[j] = v
	}
}

func (s *summaryQuantileSketch) quantiles(targets []float64) []SampleValue {
	if len(targets) == 0 {
		return nil
	}

	out := make([]SampleValue, len(targets))
	if len(s.values) == 0 {
		for i := range out {
			out[i] = math.NaN()
		}
		return out
	}

	s.scratch = growCopy(s.scratch, s.values)
	sort.Float64s(s.scratch)
	for i, q := range targets {
		out[i] = sampleQuantileLinear(s.scratch, q)
	}
	return out
}

func (s *summaryQuantileSketch) next() uint64 {
	x := s.rng
	x ^= x << 13
	x ^= x >> 7
	x ^= x << 17
	s.rng = x
	return x
}

func growCopy(dst, src []SampleValue) []SampleValue {
	if cap(dst) < len(src) {
		dst = make([]SampleValue, len(src))
	} else {
		dst = dst[:len(src)]
	}
	copy(dst, src)
	return dst
}

func sampleQuantileLinear(sorted []SampleValue, q float64) SampleValue {
	last := len(sorted) - 1
	if q <= 0 {
		return sorted[0]
	}
	if q >= 1 {
		return sorted[last]
	}
	pos := q * float64(last)
	low := int(math.Floor(pos))
	high := int(math.Ceil(pos))
	if low == high {
		return sorted[low]
	}
	w := pos - float64(low)
	return sorted[low]*(1-w) + sorted[high]*w
}

func summarySketchSeed(key string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(key))
	seed := h.Sum64()
	if seed == 0 {
		return 1
	}
	return seed
}
