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

func TestResourceCapacityDoesNotInvalidateDocumentMeasurements(t *testing.T) {
	a, now := newAgg(5 * time.Minute)
	a.Ingest(mk(*now, "document", "/", v(beacon.LCP, 100)))
	for i := 0; i < maxWindowObservations+1; i++ {
		a.Ingest(
			&beacon.Beacon{
				Site:         "s",
				ExperienceID: "resources",
				SessionID:    "resources",
				Received:     *now,
				Resources: []beacon.Resource{
					{ID: fmt.Sprint(i), Host: "example.com", HasDuration: true, DurationMS: 10, Initiator: "fetch"},
				},
				PageHost: "example.com",
			},
		)
	}
	s := a.Snapshot()
	assert.EqualValues(t, 1, s.PageviewsWindow)
	assert.Zero(t, s.WindowLost)
	assert.Equal(t, 1, s.Vitals[beacon.LCP].N)
	assert.Zero(t, s.Vitals[beacon.LCP].Lost)
	assert.Equal(t, 100., s.Vitals[beacon.LCP].P75)
	assert.Positive(t, s.API.Lost)
	assert.Positive(t, s.SessionsLost, "resource identity loss can admit replayed session activity")
}
func TestIdentityLossOnlyInvalidatesAffectedMeasurements(t *testing.T) {
	a, now := newAgg(5 * time.Minute)
	for i := 0; i < maxWindowObservations+1; i++ {
		b := mk(*now, fmt.Sprint(i), "/", v(beacon.FCP, 100))
		b.Events = nil
		a.Ingest(b)
	}
	s := a.Snapshot()
	assert.Positive(t, s.Vitals[beacon.FCP].Lost)
	assert.Zero(t, s.Vitals[beacon.LCP].Lost)
	assert.Zero(t, s.WindowLost)
	assert.Zero(t, s.API.Lost)
	assert.Zero(t, s.Load.Lost)
	assert.Positive(t, s.SessionsLost)
}
func TestCapacityEvictsExpiredReceiptsBeforeCurrentObservations(t *testing.T) {
	a, now := newAgg(30 * time.Minute)
	put := func(id string, at time.Time) {
		b := mk(at, id, "/", v(beacon.LCP, 100))
		b.Events = nil
		a.Ingest(b)
	}
	put("fresh", *now)
	for i := 0; i < maxWindowObservations-1; i++ {
		put(fmt.Sprint(i), now.Add(-2*time.Second))
	}
	*now = now.Add(30*time.Minute - time.Second)
	put("new", *now)
	s := a.Snapshot()
	assert.Equal(t, 2, s.Vitals[beacon.LCP].N)
	assert.Equal(t, 2, s.ObservedSessions)
	assert.Zero(t, s.Vitals[beacon.LCP].Lost, "expired observations do not create measurement loss")
}
func TestLateRevisionRetainsChronologicalCapacityOrder(t *testing.T) {
	a, now := newAgg(time.Minute)
	first := mk(*now, "oldest", "/", v(beacon.LCP, 100))
	first.Events = nil
	a.Ingest(first)
	for i := 0; i < maxWindowObservations-1; i++ {
		b := mk(now.Add(time.Nanosecond), fmt.Sprint(i), "/", v(beacon.FCP, 100))
		b.Events = nil
		a.Ingest(b)
	}
	first.Vitals[0].Revision = 2
	a.Ingest(first)
	b := mk(now.Add(2*time.Nanosecond), "new", "/", v(beacon.FCP, 100))
	b.Events = nil
	a.Ingest(b)
	s := a.Snapshot()
	assert.Zero(
		t,
		s.Vitals[beacon.LCP].N,
		"an older revision receipt does not move the observation past newer receipts",
	)
	assert.Equal(t, maxWindowObservations, s.Vitals[beacon.FCP].N)
}
func TestLateDominantErrorAndResourceHostEnterTopGroups(t *testing.T) {
	t.Run("errors", func(t *testing.T) {
		a, now := newAgg(time.Minute)
		for i := 0; i < 200; i++ {
			b := mk(*now, "s", "/")
			b.Events = nil
			b.Errors = []beacon.Error{{Fingerprint: fmt.Sprintf("early-%03d", i)}}
			a.Ingest(b)
		}
		for i := 0; i < 300; i++ {
			b := mk(*now, "s", "/")
			b.Events = nil
			b.Errors = []beacon.Error{{Fingerprint: "late"}}
			a.Ingest(b)
		}
		groups := a.Snapshot().ErrorGroups
		require.Len(t, groups, 11)
		assert.Equal(t, "late", groups[0].Fingerprint)
		assert.EqualValues(t, 300, groups[0].Count)
		assert.True(t, groups[10].Other)
		assert.EqualValues(t, 191, groups[10].Count)
	})
	t.Run("hosts", func(t *testing.T) {
		a, now := newAgg(time.Minute)
		for i := 0; i < 1200; i++ {
			host := fmt.Sprintf("host-%04d.example.com", i)
			if i >= 1000 {
				host = "late.example.com"
			}
			b := mk(*now, "s", "/")
			b.Events = nil
			b.Resources = []beacon.Resource{{ID: fmt.Sprint(i), Host: host, HasDuration: true, DurationMS: float64(i)}}
			a.Ingest(b)
		}
		groups := a.Snapshot().ResourceHosts
		require.Len(t, groups, 11)
		assert.Equal(t, "late.example.com", groups[0].Host)
		assert.EqualValues(t, 200, groups[0].Count)
		assert.True(t, groups[10].Other)
		assert.EqualValues(t, 991, groups[10].Count)
		assert.Zero(t, groups[10].Lost)
		assert.Equal(t, 991, groups[10].Duration.N)
	})
}

