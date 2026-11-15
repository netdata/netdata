// SPDX-License-Identifier: GPL-3.0-or-later

package query

import (
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/aggregate"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/geoip"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/history"
)

// Receiver reports ingress availability without exposing listener configuration.
type Receiver struct {
	Serving   bool
	PublicURL string
	GeoIP     geoip.Status
}

// Site contains only the copied observations needed to investigate one site.
type Site struct {
	VnodeGUID         string
	Name, Label       string
	AllowedOrigins    []string
	Sampling          Sampling
	Capture           Capture
	Activity          Activity
	Generation        string
	CollectionEnabled bool
	Rejected          Rejection
	ScriptURL         string
}
type Capture struct {
	Geolocation        string
	FrustrationSignals bool
}
type Sampling struct {
	MeasureRate, InvestigateRate float64
	KeepErrors, KeepPoorVitals   bool
}
type Rejection struct {
	Origin string
	At     time.Time
}

// These aliases name the domain values Functions read through this package.
// Service owns their copies and redaction; callers receive no runtime owner.
type (
	Activity        = aggregate.SiteActivity
	Page            = aggregate.PageInfo
	LiveObservation = aggregate.LiveRow
	Session         = history.SessionRecord
	SessionEvent    = history.SessionEventRecord
	ErrorGroup      = history.ErrorGroup
)

type LiveEvent struct {
	LiveObservation
	Generation string
}

// History read filters.
type (
	HistoryFilter = history.Filter
	SessionFilter = history.SessionFilter
	ErrorFilter   = history.ErrorFilter
)
