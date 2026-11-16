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
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/cmd/internal/discoveryproviders"
	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/agent"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery"
	"github.com/netdata/netdata/go/plugins/plugin/dem"
	"github.com/netdata/netdata/go/plugins/plugin/dem/journalstore"
	rumhistory "github.com/netdata/netdata/go/plugins/plugin/dem/rum/history"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/artifacts"
	synthetichistory "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/history"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The retained fixture: one RUM session of a retired site and two failed runs of a retired journey, one
// associated with the storefront vnode and one with another node.
const (
	siteGUID      = "11111111-1111-4111-8111-111111111111"
	otherGUID     = "33333333-3333-4333-8333-333333333333"
	retainedRunID = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

// allCollectorsDisabled is a dem.conf that selects no collector.
const allCollectorsDisabled = "modules:\n  receiver: no\n  rum: no\n  journey: no\n  lighthouse: no\n"

// retainedComponents composes DEM over a reopened journal holding the retained fixture and an empty capture store.
func retainedComponents(t *testing.T) dem.Components {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	history, err := journalstore.Open(ctx, root)
	require.NoError(t, err)
	cfg := dem.DefaultConfig()
	require.NoError(t, history.EnforceHistoryRetention(ctx, cfg.History.Days, cfg.History.MaxBytes))
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
	for id, vnode := range map[string]string{retainedRunID: siteGUID, "cccccccccccccccccccccccccccccccc": otherGUID} {
		_, err = synthetichistory.NewStore(history).AppendRun(ctx, "complete", synthetic.Run{
			ID:           id,
			VnodeGUID:    vnode,
			JobID:        "journey:retired",
			Name:         "retired",
			Kind:         synthetic.Journey,
			StartedUS:    now,
			CompletedUS:  now + 1,
			Outcome:      synthetic.Failed,
			CaptureState: "disabled",
		})
		require.NoError(t, err)
	}
	require.NoError(t, history.Close())
	history, err = journalstore.Open(ctx, root)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, history.Close()) })
	require.NoError(t, history.EnforceHistoryRetention(ctx, cfg.History.Days, cfg.History.MaxBytes))
	captures, err := artifacts.Open(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, captures.Close()) })
	return dem.New(dem.Dependencies{
		History:   history,
		Artifacts: captures,
	}, cfg)
}

// configDir writes demConf, an empty job list for every module and the storefront vnode.
func configDir(t *testing.T, demConf string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "vnodes"), 0700))
	require.NoError(
		t,
		os.WriteFile(
			filepath.Join(dir, "vnodes", "sites.yaml"),
			[]byte("- hostname: storefront\n  guid: "+siteGUID+"\n"),
			0600,
		),
	)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "dem.conf"), []byte(demConf), 0600))
	for _, module := range []string{"receiver", "rum", "journey", "lighthouse"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, module+".conf"), []byte("jobs: []\n"), 0600))
	}
	return dir
}

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

// demAgent is an Agent composed like dem.plugin, driven over its Function wire.
type demAgent struct {
	*agent.Agent
	input io.Writer
	wire  *functionWire
	done  chan error
}

// startAgent runs an Agent over components with the configuration in dir; module selects one module as the
// command line does. The test cleanup stops it.
func startAgent(t *testing.T, components dem.Components, dir, module string) *demAgent {
	t.Helper()
	a := &demAgent{
		Agent: agent.New(agent.Config{
			Name:                    "dem",
			VarLibDir:               t.TempDir(),
			PluginConfigDir:         []string{dir},
			CollectorsConfigDir:     []string{dir},
			ModuleRegistry:          components.Collectors,
			ProcessFunctions:        components.Functions,
			RunModule:               module,
			ShutdownTimeout:         time.Second,
			DisableServiceDiscovery: true,
			DiscoveryProviders:      []discovery.ProviderFactory{discoveryproviders.File(), discoveryproviders.Dummy()},
		}),
		wire: &functionWire{},
		done: make(chan error, 1),
	}
	reader, writer := io.Pipe()
	a.In, a.Out, a.input = reader, a.wire, writer
	ctx, cancel := context.WithCancel(context.Background())
	go func() { a.done <- a.RunContext(ctx); close(a.done) }()
	t.Cleanup(func() {
		cancel()
		_ = writer.Close()
		_ = reader.Close()
		select {
		case err := <-a.done:
			if err != nil {
				assert.ErrorIs(t, err, context.Canceled)
			}
		case <-time.After(3 * time.Second):
			t.Error("Agent did not join")
		}
	})
	return a
}

