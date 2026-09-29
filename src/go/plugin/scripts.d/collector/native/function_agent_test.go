// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery"
	discoveryfile "github.com/netdata/netdata/go/plugins/plugin/agent/discovery/file"
	"github.com/netdata/netdata/go/plugins/plugin/agent/policy"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

type nativeAgent struct {
	input *io.PipeWriter
	out   *jobOutput
}

func startNativeAgent(t *testing.T, registry collectorapi.Registry, dir string) *nativeAgent {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	reader, writer := io.Pipe()
	out := &jobOutput{}
	a := agent.New(
		agent.Config{
			Name:                    "scripts.d",
			ModuleRegistry:          registry,
			PluginConfigDir:         []string{dir},
			VarLibDir:               dir,
			RunModePolicy:           policy.Agent(false),
			DisableServiceDiscovery: true,
			DiscoveryProviders: []discovery.ProviderFactory{
				discovery.NewProviderFactory(
					"file",
					func(build discovery.BuildContext) (discovery.Discoverer, bool, error) {
						d, err := discoveryfile.NewDiscovery(
							discoveryfile.Config{
								Registry: build.Registry,
								Watch:    []string{filepath.Join(dir, "*.conf")},
							},
						)
						return d, true, err
					},
				),
			},
		},
	)
	a.In = reader
	a.Out = out
	done := make(chan error, 1)
	go func() { defer reader.Close(); done <- a.RunContext(ctx) }()
	t.Cleanup(func() {
		cancel()
		_ = writer.Close()
		_ = reader.Close()
		select {
		case err := <-done:
			if err != nil {
				assert.True(t, agent.ContainsOnlyProcessControlErrors(err, context.Canceled), "%v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("Agent did not stop")
		}
	})
	require.Eventually(
		t,
		func() bool { return strings.Contains(out.String(), "scripts.d:collector:native-fixture") },
		5*time.Second,
		10*time.Millisecond,
	)
	return &nativeAgent{
		input: writer,
		out:   out,
	}
}
func (a *nativeAgent) send(t *testing.T, id, command, payload string) {
	t.Helper()
	var err error
	if payload == "" {
		_, err = fmt.Fprintf(a.input, "FUNCTION %s 5 %q 0xFFFF \"user=test\"\n", id, command)
	} else {
		_, err = fmt.Fprintf(a.input, "FUNCTION_PAYLOAD %s 5 %q 0xFFFF \"user=test\" application/json\n%s\nFUNCTION_PAYLOAD_END\n", id, command, payload)
	}
	require.NoError(t, err)
}
func (a *nativeAgent) result(t *testing.T, id string, code int) string {
	t.Helper()
	var result string
	require.Eventually(t, func() bool {
		wire := a.out.String()
		start := strings.Index(wire, "FUNCTION_RESULT_BEGIN "+id+" ")
		if start < 0 {
			return false
		}
		end := strings.Index(wire[start:], "FUNCTION_RESULT_END")
		if end < 0 {
			return false
		}
		result = wire[start : start+end]
		return true
	}, 6*time.Second, 10*time.Millisecond)
	if code != 0 {
		require.Contains(t, result, fmt.Sprintf("FUNCTION_RESULT_BEGIN %s %d ", id, code))
	}
	return result
}
func (a *nativeAgent) call(t *testing.T, id, command, payload string, code int) string {
	t.Helper()
	a.send(t, id, command, payload)
	return a.result(t, id, code)
}
func (a *nativeAgent) enable(t *testing.T, name string, count int) {
	t.Helper()
	a.call(
		t,
		"add-"+name,
		"config scripts.d:collector:native-fixture add "+name,
		fmt.Sprintf(`{"update_every":1,"config":{"text":"synthetic","count":%d}}`, count),
		202,
	)
	a.call(t, "enable-"+name, "config scripts.d:collector:native-fixture:"+name+" enable", "", 202)
	require.Eventually(t, func() bool {
		return strings.Contains(a.out.String(), "CONFIG scripts.d:collector:native-fixture:"+name+" status running")
	}, 5*time.Second, 10*time.Millisecond)
}
func functionJSON(t *testing.T, result string) map[string]any {
	t.Helper()
	_, body, ok := strings.Cut(result, "\n")
	require.True(t, ok)
	var data map[string]any
	require.NoError(t, json.Unmarshal([]byte(body), &data))
	return data
}
func validateFunctionUI(t *testing.T, result string) {
	t.Helper()
	path, err := filepath.Abs("../../../../../plugins.d/FUNCTION_UI_SCHEMA.json")
	require.NoError(t, err)
	compiler := jsonschema.NewCompiler()
	schema, err := compiler.Compile(path)
	require.NoError(t, err)
	require.NoError(t, schema.Validate(functionJSON(t, result)))
}

func TestFunctionAgentRouting(t *testing.T) {
	setupRunner(t)
	for _, mode := range []string{modeOneshot, modePersistent} {
		t.Run(mode, func(t *testing.T) {
			registry, dir := functionFixture(t, mode, true)
			a := startNativeAgent(t, registry, dir)
			a.enable(t, "alpha", 23)
			a.enable(t, "beta", 41)
			result := a.call(t, "bootstrap", "native-fixture:items info", "", 200)
			validateFunctionUI(t, result)
			data := functionJSON(t, result)
			assert.Contains(t, fmt.Sprint(data["required_params"]), "alpha")
			assert.Contains(t, fmt.Sprint(data["required_params"]), "beta")
			_, err := os.Stat(filepath.Join(dir, "requests"))
			require.ErrorIs(t, err, os.ErrNotExist, "bootstrap info must not execute the script")
			a.call(t, "unknown", "native-fixture:items __job:missing", "", 404)
			result = a.call(t, "info", "native-fixture:items info __job:alpha", "", 200)
			validateFunctionUI(t, result)
			assert.Contains(t, result, `"queue"`)
			result = a.call(
				t,
				"data",
				"native-fixture:items __job:beta queue:undeclared",
				`{"filter":"synthetic"}`,
				200,
			)
			validateFunctionUI(t, result)
			rows := functionJSON(t, result)["data"].([]any)
			assert.Equal(t, float64(41), rows[0].([]any)[0])
			raw := a.call(t, "raw", "native-fixture:items __job:alpha raw", "", 500)
			assert.Contains(t, raw, `9007199254740993`)
			result = a.call(t, "error", "native-fixture:items __job:alpha error", "", 503)
			validateFunctionUI(t, result)
			for _, line := range strings.Split(a.out.String(), "\n") {
				if strings.HasPrefix(line, "CHART ") {
					assert.NotContains(t, line, "native-fixture")
				}
			}
			requests, err := os.ReadFile(filepath.Join(dir, "requests"))
			require.NoError(t, err)
			assert.NotContains(t, string(requests), `"method": "collect"`)
			a.call(t, "disable-alpha", "config scripts.d:collector:native-fixture:alpha disable", "", 200)
			a.call(t, "detached-alpha", "native-fixture:items __job:alpha", "", 404)
			a.call(t, "beta-survives", "native-fixture:items __job:beta", "", 200)
			a.call(t, "disable-beta", "config scripts.d:collector:native-fixture:beta disable", "", 200)
			require.Eventually(
				t,
				func() bool { return strings.Contains(a.out.String(), `FUNCTION_DEL GLOBAL "native-fixture:items"`) },
				3*time.Second,
				10*time.Millisecond,
			)
		})
	}
}

func TestFunctionAgentReplacement(t *testing.T) {
	setupRunner(t)
	registry, dir := functionFixture(t, modePersistent, true)
	a := startNativeAgent(t, registry, dir)
	a.enable(t, "alpha", 23)
	a.send(t, "active", "native-fixture:items __job:alpha wait", "")
	waitFile(t, filepath.Join(dir, "23.active"))
	a.send(t, "queued", "native-fixture:items __job:alpha queued-predecessor", "")
	a.call(
		t,
		"replace",
		"config scripts.d:collector:native-fixture:alpha update",
		`{"update_every":1,"config":{"text":"replacement","count":41}}`,
		202,
	)
	old := a.result(t, "active", 0)
	assert.NotContains(t, old, "active 200 ")
	old = a.result(t, "queued", 0)
	assert.NotContains(t, old, "queued 200 ")
	// Readiness is observable through the current job's Function output.
	var current string
	require.Eventually(t, func() bool {
		id := fmt.Sprintf("current-%d", time.Now().UnixNano())
		current = a.call(t, id, "native-fixture:items __job:alpha", "", 0)
		return strings.Contains(current, `[[41,`)
	}, 5*time.Second, 20*time.Millisecond)
	validateFunctionUI(t, current)
	requests, err := os.ReadFile(filepath.Join(dir, "requests"))
	require.NoError(t, err)
	assert.NotContains(t, string(requests), "queued-predecessor")
	a.send(t, "disable-active", "native-fixture:items __job:alpha wait", "")
	waitFile(t, filepath.Join(dir, "41.active"))
	a.call(t, "disable", "config scripts.d:collector:native-fixture:alpha disable", "", 200)
	old = a.result(t, "disable-active", 0)
	assert.NotContains(t, old, "disable-active 200 ")
	assert.Contains(t, a.out.String(), `FUNCTION_DEL GLOBAL "native-fixture:items"`)
}

func TestFunctionAgentActiveCancellation(t *testing.T) {
	setupRunner(t)
	registry, dir := functionFixture(t, modePersistent, false)
	a := startNativeAgent(t, registry, dir)
	a.enable(t, "alpha", 23)
	a.send(t, "active", "native-fixture:items __job:alpha wait", "")
	waitFile(t, filepath.Join(dir, "23.active"))
	_, err := fmt.Fprintln(a.input, "FUNCTION_CANCEL active")
	require.NoError(t, err)
	a.result(t, "active", 499)
	before := strings.Count(a.out.String(), " = 23")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "release"), nil, 0644))
	a.call(t, "after", "native-fixture:items __job:alpha", "", 200)
	require.Eventually(t, func() bool {
		return strings.Count(a.out.String(), " = 23") > before
	}, 3*time.Second, 10*time.Millisecond, "collection must resume after caller cancellation")
	assert.NotContains(t, a.out.String(), "status failed")
	assert.NotContains(t, a.out.String(), `FUNCTION_DEL GLOBAL "native-fixture:items"`)
}

