// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

type metricKind uint8

type metricMode uint8

const (
	kindGauge metricKind = iota
	kindCounter
	kindHistogram
	kindSummary
	kindStateSet
)

const (
	modeSnapshot metricMode = iota
	modeStateful
)

type instrumentDescriptor struct {
	name      string
	kind      metricKind
	mode      metricMode
	freshness FreshnessPolicy // visibility policy used by Read()
	window    MetricWindow
	histogram *histogramSchema // set for kindHistogram only
	summary   *summarySchema   // set for kindSummary only
	stateSet  *stateSetSchema  // set for kindStateSet only
	meta      MetricMeta
}

type histogramSchema struct {
	bounds []float64
}

type summarySchema struct {
	quantiles     []float64
	reservoirSize int
}

type stateSetSchema struct {
	mode   StateSetMode
	states []string
	index  map[string]struct{}
}

type committedSeries struct {
	id     SeriesID
	hash64 uint64
	key    string
	name   string
	// labels are immutable after series publish and can be safely shared across snapshots.
	labels    []Label
	labelsKey string
	desc      *instrumentDescriptor
	value     SampleValue // last committed sample value
	// Internal successful-cycle clock used only for retention aging.
	lastSeenSuccessCycle uint64
	// Internal runtime clock (unix nanos) used only by runtime retention.
	runtimeLastSeenUnixNano int64

	// Counter two-sample state (used by Delta()).
	counterCurrent     SampleValue
	counterPrevious    SampleValue
	counterHasPrev     bool
	counterCurrentSeq  uint64
	counterPreviousSeq uint64

	// Histogram current sample (used by Histogram()).
	histogramCount      SampleValue
	histogramSum        SampleValue
	histogramCumulative []SampleValue

	// Summary current sample (used by Summary()).
	summaryCount     SampleValue
	summarySum       SampleValue
	summaryQuantiles []SampleValue
	summarySketch    *summaryQuantileSketch // cumulative stateful quantile estimator

	// StateSet current sample (used by StateSet()).
	stateSetValues map[string]bool

	meta SeriesMeta
}

type readSnapshot struct {
	collectMeta CollectMeta
	series      map[string]*committedSeries   // key => series
	byName      map[string][]*committedSeries // metric name => stable ordered series list
	// runtimeBase links runtime snapshots in overlay mode (nil for materialized snapshots).
	runtimeBase *readSnapshot
	// runtimeDepth tracks overlay chain depth for runtime compaction heuristics.
	runtimeDepth int
}

type cycleFrame struct {
	seq        uint64
	gauges     map[string]*stagedGauge
	counters   map[string]*stagedCounter
	histograms map[string]*stagedHistogram
	summaries  map[string]*stagedSummary
	stateSet   map[string]*stagedStateSet
}

type storeCore struct {
	mu sync.RWMutex

	sequence    uint64
	successSeq  uint64
	active      *cycleFrame
	instruments map[string]*instrumentDescriptor // metric name => descriptor (mode/kind locked)
	// Captured schema for snapshot histograms declared without explicit bounds.
	// Accessed only under c.mu during cycle commit.
	snapshotHistogramSchema map[string]*histogramSchema // metric name => captured bounds
	retention               collectorRetentionPolicy

	snapshot atomic.Pointer[readSnapshot] // atomically swapped immutable read view
}

type collectorRetentionPolicy struct {
	expireAfterSuccessCycles uint64
	maxSeries                int
}

const (
	defaultCollectorExpireAfterSuccessCycles uint64 = 10
	defaultCollectorMaxSeries                       = 0 // disabled
)

type storeView struct {
	core *storeCore
}

type managedStore struct {
	core *storeCore
}

type storeCycleController struct {
	core *storeCore
}

// NewCollectorStore creates a collection store with staged writes and immutable read snapshots.
func NewCollectorStore() CollectorStore {
	core := &storeCore{
		instruments:             make(map[string]*instrumentDescriptor),
		snapshotHistogramSchema: make(map[string]*histogramSchema),
		retention: collectorRetentionPolicy{
			expireAfterSuccessCycles: defaultCollectorExpireAfterSuccessCycles,
			maxSeries:                defaultCollectorMaxSeries,
		},
	}
	core.snapshot.Store(&readSnapshot{
		collectMeta: CollectMeta{LastAttemptStatus: CollectStatusUnknown},
		series:      make(map[string]*committedSeries),
		byName:      make(map[string][]*committedSeries),
	})
	return &storeView{core: core}
}