// waitFunctions waits until the Agent announces every named Function.
func (a *demAgent) waitFunctions(t *testing.T, names ...string) {
	t.Helper()
	require.Eventually(t, func() bool {
		wire := a.wire.String()
		for _, name := range names {
			if !strings.Contains(wire, `FUNCTION GLOBAL "`+name+`"`) {
				return false
			}
		}
		return true
	}, 3*time.Second, time.Millisecond)
}

// restart replaces the Agent run generation, as a plugin restart does.
func (a *demAgent) restart(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	require.NoError(t, a.Restart(ctx))
}

// stop terminates the Agent and requires a clean exit.
func (a *demAgent) stop(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	require.NoError(t, a.Terminate(ctx))
	require.NoError(t, <-a.done)
}

func (a *demAgent) call(t *testing.T, id, command string) map[string]any {
	t.Helper()
	return a.request(t, id, command, "", 200)
}

// request sends one Function request, with payload as its body when set, and returns the decoded response after
// checking its status. Each request needs an ID unique within the Agent's lifetime.
func (a *demAgent) request(t *testing.T, id, command, payload string, status int) map[string]any {
	t.Helper()
	permissions := "0x1b"
	if strings.HasPrefix(command, "config ") {
		permissions = "0xFFFF"
	}
	line := fmt.Sprintf("FUNCTION %s 10 %q %s \"user=fixture\"\n", id, command, permissions)
	if payload != "" {
		line = fmt.Sprintf(
			"FUNCTION_PAYLOAD %s 10 %q %s \"user=fixture\" application/json\n%s\nFUNCTION_PAYLOAD_END\n",
			id,
			command,
			permissions,
			payload,
		)
	}
	written := make(chan error, 1)
	go func() {
		_, err := io.WriteString(a.input, line)
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
		_, response, ok := strings.Cut(a.wire.String(), begin)
		if !ok {
			return false
		}
		header, response, ok = strings.Cut(response, "\n")
		if !ok {
			return false
		}
		body, _, ok = strings.Cut(response, "FUNCTION_RESULT_END")
		return ok
	}, 3*time.Second, time.Millisecond, "wire: %s", a.wire.String())
	require.True(
		t,
		strings.HasPrefix(header, fmt.Sprintf("%d application/json", status)),
		"unexpected result for %s: %s %s",
		command,
		header,
		body,
	)
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

// uiSchema compiles the Function UI schema that table responses must satisfy.
func uiSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	raw, err := os.ReadFile("../../../plugins.d/FUNCTION_UI_SCHEMA.json")
	require.NoError(t, err)
	var doc any
	require.NoError(t, json.Unmarshal(raw, &doc))
	compiler := jsonschema.NewCompiler()
	require.NoError(t, compiler.AddResource("schema.json", doc))
	schema, err := compiler.Compile("schema.json")
	require.NoError(t, err)
	return schema
}

// The process Functions are registered and read retained history whatever collectors the configuration or the
// command line selects.
func TestDEMFunctionsIndependentOfCollectorSelection(t *testing.T) {
	tests := map[string]struct{ config, module string }{
		"all collectors disabled":  {config: allCollectorsDisabled},
		"RUM and journey disabled": {config: "modules:\n  receiver: no\n  rum: no\n  journey: no\n  lighthouse: yes\n"},
		"Lighthouse CLI selection": {module: "lighthouse"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			a := startAgent(t, retainedComponents(t), configDir(t, test.config), test.module)
			a.waitFunctions(t, "rum-sessions", "synthetics-run", "synthetics-artifacts", "dem-history-status")
			rows := a.call(t, "rum", "rum-sessions site:retired")["data"].([]any)
			require.Len(t, rows, 1)
			assert.Equal(t, "retired", rows[0].([]any)[0])
			run := a.call(t, "synthetic", "synthetics-run job_id:journey:retired run_id:"+retainedRunID)["run"]
			assert.Equal(t, retainedRunID, run.(map[string]any)["id"])
			assert.Equal(t, string(synthetic.Failed), run.(map[string]any)["outcome"])
			a.stop(t)
			assert.NotContains(t, a.wire.String(), "DISABLE")
			assert.NotContains(t, a.wire.String(), "CHART ")
		})
	}
}

