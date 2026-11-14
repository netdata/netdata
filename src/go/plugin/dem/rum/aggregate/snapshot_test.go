// SPDX-License-Identifier: GPL-3.0-or-later

package aggregate

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadsPreserveEveryVitalAndEmptyPopulation(t *testing.T) {
	for _, name := range beacon.Vitals {
		t.Run(name, func(t *testing.T) {
			a, now := newAgg(time.Minute, SiteCfg{
				Name:       "s",
				PageGroups: -1,
				Countries:  -1,
			})
			input := mk(*now, "measured", "/measured", v(name, 0))
			input.View = "checkout"
			input.ViewID = "checkout-visit"
			input.Events = append(input.Events, beacon.Event{
				Kind:     beacon.EventView,
				ID:       "view",
				Revision: 1,
			})
			a.Ingest(input)
			activity := mk(*now, "activity", "/activity")
			a.Ingest(activity)
			session := mk(*now, "session", "/session")
			session.Events = nil
			session.Logs = []beacon.Log{{Message: "accepted activity"}}
			a.Ingest(session)

			wantVitals := make(map[string]VitalStats)
			for _, vital := range beacon.Vitals {
				wantVitals[vital] = VitalStats{}
			}
			wantVitals[name] = VitalStats{
				N:    1,
				Good: 1,
			}
			snap := a.Snapshot()
			assert.Equal(t, wantVitals, snap.Vitals)
			assert.EqualValues(t, 2, snap.PageviewsWindow)
			assert.Equal(t, 3, snap.ObservedSessions)
			for _, kind := range facetKinds {
				groups := snap.Breakdowns[kind]
				require.Len(t, groups, 1, kind)
				if kind == KindView {
					assert.Nil(t, groups[0].Vitals)
					assert.EqualValues(t, 1, groups[0].ApplicationViews)
				} else {
					assert.Equal(t, wantVitals, groups[0].Vitals, kind)
					assert.EqualValues(t, 2, groups[0].Pageviews, kind)
				}
				if kind == KindPage || kind == KindCountry || kind == KindView {
					assert.True(t, groups[0].Other, kind)
				}
			}
			pages := a.Pages("")
			require.Len(t, pages, 3)
			assert.Equal(t, "/activity", pages[0].Page)
			assert.Equal(t, "/measured", pages[1].Page)
			assert.Equal(t, "/session", pages[2].Page)
			assert.Equal(t, wantVitals, pages[1].Vitals)
			assert.Zero(t, pages[2].PageviewsWindow)
			assert.Equal(t, 1, pages[2].Sessions)
			for _, page := range []PageInfo{pages[0], pages[2]} {
				for _, vital := range beacon.Vitals {
					assert.Equal(t, VitalStats{}, page.Vitals[vital])
				}
			}
		})
	}
}

func TestVitalPopulationDoesNotInventOtherMeasurements(t *testing.T) {
	a, now := newAgg(5 * time.Minute)
	for i := range 3 {
		b := mk(*now, fmt.Sprint(i), "/", v(beacon.FCP, 100))
		b.Events = nil
		a.Ingest(b)
	}
	s := a.Snapshot()
	assert.Equal(t, 3, s.Vitals[beacon.FCP].N)
	assert.Equal(t, 100., s.Vitals[beacon.FCP].P75)
	assert.Zero(t, s.Vitals[beacon.LCP].N)
	assert.Zero(t, s.PageviewsWindow)
	assert.Zero(t, s.API.N)
	assert.Zero(t, s.Load.N)
	assert.Equal(t, 3, s.ObservedSessions)
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
	}
	pages := a.Pages("")
	require.Len(t, pages, 1101)
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
		assert.Equal(t, 991, groups[10].Duration.N)
	})
}

func TestConcurrentReadsCaptureRevisionAndVnodeTogether(t *testing.T) {
	a, now := newAgg(5*time.Minute, SiteCfg{Name: "s", PageGroups: 20, Countries: 20})
	var workers sync.WaitGroup
	start := make(chan struct{})
	workers.Go(func() {
		<-start
		for i := range 1000 {
			guid, page, value := vnodeA, "/alpha", 100.
			if i/10%2 != 0 {
				guid, page, value = vnodeB, "/beta", 200
			}
			value += float64(i % 10)
			a.SetVnodeGUID(guid)
			b := mk(*now, "session", page, v(beacon.LCP, value))
			b.Vitals[0].Revision = uint64(i + 1)
			a.Ingest(b)
			if i%10 > 0 {
				b.Vitals[0].Revision--
				b.Vitals[0].Value = 9999
				a.Ingest(b)
			}
		}
	})
	for range 4 {
		workers.Go(func() {
			<-start
			for range 500 {
				for _, page := range a.Pages("") {
					want := 100.
					if page.VnodeGUID == vnodeB {
						want = 200
					}
					require.Equal(t, map[string]string{vnodeA: "/alpha", vnodeB: "/beta"}[page.VnodeGUID], page.Page)
					require.Equal(t, 1, page.PageviewsWindow)
					require.Equal(t, 1, page.Sessions)
					require.Equal(t, 1, page.Vitals[beacon.LCP].N)
					require.GreaterOrEqual(t, page.Vitals[beacon.LCP].P75, want)
					require.LessOrEqual(t, page.Vitals[beacon.LCP].P75, want+9)
				}
				s := a.Snapshot()
				require.EqualValues(t, s.Vitals[beacon.LCP].N, s.PageviewsWindow)
				require.Equal(t, s.Vitals[beacon.LCP].N, s.ObservedSessions)
				for _, group := range s.Breakdowns[KindPage] {
					want := 100.
					if group.Value == "/beta" {
						want = 200
					}
					require.GreaterOrEqual(t, group.Vitals[beacon.LCP].P75, want)
					require.LessOrEqual(t, group.Vitals[beacon.LCP].P75, want+9)
					require.Equal(t, s.Vitals[beacon.LCP], group.Vitals[beacon.LCP])
				}
			}
		})
	}
	close(start)
	workers.Wait()
	pages := a.Pages("")
	require.Len(t, pages, 1)
	require.Equal(t, vnodeB, pages[0].VnodeGUID)
	require.Equal(t, 209., pages[0].Vitals[beacon.LCP].P75)
	require.Equal(t, 209., a.Snapshot().Vitals[beacon.LCP].P75)
}

// With fixed facet membership, exact read-buffer allocation counts depend on
// populated distributions, not retained sample count or attribution selectors.
func TestReadAllocationCountDoesNotGrowWithSamples(t *testing.T) {
	allocs := func(samples int) map[string]float64 {
		a, now := newAgg(time.Minute)
		for i := range samples {
			input := mk(*now, fmt.Sprint(i), "/page")
			for _, name := range beacon.Vitals {
				metric := v(name, 0)
				metric.Element = "#element"
				input.Vitals = append(input.Vitals, metric)
			}
			a.Ingest(input)
		}
		return map[string]float64{
			"snapshot": testing.AllocsPerRun(3, func() { a.Snapshot() }),
			"pages":    testing.AllocsPerRun(3, func() { a.Pages("") }),
		}
	}
	small, large := allocs(100), allocs(10000)
	for kind, count := range small {
		assert.LessOrEqual(
			t,
			large[kind],
			count+10,
			"%s: 100 samples: %.0f allocations; 10000 samples: %.0f",
			kind,
			count,
			large[kind],
		)
	}
}
