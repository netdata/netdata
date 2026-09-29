// SPDX-License-Identifier: GPL-3.0-or-later

package nativefunc

import (
	"context"
	"errors"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
)

// Deps leaves process and stream ownership with the collector.
type Deps interface {
	ExecuteFunction(context.Context, funcapi.RawMethodRequest) (*funcapi.FunctionResponse, error)
}

type router struct {
	deps    Deps
	methods map[string]bool
}

func New(deps Deps, methods []funcapi.FunctionConfig) funcapi.MethodHandler {
	r := &router{
		deps:    deps,
		methods: make(map[string]bool, len(methods)),
	}
	for _, method := range methods {
		r.methods[method.ID] = true
	}
	return r
}

func (*router) Cleanup(context.Context)                                             {}
func (*router) MethodParams(context.Context, string) ([]funcapi.ParamConfig, error) { return nil, nil }
func (*router) Handle(context.Context, string, funcapi.ResolvedParams) *funcapi.FunctionResponse {
	return funcapi.ErrorResponse(400, "native Functions require the raw request contract")
}
func (r *router) HandleRaw(ctx context.Context, req funcapi.RawMethodRequest) *funcapi.FunctionResponse {
	if !r.methods[req.Method] {
		return funcapi.ErrorResponse(404, "unknown native Function")
	}
	result, err := r.deps.ExecuteFunction(ctx, req)
	if err == nil {
		return result
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return funcapi.ErrorResponse(504, "native Function timed out")
	case errors.Is(err, context.Canceled):
		return funcapi.ErrorResponse(499, "native Function canceled")
	default:
		return funcapi.ErrorResponse(502, "native Function execution failed")
	}
}
