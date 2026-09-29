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
	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/collector/native/nativefunc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func readLines(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

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
			_, err := tc.collector(t).ExecuteFunction(context.Background(), funcapi.RawMethodRequest{
				Method: tc.method,
			})
			require.ErrorContains(t, err, "unavailable")
			requireNoFile(t, filepath.Join(dir, "starts"))
		})
	}
}

func TestFunctionPeersBothModes(t *testing.T) {
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
			result, err := c.ExecuteFunction(ctx, input)
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
			result, err = c.ExecuteFunction(ctx, funcapi.RawMethodRequest{
				Method: "items",
				Args:   []string{"error"},
			})
			require.NoError(t, err)
			assert.Equal(t, 503, result.Status)
			_, err = c.ExecuteFunction(ctx, funcapi.RawMethodRequest{
				Method:  "items",
				Payload: make([]byte, maxMessageBytes*3/4),
			})
			require.ErrorContains(t, err, "exceeds 64 MiB", "oversized requests are rejected before reaching the peer")
			_, err = c.ExecuteFunction(ctx, funcapi.RawMethodRequest{
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

func TestPersistentFunctionQueue(t *testing.T) {
	setupRunner(t)
	registry, dir := functionFixture(t, modePersistent, false)
	c := initFunctionCollector(t, registry)
	startRuntime(t, c).waitReady(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	active := make(chan error, 1)
	go func() {
		_, err := c.ExecuteFunction(ctx, funcapi.RawMethodRequest{
			Method: "items",
			Args:   []string{"wait"},
		})
		active <- err
	}()
	waitFile(t, filepath.Join(dir, "23.active"))
	queued, queuedCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer queuedCancel()
	_, err := c.ExecuteFunction(queued, funcapi.RawMethodRequest{
		Method: "items",
		Args:   []string{"must-not-reach-peer"},
	})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	// Collection's deadline includes time spent waiting behind an interactive call.
	_, err = c.collectPersistent(context.Background())
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "release"), nil, 0644))
	select {
	case err := <-active:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatal("active call stuck")
	}
	_, err = c.collectPersistent(context.Background())
	require.NoError(t, err)
	requests := readLines(t, filepath.Join(dir, "requests"))
	assert.NotContains(t, strings.Join(requests, "\n"), "must-not-reach-peer")
	assert.Len(t, requests, 2)
}

func TestPersistentFunction_CanceledRequestAtDequeue(t *testing.T) {
	setupRunner(t)
	registry, dir := functionFixture(t, modePersistent, true)
	c := initFunctionCollector(t, registry)
	startRuntime(t, c).waitReady(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// Exercise the actual receive boundary deterministically: a select may deliver
	// a sender whose context is already canceled. No synthetic runtime is installed.
	c.runtimeMu.Lock()
	r := c.runtime
	c.runtimeMu.Unlock()
	request := scriptRequest{
		ctx: ctx,
		function: &funcapi.RawMethodRequest{
			Method: "items",
		},
		reply: make(chan scriptResult, 1),
	}
	select {
	case r.requests <- request:
	case <-time.After(time.Second):
		t.Fatal("request not received")
	}
	select {
	case result := <-request.reply:
		require.ErrorIs(t, result.err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("request not rejected")
	}
	_, err := c.ExecuteFunction(context.Background(), funcapi.RawMethodRequest{
		Method: "items",
	})
	require.NoError(t, err)
	assert.Len(t, readLines(t, filepath.Join(dir, "requests")), 1)
}

func TestFunction_ActiveCancellation(t *testing.T) {
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
				_, err := c.ExecuteFunction(ctx, funcapi.RawMethodRequest{
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
			_, err := c.ExecuteFunction(context.Background(), funcapi.RawMethodRequest{
				Method: "items",
			})
			require.Error(t, err)
		})
	}
}

func TestPersistentFunction_MalformedReplyStopsSession(t *testing.T) {
	setupRunner(t)
	registry, _ := functionFixture(t, modePersistent, true)
	c := initFunctionCollector(t, registry)
	r := startRuntime(t, c)
	r.waitReady(t)
	_, err := c.ExecuteFunction(context.Background(), funcapi.RawMethodRequest{
		Method: "items",
		Args:   []string{"malformed"},
	})
	require.Error(t, err)
	r.wait(t)
	require.Error(t, r.err)
}

func TestPersistentCollection_RecoversQueueBudget(t *testing.T) {
	setupRunner(t)
	registry, dir := functionFixture(t, modePersistent, false)
	peer := strings.Replace(functionPeer, `if req["method"] == "collect":`, `if req["method"] == "collect":
        time.sleep(0.5)`, 1)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "peer.py"), []byte(peer), 0644))
	c := initFunctionCollector(t, registry)
	c.Timeout = confopt.Duration(time.Second)
	startRuntime(t, c).waitReady(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	active := make(chan error, 1)
	go func() {
		_, err := c.ExecuteFunction(ctx, funcapi.RawMethodRequest{
			Method: "items",
			Args:   []string{"wait"},
		})
		active <- err
	}()
	waitFile(t, filepath.Join(dir, "23.active"))
	collected := make(chan error, 1)
	go func() { collected <- c.Collect(ctx) }()
	// Leave less than the peer's 500ms operation time in the caller budget.
	time.Sleep(700 * time.Millisecond)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "release"), nil, 0644))
	require.NoError(t, <-active)
	require.ErrorIs(t, <-collected, context.DeadlineExceeded)
	requests := strings.Join(readLines(t, filepath.Join(dir, "requests")), "\n")
	require.Contains(t, requests, `"method": "collect"`, "collection must reach the peer before its caller times out")
	_, err := c.ExecuteFunction(ctx, funcapi.RawMethodRequest{
		Method: "items",
	})
	require.NoError(t, err, "drain the timed-out collection before the next reply")
	samples, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.NotEmpty(t, samples, "collection must still publish after recovery")
}

