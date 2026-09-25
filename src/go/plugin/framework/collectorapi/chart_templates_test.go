// SPDX-License-Identifier: GPL-3.0-or-later

package collectorapi

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type staticTemplateFixture struct{ calls int }

func (s *staticTemplateFixture) ChartTemplateYAML() string {
	s.calls++
	return "version: v1\ngroups:\n  - family: Empty\n"
}

type dynamicTemplateFixture struct {
	set   *chartengine.TemplateSet
	calls int
}

func (s *dynamicTemplateFixture) ChartTemplateSet() *chartengine.TemplateSet { s.calls++; return s.set }

type bothTemplateFixture struct {
	*staticTemplateFixture
	*dynamicTemplateFixture
}
type policyTemplateFixture struct {
	*dynamicTemplateFixture
	policyCalls int
	policy      chartengine.EnginePolicy
}

func (s *policyTemplateFixture) EnginePolicy() chartengine.EnginePolicy {
	s.policyCalls++
	return s.policy
}

func TestChartTemplateSourceProviderValidation(t *testing.T) {
	for name, collector := range map[string]any{"missing": struct{}{}, "both": &bothTemplateFixture{&staticTemplateFixture{}, &dynamicTemplateFixture{}}} {
		t.Run(name, func(t *testing.T) {
			_, err := NewChartTemplateSource(collector)
			require.ErrorContains(t, err, "exactly one")
		})
	}
	for name, set := range map[string]*chartengine.TemplateSet{"nil": nil, "zero": {}} {
		t.Run(name, func(t *testing.T) {
			source, err := NewChartTemplateSource(&dynamicTemplateFixture{
				set: set,
			})
			require.NoError(t, err)
			_, err = source.Capture()
			require.Error(t, err)
		})
	}
}

func TestChartTemplateSourceCaptureAndFixedPolicy(t *testing.T) {
	static := &staticTemplateFixture{}
	source, err := NewChartTemplateSource(static)
	require.NoError(t, err)
	first, err := source.Capture()
	require.NoError(t, err)
	second, err := source.Capture()
	require.NoError(t, err)
	assert.Same(t, first, second)
	assert.Equal(t, 1, static.calls)
	initial, err := chartengine.NewTemplateSet(chartengine.TemplateSetSpec{})
	require.NoError(t, err)
	collector := &policyTemplateFixture{
		dynamicTemplateFixture: &dynamicTemplateFixture{
			set: initial,
		},
		policy: chartengine.EnginePolicy{
			Autogen: &chartengine.AutogenPolicy{
				Enabled: true,
			},
		},
	}
	source, err = NewChartTemplateSource(collector)
	require.NoError(t, err)
	first, err = source.Capture()
	require.NoError(t, err)
	second, err = source.Capture()
	require.NoError(t, err)
	assert.Same(t, first, second)
	assert.Equal(t, 2, collector.calls)
	assert.Equal(t, 1, collector.policyCalls)
	// The captured override masks later raw policy changes.
	collector.set, err = chartengine.NewTemplateSet(chartengine.TemplateSetSpec{
		Policy: chartengine.EnginePolicy{
			Autogen: &chartengine.AutogenPolicy{
				Enabled: true,
			},
		},
	})
	require.NoError(t, err)
	_, err = source.Capture()
	require.NoError(t, err)
	collector.set, err = chartengine.NewTemplateSet(chartengine.TemplateSetSpec{
		FallbackContextNamespace: "changed",
	})
	require.NoError(t, err)
	_, err = source.Capture()
	require.ErrorContains(t, err, "fixed")
	collector.set = initial
	recovered, err := source.Capture()
	require.NoError(t, err)
	assert.True(t, recovered.SameGlobalPolicy(first))
	assert.Equal(t, 1, collector.policyCalls)
}
