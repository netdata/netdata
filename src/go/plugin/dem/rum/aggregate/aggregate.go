// SPDX-License-Identifier: GPL-3.0-or-later

// Package aggregate is the in-memory RUM aggregator: a sliding
// window of web-vital samples per (vital, breakdown) for one site with
// percentiles and CWV ratings computed at snapshot time, monotonic
// counters, a session LRU and top-N breakdown folding into "other".
package aggregate

import (
	"container/list"
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

// SiteCfg is the immutable configuration for one site. Name is the stable
// identity; DisplayName is presentation metadata.
type SiteCfg struct {
	Name, DisplayName string
	PageGroups        int
	Countries         int
	Investigate       InvestigateCfg
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

// Aggregator owns the rolling measurements and investigation state for one site.
type Aggregator struct {
	mu     sync.Mutex
	window time.Duration
	now    func() time.Time
	site   siteState

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

type pageviewKey struct{ session, page string }

type siteState struct {
	cfg      SiteCfg
	series   map[seriesKey]*series
	groups   map[groupKey]*group
	groupN   map[string]int             // kind → tracked group count
	other    map[string]*group          // kind → fold counters
	top      map[string]map[string]bool // kind → values emitted as own instances (last snapshot)
	counters map[string]uint64
	dedup    map[pageviewKey]time.Time
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

// New creates an aggregator for one site with immutable configuration.
func New(window time.Duration, cfg SiteCfg) *Aggregator {
	return &Aggregator{
		window: window,
		now:    time.Now,
		site: siteState{
			cfg:         cfg,
			series:      map[seriesKey]*series{},
			groups:      map[groupKey]*group{},
			groupN:      map[string]int{},
			other:       map[string]*group{},
			top:         map[string]map[string]bool{},
			counters:    map[string]uint64{},
			dedup:       map[pageviewKey]time.Time{},
			sess:        list.New(),
			sessIdx:     map[string]*list.Element{},
			errGroups:   list.New(),
			errGroupIdx: map[string]*list.Element{},
			resHosts:    map[string]*resHostGroup{},
		},
	}
}
