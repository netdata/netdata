// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"context"
	"fmt"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

// funcRouter routes method calls to appropriate function handlers.
type funcRouter struct {
	ifaceCache *ifaceCache

	handlers map[string]funcapi.MethodHandler
}

type registeredSNMPFunction struct {
	methodID string
	handler  funcapi.MethodHandler
}

func newFuncRouter(ifaceCache *ifaceCache, extraHandlers ...registeredSNMPFunction) *funcRouter {
	r := &funcRouter{
		ifaceCache: ifaceCache,
		handlers:   make(map[string]funcapi.MethodHandler),
	}
	r.registerHandler(ifacesMethodID, newFuncInterfaces(r))
	for _, h := range extraHandlers {
		if h.methodID != "" {
			r.registerHandler(h.methodID, h.handler)
		}
	}
	addTopologyFunctionHandler(r.handlers)
	return r
}

func (r *funcRouter) registerHandler(method string, handler funcapi.MethodHandler) {
	if r == nil || handler == nil {
		return
	}
	if r.handlers == nil {
		r.handlers = make(map[string]funcapi.MethodHandler)
	}
	r.handlers[method] = handler
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

func snmpBaseMethods() []funcapi.MethodConfig {
	methods := []funcapi.MethodConfig{
		ifacesMethodConfig(),
	}
	methods = append(methods, collectorSpecificMethodConfigs()...)
	return appendTopologyMethodConfig(methods)
}

func snmpFunctionHandler(job collectorapi.RuntimeJob) funcapi.MethodHandler {
	c, ok := job.Collector().(*Collector)
	if !ok {
		return nil
	}
	return c.funcRouter
}
