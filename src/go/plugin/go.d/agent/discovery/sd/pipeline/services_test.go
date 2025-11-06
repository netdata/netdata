// SPDX-License-Identifier: GPL-3.0-or-later

package pipeline

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/sd/model"

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
				{"name": "mock1-1"},
			},
		},
		"one rule -> two configs (YAML list)": {
			configYAML: configA,
			target:     newMockTarget("mock2"),
			wantConfigs: []confgroup.Config{
				{"name": "mock2-2"},
				{"name": "mock2-3"},
			},
		},
		"multiple rules aggregated (no drop before)": {
			configYAML: configA,
			target:     newMockTarget("mock3"),
			wantConfigs: []confgroup.Config{
				{"name": "mock3-2"},
				{"name": "mock3-3"},
				{"name": "mock3-4"},
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
