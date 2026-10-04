// SPDX-License-Identifier: GPL-3.0-or-later

package agg

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/stretchr/testify/assert"
)

var t0 = time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)

func newAgg(window time.Duration, cfgs ...SiteCfg) (*Aggregator, *time.Time) {
	cfg := SiteCfg{
		Name:        "s",
		DisplayName: "S",
		PageGroups:  20,
		Countries:   20,
	}
	if len(cfgs) > 1 {
		panic("newAgg accepts one site config")
	}
	if len(cfgs) == 1 {
		cfg = cfgs[0]
	}
	now := t0
	a := New(window, cfg)
	a.now = func() time.Time { return now }
	return a, &now
}

func mk(now time.Time, session, page string, vitals ...beacon.Vital) *beacon.Beacon {
	return &beacon.Beacon{
		Site:      "s",
		Received:  now,
		SessionID: session,
		Path:      page,
		PageGroup: beacon.PageGroup(page),
		Browser:   "Chrome",
		Device:    "desktop",
		Country:   "GR",
		Vitals:    vitals,
	}
}

func v(name string, val float64) beacon.Vital {
	return beacon.Vital{
		Name:  name,
		Value: val,
	}
}

func TestPercentiles(t *testing.T) {
	tests := map[string]struct {
		vals []float64
		p50  float64
		p75  float64
		p95  float64
	}{
		"1..100":       {vals: seq(1, 100), p50: 50, p75: 75, p95: 95},
		"single":       {vals: []float64{7}, p50: 7, p75: 7, p95: 7},
		"two":          {vals: []float64{10, 20}, p50: 10, p75: 20, p95: 20},
		"unsorted":     {vals: []float64{5, 1, 4, 2, 3}, p50: 3, p75: 4, p95: 5},
		"four samples": {vals: []float64{100, 200, 300, 400}, p50: 200, p75: 300, p95: 400},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := [3]float64{percentile(tc.vals, 0.5), percentile(tc.vals, 0.75), percentile(tc.vals, 0.95)}
			want := [3]float64{tc.p50, tc.p75, tc.p95}
			if got != want {
				t.Fatalf("got %v want %v", got, want)
			}
		})
	}
}

func seq(from, to int) []float64 {
	out := make([]float64, 0, to-from+1)
	for i := from; i <= to; i++ {
		out = append(out, float64(i))
	}
	return out
}

func TestRatings(t *testing.T) {
	tests := map[string]struct {
		vital string
		vals  []float64
		want  VitalStats
	}{
		"lcp mixed": {
			vital: beacon.LCP, vals: []float64{1000, 2500, 3000, 5000},
			want: VitalStats{
				N:         4,
				P50:       2500,
				P75:       3000,
				P95:       5000,
				Good:      2,
				NeedsImpr: 1,
				Poor:      1,
			},
		},
		"cls boundary good inclusive": {
			vital: beacon.CLS, vals: []float64{0.1, 0.25, 0.26},
			want: VitalStats{
				N:         3,
				P50:       0.25,
				P75:       0.26,
				P95:       0.26,
				Good:      1,
				NeedsImpr: 1,
				Poor:      1,
			},
		},
		"inp all good": {
			vital: beacon.INP, vals: []float64{50, 100},
			want: VitalStats{
				N:         2,
				P50:       50,
				P75:       100,
				P95:       100,
				Good:      2,
				NeedsImpr: 0,
				Poor:      0,
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := stats(tc.vital, tc.vals); got != tc.want {
				t.Fatalf("got %+v want %+v", got, tc.want)
			}
		})
	}
}

