// SPDX-License-Identifier: GPL-3.0-or-later

// Package aggregate owns bounded, canonical receipt-window RUM measurements
// for one site. It replaces identified metric revisions, derives consistent
// facets at read time, and keeps investigation detail independent of measurement.
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
	KindView    = "view"
	KindVersion = "version"
)

// Kinds lists the breakdowns in emission order. Application views carry
// activity and error counts, without document-scoped vital distributions.
var Kinds = []string{KindBrowser, KindDevice, KindCountry, KindPage, KindVersion, KindView}

// versionTopN is the fixed top-N for the version breakdown ("top-10 by pageviews"); unlike page/country it has no per-site config.
const versionTopN = 10

// Other is the display value of a synthetic fold; output also carries a typed flag.
const Other = "other"

// Site-level cumulative event and receiver/export diagnostic counters.
const (
	CounterPageviews           = "document_views"
	CounterApplicationViews    = "application_views"
	CounterInvalidMeasurements = "invalid_measurements"
	CounterJSErrors            = "js_errors"
	CounterAccepted            = "accepted"
	CounterOTLPSent            = "otlp_sent"
	CounterOTLPDropped         = "otlp_dropped"
	CounterOTLPErrors          = "otlp_errors"
	CounterSamplesDropped      = "samples_dropped"

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
	maxWindowObservations = 10000
	identityRetention     = 30 * time.Minute
	browserTopN           = 20
	unknownValue          = "unknown"
)

// SiteCfg is the immutable configuration for one site. Name is the stable
// identity; DisplayName is presentation metadata.
type SiteCfg struct {
	Name, DisplayName  string
	PageGroups         int
	Countries          int
	Investigate        InvestigateCfg
	FrustrationSignals bool
}

// InvestigateCfg contains effective detail-selection policy. Rate is the
// baseline share of measured sessions; zero selects only configured overrides.
// Config owns defaults before constructing the aggregator.
type InvestigateCfg struct {
	Rate                       float64
	KeepErrors, KeepPoorVitals bool
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

// Measurement retention is independent of investigation sampling and detail LRU.
// Bounds reuse the former sample/group budget, now once per canonical population.
type attributes struct{ browser, device, country, page, version, view string }
type observationKey struct{ experience, name string }
type vitalObservation struct {
	key      observationKey
	metricID string
	revision uint64
	received time.Time
	value    float64
	element  string
	attrs    attributes
}
type identityKey struct{ kind, experience, item string }
type identity struct {
	key      identityKey
	metricID string
	revision uint64
	received time.Time
	attrs    attributes
}
type sessionObservation struct {
	key, page string
	received  time.Time
}
type activityObservation struct {
	received                               time.Time
	attrs                                  attributes
	documents, views, errors, frustrations uint64
	fingerprint, message                   string
	resource                               *beacon.Resource
	pageHost                               string
}

// A loss interval conservatively remains incomplete until its newest lost
// observation leaves the window. It needs constant space during overload.
type lossInterval struct {
	until time.Time
	count uint64
}

func (l *lossInterval) add(received time.Time, window time.Duration) {
	until := received.Add(window)
	if until.After(l.until) {
		l.until = until
	}
	l.count++
}
func (l *lossInterval) current(now time.Time) uint64 {
	if now.After(l.until) {
		l.count = 0
		l.until = time.Time{}
	}
	return l.count
}

type siteState struct {
	cfg                                                        SiteCfg
	window                                                     time.Duration
	counters                                                   map[string]uint64
	observations                                               map[observationKey]*receiptEntry
	observationOrder                                           receiptHeap
	identities                                                 map[identityKey]*receiptEntry
	identityOrder                                              receiptHeap
	activity, resources                                        receiptHeap
	sessions                                                   map[string]*receiptEntry
	sessionOrder                                               receiptHeap
	pageSessions                                               map[string]*receiptEntry
	pageSessionOrder                                           receiptHeap
	vitalLoss                                                  map[string]*lossInterval
	activityLoss, resourceLoss, sessionLoss, pageSessionLoss   lossInterval
	identityLoss                                               map[string]*lossInterval
	identitySessionLoss                                        lossInterval
	resourceIdentities                                         map[identityKey]*receiptEntry
	resourceIdentityOrder                                      receiptHeap
	sess                                                       *list.List
	sessIdx                                                    map[string]*list.Element
	accepted, rejected, bots                                   []time.Time
	lastAccepted                                               time.Time
	resourcesSeen                                              bool
	firstPartyResources, thirdPartyResources, unknownResources uint64
}

// New creates an aggregator for one site with immutable configuration.
func New(window time.Duration, cfg SiteCfg) *Aggregator {
	return &Aggregator{
		window: window,
		now:    time.Now,
		site: siteState{
			cfg:                cfg,
			window:             window,
			counters:           map[string]uint64{},
			observations:       map[observationKey]*receiptEntry{},
			identities:         map[identityKey]*receiptEntry{},
			sessions:           map[string]*receiptEntry{},
			pageSessions:       map[string]*receiptEntry{},
			vitalLoss:          map[string]*lossInterval{},
			identityLoss:       map[string]*lossInterval{},
			resourceIdentities: map[identityKey]*receiptEntry{},
			sess:               list.New(),
			sessIdx:            map[string]*list.Element{},
		},
	}
}