func TestOlderIncomingCapacityLossPreservesNewerMeasurementsAndDetail(t *testing.T) {
	a, now := newAgg(time.Minute)
	for i := 0; i < maxWindowObservations; i++ {
		b := mk(*now, fmt.Sprint(i), "/", v(beacon.FCP, 100))
		b.Events = nil
		a.Ingest(b)
	}
	old := mk(now.Add(-time.Second), "old", "/old", v(beacon.LCP, 200))
	old.Events = nil
	result := a.Ingest(old)
	require.Len(
		t,
		result.Observation.Vitals,
		1,
		"measurement capacity must not filter accepted investigation/export detail",
	)
	assert.Equal(t, "/old", result.Observation.Vitals[0].Origin.PageGroup)
	s := a.Snapshot()
	assert.Equal(t, maxWindowObservations, s.Vitals[beacon.FCP].N)
	assert.Zero(t, s.Vitals[beacon.LCP].N)
	assert.Zero(t, s.Vitals[beacon.FCP].Lost)
	assert.Positive(t, s.Vitals[beacon.LCP].Lost)
}

func TestActivitySummaryMatchesSnapshotThroughLossAndExpiry(t *testing.T) {
	a, now := newAgg(time.Minute)
	for i := 0; i < maxWindowObservations+1; i++ {
		b := mk(*now, fmt.Sprint(i), "/", v(beacon.FCP, 100))
		b.ViewID = "view"
		b.View = "view"
		b.Events = append(b.Events, beacon.Event{
			Kind:     beacon.EventView,
			ID:       "view",
			Revision: 1,
		})
		b.Errors = []beacon.Error{{Fingerprint: "error"}}
		a.Ingest(b)
	}
	for _, advance := range []time.Duration{0, time.Minute + time.Second} {
		*now = now.Add(advance)
		s := a.Snapshot()
		activity := a.Activity()
		assert.EqualValues(t, s.PageviewsWindow, activity.PageviewsWindow)
		assert.Equal(t, s.ApplicationViewsWindow, activity.ApplicationViewsWindow)
		assert.Equal(t, s.JSErrorsWindow, activity.JSErrorsWindow)
		assert.Equal(t, s.ObservedSessions, activity.ObservedSessions)
		assert.Equal(t, s.WindowLost, activity.WindowLost)
		assert.Equal(t, s.SessionsLost, activity.SessionsLost)
	}
}

