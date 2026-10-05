// SPDX-License-Identifier: GPL-3.0-or-later
package aggregate

import (
	"sort"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
)

type PageInfo struct {
	Site, Page                              string
	PageviewsWindow, Sessions, ErrorsWindow int
	Vitals                                  map[string]VitalStats
	Lost, SessionsLost                      uint64
	LCPElement, INPElement, CLSElement      string
	FrustrationWindow                       int
	FrustrationsKnown                       bool
}
type elemSample struct {
	vital, sel string
	poor       bool
}

// Pages returns all retained page groups, independent of the chart top-N.
// Its inventory is bounded by the canonical retained observations.
func (a *Aggregator) Pages() []PageInfo {
	a.mu.Lock()
	defer a.mu.Unlock()
	r := a.site.readWindow(a.now(), true)
	var out []PageInfo
	for key, p := range r.groups {
		if key.kind != KindPage {
			continue
		}
		out = append(out, PageInfo{
			Site:              a.site.cfg.Name,
			Page:              key.value,
			PageviewsWindow:   int(p.documents),
			Sessions:          p.sessions,
			ErrorsWindow:      int(p.errors),
			Vitals:            r.vitals(p),
			Lost:              r.windowLost,
			SessionsLost:      r.pageSessionsLost,
			LCPElement:        topElement(p.elements, beacon.LCP),
			INPElement:        topElement(p.elements, beacon.INP),
			CLSElement:        topElement(p.elements, beacon.CLS),
			FrustrationWindow: int(p.frustrations),
			FrustrationsKnown: a.site.cfg.FrustrationSignals,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Page < out[j].Page })
	return out
}
func topElement(samples []elemSample, vital string) string {
	poor, all := map[string]int{}, map[string]int{}
	for _, v := range samples {
		if v.vital != vital {
			continue
		}
		all[v.sel]++
		if v.poor {
			poor[v.sel]++
		}
	}
	if len(poor) > 0 {
		return mostCommon(poor)
	}
	return mostCommon(all)
}
func mostCommon(counts map[string]int) string {
	best, n := "", 0
	for sel, count := range counts {
		if count > n || count == n && sel < best {
			best, n = sel, count
		}
	}
	return best
}
