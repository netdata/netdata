// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

// The development examples under scripts.d/development are documented starting
// points; each test runs one family through its real harness.

func exampleManifest(t *testing.T, name string) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("../../development", name, "manifest.yaml"))
	require.NoError(t, err)
	return path
}

func TestExamples_Direct(t *testing.T) {
	setupRunner(t)
	for name, tc := range map[string]struct {
		tool string
		file string
	}{
		"bash":   {tool: "bash", file: "collect.sh"},
		"python": {tool: "python3", file: "collect.py"},
	} {
		for _, mode := range []string{modeOneshot, modePersistent} {
			t.Run(name+"/"+mode, func(t *testing.T) {
				executable := requireTool(t, tc.tool)
				path, err := filepath.Abs(filepath.Join("../../development", "direct-"+name, tc.file))
				require.NoError(t, err)
				c := New()
				c.validateExecutable = statExecutable
				c.Command = []string{executable, path}
				c.Mode = confopt.Enum[jobModeSpec](mode)
				require.NoError(t, c.Init(context.Background()))
				t.Cleanup(func() { c.Cleanup(context.Background()) })
				require.NoError(t, c.Check(context.Background()))
				if mode == modePersistent {
					startRuntime(t, c).waitReady(t)
				}
				values, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
				require.NoError(t, err)
				assert.Equal(t, float64(21.5), values["temperature"])
				if name == "python" {
					assert.Equal(t, float64(1), values[`worker{worker="in progress"}`])
					assert.Equal(t, float64(0), values[`worker{worker="stopped"}`])
					assert.Equal(t, float64(1), values[`features{features="read"}`])
					assert.Equal(t, float64(1), values[`features{features="write"}`])
				}
			})
		}
	}
}

func TestExamples_Persistent(t *testing.T) {
	setupRunner(t)
	for name, tool := range map[string]string{"persistent-bash": "bash", "persistent-python": "python3"} {
		t.Run(name, func(t *testing.T) {
			requireTool(t, tool)
			c := newManifestCollector(exampleManifest(t, name))
			require.NoError(t, c.Init(context.Background()))
			require.NoError(t, c.Check(context.Background()))
			startRuntime(t, c).waitReady(t)
			first, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
			require.NoError(t, err)
			assert.Equal(t, float64(1), first[`processed_total{queue="mail"}`])
			assert.Equal(t, float64(1), first[`native.check.backlog{native.check.backlog="critical",queue="mail"}`])
			_, err = collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
			require.ErrorIs(t, err, errCollectionFailed)
			third, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
			require.NoError(t, err)
			assert.Equal(t, float64(3), third[`processed_total{queue="mail"}`])
			assert.Equal(t, float64(1), third[`native.check.backlog{native.check.backlog="ok",queue="mail"}`])
		})
	}
}

func TestExamples_Configured(t *testing.T) {
	setupRunner(t)
	for name, tool := range map[string]string{"configured-bash": "jq", "configured-python": "python3"} {
		t.Run(name, func(t *testing.T) {
			requireTool(t, tool)
			c := newManifestCollector(exampleManifest(t, name))
			c.ScriptConfig = Settings{
				"queue": "batch",
				"depth": float64(13),
			}
			out := &wireOutput{}
			job, _ := startTestJob(t, c, out)
			tickUntil(t, job, func() bool { return strings.Contains(out.String(), "SET 'warning' = 1") })
			assert.Contains(t, out.String(), "CLABEL 'queue' 'batch'")
		})
	}
}

func TestExamples_Functions(t *testing.T) {
	setupRunner(t)
	for name, tool := range map[string]string{"functions-bash": "jq", "functions-python": "python3"} {
		for _, mode := range []string{modeOneshot, modePersistent} {
			t.Run(name+"/"+mode, func(t *testing.T) {
				requireTool(t, tool)
				dir := t.TempDir()
				manifest := filepath.Join(dir, "manifest.yaml")
				writeExampleInMode(t, exampleManifest(t, name), manifest, mode)
				a := startTestAgent(t, loadTestPackages(t, writeManifestInventory(t, dir, manifest)), dir)
				a.startJob(t, "example", `{"update_every":1}`)
				for id, command := range map[string]string{"bootstrap": "info", "info": "info __job:example", "data": "__job:example"} {
					validateFunctionUI(t, a.call(t, id, "native-fixture:items "+command, "", 200))
				}
				if name != "functions-bash" {
					return
				}
				result := a.call(
					t,
					"selected",
					"native-fixture:items",
					`{"selections":{"__job":["example"],"queue":["batch"]}}`,
					200,
				)
				validateFunctionUI(t, result)
				assert.Contains(t, result, `["batch",17]`)
				for id, request := range map[string]struct{ command, payload string }{
					"invalid-selection":   {command: "__job:example", payload: `{"selections":{"queue":["missing"]}}`},
					"duplicate-selection": {command: "__job:example", payload: `{"selections":{"queue":["mail","batch"]}}`},
					"invalid":             {command: "__job:example queue:missing"},
				} {
					validateFunctionUI(t, a.call(t, id, "native-fixture:items "+request.command, request.payload, 400))
				}
				a.waitOutput(t, " = 17")
			})
		}
	}
}

