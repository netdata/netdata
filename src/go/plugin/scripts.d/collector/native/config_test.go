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

func configuredFixture(t *testing.T, body, mode string) (collectorapi.Registry, string) {
	t.Helper()
	c, dir := fixtureCollector(t, body)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config_schema.json"), []byte(fixtureSchema), 0644))
	manifest, err := os.ReadFile(c.Manifest)
	require.NoError(t, err)
	manifest = append(manifest, []byte("config_schema: config_schema.json\nmode: "+mode+"\n")...)
	require.NoError(t, os.WriteFile(c.Manifest, manifest, 0644))
	inventory := filepath.Join(dir, "packages.yaml")
	require.NoError(
		t,
		os.WriteFile(
			inventory,
			[]byte(fmt.Sprintf("version: v1\npackages:\n  - name: fixture\n    manifest: %s\n", c.Manifest)),
			0644,
		),
	)
	registry, err := loadPackages(
		inventory,
		collectorapi.Registry{},
		func(path string) (string, error) { _, err := os.Stat(path); return path, err },
	)
	require.NoError(t, err)
	return registry, dir
}

func TestPackageConfigurationDefaultsAndIsolation(t *testing.T) {
	registry, dir := configuredFixture(t, "touch \"$(dirname \"$0\")/started\"\n", modePersistent)
	creator := registry["native-fixture"]
	c := creator.CreateV2().(*Collector)
	require.NoError(
		t,
		yaml.Unmarshal([]byte("config:\n  text: synthetic\n  count: 0\n  enabled: false\n  optional: null\n"), c),
	)
	expected := Settings{
		"text":     "synthetic",
		"count":    float64(0),
		"enabled":  false,
		"optional": nil,
		"nested":   map[string]any{"value": float64(3)},
	}
	assert.Equal(t, expected, c.Configuration().(Config).ScriptConfig)
	require.NoError(t, c.Init(context.Background()))
	var envelope struct {
		Version string
		Config  Settings
	}
	require.NoError(t, json.Unmarshal(c.configInput, &envelope))
	assert.Equal(t, expected, envelope.Config)
	second := creator.CreateV2().(*Collector)
	c.ScriptConfig["nested"] = map[string]any{"value": float64(91)}
	assert.Equal(t, map[string]any{"value": float64(3)}, second.Configuration().(Config).ScriptConfig["nested"])
	_, err := os.Stat(filepath.Join(dir, "started"))
	require.ErrorIs(t, err, os.ErrNotExist)
	// A bound creator keeps its validated definition even if the disk manifest changes.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "manifest.yaml"), []byte("invalid: manifest"), 0644))
	require.NoError(t, c.Init(context.Background()))
	c.Manifest = "/different/package"
	require.ErrorContains(t, c.Init(context.Background()), "cannot override manifest")
}

func TestPackageFormLocalReferences(t *testing.T) {
	registry, _ := configuredFixture(t, "exit 0\n", modeOneshot)
	var form map[string]any
	require.NoError(t, json.Unmarshal([]byte(registry["native-fixture"].JobConfigSchema), &form))
	compiler := jsonschema.NewCompiler()
	require.NoError(t, compiler.AddResource("urn:fixture:form", form["jsonSchema"]))
	schema, err := compiler.Compile("urn:fixture:form")
	require.NoError(t, err)
	require.NoError(t, schema.Validate(map[string]any{"config": map[string]any{"text": "valid"}}))
	require.Error(t, schema.Validate(map[string]any{"config": map[string]any{"text": false}}))
	assert.Equal(
		t,
		map[string]any{"ui:widget": "password"},
		form["uiSchema"].(map[string]any)["config"].(map[string]any)["text"],
	)
}

