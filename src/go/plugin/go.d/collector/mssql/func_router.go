// SPDX-License-Identifier: GPL-3.0-or-later

package mssql

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

// mssqlFunctionTimeout is one Function's query budget together with its `functions.<option>`
// config key, so a timeout response can name the setting that controls it.
type mssqlFunctionTimeout struct {
	option string
	value  time.Duration
}

// contextError maps a context failure to the Function response for it, or returns nil
// when err is not a timeout or cancellation.
func (t mssqlFunctionTimeout) contextError(ctx context.Context, err error) *funcapi.FunctionResponse {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return funcapi.ErrorResponse(
			504,
			"%s query timed out; Function timeout is %s (functions.%s.timeout), but the request deadline may be shorter",
			t.option,
			t.value,
			t.option,
		)
	}
	if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
		return funcapi.ErrorResponse(499, "query canceled")
	}
	return nil
}

// runFunction bounds a Function request by its own timeout and resolves the SQL engine
// edition before collect runs edition-specific SQL.
func (c *Collector) runFunction(
	ctx context.Context,
	timeout mssqlFunctionTimeout,
	collect func(context.Context) *funcapi.FunctionResponse,
) *funcapi.FunctionResponse {
	if c.functionDB == nil {
		return funcapi.UnavailableResponse("collector is still initializing, please retry in a few seconds")
	}
	queryCtx, cancel := context.WithTimeout(ctx, timeout.value)
	defer cancel()
	if _, err := c.ensureEngineEdition(queryCtx); err != nil {
		if response := timeout.contextError(queryCtx, err); response != nil {
			return response
		}
		return funcapi.ErrorResponse(500, "failed to detect SQL engine edition: %v", err)
	}
	return collect(queryCtx)
}

func (c *Collector) xeReadPermission() string {
	if c.isAzureSQLDatabase() {
		return "VIEW DATABASE PERFORMANCE STATE"
	}
	if c.currentMajorVersion() >= 16 {
		return "VIEW SERVER PERFORMANCE STATE"
	}
	return "VIEW SERVER STATE"
}

// funcRouter routes method calls to appropriate function handlers.
type funcRouter struct {
	collector *Collector

	handlers map[string]funcapi.MethodHandler
}

func newFuncRouter(c *Collector) *funcRouter {
	r := &funcRouter{
		collector: c,
		handlers:  make(map[string]funcapi.MethodHandler),
	}
	r.handlers[topQueriesMethodID] = newFuncTopQueries(r)
	r.handlers[deadlockInfoMethodID] = newFuncDeadlockInfo(r)
	r.handlers[errorInfoMethodID] = newFuncErrorInfo(r)
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

func mssqlMethods() []funcapi.FunctionConfig {
	return []funcapi.FunctionConfig{
		topQueriesFunctionConfig(),
		deadlockInfoFunctionConfig(),
		errorInfoFunctionConfig(),
	}
}

func mssqlFunctionHandler(job collectorapi.RuntimeJob) funcapi.MethodHandler {
	c, ok := job.Collector().(*Collector)
	if !ok {
		return nil
	}
	return c.funcRouter
}
