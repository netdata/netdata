// SPDX-License-Identifier: GPL-3.0-or-later

package aggregate

import (
	"time"
)

func (s *series) evict(cutoff time.Time) {
	i := 0
	for i < len(s.ts) && s.ts[i].Before(cutoff) {
		i++
	}
	if i > 0 {
		s.ts = append(s.ts[:0], s.ts[i:]...)
		s.vals = append(s.vals[:0], s.vals[i:]...)
	}
}

func evictTimes(ts []time.Time, cutoff time.Time) []time.Time {
	i := 0
	for i < len(ts) && ts[i].Before(cutoff) {
		i++
	}
	return append(ts[:0], ts[i:]...)
}

func evictTsCounts(tc []tsCount, cutoff time.Time) []tsCount {
	i := 0
	for i < len(tc) && tc[i].t.Before(cutoff) {
		i++
	}
	return append(tc[:0], tc[i:]...)
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
