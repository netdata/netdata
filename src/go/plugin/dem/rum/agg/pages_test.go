// SPDX-License-Identifier: GPL-3.0-or-later

package agg

import (
	"strconv"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
)

func TestPagesReturnsRankedInstancesOnly(t *testing.T) {
	a, now := newAgg(5 * time.Minute)
	// Round 1: materialize the top set (see TestTopNFoldingIntoOther).
	a.Ingest(mk(*now, "s1", "/a", v(beacon.LCP, 1000)))
	a.Ingest(mk(*now, "s2", "/b", v(beacon.LCP, 2000)))
	*now = now.Add(10 * time.Second)
	a.Snapshot()

	// Round 2: real activity lands on the now-promoted groups.
	b1 := mk(*now, "t1", "/a", v(beacon.LCP, 1200), v(beacon.INP, 80))
	a.Ingest(b1)
	b2 := mk(*now, "t2", "/a", v(beacon.LCP, 1400))
	b2.Errors = []beacon.Error{{Type: "E", Message: "m", Fingerprint: "fp1"}}
	a.Ingest(b2)
	*now = now.Add(10 * time.Second)

	got := a.Pages()
	byPage := map[string]PageInfo{}
	for _, p := range got {
		byPage[p.Page] = p
	}
	pa, ok := byPage["/a"]
	if !ok {
		t.Fatalf("expected /a in %+v", got)
	}
	// 3 total: round 1's "s1" plus round 2's "t1"/"t2" (the real group
	// tracks its own activity regardless of top-set/routing status).
	if pa.PageviewsWindow != 3 || pa.Sessions != 3 || pa.ErrorsWindow != 1 {
		t.Fatalf("/a = %+v", pa)
	}
	if pa.LCPP75MS == 0 || pa.INPP75MS == 0 {
		t.Fatalf("/a p75 missing: %+v", pa)
	}
	if _, ok := byPage["/b"]; !ok {
		t.Fatalf("expected /b (top-N ranked but no new activity) in %+v", got)
	}
}

func TestPagesEmptyBeforeAnyRank(t *testing.T) {
	a, now := newAgg(5 * time.Minute)
	a.Ingest(mk(*now, "s1", "/a"))
	// No Snapshot()/rank() has run yet: top["page"] is nil.
	if got := a.Pages(); len(got) != 0 {
		t.Fatalf("expected no pages before the first rank, got %+v", got)
	}
}

func TestPagesStayWithTheirOwner(t *testing.T) {
	for _, site := range []string{"a", "b"} {
		a, now := newAgg(5*time.Minute, SiteCfg{
			Name:       site,
			PageGroups: 20,
			Countries:  20,
		})
		for _, incoming := range []string{"a", "b"} {
			a.Ingest(&beacon.Beacon{
				Site:      incoming,
				Received:  *now,
				SessionID: "shared",
				PageGroup: "/" + incoming,
			})
		}
		a.Snapshot()
		got := a.Pages()
		if len(got) != 1 || got[0].Site != site || got[0].Page != "/"+site {
			t.Fatalf("site %s pages = %+v", site, got)
		}
	}
}

func TestPagesSessionsBounded(t *testing.T) {
	a, now := newAgg(5 * time.Minute)
	a.Ingest(mk(*now, "s0", "/a"))
	*now = now.Add(10 * time.Second)
	a.Snapshot() // materialize top set

	for i := 0; i < maxPageGroupSessions+50; i++ {
		b := mk(*now, "sess"+strconv.Itoa(i), "/a")
		a.Ingest(b)
	}
	*now = now.Add(10 * time.Second)
	got := a.Pages()
	if len(got) != 1 || got[0].Sessions != maxPageGroupSessions {
		t.Fatalf("sessions bound: %+v", got)
	}
}

func TestPageVitalPresenceDistinguishesZeroFromMissing(t *testing.T) {
	a, now := newAgg(5 * time.Minute)
	a.Ingest(mk(*now, "browser", "/shop", v(beacon.CLS, 0)))
	a.Snapshot()
	pages := a.Pages()
	if len(pages) != 1 {
		t.Fatalf("missing observed page: %+v", pages)
	}
	if !pages[0].HasCLS || pages[0].CLSP75 != 0 || pages[0].HasLCP || pages[0].HasINP {
		t.Fatalf("zero CLS is a sample; missing latency is not: %+v", pages[0])
	}
}
