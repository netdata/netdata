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
	demjournal "github.com/netdata/netdata/go/plugins/plugin/dem/journal"
	rumhistory "github.com/netdata/netdata/go/plugins/plugin/dem/rum/history"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/artifacts"
	synthetichistory "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/history"
	"github.com/santhosh-tekuri/jsonschema/v6"
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
	return w.request(t, input, id, command, "", 200)
}
func (w *functionWire) request(t *testing.T, input io.Writer, id, command, payload string, status int) map[string]any {
	t.Helper()
	permissions := "0x1b"
	if strings.HasPrefix(command, "config ") {
		permissions = "0xFFFF"
	}
	line := fmt.Sprintf("FUNCTION %s 10 %q %s \"user=fixture\"\n", id, command, permissions)
	if payload != "" {
		line = fmt.Sprintf("FUNCTION_PAYLOAD %s 10 %q %s \"user=fixture\" application/json\n%s\nFUNCTION_PAYLOAD_END\n", id, command, permissions, payload)
	}
	written := make(chan error, 1)
	go func() {
		_, err := io.WriteString(input, line)
		written <- err
	}()
	select {
	case err := <-written:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("Function ingress blocked")
	}
	begin := "FUNCTION_RESULT_BEGIN " + id + " "
	var body, header string
	require.Eventually(t, func() bool {
		_, response, ok := strings.Cut(w.String(), begin)
		if !ok {
			return false
		}
		header, response, ok = strings.Cut(response, "\n")
		if !ok {
			return false
		}
		body, _, ok = strings.Cut(response, "FUNCTION_RESULT_END")
		return ok
	}, 3*time.Second, time.Millisecond, "wire: %s", w.String())
	require.True(t, strings.HasPrefix(header, fmt.Sprintf("%d application/json", status)), "unexpected result for %s: %s %s", command, header, body)
	var response map[string]any
	require.NoError(t, json.Unmarshal([]byte(body), &response))
	return response
}

// selectionData returns a table response's rows without its age columns. An
// age counts whole seconds back from the clock of its own call, so two calls
// that select the same rows can differ by one across a second boundary.
func selectionData(response map[string]any) any {
	columns, _ := response["columns"].(map[string]any)
	rows, _ := response["data"].([]any)
	for key, column := range columns {
		if !strings.HasSuffix(key, "_age_s") {
			continue
		}
		index := int(column.(map[string]any)["index"].(float64))
		for _, row := range rows {
			row.([]any)[index] = nil
		}
	}
	return response["data"]
}

