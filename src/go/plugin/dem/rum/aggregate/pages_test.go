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
	require.Len(t, a.Pages(""), 2)
	groups := a.Snapshot().Breakdowns[KindPage]
	require.Len(t, groups, 2)
	assert.Equal(t, "/popular", groups[0].Value)
	assert.True(t, groups[1].Other)
	assert.EqualValues(t, 4, groups[0].Pageviews)
	assert.EqualValues(t, 1, groups[1].Pageviews)
	assert.Equal(t, 6000., groups[1].Vitals[beacon.LCP].P75)
}

func TestPageInventoryIncludesEveryRetainedGroup(t *testing.T) {
	a, now := newAgg(time.Minute)
	for i := range 1101 {
		a.Ingest(mk(*now, "s", fmt.Sprintf("/page_%d", i)))
	}
	pages := a.Pages("")
	assert.Len(t, pages, 1101)
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
	pages := a.Pages("")
	require.Len(t, pages, 1)
	assert.Equal(t, "#new", pages[0].LCPElement)
	assert.Equal(t, 1, pages[0].Vitals[beacon.LCP].N)
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
