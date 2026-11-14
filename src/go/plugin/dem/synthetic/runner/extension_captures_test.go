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

const captureInit = `{"type":"extension_init","extension_init":{"selected":["failure_evidence.v1","capture_details.v1","browser_summary.v1"]}}`

func captureTest(id string, accepted, unaccepted int64) map[string]any {
	return map[string]any{"record": "test", "test": synthetic.CaptureTest{ArtifactTest: synthetic.ArtifactTest{ID: id, Title: "Checkout"}, Status: "failed", ExpectedStatus: "passed", Accepted: accepted, Unaccepted: unaccepted}}
}

func captureLink(id, testID string) map[string]any {
	return map[string]any{"record": "capture", "id": id, "test_id": testID}
}

func consumeCaptures(t *testing.T, lines ...string) sinkSnapshot {
	t.Helper()
	result := success()
	result.CaptureState = "not_needed"
	request := synthetic.Request{Kind: synthetic.Journey, Capture: true, BrowserErrorSummary: true}
	return consumeWithResult(t, request, map[string]string{"token": "synthetic-secret"}, result, lines...)
}

func TestCaptureReportingNegotiationAndIsolation(t *testing.T) {
	for name, tc := range map[string]struct {
		lines  []string
		status string
	}{
		"old runtime":    {nil, "negotiation_not_reported"},
		"declined":       {[]string{`{"type":"extension_init","extension_init":{"selected":[]}}`}, "unsupported"},
		"selected":       {[]string{captureInit}, "partial"},
		"complete":       {[]string{captureInit, extensionLine(captureDetailsFeature, map[string]any{"record": "complete"})}, "complete"},
		"unknown record": {[]string{captureInit, extensionLine(captureDetailsFeature, map[string]any{"record": "future"})}, "invalid"},
		"null body":      {[]string{captureInit, extensionLine(captureDetailsFeature, nil)}, "invalid"},
		"wrong type":     {[]string{captureInit, extensionLine(captureDetailsFeature, map[string]any{"record": "test", "test": true})}, "invalid"},
		"before init":    {[]string{extensionLine(captureDetailsFeature, map[string]any{"record": "complete"}), captureInit}, "invalid"},
		"late record":    {[]string{captureInit, extensionLine(captureDetailsFeature, map[string]any{"record": "complete"}), extensionLine(captureDetailsFeature, captureTest("late", 0, 0))}, "invalid"},
		"additive field": {[]string{captureInit, extensionLine(captureDetailsFeature, map[string]any{"record": "complete", "future": 42})}, "complete"},
	} {
		t.Run(name, func(t *testing.T) {
			s := consumeCaptures(t, tc.lines...)
			require.NotNil(t, s.captures)
			assert.Equal(t, tc.status, s.captures.Reporting)
			assert.NotEqual(t, "invalid", s.evidence.ReportingStatus())
			assert.NotEqual(t, "invalid", s.browser.ReportingStatus())
		})
	}
}

