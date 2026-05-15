// SPDX-License-Identifier: GPL-3.0-or-later

package funcctl

import (
	"encoding/json"
	"fmt"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/functions"
)

type methodResponseWriter func(dataResp *funcapi.FunctionResponse, methodParams []funcapi.ParamConfig, updateEvery int)

func (c *Controller) respondWithParams(fn functions.Function, moduleName string, dataResp *funcapi.FunctionResponse, methodParams []funcapi.ParamConfig, updateEvery int, methodType string, includeJobParam bool) {
	c.respondMethodDataWithParams(
		fn,
		dataResp,
		methodParams,
		updateEvery,
		methodType,
		func(params []funcapi.ParamConfig) []string {
			return buildAcceptedParams(params, includeJobParam)
		},
		func(params []funcapi.ParamConfig) []map[string]any {
			return c.buildRequiredParams(moduleName, params, includeJobParam)
		},
	)
}

func (c *Controller) respondJobMethodWithParams(fn functions.Function, dataResp *funcapi.FunctionResponse, methodParams []funcapi.ParamConfig, updateEvery int, methodType string) {
	c.respondMethodDataWithParams(
		fn,
		dataResp,
		methodParams,
		updateEvery,
		methodType,
		buildJobMethodAcceptedParams,
		buildJobMethodRequiredParams,
	)
}

func (c *Controller) respondMethodDataWithParams(
	fn functions.Function,
	dataResp *funcapi.FunctionResponse,
	methodParams []funcapi.ParamConfig,
	updateEvery int,
	methodType string,
	buildAccepted func([]funcapi.ParamConfig) []string,
	buildRequired func([]funcapi.ParamConfig) []map[string]any,
) {
	if dataResp == nil {
		c.respondError(fn, 500, "internal error: module returned nil response")
		return
	}
	if dataResp.Status >= 400 {
		c.respondError(fn, dataResp.Status, "%s", dataResp.Message)
		return
	}

	paramsForResponse := methodParams
	if len(dataResp.RequiredParams) > 0 {
		paramsForResponse = funcapi.MergeParamConfigs(paramsForResponse, dataResp.RequiredParams)
	}

	resp := map[string]any{
		"v":               3,
		"update_every":    updateEvery,
		"status":          dataResp.Status,
		"type":            resolveResponseType(dataResp.ResponseType, methodType),
		"has_history":     false,
		"help":            dataResp.Help,
		"accepted_params": buildAccepted(paramsForResponse),
		"required_params": buildRequired(paramsForResponse),
	}

	if dataResp.Columns != nil {
		resp["columns"] = dataResp.Columns
	}
	if dataResp.Data != nil {
		resp["data"] = dataResp.Data
	}
	if dataResp.DefaultSortColumn != "" {
		resp["default_sort_column"] = dataResp.DefaultSortColumn
	}
	if len(dataResp.Charts) > 0 {
		resp["charts"] = dataResp.Charts
	}
	if len(dataResp.DefaultCharts) > 0 {
		resp["default_charts"] = dataResp.DefaultCharts.Build()
	}
	if len(dataResp.GroupBy) > 0 {
		resp["group_by"] = dataResp.GroupBy
	}

	c.respondJSON(fn, resp)
}

func resolveResponseType(dataType, methodType string) string {
	if dataType != "" {
		return dataType
	}
	if methodType != "" {
		return methodType
	}
	return "table"
}

func (c *Controller) respondError(fn functions.Function, status int, format string, args ...any) {
	c.respondJSON(fn, map[string]any{
		"status":       status,
		"errorMessage": fmt.Sprintf(format, args...),
	})
}

func (c *Controller) respondJSON(fn functions.Function, resp map[string]any) {
	data, err := json.Marshal(resp)
	if err != nil {
		c.Errorf("failed to marshal function response: %v", err)
		c.sendJSON(fn, string(functions.BuildJSONPayload(500, "internal error: failed to encode response")), 500)
		return
	}

	code := 200
	if status, ok := resp["status"]; ok {
		switch value := status.(type) {
		case int:
			code = value
		case int64:
			code = int(value)
		case float64:
			code = int(value)
		}
	}

	c.sendJSON(fn, string(data), code)
}

func (c *Controller) sendJSON(fn functions.Function, payload string, code int) {
	if c.jsonWriter != nil {
		c.jsonWriter([]byte(payload), code)
		return
	}
	if c.api == nil {
		return
	}

	c.api.SendJSONWithCode(dyncfg.NewFunction(fn), payload, code)
}
