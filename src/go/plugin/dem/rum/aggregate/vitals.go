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

// Read distributions use fixed slots for the five supported vitals, in
// beacon.Vitals order. Canonical ingestion still owns named observations; only
// query-owned values use slots.
const (
	lcpIndex = iota
	fcpIndex
	ttfbIndex
	inpIndex
	clsIndex
	vitalCount
)

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

// threshold holds one vital's Core Web Vitals boundaries. Promotion, chart
// ratings and page selectors all rate through it.
type threshold struct{ good, poor float64 }

// isGood reports a value at or below the good boundary.
func (t threshold) isGood(v float64) bool { return v <= t.good }

// isPoor reports a value above the poor boundary.
func (t threshold) isPoor(v float64) bool { return v > t.poor }

// thresholds lists the supported vitals; values between the boundaries need improvement.
var thresholds = map[string]threshold{
	beacon.LCP:  {good: 2500, poor: 4000},
	beacon.FCP:  {good: 1800, poor: 3000},
	beacon.TTFB: {good: 800, poor: 1800},
	beacon.INP:  {good: 200, poor: 500},
	beacon.CLS:  {good: 0.1, poor: 0.25},
}

// pctStats consumes an owned read distribution, sorting it in place;
// navigation timing has no CWV rating.
func pctStats(vals []float64) VitalStats {
	sort.Float64s(vals)
	return VitalStats{
		N:   len(vals),
		P50: pctSorted(vals, 0.50),
		P75: pctSorted(vals, 0.75),
		P95: pctSorted(vals, 0.95),
	}
}

// stats adds ratings to the percentiles of an owned read distribution.
func stats(vital string, vals []float64) VitalStats {
	st := pctStats(vals)
	th := thresholds[vital]
	for _, v := range vals {
		switch {
		case th.isGood(v):
			st.Good++
		case th.isPoor(v):
			st.Poor++
		}
	}
	// Counts combine only for aligned, complete and disjoint populations;
	// overlapping rolling windows must not be summed over time.
	st.NeedsImpr = st.N - st.Good - st.Poor
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
