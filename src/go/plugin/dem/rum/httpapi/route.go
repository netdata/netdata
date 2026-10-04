// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/config"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/diagnostics"
)

// Processor accepts normalized observations and accounts for rejected requests.
// The owning collector sequences aggregation and export behind this boundary.
type Processor interface {
	Ingest(*beacon.Beacon)
	Reject(site, reason string)
}

// Route is one immutable site policy and its runtime-owned processor and diagnostic state.
// Registration leases fence access; receiver replacement does not replace it.
type Route struct {
	config      config.Site
	policy      *site
	processor   Processor
	diagnostics *diagnostics.State
	rateMu      sync.Mutex
	rate        float64
	bucket      bucket
}

func NewRoute(cfg config.Site, processor Processor, state *diagnostics.State) *Route {
	return &Route{
		config:      cfg,
		policy:      compileSite(cfg),
		processor:   processor,
		diagnostics: state,
	}
}
func (s *Route) Config() config.Site { return s.config }
func (s *Route) allowSite(rate float64, now time.Time) bool {
	s.rateMu.Lock()
	defer s.rateMu.Unlock()
	if s.rate != rate {
		s.rate = rate
		s.bucket.tokens = rate
		s.bucket.last = now
	}
	elapsed := now.Sub(s.bucket.last).Seconds()
	if elapsed > 0 {
		s.bucket.tokens = min(rate, s.bucket.tokens+elapsed*rate)
		s.bucket.last = now
	}
	if s.bucket.tokens < 1 {
		return false
	}
	s.bucket.tokens--
	return true
}
