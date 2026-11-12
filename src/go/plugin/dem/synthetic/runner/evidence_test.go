// SPDX-License-Identifier: GPL-3.0-or-later
package runner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const evidenceInit = `{"type":"extension_init","extension_init":{"selected":["failure_evidence.v1"]}}`

func evidenceRecord(record string, key string, value any) string {
	data := map[string]any{"record": record}
	if key != "" {
		data[key] = value
	}
	raw, err := json.Marshal(map[string]any{"type": "extension", "extension": map[string]any{"feature": failureEvidenceFeature, "data": data}})
	if err != nil {
		panic(err)
	}
	return string(raw)
}
func evidenceStart(id string) synthetic.EvidenceTest {
	return synthetic.EvidenceTest{ID: id, Title: "Checkout", Status: "running", ExpectedStatus: "passed", Location: &synthetic.EvidenceLocation{File: "journey.spec.ts", Line: 2, Column: 1}}
}
func evidenceStep(id, parent string) synthetic.EvidenceStep {
	return synthetic.EvidenceStep{ID: id, TestID: "t1", ParentID: parent, Title: "Submit order", Category: "test.step", State: "running", Location: &synthetic.EvidenceLocation{File: "journey.spec.ts", Line: 3, Column: 2}}
}
func consumeEvidence(t *testing.T, lines ...string) (*synthetic.FailureEvidence, sinkSnapshot) {
	t.Helper()
	terminal, err := json.Marshal(frame{Type: "result", Result: pointerResult(success())})
	require.NoError(t, err)
	lines = append(lines, string(terminal))
	s := newSink(synthetic.Journey, false, newRedactor(nil))
	require.NoError(t, s.consume(strings.NewReader(strings.Join(lines, "\n"))))
	snap := s.snapshot()
	require.NotNil(t, snap.result)
	require.Equal(t, synthetic.Success, snap.result.Status, "optional diagnostics must not rewrite core truth")
	require.NoError(t, snap.err)
	return snap.evidence, snap
}
func pointerResult(r reporterResult) *reporterResult { return &r }

func TestEvidenceNegotiationAndOptionalErrors(t *testing.T) {
	cases := map[string]struct {
		lines  []string
		status string
	}{
		"old runtime":             {nil, "unavailable"},
		"declined":                {[]string{`{"type":"extension_init","extension_init":{"selected":[]}}`}, "unsupported"},
		"accepted":                {[]string{evidenceInit}, "partial"},
		"complete":                {[]string{evidenceInit, evidenceRecord("complete", "", nil)}, "complete"},
		"missing init body":       {[]string{`{"type":"extension_init"}`}, "invalid"},
		"null init":               {[]string{`{"type":"extension_init","extension_init":null}`}, "invalid"},
		"scalar init":             {[]string{`{"type":"extension_init","extension_init":3}`}, "invalid"},
		"missing selected":        {[]string{`{"type":"extension_init","extension_init":{}}`}, "invalid"},
		"unrequested":             {[]string{`{"type":"extension_init","extension_init":{"selected":["unknown.v1"]}}`}, "invalid"},
		"duplicate":               {[]string{evidenceInit, evidenceInit}, "invalid"},
		"late":                    {[]string{`{"type":"event","event":{"kind":"suite","at_ms":0}}`, evidenceInit}, "invalid"},
		"missing extension body":  {[]string{evidenceInit, `{"type":"extension"}`}, "invalid"},
		"null extension body":     {[]string{evidenceInit, `{"type":"extension","extension":null}`}, "invalid"},
		"scalar data":             {[]string{evidenceInit, `{"type":"extension","extension":{"feature":"failure_evidence.v1","data":4}}`}, "invalid"},
		"unknown variant":         {[]string{evidenceInit, evidenceRecord("future", "", nil)}, "invalid"},
		"after completion":        {[]string{evidenceInit, evidenceRecord("complete", "", nil), evidenceRecord("complete", "", nil)}, "invalid"},
		"unknown feature":         {[]string{evidenceInit, `{"type":"extension","extension":{"feature":"future.v1","data":{"anything":true}}}`}, "partial"},
		"unselected feature":      {[]string{`{"type":"extension_init","extension_init":{"selected":[]}}`, evidenceRecord("complete", "", nil)}, "unsupported"},
		"unknown init member":     {[]string{`{"type":"extension_init","extension_init":{"selected":["failure_evidence.v1"],"future":true}}`}, "invalid"},
		"unknown envelope member": {[]string{evidenceInit, `{"type":"extension","extension":{"feature":"failure_evidence.v1","future":true,"data":{"record":"complete"}}}`}, "invalid"},
		"data before init":        {[]string{evidenceRecord("complete", "", nil), evidenceInit}, "invalid"},
		"additive fields":         {[]string{evidenceInit, `{"type":"extension","extension":{"feature":"failure_evidence.v1","data":{"record":"complete","future":{"private":"discard"}}}}`}, "complete"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e, _ := consumeEvidence(t, tc.lines...)
			require.Equal(t, tc.status, e.ReportingStatus())
		})
	}
}

