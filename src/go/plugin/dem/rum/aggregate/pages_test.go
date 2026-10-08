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

func TestPagesIncludesLowTrafficOutsideChartTopN(t *testing.T) {
	a, now := newAgg(time.Minute, SiteCfg{
		Name:       "s",
		PageGroups: 1,
		Countries:  1,
	})
	for i := range 4 {
		a.Ingest(mk(*now, fmt.Sprint(i), "/popular", v(beacon.LCP, 100)))
	}
	a.Ingest(mk(*now, "slow", "/rare", v(beacon.LCP, 6000)))
	require.Len(t, a.Pages(), 2)
	groups := a.Snapshot().Breakdowns[KindPage]
	require.Len(t, groups, 2)
	assert.Equal(t, "/popular", groups[0].Value)
	assert.True(t, groups[1].Other)
	assert.EqualValues(t, 4, groups[0].Pageviews)
	assert.EqualValues(t, 1, groups[1].Pageviews)
	assert.Equal(t, 6000., groups[1].Vitals[beacon.LCP].P75)
}
func TestFacetPartitionTracksCurrentPopulationAndTypedOther(t *testing.T) {
	a, now := newAgg(time.Minute, SiteCfg{
		Name:       "s",
		PageGroups: 1,
		Countries:  1,
	})
	for i := range 3 {
		b := mk(*now, fmt.Sprint(i), "/a", v(beacon.LCP, 100))
		b.AppVersion = "other"
		a.Ingest(b)
	}
	b := mk(*now, "rare", "/b", v(beacon.LCP, 6000))
	b.AppVersion = "release"
	a.Ingest(b)
	snap := a.Snapshot()
	for _, kind := range []string{KindPage, KindVersion, KindCountry} {
		var views uint64
		var n int
		for _, g := range snap.Breakdowns[kind] {
			views += g.Pageviews
			n += g.Vitals[beacon.LCP].N
		}
		assert.Equal(t, snap.PageviewsWindow, views)
		assert.Equal(t, snap.Vitals[beacon.LCP].N, n)
	}
	var realOther bool
	for _, g := range snap.Breakdowns[KindVersion] {
		if g.Value == Other && !g.Other {
			realOther = true
		}
	}
	assert.True(t, realOther)
	*now = now.Add(time.Minute + time.Second)
	a.Ingest(mk(*now, "new", "/b", v(beacon.LCP, 20)))
	groups := a.Snapshot().Breakdowns[KindPage]
	require.Len(t, groups, 1)
	assert.Equal(t, "/b", groups[0].Value)
	assert.EqualValues(t, 1, groups[0].Pageviews)
}
func TestSessionMeasurementIndependentOfDetailLRU(t *testing.T) {
	a, now := newAgg(time.Minute)
	for i := range maxTrackedSessions + 10 {
		a.Ingest(mk(*now, fmt.Sprint(i), "/"))
	}
	assert.Equal(t, maxTrackedSessions+10, a.Snapshot().ObservedSessions)
	assert.Equal(t, maxTrackedSessions+10, a.Pages()[0].Sessions)
	*now = now.Add(time.Minute + time.Second)
	assert.Zero(t, a.Activity().ObservedSessions)
	assert.Empty(t, a.Pages())
}
func TestObservedSessionCapacityIsExplicit(t *testing.T) {
	a, now := newAgg(time.Minute)
	for i := range maxWindowObservations + 1 {
		b := mk(*now, fmt.Sprint(i), "/")
		b.Events = nil
		b.Logs = []beacon.Log{{Message: "accepted activity"}}
		a.Ingest(b)
	}
	s := a.Snapshot()
	assert.Equal(t, maxWindowObservations, s.ObservedSessions)
	assert.Positive(t, s.SessionsLost)
	assert.Positive(t, a.Pages()[0].SessionsLost)
	*now = now.Add(time.Minute + time.Second)
	assert.Zero(t, a.Snapshot().SessionsLost)
}
func TestPageInventoryIncludesEveryRetainedGroup(t *testing.T) {
	a, now := newAgg(time.Minute)
	for i := range 1101 {
		a.Ingest(mk(*now, "s", fmt.Sprintf("/page_%d", i)))
	}
	pages := a.Pages()
	assert.Len(t, pages, 1101)
	assert.Zero(t, pages[0].Lost)
}
func TestSelectorsUseReplacedVitalPopulation(t *testing.T) {
	a, now := newAgg(time.Minute)
	b := mk(*now, "s", "/", v(beacon.LCP, 6000))
	b.Vitals[0].Element = "#old"
	a.Ingest(b)
	b.Vitals[0].Revision = 2
	b.Vitals[0].Value = 100
	b.Vitals[0].Element = "#new"
	a.Ingest(b)
	pages := a.Pages()
	require.Len(t, pages, 1)
	assert.Equal(t, "#new", pages[0].LCPElement)
	assert.Equal(t, 1, pages[0].Vitals[beacon.LCP].N)
}

func TestDuplicateReportDoesNotRefreshObservedSessions(t *testing.T) {
	a, now := newAgg(time.Minute)
	b := mk(*now, "browser", "/entry", v(beacon.LCP, 100))
	a.Ingest(b)
	assert.Equal(t, 1, a.Snapshot().ObservedSessions)
	*now = now.Add(time.Minute + time.Second)
	assert.Zero(t, a.Snapshot().ObservedSessions)
	assert.Empty(t, a.Pages())
	b.Received = *now
	result := a.Ingest(b)
	assert.Empty(t, result.Observation.Vitals)
	assert.Empty(t, result.Observation.Events)
	assert.Zero(t, a.Snapshot().ObservedSessions)
	assert.Empty(t, a.Pages())
	b.Vitals[0].Revision = 2
	a.Ingest(b)
	assert.Equal(t, 1, a.Snapshot().ObservedSessions)
	pages := a.Pages()
	require.Len(t, pages, 1)
	assert.Equal(t, 1, pages[0].Sessions)
}
