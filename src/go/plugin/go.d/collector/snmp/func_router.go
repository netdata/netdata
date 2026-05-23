// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"context"
	"fmt"
	"sync"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

// funcRouter routes method calls to appropriate function handlers.
type funcRouter struct {
	ifaceCache *ifaceCache

	mu       sync.RWMutex
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
	for _, h := range collectorSpecificFunctionHandlers() {
		if h.methodID != "" {
			r.registerHandler(h.methodID, h.handler)
		}
	}
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
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.handlers == nil {
		r.handlers = make(map[string]funcapi.MethodHandler)
	}
	r.handlers[method] = handler
}

// Compile-time interface check.
var _ funcapi.MethodHandler = (*funcRouter)(nil)

func (r *funcRouter) MethodParams(ctx context.Context, method string) ([]funcapi.ParamConfig, error) {
	r.mu.RLock()
	h, ok := r.handlers[method]
	r.mu.RUnlock()

	if ok {
		return h.MethodParams(ctx, method)
	}
	return nil, fmt.Errorf("unknown method: %s", method)
}

func (r *funcRouter) Handle(ctx context.Context, method string, params funcapi.ResolvedParams) *funcapi.FunctionResponse {
	r.mu.RLock()
	h, ok := r.handlers[method]
	r.mu.RUnlock()

	if ok {
		return h.Handle(ctx, method, params)
	}
	return funcapi.NotFoundResponse(method)
}

func (r *funcRouter) Cleanup(ctx context.Context) {
	r.mu.RLock()
	handlers := make([]funcapi.MethodHandler, 0, len(r.handlers))
	for _, h := range r.handlers {
		handlers = append(handlers, h)
	}
	r.mu.RUnlock()

	for _, h := range handlers {
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
