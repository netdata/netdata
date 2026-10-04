// SPDX-License-Identifier: GPL-3.0-or-later

// Package functions presents synthetic queries through Netdata Functions.
package functions

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	model "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/query"
)

// Deps exposes snapshots and immutable retained evidence, without browser
// admission or a second desired-configuration registry.
type Deps interface {
	Jobs(time.Time) []query.Job
	ArtifactStats() query.ArtifactStats
	Runs(context.Context, query.RunFilter) (query.RunPage, error)
	Run(context.Context, string, string) (model.Run, error)
	Evidence(context.Context, string, string, string) (query.Evidence, error)
}

type Handler struct{ deps Deps }

func New(deps Deps) *Handler {
	return &Handler{
		deps: deps,
	}
}

func (h *Handler) Cleanup(context.Context) {}

func (h *Handler) MethodParams(context.Context, string) ([]funcapi.ParamConfig, error) {
	return nil, nil
}

func (h *Handler) Handle(context.Context, string, funcapi.ResolvedParams) *funcapi.FunctionResponse {
	return funcapi.ErrorResponse(400, "raw synthetic request required")
}

func (h *Handler) HandleRaw(ctx context.Context, req funcapi.RawMethodRequest) *funcapi.FunctionResponse {
	var spec *method
	for i := range methods {
		if methods[i].id == req.Method {
			spec = &methods[i]
			break
		}
	}
	if spec == nil {
		return funcapi.NotFoundResponse(req.Method)
	}
	// Metadata discovery never queries active jobs, history or artifact storage.
	if req.Info {
		return &funcapi.FunctionResponse{
			Status:            200,
			Help:              spec.help,
			Columns:           spec.columns,
			DefaultSortColumn: spec.sort,
		}
	}
	if !permitted(req.Permissions, 0x1b) {
		return funcapi.ErrorResponse(403, "member permissions are required")
	}
	if len(req.Payload) > 0 {
		return funcapi.ErrorResponse(400, "this read-only Function does not accept a payload")
	}
	args, err := parseArgs(req.Args, spec.params)
	if err != nil {
		return funcapi.ErrorResponse(400, "%v", err)
	}
	if req.Method == "synthetics-artifact" && args["artifact_id"] != "" && !permitted(req.Permissions, 0x7b) {
		return funcapi.ErrorResponse(403, "administrator permissions are required for artifact bytes")
	}
	if err := ctx.Err(); err != nil {
		return functionError(err)
	}
	if h.deps == nil {
		return funcapi.UnavailableResponse("synthetic observations are unavailable")
	}
	response := map[string]any{
		"status":              200,
		"type":                "table",
		"has_history":         spec.history,
		"update_every":        10,
		"help":                spec.help,
		"columns":             spec.columns,
		"data":                [][]any{},
		"default_sort_column": spec.sort,
		"accepted_params":     spec.params,
	}
	switch req.Method {
	case "synthetics-checks":
		return h.checks(ctx, args, response)
	case "synthetics-runs":
		return h.runs(ctx, args, response)
	case "synthetics-run":
		return h.run(ctx, args, response)
	case "synthetics-artifact":
		if args["job_id"] == "" || args["run_id"] == "" {
			return funcapi.ErrorResponse(400, "job_id and run_id are required")
		}
		return h.artifact(ctx, args, response)
	}

	return funcapi.RawResponse(response)
}

func timestamp(value int64) any {
	if value == 0 {
		return nil
	}
	return value
}

func number(value *float64) any {
	if value == nil || math.IsNaN(*value) || math.IsInf(*value, 0) {
		return nil
	}
	return *value
}

func functionError(err error) *funcapi.FunctionResponse {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return funcapi.ErrorResponse(499, "request cancelled")
	case errors.Is(err, query.ErrNotFound):
		return funcapi.ErrorResponse(404, "requested evidence is unavailable")
	case errors.Is(err, query.ErrArtifactNotRecorded):
		return funcapi.ErrorResponse(404, "%v", err)
	case errors.Is(err, query.ErrTooLarge):
		return funcapi.ErrorResponse(413, "artifact exceeds the fetch limit")
	case errors.Is(err, query.ErrInvalidID):
		return funcapi.ErrorResponse(400, "invalid run or artifact identifier")
	default:
		return funcapi.InternalErrorResponse("synthetic evidence could not be read")
	}
}
