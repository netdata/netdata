// SPDX-License-Identifier: GPL-3.0-or-later
package synthetic

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// storedRunFixture sets every optional evidence shape a Go writer can record.
func storedRunFixture() Run {
	declared := 2
	duration := 12.5
	score := 87.0
	text := "console text"
	url := "https://example.com/path"
	location := &EvidenceLocation{File: "journey.spec.ts", Line: 4, Column: 7}
	test := EvidenceTest{ID: "t1", Title: "Checkout", Location: location, Status: "failed", ExpectedStatus: "passed", DurationMS: &duration, ReportingComplete: true, TextTruncated: true}
	step := EvidenceStep{ID: "s1", TestID: "t1", ParentOmitted: true, Title: "Submit", Category: "test.step", Location: location, State: "running"}
	return Run{
		Captures: &CaptureReport{
			Trace: "supported", Reporting: "complete", Truncated: true,
			Tests:        []CaptureTest{{ArtifactTest: ArtifactTest{ID: "t1", Title: "Checkout", TextTruncated: true}, Status: "failed", ExpectedStatus: "passed", Accepted: 1, Unaccepted: 2}},
			Associations: []CaptureAssociation{{ID: "shot-1", TestID: "t1"}},
			Unpublished:  []CaptureOmission{{ID: "shot-2", Reason: "too_large"}},
		},
		BrowserSummary: &BrowserSummary{
			Availability: "invalid", Reason: "record_after_completion", ReporterComplete: false, Coverage: "fixture_context",
			Run:   &BrowserAggregate{Tests: BrowserTestCounts{Observed: 2, NoSummary: 1}, Counts: BrowserCounts{Uncaught: 1, ConsoleError: 1, Transport: 1, HTTPError: 1}, Omissions: []string{"pending"}},
			Tests: []BrowserTest{{ID: "t1", State: "observed", Counts: &BrowserCounts{Uncaught: 1}, SampleIDs: []int{1, 2}}, {ID: "t2", State: "invalid", Reason: "invalid_annotation"}},
			Samples: []BrowserSample{
				{ID: 1, TestID: "t1", Category: "uncaught", Message: "boom", TextTruncated: true},
				{ID: 2, TestID: "t1", Category: "console_error", State: "resolved", Args: []BrowserArgument{{Value: nil}, {Value: "x"}, {Value: 1.5}, {Value: true}, {Omitted: "object"}}, ArgsOmitted: 3},
				{ID: 3, TestID: "t1", Category: "console_error", State: "resolved", Text: &text},
				{ID: 4, TestID: "t1", Category: "console_error", State: "unavailable", TextOmitted: "too_long"},
				{ID: 5, TestID: "t1", Category: "http_error", Method: "GET", URL: &url, Status: 503},
				{ID: 6, TestID: "t1", Category: "transport", Method: "POST", URLOmitted: "too_long", Error: "refused"},
			},
			Truncated: []string{"samples"},
		},
		FailureEvidence: &FailureEvidence{
			Availability: "supported", ReporterComplete: true, Truncated: []string{"text"},
			Tests: []EvidenceTest{test}, Steps: []EvidenceStep{step},
			FirstFailure: &FailureDetail{PathIncomplete: true, Test: test, Path: []EvidenceStep{step}, OmittedAncestors: 2, Message: "expected receipt", Location: location, TextTruncated: true},
			StopContext:  &FailureDetail{Test: test, Path: []EvidenceStep{}},
		},
		Runtime:     &RuntimeIdentity{Version: "1.0.0", SHA256: "abc", Contract: ExecutionContract},
		ID:          "0123456789abcdef0123456789abcdef",
		JobID:       "journey:checkout",
		Kind:        Journey,
		Name:        "checkout",
		VnodeGUID:   "11111111-2222-3333-4444-555555555555",
		Target:      "https://example.com",
		ScheduledUS: 1, QueuedUS: 2, StartedUS: 3, CompletedUS: 4,
		DurationMS:        &duration,
		Outcome:           Failed,
		TerminationReason: ExecutionBudget,
		Error:             "failed",
		Tests:             &TestCounts{Declared: 2, Failed: 1, NotRun: 1},
		Metrics:           &LabMetrics{Performance: &score},
		Events:            []Event{{DeclaredTests: &declared, Kind: "suite", AtMS: 1, Phase: "discovery"}, {Kind: "test", AtMS: 2, TestID: "t1", DurationMS: &duration}},
		DroppedEvents:     1,
		CaptureState:      "available",
		Artifacts:         []Artifact{{ID: "shot-1", Kind: "screenshot", MIME: "image/png", Bytes: 10, SHA256: "def", Test: ArtifactTest{ID: "t1", Title: "Checkout"}}},
		HistoryError:      "sync",
	}
}

