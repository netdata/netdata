// SPDX-License-Identifier: GPL-3.0-or-later

package mysql

import (
	"context"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

// funcRouter routes method calls to appropriate function handlers.
type funcRouter struct {
	collector *Collector

	topQueries *funcTopQueries
}

func newFuncRouter(c *Collector) *funcRouter {
	r := &funcRouter{collector: c}
	r.topQueries = newFuncTopQueries(r)
	return r
}

// Compile-time interface check.
var _ funcapi.MethodHandler = (*funcRouter)(nil)

func (r *funcRouter) MethodParams(ctx context.Context, method string) ([]funcapi.ParamConfig, error) {
	switch method {
	case "top-queries":
		return r.topQueries.MethodParams(ctx, method)
	default:
		return nil, nil
	}
}

func (r *funcRouter) Handle(ctx context.Context, method string, params funcapi.ResolvedParams) *funcapi.FunctionResponse {
	switch method {
	case "top-queries":
		return r.topQueries.Handle(ctx, method, params)
	default:
		return funcapi.NotFoundResponse(method)
	}
}

func mysqlMethods() []module.MethodConfig {
	sortOptions := buildTopQueriesSortOptions(topQueriesColumns)

	return []module.MethodConfig{
		{
			UpdateEvery:  10,
			ID:           "top-queries",
			Name:         "Top Queries",
			Help:         "Top SQL queries from performance_schema",
			RequireCloud: true,
			RequiredParams: []funcapi.ParamConfig{
				{
					ID:         topQueriesParamSort,
					Name:       "Filter By",
					Help:       "Select the primary sort column",
					Selection:  funcapi.ParamSelect,
					Options:    sortOptions,
					UniqueView: true,
				},
			},
		},
	}
}

func mysqlFunctionHandler(job *module.Job) funcapi.MethodHandler {
	c, ok := job.Module().(*Collector)
	if !ok {
		return nil
	}
	return c.funcRouter
}
