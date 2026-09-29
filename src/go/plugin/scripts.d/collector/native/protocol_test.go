// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"context"
	"encoding/json"
	"errors"
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
		data    string
		wantErr error // nil: success; errAny: any error that is not errCollectionFailed
	}{
		"snapshot":          {data: `{"id":"1","result":` + snapshotJSON("critical") + `}`},
		"collection failed": {data: `{"id":"1","error":"collection_failed"}`, wantErr: errCollectionFailed},
		"missing result":    {data: `{"id":"1"}`, wantErr: errAny},
		"numeric id":        {data: `{"id":1,"result":{"version":"v1"}}`, wantErr: errAny},
		"null result":       {data: `{"id":"1","result":null}`, wantErr: errAny},
		"null error":        {data: `{"id":"1","error":null}`, wantErr: errAny},
		"result and error": {
			data:    `{"id":"1","result":{"version":"v1"},"error":"collection_failed"}`,
			wantErr: errAny,
		},
		"null checks":       {data: `{"id":"1","result":{"version":"v1","checks":null}}`, wantErr: errAny},
		"duplicate id":      {data: `{"id":"1","id":"1","result":{"version":"v1"}}`, wantErr: errAny},
		"case folded field": {data: `{"id":"1","Result":{"version":"v1"}}`, wantErr: errAny},
		"unknown error":     {data: `{"id":"1","error":"arbitrary text"}`, wantErr: errAny},
		"wrong id":          {data: `{"id":"2","error":"collection_failed"}`, wantErr: errAny},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := c.definition.decodeReply([]byte(tc.data), "1")
			switch tc.wantErr {
			case nil:
				require.NoError(t, err)
			case errAny:
				require.Error(t, err)
				require.NotErrorIs(t, err, errCollectionFailed)
			default:
				require.ErrorIs(t, err, tc.wantErr)
			}
		})
	}
}

var errAny = errors.New("any error")

func TestDecodeFunctionReply(t *testing.T) {
	tests := map[string]struct {
		body       string // result object, wrapped in the id 1 envelope
		reply      string // complete reply, used instead of body
		info       bool
		wantStatus int
		wantRows   string // JSON of managed data or raw_response data
		wantErr    bool
	}{
		"managed rows keep exact numbers": {
			body:       `{"version":"v1","status":200,"columns":{},"data":[[9007199254740993,null]]}`,
			wantStatus: 200,
			wantRows:   `[[9007199254740993,null]]`,
		},
		"raw rows keep exact numbers": {
			body:       `{"version":"v1","raw_response":{"status":500,"data":[[9007199254740993,null]]}}`,
			wantStatus: 500,
			wantRows:   `[[9007199254740993,null]]`,
		},
		"info without rows": {body: `{"version":"v1","status":200}`, info: true, wantStatus: 200},
		"error without rows": {
			body:       `{"version":"v1","status":503,"message":"unavailable"}`,
			wantStatus: 503,
		},
		"missing status":        {body: `{"version":"v1"}`, wantErr: true},
		"wrong version":         {body: `{"version":"v2","status":500}`, wantErr: true},
		"null status":           {body: `{"version":"v1","status":null}`, wantErr: true},
		"wrong case":            {body: `{"Version":"v1","status":500}`, wantErr: true},
		"duplicate":             {body: `{"version":"v1","status":500,"status":200}`, wantErr: true},
		"missing rows":          {body: `{"version":"v1","status":200,"columns":{}}`, wantErr: true},
		"raw and managed":       {body: `{"version":"v1","status":200,"raw_response":{"status":500}}`, wantErr: true},
		"raw string status":     {body: `{"version":"v1","raw_response":{"status":"500"}}`, wantErr: true},
		"raw fractional status": {body: `{"version":"v1","raw_response":{"status":500.1}}`, wantErr: true},
		"raw rounded fraction": {
			body:    `{"version":"v1","raw_response":{"status":500.00000000000000000001}}`,
			wantErr: true,
		},
		"raw nested duplicate": {
			body:    `{"version":"v1","raw_response":{"status":500,"data":{"x":1,"x":2}}}`,
			wantErr: true,
		},
		"invalid chart pair": {
			body:    `{"version":"v1","status":200,"columns":{},"data":[],"default_charts":[["one"]]}`,
			wantErr: true,
		},
		"reserved selector": {
			body:    `{"version":"v1","status":200,"columns":{},"data":[],"required_params":[{"id":"__job","name":"Job"}]}`,
			wantErr: true,
		},
		"null envelope": {reply: `null`, wantErr: true},
		"null result":   {reply: `{"id":"1","result":null}`, wantErr: true},
		"wrong id":      {reply: `{"id":"2","result":{"version":"v1","status":500}}`, wantErr: true},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			reply := tc.reply
			if reply == "" {
				reply = `{"id":"1","result":` + tc.body + `}`
			}
			got, err := decodeFunctionReply([]byte(reply), "1", tc.info)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			var rows any = got.Data
			status := got.Status
			if got.RawResponse != nil {
				status = got.RawResponse["status"].(int)
				rows = got.RawResponse["data"]
			}
			assert.Equal(t, tc.wantStatus, status)
			if tc.wantRows != "" {
				encoded, err := json.Marshal(rows)
				require.NoError(t, err)
				assert.JSONEq(t, tc.wantRows, string(encoded))
			}
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
