// SPDX-License-Identifier: GPL-3.0-or-later

package aggregate

import (
	"maps"
	"sort"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
)

// VitalStats describes the exact retained population. Percentiles are unavailable when N is zero.
type VitalStats struct {
	N                     int
	P50, P75, P95         float64
	Good, NeedsImpr, Poor int
}

// Group contains current-window gauges. Other identifies the synthetic fold;
// a literal value "other" remains a separate real group.
type Group struct {
	Value                                 string
	Other                                 bool
	Vitals                                map[string]VitalStats
	Pageviews, JSErrors, ApplicationViews uint64
}
type Snapshot struct {
	Name, DisplayName                                          string
	Vitals                                                     map[string]VitalStats
	Counters                                                   map[string]uint64
	ObservedSessions                                           int
	Breakdowns                                                 map[string][]Group
	Load, DCL, API                                             VitalStats
	ErrorGroups                                                []ErrorChartGroup
	ResourcesSeen                                              bool
	FirstPartyResources, ThirdPartyResources, UnknownResources uint64
	ResourceHosts                                              []ResourceHostGroup
	PageviewsWindow, JSErrorsWindow, ApplicationViewsWindow    uint64
}

const (
	facetBrowser = iota
	facetDevice
	facetCountry
	facetPage
	facetVersion
	facetView
	facetCount
)

// facetKinds lists the breakdowns in emission order. Application views carry
// activity and error counts, without document-scoped vital distributions.
var facetKinds = [facetCount]string{KindBrowser, KindDevice, KindCountry, KindPage, KindVersion, KindView}

type facetValue struct {
	kind  int
	value string
}
type population struct {
	vitalValues                            *vitalDistributions
	documents, views, errors, frustrations uint64
	sessions                               int
	elements                               []elemSample
}

// Distributions are allocated only for groups containing vitals. Counts allow
// exact-sized owned buffers, avoiding repeated growth while ingestion is locked.
type vitalDistributions struct {
	counts       [vitalCount]int
	values       [vitalCount][]float64
	elementCount int
}

func (p *population) countVital(index int) {
	if p.vitalValues == nil {
		p.vitalValues = &vitalDistributions{}
	}
	p.vitalValues.counts[index]++
}
func (p *population) reserveVitals() {
	if p.vitalValues == nil {
		return
	}
	for index, count := range p.vitalValues.counts {
		if count > 0 {
			p.vitalValues.values[index] = make([]float64, 0, count)
		}
	}
	if p.vitalValues.elementCount > 0 {
		p.elements = make([]elemSample, 0, p.vitalValues.elementCount)
	}
}

type windowRead struct {
	global         population
	groups         [facetCount]map[string]*population
	load, dcl, api []float64
	resourceGroups map[hostKey]*resourcePopulation
	errorGroups    map[string]*errorPopulation // by fingerprint
	sessions       int
}

func (r *windowRead) group(kind int, value string) *population {
	if value == "" {
		value = unknownValue
	}
	groups := r.groups[kind]
	if g := groups[value]; g != nil {
		return g
	}
	if groups == nil {
		groups = make(map[string]*population)
		r.groups[kind] = groups
	}
	g := &population{}
	groups[value] = g
	return g
}
func attributePairs(v *attributes, pageOnly bool) []facetValue {
	if pageOnly {
		return []facetValue{{kind: facetPage, value: v.page}}
	}
	return []facetValue{
		{kind: facetBrowser, value: v.browser},
		{kind: facetDevice, value: v.device},
		{kind: facetCountry, value: v.country},
		{kind: facetPage, value: v.page},
		{kind: facetVersion, value: v.version},
	}
}

// readWindow captures owned values and counts under the aggregator mutex.
// Its distributions can be sorted after unlocking. Page reads derive only the page facet.
func (st *siteState) readWindow(now time.Time, pageOnly bool) *windowRead {
	cutoff := now.Add(-st.window)
	st.expire(now)
	r := &windowRead{
		resourceGroups: map[hostKey]*resourcePopulation{},
		errorGroups:    map[string]*errorPopulation{},
	}
	// Count the same retained population before copying. This extra traversal
	// trades slice growth/copying for exact buffers without persistent accounting.
	for _, el := range st.observations.order.entries {
		v, name := &el.Value, el.key.name
		if el.received.Before(cutoff) {
			continue
		}
		index, ok := vitalIndex(name)
		if !ok {
			continue
		}
		if !pageOnly {
			r.global.countVital(index)
		}
		for _, kv := range attributePairs(v.attrs, pageOnly) {
			g := r.group(kv.kind, kv.value)
			g.countVital(index)
			if pageOnly && hasPageElement(name, v.element) {
				g.vitalValues.elementCount++
			}
		}
	}
	r.global.reserveVitals()
	for _, groups := range r.groups {
		for _, g := range groups {
			g.reserveVitals()
		}
	}
	for _, el := range st.observations.order.entries {
		v, name := &el.Value, el.key.name
		if el.received.Before(cutoff) {
			continue
		}
		index, ok := vitalIndex(name)
		if !ok {
			if !pageOnly {
				switch name {
				case navLoadName:
					r.load = append(r.load, v.value)
				case navDCLName:
					r.dcl = append(r.dcl, v.value)
				}
			}
			continue
		}
		if !pageOnly {
			r.global.vitalValues.values[index] = append(r.global.vitalValues.values[index], v.value)
		}
		for _, kv := range attributePairs(v.attrs, pageOnly) {
			g := r.group(kv.kind, kv.value)
			g.vitalValues.values[index] = append(g.vitalValues.values[index], v.value)
			if pageOnly && hasPageElement(name, v.element) {
				g.elements = append(g.elements, elemSample{
					vital: name,
					sel:   v.element,
					poor:  thresholds[name].isPoor(v.value),
				})
			}
		}
	}
	for _, el := range st.activity.entries {
		v := &el.Value
		if el.received.Before(cutoff) {
			continue
		}
		r.global.documents += v.documents
		r.global.views += v.views
		r.global.errors += v.errors
		r.global.frustrations += v.frustrations
		for _, kv := range attributePairs(v.attrs, pageOnly) {
			g := r.group(kv.kind, kv.value)
			g.documents += v.documents
			g.views += v.views
			g.errors += v.errors
			g.frustrations += v.frustrations
		}
		if !pageOnly && (v.attrs.view != "" || v.views > 0) {
			g := r.group(facetView, v.attrs.view)
			g.views += v.views
			g.errors += v.errors
		}
		if !pageOnly && v.errors > 0 {
			r.addError(v)
		}
	}
	if !pageOnly {
		for _, el := range st.resources.entries {
			r.addResource(&el.Value)
		}
	}
	r.sessions = st.sessions.len()
	for _, el := range st.pageSessions.order.entries {
		if !el.received.Before(cutoff) {
			r.group(facetPage, el.Value).sessions++
		}
	}
	return r
}