// AsCycleManagedStore exposes runtime cycle control for stores created by NewCollectorStore.
// This is intended for runtime internals, not collector code.
func AsCycleManagedStore(s CollectorStore) (CycleManagedStore, bool) {
	switch v := s.(type) {
	case *managedStore:
		return v, true
	case *storeView:
		return &managedStore{core: v.core}, true
	default:
		return nil, false
	}
}

func (s *storeView) Read(opts ...ReadOption) Reader {
	cfg := resolveReadConfig(opts...)
	snap := s.core.snapshot.Load()
	if cfg.flatten {
		snap = flattenSnapshot(snap)
	}
	return &storeReader{snap: snap, raw: cfg.raw, flattened: cfg.flatten}
}

func (s *storeView) Write() Writer {
	return &writeView{backend: s.core}
}

func (s *managedStore) Read(opts ...ReadOption) Reader {
	return (&storeView{core: s.core}).Read(opts...)
}

func (s *managedStore) Write() Writer {
	return (&storeView{core: s.core}).Write()
}

func (s *managedStore) CycleController() CycleController {
	return &storeCycleController{core: s.core}
}

// BeginCycle opens a new staged frame for collection writes.
func (c *storeCycleController) BeginCycle() {
	c.core.mu.Lock()
	defer c.core.mu.Unlock()

	if c.core.active != nil {
		panic(errCycleActive)
	}

	c.core.sequence++
	c.core.active = &cycleFrame{
		seq:        c.core.sequence,
		gauges:     make(map[string]*stagedGauge),
		counters:   make(map[string]*stagedCounter),
		histograms: make(map[string]*stagedHistogram),
		summaries:  make(map[string]*stagedSummary),
		stateSet:   make(map[string]*stagedStateSet),
	}
}

