// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopologyfunc

import (
	"context"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	topologyv1 "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
)

const (
	FunctionName = "snmp:topology:snmp"
	MethodID     = "topology:snmp"
)

const topologyUnavailable = "topology data not available yet, please retry after topology refresh"

type topologyHandler struct {
	deps Deps
}

var _ funcapi.MethodHandler = (*topologyHandler)(nil)

func NewHandler(deps Deps) funcapi.MethodHandler {
	return &topologyHandler{deps: deps}
}

func (h *topologyHandler) MethodParams(_ context.Context, method string) ([]funcapi.ParamConfig, error) {
	if method != MethodID {
		return nil, nil
	}

	return []funcapi.ParamConfig{
		nodesIdentityParamConfig(),
		mapTypeParamConfig(),
		inferenceStrategyParamConfig(),
		managedFocusParamConfig(managedFocusParamOptions(h.deps)),
		depthParamConfig(),
	}, nil
}

func (h *topologyHandler) Handle(_ context.Context, method string, params funcapi.ResolvedParams) *funcapi.FunctionResponse {
	if method != MethodID {
		return funcapi.NotFoundResponse(method)
	}
	if h.deps == nil {
		return funcapi.UnavailableResponse(topologyUnavailable)
	}

	options := resolveQueryOptions(params)
	snapshot, ok, err := h.deps.Snapshot(options)
	if err != nil {
		return funcapi.InternalErrorResponse("failed to build topology response: %v", err)
	}
	if !ok {
		return funcapi.UnavailableResponse(topologyUnavailable)
	}
	required := functionConfig().RequiredParams
	required[3] = managedFocusParamConfig(managedFocusTargetOptions(snapshot.FocusTargets))
	required[3].Focus = rootFocusConfig(options.EliminateNonIPInferred)

	return &funcapi.FunctionResponse{
		Status:         200,
		Help:           "SNMP topology and neighbor discovery data",
		ResponseType:   topologyv1.ResponseType,
		Data:           snapshot.Data,
		RequiredParams: required,
	}
}

func (h *topologyHandler) Cleanup(context.Context) {}
