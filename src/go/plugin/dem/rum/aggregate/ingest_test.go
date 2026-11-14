// SPDX-License-Identifier: GPL-3.0-or-later

package aggregate

import (
	"encoding/json"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/faro"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	pages := a.Pages("")
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
	pages := a.Pages("")
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

// A vital's replay evidence decides whether a later report of the same vital is
// a newer revision, a replay, or a different measurement reusing the name.
func TestVitalIdentityAdmission(t *testing.T) {
	tests := map[string]struct {
		renewedAfter  time.Duration // when set, revision 2 is admitted first, this long after the first report
		metricID      string
		revision      uint64
		clockAdvance  time.Duration
		receivedAhead time.Duration
		admitted      bool
		invalid       uint64
	}{
		"newer revision replaces the value": {
			metricID: "lcp-1", revision: 2, admitted: true,
		},
		"replayed revision is ignored": {
			metricID: "lcp-1", revision: 1,
		},
		"another metric identity is invalid": {
			metricID: "lcp-2", revision: 2, invalid: 1,
		},
		"another metric identity stays invalid after the window": {
			metricID: "lcp-2", revision: 2, clockAdvance: 6 * time.Minute, invalid: 1,
		},
		"newer revision is admitted after the window": {
			metricID: "lcp-1", revision: 2, clockAdvance: 6 * time.Minute, admitted: true,
		},
		"a newer revision extends the evidence": {
			renewedAfter: 20 * time.Minute, metricID: "lcp-2", revision: 3, clockAdvance: 20 * time.Minute, invalid: 1,
		},
		"a replay after an extension is still ignored": {
			renewedAfter: 20 * time.Minute, metricID: "lcp-1", revision: 1, clockAdvance: 20 * time.Minute,
		},
		"evidence older than its retention at receipt no longer applies": {
			metricID: "lcp-2", revision: 1, receivedAhead: identityRetention + time.Nanosecond, admitted: true,
		},
		"evidence at exactly its retention still applies": {
			metricID: "lcp-1", revision: 1, receivedAhead: identityRetention,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			a, now := newAgg(5 * time.Minute)
			first := mk(*now, "session", "/page", beacon.Vital{ID: "lcp-1", Revision: 1, Name: beacon.LCP, Value: 100})
			require.Len(t, a.Ingest(first).Observation.Vitals, 1)
			if tc.renewedAfter > 0 {
				*now = now.Add(tc.renewedAfter)
				renewal := *first
				renewal.Received = *now
				renewal.Vitals = []beacon.Vital{{ID: "lcp-1", Revision: 2, Name: beacon.LCP, Value: 150}}
				require.Len(t, a.Ingest(&renewal).Observation.Vitals, 1)
			}

			*now = now.Add(tc.clockAdvance)
			next := *first
			next.Received = now.Add(tc.receivedAhead)
			next.Vitals = []beacon.Vital{{ID: tc.metricID, Revision: tc.revision, Name: beacon.LCP, Value: 200}}
			result := a.Ingest(&next)

			assert.Equal(t, tc.admitted, len(result.Observation.Vitals) == 1)
			assert.Equal(t, tc.invalid, a.Snapshot().Counters[CounterInvalidMeasurements])
		})
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

// Everything one report retains refers to one attribute set; a later report
// of the same vital keeps the set of its first admission.
func TestReportItemsShareAttributes(t *testing.T) {
	a, now := newAgg(5 * time.Minute)
	b := mk(*now, "session", "/page", v(beacon.LCP, 100), v(beacon.CLS, 0.1))
	b.Resources = []beacon.Resource{{ID: "resource", Host: "example.org"}}
	a.Ingest(b)

	lcp := observationKey{b.ExperienceID, beacon.LCP}
	cls := observationKey{b.ExperienceID, beacon.CLS}
	shared := a.site.observations.get(lcp).Value.attrs
	require.NotNil(t, shared)
	assert.Same(t, shared, a.site.observations.get(cls).Value.attrs)
	assert.Same(t, shared, a.site.vitalIdentities.get(lcp).Value.attrs)
	assert.Same(t, shared, a.site.vitalIdentities.get(cls).Value.attrs)
	assert.Same(t, shared, a.site.activity.entries[0].Value.attrs)
	assert.Same(t, shared, a.site.resources.entries[0].Value.attrs)

	*now = now.Add(time.Second)
	update := *b
	update.Received = *now
	update.Browser = "Firefox"
	update.Vitals = []beacon.Vital{{ID: beacon.LCP, Revision: 2, Name: beacon.LCP, Value: 200}}
	a.Ingest(&update)
	assert.Same(t, shared, a.site.observations.get(lcp).Value.attrs)
	assert.Equal(t, "Chrome", shared.browser)
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

// Navigation needs an experience and a revision. A non-finite or negative
// timing field is dropped from an otherwise admitted revision.
func TestInvalidNavigationTimingIsCounted(t *testing.T) {
	a, now := newAgg(time.Minute)
	invalid := func() uint64 { return a.Snapshot().Counters[CounterInvalidMeasurements] }
	b := mk(*now, "s", "/")
	b.Events = nil
	b.Navigation = &beacon.Navigation{Revision: 0, HasLoad: true, LoadMS: 10}
	assert.Nil(t, a.Ingest(b).Observation.Navigation)
	b.ExperienceID = ""
	b.Navigation = &beacon.Navigation{Revision: 1, HasLoad: true, LoadMS: 10}
	assert.Nil(t, a.Ingest(b).Observation.Navigation)
	require.EqualValues(t, 2, invalid())

	b.ExperienceID = "document"
	b.Navigation = &beacon.Navigation{Revision: 1, HasLoad: true, LoadMS: math.NaN(), HasDCL: true, DCLMS: -1}
	got := a.Ingest(b).Observation.Navigation
	require.NotNil(t, got)
	assert.False(t, got.HasLoad)
	assert.False(t, got.HasDCL)
	assert.EqualValues(t, 4, invalid())
	s := a.Snapshot()
	assert.Zero(t, s.Load.N)
	assert.Zero(t, s.DCL.N)
}

// Decoded navigation reports without valid timings do not replace a revision.
func TestNavigationRevisionIgnoresEmptyTimingReports(t *testing.T) {
	for name, timings := range map[string]string{
		"metadata only":   "",
		"invalid timings": `,"pageLoadTime":"-1","domContentLoadHandlerTime":"NaN"`,
	} {
		for _, separate := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/separate=%t", name, separate), func(t *testing.T) {
				a := New(time.Hour, SiteCfg{Name: "shop"})
				valid := `{"name":"faro.performance.navigation","attributes":{"observation_id":"nav1","observation_sequence":"1","pageLoadTime":"123","domContentLoadHandlerTime":"10"}}`
				empty := `{"name":"faro.performance.navigation","attributes":{"observation_id":"nav2","observation_sequence":"2"` + timings + `}}`
				batches := []string{valid + "," + empty}
				if separate {
					batches = []string{valid, empty}
				}
				for i, events := range batches {
					raw := `{"meta":{"page":{"id":"document","url":"https://shop.example/entry"}},"events":[` + events + `]}`
					b, err := faro.Decode([]byte(raw), faro.Options{Site: "shop", Now: time.Now()})
					require.NoError(t, err)
					if separate && i == 1 {
						assert.Nil(t, b.Navigation)
					}
					a.Ingest(b)
				}
				snapshot := a.Snapshot()
				assert.EqualValues(t, 1, snapshot.Load.N)
				assert.Equal(t, float64(123), snapshot.Load.P75)
				assert.EqualValues(t, 1, snapshot.DCL.N)
				assert.Equal(t, float64(10), snapshot.DCL.P75)

				// A valid partial revision still replaces the previous report,
				// including removing its now-absent load measurement.
				raw := `{"meta":{"page":{"id":"document","url":"https://shop.example/entry"}},"events":[{"name":"faro.performance.navigation","attributes":{"observation_id":"nav3","observation_sequence":"3","domContentLoadHandlerTime":"0"}}]}`
				b, err := faro.Decode([]byte(raw), faro.Options{Site: "shop", Now: time.Now()})
				require.NoError(t, err)
				a.Ingest(b)
				snapshot = a.Snapshot()
				assert.Zero(t, snapshot.Load.N)
				assert.EqualValues(t, 1, snapshot.DCL.N)
				assert.Zero(t, snapshot.DCL.P75)
			})
		}
	}
}

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

