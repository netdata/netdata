// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

func (r *runtimeStoreBackend) commitRuntimeWrite(apply func(old, next *readSnapshot, seq uint64, nowUnixNano int64)) {
	r.core.mu.Lock()
	defer r.core.mu.Unlock()

	oldSnap := r.core.snapshot.Load()
	next := &readSnapshot{
		collectMeta: oldSnap.collectMeta,
		series:      make(map[string]*committedSeries, 1),
		// byName index is built lazily by readers for runtime snapshots.
		byName:       nil,
		runtimeBase:  oldSnap,
		runtimeDepth: oldSnap.runtimeDepth + 1,
	}

	nowUnixNano := r.now().UnixNano()
	r.core.sequence++
	seq := r.core.sequence
	apply(oldSnap, next, seq, nowUnixNano)

	r.writesSinceCompaction++
	var evicted []string
	if r.shouldCompactRuntimeSnapshot(next) {
		next, evicted = r.compactRuntimeSnapshot(next, nowUnixNano)
		r.writesSinceCompaction = 0
	}
	for _, key := range evicted {
		delete(r.summarySketches, key)
	}

	next.collectMeta.LastAttemptSeq = seq
	next.collectMeta.LastAttemptStatus = CollectStatusSuccess
	next.collectMeta.LastSuccessSeq = seq
	r.core.snapshot.Store(next)
}

func runtimeEnsureSeriesMutable(old, next *readSnapshot, key, name, hostScopeKey string, hostScope HostScope, labels []Label, labelsKey string, desc *instrumentDescriptor) *committedSeries {
	series, _ := runtimeEnsureSeriesMutableWithClone(old, next, key, name, hostScopeKey, hostScope, labels, labelsKey, desc, committedSeriesCloneFull)
	return series
}

func runtimeEnsureHistogramSeriesMutable(old, next *readSnapshot, key, name, hostScopeKey string, hostScope HostScope, labels []Label, labelsKey string, desc *instrumentDescriptor) *committedSeries {
	series, previous := runtimeEnsureSeriesMutableWithClone(old, next, key, name, hostScopeKey, hostScope, labels, labelsKey, desc, committedSeriesCloneHistogramMutation)
	if previous != nil {
		rememberHistogramPreviousFrom(series, previous, desc)
	}
	return series
}

func runtimeEnsureSeriesMutableWithClone(old, next *readSnapshot, key, name, hostScopeKey string, hostScope HostScope, labels []Label, labelsKey string, desc *instrumentDescriptor, cloneKind committedSeriesCloneKind) (*committedSeries, *committedSeries) {
	series := next.series[key]
	if series != nil {
		ensureSeriesMeta(series.desc, &series.meta)
		return series, nil
	}
	if existing, ok := lookupSnapshotSeries(old, key); ok {
		series = cloneCommittedSeriesForKind(existing, cloneKind)
		ensureSeriesMeta(series.desc, &series.meta)
		next.series[key] = series
		return series, existing
	}
	series = newCommittedSeries(key, name, hostScopeKey, hostScope, labels, labelsKey, desc)
	next.series[key] = series
	return series, nil
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
		byName:       nil,
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
