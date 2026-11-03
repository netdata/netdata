// SPDX-License-Identifier: GPL-3.0-or-later

package nativefunc

import (
	"encoding/json"
	"errors"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/collector/native/internal/strictjson"
)

// Result uses public Function field names, not Go implementation serialization.
// RawResponse explicitly transfers complete response ownership to the script.
// Decode it with json.Decoder.UseNumber: arbitrary data keeps exact numbers and
// a raw response status must be a json.Number.
type Result struct {
	Version           string                           `json:"version"`
	Status            int                              `json:"status,omitempty"`
	Message           string                           `json:"message,omitempty"`
	Help              string                           `json:"help,omitempty"`
	ResponseType      string                           `json:"type,omitempty"`
	Columns           map[string]any                   `json:"columns,omitempty"`
	Data              [][]any                          `json:"data,omitempty"`
	DefaultSortColumn string                           `json:"default_sort_column,omitempty"`
	RequiredParams    []Parameter                      `json:"required_params,omitempty"`
	Charts            map[string]funcapi.ChartConfig   `json:"charts,omitempty"`
	DefaultCharts     [][]string                       `json:"default_charts,omitempty"`
	GroupBy           map[string]funcapi.GroupByConfig `json:"group_by,omitempty"`
	RawResponse       map[string]any                   `json:"raw_response,omitempty"`
}

// ResultShape is the strict JSON shape of a Result.
func ResultShape() *strictjson.Shape {
	return strictjson.Object(strictjson.Fields{
		// Arbitrary Function data can contain null and large integers.
		"columns":         strictjson.Any(),
		"data":            strictjson.Any(),
		"raw_response":    strictjson.Any(),
		"required_params": parameterShape(strictjson.Object),
		"charts":          strictjson.Map(strictjson.Object(nil, "name", "type", "columns")),
		"group_by":        strictjson.Map(strictjson.Object(nil, "name", "columns")),
	}, "version", "status", "message", "help", "type", "default_sort_column", "default_charts")
}

// Response validates the result and converts it to a framework response.
// info reports an info request, whose managed reply needs no rows.
func (r Result) Response(info bool) (*funcapi.FunctionResponse, error) {
	if r.Version != "v1" {
		return nil, errors.New("unsupported Function result version")
	}
	if r.RawResponse != nil {
		return r.rawResponse()
	}
	if !validStatus(int64(r.Status)) {
		return nil, errors.New("invalid Function status")
	}
	params, err := paramConfigs(r.RequiredParams)
	if err != nil {
		return nil, err
	}
	if r.Status < 400 && !info && (r.Columns == nil || r.Data == nil) {
		return nil, errors.New("managed Function data requires columns and rows")
	}
	charts := make(funcapi.DefaultCharts, 0, len(r.DefaultCharts))
	for _, item := range r.DefaultCharts {
		if len(item) != 2 {
			return nil, errors.New("default Function chart requires chart and group")
		}
		charts = append(charts, funcapi.DefaultChart{
			Chart:   item[0],
			GroupBy: item[1],
		})
	}
	return &funcapi.FunctionResponse{
		Status:            r.Status,
		Message:           r.Message,
		Help:              r.Help,
		ResponseType:      r.ResponseType,
		Columns:           r.Columns,
		Data:              r.Data,
		DefaultSortColumn: r.DefaultSortColumn,
		RequiredParams:    params,
		ChartingConfig: funcapi.ChartingConfig{
			Charts:        r.Charts,
			DefaultCharts: charts,
			GroupBy:       r.GroupBy,
		},
	}, nil
}

func (r Result) rawResponse() (*funcapi.FunctionResponse, error) {
	managed := r.Status != 0 || r.Message != "" || r.Help != "" || r.ResponseType != "" || r.Columns != nil ||
		r.Data != nil || r.DefaultSortColumn != "" || r.RequiredParams != nil || r.Charts != nil ||
		r.DefaultCharts != nil || r.GroupBy != nil
	if managed {
		return nil, errors.New("raw Function response cannot include managed fields")
	}
	n, ok := r.RawResponse["status"].(json.Number)
	if !ok {
		return nil, errors.New("raw Function response requires a numeric status")
	}
	status, err := n.Int64()
	if err != nil || !validStatus(status) {
		return nil, errors.New("invalid raw Function status")
	}
	// The framework's raw status adapter expects int; retain json.Number elsewhere.
	r.RawResponse["status"] = int(status)
	return funcapi.RawResponse(r.RawResponse), nil
}

func validStatus(status int64) bool { return status >= 100 && status <= 599 }
