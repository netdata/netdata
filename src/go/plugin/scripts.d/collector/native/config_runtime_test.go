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

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/agent"
	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery"
	discoveryfile "github.com/netdata/netdata/go/plugins/plugin/agent/discovery/file"
	"github.com/netdata/netdata/go/plugins/plugin/agent/policy"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func configPeer(t *testing.T) string {
	t.Helper()
	jq, err := exec.LookPath("jq")
	if err != nil {
		t.Skip("configured Bash peer requires jq")
	}
	return bashHelper(t) + fmt.Sprintf(`
nd_read_config
count=$(printf '%%s' "$ND_CONFIG" | %q -er '.config.count')
if [[ $1 == serve ]]; then nd_ready; fi
collect_snapshot() {
    nd_begin
    nd_metric depth "$count" queue mail
    nd_end
}
if [[ $1 == serve ]]; then while nd_next; do collect_snapshot; done
else collect_snapshot; fi
`, jq)
}

func TestConfiguredPeersBothModes(t *testing.T) {
	setupRunner(t)
	for _, mode := range []string{modeOneshot, modePersistent} {
		t.Run(mode, func(t *testing.T) {
			registry, _ := configuredFixture(t, configPeer(t), mode)
			c := registry["native-fixture"].CreateV2().(*Collector)
			// Preserve strings, nested values, explicit zero and false through YAML and stdin.
			require.NoError(
				t,
				yaml.Unmarshal(
					[]byte("config:\n  text: 'quotes \" and \\ unicode λ'\n  count: 23\n  enabled: false\n"),
					c,
				),
			)
			out := &jobOutput{}
			job := startTestJob(t, c, out)
			tickUntil(t, job, func() bool { return strings.Contains(out.String(), " = 23") })
			assert.NotContains(t, out.String(), "unicode")
		})
	}
}

func TestConfigEnvelopeRoundTrip(t *testing.T) {
	setupRunner(t)
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("Python peer requires python3")
	}
	for _, mode := range []string{modeOneshot, modePersistent} {
		t.Run(mode, func(t *testing.T) {
			registry, dir := configuredFixture(
				t,
				fmt.Sprintf("exec %q \"$(dirname \"$0\")/peer.py\" \"$@\"\n", python),
				mode,
			)
			c := registry["native-fixture"].CreateV2().(*Collector)
			script := filepath.Join(dir, "peer.py")
			body := `import json, pathlib, sys
config = json.loads(sys.stdin.readline())
pathlib.Path(__file__).with_suffix('.received').write_text(json.dumps(config))
result = {"version":"v1", "metrics":[{"name":"depth","value":config["config"]["count"],"labels":{"queue":"mail"}}]}
if sys.argv[-1] == 'serve':
    print(json.dumps({"version":"v1","ready":True}), flush=True)
    for line in sys.stdin:
        request = json.loads(line)
        print(json.dumps({"id":request["id"],"result":result}), flush=True)
else:
    assert sys.stdin.read() == ''
    print(json.dumps(result))
`
			require.NoError(t, os.WriteFile(script, []byte(body), 0644))
			c.ScriptConfig = Settings{
				"text":     "line one\nline two\r\tλ\\\"",
				"count":    float64(0),
				"enabled":  false,
				"optional": nil,
			}
			require.NoError(t, c.Init(context.Background()))
			var stop context.CancelFunc
			if mode == modePersistent {
				ctx, cancel := context.WithCancel(context.Background())
				stop = cancel
				ready := make(chan struct{})
				done := make(chan error, 1)
				go func() { done <- c.Run(ctx, func() { close(ready) }) }()
				t.Cleanup(func() {
					cancel()
					select {
					case <-done:
					case <-time.After(3 * time.Second):
						t.Error("peer did not stop")
					}
				})
				select {
				case <-ready:
				case <-time.After(3 * time.Second):
					t.Fatal("peer not ready")
				}
			}
			values, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
			require.NoError(t, err)
			assert.Equal(t, map[string]float64{`depth{queue="mail"}`: 0}, values)
			received, err := os.ReadFile(filepath.Join(dir, "peer.received"))
			require.NoError(t, err)
			var got, want any
			require.NoError(t, json.Unmarshal(received, &got))
			require.NoError(t, json.Unmarshal(c.configInput, &want))
			assert.Equal(t, want, got)
			if stop != nil {
				stop()
			}
		})
	}
}

