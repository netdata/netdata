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

const evidenceInit = `{"type":"extension_init","extension_init":{"selected":["failure_evidence.v1"]}}`

func evidenceRecord(record string, key string, value any) string {
	data := map[string]any{"record": record}
	if key != "" {
		data[key] = value
	}
	return extensionLine(failureEvidenceFeature, data)
}

func evidenceStart(id string) synthetic.EvidenceTest {
	return synthetic.EvidenceTest{ID: id, Title: "Checkout", Status: "running", ExpectedStatus: "passed", Location: &synthetic.EvidenceLocation{File: "journey.spec.ts", Line: 2, Column: 1}}
}

func evidenceStep(id, parent string) synthetic.EvidenceStep {
	return synthetic.EvidenceStep{ID: id, TestID: "t1", ParentID: parent, Title: "Submit order", Category: "test.step", State: "running", Location: &synthetic.EvidenceLocation{File: "journey.spec.ts", Line: 3, Column: 2}}
}

func consumeEvidence(t *testing.T, lines ...string) (*synthetic.FailureEvidence, sinkSnapshot) {
	t.Helper()
	snapshot := consumeWithResult(t, synthetic.Request{Kind: synthetic.Journey}, nil, success(), lines...)
	return snapshot.evidence, snapshot
}

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
		"oversize":                 `{"type":"extension","extension":{"data":"` + strings.Repeat("x", frameMaxBytes) + `"}}`,
	}
	for name, line := range cases {
		t.Run(name, func(t *testing.T) {
			s := newSink(synthetic.Request{Kind: synthetic.Journey}, newRedactor(nil))
			require.Error(t, s.consume(strings.NewReader(evidenceInit+"\n"+line)))
		})
	}
	s := newSink(synthetic.Request{Kind: synthetic.Journey}, newRedactor(nil))
	require.ErrorContains(t, s.consume(strings.NewReader(resultLine(success())+"\n"+evidenceInit)), "after its terminal")
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
	assert.Equal(t, &synthetic.FailureEvidence{
		Availability: "supported",
		Tests:        []synthetic.EvidenceTest{end, second},
		Steps:        []synthetic.EvidenceStep{stepEnd, pending},
		FirstFailure: &failure,
	}, e)
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
	s := newSink(synthetic.Request{Kind: synthetic.Journey}, newRedactor(map[string]string{"DEM_SECRET_TOKEN": "private-value"}))
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
	e, _ := consumeEvidence(t, evidenceInit, `{"type":"extension","extension":{"feature":"failure_evidence.v1","data":{"record":"complete","large":"`+strings.Repeat("x", extensionBodyLimit)+`"}}}`)
	assert.Equal(t, "invalid", e.ReportingStatus())
}

func TestEvidenceRedactionExpansionDisclosesAllClipping(t *testing.T) {
	test := evidenceStart("t1")
	test.Status, test.ReportingComplete = "failed", true
	failure := synthetic.FailureDetail{Test: test, Message: strings.Repeat("token=a ", 200) + strings.Repeat("z", 400)}
	s := newSink(synthetic.Request{Kind: synthetic.Journey}, newRedactor(nil))
	require.NoError(t, s.consume(strings.NewReader(evidenceInit+"\n"+evidenceRecord("failure", "failure", failure))))
	got := s.snapshot().evidence.FirstFailure
	require.NotNil(t, got)
	assert.True(t, got.TextTruncated, "pattern redaction expanded the message past its2000-character budget")
	assert.Contains(t, s.snapshot().evidence.Truncated, "text")
	assert.NotContains(t, got.Message, "token=a")
}

