// SPDX-License-Identifier: GPL-3.0-or-later

package syntheticfunc

import (
	"context"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem/internal/rawfunc"
	model "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
)

var runMethod = method{
	Method: rawfunc.Method{
		ID:          "synthetics-run",
		Title:       "Synthetic run detail",
		Help:        "One retained or latest active attempt, independent of picker bounds, with original test phases, errors, counts, nullable lab measurements and bounded reporter events. Missing completion means no verified terminal observation.",
		Sort:        "at_ms",
		UpdateEvery: 10,
		Params:      []string{"job_id", "run_id"},
		Columns:     runColumns.Build(),
	},
	serve: (*Handler).run,
}

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
		rows = append(rows, runColumns.Row(e))
	}
	response["data"] = rows
	response["run"] = runDetail(run)
	response["dropped_events"] = run.DroppedEvents
	return funcapi.RawResponse(response)
}

var runColumns = rawfunc.Columns[model.Event]{
	{
		// Reporter event times are Unix milliseconds.
		Name:      "at_ms",
		Tooltip:   "Event time",
		Type:      funcapi.FieldTypeTimestamp,
		Transform: funcapi.FieldTransformDatetime,
		Visible:   true,
		Sortable:  true,
		Value:     func(e model.Event) any { return e.AtMS },
	},
	column("kind", "Event", funcapi.FieldTypeString, "", func(e model.Event) any { return e.Kind }),
	column("test_id", "Test", funcapi.FieldTypeString, "", func(e model.Event) any { return e.TestID }),
	column("title", "Title", funcapi.FieldTypeString, "", func(e model.Event) any { return e.Title }),
	column("phase", "Phase", funcapi.FieldTypeString, "", func(e model.Event) any { return e.Phase }),
	column("status", "Actual status", funcapi.FieldTypeString, "", func(e model.Event) any { return e.Status }),
	column("expected_status", "Expected status", funcapi.FieldTypeString, "", func(e model.Event) any { return e.ExpectedStatus }),
	column("duration_ms", "Duration", funcapi.FieldTypeFloat, "ms", func(e model.Event) any { return number(e.DurationMS) }),
	column("message", "Message", funcapi.FieldTypeString, "", func(e model.Event) any { return e.Message }),
}

func runDetail(run model.Run) map[string]any {
	// Run copies and history records keep an empty artifact list as null; the Function shows it as empty.
	artifacts := run.Artifacts
	if artifacts == nil {
		artifacts = []model.Artifact{}
	}
	detail := map[string]any{
		"id":                      run.ID,
		"job_id":                  run.JobID,
		"vnode_guid":              run.VnodeGUID,
		"kind":                    run.Kind,
		"name":                    run.Name,
		"target":                  run.Target,
		"started_us":              timestamp(run.StartedUS),
		"scheduled_us":            timestamp(run.ScheduledUS),
		"queued_us":               timestamp(run.QueuedUS),
		"completed_us":            timestamp(run.CompletedUS),
		"duration_ms":             number(run.DurationMS),
		"outcome":                 run.Outcome,
		"termination_reason":      run.TerminationReason,
		"error":                   run.Error,
		"tests":                   run.Tests,
		"capture_state":           run.CaptureState,
		"artifacts":               artifacts,
		"captures":                run.Captures,
		"history_error":           run.HistoryError,
		"dropped_events":          run.DroppedEvents,
		"metrics":                 nil,
		"runtime":                 run.Runtime,
		"failure_evidence":        run.FailureEvidence,
		"browser_summary":         run.BrowserSummary,
		"browser_summary_status":  run.BrowserSummary.ReportingStatus(),
		"failure_evidence_status": run.FailureEvidence.ReportingStatus(),
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
