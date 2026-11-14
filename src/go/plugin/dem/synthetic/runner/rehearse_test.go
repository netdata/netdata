// SPDX-License-Identifier: GPL-3.0-or-later

package runner

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/stretchr/testify/assert"
)

func TestRehearsalFailureTextUsesRetainedSummaries(t *testing.T) {
	evidence := &synthetic.FailureEvidence{
		Availability: "invalid", Reason: "invalid_record",
		Steps: []synthetic.EvidenceStep{{ID: "s1", TestID: "t", Title: "wait for receipt", State: "running"}},
		FirstFailure: &synthetic.FailureDetail{
			Test:     synthetic.EvidenceTest{ID: "t", Title: "checkout", Status: "failed", ExpectedStatus: "passed"},
			Path:     []synthetic.EvidenceStep{{ID: "s2", TestID: "t", Title: "submit", State: "failed"}},
			Location: &synthetic.EvidenceLocation{File: "checkout.spec.js", Line: 10, Column: 2}, Message: "expected receipt",
		},
	}
	run := synthetic.Run{Error: "core timeout", FailureEvidence: evidence}
	assert.Equal(t, "core timeout\nFirst unexpected failure: "+evidence.FailureSummary()+"\nLast observed running step: "+evidence.LastObservedStepSummary(), rehearsalFailureText(run))
	assert.Equal(t, "core timeout", rehearsalFailureText(synthetic.Run{Error: "core timeout"}))
	assert.Equal(t, "core timeout", rehearsalFailureText(synthetic.Run{Error: "core timeout", FailureEvidence: &synthetic.FailureEvidence{Availability: "unsupported"}}))
}

func TestRehearsalStopContextAndIdenticalErrorDeduplication(t *testing.T) {
	evidence := &synthetic.FailureEvidence{
		FirstFailure: &synthetic.FailureDetail{Test: synthetic.EvidenceTest{Title: "assertion"}, Message: "receipt missing"},
		StopContext:  &synthetic.FailureDetail{Test: synthetic.EvidenceTest{Title: "later wait", Status: "interrupted", ExpectedStatus: "passed"}, PathIncomplete: true},
	}
	text := rehearsalFailureText(synthetic.Run{Error: "receipt missing", FailureEvidence: evidence})
	assert.Equal(t, "First unexpected failure: assertion: receipt missing\nPending before cleanup: later wait [interrupted; expected passed] [path incomplete]", text)
	assert.Contains(t, rehearsalFailureText(synthetic.Run{Error: "different core error", FailureEvidence: evidence}), "different core error\nFirst unexpected failure:")
}
