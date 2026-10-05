// SPDX-License-Identifier: GPL-3.0-or-later

// rum-pages FUNCTION read: the existing top-N page
// groups, extended with LCP/INP/CLS p75, distinct sessions
// and a windowed error count. Kept in its own file since it is a read
// path over aggregate.go's group state, not new state of its own.
package aggregate

import (
	"sort"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
)

// PageInfo is one rum-pages row.
type PageInfo struct {
	Site                   string
	Page                   string
	PageviewsWindow        int
	Sessions               int
	LCPP75MS               float64
	INPP75MS               float64
	CLSP75                 float64
	HasLCP, HasINP, HasCLS bool
	ErrorsWindow           int
	// Elements behind this page's vitals: the selector most often
	// attributed to a poor value in the window, else the most common one.
	LCPElement, INPElement, CLSElement string
	// FrustrationWindow counts rage, dead and error clicks.
	FrustrationWindow int
}

// Pages returns this site's currently top-N-ranked page groups — the same set
// the rum.page_* charts show, not the full (larger, unranked) internal
// page-group cardinality. Read-only: counts
// windowed samples without evicting or touching rank/top-set state,
// which Snapshot() already maintains on the chart-emission cadence.
func (a *Aggregator) Pages() []PageInfo {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := a.now()
	cutoff := now.Add(-a.window)
	var out []PageInfo
	st := &a.site
	values := make([]string, 0, len(st.top[KindPage]))
	for v := range st.top[KindPage] {
		values = append(values, v)
	}
	sort.Strings(values)
	for _, v := range values {
		g, ok := st.groups[groupKey{KindPage, v}]
		if !ok {
			continue
		}
		p75 := st.p75s(KindPage, v)
		_, hasLCP := p75[beacon.LCP]
		_, hasINP := p75[beacon.INP]
		_, hasCLS := p75[beacon.CLS]
		out = append(out, PageInfo{
			Site:              st.cfg.Name,
			Page:              v,
			PageviewsWindow:   countTimes(g.pvTimes, cutoff),
			Sessions:          len(g.sessions),
			LCPP75MS:          p75[beacon.LCP],
			INPP75MS:          p75[beacon.INP],
			CLSP75:            p75[beacon.CLS],
			HasLCP:            hasLCP,
			HasINP:            hasINP,
			HasCLS:            hasCLS,
			ErrorsWindow:      countTsCounts(g.errWindow, cutoff),
			LCPElement:        topElement(g.elements, beacon.LCP, cutoff),
			INPElement:        topElement(g.elements, beacon.INP, cutoff),
			CLSElement:        topElement(g.elements, beacon.CLS, cutoff),
			FrustrationWindow: countTsCounts(g.frWindow, cutoff),
		})
	}
	return out
}

// topElement picks the selector most often attributed to a poor value of
// vital since cutoff, falling back to the most common selector overall.
func topElement(samples []elemSample, vital string, cutoff time.Time) string {
	poor, all := map[string]int{}, map[string]int{}
	for _, e := range samples {
		if e.vital != vital || e.ts.Before(cutoff) {
			continue
		}
		all[e.sel]++
		if e.poor {
			poor[e.sel]++
		}
	}
	if len(poor) > 0 {
		return mostCommon(poor)
	}
	return mostCommon(all)
}

func mostCommon(counts map[string]int) string {
	best, n := "", 0
	for sel, c := range counts {
		if c > n || (c == n && sel < best) {
			best, n = sel, c
		}
	}
	return best
}
