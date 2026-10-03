// SPDX-License-Identifier: GPL-3.0-or-later

// Package agg is the in-memory RUM aggregator: a sliding
// window of web-vital samples per (site, vital, breakdown) with
// percentiles and CWV ratings computed at snapshot time, monotonic
// counters, a session LRU and top-N breakdown folding into "other".
package agg

import (
	"container/list"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
)

// Breakdown kinds (label names) in emission order.
const (
	KindBrowser = "browser"
	KindDevice  = "device"
	KindCountry = "country"
	KindPage    = "page"
	KindVersion = "version" // declared only once a version is seen
)

// Kinds lists the breakdowns in emission order. version is appended last
// so existing browser/device/country/page chart priorities
// are unaffected.
var Kinds = []string{KindBrowser, KindDevice, KindCountry, KindPage, KindVersion}

// versionTopN is the fixed top-N for the version breakdown ("top-10 by pageviews"); unlike page/country it has no per-site config.
const versionTopN = 10

// Other is the fold bucket for values outside the top-N.
const Other = "other"

// Site-level counter names (rum.pageviews / rum.js_errors / rum.beacons /
// rum.otlp dimensions plus internal bookkeeping).
const (
	CounterPageviews      = "pageviews"
	CounterJSErrors       = "js_errors"
	CounterAccepted       = "accepted"
	CounterOTLPSent       = "otlp_sent"
	CounterOTLPDropped    = "otlp_dropped"
	CounterOTLPErrors     = "otlp_errors"
	CounterSamplesDropped = "samples_dropped"

	// History writer counters: the rum.history chart's
	// written/dropped dims. Fed by the history writer's own goroutine via
	// Add, never from inside Ingest (see HistorySink's doc comment).
	CounterHistoryWritten = "history_written"
	CounterHistoryDropped = "history_dropped"
	// Browser span export, the rum.spans chart.
	CounterSpansSent    = "spans_sent"
	CounterSpansDropped = "spans_dropped"
	CounterSpansErrors  = "spans_errors"
	// Frustration signals, the rum.frustration chart.
	CounterRageClicks  = "rage_clicks"
	CounterDeadClicks  = "dead_clicks"
	CounterErrorClicks = "error_clicks"
)

var frustrationCounters = map[string]string{
	beacon.RageClickEvent: CounterRageClicks, beacon.DeadClickEvent: CounterDeadClicks, beacon.ErrorClickEvent: CounterErrorClicks,
}

const (
	maxSamplesPerSeries  = 10000
	pageviewDedup        = 10 * time.Second
	maxTrackedGroups     = 1000 // per kind; beyond this new values fold straight into other
	browserTopN          = 20   // browser names come from UA parsing and are client-controlled
	unknownValue         = "unknown"
	maxPageGroupSessions = 2000 // per page group, bounded distinct-session set (rum-pages "sessions")
	maxPageElements      = 300  // per page group, attributed vital samples kept for rum-pages
)

// SiteCfg is what the aggregator needs per site.
type SiteCfg struct {
	Key, Name   string
	PageGroups  int
	Countries   int
	Investigate InvestigateCfg
}

// InvestigateCfg is investigate sampling: the share of measured
// sessions kept in full (history, events), plus sessions always kept
// because they had an error or a poor Web Vital. Rate 0 means unset: keep
// every session.
type InvestigateCfg struct {
	Rate                       float64
	KeepErrors, KeepPoorVitals bool
}

func (c InvestigateCfg) rate() float64 {
	if c.Rate <= 0 {
		return 1
	}
	return c.Rate
}

// alwaysKeep reports whether b alone makes its session investigated.
func (c InvestigateCfg) alwaysKeep(b *beacon.Beacon) bool {
	return (c.KeepErrors && len(b.Errors) > 0) || (c.KeepPoorVitals && b.HasPoorVital())
}

