// SPDX-License-Identifier: GPL-3.0-or-later

package aggregate

import (
	"math"
	"sort"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
)

// navLoadName/navDCLName are series keys distinct from any beacon.Vitals
// name (document load time and DOMContentLoaded handler duration; neither is a CWV).
const (
	navLoadName = "__nav_load"
	navDCLName  = "__nav_dcl"
)

// Read distributions use fixed slots for the five supported vitals. Canonical
// ingestion still owns named observations; only query-owned values use slots.
const (
	lcpIndex = iota
	fcpIndex
	ttfbIndex
	inpIndex
	clsIndex
	vitalCount
)

var vitalNames = [vitalCount]string{beacon.LCP, beacon.FCP, beacon.TTFB, beacon.INP, beacon.CLS}

func vitalIndex(name string) (int, bool) {
	switch name {
	case beacon.LCP:
		return lcpIndex, true
	case beacon.FCP:
		return fcpIndex, true
	case beacon.TTFB:
		return ttfbIndex, true
	case beacon.INP:
		return inpIndex, true
	case beacon.CLS:
		return clsIndex, true
	default:
		return 0, false
	}
}

// thresholds are the CWV good/poor boundaries (good ≤ first, poor > second).
var thresholds = map[string][2]float64{
	beacon.LCP:  {2500, 4000},
	beacon.FCP:  {1800, 3000},
	beacon.TTFB: {800, 1800},
	beacon.INP:  {200, 500},
	beacon.CLS:  {0.1, 0.25},
}

// pctStats consumes an owned read distribution; navigation timing has no CWV rating.
func pctStats(vals []float64) VitalStats {
	sorted := vals
	sort.Float64s(sorted)
	return VitalStats{
		N:   len(sorted),
		P50: pctSorted(sorted, 0.50),
		P75: pctSorted(sorted, 0.75),
		P95: pctSorted(sorted, 0.95),
	}
}

// stats consumes an owned read distribution without copying its values again.
func stats(vital string, vals []float64) VitalStats {
	sorted := vals
	sort.Float64s(sorted)
	st := VitalStats{
		N:   len(sorted),
		P50: pctSorted(sorted, 0.50),
		P75: pctSorted(sorted, 0.75),
		P95: pctSorted(sorted, 0.95),
	}
	th := thresholds[vital]
	good, poor := 0, 0
	for _, v := range sorted {
		switch {
		case v <= th[0]:
			good++
		case v > th[1]:
			poor++
		}
	}
	n := len(sorted)
	// Counts combine only for aligned, complete and disjoint populations;
	// overlapping rolling windows must not be summed over time.
	st.Good = good
	st.Poor = poor
	st.NeedsImpr = n - good - poor
	return st
}

func pctSorted(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(math.Ceil(float64(len(sorted))*p)) - 1 // nearest rank
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}
