// SPDX-License-Identifier: GPL-3.0-or-later

package catofunc

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	topologyv1 "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
)

type fakeDeps struct {
	data *topologyv1.Data
	ok   bool
}

func (d fakeDeps) CurrentTopology() (*topologyv1.Data, bool) {
	return d.data, d.ok
}

func TestRouter_Handle(t *testing.T) {
	tests := map[string]struct {
		deps   fakeDeps
		method string
		check  func(*testing.T, *funcapi.FunctionResponse, *topologyv1.Data)
	}{
		"returns current topology": {
			deps:   fakeDeps{data: &topologyv1.Data{Producer: topologyv1.Producer{Source: "cato_networks"}}, ok: true},
			method: TopologyMethodID,
			check: func(t *testing.T, resp *funcapi.FunctionResponse, data *topologyv1.Data) {
				require.Equal(t, 200, resp.Status)
				require.Equal(t, "topology", resp.ResponseType)
				require.Same(t, data, resp.Data)
			},
		},
		"returns unavailable without snapshot": {
			method: TopologyMethodID,
			check: func(t *testing.T, resp *funcapi.FunctionResponse, _ *topologyv1.Data) {
				require.Equal(t, 503, resp.Status)
			},
		},
		"rejects unknown method": {
			method: "unknown",
			check: func(t *testing.T, resp *funcapi.FunctionResponse, _ *topologyv1.Data) {
				require.Equal(t, 404, resp.Status)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			handler := NewRouter(tc.deps)
			resp := handler.Handle(context.Background(), tc.method, nil)
			tc.check(t, resp, tc.deps.data)
		})
	}
}

func TestRouter_MethodParams(t *testing.T) {
	tests := map[string]struct {
		method  string
		wantErr string
	}{
		"known topology method": {
			method: TopologyMethodID,
		},
		"unknown method": {
			method:  "unknown",
			wantErr: "unknown method: unknown",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			handler := NewRouter(fakeDeps{})
			params, err := handler.MethodParams(context.Background(), tc.method)

			if tc.wantErr != "" {
				require.Nil(t, params)
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Nil(t, params)
		})
	}
}

func TestRouter_Cleanup(t *testing.T) {
	tests := map[string]struct {
		handlers map[string]funcapi.MethodHandler
		check    func(*testing.T, *cleanupHandler)
	}{
		"forwards cleanup to handlers": {
			handlers: map[string]funcapi.MethodHandler{
				"test": &cleanupHandler{},
			},
			check: func(t *testing.T, h *cleanupHandler) {
				require.True(t, h.called)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			h := tc.handlers["test"].(*cleanupHandler)
			r := &router{handlers: tc.handlers}

			r.Cleanup(context.Background())

			tc.check(t, h)
		})
	}
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
