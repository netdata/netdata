// SPDX-License-Identifier: GPL-3.0-or-later
package query

import (
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/agg"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/history"
)

// Receiver reports ingress availability without exposing listener configuration.
type Receiver struct {
	Serving   bool
	PublicURL string
}

// Site contains only the copied observations needed to investigate one site.
type Site struct {
	Name, Label    string
	AllowedOrigins []string
	Sampling       Sampling
	Activity       Activity
	Reach, Snippet Diagnostic
	Rejected       Rejection
	PublicBase     string
}
type Sampling struct {
	MeasureRate, InvestigateRate float64
	KeepErrors, KeepPoorVitals   bool
}
type Diagnostic struct{ State, Detail string }
type Rejection struct {
	Origin string
	At     time.Time
}

// These distinct result types preserve the underlying snapshot value vocabulary.
// Service owns their copies and redaction; callers receive no runtime owner.
type Activity agg.SiteActivity
type Page agg.PageInfo
type LiveObservation agg.LiveRow
type LiveEvent struct {
	LiveObservation
	Generation string
}
type Session history.SessionRecord
type SessionEvent history.SessionEventRecord

// Details distinguishes unrequested statistics from observed zero values.
type ErrorGroup history.ErrorGroup
