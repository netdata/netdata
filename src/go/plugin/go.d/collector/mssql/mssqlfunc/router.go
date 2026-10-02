// SPDX-License-Identifier: GPL-3.0-or-later

package mssqlfunc

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
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
func (r *router) runFunction(
	ctx context.Context,
	timeout mssqlFunctionTimeout,
	collect func(context.Context) *funcapi.FunctionResponse,
) *funcapi.FunctionResponse {
	if r.deps.DB() == nil {
		return funcapi.UnavailableResponse("collector is still initializing, please retry in a few seconds")
	}
	queryCtx, cancel := context.WithTimeout(ctx, timeout.value)
	defer cancel()
	if err := r.deps.EnsureServerInfo(queryCtx); err != nil {
		if response := timeout.contextError(queryCtx, err); response != nil {
			return response
		}
		return funcapi.ErrorResponse(500, "failed to detect SQL engine edition: %v", err)
	}
	return collect(queryCtx)
}

func (r *router) xeReadPermission() string {
	if r.deps.ServerInfo().AzureSQLDatabase {
		return "VIEW DATABASE PERFORMANCE STATE"
	}
	if r.deps.ServerInfo().MajorVersion >= 16 {
		return "VIEW SERVER PERFORMANCE STATE"
	}
	return "VIEW SERVER STATE"
}

// router routes method calls to appropriate function handlers.
type router struct {
	deps Deps
	cfg  FunctionsConfig
	log  *logger.Logger

	// top-queries source discovery caches (per-instance to handle different SQL Server versions).
	// Each probe has its own lock so they cannot block each other on the database round trip.
	queryStoreColsMu      sync.RWMutex
	queryStoreCols        map[string]bool
	queryStoreSupportedMu sync.RWMutex
	queryStoreSupported   *bool // nil until the capability probe has run
	planCacheColsMu       sync.RWMutex
	planCacheCols         map[string]bool

	handlers map[string]funcapi.MethodHandler
}

func NewRouter(deps Deps, log *logger.Logger, cfg FunctionsConfig) funcapi.MethodHandler {
	return newRouter(deps, log, cfg)
}

func newRouter(deps Deps, log *logger.Logger, cfg FunctionsConfig) *router {
	r := &router{
		deps:     deps,
		log:      log,
		cfg:      cfg,
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

func Methods() []funcapi.FunctionConfig {
	return []funcapi.FunctionConfig{
		topQueriesFunctionConfig(),
		deadlockInfoFunctionConfig(),
		errorInfoFunctionConfig(),
	}
}
