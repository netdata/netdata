// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollector_ConfigurationSerialize(t *testing.T) {
	collecttest.TestConfigurationSerialize(
		t,
		New(),
		[]byte(
			`{"manifest":"/opt/custom/manifest.yaml","mode":"auto","snapshot_format":"auto","update_every":15,"timeout":3.5,"autodetection_retry":60,"config":{"enabled":false,"count":0,"nested":{"optional":null}}}`,
		),
		[]byte(
			"manifest: /opt/custom/manifest.yaml\nmode: auto\nsnapshot_format: auto\nupdate_every: 15\ntimeout: 3.5\nautodetection_retry: 60\nconfig:\n  enabled: false\n  count: 0\n  nested:\n    optional: null\n",
		),
	)
}

func TestCollector_ConfigSchemaMatchesMetadata(t *testing.T) {
	collecttest.AssertConfigSchemaMatchesMetadataWith(
		t,
		"config_schema.json",
		"metadata.dev.yaml",
		collecttest.ConfigSchemaCheck{
			Defaults: true,
		},
	)
}

func TestCollector_Init(t *testing.T) {
	tests := map[string]struct {
		prepare     func(c *Collector)
		wantErrText string
	}{
		"valid manifest": {
			prepare: func(*Collector) {},
		},
		"relative manifest": {
			prepare:     func(c *Collector) { c.Manifest = "relative.yaml" },
			wantErrText: "absolute",
		},
		"nonpositive timeout": {
			prepare:     func(c *Collector) { c.Timeout = 0 },
			wantErrText: "positive",
		},
		"nonpositive update_every": {
			prepare:     func(c *Collector) { c.UpdateEvery = 0 },
			wantErrText: "positive",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			c, _ := fixtureCollector(t, "exit 0\n")
			tc.prepare(c)
			err := c.Init(context.Background())
			if tc.wantErrText == "" {
				require.NoError(t, err)
				require.NoError(t, c.Check(context.Background()))
				return
			}
			require.ErrorContains(t, err, tc.wantErrText)
			require.Error(t, c.Check(context.Background()), "a failed Init must not pass Check")
		})
	}
}

func TestCollector_InitUsesProductionPathPolicy(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission validation is not implemented on Windows")
	}
	c, dir := fixtureCollector(t, "exit 0\n")
	for name, configure := range map[string]func(*Collector){
		"manifest":       func(untrusted *Collector) { untrusted.Manifest = c.Manifest },
		"direct command": func(untrusted *Collector) { untrusted.Command = []string{filepath.Join(dir, "collect.sh")} },
	} {
		t.Run(name, func(t *testing.T) {
			untrusted := New()
			configure(untrusted)
			require.Error(
				t,
				untrusted.Init(context.Background()),
				"production path validation must reject the test-owned executable",
			)
		})
	}
}

// Candidate validation stays local: neither Init nor Check runs the executable.
func TestCollector_InitCheckDoNotRunScript(t *testing.T) {
	for _, mode := range []string{modeOneshot, modePersistent} {
		t.Run(mode, func(t *testing.T) {
			c, dir := fixtureCollector(t, "printf ran > \"$(dirname \"$0\")/executed\"\nexit 99\n")
			appendFile(t, c.Manifest, "mode: "+mode+"\n")
			require.NoError(t, c.Init(context.Background()))
			require.NoError(t, c.Check(context.Background()))
			requireNoFile(t, filepath.Join(dir, "executed"))
		})
	}
}

// A registered job keeps the startup definition even if package files change.
func TestCollector_InitRegisteredPackage(t *testing.T) {
	registry, dir := configuredFixture(t, "exit 0\n", modeOneshot)
	c := registry["native-fixture"].CreateV2().(*Collector)
	c.ScriptConfig = Settings{
		"text": "synthetic",
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "manifest.yaml"), []byte("invalid: manifest"), 0644))
	require.NoError(t, c.Init(context.Background()))
	c.Manifest = "/different/package"
	require.ErrorContains(t, c.Init(context.Background()), "cannot override manifest")
}

func TestCollector_InitRejectsUnregisteredFunctions(t *testing.T) {
	_, dir := functionFixture(t, modeOneshot, true)
	c := newManifestCollector(filepath.Join(dir, "manifest.yaml"))
	require.ErrorContains(t, c.Init(context.Background()), "must be registered")
}

func TestCollector_Configuration(t *testing.T) {
	registry, _ := configuredFixture(t, "exit 0\n", modeOneshot)
	c := registry["native-fixture"].CreateV2().(*Collector)
	cfg := c.Configuration().(Config)
	assert.Empty(t, cfg.Manifest, "registered packages do not expose a manifest option")
	assert.Equal(t, float64(17), cfg.ScriptConfig["count"], "registered packages expose effective defaults")
}

func TestCollector_DirectConfigurationSerialize(t *testing.T) {
	collecttest.TestConfigurationSerialize(
		t,
		New(),
		[]byte(
			`{"command":["/opt/custom/collect","fixed argument"],"mode":"persistent","snapshot_format":"lines","update_every":15,"timeout":3.5}`,
		),
		[]byte(
			"command: [/opt/custom/collect, fixed argument]\nmode: persistent\nsnapshot_format: lines\nupdate_every: 15\ntimeout: 3.5\n",
		),
	)
}

