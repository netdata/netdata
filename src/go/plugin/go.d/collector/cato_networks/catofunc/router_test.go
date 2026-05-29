// SPDX-License-Identifier: GPL-3.0-or-later

package catofunc

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/pkg/topology"
)

type fakeDeps struct {
	data *topology.Data
	ok   bool
}

func (d fakeDeps) CurrentTopology() (*topology.Data, bool) {
	return d.data, d.ok
}

func TestTopologyHandlerReturnsCurrentTopology(t *testing.T) {
	data := &topology.Data{Source: "cato_networks"}
	handler := NewRouter(fakeDeps{data: data, ok: true})

	resp := handler.Handle(context.Background(), TopologyMethodID, nil)

	require.Equal(t, 200, resp.Status)
	require.Equal(t, "topology", resp.ResponseType)
	require.Same(t, data, resp.Data)
}

func TestTopologyHandlerReturnsUnavailableWithoutSnapshot(t *testing.T) {
	handler := NewRouter(fakeDeps{})

	resp := handler.Handle(context.Background(), TopologyMethodID, nil)

	require.Equal(t, 503, resp.Status)
}

func TestTopologyHandlerRejectsUnknownMethod(t *testing.T) {
	handler := NewRouter(fakeDeps{})

	resp := handler.Handle(context.Background(), "unknown", nil)

	require.Equal(t, 404, resp.Status)
}

func TestRouterMethodParamsRejectsUnknownMethod(t *testing.T) {
	handler := NewRouter(fakeDeps{})

	params, err := handler.MethodParams(context.Background(), "unknown")

	require.Nil(t, params)
	require.ErrorContains(t, err, "unknown method: unknown")
}

func TestRouterCleanupForwardsToHandlers(t *testing.T) {
	h := &cleanupHandler{}
	r := &router{
		handlers: map[string]funcapi.MethodHandler{
			"test": h,
		},
	}

	r.Cleanup(context.Background())

	require.True(t, h.called)
}

type cleanupHandler struct {
	called bool
}

func (h *cleanupHandler) MethodParams(context.Context, string) ([]funcapi.ParamConfig, error) {
	return nil, errors.New("not implemented")
}

func (h *cleanupHandler) Handle(context.Context, string, funcapi.ResolvedParams) *funcapi.FunctionResponse {
	return nil
}

func (h *cleanupHandler) Cleanup(context.Context) {
	h.called = true
}
