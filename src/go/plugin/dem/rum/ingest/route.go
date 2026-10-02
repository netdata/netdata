// SPDX-License-Identifier: GPL-3.0-or-later

package ingest

import (
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/config"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
)

// Route is one immutable site policy and its runtime-owned sinks/diagnostics.
// Registration leases fence access; receiver replacement does not replace it.
type Route struct {
	config config.RumSite
	policy *site
	sink   beacon.Sink
	reach  reachState
	rateMu sync.Mutex
	rate   float64
	bucket bucket
}

func NewRoute(cfg config.RumSite, sink beacon.Sink) *Route {
	return &Route{
		config: cfg,
		policy: compileSite(cfg),
		sink:   sink,
	}
}
func (s *Route) Config() config.RumSite { return s.config }
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
