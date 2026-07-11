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
		"duplicate normalized region": {mutate: func(c *Config) { c.Rules[0].Regions = []string{"us-east-1", " US-EAST-1 "} }, wantErr: true},
		"future rule field":           {mutate: func(c *Config) { c.Rules[0].Query = map[string]any{"period": 300} }, wantErr: true},
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

	tests := map[string]func(map[string]any){
		"unknown top-level field": func(cfg map[string]any) { cfg["unknown"] = true },
		"default credentials with access key": func(cfg map[string]any) {
			cfg["credentials"] = map[string]any{"sdk_default": map[string]any{"type": "default", "access_key_id": "must-not-be-accepted"}}
		},
		"static credentials without secret": func(cfg map[string]any) {
			cfg["credentials"] = map[string]any{"sdk_default": map[string]any{"type": "static", "access_key_id": "key"}}
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := make(map[string]any, len(valid))
			for key, value := range valid {
				cfg[key] = value
			}
			mutate(cfg)
			assert.Error(t, schema.Validate(cfg))
		})
	}
}

func TestConfig_TagsDecode(t *testing.T) {
	var cfg Config
	require.NoError(t, yaml.Unmarshal([]byte("tags:\n  - name: owner\n  - name: Name\n    rename: instance_name\n"), &cfg))
	assert.Equal(t, []TagConfig{{Name: "owner"}, {Name: "Name", Rename: "instance_name"}}, cfg.Tags)
}

func TestConfig_YAMLRejectsUnknownKeys(t *testing.T) {
	tests := map[string]string{
		"root":       "unknown: true\n",
		"credential": "credentials:\n  base:\n    type: default\n    typo: true\n",
		"target":     "targets:\n  - name: base\n    credentials: base\n    typo: true\n",
		"rule":       "rules:\n  - name: base\n    targets: [base]\n    regions: [us-east-1]\n    typo: true\n",
		"profiles":   "rules:\n  - name: base\n    targets: [base]\n    regions: [us-east-1]\n    profiles:\n      typo: true\n",
		"discovery":  "discovery:\n  typo: true\n",
		"tags":       "tags:\n  - name: owner\n    typo: true\n",
	}
	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			var cfg Config
			assert.ErrorContains(t, yaml.Unmarshal([]byte(data), &cfg), "unknown config key")
		})
	}
}

func TestConfig_YAMLAcceptsFrameworkKeys(t *testing.T) {
	var cfg Config
	err := yaml.Unmarshal([]byte("name: example\nmodule: cloudwatch\npriority: 100\nlabels:\n  site: test\n"), &cfg)
	assert.NoError(t, err)
}

func TestNormalizeRegions(t *testing.T) {
	assert.Equal(t, []string{"us-east-1", "eu-west-1"},
		normalizeRegions([]string{"us-east-1", " US-EAST-1 ", "EU-West-1", "", "eu-west-1"}))
}
