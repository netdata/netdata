// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"context"
	"fmt"
	"strings"

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

const (
	topologyParamView = "topology_view"

	topologyViewL2     = "l2"
	topologyViewL3     = "l3"
	topologyViewMerged = "merged"
)

func topologyViewParamConfig() funcapi.ParamConfig {
	return funcapi.ParamConfig{
		ID:        topologyParamView,
		Name:      "Topology View",
		Help:      "Select topology view: L2, L3, or merged",
		Selection: funcapi.ParamSelect,
		Options: []funcapi.ParamOption{
			{ID: topologyViewL2, Name: "L2", Default: true},
			{ID: topologyViewL3, Name: "L3"},
			{ID: topologyViewMerged, Name: "Merged"},
		},
	}
}

func topologyMethodConfig() funcapi.MethodConfig {
	return funcapi.MethodConfig{
		ID:           topologyMethodID,
		Name:         "Topology (SNMP)",
		UpdateEvery:  10,
		Help:         "SNMP topology and neighbor discovery data (view selector: L2/L3/merged)",
		RequireCloud: true,
		ResponseType: "topology",
		AgentWide:    true,
		RequiredParams: []funcapi.ParamConfig{
			topologyViewParamConfig(),
		},
	}
}

func (f *funcTopology) MethodParams(_ context.Context, method string) ([]funcapi.ParamConfig, error) {
	if method != topologyMethodID {
		return nil, nil
	}
	return []funcapi.ParamConfig{
		topologyViewParamConfig(),
	}, nil
}

func (f *funcTopology) Cleanup(_ context.Context) {}

func (f *funcTopology) Handle(_ context.Context, method string, params funcapi.ResolvedParams) *funcapi.FunctionResponse {
	if method != topologyMethodID {
		return funcapi.NotFoundResponse(method)
	}

	view := topologyViewL2
	if fView := normalizeTopologyView(params.GetOne(topologyParamView)); fView != "" {
		view = fView
	}

	switch view {
	case topologyViewL3, topologyViewMerged:
		return funcapi.UnavailableResponse(fmt.Sprintf("snmp topology view %q is not available yet", view))
	}

	if snmpTopologyRegistry == nil {
		return funcapi.UnavailableResponse("topology data not available yet, please retry after data collection")
	}

	data, ok := snmpTopologyRegistry.snapshot()

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

func normalizeTopologyView(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", topologyViewL2:
		return topologyViewL2
	case topologyViewL3:
		return topologyViewL3
	case topologyViewMerged:
		return topologyViewMerged
	default:
		return ""
	}
}