// CommitCycleSuccess publishes staged writes into a new committed snapshot.
func (c *storeCycleController) CommitCycleSuccess() {
	c.core.mu.Lock()
	defer c.core.mu.Unlock()

	if c.core.active == nil {
		panic(errCycleMissing)
	}

	oldSnap := c.core.snapshot.Load()
	successSeq := c.core.successSeq + 1
	next := &readSnapshot{
		collectMeta: oldSnap.collectMeta,
		series:      make(map[string]*committedSeries, len(oldSnap.series)),
		byName:      nil,
	}

	for k, s := range oldSnap.series {
		next.series[k] = s
	}

	for key, staged := range c.core.active.gauges {
		series := getOrCreateCommitSeries(oldSnap, next, key, staged.name, staged.labels, staged.labelsKey, staged.desc)
		series.value = staged.value
		markSeriesSeen(series, c.core.active.seq, successSeq)
	}

	for key, staged := range c.core.active.counters {
		series := getOrCreateCommitSeries(oldSnap, next, key, staged.name, staged.labels, staged.labelsKey, staged.desc)

		hadCurrent := series.desc != nil && series.desc.kind == kindCounter && series.counterCurrentSeq > 0
		if hadCurrent {
			series.counterPrevious = series.counterCurrent
			series.counterPreviousSeq = series.counterCurrentSeq
			series.counterHasPrev = true
		} else {
			series.counterPrevious = 0
			series.counterPreviousSeq = 0
			series.counterHasPrev = false
		}

		series.counterCurrent = staged.current
		series.counterCurrentSeq = c.core.active.seq
		series.value = staged.current // Value() for counters returns current total.
		markSeriesSeen(series, c.core.active.seq, successSeq)
	}

	for key, staged := range c.core.active.histograms {
		series := getOrCreateCommitSeries(oldSnap, next, key, staged.name, staged.labels, staged.labelsKey, staged.desc)

		if series.desc == nil {
			panic("metrix: missing histogram descriptor")
		}
		if series.desc.histogram == nil {
			schema := c.core.snapshotHistogramSchema[series.name]
			if schema == nil {
				schema = &histogramSchema{bounds: append([]float64(nil), staged.bounds...)}
				c.core.snapshotHistogramSchema[series.name] = schema
			} else if !equalHistogramBounds(schema.bounds, staged.bounds) {
				panic("metrix: histogram schema drift detected")
			}
			// Descriptor pointers can be shared across published snapshots.
			// Never mutate shared descriptor state in-place; attach schema via a cloned descriptor.
			series.desc = cloneInstrumentDescriptorWithHistogram(series.desc, schema.bounds)
		} else if !equalHistogramBounds(series.desc.histogram.bounds, staged.bounds) {
			panic("metrix: histogram schema drift detected")
		}

		series.histogramCount = staged.count
		series.histogramSum = staged.sum
		series.histogramCumulative = append(series.histogramCumulative[:0], staged.cumulative...)
		markSeriesSeen(series, c.core.active.seq, successSeq)
	}

	for key, staged := range c.core.active.summaries {
		series := getOrCreateCommitSeries(oldSnap, next, key, staged.name, staged.labels, staged.labelsKey, staged.desc)

		if staged.desc.mode == modeStateful && len(staged.desc.summaryQuantiles()) > 0 {
			if staged.sketch != nil {
				staged.quantileValues = staged.sketch.quantiles(staged.desc.summaryQuantiles())
			} else {
				// Defensive fallback for malformed staged state.
				staged.quantileValues = nanSummaryQuantiles(staged.desc.summaryQuantiles())
			}
		}

		series.summaryCount = staged.count
		series.summarySum = staged.sum
		if len(staged.quantileValues) > 0 {
			series.summaryQuantiles = append(series.summaryQuantiles[:0], staged.quantileValues...)
		} else {
			series.summaryQuantiles = nil
		}
		if staged.sketch != nil && series.desc != nil && series.desc.window == WindowCumulative {
			series.summarySketch = staged.sketch.clone()
		} else {
			series.summarySketch = nil
		}
		markSeriesSeen(series, c.core.active.seq, successSeq)
	}

	for key, staged := range c.core.active.stateSet {
		series := getOrCreateCommitSeries(oldSnap, next, key, staged.name, staged.labels, staged.labelsKey, staged.desc)

		series.stateSetValues = cloneStateMap(staged.states)
		markSeriesSeen(series, c.core.active.seq, successSeq)
	}

	applyCollectorRetention(next.series, c.core.retention, successSeq)
	next.collectMeta.LastAttemptSeq = c.core.active.seq
	next.collectMeta.LastAttemptStatus = CollectStatusSuccess
	next.collectMeta.LastSuccessSeq = c.core.active.seq

	c.core.snapshot.Store(next)
	c.core.successSeq = successSeq
	c.core.active = nil
}

func newCommittedSeries(key, name string, labels []Label, labelsKey string, desc *instrumentDescriptor) *committedSeries {
	return &committedSeries{
		id:        SeriesID(key),
		hash64:    seriesIDHash(SeriesID(key)),
		key:       key,
		name:      name,
		labels:    append([]Label(nil), labels...),
		labelsKey: labelsKey,
		desc:      desc,
		meta:      baseSeriesMeta(desc),
	}
}

func ensureCommitSeriesMutable(old, next *readSnapshot, key string) *committedSeries {
	series := next.series[key]
	if series == nil {
		return nil
	}
	if oldSeries, ok := old.series[key]; ok && oldSeries == series {
		series = cloneCommittedSeries(series)
		next.series[key] = series
	}
	ensureSeriesMeta(series.desc, &series.meta)
	return series
}

func getOrCreateCommitSeries(old, next *readSnapshot, key, name string, labels []Label, labelsKey string, desc *instrumentDescriptor) *committedSeries {
	series := ensureCommitSeriesMutable(old, next, key)
	if series != nil {
		return series
	}
	series = newCommittedSeries(key, name, labels, labelsKey, desc)
	next.series[key] = series
	return series
}

func markSeriesSeen(series *committedSeries, attemptSeq, successSeq uint64) {
	series.meta.LastSeenSuccessSeq = attemptSeq
	series.lastSeenSuccessCycle = successSeq
}

