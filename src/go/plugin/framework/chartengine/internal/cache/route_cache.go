// SPDX-License-Identifier: GPL-3.0-or-later

package cache

import (
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

type routeCacheEntry[T any] struct {
	identity metrix.SeriesIdentity
	revision uint64
	values   []T
	// lastSeenBuild tracks last successful build sequence that observed this series.
	lastSeenBuild uint64
}

type RetainSeenStats struct {
	EntriesBefore int
	EntriesAfter  int
	Pruned        int
	FullDrop      bool
}

// RouteCache stores resolved routes keyed by metrix series identity.
//
// The cache is intentionally unbounded and is pruned by RetainSeen(), so
// lifecycle remains aligned with metrix retention/source-of-truth.
// Synchronization is intentionally delegated to chartengine, which serializes
// BuildPlan/Load state transitions under Engine.mu.
type RouteCache[T any] struct {
	buckets map[uint64][]routeCacheEntry[T]

	// seenBuild is the build sequence currently tracked by seenCount.
	seenBuild uint64
	// seenCount is number of cache entries marked seen in seenBuild.
	seenCount int
	// entryCount is total number of cache entries.
	entryCount int
}

func NewRouteCache[T any]() *RouteCache[T] {
	return &RouteCache[T]{
		buckets: make(map[uint64][]routeCacheEntry[T]),
	}
}

func (c *RouteCache[T]) Lookup(identity metrix.SeriesIdentity, revision uint64, buildSeq uint64) ([]T, bool) {
	bucket := c.buckets[identity.Hash64]
	for i := range bucket {
		if bucket[i].identity.ID != identity.ID {
			continue
		}
		if bucket[i].revision != revision {
			return nil, false
		}
		c.markSeen(&bucket[i], buildSeq)
		// Return immutable cached values directly to avoid per-lookup allocations.
		// Callers must treat the returned slice as read-only.
		return bucket[i].values, true
	}
	return nil, false
}

func (c *RouteCache[T]) MarkSeenIfPresent(identity metrix.SeriesIdentity, buildSeq uint64) {
	bucket := c.buckets[identity.Hash64]
	for i := range bucket {
		if bucket[i].identity.ID != identity.ID {
			continue
		}
		c.markSeen(&bucket[i], buildSeq)
		return
	}
}

func (c *RouteCache[T]) Store(identity metrix.SeriesIdentity, revision uint64, buildSeq uint64, values []T) {
	bucket := c.buckets[identity.Hash64]
	for i := range bucket {
		if bucket[i].identity.ID != identity.ID {
			continue
		}
		bucket[i].revision = revision
		bucket[i].values = cloneSlice(values)
		c.markSeen(&bucket[i], buildSeq)
		c.buckets[identity.Hash64] = bucket
		return
	}

	entry := routeCacheEntry[T]{
		identity:      identity,
		revision:      revision,
		values:        cloneSlice(values),
		lastSeenBuild: buildSeq,
	}
	c.buckets[identity.Hash64] = append(bucket, entry)
	c.entryCount++
	if c.seenBuild != buildSeq {
		c.seenBuild = buildSeq
		c.seenCount = 0
	}
	c.seenCount++
}

// RetainSeen keeps entries that were observed in current successful build sequence.
func (c *RouteCache[T]) RetainSeen(buildSeq uint64) RetainSeenStats {
	stats := RetainSeenStats{
		EntriesBefore: c.entryCount,
		EntriesAfter:  c.entryCount,
	}
	if c.entryCount == 0 {
		return stats
	}
	if c.seenBuild != buildSeq {
		// No cached entries were observed in this build; drop all.
		clear(c.buckets)
		c.entryCount = 0
		c.seenBuild = buildSeq
		c.seenCount = 0
		stats.EntriesAfter = 0
		stats.Pruned = stats.EntriesBefore
		stats.FullDrop = true
		return stats
	}
	if c.seenCount == c.entryCount {
		// Steady-state fast path: all cached entries were seen this build.
		return stats
	}

	keptTotal := 0
	for hash, bucket := range c.buckets {
		kept := retainSeenEntries(bucket, buildSeq)
		if len(kept) == 0 {
			delete(c.buckets, hash)
			continue
		}
		c.buckets[hash] = kept
		keptTotal += len(kept)
	}
	c.entryCount = keptTotal
	c.seenCount = keptTotal
	stats.EntriesAfter = keptTotal
	stats.Pruned = stats.EntriesBefore - keptTotal
	return stats
}

func (c *RouteCache[T]) markSeen(entry *routeCacheEntry[T], buildSeq uint64) {
	if c.seenBuild != buildSeq {
		c.seenBuild = buildSeq
		c.seenCount = 0
	}
	if entry.lastSeenBuild == buildSeq {
		return
	}
	entry.lastSeenBuild = buildSeq
	c.seenCount++
}

func cloneSlice[T any](in []T) []T {
	if len(in) == 0 {
		return nil
	}
	out := make([]T, len(in))
	copy(out, in)
	return out
}