func TestStopContextIsProtectedAndIndependentOfFirstFailure(t *testing.T) {
	failureTest := evidenceStart("earlier")
	failureTest.Status, failureTest.ReportingComplete = "failed", true
	failure := synthetic.FailureDetail{Test: failureTest, Message: "earlier assertion", Path: []synthetic.EvidenceStep{}}
	stoppedTest := evidenceStart("t1")
	stoppedTest.Status, stoppedTest.ReportingComplete = "interrupted", true
	stop := synthetic.FailureDetail{Test: stoppedTest, Path: []synthetic.EvidenceStep{evidenceStep("late", "")}}
	lines := []string{evidenceInit, evidenceRecord("failure", "failure", failure)}
	for n := 0; n < evidenceUserStepLimit; n++ {
		step := evidenceStep(fmt.Sprintf("s%d", n), "")
		lines = append(lines, evidenceRecord("step", "step", step))
	}
	lines = append(lines, evidenceRecord("stop", "stop", stop), evidenceRecord("complete", "", nil))
	e, _ := consumeEvidence(t, lines...)
	require.Equal(t, "complete", e.ReportingStatus())
	assert.Equal(t, &failure, e.FirstFailure)
	assert.Equal(t, &stop, e.StopContext)
	assert.Contains(t, e.StopSummary(), "Submit order")
	for name, mutate := range map[string]func(*synthetic.FailureDetail){
		"success":              func(f *synthetic.FailureDetail) { f.Test.Status = "passed" },
		"invented cause":       func(f *synthetic.FailureDetail) { f.Message = "cleanup failed" },
		"undisclosed absence":  func(f *synthetic.FailureDetail) { f.Path = nil },
		"complete passed leaf": func(f *synthetic.FailureDetail) { f.Path[0].State = "passed" },
		"complete failed leaf": func(f *synthetic.FailureDetail) { f.Path[0].State = "failed" },
	} {
		t.Run(name, func(t *testing.T) {
			bad := stop
			bad.Path = append([]synthetic.EvidenceStep(nil), stop.Path...)
			mutate(&bad)
			e, _ := consumeEvidence(t, evidenceInit, evidenceRecord("stop", "stop", bad))
			assert.Equal(t, "invalid", e.ReportingStatus())
			assert.Nil(t, e.StopContext)
		})
	}
	for _, state := range []string{"passed", "failed"} {
		incomplete := stop
		incomplete.Path = append([]synthetic.EvidenceStep(nil), stop.Path...)
		incomplete.Path[0].State = state
		incomplete.PathIncomplete = true
		e, _ := consumeEvidence(t, evidenceInit, evidenceRecord("stop", "stop", incomplete))
		assert.Equal(t, "partial", e.ReportingStatus())
		assert.Equal(t, &incomplete, e.StopContext, "a common ancestor need not still be running")
	}
	e, _ = consumeEvidence(t, evidenceInit, evidenceRecord("stop", "stop", stop), evidenceRecord("stop", "stop", stop))
	assert.Equal(t, "invalid", e.ReportingStatus())
	assert.Contains(t, e.StopSummary(), "[partial: evidence invalid]")
}

func TestStopContextRedactionAndSnapshotOwnership(t *testing.T) {
	test := evidenceStart("t1")
	test.Status, test.ReportingComplete = "timedOut", true
	step := evidenceStep("s", "")
	step.Title = "secret-value"
	stop := synthetic.FailureDetail{Test: test, Path: []synthetic.EvidenceStep{step}}
	sink := newSink(synthetic.Request{Kind: synthetic.Journey}, newRedactor(map[string]string{"DEM_SECRET_TOKEN": "secret-value"}))
	require.NoError(t, sink.consume(strings.NewReader(evidenceInit+"\n"+evidenceRecord("stop", "stop", stop))))
	snapshot := sink.snapshot()
	encoded, err := json.Marshal(snapshot.evidence)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "secret-value")
	snapshot.evidence.StopContext.Path[0].Location.File = "changed"
	assert.Equal(t, "journey.spec.ts", sink.snapshot().evidence.StopContext.Path[0].Location.File)
}

