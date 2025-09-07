// SPDX-License-Identifier: GPL-3.0-or-later

package pipeline

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/discovery/sd/model"

	"github.com/goccy/go-yaml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTargetClassificator_classify(t *testing.T) {
	config := `
- selector: "rule0"
  tags: "skip"
  match:
    - tags: "skip"
      expr: '{{ glob .Name "*" }}'
- selector: "!skip rule1"
  tags: "foo1"
  match:
    - tags: "bar1"
      expr: '{{ glob .Name "mock*1*" }}'
    - tags: "bar2"
      expr: '{{ glob .Name "mock*2*" }}'
- selector: "!skip rule2"
  tags: "foo2"
  match:
    - tags: "bar3"
      expr: '{{ glob .Name "mock*3*" }}'
    - tags: "bar4"
      expr: '{{ glob .Name "mock*4*" }}'
- selector: "rule3"
  tags: "-skip foo3"
  match:
    - tags: "bar5"
      expr: '{{ glob .Name "mock*5*" }}'
    - tags: "bar6"
      expr: '{{ glob .Name "mock*6*" }}'
`
	tests := map[string]struct {
		target   model.Target
		wantTags model.Tags
	}{
		"no rules match": {
			target:   newMockTarget("mock1"),
			wantTags: nil,
		},
		"one rule one match": {
			target:   newMockTarget("mock4", "rule2"),
			wantTags: mustParseTags("foo2 bar4"),
		},
		"one rule two match": {
			target:   newMockTarget("mock56", "rule3"),
			wantTags: mustParseTags("-skip foo3 bar5 bar6"),
		},
		"all rules all matches": {
			target:   newMockTarget("mock123456", "rule1 rule2 rule3"),
			wantTags: mustParseTags("-skip foo1 foo2 foo3 bar1 bar2 bar3 bar4 bar5 bar6"),
		},
		"applying labels after every rule": {
			target:   newMockTarget("mock123456", "rule0 rule1 rule2 rule3"),
			wantTags: mustParseTags("-skip foo3 bar5 bar6"),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var cfg []ClassifyRuleConfig

			err := yaml.Unmarshal([]byte(config), &cfg)
			require.NoError(t, err, "yaml unmarshalling of config")

			clr, err := newTargetClassificator(cfg)
			require.NoError(t, err, "targetClassificator creation")

			assert.Equal(t, test.wantTags, clr.classify(test.target))
		})
	}
}