// storedRunJSON is the history DEM_DATA encoding of storedRunFixture. Stored
// records are read back by later Agents, so the encoding is a contract.
const storedRunJSON = `{"captures":{"trace":"supported","reporting":"complete","truncated":true,"tests":[{"id":"t1","title":"Checkout","text_truncated":true,"status":"failed","expected_status":"passed","accepted":1,"unaccepted":2}],"associations":[{"id":"shot-1","test_id":"t1"}],"unpublished":[{"id":"shot-2","reason":"too_large"}]},"browser_summary":{"availability":"invalid","reason":"record_after_completion","reporter_complete":false,"coverage":"fixture_context","run":{"tests":{"observed":2,"no_summary":1},"counts":{"uncaught":1,"console_error":1,"transport":1,"http_error":1},"omissions":["pending"]},"tests":[{"id":"t1","state":"observed","counts":{"uncaught":1},"sample_ids":[1,2]},{"id":"t2","state":"invalid","reason":"invalid_annotation"}],"samples":[{"id":1,"test_id":"t1","category":"uncaught","message":"boom","text_truncated":true},{"id":2,"test_id":"t1","category":"console_error","state":"resolved","args":[{"value":null},{"value":"x"},{"value":1.5},{"value":true},{"omitted":"object"}],"args_omitted":3},{"id":3,"test_id":"t1","category":"console_error","state":"resolved","text":"console text"},{"id":4,"test_id":"t1","category":"console_error","state":"unavailable","text_omitted":"too_long"},{"id":5,"test_id":"t1","category":"http_error","method":"GET","url":"https://example.com/path","status":503},{"id":6,"test_id":"t1","category":"transport","method":"POST","url_omitted":"too_long","error":"refused"}],"truncated":["samples"]},"failure_evidence":{"availability":"supported","reporter_complete":true,"truncated":["text"],"tests":[{"id":"t1","title":"Checkout","location":{"file":"journey.spec.ts","line":4,"column":7},"status":"failed","expected_status":"passed","duration_ms":12.5,"reporting_complete":true,"text_truncated":true}],"steps":[{"id":"s1","test_id":"t1","parent_omitted":true,"title":"Submit","category":"test.step","location":{"file":"journey.spec.ts","line":4,"column":7},"state":"running"}],"first_failure":{"path_incomplete":true,"test":{"id":"t1","title":"Checkout","location":{"file":"journey.spec.ts","line":4,"column":7},"status":"failed","expected_status":"passed","duration_ms":12.5,"reporting_complete":true,"text_truncated":true},"path":[{"id":"s1","test_id":"t1","parent_omitted":true,"title":"Submit","category":"test.step","location":{"file":"journey.spec.ts","line":4,"column":7},"state":"running"}],"omitted_ancestors":2,"message":"expected receipt","location":{"file":"journey.spec.ts","line":4,"column":7},"text_truncated":true},"stop_context":{"test":{"id":"t1","title":"Checkout","location":{"file":"journey.spec.ts","line":4,"column":7},"status":"failed","expected_status":"passed","duration_ms":12.5,"reporting_complete":true,"text_truncated":true},"path":[],"omitted_ancestors":0,"message":""}},"runtime":{"version":"1.0.0","sha256":"abc","contract":2},"id":"0123456789abcdef0123456789abcdef","job_id":"journey:checkout","kind":"journey","name":"checkout","vnode_guid":"11111111-2222-3333-4444-555555555555","target":"https://example.com","scheduled_us":1,"queued_us":2,"started_us":3,"completed_us":4,"duration_ms":12.5,"outcome":"failed","termination_reason":"execution_budget","error":"failed","tests":{"declared":2,"passed":0,"failed":1,"timed_out":0,"skipped":0,"expected_failure":0,"not_run":1},"metrics":{"performance":87},"events":[{"declared_tests":2,"kind":"suite","at_ms":1,"phase":"discovery"},{"kind":"test","at_ms":2,"test_id":"t1","duration_ms":12.5}],"dropped_events":1,"capture_state":"available","artifacts":[{"id":"shot-1","kind":"screenshot","mime":"image/png","bytes":10,"sha256":"def","test":{"id":"t1","title":"Checkout"}}],"history_error":"sync"}`

func TestStoredRunEncodingRoundTrip(t *testing.T) {
	fixture := storedRunFixture()
	encoded, err := json.Marshal(fixture)
	require.NoError(t, err)
	assert.Equal(t, storedRunJSON, string(encoded))

	var decoded Run
	require.NoError(t, json.Unmarshal([]byte(storedRunJSON), &decoded))
	assert.Equal(t, fixture, decoded)

	assert.Equal(t, fixture, CloneRun(fixture))
	cloned, err := json.Marshal(CloneRun(fixture))
	require.NoError(t, err)
	assert.Equal(t, storedRunJSON, string(cloned))
}

func TestCloneRunStoresEmptyListsAsNull(t *testing.T) {
	// History records an attempt without events or artifacts as null lists.
	encoded, err := json.Marshal(CloneRun(Run{Events: []Event{}, Artifacts: []Artifact{}}))
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"events":null`)
	assert.Contains(t, string(encoded), `"artifacts":null`)
}

func TestCloneRunIsolatesPointers(t *testing.T) {
	original := storedRunFixture()
	before, err := json.Marshal(original)
	require.NoError(t, err)
	cloned := CloneRun(original)
	*cloned.DurationMS = 99
	*cloned.Metrics.Performance = 1
	*cloned.Events[0].DeclaredTests = 9
	*cloned.Events[1].DurationMS = 9
	cloned.Runtime.Version = "changed"
	cloned.Tests.Failed = 9
	cloned.Artifacts[0].ID = "changed"
	cloned.Events[0].Kind = "changed"
	cloned.Captures.Tests[0].Title = "changed"
	cloned.BrowserSummary.Samples[1].Args[1].Value = "changed"
	*cloned.BrowserSummary.Samples[2].Text = "changed"
	*cloned.BrowserSummary.Samples[4].URL = "changed"
	cloned.BrowserSummary.Tests[0].Counts.Uncaught = 9
	cloned.BrowserSummary.Run.Omissions[0] = "changed"
	after, err := json.Marshal(original)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after))
}
