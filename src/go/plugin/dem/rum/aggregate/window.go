// SPDX-License-Identifier: GPL-3.0-or-later

package aggregate

import (
	"time"
)

// Receipts can arrive out of order when concurrent HTTP body reads finish.
// Filter in place and preserve timestamp/value pairing and ingestion order.
func (s *series) evict(cutoff time.Time) {
	n := 0
	for i, ts := range s.ts {
		if !ts.Before(cutoff) {
			if n != i {
				s.ts[n], s.vals[n] = ts, s.vals[i]
			}
			n++
		}
	}
	s.ts, s.vals = s.ts[:n], s.vals[:n]
}

func evictTimes(ts []time.Time, cutoff time.Time) []time.Time {
	n := 0
	for i, t := range ts {
		if !t.Before(cutoff) {
			if n != i {
				ts[n] = t
			}
			n++
		}
	}
	return ts[:n]
}

func evictTsCounts(tc []tsCount, cutoff time.Time) []tsCount {
	n := 0
	for i, c := range tc {
		if !c.t.Before(cutoff) {
			if n != i {
				tc[n] = c
			}
			n++
		}
	}
	return tc[:n]
}

// countTsCounts sums n for entries at/after cutoff, without mutating tc
// (used by read-only FUNCTION accessors that must not disturb chart state).
func countTsCounts(tc []tsCount, cutoff time.Time) int {
	var n uint64
	for _, c := range tc {
		if !c.t.Before(cutoff) {
			n += c.n
		}
	}
	return int(n)
}

// countTimes counts entries at/after cutoff without mutating ts.
func countTimes(ts []time.Time, cutoff time.Time) int {
	n := 0
	for _, t := range ts {
		if !t.Before(cutoff) {
			n++
		}
	}
	return n
}
