// SPDX-License-Identifier: GPL-3.0-or-later

package aggregate

import (
	"sort"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
)

type PageInfo struct {
	VnodeGUID                               string
	Site, Page                              string
	PageviewsWindow, Sessions, ErrorsWindow int
	Vitals                                  map[string]VitalStats
	LCPElement, INPElement, CLSElement      string
	FrustrationWindow                       int
	FrustrationsKnown                       bool
}
type elemSample struct {
	vital, sel string
	poor       bool
}

func hasPageElement(vital, element string) bool {
	return element != "" && (vital == beacon.LCP || vital == beacon.INP || vital == beacon.CLS)
}

// Pages returns all retained page groups, independent of the chart top-N.
// An empty vnodeGUID selects all current associations. Its inventory is bounded
// by the canonical retained observations.
func (a *Aggregator) Pages(vnodeGUID string) []PageInfo {
	a.mu.Lock()
	if vnodeGUID != "" && a.site.cfg.VnodeGUID != vnodeGUID {
		a.mu.Unlock()
		return nil
	}
	cfg := a.site.cfg
	r := a.site.readWindow(a.now(), true)
	a.mu.Unlock()
	var out []PageInfo
	if len(r.groups[facetPage]) > 0 {
		out = make([]PageInfo, 0, len(r.groups[facetPage]))
	}
	for page, p := range r.groups[facetPage] {
		out = append(out, PageInfo{
			Site:              cfg.Name,
			VnodeGUID:         cfg.VnodeGUID,
			Page:              page,
			PageviewsWindow:   int(p.documents),
			Sessions:          p.sessions,
			ErrorsWindow:      int(p.errors),
			Vitals:            r.vitals(p),
			LCPElement:        topElement(p.elements, beacon.LCP),
			INPElement:        topElement(p.elements, beacon.INP),
			CLSElement:        topElement(p.elements, beacon.CLS),
			FrustrationWindow: int(p.frustrations),
			FrustrationsKnown: cfg.FrustrationSignals,
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
