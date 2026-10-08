// SPDX-License-Identifier: GPL-3.0-or-later
package functions

import (
	"context"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	model "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
)

func (h *Handler) run(ctx context.Context, args map[string]string, response map[string]any) *funcapi.FunctionResponse {
	if args["job_id"] == "" || args["run_id"] == "" {
		return funcapi.ErrorResponse(400, "job_id and run_id are required")
	}
	run, err := h.deps.Run(ctx, args["job_id"], args["run_id"])
	if err != nil {
		return functionError(err)
	}
	rows := make([][]any, 0, len(run.Events))
	for _, e := range run.Events {
		rows = append(
			rows,
			[]any{
				e.AtMS,
				e.Kind,
				e.TestID,
				e.Title,
				e.Phase,
				e.Status,
				e.ExpectedStatus,
				number(e.DurationMS),
				e.Message,
			},
		)
	}
	response["data"] = rows
	response["run"] = runDetail(run)
	response["dropped_events"] = run.DroppedEvents
	return funcapi.RawResponse(response)
}

var runMethod = method{
	id:     "synthetics-run",
	title:  "Synthetic run detail",
	help:   "One retained or latest active attempt, independent of picker bounds, with original test phases, errors, counts, nullable lab measurements and bounded reporter events. Missing completion means no verified terminal observation.",
	sort:   "at_ms",
	params: []string{"job_id", "run_id"},
	columns: columns(
		column{
			"at_ms",
			"Event time",
			funcapi.FieldTypeTimestamp,
			"",
		},
		column{"kind", "Event", funcapi.FieldTypeString, ""},
		column{"test_id", "Test", funcapi.FieldTypeString, ""},
		column{"title", "Title", funcapi.FieldTypeString, ""},
		column{"phase", "Phase", funcapi.FieldTypeString, ""},
		column{"status", "Actual status", funcapi.FieldTypeString, ""},
		column{"expected_status", "Expected status", funcapi.FieldTypeString, ""},
		column{"duration_ms", "Duration", funcapi.FieldTypeFloat, "ms"},
		column{"message", "Message", funcapi.FieldTypeString, ""},
	),
}

func runDetail(run model.Run) map[string]any {
	detail := map[string]any{
		"id":             run.ID,
		"job_id":         run.JobID,
		"kind":           run.Kind,
		"name":           run.Name,
		"target":         run.Target,
		"started_us":     timestamp(run.StartedUS),
		"completed_us":   timestamp(run.CompletedUS),
		"duration_ms":    number(run.DurationMS),
		"outcome":        run.Outcome,
		"error":          run.Error,
		"tests":          run.Tests,
		"capture_state":  run.CaptureState,
		"artifacts":      run.Artifacts,
		"history_error":  run.HistoryError,
		"dropped_events": run.DroppedEvents,
		"metrics":        nil,
	}
	if m := run.Metrics; m != nil {
		detail["metrics"] = map[string]any{
			"performance": number(m.Performance),
			"fcp_ms":      number(m.FCPMS),
			"lcp_ms":      number(m.LCPMS),
			"tbt_ms":      number(m.TBTMS),
			"si_ms":       number(m.SIMS),
			"cls":         number(m.CLS),
		}
	}
	return detail
}
