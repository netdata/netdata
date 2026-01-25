// SPDX-License-Identifier: GPL-3.0-or-later

package oracledb

import (
	"context"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

// funcRouter routes method calls to appropriate function handlers.
// Uses shared SQL connection from Collector (OracleDB metrics collection uses SQL).
type funcRouter struct {
	collector *Collector // for shared DB access and config

	topQueries     *funcTopQueries
	runningQueries *funcRunningQueries
}

func newFuncRouter(c *Collector) *funcRouter {
	r := &funcRouter{collector: c}
	r.topQueries = newFuncTopQueries(r)
	r.runningQueries = newFuncRunningQueries(r)
	return r
}

// Compile-time interface check.
var _ funcapi.MethodHandler = (*funcRouter)(nil)

func (r *funcRouter) MethodParams(ctx context.Context, method string) ([]funcapi.ParamConfig, error) {
	switch method {
	case "top-queries":
		return r.topQueries.MethodParams(ctx, method)
	case "running-queries":
		return r.runningQueries.MethodParams(ctx, method)
	default:
		return nil, nil
	}
}

func (r *funcRouter) Handle(ctx context.Context, method string, params funcapi.ResolvedParams) *funcapi.FunctionResponse {
	switch method {
	case "top-queries":
		return r.topQueries.Handle(ctx, method, params)
	case "running-queries":
		return r.runningQueries.Handle(ctx, method, params)
	default:
		return funcapi.NotFoundResponse(method)
	}
}

func (r *funcRouter) topQueriesLimit() int {
	if r.collector.TopQueriesLimit > 0 {
		return r.collector.TopQueriesLimit
	}
	return 500
}

func oracledbMethods() []module.MethodConfig {
	return []module.MethodConfig{
		{
			UpdateEvery:  10,
			ID:           "top-queries",
			Name:         "Top Queries",
			Help:         "Top SQL statements from V$SQLSTATS. WARNING: Query text may contain unmasked literals (potential PII).",
			RequireCloud: true,
			RequiredParams: []funcapi.ParamConfig{
				buildSortParam(topQueriesColumns),
			},
		},
		{
			UpdateEvery:  10,
			ID:           "running-queries",
			Name:         "Running Queries",
			Help:         "Currently running SQL statements from V$SESSION. WARNING: Query text may contain unmasked literals (potential PII).",
			RequireCloud: true,
			RequiredParams: []funcapi.ParamConfig{
				buildSortParam(runningQueriesColumns),
			},
		},
	}
}

func oracledbFunctionHandler(job *module.Job) funcapi.MethodHandler {
	c, ok := job.Module().(*Collector)
	if !ok {
		return nil
	}
	return c.funcRouter
}

// sortableColumn is an interface for columns that can be used in sort options.
type sortableColumn interface {
	isSortOption() bool
	sortLabel() string
	isDefaultSort() bool
	name() string
}

func (c topQueriesColumn) isSortOption() bool  { return c.IsSortOption }
func (c topQueriesColumn) sortLabel() string   { return c.SortLabel }
func (c topQueriesColumn) isDefaultSort() bool { return c.IsDefaultSort }
func (c topQueriesColumn) name() string        { return c.Name }

func (c runningQueriesColumn) isSortOption() bool  { return c.IsSortOption }
func (c runningQueriesColumn) sortLabel() string   { return c.SortLabel }
func (c runningQueriesColumn) isDefaultSort() bool { return c.IsDefaultSort }
func (c runningQueriesColumn) name() string        { return c.Name }

func buildSortParam[T sortableColumn](cols []T) funcapi.ParamConfig {
	var options []funcapi.ParamOption
	sortDir := funcapi.FieldSortDescending
	for _, col := range cols {
		if !col.isSortOption() {
			continue
		}
		opt := funcapi.ParamOption{
			ID:     col.name(),
			Column: col.name(),
			Name:   col.sortLabel(),
			Sort:   &sortDir,
		}
		if col.isDefaultSort() {
			opt.Default = true
		}
		options = append(options, opt)
	}
	return funcapi.ParamConfig{
		ID:         "__sort",
		Name:       "Filter By",
		Help:       "Select the primary sort column",
		Selection:  funcapi.ParamSelect,
		Options:    options,
		UniqueView: true,
	}
}