func TestDEMHistoryFunctionsIndependentOfCollectorSelection(t *testing.T) {
	raw, err := os.ReadFile("../../../plugins.d/FUNCTION_UI_SCHEMA.json")
	require.NoError(t, err)
	var doc any
	require.NoError(t, json.Unmarshal(raw, &doc))
	compiler := jsonschema.NewCompiler()
	require.NoError(t, compiler.AddResource("schema.json", doc))
	schema, err := compiler.Compile("schema.json")
	require.NoError(t, err)

	tests := map[string]struct{ config, module string }{
		"all collectors disabled":  {config: "modules:\n  receiver: no\n  rum: no\n  journey: no\n  lighthouse: no\n"},
		"RUM and journey disabled": {config: "modules:\n  receiver: no\n  rum: no\n  journey: no\n  lighthouse: yes\n"},
		"Lighthouse CLI selection": {module: "lighthouse"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			historyRoot := t.TempDir()
			history, err := demjournal.Open(ctx, historyRoot)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, history.Close()) })
			cfg := dem.DefaultConfig()
			require.NoError(t, history.EnforceHistoryRetention(ctx, cfg.History.Days, cfg.History.MaxBytes))
			const siteGUID = "11111111-1111-4111-8111-111111111111"
			now := time.Now().UnixMicro()
			_, err = rumhistory.NewStore(history).AppendEvent(ctx, rumhistory.EventRecord{
				Site:       "retired",
				VnodeGUID:  siteGUID,
				SessionID:  "session",
				ObservedUS: now,
				Type:       "pageview",
				Page:       "/checkout",
			})
			require.NoError(t, err)
			run := synthetic.Run{
				ID:           "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
				VnodeGUID:    siteGUID,
				JobID:        "journey:retired",
				Name:         "retired",
				Kind:         synthetic.Journey,
				StartedUS:    now,
				CompletedUS:  now + 1,
				Outcome:      synthetic.Failed,
				CaptureState: "disabled",
			}
			_, err = synthetichistory.NewStore(history).AppendRun(ctx, "complete", run)
			require.NoError(t, err)
			other := run
			other.ID = "cccccccccccccccccccccccccccccccc"
			other.VnodeGUID = "33333333-3333-4333-8333-333333333333"
			_, err = synthetichistory.NewStore(history).AppendRun(ctx, "complete", other)
			require.NoError(t, err)
			captures, err := artifacts.Open(t.TempDir())
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, captures.Close()) })
			require.NoError(t, history.Close())
			history, err = demjournal.Open(ctx, historyRoot)
			require.NoError(t, err)
			require.NoError(t, history.EnforceHistoryRetention(ctx, cfg.History.Days, cfg.History.MaxBytes))
			components := dem.New(dem.Dependencies{
				Artifacts: captures,
				History:   history,
			}, dem.DefaultConfig())
			var created atomic.Int32
			var handlersMu sync.Mutex
			handlers := map[string][]funcapi.MethodHandler{}
			for i := range components.Functions {
				id := components.Functions[i].ID
				factory := components.Functions[i].NewHandler
				contextFactory := components.Functions[i].NewHandlerWithContext
				components.Functions[i].NewHandler = nil
				components.Functions[i].NewHandlerWithContext = func(ctx funcapi.ProcessFunctionContext) funcapi.MethodHandler {
					var h funcapi.MethodHandler
					if contextFactory != nil {
						h = contextFactory(ctx)
					} else {
						h = factory()
					}
					handlersMu.Lock()
					handlers[id] = append(handlers[id], h)
					handlersMu.Unlock()
					created.Add(1)
					return h
				}
			}
			dir := t.TempDir()
			require.NoError(t, os.Mkdir(filepath.Join(dir, "vnodes"), 0700))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "vnodes", "sites.yaml"), []byte("- hostname: storefront\n  guid: "+siteGUID+"\n"), 0600))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "dem.conf"), []byte(test.config), 0600))
			for _, module := range []string{"receiver", "rum", "journey", "lighthouse"} {
				require.NoError(t, os.WriteFile(filepath.Join(dir, module+".conf"), []byte("jobs: []\n"), 0600))
			}
			a := agent.New(agent.Config{
				Name:                    "dem",
				VarLibDir:               t.TempDir(),
				PluginConfigDir:         []string{dir},
				CollectorsConfigDir:     []string{dir},
				ModuleRegistry:          components.Collectors,
				ProcessFunctions:        components.Functions,
				RunModule:               test.module,
				ShutdownTimeout:         time.Second,
				DisableServiceDiscovery: true,
				DiscoveryProviders: []discovery.ProviderFactory{
					discoveryproviders.File(),
					discoveryproviders.Dummy(),
				},
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
				return strings.Contains(output.String(), `FUNCTION GLOBAL "rum-sessions"`) &&
					strings.Contains(output.String(), `FUNCTION GLOBAL "synthetics-run"`) &&
					strings.Contains(output.String(), `FUNCTION GLOBAL "synthetics-artifacts"`) &&
					strings.Contains(output.String(), `FUNCTION GLOBAL "dem-history-status"`)
			}, 3*time.Second, time.Millisecond)
			for generation := 0; generation < 2; generation++ {
				suffix := fmt.Sprint(generation)
				catalogue := output.call(t, writer, "catalogue"+suffix, "synthetics-artifacts")
				require.NoError(t, schema.Validate(catalogue))
				assert.Empty(t, catalogue["data"])
				assert.Contains(t, catalogue, "storage")
				status := output.call(t, writer, "status"+suffix, "dem-history-status")
				require.NoError(t, schema.Validate(status))
				statusRows := status["data"].([]any)
				require.Len(t, statusRows, 1)
				assert.Equal(t, float64(30), statusRows[0].([]any)[0])
				assert.Equal(t, float64(1<<30), statusRows[0].([]any)[1])
				assert.Equal(t, false, status["has_history"])
				response := output.call(t, writer, "rum"+suffix, "rum-sessions site:retired")
				rows := response["data"].([]any)
				require.Len(t, rows, 1)
				assert.Equal(t, "retired", rows[0].([]any)[0])
				response = output.call(
					t,
					writer,
					"synthetic"+suffix,
					"synthetics-run job_id:journey:retired run_id:"+run.ID,
				)
				assert.Equal(t, run.ID, response["run"].(map[string]any)["id"])
				assert.Equal(t, string(synthetic.Failed), response["run"].(map[string]any)["outcome"])
				info := output.call(t, writer, "info"+suffix, "synthetics-runs info")
				assert.NotContains(t, fmt.Sprint(info), "__job")
				for _, method := range []string{"rum-sites", "rum-pages", "rum-live", "rum-sessions", "rum-errors", "rum-session-events", "synthetics-checks", "synthetics-runs", "synthetics-artifacts"} {
					id := method + suffix
					args := ""
					if method == "rum-session-events" {
						args = " session_id:session"
					}
					byName := output.call(t, writer, "name-"+id, method+args+" vnode:storefront")
					byGUID := output.call(t, writer, "guid-"+id, method+args+" vnode_guid:"+siteGUID)
					assert.Equal(t, selectionData(byGUID), selectionData(byName))
					assert.Contains(t, byName["accepted_params"], "vnode")
					for n, selector := range []string{"vnode:unknown", "vnode:", "vnode:storefront vnode_guid:", "vnode:storefront vnode:storefront"} {
						output.request(t, writer, fmt.Sprintf("invalid-%s-%d", id, n), method+args+" "+selector, "", 400)
					}
				}
				if generation == 0 {
					const replacement = "22222222-2222-4222-8222-222222222222"
					output.request(t, writer, "vnode-add", "config dem:vnode add selection", `{"hostname":"display-name","guid":"33333333-3333-4333-8333-333333333333"}`, 202)
					selected := output.call(t, writer, "dynamic-before", "synthetics-runs vnode:selection")
					assert.Len(t, selected["data"], 1)
					output.request(t, writer, "vnode-update", "config dem:vnode:selection update", `{"hostname":"new-display-name","guid":"`+replacement+`"}`, 202)
					updated := output.call(t, writer, "dynamic-updated", "synthetics-runs vnode:selection")
					assert.Empty(t, updated["data"])
					output.request(t, writer, "hostname-is-not-name", "synthetics-runs vnode:new-display-name", "", 400)
					output.request(t, writer, "vnode-remove", "config dem:vnode:selection remove", "", 200)
					output.request(t, writer, "removed-name", "synthetics-runs vnode:selection", "", 400)
					fallback := output.call(t, writer, "retained-guid", "synthetics-runs vnode_guid:"+other.VnodeGUID)
					assert.Len(t, fallback["data"], 1)
					output.request(t, writer, "vnode-reuse", "config dem:vnode add selection", `{"hostname":"reused-name","guid":"`+replacement+`"}`, 202)
				}
				if generation == 1 {
					// The Agent owns DynCfg persistence and replays configuration after plugin restart.
					output.request(t, writer, "before-replay", "synthetics-runs vnode:selection", "", 400)
					output.request(t, writer, "vnode-replay", "config dem:vnode add selection", `{"hostname":"replayed-name","guid":"`+other.VnodeGUID+`"}`, 202)
				}
				selection := output.call(t, writer, "dynamic-current"+suffix, "synthetics-runs vnode:selection")
				assert.Len(t, selection["data"], generation)
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
			require.EqualValues(t, 6, created.Load())
			handlersMu.Lock()
			defer handlersMu.Unlock()
			require.Len(t, handlers, 3)
			for _, instances := range handlers {
				require.Len(t, instances, 2)
				assert.NotSame(t, instances[0], instances[1])
			}
			assert.NotContains(t, output.String(), "DISABLE")
			assert.NotContains(t, output.String(), "CHART ")
		})
	}
}

