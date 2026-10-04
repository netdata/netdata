// SPDX-License-Identifier: GPL-3.0-or-later

package aggregate

import (
	"sort"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
)

// VitalStats are the emitted statistics of one vital over the window.
type VitalStats struct {
	N               int
	P50, P75, P95   float64
	Good, NeedsImpr int // samples in the window per CWV rating bucket
	Poor            int
}

// Group is one breakdown instance ready for emission. P75 holds only
// vitals with samples in the window (gaps otherwise).
type Group struct {
	Value     string
	P75       map[string]float64
	Pageviews uint64
	JSErrors  uint64
}

// Snapshot is the emission-ready state of one site.
type Snapshot struct {
	Name           string
	DisplayName    string
	Vitals         map[string]VitalStats // only vitals with samples
	Counters       map[string]uint64
	ActiveSessions int
	Breakdowns     map[string][]Group // kind → ranked instances, other last when present

	// Load/DCL are navigation timing; N==0 means no sample
	// in the window. Good/NeedsImpr/Poor are always 0 — no CWV rating is
	// computed for these two.
	Load, DCL VitalStats

	// API is first-party fetch/XHR duration from resource timing (rum.api);
	// N==0 means no sample in the window.
	API VitalStats

	// ErrorGroups is the rum.error_groups chart's ranked instances
	// (top-10 + other).
	ErrorGroups []ErrorChartGroup

	// Resources: ResourcesSeen gates rum.resources/
	// rum.resource_host_* declaration.
	ResourcesSeen                            bool
	FirstPartyResources, ThirdPartyResources uint64
	ResourceHosts                            []ResourceHostGroup

	// Window totals: page views and JS errors counted over the same
	// sliding window as the percentiles. Absolute counts, so a ratio over
	// sites/nodes stays traffic-weighted.
	PageviewsWindow, JSErrorsWindow uint64
}

// Snapshot evicts expired samples/sessions/groups and returns this site's
// emission state. It also fixes the top sets used to route counters until
// the next call.
func (a *Aggregator) Snapshot() Snapshot {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.site.snapshot(a.now(), a.window)
}

func (st *siteState) snapshot(now time.Time, window time.Duration) Snapshot {
	cutoff := now.Add(-window)
	expire := now.Add(-2 * window)

	// rum-sites FUNCTION rings: evicted here too so they stay
	// bounded even when nothing polls Activity between chart ticks.
	st.evictActivity(now, window)

	// Evict windowed samples; drop empty series.
	for k, s := range st.series {
		s.evict(cutoff)
		if len(s.vals) == 0 {
			delete(st.series, k)
		}
	}
	// Expire idle groups and de-dup keys.
	for k, g := range st.groups {
		if g.lastSeen.Before(expire) {
			delete(st.groups, k)
			st.groupN[k.kind]--
			for sk := range st.series {
				if sk.kind == k.kind && sk.value == k.value {
					delete(st.series, sk)
				}
			}
			delete(st.top[k.kind], k.value)
			continue
		}
		g.pvTimes = evictTimes(g.pvTimes, cutoff)
		if k.kind == KindPage {
			g.errWindow = evictTsCounts(g.errWindow, cutoff)
			g.frWindow = evictTsCounts(g.frWindow, cutoff)
			n := 0
			for i, e := range g.elements {
				if !e.ts.Before(cutoff) {
					if n != i {
						g.elements[n] = e
					}
					n++
				}
			}
			clear(g.elements[n:])
			g.elements = g.elements[:n]
		}
	}
	for k, t := range st.dedup {
		if t.Before(cutoff) {
			delete(st.dedup, k)
		}
	}
	st.evictSessions(now)

	snap := Snapshot{
		Name:           st.cfg.Name,
		DisplayName:    st.cfg.DisplayName,
		Vitals:         map[string]VitalStats{},
		Counters:       make(map[string]uint64, len(st.counters)),
		ActiveSessions: st.sess.Len(),
		Breakdowns:     map[string][]Group{},
	}
	snap.PageviewsWindow = uint64(len(st.pvWindow))
	for _, tc := range st.jsErrWindow {
		snap.JSErrorsWindow += tc.n
	}
	for k, v := range st.counters {
		snap.Counters[k] = v
	}
	for _, vital := range beacon.Vitals {
		if s, ok := st.series[seriesKey{vital, "", ""}]; ok {
			snap.Vitals[vital] = stats(vital, s.vals)
		}
	}
	if s, ok := st.series[seriesKey{navLoadName, "", ""}]; ok {
		snap.Load = pctStats(s.vals)
	}
	if s, ok := st.series[seriesKey{navDCLName, "", ""}]; ok {
		snap.DCL = pctStats(s.vals)
	}
	if s, ok := st.series[seriesKey{apiName, "", ""}]; ok {
		snap.API = pctStats(s.vals)
	}
	snap.ErrorGroups = st.rankErrorGroups()

	snap.ResourcesSeen = st.resourcesSeen
	snap.FirstPartyResources = st.firstPartyResources
	snap.ThirdPartyResources = st.thirdPartyResources
	if st.resourcesSeen {
		snap.ResourceHosts = st.rankResourceHosts(cutoff)
	}

	for _, kind := range Kinds {
		snap.Breakdowns[kind] = st.rank(kind)
	}
	return snap
}

