// SPDX-License-Identifier: GPL-3.0-or-later
package functions

import (
	"context"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
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
	help:    "Retained attempts selected by start time, with their latest retained outcome even when completion is outside the range. after and before include their entire Unix second or accept negative offsets from now; omitted bounds cover retained history through now. Results are capped at 2000 and disclose truncation.",
	sort:    "started_us",
	history: true,
	params:  []string{"job_id", "kind", "outcome", "after", "before", "limit"},
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
	),
}
