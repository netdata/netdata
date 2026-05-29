// SPDX-License-Identifier: GPL-3.0-or-later

package catofunc

import (
	"context"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
)

const (
	TopologyMethodID = "topology:cato_networks"

	ActorTypeSite    = "cato_site"
	ActorTypePop     = "cato_pop"
	ActorTypeBGPPeer = "bgp_peer"

	LinkTypeTunnel = "cato_tunnel"
	LinkTypeBGP    = "bgp_session"
)

const topologyUnavailable = "Cato Networks topology data is not available yet"

type funcTopology struct {
	router *router
}

func newFuncTopology(r *router) *funcTopology {
	return &funcTopology{router: r}
}

var _ funcapi.MethodHandler = (*funcTopology)(nil)

func (f *funcTopology) MethodParams(context.Context, string) ([]funcapi.ParamConfig, error) {
	return nil, nil
}

func (f *funcTopology) Handle(_ context.Context, method string, _ funcapi.ResolvedParams) *funcapi.FunctionResponse {
	if method != TopologyMethodID {
		return funcapi.NotFoundResponse(method)
	}
	if f.router == nil || f.router.deps == nil {
		return funcapi.UnavailableResponse(topologyUnavailable)
	}

	data, ok := f.router.deps.CurrentTopology()
	if !ok {
		return funcapi.UnavailableResponse(topologyUnavailable)
	}

	return &funcapi.FunctionResponse{
		Status:       200,
		Help:         "Cato Networks site, PoP, tunnel, and BGP topology data",
		ResponseType: "topology",
		Data:         data,
	}
}

func (f *funcTopology) Cleanup(context.Context) {}
