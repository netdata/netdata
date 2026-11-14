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

func TestReceiptIndexOrder(t *testing.T) {
	t0 := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	type index = receiptIndex[string, struct{}]
	tests := map[string]struct {
		change func(x *index)
		want   []string
	}{
		"receipt time, then insertion order": {
			change: func(*index) {},
			want:   []string{"a", "b", "c", "d"},
		},
		"a later receipt moves the entry": {
			change: func(x *index) { x.renew(x.get("a"), t0.Add(time.Minute)) },
			want:   []string{"b", "c", "d", "a"},
		},
		"an earlier receipt keeps the entry in place": {
			change: func(x *index) { x.renew(x.get("d"), t0.Add(-time.Minute)) },
			want:   []string{"a", "b", "c", "d"},
		},
		"an equal receipt keeps insertion order": {
			change: func(x *index) { x.renew(x.get("b"), t0.Add(time.Second)) },
			want:   []string{"a", "b", "c", "d"},
		},
		"removal leaves the rest ordered": {
			change: func(x *index) { x.remove(x.get("b")) },
			want:   []string{"a", "c", "d"},
		},
		"expiry removes only entries received before the cutoff": {
			change: func(x *index) { x.expire(t0.Add(time.Second)) },
			want:   []string{"b", "c", "d"},
		},
		"a stale entry is removed on lookup": {
			change: func(x *index) {
				require.Nil(t, x.current("a", t0.Add(time.Minute+time.Nanosecond), time.Minute))
				require.NotNil(t, x.current("b", t0.Add(time.Minute+time.Second), time.Minute))
			},
			want: []string{"b", "c", "d"},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			x := newReceiptIndex[string, struct{}]()
			x.add(t0.Add(2*time.Second), "d", struct{}{})
			x.add(t0.Add(time.Second), "b", struct{}{})
			x.add(t0, "a", struct{}{})
			x.add(t0.Add(time.Second), "c", struct{}{})
			tc.change(&x)
			require.Equal(t, len(tc.want), x.len())
			var got []string
			for e := x.order.popExpired(t0.Add(time.Hour)); e != nil; e = x.order.popExpired(t0.Add(time.Hour)) {
				got = append(got, e.key)
				assert.Same(t, e, x.get(e.key))
			}
			assert.Equal(t, tc.want, got)
		})
	}
}

// Every retained population stays exact: no count cap, sampling or
// approximation applies within the window.
func TestRetainedPopulationsAreExact(t *testing.T) {
	a, now := newAgg(5*time.Minute, SiteCfg{Name: "s", PageGroups: 20, Countries: 20})
	const documents = 10001
	for i := range documents {
		b := mk(*now, fmt.Sprint(i), "/entry", v(beacon.LCP, 100), v(beacon.INP, 10), v(beacon.CLS, 0.1))
		b.PageHost = "example.org"
		b.Resources = []beacon.Resource{{ID: "resource", Host: "example.org", Initiator: "fetch", HasDuration: true, DurationMS: 5}}
		a.Ingest(b)
		a.Ingest(b) // Every population must retain replay evidence, too.
	}
	s := a.Snapshot()
	require.EqualValues(t, documents, s.PageviewsWindow)
	require.Equal(t, documents, s.ObservedSessions)
	for _, name := range []string{beacon.LCP, beacon.INP, beacon.CLS} {
		require.Equal(t, documents, s.Vitals[name].N, name)
		require.Equal(t, documents, s.Vitals[name].Good, name)
	}
	require.Equal(t, 100., s.Vitals[beacon.LCP].P75)
	require.Equal(t, documents, s.API.N)
	require.Equal(t, 5., s.API.P75)
	pages := a.Pages("")
	require.Len(t, pages, 1)
	require.Equal(t, documents, pages[0].Sessions)
	require.Equal(t, documents, pages[0].PageviewsWindow)
}

func TestCanonicalPopulationExpiresInclusively(t *testing.T) {
	a, now := newAgg(time.Minute)
	for i := range 10001 {
		b := mk(*now, "", fmt.Sprint(i), v(beacon.LCP, 100))
		b.Events = nil
		a.Ingest(b)
	}
	*now = now.Add(time.Minute)
	s := a.Snapshot()
	assert.Equal(t, 10001, s.Vitals[beacon.LCP].N)
	assert.Equal(t, 100., s.Vitals[beacon.LCP].P75)
	*now = now.Add(time.Nanosecond)
	assert.Zero(t, a.Snapshot().Vitals[beacon.LCP].N)
}

