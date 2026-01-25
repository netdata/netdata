// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"context"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

// funcRouter routes method calls to appropriate function handlers.
type funcRouter struct {
	ifaceCache *ifaceCache

	interfaces *funcInterfaces
}

func newFuncRouter(cache *ifaceCache) *funcRouter {
	r := &funcRouter{ifaceCache: cache}
	r.interfaces = newFuncInterfaces(r)
	return r
}

// Compile-time interface check.
var _ funcapi.MethodHandler = (*funcRouter)(nil)

func (r *funcRouter) MethodParams(ctx context.Context, method string) ([]funcapi.ParamConfig, error) {
	switch method {
	case "interfaces":
		return r.interfaces.MethodParams(ctx, method)
	default:
		return nil, nil
	}
}

func (r *funcRouter) Handle(ctx context.Context, method string, params funcapi.ResolvedParams) *funcapi.FunctionResponse {
	switch method {
	case "interfaces":
		return r.interfaces.Handle(ctx, method, params)
	default:
		return funcapi.NotFoundResponse(method)
	}
}

func snmpMethods() []module.MethodConfig {
	return []module.MethodConfig{
		{
			ID:          "interfaces",
			Name:        "Network Interfaces",
			Help:        "Network interface traffic and status metrics",
			UpdateEvery: 10,
			RequiredParams: []funcapi.ParamConfig{{
				ID:        "if_type_group",
				Name:      "Type Group",
				Help:      "Filter by interface type group",
				Selection: funcapi.ParamSelect,
				Options: []funcapi.ParamOption{
					{ID: "ethernet", Name: "Ethernet", Default: true},
					{ID: "aggregation", Name: "Aggregation"},
					{ID: "virtual", Name: "Virtual"},
					{ID: "other", Name: "Other"},
				},
			}},
		},
	}
}

func snmpFunctionHandler(job *module.Job) funcapi.MethodHandler {
	c, ok := job.Module().(*Collector)
	if !ok {
		return nil
	}
	return c.funcRouter
}
