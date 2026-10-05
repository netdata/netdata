// SPDX-License-Identifier: GPL-3.0-or-later

package aggregate

import (
	"container/list"
	"sort"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
)

// Lost is conservative retained capacity-loss evidence, not a denominator.
// Percentiles are unavailable while Lost>0; N and ratings describe retained samples.
type VitalStats struct {
	N                     int
	P50, P75, P95         float64
	Good, NeedsImpr, Poor int
	Lost                  uint64
}

// Group contains current-window gauges. Other identifies the synthetic fold;
// a literal value "other" remains a separate real group.
type Group struct {
	Value                                       string
	Other                                       bool
	Vitals                                      map[string]VitalStats
	Pageviews, JSErrors, ApplicationViews, Lost uint64
}
type Snapshot struct {
	Name, DisplayName                                          string
	Vitals                                                     map[string]VitalStats
	Counters                                                   map[string]uint64
	ObservedSessions                                           int
	SessionsLost, WindowLost                                   uint64
	Breakdowns                                                 map[string][]Group
	Load, DCL, API                                             VitalStats
	ErrorGroups                                                []ErrorChartGroup
	ResourcesSeen                                              bool
	FirstPartyResources, ThirdPartyResources, UnknownResources uint64
	ResourceHosts                                              []ResourceHostGroup
	PageviewsWindow, JSErrorsWindow, ApplicationViewsWindow    uint64
}
type groupKey struct {
	kind, value string
	other       bool
}
type population struct {
	values                                 map[string][]float64
	documents, views, errors, frustrations uint64
	sessions                               int
	elements                               []elemSample
}
type windowRead struct {
	global                                                     population
	groups                                                     map[groupKey]*population
	groupCounts                                                map[string]int
	resourceGroups                                             map[hostKey]*resourcePopulation
	errorGroups                                                map[errorKey]*errorPopulation
	sessions                                                   int
	windowLost, sessionsLost, pageSessionsLost, pageGroupsLost uint64
	vitalLost                                                  map[string]uint64
}