// AbortCycle discards staged writes and publishes metadata-only failed-attempt status.
func (c *storeCycleController) AbortCycle() {
	c.core.mu.Lock()
	defer c.core.mu.Unlock()

	if c.core.active == nil {
		panic(errCycleMissing)
	}

	oldSnap := c.core.snapshot.Load()
	// Alias previous committed maps directly. Safe by invariant:
	// committed series/snapshots are immutable after publish.
	abortSnap := &readSnapshot{
		collectMeta: oldSnap.collectMeta,
		series:      oldSnap.series,
		byName:      oldSnap.byName,
	}

	abortSnap.collectMeta.LastAttemptSeq = c.core.active.seq
	abortSnap.collectMeta.LastAttemptStatus = CollectStatusFailed
	c.core.snapshot.Store(abortSnap)

	c.core.active = nil
}

// buildByName builds deterministic per-name iteration lists for snapshot readers.
func buildByName(series map[string]*committedSeries) map[string][]*committedSeries {
	byName := make(map[string][]*committedSeries)
	for _, s := range series {
		if s.desc == nil || !isScalarKind(s.desc.kind) {
			continue
		}
		byName[s.name] = append(byName[s.name], s)
	}
	for _, lst := range byName {
		sort.Slice(lst, func(i, j int) bool {
			return lst[i].labelsKey < lst[j].labelsKey
		})
	}
	return byName
}

func applyCollectorRetention(series map[string]*committedSeries, policy collectorRetentionPolicy, successSeq uint64) {
	if policy.expireAfterSuccessCycles > 0 {
		for key, s := range series {
			seen := s.lastSeenSuccessCycle
			if seen == 0 || successSeq < seen {
				continue
			}
			if successSeq-seen >= policy.expireAfterSuccessCycles {
				delete(series, key)
			}
		}
	}

	evictOldestSeries(series, policy.maxSeries, func(s *committedSeries) uint64 {
		return s.lastSeenSuccessCycle
	}, nil)
}

func defaultFreshness(mode metricMode) FreshnessPolicy {
	if mode == modeSnapshot {
		return FreshnessCycle
	}
	return FreshnessCommitted
}