func TestTotalsAndCounters(t *testing.T) {
	a, now := newAgg(5 * time.Minute)
	a.Ingest(mk(*now, "s1", "/a", v(beacon.LCP, 1000), v(beacon.CLS, 0.05)))
	a.Ingest(mk(now.Add(time.Second), "s2", "/b", v(beacon.LCP, 3000)))
	b := mk(now.Add(2*time.Second), "s2", "/b")
	b.Errors = []beacon.Error{{Type: "E"}, {Type: "E"}}
	a.Ingest(b)
	a.Reject("s", beacon.RejectOrigin)
	a.Reject("s", beacon.RejectOrigin)
	a.Add(CounterOTLPSent, 5)
	*now = now.Add(3 * time.Second)

	snap := a.Snapshot()
	wantVitals := map[string]VitalStats{
		beacon.LCP: {N: 2, P50: 1000, P75: 3000, P95: 3000, Good: 1, NeedsImpr: 1, Poor: 0},
		beacon.CLS: {N: 1, P50: 0.05, P75: 0.05, P95: 0.05, Good: 1, NeedsImpr: 0, Poor: 0},
	}
	if !reflect.DeepEqual(snap.Vitals, wantVitals) {
		t.Fatalf("vitals:\ngot  %+v\nwant %+v", snap.Vitals, wantVitals)
	}
	wantCounters := map[string]uint64{
		CounterAccepted: 3, CounterPageviews: 2, CounterJSErrors: 2, beacon.RejectOrigin: 2, CounterOTLPSent: 5,
	}
	if !reflect.DeepEqual(snap.Counters, wantCounters) {
		t.Fatalf("counters:\ngot  %v\nwant %v", snap.Counters, wantCounters)
	}
	if snap.ActiveSessions != 2 || snap.Name != "s" || snap.DisplayName != "S" {
		t.Fatalf("sessions=%d site=%s name=%s", snap.ActiveSessions, snap.Name, snap.DisplayName)
	}
}

func TestPageviewDedup(t *testing.T) {
	tests := map[string]struct {
		beacons func(now time.Time) []*beacon.Beacon
		want    uint64
	}{
		"same session+page within 10s counts once": {
			beacons: func(n time.Time) []*beacon.Beacon {
				return []*beacon.Beacon{
					mk(n, "s1", "/a"),
					mk(n.Add(5*time.Second), "s1", "/a"),
					mk(n.Add(9*time.Second), "s1", "/a"),
				}
			}, want: 1,
		},
		"after 10s counts again": {
			beacons: func(n time.Time) []*beacon.Beacon {
				return []*beacon.Beacon{mk(n, "s1", "/a"), mk(n.Add(11*time.Second), "s1", "/a")}
			}, want: 2,
		},
		"different page same session": {
			beacons: func(n time.Time) []*beacon.Beacon {
				return []*beacon.Beacon{mk(n, "s1", "/a"), mk(n, "s1", "/b")}
			}, want: 2,
		},
		"different session same page": {
			beacons: func(n time.Time) []*beacon.Beacon {
				return []*beacon.Beacon{mk(n, "s1", "/a"), mk(n, "s2", "/a")}
			}, want: 2,
		},
		"page id preferred over path": {
			beacons: func(n time.Time) []*beacon.Beacon {
				b1, b2 := mk(n, "s1", "/a"), mk(n, "s1", "/a")
				b1.PageID, b2.PageID = "p1", "p2"
				return []*beacon.Beacon{b1, b2}
			}, want: 2,
		},
		"no session id: every beacon is a view": {
			beacons: func(n time.Time) []*beacon.Beacon {
				return []*beacon.Beacon{mk(n, "", "/a"), mk(n, "", "/a")}
			}, want: 2,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			a, now := newAgg(5 * time.Minute)
			var flags []bool
			for _, b := range tc.beacons(*now) {
				a.Ingest(b)
				flags = append(flags, b.PageView)
			}
			*now = now.Add(30 * time.Second)
			snap := a.Snapshot()
			if snap.Counters[CounterPageviews] != tc.want {
				t.Fatalf("pageviews=%d want %d (flags %v)", snap.Counters[CounterPageviews], tc.want, flags)
			}
			var flagged uint64
			for _, f := range flags {
				if f {
					flagged++
				}
			}
			if flagged != tc.want {
				t.Fatalf("PageView flags set on %d beacons, want %d", flagged, tc.want)
			}
		})
	}
}

