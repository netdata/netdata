// SPDX-License-Identifier: GPL-3.0-or-later

package aggregate

import (
	"fmt"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var t0 = time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)

func newAgg(window time.Duration, cfgs ...SiteCfg) (*Aggregator, *time.Time) {
	cfg := SiteCfg{
		Investigate: InvestigateCfg{
			Rate: 1,
		},
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
		ExperienceID: fmt.Sprintf("%s|%s|%d", session, page, now.UnixNano()),
		Events: []beacon.Event{
			{ID: "activation", Revision: 1, Kind: beacon.EventDocument, Name: "document_activated"},
		},
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
		ID:       name,
		Revision: 1,
		Name:     name,
		Value:    val,
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
			result := pctStats(tc.vals)
			got := [3]float64{result.P50, result.P75, result.P95}
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

func TestLatestRevisionAndExplicitActivations(t *testing.T) {
	a, now := newAgg(5 * time.Minute)
	b := mk(*now, "s1", "/entry", v(beacon.CLS, .1))
	b.View = "home"
	b.ViewID = "view1"
	original := *b
	require.True(t, a.Ingest(b).PageView)
	*now = now.Add(11 * time.Second)
	b.Received = *now
	assert.False(t, a.Ingest(b).PageView)
	assert.Equal(t, 1, a.Snapshot().Vitals[beacon.CLS].N)
	b.Vitals = append([]beacon.Vital{}, b.Vitals...)
	b.Vitals[0].Revision = 3
	b.Vitals[0].Value = .5
	b.PageGroup = "/changed"
	b.View = "checkout"
	b.ViewID = "view2"
	a.Ingest(b)
	b.Vitals[0].Revision = 2
	b.Vitals[0].Value = .2
	result := a.Ingest(b)
	assert.Empty(t, result.Observation.Vitals)
	snap := a.Snapshot()
	assert.Equal(t, .5, snap.Vitals[beacon.CLS].P75)
	assert.EqualValues(t, 1, snap.Counters[CounterPageviews])
	pages := a.Pages()
	for _, p := range pages {
		if p.Page == "/entry" {
			assert.Equal(t, .5, p.Vitals[beacon.CLS].P75)
		}
	}
	assert.Equal(t, .1, original.Vitals[0].Value)
	// A restored document is a new activation even in the same session/path.
	b.ExperienceID = "restored"
	b.Vitals[0].ID = "restored-cls"
	b.Vitals[0].Revision = 1
	assert.True(t, a.Ingest(b).PageView)
	assert.Equal(t, 2, a.Snapshot().Vitals[beacon.CLS].N)
}
func TestDuplicatesDoNotExtendSampleOrIdentityRetention(t *testing.T) {
	a, now := newAgg(time.Minute)
	b := mk(*now, "s", "/", v(beacon.LCP, 100))
	a.Ingest(b)
	*now = now.Add(50 * time.Second)
	b.Received = *now
	a.Ingest(b)
	*now = now.Add(11 * time.Second)
	assert.Zero(t, a.Snapshot().Vitals[beacon.LCP].N)
	// A genuinely newer update is eligible after its preceding sample expires.
	b.Received = *now
	b.Vitals[0].Revision = 2
	b.Vitals[0].Value = 200
	b.PageGroup = "/wrong"
	a.Ingest(b)
	assert.Equal(t, 200., a.Snapshot().Vitals[beacon.LCP].P75)
	pages := a.Pages()
	require.NotEmpty(t, pages)
	var found bool
	for _, p := range pages {
		if p.Page == "/" {
			found = true
			assert.Equal(t, 1, p.Vitals[beacon.LCP].N)
		}
	}
	assert.True(t, found)
	*now = now.Add(31 * time.Minute)
	b.Received = *now
	// The explicitly bounded replay contract permits this old report again.
	assert.Len(t, a.Ingest(b).Observation.Vitals, 1)
}
func TestInvalidIdentityMissingAndZero(t *testing.T) {
	a, now := newAgg(time.Minute)
	b := mk(*now, "s", "/", v(beacon.CLS, 0))
	b.ExperienceID = ""
	a.Ingest(b)
	s := a.Snapshot()
	assert.Zero(t, s.Counters[CounterPageviews])
	assert.Zero(t, s.Vitals[beacon.CLS].N)
	assert.EqualValues(t, 2, s.Counters[CounterInvalidMeasurements])
	b.ExperienceID = "valid"
	a.Ingest(b)
	s = a.Snapshot()
	require.Len(t, s.Vitals, 5)
	assert.Equal(t, 1, s.Vitals[beacon.CLS].N)
	assert.Zero(t, s.Vitals[beacon.CLS].P75)
	assert.Zero(t, s.Vitals[beacon.INP].N)
}
func TestNavigationRevisionAndBFCacheAbsence(t *testing.T) {
	a, now := newAgg(time.Minute)
	b := mk(*now, "s", "/")
	b.Navigation = &beacon.Navigation{
		Revision: 1,
		HasLoad:  true,
		LoadMS:   0,
		HasDCL:   true,
		DCLMS:    5,
	}
	a.Ingest(b)
	a.Ingest(b)
	s := a.Snapshot()
	assert.Equal(t, 1, s.Load.N)
	assert.Zero(t, s.Load.P75)
	assert.Equal(t, 5., s.DCL.P75)
	b.Navigation = &beacon.Navigation{
		Revision: 2,
		HasLoad:  true,
		LoadMS:   20,
	}
	a.Ingest(b)
	s = a.Snapshot()
	assert.Equal(t, 20., s.Load.P75)
	assert.Zero(t, s.DCL.N)
	b.ExperienceID = "bfcache"
	b.Navigation = nil
	a.Ingest(b)
	assert.Equal(t, 1, a.Snapshot().Load.N)
}
func TestCanonicalCapacityLossAndRecovery(t *testing.T) {
	a, now := newAgg(time.Minute)
	for i := range maxWindowObservations + 1 {
		b := mk(*now, "", fmt.Sprint(i), v(beacon.LCP, 100))
		b.Events = nil
		a.Ingest(b)
	}
	s := a.Snapshot()
	assert.Equal(t, maxWindowObservations, s.Vitals[beacon.LCP].N)
	assert.Positive(t, s.Vitals[beacon.LCP].Lost)
	assert.Zero(t, s.Vitals[beacon.LCP].P75)
	assert.Positive(t, s.Counters[CounterSamplesDropped])
	*now = now.Add(time.Minute + time.Nanosecond)
	s = a.Snapshot()
	assert.Zero(t, s.Vitals[beacon.LCP].N)
	assert.Zero(t, s.Vitals[beacon.LCP].Lost)
}
func TestLossEpisodeDoesNotClearBeforeNewestLossExpires(t *testing.T) {
	a, now := newAgg(time.Minute)
	for i := range maxWindowObservations {
		b := mk(*now, "", fmt.Sprint(i), v(beacon.LCP, 100))
		b.Events = nil
		a.Ingest(b)
	}
	*now = now.Add(30 * time.Second)
	for i := range 2 {
		b := mk(*now, "", fmt.Sprint(i+maxWindowObservations), v(beacon.LCP, 100))
		b.Events = nil
		a.Ingest(b)
	}
	*now = now.Add(31 * time.Second)
	// Identity evidence was lost at t+30: possible duplicate admission persists
	// through t+90 even after all original sample losses age out at t+60.
	assert.Positive(t, a.Snapshot().Vitals[beacon.LCP].Lost)
	*now = now.Add(30 * time.Second)
	assert.Zero(t, a.Snapshot().Vitals[beacon.LCP].Lost)
}
func TestMismatchedIdentityHasNoEffects(t *testing.T) {
	a, now := newAgg(time.Minute)
	b := mk(*now, "s", "/", v(beacon.LCP, 10))
	b.Site = "other"
	assert.False(t, a.Ingest(b).Accepted)
	assert.Empty(t, a.Snapshot().Counters)
	assert.Empty(t, a.Pages())
}
func itoa(i int) string { return fmt.Sprint(i) }

func TestApplicationViewsAreSeparateAndRequireExplicitEvents(t *testing.T) {
	a, now := newAgg(time.Minute)
	b := mk(*now, "s", "/entry", v(beacon.LCP, 200))
	b.View = "home"
	b.ViewID = "home-1"
	a.Ingest(b)
	b.Events = []beacon.Event{{Kind: beacon.EventView, ID: "home-1", Revision: 1}}
	a.Ingest(b)
	a.Ingest(b)
	b.View = "checkout"
	b.ViewID = "checkout-1"
	b.Events = nil
	a.Ingest(b)
	assert.EqualValues(t, 1, a.Snapshot().ApplicationViewsWindow, "metadata alone does not invent transitions")
	b.Events = []beacon.Event{{Kind: beacon.EventView, ID: "checkout-1", Revision: 2}}
	a.Ingest(b)
	s := a.Snapshot()
	assert.EqualValues(t, 1, s.PageviewsWindow)
	assert.EqualValues(t, 2, s.ApplicationViewsWindow)
	for _, g := range s.Breakdowns[KindView] {
		assert.Empty(t, g.Vitals)
		assert.Zero(t, g.Pageviews)
	}
}
func TestUnknownDimensionsRemainVisible(t *testing.T) {
	a, now := newAgg(time.Minute)
	b := mk(*now, "s", "/", v(beacon.LCP, 20))
	b.Country = ""
	b.Device = ""
	b.Browser = ""
	a.Ingest(b)
	s := a.Snapshot()
	for _, kind := range []string{KindCountry, KindDevice, KindBrowser, KindVersion} {
		groups := s.Breakdowns[kind]
		require.Len(t, groups, 1)
		assert.Equal(t, unknownValue, groups[0].Value)
		assert.Equal(t, 1, groups[0].Vitals[beacon.LCP].N)
	}
}
func TestRevisionOriginKeepsMeasurementDimensionsAndReportContext(t *testing.T) {
	a, now := newAgg(time.Minute)
	history := &fakeHistorySink{}
	a.SetHistorySink(history)
	b := mk(*now, "s", "/entry", v(beacon.LCP, 6000))
	b.View = "first"
	b.ViewID = "first-1"
	b.UserID = "before"
	b.AppVersion = "v1"
	a.Ingest(b)
	b.Vitals[0].Revision = 2
	b.PageGroup = "/later"
	b.Browser = "Firefox"
	b.AppVersion = "v2"
	b.UserID = "after"
	b.View = "later"
	b.ViewID = "later-1"
	result := a.Ingest(b)
	require.Len(t, result.Observation.Vitals, 1)
	origin := result.Observation.Vitals[0].Origin
	require.NotNil(t, origin)
	assert.Equal(t, "/entry", origin.PageGroup)
	assert.Equal(t, "v1", origin.AppVersion)
	assert.Equal(t, "Chrome", origin.Browser)
	last := history.events[len(history.events)-1]
	assert.Equal(t, "/entry", last.Page)
	assert.Equal(t, "v1", last.Version)
	assert.Equal(t, "after", last.UserID)
	assert.Equal(t, "later", last.View)
	assert.Equal(t, "later-1", last.ViewID)
	assert.Nil(t, b.Vitals[0].Origin, "input is immutable")
}
func TestInactiveDetailPromotionExpiresOnIngestion(t *testing.T) {
	a, now, _ := sampledAgg(0, true, false)
	b := mk(*now, "s", "/")
	b.Errors = []beacon.Error{{Message: "failure"}}
	require.True(t, a.Ingest(b).Investigated)
	*now = now.Add(31 * time.Minute)
	assert.False(t, a.Ingest(mk(*now, "s", "/new")).Investigated)
}

func TestFilteredObservationPreservesOnlyAdmittedTimingEvents(t *testing.T) {
	a, now := newAgg(time.Minute)
	b := mk(*now, "s", "/")
	b.Navigation = &beacon.Navigation{
		Revision: 1,
		HasLoad:  true,
		LoadMS:   10,
	}
	b.Resources = []beacon.Resource{{ID: "resource", HasDuration: true, DurationMS: 20}, {ID: "self", Self: true}}
	b.Events = append(
		b.Events,
		beacon.Event{
			Kind:     beacon.EventNavigation,
			Revision: 1,
		},
		beacon.Event{
			Kind:     beacon.EventResource,
			ID:       "resource",
			Revision: 2,
		},
		beacon.Event{
			Kind:     beacon.EventResource,
			ID:       "self",
			Revision: 3,
		},
	)
	result := a.Ingest(b)
	require.NotNil(t, result.Observation)
	assert.Len(t, result.Observation.Events, 3)
	assert.Len(t, result.Observation.Resources, 1)
	result = a.Ingest(b)
	assert.Empty(t, result.Observation.Events)
	assert.Nil(t, result.Observation.Navigation)
	assert.Empty(t, result.Observation.Resources)
}

func TestIdentityCapacityLossInvalidatesObservedSessions(t *testing.T) {
	a, now := newAgg(time.Minute)
	old := mk(now.Add(-61*time.Second), "expired", "/old", v(beacon.LCP, 100))
	old.Events = nil
	a.Ingest(old)
	require.Zero(t, a.Snapshot().ObservedSessions)
	for i := range maxWindowObservations {
		a.Ingest(&beacon.Beacon{
			Site:         "s",
			ExperienceID: fmt.Sprint(i),
			Received:     *now,
			Events:       []beacon.Event{{Kind: beacon.EventDocument, ID: "activation", Revision: 1}},
		})
	}
	old.Received = *now
	a.Ingest(old)
	snapshot := a.Snapshot()
	require.Positive(t, snapshot.WindowLost)
	require.Positive(t, snapshot.SessionsLost, "replay evidence loss also compromises session membership")
	require.Positive(t, a.Activity().SessionsLost)
	for _, page := range a.Pages() {
		if page.Page == "/old" {
			require.Positive(t, page.SessionsLost)
		}
	}
	*now = now.Add(time.Minute + time.Second)
	snapshot = a.Snapshot()
	require.Zero(t, snapshot.SessionsLost)
	require.Zero(t, snapshot.ObservedSessions)
}