// TestEvidenceWireBounds accepts each failure_evidence.v1 budget at its limit
// and invalidates the feature one past it. Text budgets count encoded JSON
// bytes, quotes included.
func TestEvidenceWireBounds(t *testing.T) {
	text := func(n int) string { return strings.Repeat("x", n-2) }
	ended := func(test synthetic.EvidenceTest) synthetic.EvidenceTest {
		test.Status, test.ReportingComplete = "failed", true
		return test
	}
	chain := func(n int) []synthetic.EvidenceStep {
		path := make([]synthetic.EvidenceStep, n)
		for i := range path {
			path[i] = evidenceStep(fmt.Sprintf("p%d", i), "")
			if i > 0 {
				path[i].ParentID = path[i-1].ID
			}
		}
		return path
	}
	// The serialized-step budget binds before the field budgets; size the
	// message so the whole step encodes to exactly n bytes.
	sizedStep := func(n int) synthetic.EvidenceStep {
		step := evidenceStep(text(64), "")
		duration := float64(maxSafeInteger)
		step.TestID, step.ParentID, step.ParentOmitted = text(128), strings.Repeat("p", 62), true
		step.Title, step.Category, step.Location.File = text(256), text(64), text(256)
		step.Location.Line, step.Location.Column = maxSafeInteger, maxSafeInteger
		step.DurationMS, step.TextTruncated = &duration, true
		raw, err := json.Marshal(step)
		require.NoError(t, err)
		step.Message = strings.Repeat("m", n-len(raw)-len(`,"message":""`))
		return step
	}
	// Every field budget at once still encodes a test far below the 1024-byte
	// producer bound, so that bound never binds on its own.
	maximalTest := func() synthetic.EvidenceTest {
		test := evidenceStart(text(128))
		duration := float64(maxSafeInteger)
		test.Title, test.Location.File = text(256), text(256)
		test.Location.Line, test.Location.Column = maxSafeInteger, maxSafeInteger
		test.Status, test.ExpectedStatus, test.ReportingComplete = "skipped", "skipped", true
		test.DurationMS, test.TextTruncated = &duration, true
		return test
	}
	type testCase struct {
		record, key string
		value       any
		want        string
	}
	cases := map[string]testCase{
		"test ID at limit":         {"test", "test", func() synthetic.EvidenceTest { v := evidenceStart(text(128)); return v }(), "partial"},
		"test ID past limit":       {"test", "test", evidenceStart(text(129)), "invalid"},
		"title at limit":           {"test", "test", func() synthetic.EvidenceTest { v := evidenceStart("t1"); v.Title = text(256); return v }(), "partial"},
		"title past limit":         {"test", "test", func() synthetic.EvidenceTest { v := evidenceStart("t1"); v.Title = text(257); return v }(), "invalid"},
		"source at limit":          {"test", "test", func() synthetic.EvidenceTest { v := evidenceStart("t1"); v.Location.File = text(256); return v }(), "partial"},
		"source past limit":        {"test", "test", func() synthetic.EvidenceTest { v := evidenceStart("t1"); v.Location.File = text(257); return v }(), "invalid"},
		"test at all budgets":      {"test", "test", maximalTest(), "partial"},
		"step ID at limit":         {"step", "step", func() synthetic.EvidenceStep { v := evidenceStep(text(64), ""); return v }(), "partial"},
		"step ID past limit":       {"step", "step", evidenceStep(text(65), ""), "invalid"},
		"parent ID at limit":       {"step", "step", func() synthetic.EvidenceStep { v := evidenceStep("s1", text(64)); v.ParentOmitted = true; return v }(), "partial"},
		"parent ID past limit":     {"step", "step", func() synthetic.EvidenceStep { v := evidenceStep("s1", text(65)); v.ParentOmitted = true; return v }(), "invalid"},
		"category at limit":        {"step", "step", func() synthetic.EvidenceStep { v := evidenceStep("s1", ""); v.Category = text(64); return v }(), "partial"},
		"category past limit":      {"step", "step", func() synthetic.EvidenceStep { v := evidenceStep("s1", ""); v.Category = text(65); return v }(), "invalid"},
		"step message at limit":    {"step", "step", func() synthetic.EvidenceStep { v := evidenceStep("s1", ""); v.Message = text(1024); return v }(), "partial"},
		"step message past limit":  {"step", "step", func() synthetic.EvidenceStep { v := evidenceStep("s1", ""); v.Message = text(1025); return v }(), "invalid"},
		"step bytes at limit":      {"step", "step", sizedStep(2048), "partial"},
		"step bytes past limit":    {"step", "step", sizedStep(2049), "invalid"},
		"path at limit":            {"failure", "failure", synthetic.FailureDetail{Test: ended(evidenceStart("t1")), Path: chain(16)}, "partial"},
		"path past limit":          {"failure", "failure", synthetic.FailureDetail{Test: ended(evidenceStart("t1")), Path: chain(17)}, "invalid"},
		"message at rune limit":    {"failure", "failure", synthetic.FailureDetail{Test: ended(evidenceStart("t1")), Path: []synthetic.EvidenceStep{}, Message: strings.Repeat("é", textLimit)}, "partial"},
		"message past rune limit":  {"failure", "failure", synthetic.FailureDetail{Test: ended(evidenceStart("t1")), Path: []synthetic.EvidenceStep{}, Message: strings.Repeat("é", textLimit+1)}, "invalid"},
		"message at widest escape": {"failure", "failure", synthetic.FailureDetail{Test: ended(evidenceStart("t1")), Path: []synthetic.EvidenceStep{}, Message: strings.Repeat("\x01", textLimit)}, "partial"},
		"omitted ancestors safe":   {"failure", "failure", synthetic.FailureDetail{Test: ended(evidenceStart("t1")), Path: []synthetic.EvidenceStep{}, OmittedAncestors: maxSafeInteger}, "partial"},
		"omitted ancestors unsafe": {"failure", "failure", synthetic.FailureDetail{Test: ended(evidenceStart("t1")), Path: []synthetic.EvidenceStep{}, OmittedAncestors: maxSafeInteger + 1}, "invalid"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e, _ := consumeEvidence(t, evidenceInit, evidenceRecord(tc.record, tc.key, tc.value))
			assert.Equal(t, tc.want, e.ReportingStatus())
		})
	}
}

