// SPDX-License-Identifier: GPL-3.0-or-later
package runner

import (
	"encoding/json"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
)

const browserSummaryFeature = "browser_summary.v1"

// extensions owns the shared negotiation and envelope boundary. Feature bodies
// have separate validity: an identifiable malformed body cannot poison a peer.
type extensions struct {
	evidence                 *evidenceState
	browser                  *browserState
	captures                 *captureState
	initialized, coreStarted bool
}

func newExtensions(kind synthetic.Kind, browser, capture bool) *extensions {
	e := &extensions{evidence: newEvidenceState(kind)}
	if kind == synthetic.Journey && capture {
		e.captures = newCaptureState()
	}
	if kind == synthetic.Journey && browser {
		e.browser = &browserState{value: &synthetic.BrowserSummary{Availability: "unavailable", Reason: "negotiation_not_reported", Coverage: "fixture_context"}}
	}
	return e
}
func (e *extensions) core() { e.coreStarted = true }
func (e *extensions) invalid(reason string) {
	e.evidence.invalid(reason)
	e.browser.invalid(reason)
	e.captures.invalid()
}
func (e *extensions) init(raw json.RawMessage) {
	if e.evidence == nil {
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
	if len(raw) > evidencePayloadLimit || decodeCore(raw, &body) != nil || body.Selected == nil || len(body.Selected) > 3 {
		e.invalid("invalid_init")
		return
	}
	fe, bs, cd := false, false, false
	for _, f := range body.Selected {
		switch f {
		case failureEvidenceFeature:
			if fe {
				e.invalid("invalid_init")
				return
			}
			fe = true
		case browserSummaryFeature:
			if bs || e.browser == nil {
				e.invalid("unrequested_selection")
				return
			}
			bs = true
		case captureDetailsFeature:
			if cd || e.captures == nil {
				e.invalid("unrequested_selection")
				return
			}
			cd = true
		default:
			e.invalid("unrequested_selection")
			return
		}
	}
	if e.evidence.value.Availability != "invalid" {
		if fe {
			e.evidence.value.Availability = "supported"
			e.evidence.value.Reason = ""
		} else {
			e.evidence.value.Availability = "unsupported"
			e.evidence.value.Reason = "runtime_declined"
		}
	}
	if e.browser != nil && e.browser.value.Availability != "invalid" {
		if bs {
			e.browser.value.Availability = "supported"
			e.browser.value.Reason = ""
		} else {
			e.browser.value.Availability = "unsupported"
			e.browser.value.Reason = "runtime_declined"
		}
	}
	if e.captures != nil && e.captures.value.Reporting != "invalid" {
		if cd {
			e.captures.value.Reporting = "partial"
		} else {
			e.captures.value.Reporting = "unsupported"
		}
	}
}
func (e *extensions) accept(raw json.RawMessage, r *textRedactor) {
	if e.evidence == nil {
		return
	}
	var envelope struct {
		Feature string          `json:"feature"`
		Data    json.RawMessage `json:"data"`
	}
	if len(raw) > evidencePayloadLimit || decodeCore(raw, &envelope) != nil || envelope.Feature == "" {
		e.invalid("invalid_extension")
		return
	}
	if !e.initialized {
		if envelope.Feature == failureEvidenceFeature {
			e.evidence.invalid("data_before_init")
		}
		if envelope.Feature == browserSummaryFeature {
			e.browser.invalid("data_before_init")
		}
		if envelope.Feature == captureDetailsFeature {
			e.captures.invalid()
		}
		return
	}
	switch envelope.Feature {
	case failureEvidenceFeature:
		if e.evidence.value.Availability == "supported" {
			e.evidence.accept(envelope.Data, r)
		}
	case browserSummaryFeature:
		if e.browser != nil && e.browser.value.Availability == "supported" {
			e.browser.accept(envelope.Data, r)
		}
	case captureDetailsFeature:
		if e.captures != nil && (e.captures.value.Reporting == "partial" || e.captures.value.Reporting == "complete") {
			e.captures.accept(envelope.Data, r)
		}
	}
}