// Aggregator implements beacon.Sink and produces Snapshots.
type Aggregator struct {
	mu     sync.Mutex
	window time.Duration
	now    func() time.Time
	sites  map[string]*siteState

	// live is the rum-live FUNCTION ring for this aggregator runtime, guarded by mu.
	live      []LiveRow
	liveStart int // rows before it have left the ring (see evictLive)
	// liveSeq orders rows within this runtime; the routing owner supplies its generation.
	liveSeq uint64

	// history is the optional RUM history sink; nil disables persistence
	// without affecting live aggregation.
	history HistorySink
}

type seriesKey struct{ vital, kind, value string }
type groupKey struct{ kind, value string }

type series struct {
	ts   []time.Time
	vals []float64
}

type group struct {
	pageviews uint64
	jsErrors  uint64
	pvTimes   []time.Time // windowed page views, for ranking
	lastSeen  time.Time

	// Page groups only (rum-pages): windowed error count and
	// a bounded distinct-session set. Left nil/empty for every other kind.
	errWindow []tsCount
	sessions  map[string]struct{}
	// elements are attributed vital samples and frWindow the
	// frustration signals, both over the aggregation window.
	elements []elemSample
	frWindow []tsCount
}

// elemSample is one vital sample with the element Faro attributed it to.
type elemSample struct {
	ts         time.Time
	vital, sel string
	poor       bool
}

type siteState struct {
	cfg      SiteCfg
	series   map[seriesKey]*series
	groups   map[groupKey]*group
	groupN   map[string]int             // kind → tracked group count
	other    map[string]*group          // kind → fold counters
	top      map[string]map[string]bool // kind → values emitted as own instances (last snapshot)
	counters map[string]uint64
	dedup    map[string]time.Time
	sess     *list.List // front = most recent
	sessIdx  map[string]*list.Element

	// rum-sites FUNCTION reads: kept separate from the
	// breakdown series above so polling the function never disturbs chart
	// ranking state.
	accepted     []time.Time // accepted-beacon timestamps, 60s ring (beacons_per_min)
	rejected     []time.Time // origin+rate+size reject timestamps, 60s ring (rejected_per_min)
	bots         []time.Time // filtered bot beacons, 60s ring (bots_per_min)
	pvWindow     []time.Time // page-view timestamps, aggregation-window ring (pageviews_window)
	jsErrWindow  []tsCount   // js-error batches, aggregation-window ring (js_errors_window)
	lastAccepted time.Time   // last accepted beacon time; zero = never (last_beacon_age_s)

	// Error groups: fingerprint → bounded summary, LRU
	// by last seen.
	errGroups   *list.List // front = most recent
	errGroupIdx map[string]*list.Element

	// Resource timing: sticky once any resource event
	// is seen (gates rum.resources/rum.resource_host_* declaration).
	resourcesSeen                            bool
	firstPartyResources, thirdPartyResources uint64
	resHosts                                 map[string]*resHostGroup
	resHostN                                 int
}

// tsCount is one timestamped batch count (a beacon can carry several JS
// errors at once).
type tsCount struct {
	t time.Time
	n uint64
}

// New creates an aggregator with the given sliding window.
func New(window time.Duration) *Aggregator {
	return &Aggregator{
		window: window,
		now:    time.Now,
		sites:  map[string]*siteState{},
	}
}

// Configure applies window and per-site limits; sites absent from the
// list are dropped. Native site jobs configure a fresh aggregator before admission.
func (a *Aggregator) Configure(window time.Duration, sites []SiteCfg) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.window = window
	want := map[string]bool{}
	for _, sc := range sites {
		want[sc.Key] = true
		if st, ok := a.sites[sc.Key]; ok {
			st.cfg = sc
			continue
		}
		a.sites[sc.Key] = &siteState{
			cfg:         sc,
			series:      map[seriesKey]*series{},
			groups:      map[groupKey]*group{},
			groupN:      map[string]int{},
			other:       map[string]*group{},
			top:         map[string]map[string]bool{},
			counters:    map[string]uint64{},
			dedup:       map[string]time.Time{},
			sess:        list.New(),
			sessIdx:     map[string]*list.Element{},
			errGroups:   list.New(),
			errGroupIdx: map[string]*list.Element{},
			resHosts:    map[string]*resHostGroup{},
		}
	}
	for k := range a.sites {
		if !want[k] {
			delete(a.sites, k)
		}
	}
}