func (c *storeCore) registerInstrument(name string, kind metricKind, mode metricMode, opts ...InstrumentOption) (*instrumentDescriptor, error) {
	cfg := instrumentConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt.apply(&cfg)
		}
	}

	if cfg.windowSet && !isWindowAllowed(kind, mode) {
		return nil, fmt.Errorf("metrix: WithWindow is valid only for stateful histogram/summary")
	}
	if len(cfg.histogramBounds) > 0 && kind != kindHistogram {
		return nil, fmt.Errorf("metrix: histogram bounds are invalid for this instrument kind")
	}
	if len(cfg.summaryQuantile) > 0 && kind != kindSummary {
		return nil, fmt.Errorf("metrix: summary quantiles are invalid for this instrument kind")
	}
	if cfg.summaryReservoirSet && !(kind == kindSummary && mode == modeStateful) {
		return nil, fmt.Errorf("metrix: summary reservoir size is valid only for stateful summaries")
	}
	if (len(cfg.states) > 0 || cfg.stateSetMode != nil) && kind != kindStateSet {
		return nil, fmt.Errorf("metrix: stateset options are invalid for this instrument kind")
	}

	window := WindowCumulative
	if cfg.windowSet {
		window = cfg.window
	}

	fresh := defaultFreshness(mode)
	if cfg.freshnessSet {
		fresh = cfg.freshness
	}
	if mode == modeStateful && window == WindowCycle && (kind == kindHistogram || kind == kindSummary) {
		if cfg.freshnessSet && fresh != FreshnessCycle {
			return nil, fmt.Errorf("metrix: window=cycle requires FreshnessCycle")
		}
		fresh = FreshnessCycle
	}
	if mode == modeSnapshot && fresh == FreshnessCommitted {
		return nil, fmt.Errorf("metrix: snapshot instruments cannot use FreshnessCommitted")
	}

	metricMeta := MetricMeta{
		Description: strings.TrimSpace(cfg.description),
		ChartFamily: strings.TrimSpace(cfg.chartFamily),
		Unit:        strings.TrimSpace(cfg.unit),
	}

	var histogram *histogramSchema
	if kind == kindHistogram {
		s, err := buildHistogramSchema(cfg, mode)
		if err != nil {
			return nil, err
		}
		histogram = s
	}

	var summary *summarySchema
	if kind == kindSummary {
		s, err := buildSummarySchema(cfg)
		if err != nil {
			return nil, err
		}
		summary = s
	}

	var schema *stateSetSchema
	if kind == kindStateSet {
		s, err := buildStateSetSchema(cfg)
		if err != nil {
			return nil, err
		}
		schema = s
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if d, ok := c.instruments[name]; ok {
		if d.kind != kind {
			return nil, fmt.Errorf("metrix: instrument kind mismatch for %s", name)
		}
		if d.mode != mode {
			return nil, fmt.Errorf("metrix: instrument mode mismatch for %s", name)
		}
		if d.freshness != fresh {
			return nil, fmt.Errorf("metrix: instrument freshness mismatch for %s", name)
		}
		if d.window != window {
			return nil, fmt.Errorf("metrix: instrument window mismatch for %s", name)
		}
		if kind == kindHistogram {
			if !(mode == modeSnapshot && histogram == nil) && !equalHistogramSchema(d.histogram, histogram) {
				return nil, fmt.Errorf("metrix: histogram schema mismatch for %s", name)
			}
		}
		if kind == kindSummary && !equalSummarySchema(d.summary, summary) {
			return nil, fmt.Errorf("metrix: summary schema mismatch for %s", name)
		}
		if kind == kindStateSet && !equalStateSetSchema(d.stateSet, schema) {
			return nil, fmt.Errorf("metrix: stateset schema mismatch for %s", name)
		}
		if cfg.descriptionSet && d.meta.Description != metricMeta.Description {
			return nil, fmt.Errorf("metrix: metric description mismatch for %s", name)
		}
		if cfg.chartFamilySet && d.meta.ChartFamily != metricMeta.ChartFamily {
			return nil, fmt.Errorf("metrix: metric chart family mismatch for %s", name)
		}
		if cfg.unitSet && d.meta.Unit != metricMeta.Unit {
			return nil, fmt.Errorf("metrix: metric unit mismatch for %s", name)
		}
		return d, nil
	}

	d := &instrumentDescriptor{
		name:      name,
		kind:      kind,
		mode:      mode,
		freshness: fresh,
		window:    window,
		histogram: histogram,
		summary:   summary,
		stateSet:  schema,
		meta:      metricMeta,
	}
	c.instruments[name] = d
	return d, nil
}

// makeSeriesKey joins metric name and canonical label key into one stable identity key.
func makeSeriesKey(name, labelsKey string) string {
	if labelsKey == "" {
		return name
	}
	return name + "\xfe" + labelsKey
}

func cloneCommittedSeries(s *committedSeries) *committedSeries {
	cp := *s
	ensureSeriesMeta(cp.desc, &cp.meta)
	// cp.labels intentionally reuses the original immutable label slice.
	// Label identity is part of the series key and is never mutated after publish.
	if s.stateSetValues != nil {
		cp.stateSetValues = cloneStateMap(s.stateSetValues)
	}
	if len(s.histogramCumulative) > 0 {
		cp.histogramCumulative = append([]SampleValue(nil), s.histogramCumulative...)
	}
	if len(s.summaryQuantiles) > 0 {
		cp.summaryQuantiles = append([]SampleValue(nil), s.summaryQuantiles...)
	}
	if s.summarySketch != nil {
		cp.summarySketch = s.summarySketch.clone()
	}
	return &cp
}

func cloneInstrumentDescriptorWithHistogram(desc *instrumentDescriptor, bounds []float64) *instrumentDescriptor {
	cp := *desc
	cp.histogram = &histogramSchema{bounds: append([]float64(nil), bounds...)}
	return &cp
}

