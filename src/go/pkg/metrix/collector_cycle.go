// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"errors"
	"fmt"
	"maps"
)

// BeginCycle opens a new staged frame for collection writes.
func (c *storeCycleController) BeginCycle() {
	c.core.mu.Lock()
	defer c.core.mu.Unlock()

	if c.core.active != nil {
		panic(errCycleActive)
	}

	c.core.sequence++
	c.core.active = &cycleFrame{
		seq:                c.core.sequence,
		hostScopes:         make(map[string]HostScope),
		gauges:             make(map[string]*stagedGauge),
		counters:           make(map[string]*stagedCounter),
		histograms:         make(map[string]*stagedHistogram),
		summaries:          make(map[string]*stagedSummary),
		stateSet:           make(map[string]*stagedStateSet),
		measureSetGauges:   make(map[string]*stagedMeasureSet),
		measureSetCounters: make(map[string]*stagedMeasureSet),
		pendingInstruments: make(map[string][]*instrumentDescriptor),
	}
}

// abortWithError republishes the previously committed series as a failed attempt,
// clears the active cycle, and returns err. All staged writes and staged
// registrations from the aborted cycle are discarded (never merged).
func (c *storeCycleController) abortWithError(oldSnap *readSnapshot, err error) error {
	abortSnap := &readSnapshot{
		collectMeta: oldSnap.collectMeta,
		series:      oldSnap.series,
		byName:      oldSnap.byName,
	}
	abortSnap.collectMeta.LastAttemptSeq = c.core.active.seq
	abortSnap.collectMeta.LastAttemptStatus = CollectStatusFailed
	c.core.snapshot.Store(abortSnap)
	c.core.active = nil
	return err
}

// dropStagedNames removes all staged writes and staged registrations for the given set
// of names from the frame in a single pass per staged map, so ambiguous multi-kind names
// are ignored for the cycle without failing the commit - O(touched), not O(dropped*touched).
func (f *cycleFrame) dropStagedNames(names map[string]struct{}) {
	in := func(name string) bool { _, ok := names[name]; return ok }
	maps.DeleteFunc(f.gauges, func(_ string, s *stagedGauge) bool { return in(s.name) })
	maps.DeleteFunc(f.counters, func(_ string, s *stagedCounter) bool { return in(s.name) })
	maps.DeleteFunc(f.histograms, func(_ string, s *stagedHistogram) bool { return in(s.name) })
	maps.DeleteFunc(f.summaries, func(_ string, s *stagedSummary) bool { return in(s.name) })
	maps.DeleteFunc(f.stateSet, func(_ string, s *stagedStateSet) bool { return in(s.name) })
	maps.DeleteFunc(f.measureSetGauges, func(_ string, s *stagedMeasureSet) bool { return in(s.name) })
	maps.DeleteFunc(f.measureSetCounters, func(_ string, s *stagedMeasureSet) bool { return in(s.name) })
	for name := range names {
		delete(f.pendingInstruments, name)
	}
}

