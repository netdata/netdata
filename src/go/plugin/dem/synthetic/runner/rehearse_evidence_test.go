//go:build linux || darwin || freebsd

// SPDX-License-Identifier: GPL-3.0-or-later
package runner

import (
	"context"
	"encoding/json"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
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

func TestRehearseUsesWireFailureEvidence(t *testing.T) {
	e, history, _ := testEngine(t)
	e.start = starter(t, func(ctx context.Context, args []string, raw []byte, out, stderr *os.File) ndexec.TreeResult {
		encoder := json.NewEncoder(out)
		if err := encoder.Encode(map[string]any{"type": "extension_init", "extension_init": map[string]any{"selected": []string{failureEvidenceFeature}}}); !assert.NoError(t, err) {
			return ndexec.TreeResult{Drained: true, Err: err}
		}
		failure := synthetic.FailureDetail{Test: synthetic.EvidenceTest{ID: "t", Title: "checkout", Status: "failed", ExpectedStatus: "passed", ReportingComplete: true}, Message: "receipt synthetic-secret missing", Location: &synthetic.EvidenceLocation{File: "checkout.spec.js", Line: 10, Column: 2}}
		if err := encoder.Encode(map[string]any{"type": "extension", "extension": map[string]any{"feature": failureEvidenceFeature, "data": map[string]any{"record": "failure", "failure": failure}}}); !assert.NoError(t, err) {
			return ndexec.TreeResult{Drained: true, Err: err}
		}
		return resultPayload(reporterResult{Status: synthetic.Failed, Tests: &synthetic.TestCounts{Declared: 1, Failed: 1}, Error: "core assertion", CaptureState: "disabled"})(ctx, args, raw, out, stderr)
	})
	request := journey()
	request.Secrets = map[string]string{"DEM_SECRET_TOKEN": "synthetic-secret"}
	err := e.Rehearse(t.Context(), request)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "First unexpected failure: checkout [failed; expected passed] (checkout.spec.js:10:2): receipt")
	assert.NotContains(t, err.Error(), "synthetic-secret")
	assert.Empty(t, history.runs)
}

func TestRehearseOptionalInvalidityPreservesSuccess(t *testing.T) {
	e, history, _ := testEngine(t)
	e.start = starter(t, func(ctx context.Context, args []string, raw []byte, out, stderr *os.File) ndexec.TreeResult {
		encoder := json.NewEncoder(out)
		if err := encoder.Encode(map[string]any{"type": "extension_init", "extension_init": map[string]any{"selected": []string{failureEvidenceFeature}}}); !assert.NoError(t, err) {
			return ndexec.TreeResult{Drained: true, Err: err}
		}
		if err := encoder.Encode(map[string]any{"type": "extension", "extension": map[string]any{"feature": failureEvidenceFeature, "data": map[string]any{"record": "invalid"}}}); !assert.NoError(t, err) {
			return ndexec.TreeResult{Drained: true, Err: err}
		}
		return resultPayload(success())(ctx, args, raw, out, stderr)
	})
	require.NoError(t, e.Rehearse(t.Context(), journey()))
	assert.Empty(t, history.runs)
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
