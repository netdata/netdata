// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

// Real peers expose file handshakes so cancellation tests do not guess when
// the host has written a request. Only path-policy and privilege dropping are replaced.
const functionPeer = `import json, os, pathlib, sys, time
root = pathlib.Path(__file__).parent
config = json.loads(sys.stdin.readline())["config"]
count = config["count"]
with (root / "starts").open("a") as f:
    f.write(str(os.getpid()) + "\n")
def reply(req):
    with (root / "requests").open("a") as f:
        f.write(json.dumps(req) + "\n")
    if req["method"] == "collect":
        return {"version":"v1", "metrics":[{"name":"depth","value":count,"labels":{"queue":"mail"}}]}
    args = req["args"]
    if "wait" in args:
        (root / (str(count) + ".active")).touch()
        while not (root / "release").exists(): time.sleep(0.01)
    if "error" in args: return {"version":"v1","status":503,"message":"temporarily unavailable"}
    if "raw" in args: return {"version":"v1","raw_response":{"status":500,"errorMessage":"synthetic","exact":9007199254740993}}
    if "malformed" in args: return {"version":"v2","status":200}
    if req["info"]: return {"version":"v1","status":200}
    return {"version":"v1","status":200,"columns":{"count":{"index":0,"name":"Count","type":"integer"},"request":{"index":1,"name":"Request","type":"string"}},"data":[[count,json.dumps(req)]]}
if sys.argv[-1] == 'serve':
    print(json.dumps({"version":"v1","ready":True}), flush=True)
    for line in sys.stdin:
        req = json.loads(line)
        print(json.dumps({"id":req["id"],"result":reply(req)}), flush=True)
elif sys.argv[-1] == 'function':
    req = json.loads(sys.stdin.readline())
    assert sys.stdin.read() == ''
    print(json.dumps({"id":req["id"],"result":reply(req)}))
else:
    assert sys.stdin.read() == ''
    print(json.dumps(reply({"method":"collect"})))
`

func functionFixture(t *testing.T, mode string, only bool) (collectorapi.Registry, string) {
	t.Helper()
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("Function peer requires python3")
	}
	_, dir := configuredFixture(t, fmt.Sprintf("exec %q \"$(dirname \"$0\")/peer.py\" \"$@\"\n", python), mode)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "peer.py"), []byte(functionPeer), 0644))
	path := filepath.Join(dir, "manifest.yaml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, yaml.Unmarshal(data, &doc))
	if only {
		delete(doc, "metrics")
		delete(doc, "checks")
		delete(doc, "charts")
	}
	data, err = yaml.Marshal(doc)
	require.NoError(t, err)
	data = append(data, []byte(`functions:
  - id: items
    name: Items
    help: List synthetic queue items.
    accepted_params: [filter]
    required_params:
      - id: queue
        name: Queue
        type: select
        options:
          - {id: mail, name: Mail, defaultSelected: true}
`)...)
	require.NoError(t, os.WriteFile(path, data, 0644))
	registry, err := loadPackages(
		context.Background(),
		filepath.Join(dir, "packages.yaml"),
		collectorapi.Registry{},
		func(path string) (string, error) { _, err := os.Stat(path); return path, err },
	)
	require.NoError(t, err)
	return registry, dir
}

func initFunctionCollector(t *testing.T, registry collectorapi.Registry) *Collector {
	t.Helper()
	c := registry["native-fixture"].CreateV2().(*Collector)
	c.ScriptConfig = Settings{
		"text":  "synthetic",
		"count": float64(23),
	}
	c.Timeout = confopt.Duration(time.Second)
	require.NoError(t, c.Init(context.Background()))
	require.NoError(t, c.Check(context.Background()))
	return c
}
func waitFile(t *testing.T, path string) {
	t.Helper()
	require.Eventually(
		t,
		func() bool { _, err := os.Stat(path); return err == nil },
		3*time.Second,
		10*time.Millisecond,
	)
}

