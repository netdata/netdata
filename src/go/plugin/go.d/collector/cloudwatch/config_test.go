// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/cloudwatch/internal/awsauth"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func validBaseConfig() Config {
	return Config{
		UpdateEvery: 60,
		Credentials: map[string]awsauth.CredentialConfig{
			"sdk_default": {Type: awsauth.CredentialTypeDefault},
		},
		Targets:     []TargetConfig{{Name: "base", Credentials: "sdk_default"}},
		Rules:       []RuleConfig{{Name: "base-defaults", Targets: []string{"base"}, Regions: []string{"us-east-1"}}},
		Discovery:   DiscoveryConfig{RefreshEvery: 300},
		QueryOffset: 600,
		Timeout:     defaultTimeout,
	}
}

func TestConfig_validate(t *testing.T) {
	tests := map[string]struct {
		mutate  func(*Config)
		wantErr bool
	}{
		"valid":                        {mutate: func(*Config) {}},
		"no credentials":               {mutate: func(c *Config) { c.Credentials = nil }, wantErr: true},
		"no targets":                   {mutate: func(c *Config) { c.Targets = nil }, wantErr: true},
		"no rules":                     {mutate: func(c *Config) { c.Rules = nil }, wantErr: true},
		"unknown credential reference": {mutate: func(c *Config) { c.Targets[0].Credentials = "missing" }, wantErr: true},
		"duplicate target":             {mutate: func(c *Config) { c.Targets = append(c.Targets, c.Targets[0]) }, wantErr: true},
		"invalid role ARN account": {mutate: func(c *Config) {
			c.Targets[0].AssumeRole = &awsauth.AssumeRoleConfig{RoleARN: "arn:aws:iam::account:role/example"}
		}, wantErr: true},
		"rule without regions":        {mutate: func(c *Config) { c.Rules[0].Regions = nil }, wantErr: true},
		"invalid region":              {mutate: func(c *Config) { c.Rules[0].Regions = []string{"global"} }, wantErr: true},
		"noncanonical region":         {mutate: func(c *Config) { c.Rules[0].Regions = []string{"us-east-1", " US-EAST-1 "} }, wantErr: true},
		"update_every below minimum":  {mutate: func(c *Config) { c.UpdateEvery = 30 }, wantErr: true},
		"refresh_every below minimum": {mutate: func(c *Config) { c.Discovery.RefreshEvery = 30 }, wantErr: true},
		"negative query_offset":       {mutate: func(c *Config) { c.QueryOffset = -1 }, wantErr: true},
		"negative timeout":            {mutate: func(c *Config) { c.Timeout = confopt.Duration(-time.Second) }, wantErr: true},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := validBaseConfig()
			tc.mutate(&cfg)
			err := cfg.validate()
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestConfigSchema_RuntimeContract(t *testing.T) {
	data, err := os.ReadFile("config_schema.json")
	require.NoError(t, err)
	var doc struct {
		JSONSchema struct {
			Required   []string                   `json:"required"`
			Properties map[string]json.RawMessage `json:"properties"`
		} `json:"jsonSchema"`
	}
	require.NoError(t, json.Unmarshal(data, &doc))
	assert.ElementsMatch(t, []string{"credentials", "targets", "rules"}, doc.JSONSchema.Required)
	for _, key := range []string{
		"update_every", "autodetection_retry", "vnode", "credentials", "targets", "rules",
		"discovery", "tags", "query_offset", "timeout",
	} {
		assert.Contains(t, doc.JSONSchema.Properties, key)
	}
	for key, want := range map[string]string{
		"credentials": `{"sdk_default":{"type":"default"}}`,
		"targets":     `[{"name":"base","credentials":"sdk_default"}]`,
		"rules":       `[{"name":"base-defaults","targets":["base"],"regions":["us-east-1"]}]`,
	} {
		var property struct {
			Default json.RawMessage `json:"default"`
		}
		require.NoError(t, json.Unmarshal(doc.JSONSchema.Properties[key], &property))
		assert.JSONEq(t, want, string(property.Default), key)
	}
	assert.Equal(t, 2, strings.Count(string(data), `"sensitive": true`), "only secret access key and session token are sensitive widgets")
	assert.NotContains(t, string(data), `"additionalProperties"`)
}

func TestConfigSchema_ValidationParity(t *testing.T) {
	data, err := os.ReadFile("config_schema.json")
	require.NoError(t, err)
	var doc struct {
		JSONSchema any `json:"jsonSchema"`
	}
	require.NoError(t, json.Unmarshal(data, &doc))

	compiler := jsonschema.NewCompiler()
	require.NoError(t, compiler.AddResource("cloudwatch-config.json", doc.JSONSchema))
	schema, err := compiler.Compile("cloudwatch-config.json")
	require.NoError(t, err)

	valid := map[string]any{
		"name":        "cloudwatch",
		"credentials": map[string]any{"sdk_default": map[string]any{"type": "default"}},
		"targets":     []any{map[string]any{"name": "base", "credentials": "sdk_default"}},
		"rules":       []any{map[string]any{"name": "base-defaults", "targets": []any{"base"}, "regions": []any{"us-east-1"}}},
	}
	require.NoError(t, schema.Validate(valid))
	require.NoError(t, validateRuntimeConfigMap(t, valid))

	strictCases := map[string]func(map[string]any){
		"credential type has surrounding whitespace": func(cfg map[string]any) {
			cfg["credentials"] = map[string]any{"sdk_default": map[string]any{"type": " default "}}
		},
		"credential type is mixed case": func(cfg map[string]any) {
			cfg["credentials"] = map[string]any{"sdk_default": map[string]any{"type": "DEFAULT"}}
		},
		"static access key is whitespace only": func(cfg map[string]any) {
			cfg["credentials"] = map[string]any{"sdk_default": map[string]any{
				"type": "static", "access_key_id": " ", "secret_access_key": "secret",
			}}
		},
		"target name has surrounding whitespace": func(cfg map[string]any) {
			cfg["targets"] = []any{map[string]any{"name": " base ", "credentials": "sdk_default"}}
		},
		"target reference has surrounding whitespace": func(cfg map[string]any) {
			cfg["rules"] = []any{map[string]any{"name": "base-defaults", "targets": []any{" base "}, "regions": []any{"us-east-1"}}}
		},
		"profile reference has surrounding whitespace": func(cfg map[string]any) {
			cfg["rules"] = []any{map[string]any{
				"name": "base-defaults", "targets": []any{"base"}, "regions": []any{"us-east-1"},
				"profiles": map[string]any{"defaults": false, "include": []any{" ec2 "}},
			}}
		},
		"region is not canonical lowercase": func(cfg map[string]any) {
			cfg["rules"] = []any{map[string]any{"name": "base-defaults", "targets": []any{"base"}, "regions": []any{"US-EAST-1"}}}
		},
	}
	for name, mutate := range strictCases {
		t.Run(name, func(t *testing.T) {
			cfg := cloneConfigMap(t, valid)
			mutate(cfg)
			assert.Error(t, schema.Validate(cfg))
			assert.Error(t, validateRuntimeConfigMap(t, cfg))
		})
	}

	t.Run("required static fields remain enforced", func(t *testing.T) {
		cfg := cloneConfigMap(t, valid)
		cfg["credentials"] = map[string]any{"sdk_default": map[string]any{"type": "static", "access_key_id": "key"}}
		assert.Error(t, schema.Validate(cfg))
		assert.Error(t, validateRuntimeConfigMap(t, cfg))
	})

	unknownFieldCases := map[string]func(map[string]any){
		"unknown top-level field": func(cfg map[string]any) { cfg["unknown"] = true },
		"unknown nested field": func(cfg map[string]any) {
			cfg["targets"] = []any{map[string]any{"name": "base", "credentials": "sdk_default", "unknown": true}}
		},
	}
	for name, mutate := range unknownFieldCases {
		t.Run("unknown fields ignored/"+name, func(t *testing.T) {
			cfg := cloneConfigMap(t, valid)
			mutate(cfg)
			assert.NoError(t, schema.Validate(cfg))
			assert.NoError(t, validateRuntimeConfigMap(t, cfg))
		})
	}

	t.Run("runtime rejects default credentials with a non-empty static field", func(t *testing.T) {
		cfg := cloneConfigMap(t, valid)
		cfg["credentials"] = map[string]any{"sdk_default": map[string]any{
			"type": "default", "access_key_id": "rejected-at-runtime",
		}}
		assert.NoError(t, schema.Validate(cfg))
		assert.Error(t, validateRuntimeConfigMap(t, cfg))
	})
}

func cloneConfigMap(t *testing.T, src map[string]any) map[string]any {
	t.Helper()
	data, err := json.Marshal(src)
	require.NoError(t, err)
	var dst map[string]any
	require.NoError(t, json.Unmarshal(data, &dst))
	return dst
}

func validateRuntimeConfigMap(t *testing.T, raw map[string]any) error {
	t.Helper()
	data, err := yaml.Marshal(raw)
	require.NoError(t, err)
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return err
	}
	_, _, err = compileTestConfig(t, cfg)
	return err
}

func TestConfig_TagsDecode(t *testing.T) {
	var cfg Config
	require.NoError(t, yaml.Unmarshal([]byte("tags:\n  - name: owner\n  - name: Name\n    rename: instance_name\n"), &cfg))
	assert.Equal(t, []TagConfig{{Name: "owner"}, {Name: "Name", Rename: "instance_name"}}, cfg.Tags)
}

func TestConfig_validateRejectsReservedRuleFields(t *testing.T) {
	tests := map[string]func(*RuleConfig){
		"filters": func(rule *RuleConfig) { rule.Filters = map[string]any{} },
		"labels":  func(rule *RuleConfig) { rule.Labels = []any{} },
		"series":  func(rule *RuleConfig) { rule.Series = false },
		"query":   func(rule *RuleConfig) { rule.Query = "unsupported" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := validBaseConfig()
			mutate(&cfg.Rules[0])
			assert.ErrorContains(t, cfg.validate(), "reserved for a later phase")
		})
	}
}

func TestConfig_DecodedReservedRuleFieldsReachRuntimeValidation(t *testing.T) {
	tests := map[string]struct {
		decode func(*RuleConfig) error
	}{
		"YAML": {decode: func(rule *RuleConfig) error {
			return yaml.Unmarshal([]byte("filters: {}\n"), rule)
		}},
		"JSON": {decode: func(rule *RuleConfig) error {
			return json.Unmarshal([]byte(`{"query":{}}`), rule)
		}},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := validBaseConfig()
			require.NoError(t, tc.decode(&cfg.Rules[0]))
			assert.ErrorContains(t, cfg.validate(), "reserved for a later phase")
		})
	}
}

func TestNormalizeRegions(t *testing.T) {
	assert.Equal(t, []string{"us-east-1", "eu-west-1"},
		normalizeRegions([]string{"us-east-1", "us-east-1", "eu-west-1", "", "eu-west-1"}))
}
