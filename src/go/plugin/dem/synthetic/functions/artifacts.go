// SPDX-License-Identifier: GPL-3.0-or-later
package functions

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/artifacts"
)

var artifactsMethod = method{
	id:     "synthetics-artifacts",
	title:  "Retained synthetic captures",
	help:   "Retained capture sets, newest publication first, independently of run history or active monitors. Optional job_id filters rows; storage facts remain global. before accepts the previous next cursor; limit is 1–2000. Retention can remove sets between pages. Metadata does not verify payload integrity. Storage sizes are logical file lengths; protected workspace and pending deletion are outside the retained-capture allowance.",
	sort:   "published_us",
	params: []string{"job_id", "before", "limit"},
	columns: columns(
		column{"run_id", "Run", funcapi.FieldTypeString, ""},
		column{"job_id", "Monitor", funcapi.FieldTypeString, ""},
		column{"published_us", "Published", funcapi.FieldTypeTimestamp, ""},
		column{"bytes", "Set size", funcapi.FieldTypeInteger, "bytes"},
		column{"artifact_count", "Artifacts", funcapi.FieldTypeInteger, ""},
	),
}

func (h *Handler) artifacts(
	ctx context.Context,
	args map[string]string,
	response map[string]any,
) *funcapi.FunctionResponse {
	limit, err := rowLimit(args["limit"])
	if err != nil {
		return funcapi.ErrorResponse(400, "%v", err)
	}
	cursor, err := parseArtifactCursor(args["before"])
	if err != nil {
		return funcapi.ErrorResponse(400, "%v", err)
	}
	page, err := h.deps.Catalogue(ctx, artifacts.Filter{
		JobID:  args["job_id"],
		Before: cursor,
		Limit:  limit,
	})
	if err != nil {
		return functionError(err)
	}
	rows := make([][]any, 0, len(page.Sets))
	for _, set := range page.Sets {
		rows = append(rows, []any{set.RunID, set.JobID, set.PublishedUS, set.Bytes, set.Artifacts})
	}
	response["data"] = rows
	response["limit"] = limit
	response["truncated"] = page.Next != nil
	if page.Next != nil {
		response["next"] = strconv.FormatInt(page.Next.PublishedUS, 10) + "." + page.Next.RunID
	}
	response["storage"] = struct {
		artifacts.Status
		PolicyError string `json:"policy_error,omitempty"`
	}{Status: page.Status, PolicyError: page.PolicyError}
	return funcapi.RawResponse(response)
}

// The cursor is a decimal microsecond timestamp and a lowercase hexadecimal run
// ID. It needs no quoting or escaping in Function command arguments.
func parseArtifactCursor(value string) (*artifacts.Cursor, error) {
	if value == "" {
		return nil, nil
	}
	stamp, runID, ok := strings.Cut(value, ".")
	published, err := strconv.ParseInt(stamp, 10, 64)
	valid := ok && err == nil && published > 0 && strconv.FormatInt(published, 10) == stamp && len(runID) == 32
	for _, char := range runID {
		valid = valid && (char >= '0' && char <= '9' || char >= 'a' && char <= 'f')
	}
	if !valid {
		return nil, errors.New("before must be a returned artifact catalogue cursor")
	}
	return &artifacts.Cursor{
		PublishedUS: published,
		RunID:       runID,
	}, nil
}