func TestCaptureAssociationRetainsMixedOmissionsAndRedacts(t *testing.T) {
	test := captureTest("t1", 2, 1)
	value := test["test"].(synthetic.CaptureTest)
	value.Title = "synthetic-secret " + strings.Repeat("x", 220)
	test["test"] = value
	s := consumeCaptures(t, captureInit, extensionLine(captureDetailsFeature, test), extensionLine(captureDetailsFeature, captureLink("s1", "t1")), extensionLine(captureDetailsFeature, captureLink("s2", "t1")), extensionLine(captureDetailsFeature, map[string]any{"record": "complete"}))
	require.Len(t, s.captures.Tests, 1)
	require.Len(t, s.captures.Associations, 2)
	assert.Equal(t, int64(1), s.captures.Tests[0].Unaccepted)
	assert.Contains(t, s.captures.Tests[0].Title, "[REDACTED]")
	assert.NotContains(t, s.captures.Tests[0].Title, "synthetic-secret")
	run := synthetic.Run{CaptureState: "available", Captures: s.captures}
	candidates := []synthetic.Capture{{ID: "s1"}}
	joinCaptures(&run, candidates, []synthetic.Capture{{ID: "s2"}})
	assert.Equal(t, "t1", candidates[0].Test.ID)
	assert.Equal(t, []synthetic.CaptureOmission{{ID: "s2", Reason: "not_eligible"}}, run.Captures.Unpublished)
	// Snapshot cloning is independent, including test/link/disposition slices.
	originalTitle := run.Captures.Tests[0].Title
	cloned := synthetic.CloneRun(run)
	run.Captures.Tests[0].Title = "mutated"
	run.Captures.Associations[0].ID = "changed"
	run.Captures.Unpublished[0].Reason = "changed"
	assert.Equal(t, originalTitle, cloned.Captures.Tests[0].Title)
	assert.Equal(t, "s1", cloned.Captures.Associations[0].ID)
	assert.Equal(t, "not_eligible", cloned.Captures.Unpublished[0].Reason)
}

func TestCaptureConflictingIdentityIsNotRetained(t *testing.T) {
	for _, conflict := range []map[string]any{captureLink("s1", "t2"), captureTest("t1", 1, 0)} {
		s := consumeCaptures(t, captureInit, extensionLine(captureDetailsFeature, captureTest("t1", 1, 0)), extensionLine(captureDetailsFeature, captureTest("t2", 0, 0)), extensionLine(captureDetailsFeature, captureLink("s1", "t1")), extensionLine(captureDetailsFeature, conflict))
		assert.Equal(t, "invalid", s.captures.Reporting)
		assert.Empty(t, s.captures.Associations)
		candidates := []synthetic.Capture{{ID: "s1"}}
		joinCaptures(&synthetic.Run{CaptureState: "available", Captures: s.captures}, candidates, nil)
		assert.Empty(t, candidates[0].Test.ID)
	}
}

func TestCaptureDetailLimitNeverLimitsCandidateAuthority(t *testing.T) {
	lines := []string{captureInit}
	var candidates []synthetic.Capture
	for i := 0; i < 150; i++ {
		id := fmt.Sprintf("t%d", i)
		capture := fmt.Sprintf("s%d", i)
		lines = append(lines, extensionLine(captureDetailsFeature, captureTest(id, 1, 0)), extensionLine(captureDetailsFeature, captureLink(capture, id)))
		candidates = append(candidates, synthetic.Capture{ID: capture})
	}
	lines = append(lines, extensionLine(captureDetailsFeature, map[string]any{"record": "complete"}))
	s := consumeCaptures(t, lines...)
	assert.Equal(t, "complete", s.captures.Reporting)
	assert.True(t, s.captures.Truncated)
	assert.Equal(t, captureDetailLimit, len(s.captures.Tests)+len(s.captures.Associations))
	joinCaptures(&synthetic.Run{CaptureState: "available", Captures: s.captures}, candidates, nil)
	assert.Len(t, candidates, 150)
	assert.Equal(t, "t0", candidates[0].Test.ID)
	assert.Empty(t, candidates[149].Test.ID)
}