func TestWindowEviction(t *testing.T) {
	a, now := newAgg(time.Minute)
	a.Ingest(mk(*now, "s1", "/a", v(beacon.LCP, 1000)))
	a.Ingest(mk(now.Add(30*time.Second), "s1", "/a", v(beacon.LCP, 3000)))

	*now = now.Add(45 * time.Second) // both inside the 1m window
	if got := a.Snapshot().Vitals[beacon.LCP]; got.N != 2 || got.P95 != 3000 {
		t.Fatalf("inside window: %+v", got)
	}
	*now = t0.Add(75 * time.Second) // first sample is now older than 1m
	if got := a.Snapshot().Vitals[beacon.LCP]; got.N != 1 || got.P50 != 3000 {
		t.Fatalf("after partial eviction: %+v", got)
	}
	*now = t0.Add(3 * time.Minute)
	if snap := a.Snapshot(); len(snap.Vitals) != 0 {
		t.Fatalf("after full eviction vitals must be absent (gap), got %+v", snap.Vitals)
	}
}

func TestSampleCap(t *testing.T) {
	a, now := newAgg(30 * time.Minute)
	for i := 1; i <= maxSamplesPerSeries+5; i++ {
		a.Ingest(mk(*now, "s1", "/a", v(beacon.INP, float64(i))))
	}
	snap := a.Snapshot()
	got := snap.Vitals[beacon.INP]
	if got.N != maxSamplesPerSeries {
		t.Fatalf("N=%d want %d", got.N, maxSamplesPerSeries)
	}
	// Oldest five (1..5) dropped: kept 6..10005, nearest-rank p50 is the 5000th = 5005.
	if got.P50 != 5005 {
		t.Fatalf("p50=%v want 5005 (oldest dropped first)", got.P50)
	}
	// The totals series and the four breakdown copies each dropped five.
	if snap.Counters[CounterSamplesDropped] != 5*5 {
		t.Fatalf("samples_dropped=%d want 25", snap.Counters[CounterSamplesDropped])
	}
}

func TestTopNFoldingIntoOther(t *testing.T) {
	a, now := newAgg(5*time.Minute, SiteCfg{
		Name:        "s",
		DisplayName: "S",
		PageGroups:  2,
		Countries:   20,
	})
	views := map[string]int{"/a": 3, "/b": 2, "/c": 1, "/d": 1}
	i := 0
	for page, n := range views {
		for j := 0; j < n; j++ {
			i++
			a.Ingest(mk(*now, "sess"+page+string(rune('0'+j)), page, v(beacon.LCP, float64(1000*i))))
		}
	}
	*now = now.Add(10 * time.Second)
	snap := a.Snapshot()

	// Round 1: the top set did not exist yet, so every count routed to other.
	got := snap.Breakdowns[KindPage]
	if len(got) != 3 || got[0].Value != "/a" || got[1].Value != "/b" || got[2].Value != Other {
		t.Fatalf("round 1 ranking: %+v", got)
	}
	if got[0].Pageviews != 0 || got[1].Pageviews != 0 || got[2].Pageviews != 7 {
		t.Fatalf("round 1 counters must all sit in other: %+v", got)
	}
	if _, ok := got[2].P75[beacon.LCP]; !ok || len(got[0].P75) != 1 {
		t.Fatalf("p75 must be present for top and other: %+v", got)
	}

	// Round 2: /a and /b are emitted instances now; /c stays folded.
	a.Ingest(mk(*now, "x1", "/a", v(beacon.LCP, 100)))
	a.Ingest(mk(*now, "x2", "/c", v(beacon.LCP, 100)))
	*now = now.Add(10 * time.Second)
	got = a.Snapshot().Breakdowns[KindPage]
	want := []Group{
		{Value: "/a", P75: got[0].P75, Pageviews: 1, JSErrors: 0},
		{Value: "/b", P75: got[1].P75, Pageviews: 0, JSErrors: 0},
		{Value: Other, P75: got[2].P75, Pageviews: 8, JSErrors: 0},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round 2:\ngot  %+v\nwant %+v", got, want)
	}
}

