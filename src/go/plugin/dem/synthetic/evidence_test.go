// SPDX-License-Identifier: GPL-3.0-or-later

package synthetic

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCloneRunFailureEvidence(t *testing.T) {
	duration := 1.5
	location := &EvidenceLocation{File: "journey.spec.js", Line: 10, Column: 2}
	test := EvidenceTest{ID: "t", Title: "checkout", Location: location, DurationMS: &duration}
	step := EvidenceStep{ID: "s1", TestID: "t", Title: "submit", Location: location, DurationMS: &duration}
	original := Run{FailureEvidence: &FailureEvidence{
		Availability: "supported", Truncated: []string{"text"},
		Tests: []EvidenceTest{test}, Steps: []EvidenceStep{step},
		FirstFailure: &FailureDetail{Test: test, Path: []EvidenceStep{step}, Location: location, Message: "assertion"},
		StopContext:  &FailureDetail{Test: test, Path: []EvidenceStep{step}, Location: location},
	}}
	before, err := json.Marshal(original)
	require.NoError(t, err)
	cloned := CloneRun(original)
	require.Equal(t, original, cloned)
	evidence := cloned.FailureEvidence
	evidence.Availability = "invalid"
	evidence.Truncated[0] = "tests"
	evidence.Tests[0].Title = "changed"
	evidence.Tests[0].Location.File = "changed"
	*evidence.Tests[0].DurationMS = 2
	evidence.Steps[0].Title = "changed"
	evidence.Steps[0].Location.File = "changed"
	*evidence.Steps[0].DurationMS = 3
	evidence.FirstFailure.Message = "changed"
	evidence.FirstFailure.Location.File = "changed"
	evidence.FirstFailure.Test.Location.File = "changed"
	*evidence.FirstFailure.Test.DurationMS = 4
	evidence.FirstFailure.Path[0].Location.File = "changed"
	*evidence.FirstFailure.Path[0].DurationMS = 5
	evidence.FirstFailure.Path[0].Title = "changed"
	evidence.StopContext.Test.Location.File = "changed"
	*evidence.StopContext.Test.DurationMS = 6
	evidence.StopContext.Path[0].Location.File = "changed"
	*evidence.StopContext.Path[0].DurationMS = 7
	evidence.StopContext.Path[0].Title = "changed"
	evidence.StopContext.Location.File = "changed"
	after, err := json.Marshal(original)
	require.NoError(t, err)
	assert.JSONEq(t, string(before), string(after))
	assert.Nil(t, CloneRun(Run{}).FailureEvidence)
	assert.Nil(t, CloneFailureEvidence(nil))
}

func TestFailureEvidenceSummaries(t *testing.T) {
	for _, test := range []struct {
		evidence *FailureEvidence
		status   string
	}{
		{nil, "not_recorded"},
		{&FailureEvidence{Availability: "unsupported"}, "unsupported"},
		{&FailureEvidence{Availability: "unavailable"}, "unavailable"},
		{&FailureEvidence{Availability: "invalid", ReporterComplete: true}, "invalid"},
		{&FailureEvidence{Availability: "supported"}, "partial"},
		{&FailureEvidence{Availability: "supported", ReporterComplete: true, Truncated: []string{"tests"}}, "complete"},
	} {
		assert.Equal(t, test.status, test.evidence.ReportingStatus())
	}
	var absent *FailureEvidence
	assert.Empty(t, absent.FailureSummary())
	assert.Empty(t, absent.LastObservedStepSummary())
	evidence := &FailureEvidence{
		Truncated: []string{"user_steps"},
		Steps: []EvidenceStep{
			{Title: "older sibling", State: "running"},
			{Title: "newer sibling", State: "running"},
			{Title: "completed sibling", State: "passed"},
		},
		FirstFailure: &FailureDetail{
			Test:             EvidenceTest{Title: "checkout"},
			Path:             []EvidenceStep{{Title: "submit", ParentOmitted: true}},
			OmittedAncestors: 2, Message: "expected receipt", TextTruncated: true,
			Location: &EvidenceLocation{File: "checkout.spec.js", Line: 12, Column: 3},
		},
	}
	assert.Equal(t, "checkout > … > submit (checkout.spec.js:12:3): expected receipt [text truncated]", evidence.FailureSummary())
	assert.Equal(t, "newer sibling [partial: step detail truncated]", evidence.LastObservedStepSummary())
	evidence.Steps[1].State = "passed"
	assert.Equal(t, "older sibling [partial: step detail truncated]", evidence.LastObservedStepSummary())
	evidence.Steps[0].State = "failed"
	assert.Empty(t, evidence.LastObservedStepSummary())
}

