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

func TestFunctionReply(t *testing.T) {
	for name, body := range map[string]string{
		"null envelope":         `null`,
		"null result":           `{"id":"1","result":null}`,
		"wrong id":              `{"id":"2","result":{"version":"v1","status":500}}`,
		"wrong version":         `{"id":"1","result":{"version":"v2","status":500}}`,
		"missing status":        `{"id":"1","result":{"version":"v1"}}`,
		"null status":           `{"id":"1","result":{"version":"v1","status":null}}`,
		"wrong case":            `{"id":"1","result":{"Version":"v1","status":500}}`,
		"duplicate":             `{"id":"1","result":{"version":"v1","status":500,"status":200}}`,
		"missing rows":          `{"id":"1","result":{"version":"v1","status":200,"columns":{}}}`,
		"raw and managed":       `{"id":"1","result":{"version":"v1","status":200,"raw_response":{"status":500}}}`,
		"raw string status":     `{"id":"1","result":{"version":"v1","raw_response":{"status":"500"}}}`,
		"raw fractional status": `{"id":"1","result":{"version":"v1","raw_response":{"status":500.1}}}`,
		"raw rounded fraction":  `{"id":"1","result":{"version":"v1","raw_response":{"status":500.00000000000000000001}}}`,
		"raw nested duplicate":  `{"id":"1","result":{"version":"v1","raw_response":{"status":500,"data":{"x":1,"x":2}}}}`,
		"invalid chart pair":    `{"id":"1","result":{"version":"v1","status":200,"columns":{},"data":[],"default_charts":[["one"]]}}`,
		"reserved selector":     `{"id":"1","result":{"version":"v1","status":200,"columns":{},"data":[],"required_params":[{"id":"__job","name":"Job"}]}}`,
	} {
		t.Run(
			name,
			func(t *testing.T) { _, err := decodeFunctionReply([]byte(body), "1", false); require.Error(t, err) },
		)
	}
	for _, raw := range []bool{false, true} {
		body := `{"version":"v1","status":200,"columns":{},"data":[[9007199254740993,null]]}`
		if raw {
			body = `{"version":"v1","raw_response":{"status":500,"data":[[9007199254740993,null]]}}`
		}
		got, err := decodeFunctionReply([]byte(`{"id":"1","result":`+body+`}`), "1", false)
		require.NoError(t, err)
		var rows any = got.Data
		if raw {
			assert.Equal(t, 500, got.RawResponse["status"])
			rows = got.RawResponse["data"]
		}
		encoded, err := json.Marshal(rows)
		require.NoError(t, err)
		assert.JSONEq(t, `[[9007199254740993,null]]`, string(encoded))
	}
	_, err := decodeFunctionReply([]byte(`{"id":"1","result":{"version":"v1","status":200}}`), "1", true)
	require.NoError(t, err)
	got, err := decodeFunctionReply(
		[]byte(`{"id":"1","result":{"version":"v1","status":503,"message":"unavailable"}}`),
		"1",
		false,
	)
	require.NoError(t, err)
	assert.Equal(t, 503, got.Status)
}

func TestFunctionRequest(t *testing.T) {
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
	frame, err := encodeFunctionRequest(request, "7", ctx)
	require.NoError(t, err)
	var got struct {
		ID, Method, Function, Source, Permissions string
		Info                                      bool
		Args                                      []string
		Payload                                   []byte `json:"payload_base64"`
		Deadline                                  int64  `json:"deadline_unix_ms"`
	}
	require.NoError(t, json.Unmarshal(frame, &got))
	assert.Equal(t, "7", got.ID)
	assert.Equal(t, "function", got.Method)
	assert.Equal(t, request.Method, got.Function)
	assert.Equal(t, request.Args, got.Args)
	assert.Equal(t, request.Payload, got.Payload)
	assert.True(t, got.Info)
	deadline, _ := ctx.Deadline()
	assert.Equal(t, deadline.UnixMilli(), got.Deadline)
	request.Args = []string{strings.Repeat("x", maxResponseBytes)}
	_, err = encodeFunctionRequest(request, "7", ctx)
	require.ErrorContains(t, err, "exceeds 1 MiB")
	// Exact encoded boundary including the terminating LF.
	request = funcapi.RawMethodRequest{
		Method: "items",
		Args:   []string{""},
	}
	frame, err = encodeFunctionRequest(request, "7", ctx)
	require.NoError(t, err)
	request.Args[0] = strings.Repeat("x", maxResponseBytes-len(frame))
	frame, err = encodeFunctionRequest(request, "7", ctx)
	require.NoError(t, err)
	assert.Len(t, frame, maxResponseBytes)
	request.Args[0] += "x"
	_, err = encodeFunctionRequest(request, "7", ctx)
	require.Error(t, err)
}
