// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery"
	discoveryfile "github.com/netdata/netdata/go/plugins/plugin/agent/discovery/file"
	"github.com/netdata/netdata/go/plugins/plugin/agent/policy"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/jobruntime"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Runtime drivers shared by tests: the job runtime, Collector.Run and a real
// Agent. Package fixtures and peers live in test_helpers_test.go.

// wireOutput is a concurrency-safe plugin output buffer.
type wireOutput struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *wireOutput) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(p)
}

func (b *wireOutput) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}

func (b *wireOutput) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buffer.Reset()
}

// chartID returns the ID of the first chart with contextName in the wire output.
func chartID(wire, contextName string) string {
	for line := range strings.SplitSeq(wire, "\n") {
		if strings.HasPrefix(line, "CHART '") && strings.Contains(line, "'"+contextName+"'") {
			return strings.SplitN(line, "'", 3)[1]
		}
	}
	return ""
}

// startTestJob drives the real job runtime: autodetection, Run readiness, ticks and emission.
func startTestJob(t *testing.T, c *Collector, out *wireOutput) (*jobruntime.JobV2, *jobruntime.ManagedRun) {
	t.Helper()
	job := jobruntime.NewJobV2(jobruntime.JobV2Config{
		PluginName:  "scripts.d",
		Name:        "fixture",
		ModuleName:  "native",
		FullName:    "scripts_d_fixture",
		Module:      c,
		Out:         out,
		UpdateEvery: 1,
	})
	require.NoError(t, job.AutoDetectionManaged(context.Background()))
	done := make(chan struct{})
	run := jobruntime.NewManagedRun(context.Background(), nil)
	go func() { defer close(done); job.StartManaged(run) }()
	t.Cleanup(func() {
		job.Stop()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("job did not stop")
		}
		job.Cleanup()
	})
	require.Eventually(t, run.Running, 3*time.Second, 10*time.Millisecond)
	return job, run
}

func tickUntil(t *testing.T, job *jobruntime.JobV2, condition func() bool) {
	t.Helper()
	require.Eventually(t, func() bool { job.Tick(1); return condition() }, 3*time.Second, 100*time.Millisecond)
}

// testRuntime runs Collector.Run directly, without the job runtime.
type testRuntime struct {
	cancel context.CancelFunc
	ready  chan struct{}
	done   chan struct{}
	err    error // Read after done closes.
}

func startRuntime(t testing.TB, c *Collector) *testRuntime {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	r := &testRuntime{
		cancel: cancel,
		ready:  make(chan struct{}),
		done:   make(chan struct{}),
	}
	go func() {
		defer close(r.done)
		r.err = c.Run(ctx, func() { close(r.ready) })
	}()
	t.Cleanup(func() { cancel(); r.wait(t) })
	return r
}

func (r *testRuntime) wait(t testing.TB) {
	t.Helper()
	select {
	case <-r.done:
	case <-time.After(3 * time.Second):
		t.Fatal("persistent runtime did not stop")
	}
}

func (r *testRuntime) waitReady(t testing.TB) {
	t.Helper()
	select {
	case <-r.ready:
	case <-r.done:
		t.Fatalf("runtime failed before ready: %v", r.err)
	case <-time.After(3 * time.Second):
		t.Fatal("persistent runtime did not become ready")
	}
}

// testAgent is a real scripts.d Agent over an in-memory plugins.d stream.
type testAgent struct {
	input *io.PipeWriter
	out   *wireOutput
}

func startTestAgent(t *testing.T, registry collectorapi.Registry, dir string) *testAgent {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	reader, writer := io.Pipe()
	out := &wireOutput{}
	fileDiscovery := discovery.NewProviderFactory(
		"file",
		func(build discovery.BuildContext) (discovery.Discoverer, bool, error) {
			d, err := discoveryfile.NewDiscovery(discoveryfile.Config{
				Registry: build.Registry,
				Watch:    []string{filepath.Join(dir, "*.conf")},
			})
			return d, true, err
		},
	)
	plugin := agent.New(agent.Config{
		Name:                    "scripts.d",
		ModuleRegistry:          registry,
		PluginConfigDir:         []string{dir},
		VarLibDir:               dir,
		RunModePolicy:           policy.Agent(false),
		DisableServiceDiscovery: true,
		DiscoveryProviders:      []discovery.ProviderFactory{fileDiscovery},
	})
	plugin.In = reader
	plugin.Out = out
	done := make(chan error, 1)
	go func() { defer reader.Close(); done <- plugin.RunContext(ctx) }()
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
	a := &testAgent{
		input: writer,
		out:   out,
	}
	a.waitOutput(t, "scripts.d:collector:native-fixture")
	return a
}

func (a *testAgent) send(t *testing.T, id, command, payload string) {
	t.Helper()
	var err error
	if payload == "" {
		_, err = fmt.Fprintf(a.input, "FUNCTION %s 5 %q 0xFFFF \"user=test\"\n", id, command)
	} else {
		_, err = fmt.Fprintf(a.input, "FUNCTION_PAYLOAD %s 5 %q 0xFFFF \"user=test\" application/json\n%s\nFUNCTION_PAYLOAD_END\n", id, command, payload)
	}
	require.NoError(t, err)
}

func (a *testAgent) cancel(t *testing.T, id string) {
	t.Helper()
	_, err := fmt.Fprintln(a.input, "FUNCTION_CANCEL "+id)
	require.NoError(t, err)
}

// result waits for a Function result. A nonzero code must match.
func (a *testAgent) result(t *testing.T, id string, code int) string {
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

func (a *testAgent) call(t *testing.T, id, command, payload string, code int) string {
	t.Helper()
	a.send(t, id, command, payload)
	return a.result(t, id, code)
}

func (a *testAgent) waitOutput(t *testing.T, text string) {
	t.Helper()
	require.Eventually(
		t,
		func() bool { return strings.Contains(a.out.String(), text) },
		5*time.Second,
		10*time.Millisecond,
	)
}

// startJob adds and enables a native-fixture DynCfg job.
func (a *testAgent) startJob(t *testing.T, name, payload string) {
	t.Helper()
	a.call(t, "add-"+name, "config scripts.d:collector:native-fixture add "+name, payload, 202)
	a.call(t, "enable-"+name, "config scripts.d:collector:native-fixture:"+name+" enable", "", 202)
	a.waitOutput(t, "CONFIG scripts.d:collector:native-fixture:"+name+" status running")
}

// enable starts a configured job reporting count.
func (a *testAgent) enable(t *testing.T, name string, count int) {
	t.Helper()
	a.startJob(t, name, fmt.Sprintf(`{"update_every":1,"config":{"text":"synthetic","count":%d}}`, count))
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
	schema, err := jsonschema.NewCompiler().Compile(path)
	require.NoError(t, err)
	require.NoError(t, schema.Validate(functionJSON(t, result)))
}
