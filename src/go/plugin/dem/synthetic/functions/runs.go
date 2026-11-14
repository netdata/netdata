// SPDX-License-Identifier: GPL-3.0-or-later
package functions

import (
	"context"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	model "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
)

func (h *Handler) runs(ctx context.Context, args map[string]string, response map[string]any) *funcapi.FunctionResponse {
	filter, err := runFilter(args, time.Now().Unix())
	if err != nil {
		return funcapi.ErrorResponse(400, "%v", err)
	}
	page, err := h.deps.Runs(ctx, filter)
	if err != nil {
		return functionError(err)
	}
	rows := make([][]any, 0, len(page.Runs))
	for _, run := range page.Runs {
		rows = append(
			rows,
			[]any{
				run.ID,
				run.JobID,
				run.Kind,
				run.Name,
				run.Target,
				timestamp(run.StartedUS),
				timestamp(run.CompletedUS),
				run.Outcome,
				number(run.DurationMS),
				run.CaptureState,
				run.Error,
				run.HistoryError,
				run.VnodeGUID,
				timestamp(run.ScheduledUS),
				timestamp(run.QueuedUS),
				run.TerminationReason,
				run.FailureEvidence.ReportingStatus(),
				run.FailureEvidence.FailureSummary(),
				run.FailureEvidence.LastObservedStepSummary(),
				run.FailureEvidence.StopSummary(),
				run.BrowserSummary.ReportingStatus(),
				run.BrowserSummary.Summary(),
			},
		)
	}
	response["data"] = rows
	response["truncated"] = page.Truncated
	response["limit"] = filter.Limit
	response["after"] = filter.After
	response["before"] = *filter.Before
	return funcapi.RawResponse(response)
}

var runsMethod = method{
	id:      "synthetics-runs",
	title:   "Synthetic runs",
	help:    "Retained attempts selected by admission/start time, with their latest retained outcome even when completion is outside the range. after and before include their entire Unix second or accept negative offsets from now; omitted bounds cover retained history through now. Results are capped at 2000 and disclose truncation. Optional vnode selects a configured name; vnode_guid selects a recorded canonical GUID instead. Omit both to select all sites; the selectors are mutually exclusive. Blank Vnode GUID cells are unassigned.",
	sort:    "started_us",
	history: true,
	params:  []string{"job_id", "kind", "outcome", "after", "before", "limit", "vnode", "vnode_guid"},
	columns: columns(
		column{
			"run_id",
			"Run",
			funcapi.FieldTypeString,
			"",
		},
		column{"job_id", "Monitor", funcapi.FieldTypeString, ""},
		column{"kind", "Kind", funcapi.FieldTypeString, ""},
		column{"name", "Name", funcapi.FieldTypeString, ""},
		column{"target", "Target", funcapi.FieldTypeString, ""},
		column{"started_us", "Started", funcapi.FieldTypeTimestamp, ""},
		column{"completed_us", "Completed", funcapi.FieldTypeTimestamp, ""},
		column{"outcome", "Outcome", funcapi.FieldTypeString, ""},
		column{"duration_ms", "Attempt duration", funcapi.FieldTypeFloat, "ms"},
		column{"capture_state", "Capture state", funcapi.FieldTypeString, ""},
		column{"error", "Error", funcapi.FieldTypeString, ""},
		column{"history_error", "History error", funcapi.FieldTypeString, ""},
		column{"vnode_guid", "Vnode GUID", funcapi.FieldTypeString, ""},
		column{"scheduled_us", "Selected due time", funcapi.FieldTypeTimestamp, ""},
		column{"queued_us", "Original admission queue time", funcapi.FieldTypeTimestamp, ""},
		column{"termination_reason", "Termination reason", funcapi.FieldTypeString, ""},
		column{"failure_evidence_status", "Evidence reporting", funcapi.FieldTypeString, ""},
		column{"first_failure", model.FirstFailureLabel, funcapi.FieldTypeString, ""},
		column{"last_observed_step", model.LastObservedStepLabel, funcapi.FieldTypeString, ""},
		column{"stop_context", model.StopContextLabel, funcapi.FieldTypeString, ""},
		column{"browser_summary_status", "Browser summary reporting", funcapi.FieldTypeString, ""},
		column{"browser_errors", model.BrowserErrorsLabel, funcapi.FieldTypeString, ""},
	),
}