func TestSessionLifecycleOccurrenceAdmission(t *testing.T) {
	for _, name := range []string{"session_start", "session_resume", "session_extend"} {
		t.Run(name, func(t *testing.T) {
			now := time.Now()
			decode := func(id, revision string, received time.Time) *beacon.Beacon {
				raw, err := json.Marshal(map[string]any{
					"meta":   map[string]any{"page": map[string]string{"id": "document", "url": "https://example.org/entry"}, "session": map[string]string{"id": "session"}},
					"events": []any{map[string]any{"name": name, "attributes": map[string]string{"observation_id": id, "observation_sequence": revision}}},
				})
				require.NoError(t, err)
				b, err := faro.Decode(raw, faro.Options{
					Site: "shop",
					Now:  received,
				})
				require.NoError(t, err)
				return b
			}
			a := New(time.Minute, SiteCfg{
				Name: "shop",
				Investigate: InvestigateCfg{
					Rate: 1,
				},
			})
			history := &fakeHistorySink{}
			a.SetHistorySink(history)
			first := a.Ingest(decode("occurrence-1", "1", now.Add(-2*time.Minute)))
			require.Len(t, first.Observation.Events, 1)
			require.Zero(t, a.Activity().ObservedSessions)
			// An occurrence is counted once, including reports with a higher revision.
			for _, revision := range []string{"1", "2"} {
				replay := a.Ingest(decode("occurrence-1", revision, now))
				require.Empty(t, replay.Observation.Events)
				require.Zero(t, a.Activity().ObservedSessions)
			}
			fresh := a.Ingest(decode("occurrence-2", "3", now))
			require.Len(t, fresh.Observation.Events, 1)
			require.EqualValues(t, 1, a.Activity().ObservedSessions)
			require.Zero(t, a.Activity().PageviewsWindow)
			require.Zero(t, a.Snapshot().Counters[CounterInvalidMeasurements])
			require.Empty(t, history.events, "session lifecycle does not produce generic history rows")
			events, found := a.SessionEvents("session")
			require.True(t, found)
			require.Empty(t, events, "session lifecycle does not produce native timeline rows")
		})
	}
}