func TestLeavingTopSetRoutesFutureCountsToOther(t *testing.T) {
	a, now := newAgg(5*time.Minute, SiteCfg{
		Name:        "s",
		DisplayName: "S",
		PageGroups:  1,
		Countries:   20,
	})
	a.Ingest(mk(*now, "s1", "/a"))
	*now = now.Add(10 * time.Second)
	a.Snapshot() // top = {/a}
	for i := 0; i < 3; i++ {
		a.Ingest(mk(*now, "t"+string(rune('0'+i)), "/b"))
	}
	*now = now.Add(10 * time.Second)
	snap := a.Snapshot()
	if got := snap.Breakdowns[KindPage]; len(got) != 2 || got[0].Value != "/b" || got[1].Value != Other {
		t.Fatalf("ranking: %+v", got)
	}
	assert.Equal(t, []Group{
		{Value: "/b", P75: map[string]float64{}},
		{Value: Other, P75: map[string]float64{}, Pageviews: 4},
	}, snap.Breakdowns[KindPage])
	a.Ingest(mk(*now, "after-demotion", "/a", v(beacon.LCP, 700)))
	snap = a.Snapshot()
	assert.Equal(t, []Group{
		{Value: "/b", P75: map[string]float64{}},
		{Value: Other, P75: map[string]float64{beacon.LCP: 700}, Pageviews: 5},
	}, snap.Breakdowns[KindPage])
}

func TestInstanceExpiryAfterTwiceWindow(t *testing.T) {
	a, now := newAgg(time.Minute)
	a.Ingest(mk(*now, "s1", "/a", v(beacon.LCP, 1)))
	*now = now.Add(10 * time.Second)
	snap := a.Snapshot()
	// /a plus the other fold bucket (round-one routing, see TestTopNFoldingIntoOther).
	if got := snap.Breakdowns[KindPage]; len(got) != 2 || got[0].Value != "/a" || got[1].Value != Other {
		t.Fatalf("setup: %+v", got)
	}
	*now = t0.Add(2*time.Minute + time.Second)
	snap = a.Snapshot()
	// Only the monotonic other bucket survives expiry (its counters keep
	// emitting) — for the four kinds every beacon populates. version is
	// never set by mk() and stays empty (declared only once
	// a version is seen), so it is asserted separately below.
	for _, kind := range []string{KindBrowser, KindDevice, KindCountry, KindPage} {
		assert.Equal(t, []Group{{Value: Other, P75: map[string]float64{}, Pageviews: 1}}, snap.Breakdowns[kind], kind)
	}
	if got := snap.Breakdowns[KindVersion]; len(got) != 0 {
		t.Fatalf("version breakdown must stay empty when no beacon ever carried a version: %+v", got)
	}
	// Reappearance creates active samples and preserves monotonic fold totals.
	a.Ingest(mk(*now, "s9", "/a", v(beacon.LCP, 7)))
	*now = now.Add(10 * time.Second)
	snap = a.Snapshot()
	for kind, value := range map[string]string{KindBrowser: "Chrome", KindDevice: "desktop", KindCountry: "GR", KindPage: "/a"} {
		assert.Equal(t, []Group{
			{Value: value, P75: map[string]float64{beacon.LCP: 7}},
			{Value: Other, P75: map[string]float64{}, Pageviews: 2},
		}, snap.Breakdowns[kind], kind)
	}
}

func TestUnknownCountryExcludedFromBreakdown(t *testing.T) {
	a, now := newAgg(5 * time.Minute)
	b := mk(*now, "s1", "/a", v(beacon.LCP, 1))
	b.Country = ""
	a.Ingest(b)
	*now = now.Add(10 * time.Second)
	snap := a.Snapshot()
	if len(snap.Breakdowns[KindCountry]) != 0 {
		t.Fatalf("country breakdown must be empty: %+v", snap.Breakdowns[KindCountry])
	}
	if snap.Vitals[beacon.LCP].N != 1 || snap.Counters[CounterPageviews] != 1 {
		t.Fatalf("totals must still count: %+v %v", snap.Vitals, snap.Counters)
	}
}

