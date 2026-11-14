// SPDX-License-Identifier: GPL-3.0-or-later
package syntheticfunc

import (
	"context"
	"encoding/base64"
	"errors"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem/internal/rawfunc"
	model "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/query"
)

var artifactMethod = method{
	Method: rawfunc.Method{
		ID:          "synthetics-artifact",
		Title:       "Synthetic artifacts",
		Help:        "Retained capture metadata for one run; file bytes are verified when fetched. Test identifies the associated test, not a page, failing step or screenshot time. Missing-capture observations and publication decisions are in synthetics-run while history remains. Supplying artifact_id requests base64-encoded bytes and requires administrator-equivalent permissions. Screenshots, traces and HTML reports may contain sensitive content. Missing retained files are expired or unavailable; fetches are limited to 5 MiB.",
		Sort:        "artifact_id",
		UpdateEvery: 10,
		Params:      []string{"job_id", "run_id", "artifact_id"},
		Columns:     artifactColumns.Build(),
	},
	serve: (*Handler).artifact,
}

var artifactColumns = rawfunc.Columns[query.Artifact]{
	column("artifact_id", "Artifact", funcapi.FieldTypeString, "", func(a query.Artifact) any { return a.ID }),
	column("kind", "Kind", funcapi.FieldTypeString, "", func(a query.Artifact) any { return a.Kind }),
	column("mime", "Content type", funcapi.FieldTypeString, "", func(a query.Artifact) any { return a.MIME }),
	column("bytes", "Size", funcapi.FieldTypeInteger, "bytes", func(a query.Artifact) any { return a.Bytes }),
	column("sha256", "SHA-256", funcapi.FieldTypeString, "", func(a query.Artifact) any { return a.SHA256 }),
	column("availability", "Availability", funcapi.FieldTypeString, "", func(a query.Artifact) any { return a.Availability }),
	column("test", "Test", funcapi.FieldTypeString, "", func(a query.Artifact) any {
		if a.Kind != model.ReportCapture && a.Test.ID == "" {
			return "Test not recorded"
		}
		return a.Test.Title
	}),
	column("label", "Label", funcapi.FieldTypeString, "", func(a query.Artifact) any {
		switch a.Kind {
		case model.ReportCapture:
			return "Lighthouse report"
		case model.TraceCapture:
			return "Trace"
		default:
			return "Screenshot"
		}
	}),
}

func (h *Handler) artifact(
	ctx context.Context,
	args map[string]string,
	response map[string]any,
) *funcapi.FunctionResponse {
	if args["job_id"] == "" || args["run_id"] == "" {
		return funcapi.ErrorResponse(400, "job_id and run_id are required")
	}
	evidence, err := h.deps.Evidence(ctx, args["job_id"], args["run_id"], args["artifact_id"])
	if errors.Is(err, query.ErrExpired) {
		return funcapi.RawResponse(map[string]any{
			"status": 404, "errorMessage": err.Error(), "capture_state": evidence.CaptureState,
			"job_id": evidence.JobID, "run_id": evidence.RunID, "vnode_guid": evidence.VnodeGUID,
		})
	}
	if err != nil {
		return functionError(err)
	}
	response["capture_state"] = evidence.CaptureState
	response["run_id"] = evidence.RunID
	response["job_id"] = evidence.JobID
	response["vnode_guid"] = evidence.VnodeGUID
	rows := make([][]any, 0, len(evidence.Artifacts))
	for _, item := range evidence.Artifacts {
		rows = append(rows, artifactColumns.Row(item))
	}
	response["data"] = rows
	if args["artifact_id"] != "" {
		response["encoding"] = "base64"
		response["data_base64"] = base64.StdEncoding.EncodeToString(evidence.Body)
	}
	return funcapi.RawResponse(response)
}
