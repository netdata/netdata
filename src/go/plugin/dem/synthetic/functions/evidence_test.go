// SPDX-License-Identifier: GPL-3.0-or-later
package functions_test

import (
	model "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	synthetichistory "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/history"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestFailureEvidenceFunctionsAreAdditive(t *testing.T) {
	for _, rich := range []bool{false, true} {
		name := "old history"
		if rich {
			name = "rich history"
		}
		t.Run(name, func(t *testing.T) {
			run := model.Run{ID: "r", JobID: "journey:a", Kind: model.Journey, Outcome: model.Timeout, TerminationReason: model.ExecutionBudget, Error: "core error", Events: []model.Event{{Kind: "step_end", Title: "legacy event"}}}
			if rich {
				run.BrowserSummary = &model.BrowserSummary{Availability: "supported", Coverage: "fixture_context", Run: &model.BrowserAggregate{Tests: model.BrowserTestCounts{Observed: 1}, Counts: model.BrowserCounts{ConsoleError: 2}, Omissions: []string{"candidate_limit"}}}
				run.FailureEvidence = &model.FailureEvidence{
					Availability: "supported", Truncated: []string{"user_steps"},
					Steps:        []model.EvidenceStep{{ID: "s1", TestID: "t", Title: "pending submit", State: "running"}},
					FirstFailure: &model.FailureDetail{Test: model.EvidenceTest{ID: "t", Title: "checkout", Status: "failed", ExpectedStatus: "passed"}, Message: "expected receipt", PathIncomplete: true},
					StopContext:  &model.FailureDetail{Test: model.EvidenceTest{ID: "later", Title: "login", Status: "interrupted", ExpectedStatus: "passed"}, PathIncomplete: true},
				}
			}
			src := &source{history: map[string]model.Run{"journey:a/r": run}, page: synthetichistory.RunPage{Runs: []model.Run{run}}}
			h := newHandler(src)
			detailResponse := call(h, "synthetics-run", "job_id:journey:a", "run_id:r")
			rows := responseData(t, detailResponse)
			require.Len(t, rows, 1)
			assert.Equal(t, "legacy event", rowValues(t, detailResponse, rows[0])["title"])
			detail := detailResponse.RawResponse["run"].(map[string]any)
			status := "not_recorded"
			if rich {
				status = "partial"
			}
			assert.Equal(t, map[string]any{
				"id": "r", "job_id": "journey:a", "kind": model.Journey,
				"vnode_guid": "", "name": "", "target": "",
				"started_us": nil, "scheduled_us": nil, "queued_us": nil, "completed_us": nil,
				"duration_ms": nil, "outcome": model.Timeout, "termination_reason": model.ExecutionBudget,
				"error": "core error", "tests": (*model.TestCounts)(nil),
				"capture_state": "", "artifacts": []model.Artifact(nil), "history_error": "",
				"dropped_events": 0, "metrics": nil, "runtime": (*model.RuntimeIdentity)(nil),
				"failure_evidence": run.FailureEvidence, "failure_evidence_status": status,
				"browser_summary": run.BrowserSummary, "browser_summary_status": status,
			}, detail)
			validate(t, detailResponse)
			listResponse := call(h, "synthetics-runs")
			listRows := responseData(t, listResponse)
			require.Len(t, listRows, 1)
			values := rowValues(t, listResponse, listRows[0])
			assert.Equal(t, run.FailureEvidence.ReportingStatus(), values["failure_evidence_status"])
			assert.Equal(t, run.FailureEvidence.FailureSummary(), values["first_failure"])
			if rich {
				assert.Contains(t, values["first_failure"], "[path incomplete]")
			}
			assert.Equal(t, run.FailureEvidence.LastObservedStepSummary(), values["last_observed_step"])
			assert.Equal(t, run.FailureEvidence.StopSummary(), values["stop_context"])
			assert.Equal(t, run.BrowserSummary.ReportingStatus(), values["browser_summary_status"])
			if rich {
				assert.Contains(t, values["browser_errors"], "tests: observed=1")
				assert.Contains(t, values["browser_errors"], "console_error=2")
				assert.Contains(t, values["browser_errors"], "[omissions: candidate_limit]")
			} else {
				assert.Equal(t, "", values["browser_errors"])
			}
			columns := listResponse.RawResponse["columns"].(map[string]any)
			assert.Equal(t, 15, columns["termination_reason"].(map[string]any)["index"])
			assert.Equal(t, 16, columns["failure_evidence_status"].(map[string]any)["index"])
			assert.Equal(t, 17, columns["first_failure"].(map[string]any)["index"])
			assert.Equal(t, 18, columns["last_observed_step"].(map[string]any)["index"])
			assert.Equal(t, 19, columns["stop_context"].(map[string]any)["index"])
			assert.Equal(t, 20, columns["browser_summary_status"].(map[string]any)["index"])
			assert.Equal(t, 21, columns["browser_errors"].(map[string]any)["index"])
			validate(t, listResponse)
		})
	}
}
