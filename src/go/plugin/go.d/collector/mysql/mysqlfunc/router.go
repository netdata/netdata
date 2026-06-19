// SPDX-License-Identifier: GPL-3.0-or-later

package mysqlfunc

import (
	"context"
	"fmt"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
)

// funcRouter routes method calls to appropriate function handlers.
type router struct {
	deps Deps
	cfg  FunctionsConfig
	log  *logger.Logger

	handlers map[string]funcapi.MethodHandler
}

func newRouter(deps Deps, log *logger.Logger, cfg FunctionsConfig) *router {
	r := &router{
		deps:     deps,
		cfg:      cfg,
		log:      log,
		handlers: make(map[string]funcapi.MethodHandler),
	}
	r.handlers[topQueriesMethodID] = newFuncTopQueries(r)
	r.handlers[deadlockInfoMethodID] = newFuncDeadlockInfo(r)
	r.handlers[errorInfoMethodID] = newFuncErrorInfo(r)
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

func Methods() []funcapi.MethodConfig {
	return []funcapi.MethodConfig{
		topQueriesMethodConfig(),
		deadlockInfoMethodConfig(),
		errorInfoMethodConfig(),
	}
}

func NewRouter(deps Deps, log *logger.Logger, cfg FunctionsConfig) funcapi.MethodHandler {
	return newRouter(deps, log, cfg)
}