func TestPopulationExpiresByEachReceipt(t *testing.T) {
	a, now := newAgg(time.Minute)
	for i := range 10000 {
		b := mk(*now, "", fmt.Sprint(i), v(beacon.LCP, 100))
		b.Events = nil
		a.Ingest(b)
	}
	*now = now.Add(30 * time.Second)
	for i := range 2 {
		b := mk(*now, "", fmt.Sprint(i+10000), v(beacon.LCP, 200))
		b.Events = nil
		a.Ingest(b)
	}
	*now = now.Add(31 * time.Second)
	s := a.Snapshot()
	assert.Equal(t, 2, s.Vitals[beacon.LCP].N)
	assert.Equal(t, 200., s.Vitals[beacon.LCP].P75)
	*now = now.Add(30 * time.Second)
	assert.Zero(t, a.Snapshot().Vitals[beacon.LCP].N)
}

func TestIngestExpiresCanonicalStateWithoutReads(t *testing.T) {
	a, now := newAgg(5 * time.Minute)
	first := mk(*now, "old", "/old", v(beacon.LCP, 100))
	first.Resources = []beacon.Resource{{ID: "resource", Host: "example.org"}}
	a.Ingest(first)
	*now = now.Add(5*time.Minute + time.Nanosecond)
	a.Ingest(mk(*now, "current", "/current", v(beacon.LCP, 200)))
	// Inspect retained ownership before any reader can trigger cleanup.
	require.Equal(t, 1, a.site.observations.len())
	require.Equal(t, 1, a.site.activity.Len())
	require.Zero(t, a.site.resources.Len())
	require.Equal(t, 1, a.site.sessions.len())
	require.Equal(t, 1, a.site.pageSessions.len())
	identities := func() int { return a.site.identities.len() + a.site.vitalIdentities.len() }
	require.Equal(t, 4, identities(), "identity evidence outlives measurement windows")
	require.Equal(t, 1, a.site.resourceIdentities.len())
	*now = now.Add(30 * time.Minute)
	a.Ingest(mk(*now, "last", "/last"))
	require.Equal(t, 3, identities(), "the exact 30-minute boundary remains included")
	*now = now.Add(time.Nanosecond)
	a.Ingest(&beacon.Beacon{Site: "s", Received: *now})
	require.Equal(t, 1, identities())
	require.Zero(t, a.site.resourceIdentities.len())
}

func TestExpiryPreservesCurrentObservations(t *testing.T) {
	a, now := newAgg(30 * time.Minute)
	put := func(id string, at time.Time) {
		b := mk(at, id, "/", v(beacon.LCP, 100))
		b.Events = nil
		a.Ingest(b)
	}
	put("fresh", *now)
	for i := 0; i < 10000-1; i++ {
		put(fmt.Sprint(i), now.Add(-2*time.Second))
	}
	*now = now.Add(30*time.Minute - time.Second)
	put("new", *now)
	s := a.Snapshot()
	assert.Equal(t, 2, s.Vitals[beacon.LCP].N)
	assert.Equal(t, 2, s.ObservedSessions)
}

// A replaced value belongs to the window of its latest receipt.
func TestNewerRevisionRenewsWindowMembership(t *testing.T) {
	a, now := newAgg(5 * time.Minute)
	first := mk(*now, "session", "/page", beacon.Vital{ID: "lcp-1", Revision: 1, Name: beacon.LCP, Value: 100})
	a.Ingest(first)

	*now = now.Add(4 * time.Minute)
	update := *first
	update.Received = *now
	update.Vitals = []beacon.Vital{{ID: "lcp-1", Revision: 2, Name: beacon.LCP, Value: 200}}
	a.Ingest(&update)

	*now = now.Add(2 * time.Minute)
	assert.Equal(t, VitalStats{N: 1, P50: 200, P75: 200, P95: 200, Good: 1}, a.Snapshot().Vitals[beacon.LCP])
	*now = now.Add(3*time.Minute + time.Nanosecond)
	assert.Equal(t, VitalStats{}, a.Snapshot().Vitals[beacon.LCP])
}

