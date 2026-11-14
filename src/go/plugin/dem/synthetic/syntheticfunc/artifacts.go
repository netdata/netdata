// SPDX-License-Identifier: GPL-3.0-or-later
package syntheticfunc

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem/internal/rawfunc"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/artifacts"
)

var artifactsMethod = method{
	Method: rawfunc.Method{
		ID:          "synthetics-artifacts",
		Title:       "Retained synthetic captures",
		Help:        "Retained capture sets, newest publication first, independently of run history or active monitors. Optional job_id filters rows; storage facts remain global. before accepts the previous next cursor; limit is 1–" + strconv.Itoa(rawfunc.MaxRows) + ". Retention can remove sets between pages. Metadata does not verify payload integrity. Storage sizes are logical file lengths; protected workspace and pending deletion are outside the retained-capture allowance. Optional vnode selects a configured name; vnode_guid selects a recorded canonical GUID instead. Omit both to select all sites; the selectors are mutually exclusive. Blank Vnode GUID cells are unassigned.",
		Sort:        "published_us",
		UpdateEvery: 10,
		Params:      []string{"job_id", "before", "limit", "vnode", "vnode_guid"},
		Columns:     artifactsColumns.Build(),
	},
	serve: (*Handler).artifacts,
}

var artifactsColumns = rawfunc.Columns[artifacts.Set]{
	column("run_id", "Run", funcapi.FieldTypeString, "", func(s artifacts.Set) any { return s.RunID }),
	column("job_id", "Monitor", funcapi.FieldTypeString, "", func(s artifacts.Set) any { return s.JobID }),
	column("published_us", "Published", funcapi.FieldTypeTimestamp, "", func(s artifacts.Set) any { return s.PublishedUS }),
	column("bytes", "Set size", funcapi.FieldTypeInteger, "bytes", func(s artifacts.Set) any { return s.Bytes }),
	column("artifact_count", "Artifacts", funcapi.FieldTypeInteger, "", func(s artifacts.Set) any { return s.Artifacts }),
	column("vnode_guid", "Vnode GUID", funcapi.FieldTypeString, "", func(s artifacts.Set) any { return s.VnodeGUID }),
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
		JobID:     args["job_id"],
		VnodeGUID: args["vnode_guid"],
		Before:    cursor,
		Limit:     limit,
	})
	if err != nil {
		return functionError(err)
	}
	rows := make([][]any, 0, len(page.Sets))
	for _, set := range page.Sets {
		rows = append(rows, artifactsColumns.Row(set))
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
