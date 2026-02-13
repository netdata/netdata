// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"fmt"
	"sort"
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
	stateSet  *stateSetSchema // set for kindStateSet only
}

type stateSetSchema struct {
	mode   StateSetMode
	states []string
	index  map[string]struct{}
}

type committedSeries struct {
	id        SeriesID
	key       string
	name      string
	labels    []Label
	labelsKey string
	desc      *instrumentDescriptor
	value     SampleValue // last committed sample value

	// Counter two-sample state (used by Delta()).
	counterCurrent            SampleValue
	counterPrevious           SampleValue
	counterHasPrev            bool
	counterCurrentAttemptSeq  uint64
	counterPreviousAttemptSeq uint64

	// StateSet current sample (used by StateSet()).
	stateSetValues map[string]bool

	meta SeriesMeta
}

type readSnapshot struct {
	collectMeta CollectMeta
	series      map[string]*committedSeries   // key => series
	byName      map[string][]*committedSeries // metric name => stable ordered series list
}

type cycleFrame struct {
	seq      uint64
	gauges   map[string]*stagedGauge
	counters map[string]*stagedCounter
	stateSet map[string]*stagedStateSet
}

type storeCore struct {
	mu sync.RWMutex

	sequence    uint64
	active      *cycleFrame
	instruments map[string]*instrumentDescriptor // metric name => descriptor (mode/kind locked)

	snapshot atomic.Pointer[readSnapshot] // atomically swapped immutable read view
}

type storeView struct {
	core *storeCore
}

type managedStore struct {
	core *storeCore
}

type storeCycleController struct {
	core *storeCore
}

// NewStore creates a collection store with staged writes and immutable read snapshots.
func NewStore() Store {
	core := &storeCore{
		instruments: make(map[string]*instrumentDescriptor),
	}
	core.snapshot.Store(&readSnapshot{
		collectMeta: CollectMeta{LastAttemptStatus: CollectStatusUnknown},
		series:      make(map[string]*committedSeries),
		byName:      make(map[string][]*committedSeries),
	})
	return &storeView{core: core}
}

// AsCycleManagedStore exposes runtime cycle control for stores created by NewStore.
// This is intended for runtime internals, not collector code.
func AsCycleManagedStore(s Store) (CycleManagedStore, bool) {
	switch v := s.(type) {
	case *managedStore:
		return v, true
	case *storeView:
		return &managedStore{core: v.core}, true
	default:
		return nil, false
	}
}

func (s *storeView) Read() Reader {
	return &storeReader{snap: s.core.snapshot.Load(), raw: false}
}

func (s *storeView) ReadRaw() Reader {
	return &storeReader{snap: s.core.snapshot.Load(), raw: true}
}

func (s *storeView) Write() Writer {
	return &writeView{backend: s.core}
}

func (s *managedStore) Read() Reader {
	return (&storeView{core: s.core}).Read()
}

func (s *managedStore) ReadRaw() Reader {
	return (&storeView{core: s.core}).ReadRaw()
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
		seq:      c.core.sequence,
		gauges:   make(map[string]*stagedGauge),
		counters: make(map[string]*stagedCounter),
		stateSet: make(map[string]*stagedStateSet),
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
	next := &readSnapshot{
		collectMeta: oldSnap.collectMeta,
		series:      make(map[string]*committedSeries, len(oldSnap.series)),
		byName:      make(map[string][]*committedSeries),
	}

	for k, s := range oldSnap.series {
		next.series[k] = cloneCommittedSeries(s)
	}

	for key, staged := range c.core.active.gauges {
		series := next.series[key]
		if series == nil {
			series = &committedSeries{
				id:        SeriesID(key),
				key:       key,
				name:      staged.name,
				labels:    append([]Label(nil), staged.labels...),
				labelsKey: staged.labelsKey,
				desc:      staged.desc,
			}
			next.series[key] = series
		}
		series.value = staged.value
		series.meta.LastSeenSuccessSeq = c.core.active.seq
	}

	for key, staged := range c.core.active.counters {
		series := next.series[key]
		if series == nil {
			series = &committedSeries{
				id:        SeriesID(key),
				key:       key,
				name:      staged.name,
				labels:    append([]Label(nil), staged.labels...),
				labelsKey: staged.labelsKey,
				desc:      staged.desc,
			}
			next.series[key] = series
		}

		hadCurrent := series.desc != nil && series.desc.kind == kindCounter && series.counterCurrentAttemptSeq > 0
		if hadCurrent {
			series.counterPrevious = series.counterCurrent
			series.counterPreviousAttemptSeq = series.counterCurrentAttemptSeq
			series.counterHasPrev = true
		} else {
			series.counterPrevious = 0
			series.counterPreviousAttemptSeq = 0
			series.counterHasPrev = false
		}

		series.counterCurrent = staged.current
		series.counterCurrentAttemptSeq = c.core.active.seq
		series.value = staged.current // Value() for counters returns current total.
		series.meta.LastSeenSuccessSeq = c.core.active.seq
	}

	for key, staged := range c.core.active.stateSet {
		series := next.series[key]
		if series == nil {
			series = &committedSeries{
				id:        SeriesID(key),
				key:       key,
				name:      staged.name,
				labels:    append([]Label(nil), staged.labels...),
				labelsKey: staged.labelsKey,
				desc:      staged.desc,
			}
			next.series[key] = series
		}

		series.stateSetValues = cloneStateMap(staged.states)
		series.meta.LastSeenSuccessSeq = c.core.active.seq
	}

	next.collectMeta.LastAttemptSeq = c.core.active.seq
	next.collectMeta.LastAttemptStatus = CollectStatusSuccess
	next.collectMeta.LastSuccessSeq = c.core.active.seq
	next.byName = buildByName(next.series)

	c.core.snapshot.Store(next)
	c.core.active = nil
}

// AbortCycle discards staged writes and publishes metadata-only failed-attempt status.
func (c *storeCycleController) AbortCycle() {
	c.core.mu.Lock()
	defer c.core.mu.Unlock()

	if c.core.active == nil {
		panic(errCycleMissing)
	}

	oldSnap := c.core.snapshot.Load()
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
	if (len(cfg.states) > 0 || cfg.stateSetMode != nil) && kind != kindStateSet {
		return nil, fmt.Errorf("metrix: stateset options are invalid for this instrument kind")
	}

	fresh := defaultFreshness(mode)
	if cfg.freshnessSet {
		fresh = cfg.freshness
	}
	if mode == modeSnapshot && fresh == FreshnessCommitted {
		return nil, fmt.Errorf("metrix: snapshot instruments cannot use FreshnessCommitted")
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
		if kind == kindStateSet && !equalStateSetSchema(d.stateSet, schema) {
			return nil, fmt.Errorf("metrix: stateset schema mismatch for %s", name)
		}
		return d, nil
	}

	d := &instrumentDescriptor{name: name, kind: kind, mode: mode, freshness: fresh, stateSet: schema}
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
	if len(s.labels) > 0 {
		cp.labels = append([]Label(nil), s.labels...)
	}
	if s.stateSetValues != nil {
		cp.stateSetValues = cloneStateMap(s.stateSetValues)
	}
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

func isWindowAllowed(kind metricKind, mode metricMode) bool {
	return mode == modeStateful && (kind == kindHistogram || kind == kindSummary)
}
