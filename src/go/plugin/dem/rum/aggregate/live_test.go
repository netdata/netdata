// SPDX-License-Identifier: GPL-3.0-or-later

package aggregate

import (
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
)

// TestLiveQualifyingBeaconsOnly pins the append condition: a page
// view, any vital, or a JS error — a beacon with none of those must not
// consume a ring slot (or a seq number).
func TestLiveQualifyingBeaconsOnly(t *testing.T) {
	a, now := newAgg(5 * time.Minute)

	// First beacon for (session, page): counts as a page view.
	a.Ingest(&beacon.Beacon{
		Site:      "s",
		Received:  *now,
		SessionID: "sess",
		Path:      "/a",
		PageGroup: "/a",
	})
	// Same (session, page) immediately after: deduped, no vitals/errors —
	// must not qualify.
	a.Ingest(&beacon.Beacon{
		Site:      "s",
		Received:  *now,
		SessionID: "sess",
		Path:      "/a",
		PageGroup: "/a",
	})
	// Same again, but this time with a JS error: qualifies independently
	// of PageView.
	a.Ingest(&beacon.Beacon{
		Site:      "s",
		Received:  *now,
		SessionID: "sess",
		Path:      "/a",
		PageGroup: "/a",
		Errors:    []beacon.Error{{Type: "TypeError", Message: "x"}},
	})
	// Same again, with a vital: qualifies too.
	a.Ingest(&beacon.Beacon{
		Site:      "s",
		Received:  *now,
		SessionID: "sess",
		Path:      "/a",
		PageGroup: "/a",
		Vitals:    []beacon.Vital{{Name: beacon.LCP, Value: 1200}},
	})

	rows, next := a.Live(0, 10)
	if len(rows) != 3 {
		t.Fatalf("expected 3 qualifying rows, got %d: %+v", len(rows), rows)
	}
	if next != rows[len(rows)-1].Seq {
		t.Fatalf("next = %d, want last row's seq %d", next, rows[len(rows)-1].Seq)
	}
	if !rows[0].PageView || rows[0].Errors != 0 {
		t.Fatalf("row0 (pageview) = %+v", rows[0])
	}
	if rows[1].PageView || rows[1].Errors != 1 {
		t.Fatalf("row1 (error) = %+v", rows[1])
	}
	if rows[2].PageView || !rows[2].HasLCP || rows[2].LCPMS != 1200 {
		t.Fatalf("row2 (vital) = %+v", rows[2])
	}
	// Seq is monotonic and 1-based across the qualifying rows only.
	for i, r := range rows {
		if r.Seq != uint64(i+1) {
			t.Fatalf("row %d seq = %d, want %d", i, r.Seq, i+1)
		}
	}
}

// TestLiveGeoAndPageFieldsCarried checks that the row carries the beacon
// geo/page/browser/device fields through unchanged.
func TestLiveGeoAndPageFieldsCarried(t *testing.T) {
	a, now := newAgg(5 * time.Minute)
	a.Ingest(&beacon.Beacon{
		Site:      "s",
		Received:  *now,
		Path:      "/checkout",
		PageGroup: "/checkout",
		Browser:   "Chrome",
		Device:    "mobile",
		Country:   "GR",
		City:      "Athens",
		Lat:       38.0,
		Lon:       23.7,
		HasGeo:    true,
	})
	rows, _ := a.Live(0, 10)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row (no-session beacons always count as a page view), got %d", len(rows))
	}
	r := rows[0]
	if r.Site != "s" || r.Page != "/checkout" || r.Browser != "Chrome" || r.Device != "mobile" ||
		r.Country != "GR" || r.City != "Athens" || r.Lat != 38.0 || r.Lon != 23.7 || !r.HasGeo {
		t.Fatalf("row = %+v", r)
	}
}