func TestNavigationTiming(t *testing.T) {
	a, now := newAgg(5 * time.Minute)
	b1 := mk(*now, "s1", "/a")
	b1.Navigation = &beacon.Navigation{
		LoadMS:  1000,
		HasLoad: true,
		DCLMS:   200,
		HasDCL:  true,
	}
	a.Ingest(b1)
	b2 := mk(*now, "s2", "/b")
	b2.Navigation = &beacon.Navigation{
		LoadMS:  2000,
		HasLoad: true,
	} // no DCL sample
	a.Ingest(b2)
	*now = now.Add(10 * time.Second)
	snap := a.Snapshot()
	if snap.Load.N != 2 || snap.Load.P50 != 1000 || snap.Load.P95 != 2000 {
		t.Fatalf("Load = %+v", snap.Load)
	}
	if snap.Load.Good != 0 || snap.Load.NeedsImpr != 0 || snap.Load.Poor != 0 {
		t.Fatalf("Load must carry no CWV rating: %+v", snap.Load)
	}
	if snap.DCL.N != 1 || snap.DCL.P50 != 200 {
		t.Fatalf("DCL = %+v", snap.DCL)
	}
}

func TestNavigationTimingAbsentWhenNoEvent(t *testing.T) {
	a, now := newAgg(5 * time.Minute)
	a.Ingest(mk(*now, "s1", "/a"))
	*now = now.Add(10 * time.Second)
	snap := a.Snapshot()
	if snap.Load.N != 0 || snap.DCL.N != 0 {
		t.Fatalf("expected no navigation timing samples: load=%+v dcl=%+v", snap.Load, snap.DCL)
	}
}

func TestVersionBreakdownDeclaredOnlyWhenSeen(t *testing.T) {
	a, now := newAgg(5 * time.Minute)
	// No beacon ever carries a version: the breakdown must stay empty.
	a.Ingest(mk(*now, "s0", "/a", v(beacon.LCP, 1000)))
	*now = now.Add(10 * time.Second)
	if got := a.Snapshot().Breakdowns[KindVersion]; len(got) != 0 {
		t.Fatalf("version breakdown must be empty until a version is seen: %+v", got)
	}

	b1 := mk(*now, "s1", "/a", v(beacon.LCP, 2000))
	b1.AppVersion = "1.0.0"
	a.Ingest(b1)
	b2 := mk(*now, "s2", "/b", v(beacon.LCP, 3000))
	b2.AppVersion = "2.0.0"
	a.Ingest(b2)
	*now = now.Add(10 * time.Second)
	a.Snapshot() // round 1: materializes the top set (see TestTopNFoldingIntoOther)

	b1 = mk(
		*now,
		"t1",
		"/a",
		v(beacon.LCP, 2000),
	) // fresh session ids: round 1's are still within the pageview dedup window
	b1.AppVersion = "1.0.0"
	a.Ingest(b1)
	b2 = mk(*now, "t2", "/b", v(beacon.LCP, 3000))
	b2.AppVersion = "2.0.0"
	a.Ingest(b2)
	*now = now.Add(10 * time.Second)
	got := a.Snapshot().Breakdowns[KindVersion]
	// 1.0.0 + 2.0.0 plus the "other" fold bucket, which persists (monotonic
	// counters) from round 1's pre-top routing — same precedent as
	// TestTopNFoldingIntoOther's round 2.
	if len(got) != 3 {
		t.Fatalf("version breakdown = %+v want 3 instances", got)
	}
	byValue := map[string]Group{}
	for _, g := range got {
		byValue[g.Value] = g
	}
	if byValue["1.0.0"].Pageviews != 1 || byValue["1.0.0"].P75[beacon.LCP] != 2000 {
		t.Fatalf("1.0.0 group = %+v", byValue["1.0.0"])
	}
	if byValue["2.0.0"].Pageviews != 1 || byValue["2.0.0"].P75[beacon.LCP] != 3000 {
		t.Fatalf("2.0.0 group = %+v", byValue["2.0.0"])
	}
}

