// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/cmd/internal/discoveryproviders"
	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/agent"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery"
	"github.com/netdata/netdata/go/plugins/plugin/dem"
	"github.com/netdata/netdata/go/plugins/plugin/dem/store"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type functionWire struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (w *functionWire) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buffer.Write(p)
}
func (w *functionWire) String() string { w.mu.Lock(); defer w.mu.Unlock(); return w.buffer.String() }
func (w *functionWire) call(t *testing.T, input io.Writer, id, command string) map[string]any {
	t.Helper()
	written := make(chan error, 1)
	go func() {
		_, err := fmt.Fprintf(input, "FUNCTION %s 10 %q 0x1b \"user=fixture\"\n", id, command)
		written <- err
	}()
	select {
	case err := <-written:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("Function ingress blocked")
	}
	begin := "FUNCTION_RESULT_BEGIN " + id + " 200 application/json"
	var body string
	require.Eventually(t, func() bool {
		_, response, ok := strings.Cut(w.String(), begin)
		if !ok {
			return false
		}
		_, response, ok = strings.Cut(response, "\n")
		if !ok {
			return false
		}
		body, _, ok = strings.Cut(response, "FUNCTION_RESULT_END")
		return ok
	}, 3*time.Second, time.Millisecond, "wire: %s", w.String())
	var response map[string]any
	require.NoError(t, json.Unmarshal([]byte(body), &response))
	return response
}
func TestDEMHistoryFunctionsIndependentOfCollectorSelection(t *testing.T) {
	tests := map[string]struct{ config, module string }{
		"all collectors disabled":  {config: "modules:\n  receiver: no\n  rum: no\n  journey: no\n  lighthouse: no\n"},
		"RUM and journey disabled": {config: "modules:\n  receiver: no\n  rum: no\n  journey: no\n  lighthouse: yes\n"},
		"Lighthouse CLI selection": {module: "lighthouse"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			history, err := store.Open(ctx, "")
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, history.Close()) })
			now := time.Now().UnixMicro()
			_, err = history.AppendRumEvent(ctx, store.RumEventRecord{
				Site:      "retired",
				SessionID: "session",
				TSUnixUS:  now,
				Type:      "pageview",
				Page:      "/checkout",
			})
			require.NoError(t, err)
			run := synthetic.Run{
				ID:           "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
				JobID:        "journey:retired",
				Name:         "retired",
				Kind:         synthetic.Journey,
				StartedUS:    now,
				CompletedUS:  now + 1,
				Outcome:      synthetic.Failed,
				CaptureState: "disabled",
			}
			_, err = history.AppendSyntheticRun(ctx, "complete", run)
			require.NoError(t, err)
			components := dem.New(dem.Dependencies{
				History: history,
			}, dem.DefaultConfig())
			var created atomic.Int32
			var handlersMu sync.Mutex
			handlers := map[string][]funcapi.MethodHandler{}
			for i := range components.Functions {
				id := components.Functions[i].ID
				factory := components.Functions[i].NewHandler
				components.Functions[i].NewHandler = func() funcapi.MethodHandler {
					h := factory()
					handlersMu.Lock()
					handlers[id] = append(handlers[id], h)
					handlersMu.Unlock()
					created.Add(1)
					return h
				}
			}
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "dem.conf"), []byte(test.config), 0600))
			for _, module := range []string{"receiver", "rum", "journey", "lighthouse"} {
				require.NoError(t, os.WriteFile(filepath.Join(dir, module+".conf"), []byte("jobs: []\n"), 0600))
			}
			a := agent.New(agent.Config{
				Name:                    "dem",
				PluginConfigDir:         []string{dir},
				CollectorsConfigDir:     []string{dir},
				ModuleRegistry:          components.Collectors,
				ProcessFunctions:        components.Functions,
				RunModule:               test.module,
				ShutdownTimeout:         time.Second,
				DisableServiceDiscovery: true,
				DiscoveryProviders:      []discovery.ProviderFactory{discoveryproviders.File(), discoveryproviders.Dummy()},
			})
			reader, writer := io.Pipe()
			output := &functionWire{}
			a.In, a.Out = reader, output
			runCtx, cancel := context.WithCancel(ctx)
			done := make(chan error, 1)
			go func() { done <- a.RunContext(runCtx); close(done) }()
			t.Cleanup(func() {
				cancel()
				_ = writer.Close()
				_ = reader.Close()
				select {
				case <-done:
				case <-time.After(3 * time.Second):
					t.Error("Agent did not join")
				}
			})
			require.Eventually(t, func() bool {
				return strings.Contains(output.String(), `FUNCTION GLOBAL "rum-sessions"`) && strings.Contains(output.String(), `FUNCTION GLOBAL "synthetics-run"`)
			}, 3*time.Second, time.Millisecond)
			for generation := 0; generation < 2; generation++ {
				suffix := fmt.Sprint(generation)
				response := output.call(t, writer, "rum"+suffix, "rum-sessions site:retired")
				rows := response["data"].([]any)
				require.Len(t, rows, 1)
				assert.Equal(t, "retired", rows[0].([]any)[0])
				response = output.call(t, writer, "synthetic"+suffix, "synthetics-run job_id:journey:retired run_id:"+run.ID)
				assert.Equal(t, run.ID, response["run"].(map[string]any)["id"])
				assert.Equal(t, string(synthetic.Failed), response["run"].(map[string]any)["outcome"])
				info := output.call(t, writer, "info"+suffix, "synthetics-runs info")
				assert.NotContains(t, fmt.Sprint(info), "__job")
				if generation == 0 {
					restartCtx, stop := context.WithTimeout(ctx, 3*time.Second)
					require.NoError(t, a.Restart(restartCtx))
					stop()
				}
			}
			stopCtx, stop := context.WithTimeout(ctx, 3*time.Second)
			defer stop()
			require.NoError(t, a.Terminate(stopCtx))
			require.NoError(t, <-done)
			require.EqualValues(t, 4, created.Load())
			handlersMu.Lock()
			defer handlersMu.Unlock()
			require.Len(t, handlers, 2)
			for _, instances := range handlers {
				require.Len(t, instances, 2)
				assert.NotSame(t, instances[0], instances[1])
			}
			assert.NotContains(t, output.String(), "DISABLE")
			assert.NotContains(t, output.String(), "CHART ")
		})
	}
}
