// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/collector/native/nativefunc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollector_ExecuteFunctionUnavailable(t *testing.T) {
	registry, dir := functionFixture(t, modeOneshot, false)
	tests := map[string]struct {
		collector func(t *testing.T) *Collector
		method    string
	}{
		"not initialized": {
			collector: func(*testing.T) *Collector { return registry["native-fixture"].CreateV2().(*Collector) },
			method:    "items",
		},
		"undeclared method": {
			collector: func(t *testing.T) *Collector { return initFunctionCollector(t, registry) },
			method:    "other",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := tc.collector(t).executeFunction(context.Background(), funcapi.RawMethodRequest{
				Method: tc.method,
			})
			require.ErrorContains(t, err, "unavailable")
			requireNoFile(t, filepath.Join(dir, "starts"))
		})
	}
}

func TestCollector_ExecuteFunction(t *testing.T) {
	setupRunner(t)
	for _, mode := range []string{modeOneshot, modePersistent} {
		t.Run(mode, func(t *testing.T) {
			registry, dir := functionFixture(t, mode, true)
			c := initFunctionCollector(t, registry)
			requireNoFile(t, filepath.Join(dir, "starts"))
			if mode == modePersistent {
				startRuntime(t, c).waitReady(t)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			input := funcapi.RawMethodRequest{
				Method:      "items",
				Args:        []string{"queue:undeclared", "filter:quotes \" λ\n"},
				Payload:     []byte{0, 255, 10},
				ContentType: "application/octet-stream",
				Permissions: "0xFFFF",
				Source:      "synthetic",
			}
			result, err := c.executeFunction(ctx, input)
			require.NoError(t, err)
			require.Equal(t, 200, result.Status)
			rows := result.Data.([][]any)
			require.Len(t, rows, 1)
			assert.Equal(t, json.Number("23"), rows[0][0])
			var echoed struct {
				Args        []string
				Payload     []byte `json:"payload_base64"`
				ContentType string `json:"content_type"`
			}
			require.NoError(t, json.Unmarshal([]byte(rows[0][1].(string)), &echoed))
			// Raw handlers deliberately receive selectors untouched, including undeclared values.
			assert.Equal(t, input.Args, echoed.Args)
			assert.Equal(t, input.Payload, echoed.Payload)
			assert.Equal(t, input.ContentType, echoed.ContentType)
			result, err = c.executeFunction(ctx, funcapi.RawMethodRequest{
				Method: "items",
				Args:   []string{"error"},
			})
			require.NoError(t, err)
			assert.Equal(t, 503, result.Status)
			_, err = c.executeFunction(ctx, funcapi.RawMethodRequest{
				Method:  "items",
				Payload: make([]byte, maxMessageBytes*3/4),
			})
			require.ErrorContains(t, err, "exceeds 64 MiB", "oversized requests are rejected before reaching the peer")
			_, err = c.executeFunction(ctx, funcapi.RawMethodRequest{
				Method: "items",
				Info:   true,
			})
			require.NoError(t, err)
			wantStarts := 3
			if mode == modePersistent {
				wantStarts = 1
			}
			assert.Len(t, readLines(t, filepath.Join(dir, "starts")), wantStarts)
			assert.Len(t, readLines(t, filepath.Join(dir, "requests")), 3)
		})
	}
}

func TestCollector_ExecuteFunctionActiveCancellation(t *testing.T) {
	setupRunner(t)
	for _, mode := range []string{modeOneshot, modePersistent} {
		t.Run(mode, func(t *testing.T) {
			registry, dir := functionFixture(t, mode, true)
			c := initFunctionCollector(t, registry)
			var r *testRuntime
			if mode == modePersistent {
				r = startRuntime(t, c)
				r.waitReady(t)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() {
				_, err := c.executeFunction(ctx, funcapi.RawMethodRequest{
					Method: "items",
					Args:   []string{"wait"},
				})
				done <- err
			}()
			waitFile(t, filepath.Join(dir, "23.active"))
			cancel()
			select {
			case err := <-done:
				require.ErrorIs(t, err, context.Canceled)
			case <-time.After(3 * time.Second):
				t.Fatal("Function not canceled")
			}
			if r == nil {
				return
			}
			// The peer never replies: the drain budget expires and stops the session.
			r.wait(t)
			require.ErrorIs(t, r.err, context.DeadlineExceeded)
			_, err := c.executeFunction(context.Background(), funcapi.RawMethodRequest{
				Method: "items",
			})
			require.Error(t, err)
		})
	}
}

func TestCollector_ExecuteFunctionOneshotDiagnostics(t *testing.T) {
	setupRunner(t)
	tests := map[string]struct {
		peerBody       string
		wantDiagnostic string
	}{
		"exit": {
			peerBody:       `print("private-script-output", file=sys.stderr); sys.exit(7)`,
			wantDiagnostic: "command exited with status 7",
		},
		"oversized": {
			peerBody:       fmt.Sprintf(`print("x" * %d); sys.exit(0)`, maxMessageBytes+1),
			wantDiagnostic: "response exceeds 64 MiB",
		},
		"invalid": {
			peerBody:       `print('{"private-script-output":true}'); sys.exit(0)`,
			wantDiagnostic: "invalid reply",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			registry, dir := functionFixture(t, modeOneshot, true)
			peer := strings.Replace(functionPeer, "count = config[\"count\"]", tc.peerBody, 1)
			require.NoError(t, os.WriteFile(filepath.Join(dir, "peer.py"), []byte(peer), 0644))
			c := initFunctionCollector(t, registry)
			var logs bytes.Buffer
			c.Logger = logger.NewWithWriter(&logs)
			router := nativefunc.NewRouter(funcDeps{
				c: c,
			}, c.definition.methods).(funcapi.RawMethodHandler)
			result := router.HandleRaw(context.Background(), funcapi.RawMethodRequest{
				Method:  "items",
				Args:    []string{"private-argument"},
				Payload: []byte("private-payload"),
				Source:  "private-source",
			})
			assert.Equal(t, 502, result.Status)
			assert.Contains(t, logs.String(), "one-shot Function")
			assert.Contains(t, logs.String(), tc.wantDiagnostic)
			assert.NotContains(t, logs.String(), "private-")
			assert.NotContains(t, logs.String(), dir)
			assert.NotContains(t, logs.String(), "synthetic")
		})
	}
}