// A caller that stops waiting leaves the peer's owed reply to be drained; the
// session survives and the late reply is never delivered to another caller.
func TestPersistentFunction_CallerStopRecovery(t *testing.T) {
	setupRunner(t)
	tests := map[string]struct {
		deadline bool // caller deadline instead of explicit cancellation
		wantErr  error
	}{
		"cancel":   {deadline: false, wantErr: context.Canceled},
		"deadline": {deadline: true, wantErr: context.DeadlineExceeded},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			registry, dir := functionFixture(t, modePersistent, false)
			c := initFunctionCollector(t, registry)
			startRuntime(t, c).waitReady(t)
			ctx, cancel := context.WithCancel(context.Background())
			if tc.deadline {
				cancel()
				ctx, cancel = context.WithTimeout(context.Background(), 300*time.Millisecond)
			}
			defer cancel()
			done := make(chan error, 1)
			go func() {
				_, err := c.ExecuteFunction(ctx, funcapi.RawMethodRequest{
					Method: "items",
					Args:   []string{"wait"},
				})
				done <- err
			}()
			waitFile(t, filepath.Join(dir, "23.active"))
			if !tc.deadline {
				cancel()
			}
			select {
			case err := <-done:
				require.ErrorIs(t, err, tc.wantErr)
			case <-time.After(time.Second):
				t.Fatal("caller kept waiting for recovery")
			}
			// The caller has already returned while the peer still owes its reply.
			require.NoError(t, os.WriteFile(filepath.Join(dir, "release"), nil, 0644))
			nextCtx, nextCancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer nextCancel()
			result, err := c.ExecuteFunction(nextCtx, funcapi.RawMethodRequest{
				Method: "items",
				Info:   true,
			})
			require.NoError(t, err)
			assert.Nil(t, result.Data, "the canceled request's data must not become the info reply")
			_, err = c.collectPersistent(nextCtx)
			require.NoError(t, err)
			assert.Len(t, readLines(t, filepath.Join(dir, "starts")), 1, "recovery reuses the peer")
		})
	}
}

func TestPersistentFunction_CallerStopRecoveryFailure(t *testing.T) {
	setupRunner(t)
	tests := map[string]struct {
		releaseReply bool // false: the peer never replies within the drain budget
		args         []string
		wantErrIs    error
		wantErrText  string
	}{
		"missing reply": {
			args:      []string{"wait"},
			wantErrIs: context.DeadlineExceeded,
		},
		"invalid reply": {
			releaseReply: true,
			args:         []string{"wait", "malformed"},
			wantErrText:  "version",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			registry, dir := functionFixture(t, modePersistent, true)
			c := initFunctionCollector(t, registry)
			r := startRuntime(t, c)
			r.waitReady(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() {
				_, err := c.ExecuteFunction(ctx, funcapi.RawMethodRequest{
					Method: "items",
					Args:   tc.args,
				})
				done <- err
			}()
			waitFile(t, filepath.Join(dir, "23.active"))
			cancel()
			require.ErrorIs(t, <-done, context.Canceled)
			if tc.releaseReply {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "release"), nil, 0644))
			}
			r.wait(t)
			if tc.wantErrIs != nil {
				require.ErrorIs(t, r.err, tc.wantErrIs)
			}
			if tc.wantErrText != "" {
				require.ErrorContains(t, r.err, tc.wantErrText)
			}
		})
	}
}

func TestOneshotFunction_Diagnostics(t *testing.T) {
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
			router := nativefunc.NewRouter(c, c.definition.methods).(funcapi.RawMethodHandler)
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