func TestSessionLifecycleRequiresIdentity(t *testing.T) {
	for _, missing := range []string{"experience", "session", "id", "revision"} {
		t.Run(missing, func(t *testing.T) {
			b := &beacon.Beacon{
				Site:         "shop",
				Received:     time.Now(),
				ExperienceID: "document",
				SessionID:    "session",
				Events:       []beacon.Event{{Kind: beacon.EventSession, Name: "session_start", ID: "occurrence", Revision: 1}},
			}
			switch missing {
			case "experience":
				b.ExperienceID = ""
			case "session":
				b.SessionID = ""
			case "id":
				b.Events[0].ID = ""
			case "revision":
				b.Events[0].Revision = 0
			}
			a := New(time.Minute, SiteCfg{
				Name: "shop",
			})
			require.Empty(t, a.Ingest(b).Observation.Events)
			require.Zero(t, a.Activity().ObservedSessions)
			require.EqualValues(t, 1, a.Snapshot().Counters[CounterInvalidMeasurements])
		})
	}
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

func TestInputRemainsImmutableThroughFiltering(t *testing.T) {
	a, now := newAgg(time.Minute)
	b := mk(*now, "s", "/", v(beacon.CLS, .1))
	b.Resources = []beacon.Resource{{ID: "r", HasDuration: true, DurationMS: 0}}
	a.Ingest(b)
	second := a.Ingest(b)
	require.NotNil(t, second.Observation)
	assert.Empty(t, second.Observation.Events)
	assert.Empty(t, second.Observation.Vitals)
	assert.Empty(t, second.Observation.Resources)
	assert.Len(t, b.Vitals, 1)
	assert.Len(t, b.Events, 1)
	assert.Len(t, b.Resources, 1)
}