// CommitCycleSuccess publishes staged writes into a new committed snapshot.
func (c *storeCycleController) CommitCycleSuccess() error {
	c.core.mu.Lock()
	defer c.core.mu.Unlock()

	if c.core.active == nil {
		panic(errCycleMissing)
	}

	oldSnap := c.core.snapshot.Load()
	if c.core.active.err != nil {
		return c.abortWithError(oldSnap, c.core.active.err)
	}

	// Reconcile this cycle's observed descriptors against the committed registry
	// before publishing anything. Two incompatible kinds observed for one name in
	// the same cycle is unresolvable and fails the whole commit; because no committed
	// state has been mutated yet, that failure is transactional.
	resolution := c.core.resolveObservedDescriptors()
	if resolution.failErr != nil {
		return c.abortWithError(oldSnap, resolution.failErr)
	}
	successSeq := c.core.successSeq + 1
	next := &readSnapshot{
		collectMeta: oldSnap.collectMeta,
		series:      make(map[string]*committedSeries, len(oldSnap.series)),
		byName:      nil,
	}
	commitHostScopes := make(map[string]HostScope)

	maps.Copy(next.series, oldSnap.series)

	// Apply supersessions: a name whose committed kind was not observed this cycle is
	// replaced by the newly observed kind. Drop its carried-forward series and its
	// committed descriptor so the staged loops create fresh series for the new kind
	// (rather than mutating a series that still carries the old descriptor). One pass over
	// next.series regardless of how many names supersede - O(live), not O(superseded*live).
	if len(resolution.supersede) > 0 {
		supersedeSet := make(map[string]struct{}, len(resolution.supersede))
		for _, name := range resolution.supersede {
			supersedeSet[name] = struct{}{}
			delete(c.core.instruments, name)
			// Keep instrumentZeroSince a subset of instruments: if the new kind's series does
			// not survive this commit (e.g. maxSeries-evicted before canonicalization), the
			// name never re-enters instruments, so the sweep would never revisit and clear a
			// leftover idle stamp.
			delete(c.core.instrumentZeroSince, name)
		}
		maps.DeleteFunc(next.series, func(_ string, series *committedSeries) bool {
			_, ok := supersedeSet[series.name]
			return ok
		})
	}

	// Drop ambiguous multi-kind names: remove their staged writes and staged registration
	// so they are ignored this cycle (prior committed series carry forward untouched). One
	// pass per staged map - O(touched), not O(dropped*touched).
	if len(resolution.drop) > 0 {
		dropSet := make(map[string]struct{}, len(resolution.drop))
		for _, name := range resolution.drop {
			dropSet[name] = struct{}{}
		}
		c.core.active.dropStagedNames(dropSet)
	}

	for key, staged := range c.core.active.gauges {
		commitHostScopes[staged.hostScopeKey] = staged.hostScope
		series := getOrCreateCommitSeries(oldSnap, next, key, staged.name, staged.hostScopeKey, staged.hostScope, staged.labels, staged.labelsKey, staged.desc)
		series.value = staged.value
		markSeriesSeen(series, c.core.active.seq, successSeq)
	}

	for key, staged := range c.core.active.counters {
		commitHostScopes[staged.hostScopeKey] = staged.hostScope
		series := getOrCreateCommitSeries(oldSnap, next, key, staged.name, staged.hostScopeKey, staged.hostScope, staged.labels, staged.labelsKey, staged.desc)

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
		commitHostScopes[staged.hostScopeKey] = staged.hostScope
		// The canonical descriptor already carries concrete bounds (the resolver reduces a
		// nil-bounds observation to its observed bounds), and the resolver groups a name's
		// writes by effective bounds, so every staged write of an accepted name shares the
		// canonical bounds - no per-series schema capture or drift check is needed here.
		series := getOrCreateCommitSeries(oldSnap, next, key, staged.name, staged.hostScopeKey, staged.hostScope, staged.labels, staged.labelsKey, staged.desc)
		series.histogramCount = staged.count
		series.histogramSum = staged.sum
		series.histogramCumulative = append(series.histogramCumulative[:0], staged.cumulative...)
		markSeriesSeen(series, c.core.active.seq, successSeq)
	}

	for key, staged := range c.core.active.summaries {
		commitHostScopes[staged.hostScopeKey] = staged.hostScope
		series := getOrCreateCommitSeries(oldSnap, next, key, staged.name, staged.hostScopeKey, staged.hostScope, staged.labels, staged.labelsKey, staged.desc)

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
		commitHostScopes[staged.hostScopeKey] = staged.hostScope
		series := getOrCreateCommitSeries(oldSnap, next, key, staged.name, staged.hostScopeKey, staged.hostScope, staged.labels, staged.labelsKey, staged.desc)

		series.stateSetValues = cloneStateMap(staged.states)
		markSeriesSeen(series, c.core.active.seq, successSeq)
	}

	for key, staged := range c.core.active.measureSetGauges {
		commitHostScopes[staged.hostScopeKey] = staged.hostScope
		series := getOrCreateCommitSeries(oldSnap, next, key, staged.name, staged.hostScopeKey, staged.hostScope, staged.labels, staged.labelsKey, staged.desc)
		series.measureSetValues = append(series.measureSetValues[:0], staged.values...)
		markSeriesSeen(series, c.core.active.seq, successSeq)
	}

	for key, staged := range c.core.active.measureSetCounters {
		commitHostScopes[staged.hostScopeKey] = staged.hostScope
		series := getOrCreateCommitSeries(oldSnap, next, key, staged.name, staged.hostScopeKey, staged.hostScope, staged.labels, staged.labelsKey, staged.desc)

		if series.desc != nil && series.desc.kind == kindMeasureSet && series.desc.measureSet != nil && series.desc.measureSet.semantics == MeasureSetSemanticsCounter && series.measureSetCurrentSeq > 0 {
			series.measureSetPreviousValues = append(series.measureSetPreviousValues[:0], series.measureSetValues...)
			series.measureSetPreviousSeq = series.measureSetCurrentSeq
			series.measureSetHasPrev = true
		} else {
			series.measureSetPreviousValues = nil
			series.measureSetPreviousSeq = 0
			series.measureSetHasPrev = false
		}

		series.measureSetValues = append(series.measureSetValues[:0], staged.values...)
		series.measureSetCurrentSeq = c.core.active.seq
		markSeriesSeen(series, c.core.active.seq, successSeq)
	}

	refreshCommittedHostScopes(oldSnap, next, commitHostScopes)
	applyCollectorRetention(next.series, c.core.retention, successSeq)

	// Canonicalize the published snapshot in one pass, after retention: every surviving
	// series of a name observed this cycle gets the single canonical descriptor, and the
	// registry is set to match. This is the sole place descriptors are canonicalized, so
	// instruments[name], every series.desc (including carried, unobserved series of an
	// observed name), and the reader all agree regardless of label or iteration order.
	// Running after retention means an evicted-this-cycle name leaves no orphan entry.
	// Names with only carried series keep their committed entry; a defensive fill covers
	// any surviving series whose name somehow lacks one.
	// liveNames is collected here (folded into the existing scan) and consumed by the
	// descriptor-universe sweep below, so the sweep needs no separate pass over next.series.
	liveNames := make(map[string]struct{})
	for key := range next.series {
		series := next.series[key]
		liveNames[series.name] = struct{}{}
		canonical, ok := resolution.canonical[series.name]
		if !ok {
			if _, exists := c.core.instruments[series.name]; !exists {
				c.core.instruments[series.name] = series.desc
			}
			continue
		}
		// This pass scans every live series (O(live-series)), but only clones/rewrites a
		// series whose descriptor actually changes. With a no-op merge canonical == the
		// committed descriptor, so unchanged retained series are skipped: the clone/allocation
		// work stays O(touched), not O(retained).
		if series.desc != canonical {
			ensureCommitSeriesMutable(oldSnap, next, key).desc = canonical
		}
		if c.core.instruments[series.name] != canonical {
			c.core.instruments[series.name] = canonical
		}
	}

	// Bound descriptor growth: evict descriptors whose series are gone and whose grace has
	// elapsed. Runs after canonicalization so the live-name set is final.
	evicted := c.core.sweepDescriptorUniverse(liveNames, successSeq)

	next.collectMeta.LastAttemptSeq = c.core.active.seq
	next.collectMeta.LastAttemptStatus = CollectStatusSuccess
	next.collectMeta.LastSuccessSeq = c.core.active.seq
	next.collectMeta.EvictedDescriptors += evicted
	next.collectMeta.DroppedNames += uint64(len(resolution.drop))

	c.core.snapshot.Store(next)
	c.core.successSeq = successSeq
	c.core.active = nil
	return nil
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

func (c *storeCore) prepareHostScopeForWriteLocked(scope HostScope) (HostScope, bool) {
	scope = mustNormalizeHostScope(scope)
	if c.active == nil {
		return scope, true
	}
	if existing, ok := c.active.hostScopes[scope.ScopeKey]; ok {
		if hostScopeEqual(existing, scope) {
			return existing, true
		}
		c.recordCycleErrorLocked(fmt.Errorf("%w: scope_key=%q", ErrHostScopeConflict, scope.ScopeKey))
		return scope, false
	}
	c.active.hostScopes[scope.ScopeKey] = cloneHostScope(scope)
	return scope, true
}

func (c *storeCore) recordCycleErrorLocked(err error) {
	if err == nil || c.active == nil {
		return
	}
	c.active.err = errors.Join(c.active.err, err)
}