// The history status reports the configured retention policy.
func TestDEMHistoryStatusReportsRetentionPolicy(t *testing.T) {
	a := startAgent(t, retainedComponents(t), configDir(t, allCollectorsDisabled), "")
	a.waitFunctions(t, "dem-history-status")
	status := a.call(t, "status", "dem-history-status")
	rows := status["data"].([]any)
	require.Len(t, rows, 1)
	assert.Equal(t, float64(30), rows[0].([]any)[0])
	assert.Equal(t, float64(1<<30), rows[0].([]any)[1])
	assert.Equal(t, false, status["has_history"])
}

// The Cloud UI's Function table asks for info with its time window and posts data requests as a body; the answers
// satisfy the Function UI schema.
func TestDEMFunctionsAnswerCloudUIRequests(t *testing.T) {
	schema := uiSchema(t)
	a := startAgent(t, retainedComponents(t), configDir(t, allCollectorsDisabled), "")
	a.waitFunctions(t, "rum-pages", "rum-sessions", "synthetics-checks", "synthetics-artifacts", "dem-history-status")
	for _, method := range []string{
		"rum-pages",
		"rum-sessions",
		"synthetics-checks",
		"synthetics-journey-runs",
		"synthetics-lighthouse-runs",
		"dem-history-status",
	} {
		a.call(t, "info-"+method, method+" info after:1700000000 before:1700000900")
	}
	assert.NotContains(t, fmt.Sprint(a.call(t, "journey-info", "synthetics-journey-runs info")), "__job")
	const tableBody = `{"after":1700000000,"before":1700000900,"query":"","selections":{},"timeout":120000,"last":200}`
	tests := map[string]struct{ command, payload string }{
		"rum-sessions body":               {command: "rum-sessions", payload: tableBody},
		"synthetics-journey-runs body":    {command: "synthetics-journey-runs", payload: tableBody},
		"synthetics-lighthouse-runs body": {command: "synthetics-lighthouse-runs", payload: tableBody},
		"dem-history-status body":         {command: "dem-history-status", payload: `{"timeout":120000,"last":200}`},
		"dem-history-status":              {command: "dem-history-status"},
		"synthetics-artifacts":            {command: "synthetics-artifacts"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			response := a.request(t, strings.ReplaceAll(name, " ", "-"), test.command, test.payload, 200)
			require.NoError(t, schema.Validate(response))
		})
	}
	catalogue := a.call(t, "catalogue", "synthetics-artifacts")
	assert.Empty(t, catalogue["data"])
	assert.Contains(t, catalogue, "storage")
}

// Every vnode-scoped method selects the same rows by vnode name and by GUID, and rejects unknown, empty, mixed and
// malformed selectors.
func TestDEMFunctionVnodeSelectors(t *testing.T) {
	a := startAgent(t, retainedComponents(t), configDir(t, allCollectorsDisabled), "")
	a.waitFunctions(t, "rum-sessions", "synthetics-artifacts")
	for _, method := range []string{
		"rum-sites",
		"rum-pages",
		"rum-live",
		"rum-sessions",
		"rum-errors",
		"rum-session-events",
		"synthetics-checks",
		"synthetics-journey-runs",
		"synthetics-lighthouse-runs",
		"synthetics-artifacts",
	} {
		t.Run(method, func(t *testing.T) {
			args := ""
			if method == "rum-session-events" {
				args = " session_id:session"
			}
			byName := a.call(t, "name-"+method, method+args+" vnode:storefront")
			byGUID := a.call(t, "guid-"+method, method+args+" vnode_guid:"+siteGUID)
			assert.Equal(t, selectionData(byGUID), selectionData(byName))
			assert.Contains(t, byName["accepted_params"], "vnode")
			invalid := []string{"vnode:unknown", "vnode:", "vnode:storefront vnode_guid:", "vnode_guid:not-a-guid"}
			for n, selector := range invalid {
				a.request(t, fmt.Sprintf("invalid-%s-%d", method, n), method+args+" "+selector, "", 400)
			}
		})
	}
}

