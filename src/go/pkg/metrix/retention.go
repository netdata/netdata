// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"cmp"
	"sort"
)

type retentionCandidate[T cmp.Ordered] struct {
	key      string
	lastSeen T
}

// evictOldestSeries enforces max-series cardinality with deterministic ordering.
// Oldest lastSeen values are evicted first; equal ages tie-break by series key.
func evictOldestSeries[T cmp.Ordered](
	series map[string]*committedSeries,
	maxSeries int,
	lastSeen func(*committedSeries) T,
	onEvict func(key string),
) {
	if maxSeries <= 0 || len(series) <= maxSeries {
		return
	}

	candidates := make([]retentionCandidate[T], 0, len(series))
	for key, s := range series {
		candidates = append(candidates, retentionCandidate[T]{
			key:      key,
			lastSeen: lastSeen(s),
		})
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].lastSeen != candidates[j].lastSeen {
			return candidates[i].lastSeen < candidates[j].lastSeen
		}
		return candidates[i].key < candidates[j].key
	})

	evictCount := len(series) - maxSeries
	for i := 0; i < evictCount; i++ {
		key := candidates[i].key
		delete(series, key)
		if onEvict != nil {
			onEvict(key)
		}
	}
}
