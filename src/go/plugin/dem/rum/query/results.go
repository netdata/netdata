// SPDX-License-Identifier: GPL-3.0-or-later
package query

import (
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/aggregate"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/history"
)

// Receiver reports ingress availability without exposing listener configuration.
type Receiver struct {
	Serving   bool
	PublicURL string
}

// Site contains only the copied observations needed to investigate one site.
type Site struct {
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

// These distinct result types preserve the underlying snapshot value vocabulary.
// Service owns their copies and redaction; callers receive no runtime owner.
type Activity aggregate.SiteActivity
type Page aggregate.PageInfo
type LiveObservation aggregate.LiveRow
type LiveEvent struct {
	LiveObservation
	Generation string
}
type Session history.SessionRecord
type SessionEvent history.SessionEventRecord

// Details distinguishes unrequested statistics from observed zero values.
type ErrorGroup history.ErrorGroup
