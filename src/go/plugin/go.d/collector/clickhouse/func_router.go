// SPDX-License-Identifier: GPL-3.0-or-later

package clickhouse

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

func clickhouseMethods() []module.MethodConfig {
	sortOptions := buildSortOptions(topQueriesColumns)
	return []module.MethodConfig{
		{
			UpdateEvery:  10,
			ID:           "top-queries",
			Name:         "Top Queries",
			Help:         "Top SQL queries from ClickHouse system.query_log",
			RequireCloud: true,
			RequiredParams: []funcapi.ParamConfig{{
				ID:         topQueriesParamSort,
				Name:       "Filter By",
				Help:       "Select the primary sort column",
				Selection:  funcapi.ParamSelect,
				Options:    sortOptions,
				UniqueView: true,
			}},
		},
	}
}

func clickhouseFunctionHandler(job *module.Job) funcapi.MethodHandler {
	c, ok := job.Module().(*Collector)
	if !ok {
		return nil
	}
	return c.funcRouter
}

// buildSortOptions builds sort options for method registration (before handler exists).
func buildSortOptions(cols []topQueriesColumn) []funcapi.ParamOption {
	var sortOptions []funcapi.ParamOption
	sortDir := funcapi.FieldSortDescending
	for _, col := range cols {
		if col.IsSortOption {
			sortOptions = append(sortOptions, funcapi.ParamOption{
				ID:      col.Name,
				Column:  col.Name,
				Name:    "Top queries by " + col.SortLabel,
				Default: col.IsDefaultSort,
				Sort:    &sortDir,
			})
		}
	}
	return sortOptions
}
