// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"context"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
)

// Compile-time interface check.
var _ funcapi.MethodHandler = (*funcTopology)(nil)

type funcTopology struct {
	router *funcRouter
}

func newFuncTopology(r *funcRouter) *funcTopology {
	return &funcTopology{router: r}
}

const topologyMethodID = "topology:snmp"

func topologyMethodConfig() funcapi.MethodConfig {
	return funcapi.MethodConfig{
		ID:           topologyMethodID,
		Name:         "Topology (SNMP)",
		UpdateEvery:  10,
		Help:         "SNMP topology and neighbor discovery data",
		RequireCloud: true,
		ResponseType: "topology",
	}
}

func (f *funcTopology) MethodParams(_ context.Context, method string) ([]funcapi.ParamConfig, error) {
	if method != topologyMethodID {
		return nil, nil
	}
	return nil, nil
}

func (f *funcTopology) Cleanup(_ context.Context) {}

func (f *funcTopology) Handle(_ context.Context, method string, _ funcapi.ResolvedParams) *funcapi.FunctionResponse {
	if method != topologyMethodID {
		return funcapi.NotFoundResponse(method)
	}

	if f.router.topologyCache == nil {
		return funcapi.UnavailableResponse("topology data not available yet, please retry after data collection")
	}

	f.router.topologyCache.mu.RLock()
	data, ok := f.router.topologyCache.snapshot()
	f.router.topologyCache.mu.RUnlock()

	if !ok {
		return funcapi.UnavailableResponse("topology data not available yet, please retry after data collection")
	}

	return &funcapi.FunctionResponse{
		Status:       200,
		Help:         "SNMP topology and neighbor discovery data",
		ResponseType: "topology",
		Data:         data,
	}
}
