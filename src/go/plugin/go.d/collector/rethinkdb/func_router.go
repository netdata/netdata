// SPDX-License-Identifier: GPL-3.0-or-later

package rethinkdb

import (
	"context"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

// funcRouter routes method calls to appropriate function handlers.
type funcRouter struct {
	collector *Collector

	runningQueries *funcRunningQueries
}

func newFuncRouter(c *Collector) *funcRouter {
	r := &funcRouter{collector: c}
	r.runningQueries = newFuncRunningQueries(r)
	return r
}

// Compile-time interface check.
var _ funcapi.MethodHandler = (*funcRouter)(nil)

func (r *funcRouter) MethodParams(ctx context.Context, method string) ([]funcapi.ParamConfig, error) {
	switch method {
	case runningQueriesMethodID:
		return r.runningQueries.MethodParams(ctx, method)
	default:
		return nil, nil
	}
}

func (r *funcRouter) Handle(ctx context.Context, method string, params funcapi.ResolvedParams) *funcapi.FunctionResponse {
	if r.collector.rdb == nil {
		conn, err := r.collector.newConn(r.collector.Config)
		if err != nil {
			return funcapi.UnavailableResponse("collector is still initializing, please retry in a few seconds")
		}
		r.collector.rdb = conn
	}

	switch method {
	case runningQueriesMethodID:
		return r.runningQueries.Handle(ctx, method, params)
	default:
		return funcapi.NotFoundResponse(method)
	}
}

func rethinkdbMethods() []module.MethodConfig {
	var sortOptions []funcapi.ParamOption
	for _, col := range rethinkRunningColumns {
		if !col.IsSortOption {
			continue
		}
		sortDir := funcapi.FieldSortDescending
		sortOptions = append(sortOptions, funcapi.ParamOption{
			ID:      col.Name,
			Column:  col.Name,
			Name:    col.SortLabel,
			Sort:    &sortDir,
			Default: col.IsDefaultSort,
		})
	}
	return []module.MethodConfig{
		{
			UpdateEvery:  10,
			ID:           runningQueriesMethodID,
			Name:         "Running Queries",
			Help:         "Currently running queries from rethinkdb.jobs. WARNING: Query text may contain unmasked literals (potential PII).",
			RequireCloud: true,
			RequiredParams: []funcapi.ParamConfig{{
				ID:         "__sort",
				Name:       "Filter By",
				Help:       "Select the primary sort column",
				Selection:  funcapi.ParamSelect,
				Options:    sortOptions,
				UniqueView: true,
			}},
		},
	}
}

func rethinkdbFunctionHandler(job *module.Job) funcapi.MethodHandler {
	c, ok := job.Module().(*Collector)
	if !ok {
		return nil
	}
	return c.funcRouter
}