// Reject counts a collector rejection (beacon.Reject* reasons) and, for
// origin/rate/size, feeds the 60s ring behind rejected_per_min — an invalid body never reached a rate worth surfacing.
func (a *Aggregator) Reject(site, reason string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	st, ok := a.sites[site]
	if !ok {
		return
	}
	st.counters[reason]++
	switch reason {
	case beacon.RejectOrigin, beacon.RejectRate, beacon.RejectSize:
		st.rejected = append(st.rejected, a.now())
	case beacon.RejectBot:
		st.bots = append(st.bots, a.now())
	}
}

// Add bumps a site-level monotonic counter (used by the OTLP exporter for
// otlp_sent/dropped/errors). Unknown sites are ignored.
func (a *Aggregator) Add(site, counter string, n uint64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if st, ok := a.sites[site]; ok {
		st.counters[counter] += n
	}
}

// Ingest records one accepted beacon. It sets b.PageView when the beacon
// counts as a new page view so downstream sinks agree with the charts.
func (a *Aggregator) Ingest(b *beacon.Beacon) {
	a.mu.Lock()
	defer a.mu.Unlock()
	st, ok := a.sites[b.Site]
	if !ok {
		return
	}
	now := b.Received
	if now.IsZero() {
		now = a.now()
	}
	st.counters[CounterAccepted]++
	st.accepted = append(st.accepted, now)
	st.lastAccepted = now

	// Page view: one per (session, page) within pageviewDedup. Beacons
	// without a session id cannot be de-duplicated and count as views.
	page := b.PageID
	if page == "" {
		page = b.Path
	}
	if b.SessionID == "" {
		b.PageView = true
	} else {
		key := b.SessionID + "|" + page
		if last, seen := st.dedup[key]; !seen || now.Sub(last) > pageviewDedup {
			b.PageView = true
		}
		st.dedup[key] = now
	}

	var frustrations uint64
	for _, e := range b.Events {
		if c, ok := frustrationCounters[e.Name]; ok {
			st.counters[c]++
			frustrations++
		}
	}
	bd := breakdowns(b)
	if b.PageView {
		st.counters[CounterPageviews]++
		st.pvWindow = append(st.pvWindow, now)
	}
	if n := uint64(len(b.Errors)); n > 0 {
		st.counters[CounterJSErrors] += n
		st.jsErrWindow = append(st.jsErrWindow, tsCount{now, n})
		for _, e := range b.Errors {
			st.recordError(e, now)
		}
	}
	for _, kv := range bd {
		real := st.group(kv.kind, kv.value, now) // the actual tracked instance, nil if the per-kind cap is full
		target := real                           // where monotonic counters land (routed to other pre-promotion)
		if real == nil {
			target = st.otherGroup(kv.kind)
		} else if !st.top[kv.kind][kv.value] {
			// tracked but not an emitted instance: counters go to other,
			// ranking still sees the views so it can enter the top set
			if b.PageView {
				real.pvTimes = append(real.pvTimes, now)
			}
			real.lastSeen = now
			st.recordVitals(b, kv.kind, kv.value, now)
			target = st.otherGroup(kv.kind)
		} else {
			if b.PageView {
				real.pvTimes = append(real.pvTimes, now)
			}
			real.lastSeen = now
			st.recordVitals(b, kv.kind, kv.value, now)
		}
		if b.PageView {
			target.pageviews++
		}
		target.jsErrors += uint64(len(b.Errors))

		// rum-pages reads the real page group directly, not
		// routed through other: it always describes that page's own
		// activity, top-N ranked or not.
		if kv.kind == KindPage && real != nil {
			if n := len(b.Errors); n > 0 {
				real.errWindow = append(real.errWindow, tsCount{now, uint64(n)})
			}
			if frustrations > 0 {
				real.frWindow = append(real.frWindow, tsCount{now, frustrations})
			}
			for _, v := range b.Vitals {
				if v.Element == "" {
					continue
				}
				if len(real.elements) >= maxPageElements {
					real.elements = real.elements[1:]
				}
				real.elements = append(
					real.elements,
					elemSample{
						ts:    now,
						vital: v.Name,
						sel:   v.Element,
						poor:  v.Poor(),
					},
				)
			}
			if b.SessionID != "" {
				if real.sessions == nil {
					real.sessions = map[string]struct{}{}
				}
				if len(real.sessions) < maxPageGroupSessions {
					real.sessions[b.SessionID] = struct{}{}
				}
			}
		}
	}
	st.recordVitals(b, "", "", now)
	if b.Navigation != nil {
		if b.Navigation.HasLoad {
			st.addSample(seriesKey{navLoadName, "", ""}, b.Navigation.LoadMS, now)
		}
		if b.Navigation.HasDCL {
			st.addSample(seriesKey{navDCLName, "", ""}, b.Navigation.DCLMS, now)
		}
	}
	for _, r := range b.Resources {
		st.recordResource(b.PageHost, r, now)
	}

	// Investigate sampling: measure (everything above) always runs;
	// history and events follow the session's decision.
	var investigated bool
	if b.SessionID != "" {
		investigated = st.touchSession(b, now, a.history)
	} else {
		inv := st.cfg.Investigate
		investigated = inv.rate() >= 1 || inv.alwaysKeep(b)
	}
	b.SampledOut = !investigated
	// Sessions already emitted self-contained error events with their timeline.
	// Sessionless errors remain investigable without creating a parent session.
	if investigated && b.SessionID == "" && a.history != nil {
		for _, e := range b.Errors {
			if e.Fingerprint == "" {
				continue
			}
			a.history.Event(HistoryEvent{
				Site:        st.cfg.Key,
				TSUnixUS:    now.UnixMicro(),
				Type:        "error",
				Page:        b.PageGroup,
				Browser:     b.Browser,
				Device:      b.Device,
				Country:     b.Country,
				City:        b.City,
				Version:     b.AppVersion,
				UserID:      b.UserID,
				Fingerprint: e.Fingerprint,
				ErrorType:   e.Type,
				Message:     e.Message,
				Text:        truncateRunes(e.Type+": "+e.Message, sessionEventTextMax),
				SampleStack: truncateStack(e.Stack),
			})
		}
	}

	a.appendLive(b, now)
}

