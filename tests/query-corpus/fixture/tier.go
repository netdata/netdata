// SPDX-License-Identifier: GPL-3.0-or-later

package fixture

import (
	"math"
	"strconv"
	"strings"
)

// TierPoint is the oracle's view of one tier rollup window of one dimension,
// mirroring store_metric_at_tier (src/database/rrddim-collection.c) and the
// tier page slot storage_number_tier1_t — float32 sum/min/max with exact
// uint16 count/anomaly_count (src/libnetdata/storage_number/storage_number.h,
// src/database/engine/page.c write/read paths). Sum/Min/Max already carry
// the single float32 write-rounding; EndT is the wall-clock-aligned window
// end, which is the tier point's stored timestamp.
type TierPoint struct {
	EndT         int64
	Sum          float64
	Min          float64
	Max          float64
	Count        int
	AnomalyCount int
	Reset        bool // a reset sample fell in the window — LOST at tier1+ (pages store no flags)
	Empty        bool // stored, but every sample in the window was a gap (NAN/count-0 point)
}

// TierWindows rolls the dimension's points into tier windows of the given
// granularity (chart update_every × tier grouping, in seconds), keyed by the
// aligned window end. Windows the engine never stores — whole-chart gaps
// where no sample exists at all — are absent from the map; windows whose
// samples are all gaps are present with Empty set (the engine stores a
// NAN/count-0 point for them).
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
func (d Dimension) TierWindows(granularity int64) map[int64]TierPoint {
	out := make(map[int64]TierPoint)
	for _, p := range d.Points {
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
		if !strings.ContainsRune(p.Flags, 'E') {
			if v, err := strconv.ParseFloat(p.Collected, 64); err == nil {
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
				if strings.ContainsRune(p.Flags, 'R') {
					tp.Reset = true
				}
			}
		}
		out[end] = tp
	}

	// accumulation happens in double; ONE float32 cast per field at page write
	for end, tp := range out {
		if !tp.Empty {
			tp.Sum = float64(float32(tp.Sum))
			tp.Min = float64(float32(tp.Min))
			tp.Max = float64(float32(tp.Max))
			out[end] = tp
		}
	}
	return out
}