// writeExampleInMode copies an example manifest with an absolute command and the given mode.
func writeExampleInMode(t *testing.T, source, target, mode string) {
	t.Helper()
	data, err := os.ReadFile(source)
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, yaml.Unmarshal(data, &doc))
	command := doc["command"].([]any)
	command[0] = filepath.Join(filepath.Dir(source), command[0].(string))
	doc["mode"] = mode
	data, err = yaml.Marshal(doc)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(target, data, 0644))
}

func TestExamples_SelfContained(t *testing.T) {
	setupRunner(t)
	examples, err := filepath.Abs("../../development")
	require.NoError(t, err)
	for _, language := range []string{"bash", "python", "go"} {
		t.Run(language, func(t *testing.T) {
			var program []byte
			switch language {
			case "bash":
				program = readFile(t, filepath.Join(examples, "self-contained-bash", "collect.sh"))
			case "python":
				requireTool(t, "python3")
				program = readFile(t, filepath.Join(examples, "self-contained-python", "collect.py"))
			case "go":
				program = buildGoExample(t, filepath.Join(examples, "self-contained-go"))
			}
			for _, mode := range []string{modeOneshot, modePersistent} {
				t.Run(mode, func(t *testing.T) {
					// Deploy only one file; the Go source's embedded YAML is not present.
					dir := t.TempDir()
					path := filepath.Join(dir, "package")
					require.NoError(t, os.WriteFile(path, program, 0755))
					command := []string{path}
					if mode == modePersistent {
						command = append(command, "--persistent")
					}
					registry := loadTestPackages(t, writeCommandInventory(t, command))
					entries, err := os.ReadDir(dir)
					require.NoError(t, err)
					assert.Len(t, entries, 1, "description must not extract sidecars")
					a := startTestAgent(t, registry, dir)
					a.startJob(t, "example", `{"update_every":1}`)
					if language != "bash" {
						validateFunctionUI(t, a.call(t, "info", "native-fixture:items info __job:example", "", 200))
						result := a.call(t, "data", "native-fixture:items __job:example", "", 200)
						validateFunctionUI(t, result)
						assert.Contains(t, result, "[[17]]")
					}
					a.waitOutput(t, " = 17")
					assert.Contains(t, a.out.String(), "selfcontained_"+language+".depth")
				})
			}
		})
	}
}

func buildGoExample(t *testing.T, source string) []byte {
	t.Helper()
	goBin := requireTool(t, "go")
	binary := filepath.Join(t.TempDir(), "example")
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, goBin, "build", "-o", binary, source).CombinedOutput()
	require.NoError(t, err, "%s", out)
	return readFile(t, binary)
}

func TestExamples_Lines(t *testing.T) {
	setupRunner(t)
	for _, language := range []string{"bash", "python"} {
		tool, file := "bash", "collect.sh"
		if language == "python" {
			tool, file = "python3", "collect.py"
		}
		executable := requireTool(t, tool)
		path, err := filepath.Abs(filepath.Join("../../development", "lines-"+language, file))
		require.NoError(t, err)
		for _, mode := range []string{modeOneshot, modePersistent} {
			t.Run(language+"/"+mode, func(t *testing.T) {
				c := New()
				c.validateExecutable = statExecutable
				c.Command = []string{executable, path}
				c.Mode = confopt.Enum[jobModeSpec](mode)
				c.SnapshotFormat = formatLines
				c.Timeout = confopt.Duration(5 * time.Second)
				require.NoError(t, c.Init(context.Background()))
				if mode == modePersistent {
					startRuntime(t, c).waitReady(t)
				}
				values, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
				require.NoError(t, err)
				assert.Equal(t, float64(21.5), values["temperature"])
				queue := `queue.depth{queue="mail"}`
				if language == "python" {
					queue = `queue.depth{queue="batch, \"night\" | café 😀"}`
				}
				assert.Equal(t, float64(17), values[queue])
				assert.Equal(t, float64(12345), values[`requests_total{service="api"}`])
			})
		}
	}
}

func TestJob_LinesAutogen(t *testing.T) {
	setupRunner(t)
	executable := requireTool(t, "bash")
	path, err := filepath.Abs("../../development/lines-bash/collect.sh")
	require.NoError(t, err)
	c := New()
	c.validateExecutable = statExecutable
	c.Command = []string{executable, path}
	c.SnapshotFormat = formatLines
	out := &wireOutput{}
	job, _ := startTestJob(t, c, out)
	tickUntil(t, job, func() bool { return strings.Contains(out.String(), "SET 'requests_total' = 12345") })
	wire := out.String()
	assert.Contains(t, wire, "'Celsius'")
	assert.Contains(t, wire, "'API requests' 'requests/s'")
	assert.Contains(t, wire, "DIMENSION 'requests_total' 'requests_total' 'incremental'")
	assert.Contains(t, wire, "CLABEL 'queue' 'mail'")
}
