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
		if args["vnode_guid"] != "" && args["vnode_guid"] != j.VnodeGUID {
			continue
		}
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
		var completed, duration, queueAge, delay any
		if j.QueuedUS > 0 {
			queueAge = j.QueueAge.Seconds()
		}
		if j.AdmittedUS > 0 {
			delay = j.SchedulingDelay.Seconds()
		}
		if last := j.Latest; j.HasObservation {
			id = last.ID
			outcome = string(last.Outcome)
			detail = last.Error
			historyErr = last.HistoryError
			completed = timestamp(last.CompletedUS)
			if last.DurationMS.Present {
				duration = last.DurationMS.Value
			}
		}
		rows = append(
			rows,
			[]any{
				j.JobID,
				j.Kind,
				j.Name,
				j.Target,
				string(j.State),
				j.Fresh,
				id,
				outcome,
				completed,
				duration,
				timestamp(j.LastSuccessUS),
				timestamp(j.LastFailureUS),
				detail,
				historyErr,
				j.VnodeGUID,
				int(j.Interval / time.Second),
				j.Scheduled,
				j.Admitted,
				j.Coalesced,
				j.Pending,
				queueAge,
				delay,
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
	help:   "Active native monitors and their latest observation. Disabled and failed-startup configuration remains in native job status. Freshness belongs to the last observation, independently of a waiting or running attempt. Check interval controls browser execution. Occurrence counters reset on native activation and continue across vnode changes; coalesced occurrences never ran. Queue age preserves the oldest admission wait, while admission delay describes the selected due occurrence. Optional vnode selects a configured name; vnode_guid selects the current canonical GUID instead. Omit both to select all sites; the selectors are mutually exclusive. Blank Vnode GUID cells are unassigned.",
	sort:   "job_id",
	params: []string{"job_id", "kind", "limit", "vnode", "vnode_guid"},
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
		column{"last_success_us", "Last success in this vnode association", funcapi.FieldTypeTimestamp, ""},
		column{"last_failure_us", "Last failure in this vnode association", funcapi.FieldTypeTimestamp, ""},
		column{"error", "Last error", funcapi.FieldTypeString, ""},
		column{"history_error", "History error", funcapi.FieldTypeString, ""},
		column{"vnode_guid", "Vnode GUID", funcapi.FieldTypeString, ""},
		column{"check_every", "Check interval", funcapi.FieldTypeInteger, "s"},
		column{"scheduled", "Scheduled occurrences", funcapi.FieldTypeInteger, ""},
		column{"admitted", "Admitted attempts", funcapi.FieldTypeInteger, ""},
		column{"coalesced", "Coalesced occurrences", funcapi.FieldTypeInteger, ""},
		column{"pending", "Pending occurrence", funcapi.FieldTypeBoolean, ""},
		column{"queue_age", "Oldest admission wait", funcapi.FieldTypeFloat, "s"},
		column{"scheduling_delay", "Latest admission delay", funcapi.FieldTypeFloat, "s"},
	),
}