// navLoadName/navDCLName are series keys distinct from any beacon.Vitals
// name (rum.load/rum.dcl, no CWV rating computed for either).
const (
	navLoadName = "__nav_load"
	navDCLName  = "__nav_dcl"
)

// ---- rum-sites FUNCTION activity ----

// SiteActivity is the live per-site read the rum-sites FUNCTION renders.
// Unlike Snapshot, computing it never touches breakdown ranking/top-N
// state, so polling it has no side effect on chart emission.
type SiteActivity struct {
	BeaconsPerMin  int
	RejectedPerMin int
	BotsPerMin     int // bot beacons filtered out
	LastBeaconAgeS int // -1 = never accepted a beacon
	ActiveSessions int
	// InvestigatedSessions is how many of ActiveSessions are kept in full
	// under investigate sampling.
	InvestigatedSessions int
	PageviewsWindow      int
	JSErrorsWindow       uint64
}

// Activity returns the current read for every tracked site. A site absent
// from the result has no state in this aggregator runtime.
func (a *Aggregator) Activity() map[string]SiteActivity {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := a.now()
	out := make(map[string]SiteActivity, len(a.sites))
	for k, st := range a.sites {
		st.evictActivity(now, a.window)
		age := -1
		if !st.lastAccepted.IsZero() {
			age = int(now.Sub(st.lastAccepted) / time.Second)
		}
		var jsErr uint64
		for _, tc := range st.jsErrWindow {
			jsErr += tc.n
		}
		out[k] = SiteActivity{
			BeaconsPerMin:        len(st.accepted),
			RejectedPerMin:       len(st.rejected),
			BotsPerMin:           len(st.bots),
			LastBeaconAgeS:       age,
			ActiveSessions:       st.sess.Len(),
			InvestigatedSessions: st.investigatedSessions(),
			PageviewsWindow:      len(st.pvWindow),
			JSErrorsWindow:       jsErr,
		}
	}
	return out
}

