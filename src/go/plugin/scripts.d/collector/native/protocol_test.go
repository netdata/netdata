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

func TestDecodeSnapshot(t *testing.T) {
	c, _ := fixtureCollector(t, "exit 0\n")
	tests := map[string]struct {
		data       string
		wantLabels map[string]string // labels of the first metric
		wantErr    bool
	}{
		"label keys are data": {
			data:       `{"version":"v1","metrics":[{"name":"depth","value":0,"labels":{"Version":"x","metrics":"y","State":"z"}}]}`,
			wantLabels: map[string]string{"Version": "x", "metrics": "y", "State": "z"},
		},
		"truncated":           {data: `{"version":"v1"`, wantErr: true},
		"trailing":            {data: snapshotJSON("ok") + `{}`, wantErr: true},
		"unsupported version": {data: `{"version":"v2"}`, wantErr: true},
		"unknown field":       {data: `{"version":"v1","surprise":1}`, wantErr: true},
		"case folded field":   {data: `{"version":"v1","Metrics":[]}`, wantErr: true},
		"case folded check state": {
			data:    `{"version":"v1","checks":[{"id":"backlog","state":"critical","State":"ok","labels":{"queue":"mail"}}]}`,
			wantErr: true,
		},
		"null metric label": {
			data:    `{"version":"v1","metrics":[{"name":"depth","value":1,"labels":{"queue":null}}]}`,
			wantErr: true,
		},
		"null check metadata": {
			data:    `{"version":"v1","checks":[{"id":"backlog","state":"ok","labels":{"queue":"mail","region":null}}]}`,
			wantErr: true,
		},
		"null metrics": {data: `{"version":"v1","metrics":null}`, wantErr: true},
		"null checks":  {data: `{"version":"v1","checks":null}`, wantErr: true},
		"null labels": {
			data:    `{"version":"v1","metrics":[{"name":"depth","value":1,"labels":null}]}`,
			wantErr: true,
		},
		"missing value": {data: `{"version":"v1","metrics":[{"name":"depth"}]}`, wantErr: true},
		"null value":    {data: `{"version":"v1","metrics":[{"name":"depth","value":null}]}`, wantErr: true},
		"overflow":      {data: `{"version":"v1","metrics":[{"name":"depth","value":1e999}]}`, wantErr: true},
		"negative counter": {
			data:    `{"version":"v1","metrics":[{"name":"processed_total","value":-1}]}`,
			wantErr: true,
		},
		"undeclared metric": {data: `{"version":"v1","metrics":[{"name":"new","value":0}]}`, wantErr: true},
		"invalid label key": {
			data:    `{"version":"v1","metrics":[{"name":"depth","value":0,"labels":{"bad-key":"x"}}]}`,
			wantErr: true,
		},
		"duplicate empty labels": {
			data:    `{"version":"v1","metrics":[{"name":"depth","value":0},{"name":"depth","value":1,"labels":{}}]}`,
			wantErr: true,
		},
		"duplicate reordered labels": {
			data:    `{"version":"v1","metrics":[{"name":"depth","value":0,"labels":{"a":"1","b":"2"}},{"name":"depth","value":1,"labels":{"b":"2","a":"1"}}]}`,
			wantErr: true,
		},
		"duplicate key": {data: `{"version":"v1","version":"v1"}`, wantErr: true},
		"duplicate label key": {
			data:    `{"version":"v1","metrics":[{"name":"depth","value":0,"labels":{"queue":"first","queue":"second"}}]}`,
			wantErr: true,
		},
		"missing identity": {data: `{"version":"v1","checks":[{"id":"backlog","state":"ok"}]}`, wantErr: true},
		"invalid state": {
			data:    strings.Replace(snapshotJSON("ok"), `"state":"ok"`, `"state":"bad"`, 1),
			wantErr: true,
		},
		"duplicate check": {
			data:    `{"version":"v1","checks":[{"id":"backlog","state":"ok","labels":{"queue":"a","region":"east"}},{"id":"backlog","state":"critical","labels":{"queue":"a","region":"west"}}]}`,
			wantErr: true,
		},
		"invalid utf8": {data: "{\"version\":\"v1\",\"x\":\"\xff\"}", wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			result, err := c.definition.decodeSnapshot([]byte(tc.data))
			if tc.wantErr {
				require.Error(t, err)
				assert.NotContains(t, err.Error(), "1e999", "errors must not expose raw sample values")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantLabels, result.Metrics[0].Labels)
		})
	}
}

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
	c, _ := fixtureCollector(t, "exit 0\n")
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
			_, err := c.definition.decodeReply([]byte(tc.data), "1")
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