// TestLiveAfterCursor exercises the after cursor: rows with seq > after,
// oldest first.
func TestLiveAfterCursor(t *testing.T) {
	a, now := newAgg(5 * time.Minute)
	for i := 0; i < 3; i++ {
		a.Ingest(&beacon.Beacon{
			Site:     "s",
			Received: *now,
			Vitals:   []beacon.Vital{{Name: beacon.LCP, Value: 1000}},
		})
	}
	all, next := a.Live(0, 10)
	if len(all) != 3 || next != 3 {
		t.Fatalf("all = %+v next=%d", all, next)
	}
	rows, next := a.Live(1, 10)
	if len(rows) != 2 || rows[0].Seq != 2 || rows[1].Seq != 3 {
		t.Fatalf("after=1: rows = %+v", rows)
	}
	if next != 3 {
		t.Fatalf("next = %d, want 3", next)
	}
	rows, next = a.Live(3, 10)
	if len(rows) != 0 {
		t.Fatalf("after=3 (caught up): rows = %+v", rows)
	}
	if next != 3 {
		t.Fatalf("next must stay at the given after (3) when nothing matched, got %d", next)
	}
}

func TestLiveStaysWithItsOwner(t *testing.T) {
	a, now := newAgg(5*time.Minute, SiteCfg{
		Name:       "a",
		PageGroups: 5,
		Countries:  5,
	})
	other, _ := newAgg(5*time.Minute, SiteCfg{
		Name:       "b",
		PageGroups: 5,
		Countries:  5,
	})
	for _, site := range []string{"a", "b", "a"} {
		for _, target := range []*Aggregator{a, other} {
			target.Ingest(&beacon.Beacon{
				Site:     site,
				Received: *now,
				Vitals:   []beacon.Vital{{Name: beacon.LCP, Value: 1}},
			})
		}
	}
	rows, next := a.Live(0, 10)
	if len(rows) != 2 || next != 2 {
		t.Fatalf("site a rows=%+v next=%d", rows, next)
	}
	for _, row := range rows {
		if row.Site != "a" {
			t.Fatalf("leaked row from another site: %+v", row)
		}
	}
	rows, next = other.Live(0, 10)
	if len(rows) != 1 || next != 1 || rows[0].Site != "b" {
		t.Fatalf("site b rows=%+v next=%d", rows, next)
	}
}

// TestLiveMaxRowsCap checks the caller-supplied cap. next tracks the last
// row actually returned, not the ring maximum, so a follow-up poll resumes
// where the previous one left off.
func TestLiveMaxRowsCap(t *testing.T) {
	a, now := newAgg(5 * time.Minute)
	for i := 0; i < 5; i++ {
		a.Ingest(&beacon.Beacon{
			Site:     "s",
			Received: *now,
			Vitals:   []beacon.Vital{{Name: beacon.LCP, Value: 1}},
		})
	}
	rows, next := a.Live(0, 2)
	if len(rows) != 2 || rows[0].Seq != 1 || rows[1].Seq != 2 {
		t.Fatalf("capped rows = %+v", rows)
	}
	if next != 2 {
		t.Fatalf("next = %d, want 2 (the last row actually returned)", next)
	}
	rows, next = a.Live(next, 2)
	if len(rows) != 2 || rows[0].Seq != 3 || rows[1].Seq != 4 {
		t.Fatalf("second page = %+v", rows)
	}
	if next != 4 {
		t.Fatalf("next = %d, want 4", next)
	}
}