// A DynCfg vnode selects by its name, follows its GUID through updates, stops selecting once removed and, after a
// plugin restart, selects again only when the Agent replays it.
func TestDEMFunctionVnodeConfigLifecycle(t *testing.T) {
	const replacement = "22222222-2222-4222-8222-222222222222"
	a := startAgent(t, retainedComponents(t), configDir(t, allCollectorsDisabled), "")
	a.waitFunctions(t, "synthetics-journey-runs")
	runs := func(id, selector string) []any {
		data, _ := a.call(t, id, "synthetics-journey-runs "+selector)["data"].([]any)
		return data
	}
	a.request(t, "add", "config dem:vnode add selection", `{"hostname":"display-name","guid":"`+otherGUID+`"}`, 202)
	assert.Len(t, runs("added", "vnode:selection"), 1)
	a.request(
		t,
		"update",
		"config dem:vnode:selection update",
		`{"hostname":"new-display-name","guid":"`+replacement+`"}`,
		202,
	)
	assert.Empty(t, runs("updated", "vnode:selection"))
	a.request(t, "hostname-is-not-name", "synthetics-journey-runs vnode:new-display-name", "", 400)
	a.request(t, "remove", "config dem:vnode:selection remove", "", 200)
	a.request(t, "removed", "synthetics-journey-runs vnode:selection", "", 400)
	assert.Len(t, runs("retained-guid", "vnode_guid:"+otherGUID), 1)
	a.request(t, "reuse", "config dem:vnode add selection", `{"hostname":"reused-name","guid":"`+replacement+`"}`, 202)
	assert.Empty(t, runs("reused", "vnode:selection"))
	a.restart(t)
	// The Agent owns DynCfg persistence and replays configuration after plugin restart.
	a.request(t, "before-replay", "synthetics-journey-runs vnode:selection", "", 400)
	a.request(t, "replay", "config dem:vnode add selection", `{"hostname":"replayed-name","guid":"`+otherGUID+`"}`, 202)
	assert.Len(t, runs("replayed", "vnode:selection"), 1)
	a.stop(t)
}

// Every Agent run generation builds and serves through its own handler of each process Function provider.
func TestDEMFunctionHandlersPerGeneration(t *testing.T) {
	components := retainedComponents(t)
	var mu sync.Mutex
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
			mu.Lock()
			handlers[id] = append(handlers[id], h)
			mu.Unlock()
			return h
		}
	}
	a := startAgent(t, components, configDir(t, allCollectorsDisabled), "")
	a.waitFunctions(t, "rum-sessions", "synthetics-checks", "dem-history-status")
	for generation := range 2 {
		for _, command := range []string{"rum-sessions site:retired", "synthetics-checks", "dem-history-status"} {
			a.call(t, fmt.Sprintf("%s-%d", strings.Fields(command)[0], generation), command)
		}
		if generation == 0 {
			a.restart(t)
		}
	}
	a.stop(t)
	mu.Lock()
	defer mu.Unlock()
	require.Len(t, handlers, len(components.Functions))
	for id, instances := range handlers {
		require.Len(t, instances, 2, id)
		assert.NotSame(t, instances[0], instances[1], id)
	}
}

// With no job, the RUM and synthetic Functions appear only once retained data of their kind exists.
func TestDEMFunctionsWaitForDataWithoutJobs(t *testing.T) {
	history, err := journalstore.Open(t.Context(), "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, history.Close()) })
	captures, err := artifacts.Open("")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, captures.Close()) })
	components := dem.New(dem.Dependencies{
		History:   history,
		Artifacts: captures,
	}, dem.DefaultConfig())
	a := startAgent(t, components, configDir(t, allCollectorsDisabled), "")
	a.waitFunctions(t, "dem-history-status")
	a.call(t, "empty-status", "dem-history-status")
	assert.NotContains(t, a.wire.String(), `FUNCTION GLOBAL "rum-`)
	assert.NotContains(t, a.wire.String(), `FUNCTION GLOBAL "synthetics-`)
	_, err = rumhistory.NewStore(history).AppendEvent(t.Context(), rumhistory.EventRecord{
		Site:       "retired",
		SessionID:  "session",
		ObservedUS: time.Now().UnixMicro(),
		Type:       "pageview",
		Page:       "/",
	})
	require.NoError(t, err)
	a.waitFunctions(t, "rum-sites", "rum-pages", "rum-live", "rum-sessions", "rum-errors", "rum-session-events")
	assert.NotContains(t, a.wire.String(), `FUNCTION GLOBAL "synthetics-`)
	assert.NotContains(t, a.wire.String(), "hidden")
	result := a.call(t, "retained-rum", "rum-sessions site:retired")
	assert.Len(t, result["data"], 1)
}
