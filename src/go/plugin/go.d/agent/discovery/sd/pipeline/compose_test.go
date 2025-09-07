// SPDX-License-Identifier: GPL-3.0-or-later

package pipeline

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/sd/model"

	"github.com/goccy/go-yaml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigComposer_compose(t *testing.T) {
	config := `
- selector: "rule1"
  config:
    - selector: "bar1"
      template: |
        name: {{ .Name }}-1
    - selector: "bar2"
      template: |
        name: {{ .Name }}-2
- selector: "rule2"
  config:
    - selector: "bar3"
      template: |
        name: {{ .Name }}-3
    - selector: "bar4"
      template: |
        name: {{ .Name }}-4
- selector: "rule3"
  config:
    - selector: "bar5"
      template: |
        name: {{ .Name }}-5
    - selector: "bar6"
      template: |
        - name: {{ .Name }}-6
        - name: {{ .Name }}-7
`
	tests := map[string]struct {
		target      model.Target
		wantConfigs []confgroup.Config
	}{
		"no rules matches": {
			target:      newMockTarget("mock"),
			wantConfigs: nil,
		},
		"one rule one config": {
			target: newMockTarget("mock", "rule1 bar1"),
			wantConfigs: []confgroup.Config{
				{"name": "mock-1"},
			},
		},
		"one rule two config": {
			target: newMockTarget("mock", "rule2 bar3 bar4"),
			wantConfigs: []confgroup.Config{
				{"name": "mock-3"},
				{"name": "mock-4"},
			},
		},
		"all rules all configs": {
			target: newMockTarget("mock", "rule1 bar1 bar2 rule2 bar3 bar4 rule3 bar5 bar6"),
			wantConfigs: []confgroup.Config{
				{"name": "mock-1"},
				{"name": "mock-2"},
				{"name": "mock-3"},
				{"name": "mock-4"},
				{"name": "mock-5"},
				{"name": "mock-6"},
				{"name": "mock-7"},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var cfg []ComposeRuleConfig

			err := yaml.Unmarshal([]byte(config), &cfg)
			require.NoErrorf(t, err, "yaml unmarshalling of config")

			cmr, err := newConfigComposer(cfg)
			require.NoErrorf(t, err, "configComposer creation")

			assert.Equal(t, test.wantConfigs, cmr.compose(test.target))
		})
	}
}
