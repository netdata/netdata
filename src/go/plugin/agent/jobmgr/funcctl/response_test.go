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

	controller.respondWithParams(functions.Function{}, "snmp", dataResp, nil, 1, "")

	assert.Equal(t, "topology", (*resp)["type"])
}

func TestRespondWithParams_MethodTypeFallback(t *testing.T) {
	controller, resp := newTestControllerWithCapture(t)

	dataResp := &funcapi.FunctionResponse{
		Status: 200,
	}

	controller.respondWithParams(functions.Function{}, "snmp", dataResp, nil, 1, "flows")

	assert.Equal(t, "flows", (*resp)["type"])
}

func TestHandleMethodFuncInfo_UsesResponseType(t *testing.T) {
	controller, resp := newTestControllerWithCapture(t)

	controller.registry.registerModule("snmp", collectorapi.Creator{
		Methods: func() []funcapi.MethodConfig {
			return []funcapi.MethodConfig{{ID: "topology", ResponseType: "topology"}}
		},
	})

	controller.handleMethodFuncInfo("snmp", "topology", functions.Function{})

	assert.Equal(t, "topology", (*resp)["type"])
}

func TestHandleJobMethodFuncInfo_UsesResponseType(t *testing.T) {
	controller, resp := newTestControllerWithCapture(t)

	controller.registry.registerModule("netflow", collectorapi.Creator{})
	controller.registry.registerJobMethods("netflow", "job1", []funcapi.MethodConfig{
		{ID: "flows", ResponseType: "flows"},
	})

	controller.handleJobMethodFuncInfo("netflow", "job1", "flows", functions.Function{})

	assert.Equal(t, "flows", (*resp)["type"])
}
