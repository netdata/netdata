// SPDX-License-Identifier: GPL-3.0-or-later
package functions

import (
	"context"
	"encoding/base64"
	"errors"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/query"
)

var artifactMethod = method{
	id:     "synthetics-artifact",
	title:  "Synthetic artifacts",
	help:   "Retained capture metadata for one run; file bytes are verified when fetched. Supplying artifact_id requests base64-encoded bytes and requires administrator-equivalent permissions. Screenshots and HTML reports may contain sensitive content. Missing retained files are expired or unavailable; fetches are limited to 5 MiB.",
	sort:   "artifact_id",
	params: []string{"job_id", "run_id", "artifact_id"},
	columns: columns(
		column{
			"artifact_id",
			"Artifact",
			funcapi.FieldTypeString,
			"",
		},
		column{"kind", "Kind", funcapi.FieldTypeString, ""},
		column{"mime", "Content type", funcapi.FieldTypeString, ""},
		column{"bytes", "Size", funcapi.FieldTypeInteger, "bytes"},
		column{"sha256", "SHA-256", funcapi.FieldTypeString, ""},
		column{"availability", "Availability", funcapi.FieldTypeString, ""},
	),
}

func (h *Handler) artifact(
	ctx context.Context,
	args map[string]string,
	response map[string]any,
) *funcapi.FunctionResponse {
	evidence, err := h.deps.Evidence(ctx, args["job_id"], args["run_id"], args["artifact_id"])
	if errors.Is(err, query.ErrExpired) {
		return funcapi.RawResponse(map[string]any{
			"status": 404, "errorMessage": err.Error(), "capture_state": evidence.CaptureState,
			"job_id": evidence.JobID, "run_id": evidence.RunID,
		})
	}
	if err != nil {
		return functionError(err)
	}
	response["capture_state"] = evidence.CaptureState
	response["run_id"] = evidence.RunID
	response["job_id"] = evidence.JobID
	rows := make([][]any, 0, len(evidence.Artifacts))
	for _, item := range evidence.Artifacts {
		rows = append(rows, []any{item.ID, item.Kind, item.MIME, item.Bytes, item.SHA256, item.Availability})
	}
	response["data"] = rows
	if args["artifact_id"] != "" {
		response["encoding"] = "base64"
		response["data_base64"] = base64.StdEncoding.EncodeToString(evidence.Body)
	}
	return funcapi.RawResponse(response)
}
