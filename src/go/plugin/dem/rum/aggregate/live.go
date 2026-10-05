// SPDX-License-Identifier: GPL-3.0-or-later

// live.go is the rum-live FUNCTION data source: a bounded ring of recently admitted
// document activations, application-view occurrences, vital reports and JS errors.
// The ring belongs to this aggregator runtime and is guarded by its mutex.
package aggregate

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
	ExperienceID, View, ViewID, MetricID, Kind, Vital string
	Revision                                          uint64
	Seq                                               uint64
	TS                                                time.Time
	Site                                              string
	Country                                           string // ISO code, "" when unknown
	City                                              string // "" when unknown
	Lat, Lon                                          float64
	HasGeo                                            bool

	Page            string
	Browser, Device string
	PageView        bool

	LCPMS, INPMS, FCPMS, TTFBMS     float64
	HasLCP, HasINP, HasFCP, HasTTFB bool
	CLS                             float64
	HasCLS                          bool

	Errors int
}

// appendLive gives each measurement its own unambiguous update identity.
func (a *Aggregator) appendLive(b *beacon.Beacon, pageView bool, now time.Time) {
	base := LiveRow{
		TS:           now,
		Site:         b.Site,
		Country:      b.Country,
		City:         b.City,
		Lat:          b.Lat,
		Lon:          b.Lon,
		HasGeo:       b.HasGeo,
		Page:         b.PageGroup,
		Browser:      b.Browser,
		Device:       b.Device,
		ExperienceID: b.ExperienceID,
		View:         b.View,
		ViewID:       b.ViewID,
	}
	appendRow := func(row LiveRow) {
		a.liveSeq++
		row.Seq = a.liveSeq
		a.live = append(a.live, row)
		if len(a.live)-a.liveStart > liveRingCap {
			a.liveStart = len(a.live) - liveRingCap
		}
	}
	if pageView {
		row := base
		row.Kind = "document"
		for _, ev := range b.Events {
			if ev.Kind == beacon.EventDocument {
				row.Revision = ev.Revision
				break
			}
		}
		row.PageView = true
		appendRow(row)
	}
	for _, ev := range b.Events {
		if ev.Kind == beacon.EventView {
			row := base
			row.Kind = "view"
			row.Revision = ev.Revision
			appendRow(row)
		}
	}
	for _, v := range b.Vitals {
		row := base
		row.Kind = "vital"
		row.Vital = v.Name
		row.MetricID = v.ID
		row.Revision = v.Revision
		if origin := v.Origin; origin != nil {
			if origin.Country != row.Country {
				// Current location must not contradict the saved measurement country.
				row.City, row.Lat, row.Lon, row.HasGeo = "", 0, 0, false
			}
			row.Page, row.Browser, row.Device, row.Country = origin.PageGroup, origin.Browser, origin.Device, origin.Country
		}
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
		appendRow(row)
	}
	if len(b.Errors) > 0 {
		row := base
		row.Kind = "error"
		row.Errors = len(b.Errors)
		appendRow(row)
	}
	a.evictLive(now)
}

// evictLive drops the expired prefix of the ring. Live also filters stale
// rows behind fresh ones, because receipt times can arrive out of order. Called
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
	now := a.now()
	a.evictLive(now)
	cutoff := now.Add(-a.window)

	// A cursor beyond anything this runtime issued cannot identify a row
	// in this ring; serve the current ring from its start.
	if after > a.liveSeq {
		after = 0
	}
	next := after
	out := make([]LiveRow, 0, maxRows)
	for _, row := range a.live[a.liveStart:] {
		if row.Seq <= after || row.TS.Before(cutoff) {
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
