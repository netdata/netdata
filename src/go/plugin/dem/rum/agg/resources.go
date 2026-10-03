// SPDX-License-Identifier: GPL-3.0-or-later

// Resource timing state: first/third-party counters
// and a bounded top-10-by-count host ranking, feeding rum.resources /
// rum.resource_host_p75 / rum.resource_host_count. Declared only once a
// faro.performance.resource event has actually been seen for the site —
// confirmed present on real pages (testdata/faro-samples/resource-event.json,
// captured from a live www.netdata.cloud page), so this is not
// speculative: a small demo page simply never triggers third-party
// fetches within the beacon window.
package agg

import (
	"net"
	"sort"
	"strings"
	"time"

	"golang.org/x/net/publicsuffix"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
)

// resourceHostTopN is the fixed top-N for the resource-host charts ("top-10 hosts by count"); unlike page/country there is no
// per-site config and no "other" fold — hosts beyond the top-10 (or the
// maxTrackedGroups cap) simply are not charted, only counted toward the
// site-level first_party/third_party totals.
const resourceHostTopN = 10

type resHostGroup struct {
	count uint64
	dur   series
}

// recordResource updates the first/third-party totals and the per-host
// ranking state for one resource-timing sample. pageHost is the page's
// own host (beacon.Beacon.PageHost); an empty pageHost or resource host
// counts as third-party (conservative: cannot prove same-site).
func (st *siteState) recordResource(pageHost string, r beacon.Resource, now time.Time) {
	st.resourcesSeen = true
	if isFirstParty(pageHost, r.Host) {
		st.firstPartyResources++
	} else {
		st.thirdPartyResources++
	}
	if isAPICall(pageHost, r) {
		st.addSample(seriesKey{apiName, "", ""}, r.DurationMS, now)
	}
	g, ok := st.resHosts[r.Host]
	if !ok {
		if st.resHostN >= maxTrackedGroups {
			return // bounded, like every other breakdown cardinality
		}
		g = &resHostGroup{}
		st.resHosts[r.Host] = g
		st.resHostN++
	}
	g.count++
	if r.DurationMS > 0 {
		// Same per-series cap as addSample: a busy shared CDN host can see
		// far more samples per window than the 10s eviction tick removes.
		if len(g.dur.vals) >= maxSamplesPerSeries {
			g.dur.ts = g.dur.ts[1:]
			g.dur.vals = g.dur.vals[1:]
			st.counters[CounterSamplesDropped]++
		}
		g.dur.ts = append(g.dur.ts, now)
		g.dur.vals = append(g.dur.vals, r.DurationMS)
	}
}

// apiName is the rum.api series key, distinct from any beacon.Vitals name.
const apiName = "__api"

// isAPICall reports whether a resource-timing entry is one of the site's
// own API requests for rum.api: a first-party fetch/XHR with a duration,
// not the snippet's beacon. Third-party calls (forms, analytics) stay
// visible per host in rum.resource_host_p75 instead.
func isAPICall(pageHost string, r beacon.Resource) bool {
	if r.Self || r.DurationMS <= 0 {
		return false
	}
	if r.Initiator != "fetch" && r.Initiator != "xmlhttprequest" {
		return false
	}
	return isFirstParty(pageHost, r.Host)
}

// isFirstParty reports whether resHost is the same registrable domain as
// pageHost. publicsuffix is already a transitive
// dependency (golang.org/x/net, via grpc) so no new module is added.
// Hosts publicsuffix cannot classify (bare IPs, "localhost") fall back to
// an exact host match, which is the strictest possible definition of
// "same site" for those inputs. Faro's httpHost carries a non-default
// port ("www.example.com:8443") while pageHost never does, so ports are
// ignored: the registrable domain decides, as for cookies.
func isFirstParty(pageHost, resHost string) bool {
	if pageHost == "" || resHost == "" || resHost == unknownValue {
		return false
	}
	pageHost, resHost = strings.ToLower(stripPort(pageHost)), strings.ToLower(stripPort(resHost))
	if pageHost == resHost {
		return true
	}
	pe, err1 := publicsuffix.EffectiveTLDPlusOne(pageHost)
	re, err2 := publicsuffix.EffectiveTLDPlusOne(resHost)
	if err1 != nil || err2 != nil {
		return false
	}
	return pe == re
}

// stripPort drops a ":port" suffix, leaving bare and bracketed IPv6 hosts
// ("::1", "[::1]") as they are unless a port follows the bracket.
func stripPort(host string) string {
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	return host
}

// ResourceHostGroup is one rum.resource_host_p75/count chart instance.
type ResourceHostGroup struct {
	Host        string
	Count       uint64
	HasDuration bool
	P75Duration float64 // ms; 0 when no duration sample was ever recorded
}

// rankResourceHosts ranks tracked hosts by all-time count, top-10. There
// is no "other" fold; site-level resource counters still include every host.
func (st *siteState) rankResourceHosts(cutoff time.Time) []ResourceHostGroup {
	type cand struct {
		host string
		g    *resHostGroup
	}
	cands := make([]cand, 0, len(st.resHosts))
	for h, g := range st.resHosts {
		g.dur.evict(cutoff)
		cands = append(cands, cand{h, g})
	}
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].g.count != cands[j].g.count {
			return cands[i].g.count > cands[j].g.count
		}
		return cands[i].host < cands[j].host
	})
	n := resourceHostTopN
	if n > len(cands) {
		n = len(cands)
	}
	out := make([]ResourceHostGroup, 0, n)
	for _, c := range cands[:n] {
		out = append(
			out,
			ResourceHostGroup{
				Host:        c.host,
				Count:       c.g.count,
				HasDuration: len(c.g.dur.vals) > 0,
				P75Duration: percentile(c.g.dur.vals, 0.75),
			},
		)
	}
	return out
}