func (a *Aggregator) Snapshot() Snapshot {
	a.mu.Lock()
	st := &a.site
	r := st.readWindow(a.now(), false)
	cfg := st.cfg
	snap := Snapshot{
		Name:                   cfg.Name,
		DisplayName:            cfg.DisplayName,
		Counters:               maps.Clone(st.counters),
		ObservedSessions:       r.sessions,
		ResourcesSeen:          st.resourcesSeen,
		FirstPartyResources:    st.firstPartyResources,
		ThirdPartyResources:    st.thirdPartyResources,
		UnknownResources:       st.unknownResources,
		PageviewsWindow:        r.global.documents,
		JSErrorsWindow:         r.global.errors,
		ApplicationViewsWindow: r.global.views,
	}
	a.mu.Unlock()

	snap.Vitals = r.vitals(&r.global)
	snap.Breakdowns = map[string][]Group{}
	snap.Load = pctStats(r.load)
	snap.DCL = pctStats(r.dcl)
	snap.API = pctStats(r.api)
	for facet, kind := range facetKinds {
		snap.Breakdowns[kind] = r.rank(facet, cfg.topN(kind))
	}
	snap.ResourceHosts = r.rankResourceHosts()
	snap.ErrorGroups = r.rankErrorGroups()
	return snap
}
func (r *windowRead) vitals(p *population) map[string]VitalStats {
	out := make(map[string]VitalStats, len(beacon.Vitals))
	for index, name := range beacon.Vitals {
		if p.vitalValues == nil {
			out[name] = VitalStats{}
		} else {
			out[name] = stats(name, p.vitalValues.values[index])
		}
	}
	return out
}
func (cfg SiteCfg) topN(kind string) int {
	switch kind {
	case KindPage, KindView:
		return cfg.PageGroups
	case KindCountry:
		return cfg.Countries
	case KindBrowser:
		return browserTopN
	case KindVersion:
		return versionTopN
	default:
		return 4
	}
}
func (r *windowRead) rank(kind int, n int) []Group {
	if len(r.groups[kind]) == 0 {
		return nil
	}
	type rankedGroup struct {
		value      string
		population *population
	}
	groups := make([]rankedGroup, 0, len(r.groups[kind]))
	for value, p := range r.groups[kind] {
		groups = append(groups, rankedGroup{value, p})
	}
	sort.Slice(groups, func(i, j int) bool {
		a, b := groups[i], groups[j]
		av, bv := a.population.documents, b.population.documents
		if kind == facetView {
			av, bv = a.population.views, b.population.views
		}
		if av != bv {
			return av > bv
		}
		return a.value < b.value
	})
	n = max(0, min(n, len(groups)))
	out := make([]Group, 0, min(n+1, len(groups)))
	for _, g := range groups[:n] {
		out = append(out, r.emitGroup(kind, g.value, false, g.population))
	}
	if n < len(groups) {
		other := &population{}
		for _, g := range groups[n:] {
			if g.population.vitalValues == nil {
				continue
			}
			if other.vitalValues == nil {
				other.vitalValues = &vitalDistributions{}
			}
			for index, values := range g.population.vitalValues.values {
				other.vitalValues.counts[index] += len(values)
			}
		}
		other.reserveVitals()
		for _, g := range groups[n:] {
			mergePopulation(other, g.population)
		}
		out = append(out, r.emitGroup(kind, Other, true, other))
	}
	return out
}
func mergePopulation(dst, src *population) {
	dst.documents += src.documents
	dst.views += src.views
	dst.errors += src.errors
	if src.vitalValues != nil {
		for index, values := range src.vitalValues.values {
			dst.vitalValues.values[index] = append(dst.vitalValues.values[index], values...)
		}
	}
}
func (r *windowRead) emitGroup(kind int, value string, other bool, p *population) Group {
	out := Group{
		Value:            value,
		Other:            other,
		Pageviews:        p.documents,
		ApplicationViews: p.views,
		JSErrors:         p.errors,
	}
	if kind != facetView {
		out.Vitals = r.vitals(p)
	}
	return out
}
