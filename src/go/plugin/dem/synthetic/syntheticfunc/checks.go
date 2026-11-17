// SPDX-License-Identifier: GPL-3.0-or-later

package syntheticfunc

import (
	"context"
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem/internal/rawfunc"
	model "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/registry"
)

var checksMethod = method{
	Method: rawfunc.Method{
		ID:    "synthetics-checks",
		Title: "Synthetic monitors",
		Help: "The current status of every active journey and Lighthouse job: its latest result and whether it " +
			"is still fresh, its last success and failure, and whether its checks are waiting for a free browser. " +
			"Disabled jobs and jobs that failed to start are not listed. Pass job_id to show one job and kind " +
			"(journey or lighthouse) to show one kind. " + limitHelp + " " + rawfunc.VnodeHelp,
		Sort:        "job_id",
		UpdateEvery: 10,
		Params:      []string{"job_id", "kind", "limit", "vnode", "vnode_guid"},
		Columns:     checksColumns.Build(),
	},
	serve: (*Handler).checks,
}

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
		rows = append(rows, checksColumns.Row(j))
	}
	response["data"] = rows
	response["truncated"] = truncated
	response["limit"] = limit
	return funcapi.RawResponse(response)
}

// validKind accepts the synthetics-checks kind filter: empty, journey or lighthouse.
func validKind(value string) error {
	if value != "" && value != string(model.Journey) && value != string(model.Lighthouse) {
		return errors.New("kind must be journey or lighthouse")
	}
	return nil
}

var checksColumns = rawfunc.Columns[registry.Job]{
	column("job_id", "Monitor", funcapi.FieldTypeString, "", func(j registry.Job) any { return j.JobID }),
	column("kind", "Kind", funcapi.FieldTypeString, "", func(j registry.Job) any { return j.Kind }),
	column("name", "Name", funcapi.FieldTypeString, "", func(j registry.Job) any { return j.Name }),
	column("target", "Target", funcapi.FieldTypeString, "", func(j registry.Job) any { return j.Target }),
	column("state", "Current state", funcapi.FieldTypeString, "", func(j registry.Job) any { return string(j.State) }),
	column(
		"fresh",
		"Last observation fresh",
		funcapi.FieldTypeBoolean,
		"",
		func(j registry.Job) any { return j.Fresh },
	),
	column("run_id", "Last run", funcapi.FieldTypeString, "", func(j registry.Job) any { return j.Latest.ID }),
	column("outcome", "Last outcome", funcapi.FieldTypeString, "", func(j registry.Job) any {
		return string(j.Latest.Outcome)
	}),
	column("completed_us", "Last completion", funcapi.FieldTypeTimestamp, "", func(j registry.Job) any {
		return timestamp(j.Latest.CompletedUS)
	}),
	column("duration_ms", "Attempt duration", funcapi.FieldTypeFloat, "ms", func(j registry.Job) any {
		if !j.Latest.DurationMS.Present {
			return nil
		}
		return j.Latest.DurationMS.Value
	}),
	column("last_success_us", "Last success in this vnode association", funcapi.FieldTypeTimestamp, "",
		func(j registry.Job) any { return timestamp(j.LastSuccessUS) }),
	column("last_failure_us", "Last failure in this vnode association", funcapi.FieldTypeTimestamp, "",
		func(j registry.Job) any { return timestamp(j.LastFailureUS) }),
	column("error", "Last error", funcapi.FieldTypeString, "", func(j registry.Job) any { return j.Latest.Error }),
	column("history_error", "History error", funcapi.FieldTypeString, "", func(j registry.Job) any {
		return j.Latest.HistoryError
	}),
	column("vnode_guid", "Vnode GUID", funcapi.FieldTypeString, "", func(j registry.Job) any { return j.VnodeGUID }),
	column("check_every", "Check interval", funcapi.FieldTypeInteger, "s", func(j registry.Job) any {
		return int(j.Interval / time.Second)
	}),
	column(
		"scheduled",
		"Scheduled occurrences",
		funcapi.FieldTypeInteger,
		"",
		func(j registry.Job) any { return j.Scheduled },
	),
	column(
		"admitted",
		"Admitted attempts",
		funcapi.FieldTypeInteger,
		"",
		func(j registry.Job) any { return j.Admitted },
	),
	column(
		"coalesced",
		"Coalesced occurrences",
		funcapi.FieldTypeInteger,
		"",
		func(j registry.Job) any { return j.Coalesced },
	),
	column(
		"pending",
		"Pending occurrence",
		funcapi.FieldTypeBoolean,
		"",
		func(j registry.Job) any { return j.Pending },
	),
	column("queue_age", "Oldest admission wait", funcapi.FieldTypeFloat, "s", func(j registry.Job) any {
		if j.QueuedUS <= 0 {
			return nil
		}
		return j.QueueAge.Seconds()
	}),
	column("scheduling_delay", "Latest admission delay", funcapi.FieldTypeFloat, "s", func(j registry.Job) any {
		if j.AdmittedUS <= 0 {
			return nil
		}
		return j.SchedulingDelay.Seconds()
	}),
}
