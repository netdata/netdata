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
			for _, kind := range Kinds {
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

func TestPageSelectorsPreservePoorAndLexicalPrecedence(t *testing.T) {
	a, now := newAgg(time.Minute)
	for i := range 5 {
		input := mk(*now, fmt.Sprint(i), "/page")
		for _, name := range beacon.Vitals {
			metric := v(name, 0)
			metric.Element = "#common"
			switch name {
			case beacon.LCP, beacon.CLS:
				if i < 2 {
					metric.Value = 5000
					metric.Element = []string{"#z", "#a"}[i]
				}
			case beacon.INP:
				metric.Value = 500 // The exact poor boundary is not poor.
				if i == 0 {
					metric.Value = 501
					metric.Element = "#poor"
				}
			}
			if i == 4 {
				metric.Element = ""
			}
			input.Vitals = append(input.Vitals, metric)
		}
		a.Ingest(input)
	}
	pages := a.Pages("")
	require.Len(t, pages, 1)
	assert.Equal(t, "#a", pages[0].LCPElement)
	assert.Equal(t, "#poor", pages[0].INPElement)
	assert.Equal(t, "#a", pages[0].CLSElement)
	assert.Equal(t, a.Snapshot().Vitals, pages[0].Vitals)
	for _, name := range beacon.Vitals {
		assert.Equal(t, 5, pages[0].Vitals[name].N)
	}
}
