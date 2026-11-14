// SPDX-License-Identifier: GPL-3.0-or-later

package syntheticfunc

import (
	"context"
	"strconv"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem/internal/rawfunc"
	model "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
)

var runsMethod = method{
	Method: rawfunc.Method{
		ID:          "synthetics-runs",
		Title:       "Synthetic runs",
		Help:        "Retained attempts selected by admission/start time, with their latest retained outcome even when completion is outside the range. after and before include their entire Unix second or accept negative offsets from now; omitted bounds cover retained history through now. Results are capped at " + strconv.Itoa(rawfunc.MaxRows) + " and disclose truncation. Optional vnode selects a configured name; vnode_guid selects a recorded canonical GUID instead. Omit both to select all sites; the selectors are mutually exclusive. Blank Vnode GUID cells are unassigned.",
		Sort:        "started_us",
		UpdateEvery: 10,
		History:     true,
		Params:      []string{"job_id", "kind", "outcome", "after", "before", "limit", "vnode", "vnode_guid"},
		Columns:     runsColumns.Build(),
	},
	serve: (*Handler).runs,
}

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
		rows = append(rows, runsColumns.Row(run))
	}
	response["data"] = rows
	response["truncated"] = page.Truncated
	response["limit"] = filter.Limit
	response["after"] = filter.After
	response["before"] = *filter.Before
	return funcapi.RawResponse(response)
}

var runsColumns = rawfunc.Columns[model.Run]{
	column("run_id", "Run", funcapi.FieldTypeString, "", func(r model.Run) any { return r.ID }),
	column("job_id", "Monitor", funcapi.FieldTypeString, "", func(r model.Run) any { return r.JobID }),
	column("kind", "Kind", funcapi.FieldTypeString, "", func(r model.Run) any { return r.Kind }),
	column("name", "Name", funcapi.FieldTypeString, "", func(r model.Run) any { return r.Name }),
	column("target", "Target", funcapi.FieldTypeString, "", func(r model.Run) any { return r.Target }),
	column("started_us", "Started", funcapi.FieldTypeTimestamp, "", func(r model.Run) any { return timestamp(r.StartedUS) }),
	column("completed_us", "Completed", funcapi.FieldTypeTimestamp, "", func(r model.Run) any {
		return timestamp(r.CompletedUS)
	}),
	column("outcome", "Outcome", funcapi.FieldTypeString, "", func(r model.Run) any { return r.Outcome }),
	column("duration_ms", "Attempt duration", funcapi.FieldTypeFloat, "ms", func(r model.Run) any { return number(r.DurationMS) }),
	column("capture_state", "Capture state", funcapi.FieldTypeString, "", func(r model.Run) any { return r.CaptureState }),
	column("error", "Error", funcapi.FieldTypeString, "", func(r model.Run) any { return r.Error }),
	column("history_error", "History error", funcapi.FieldTypeString, "", func(r model.Run) any { return r.HistoryError }),
	column("vnode_guid", "Vnode GUID", funcapi.FieldTypeString, "", func(r model.Run) any { return r.VnodeGUID }),
	column("scheduled_us", "Selected due time", funcapi.FieldTypeTimestamp, "", func(r model.Run) any {
		return timestamp(r.ScheduledUS)
	}),
	column("queued_us", "Original admission queue time", funcapi.FieldTypeTimestamp, "", func(r model.Run) any {
		return timestamp(r.QueuedUS)
	}),
	column("termination_reason", "Termination reason", funcapi.FieldTypeString, "", func(r model.Run) any {
		return r.TerminationReason
	}),
	column("failure_evidence_status", "Evidence reporting", funcapi.FieldTypeString, "", func(r model.Run) any {
		return r.FailureEvidence.ReportingStatus()
	}),
	column("first_failure", model.FirstFailureLabel, funcapi.FieldTypeString, "", func(r model.Run) any {
		return r.FailureEvidence.FailureSummary()
	}),
	column("last_observed_step", model.LastObservedStepLabel, funcapi.FieldTypeString, "", func(r model.Run) any {
		return r.FailureEvidence.LastObservedStepSummary()
	}),
	column("stop_context", model.StopContextLabel, funcapi.FieldTypeString, "", func(r model.Run) any {
		return r.FailureEvidence.StopSummary()
	}),
	column("browser_summary_status", "Browser summary reporting", funcapi.FieldTypeString, "", func(r model.Run) any {
		return r.BrowserSummary.ReportingStatus()
	}),
	column("browser_errors", model.BrowserErrorsLabel, funcapi.FieldTypeString, "", func(r model.Run) any {
		return r.BrowserSummary.Summary()
	}),
}
