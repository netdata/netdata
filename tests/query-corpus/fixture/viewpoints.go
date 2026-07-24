// SPDX-License-Identifier: GPL-3.0-or-later

package fixture

import (
	"strconv"
	"strings"
)

// The virtual-points view oracle: a faithful port of
// rrd2rrdr_query_execute (query-execute.c) for the DEFAULT (virtual
// points) mode.
//
// Per output line ending at now_end:
//   - the OUTER fetch consumes db points ending STRICTLY BEFORE
//     now_end, adding each whole (when it ends after the line's
//     start); fetching shifts the three-point memory (last2 ← last1 ←
//     new ← fetched);
//   - the INNER step adds ONE boundary point: the pending fetched
//     point interpolated AT now_end via last1 when the line end cuts
//     it (query_interpolate_point: only when the point is wider than
//     1s, exactly adjacent to last1, and both numeric — the
//     interpolation happens on a COPY, so the point keeps its original
//     bounds as the next anchor); otherwise last1 re-interpolated via
//     last2 while the line still ends inside it; otherwise nothing (a
//     gap line).
// The time grouping then flushes per line over the values collected.

// DBPoint is one stored point as the query engine fetches it.
type DBPoint struct {
	Start, End int64
	Value      float64 // ignored when Gap
	Gap        bool    // a stored NAN/empty point
}

type vpPoint struct {
	start, end int64
	value      float64
	ok         bool
}

const vpUnset = int64(-1) << 62

func vpInterpolate(this, last vpPoint, now int64) float64 {
	if this.end-this.start > 1 && last.end == this.start && this.ok && last.ok {
		return last.value + (this.value-last.value)*
			(1.0-float64(this.end-now)/float64(this.end-this.start))
	}
	return this.value
}

// ViewBuckets slices the db point stream into the values each output
// line's time grouping receives: lines end at after+(i+1)*ueView. Gap
// points contribute nothing (the engine skips non-numeric values); a
// line collecting no values is an EMPTY bucket.
//
// Key consumption rule (matches the engine): only a FRESHLY FETCHED
// point ending strictly before the line end is added whole; a pending
// straddler contributes via boundary interpolation only, and the next
// fetch shifts it into the anchor slot WITHOUT re-adding it.
func ViewBuckets(points []DBPoint, after, ueView int64, lines int) [][]float64 {
	last2 := vpPoint{start: vpUnset, end: vpUnset}
	last1 := vpPoint{start: vpUnset, end: vpUnset}
	newP := vpPoint{start: vpUnset, end: vpUnset}
	haveNew := false
	idx := 0

	out := make([][]float64, lines)
	for line := range lines {
		nowEnd := after + int64(line+1)*ueView
		nowStart := nowEnd - ueView
		var bucket []float64

		// outer: fetch (with the three-point shift) until the pending
		// point reaches the line end; whole-add each fresh fetch that
		// ends inside the line
		for !haveNew || newP.end < nowEnd {
			if idx >= len(points) {
				haveNew = false
				break
			}
			p := points[idx]
			idx++
			last2 = last1
			last1 = newP
			newP = vpPoint{start: p.Start, end: p.End, value: p.Value, ok: !p.Gap}
			haveNew = true
			if newP.end < nowEnd && newP.end > nowStart && newP.ok {
				bucket = append(bucket, newP.value)
			}
		}

		// inner: the single boundary point for this line
		switch {
		case haveNew && nowEnd > newP.start:
			if newP.ok {
				bucket = append(bucket, vpInterpolate(newP, last1, nowEnd))
			}
		case last1.end != vpUnset && nowEnd <= last1.end:
			if last1.ok {
				bucket = append(bucket, vpInterpolate(last1, last2, nowEnd))
			}
		}

		out[line] = bucket
	}
	return out
}

// DBPoints converts a fixture dimension into its stored tier0 point
// stream: each sample covers (T-ue, T], SNRoundTrip'd; gap samples are
// stored NAN points on live charts.
func (d Dimension) DBPoints(ue int64) []DBPoint {
	out := make([]DBPoint, 0, len(d.Points))
	for _, p := range d.Points {
		dp := DBPoint{Start: p.T - ue, End: p.T}
		// gap detection matches the sibling oracles (fixture.go, tier.go):
		// any E-carrying flags mark the slot empty
		if v, err := strconv.ParseFloat(p.Collected, 64); err == nil && !strings.ContainsRune(p.Flags, 'E') {
			dp.Value = SNRoundTrip(v)
		} else {
			dp.Gap = true
		}
		out = append(out, dp)
	}
	return out
}