func TestCollector_InitDirectCommand(t *testing.T) {
	tests := map[string]struct {
		prepare  func(*Collector)
		wantErr  string
		wantMode string
	}{
		"default one-shot":  {wantMode: modeOneshot},
		"empty mode":        {prepare: func(c *Collector) { c.Mode = "" }, wantMode: modeOneshot},
		"explicit one-shot": {prepare: func(c *Collector) { c.Mode = modeOneshot }, wantMode: modeOneshot},
		"persistent": {
			prepare:  func(c *Collector) { c.Mode = modePersistent },
			wantMode: modePersistent,
		},
		"invalid mode": {prepare: func(c *Collector) { c.Mode = "push" }, wantErr: "mode must"},
		"relative command": {
			prepare: func(c *Collector) { c.Command = []string{"relative"} },
			wantErr: "absolute executable",
		},
		"empty command": {
			prepare: func(c *Collector) { c.Command = []string{} },
			wantErr: "absolute executable",
		},
		"no source": {prepare: func(c *Collector) { c.Command = nil }, wantErr: "exactly one"},
		"two sources": {
			prepare: func(c *Collector) { c.Manifest = "/manifest.yaml" },
			wantErr: "exactly one",
		},
		"config without schema": {
			prepare: func(c *Collector) {
				c.ScriptConfig = Settings{
					"text": "synthetic",
				}
			},
			wantErr: "does not declare config_schema",
		},
		"manifest with mode override": {
			prepare: func(c *Collector) { c.Command = nil; c.Manifest = "/manifest.yaml"; c.Mode = modePersistent },
			wantErr: "mode must be auto",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			c, dir := fixtureCollector(t, "touch \"$(dirname \"$0\")/executed\"\nexit 99\n")
			c.Manifest = ""
			c.Command = []string{filepath.Join(dir, "collect.sh"), "fixed argument"}
			if tc.prepare != nil {
				tc.prepare(c)
			}
			err := c.Init(context.Background())
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				require.Error(t, c.Check(context.Background()))
			} else {
				require.NoError(t, err)
				require.NoError(t, c.Check(context.Background()))
				assert.Equal(t, tc.wantMode, c.definition.Mode)
				assert.Equal(t, c.Command, c.definition.command)
				assert.False(t, c.definition.functionOnly())
				c.Command[1] = "changed"
				assert.Equal(t, "fixed argument", c.definition.command[1], "definition owns its command")
			}
			requireNoFile(t, filepath.Join(dir, "executed"))
		})
	}
}

func TestCollector_DirectCollection(t *testing.T) {
	setupRunner(t)
	python := requireTool(t, "python3")
	const peer = `import json, sys
assert sys.argv[1] == 'fixed argument'
result = {"version":"v1","metrics":[{"name":"depth","samples":[{"value":1.5}]}]}
if sys.argv[-1] == 'serve':
    print(json.dumps({"version":"v1","ready":True}), flush=True)
    for line in sys.stdin:
        request = json.loads(line)
        print(json.dumps({"id":request["id"],"result":result}), flush=True)
else:
    assert sys.argv[-1] == 'collect'
    assert sys.stdin.read() == ''
    print(json.dumps(result))
`
	for _, mode := range []string{modeAuto, modeOneshot, modePersistent} {
		t.Run(mode, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "peer.py")
			require.NoError(t, os.WriteFile(path, []byte(peer), 0644))
			c := New()
			c.validateExecutable = statExecutable
			c.Command = []string{python, path, "fixed argument"}
			c.Mode = confopt.Enum[jobModeSpec](mode)
			require.NoError(t, c.Init(context.Background()))
			if mode == modePersistent {
				startRuntime(t, c).waitReady(t)
			}
			require.NoError(t, collectAndCommit(t, c))
			assert.Equal(t, map[string]float64{"depth": 1.5}, rawSeries(c))
		})
	}
}

func TestCollector_RegisteredSourceOverrides(t *testing.T) {
	registry, _ := configuredFixture(t, "exit 0\n", modeOneshot)
	for name, set := range map[string]func(*Collector){
		"manifest":        func(c *Collector) { c.Manifest = "/different/package" },
		"command":         func(c *Collector) { c.Command = []string{"/different/command"} },
		"empty command":   func(c *Collector) { c.Command = []string{} },
		"mode":            func(c *Collector) { c.Mode = modePersistent },
		"snapshot format": func(c *Collector) { c.SnapshotFormat = formatJSON },
	} {
		t.Run(name, func(t *testing.T) {
			c := registry["native-fixture"].CreateV2().(*Collector)
			set(c)
			require.ErrorContains(t, c.Init(context.Background()), "cannot override")
			cfg := c.Configuration().(Config)
			assert.Empty(t, cfg.Manifest)
			assert.Nil(t, cfg.Command)
			assert.Empty(t, cfg.Mode)
			assert.Empty(t, cfg.SnapshotFormat)
		})
	}
}