func TestFailureSummaryUnexpectedPass(t *testing.T) {
	evidence := &FailureEvidence{FirstFailure: &FailureDetail{Test: EvidenceTest{Title: "known issue", Status: "passed", ExpectedStatus: "failed"}}}
	assert.Equal(t, "known issue [passed; expected failed]", evidence.FailureSummary())
}

func TestLastObservedStepPathHandlesMissingAndCyclicParents(t *testing.T) {
	evidence := &FailureEvidence{Availability: "invalid", Tests: []EvidenceTest{{ID: "t", Title: "checkout"}}, Steps: []EvidenceStep{
		{ID: "s1", TestID: "t", Title: "purchase", State: "passed"},
		{ID: "s2", TestID: "t", ParentID: "s1", Title: "submit", State: "running"},
	}}
	assert.Equal(t, "checkout > purchase > submit [partial: evidence invalid]", evidence.LastObservedStepSummary())
	evidence.Steps[0].ParentID = "s2"
	assert.Equal(t, "checkout > … > purchase > submit [partial: evidence invalid]", evidence.LastObservedStepSummary())
	evidence.Steps[0].ParentID = "missing"
	assert.Equal(t, "checkout > … > purchase > submit [partial: evidence invalid]", evidence.LastObservedStepSummary())
	evidence.Steps[1].ParentOmitted = true
	assert.Equal(t, "checkout > … > submit [partial: evidence invalid]", evidence.LastObservedStepSummary())
}

func TestFailureSummaryDisclosesIncompletePath(t *testing.T) {
	evidence := &FailureEvidence{FirstFailure: &FailureDetail{
		Test:           EvidenceTest{ID: "t", Title: "checkout", Status: "failed", ExpectedStatus: "passed"},
		Path:           []EvidenceStep{{ID: "s1", TestID: "t", Title: "purchase", State: "failed"}},
		PathIncomplete: true, Message: "receipt missing",
	}}
	assert.Equal(t, "checkout [failed; expected passed] > purchase: receipt missing [path incomplete]", evidence.FailureSummary())
	clone := CloneFailureEvidence(evidence)
	require.Equal(t, evidence, clone)
	encoded, err := json.Marshal(clone)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"path_incomplete":true`)
	var decoded FailureEvidence
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	assert.True(t, decoded.FirstFailure.PathIncomplete)
}

func TestCloneRunPreservesEmptyFailurePath(t *testing.T) {
	for _, raw := range []string{`{"path":[]}`, `{"path":null}`} {
		t.Run(raw, func(t *testing.T) {
			var failure FailureDetail
			require.NoError(t, json.Unmarshal([]byte(raw), &failure))
			original := Run{FailureEvidence: &FailureEvidence{FirstFailure: &failure}}
			cloned := CloneRun(original)
			assert.Equal(t, original, cloned)
			before, err := json.Marshal(original)
			require.NoError(t, err)
			after, err := json.Marshal(cloned)
			require.NoError(t, err)
			assert.JSONEq(t, string(before), string(after))
		})
	}
}

func TestFailureSummaryDisclosesInvalidEvidence(t *testing.T) {
	evidence := &FailureEvidence{
		Availability: "invalid",
		FirstFailure: &FailureDetail{
			Test:    EvidenceTest{Title: "checkout"},
			Message: "receipt missing",
		},
	}
	assert.Equal(t, "checkout: receipt missing [partial: evidence invalid]", evidence.FailureSummary())
}
