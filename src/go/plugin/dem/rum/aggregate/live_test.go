// SPDX-License-Identifier: GPL-3.0-or-later
package aggregate

import (
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLiveSeparatesIdentifiedReportsFromDocumentActivity(t *testing.T) {
	a, now := newAgg(time.Minute)
	b := mk(*now, "s", "/", v(beacon.LCP, 10), v(beacon.CLS, 0))
	b.View = "home"
	b.ViewID = "home-1"
	b.City = "Athens"
	b.HasGeo = true
	a.Ingest(b)
	rows, next := a.Live(0, 100)
	require.Len(t, rows, 3)
	assert.Equal(t, "document", rows[0].Kind)
	assert.EqualValues(t, 1, rows[0].Revision)
	for _, r := range rows[1:] {
		assert.Equal(t, "vital", r.Kind)
		assert.NotEmpty(t, r.MetricID)
		assert.EqualValues(t, 1, r.Revision)
		assert.Equal(t, b.ExperienceID, r.ExperienceID)
		assert.Equal(t, "home-1", r.ViewID)
	}
	*now = now.Add(time.Second)
	b.Received = *now
	a.Ingest(b)
	rows, _ = a.Live(next, 100)
	assert.Empty(t, rows)
	b.Vitals = b.Vitals[:1]
	b.Vitals[0].Revision = 2
	b.Vitals[0].Value = 6000
	b.PageGroup = "/wrong"
	b.View = "checkout"
	b.ViewID = "checkout-1"
	a.Ingest(b)
	rows, _ = a.Live(next, 100)
	require.Len(t, rows, 1)
	assert.Equal(t, "/", rows[0].Page)
	assert.Equal(t, "checkout", rows[0].View)
	assert.EqualValues(t, 2, rows[0].Revision)
}
func TestLiveVitalOriginDoesNotMixCountriesAndLocations(t *testing.T) {
	for name, tc := range map[string]struct {
		originCountry  string
		currentCountry string
		wantCity       string
		wantLat        float64
		wantLon        float64
		wantHasGeo     bool
	}{
		"country changed":         {originCountry: "GR", currentCountry: "US"},
		"origin country unknown":  {currentCountry: "US"},
		"current country unknown": {originCountry: "GR"},
		"same country": {
			originCountry: "GR", currentCountry: "GR",
			wantCity: "Current City", wantLat: 38, wantLon: 23.7, wantHasGeo: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			a, now := newAgg(time.Minute)
			b := mk(*now, "s", "/entry", v(beacon.LCP, 10))
			b.Country = tc.originCountry
			a.Ingest(b)
			_, cursor := a.Live(0, 100)

			*now = now.Add(time.Second)
			b.Received = *now
			b.Country = tc.currentCountry
			b.City, b.Lat, b.Lon, b.HasGeo = "Current City", 38, 23.7, true
			b.View, b.ViewID = "checkout", "checkout-1"
			b.Vitals[0].Revision = 2
			b.Events = nil
			b.Errors = []beacon.Error{{Message: "current error"}}
			a.Ingest(b)

			rows, _ := a.Live(cursor, 100)
			assert.Equal(t, []LiveRow{
				{
					Seq: cursor + 1, TS: *now, Site: "s",
					ExperienceID: b.ExperienceID, View: "checkout", ViewID: "checkout-1",
					Kind: "vital", Vital: beacon.LCP, MetricID: beacon.LCP, Revision: 2,
					Page: "/entry", Browser: "Chrome", Device: "desktop",
					Country: tc.originCountry, City: tc.wantCity,
					Lat: tc.wantLat, Lon: tc.wantLon, HasGeo: tc.wantHasGeo,
					LCPMS: 10, HasLCP: true,
				},
				{
					Seq: cursor + 2, TS: *now, Site: "s",
					ExperienceID: b.ExperienceID, View: "checkout", ViewID: "checkout-1",
					Kind: "error", Page: "/entry", Browser: "Chrome", Device: "desktop",
					Country: tc.currentCountry, City: "Current City",
					Lat: 38, Lon: 23.7, HasGeo: true, Errors: 1,
				},
			}, rows)
		})
	}
}

func TestLiveRingAndCursorAcrossWraps(t *testing.T) {
	a, now := newAgg(time.Minute)
	for i := range 3 * liveRingCap {
		b := mk(*now, "s", "/")
		b.Events = nil
		b.Errors = []beacon.Error{{Message: itoa(i)}}
		a.Ingest(b)
	}
	rows, next := a.Live(0, 0)
	require.Len(t, rows, liveRingCap)
	assert.EqualValues(t, 3*liveRingCap, next)
	rows, _ = a.Live(next+100, 5)
	require.Len(t, rows, 5)
	assert.EqualValues(t, 2*liveRingCap+1, rows[0].Seq)
	*now = now.Add(time.Minute + time.Second)
	rows, _ = a.Live(0, 100)
	assert.Empty(t, rows)
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
