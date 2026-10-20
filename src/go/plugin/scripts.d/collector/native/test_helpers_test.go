// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

// Package fixtures, script peers and small helpers shared by tests. Runtime
// drivers (job, Run, Agent) live in test_harness_test.go.

// setupRunner exercises the production command and cancellation path. The shim
// substitutes only privilege dropping, which an unprivileged unit test cannot exercise.
func setupRunner(t testing.TB) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Bash fixture requires Unix; protocol tests are portable")
	}
	path := filepath.Join(t.TempDir(), "nd-run")
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\nexec \"$@\"\n"), 0755))
	t.Cleanup(ndexec.SetRunnerPathsForTests(path, ""))
}

// statExecutable replaces the production executable path policy, which has its
// own tests in pathvalidate. Everything after it runs unchanged.
func statExecutable(path string) (string, error) {
	_, err := os.Stat(path)
	return path, err
}

// newManifestCollector returns an uninitialized generic job for a manifest.
func newManifestCollector(manifest string) *Collector {
	c := New()
	c.Manifest = manifest
	c.validateExecutable = statExecutable
	return c
}

const fixtureCharts = `version: v1
context_namespace: fixture
engine:
  autogen:
    enabled: false
groups:
  - family: Queue
    metrics: [depth, processed_total]
    charts:
      - id: depth
        title: Queue Depth
        context: depth
        units: jobs
        instances:
          by_labels: [queue]
        label_promotion: [region]
        dimensions:
          - selector: depth
            name: depth
      - id: processed
        title: Jobs Processed
        context: processed
        units: jobs/s
        instances:
          by_labels: [queue]
        dimensions:
          - selector: processed_total
            name: processed
`

const fixtureManifest = `version: v1
command: [./collect.sh]
charts: charts.yaml
`

// fixtureCollector writes a one-shot package whose Bash command runs body, and
// returns an initialized generic job plus the package directory.
func fixtureCollector(t *testing.T, body string) (*Collector, string) {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "collect.sh"), []byte("#!/bin/bash\nset -eu\n"+body), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "charts.yaml"), []byte(fixtureCharts), 0644))
	path := filepath.Join(dir, "manifest.yaml")
	require.NoError(t, os.WriteFile(path, []byte(fixtureManifest), 0644))
	c := newManifestCollector(path)
	require.NoError(t, c.Init(context.Background()))
	require.NoError(t, c.Check(context.Background()))
	return c, dir
}

// persistentCollector is fixtureCollector in persistent mode; body runs for serve.
func persistentCollector(t *testing.T, body string) (*Collector, string) {
	t.Helper()
	c, dir := fixtureCollector(t, "[[ $1 == serve ]]\n"+body)
	appendFile(t, c.Manifest, "mode: persistent\n")
	require.NoError(t, c.Init(context.Background()))
	require.NoError(t, c.Check(context.Background()))
	return c, dir
}

func appendFile(t *testing.T, path, data string) {
	t.Helper()
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, append(content, data...), 0644))
}

// snapshotJSON matches the fixture package, with the backlog check in state.
func snapshotJSON(state string) string {
	return fmt.Sprintf(
		`{"version":"v1","metrics":[{"name":"depth","unit":"jobs","samples":[{"value":17,"labels":{"queue":"mail","region":"east"}}]},{"name":"processed_total","type":"counter","unit":"jobs","samples":[{"value":100,"labels":{"queue":"mail"}}]}],"checks":[{"id":"backlog","title":"Queue Backlog","by_labels":["queue"],"samples":[{"state":%q,"labels":{"queue":"mail"}}]}]}`,
		state,
	)
}

func bashHelperPath(t *testing.T) string {
	t.Helper()
	path, err := filepath.Abs("../../lib/native.sh")
	require.NoError(t, err)
	return path
}

// bashHelper sources the shipped Bash protocol helper.
func bashHelper(t *testing.T) string {
	t.Helper()
	return "source '" + bashHelperPath(t) + "'\n"
}

// requireTool skips the test when an external peer dependency is missing.
func requireTool(t *testing.T, name string) string {
	t.Helper()
	path, err := exec.LookPath(name)
	if err != nil {
		t.Skipf("test requires %s", name)
	}
	return path
}

// jsonEscape spells s with JSON unicode escapes (UTF-16 surrogate pairs outside the
// BMP, as Python json.dumps emits). Escapes are built at run time so the test
// source cannot lose them to tools that decode escape sequences.
func jsonEscape(s string) string {
	var b strings.Builder
	for _, unit := range utf16.Encode([]rune(s)) {
		b.WriteString("\\" + fmt.Sprintf("u%04x", unit))
	}
	return b.String()
}

const fixtureSchema = `{
 "jsonSchema": {
  "$schema": "http://json-schema.org/draft-07/schema#", "type": "object", "title": "Fixture", "description": "Fixture settings.",
  "additionalProperties": false,
  "properties": {
   "text": {"$ref": "#/definitions/text"},
   "count": {"type": "integer", "minimum": 0, "default": 17},
   "enabled": {"type": "boolean", "default": true},
   "optional": {"type": ["string", "null"], "default": "fallback"},
   "nested": {"type": "object", "default": {}, "properties": {"value": {"type": "integer", "default": 3}}}
  },
  "required": ["text"], "definitions": {"text": {"type": "string", "minLength": 1}}
 },
 "uiSchema": {"text": {"ui:widget": "password"}}
}`