func TestFunctionPeersBothModes(t *testing.T) {
	setupRunner(t)
	for _, mode := range []string{modeOneshot, modePersistent} {
		t.Run(mode, func(t *testing.T) {
			registry, dir := functionFixture(t, mode, true)
			assert.True(t, registry["native-fixture"].FunctionOnly)
			var form struct {
				JSONSchema struct{ Properties map[string]any } `json:"jsonSchema"`
			}
			require.NoError(t, json.Unmarshal([]byte(registry["native-fixture"].JobConfigSchema), &form))
			assert.NotContains(t, form.JSONSchema.Properties, "update_every")
			if mode == modeOneshot {
				assert.NotContains(t, form.JSONSchema.Properties, "timeout")
			} else {
				assert.Contains(t, form.JSONSchema.Properties, "timeout")
			}
			c := initFunctionCollector(t, registry)
			_, err := os.Stat(filepath.Join(dir, "starts"))
			require.ErrorIs(t, err, os.ErrNotExist)
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
			assert.Equal(t, input.Args, echoed.Args)
			assert.Equal(t, input.Payload, echoed.Payload)
			assert.Equal(t, input.ContentType, echoed.ContentType)
			// Raw handlers deliberately receive selectors untouched, including undeclared values.
			result, err = c.ExecuteFunction(ctx, funcapi.RawMethodRequest{
				Method: "items",
				Args:   []string{"error"},
			})
			require.NoError(t, err)
			assert.Equal(t, 503, result.Status)
			_, err = c.ExecuteFunction(
				ctx,
				funcapi.RawMethodRequest{
					Method:  "items",
					Payload: make([]byte, maxMessageBytes*3/4),
				},
			)
			require.ErrorContains(t, err, "exceeds 64 MiB")
			_, err = c.ExecuteFunction(ctx, funcapi.RawMethodRequest{
				Method: "items",
				Info:   true,
			})
			require.NoError(t, err)
			starts, err := os.ReadFile(filepath.Join(dir, "starts"))
			require.NoError(t, err)
			want := 3
			if mode == modePersistent {
				want = 1
			}
			assert.Len(t, strings.Fields(string(starts)), want)
			requests, err := os.ReadFile(filepath.Join(dir, "requests"))
			require.NoError(t, err)
			assert.Len(t, strings.Split(strings.TrimSpace(string(requests)), "\n"), 3)
		})
	}
}

func TestPersistentFunctionQueue(t *testing.T) {
	setupRunner(t)
	registry, dir := functionFixture(t, modePersistent, false)
	c := initFunctionCollector(t, registry)
	runtime := startRuntime(t, c)
	runtime.waitReady(t)
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
	queued, qcancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer qcancel()
	_, err := c.ExecuteFunction(
		queued,
		funcapi.RawMethodRequest{
			Method: "items",
			Args:   []string{"must-not-reach-peer"},
		},
	)
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
	requests, err := os.ReadFile(filepath.Join(dir, "requests"))
	require.NoError(t, err)
	assert.NotContains(t, string(requests), "must-not-reach-peer")
	assert.Len(t, strings.Split(strings.TrimSpace(string(requests)), "\n"), 2)
}

func TestCanceledRequestAtDequeue(t *testing.T) {
	setupRunner(t)
	registry, dir := functionFixture(t, modePersistent, true)
	c := initFunctionCollector(t, registry)
	startRuntime(t, c).waitReady(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// Exercise the actual receive boundary deterministically: a select may deliver
	// a sender whose context is already canceled. No synthetic runtime is installed.
	c.runtimeMu.Lock()
	runtime := c.runtime
	c.runtimeMu.Unlock()
	request := scriptRequest{
		ctx: ctx,
		function: &funcapi.RawMethodRequest{
			Method: "items",
		},
		reply: make(chan scriptResult, 1),
	}
	select {
	case runtime.requests <- request:
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
	requests, err := os.ReadFile(filepath.Join(dir, "requests"))
	require.NoError(t, err)
	assert.Len(t, strings.Split(strings.TrimSpace(string(requests)), "\n"), 1)
}

func TestActiveFunctionCancellation(t *testing.T) {
	setupRunner(t)
	for _, mode := range []string{modeOneshot, modePersistent} {
		t.Run(mode, func(t *testing.T) {
			registry, dir := functionFixture(t, mode, true)
			c := initFunctionCollector(t, registry)
			var runtime *testRuntime
			if mode == modePersistent {
				runtime = startRuntime(t, c)
				runtime.waitReady(t)
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
			if runtime != nil {
				runtime.wait(t)
				require.ErrorIs(t, runtime.err, context.DeadlineExceeded)
				_, err := c.ExecuteFunction(context.Background(), funcapi.RawMethodRequest{
					Method: "items",
				})
				require.Error(t, err)
			}
		})
	}
}

func TestMalformedFunctionReplyStopsSession(t *testing.T) {
	setupRunner(t)
	registry, _ := functionFixture(t, modePersistent, true)
	c := initFunctionCollector(t, registry)
	r := startRuntime(t, c)
	r.waitReady(t)
	_, err := c.ExecuteFunction(
		context.Background(),
		funcapi.RawMethodRequest{
			Method: "items",
			Args:   []string{"malformed"},
		},
	)
	require.Error(t, err)
	r.wait(t)
	require.Error(t, r.err)
}

func TestGenericManifestRejectsFunctions(t *testing.T) {
	registry, dir := functionFixture(t, modeOneshot, true)
	require.NotNil(t, registry)
	c := New()
	c.Manifest = filepath.Join(dir, "manifest.yaml")
	c.validateExecutable = func(path string) (string, error) { return path, nil }
	require.ErrorContains(t, c.Init(context.Background()), "must be registered")
}
