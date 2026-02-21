// SPDX-License-Identifier: GPL-3.0-or-later

package cache

func retainSeenEntries[T any](
	bucket []routeCacheEntry[T],
	buildSeq uint64,
) []routeCacheEntry[T] {
	kept := bucket[:0]
	for i := range bucket {
		if bucket[i].lastSeenBuild != buildSeq {
			continue
		}
		kept = append(kept, bucket[i])
	}
	return kept
}