// configuredFixture registers the fixture package with fixtureSchema as
// native-fixture and returns the registry plus the package directory.
func configuredFixture(t *testing.T, body, mode string) (collectorapi.Registry, string) {
	t.Helper()
	c, dir := fixtureCollector(t, body)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config_schema.json"), []byte(fixtureSchema), 0644))
	appendFile(t, c.Manifest, "config_schema: config_schema.json\nmode: "+mode+"\n")
	return loadTestPackages(t, writeManifestInventory(t, dir, c.Manifest)), dir
}

// writeManifestInventory registers a manifest as native-fixture in dir/packages.yaml.
func writeManifestInventory(t *testing.T, dir, manifest string) string {
	t.Helper()
	path := filepath.Join(dir, "packages.yaml")
	inventory := fmt.Sprintf("version: v1\npackages:\n  - name: fixture\n    manifest: %s\n", manifest)
	require.NoError(t, os.WriteFile(path, []byte(inventory), 0644))
	return path
}

// writeCommandInventory registers a self-contained command as native-fixture.
func writeCommandInventory(t *testing.T, command []string) string {
	t.Helper()
	data, err := yaml.Marshal(map[string]any{
		"version":  "v1",
		"packages": []any{map[string]any{"name": "fixture", "command": command}},
	})
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "packages.yaml")
	require.NoError(t, os.WriteFile(path, data, 0644))
	return path
}

func loadTestPackages(t *testing.T, inventory string) collectorapi.Registry {
	t.Helper()
	registry, err := loadPackages(context.Background(), inventory, collectorapi.Registry{}, statExecutable)
	require.NoError(t, err)
	return registry
}

// configPeer is a configured Bash peer reporting config.count as depth in both modes.
func configPeer(t *testing.T) string {
	t.Helper()
	jq := requireTool(t, "jq")
	return bashHelper(t) + fmt.Sprintf(`
nd_read_config
count=$(printf '%%s' "$ND_CONFIG" | %q -er '.config.count')
if [[ $1 == serve ]]; then nd_ready; fi
collect_snapshot() {
    nd_begin
    nd_metric depth gauge jobs
    nd_sample "$ND_FAMILY" "$count" queue mail
    nd_end
}
if [[ $1 == serve ]]; then while nd_next; do collect_snapshot; done
else collect_snapshot; fi
`, jq)
}

// functionPeer is a real Python peer for all operations. It exposes file
// handshakes so cancellation tests do not guess when the host has written a
// request: <count>.active while a "wait" request blocks, and release to finish it.
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
        return {"version":"v1", "metrics":[{"name":"depth","unit":"jobs","samples":[{"value":count,"labels":{"queue":"mail"}}]}]}
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

const fixtureFunctions = `functions:
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
`

// functionFixture registers the configured fixture with an items Function
// served by functionPeer. functionOnly disables collection and drops charts.
func functionFixture(t *testing.T, mode string, functionOnly bool) (collectorapi.Registry, string) {
	t.Helper()
	python := requireTool(t, "python3")
	_, dir := configuredFixture(t, fmt.Sprintf("exec %q \"$(dirname \"$0\")/peer.py\" \"$@\"\n", python), mode)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "peer.py"), []byte(functionPeer), 0644))
	path := filepath.Join(dir, "manifest.yaml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, yaml.Unmarshal(data, &doc))
	if functionOnly {
		doc["collect"] = false
		delete(doc, "charts")
	}
	data, err = yaml.Marshal(doc)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, append(data, fixtureFunctions...), 0644))
	return loadTestPackages(t, filepath.Join(dir, "packages.yaml")), dir
}

// initFunctionCollector creates and initializes a native-fixture job with count 23.
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
	require.Eventually(t, func() bool { return fileExists(path) }, 3*time.Second, 10*time.Millisecond)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func requireNoFile(t *testing.T, path string) {
	t.Helper()
	_, err := os.Stat(path)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return data
}

func readLines(t *testing.T, path string) []string {
	t.Helper()
	return strings.Split(strings.TrimSpace(string(readFile(t, path))), "\n")
}

// readyLine is a Bash persistent peer's ready handshake.
const readyLine = "printf '%s\\n' '{\"version\":\"v1\",\"ready\":true}'\n"

// collectAndCommit runs one cycle and commits it even on failure, so tests can
// prove a failed collection staged no samples.
func collectAndCommit(t *testing.T, c *Collector) error {
	t.Helper()
	managed, ok := metrix.AsCycleManagedStore(c.store)
	require.True(t, ok)
	managed.CycleController().BeginCycle()
	err := c.Collect(context.Background())
	managed.CycleController().CommitCycleSuccess()
	return err
}

func rawSeries(c *Collector) map[string]float64 {
	values := map[string]float64{}
	c.store.Read(metrix.ReadRaw()).ForEachSeries(func(name string, _ metrix.LabelView, value float64) {
		values[name] = value
	})
	return values
}