func newPopulation() *population {
	return &population{
		values: map[string][]float64{},
	}
}
func (r *windowRead) group(kind, value string) *population {
	if value == "" {
		value = unknownValue
	}
	key := groupKey{
		kind:  kind,
		value: value,
	}
	if g := r.groups[key]; g != nil {
		return g
	}
	if r.groupCounts[kind] >= maxTrackedGroups {
		key = groupKey{
			kind:  kind,
			value: Other,
			other: true,
		}
		if kind == KindPage {
			r.pageGroupsLost++
		}
	} else {
		r.groupCounts[kind]++
	}
	if g := r.groups[key]; g != nil {
		return g
	}
	g := newPopulation()
	r.groups[key] = g
	return g
}
func attributePairs(v attributes) []groupKey {
	return []groupKey{
		{kind: KindBrowser, value: v.browser},
		{kind: KindDevice, value: v.device},
		{kind: KindCountry, value: v.country},
		{kind: KindPage, value: v.page},
		{kind: KindVersion, value: v.version},
	}
}
func (st *siteState) readWindow(now time.Time) *windowRead {
	cutoff := now.Add(-st.window)
	st.expire(now)
	r := &windowRead{
		global:         *newPopulation(),
		groups:         map[groupKey]*population{},
		groupCounts:    map[string]int{},
		resourceGroups: map[hostKey]*resourcePopulation{},
		errorGroups:    map[errorKey]*errorPopulation{},
		vitalLost:      map[string]uint64{},
	}
	r.windowLost = st.activityLoss.current(now) + st.identityLoss.current(now)
	r.sessionsLost = st.sessionLoss.current(now)
	r.pageSessionsLost = st.pageSessionLoss.current(now)
	for name, loss := range st.vitalLoss {
		r.vitalLost[name] = loss.current(now)
	}
	// Identity loss can admit an old report again, so all derived measurements
	// remain visibly incomplete for this bounded replay-loss interval.
	identityLost := st.identityLoss.current(now)
	r.sessionsLost += identityLost
	r.pageSessionsLost += identityLost
	for _, name := range append(append([]string{}, beacon.Vitals...), navLoadName, navDCLName) {
		r.vitalLost[name] += identityLost
	}
	for el := st.observationOrder.Front(); el != nil; el = el.Next() {
		v := el.Value.(*vitalObservation)
		if v.received.Before(cutoff) {
			continue
		}
		r.global.values[v.key.name] = append(r.global.values[v.key.name], v.value)
		if _, ok := thresholds[v.key.name]; !ok {
			continue
		}
		for _, kv := range attributePairs(v.attrs) {
			g := r.group(kv.kind, kv.value)
			g.values[v.key.name] = append(g.values[v.key.name], v.value)
			if kv.kind == KindPage && v.element != "" {
				g.elements = append(g.elements, elemSample{
					vital: v.key.name,
					sel:   v.element,
					poor:  v.value > thresholds[v.key.name][1],
				})
			}
		}
	}
	for el := st.activity.Front(); el != nil; el = el.Next() {
		v := el.Value.(*activityObservation)
		if v.received.Before(cutoff) {
			continue
		}
		if v.resource != nil {
			r.addResource(v)
			continue
		}
		r.global.documents += v.documents
		r.global.views += v.views
		r.global.errors += v.errors
		r.global.frustrations += v.frustrations
		for _, kv := range attributePairs(v.attrs) {
			g := r.group(kv.kind, kv.value)
			g.documents += v.documents
			g.views += v.views
			g.errors += v.errors
			g.frustrations += v.frustrations
		}
		if v.attrs.view != "" || v.views > 0 {
			g := r.group(KindView, v.attrs.view)
			g.views += v.views
			g.errors += v.errors
		}
		if v.errors > 0 {
			r.addError(v)
		}
	}
	for el := st.sessionOrder.Front(); el != nil; el = el.Next() {
		if !el.Value.(*sessionObservation).received.Before(cutoff) {
			r.sessions++
		}
	}
	for el := st.pageSessionOrder.Front(); el != nil; el = el.Next() {
		v := el.Value.(*sessionObservation)
		if !v.received.Before(cutoff) {
			r.group(KindPage, v.page).sessions++
		}
	}
	return r
}