func cloneStateMap(in map[string]bool) map[string]bool {
	if in == nil {
		return nil
	}
	out := make(map[string]bool, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func isScalarKind(kind metricKind) bool {
	return kind == kindGauge || kind == kindCounter
}

func buildHistogramSchema(cfg instrumentConfig, mode metricMode) (*histogramSchema, error) {
	bounds, err := normalizeHistogramBounds(cfg.histogramBounds)
	if err != nil {
		return nil, err
	}
	if mode == modeStateful && len(bounds) == 0 {
		return nil, fmt.Errorf("%w for stateful histogram", errHistogramBounds)
	}
	if len(bounds) == 0 {
		return nil, nil
	}
	return &histogramSchema{bounds: bounds}, nil
}

func buildSummarySchema(cfg instrumentConfig) (*summarySchema, error) {
	if cfg.summaryReservoirSet && cfg.summaryReservoir <= 0 {
		return nil, fmt.Errorf("metrix: summary reservoir size must be > 0")
	}

	qs, err := normalizeSummaryQuantiles(cfg.summaryQuantile)
	if err != nil {
		return nil, err
	}

	if len(qs) == 0 {
		return nil, nil
	}

	size := defaultSummaryReservoirSize
	if cfg.summaryReservoirSet {
		size = cfg.summaryReservoir
	}

	return &summarySchema{
		quantiles:     qs,
		reservoirSize: size,
	}, nil
}

func buildStateSetSchema(cfg instrumentConfig) (*stateSetSchema, error) {
	if len(cfg.states) == 0 {
		return nil, fmt.Errorf("metrix: stateset requires WithStateSetStates")
	}

	mode := ModeBitSet
	if cfg.stateSetMode != nil {
		mode = *cfg.stateSetMode
	}

	seen := make(map[string]struct{}, len(cfg.states))
	states := make([]string, 0, len(cfg.states))
	for _, st := range cfg.states {
		if st == "" {
			return nil, fmt.Errorf("metrix: stateset state cannot be empty")
		}
		if _, ok := seen[st]; ok {
			return nil, fmt.Errorf("metrix: duplicate stateset state %q", st)
		}
		seen[st] = struct{}{}
		states = append(states, st)
	}

	return &stateSetSchema{
		mode:   mode,
		states: states,
		index:  seen,
	}, nil
}

func equalStateSetSchema(a, b *stateSetSchema) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.mode != b.mode || len(a.states) != len(b.states) {
		return false
	}
	for i := range a.states {
		if a.states[i] != b.states[i] {
			return false
		}
	}
	return true
}

func equalHistogramSchema(a, b *histogramSchema) bool {
	if a == nil || b == nil {
		return a == b
	}
	return equalHistogramBounds(a.bounds, b.bounds)
}

func equalSummarySchema(a, b *summarySchema) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.reservoirSize != b.reservoirSize || len(a.quantiles) != len(b.quantiles) {
		return false
	}
	for i := range a.quantiles {
		if a.quantiles[i] != b.quantiles[i] {
			return false
		}
	}
	return true
}

func equalHistogramBounds(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func normalizeHistogramBounds(in []float64) ([]float64, error) {
	if len(in) == 0 {
		return nil, nil
	}

	bounds := append([]float64(nil), in...)
	out := make([]float64, 0, len(bounds))
	prev := math.Inf(-1)
	for i, b := range bounds {
		if math.IsNaN(b) || math.IsInf(b, -1) {
			return nil, fmt.Errorf("%w: invalid upper bound", errHistogramPoint)
		}
		if math.IsInf(b, +1) {
			if i != len(bounds)-1 {
				return nil, fmt.Errorf("%w: +Inf bucket must be last", errHistogramPoint)
			}
			break // +Inf is implicit.
		}
		if b <= prev {
			return nil, fmt.Errorf("%w: bounds must be strictly increasing", errHistogramPoint)
		}
		out = append(out, b)
		prev = b
	}
	return out, nil
}

func normalizeSummaryQuantiles(in []float64) ([]float64, error) {
	if len(in) == 0 {
		return nil, nil
	}

	qs := append([]float64(nil), in...)
	prev := -1.0
	for _, q := range qs {
		if math.IsNaN(q) || q < 0 || q > 1 {
			return nil, fmt.Errorf("metrix: invalid summary quantile %v", q)
		}
		if q <= prev {
			return nil, fmt.Errorf("metrix: summary quantiles must be strictly increasing")
		}
		prev = q
	}
	return qs, nil
}

func isWindowAllowed(kind metricKind, mode metricMode) bool {
	return mode == modeStateful && (kind == kindHistogram || kind == kindSummary)
}