func TestOlderRevisionCannotRefreshWindow(t *testing.T) {
	a, now := newAgg(time.Minute)
	b := mk(*now, "s", "/", v(beacon.CLS, .5))
	b.Vitals[0].Revision = 2
	a.Ingest(b)
	*now = now.Add(50 * time.Second)
	b.Received = *now
	b.Vitals[0].Revision = 1
	b.Vitals[0].Value = .1
	a.Ingest(b)
	*now = now.Add(11 * time.Second)
	assert.Zero(t, a.Snapshot().Vitals[beacon.CLS].N)
}

func TestLateRevisionRetainsChronologicalExpiryOrder(t *testing.T) {
	a, now := newAgg(time.Minute)
	first := mk(*now, "oldest", "/", v(beacon.LCP, 100))
	first.Events = nil
	a.Ingest(first)
	for i := 0; i < 10000-1; i++ {
		b := mk(now.Add(time.Nanosecond), fmt.Sprint(i), "/", v(beacon.FCP, 100))
		b.Events = nil
		a.Ingest(b)
	}
	*now = now.Add(30 * time.Second)
	first.Vitals[0].Revision = 2
	a.Ingest(first)
	b := mk(first.Received.Add(2*time.Nanosecond), "new", "/", v(beacon.FCP, 100))
	b.Events = nil
	a.Ingest(b)
	*now = first.Received.Add(time.Minute + time.Nanosecond)
	s := a.Snapshot()
	assert.Zero(t, s.Vitals[beacon.LCP].N, "an older revision receipt must not renew expiry")
	assert.Equal(t, 10000, s.Vitals[beacon.FCP].N)
}

func TestOutOfOrderReceiptsDoNotHideExpiredState(t *testing.T) {
	a, now := newAgg(time.Minute)
	a.Ingest(mk(now.Add(-10*time.Second), "new", "/new", v(beacon.LCP, 10)))
	a.Ingest(mk(now.Add(-2*time.Minute), "old", "/old", v(beacon.LCP, 1000)))
	snap := a.Snapshot()
	assert.Equal(t, 1, snap.Vitals[beacon.LCP].N)
	assert.EqualValues(t, 1, snap.PageviewsWindow)
	assert.Equal(t, 1, snap.ObservedSessions)
	rows, _ := a.Live(0, 100, "")
	require.NotEmpty(t, rows)
	for _, row := range rows {
		assert.Equal(t, "/new", row.Page)
	}
}

func TestOlderIncomingReceiptRetainsMeasurementsAndDetail(t *testing.T) {
	a, now := newAgg(time.Minute)
	for i := 0; i < 10000; i++ {
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
		"an out-of-order receipt retains accepted investigation/export detail",
	)
	assert.Equal(t, "/old", result.Observation.Vitals[0].Origin.PageGroup)
	s := a.Snapshot()
	assert.Equal(t, 10000, s.Vitals[beacon.FCP].N)
	assert.Equal(t, 1, s.Vitals[beacon.LCP].N)
	assert.Equal(t, 200., s.Vitals[beacon.LCP].P75)
}

func TestIdentityRetentionPreventsExpiredSessionReplay(t *testing.T) {
	a, now := newAgg(time.Minute)
	old := mk(now.Add(-61*time.Second), "expired", "/old", v(beacon.LCP, 100))
	old.Events = nil
	a.Ingest(old)
	require.Zero(t, a.Snapshot().ObservedSessions)
	for i := range 10001 {
		a.Ingest(&beacon.Beacon{
			Site: "s", ExperienceID: fmt.Sprint(i), Received: *now,
			Events: []beacon.Event{{Kind: beacon.EventDocument, ID: "activation", Revision: 1}},
		})
	}
	old.Received = *now
	result := a.Ingest(old)
	require.Empty(t, result.Observation.Vitals)
	require.Zero(t, a.Snapshot().ObservedSessions)
	require.Zero(t, a.Activity().ObservedSessions)
	for _, page := range a.Pages("") {
		require.NotEqual(t, "/old", page.Page)
	}
}