func TestDEMFunctionsWaitForDataWithoutJobs(t *testing.T) {
	history, err := demjournal.Open(t.Context(), "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, history.Close()) })
	captures, err := artifacts.Open("")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, captures.Close()) })
	components := dem.New(dem.Dependencies{
		History:   history,
		Artifacts: captures,
	}, dem.DefaultConfig())
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "dem.conf"), []byte("modules:\n  receiver: no\n  rum: no\n  journey: no\n  lighthouse: no\n"), 0600))
	a := agent.New(agent.Config{
		Name:                    "dem",
		VarLibDir:               t.TempDir(),
		PluginConfigDir:         []string{dir},
		CollectorsConfigDir:     []string{dir},
		ModuleRegistry:          components.Collectors,
		ProcessFunctions:        components.Functions,
		ShutdownTimeout:         time.Second,
		DisableServiceDiscovery: true,
		DiscoveryProviders:      []discovery.ProviderFactory{discoveryproviders.File(), discoveryproviders.Dummy()},
	})
	reader, writer := io.Pipe()
	output := &functionWire{}
	a.In, a.Out = reader, output
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- a.RunContext(ctx) }()
	t.Cleanup(func() {
		cancel()
		_ = writer.Close()
		_ = reader.Close()
		select {
		case err := <-done:
			if err != nil {
				require.ErrorIs(t, err, context.Canceled)
			}
		case <-time.After(3 * time.Second):
			t.Error("Agent did not join")
		}
	})
	require.Eventually(t, func() bool { return strings.Contains(output.String(), `FUNCTION GLOBAL "dem-history-status"`) }, 3*time.Second, time.Millisecond)
	output.call(t, writer, "empty-status", "dem-history-status")
	assert.NotContains(t, output.String(), `FUNCTION GLOBAL "rum-`)
	assert.NotContains(t, output.String(), `FUNCTION GLOBAL "synthetics-`)
	_, err = rumhistory.NewStore(history).AppendEvent(t.Context(), rumhistory.EventRecord{
		Site:       "retired",
		SessionID:  "session",
		ObservedUS: time.Now().UnixMicro(),
		Type:       "pageview",
		Page:       "/",
	})
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		wire := output.String()
		for _, method := range []string{"rum-sites", "rum-pages", "rum-live", "rum-sessions", "rum-errors", "rum-session-events"} {
			if !strings.Contains(wire, `FUNCTION GLOBAL "`+method+`"`) {
				return false
			}
		}
		return true
	}, 3*time.Second, time.Millisecond)
	assert.NotContains(t, output.String(), `FUNCTION GLOBAL "synthetics-`)
	assert.NotContains(t, output.String(), "hidden")
	result := output.call(t, writer, "retained-rum", "rum-sessions site:retired")
	assert.Len(t, result["data"], 1)
}
