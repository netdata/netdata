// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

// The limit covers the JSON envelope, effective defaults and terminating LF.
func TestConfigEnvelope_SizeLimit(t *testing.T) {
	registry, _ := configuredFixture(t, "exit 0\n", modeOneshot)
	definition := registry["native-fixture"].CreateV2().(*Collector).definition
	settings := Settings{
		"text": "x",
	}
	frame, err := definition.configEnvelope(settings)
	require.NoError(t, err)
	settings["text"] = strings.Repeat("x", maxMessageBytes-len(frame)+1)
	frame, err = definition.configEnvelope(settings)
	require.NoError(t, err)
	assert.Len(t, frame, maxMessageBytes)
	settings["text"] = settings["text"].(string) + "x"
	_, err = definition.configEnvelope(settings)
	require.ErrorContains(t, err, "config envelope exceeds 64 MiB")
}

func TestConfigEnvelope_PackageWithoutForm(t *testing.T) {
	c, _ := fixtureCollector(t, "exit 0\n")
	envelope, err := c.definition.configEnvelope(nil)
	require.NoError(t, err)
	assert.Nil(t, envelope, "schema-free packages receive EOF on stdin")
	_, err = c.definition.configEnvelope(Settings{
		"text": "x",
	})
	require.ErrorContains(t, err, "does not declare config_schema")
}

func TestConfigEnvelope_ConfiguredPeers(t *testing.T) {
	setupRunner(t)
	for _, mode := range []string{modeOneshot, modePersistent} {
		t.Run(mode, func(t *testing.T) {
			registry, _ := configuredFixture(t, configPeer(t), mode)
			c := registry["native-fixture"].CreateV2().(*Collector)
			// Preserve strings, nested values, explicit zero and false through YAML and stdin.
			config := "config:\n  text: 'quotes \" and \\ unicode λ'\n  count: 23\n  enabled: false\n"
			require.NoError(t, yaml.Unmarshal([]byte(config), c))
			out := &wireOutput{}
			job, _ := startTestJob(t, c, out)
			tickUntil(t, job, func() bool { return strings.Contains(out.String(), " = 23") })
			assert.NotContains(t, out.String(), "unicode")
		})
	}
}

// A Python peer receives exactly the envelope Init encoded, in both modes.
func TestConfigEnvelope_RoundTrip(t *testing.T) {
	setupRunner(t)
	python := requireTool(t, "python3")
	const peer = `import json, pathlib, sys
config = json.loads(sys.stdin.readline())
pathlib.Path(__file__).with_suffix('.received').write_text(json.dumps(config))
result = {"version":"v1", "metrics":[{"name":"depth","unit":"jobs","samples":[{"value":config["config"]["count"],"labels":{"queue":"mail"}}]}]}
if sys.argv[-1] == 'serve':
    print(json.dumps({"version":"v1","ready":True}), flush=True)
    for line in sys.stdin:
        request = json.loads(line)
        print(json.dumps({"id":request["id"],"result":result}), flush=True)
else:
    assert sys.stdin.read() == ''
    print(json.dumps(result))
`
	for _, mode := range []string{modeOneshot, modePersistent} {
		t.Run(mode, func(t *testing.T) {
			registry, dir := configuredFixture(
				t,
				fmt.Sprintf("exec %q \"$(dirname \"$0\")/peer.py\" \"$@\"\n", python),
				mode,
			)
			require.NoError(t, os.WriteFile(filepath.Join(dir, "peer.py"), []byte(peer), 0644))
			c := registry["native-fixture"].CreateV2().(*Collector)
			c.ScriptConfig = Settings{
				"text":     "line one\nline two\r\tλ\\\"",
				"count":    float64(0),
				"enabled":  false,
				"optional": nil,
			}
			require.NoError(t, c.Init(context.Background()))
			if mode == modePersistent {
				startRuntime(t, c).waitReady(t)
			}
			values, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
			require.NoError(t, err)
			assert.Equal(t, map[string]float64{`depth{queue="mail"}`: 0}, values)
			received, err := os.ReadFile(filepath.Join(dir, "peer.received"))
			require.NoError(t, err)
			var got, want any
			require.NoError(t, json.Unmarshal(received, &got))
			require.NoError(t, json.Unmarshal(c.configEnvelope, &want))
			assert.Equal(t, want, got)
		})
	}
}