func TestEvidenceKeepsSharedAndCoreFramesStrict(t *testing.T) {
	cases := map[string]string{
		"unknown outer":            `{"type":"extension","surprise":1}`,
		"mixed null":               `{"type":"extension","event":null}`,
		"mixed extension families": `{"type":"extension","extension_init":null}`,
		"mixed core":               `{"type":"event","event":{"kind":"suite"},"extension":null}`,
		"unknown type":             `{"type":"future"}`,
		"invalid JSON":             `{"type":"extension",`,
		"two values":               `{"type":"extension"} {}`,
		"core unknown key":         `{"type":"event","event":{"kind":"suite","new":true}}`,
		"core bad enum":            `{"type":"event","event":{"kind":"future"}}`,
		"oversize":                 `{"type":"extension","extension":{"data":"` + strings.Repeat("x", FrameMaxBytes) + `"}}`,
	}
	for name, line := range cases {
		t.Run(name, func(t *testing.T) {
			s := newSink(synthetic.Journey, false, newRedactor(nil))
			require.Error(t, s.consume(strings.NewReader(evidenceInit+"\n"+line)))
		})
	}
	s := newSink(synthetic.Journey, false, newRedactor(nil))
	raw, _ := json.Marshal(frame{Type: "result", Result: pointerResult(success())})
	require.ErrorContains(t, s.consume(strings.NewReader(string(raw)+"\n"+evidenceInit)), "after its terminal")
}

func TestEvidenceUpsertsAndPreservesFirstFailureAcrossLaterInterruption(t *testing.T) {
	test := evidenceStart("t1")
	step := evidenceStep("s1", "")
	end := test
	end.Status = "failed"
	end.ReportingComplete = true
	stepEnd := step
	stepEnd.State = "failed"
	stepEnd.Message = "expected checkout confirmation"
	path := stepEnd
	path.Message = ""
	failure := synthetic.FailureDetail{Test: end, Path: []synthetic.EvidenceStep{path}, Message: stepEnd.Message}
	second := evidenceStart("t2")
	second.Title = "Login"
	pending := evidenceStep("s2", "")
	pending.TestID = "t2"
	pending.Title = "Waiting for login"
	e, _ := consumeEvidence(t, evidenceInit, evidenceRecord("test", "test", test), evidenceRecord("step", "step", step), evidenceRecord("step", "step", stepEnd), evidenceRecord("test", "test", end), evidenceRecord("failure", "failure", failure), evidenceRecord("test", "test", second), evidenceRecord("step", "step", pending))
	require.Equal(t, "partial", e.ReportingStatus())
	require.Len(t, e.Tests, 2)
	require.Len(t, e.Steps, 2)
	assert.True(t, e.Tests[0].ReportingComplete)
	assert.False(t, e.Tests[1].ReportingComplete)
	require.NotNil(t, e.FirstFailure)
	assert.Equal(t, "t1", e.FirstFailure.Test.ID)
	assert.Contains(t, e.LastObservedStepSummary(), "Waiting for login")
}

func TestEvidenceCapsKeepLifecycleUpdatesAndLateFailure(t *testing.T) {
	lines := []string{evidenceInit}
	for i := 0; i < evidenceTestLimit+2; i++ {
		test := evidenceStart(fmt.Sprintf("t%d", i))
		lines = append(lines, evidenceRecord("test", "test", test))
		test.Status = "passed"
		test.ReportingComplete = true
		lines = append(lines, evidenceRecord("test", "test", test))
	}
	for i := 0; i < evidenceUserStepLimit+2; i++ {
		step := evidenceStep(fmt.Sprintf("s%d", i), "")
		lines = append(lines, evidenceRecord("step", "step", step))
		step.State = "passed"
		lines = append(lines, evidenceRecord("step", "step", step))
	}
	test := evidenceStart("late")
	test.Status = "failed"
	test.ReportingComplete = true
	lines = append(lines, evidenceRecord("failure", "failure", synthetic.FailureDetail{Test: test, Path: []synthetic.EvidenceStep{}, Message: "late assertion"}), evidenceRecord("complete", "", nil))
	e, _ := consumeEvidence(t, lines...)
	require.Equal(t, "complete", e.ReportingStatus())
	assert.ElementsMatch(t, []string{"tests", "user_steps"}, e.Truncated)
	require.Len(t, e.Tests, evidenceTestLimit)
	require.Len(t, e.Steps, evidenceUserStepLimit)
	for _, test := range e.Tests {
		assert.True(t, test.ReportingComplete)
	}
	for _, step := range e.Steps {
		assert.Equal(t, "passed", step.State)
	}
	require.NotNil(t, e.FirstFailure)
	assert.Equal(t, "late", e.FirstFailure.Test.ID)
}

func TestEvidenceInvalidUpdateDiscardsOnlyDisputedItem(t *testing.T) {
	first := evidenceStart("t1")
	second := evidenceStart("t2")
	bad := second
	bad.Title = "different identity"
	bad.Status = "passed"
	bad.ReportingComplete = true
	e, _ := consumeEvidence(t, evidenceInit, evidenceRecord("test", "test", first), evidenceRecord("test", "test", second), evidenceRecord("test", "test", bad))
	require.Equal(t, "invalid", e.ReportingStatus())
	require.Len(t, e.Tests, 1)
	assert.Equal(t, "t1", e.Tests[0].ID)
}

