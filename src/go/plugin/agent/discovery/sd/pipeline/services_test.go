// SPDX-License-Identifier: GPL-3.0-or-later

package pipeline

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/discovery/sd/model"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestServiceEngine_compose(t *testing.T) {
	// Config A:
	// - rule1: matches exactly Name == "mock1" and yields 1 config
	// - rule2: matches Name == "mock2" or "mock3" and yields 2 configs (YAML list)
	// - drop1: matches Name == "dropme" but has no config_template => hard drop (break)
	// - rule3: another rule matching Name == "mock3" to validate multi-rule aggregation
	configA := `
- id: "rule1"
  match: '{{ eq .Name "mock1" }}'
  config_template: |
    name: {{ .Name }}-1
- id: "rule2"
  match: '{{ or (eq .Name "mock2") (eq .Name "mock3") }}'
  config_template: |
    - name: {{ .Name }}-2
    - name: {{ .Name }}-3
- id: "drop1"
  match: '{{ eq .Name "dropme" }}'
- id: "rule3"
  match: '{{ eq .Name "mock3" }}'
  config_template: |
    - name: {{ .Name }}-4
`

	// Config B:
	// - drop3: matches Name == "mock3" with no template => hard drop (break)
	// - rule2/rule3 exist but must NOT be evaluated if drop3 matches
	configB := `
- id: "rule1"
  match: '{{ eq .Name "mock1" }}'
  config_template: |
    name: {{ .Name }}-1
- id: "drop3"
  match: '{{ eq .Name "mock3" }}'
- id: "rule2"
  match: '{{ or (eq .Name "mock2") (eq .Name "mock3") }}'
  config_template: |
    - name: {{ .Name }}-2
    - name: {{ .Name }}-3
- id: "rule3"
  match: '{{ eq .Name "mock3" }}'
  config_template: |
    - name: {{ .Name }}-4
`

	type tc struct {
		configYAML  string
		target      model.Target
		wantConfigs []confgroup.Config
	}

	tests := map[string]tc{
		"no rules match": {
			configYAML:  configA,
			target:      newMockTarget("nothing"),
			wantConfigs: nil,
		},
		"drop rule hit (no config_template)": {
			configYAML:  configA,
			target:      newMockTarget("dropme"),
			wantConfigs: nil,
		},
		"one rule -> one config": {
			configYAML: configA,
			target:     newMockTarget("mock1"),
			wantConfigs: []confgroup.Config{
				{"name": "mock1-1", "module": "rule1"},
			},
		},
		"one rule -> two configs (YAML list)": {
			configYAML: configA,
			target:     newMockTarget("mock2"),
			wantConfigs: []confgroup.Config{
				{"name": "mock2-2", "module": "rule2"},
				{"name": "mock2-3", "module": "rule2"},
			},
		},
		"multiple rules aggregated (no drop before)": {
			configYAML: configA,
			target:     newMockTarget("mock3"),
			wantConfigs: []confgroup.Config{
				{"name": "mock3-2", "module": "rule2"},
				{"name": "mock3-3", "module": "rule2"},
				{"name": "mock3-4", "module": "rule3"},
			},
		},
		"hard drop stops further evaluation": {
			configYAML:  configB,
			target:      newMockTarget("mock3"),
			wantConfigs: nil, // drop3 matches first => break => rule2/rule3 ignored
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var cfg []ServiceRuleConfig
			err := yaml.Unmarshal([]byte(test.configYAML), &cfg)
			require.NoErrorf(t, err, "yaml unmarshalling of services config")

			svr, err := newServiceEngine(cfg)
			require.NoErrorf(t, err, "service engine creation")

			assert.Equal(t, test.wantConfigs, svr.compose(test.target))
		})
	}
}

