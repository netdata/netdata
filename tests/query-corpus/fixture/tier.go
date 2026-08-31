// SPDX-License-Identifier: GPL-3.0-or-later

package fixture

import (
	"math"
	"strings"
)

// TierPoint is the oracle's view of one persisted higher-tier rollup window
// of one dimension.
//
// Source: netdata/netdata @ 043f50ec075441010c1495250871d37a8ac69f8d
//   - alignment and completed-point lifecycle:
//     src/database/rrddim-collection.c:9-11,17-60,63-108
//   - the original collected double feeding every higher tier:
//     src/database/rrddim-collection.c:149-180
//   - page slot layout:
//     src/libnetdata/storage_number/storage_number.h:78-84
//   - float32 page write/read, which does not retain generic flags:
//     src/database/engine/page.c:954-967,1088-1099
//
// Sum/Min/Max already carry the single float32 page-write rounding. EndT is
// the wall-clock-aligned window end and stored timestamp. Count and GapCount
// partition the window's nominal source slots for a stable cadence.
type TierPoint struct {
	EndT         int64
	Sum          float64
	Min          float64
	Max          float64
	Count        int
	GapCount     int
	AnomalyCount int
	Empty        bool // stored, but every sample in the window was a gap (NAN/count-0 point)
}

// TierWindows rolls the dimension's points into tier windows of the given
// granularity (chart update_every × tier grouping, in seconds), keyed by the
// aligned window end. Windows the engine never stores — whole-chart gaps
// where no sample exists at all — are absent from the map; windows whose
// samples are all gaps are present with Empty set (the engine stores a
// NAN/count-0 point for them). updateEvery defines the nominal source-slot
// duration; this oracle deliberately does not model a cadence change inside a
// persisted rollup because the legacy page format cannot retain that history.
//
// Values are the ORIGINAL collected doubles: higher tiers aggregate the
// pre-quantization value (rrddim-collection.c builds the tier STORAGE_POINT
// from the collected double, not from the tier0 storage_number), unlike the
// tier0 oracle which applies SNRoundTrip.
// Point times must already sit on the absolute update_every grid
// (t % ue == 0): storage keeps off-grid sample times as pushed, but
// every query re-grids to absolute ue multiples (the update_every
// sweep's TestOffGridTimestamps pins this), so an oracle fed off-grid
// fixture times would key the windows wrong. On the aligned grid,
// window boundaries coincide with sample ends and end-assignment is
// exact.
func (d Dimension) TierWindows(granularity, updateEvery int64) map[int64]TierPoint {
	if granularity <= 0 || updateEvery <= 0 || granularity%updateEvery != 0 {
		panic("fixture: tier granularity must be a positive multiple of update_every")
	}
	nominalSlots := int(granularity / updateEvery)
	out := make(map[int64]TierPoint)
	for _, p := range d.pointsInTimeOrder() {
		end := p.T
		if rem := end % granularity; rem != 0 {
			end += granularity - rem
		}
		tp, seen := out[end]
		if !seen {
			tp = TierPoint{EndT: end, Empty: true}
		}
		// gap samples advance the window but contribute nothing — not even
		// flags (the engine merges only non-NAN points into the virtual point)
		if v, collected := p.CollectedValue(d.ID); collected {
			if tp.Empty {
				tp.Sum, tp.Min, tp.Max = v, v, v
				tp.Empty = false
			} else {
				tp.Sum += v
				tp.Min = math.Min(tp.Min, v)
				tp.Max = math.Max(tp.Max, v)
			}
			tp.Count++
			if !strings.ContainsRune(p.Flags, 'A') {
				tp.AnomalyCount++
			}
		}
		out[end] = tp
	}

	// accumulation happens in double; ONE float32 cast per field at page write
	for end, tp := range out {
		tp.GapCount = nominalSlots - tp.Count
		if tp.GapCount < 0 {
			panic("fixture: tier point count exceeds its nominal slot count")
		}
		if !tp.Empty {
			tp.Sum = float64(float32(tp.Sum))
			tp.Min = float64(float32(tp.Min))
			tp.Max = float64(float32(tp.Max))
		}
		out[end] = tp
	}
	return out
}