// evictActivity evicts the rings behind Activity independently of the
// breakdown window logic: beacons_per_min/rejected_per_min always use a
// fixed 60s cutoff, pageviews_window/js_errors_window follow the
// configured aggregation window. lastAccepted is never evicted so a
// long-silent site still reports an accurate age.
func (st *siteState) evictActivity(now time.Time, window time.Duration) {
	oneMinAgo := now.Add(-time.Minute)
	cutoff := now.Add(-window)
	st.accepted = evictTimes(st.accepted, oneMinAgo)
	st.rejected = evictTimes(st.rejected, oneMinAgo)
	st.bots = evictTimes(st.bots, oneMinAgo)
	st.pvWindow = evictTimes(st.pvWindow, cutoff)
	st.jsErrWindow = evictTsCounts(st.jsErrWindow, cutoff)
}

type kv struct{ kind, value string }

// breakdowns lists the (kind, value) pairs a beacon contributes to. An
// unknown country is excluded from the country breakdown.
func breakdowns(b *beacon.Beacon) []kv {
	out := make([]kv, 0, 4)
	browser := b.Browser
	if browser == "" {
		browser = unknownValue
	}
	out = append(out, kv{KindBrowser, browser})
	device := b.Device
	if device == "" {
		device = beacon.DeviceDesktop
	}
	out = append(out, kv{KindDevice, device})
	if b.Country != "" {
		out = append(out, kv{KindCountry, b.Country})
	}
	page := b.PageGroup
	if page == "" {
		page = "/"
	}
	out = append(out, kv{KindPage, page})
	if b.AppVersion != "" {
		out = append(out, kv{KindVersion, b.AppVersion})
	}
	return out
}

// group returns the tracked group for (kind, value), creating it unless
// the per-kind tracking cap is reached (nil then).
func (st *siteState) group(kind, value string, now time.Time) *group {
	k := groupKey{kind, value}
	if g, ok := st.groups[k]; ok {
		return g
	}
	if st.groupN[kind] >= maxTrackedGroups {
		return nil
	}
	g := &group{
		lastSeen: now,
	}
	st.groups[k] = g
	st.groupN[kind]++
	return g
}

func (st *siteState) otherGroup(kind string) *group {
	g, ok := st.other[kind]
	if !ok {
		g = &group{}
		st.other[kind] = g
	}
	return g
}

func (st *siteState) recordVitals(b *beacon.Beacon, kind, value string, now time.Time) {
	for _, v := range b.Vitals {
		st.addSample(seriesKey{v.Name, kind, value}, v.Value, now)
	}
}

// addSample appends one windowed sample to a series, dropping the oldest
// once the per-series cap is hit (shared by vitals and navigation timing).
func (st *siteState) addSample(k seriesKey, val float64, now time.Time) {
	s, ok := st.series[k]
	if !ok {
		s = &series{}
		st.series[k] = s
	}
	if len(s.vals) >= maxSamplesPerSeries {
		s.ts = s.ts[1:]
		s.vals = s.vals[1:]
		st.counters[CounterSamplesDropped]++
	}
	s.ts = append(s.ts, now)
	s.vals = append(s.vals, val)
}

// ---- snapshot ----

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

// SiteSnapshot is the emission-ready state of one site.
type SiteSnapshot struct {
	Site           string
	Name           string
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

// Snapshot evicts expired samples/sessions/groups and returns per-site
// snapshots sorted by key. It also fixes the top sets used to route
// counters until the next call.
func (a *Aggregator) Snapshot() []SiteSnapshot {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := a.now()
	keys := make([]string, 0, len(a.sites))
	for k := range a.sites {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]SiteSnapshot, 0, len(keys))
	for _, k := range keys {
		out = append(out, a.sites[k].snapshot(now, a.window))
	}
	return out
}

