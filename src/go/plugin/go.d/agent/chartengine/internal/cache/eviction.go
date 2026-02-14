// SPDX-License-Identifier: GPL-3.0-or-later

package cache

import "github.com/netdata/netdata/go/plugins/pkg/metrix"

func retainAliveEntries[T any](
	bucket []routeCacheEntry[T],
	alive map[metrix.SeriesID]struct{},
) []routeCacheEntry[T] {
	kept := bucket[:0]
	for i := range bucket {
		if _, ok := alive[bucket[i].identity.ID]; !ok {
			continue
		}
		kept = append(kept, bucket[i])
	}
	return kept
}
