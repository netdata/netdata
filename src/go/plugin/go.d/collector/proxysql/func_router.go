// SPDX-License-Identifier: GPL-3.0-or-later

package proxysql

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
	case topQueriesMethodID:
		return r.topQueries.MethodParams(ctx, method)
	default:
		return nil, nil
	}
}

func (r *funcRouter) Handle(ctx context.Context, method string, params funcapi.ResolvedParams) *funcapi.FunctionResponse {
	switch method {
	case topQueriesMethodID:
		return r.topQueries.Handle(ctx, method, params)
	default:
		return funcapi.NotFoundResponse(method)
	}
}

func proxysqlMethods() []module.MethodConfig {
	return []module.MethodConfig{
		{
			UpdateEvery:  10,
			ID:           topQueriesMethodID,
			Name:         "Top Queries",
			Help:         "Top SQL queries from ProxySQL query digest stats",
			RequireCloud: true,
			RequiredParams: []funcapi.ParamConfig{{
				ID:         paramSort,
				Name:       "Filter By",
				Help:       "Select the primary sort column",
				Selection:  funcapi.ParamSelect,
				Options:    buildProxySQLSortOptions(proxysqlAllColumns),
				UniqueView: true,
			}},
		},
	}
}

func proxysqlFunctionHandler(job *module.Job) funcapi.MethodHandler {
	c, ok := job.Module().(*Collector)
	if !ok {
		return nil
	}
	return c.funcRouter
}