// TestEvidenceOtherStepPoolTruncates checks the non-user step pool, which is
// independent of the user-step pool covered above.
func TestEvidenceOtherStepPoolTruncates(t *testing.T) {
	lines := []string{evidenceInit}
	for i := range evidenceOtherStepLimit + 1 {
		step := evidenceStep(fmt.Sprintf("s%d", i), "")
		step.Category = "pw:api"
		lines = append(lines, evidenceRecord("step", "step", step))
	}
	e, _ := consumeEvidence(t, lines...)
	assert.Equal(t, "partial", e.ReportingStatus())
	assert.Equal(t, []string{"other_steps"}, e.Truncated)
	assert.Len(t, e.Steps, evidenceOtherStepLimit)
}

// TestExtensionBodyBound applies the shared 32 KiB body limit to the
// initialization and extension envelopes alike; exceeding it invalidates
// every optional feature without failing the attempt.
func TestExtensionBodyBound(t *testing.T) {
	sized := func(prefix, pad, suffix string, n int) string {
		return prefix + strings.Repeat(pad, n-len(prefix)-len(suffix)) + suffix
	}
	// Insignificant whitespace pads the frozen-key init body to an exact size.
	initBody := func(n int) string {
		return `{"type":"extension_init","extension_init":` + sized(`{"selected":["failure_evidence.v1","browser_summary.v1"]`, " ", `}`, n) + `}`
	}
	envelope := func(n int) string {
		return `{"type":"extension","extension":` + sized(`{"feature":"failure_evidence.v1","data":{"record":"complete","pad":"`, "x", `"}}`, n) + `}`
	}
	for name, tc := range map[string]struct {
		lines []string
		want  string
	}{
		"envelope at limit":   {[]string{browserInit, envelope(32 << 10)}, "complete"},
		"envelope past limit": {[]string{browserInit, envelope(32<<10 + 1)}, "invalid"},
		"init at limit":       {[]string{initBody(32 << 10)}, "partial"},
		"init past limit":     {[]string{initBody(32<<10 + 1)}, "invalid"},
	} {
		t.Run(name, func(t *testing.T) {
			s := consumeBrowser(t, tc.lines...)
			require.NoError(t, s.err)
			assert.Equal(t, tc.want, s.evidence.ReportingStatus())
			if tc.want == "invalid" {
				assert.Equal(t, "invalid", s.browser.ReportingStatus())
			}
		})
	}
}