func TestExpiredVitalKeepsDocumentOriginInLiveAndHistory(t *testing.T) {
	a, now := newAgg(time.Minute)
	sink := &fakeHistorySink{}
	a.SetHistorySink(sink)
	b := mk(now.Add(-2*time.Second), "document", "/entry", v(beacon.LCP, 100))
	b.Events = nil
	b.Browser, b.Device, b.Country, b.AppVersion = "entry-browser", "desktop", "GR", "1"
	a.Ingest(b)
	// The measurement expires while its identity still owns the frozen origin.
	*now = now.Add(time.Minute)
	require.Zero(t, a.Snapshot().Vitals[beacon.LCP].N)
	b.Received = *now
	b.PageGroup = "/changed"
	b.Browser, b.Device, b.Country, b.AppVersion = "new-browser", "mobile", "US", "2"
	b.UserID, b.View, b.ViewID = "current-user", "checkout", "current-view"
	b.Vitals[0].Revision = 2
	b.Vitals[0].Value = 5001
	result := a.Ingest(b)
	require.Len(t, result.Observation.Vitals, 1)
	assert.Equal(t, "/entry", result.Observation.Vitals[0].Origin.PageGroup)
	rows, _ := a.Live(0, 100, "")
	require.Len(t, rows, 1)
	assert.Equal(t, uint64(2), rows[0].Revision)
	assert.Equal(t, "/entry", rows[0].Page)
	assert.Equal(t, "entry-browser", rows[0].Browser)
	assert.Equal(t, "desktop", rows[0].Device)
	assert.Equal(t, "GR", rows[0].Country)
	assert.Equal(t, "checkout", rows[0].View)
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

// The observation window controls session measurement independently of details.
func TestObservedSessionsAgeByWindow(t *testing.T) {
	a, now := newAgg(30 * time.Minute)
	a.Ingest(mk(*now, "s1", "/a"))
	a.Ingest(mk(*now, "s2", "/a"))
	*now = now.Add(20 * time.Minute)
	a.Ingest(mk(*now, "s1", "/b"))
	*now = t0.Add(31 * time.Minute) // s2 idle 31m, s1 idle 11m
	if got := a.Snapshot().ObservedSessions; got != 1 {
		t.Fatalf("active=%d want 1", got)
	}
}

func TestObservedSessionsRetainAllAcceptedActivity(t *testing.T) {
	a, now := newAgg(time.Minute)
	for i := range 10000 + 1 {
		b := mk(*now, fmt.Sprint(i), "/")
		b.Events = nil
		b.Logs = []beacon.Log{{Message: "accepted activity"}}
		a.Ingest(b)
	}
	s := a.Snapshot()
	assert.Equal(t, 10001, s.ObservedSessions)
	assert.Equal(t, 10001, a.Pages("")[0].Sessions)
	*now = now.Add(time.Minute + time.Second)
	assert.Zero(t, a.Snapshot().ObservedSessions)
}

func TestSessionMeasurementIndependentOfDetailLRU(t *testing.T) {
	a, now := newAgg(time.Minute)
	for i := range maxTrackedSessions + 10 {
		a.Ingest(mk(*now, fmt.Sprint(i), "/"))
	}
	assert.Equal(t, maxTrackedSessions+10, a.Snapshot().ObservedSessions)
	assert.Equal(t, maxTrackedSessions+10, a.Pages("")[0].Sessions)
	*now = now.Add(time.Minute + time.Second)
	assert.Zero(t, a.Activity().ObservedSessions)
	assert.Empty(t, a.Pages(""))
}

func TestDuplicateReportDoesNotRefreshObservedSessions(t *testing.T) {
	a, now := newAgg(time.Minute)
	b := mk(*now, "browser", "/entry", v(beacon.LCP, 100))
	a.Ingest(b)
	assert.Equal(t, 1, a.Snapshot().ObservedSessions)
	*now = now.Add(time.Minute + time.Second)
	assert.Zero(t, a.Snapshot().ObservedSessions)
	assert.Empty(t, a.Pages(""))
	b.Received = *now
	result := a.Ingest(b)
	assert.Empty(t, result.Observation.Vitals)
	assert.Empty(t, result.Observation.Events)
	assert.Zero(t, a.Snapshot().ObservedSessions)
	assert.Empty(t, a.Pages(""))
	b.Vitals[0].Revision = 2
	a.Ingest(b)
	assert.Equal(t, 1, a.Snapshot().ObservedSessions)
	pages := a.Pages("")
	require.Len(t, pages, 1)
	assert.Equal(t, 1, pages[0].Sessions)
}
