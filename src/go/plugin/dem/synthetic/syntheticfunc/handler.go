// SPDX-License-Identifier: GPL-3.0-or-later

// Package syntheticfunc presents synthetic queries through Netdata Functions.
package syntheticfunc

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem/internal/rawfunc"
	model "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/artifacts"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/history"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/query"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/registry"
)

// Deps exposes snapshots and immutable retained evidence, without browser
// admission or a second desired-configuration registry.
type Deps interface {
	Jobs(time.Time) []registry.Job
	Catalogue(context.Context, artifacts.Filter) (query.CataloguePage, error)
	Runs(context.Context, history.RunFilter) (history.RunPage, error)
	Run(context.Context, string, string) (model.Run, error)
	Evidence(context.Context, string, string, string) (query.Evidence, error)
}

type Handler struct {
	deps            Deps
	lookupVnodeGUID func(string) (string, bool)
}

func New(deps Deps, lookupVnodeGUID func(string) (string, bool)) *Handler {
	return &Handler{
		deps:            deps,
		lookupVnodeGUID: lookupVnodeGUID,
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
	i := slices.IndexFunc(methods, func(m method) bool { return m.ID == req.Method })
	if i < 0 {
		return funcapi.NotFoundResponse(req.Method)
	}
	spec := methods[i]
	// Metadata discovery never queries active jobs, history or artifact storage.
	if req.Info {
		return &funcapi.FunctionResponse{
			Status:            200,
			Help:              spec.Help,
			Columns:           spec.Columns,
			DefaultSortColumn: spec.Sort,
		}
	}
	if r := rawfunc.RequireMember(req); r != nil {
		return r
	}
	if r := rawfunc.NoPayload(req); r != nil {
		return r
	}
	args, err := rawfunc.ParseArgs(req, spec.Params)
	if err != nil {
		return funcapi.ErrorResponse(400, "%v", err)
	}
	if spec.ID == artifactMethod.ID && args["artifact_id"] != "" && !rawfunc.Permitted(req, rawfunc.AdminAccess) {
		return funcapi.ErrorResponse(403, "administrator permissions are required for artifact bytes")
	}
	if r := rawfunc.Cancelled(ctx.Err()); r != nil {
		return r
	}
	if err := rawfunc.ResolveVnode(args, h.lookupVnodeGUID); err != nil {
		return funcapi.ErrorResponse(400, "%v", err)
	}
	return spec.serve(h, ctx, args, spec.Table())
}

func timestamp(value int64) any {
	if value == 0 {
		return nil
	}
	return value
}

// number reports an absent measurement as null. Values are finite: the runner
// validates reported ones, and history JSON cannot hold NaN or infinity.
func number(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}

func functionError(err error) *funcapi.FunctionResponse {
	if r := rawfunc.Cancelled(err); r != nil {
		return r
	}
	switch {
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
