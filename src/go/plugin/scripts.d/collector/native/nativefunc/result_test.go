// SPDX-License-Identifier: GPL-3.0-or-later

package nativefunc

import (
	"encoding/json"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResult_Response(t *testing.T) {
	charts := map[string]funcapi.ChartConfig{"depth": {Name: "Depth", Type: "line", Columns: []string{"count"}}}
	groups := map[string]funcapi.GroupByConfig{"queue": {Name: "Queue", Columns: []string{"queue"}}}
	columns := map[string]any{"count": map[string]any{"index": json.Number("0")}}
	rows := [][]any{{json.Number("9007199254740993")}}
	tests := map[string]struct {
		result  Result
		info    bool
		want    *funcapi.FunctionResponse
		wantErr bool
	}{
		"managed table": {
			result: Result{
				Version:           "v1",
				Status:            200,
				Message:           "ok",
				Help:              "Items help.",
				ResponseType:      "table",
				Columns:           columns,
				Data:              rows,
				DefaultSortColumn: "count",
				RequiredParams: []Parameter{{
					ID:      "queue",
					Name:    "Queue",
					Type:    "multiselect",
					Options: []Option{{ID: "mail", Name: "Mail", Default: true}},
				}},
				Charts:        charts,
				DefaultCharts: [][]string{{"depth", "queue"}},
				GroupBy:       groups,
			},
			want: &funcapi.FunctionResponse{
				Status:            200,
				Message:           "ok",
				Help:              "Items help.",
				ResponseType:      "table",
				Columns:           columns,
				Data:              rows,
				DefaultSortColumn: "count",
				RequiredParams: []funcapi.ParamConfig{{
					ID:        "queue",
					Name:      "Queue",
					Selection: funcapi.ParamMultiSelect,
					Options:   []funcapi.ParamOption{{ID: "mail", Name: "Mail", Default: true}},
				}},
				ChartingConfig: funcapi.ChartingConfig{
					Charts:        charts,
					DefaultCharts: funcapi.DefaultCharts{{Chart: "depth", GroupBy: "queue"}},
					GroupBy:       groups,
				},
			},
		},
		"info without rows": {
			result: Result{
				Version: "v1",
				Status:  200,
			},
			info: true,
			want: &funcapi.FunctionResponse{
				Status:         200,
				Data:           [][]any(nil),
				RequiredParams: []funcapi.ParamConfig{},
				ChartingConfig: funcapi.ChartingConfig{
					DefaultCharts: funcapi.DefaultCharts{},
				},
			},
		},
		"error without rows": {
			result: Result{
				Version: "v1",
				Status:  503,
				Message: "unavailable",
			},
			want: &funcapi.FunctionResponse{
				Status:         503,
				Message:        "unavailable",
				Data:           [][]any(nil),
				RequiredParams: []funcapi.ParamConfig{},
				ChartingConfig: funcapi.ChartingConfig{
					DefaultCharts: funcapi.DefaultCharts{},
				},
			},
		},
		"raw response": {
			result: Result{
				Version:     "v1",
				RawResponse: map[string]any{"status": json.Number("500"), "exact": json.Number("9007199254740993")},
			},
			want: funcapi.RawResponse(map[string]any{"status": 500, "exact": json.Number("9007199254740993")}),
		},
		"unsupported version": {result: Result{
			Version: "v2",
			Status:  200,
		}, wantErr: true},
		"missing status": {result: Result{
			Version: "v1",
		}, info: true, wantErr: true},
		"status out of range": {result: Result{
			Version: "v1",
			Status:  600,
		}, info: true, wantErr: true},
		"missing rows": {result: Result{
			Version: "v1",
			Status:  200,
			Columns: columns,
		}, wantErr: true},
		"invalid default chart": {
			result: Result{
				Version:       "v1",
				Status:        200,
				Columns:       columns,
				Data:          rows,
				DefaultCharts: [][]string{{"depth"}},
			},
			wantErr: true,
		},
		"reserved selector": {
			result: Result{
				Version:        "v1",
				Status:         200,
				Columns:        columns,
				Data:           rows,
				RequiredParams: []Parameter{{ID: "__job", Name: "Job"}},
			},
			wantErr: true,
		},
		"raw with managed field": {
			result: Result{
				Version:     "v1",
				Status:      200,
				RawResponse: map[string]any{"status": json.Number("500")},
			},
			wantErr: true,
		},
		"raw without status": {
			result: Result{
				Version:     "v1",
				RawResponse: map[string]any{},
			},
			wantErr: true,
		},
		"raw string status": {
			result: Result{
				Version:     "v1",
				RawResponse: map[string]any{"status": "500"},
			},
			wantErr: true,
		},
		"raw fractional status": {
			result: Result{
				Version:     "v1",
				RawResponse: map[string]any{"status": json.Number("500.1")},
			},
			wantErr: true,
		},
		"raw status out of range": {
			result: Result{
				Version:     "v1",
				RawResponse: map[string]any{"status": json.Number("700")},
			},
			wantErr: true,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := tc.result.Response(tc.info)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}