func TestEvidenceExpectedStatusIsObservedAtEnd(t *testing.T) {
	start := evidenceStart("t1")
	end := start
	end.Status = "failed"
	end.ExpectedStatus = "failed"
	end.ReportingComplete = true
	e, _ := consumeEvidence(t, evidenceInit, evidenceRecord("test", "test", start), evidenceRecord("test", "test", end), evidenceRecord("complete", "", nil))
	require.Equal(t, "complete", e.ReportingStatus())
	assert.Equal(t, "failed", e.Tests[0].ExpectedStatus)
	e, _ = consumeEvidence(t, evidenceInit, evidenceRecord("failure", "failure", synthetic.FailureDetail{Test: end}))
	assert.Equal(t, "invalid", e.ReportingStatus(), "expected failure cannot be presented as unexpected")
}

func TestEvidenceRedactsBeforePersistenceAndSnapshotsOwnData(t *testing.T) {
	test := evidenceStart("t1")
	test.Title = "private-value"
	test.Location.File = "private-value.ts"
	test.TextTruncated = true
	step := evidenceStep("s1", "")
	step.Title = "private-value"
	step.Message = "token=private-value"
	step.TextTruncated = true
	s := newSink(synthetic.Journey, false, newRedactor(map[string]string{"DEM_SECRET_TOKEN": "private-value"}))
	require.NoError(t, s.consume(strings.NewReader(strings.Join([]string{evidenceInit, evidenceRecord("test", "test", test), evidenceRecord("step", "step", step)}, "\n"))))
	snapshot := s.snapshot()
	raw, err := json.Marshal(snapshot.evidence)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "private-value")
	snapshot.evidence.Tests[0].Location.File = "mutated"
	assert.NotEqual(t, "mutated", s.snapshot().evidence.Tests[0].Location.File)
}

func TestEvidencePayloadAndSourceBoundsAreNonfatal(t *testing.T) {
	for name, change := range map[string]func(*synthetic.EvidenceTest){
		"title encoded bytes": func(test *synthetic.EvidenceTest) { test.Title = strings.Repeat("\x00", 100) },
		"absolute source":     func(test *synthetic.EvidenceTest) { test.Location.File = "/etc/private.ts" },
		"traversal":           func(test *synthetic.EvidenceTest) { test.Location.File = "../private.ts" },
		"invalid time":        func(test *synthetic.EvidenceTest) { test.DurationMS = new(float64); *test.DurationMS = -1 },
	} {
		t.Run(name, func(t *testing.T) {
			test := evidenceStart("t1")
			change(&test)
			e, _ := consumeEvidence(t, evidenceInit, evidenceRecord("test", "test", test))
			assert.Equal(t, "invalid", e.ReportingStatus())
		})
	}
	e, _ := consumeEvidence(t, evidenceInit, `{"type":"extension","extension":{"feature":"failure_evidence.v1","data":{"record":"complete","large":"`+strings.Repeat("x", evidencePayloadLimit)+`"}}}`)
	assert.Equal(t, "invalid", e.ReportingStatus())
}

// Baseline and extended streams use the same core parser. Benchmark separately
// so optional-field overhead and requested evidence work remain attributable.
func BenchmarkFailureEvidenceStream(b *testing.B) {
	var baseline bytes.Buffer
	for i := 0; i < 30; i++ {
		emit(&baseline, frame{Type: "event", Event: &synthetic.Event{Kind: "step", AtMS: 1, Title: "normal step", Status: "passed"}})
	}
	emit(&baseline, frame{Type: "result", Result: pointerResult(success())})
	for name, input := range map[string]string{"baseline": baseline.String(), "extended": evidenceInit + "\n" + evidenceRecord("test", "test", evidenceStart("t1")) + "\n" + evidenceRecord("step", "step", evidenceStep("s1", "")) + "\n" + baseline.String()} {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				s := newSink(synthetic.Journey, false, newRedactor(nil))
				if err := s.consume(strings.NewReader(input)); err != nil {
					b.Fatal(err)
				}
				_ = s.snapshot()
			}
		})
	}
}

func TestEvidenceRedactionExpansionDisclosesAllClipping(t *testing.T) {
	test := evidenceStart("t1")
	test.Status, test.ReportingComplete = "failed", true
	failure := synthetic.FailureDetail{Test: test, Message: strings.Repeat("token=a ", 200) + strings.Repeat("z", 400)}
	s := newSink(synthetic.Journey, false, newRedactor(nil))
	require.NoError(t, s.consume(strings.NewReader(evidenceInit+"\n"+evidenceRecord("failure", "failure", failure))))
	got := s.snapshot().evidence.FirstFailure
	require.NotNil(t, got)
	assert.True(t, got.TextTruncated, "pattern redaction expanded the message past its2000-character budget")
	assert.Contains(t, s.snapshot().evidence.Truncated, "text")
	assert.NotContains(t, got.Message, "token=a")
}
