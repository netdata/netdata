// SPDX-License-Identifier: GPL-3.0-or-later

package cache

import (
	"sync"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

type routeCacheEntry[T any] struct {
	identity metrix.SeriesIdentity
	revision uint64
	values   []T
}

// RouteCache stores resolved routes keyed by metrix series identity.
//
// The cache is intentionally unbounded and is pruned by RetainSeries(), so
// lifecycle remains aligned with metrix retention/source-of-truth.
type RouteCache[T any] struct {
	mu      sync.RWMutex
	buckets map[uint64][]routeCacheEntry[T]
}

func NewRouteCache[T any]() *RouteCache[T] {
	return &RouteCache[T]{
		buckets: make(map[uint64][]routeCacheEntry[T]),
	}
}

func (c *RouteCache[T]) Lookup(identity metrix.SeriesIdentity, revision uint64) ([]T, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	bucket := c.buckets[identity.Hash64]
	for i := range bucket {
		entry := bucket[i]
		if entry.identity.ID != identity.ID {
			continue
		}
		if entry.revision != revision {
			return nil, false
		}
		// Return immutable cached values directly to avoid per-lookup allocations.
		// Callers must treat the returned slice as read-only.
		return entry.values, true
	}
	return nil, false
}

func (c *RouteCache[T]) Store(identity metrix.SeriesIdentity, revision uint64, values []T) {
	c.mu.Lock()
	defer c.mu.Unlock()

	bucket := c.buckets[identity.Hash64]
	for i := range bucket {
		if bucket[i].identity.ID != identity.ID {
			continue
		}
		bucket[i].revision = revision
		bucket[i].values = cloneSlice(values)
		c.buckets[identity.Hash64] = bucket
		return
	}
	c.buckets[identity.Hash64] = append(bucket, routeCacheEntry[T]{
		identity: identity,
		revision: revision,
		values:   cloneSlice(values),
	})
}

// RetainSeries keeps entries only for currently retained series IDs.
func (c *RouteCache[T]) RetainSeries(alive map[metrix.SeriesID]struct{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(alive) == 0 {
		clear(c.buckets)
		return
	}
	for hash, bucket := range c.buckets {
		kept := retainAliveEntries(bucket, alive)
		if len(kept) == 0 {
			delete(c.buckets, hash)
			continue
		}
		c.buckets[hash] = kept
	}
}

func cloneSlice[T any](in []T) []T {
	if len(in) == 0 {
		return nil
	}
	out := make([]T, len(in))
	copy(out, in)
	return out
}
