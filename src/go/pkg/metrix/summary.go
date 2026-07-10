// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"fmt"
	"math"
	"sort"
	"strconv"
)

const SummaryQuantileLabel = "quantile"
const defaultSummaryReservoirSize = 1024

// snapshotSummaryInstrument writes sampled full summary points.
type snapshotSummaryInstrument struct {
	backend meterBackend
	desc    *instrumentDescriptor
	scope   HostScope
	base    []LabelSet
}

// statefulSummaryInstrument writes observed samples into maintained summary state.
type statefulSummaryInstrument struct {
	backend meterBackend
	desc    *instrumentDescriptor
	scope   HostScope
	base    []LabelSet
}

// stagedSummary holds one in-cycle summary sample for a single series identity.
type stagedSummary struct {
	key          string
	name         string
	hostScopeKey string
	hostScope    HostScope
	labels       []Label
	labelsKey    string
	desc         *instrumentDescriptor

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
		scope:   m.scope,
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
		scope:   m.scope,
		base:    appendLabelSets(m.sets, nil),
	}
}

// ObservePoint writes one full summary point for this collect cycle.
func (s *snapshotSummaryInstrument) ObservePoint(p SummaryPoint, labels ...LabelSet) {
	s.backend.recordSummaryObservePoint(s.desc, s.scope, p, appendLabelSets(s.base, labels))
}

// Observe adds one sample to a stateful summary for this collect cycle.
func (s *statefulSummaryInstrument) Observe(v SampleValue, labels ...LabelSet) {
	s.backend.recordSummaryObserve(s.desc, s.scope, v, appendLabelSets(s.base, labels))
}

// recordSummaryObservePoint writes one full summary point into the active frame.
func (c *storeCore) recordSummaryObservePoint(desc *instrumentDescriptor, scope HostScope, point SummaryPoint, sets []LabelSet) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.active == nil {
		panic(errCycleInactive)
	}

	labels, labelsKey, err := labelsFromSet(sets, c)
	if err != nil {
		panic(err)
	}
	if labelsContainKey(labels, SummaryQuantileLabel) {
		panic(errSummaryLabelKey)
	}
	scope, ok := c.prepareHostScopeForWriteLocked(scope)
	if !ok {
		return
	}

	count, sum, quantiles := normalizeSummaryPoint(point, desc.summary)

	key := makeSeriesKey(scope.ScopeKey, desc.name, labelsKey)
	entry, ok := c.active.summaries[key]
	if ok && entry.desc != desc {
		canonical, proceed := c.reconcileSameKeyDesc(key, entry.desc, desc)
		if !proceed {
			return
		}
		entry.desc = canonical
	}
	if !ok {
		entry = &stagedSummary{
			key:          key,
			name:         desc.name,
			hostScopeKey: scope.ScopeKey,
			hostScope:    scope,
			labels:       labels,
			labelsKey:    labelsKey,
			desc:         desc,
		}
		c.active.summaries[key] = entry
	}

	entry.count = count
	entry.sum = sum
	entry.quantileValues = append(entry.quantileValues[:0], quantiles...)
	entry.sketch = nil
}

// recordSummaryObserve adds one sample to a stateful summary in the active frame.
func (c *storeCore) recordSummaryObserve(desc *instrumentDescriptor, scope HostScope, value SampleValue, sets []LabelSet) {
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
	if labelsContainKey(labels, SummaryQuantileLabel) {
		panic(errSummaryLabelKey)
	}
	scope, ok := c.prepareHostScopeForWriteLocked(scope)
	if !ok {
		return
	}

	key := makeSeriesKey(scope.ScopeKey, desc.name, labelsKey)
	entry, ok := c.active.summaries[key]
	if ok && entry.desc != desc {
		canonical, proceed := c.reconcileSameKeyDesc(key, entry.desc, desc)
		if !proceed {
			return
		}
		entry.desc = canonical
	}
	if !ok {
		entry = &stagedSummary{
			key:          key,
			name:         desc.name,
			hostScopeKey: scope.ScopeKey,
			hostScope:    scope,
			labels:       labels,
			labelsKey:    labelsKey,
			desc:         desc,
		}
		if desc.window == WindowCumulative {
			if existing := c.baselineSeriesForWrite(key, desc); existing != nil {
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
		// A summary may report a NaN quantile value (e.g. an empty observation window). Store it;
		// chartengine renders a non-finite dimension value as a gap (SETEMPTY). Reject only Inf.
		if math.IsInf(float64(q.Value), 0) {
			panic(fmt.Errorf("%w: infinite quantile value %v", errSummaryPoint, q.Value))
		}
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