func TestCaptureMetadataCannotWidenCoreWire(t *testing.T) {
	s := newSink(synthetic.Request{Kind: synthetic.Journey, Capture: true}, newRedactor(nil))
	err := s.consume(strings.NewReader(`{"type":"result","result":{"status":"failed","capture_state":"available","artifacts":[{"id":"s1","kind":"screenshot","path":"output/x.png","mime":"image/png","test":{"id":"t1"}}]}}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown field "test"`)
}

func TestCaptureCountsMustBeObservedRatherThanDefaulted(t *testing.T) {
	for _, counts := range []string{``, `,"accepted":0`, `,"unaccepted":0`, `,"accepted":null,"unaccepted":0`, `,"accepted":0,"unaccepted":null`} {
		line := `{"type":"extension","extension":{"feature":"capture_details.v1","data":{"record":"test","test":{"id":"t1","title":"Checkout","status":"failed","expected_status":"passed"` + counts + `}}}}`
		s := consumeCaptures(t, captureInit, line)
		assert.Equal(t, "invalid", s.captures.Reporting)
		assert.Empty(t, s.captures.Tests, "missing counts are not observations of zero")
	}
}

func TestTraceSupportReporting(t *testing.T) {
	selected := `{"type":"extension_init","extension_init":{"selected":["failure_evidence.v1","browser_summary.v1","capture_details.v1","failure_trace.v1"]}}`
	for name, tc := range map[string]struct {
		trace bool
		lines []string
		want  string
	}{
		"disabled":                {},
		"old runtime":             {trace: true, want: "negotiation_not_reported"},
		"old selection":           {trace: true, lines: []string{captureInit}, want: "unsupported"},
		"selected":                {trace: true, lines: []string{selected}, want: "supported"},
		"invalid init":            {trace: true, lines: []string{`{"type":"extension_init","extension_init":{"selected":null}}`}, want: "invalid"},
		"duplicate selection":     {trace: true, lines: []string{`{"type":"extension_init","extension_init":{"selected":["failure_trace.v1","failure_trace.v1"]}}`}, want: "invalid"},
		"fifth duplicate":         {trace: true, lines: []string{strings.Replace(selected, `"failure_trace.v1"]`, `"failure_trace.v1","failure_trace.v1"]`, 1)}, want: "invalid"},
		"fifth unknown":           {trace: true, lines: []string{strings.Replace(selected, `"failure_trace.v1"]`, `"failure_trace.v1","future.v1"]`, 1)}, want: "invalid"},
		"duplicate init":          {trace: true, lines: []string{selected, selected}, want: "invalid"},
		"invalid envelope":        {trace: true, lines: []string{selected, `{"type":"extension","extension":{"data":{}}}`}, want: "invalid"},
		"invalid capture details": {trace: true, lines: []string{selected, extensionLine(captureDetailsFeature, map[string]any{"record": "future"})}, want: "supported"},
		"unrequested selection":   {lines: []string{selected}},
	} {
		t.Run(name, func(t *testing.T) {
			s := newSink(synthetic.Request{Kind: synthetic.Journey, Capture: true, BrowserErrorSummary: true, Trace: tc.trace}, newRedactor(nil))
			r := success()
			r.CaptureState = "not_needed"
			terminal, err := json.Marshal(frame{Type: "result", Result: &r})
			require.NoError(t, err)
			require.NoError(t, s.consume(strings.NewReader(strings.Join(append(tc.lines, string(terminal)), "\n"))))
			snap := s.snapshot()
			require.NotNil(t, snap.captures)
			assert.Equal(t, tc.want, snap.captures.Trace)
			if !tc.trace {
				raw, err := json.Marshal(snap.captures)
				require.NoError(t, err)
				assert.NotContains(t, string(raw), `"trace"`)
			}
			if name == "invalid capture details" {
				assert.Equal(t, "invalid", snap.captures.Reporting)
			}
			if name == "unrequested selection" {
				assert.Equal(t, "invalid", snap.captures.Reporting)
			}
		})
	}
}

func TestTraceCandidatesUseRequestAuthority(t *testing.T) {
	for _, trace := range []bool{false, true} {
		for _, interrupted := range []bool{false, true} {
			for name, init := range map[string]string{
				"old peer":  "",
				"selected":  `{"type":"extension_init","extension_init":{"selected":["failure_trace.v1"]}}`,
				"declined":  `{"type":"extension_init","extension_init":{"selected":[]}}`,
				"malformed": `{"type":"extension_init","extension_init":{"selected":null}}`,
			} {
				t.Run(fmt.Sprintf("trace=%t/interrupted=%t/%s", trace, interrupted, name), func(t *testing.T) {
					candidate := synthetic.Capture{ID: "trace1", Kind: "trace", MIME: "application/zip", Path: "output/trace.zip"}
					r := reporterResult{Status: synthetic.Failed, Tests: &synthetic.TestCounts{Declared: 1, Failed: 1}, CaptureState: "available", Artifacts: []synthetic.Capture{candidate}}
					if interrupted {
						r.Status = synthetic.Timeout
						r.TerminationReason = synthetic.ExecutionBudget
						r.Artifacts = nil
						r.InterruptedArtifacts = []synthetic.Capture{candidate}
					}
					raw, err := json.Marshal(frame{Type: "result", Result: &r})
					require.NoError(t, err)
					s := newSink(synthetic.Request{Kind: synthetic.Journey, Capture: true, Trace: trace}, newRedactor(nil))
					err = s.consume(strings.NewReader(strings.TrimSpace(init + "\n" + string(raw))))
					if trace {
						require.NoError(t, err)
						require.NotNil(t, s.snapshot().result)
					} else {
						require.Error(t, err)
						assert.Contains(t, err.Error(), "unsupported")
					}
				})
			}
		}
	}
}

// TestCaptureWireBounds accepts each capture_details.v1 field budget at its
// limit and invalidates the feature one past it. Records past the record
// budget are truncated instead; the feature stays valid.
func TestCaptureWireBounds(t *testing.T) {
	text := func(n int) string { return strings.Repeat("x", n-2) }
	withTitle := func(n int) map[string]any {
		record := captureTest("t1", 1, 0)
		value := record["test"].(synthetic.CaptureTest)
		value.Title = text(n)
		record["test"] = value
		return record
	}
	for name, tc := range map[string]struct {
		lines []string
		want  string
	}{
		"test ID at limit":   {[]string{captureInit, extensionLine(captureDetailsFeature, captureTest(text(128), 1, 0))}, "partial"},
		"test ID past limit": {[]string{captureInit, extensionLine(captureDetailsFeature, captureTest(text(129), 1, 0))}, "invalid"},
		"title at limit":     {[]string{captureInit, extensionLine(captureDetailsFeature, withTitle(256))}, "partial"},
		"title past limit":   {[]string{captureInit, extensionLine(captureDetailsFeature, withTitle(257))}, "invalid"},
		"link test ID limit": {[]string{captureInit, extensionLine(captureDetailsFeature, captureTest(text(128), 1, 0)), extensionLine(captureDetailsFeature, captureLink("s1", text(128)))}, "partial"},
		"counts at safe max": {[]string{captureInit, extensionLine(captureDetailsFeature, captureTest("t1", maxSafeInteger, maxSafeInteger))}, "partial"},
		"counts past safe":   {[]string{captureInit, extensionLine(captureDetailsFeature, captureTest("t1", maxSafeInteger+1, 0))}, "invalid"},
		"records at limit":   {captureRecords(captureDetailLimit), "partial"},
		"records past limit": {captureRecords(captureDetailLimit + 1), "partial"},
	} {
		t.Run(name, func(t *testing.T) {
			s := consumeCaptures(t, tc.lines...)
			assert.Equal(t, tc.want, s.captures.Reporting)
		})
	}
	s := consumeCaptures(t, captureRecords(captureDetailLimit)...)
	assert.False(t, s.captures.Truncated)
	assert.Len(t, s.captures.Tests, captureDetailLimit)
	s = consumeCaptures(t, captureRecords(captureDetailLimit+1)...)
	assert.True(t, s.captures.Truncated, "detail past the record budget is truncated, not invalid")
	assert.Len(t, s.captures.Tests, captureDetailLimit)
}

func captureRecords(n int) []string {
	lines := []string{captureInit}
	for i := range n {
		lines = append(lines, extensionLine(captureDetailsFeature, captureTest(fmt.Sprintf("t%d", i), 1, 0)))
	}
	return lines
}