func TestPackageFormEscapedAndBooleanReferences(t *testing.T) {
	dir := t.TempDir()
	body := `{
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
	}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "schema.json"), []byte(body), 0644))
	config, err := loadPackageConfig(dir, "schema.json")
	require.NoError(t, err)
	composed, err := packageForm("native-fixture", manifest{
		config: config,
	})
	require.NoError(t, err)
	var form map[string]any
	require.NoError(t, json.Unmarshal([]byte(composed), &form))
	compiler := jsonschema.NewCompiler()
	compiler.UseLoader(nil)
	require.NoError(t, compiler.AddResource("urn:fixture:form", form["jsonSchema"]))
	schema, err := compiler.Compile("urn:fixture:form")
	require.NoError(t, err)
	require.NoError(t, schema.Validate(map[string]any{"config": map[string]any{"text": "valid"}}))
	require.Error(t, schema.Validate(map[string]any{"config": map[string]any{"text": false}}))
	require.Error(t, schema.Validate(map[string]any{"config": map[string]any{"blocked": "anything"}}))
}

func TestPackageSchemaRejectsUnsafeOrAmbiguousInputs(t *testing.T) {
	cases := map[string]string{
		"invalid default": strings.Replace(fixtureSchema, `"default": 17`, `"default": -1`, 1),
		"external reference": strings.Replace(
			fixtureSchema,
			"#/definitions/text",
			"https://invalid.example/schema",
			1,
		),
		"file reference": strings.Replace(fixtureSchema, "#/definitions/text", "file:///private/secret", 1),
		"reference default": strings.Replace(
			fixtureSchema,
			`"default": "fallback"`,
			`"default": "${env:SYNTHETIC}"`,
			1,
		),
		"conditional default": strings.Replace(
			fixtureSchema,
			`"minLength": 1`,
			`"minLength": 1, "default":"bad"`,
			1,
		),
		"resource ID": strings.Replace(
			fixtureSchema,
			`"title": "Fixture"`,
			`"title": "Fixture", "$id":"urn:other"`,
			1,
		),
		"duplicate key":          strings.Replace(fixtureSchema, `"minLength": 1`, `"minLength": 1, "minLength":2`, 1),
		"unknown envelope field": strings.Replace(fixtureSchema, `"uiSchema":`, `"unexpected":true,"uiSchema":`, 1),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "schema.json"), []byte(body), 0644))
			_, err := loadPackageConfig(dir, "schema.json")
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "SYNTHETIC")
		})
	}
}

func TestPackageSchemaReferenceBoundaries(t *testing.T) {
	dir := t.TempDir()
	external := filepath.Join(dir, "external.json")
	require.NoError(
		t,
		os.WriteFile(external, []byte(`{"$schema":"http://json-schema.org/draft-07/schema#","type":"string"}`), 0644),
	)
	for name, extra := range map[string]string{
		"hidden external reference": fmt.Sprintf(`"properties":{"text":{"$ref":"#/x-hidden"}},"x-hidden":{"$ref":%q}`, "file://"+external),
		"hidden local reference":    `"properties":{"text":{"$ref":"#/x-hidden"}},"x-hidden":{"$ref":"#/definitions/text"},"definitions":{"text":{"type":"string"}}`,
		"external nested dialect":   fmt.Sprintf(`"properties":{"text":{"type":"string","$schema":%q}}`, "file://"+external),
	} {
		t.Run(name, func(t *testing.T) {
			body := `{"jsonSchema":{"$schema":"http://json-schema.org/draft-07/schema#","type":"object",` + extra + `},"uiSchema":{}}`
			require.NoError(t, os.WriteFile(filepath.Join(dir, "schema.json"), []byte(body), 0644))
			_, err := loadPackageConfig(dir, "schema.json")
			require.Error(t, err)
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
	factory, err := joboutput.NewConfigModuleFactory(
		joboutput.ConfigModuleFactoryConfig{
			Modules: registry,
			Configs: configs,
		},
	)
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
	for name, value := range map[string]any{"wrong type": map[string]any{"text": 45}, "unknown key": map[string]any{"text": "synthetic", "private-value": true}, "null required": map[string]any{"text": nil}} {
		t.Run(name, func(t *testing.T) {
			invalid := confgroup.Config{
				"name":   "job",
				"module": "native-fixture",
				"config": value,
			}
			invalid.SetSourceType(confgroup.TypeDyncfg)
			err := factory.Test(context.Background(), invalid)
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "private-value")
		})
	}
	input["config"] = "SYNTHETIC_INVALID_OBJECT"
	err = factory.Validate(context.Background(), input)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "SYNTHETIC_INVALID_OBJECT")
	_, err = os.Stat(filepath.Join(dir, "started"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestPackageInventoryValidation(t *testing.T) {
	_, dir := configuredFixture(t, "exit 0\n", modeOneshot)
	manifest := filepath.Join(dir, "manifest.yaml")
	valid := fmt.Sprintf("  - name: fixture\n    manifest: %s\n", manifest)
	for name, entries := range map[string]string{
		"duplicate":         valid + valid,
		"invalid name":      valid + "  - name: bad_name\n    manifest: /unused\n",
		"relative manifest": "  - name: relative\n    manifest: relative.yaml\n",
		"unknown field":     valid + "    extra: true\n",
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "inventory.yaml")
			require.NoError(t, os.WriteFile(path, []byte("version: v1\npackages:\n"+entries), 0644))
			base := collectorapi.Registry{}
			_, err := loadPackages(path, base, func(path string) (string, error) { return path, nil })
			require.Error(t, err)
			assert.Empty(t, base, "failed registration must not partially mutate the caller's registry")
		})
	}
}
