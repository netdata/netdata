// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/joboutput"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets"
	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestLoadPackages_Inventory(t *testing.T) {
	c, _ := fixtureCollector(t, "exit 0\n")
	valid := fmt.Sprintf("  - name: fixture\n    manifest: %s\n", c.Manifest)
	tests := map[string]struct {
		inventory string
		wantErr   bool
	}{
		"valid":               {inventory: "version: v1\npackages:\n" + valid},
		"unsupported version": {inventory: "version: v2\npackages:\n" + valid, wantErr: true},
		"second document":     {inventory: "version: v1\npackages:\n" + valid + "---\nversion: v1\n", wantErr: true},
		"unknown field":       {inventory: "version: v1\npackages:\n" + valid + "    extra: true\n", wantErr: true},
		"duplicate":           {inventory: "version: v1\npackages:\n" + valid + valid, wantErr: true},
		"invalid name": {
			inventory: "version: v1\npackages:\n" + valid + "  - name: bad_name\n    manifest: /unused\n",
			wantErr:   true,
		},
		"relative manifest": {
			inventory: "version: v1\npackages:\n  - name: relative\n    manifest: relative.yaml\n",
			wantErr:   true,
		},
		"both sources": {
			inventory: "version: v1\npackages:\n  - name: fixture\n    manifest: /unused\n    command: [/bin/sh]\n",
			wantErr:   true,
		},
		"neither source": {inventory: "version: v1\npackages:\n  - name: fixture\n", wantErr: true},
		"empty command":  {inventory: "version: v1\npackages:\n  - name: fixture\n    command: []\n", wantErr: true},
		"relative command": {
			inventory: "version: v1\npackages:\n  - name: fixture\n    command: [relative-script]\n",
			wantErr:   true,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "packages.yaml")
			require.NoError(t, os.WriteFile(path, []byte(tc.inventory), 0644))
			base := collectorapi.Registry{
				"base": {},
			}
			registry, err := loadPackages(context.Background(), path, base, statExecutable)
			assert.Equal(t, collectorapi.Registry{
				"base": {},
			}, base, "registration never mutates the caller's registry")
			if tc.wantErr {
				require.Error(t, err)
				assert.Nil(t, registry)
				return
			}
			require.NoError(t, err)
			assert.ElementsMatch(t, []string{"base", "native-fixture"}, slices.Collect(maps.Keys(registry)))
		})
	}
}

func TestLoadPackages_Cancellation(t *testing.T) {
	_, dir := configuredFixture(t, "exit 0\n", modeOneshot)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	base := collectorapi.Registry{}
	registry, err := loadPackages(ctx, filepath.Join(dir, "packages.yaml"), base, func(path string) (string, error) {
		// Cancellation can arrive while the final entry is still being validated.
		cancel()
		return statExecutable(path)
	})
	require.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, registry, "canceled registration must not publish the new registry")
	assert.Empty(t, base)
}

func TestPackageCreator_DefaultsAndIsolation(t *testing.T) {
	registry, dir := configuredFixture(t, "touch \"$(dirname \"$0\")/started\"\n", modePersistent)
	creator := registry["native-fixture"]
	c := creator.CreateV2().(*Collector)
	config := "config:\n  text: synthetic\n  count: 0\n  enabled: false\n  optional: null\n"
	require.NoError(t, yaml.Unmarshal([]byte(config), c))
	want := Settings{
		"text":     "synthetic",
		"count":    float64(0),
		"enabled":  false,
		"optional": nil,
		"nested":   map[string]any{"value": float64(3)},
	}
	assert.Equal(t, want, c.Configuration().(Config).ScriptConfig)
	require.NoError(t, c.Init(context.Background()))
	var envelope struct {
		Version string
		Config  Settings
	}
	require.NoError(t, json.Unmarshal(c.configEnvelope, &envelope))
	assert.Equal(t, want, envelope.Config)
	second := creator.CreateV2().(*Collector)
	c.ScriptConfig["nested"] = map[string]any{"value": float64(91)}
	assert.Equal(t, map[string]any{"value": float64(3)}, second.Configuration().(Config).ScriptConfig["nested"])
	requireNoFile(t, filepath.Join(dir, "started"))
}

