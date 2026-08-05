// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"testing"

	metrixselector "github.com/netdata/netdata/go/plugins/pkg/metrix/selector"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithEnginePolicySnapshotsAtOptionConstruction(t *testing.T) {
	autogen := &AutogenPolicy{
		Enabled: true,
		Rules: []AutogenRule{{
			Scope: "metric_*",
			Selector: metrixselector.Expr{
				Allow: []string{"metric_*"},
				Deny:  []string{"private_*"},
			},
		}},
	}
	selector := &metrixselector.Expr{
		Allow: []string{"allowed_metric"},
		Deny:  []string{"denied_metric"},
	}
	opt := WithEnginePolicy(EnginePolicy{
		Autogen:  autogen,
		Selector: selector,
	})

	autogen.Enabled = false
	autogen.Rules[0].Scope = "caller_mutation"
	autogen.Rules[0].Selector.Allow[0] = "caller_mutation"
	autogen.Rules[0].Selector.Deny[0] = "caller_mutation"
	selector.Allow[0] = "caller_mutation"
	selector.Deny[0] = "caller_mutation"

	first, err := New(opt)
	require.NoError(t, err)
	assert.True(t, first.state.cfg.autogenOverride.value.Enabled)
	assert.Equal(t, "metric_*", first.state.cfg.autogenOverride.value.Rules[0].Scope)
	assert.Equal(t, []string{"metric_*"}, first.state.cfg.autogenOverride.value.Rules[0].Selector.Allow)
	assert.Equal(t, []string{"private_*"}, first.state.cfg.autogenOverride.value.Rules[0].Selector.Deny)
	assert.True(t, first.state.cfg.selector.Matches("allowed_metric", sortedLabelView(nil)))
	assert.False(t, first.state.cfg.selector.Matches("denied_metric", sortedLabelView(nil)))

	first.state.cfg.autogenOverride.value.Rules[0].Scope = "engine_mutation"
	first.state.cfg.autogenOverride.value.Rules[0].Selector.Allow[0] = "engine_mutation"
	first.state.cfg.autogenOverride.value.Rules[0].Selector.Deny[0] = "engine_mutation"
	second, err := New(opt)
	require.NoError(t, err)
	assert.True(t, second.state.cfg.autogenOverride.value.Enabled)
	assert.Equal(t, "metric_*", second.state.cfg.autogenOverride.value.Rules[0].Scope)
	assert.Equal(t, []string{"metric_*"}, second.state.cfg.autogenOverride.value.Rules[0].Selector.Allow)
	assert.Equal(t, []string{"private_*"}, second.state.cfg.autogenOverride.value.Rules[0].Selector.Deny)
	assert.True(t, second.state.cfg.selector.Matches("allowed_metric", sortedLabelView(nil)))
	assert.False(t, second.state.cfg.selector.Matches("denied_metric", sortedLabelView(nil)))
}

func TestWithEnginePolicyAutogenRuleValidation(t *testing.T) {
	tests := map[string]struct {
		rules   []AutogenRule
		wantErr string
	}{
		"nil rules": {},
		"programmatic empty rules": {
			rules: []AutogenRule{},
		},
		"empty scope": {
			rules: []AutogenRule{{
				Selector: metrixselector.Expr{Deny: []string{"metric"}},
			}},
			wantErr: "scope",
		},
		"empty selector": {
			rules: []AutogenRule{{
				Scope: "metric_*",
			}},
			wantErr: "selector",
		},
		"whitespace selector entry": {
			rules: []AutogenRule{{
				Scope:    "metric_*",
				Selector: metrixselector.Expr{Allow: []string{" "}},
			}},
			wantErr: "allow[0]",
		},
		"invalid selector": {
			rules: []AutogenRule{{
				Scope:    "metric_*",
				Selector: metrixselector.Expr{Allow: []string{`metric{label="value",}`}},
			}},
			wantErr: "invalid selector syntax",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := New(WithEnginePolicy(EnginePolicy{Autogen: &AutogenPolicy{
				Enabled: true,
				Rules:   test.rules,
			}}))
			if test.wantErr != "" {
				require.ErrorContains(t, err, test.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestResolveEffectivePolicyAutogenRules(t *testing.T) {
	spec, validation, err := charttpl.DecodeYAMLValidated([]byte(`
version: v1
engine:
  autogen:
    enabled: true
    rules:
      - scope: "metric_*"
        selector:
          deny: ["metric_private_*"]
groups:
  - family: Test
    metrics: [metric]
    charts:
      - title: Test
        context: test
        units: units
        dimensions:
          - selector: metric
            name: metric
`))
	require.NoError(t, err)

	resolved, err := resolveEffectivePolicy(engineConfig{}, spec.Engine, validation.AutogenRules())
	require.NoError(t, err)
	require.Len(t, resolved.autogen.Rules, 1)
	assert.Equal(t, "metric_*", resolved.autogen.Rules[0].Scope)
	assert.False(t, autogenRulesSelect(resolved.autogenRules, "metric_private_total", sortedLabelView(nil)))
	assert.True(t, autogenRulesSelect(resolved.autogenRules, "metric_public_total", sortedLabelView(nil)))
	assert.True(t, autogenRulesSelect(resolved.autogenRules, "other_total", sortedLabelView(nil)))
}