func (st *siteState) topN(kind string) int {
	switch kind {
	case KindPage:
		return st.cfg.PageGroups
	case KindCountry:
		return st.cfg.Countries
	case KindBrowser:
		return browserTopN
	case KindVersion:
		return versionTopN
	default:
		return 4 // device: mobile|tablet|desktop|bot (bot only with bots: include)
	}
}

// rank picks the top-N groups by windowed page views (ties by value),
// folds the rest into other and records the new top set for routing.
func (st *siteState) rank(kind string) []Group {
	type cand struct {
		value string
		views int
	}
	var cands, forced []cand
	for k, g := range st.groups {
		if k.kind != kind {
			continue
		}
		c := cand{k.value, len(g.pvTimes)}
		// A real value spelled like the fold sentinel can never rank: it would
		// share the other instance's chart id.
		if k.value == Other {
			forced = append(forced, c)
			continue
		}
		cands = append(cands, c)
	}
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].views != cands[j].views {
			return cands[i].views > cands[j].views
		}
		return cands[i].value < cands[j].value
	})
	n := st.topN(kind)
	if n > len(cands) {
		n = len(cands)
	}
	newTop := make(map[string]bool, n)
	out := make([]Group, 0, n+1)
	for _, c := range cands[:n] {
		newTop[c.value] = true
		g := st.groups[groupKey{kind, c.value}]
		out = append(out, Group{
			Value:     c.value,
			P75:       st.p75s(kind, c.value),
			Pageviews: g.pageviews,
			JSErrors:  g.jsErrors,
		})
	}
	// Future counts for values outside the new top set route to other.
	st.top[kind] = newTop

	// other: union of non-top samples for p75; monotonic fold counters.
	rest := append(append([]cand(nil), cands[n:]...), forced...)
	og, hasOther := st.other[kind]
	if len(rest) > 0 || hasOther {
		p75 := map[string]float64{}
		for _, vital := range beacon.Vitals {
			var vals []float64
			for _, c := range rest {
				if s, ok := st.series[seriesKey{vital, kind, c.value}]; ok {
					vals = append(vals, s.vals...)
				}
			}
			if len(vals) > 0 {
				p75[vital] = percentile(vals, 0.75)
			}
		}
		g := Group{
			Value: Other,
			P75:   p75,
		}
		if hasOther {
			g.Pageviews, g.JSErrors = og.pageviews, og.jsErrors
		}
		out = append(out, g)
	}
	return out
}

func (st *siteState) p75s(kind, value string) map[string]float64 {
	out := map[string]float64{}
	for _, vital := range beacon.Vitals {
		if s, ok := st.series[seriesKey{vital, kind, value}]; ok && len(s.vals) > 0 {
			out[vital] = percentile(s.vals, 0.75)
		}
	}
	return out
}
