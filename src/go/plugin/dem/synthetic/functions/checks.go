// SPDX-License-Identifier: GPL-3.0-or-later
package functions

import (
	"context"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
)

func (h *Handler) checks(
	ctx context.Context,
	args map[string]string,
	response map[string]any,
) *funcapi.FunctionResponse {
	response["artifacts"] = h.deps.ArtifactStats()
	if err := validKind(args["kind"]); err != nil {
		return funcapi.ErrorResponse(400, "%v", err)
	}
	limit, err := rowLimit(args["limit"])
	if err != nil {
		return funcapi.ErrorResponse(400, "%v", err)
	}
	rows := make([][]any, 0)
	truncated := false
	for _, j := range h.deps.Jobs(time.Now()) {
		if args["job_id"] != "" && args["job_id"] != j.JobID {
			continue
		}
		if args["kind"] != "" && args["kind"] != string(j.Kind) {
			continue
		}
		if len(rows) == limit {
			truncated = true
			break
		}
		var id, outcome, detail, historyErr string
		var completed, duration any
		if last := j.Latest; last != nil {
			id = last.ID
			outcome = string(last.Outcome)
			detail = last.Error
			historyErr = last.HistoryError
			completed = timestamp(last.CompletedUS)
			duration = number(last.DurationMS)
		}
		rows = append(
			rows,
			[]any{
				j.JobID,
				j.Kind,
				j.Name,
				j.Target,
				j.State,
				j.Fresh,
				id,
				outcome,
				completed,
				duration,
				timestamp(j.LastSuccessUS),
				timestamp(j.LastFailureUS),
				detail,
				historyErr,
			},
		)
	}
	response["data"] = rows
	response["truncated"] = truncated
	response["limit"] = limit
	return funcapi.RawResponse(response)
}

var checksMethod = method{
	id:     "synthetics-checks",
	title:  "Synthetic monitors",
	help:   "Active native monitors and their latest observation. Disabled and failed-startup configuration remains in native job status. Freshness belongs to the last observation, independently of a waiting or running attempt.",
	sort:   "job_id",
	params: []string{"job_id", "kind", "limit"},
	columns: columns(
		column{
			"job_id",
			"Monitor",
			funcapi.FieldTypeString,
			"",
		},
		column{"kind", "Kind", funcapi.FieldTypeString, ""},
		column{"name", "Name", funcapi.FieldTypeString, ""},
		column{"target", "Target", funcapi.FieldTypeString, ""},
		column{"state", "Current state", funcapi.FieldTypeString, ""},
		column{"fresh", "Last observation fresh", funcapi.FieldTypeBoolean, ""},
		column{"run_id", "Last run", funcapi.FieldTypeString, ""},
		column{"outcome", "Last outcome", funcapi.FieldTypeString, ""},
		column{"completed_us", "Last completion", funcapi.FieldTypeTimestamp, ""},
		column{"duration_ms", "Attempt duration", funcapi.FieldTypeFloat, "ms"},
		column{"last_success_us", "Last success in this activation", funcapi.FieldTypeTimestamp, ""},
		column{"last_failure_us", "Last failure in this activation", funcapi.FieldTypeTimestamp, ""},
		column{"error", "Last error", funcapi.FieldTypeString, ""},
		column{"history_error", "History error", funcapi.FieldTypeString, ""},
	),
}