func TestServiceEngine_composeHTTPItems(t *testing.T) {
	tests := map[string]struct {
		configYAML  string
		target      model.Target
		wantConfigs []confgroup.Config
	}{
		"full job pass-through preserves module": {
			configYAML: `
- id: "passthrough"
  match: '{{ true }}'
  config_template: |
    {{ .Item | toYaml }}
`,
			target: &itemTarget{Item: map[string]any{
				"module": "nginx",
				"name":   "local",
				"url":    "http://127.0.0.1/stub_status",
			}},
			wantConfigs: []confgroup.Config{
				{"module": "nginx", "name": "local", "url": "http://127.0.0.1/stub_status"},
			},
		},
		"full job pass-through preserves numeric fields": {
			configYAML: `
- id: "passthrough"
  match: '{{ true }}'
  config_template: |
    {{ .Item | toYaml }}
`,
			target: &itemTarget{Item: map[string]any{
				"module": "httpcheck",
				"name":   "api",
				"url":    "http://127.0.0.1/health",
				"port":   float64(80),
			}},
			wantConfigs: []confgroup.Config{
				{"module": "httpcheck", "name": "api", "url": "http://127.0.0.1/health", "port": 80},
			},
		},
		"endpoint object fills module from rule id": {
			configYAML: `
- id: "httpcheck"
  match: '{{ hasKey .Item "url" }}'
  config_template: |
    name: {{ .Item.name }}
    url: {{ .Item.url }}
`,
			target: &itemTarget{Item: map[string]any{
				"name": "api",
				"url":  "http://127.0.0.1/health",
			}},
			wantConfigs: []confgroup.Config{
				{"module": "httpcheck", "name": "api", "url": "http://127.0.0.1/health"},
			},
		},
		"scalar endpoint string uses TUID": {
			configYAML: `
- id: "httpcheck"
  match: '{{ kindIs "string" .Item }}'
  config_template: |
    name: {{ .TUID }}
    url: {{ .Item }}
`,
			target: &itemTarget{Item: "http://127.0.0.1/health", tuid: "http_item_1"},
			wantConfigs: []confgroup.Config{
				{"module": "httpcheck", "name": "http_item_1", "url": "http://127.0.0.1/health"},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var cfg []ServiceRuleConfig
			err := yaml.Unmarshal([]byte(test.configYAML), &cfg)
			require.NoErrorf(t, err, "yaml unmarshalling of services config")

			svr, err := newServiceEngine(cfg)
			require.NoErrorf(t, err, "service engine creation")

			assert.Equal(t, test.wantConfigs, svr.compose(test.target))
		})
	}
}

func TestStockTypesenseRulesPreserveAPIKeyAsString(t *testing.T) {
	tests := map[string]struct {
		file    string
		jobName string
		target  func(string) model.Target
	}{
		"docker": {
			file:    "docker.conf",
			jobName: "docker_typesense",
			target: func(apiKey string) model.Target {
				return &typesenseStockTarget{
					Name:    "typesense",
					Image:   "typesense/typesense:latest",
					Command: "typesense-server --api-key=" + apiKey,
					Address: "127.0.0.1:8108",
				}
			},
		},
		"network listener": {
			file:    "net_listeners.conf",
			jobName: "local",
			target: func(apiKey string) model.Target {
				return &typesenseStockTarget{
					Port:    "8108",
					Comm:    "typesense-server",
					Cmdline: "typesense-server --api-key=" + apiKey,
					Address: "127.0.0.1:8108",
				}
			},
		},
	}

	apiKeys := map[string]string{
		"ordinary":               "ordinary-key",
		"empty":                  "",
		"null":                   "null",
		"boolean":                "true",
		"number":                 "123",
		"flow collection":        "[abc]",
		"flow map":               "{key:value}",
		"quotes and backslashes": `quote"and\\backslash`,
		"secret reference":       "${file:/synthetic/typesense}",
		"escaped reference":      "$${env:VALUE}",
		"newline":                "line1\nline2",
		"trailing newline":       "line1\n",
		"structure injection":    "key\nextra:\n-	${env:VALUE}",
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join("../../../../go.d/config/go.d/sd", test.file)
			bs, err := os.ReadFile(path)
			require.NoError(t, err)

			var cfg Config
			require.NoError(t, yaml.Unmarshal(bs, &cfg))

			var typesenseRule ServiceRuleConfig
			for _, rule := range cfg.Services {
				if rule.ID == "typesense" {
					typesenseRule = rule
					break
				}
			}
			require.Equal(t, "typesense", typesenseRule.ID)

			svr, err := newServiceEngine([]ServiceRuleConfig{typesenseRule})
			require.NoError(t, err)

			for keyName, apiKey := range apiKeys {
				t.Run(keyName, func(t *testing.T) {
					configs := svr.compose(test.target(apiKey))
					require.Len(t, configs, 1)
					assert.Equal(t, []confgroup.Config{{
						"module": "typesense", "name": test.jobName,
						"url": "http://127.0.0.1:8108", "api_key": apiKey,
					}}, configs)
				})
			}
		})
	}
}

type itemTarget struct {
	model.Base
	Item any
	tuid string
}

func (t itemTarget) TUID() string {
	if t.tuid != "" {
		return t.tuid
	}
	return "item"
}

func (t itemTarget) Hash() uint64 { return 1 }

type typesenseStockTarget struct {
	model.Base

	Name    string
	Image   string
	Command string
	Address string
	Port    string
	Comm    string
	Cmdline string
}

func (t *typesenseStockTarget) TUID() string { return "typesense" }
func (t *typesenseStockTarget) Hash() uint64 { return 1 }
