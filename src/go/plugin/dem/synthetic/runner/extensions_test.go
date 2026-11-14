// SPDX-License-Identifier: GPL-3.0-or-later

package runner

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// optionalStatus is the stored status of every optional output of one attempt.
type optionalStatus struct {
	evidence, browser string // availability/reason/reporter_complete
	captures, trace   string
}

// TestOptionalOutputStatusMatrix pins the stored status values, including
// invalidity reasons, that negotiation and feature records produce.
func TestOptionalOutputStatusMatrix(t *testing.T) {
	const (
		all        = `{"type":"extension_init","extension_init":{"selected":["failure_evidence.v1","capture_details.v1","failure_trace.v1","browser_summary.v1"]}}`
		none       = `{"type":"extension_init","extension_init":{"selected":[]}}`
		event      = `{"type":"event","event":{"kind":"suite","at_ms":0}}`
		badFrame   = `{"type":"extension","extension":{"data":{}}}`
		complete   = `{"record":"complete"}`
		browserEnd = `{"record":"complete","run":{"tests":{},"counts":{}}}`
	)
	selected := func(features ...string) string {
		return `{"type":"extension_init","extension_init":{"selected":["` + strings.Join(features, `","`) + `"]}}`
	}
	pending := "unavailable/negotiation_not_reported/false"
	supported := "supported//false"
	done := "supported//true"
	invalid := func(reason string) string { return "invalid/" + reason + "/false" }
	cases := map[string]struct {
		lines []string
		want  optionalStatus
	}{
		"not negotiated": {nil, optionalStatus{pending, pending, "negotiation_not_reported", "negotiation_not_reported"}},
		"all selected":   {[]string{all}, optionalStatus{supported, supported, "partial", "supported"}},
		"none selected":  {[]string{none}, optionalStatus{"unsupported/runtime_declined/false", "unsupported/runtime_declined/false", "unsupported", "unsupported"}},
		"all complete": {
			[]string{all, extensionLine(failureEvidenceFeature, json.RawMessage(complete)), extensionLine(browserSummaryFeature, json.RawMessage(browserEnd)), extensionLine(captureDetailsFeature, json.RawMessage(complete))},
			optionalStatus{done, done, "complete", "supported"},
		},
		// A repeated selection records the same reason for every output.
		"duplicate evidence selection": {[]string{selected(failureEvidenceFeature, failureEvidenceFeature)}, optionalStatus{invalid("unrequested_selection"), invalid("unrequested_selection"), "invalid", "invalid"}},
		"duplicate browser selection":  {[]string{selected(browserSummaryFeature, browserSummaryFeature)}, optionalStatus{invalid("unrequested_selection"), invalid("unrequested_selection"), "invalid", "invalid"}},
		"duplicate capture selection":  {[]string{selected(captureDetailsFeature, captureDetailsFeature)}, optionalStatus{invalid("unrequested_selection"), invalid("unrequested_selection"), "invalid", "invalid"}},
		"duplicate trace selection":    {[]string{selected(captureDetailsFeature, failureTraceFeature, failureTraceFeature)}, optionalStatus{invalid("unrequested_selection"), invalid("unrequested_selection"), "invalid", "invalid"}},
		"unknown selection":            {[]string{selected("future.v1")}, optionalStatus{invalid("unrequested_selection"), invalid("unrequested_selection"), "invalid", "invalid"}},
		"malformed init":               {[]string{`{"type":"extension_init","extension_init":{"selected":null}}`}, optionalStatus{invalid("invalid_init"), invalid("invalid_init"), "invalid", "invalid"}},
		"late init":                    {[]string{event, all}, optionalStatus{invalid("late_or_duplicate_init"), invalid("late_or_duplicate_init"), "invalid", "invalid"}},
		"duplicate init":               {[]string{all, all}, optionalStatus{invalid("late_or_duplicate_init"), invalid("late_or_duplicate_init"), "invalid", "invalid"}},
		"malformed envelope":           {[]string{all, badFrame}, optionalStatus{invalid("invalid_extension"), invalid("invalid_extension"), "invalid", "invalid"}},
		"malformed envelope before init": {
			[]string{badFrame, all}, optionalStatus{invalid("invalid_extension"), invalid("invalid_extension"), "invalid", "invalid"},
		},
		"complete then malformed envelope": {
			[]string{all, extensionLine(failureEvidenceFeature, json.RawMessage(complete)), badFrame},
			optionalStatus{invalid("invalid_extension"), invalid("invalid_extension"), "invalid", "invalid"},
		},
		"evidence before init": {
			[]string{extensionLine(failureEvidenceFeature, json.RawMessage(complete)), all}, optionalStatus{invalid("data_before_init"), supported, "partial", "supported"},
		},
		"browser before init": {
			[]string{extensionLine(browserSummaryFeature, json.RawMessage(browserEnd)), all}, optionalStatus{supported, invalid("data_before_init"), "partial", "supported"},
		},
		"captures before init": {
			[]string{extensionLine(captureDetailsFeature, json.RawMessage(complete)), all}, optionalStatus{supported, supported, "invalid", "supported"},
		},
		"unknown feature before init": {
			[]string{extensionLine("future.v1", json.RawMessage(complete)), all}, optionalStatus{supported, supported, "partial", "supported"},
		},
		"evidence after completion": {
			[]string{all, extensionLine(failureEvidenceFeature, json.RawMessage(complete)), extensionLine(failureEvidenceFeature, json.RawMessage(complete))},
			optionalStatus{invalid("record_after_completion"), supported, "partial", "supported"},
		},
		"browser after completion": {
			[]string{all, extensionLine(browserSummaryFeature, json.RawMessage(browserEnd)), extensionLine(browserSummaryFeature, json.RawMessage(browserEnd))},
			optionalStatus{supported, invalid("record_after_completion"), "partial", "supported"},
		},
		"captures after completion": {
			[]string{all, extensionLine(captureDetailsFeature, json.RawMessage(complete)), extensionLine(captureDetailsFeature, json.RawMessage(complete))},
			optionalStatus{supported, supported, "invalid", "supported"},
		},
		"invalid evidence record": {[]string{all, extensionLine(failureEvidenceFeature, json.RawMessage(`{"record":"future"}`))}, optionalStatus{invalid("invalid_record"), supported, "partial", "supported"}},
		"invalid browser record":  {[]string{all, extensionLine(browserSummaryFeature, json.RawMessage(`{"record":"future"}`))}, optionalStatus{supported, invalid("invalid_record"), "partial", "supported"}},
		"browser producer invalid": {
			[]string{all, extensionLine(browserSummaryFeature, json.RawMessage(`{"record":"invalid","reason":"counter_overflow"}`))},
			optionalStatus{supported, invalid("counter_overflow"), "partial", "supported"},
		},
		"declined feature data": {
			[]string{none, extensionLine(failureEvidenceFeature, json.RawMessage(`{"record":"future"}`)), extensionLine(browserSummaryFeature, json.RawMessage(`{"record":"future"}`)), extensionLine(captureDetailsFeature, json.RawMessage(`{"record":"future"}`))},
			optionalStatus{"unsupported/runtime_declined/false", "unsupported/runtime_declined/false", "unsupported", "unsupported"},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			s := newSink(synthetic.Request{Kind: synthetic.Journey, Capture: true, BrowserErrorSummary: true, Trace: true}, newRedactor(nil))
			require.NoError(t, s.consume(strings.NewReader(strings.Join(tc.lines, "\n"))))
			snap := s.snapshot()
			got := optionalStatus{
				evidence: fmt.Sprintf("%s/%s/%t", snap.evidence.Availability, snap.evidence.Reason, snap.evidence.ReporterComplete),
				browser:  fmt.Sprintf("%s/%s/%t", snap.browser.Availability, snap.browser.Reason, snap.browser.ReporterComplete),
				captures: snap.captures.Reporting,
				trace:    snap.captures.Trace,
			}
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestOptionalOutputsFollowTheRequest checks which outputs a request tracks.
func TestOptionalOutputsFollowTheRequest(t *testing.T) {
	for name, tc := range map[string]struct {
		kind                    synthetic.Kind
		capture, browser, trace bool
		evidence, captures, br  bool
	}{
		"lighthouse":      {kind: synthetic.Lighthouse, capture: true},
		"journey":         {kind: synthetic.Journey, evidence: true},
		"journey capture": {kind: synthetic.Journey, capture: true, evidence: true, captures: true},
		"journey browser": {kind: synthetic.Journey, browser: true, evidence: true, br: true},
		"journey all":     {kind: synthetic.Journey, capture: true, browser: true, trace: true, evidence: true, captures: true, br: true},
	} {
		t.Run(name, func(t *testing.T) {
			snap := newSink(synthetic.Request{Kind: tc.kind, Capture: tc.capture, BrowserErrorSummary: tc.browser, Trace: tc.trace}, newRedactor(nil)).snapshot()
			assert.Equal(t, tc.evidence, snap.evidence != nil)
			assert.Equal(t, tc.captures, snap.captures != nil)
			assert.Equal(t, tc.br, snap.browser != nil)
			if snap.captures != nil {
				assert.Equal(t, tc.trace, snap.captures.Trace != "")
			}
		})
	}
	// Lighthouse ignores optional frames entirely, malformed ones included.
	s := newSink(synthetic.Request{Kind: synthetic.Lighthouse}, newRedactor(nil))
	require.NoError(t, s.consume(strings.NewReader(`{"type":"extension","extension":{"data":{}}}`)))
}

// TestRequestedFeaturesWireOrder pins the extensions list sent to the runtime.
func TestRequestedFeaturesWireOrder(t *testing.T) {
	for name, tc := range map[string]struct {
		request synthetic.Request
		want    []string
	}{
		"lighthouse":      {synthetic.Request{Kind: synthetic.Lighthouse, Capture: true}, nil},
		"journey":         {synthetic.Request{Kind: synthetic.Journey}, []string{failureEvidenceFeature}},
		"journey browser": {synthetic.Request{Kind: synthetic.Journey, BrowserErrorSummary: true}, []string{failureEvidenceFeature, browserSummaryFeature}},
		"journey all": {
			synthetic.Request{Kind: synthetic.Journey, Capture: true, Trace: true, BrowserErrorSummary: true},
			[]string{failureEvidenceFeature, captureDetailsFeature, failureTraceFeature, browserSummaryFeature},
		},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, requestedFeatures(tc.request))
		})
	}
}
