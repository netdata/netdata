// SPDX-License-Identifier: GPL-3.0-or-later

package netflow

import (
	"context"
	"os"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
)

// Compile-time interface check.
var _ funcapi.MethodHandler = (*funcFlows)(nil)

type funcFlows struct {
	router *funcRouter
}

func newFuncFlows(r *funcRouter) *funcFlows {
	return &funcFlows{router: r}
}

const flowsMethodID = "flows"

func flowsMethodConfig() funcapi.MethodConfig {
	return funcapi.MethodConfig{
		ID:           flowsMethodID,
		Name:         "Flows",
		UpdateEvery:  10,
		Help:         "NetFlow/IPFIX/sFlow flow summary data",
		RequireCloud: true,
		ResponseType: "flows",
	}
}

func (f *funcFlows) MethodParams(_ context.Context, method string) ([]funcapi.ParamConfig, error) {
	if method != flowsMethodID {
		return nil, nil
	}
	return nil, nil
}

func (f *funcFlows) Cleanup(_ context.Context) {}

func (f *funcFlows) Handle(_ context.Context, method string, _ funcapi.ResolvedParams) *funcapi.FunctionResponse {
	if method != flowsMethodID {
		return funcapi.NotFoundResponse(method)
	}

	collector := f.router.collector
	if collector == nil || collector.aggregator == nil {
		return funcapi.UnavailableResponse("flow data not available yet, please retry after data collection")
	}

	agentID := resolveFlowAgentID(collector)
	data := collector.aggregator.Snapshot(agentID)
	if len(data.Buckets) == 0 {
		return funcapi.UnavailableResponse("flow data not available yet, please retry after data collection")
	}

	return &funcapi.FunctionResponse{
		Status:       200,
		Help:         "NetFlow/IPFIX/sFlow flow summary data",
		ResponseType: "flows",
		Data:         data,
	}
}

func resolveFlowAgentID(c *Collector) string {
	if c.Vnode != "" {
		return c.Vnode
	}
	if hostname, err := os.Hostname(); err == nil {
		return hostname
	}
	return ""
}