func TestFunctionAgentQueuedCancellation(t *testing.T) {
	setupRunner(t)
	registry, dir := functionFixture(t, modePersistent, true)
	a := startNativeAgent(t, registry, dir)
	a.enable(t, "alpha", 23)
	a.send(t, "active", "native-fixture:items __job:alpha wait", "")
	waitFile(t, filepath.Join(dir, "23.active"))
	a.send(t, "queued", "native-fixture:items __job:alpha canceled-queued", "")
	_, err := fmt.Fprintln(a.input, "FUNCTION_CANCEL queued")
	require.NoError(t, err)
	a.result(t, "queued", 499)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "release"), nil, 0644))
	a.result(t, "active", 200)
	a.call(t, "after", "native-fixture:items __job:alpha", "", 200)
	requests, err := os.ReadFile(filepath.Join(dir, "requests"))
	require.NoError(t, err)
	assert.NotContains(t, string(requests), "canceled-queued")
}

func TestFunctionDevelopmentExamples(t *testing.T) {
	setupRunner(t)
	for name, dependency := range map[string]string{"functions-bash": "jq", "functions-python": "python3"} {
		for _, mode := range []string{modeOneshot, modePersistent} {
			t.Run(name+"/"+mode, func(t *testing.T) {
				if _, err := exec.LookPath(dependency); err != nil {
					t.Skipf("example requires %s", dependency)
				}
				path, err := filepath.Abs(filepath.Join("../../development", name, "manifest.yaml"))
				require.NoError(t, err)
				data, err := os.ReadFile(path)
				require.NoError(t, err)
				var doc map[string]any
				require.NoError(t, yaml.Unmarshal(data, &doc))
				command := doc["command"].([]any)
				command[0] = filepath.Join(filepath.Dir(path), command[0].(string))
				doc["mode"] = mode
				data, err = yaml.Marshal(doc)
				require.NoError(t, err)
				dir := t.TempDir()
				path = filepath.Join(dir, "manifest.yaml")
				require.NoError(t, os.WriteFile(path, data, 0644))
				inventory := filepath.Join(dir, "packages.yaml")
				require.NoError(
					t,
					os.WriteFile(
						inventory,
						[]byte("version: v1\npackages:\n  - name: fixture\n    manifest: "+path+"\n"),
						0644,
					),
				)
				registry, err := loadPackages(
					context.Background(),
					inventory,
					collectorapi.Registry{},
					func(path string) (string, error) { _, err := os.Stat(path); return path, err },
				)
				require.NoError(t, err)
				a := startNativeAgent(t, registry, dir)
				a.call(t, "add", "config scripts.d:collector:native-fixture add example", `{"update_every":1}`, 202)
				a.call(t, "enable", "config scripts.d:collector:native-fixture:example enable", "", 202)
				require.Eventually(t, func() bool {
					return strings.Contains(
						a.out.String(),
						"CONFIG scripts.d:collector:native-fixture:example status running",
					)
				}, 5*time.Second, 10*time.Millisecond)
				for id, command := range map[string]string{"bootstrap": "info", "info": "info __job:example", "data": "__job:example"} {
					result := a.call(t, id, "native-fixture:items "+command, "", 200)
					validateFunctionUI(t, result)
				}
				if name == "functions-bash" {
					result := a.call(
						t,
						"selected",
						"native-fixture:items",
						`{"selections":{"__job":["example"],"queue":["batch"]}}`,
						200,
					)
					validateFunctionUI(t, result)
					assert.Contains(t, result, `["batch",17]`)
					result = a.call(
						t,
						"invalid-selection",
						"native-fixture:items __job:example",
						`{"selections":{"queue":["missing"]}}`,
						400,
					)
					validateFunctionUI(t, result)
					result = a.call(
						t,
						"duplicate-selection",
						"native-fixture:items __job:example",
						`{"selections":{"queue":["mail","batch"]}}`,
						400,
					)
					validateFunctionUI(t, result)
					result = a.call(t, "invalid", "native-fixture:items __job:example queue:missing", "", 400)
					validateFunctionUI(t, result)
					require.Eventually(
						t,
						func() bool { return strings.Contains(a.out.String(), " = 17") },
						3*time.Second,
						10*time.Millisecond,
					)
				}
			})
		}
	}
}