func TestSessionsLRU(t *testing.T) {
	a, now := newAgg(30 * time.Minute)
	a.Ingest(mk(*now, "s1", "/a"))
	a.Ingest(mk(*now, "s2", "/a"))
	*now = now.Add(20 * time.Minute)
	a.Ingest(mk(*now, "s1", "/b"))
	*now = t0.Add(31 * time.Minute) // s2 idle 31m, s1 idle 11m
	if got := a.Snapshot().ActiveSessions; got != 1 {
		t.Fatalf("active=%d want 1", got)
	}
	*now = t0.Add(52 * time.Minute)
	if got := a.Snapshot().ActiveSessions; got != 0 {
		t.Fatalf("active=%d want 0", got)
	}
}

func TestConstructorOwnsConfiguration(t *testing.T) {
	cfg := SiteCfg{
		Name:        "a",
		DisplayName: "A",
		PageGroups:  1,
		Countries:   1,
	}
	a, now := newAgg(5*time.Minute, cfg)
	cfg.Name, cfg.DisplayName, cfg.PageGroups = "b", "B", 10
	for _, page := range []string{"/a", "/b"} {
		a.Ingest(&beacon.Beacon{
			Site:      "a",
			Received:  *now,
			PageGroup: page,
		})
	}
	snap := a.Snapshot()
	assert.Equal(t, "a", snap.Name)
	assert.Equal(t, "A", snap.DisplayName)
	assert.Equal(t, uint64(2), snap.Counters[CounterAccepted])
	assert.Len(t, snap.Breakdowns[KindPage], 2) // one ranked group plus other
}

func TestMismatchedIdentityHasNoEffects(t *testing.T) {
	a, now := newAgg(5 * time.Minute)
	history := &fakeHistorySink{}
	a.SetHistorySink(history)
	before := a.Snapshot()
	activity := a.Activity()
	b := mk(*now, "shared", "/wrong", v(beacon.LCP, 5000))
	b.Site = "other"
	b.Errors = []beacon.Error{{Fingerprint: "fp", Type: "Error", Message: "wrong"}}
	a.Ingest(b)
	assert.False(t, b.PageView)
	assert.False(t, b.SampledOut)
	for _, reason := range []string{beacon.RejectOrigin, beacon.RejectRate, beacon.RejectSize, beacon.RejectBot} {
		a.Reject("other", reason)
	}
	assert.Equal(t, before, a.Snapshot())
	assert.Equal(t, activity, a.Activity())
	assert.Empty(t, a.Pages())
	events, ok := a.SessionEvents("shared")
	assert.False(t, ok)
	assert.Empty(t, events)
	rows, next := a.Live(0, 10)
	assert.Empty(t, rows)
	assert.Zero(t, next)
	assert.Empty(t, history.events)
	// The same session and page remain a fresh page view for the owner.
	b.Site = "s"
	a.Ingest(b)
	assert.True(t, b.PageView)
	assert.Equal(t, uint64(1), a.Snapshot().Counters[CounterAccepted])
	assert.NotEmpty(t, history.events)
	for _, event := range history.events {
		assert.Equal(t, "s", event.Site)
	}
}

func TestTrackedGroupCapFoldsNewValues(t *testing.T) {
	a, now := newAgg(5*time.Minute, SiteCfg{
		Name:       "s",
		PageGroups: 100,
		Countries:  100,
	})
	for i := 0; i < maxTrackedGroups+50; i++ {
		a.Ingest(mk(*now, "", "/p"+itoa(i)))
	}
	*now = now.Add(10 * time.Second)
	snap := a.Snapshot()
	groups := snap.Breakdowns[KindPage]
	if len(groups) != 101 || groups[100].Value != Other || groups[100].Pageviews != uint64(maxTrackedGroups+50) {
		t.Fatalf("cap: n=%d last=%+v", len(groups), groups[len(groups)-1])
	}
}

