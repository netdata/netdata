// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import "math"

// DescriptorRetentionUnbounded is the DescriptorRetentionWindow() value when series age-expiry
// is disabled (WithExpireAfterSuccessCycles(0)): a name's series - and thus its descriptor - can
// live indefinitely, so a consumer must retain its cached state for that name indefinitely too
// rather than aging it out on a finite window.
const DescriptorRetentionUnbounded uint64 = math.MaxUint64

// descriptorRetentionWindow is the number of successful commits a descriptor can outlive its
// last series: expire+grace. With age-expiry disabled (expire==0) a series - and thus its
// descriptor - can live indefinitely, so the window is unbounded; a grace so large that
// expire+grace overflows uint64 also saturates to unbounded. Both fields are fixed at
// construction, so no lock.
func (c *storeCore) descriptorRetentionWindow() uint64 {
	expire := c.retention.expireAfterSuccessCycles
	if expire == 0 {
		return DescriptorRetentionUnbounded
	}
	window := expire + c.retention.descriptorGraceCycles
	if window < expire { // uint64 overflow
		return DescriptorRetentionUnbounded
	}
	return window
}

// successfulCommits reads the success clock advanced under c.mu at commit.
func (c *storeCore) successfulCommits() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.successSeq
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

// sweepDescriptorUniverse bounds instruments growth. The descriptor universe is exactly
// the names in instruments (post-canonicalization every live name has an entry, and idle
// names linger there until swept). For each name:
//   - a live series clears any idle stamp;
//   - an idle name is stamped with successSeq the first cycle it goes idle, then evicted
//     once it has been idle for descriptorGraceCycles successful commits.
//
// It runs under c.mu at commit, after retention and canonicalization. Cost is O(universe) -
// one pass over instruments, within the commit envelope. Returns the number of descriptors
// evicted this cycle. instrumentZeroSince is lazily allocated (a store that never idles a
// name never allocates it). Deleting the current key while ranging a map is defined in Go.
func (c *storeCore) sweepDescriptorUniverse(liveNames map[string]struct{}, successSeq uint64) uint64 {
	grace := c.retention.descriptorGraceCycles
	var evicted uint64
	for name := range c.instruments {
		if _, live := liveNames[name]; live {
			delete(c.instrumentZeroSince, name) // no-op on a nil map
			continue
		}
		since, stamped := c.instrumentZeroSince[name]
		if !stamped {
			if c.instrumentZeroSince == nil {
				c.instrumentZeroSince = make(map[string]uint64)
			}
			c.instrumentZeroSince[name] = successSeq
			since = successSeq
		}
		if successSeq-since >= grace {
			delete(c.instruments, name)
			delete(c.instrumentZeroSince, name)
			evicted++
		}
	}
	return evicted
}
