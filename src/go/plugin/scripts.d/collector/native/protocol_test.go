// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeReady(t *testing.T) {
	tests := map[string]struct {
		data    string
		wantErr bool
	}{
		"ready":             {data: `{"version":"v1","ready":true}`},
		"unsupported":       {data: `{"version":"v2","ready":true}`, wantErr: true},
		"case folded field": {data: `{"version":"v1","Ready":true}`, wantErr: true},
		"duplicate key":     {data: `{"version":"v1","ready":true,"ready":true}`, wantErr: true},
		"null ready":        {data: `{"version":"v1","ready":null}`, wantErr: true},
		"not ready":         {data: `{"version":"v1","ready":false}`, wantErr: true},
		"unknown field":     {data: `{"version":"v1","ready":true,"extra":0}`, wantErr: true},
		"array":             {data: `[]`, wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := decodeReady([]byte(tc.data))
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestDecodeReply(t *testing.T) {
	tests := map[string]struct {
		data      string
		wantErr   bool
		wantErrIs error
	}{
		"snapshot": {data: `{"id":"1","result":` + snapshotJSON("critical") + `}`},
		"collection failed": {
			data:      `{"id":"1","error":"collection_failed"}`,
			wantErr:   true,
			wantErrIs: errCollectionFailed,
		},
		"missing result":    {data: `{"id":"1"}`, wantErr: true},
		"numeric id":        {data: `{"id":1,"result":{"version":"v1"}}`, wantErr: true},
		"null result":       {data: `{"id":"1","result":null}`, wantErr: true},
		"null error":        {data: `{"id":"1","error":null}`, wantErr: true},
		"result and error":  {data: `{"id":"1","result":{"version":"v1"},"error":"collection_failed"}`, wantErr: true},
		"null checks":       {data: `{"id":"1","result":{"version":"v1","checks":null}}`, wantErr: true},
		"duplicate id":      {data: `{"id":"1","id":"1","result":{"version":"v1"}}`, wantErr: true},
		"case folded field": {data: `{"id":"1","Result":{"version":"v1"}}`, wantErr: true},
		"unknown error":     {data: `{"id":"1","error":"arbitrary text"}`, wantErr: true},
		"wrong id":          {data: `{"id":"2","error":"collection_failed"}`, wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := decodeReply([]byte(tc.data), "1")
			if !tc.wantErr {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			if tc.wantErrIs != nil {
				require.ErrorIs(t, err, tc.wantErrIs)
			} else {
				require.NotErrorIs(t, err, errCollectionFailed, "only the fixed error code keeps the session")
			}
		})
	}
}

// Result conversion is tested in nativefunc; these cases cover the strict
// envelope and number preservation of the wire decode.
func TestDecodeFunctionReply(t *testing.T) {
	tests := map[string]struct {
		reply   string
		want    *funcapi.FunctionResponse
		wantErr bool
	}{
		"managed rows keep exact numbers": {
			reply: `{"id":"1","result":{"version":"v1","status":200,"columns":{},"data":[[9007199254740993,null]]}}`,
			want: &funcapi.FunctionResponse{
				Status:         200,
				Columns:        map[string]any{},
				Data:           [][]any{{json.Number("9007199254740993"), nil}},
				RequiredParams: []funcapi.ParamConfig{},
				ChartingConfig: funcapi.ChartingConfig{
					DefaultCharts: funcapi.DefaultCharts{},
				},
			},
		},
		"raw rows keep exact numbers": {
			reply: `{"id":"1","result":{"version":"v1","raw_response":{"status":500,"data":[[9007199254740993,null]]}}}`,
			want: funcapi.RawResponse(map[string]any{
				"status": 500,
				"data":   []any{[]any{json.Number("9007199254740993"), nil}},
			}),
		},
		"raw rounded fraction status": {
			reply:   `{"id":"1","result":{"version":"v1","raw_response":{"status":500.00000000000000000001}}}`,
			wantErr: true,
		},
		"null envelope": {reply: `null`, wantErr: true},
		"null result":   {reply: `{"id":"1","result":null}`, wantErr: true},
		"wrong id":      {reply: `{"id":"2","result":{"version":"v1","status":500}}`, wantErr: true},
		"unknown field": {reply: `{"id":"1","result":{"version":"v1","status":500},"extra":1}`, wantErr: true},
		"null status":   {reply: `{"id":"1","result":{"version":"v1","status":null}}`, wantErr: true},
		"wrong case":    {reply: `{"id":"1","result":{"Version":"v1","status":500}}`, wantErr: true},
		"duplicate": {
			reply:   `{"id":"1","result":{"version":"v1","status":500,"status":200}}`,
			wantErr: true,
		},
		"raw nested duplicate": {
			reply:   `{"id":"1","result":{"version":"v1","raw_response":{"status":500,"data":{"x":1,"x":2}}}}`,
			wantErr: true,
		},
		"null chart columns": {
			reply:   `{"id":"1","result":{"version":"v1","status":200,"columns":{},"data":[],"charts":{"c":{"name":"C","columns":null}}}}`,
			wantErr: true,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := decodeFunctionReply([]byte(tc.reply), "1", false)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestEncodeFunctionRequest(t *testing.T) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Minute))
	defer cancel()
	request := funcapi.RawMethodRequest{
		Method:      "items",
		Info:        true,
		Args:        []string{"filter:quotes \" λ\n"},
		Payload:     []byte{0, 255, 10},
		ContentType: "application/octet-stream",
		Permissions: "0xFFFF",
		Source:      "synthetic",
	}
	frame, err := encodeFunctionRequest(ctx, "7", request)
	require.NoError(t, err)
	require.True(t, strings.HasSuffix(string(frame), "}\n"))
	type encodedRequest struct {
		ID          string   `json:"id"`
		Method      string   `json:"method"`
		Function    string   `json:"function"`
		Info        bool     `json:"info"`
		Args        []string `json:"args"`
		Payload     []byte   `json:"payload_base64"`
		ContentType string   `json:"content_type"`
		DeadlineMS  int64    `json:"deadline_unix_ms"`
		Permissions string   `json:"permissions"`
		Source      string   `json:"source"`
	}
	var got encodedRequest
	require.NoError(t, json.Unmarshal(frame, &got))
	deadline, _ := ctx.Deadline()
	assert.Equal(t, encodedRequest{
		ID:          "7",
		Method:      "function",
		Function:    request.Method,
		Info:        true,
		Args:        request.Args,
		Payload:     request.Payload,
		ContentType: request.ContentType,
		DeadlineMS:  deadline.UnixMilli(),
		Permissions: request.Permissions,
		Source:      request.Source,
	}, got)
}

// The limit covers the complete encoded frame, including the terminating LF.
func TestEncodeFunctionRequest_SizeLimit(t *testing.T) {
	ctx := context.Background()
	request := funcapi.RawMethodRequest{
		Method: "items",
		Args:   []string{""},
	}
	frame, err := encodeFunctionRequest(ctx, "7", request)
	require.NoError(t, err)
	request.Args[0] = strings.Repeat("x", maxMessageBytes-len(frame))
	frame, err = encodeFunctionRequest(ctx, "7", request)
	require.NoError(t, err)
	assert.Len(t, frame, maxMessageBytes)
	request.Args[0] += "x"
	_, err = encodeFunctionRequest(ctx, "7", request)
	require.ErrorContains(t, err, "exceeds 64 MiB")
}
