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
		Exclude: []string{" z*", "a*", "z*"},
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
	autogen.Exclude[0] = "caller_mutation"
	selector.Allow[0] = "caller_mutation"
	selector.Deny[0] = "caller_mutation"

	first, err := New(opt)
	require.NoError(t, err)
	assert.True(t, first.state.cfg.autogenOverride.value.Enabled)
	assert.Equal(t, []string{"a*", "z*"}, first.state.cfg.autogenOverride.value.Exclude)
	assert.True(t, first.state.cfg.selector.Matches("allowed_metric", sortedLabelView(nil)))
	assert.False(t, first.state.cfg.selector.Matches("denied_metric", sortedLabelView(nil)))

	first.state.cfg.autogenOverride.value.Exclude[0] = "engine_mutation"
	second, err := New(opt)
	require.NoError(t, err)
	assert.True(t, second.state.cfg.autogenOverride.value.Enabled)
	assert.Equal(t, []string{"a*", "z*"}, second.state.cfg.autogenOverride.value.Exclude)
	assert.True(t, second.state.cfg.selector.Matches("allowed_metric", sortedLabelView(nil)))
	assert.False(t, second.state.cfg.selector.Matches("denied_metric", sortedLabelView(nil)))
}

func TestResolveEffectivePolicyAutogenExclude(t *testing.T) {
	spec, validation, err := charttpl.DecodeYAMLValidated([]byte(`
version: v1
engine:
  autogen:
    enabled: true
    exclude: [" z*", "a*", "z*"]
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

	resolved, err := resolveEffectivePolicy(engineConfig{}, spec.Engine, validation.AutogenExcludeMatcher())
	require.NoError(t, err)
	assert.Equal(t, []string{"a*", "z*"}, resolved.autogen.Exclude)
	assert.True(t, resolved.autogenExclude.MatchString("alpha"))
	assert.False(t, resolved.autogenExclude.MatchString("middle"))
}
