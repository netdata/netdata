// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"bytes"
	"context"
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

func TestPersistentCollectionRecoversQueueBudget(t *testing.T) {
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
	requests, err := os.ReadFile(filepath.Join(dir, "requests"))
	require.NoError(t, err)
	require.Contains(
		t,
		string(requests),
		`"method": "collect"`,
		"collection must reach the peer before its caller times out",
	)
	_, err = c.ExecuteFunction(ctx, funcapi.RawMethodRequest{
		Method: "items",
	})
	require.NoError(t, err, "drain the timed-out collection before the next reply")
	samples, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.NotEmpty(t, samples, "collection must still publish after recovery")
}

func TestPersistentFunctionRecovery(t *testing.T) {
	setupRunner(t)
	for _, deadline := range []bool{false, true} {
		name := "cancel"
		if deadline {
			name = "deadline"
		}
		t.Run(name, func(t *testing.T) {
			registry, dir := functionFixture(t, modePersistent, false)
			c := initFunctionCollector(t, registry)
			startRuntime(t, c).waitReady(t)
			ctx, cancel := context.WithCancel(context.Background())
			wantErr := context.Canceled
			if deadline {
				cancel()
				ctx, cancel = context.WithTimeout(context.Background(), 300*time.Millisecond)
				wantErr = context.DeadlineExceeded
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
			if !deadline {
				cancel()
			}
			select {
			case err := <-done:
				require.ErrorIs(t, err, wantErr)
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
			starts, err := os.ReadFile(filepath.Join(dir, "starts"))
			require.NoError(t, err)
			assert.Len(t, strings.Fields(string(starts)), 1, "recovery reuses the peer")
		})
	}
}

func TestPersistentFunctionRecoveryFailure(t *testing.T) {
	setupRunner(t)
	for _, malformed := range []bool{false, true} {
		name := "missing reply"
		if malformed {
			name = "invalid reply"
		}
		t.Run(name, func(t *testing.T) {
			registry, dir := functionFixture(t, modePersistent, true)
			c := initFunctionCollector(t, registry)
			runtime := startRuntime(t, c)
			runtime.waitReady(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() {
				args := []string{"wait"}
				if malformed {
					args = append(args, "malformed")
				}
				_, err := c.ExecuteFunction(ctx, funcapi.RawMethodRequest{
					Method: "items",
					Args:   args,
				})
				done <- err
			}()
			waitFile(t, filepath.Join(dir, "23.active"))
			cancel()
			require.ErrorIs(t, <-done, context.Canceled)
			if malformed {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "release"), nil, 0644))
			}
			runtime.wait(t)
			if malformed {
				require.ErrorContains(t, runtime.err, "version")
			} else {
				require.ErrorIs(t, runtime.err, context.DeadlineExceeded)
			}
		})
	}
}

func TestOneshotFunctionDiagnostics(t *testing.T) {
	setupRunner(t)
	for _, tc := range []struct{ name, body, diagnostic string }{
		{"exit", `print("private-script-output", file=sys.stderr); sys.exit(7)`, "command exited with status 7"},
		{"oversized", fmt.Sprintf(`print("x" * %d); sys.exit(0)`, maxMessageBytes+1), "response exceeds 64 MiB"},
		{"invalid", `print('{"private-script-output":true}'); sys.exit(0)`, "invalid reply"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			registry, dir := functionFixture(t, modeOneshot, true)
			peer := strings.Replace(functionPeer, "count = config[\"count\"]", tc.body, 1)
			require.NoError(t, os.WriteFile(filepath.Join(dir, "peer.py"), []byte(peer), 0644))
			c := initFunctionCollector(t, registry)
			var logs bytes.Buffer
			c.Logger = logger.NewWithWriter(&logs)
			handler := nativefunc.New(c, c.definition.methods).(funcapi.RawMethodHandler)
			result := handler.HandleRaw(context.Background(), funcapi.RawMethodRequest{
				Method:  "items",
				Args:    []string{"private-argument"},
				Payload: []byte("private-payload"),
				Source:  "private-source",
			})
			assert.Equal(t, 502, result.Status)
			assert.Contains(t, logs.String(), "one-shot Function")
			assert.Contains(t, logs.String(), tc.diagnostic)
			assert.NotContains(t, logs.String(), "private-")
			assert.NotContains(t, logs.String(), dir)
			assert.NotContains(t, logs.String(), "synthetic")
		})
	}
}
