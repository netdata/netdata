// SPDX-License-Identifier: GPL-3.0-or-later

package catofunc

import (
	"context"
	"fmt"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
)

type router struct {
	deps Deps

	handlers map[string]funcapi.MethodHandler
}

func newRouter(deps Deps) *router {
	r := &router{
		deps:     deps,
		handlers: make(map[string]funcapi.MethodHandler),
	}
	r.handlers[TopologyMethodID] = newFuncTopology(r)
	return r
}

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

func Methods(updateEvery int) []funcapi.MethodConfig {
	return []funcapi.MethodConfig{
		topologyMethodConfig(updateEvery),
	}
}

func NewRouter(deps Deps) funcapi.MethodHandler {
	return newRouter(deps)
}
