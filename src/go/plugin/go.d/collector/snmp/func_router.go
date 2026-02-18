// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"context"
	"fmt"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

// funcRouter routes method calls to appropriate function handlers.
type funcRouter struct {
	ifaceCache    *ifaceCache
	topologyCache *topologyCache

	handlers map[string]funcapi.MethodHandler
}

func newFuncRouter(cache *ifaceCache, topoCache *topologyCache) *funcRouter {
	r := &funcRouter{
		ifaceCache:    cache,
		topologyCache: topoCache,
		handlers:      make(map[string]funcapi.MethodHandler),
	}
	r.handlers[ifacesMethodID] = newFuncInterfaces(r)
	r.handlers[topologyMethodID] = newFuncTopology(r)
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

func snmpMethods() []funcapi.MethodConfig {
	return []funcapi.MethodConfig{
		ifacesMethodConfig(),
		topologyMethodConfig(),
	}
}

func snmpFunctionHandler(job *module.Job) funcapi.MethodHandler {
	c, ok := job.Module().(*Collector)
	if !ok {
		return nil
	}
	return c.funcRouter
}
