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

func captureRecord(data any) string {
	raw, err := json.Marshal(map[string]any{"type": "extension", "extension": map[string]any{"feature": captureDetailsFeature, "data": data}})
	if err != nil {
		panic(err)
	}
	return string(raw)
}
func captureTest(id string, accepted, unaccepted int64) map[string]any {
	return map[string]any{"record": "test", "test": synthetic.CaptureTest{ArtifactTest: synthetic.ArtifactTest{ID: id, Title: "Checkout"}, Status: "failed", ExpectedStatus: "passed", Accepted: accepted, Unaccepted: unaccepted}}
}
func captureLink(id, testID string) map[string]any {
	return map[string]any{"record": "capture", "id": id, "test_id": testID}
}

func consumeCaptures(t *testing.T, lines ...string) sinkSnapshot {
	t.Helper()
	r := success()
	r.CaptureState = "not_needed"
	terminal, err := json.Marshal(frame{Type: "result", Result: &r})
	require.NoError(t, err)
	s := newSink(synthetic.Journey, true, newRedactor(map[string]string{"token": "synthetic-secret"}), true)
	require.NoError(t, s.consume(strings.NewReader(strings.Join(append(lines, string(terminal)), "\n"))))
	snap := s.snapshot()
	require.NoError(t, snap.err)
	require.NotNil(t, snap.result)
	assert.Equal(t, synthetic.Success, snap.result.Status)
	return snap
}

func TestCaptureReportingNegotiationAndIsolation(t *testing.T) {
	for name, tc := range map[string]struct {
		lines  []string
		status string
	}{
		"old runtime":    {nil, "negotiation_not_reported"},
		"declined":       {[]string{`{"type":"extension_init","extension_init":{"selected":[]}}`}, "unsupported"},
		"selected":       {[]string{captureInit}, "partial"},
		"complete":       {[]string{captureInit, captureRecord(map[string]any{"record": "complete"})}, "complete"},
		"unknown record": {[]string{captureInit, captureRecord(map[string]any{"record": "future"})}, "invalid"},
		"null body":      {[]string{captureInit, captureRecord(nil)}, "invalid"},
		"wrong type":     {[]string{captureInit, captureRecord(map[string]any{"record": "test", "test": true})}, "invalid"},
		"before init":    {[]string{captureRecord(map[string]any{"record": "complete"}), captureInit}, "invalid"},
		"late record":    {[]string{captureInit, captureRecord(map[string]any{"record": "complete"}), captureRecord(captureTest("late", 0, 0))}, "invalid"},
		"additive field": {[]string{captureInit, captureRecord(map[string]any{"record": "complete", "future": 42})}, "complete"},
	} {
		t.Run(name, func(t *testing.T) {
			s := consumeCaptures(t, tc.lines...)
			require.NotNil(t, s.captures)
			assert.Equal(t, tc.status, s.captures.Reporting)
			assert.NotEqual(t, "invalid", s.evidence.Availability)
			assert.NotEqual(t, "invalid", s.browser.Availability)
		})
	}
}

func TestCaptureAssociationRetainsMixedOmissionsAndRedacts(t *testing.T) {
	test := captureTest("t1", 2, 1)
	value := test["test"].(synthetic.CaptureTest)
	value.Title = "synthetic-secret " + strings.Repeat("x", 220)
	test["test"] = value
	s := consumeCaptures(t, captureInit, captureRecord(test), captureRecord(captureLink("s1", "t1")), captureRecord(captureLink("s2", "t1")), captureRecord(map[string]any{"record": "complete"}))
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
		s := consumeCaptures(t, captureInit, captureRecord(captureTest("t1", 1, 0)), captureRecord(captureTest("t2", 0, 0)), captureRecord(captureLink("s1", "t1")), captureRecord(conflict))
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
		lines = append(lines, captureRecord(captureTest(id, 1, 0)), captureRecord(captureLink(capture, id)))
		candidates = append(candidates, synthetic.Capture{ID: capture})
	}
	lines = append(lines, captureRecord(map[string]any{"record": "complete"}))
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
	s := newSink(synthetic.Journey, true, newRedactor(nil), false)
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
