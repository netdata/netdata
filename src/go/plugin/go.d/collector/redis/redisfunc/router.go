// SPDX-License-Identifier: GPL-3.0-or-later

package redisfunc

import (
	"context"
	"fmt"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
)

// router routes method calls to the matching Function handler.
type router struct {
	deps Deps
	cfg  FunctionsConfig

	handlers map[string]funcapi.MethodHandler
}

func newRouter(deps Deps, cfg FunctionsConfig) *router {
	r := &router{
		deps:     deps,
		cfg:      cfg,
		handlers: make(map[string]funcapi.MethodHandler),
	}
	r.handlers[topQueriesMethodID] = newFuncTopQueries(r)
	return r
}

// Compile-time interface check.
var _ funcapi.MethodHandler = (*router)(nil)

func (r *router) MethodParams(ctx context.Context, method string) ([]funcapi.ParamConfig, error) {
	if h, ok := r.handlers[method]; ok {
		return h.MethodParams(ctx, method)
	}
	return nil, fmt.Errorf("unknown method: %s", method)
}

func (r *router) Handle(ctx context.Context, method string, params funcapi.ResolvedParams) *funcapi.FunctionResponse {
	if h, ok := r.handlers[method]; ok {
		return h.Handle(ctx, method, params)
	}
	return funcapi.NotFoundResponse(method)
}

func (r *router) Cleanup(ctx context.Context) {
	for _, h := range r.handlers {
		h.Cleanup(ctx)
	}
}

// Methods returns the Functions every redis job exposes.
func Methods() []funcapi.FunctionConfig {
	return []funcapi.FunctionConfig{
		topQueriesFunctionConfig(),
	}
}

// NewRouter returns the method handler serving Methods for one job.
func NewRouter(deps Deps, cfg FunctionsConfig) funcapi.MethodHandler {
	return newRouter(deps, cfg)
}
