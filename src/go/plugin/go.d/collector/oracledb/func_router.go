// SPDX-License-Identifier: GPL-3.0-or-later

package oracledb

import (
	"context"
	"fmt"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

// funcRouter routes method calls to appropriate function handlers.
// Uses shared SQL connection from Collector (OracleDB metrics collection uses SQL).
type funcRouter struct {
	collector *Collector // for shared DB access and config

	handlers map[string]funcapi.MethodHandler
}

func newFuncRouter(c *Collector) *funcRouter {
	r := &funcRouter{
		collector: c,
		handlers:  make(map[string]funcapi.MethodHandler),
	}
	r.handlers[topQueriesMethodID] = newFuncTopQueries(r)
	r.handlers[runningQueriesMethodID] = newFuncRunningQueries(r)
	return r
}

// Compile-time interface check.
var _ funcapi.MethodHandler = (*funcRouter)(nil)

func (r *funcRouter) MethodParams(ctx context.Context, method string) ([]funcapi.ParamConfig, error) {
	if h, ok := r.handlers[method]; ok {
		return h.MethodParams(ctx, method)
	}
	return nil, fmt.Errorf("unknown method: %s", method)
}

func (r *funcRouter) Handle(ctx context.Context, method string, params funcapi.ResolvedParams) *funcapi.FunctionResponse {
	if h, ok := r.handlers[method]; ok {
		return h.Handle(ctx, method, params)
	}
	return funcapi.NotFoundResponse(method)
}

func (r *funcRouter) Cleanup(ctx context.Context) {
	for _, h := range r.handlers {
		h.Cleanup(ctx)
	}
}

func (r *funcRouter) topQueriesLimit() int {
	return r.collector.topQueriesLimit()
}

func oracledbMethods() []funcapi.MethodConfig {
	return []funcapi.MethodConfig{
		topQueriesMethodConfig(),
		runningQueriesMethodConfig(),
	}
}

func oracledbFunctionHandler(job collectorapi.RuntimeJob) funcapi.MethodHandler {
	c, ok := job.Collector().(*Collector)
	if !ok {
		return nil
	}
	return c.funcRouter
}
