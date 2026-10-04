// SPDX-License-Identifier: GPL-3.0-or-later

// live.go is the rum-live FUNCTION data source: a bounded ring of recently
// accepted beacons carrying a page view, a vital, or a JS error. The ring
// belongs to this aggregator runtime and is guarded by its mutex.
package agg

import (
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
)

// liveRingCap bounds rows retained by one aggregator runtime.
const liveRingCap = 5000

// LiveRow is one rum-live ring entry. Has* distinguishes an absent vital
// from a genuine zero, matching the FUNCTION's "null when absent" column
// contract.
type LiveRow struct {
	Seq      uint64
	TS       time.Time
	Site     string
	Country  string // ISO code, "" when unknown
	City     string // "" when unknown
	Lat, Lon float64
	HasGeo   bool

	Page            string
	Browser, Device string
	PageView        bool

	LCPMS, INPMS, FCPMS, TTFBMS     float64
	HasLCP, HasINP, HasFCP, HasTTFB bool
	CLS                             float64
	HasCLS                          bool

	Errors int
}

// appendLive records one ring entry when the beacon qualifies (page view,
// any vital, or JS errors) and evicts anything past the aggregation window.
// Called from Ingest, which already holds a.mu.
func (a *Aggregator) appendLive(b *beacon.Beacon, now time.Time) {
	if !b.PageView && len(b.Vitals) == 0 && len(b.Errors) == 0 {
		return
	}
	a.liveSeq++
	row := LiveRow{
		Seq:      a.liveSeq,
		TS:       now,
		Site:     b.Site,
		Country:  b.Country,
		City:     b.City,
		Lat:      b.Lat,
		Lon:      b.Lon,
		HasGeo:   b.HasGeo,
		Page:     b.PageGroup,
		Browser:  b.Browser,
		Device:   b.Device,
		PageView: b.PageView,
		Errors:   len(b.Errors),
	}
	for _, v := range b.Vitals {
		switch v.Name {
		case beacon.LCP:
			row.LCPMS, row.HasLCP = v.Value, true
		case beacon.INP:
			row.INPMS, row.HasINP = v.Value, true
		case beacon.FCP:
			row.FCPMS, row.HasFCP = v.Value, true
		case beacon.TTFB:
			row.TTFBMS, row.HasTTFB = v.Value, true
		case beacon.CLS:
			row.CLS, row.HasCLS = v.Value, true
		}
	}
	a.live = append(a.live, row)
	if len(a.live)-a.liveStart > liveRingCap {
		a.liveStart = len(a.live) - liveRingCap
	}
	a.evictLive(now)
}

// evictLive drops ring entries older than the aggregation window. Called
// on every Ingest and Live read — rum-live is polled directly, never
// through Snapshot's chart-emission cadence, so eviction can't ride on
// that path alone.
//
// Dropped rows are skipped by advancing liveStart; the slice is compacted
// only once the dead prefix is as large as the ring, so appends stay O(1)
// instead of shifting every row each time.
func (a *Aggregator) evictLive(now time.Time) {
	cutoff := now.Add(-a.window)
	for a.liveStart < len(a.live) && a.live[a.liveStart].TS.Before(cutoff) {
		a.liveStart++
	}
	if a.liveStart >= liveRingCap || a.liveStart == len(a.live) {
		a.live = append(a.live[:0], a.live[a.liveStart:]...)
		a.liveStart = 0
	}
}

// Live returns ring rows with Seq > after, oldest first, capped at maxRows. next is the max seq among returned rows,
// or after unchanged when nothing matched. A cursor ahead of this runtime
// sequence is treated as 0; the Function routing owner fences generations.
func (a *Aggregator) Live(after uint64, maxRows int) ([]LiveRow, uint64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.evictLive(a.now())

	// A cursor beyond anything this runtime issued cannot identify a row
	// in this ring; serve the current ring from its start.
	if after > a.liveSeq {
		after = 0
	}
	next := after
	out := make([]LiveRow, 0, maxRows)
	for _, row := range a.live[a.liveStart:] {
		if row.Seq <= after {
			continue
		}
		out = append(out, row)
		next = row.Seq
		if maxRows > 0 && len(out) >= maxRows {
			break
		}
	}
	return out, next
}
