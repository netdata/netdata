// SPDX-License-Identifier: GPL-3.0-or-later
package runner

import (
	"encoding/json"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
)

// Optional output formats the Agent may request and the runtime may select
// (src/dem-runtime/README.md, "Additions and required capabilities").
const (
	failureEvidenceFeature = "failure_evidence.v1"
	captureDetailsFeature  = "capture_details.v1"
	failureTraceFeature    = "failure_trace.v1"
	browserSummaryFeature  = "browser_summary.v1"
)

// extensionBodyLimit is the runtime's 32 KiB limit on every extension_init and
// extension body, far below the 4 MiB frame ceiling.
const extensionBodyLimit = 32 << 10

// requestedFeatures lists the optional outputs an attempt asks for, in wire
// request order. ValidateRequest allows traces only on capture-enabled journeys.
func requestedFeatures(r synthetic.Request) []string {
	if r.Kind != synthetic.Journey {
		return nil
	}
	features := []string{failureEvidenceFeature}
	if r.Capture {
		features = append(features, captureDetailsFeature)
		if r.Trace {
			features = append(features, failureTraceFeature)
		}
	}
	if r.BrowserErrorSummary {
		features = append(features, browserSummaryFeature)
	}
	return features
}

// featurePhase is the negotiation and reporting lifecycle shared by every
// optional output.
type featurePhase uint8

const (
	featureRequested featurePhase = iota // no valid extension_init yet
	featureDeclined                      // extension_init did not select it
	featureSelected                      // selected; its reporter has not completed
	featureComplete                      // its reporter completed the stream
	featureInvalid                       // malformed framing or feature data
)

// featureState owns one output's lifecycle. Stored status fields are
// projections of it; records accepted before invalidation remain.
type featureState struct {
	phase  featurePhase
	reason string
}

// negotiate applies a valid extension_init unless the output is already invalid.
func (s *featureState) negotiate(selected bool) {
	if s.phase != featureRequested {
		return
	}
	s.phase = featureDeclined
	if selected {
		s.phase = featureSelected
	}
}

// invalidate keeps the first reason.
func (s *featureState) invalidate(reason string) {
	if s.phase != featureInvalid {
		s.phase, s.reason = featureInvalid, reason
	}
}

func (s *featureState) complete() { s.phase = featureComplete }

// receive reports whether a record may be decoded: only a selected, incomplete
// stream accepts records, and a record after completion invalidates it.
func (s *featureState) receive() bool {
	switch s.phase {
	case featureSelected:
		return true
	case featureComplete:
		s.invalidate("record_after_completion")
	}
	return false
}

// status projects the lifecycle onto the stored evidence and browser-summary fields.
func (s *featureState) status() synthetic.FeatureStatus {
	switch s.phase {
	case featureRequested:
		return synthetic.FeatureStatus{Availability: synthetic.FeatureUnavailable, Reason: "negotiation_not_reported"}
	case featureDeclined:
		return synthetic.FeatureStatus{Availability: synthetic.FeatureUnsupported, Reason: "runtime_declined"}
	case featureSelected:
		return synthetic.FeatureStatus{Availability: synthetic.FeatureSupported}
	case featureComplete:
		return synthetic.FeatureStatus{Availability: synthetic.FeatureSupported, ReporterComplete: true}
	default:
		return synthetic.FeatureStatus{Availability: synthetic.FeatureInvalid, Reason: s.reason}
	}
}

// word projects the lifecycle onto the one-word statuses stored in a capture
// report. selected names a selected, incomplete output: "partial" capture
// details, or "supported" traces, which have no records to complete.
func (s *featureState) word(selected string) string {
	switch s.phase {
	case featureRequested:
		return "negotiation_not_reported"
	case featureDeclined:
		return "unsupported"
	case featureSelected:
		return selected
	case featureComplete:
		return "complete"
	default:
		return string(synthetic.FeatureInvalid)
	}
}

// extensions owns negotiation and the shared envelope boundary. Each feature
// body has its own validity, so a malformed body cannot poison a peer; framing
// and initialization failures invalidate every requested output.
type extensions struct {
	requested                []string
	evidence                 *evidenceState
	browser                  *browserState
	captures                 *captureState
	initialized, coreStarted bool
}

func newExtensions(request synthetic.Request) *extensions {
	e := &extensions{requested: requestedFeatures(request)}
	for _, feature := range e.requested {
		switch feature {
		case failureEvidenceFeature:
			e.evidence = newEvidenceState()
		case browserSummaryFeature:
			e.browser = newBrowserState()
		case captureDetailsFeature:
			e.captures = newCaptureState()
		case failureTraceFeature:
			e.captures.trace = &featureState{}
		}
	}
	return e
}

// state returns the lifecycle of a requested output, or nil.
func (e *extensions) state(feature string) *featureState {
	switch {
	case feature == failureEvidenceFeature && e.evidence != nil:
		return &e.evidence.featureState
	case feature == browserSummaryFeature && e.browser != nil:
		return &e.browser.featureState
	case feature == captureDetailsFeature && e.captures != nil:
		return &e.captures.featureState
	case feature == failureTraceFeature && e.captures != nil:
		return e.captures.trace
	}
	return nil
}

func (e *extensions) core() { e.coreStarted = true }

func (e *extensions) invalid(reason string) {
	for _, feature := range e.requested {
		e.state(feature).invalidate(reason)
	}
}

func (e *extensions) init(raw json.RawMessage) {
	if len(e.requested) == 0 {
		return
	}
	if e.initialized || e.coreStarted {
		e.invalid("late_or_duplicate_init")
		return
	}
	e.initialized = true
	var body struct {
		Selected []string `json:"selected"`
	}
	if len(raw) > extensionBodyLimit || decodeCore(raw, &body) != nil || body.Selected == nil {
		e.invalid("invalid_init")
		return
	}
	selected := make(map[string]bool, len(body.Selected))
	for _, feature := range body.Selected {
		if selected[feature] || e.state(feature) == nil {
			e.invalid("unrequested_selection")
			return
		}
		selected[feature] = true
	}
	for _, feature := range e.requested {
		e.state(feature).negotiate(selected[feature])
	}
}

// accept routes one extension body to its typed decoder. Lighthouse requests
// no outputs and ignores these frames; traces have no records.
func (e *extensions) accept(raw json.RawMessage, r *textRedactor) {
	if len(e.requested) == 0 {
		return
	}
	var envelope struct {
		Feature string          `json:"feature"`
		Data    json.RawMessage `json:"data"`
	}
	if len(raw) > extensionBodyLimit || decodeCore(raw, &envelope) != nil || envelope.Feature == "" {
		e.invalid("invalid_extension")
		return
	}
	switch envelope.Feature {
	case failureEvidenceFeature:
		if e.evidence != nil && e.receive(&e.evidence.featureState) {
			e.evidence.accept(envelope.Data, r)
		}
	case browserSummaryFeature:
		if e.browser != nil && e.receive(&e.browser.featureState) {
			e.browser.accept(envelope.Data, r)
		}
	case captureDetailsFeature:
		if e.captures != nil && e.receive(&e.captures.featureState) {
			e.captures.accept(envelope.Data, r)
		}
	}
}

// receive gates a record of a requested output; data before initialization
// invalidates that output alone.
func (e *extensions) receive(s *featureState) bool {
	if !e.initialized {
		s.invalidate("data_before_init")
		return false
	}
	return s.receive()
}