// TestLiveRingCap ensures the ring never grows past liveRingCap (5000):
// the oldest entries are dropped as new ones arrive.
func TestLiveRingCap(t *testing.T) {
	a, now := newAgg(30 * time.Minute)
	total := liveRingCap + 5
	for i := 0; i < total; i++ {
		a.Ingest(&beacon.Beacon{
			Site:     "s",
			Received: *now,
			Vitals:   []beacon.Vital{{Name: beacon.LCP, Value: 1}},
		})
	}
	rows, next := a.Live(0, liveRingCap+10)
	if len(rows) != liveRingCap {
		t.Fatalf("ring length = %d, want %d", len(rows), liveRingCap)
	}
	if rows[0].Seq != uint64(total-liveRingCap+1) {
		t.Fatalf(
			"oldest retained seq = %d, want %d (the first 5 must have been dropped)",
			rows[0].Seq,
			total-liveRingCap+1,
		)
	}
	if next != uint64(total) {
		t.Fatalf("next = %d, want %d", next, total)
	}
}

// TestLiveWindowEviction checks entries older than the aggregation
// window are dropped even when nothing but a Live() read happens
// between beacons (rum-live is polled directly, not through Snapshot).
func TestLiveWindowEviction(t *testing.T) {
	a, now := newAgg(1 * time.Minute)
	a.Ingest(&beacon.Beacon{
		Site:     "s",
		Received: *now,
		Vitals:   []beacon.Vital{{Name: beacon.LCP, Value: 1}},
	})
	rows, _ := a.Live(0, 10)
	if len(rows) != 1 {
		t.Fatalf("expected the fresh row, got %+v", rows)
	}

	*now = now.Add(2 * time.Minute) // past the 1m window
	rows, _ = a.Live(0, 10)
	if len(rows) != 0 {
		t.Fatalf("expected the stale row evicted on read, got %+v", rows)
	}

	// A fresh qualifying beacon after the gap must still append normally.
	a.Ingest(&beacon.Beacon{
		Site:     "s",
		Received: *now,
		Vitals:   []beacon.Vital{{Name: beacon.LCP, Value: 1}},
	})
	rows, _ = a.Live(0, 10)
	if len(rows) != 1 {
		t.Fatalf("expected exactly the new row, got %+v", rows)
	}
}

func TestLiveCursorFromBeforeRestartServesTheRing(t *testing.T) {
	a, now := newAgg(5 * time.Minute)
	a.Ingest(mk(*now, "s1", "/a"))
	a.Ingest(mk(*now, "s2", "/b"))
	rows, next := a.Live(9999, 0)
	if len(rows) != 2 || next != 2 {
		t.Fatalf("stale cursor: got %d rows, next %d; want the whole ring (2 rows, next 2)", len(rows), next)
	}
}

// Wrapping the ring many times keeps exactly the newest liveRingCap rows
// in order, and expiry still empties it (the ring used to shift the whole
// slice on every append, which serialized ingest).
func TestLiveRingSurvivesManyWraps(t *testing.T) {
	a, now := newAgg(5 * time.Minute)
	total := 3*liveRingCap + 2000
	for i := 0; i < total; i++ {
		*now = now.Add(10 * time.Millisecond)
		b := mk(*now, "", "/a")

		a.Ingest(b)
	}
	rows, next := a.Live(0, 10*liveRingCap)
	if len(rows) != liveRingCap {
		t.Fatalf("rows = %d, want %d", len(rows), liveRingCap)
	}
	if rows[0].Seq != uint64(total-liveRingCap+1) || rows[len(rows)-1].Seq != uint64(total) || next != uint64(total) {
		t.Fatalf(
			"seq range %d..%d next %d, want %d..%d",
			rows[0].Seq,
			rows[len(rows)-1].Seq,
			next,
			total-liveRingCap+1,
			total,
		)
	}
	for i := 1; i < len(rows); i++ {
		if rows[i].Seq != rows[i-1].Seq+1 {
			t.Fatalf("gap at %d: %d then %d", i, rows[i-1].Seq, rows[i].Seq)
		}
	}
	*now = now.Add(6 * time.Minute)
	b := mk(*now, "", "/a")

	a.Ingest(b)
	if rows, _ := a.Live(0, 10*liveRingCap); len(rows) != 1 {
		t.Fatalf("after the window only the new row remains, got %d", len(rows))
	}
}
