// SPDX-License-Identifier: GPL-3.0-or-later

// Package aggregate owns exact canonical receipt-window RUM measurements
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

// versionTopN is the fixed top-N for the version breakdown ("top-10 by pageviews"); unlike page/country it has no per-site config.
const versionTopN = 10

// Other is the display value of a synthetic fold; output also carries a typed flag.
const Other = "other"

// Site-level cumulative event and receiver diagnostic counters.
const (
	CounterPageviews           = "document_views"
	CounterApplicationViews    = "application_views"
	CounterInvalidMeasurements = "invalid_measurements"
	CounterJSErrors            = "js_errors"
	CounterAccepted            = "accepted"

	// Frustration signals, the rum.frustration chart.
	CounterRageClicks  = "rage_clicks"
	CounterDeadClicks  = "dead_clicks"
	CounterErrorClicks = "error_clicks"
)

var frustrationCounters = map[string]string{
	beacon.RageClickEvent: CounterRageClicks, beacon.DeadClickEvent: CounterDeadClicks, beacon.ErrorClickEvent: CounterErrorClicks,
}

const (
	identityRetention = 30 * time.Minute
	browserTopN       = 20
	unknownValue      = "unknown"
)

// SiteCfg configures one job. VnodeGUID follows its resolved host; Name is the stable
// identity; DisplayName is presentation metadata.
type SiteCfg struct {
	VnodeGUID          string
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
	return (c.KeepErrors && len(b.Errors) > 0) || (c.KeepPoorVitals && hasPoorVital(b))
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
// Canonical populations expire by receipt time, without count-driven eviction.
// Receipt times live in the retaining receiptEntry. An attributes value is
// immutable once retained: the items one report creates share it.
type attributes struct{ browser, device, country, page, version, view string }
type observationKey struct{ experience, name string }
type vitalObservation struct {
	metricID string
	revision uint64
	value    float64
	element  string
	attrs    *attributes
}
type identityKey struct{ kind, experience, item string }

// vitalIdentity is the replay evidence of one vital: its SDK metric identity
// and the dimensions frozen when it was first admitted.
type vitalIdentity struct {
	metricID string
	revision uint64
	attrs    *attributes
}
type activityObservation struct {
	attrs                                  *attributes
	documents, views, errors, frustrations uint64
	fingerprint, message                   string
	resource                               *beacon.Resource
	api                                    bool
}

// siteState owns one site's retained populations. Keyed indexes hold one entry
// per identity; activity and resources hold every admitted report item.
type siteState struct {
	cfg      SiteCfg
	window   time.Duration
	counters map[string]uint64
	// observations is the latest canonical value per (experience, vital).
	observations receiptIndex[observationKey, vitalObservation]
	// identities hold the latest admitted revision per activation, session,
	// navigation or resource identity; vitalIdentities hold vital replay evidence.
	identities, resourceIdentities receiptIndex[identityKey, uint64]
	vitalIdentities                receiptIndex[observationKey, vitalIdentity]
	activity, resources            receiptHeap[struct{}, activityObservation]
	// sessions and pageSessions count observed sessions; a page session is keyed
	// by session and page and holds the page.
	sessions                                                   receiptIndex[string, struct{}]
	pageSessions                                               receiptIndex[string, string]
	sess                                                       *list.List
	sessIdx                                                    map[string]*list.Element
	accepted, rejected, bots                                   []time.Time
	lastActivityExpiry                                         time.Time
	lastAccepted                                               time.Time
	resourcesSeen                                              bool
	firstPartyResources, thirdPartyResources, unknownResources uint64
}

// New creates an aggregator for one site with its initial resolved vnode identity.
func New(window time.Duration, cfg SiteCfg) *Aggregator {
	return &Aggregator{
		window: window,
		now:    time.Now,
		site:   newSiteState(window, cfg),
	}
}

func newSiteState(window time.Duration, cfg SiteCfg) siteState {
	return siteState{
		cfg:                cfg,
		window:             window,
		counters:           map[string]uint64{},
		observations:       newReceiptIndex[observationKey, vitalObservation](),
		identities:         newReceiptIndex[identityKey, uint64](),
		resourceIdentities: newReceiptIndex[identityKey, uint64](),
		vitalIdentities:    newReceiptIndex[observationKey, vitalIdentity](),
		sessions:           newReceiptIndex[string, struct{}](),
		pageSessions:       newReceiptIndex[string, string](),
		sess:               list.New(),
		sessIdx:            map[string]*list.Element{},
	}
}

// SetVnodeGUID starts a fresh measurement window when the configured host changes.
// Investigation buffers and queued evidence keep their observation-time identity.
func (a *Aggregator) SetVnodeGUID(guid string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.site.cfg.VnodeGUID == guid {
		return
	}
	old := a.site
	cfg := old.cfg
	cfg.VnodeGUID = guid
	a.site = newSiteState(a.window, cfg)
	a.site.sess, a.site.sessIdx = old.sess, old.sessIdx
}