// Read-time expiry is linear in retained state. Hot ingestion never scans it.
func (st *siteState) expire(now time.Time) {
	cutoff := now.Add(-st.window)
	for el := st.observationOrder.Front(); el != nil; {
		next := el.Next()
		v := el.Value.(*vitalObservation)
		if v.received.Before(cutoff) {
			delete(st.observations, v.key)
			st.observationOrder.Remove(el)
		}
		el = next
	}
	for el := st.identityOrder.Front(); el != nil; {
		next := el.Next()
		v := el.Value.(*identity)
		if v.received.Before(now.Add(-identityRetention)) {
			delete(st.identities, v.key)
			st.identityOrder.Remove(el)
		}
		el = next
	}
	for el := st.activity.Front(); el != nil; {
		next := el.Next()
		if el.Value.(*activityObservation).received.Before(cutoff) {
			st.activity.Remove(el)
		}
		el = next
	}
	expireObservedSessions(st.sessions, st.sessionOrder, cutoff)
	expireObservedSessions(st.pageSessions, st.pageSessionOrder, cutoff)
	st.evictActivity(now)
	st.evictSessions(now)
}
func expireObservedSessions(index map[string]*list.Element, order *list.List, cutoff time.Time) {
	for el := order.Front(); el != nil; {
		next := el.Next()
		v := el.Value.(*sessionObservation)
		if v.received.Before(cutoff) {
			delete(index, v.key)
			order.Remove(el)
		}
		el = next
	}
}
func (a *Aggregator) Snapshot() Snapshot {
	a.mu.Lock()
	defer a.mu.Unlock()
	st := &a.site
	r := st.readWindow(a.now())
	snap := Snapshot{
		Name:                   st.cfg.Name,
		DisplayName:            st.cfg.DisplayName,
		Vitals:                 r.vitals(&r.global),
		Counters:               map[string]uint64{},
		ObservedSessions:       r.sessions,
		SessionsLost:           r.sessionsLost,
		WindowLost:             r.windowLost,
		Breakdowns:             map[string][]Group{},
		Load:                   measurementStats(navLoadName, r.global.values[navLoadName], r.vitalLost[navLoadName]),
		DCL:                    measurementStats(navDCLName, r.global.values[navDCLName], r.vitalLost[navDCLName]),
		API:                    measurementStats(apiName, r.global.values[apiName], r.windowLost),
		ResourcesSeen:          st.resourcesSeen,
		FirstPartyResources:    st.firstPartyResources,
		ThirdPartyResources:    st.thirdPartyResources,
		UnknownResources:       st.unknownResources,
		PageviewsWindow:        r.global.documents,
		JSErrorsWindow:         r.global.errors,
		ApplicationViewsWindow: r.global.views,
	}
	for k, v := range st.counters {
		snap.Counters[k] = v
	}
	for _, kind := range Kinds {
		snap.Breakdowns[kind] = r.rank(kind, st.topN(kind))
	}
	snap.ResourceHosts = r.rankResourceHosts()
	snap.ErrorGroups = r.rankErrorGroups()
	return snap
}
func measurementStats(name string, values []float64, lost uint64) VitalStats {
	var out VitalStats
	if _, ok := thresholds[name]; ok {
		out = stats(name, values)
	} else {
		out = pctStats(values)
	}
	out.Lost = lost
	if lost > 0 {
		out.P50 = 0
		out.P75 = 0
		out.P95 = 0
	}
	return out
}
func (r *windowRead) vitals(p *population) map[string]VitalStats {
	out := make(map[string]VitalStats, len(beacon.Vitals))
	for _, name := range beacon.Vitals {
		out[name] = measurementStats(name, p.values[name], r.vitalLost[name])
	}
	return out
}
func (st *siteState) topN(kind string) int {
	switch kind {
	case KindPage, KindView:
		return st.cfg.PageGroups
	case KindCountry:
		return st.cfg.Countries
	case KindBrowser:
		return browserTopN
	case KindVersion:
		return versionTopN
	default:
		return 4
	}
}
func (r *windowRead) rank(kind string, n int) []Group {
	var keys []groupKey
	for key := range r.groups {
		if key.kind == kind && !key.other {
			keys = append(keys, key)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := r.groups[keys[i]], r.groups[keys[j]]
		av, bv := a.documents, b.documents
		if kind == KindView {
			av, bv = a.views, b.views
		}
		if av != bv {
			return av > bv
		}
		return keys[i].value < keys[j].value
	})
	if n < 0 {
		n = 0
	}
	if n > len(keys) {
		n = len(keys)
	}
	var out []Group
	for _, key := range keys[:n] {
		out = append(out, r.emitGroup(key, r.groups[key]))
	}
	other := newPopulation()
	present := false
	if g := r.groups[groupKey{
		kind:  kind,
		value: Other,
		other: true,
	}]; g != nil {
		mergePopulation(other, g)
		present = true
	}
	for _, key := range keys[n:] {
		mergePopulation(other, r.groups[key])
		present = true
	}
	if present {
		out = append(out, r.emitGroup(groupKey{
			kind:  kind,
			value: Other,
			other: true,
		}, other))
	}
	return out
}
func mergePopulation(dst, src *population) {
	dst.documents += src.documents
	dst.views += src.views
	dst.errors += src.errors
	for name, values := range src.values {
		dst.values[name] = append(dst.values[name], values...)
	}
}
func (r *windowRead) emitGroup(key groupKey, p *population) Group {
	out := Group{
		Value:            key.value,
		Other:            key.other,
		Vitals:           r.vitals(p),
		Pageviews:        p.documents,
		ApplicationViews: p.views,
		JSErrors:         p.errors,
		Lost:             r.windowLost,
	}
	if key.kind == KindView {
		out.Vitals = nil
	}
	return out
}
