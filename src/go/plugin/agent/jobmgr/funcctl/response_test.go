// SPDX-License-Identifier: GPL-3.0-or-later

package funcctl

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/functions"
)

func newTestControllerWithCapture(t *testing.T) (*Controller, *map[string]any) {
	t.Helper()

	var resp map[string]any
	controller := New(Options{
		JSONWriter: func(payload []byte, _ int) {
			require.NoError(t, json.Unmarshal(payload, &resp))
		},
	})

	return controller, &resp
}

func TestRespondWithParams_ResponseType(t *testing.T) {
	controller, resp := newTestControllerWithCapture(t)

	dataResp := &funcapi.FunctionResponse{
		Status:       200,
		ResponseType: "topology",
	}

	controller.respondWithParams(functions.Function{}, "snmp", dataResp, nil, 1, "", true)

	assert.Equal(t, "topology", (*resp)["type"])
}

func TestRespondWithParams_MethodTypeFallback(t *testing.T) {
	controller, resp := newTestControllerWithCapture(t)

	dataResp := &funcapi.FunctionResponse{
		Status: 200,
	}

	controller.respondWithParams(functions.Function{}, "snmp", dataResp, nil, 1, "topology", true)

	assert.Equal(t, "topology", (*resp)["type"])
}

func TestRespondWithParams_AgentWideOmitsJobParam(t *testing.T) {
	controller, resp := newTestControllerWithCapture(t)

	dataResp := &funcapi.FunctionResponse{
		Status: 200,
		RequiredParams: []funcapi.ParamConfig{{
			ID:        "topology_view",
			Name:      "Topology View",
			Selection: funcapi.ParamSelect,
			Options: []funcapi.ParamOption{
				{ID: "l2", Name: "L2", Default: true},
			},
		}},
	}

	controller.respondWithParams(functions.Function{}, "snmp", dataResp, nil, 1, "topology", false)

	accepted, ok := (*resp)["accepted_params"].([]any)
	assert.True(t, ok)
	assert.Equal(t, []any{"topology_view"}, accepted)

	required, ok := (*resp)["required_params"].([]any)
	assert.True(t, ok)
	assert.Len(t, required, 1)
	req0, ok := required[0].(map[string]any)
	assert.True(t, ok)
	assert.Equal(t, "topology_view", req0["id"])
}

func TestHandleMethodFuncInfo_UsesResponseType(t *testing.T) {
	controller, resp := newTestControllerWithCapture(t)

	controller.registry.registerModule("snmp", collectorapi.Creator{
		Methods: func() []funcapi.MethodConfig {
			return []funcapi.MethodConfig{{ID: "topology:snmp", ResponseType: "topology"}}
		},
	})

	controller.handleMethodFuncInfo("snmp", "topology:snmp", functions.Function{})

	assert.Equal(t, "topology", (*resp)["type"])
}

func TestHandleMethodFuncInfo_AgentWideOmitsJobParam(t *testing.T) {
	controller, resp := newTestControllerWithCapture(t)

	controller.registry.registerModule("snmp", collectorapi.Creator{
		Methods: func() []funcapi.MethodConfig {
			return []funcapi.MethodConfig{{
				ID:           "topology:snmp",
				ResponseType: "topology",
				AgentWide:    true,
				RequiredParams: []funcapi.ParamConfig{{
					ID:        "topology_view",
					Name:      "Topology View",
					Selection: funcapi.ParamSelect,
					Options: []funcapi.ParamOption{
						{ID: "l2", Name: "L2", Default: true},
					},
				}},
			}}
		},
	})
	controller.registry.addJob("snmp", "job-a", newTestRuntimeJob("snmp", "job-a", true))

	controller.handleMethodFuncInfo("snmp", "topology:snmp", functions.Function{})

	accepted, ok := (*resp)["accepted_params"].([]any)
	assert.True(t, ok)
	assert.Equal(t, []any{"topology_view"}, accepted)

	required, ok := (*resp)["required_params"].([]any)
	assert.True(t, ok)
	assert.Len(t, required, 1)
	req0, ok := required[0].(map[string]any)
	assert.True(t, ok)
	assert.Equal(t, "topology_view", req0["id"])
}

func TestHandleJobMethodFuncInfo_UsesResponseType(t *testing.T) {
	controller, resp := newTestControllerWithCapture(t)

	controller.registry.registerModule("netflow", collectorapi.Creator{})
	controller.registry.registerJobMethods("netflow", "job1", []funcapi.MethodConfig{
		{ID: "flows:netflow", ResponseType: "flows"},
	})

	controller.handleJobMethodFuncInfo("netflow", "job1", "flows:netflow", functions.Function{})

	assert.Equal(t, "flows", (*resp)["type"])
}

func TestHandleMethodFuncInfo_IncludesPresentation(t *testing.T) {
	controller, resp := newTestControllerWithCapture(t)

	controller.registry.registerModule("snmp", collectorapi.Creator{
		Methods: func() []funcapi.MethodConfig {
			return []funcapi.MethodConfig{
				funcapi.MethodConfig{
					ID:           "topology:snmp",
					ResponseType: "topology",
				}.WithPresentation(map[string]any{
					"actor_click_behavior": "highlight_connections",
				}),
			}
		},
	})

	controller.handleMethodFuncInfo("snmp", "topology:snmp", functions.Function{})

	presentation, ok := (*resp)["presentation"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "highlight_connections", presentation["actor_click_behavior"])
}
