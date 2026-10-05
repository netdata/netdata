// SPDX-License-Identifier: GPL-3.0-or-later

package aggregate

import (
	"math"
	"sort"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
)

// navLoadName/navDCLName are series keys distinct from any beacon.Vitals
// name (rum.load/rum.dcl, no CWV rating computed for either).
const (
	navLoadName = "__nav_load"
	navDCLName  = "__nav_dcl"
)

func (st *siteState) recordVitals(b *beacon.Beacon, kind, value string, now time.Time) {
	for _, v := range b.Vitals {
		st.addSample(seriesKey{v.Name, kind, value}, v.Value, now)
	}
}

// addSample appends one windowed sample to a series, dropping the oldest
// once the per-series cap is hit (shared by vitals and navigation timing).
func (st *siteState) addSample(k seriesKey, val float64, now time.Time) {
	s, ok := st.series[k]
	if !ok {
		s = &series{}
		st.series[k] = s
	}
	if len(s.vals) >= maxSamplesPerSeries {
		s.ts = s.ts[1:]
		s.vals = s.vals[1:]
		st.counters[CounterSamplesDropped]++
	}
	s.ts = append(s.ts, now)
	s.vals = append(s.vals, val)
}

// thresholds are the CWV good/poor boundaries (good ≤ first, poor > second).
var thresholds = map[string][2]float64{
	beacon.LCP:  {2500, 4000},
	beacon.FCP:  {1800, 3000},
	beacon.TTFB: {800, 1800},
	beacon.INP:  {200, 500},
	beacon.CLS:  {0.1, 0.25},
}

// pctStats computes percentiles only, no CWV rating (navigation timing has no good/needs_improvement/poor buckets).
func pctStats(vals []float64) VitalStats {
	sorted := make([]float64, len(vals))
	copy(sorted, vals)
	sort.Float64s(sorted)
	return VitalStats{
		N:   len(sorted),
		P50: pctSorted(sorted, 0.50),
		P75: pctSorted(sorted, 0.75),
		P95: pctSorted(sorted, 0.95),
	}
}

func stats(vital string, vals []float64) VitalStats {
	sorted := make([]float64, len(vals))
	copy(sorted, vals)
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
	// Counts, not percentages: sum-aggregation across sites and nodes then
	// yields traffic-weighted ratios, like httpcheck.status.
	st.Good = good
	st.Poor = poor
	st.NeedsImpr = n - good - poor
	return st
}

// percentile is nearest-rank on an unsorted copy.
func percentile(vals []float64, p float64) float64 {
	sorted := make([]float64, len(vals))
	copy(sorted, vals)
	sort.Float64s(sorted)
	return pctSorted(sorted, p)
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
