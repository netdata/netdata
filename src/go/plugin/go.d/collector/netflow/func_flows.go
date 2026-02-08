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

const flowsMethodID = "flows:netflow"

func flowsMethodConfig() funcapi.MethodConfig {
	return funcapi.MethodConfig{
		ID:           flowsMethodID,
		Name:         "Flows (NetFlow)",
		UpdateEvery:  10,
		Help:         "NetFlow/IPFIX/sFlow flow analysis data",
		RequireCloud: true,
		ResponseType: "flows",
	}
}

func (f *funcFlows) MethodParams(_ context.Context, method string) ([]funcapi.ParamConfig, error) {
	if method != flowsMethodID {
		return nil, nil
	}
	return []funcapi.ParamConfig{
		{
			ID:        "view",
			Name:      "View",
			Help:      "Select aggregated or detailed flow view.",
			Selection: funcapi.ParamSelect,
			Options: []funcapi.ParamOption{
				{ID: viewSummary, Name: "Aggregated", Default: true},
				{ID: viewLive, Name: "Detailed"},
			},
		},
	}, nil
}

func (f *funcFlows) Cleanup(_ context.Context) {}

func (f *funcFlows) Handle(_ context.Context, method string, params funcapi.ResolvedParams) *funcapi.FunctionResponse {
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

	view := viewSummary
	if values := params.Get("view"); len(values) > 0 {
		switch values[0] {
		case "summary":
			view = viewSummary
		case "live":
			view = viewLive
		default:
			view = values[0]
		}
	}

	flowsData := collector.buildFlowsResponse(data, view)

	return &funcapi.FunctionResponse{
		Status:       200,
		Help:         "NetFlow/IPFIX/sFlow flow analysis data",
		ResponseType: "flows",
		Data:         flowsData,
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
