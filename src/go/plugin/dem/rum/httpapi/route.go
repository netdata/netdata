// SPDX-License-Identifier: GPL-3.0-or-later

package httpapi

import (
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/internal/redact"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/config"
)

// Processor accepts normalized observations and accounts for rejected requests.
// The owning collector sequences aggregation and export behind this boundary.
type Processor interface {
	Ingest(*beacon.Beacon)
	Reject(reason string)
}

// RejectedOrigin is the last origin refused by a site, including an OPTIONS
// preflight. The zero value means no origin was refused.
type RejectedOrigin struct {
	Origin string
	At     time.Time
}

// Route is one immutable site policy with its runtime-owned processor, redactor
// and last rejected origin. Registration leases fence access; receiver
// replacement does not replace it.
type Route struct {
	config    config.Site
	policy    *site
	processor Processor
	redactor  *redact.Redactor

	rateMu sync.Mutex
	rate   float64
	bucket bucket

	rejectedMu     sync.Mutex
	rejectedOrigin RejectedOrigin
}

// NewRoute compiles cfg. The redactor masks the site's configured secrets in
// decoded text; nil applies only the built-in credential patterns.
func NewRoute(cfg config.Site, processor Processor, redactor *redact.Redactor) *Route {
	return &Route{
		config:    cfg,
		policy:    compileSite(cfg),
		processor: processor,
		redactor:  redactor,
	}
}

func (s *Route) Config() config.Site { return s.config }

// Redactor is the site redactor handed to NewRoute, possibly nil.
func (s *Route) Redactor() *redact.Redactor { return s.redactor }

// LastRejectedOrigin is the last origin refused by this site, if any.
func (s *Route) LastRejectedOrigin() RejectedOrigin {
	s.rejectedMu.Lock()
	defer s.rejectedMu.Unlock()
	return s.rejectedOrigin
}

func (s *Route) rejectOrigin(origin string, at time.Time) {
	if origin == "" {
		return
	}
	s.rejectedMu.Lock()
	defer s.rejectedMu.Unlock()
	s.rejectedOrigin = RejectedOrigin{
		Origin: origin,
		At:     at,
	}
}

func (s *Route) allowSite(rate float64, now time.Time) bool {
	s.rateMu.Lock()
	defer s.rateMu.Unlock()
	if s.rate != rate {
		s.rate = rate
		s.bucket = bucket{
			tokens: rate,
			last:   now,
		}
	}
	return s.bucket.take(rate, rate, now)
}