func (st *siteState) snapshot(now time.Time, window time.Duration) SiteSnapshot {
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
			i := 0
			for i < len(g.elements) && g.elements[i].ts.Before(cutoff) {
				i++
			}
			g.elements = g.elements[i:]
		}
	}
	for k, t := range st.dedup {
		if t.Before(cutoff) {
			delete(st.dedup, k)
		}
	}
	// Sessions: TTL from the back (least recent).
	for el := st.sess.Back(); el != nil; el = st.sess.Back() {
		if now.Sub(el.Value.(*sessionDetail).lastSeen) <= beacon.SessionTTL {
			break
		}
		st.dropSession(el)
	}

	snap := SiteSnapshot{
		Site:           st.cfg.Key,
		Name:           st.cfg.Name,
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

func (s *series) evict(cutoff time.Time) {
	i := 0
	for i < len(s.ts) && s.ts[i].Before(cutoff) {
		i++
	}
	if i > 0 {
		s.ts = append(s.ts[:0], s.ts[i:]...)
		s.vals = append(s.vals[:0], s.vals[i:]...)
	}
}

func evictTimes(ts []time.Time, cutoff time.Time) []time.Time {
	i := 0
	for i < len(ts) && ts[i].Before(cutoff) {
		i++
	}
	return append(ts[:0], ts[i:]...)
}

func evictTsCounts(tc []tsCount, cutoff time.Time) []tsCount {
	i := 0
	for i < len(tc) && tc[i].t.Before(cutoff) {
		i++
	}
	return append(tc[:0], tc[i:]...)
}

// countTsCounts sums n for entries at/after cutoff, without mutating tc
// (used by read-only FUNCTION accessors that must not disturb chart state).
func countTsCounts(tc []tsCount, cutoff time.Time) int {
	var n uint64
	for _, c := range tc {
		if !c.t.Before(cutoff) {
			n += c.n
		}
	}
	return int(n)
}

// countTimes counts entries at/after cutoff without mutating ts.
func countTimes(ts []time.Time, cutoff time.Time) int {
	n := 0
	for _, t := range ts {
		if !t.Before(cutoff) {
			n++
		}
	}
	return n
}

// ---- statistics ----

// thresholds are the CWV good/poor boundaries (good ≤ first, poor > second).
var thresholds = map[string][2]float64{
	beacon.LCP:  {2500, 4000},
	beacon.FCP:  {1800, 3000},
	beacon.TTFB: {800, 1800},
	beacon.INP:  {200, 500},
	beacon.CLS:  {0.1, 0.25},
}

// pctStats computes percentiles only, no CWV rating (navigation timing has no good/needs_improvement/poor buckets).
func pctStats(vals []float64) VitalStats {
	sorted := make([]float64, len(vals))
	copy(sorted, vals)
	sort.Float64s(sorted)
	return VitalStats{
		N:   len(sorted),
		P50: pctSorted(sorted, 0.50),
		P75: pctSorted(sorted, 0.75),
		P95: pctSorted(sorted, 0.95),
	}
}

func stats(vital string, vals []float64) VitalStats {
	sorted := make([]float64, len(vals))
	copy(sorted, vals)
	sort.Float64s(sorted)
	st := VitalStats{
		N:   len(sorted),
		P50: pctSorted(sorted, 0.50),
		P75: pctSorted(sorted, 0.75),
		P95: pctSorted(sorted, 0.95),
	}
	th := thresholds[vital]
	good, poor := 0, 0
	for _, v := range sorted {
		switch {
		case v <= th[0]:
			good++
		case v > th[1]:
			poor++
		}
	}
	n := len(sorted)
	// Counts, not percentages: sum-aggregation across sites and nodes then
	// yields traffic-weighted ratios, like httpcheck.status.
	st.Good = good
	st.Poor = poor
	st.NeedsImpr = n - good - poor
	return st
}

// percentile is nearest-rank on an unsorted copy.
func percentile(vals []float64, p float64) float64 {
	sorted := make([]float64, len(vals))
	copy(sorted, vals)
	sort.Float64s(sorted)
	return pctSorted(sorted, p)
}

func pctSorted(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(math.Ceil(float64(len(sorted))*p)) - 1 // nearest rank
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}
