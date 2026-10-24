// SPDX-License-Identifier: GPL-3.0-or-later

package nativefunc

import (
	"context"
	"errors"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
)

var _ funcapi.RawMethodHandler = (*router)(nil)

type router struct {
	deps    Deps
	methods map[string]bool
}

// NewRouter dispatches raw requests for the given methods to deps. Errors map to
// fixed HTTP statuses and messages that never contain script output.
func NewRouter(deps Deps, methods []funcapi.FunctionConfig) funcapi.MethodHandler {
	r := &router{
		deps:    deps,
		methods: make(map[string]bool, len(methods)),
	}
	for _, method := range methods {
		r.methods[method.ID] = true
	}
	return r
}

func (*router) Cleanup(context.Context) {}

func (*router) MethodParams(context.Context, string) ([]funcapi.ParamConfig, error) { return nil, nil }

func (*router) Handle(context.Context, string, funcapi.ResolvedParams) *funcapi.FunctionResponse {
	return funcapi.ErrorResponse(400, "native Functions require the raw request contract")
}

func (r *router) HandleRaw(ctx context.Context, req funcapi.RawMethodRequest) *funcapi.FunctionResponse {
	if !r.methods[req.Method] {
		return funcapi.ErrorResponse(404, "unknown native Function")
	}
	result, err := r.deps.ExecuteFunction(ctx, req)
	switch {
	case err == nil:
		return result
	case errors.Is(err, context.DeadlineExceeded):
		return funcapi.ErrorResponse(504, "native Function timed out")
	case errors.Is(err, context.Canceled):
		return funcapi.ErrorResponse(499, "native Function canceled")
	default:
		return funcapi.ErrorResponse(502, "native Function execution failed")
	}
}