// Registered package forms drop the manifest option; function-only forms also
// omit the collection interval and show timeout only in persistent mode.
func TestPackageCreator_JobForm(t *testing.T) {
	setupRunner(t)
	tests := map[string]struct {
		registry         func(t *testing.T) collectorapi.Registry
		wantFunctionOnly bool
		wantProperties   []string
	}{
		"metrics package": {
			registry: func(t *testing.T) collectorapi.Registry {
				registry, _ := configuredFixture(t, "exit 0\n", modeOneshot)
				return registry
			},
			wantProperties: []string{"autodetection_retry", "config", "timeout", "update_every"},
		},
		"function-only one-shot": {
			registry: func(t *testing.T) collectorapi.Registry {
				registry, _ := functionFixture(t, modeOneshot, true)
				return registry
			},
			wantFunctionOnly: true,
			wantProperties:   []string{"autodetection_retry", "config"},
		},
		"function-only persistent": {
			registry: func(t *testing.T) collectorapi.Registry {
				registry, _ := functionFixture(t, modePersistent, true)
				return registry
			},
			wantFunctionOnly: true,
			wantProperties:   []string{"autodetection_retry", "config", "timeout"},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			creator := tc.registry(t)["native-fixture"]
			assert.Equal(t, tc.wantFunctionOnly, creator.FunctionOnly)
			var form struct {
				JSONSchema struct {
					Properties map[string]any
				} `json:"jsonSchema"`
			}
			require.NoError(t, json.Unmarshal([]byte(creator.JobConfigSchema), &form))
			assert.ElementsMatch(t, tc.wantProperties, slices.Collect(maps.Keys(form.JSONSchema.Properties)))
			var document map[string]any
			require.NoError(t, json.Unmarshal([]byte(creator.JobConfigSchema), &document))
			compiler := jsonschema.NewCompiler()
			compiler.UseLoader(nil)
			require.NoError(t, compiler.AddResource("urn:package-form", document["jsonSchema"]))
			schema, err := compiler.Compile("urn:package-form")
			require.NoError(t, err)
			require.NoError(
				t,
				schema.Validate(map[string]any{}),
				"registered form must not retain source-selection conditions",
			)
			ui := document["uiSchema"].(map[string]any)
			for _, removed := range []string{"manifest", "command", "mode", "snapshot_format"} {
				assert.NotContains(t, ui, removed)
			}

		})
	}
}

// The embedded package form keeps working local references in the job form.
func TestPackageForm(t *testing.T) {
	tests := map[string]struct {
		form    string
		valid   []map[string]any
		invalid []map[string]any
		wantUI  map[string]any
	}{
		"local references": {
			form:    fixtureSchema,
			valid:   []map[string]any{{"text": "valid"}},
			invalid: []map[string]any{{"text": false}},
			wantUI:  map[string]any{"text": map[string]any{"ui:widget": "password"}},
		},
		"escaped and boolean references": {
			form: `{
			 "jsonSchema": {
			  "$schema":"http://json-schema.org/draft-07/schema#", "type":"object",
			  "properties":{"text":{"$ref":"#/definitions/alias"},"blocked":{"$ref":"#/definitions/blocked"}},
			  "definitions":{
			   "alias":{"$ref":"#/definitions/a~1b%20~0"},
			   "a/b ~":{"type":"string"},
			   "blocked":false
			  },
			  "examples":[{"$ref":"file:///annotation-is-data"}]
			 },
			 "uiSchema":{}
			}`,
			valid:   []map[string]any{{"text": "valid"}},
			invalid: []map[string]any{{"text": false}, {"blocked": "anything"}},
			wantUI:  map[string]any{},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			// A real metrics package: its job form keeps the collection options.
			description := `{"version":"v1","config_schema":` +
				tc.form + `}`
			definition, err := parseDescription([]byte(description), []string{"/configured/program"})
			require.NoError(t, err)
			composed, err := packageForm("native-fixture", definition)
			require.NoError(t, err)
			var jobForm map[string]any
			require.NoError(t, json.Unmarshal([]byte(composed), &jobForm))
			compiler := jsonschema.NewCompiler()
			compiler.UseLoader(nil)
			require.NoError(t, compiler.AddResource("urn:fixture:form", jobForm["jsonSchema"]))
			schema, err := compiler.Compile("urn:fixture:form")
			require.NoError(t, err)
			for _, config := range tc.valid {
				require.NoError(t, schema.Validate(map[string]any{"config": config}))
			}
			for _, config := range tc.invalid {
				require.Error(t, schema.Validate(map[string]any{"config": config}))
			}
			assert.Equal(t, tc.wantUI, jobForm["uiSchema"].(map[string]any)["config"])
		})
	}
}