func TestActivity(t *testing.T) {
	a, now := newAgg(5 * time.Minute)

	act := a.Activity()
	if act != (SiteActivity{
		LastBeaconAgeS: -1,
	}) {
		t.Fatalf("never seen: %+v", act)
	}

	b := mk(*now, "sess1", "/a", v(beacon.LCP, 1000))
	b.Errors = []beacon.Error{{Type: "E"}, {Type: "E"}}
	a.Ingest(b)
	a.Reject("s", beacon.RejectOrigin)
	a.Reject("s", beacon.RejectInvalid) // must not count toward rejected_per_min

	act = a.Activity()
	want := SiteActivity{
		BeaconsPerMin:        1,
		RejectedPerMin:       1,
		LastBeaconAgeS:       0,
		ActiveSessions:       1,
		InvestigatedSessions: 1,
		PageviewsWindow:      1,
		JSErrorsWindow:       2,
	}
	if act != want {
		t.Fatalf("just ingested:\ngot  %+v\nwant %+v", act, want)
	}

	*now = now.Add(90 * time.Second) // past the fixed 60s cutoff, inside the 5m window
	act = a.Activity()
	want = SiteActivity{
		LastBeaconAgeS:       90,
		ActiveSessions:       1,
		InvestigatedSessions: 1,
		PageviewsWindow:      1,
		JSErrorsWindow:       2,
	}
	if act != want {
		t.Fatalf("after 90s:\ngot  %+v\nwant %+v", act, want)
	}

	*now = now.Add(5 * time.Minute) // past the aggregation window too
	act = a.Activity()
	if act.PageviewsWindow != 0 || act.JSErrorsWindow != 0 || act.LastBeaconAgeS != 390 {
		t.Fatalf("after window elapsed: %+v", act)
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}

// TestRankNeverPromotesTheOtherSentinel: a client-supplied browser named
// exactly like the fold sentinel must be folded, never ranked — otherwise
// two groups map to one chart instance and overwrite each other.
func TestRankNeverPromotesTheOtherSentinel(t *testing.T) {
	a, now := newAgg(5*time.Minute, SiteCfg{
		Name:        "s",
		DisplayName: "S",
		PageGroups:  20,
		Countries:   20,
	})
	for i := 0; i < browserTopN+2; i++ {
		b := mk(*now, fmt.Sprintf("s%d", i), "/p")
		b.Browser = fmt.Sprintf("Browser%02d", i)
		a.Ingest(b)
	}
	for i := 0; i < 5; i++ { // busiest value on views, so it would rank first
		b := mk(*now, fmt.Sprintf("o%d", i), "/p")
		b.Browser = Other
		a.Ingest(b)
	}
	groups := a.Snapshot().Breakdowns[KindBrowser]
	var others int
	for _, g := range groups {
		if g.Value == Other {
			others++
		}
	}
	if others != 1 {
		t.Fatalf("want exactly one %q group, got %d in %+v", Other, others, groups)
	}
	if len(groups) != browserTopN+1 {
		t.Fatalf("want %d ranked groups + other, got %d", browserTopN, len(groups))
	}
}

func TestWindowTotalsSlideWithTheWindow(t *testing.T) {
	a, now := newAgg(5 * time.Minute)
	a.Ingest(mk(*now, "s1", "/a"))
	b := mk(*now, "s2", "/b")
	b.Errors = []beacon.Error{{Type: "Error", Message: "x"}, {Type: "Error", Message: "y"}}
	a.Ingest(b)
	*now = now.Add(10 * time.Second)
	if s := a.Snapshot(); s.PageviewsWindow != 2 || s.JSErrorsWindow != 2 {
		t.Fatalf("in window: pageviews=%d errors=%d, want 2/2", s.PageviewsWindow, s.JSErrorsWindow)
	}
	*now = now.Add(6 * time.Minute)
	if s := a.Snapshot(); s.PageviewsWindow != 0 || s.JSErrorsWindow != 0 {
		t.Fatalf("after window: pageviews=%d errors=%d, want 0/0", s.PageviewsWindow, s.JSErrorsWindow)
	}
}