func TestGenericJobFormSources(t *testing.T) {
	var form map[string]any
	require.NoError(t, json.Unmarshal([]byte(configSchema), &form))
	compiler := jsonschema.NewCompiler()
	compiler.UseLoader(nil)
	require.NoError(t, compiler.AddResource("urn:native-form", form["jsonSchema"]))
	schema, err := compiler.Compile("urn:native-form")
	require.NoError(t, err)
	for name, tc := range map[string]struct {
		config  map[string]any
		wantErr bool
	}{
		"manifest":                    {config: map[string]any{"manifest": "/manifest.yaml"}},
		"manifest auto":               {config: map[string]any{"manifest": "/manifest.yaml", "mode": "auto"}},
		"manifest with unset command": {config: map[string]any{"manifest": "/manifest.yaml", "command": nil}},
		"unset command alone":         {config: map[string]any{"command": nil}, wantErr: true},
		"direct one-shot default":     {config: map[string]any{"command": []any{"/collect"}}},
		"direct auto":                 {config: map[string]any{"command": []any{"/collect"}, "mode": "auto"}},
		"direct one-shot":             {config: map[string]any{"command": []any{"/collect"}, "mode": "oneshot"}},
		"direct persistent":           {config: map[string]any{"command": []any{"/collect"}, "mode": "persistent"}},
		"no source":                   {config: map[string]any{}, wantErr: true},
		"two sources":                 {config: map[string]any{"manifest": "/manifest.yaml", "command": []any{"/collect"}}, wantErr: true},
		"manifest format auto":        {config: map[string]any{"manifest": "/manifest.yaml", "snapshot_format": "auto"}},
		"manifest format override":    {config: map[string]any{"manifest": "/manifest.yaml", "snapshot_format": "json"}, wantErr: true},
		"direct lines":                {config: map[string]any{"command": []any{"/collect"}, "snapshot_format": "lines"}},
		"direct json":                 {config: map[string]any{"command": []any{"/collect"}, "snapshot_format": "json"}},
		"invalid format":              {config: map[string]any{"command": []any{"/collect"}, "snapshot_format": "invalid"}, wantErr: true},
		"null format":                 {config: map[string]any{"command": []any{"/collect"}, "snapshot_format": nil}, wantErr: true},
		"manifest mode override":      {config: map[string]any{"manifest": "/manifest.yaml", "mode": "oneshot"}, wantErr: true},
		"invalid mode":                {config: map[string]any{"command": []any{"/collect"}, "mode": "push"}, wantErr: true},
		"empty mode":                  {config: map[string]any{"command": []any{"/collect"}, "mode": ""}, wantErr: true},
	} {
		t.Run(name, func(t *testing.T) {
			if tc.wantErr {
				require.Error(t, schema.Validate(tc.config))
			} else {
				require.NoError(t, schema.Validate(tc.config))
			}
		})
	}
}