func TestPackageDynCfgConfigOwners(t *testing.T) {
	registry, dir := configuredFixture(t, "touch \"$(dirname \"$0\")/started\"\n", modeOneshot)
	calls := 0
	resolver, err := secretresolver.NewAtomicResolver(map[string]secretresolver.AtomicProvider{
		"fixture": secretresolver.AtomicProviderFunc(
			func(context.Context, string) ([]byte, error) { calls++; return []byte("synthetic-resolved"), nil },
		),
	})
	require.NoError(t, err)
	configs, err := secrets.NewConfigResolver(resolver, func([]string) (secretresolver.AtomicScope, error) {
		t.Fatal("unexpected Store acquisition")
		return nil, nil
	})
	require.NoError(t, err)
	factory, err := joboutput.NewConfigModuleFactory(joboutput.ConfigModuleFactoryConfig{
		Modules: registry,
		Configs: configs,
	})
	require.NoError(t, err)
	input := confgroup.Config{
		"name":   "job",
		"module": "native-fixture",
		"config": map[string]any{"text": "${fixture:value}", "enabled": false, "count": 0},
	}
	input.SetSourceType(confgroup.TypeDyncfg)
	require.NoError(t, factory.Validate(context.Background(), input))
	assert.Zero(t, calls, "structural admission must not resolve references")
	raw, err := factory.Configuration(context.Background(), input)
	require.NoError(t, err)
	assert.Contains(t, string(raw), "${fixture:value}")
	assert.Contains(t, string(raw), `"count":0`)
	assert.Contains(t, string(raw), `"value":3`)
	require.NoError(t, factory.Test(context.Background(), input))
	assert.Equal(t, 1, calls)
	assert.Equal(t, "${fixture:value}", input["config"].(map[string]any)["text"])
	invalid := map[string]any{
		"wrong type":    map[string]any{"text": 45},
		"unknown key":   map[string]any{"text": "synthetic", "private-value": true},
		"null required": map[string]any{"text": nil},
	}
	for name, config := range invalid {
		t.Run(name, func(t *testing.T) {
			cfg := confgroup.Config{
				"name":   "job",
				"module": "native-fixture",
				"config": config,
			}
			cfg.SetSourceType(confgroup.TypeDyncfg)
			err := factory.Test(context.Background(), cfg)
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "private-value")
		})
	}
	input["config"] = "SYNTHETIC_INVALID_OBJECT"
	err = factory.Validate(context.Background(), input)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "SYNTHETIC_INVALID_OBJECT")
	requireNoFile(t, filepath.Join(dir, "started"))
}

// A self-contained package describes itself once at registration; jobs,
// DynCfg tests and replacements reuse that description.
func TestLoadPackages_SelfContainedPackage(t *testing.T) {
	setupRunner(t)
	// Reuse the real configured peer for the operational protocol. Its describe
	// branch runs before reading configuration and prints inline assets.
	_, dir := functionFixture(t, modePersistent, false)
	var form map[string]any
	require.NoError(t, json.Unmarshal([]byte(fixtureSchema), &form))
	description, err := json.Marshal(map[string]any{
		"version":       "v1",
		"mode":          "persistent",
		"functions":     []any{map[string]any{"id": "items", "name": "Items", "help": "Show items 😀."}},
		"config_schema": form,
	})
	require.NoError(t, err)
	describe := fmt.Sprintf(`
if sys.argv[-1] == "describe":
    assert sys.stdin.read() == ""
    with (root / "described").open("a") as f: f.write("describe\n")
    print(json.dumps(json.loads(%q)))
    sys.exit(0)
`, string(description))
	readConfig := `config = json.loads(sys.stdin.readline())["config"]`
	peer := strings.Replace(functionPeer, readConfig, describe+readConfig, 1)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "peer.py"), []byte(peer), 0644))
	command := []string{filepath.Join(dir, "collect.sh")}
	registry := loadTestPackages(t, writeCommandInventory(t, command))
	// Editing a former sidecar cannot influence the executable package.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "manifest.yaml"), []byte("invalid"), 0644))
	creator := registry["native-fixture"]
	assert.Equal(t, "Show items 😀.", creator.SharedFunctions()[0].Help)
	assert.Contains(t, creator.JobConfigSchema, "Fixture")
	for range 2 {
		c := initFunctionCollector(t, registry)
		assert.Equal(t, command, c.definition.command)
	}
	factory, err := joboutput.NewConfigModuleFactory(joboutput.ConfigModuleFactoryConfig{
		Modules: registry,
		Configs: &secrets.ConfigResolver{},
	})
	require.NoError(t, err)
	cfg := confgroup.Config{
		"name":   "test",
		"module": "native-fixture",
		"config": map[string]any{"text": "synthetic"},
	}
	cfg.SetSourceType(confgroup.TypeDyncfg)
	require.NoError(t, factory.Validate(context.Background(), cfg))
	require.NoError(t, factory.Test(context.Background(), cfg))
	// Route real requests through Agent/DynCfg, including a replacement job.
	a := startTestAgent(t, registry, dir)
	a.enable(t, "alpha", 23)
	a.call(t, "items", "native-fixture:items __job:alpha", "", 200)
	a.call(
		t,
		"update",
		"config scripts.d:collector:native-fixture:alpha update",
		`{"update_every":1,"config":{"text":"changed","count":41}}`,
		202,
	)
	require.Eventually(t, func() bool {
		id := fmt.Sprintf("after-%d", time.Now().UnixNano())
		return strings.Contains(a.call(t, id, "native-fixture:items __job:alpha", "", 0), "[[41,")
	}, 5*time.Second, 20*time.Millisecond)
	described, err := os.ReadFile(filepath.Join(dir, "described"))
	require.NoError(t, err)
	assert.Equal(t, "describe\n", string(described), "describe runs exactly once")
}
