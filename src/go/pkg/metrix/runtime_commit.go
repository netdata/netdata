// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import "slices"

func (r *runtimeStoreBackend) commitRuntimeWrite(apply func(old, next *readSnapshot, seq uint64, nowUnixNano int64)) {
	r.core.mu.Lock()
	defer r.core.mu.Unlock()

	nowUnixNano := r.now().UnixNano()
	r.core.sequence++
	seq := r.core.sequence
	r.writesSinceCompaction++
	if r.batch != nil {
		apply(r.batch.runtimeBase, r.batch, seq, nowUnixNano)
		return
	}
	next := r.newRuntimeOverlay()
	apply(next.runtimeBase, next, seq, nowUnixNano)
	r.publishRuntimeOverlay(next, nowUnixNano)
}

func (r *runtimeStoreBackend) newRuntimeOverlay() *readSnapshot {
	oldSnap := r.core.snapshot.Load()
	return &readSnapshot{
		collectMeta:  oldSnap.collectMeta,
		runtimeBase:  oldSnap,
		runtimeDepth: oldSnap.runtimeDepth + 1,
	}
}

// publishRuntimeOverlay publishes next under r.core.mu, compacting the overlay chain when due.
func (r *runtimeStoreBackend) publishRuntimeOverlay(next *readSnapshot, nowUnixNano int64) {
	var evicted []string
	if r.shouldCompactRuntimeSnapshot(next) {
		next, evicted = r.compactRuntimeSnapshot(next, nowUnixNano)
		r.writesSinceCompaction = 0
	}
	for _, key := range evicted {
		delete(r.summarySketches, key)
	}

	seq := r.core.sequence
	next.collectMeta.LastAttemptSeq = seq
	next.collectMeta.LastAttemptStatus = CollectStatusSuccess
	next.collectMeta.LastSuccessSeq = seq
	r.core.snapshot.Store(next)
}

// writeBatch runs fn with writes collected into one overlay, published when fn ends.
func (r *runtimeStoreBackend) writeBatch(fn func()) {
	r.core.mu.Lock()
	if r.batch != nil {
		r.core.mu.Unlock()
		fn()
		return
	}
	r.batch = r.newRuntimeOverlay()
	if r.batchSeries > 1 {
		r.batch.series = make(map[string]*committedSeries, r.batchSeries)
	}
	r.core.mu.Unlock()

	defer func() {
		r.core.mu.Lock()
		defer r.core.mu.Unlock()
		next := r.batch
		r.batch = nil
		r.batchSeries = len(next.series)
		if next.runtimeOne == nil && len(next.series) == 0 {
			return
		}
		r.publishRuntimeOverlay(next, r.now().UnixNano())
	}()
	fn()
}

type runtimeMutableSeriesState struct {
	series   *committedSeries
	previous *committedSeries
	expired  bool
	// owned marks a series the pending overlay already holds: a batch wrote it before.
	owned bool
}

func (r *runtimeStoreBackend) runtimeEnsureSeriesMutable(old, next *readSnapshot, key, name, hostScopeKey string, hostScope HostScope, labels []Label, labelsKey string, desc *instrumentDescriptor, nowUnixNano int64) *committedSeries {
	state := r.runtimeEnsureSeriesMutableWithClone(old, next, key, name, hostScopeKey, hostScope, labels, labelsKey, desc, nowUnixNano, committedSeriesCloneFull)
	return state.series
}

func (r *runtimeStoreBackend) runtimeEnsureHistogramSeriesMutable(
	old, next *readSnapshot,
	key, name, hostScopeKey string,
	hostScope HostScope,
	labels []Label,
	labelsKey string,
	desc *instrumentDescriptor,
	nowUnixNano int64,
) *committedSeries {
	state := r.runtimeEnsureSeriesMutableWithClone(
		old,
		next,
		key,
		name,
		hostScopeKey,
		hostScope,
		labels,
		labelsKey,
		desc,
		nowUnixNano,
		committedSeriesCloneHistogramMutation,
	)
	switch {
	case state.previous != nil:
		rememberHistogramPreviousFrom(state.series, state.previous, desc)
	case state.owned:
		// The previous state an immediate write would see is the series' own. The write
		// updates its buckets in place, so previous keeps a copy.
		previous := *state.series
		previous.histogramCumulative = slices.Clone(previous.histogramCumulative)
		rememberHistogramPreviousFrom(state.series, &previous, desc)
	}
	return state.series
}

