// SPDX-License-Identifier: GPL-3.0-or-later

package mssql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

func mssqlFunctionContextError(ctx context.Context, err error) *funcapi.FunctionResponse {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return funcapi.ErrorResponse(504, "query timed out")
	}
	if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
		return funcapi.ErrorResponse(499, "query canceled")
	}
	return nil
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

// The target query joins live sessions, so it also excludes stopped sessions.
func (c *Collector) mssqlRingBufferAvailable(ctx context.Context, sessionName string) (bool, error) {
	query := queryMSSQLErrorSessionHasRingBuffer
	if c.isAzureSQLDatabase() {
		query = queryMSSQLErrorDatabaseSessionHasRingBuffer
	}
	var count int
	err := c.db.QueryRowContext(ctx, query, sql.Named("sessionName", sessionName)).Scan(&count)
	return count > 0, err
}