// DynCfg can return configuration before Init. Its default must round-trip for
// either source without replacing a manifest's execution mode.
func TestJobModeDefaultRoundTrip(t *testing.T) {
	assert.Equal(t, modeAuto, string(New().Mode))
	assert.Equal(t, modeAuto, string(collectorapi.DefaultRegistry["native"].Config().(*Config).Mode))
	for format, codec := range map[string]struct {
		marshal   func(any) ([]byte, error)
		unmarshal func([]byte, any) error
	}{
		"json": {marshal: json.Marshal, unmarshal: json.Unmarshal},
		"yaml": {marshal: yaml.Marshal, unmarshal: yaml.Unmarshal},
	} {
		for _, source := range []string{"command", "manifest", "registered"} {
			for _, mode := range []string{"omitted", "null", "", modeAuto} {
				t.Run(format+"/"+source+"/"+mode, func(t *testing.T) {
					c, dir := fixtureCollector(t, "exit 0\n")
					appendFile(t, c.Manifest, "mode: persistent\n")
					newCollector := New
					wantMode := modePersistent
					if source == "command" {
						c.Manifest = ""
						c.Command = []string{filepath.Join(dir, "collect.sh")}
						wantMode = modeOneshot
					} else if source == "registered" {
						registry, _ := configuredFixture(t, "exit 0\n", modePersistent)
						newCollector = func() *Collector { return registry["native-fixture"].CreateV2().(*Collector) }
						c = newCollector()
						c.ScriptConfig["text"] = "synthetic"
					}
					c.Mode = "" // Also cover decoding into a zero-valued option.
					input := map[string]any{}
					switch mode {
					case "omitted":
					case "null":
						input["mode"] = nil
					default:
						input["mode"] = mode
					}
					data, err := codec.marshal(input)
					require.NoError(t, err)
					require.NoError(t, codec.unmarshal(data, &c.Config))
					data, err = codec.marshal(c.Configuration())
					require.NoError(t, err)
					var returned map[string]any
					require.NoError(t, codec.unmarshal(data, &returned))
					if source == "registered" {
						assert.NotContains(t, returned, "mode")
					} else {
						assert.Equal(t, modeAuto, returned["mode"])
					}
					reloaded := newCollector()
					reloaded.validateExecutable = statExecutable
					require.NoError(t, codec.unmarshal(data, &reloaded.Config))
					require.NoError(t, reloaded.Init(context.Background()))
					assert.Equal(t, wantMode, reloaded.definition.Mode)
				})
			}
		}
	}
}

func TestJobSnapshotFormatDefaultRoundTrip(t *testing.T) {
	assert.Equal(t, modeAuto, string(New().SnapshotFormat))
	assert.Equal(t, modeAuto, string(collectorapi.DefaultRegistry["native"].Config().(*Config).SnapshotFormat))
	for encoding, codec := range map[string]struct {
		marshal   func(any) ([]byte, error)
		unmarshal func([]byte, any) error
	}{
		"json": {marshal: json.Marshal, unmarshal: json.Unmarshal},
		"yaml": {marshal: yaml.Marshal, unmarshal: yaml.Unmarshal},
	} {
		for _, source := range []string{"command", "manifest", "registered"} {
			for _, format := range []string{"omitted", "null", "", modeAuto} {
				t.Run(encoding+"/"+source+"/"+format, func(t *testing.T) {
					c, dir := fixtureCollector(t, "exit 0\n")
					appendFile(t, c.Manifest, "snapshot_format: lines\n")
					newCollector := New
					wantFormat := formatLines
					if source == "command" {
						c.Manifest = ""
						c.Command = []string{filepath.Join(dir, "collect.sh")}
						wantFormat = formatJSON
					} else if source == "registered" {
						_, registeredDir := configuredFixture(t, "exit 0\n", modeOneshot)
						appendFile(t, filepath.Join(registeredDir, "manifest.yaml"), "snapshot_format: lines\n")
						registry, err := loadPackages(context.Background(), filepath.Join(registeredDir, "packages.yaml"), nil, statExecutable)
						require.NoError(t, err)
						newCollector = func() *Collector { return registry["native-fixture"].CreateV2().(*Collector) }
						c = newCollector()
						c.ScriptConfig["text"] = "synthetic"
					}
					c.SnapshotFormat = "" // Also cover decoding into a zero-valued option.
					input := map[string]any{}
					switch format {
					case "omitted":
					case "null":
						input["snapshot_format"] = nil
					default:
						input["snapshot_format"] = format
					}
					data, err := codec.marshal(input)
					require.NoError(t, err)
					require.NoError(t, codec.unmarshal(data, &c.Config))
					data, err = codec.marshal(c.Configuration())
					require.NoError(t, err)
					var returned map[string]any
					require.NoError(t, codec.unmarshal(data, &returned))
					if source == "registered" {
						assert.NotContains(t, returned, "snapshot_format")
					} else {
						assert.Equal(t, modeAuto, returned["snapshot_format"])
					}
					reloaded := newCollector()
					reloaded.validateExecutable = statExecutable
					require.NoError(t, codec.unmarshal(data, &reloaded.Config))
					require.NoError(t, reloaded.Init(context.Background()))
					assert.Equal(t, wantFormat, reloaded.definition.SnapshotFormat)
				})
			}
		}
	}
}