func TestFacetsRankAllRetainedGroupsBeforeFolding(t *testing.T) {
	a, now := newAgg(time.Minute, SiteCfg{
		Name:       "s",
		PageGroups: 10,
		Countries:  10,
	})
	for i := 0; i < 1400; i++ {
		group := fmt.Sprintf("/page_%04d", i)
		if i >= 1100 {
			group = "/leader"
		}
		b := mk(*now, fmt.Sprint(i), group, v(beacon.LCP, 100))
		b.AppVersion = group
		b.View = group
		b.ViewID = "view"
		b.Events = append(b.Events, beacon.Event{
			Kind:     beacon.EventView,
			ID:       "view",
			Revision: 1,
		})
		a.Ingest(b)
	}
	s := a.Snapshot()
	for _, kind := range []string{KindPage, KindVersion, KindView} {
		groups := s.Breakdowns[kind]
		require.Len(t, groups, 11)
		assert.Equal(t, "/leader", groups[0].Value)
		if kind == KindView {
			assert.EqualValues(t, 300, groups[0].ApplicationViews)
			assert.EqualValues(t, 1091, groups[10].ApplicationViews)
		} else {
			assert.EqualValues(t, 300, groups[0].Pageviews)
			assert.EqualValues(t, 1091, groups[10].Pageviews)
		}
		assert.True(t, groups[10].Other)
		assert.Zero(t, groups[10].Lost)
	}
	pages := a.Pages()
	require.Len(t, pages, 1101)
	for _, page := range pages {
		assert.Zero(t, page.Lost)
	}
}

func TestCapacityDroppedVitalKeepsDocumentOriginInLiveAndHistory(t *testing.T) {
	a, now := newAgg(time.Minute)
	sink := &fakeHistorySink{}
	a.SetHistorySink(sink)
	b := mk(now.Add(-2*time.Second), "document", "/entry", v(beacon.LCP, 100))
	b.Events = nil
	b.Browser, b.Device, b.Country, b.AppVersion = "entry-browser", "desktop", "GR", "1"
	a.Ingest(b)
	// Navigation occupies two measurement slots but one identity slot, so the
	// vital's frozen origin survives eviction of its measurement.
	for i := range maxWindowObservations / 2 {
		a.Ingest(&beacon.Beacon{
			Site:         "s",
			ExperienceID: fmt.Sprintf("navigation-%d", i),
			Received:     *now,
			Navigation: &beacon.Navigation{
				Revision: 1,
				LoadMS:   20,
				HasLoad:  true,
				DCLMS:    10,
				HasDCL:   true,
			},
		})
	}
	b.Received = now.Add(-time.Second)
	b.PageGroup = "/changed"
	b.Browser, b.Device, b.Country, b.AppVersion = "new-browser", "mobile", "US", "2"
	b.UserID, b.View, b.ViewID = "current-user", "checkout", "current-view"
	b.Vitals[0].Revision = 2
	b.Vitals[0].Value = 5001
	result := a.Ingest(b)
	require.Len(t, result.Observation.Vitals, 1)
	assert.Equal(t, "/entry", result.Observation.Vitals[0].Origin.PageGroup)
	rows, _ := a.Live(0, 100)
	require.Len(t, rows, 2)
	assert.Equal(t, uint64(2), rows[1].Revision)
	assert.Equal(t, "/entry", rows[1].Page)
	assert.Equal(t, "entry-browser", rows[1].Browser)
	assert.Equal(t, "desktop", rows[1].Device)
	assert.Equal(t, "GR", rows[1].Country)
	assert.Equal(t, "checkout", rows[1].View)
	require.Len(t, sink.events, 2)
	retained := sink.events[1]
	assert.Equal(t, "vital", retained.Type)
	assert.Equal(t, "/entry", retained.Page)
	assert.Equal(t, "entry-browser", retained.Browser)
	assert.Equal(t, "desktop", retained.Device)
	assert.Equal(t, "GR", retained.Country)
	assert.Equal(t, "1", retained.Version)
	assert.Equal(t, "current-user", retained.UserID)
	assert.Equal(t, "checkout", retained.View)
	assert.Equal(t, "current-view", retained.ViewID)
}