func TestPersistentConfigurationWriteTimeout(t *testing.T) {
	setupRunner(t)
	registry, _ := configuredFixture(t, "sleep 30\n", modePersistent)
	c := registry["native-fixture"].CreateV2().(*Collector)
	c.ScriptConfig = Settings{
		"text": strings.Repeat("x", 512*1024),
	}
	c.Timeout = confopt.Duration(150 * time.Millisecond)
	require.NoError(t, c.Init(context.Background()))
	start := time.Now()
	err := c.Run(context.Background(), func() { t.Error("readiness before config consumed") })
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, time.Since(start), 3*time.Second)
}

func TestPackageDynCfgReplacement(t *testing.T) {
	setupRunner(t)
	registry, dir := configuredFixture(t, configPeer(t), modePersistent)
	// Agent owns real request parsing, preflight, activation, and replacement.
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
				require.True(t, agent.ContainsOnlyProcessControlErrors(err, context.Canceled), "%v", err)
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
	call := func(id, command, payload string, code int) string {
		t.Helper()
		if payload == "" {
			_, err := fmt.Fprintf(
				writer,
				"FUNCTION %s 5 \"config scripts.d:collector:native-fixture%s\" 0xFFFF \"user=test\"\n",
				id,
				command,
			)
			require.NoError(t, err)
		} else {
			_, err := fmt.Fprintf(writer, "FUNCTION_PAYLOAD %s 5 \"config scripts.d:collector:native-fixture%s\" 0xFFFF \"user=test\" application/json\n%s\nFUNCTION_PAYLOAD_END\n", id, command, payload)
			require.NoError(t, err)
		}
		var result string
		require.Eventually(t, func() bool {
			wire := out.String()
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
		}, 5*time.Second, 10*time.Millisecond)
		require.Contains(t, result, fmt.Sprintf("FUNCTION_RESULT_BEGIN %s %d ", id, code))
		return result
	}
	schema := call("schema", " schema", "", 200)
	assert.Contains(t, schema, `#/properties/config/definitions/text`)
	call("add", " add job", `{"update_every":1,"config":{"text":"synthetic","count":23}}`, 202)
	call("enable", ":job enable", "", 202)
	require.Eventually(
		t,
		func() bool { return strings.Contains(out.String(), " = 23") },
		5*time.Second,
		20*time.Millisecond,
	)
	call("invalid", ":job update", `{"update_every":1,"config":{"text":"synthetic","count":-5}}`, 422)
	original := call("get-original", ":job get", "", 200)
	assert.Contains(t, original, `"count":23`)
	call("update", ":job update", `{"update_every":1,"config":{"text":"replacement","count":41}}`, 202)
	require.Eventually(
		t,
		func() bool { return strings.Contains(out.String(), " = 41") },
		5*time.Second,
		20*time.Millisecond,
	)
	call("disable", ":job disable", "", 200)
}

func TestConfiguredDevelopmentExamples(t *testing.T) {
	setupRunner(t)
	for name, interpreter := range map[string]string{"configured-bash": "jq", "configured-python": "python3"} {
		t.Run(name, func(t *testing.T) {
			if _, err := exec.LookPath(interpreter); err != nil {
				t.Skipf("example requires %s", interpreter)
			}
			manifest, err := filepath.Abs(filepath.Join("../../development", name, "manifest.yaml"))
			require.NoError(t, err)
			c := New()
			c.Manifest = manifest
			c.validateExecutable = func(path string) (string, error) { _, err := os.Stat(path); return path, err }
			c.ScriptConfig = Settings{
				"queue": "batch",
				"depth": float64(13),
			}
			out := &jobOutput{}
			job := startTestJob(t, c, out)
			tickUntil(t, job, func() bool { return strings.Contains(out.String(), "SET 'warning' = 1") })
			assert.Contains(t, out.String(), "CLABEL 'queue' 'batch'")
		})
	}
}