func (r *runtimeStoreBackend) runtimeEnsureSummarySeriesMutable(
	old, next *readSnapshot,
	key, name, hostScopeKey string,
	hostScope HostScope,
	labels []Label,
	labelsKey string,
	desc *instrumentDescriptor,
	nowUnixNano int64,
) (*committedSeries, bool) {
	state := r.runtimeEnsureSeriesMutableWithClone(
		old,
		next,
		key,
		name,
		hostScopeKey,
		hostScope,
		labels,
		labelsKey,
		desc,
		nowUnixNano,
		committedSeriesCloneSummaryOverwrite,
	)
	return state.series, state.expired
}

func (r *runtimeStoreBackend) runtimeEnsureSeriesMutableWithClone(
	old, next *readSnapshot,
	key, name, hostScopeKey string,
	hostScope HostScope,
	labels []Label,
	labelsKey string,
	desc *instrumentDescriptor,
	nowUnixNano int64,
	cloneKind committedSeriesCloneKind,
) runtimeMutableSeriesState {
	if series, ok := next.ownSeries(key); ok && series != nil {
		ensureSeriesMeta(series.desc, &series.meta)
		return runtimeMutableSeriesState{
			series: series,
			owned:  true,
		}
	}
	if existing, ok := lookupSnapshotSeries(old, key); ok {
		if r.runtimeSeriesExpired(existing, nowUnixNano) {
			series := newCommittedSeries(key, name, hostScopeKey, hostScope, labels, labelsKey, desc)
			next.putOwnSeries(key, series)
			return runtimeMutableSeriesState{
				series:  series,
				expired: true,
			}
		}
		series := cloneCommittedSeriesForKind(existing, cloneKind)
		ensureSeriesMeta(series.desc, &series.meta)
		next.putOwnSeries(key, series)
		return runtimeMutableSeriesState{
			series:   series,
			previous: existing,
		}
	}
	series := newCommittedSeries(key, name, hostScopeKey, hostScope, labels, labelsKey, desc)
	next.putOwnSeries(key, series)
	return runtimeMutableSeriesState{
		series: series,
	}
}

func (r *runtimeStoreBackend) runtimeSeriesExpired(series *committedSeries, nowUnixNano int64) bool {
	if r.retention.ttl <= 0 {
		return false
	}
	return series.runtimeLastSeenUnixNano <= nowUnixNano-int64(r.retention.ttl)
}

func (r *runtimeStoreBackend) shouldCompactRuntimeSnapshot(next *readSnapshot) bool {
	if r.compaction.maxOverlayDepth > 0 && next.runtimeDepth >= r.compaction.maxOverlayDepth {
		return true
	}
	if r.compaction.maxOverlayWrites > 0 && r.writesSinceCompaction >= r.compaction.maxOverlayWrites {
		return true
	}
	return false
}

func (r *runtimeStoreBackend) compactRuntimeSnapshot(snap *readSnapshot, nowUnixNano int64) (*readSnapshot, []string) {
	series := snapshotSeriesView(snap)
	evicted := applyRuntimeRetention(series, r.retention, nowUnixNano)
	return &readSnapshot{
		collectMeta:  snap.collectMeta,
		series:       series,
		runtimeBase:  nil,
		runtimeDepth: 0,
	}, evicted
}

func applyRuntimeRetention(series map[string]*committedSeries, policy runtimeRetentionPolicy, nowUnixNano int64) []string {
	var evicted []string

	if policy.ttl > 0 {
		cutoff := nowUnixNano - int64(policy.ttl)
		for key, s := range series {
			if s.runtimeLastSeenUnixNano <= cutoff {
				delete(series, key)
				evicted = append(evicted, key)
			}
		}
	}

	evictOldestSeries(series, policy.maxSeries, func(s *committedSeries) int64 {
		return s.runtimeLastSeenUnixNano
	}, func(key string) {
		evicted = append(evicted, key)
	})

	if len(evicted) == 0 {
		return nil
	}
	return evicted
}
